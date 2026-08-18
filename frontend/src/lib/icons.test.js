import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { ICONS } from './icons.js'

const COMPONENTS = new URL('../components/', import.meta.url)

function svelteSources() {
  const files = readdirSync(COMPONENTS, { recursive: true })
    .map(String)
    .filter((f) => f.endsWith('.svelte'))
  return files.map((f) => [f, readFileSync(new URL(f, COMPONENTS), 'utf8')])
}

// Names reach <Icon> two ways: as a literal (name="save") and inside an
// expression (name={expanded ? 'collapse' : 'expand'}). Both must be checked,
// so the expression branch pulls every quoted string out of the braces.
function iconNamesIn(src) {
  const names = new Set()
  for (const m of src.matchAll(/<Icon\b[^>]*?\bname=(?:"([^"]+)"|\{([^}]*)\})/g)) {
    if (m[1]) names.add(m[1])
    else for (const q of m[2].matchAll(/'([^']+)'/g)) names.add(q[1])
  }
  return names
}

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

test('every <Icon name=…> in a component exists in ICONS', () => {
  let checked = 0
  for (const [file, src] of svelteSources()) {
    for (const name of iconNamesIn(src)) {
      checked++
      assert.ok(ICONS[name], `${file} uses unknown icon "${name}"`)
    }
  }
  // Without this the test passes when the regex silently matches nothing —
  // which is exactly what a renamed prop or a changed tag would cause.
  assert.ok(checked > 0, 'found no <Icon> usages at all; the scan is broken')
})

const SRC = new URL('../', import.meta.url)

// Astral-plane emoji (U+1F300–U+1FAFF) are the ones with no glyph in the fonts
// a bare Linux install ships, so they render as tofu boxes. BMP symbols like ⚙
// and ↩ are fine and deliberately not matched here.
const ASTRAL = /[\u{1F300}-\u{1FAFF}]/u

test('no astral-plane emoji remain in the UI source', () => {
  const files = readdirSync(SRC, { recursive: true })
    .map(String)
    .filter((f) => (f.endsWith('.svelte') || f.endsWith('.js')) && !f.endsWith('.test.js'))
  assert.ok(files.length > 0, 'scanned no files; the walk is broken')
  const offenders = []
  for (const f of files) {
    const src = readFileSync(new URL(f, SRC), 'utf8')
    for (const [i, line] of src.split('\n').entries()) {
      if (ASTRAL.test(line)) offenders.push(`${f}:${i + 1}: ${line.trim()}`)
    }
  }
  assert.deepEqual(offenders, [], `emoji left in the UI:\n${offenders.join('\n')}`)
})
