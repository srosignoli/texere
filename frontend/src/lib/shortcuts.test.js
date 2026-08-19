import { test } from 'node:test'
import assert from 'node:assert/strict'
import { SHORTCUT_GROUPS } from './shortcuts.js'

test('every group has a title and at least one item', () => {
  assert.ok(SHORTCUT_GROUPS.length > 0, 'no groups at all')
  for (const g of SHORTCUT_GROUPS) {
    assert.ok(typeof g.title === 'string' && g.title.length, 'group without a title')
    assert.ok(Array.isArray(g.items) && g.items.length, `${g.title}: no items`)
  }
})

test('every item has keys, a label, and at least one site', () => {
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      assert.ok(Array.isArray(it.keys) && it.keys.length, `${g.title}: item without keys`)
      assert.ok(typeof it.label === 'string' && it.label.length, `${g.title}: item without a label`)
      // site is an array even when there is one site: Ctrl/Cmd+S has four.
      assert.ok(Array.isArray(it.site), `${g.title}/${it.label}: site must be an array`)
      assert.ok(it.site.length > 0, `${g.title}/${it.label}: no site`)
    }
  }
})

test('no shortcut is listed in two groups', () => {
  const seen = new Map()
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      const combo = it.keys.join('+')
      // Ctrl/Cmd+S legitimately appears twice — save a file, flush notes — so
      // the key is combo+label, not combo alone.
      const k = `${combo}|${it.label}`
      assert.ok(!seen.has(k), `${k} appears in both ${seen.get(k)} and ${g.title}`)
      seen.set(k, g.title)
    }
  }
})
