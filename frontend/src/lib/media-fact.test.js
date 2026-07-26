import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildMediaFact } from './media-fact.js'

test('builds a has-picture fact from subject and asset', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'a3f9c1.jpg' }),
    'manzana\n    has picture   media/a3f9c1.jpg\n')
})

test('a multi-word subject is preserved verbatim', () => {
  assert.equal(
    buildMediaFact({ subject: 'red delicious apple', asset: 'b7.png' }),
    'red delicious apple\n    has picture   media/b7.png\n')
})

test('subject is trimmed', () => {
  assert.equal(
    buildMediaFact({ subject: '  manzana  ', asset: 'a3.jpg' }),
    'manzana\n    has picture   media/a3.jpg\n')
})

test('an empty subject yields an empty string, not a malformed fact', () => {
  assert.equal(buildMediaFact({ subject: '', asset: 'a3.jpg' }), '')
  assert.equal(buildMediaFact({ subject: '   ', asset: 'a3.jpg' }), '')
})

test('a missing asset yields an empty string', () => {
  assert.equal(buildMediaFact({ subject: 'manzana', asset: '' }), '')
})
