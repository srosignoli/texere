package shell

import (
	"sync"
	"unicode/utf8"
)

// runeSafeCut returns how many of s's first n bytes can be taken without
// cutting a multi-byte character in half — n itself when the cut already lands
// on a rune boundary, less when it does not.
//
// It shares partialRuneTail with run.go's streamWriter rather than
// reimplementing the scan, and only ever inspects the last utf8.UTFMax bytes of
// the prefix, so a 64KiB cut does not copy 64KiB to find out where it lands.
//
// It can return 0, meaning "nothing can be taken without splitting" — when the
// room available is smaller than the rune that would go into it, or when the
// whole prefix is one incomplete rune. Callers decide what that means: topping
// up a chunk gives up and starts a new one, while emitting a fresh chunk has to
// take something, so it splits rather than loop forever or drop bytes.
func runeSafeCut(s string, n int) int {
	if n > len(s) {
		n = len(s)
	}
	if n <= 0 {
		return 0
	}
	start := n - utf8.UTFMax
	if start < 0 {
		start = 0
	}
	back := partialRuneTail([]byte(s[start:n]))
	if back > n {
		back = n
	}
	return n - back
}

// Batcher accumulates output chunks so a caller can emit them periodically
// instead of once per read. `find /` produces hundreds of thousands of lines,
// and one Wails event each would flood the bridge on its own.
//
// It deliberately holds NO timer: the ticker belongs to the caller, which keeps
// these tests deterministic rather than time-dependent.
type Batcher struct {
	mu    sync.Mutex
	buf   []Chunk
	maxCh int
}

// NewBatcher returns a Batcher whose individual chunks never exceed max bytes,
// so one Take cannot hand over a single enormous string.
func NewBatcher(max int) *Batcher {
	if max <= 0 {
		max = 64 << 10
	}
	return &Batcher{maxCh: max}
}

// Add appends text, concatenating onto the tail when it is the same stream and
// still under the cap. Chunks are never merged across streams — the UI tints
// stderr, so the boundary carries meaning.
//
// Both cut points are rune-safe. The cap used to slice on raw byte offsets,
// which cut multi-byte characters in half exactly as the read boundary in
// run.go once did — 8x rarer at 64KiB than at 8KiB, but not gone, and each
// chunk is JSON-marshalled separately for its Wails event, so the two halves
// become independent U+FFFD replacement characters that the frontend can never
// rejoin. A promoted line then carries that corruption into the user's .n4l
// permanently.
//
// Unlike streamWriter, Batcher holds NOTHING back between calls. It never needs
// to: its input already arrives rune-aligned from streamWriter, so the only job
// here is to avoid CREATING a split, which means moving the cut point — the
// bytes backed away from go into the very next chunk, within this same call.
// So there is no pending state to decide about at Take() time, and nothing that
// can be stranded if the run ends mid-batch: every byte handed to Add is in the
// buffer before Add returns. That also keeps Take() emptying the buffer
// completely, as its test asserts.
func (b *Batcher) Add(c Chunk) {
	if c.Text == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	text := c.Text
	if n := len(b.buf); n > 0 {
		tail := &b.buf[n-1]
		if tail.Stream == c.Stream && len(tail.Text) < b.maxCh {
			// A cut of 0 means the next rune does not fit in the room left;
			// leave the tail short and start a new chunk rather than split it.
			if room := runeSafeCut(text, b.maxCh-len(tail.Text)); room > 0 {
				tail.Text += text[:room]
				text = text[room:]
			}
		}
	}
	// Anything that did not fit becomes further chunks, so capping splits
	// rather than drops.
	for len(text) > 0 {
		n := runeSafeCut(text, b.maxCh)
		if n == 0 {
			// Nothing can be taken whole: either maxCh is smaller than this
			// single rune, or these bytes are a genuinely truncated tail that
			// no later byte will complete (streamWriter.flush emits one when a
			// stream ends mid-rune). Take the raw prefix — splitting is bad,
			// dropping bytes or spinning here would be worse.
			if n = len(text); n > b.maxCh {
				n = b.maxCh
			}
		}
		b.buf = append(b.buf, Chunk{Stream: c.Stream, Text: text[:n]})
		text = text[n:]
	}
}

// Take returns everything buffered and empties the buffer.
func (b *Batcher) Take() []Chunk {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.buf) == 0 {
		return nil
	}
	out := b.buf
	b.buf = nil
	return out
}
