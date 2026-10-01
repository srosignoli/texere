import { test } from 'node:test'
import assert from 'node:assert/strict'
import { concatScript, clipTime, maxGap, assertEpochSeconds } from './screencast.mjs'

const F = (file, ts) => ({ file, ts })

test('concatScript holds each frame until the next and repeats the last', () => {
  const s = concatScript([F('/a.jpg', 100), F('/b.jpg', 100.5)], 101.25)
  assert.equal(
    s,
    "ffconcat version 1.0\nfile '/a.jpg'\nduration 0.500000\nfile '/b.jpg'\nduration 0.750000\nfile '/b.jpg'\n",
  )
})

test('concatScript never writes a zero duration', () => {
  const s = concatScript([F('/a.jpg', 5), F('/b.jpg', 5)], 5)
  assert.ok(!/duration 0\.000000/.test(s), s)
})

test('concatScript refuses an empty capture', () => {
  assert.throws(() => concatScript([], 1), /no frames/)
})

test('clipTime is seconds after the first frame, never negative', () => {
  assert.equal(clipTime(101_500, 100), 1.5)
  assert.equal(clipTime(99_000, 100), 0)
})

test('maxGap reports the longest stall inside a window', () => {
  const fr = [F('a', 0), F('b', 0.1), F('c', 0.9), F('d', 1.0)]
  assert.ok(Math.abs(maxGap(fr) - 0.8) < 1e-9)
  assert.ok(Math.abs(maxGap(fr, 0.85, 1.0) - 0.1) < 1e-9)
})

test('assertEpochSeconds accepts wall-clock seconds and rejects anything else', () => {
  const now = 1_790_000_000_000
  assert.doesNotThrow(() => assertEpochSeconds(now / 1000 - 2, now))
  assert.throws(() => assertEpochSeconds(12.5, now), /not epoch seconds/)        // monotonic clock
  assert.throws(() => assertEpochSeconds(now, now), /not epoch seconds/)          // milliseconds
})
