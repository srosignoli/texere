import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildMediaFact, mediaArrow } from './media-fact.js'

test('builds a has-picture fact from subject and asset', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'a3f9c1.jpg', kind: 'image', provider: 'commons' }),
    'manzana\n"  (pic) media/a3f9c1.jpg\n')
})

test('a cell saved before this slice has no kind and stays a picture', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'a3f9c1.jpg' }),
    'manzana\n"  (pic) media/a3f9c1.jpg\n')
})

test('a pronunciation result uses the pron arrow', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'b7.wav', kind: 'audio', provider: 'pronunciation' }),
    'manzana\n"  (pron) media/b7.wav\n')
})

test('other audio uses the generic aud arrow', () => {
  for (const provider of ['commons-audio', 'file', 'url', '']) {
    assert.equal(
      buildMediaFact({ subject: 'nightingale', asset: 'c9.ogg', kind: 'audio', provider }),
      'nightingale\n"  (aud) media/c9.ogg\n',
      `provider=${provider}`)
  }
})

test('a multi-word subject is preserved verbatim', () => {
  assert.equal(
    buildMediaFact({ subject: 'red delicious apple', asset: 'b7.png', kind: 'image' }),
    'red delicious apple\n"  (pic) media/b7.png\n')
})

test('subject is trimmed', () => {
  assert.equal(
    buildMediaFact({ subject: '  manzana  ', asset: 'a3.jpg', kind: 'image' }),
    'manzana\n"  (pic) media/a3.jpg\n')
})

test('an empty subject yields an empty string, not a malformed fact', () => {
  assert.equal(buildMediaFact({ subject: '', asset: 'a3.jpg', kind: 'image' }), '')
  assert.equal(buildMediaFact({ subject: '   ', asset: 'a3.jpg', kind: 'image' }), '')
})

test('a missing asset yields an empty string', () => {
  assert.equal(buildMediaFact({ subject: 'manzana', asset: '', kind: 'image' }), '')
})

test('mediaArrow is exported so the UI can name the arrow it will write', () => {
  assert.equal(mediaArrow({ kind: 'audio', provider: 'pronunciation' }), 'pron')
  assert.equal(mediaArrow({ kind: 'audio', provider: 'commons-audio' }), 'aud')
  assert.equal(mediaArrow({ kind: 'image', provider: 'commons' }), 'pic')
  assert.equal(mediaArrow({}), 'pic')
})
