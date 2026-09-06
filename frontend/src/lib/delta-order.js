// Releases numbered stream deltas in order.
//
// Deliberately dependency-free, for the reason llm-prompt.js records: its
// caller llm.js imports the Wails runtime from ../../wailsjs/runtime/runtime,
// which is gitignored and absent on a fresh clone, so nothing that lives there
// can be loaded by Node's test runner. The ordering rule is the part of the
// fix that is worth proving, so it lives here where a test can reach it.
//
// Why this exists at all: on Linux, runtime.EventsEmit ends in
// invokeOnMainThread, which runs a call immediately when the calling goroutine
// happens to be on the GTK main thread and queues it otherwise. A goroutine is
// not pinned to a thread, so an emit that lands on the main thread overtakes
// emits already queued. In a token-per-event stream that reached the
// transcript as transposed words — "a flash togethercard", "phr theasing
// question side". The Go side now numbers every flush; this puts them back.

// createDeltaOrderer returns a sink that calls `emit` with delta text strictly
// in sequence order, holding anything that arrives early until its predecessor
// lands. Sequences start at 1, matching the Go side's pre-incremented counter.
export function createDeltaOrderer(emit) {
  let expected = 1
  const pending = new Map()

  return {
    // push accepts one delta. A seq that is not a number is passed straight
    // through rather than buffered: an unnumbered payload has nothing to order
    // it by, and holding it would stall every later delta behind it forever.
    push(seq, text) {
      if (typeof seq !== 'number' || !Number.isFinite(seq)) {
        emit(text ?? '')
        return
      }
      // A repeat of something already released is a duplicate delivery, not new
      // text; releasing it again would double a word in the preview.
      if (seq < expected) return
      pending.set(seq, text ?? '')
      while (pending.has(expected)) {
        const next = pending.get(expected)
        pending.delete(expected)
        expected += 1
        emit(next)
      }
    },

    // Anything still held back, in sequence order. Used when a stream ends with
    // a gap that will never be filled — the text is stale by then, but dropping
    // it silently is worse than showing it slightly out of place.
    flush() {
      for (const seq of [...pending.keys()].sort((a, b) => a - b)) {
        emit(pending.get(seq))
      }
      pending.clear()
    },

    reset() {
      expected = 1
      pending.clear()
    },

    // For tests and diagnostics: how many deltas are waiting on a predecessor.
    get held() {
      return pending.size
    },
  }
}
