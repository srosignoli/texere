import { test } from 'node:test'
import assert from 'node:assert/strict'
import { barHeights, sparkPath, humanDuration, fiBand } from './stats-format.js'

test('bars scale so the tallest fills the height', () => {
  assert.deepEqual(barHeights([1, 2, 4], 100), [25, 50, 100])
})

// An empty collection must render flat bars, not a chart full of NaN paths.
test('an all-zero series gives zero-height bars, not NaN', () => {
  assert.deepEqual(barHeights([0, 0, 0], 100), [0, 0, 0])
  assert.deepEqual(barHeights([], 100), [])
})

test('a single point still scales', () => {
  assert.deepEqual(barHeights([7], 50), [50])
})

test('sparkPath spans the full width and starts with a move', () => {
  const d = sparkPath([0, 10, 5], 100, 20)
  assert.match(d, /^M0[ ,]/)
  assert.ok(d.includes('100'), 'the last point must reach the right edge')
  assert.ok(!d.includes('NaN'), 'a NaN in a path silently renders nothing')
})

test('sparkPath needs at least two points', () => {
  assert.equal(sparkPath([5], 100, 20), '')
  assert.equal(sparkPath([], 100, 20), '')
})

test('humanDuration reads naturally at each scale', () => {
  assert.equal(humanDuration(0), '0m')
  assert.equal(humanDuration(45), '45m')
  assert.equal(humanDuration(125), '2h 5m')
  assert.equal(humanDuration(120), '2h')
})

// The band is the whole point of showing a forgetting index — a bare number
// does not tell you whether it is healthy.
test('fiBand classifies against the 7-13% target', () => {
  assert.equal(fiBand(-1), 'unknown')
  assert.equal(fiBand(4), 'low')
  assert.equal(fiBand(7), 'ok')
  assert.equal(fiBand(10), 'ok')
  assert.equal(fiBand(13), 'ok')
  // 14 is the first value outside the band. Without it, moving the boundary to
  // >14 leaves both 13 and 20 on the side they were already on.
  assert.equal(fiBand(14), 'high')
  assert.equal(fiBand(20), 'high')
})
