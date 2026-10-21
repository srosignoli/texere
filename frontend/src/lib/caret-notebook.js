// Follows the caret to the node it sits in and that node's Notebook, for the
// note editor's overlay and Process mode's right-hand pane.
//
// Deliberately dependency-free so Node's test runner can load it: the two
// .svelte files that used to do this each had their own copy, and the copies
// had drifted (one ignored a late failure for a node already left, the other
// did not).
//
// - Caret reports are debounced: resting on a line peeks once, scrolling through
//   fifty peeks once at the end.
// - The line resolves to a node from the span map FileNodeSpans returns, and
//   resolves again whenever that map is replaced (a load, a save).
// - A reply, success or failure, for a node the caret has already left loses.
// - The same node is not peeked twice unless forced (a Notebook was created).
//
// Callers differ in three ways, all options:
// - fallbackId: the node to assume when the spans could not be read at all.
// - ownId: a node the caller shows itself, so it is never peeked.
// - shown: while false nothing is peeked; the focus is still tracked.

import { nodeIdAtLine } from './node-span.js'

const NONE = Symbol('none')

// peek(id) resolves to the node's NotebookView, or null when it has none.
// onFocus({ id, notebook }) is called whenever the focused node or its
// Notebook changes; notebook is null while unknown, absent, not shown or ownId.
export function createCaretNotebook({
  peek, onFocus, fallbackId = null, ownId = null, shown = true, delayMs = 150,
  timers = { set: setTimeout, clear: clearTimeout },
}) {
  let spans = []
  let line = 0          // last line resolved, re-used when spans or visibility change
  let reported = 0      // last line reported by caret(), for the same-line guard
  let focusedId = NONE
  let isShown = shown
  let timer = null

  function cancelTimer() {
    if (timer != null) timers.clear(timer)
    timer = null
  }

  async function resolve(at, force = false) {
    line = at
    const id = spans.length ? nodeIdAtLine(spans, at) : fallbackId
    if (id === focusedId && !force) return
    const moved = id !== focusedId
    focusedId = id
    if (!id || id === ownId || !isShown) {
      onFocus({ id, notebook: null })
      return
    }
    if (moved) onFocus({ id, notebook: null })
    try {
      const notebook = await peek(id)
      if (focusedId !== id) return
      onFocus({ id, notebook: notebook ?? null })
    } catch {
      if (focusedId !== id) return
      onFocus({ id, notebook: null })
    }
  }

  return {
    caret(at) {
      if (at === reported) return
      reported = at
      cancelTimer()
      timer = timers.set(() => { timer = null; resolve(at) }, delayMs)
    },
    resolve,
    setSpans(next, at = line) {
      spans = Array.isArray(next) ? next : []
      if (!at) return
      // Resolve now from the line the caller knows is current: a report still
      // waiting on the debounce is about the caret before the reload.
      cancelTimer()
      reported = at
      resolve(at)
    },
    setShown(next) {
      isShown = next
      if (next) {
        if (line) resolve(line, true)
      } else if (focusedId !== NONE) {
        onFocus({ id: focusedId, notebook: null })
      }
    },
    refresh() {
      if (line) resolve(line, true)
    },
    reset() {
      cancelTimer()
      spans = []
      line = 0
      reported = 0
      focusedId = NONE
    },
    destroy: cancelTimer,
  }
}
