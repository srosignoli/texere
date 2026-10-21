import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createSwitcher } from './study-switch.js'

function setup() {
  const shown = []
  let current = 'A'
  let flushes = []
  const switchTo = createSwitcher({
    currentKey: () => current,
    flush: () => new Promise((resolve) => flushes.push(resolve)),
    show: (key, remount) => { shown.push({ key, remount }); current = key },
  })
  return { shown, switchTo, resolveFlush: (ok = true) => flushes.shift()(ok), flushCount: () => flushes.length }
}

test('switching to another document flushes first, then shows it', async () => {
  const s = setup()
  const p = s.switchTo('B')
  s.resolveFlush()
  assert.equal(await p, true)
  assert.deepEqual(s.shown, [{ key: 'B', remount: true }])
})

test('clicking back to the open document inside the flush remounts it: its Sitting has ended', async () => {
  const s = setup()
  const toB = s.switchTo('B')
  const backToA = s.switchTo('A')
  s.resolveFlush()
  await Promise.all([toB, backToA])
  assert.deepEqual(s.shown, [{ key: 'A', remount: true }])
})

test('the last click wins', async () => {
  const s = setup()
  const toB = s.switchTo('B')
  const toC = s.switchTo('C')
  s.resolveFlush()
  await Promise.all([toB, toC])
  assert.deepEqual(s.shown, [{ key: 'C', remount: true }])
})

test('a flush that fails (notes not saved) stops the switch', async () => {
  const s = setup()
  const p = s.switchTo('B')
  s.resolveFlush(false)
  assert.equal(await p, false)
  assert.deepEqual(s.shown, [])

  // The Sitting is still open, so a click on the same document shows it unchanged.
  assert.equal(await s.switchTo('A'), true)
  assert.deepEqual(s.shown, [{ key: 'A', remount: false }])
})
