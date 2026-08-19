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

test('the settings menu opens it and App mounts it', () => {
  assert.ok(read('../components/SettingsMenu.svelte').includes('llmSettingsOpen'),
    'SettingsMenu does not reference llmSettingsOpen')
  const app = read('../App.svelte')
  assert.ok(app.includes('LLMSettings'), 'App.svelte does not mount LLMSettings')
  assert.ok(app.includes('llmSettingsOpen'), 'App.svelte does not read llmSettingsOpen')
})
