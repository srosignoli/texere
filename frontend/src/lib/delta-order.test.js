import test from 'node:test'
import assert from 'node:assert/strict'

import { createDeltaOrderer } from './delta-order.js'

// Collects what the orderer released, so a test asserts the reader's text.
function collector() {
  const out = []
  return { out, emit: (t) => out.push(t), text: () => out.join('') }
}

test('releases deltas that arrive in order immediately', () => {
  const c = collector()
  const o = createDeltaOrderer(c.emit)
  o.push(1, 'Let’s work on ')
  o.push(2, 'creating a flash')
  o.push(3, 'card together.')
  assert.equal(c.text(), 'Let’s work on creating a flashcard together.')
  assert.equal(o.held, 0)
})

// The actual bug: two adjacent deltas delivered the wrong way round produced
// "a flash togethercard" in a recorded transcript.
test('holds an early delta until its predecessor lands', () => {
  const c = collector()
  const o = createDeltaOrderer(c.emit)
  o.push(1, 'Let’s work on creating a flash')
  o.push(3, ' together.')
  assert.equal(c.text(), 'Let’s work on creating a flash', 'the overtaking delta must not be released yet')
  assert.equal(o.held, 1)
  o.push(2, 'card')
  assert.equal(c.text(), 'Let’s work on creating a flashcard together.')
  assert.equal(o.held, 0)
})

test('releases a whole run once the gap is filled', () => {
  const c = collector()
  const o = createDeltaOrderer(c.emit)
  o.push(4, 'd')
  o.push(2, 'b')
  o.push(3, 'c')
  assert.equal(c.text(), '')
  assert.equal(o.held, 3)
  o.push(1, 'a')
  assert.equal(c.text(), 'abcd')
})

test('ignores a duplicate delivery of an already released delta', () => {
  const c = collector()
  const o = createDeltaOrderer(c.emit)
  o.push(1, 'once')
  o.push(1, 'once')
  o.push(2, ' only')
  assert.equal(c.text(), 'once only')
})

// An unnumbered payload has nothing to order it by. Buffering it would stall
// every later delta behind a sequence that never arrives.
test('passes an unnumbered delta straight through', () => {
  const c = collector()
  const o = createDeltaOrderer(c.emit)
  o.push(undefined, 'legacy')
  assert.equal(c.text(), 'legacy')
  o.push(1, ' then numbered')
  assert.equal(c.text(), 'legacy then numbered')
})

test('treats a missing text as empty rather than undefined', () => {
  const c = collector()
  const o = createDeltaOrderer(c.emit)
  o.push(1, undefined)
  assert.equal(c.text(), '')
})

test('flush releases what is still held, in sequence order', () => {
  const c = collector()
  const o = createDeltaOrderer(c.emit)
  o.push(3, 'c')
  o.push(2, 'b')
  assert.equal(c.text(), '')
  o.flush()
  assert.equal(c.text(), 'bc')
  assert.equal(o.held, 0)
})

test('reset returns it to the start of a new stream', () => {
  const c = collector()
  const o = createDeltaOrderer(c.emit)
  o.push(1, 'first turn')
  o.reset()
  o.push(1, ' second turn')
  assert.equal(c.text(), 'first turn second turn')
})

// The property that matters, over every ordering of a five-delta stream: the
// text released is always the text sent, in the order it was sent.
test('any delivery order of a stream reads the same', () => {
  const parts = ['The ', 'quick ', 'brown ', 'fox ', 'jumps']
  const permute = (xs) =>
    xs.length <= 1 ? [xs] : xs.flatMap((x, i) => permute([...xs.slice(0, i), ...xs.slice(i + 1)]).map((p) => [x, ...p]))
  for (const order of permute([1, 2, 3, 4, 5])) {
    const c = collector()
    const o = createDeltaOrderer(c.emit)
    for (const seq of order) o.push(seq, parts[seq - 1])
    assert.equal(c.text(), parts.join(''), `delivery order ${order.join(',')}`)
    assert.equal(o.held, 0)
  }
})
