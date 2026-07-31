package shell

import (
	"context"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestBatcherTakeEmptiesTheBuffer(t *testing.T) {
	b := NewBatcher(1024)
	b.Add(Chunk{Stream: "stdout", Text: "a"})
	if got := b.Take(); len(got) != 1 || got[0].Text != "a" {
		t.Fatalf("got %+v", got)
	}
	if got := b.Take(); len(got) != 0 {
		t.Fatalf("second Take should be empty, got %+v", got)
	}
}

// The point of batching: many adds collapse into few chunks.
func TestBatcherCoalescesSameStream(t *testing.T) {
	b := NewBatcher(1024)
	for i := 0; i < 500; i++ {
		b.Add(Chunk{Stream: "stdout", Text: "x"})
	}
	got := b.Take()
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1", len(got))
	}
	if len(got[0].Text) != 500 {
		t.Fatalf("got %d bytes, want 500", len(got[0].Text))
	}
}

// Coalescing must never merge across streams, or stderr tinting breaks.
func TestBatcherKeepsStreamsSeparateAndOrdered(t *testing.T) {
	b := NewBatcher(1024)
	b.Add(Chunk{Stream: "stdout", Text: "a"})
	b.Add(Chunk{Stream: "stderr", Text: "b"})
	b.Add(Chunk{Stream: "stdout", Text: "c"})
	got := b.Take()
	if len(got) != 3 {
		t.Fatalf("got %d chunks, want 3", len(got))
	}
	if got[0].Stream != "stdout" || got[1].Stream != "stderr" || got[2].Stream != "stdout" {
		t.Fatalf("streams out of order: %+v", got)
	}
}

// A single chunk must stay bounded so one Take cannot hand over 200MB.
func TestBatcherCapsChunkSize(t *testing.T) {
	b := NewBatcher(8)
	b.Add(Chunk{Stream: "stdout", Text: strings.Repeat("y", 40)})
	total := 0
	for _, c := range b.Take() {
		if len(c.Text) > 8 {
			t.Fatalf("chunk of %d bytes exceeds the 8-byte cap", len(c.Text))
		}
		total += len(c.Text)
	}
	if total != 40 {
		t.Fatalf("got %d bytes total, want 40 — capping must split, not drop", total)
	}
}

// chunkBytes checks every chunk is independently valid UTF-8 and within the cap,
// and returns the bytes and text they carry between them.
func chunkBytes(t *testing.T, got []Chunk, cap int) (int, string) {
	t.Helper()
	total := 0
	var joined strings.Builder
	for i, c := range got {
		// Validity PER CHUNK is the whole point: each chunk is JSON-marshalled
		// on its own for the Wails event, and json.Marshal turns an invalid
		// trailing byte into U+FFFD. The halves of a split rune are destroyed
		// separately, so the frontend can never rejoin them — concatenating
		// here before checking would hide exactly the bug being tested.
		if !utf8.ValidString(c.Text) {
			t.Errorf("chunk %d (%d bytes, %q) is not valid UTF-8", i, len(c.Text), c.Text)
		}
		if len(c.Text) > cap {
			t.Errorf("chunk %d of %d bytes exceeds the %d-byte cap", i, len(c.Text), cap)
		}
		total += len(c.Text)
		joined.WriteString(c.Text)
	}
	return total, joined.String()
}

// The chunk cap slices on raw byte offsets, so it can cut a multi-byte
// character in half — the same defect fixed at the read boundary in run.go,
// reappearing 8x more rarely at the 64KiB cap. Rarer is not gone, and a
// promoted line carries the corruption into the user's .n4l permanently.
func TestBatcherDoesNotSplitRunesAtTheCap(t *testing.T) {
	b := NewBatcher(8)
	// 7 ASCII bytes then a 3-byte rune: a naive 8-byte cut takes the arrow's
	// first byte and leaves the other two for the next chunk.
	text := strings.Repeat("x", 7) + "→" + strings.Repeat("y", 7)
	b.Add(Chunk{Stream: "stdout", Text: text})

	total, joined := chunkBytes(t, b.Take(), 8)
	if total != len(text) {
		t.Fatalf("got %d bytes, want %d — capping must split, not drop", total, len(text))
	}
	if joined != text {
		t.Fatalf("bytes did not survive:\n got %q\nwant %q", joined, text)
	}
}

// The other split point: topping up the previous chunk up to the cap.
func TestBatcherDoesNotSplitARuneWhenToppingUpTheTail(t *testing.T) {
	b := NewBatcher(8)
	b.Add(Chunk{Stream: "stdout", Text: strings.Repeat("x", 7)}) // one byte of room
	b.Add(Chunk{Stream: "stdout", Text: "→z"})                   // does not fit in it

	want := strings.Repeat("x", 7) + "→z"
	total, joined := chunkBytes(t, b.Take(), 8)
	if total != len(want) || joined != want {
		t.Fatalf("got %d bytes %q, want %d bytes %q", total, joined, len(want), want)
	}
}

// Realistic sizes, not just the tiny caps above.
func TestBatcherDoesNotSplitRunesAtTheRealCap(t *testing.T) {
	const max = 64 << 10
	b := NewBatcher(max)
	// Put the arrow astride the cap: 64KiB-1 of filler means the cut would
	// otherwise fall inside it.
	text := strings.Repeat("x", max-1) + "→" + strings.Repeat("y", 100)
	b.Add(Chunk{Stream: "stdout", Text: text})

	total, joined := chunkBytes(t, b.Take(), max)
	if total != len(text) || joined != text {
		t.Fatalf("got %d bytes, want %d, and the text must round-trip", total, len(text))
	}
}

// A tail that is genuinely truncated — run.go's streamWriter.flush emits one
// when a stream ends mid-rune — cannot be completed by anything, so it must be
// passed through rather than held or dropped.
func TestBatcherKeepsAGenuinelyTruncatedTail(t *testing.T) {
	b := NewBatcher(4)
	b.Add(Chunk{Stream: "stdout", Text: "ab\xe2\x86"})

	total := 0
	var joined strings.Builder
	for _, c := range b.Take() {
		total += len(c.Text)
		joined.WriteString(c.Text)
	}
	if total != 4 || joined.String() != "ab\xe2\x86" {
		t.Fatalf("got %d bytes %q, want the truncated tail passed through intact",
			total, joined.String())
	}
}

// A cap smaller than a single rune cannot be honoured AND kept whole. Splitting
// is the right answer — dropping bytes never is — and it must terminate.
func TestBatcherWithACapSmallerThanARuneStillSplitsRatherThanDrops(t *testing.T) {
	b := NewBatcher(2)
	b.Add(Chunk{Stream: "stdout", Text: "→"})

	total := 0
	var joined strings.Builder
	for _, c := range b.Take() {
		total += len(c.Text)
		joined.WriteString(c.Text)
	}
	if total != 3 || joined.String() != "→" {
		t.Fatalf("got %d bytes %q, want all 3 bytes of the rune", total, joined.String())
	}
}

// The production wiring, end to end: shell_methods.go passes batcher.Add
// straight to Run as the sink, so a rune has to survive BOTH the copy boundary
// in run.go and the chunk cap here. Testing the two halves separately is what
// let this defect sit downstream of a fix that was believed to close it.
func TestRunThroughBatcherKeepsRunesIntact(t *testing.T) {
	sh := shTest(t)
	// Small cap so the batcher actually splits; the command emits more than
	// exec's 32KiB copy buffer in one write, with the arrow astride it.
	b := NewBatcher(4096)
	const cmd = `a=$(printf 'x%.0s' $(seq 1 32766)); b=$(printf 'y%.0s' $(seq 1 4000)); ` +
		`printf '%s\342\206\222%s' "$a" "$b"`
	if _, err := Run(context.Background(), sh, "", cmd, b.Add); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := b.Take()
	total, joined := chunkBytes(t, got, 4096)
	if want := 32766 + 3 + 4000; total != want {
		t.Fatalf("got %d bytes, want %d", total, want)
	}
	if strings.Count(joined, "→") != 1 {
		t.Fatalf("got %d arrows, want 1", strings.Count(joined, "→"))
	}
	if strings.ContainsRune(joined, utf8.RuneError) {
		t.Fatal("output contains a replacement character")
	}
}

func TestBatcherIsSafeForConcurrentAdds(t *testing.T) {
	b := NewBatcher(1024)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				b.Add(Chunk{Stream: "stdout", Text: "z"})
			}
		}()
	}
	wg.Wait()
	total := 0
	for _, c := range b.Take() {
		total += len(c.Text)
	}
	if total != 800 {
		t.Fatalf("got %d bytes, want 800", total)
	}
}
