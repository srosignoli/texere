import { test } from 'node:test'
import assert from 'node:assert/strict'
import { THEMES, AVAILABLE_THEMES, isValidTheme, normalizeTheme } from './themes.js'

test('THEMES carries the three themes in display order', () => {
  assert.deepEqual(THEMES.map((t) => t.id), ['blue', 'dark', 'light'])
  assert.deepEqual(THEMES.map((t) => t.label), ['Blue', 'Dark', 'Light'])
})

test('isValidTheme accepts exactly the three ids', () => {
  for (const id of ['blue', 'dark', 'light']) assert.equal(isValidTheme(id), true, id)
  for (const v of ['Blue', 'solarized', '', null, undefined, 0, {}]) {
    assert.equal(isValidTheme(v), false, String(v))
  }
})

test('normalizeTheme passes valid ids through', () => {
  for (const id of ['blue', 'dark', 'light']) assert.equal(normalizeTheme(id), id)
})

// Empty is the first-run and upgraded-config path from Go; everything else
// unrecognised must land on the default rather than reaching the DOM.
test('normalizeTheme resolves anything unrecognised to blue', () => {
  for (const v of ['', 'Light', 'solarized', null, undefined, 42, {}]) {
    assert.equal(normalizeTheme(v), 'blue', String(v))
  }
})

// Blue is finished; Dark and Light are not. This flag is what the picker
// gates on — it must not be read as validity.
test('only blue is marked available', () => {
  assert.deepEqual(
    THEMES.map((t) => [t.id, t.available]),
    [
      ['blue', true],
      ['dark', false],
      ['light', false],
    ]
  )
})

// AVAILABLE_THEMES is what the picker renders — it must carry only what
// `available` allows, in the same order as THEMES.
test('AVAILABLE_THEMES contains only blue', () => {
  assert.deepEqual(AVAILABLE_THEMES.map((t) => t.id), ['blue'])
})

// The gate is UI-only: a stored 'dark' or 'light' — e.g. from a hand-edited
// config.json — must still be treated as a real, valid theme and must still
// resolve to itself, not fall back to blue just because it isn't offered.
test('unavailable themes are still valid and still normalize to themselves', () => {
  for (const id of ['dark', 'light']) {
    assert.equal(isValidTheme(id), true, id)
    assert.equal(normalizeTheme(id), id, id)
  }
})
