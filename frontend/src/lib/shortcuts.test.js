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

import { readdirSync, readFileSync } from 'node:fs'

const SRC = new URL('../', import.meta.url)

// DOM handlers: `if ((e.ctrlKey || e.metaKey) && e.key === 'k')`.
const KEY = /\bkey\s*===\s*'([^']+)'/
// The leading `!` capture matters: OmniBar has `&& !e.shiftKey && e.key === 'k'`,
// and reading that negation as a REQUIRED modifier turns Ctrl+K into
// Ctrl+Shift+K — which is a different shortcut ten lines below it.
const MOD = /(!?)\s*\w+\.(ctrlKey|metaKey|shiftKey|altKey)\b/g
// CodeMirror keymaps: `{ key: 'Mod-s', run: … }`. Requiring `run:` on the same
// line is what rejects ordinary object keys — ExplorerLegend.svelte has
// `key: 'contains'`, `key: 'leadsto'` and four more that are data, not bindings.
const CM = /\bkey:\s*'([^']+)'\s*,\s*run:/g

// Source writes `(e.key === 'e' || e.key === 'E')` and CodeMirror writes 'Mod-s';
// one canonical case so the two syntaxes agree that Ctrl/Cmd+S is one shortcut.
const canonKey = (k) => (k.length === 1 ? k.toUpperCase() : k)

function comboFromDom(line) {
  const mods = [...line.matchAll(MOD)]
  const k = line.match(KEY)
  if (!mods.length || !k) return null
  const req = new Set(mods.filter((m) => m[1] !== '!').map((m) => m[2]))
  const parts = []
  // ctrlKey||metaKey is the cross-platform idiom — one key, not two modifiers.
  if (req.has('ctrlKey') || req.has('metaKey')) parts.push('Ctrl/Cmd')
  if (req.has('shiftKey')) parts.push('Shift')
  if (req.has('altKey')) parts.push('Alt')
  return [...parts, canonKey(k[1])].join('+')
}

function comboFromCm(spec) {
  const bits = spec.split('-')
  const key = bits.pop()
  const parts = []
  if (bits.includes('Mod') || bits.includes('Ctrl') || bits.includes('Cmd')) parts.push('Ctrl/Cmd')
  if (bits.includes('Shift')) parts.push('Shift')
  if (bits.includes('Alt')) parts.push('Alt')
  return [...parts, canonKey(key)].join('+')
}

function sourceSites() {
  const files = readdirSync(SRC, { recursive: true })
    .map(String)
    .filter((f) => (f.endsWith('.svelte') || f.endsWith('.js')) && !f.endsWith('.test.js'))
  const sites = []
  for (const f of files) {
    readFileSync(new URL(f, SRC), 'utf8').split('\n').forEach((line, i) => {
      const d = comboFromDom(line)
      if (d) sites.push({ combo: d, file: f, line: i + 1 })
      for (const m of line.matchAll(CM)) {
        sites.push({ combo: comboFromCm(m[1]), file: f, line: i + 1 })
      }
    })
  }
  return sites
}

// Only MODIFIER shortcuts are compared. Unmodified keys — Escape to close,
// Enter to confirm, arrows to move — are contextual conventions, deliberately
// out of the panel's scope per the spec, and a modifier-based scan cannot see
// them anyway. Documenting them would bury the entries that matter.
const documented = () => {
  const set = new Map()
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) set.set(it.keys.join('+'), it)
  }
  return set
}

test('the scan finds the shortcuts known to exist', () => {
  const sites = sourceSites()
  // Measured on this tree while the plan was written. A materially smaller
  // number means the extractor broke, not that shortcuts were deleted — and a
  // silently-empty scan would make both directions below pass vacuously.
  assert.ok(sites.length >= 15, `found only ${sites.length} shortcut sites; the scan is broken`)
})

test('every documented shortcut exists at a site it names', () => {
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      if (it.keys.join('+') === 'F1') continue // added by this feature; Task 3 wires it
      for (const f of it.site) {
        const src = readFileSync(new URL(f, SRC), 'utf8')
        const key = it.keys[it.keys.length - 1]
        const needle = key === '←' ? 'ArrowLeft' : key === '→' ? 'ArrowRight' : key
        assert.ok(
          src.includes(`'${needle}'`) || src.includes(`'${needle.toLowerCase()}'`) ||
            src.includes(`-${needle.toLowerCase()}'`),
          `${g.title}/${it.label}: ${f} contains no handler for ${needle}`,
        )
      }
    }
  }
})

test('every modifier shortcut in source is documented', () => {
  const docs = documented()
  const undocumented = []
  for (const s of sourceSites()) {
    if (!s.combo.includes('+')) continue // unmodified: contextual, out of scope
    if (!docs.has(s.combo)) undocumented.push(`${s.combo} at ${s.file}:${s.line}`)
  }
  assert.deepEqual(undocumented, [],
    `shortcuts in source but missing from the help panel:\n${undocumented.join('\n')}`)
})
