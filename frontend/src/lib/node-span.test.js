import { test } from 'node:test'
import assert from 'node:assert/strict'
import { nodeIdAtLine } from './node-span.js'

// Mirrors what FileNodeSpans returns for:
//   3: # @id one     6: # @id two
//   4: Alpha         7: Beta
//   5: "  (means) x  8: "  (means) y
const spans = [
  { id: 'one', fromLine: 3, toLine: 5 },
  { id: 'two', fromLine: 6, toLine: 8 },
]

test('a line on the node itself resolves to that node', () => {
  assert.equal(nodeIdAtLine(spans, 4), 'one')
  assert.equal(nodeIdAtLine(spans, 7), 'two')
})

// The "# @id" line labels the node, so a caret there is inside it.
test('the id marker line resolves to the node it labels', () => {
  assert.equal(nodeIdAtLine(spans, 3), 'one')
  assert.equal(nodeIdAtLine(spans, 6), 'two')
})

// Dittos are continuation lines of the same node.
test('a ditto line resolves to its node', () => {
  assert.equal(nodeIdAtLine(spans, 5), 'one')
  assert.equal(nodeIdAtLine(spans, 8), 'two')
})

test('a line before the first node resolves to nothing', () => {
  assert.equal(nodeIdAtLine(spans, 1), null)
  assert.equal(nodeIdAtLine(spans, 2), null)
})

// Reachable in normal use: a blank line between two nodes.
test('a line between nodes resolves to nothing', () => {
  const gapped = [
    { id: 'one', fromLine: 3, toLine: 4 },
    { id: 'two', fromLine: 7, toLine: 8 },
  ]
  assert.equal(nodeIdAtLine(gapped, 5), null)
  assert.equal(nodeIdAtLine(gapped, 6), null)
})

// The span map goes stale while typing; an edit that shortens the file can leave
// the caret past every span. That must be quiet, not a crash.
test('a line past the end resolves to nothing', () => {
  assert.equal(nodeIdAtLine(spans, 99), null)
})

test('an empty or malformed span list resolves to nothing', () => {
  assert.equal(nodeIdAtLine([], 4), null)
  assert.equal(nodeIdAtLine(null, 4), null)
  assert.equal(nodeIdAtLine(undefined, 4), null)
})

test('a single-line node resolves on its only line', () => {
  assert.equal(nodeIdAtLine([{ id: 'solo', fromLine: 10, toLine: 10 }], 10), 'solo')
  assert.equal(nodeIdAtLine([{ id: 'solo', fromLine: 10, toLine: 10 }], 11), null)
})
