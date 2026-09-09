import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

// docs/demos/selectors.mjs is the contract between this UI and the demo plots,
// which live in a separate repository and are therefore never exercised by this
// CI. This test closes that gap: every selector the contract publishes must
// still resolve against the components here, so renaming a control fails a
// texere test rather than a private recording weeks later.

const root = new URL('../../../', import.meta.url).pathname
const srcDir = join(root, 'frontend', 'src')

// Components AND the plain modules beside them. CodeMirror decorations are
// applied from lib/*.js with el.className or as a theme key, never from a
// component, so a walk over .svelte alone could not verify them — and a class
// the contract cannot verify is a class the contract cannot carry, which is
// why .cm-nb-dot and .cm-arrow-unreg used to be hardcoded inside plots.
//
// Test files are excluded on purpose: a class named only in an assertion here
// would otherwise satisfy the contract while no shipping code applies it.
function allSource() {
  const out = []
  const walk = (dir) => {
    for (const name of readdirSync(dir)) {
      const p = join(dir, name)
      if (statSync(p).isDirectory()) walk(p)
      else if (name.endsWith('.svelte')) out.push(readFileSync(p, 'utf8'))
      else if (name.endsWith('.js') && !name.endsWith('.test.js')) out.push(readFileSync(p, 'utf8'))
    }
  }
  walk(srcDir)
  return out.join('\n')
}

const contract = readFileSync(join(root, 'docs', 'demos', 'selectors.mjs'), 'utf8')
const source = allSource()

// Only the lines that actually declare selectors — not the prose above them.
const selectorLines = contract
  .split('\n')
  .filter((l) => /^\s{2}\w+:/.test(l))

test('the contract declares selectors', () => {
  assert.ok(selectorLines.length > 10, `found only ${selectorLines.length} selector lines`)
})

test('every aria-label in the contract exists in a component', () => {
  const labels = [...contract.matchAll(/aria-label="([^"$]+)"/g)].map((m) => m[1])
  assert.ok(labels.length > 0, 'no literal aria-labels found in the contract')
  for (const label of new Set(labels)) {
    assert.ok(
      source.includes(`aria-label="${label}"`),
      `contract references aria-label="${label}", which no component defines`,
    )
  }
})

test('every title-prefix selector matches a component title', () => {
  const prefixes = [...contract.matchAll(/title\^="([^"]+)"/g)].map((m) => m[1])
  for (const prefix of new Set(prefixes)) {
    assert.ok(
      new RegExp(`title="${prefix.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`).test(source),
      `contract references title^="${prefix}", which no component title starts with`,
    )
  }
})

test('every class selector exists in a component', () => {
  // Vendor selectors are emitted by a library, not by our components, so they
  // are unverifiable here. They must be declared in VENDOR to be skipped —
  // an undeclared unmatched class is still a failure.
  const vendor = new Set(
    [...contract.matchAll(/^\s*'(\.[a-z-]+)',/gm)].map((m) => m[1]),
  )
  const classes = [...contract.matchAll(/'\.([a-z-]+)'/g)]
    .map((m) => m[1])
    .filter((c) => !vendor.has('.' + c))
  assert.ok(classes.length > 0, 'no class selectors found in the contract')
  for (const cls of new Set(classes)) {
    assert.ok(
      new RegExp(`class="[^"]*\\b${cls}\\b`).test(source) ||
        new RegExp(`class:${cls}\\b`).test(source) ||
        // Applied from JS: el.className = 'x', classList.add('x'), or a
        // CodeMirror theme keyed on '.x'. Quoted and whole, so a class name
        // appearing inside a longer string does not count as applying it.
        new RegExp(`['"\`]\\.?${cls}['"\`]`).test(source),
      `contract references .${cls}, which nothing in src applies`,
    )
  }
})

// The two study roots must stay distinguishable; a plot addresses them by name.
test('the dialog names in the contract are distinct and present', () => {
  const dialogs = [...contract.matchAll(/\[role="dialog"\]\[aria-label="([^"]+)"\]/g)].map((m) => m[1])
  assert.equal(new Set(dialogs).size, dialogs.length, 'contract names two dialogs identically')
  for (const name of dialogs) {
    assert.ok(source.includes(`aria-label="${name}"`), `no dialog named ${name}`)
  }
})

// A growing exemption list would quietly hollow out the check above.
test('the vendor exemption list stays small and justified', () => {
  const entries = [...contract.matchAll(/^\s*'(\.[a-z-]+)', \/\/ (.+)$/gm)]
  assert.ok(entries.length <= 3, `${entries.length} vendor exemptions — too many to trust the contract`)
  for (const [, sel, why] of entries) {
    assert.ok(why.trim().length > 10, `vendor exemption ${sel} has no real justification`)
  }
})
