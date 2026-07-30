package shell

import "sync"

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
			room := b.maxCh - len(tail.Text)
			if room > len(text) {
				room = len(text)
			}
			tail.Text += text[:room]
			text = text[room:]
		}
	}
	// Anything that did not fit becomes further chunks, so capping splits
	// rather than drops.
	for len(text) > 0 {
		n := len(text)
		if n > b.maxCh {
			n = b.maxCh
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
