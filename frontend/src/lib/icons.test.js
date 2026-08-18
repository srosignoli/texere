import { test } from 'node:test'
import assert from 'node:assert/strict'
import { ICONS } from './icons.js'

// The nine names the components reference. Spelled out rather than derived from
// ICONS itself: a test that reads its expectations out of the thing under test
// passes no matter what that thing contains.
const EXPECTED = [
  'save', 'search', 'globe', 'book-open', 'books',
  'fuzzy', 'expand', 'collapse', 'celebrate',
]

test('every expected icon exists', () => {
  for (const name of EXPECTED) {
    assert.ok(ICONS[name], `missing icon: ${name}`)
  }
})

test('ICONS holds exactly the expected names', () => {
  assert.deepEqual(Object.keys(ICONS).sort(), [...EXPECTED].sort())
})

test('every icon has a viewBox and at least one non-empty path', () => {
  for (const [name, icon] of Object.entries(ICONS)) {
    assert.match(icon.viewBox, /^0 0 \d+ \d+$/, `${name}: bad viewBox`)
    assert.ok(Array.isArray(icon.paths), `${name}: paths must be an array`)
    assert.ok(icon.paths.length > 0, `${name}: no paths`)
    for (const d of icon.paths) {
      assert.equal(typeof d, 'string', `${name}: path must be a string`)
      assert.ok(d.trim().length > 0, `${name}: empty path`)
      // A path that does not start with a move command is malformed, and the
      // browser draws nothing rather than complaining.
      assert.match(d.trim(), /^[Mm]/, `${name}: path does not start with a move`)
    }
  }
})
