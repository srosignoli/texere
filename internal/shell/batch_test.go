package shell

import (
	"strings"
	"sync"
	"testing"
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
