import { test } from 'node:test'
import assert from 'node:assert/strict'
import { getLastLanguage, setLastLanguage } from './last-language.js'

// Node has no global `localStorage` by default, which is exactly the guarded
// path this module exists to survive. Verify it deliberately rather than
// stubbing it away.
test('with no localStorage global, get returns empty and set is a silent no-op', () => {
  assert.equal(typeof globalThis.localStorage, 'undefined')
  assert.equal(getLastLanguage(), '')
  assert.doesNotThrow(() => setLastLanguage('Spanish'))
})

test('remembers and returns the last chosen language', (t) => {
  const store = new Map()
  globalThis.localStorage = {
    getItem: (k) => (store.has(k) ? store.get(k) : null),
    setItem: (k, v) => store.set(k, String(v)),
  }
  t.after(() => { delete globalThis.localStorage })

  assert.equal(getLastLanguage(), '')
  setLastLanguage('Spanish')
  assert.equal(getLastLanguage(), 'Spanish')
  setLastLanguage('French')
  assert.equal(getLastLanguage(), 'French')
})

test('setLastLanguage ignores empty/falsy names', (t) => {
  const store = new Map()
  globalThis.localStorage = {
    getItem: (k) => (store.has(k) ? store.get(k) : null),
    setItem: (k, v) => store.set(k, String(v)),
  }
  t.after(() => { delete globalThis.localStorage })

  setLastLanguage('German')
  setLastLanguage('')
  assert.equal(getLastLanguage(), 'German')
})

test('a throwing localStorage (quota/disabled) never escapes get or set', (t) => {
  globalThis.localStorage = {
    getItem: () => { throw new Error('disabled') },
    setItem: () => { throw new Error('quota exceeded') },
  }
  t.after(() => { delete globalThis.localStorage })

  assert.doesNotThrow(() => {
    assert.equal(getLastLanguage(), '')
  })
  assert.doesNotThrow(() => setLastLanguage('Italian'))
})
