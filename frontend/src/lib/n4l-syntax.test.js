import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { n4lHighlightStyle } from './n4l-highlight.js'
import { n4lTheme } from './n4l-editor.js'

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

// Same gate as the highlight style: the editor's chrome must emit no literal
// colour. Note this asserts on colour literals only — lengths, radii and the
// box-shadow's rgba() are not colours in a theme role's sense.
//
// n4lTheme() returns an EditorView.theme() extension, which — unlike
// n4lHighlightStyle (a HighlightStyle instance with a top-level `.module`) —
// is a two-element facet-provider array; the generated StyleModule lives at
// index [1].value, not at a top-level `.module`. Confirmed by inspection:
// n4lTheme()[1].value.rules holds the real generated CSS rule strings.
test('the editor theme emits only var() colours', () => {
  const rules = n4lTheme()[1].value.rules.join('\n')
  assert.match(rules, /var\(--/)
  const literals = rules.match(/#[0-9a-fA-F]{3,8}\b/g) ?? []
  assert.deepEqual(literals, [], `literal colours in editor chrome: ${literals.join(', ')}`)
})
