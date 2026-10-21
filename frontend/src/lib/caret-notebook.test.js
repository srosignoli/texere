import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createCaretNotebook } from './caret-notebook.js'

const SPANS = [
  { id: 'a', fromLine: 1, toLine: 3 },
  { id: 'b', fromLine: 5, toLine: 7 },
]

// A peek whose replies the test releases by hand, in any order.
function peeker() {
  const calls = []
  const peek = (id) => new Promise((resolve, reject) => calls.push({ id, resolve, reject }))
  return { calls, peek }
}

// Timers the test fires by hand.
function timers() {
  let pending = null
  return {
    set: (fn) => { pending = fn; return 1 },
    clear: () => { pending = null },
    fire: () => { const fn = pending; pending = null; fn?.() },
    get pending() { return pending != null },
  }
}

function setup(opts = {}) {
  const p = peeker()
  const t = timers()
  const focus = []
  const cn = createCaretNotebook({ peek: p.peek, onFocus: (f) => focus.push(f), timers: t, ...opts })
  const last = () => focus[focus.length - 1]
  return { cn, p, t, focus, last }
}

const tick = () => new Promise((r) => setImmediate(r))

test('resolves the node under the caret and peeks its notebook', async () => {
  const s = setup()
  s.cn.setSpans(SPANS)
  s.cn.resolve(2)
  assert.deepEqual(s.p.calls.map((c) => c.id), ['a'])
  s.p.calls[0].resolve({ notebookId: 'nb-a' })
  await tick()
  assert.deepEqual(s.last(), { id: 'a', notebook: { notebookId: 'nb-a' } })
})

test('a late reply for a node already left is ignored', async () => {
  const s = setup()
  s.cn.setSpans(SPANS)
  s.cn.resolve(2)
  s.cn.resolve(6)
  s.p.calls[1].resolve({ notebookId: 'nb-b' })
  await tick()
  s.p.calls[0].resolve({ notebookId: 'nb-a' })
  await tick()
  assert.deepEqual(s.last(), { id: 'b', notebook: { notebookId: 'nb-b' } })
})

test('a late failure for a node already left does not clear the current notebook', async () => {
  const s = setup()
  s.cn.setSpans(SPANS)
  s.cn.resolve(2)
  s.cn.resolve(6)
  s.p.calls[1].resolve({ notebookId: 'nb-b' })
  await tick()
  s.p.calls[0].reject(new Error('slow and failed'))
  await tick()
  assert.deepEqual(s.last(), { id: 'b', notebook: { notebookId: 'nb-b' } })
})

test('empty spans fall back to fallbackId', async () => {
  const s = setup({ fallbackId: 'x' })
  s.cn.setSpans([])
  s.cn.resolve(5)
  assert.equal(s.last().id, 'x')
  assert.deepEqual(s.p.calls.map((c) => c.id), ['x'])
})

test('empty spans without a fallback focus nothing and peek nothing', async () => {
  const s = setup()
  s.cn.setSpans([])
  s.cn.resolve(5)
  assert.deepEqual(s.last(), { id: null, notebook: null })
  assert.equal(s.p.calls.length, 0)
})

test('ownId is focused but never peeked', async () => {
  const s = setup({ fallbackId: 'x', ownId: 'x' })
  s.cn.setSpans([])
  s.cn.resolve(1)
  assert.deepEqual(s.last(), { id: 'x', notebook: null })
  assert.equal(s.p.calls.length, 0)
})

test('nothing is peeked until shown', async () => {
  const s = setup({ shown: false })
  s.cn.setSpans(SPANS)
  s.cn.resolve(2)
  assert.deepEqual(s.last(), { id: 'a', notebook: null })
  assert.equal(s.p.calls.length, 0)
  s.cn.setShown(true)
  assert.deepEqual(s.p.calls.map((c) => c.id), ['a'])
})

test('the same node is not re-peeked unless forced', async () => {
  const s = setup()
  s.cn.setSpans(SPANS)
  s.cn.resolve(1)
  s.cn.resolve(3)
  assert.equal(s.p.calls.length, 1)
  s.cn.refresh()
  assert.equal(s.p.calls.length, 2)
})

test('caret reports are debounced, and the last line wins', async () => {
  const s = setup()
  s.cn.setSpans(SPANS)
  s.cn.caret(1)
  s.cn.caret(2)
  s.cn.caret(6)
  assert.equal(s.p.calls.length, 0, 'nothing resolves before the delay')
  s.t.fire()
  assert.deepEqual(s.p.calls.map((c) => c.id), ['b'])
})

test('setSpans re-resolves at the last caret line', async () => {
  const s = setup()
  s.cn.caret(2)
  s.t.fire()
  assert.equal(s.last().id, null)
  s.cn.setSpans(SPANS)
  assert.equal(s.last().id, 'a')
})

test('reset forgets the file', async () => {
  const s = setup()
  s.cn.setSpans(SPANS)
  s.cn.caret(2)
  s.cn.reset()
  assert.equal(s.t.pending, false, 'a pending caret report must not fire into the next file')
  s.cn.refresh()
  assert.equal(s.p.calls.length, 0)
})

// A save or Push in Process mode: the caret moved (a report is waiting on the
// debounce) and the reloaded spans arrive before the timer fires. The pane
// must land on the caret's line, not the last line it resolved.
test('setSpans without a line resolves at the latest reported caret line', async () => {
  const s = setup()
  s.cn.setSpans(SPANS)
  s.cn.caret(2)
  s.t.fire()
  assert.equal(s.last().id, 'a')
  s.cn.caret(6)                 // still pending
  s.cn.setSpans(SPANS)          // the reload lands first
  assert.equal(s.last().id, 'b')
})

// Browsers refuse a setTimeout called with any `this` but the window
// ("Can only call Window.setTimeout on instances of Window"); Node does not, so
// the default timers have to be checked against a setTimeout that behaves like
// the webview's.
test('the default timers call setTimeout the way a browser allows', async () => {
  const real = { set: globalThis.setTimeout, clear: globalThis.clearTimeout }
  function strict(fn) {
    return function (...args) {
      if (this !== undefined && this !== globalThis) throw new TypeError('Can only call Window.setTimeout on instances of Window')
      return fn(...args)
    }
  }
  globalThis.setTimeout = strict(real.set)
  globalThis.clearTimeout = strict(real.clear)
  try {
    const p = peeker()
    const cn = createCaretNotebook({ peek: p.peek, onFocus: () => {}, delayMs: 0 })
    cn.setSpans(SPANS)
    cn.caret(2)
    cn.caret(6)
    cn.destroy()
  } finally {
    globalThis.setTimeout = real.set
    globalThis.clearTimeout = real.clear
  }
})
