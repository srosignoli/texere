import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { n4lHighlightStyle } from './n4l-highlight.js'
import { n4lTheme, arrowLint, notebookDotsGutter } from './n4l-editor.js'

const CSS = readFileSync(new URL('../style.css', import.meta.url), 'utf8')

const ROLES = [
  'heading', 'keyword', 'arrow-code', 'arrow-target', 'ditto',
  'marker', 'provenance', 'label', 'comment', 'invalid',
]

// Blue is today's palette and must not shift by a single byte.
const BLUE = {
  heading: '#7dd3fc', keyword: '#c084fc', 'arrow-code': '#67e8f9',
  'arrow-target': '#86efac', ditto: '#94a3b8', marker: '#fde68a',
  provenance: '#6b7280', label: '#a78bfa', comment: '#4b5563', invalid: '#f59e0b',
}

function block(theme) {
  const re = theme === 'blue'
    ? /:root\s*\{([\s\S]*?)\n\}/
    : new RegExp(`:root\\[data-theme="${theme}"\\]\\s*\\{([\\s\\S]*?)\\n\\}`)
  const m = CSS.match(re)
  assert.ok(m, `no ${theme} block in style.css`)
  return m[1]
}

function value(theme, role) {
  const m = block(theme).match(new RegExp(`--n4l-syntax-${role}\\s*:\\s*(#[0-9a-fA-F]{3,8})\\s*;`))
  return m && m[1].toLowerCase()
}

function ratio(a, b) {
  const lum = (h) => {
    const c = [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255)
      .map((v) => (v > 0.03928 ? ((v + 0.055) / 1.055) ** 2.4 : v / 12.92))
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]
  }
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p)
  return (x + 0.05) / (y + 0.05)
}

test('every syntax role is defined in all three themes', () => {
  for (const theme of ['blue', 'dark', 'light']) {
    for (const role of ROLES) {
      assert.ok(value(theme, role), `--n4l-syntax-${role} missing from ${theme}`)
    }
  }
})

// The migration's standing constraint: Blue does not move.
test('Blue keeps the exact values it has today', () => {
  for (const role of ROLES) {
    assert.equal(value('blue', role), BLUE[role], role)
  }
})

test('every Light and Dark syntax colour clears AA against its editor background', () => {
  const bg = { light: '#f6f7f9', dark: '#16171a' }
  for (const theme of ['light', 'dark']) {
    for (const role of ROLES) {
      const r = ratio(value(theme, role), bg[theme])
      assert.ok(r >= 4.5, `${theme} ${role} is ${r.toFixed(2)}:1, want >= 4.5`)
    }
  }
})

// Blue renders these as two distinct greys, comment the dimmer. A naive
// Solarized mapping sends both to base1 and loses the distinction.
test('provenance and comment stay distinct, comment dimmer', () => {
  const bg = { blue: '#1b2636', light: '#f6f7f9', dark: '#16171a' }
  for (const theme of ['blue', 'light', 'dark']) {
    const p = value(theme, 'provenance'), c = value(theme, 'comment')
    assert.notEqual(p, c, `${theme}: provenance and comment are the same colour`)
    assert.ok(ratio(c, bg[theme]) < ratio(p, bg[theme]),
      `${theme}: comment should be dimmer than provenance`)
  }
})

// The gate no visual check would reliably catch: the generated CSS must carry
// no literal colour at all.
test('the highlight style emits only var() colours', () => {
  const rules = n4lHighlightStyle.module.rules.join('\n')
  assert.match(rules, /var\(--n4l-syntax-/)
  assert.doesNotMatch(rules, /#[0-9a-fA-F]{3,8}/, `literal colour in generated CSS:\n${rules}`)
})

// Same gate as the highlight style, widened to the whole module: n4l-editor.js
// has three EditorView.theme() blocks — n4lTheme's own chrome, arrowLint's
// unregistered-arrow popover, and notebookDotsGutter's provenance dot — and
// all three must emit no literal colour. A narrower version of this test once
// only read n4lTheme()'s generated rules, which is why the other two blocks
// went unmigrated and unnoticed: the suite was green while real unthemed
// chrome remained. This asserts on colour literals only — lengths, radii and
// the box-shadow's rgba() are not colours in a theme role's sense.
const N4L_EDITOR_SRC = readFileSync(new URL('./n4l-editor.js', import.meta.url), 'utf8')

// EditorView.theme() does not return a HighlightStyle-shaped object (that
// shape — a top-level `.module` — belongs to HighlightStyle, e.g.
// n4lHighlightStyle above). It returns a CodeMirror "Extension": an
// arbitrarily nested tree of arrays and facet providers, with the generated
// StyleModule living at some provider's `.value` (confirmed by inspection: a
// bare EditorView.theme() call is a 2-element array whose second element's
// `.value` is the StyleModule; arrowLint/notebookDotsGutter nest that same
// shape inside a larger returned array alongside unrelated extensions).
// Walking the tree generically — rather than hardcoding an index path per
// function — is what lets this test survive the internal shape changing.
function collectStyleModuleRules(ext, acc = [], seen = new Set()) {
  if (ext == null || typeof ext !== 'object') return acc
  if (Array.isArray(ext)) { for (const e of ext) collectStyleModuleRules(e, acc, seen); return acc }
  if (seen.has(ext)) return acc
  seen.add(ext)
  if (ext.value && Array.isArray(ext.value.rules)) acc.push(ext.value.rules)
  if (ext.value !== undefined) collectStyleModuleRules(ext.value, acc, seen)
  if (ext.dependencies) collectStyleModuleRules(ext.dependencies, acc, seen)
  return acc
}

// Every exported function in n4l-editor.js that builds an EditorView.theme()
// extension, invoked with throwaway arguments sufficient to build the
// extension tree without mounting a real editor view.
const THEME_PRODUCERS = {
  n4lTheme: () => n4lTheme(),
  arrowLint: () => arrowLint(() => []),
  notebookDotsGutter: () => notebookDotsGutter(() => {}),
}

test('every EditorView.theme() block in n4l-editor.js emits only var() colours', () => {
  // Self-check: the number of EditorView.theme( call sites in the source must
  // equal the number of generated StyleModules this test actually inspects.
  // If a future block is added to n4l-editor.js without also adding (or
  // wiring into an existing) THEME_PRODUCERS entry that exercises it, this
  // mismatch fails loudly instead of letting the gap re-open silently — the
  // exact failure mode that hid arrowLint's and notebookDotsGutter's blocks
  // the first time.
  // Strip comments first — a mention of "EditorView.theme(" in a comment
  // (this test's own doc comments do it) must not inflate the count.
  const sourceNoComments = N4L_EDITOR_SRC
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/\/\/[^\n]*/g, '')
  const sourceCallSites = (sourceNoComments.match(/EditorView\.theme\(/g) ?? []).length
  const allRules = []
  for (const build of Object.values(THEME_PRODUCERS)) collectStyleModuleRules(build(), allRules)
  assert.equal(allRules.length, sourceCallSites,
    `found ${allRules.length} generated theme block(s) via THEME_PRODUCERS but ${sourceCallSites} ` +
    `EditorView.theme(...) call site(s) in n4l-editor.js — a theme block was added without being ` +
    `exercised by THEME_PRODUCERS in this test`)

  const rules = allRules.flat().join('\n')
  assert.match(rules, /var\(--/)
  const literals = rules.match(/#[0-9a-fA-F]{3,8}\b/g) ?? []
  assert.deepEqual(literals, [], `literal colours in editor chrome: ${literals.join(', ')}`)
})
