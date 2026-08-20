import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { llmSettingsOpen } from '../stores/workbench.js'

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8')

// A subscribe-only alias with no `set` is what made the toolbar's Search button
// throw and blank the whole webview in feature 051. Both call sites use set().
test('llmSettingsOpen is a real writable', () => {
  assert.equal(typeof llmSettingsOpen.subscribe, 'function')
  assert.equal(typeof llmSettingsOpen.set, 'function')
  assert.equal(typeof llmSettingsOpen.update, 'function')
})

test('the notebook pane no longer owns LLM settings', () => {
  const src = read('../components/notebook/NotebookPane.svelte')
  assert.ok(!src.includes('LLMSettings'), 'NotebookPane still references LLMSettings')
  assert.ok(!src.includes('nb-gear'), 'NotebookPane still has the gear button or its style')
})

// Anchored to the import statements rather than a whole-file substring search.
// A plain `.includes('llmSettingsOpen')` can't fail when the import is deleted,
// because a leftover call site (`llmSettingsOpen.set(...)`, `$llmSettingsOpen`)
// still contains the identifier as text — the exact shape of the feature-051
// false alarm the top comment in this file warns about, recurring inside the
// test meant to guard against it. These regexes require the actual `import`
// line naming the identifier, anchored to the start of a line and excluding
// lines that start with `//` or `*`, so deleting the import — or merely
// commenting it out, leaving the file broken at runtime — makes the assertion
// fail as its message claims it will.
//
// The import checks alone are not enough: they're satisfied by two import
// lines with no launcher and no mount point behind them. This test also
// requires the actual trigger (SettingsMenu calling llmSettingsOpen.set(true))
// and the actual mount ({#if $llmSettingsOpen}<LLMSettings .../> in App.svelte)
// — deleting either of those while leaving the imports in place is the
// realistic regression (e.g. someone refactors App.svelte's render block) and
// must fail here.
const liveImport = (specifier, name) =>
  new RegExp(
    `^(?!\\s*(?:\\/\\/|\\*))\\s*import\\s*\\{[^}]*\\b${name}\\b[^}]*\\}\\s*from\\s*['"]${specifier.replace(/[./]/g, '\\$&')}['"]`,
    'm',
  )

test('the settings menu opens it and App mounts it', () => {
  const menu = read('../components/SettingsMenu.svelte')
  assert.match(
    menu,
    liveImport('../stores/workbench.js', 'llmSettingsOpen'),
    'SettingsMenu does not import llmSettingsOpen from the store',
  )
  assert.ok(
    menu.includes('llmSettingsOpen.set(true)'),
    'SettingsMenu has no launcher that opens LLMSettings',
  )

  const app = read('../App.svelte')
  assert.match(
    app,
    /^(?!\s*(?:\/\/|\*))\s*import\s+LLMSettings\s+from\s*['"]\.\/components\/LLMSettings\.svelte['"]/m,
    'App.svelte does not import the LLMSettings component',
  )
  assert.match(
    app,
    liveImport('./stores/workbench.js', 'llmSettingsOpen'),
    'App.svelte does not import llmSettingsOpen from the store',
  )
  assert.match(
    app,
    /\{#if \$llmSettingsOpen\}[\s\S]{0,200}<LLMSettings\b/,
    'App.svelte does not mount LLMSettings behind llmSettingsOpen',
  )
})
