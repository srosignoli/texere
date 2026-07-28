import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cellLabel } from './cell-label.js'

test('each stacked cell type gets the name of the button that creates it', () => {
  assert.equal(cellLabel({ type: 'n4l' }), 'N4L draft')
  assert.equal(cellLabel({ type: 'graph' }), 'Graph')
  assert.equal(cellLabel({ type: 'sketch' }), 'Sketch')
  assert.equal(cellLabel({ type: 'audio' }), 'Audio')
  // The button reads "+ Scan", so the header must too — not "Image".
  assert.equal(cellLabel({ type: 'image' }), 'Scan')
})

test('a detail is appended where the type has meaningful variants', () => {
  assert.equal(cellLabel({ type: 'llm', mode: 'tutor' }), 'LLM · tutor')
  assert.equal(cellLabel({ type: 'llm', mode: 'ask' }), 'LLM · ask')
  assert.equal(cellLabel({ type: 'compute', language: 'js' }), 'Compute · js')
})

// The whole point: three stacked Media cells must not read identically.
test('a media cell names what it actually holds', () => {
  assert.equal(cellLabel({ type: 'media', kind: 'image' }), 'Media · image')
  assert.equal(cellLabel({ type: 'media', kind: 'video' }), 'Media · video')
  assert.equal(cellLabel({ type: 'media', kind: 'audio' }), 'Media · audio')
})

// "pronunciation" is the word a language learner scans for; "audio" is not.
test('a pronunciation is named as one, not as audio', () => {
  assert.equal(
    cellLabel({ type: 'media', kind: 'audio', provider: 'pronunciation' }),
    'Media · pronunciation')
  assert.equal(
    cellLabel({ type: 'media', kind: 'audio', provider: 'commons-audio' }),
    'Media · audio')
})

// Media cells saved before the audio slice have no `kind`. A dangling
// separator would look like a rendering bug.
test('a missing detail appends nothing, not a dangling separator', () => {
  assert.equal(cellLabel({ type: 'media' }), 'Media')
  assert.equal(cellLabel({ type: 'llm' }), 'LLM')
  assert.equal(cellLabel({ type: 'compute' }), 'Compute')
})

test('an unknown type stays identifiable rather than blank', () => {
  assert.equal(cellLabel({ type: 'widget' }), 'widget')
  // context is vestigial in cellRegistry and never stacked, but if one ever
  // appeared it must still render something.
  assert.equal(cellLabel({ type: 'context' }), 'context')
})

test('the header is never empty', () => {
  assert.equal(cellLabel({}), 'Cell')
  assert.equal(cellLabel({ type: '' }), 'Cell')
  assert.equal(cellLabel(null), 'Cell')
  assert.equal(cellLabel(undefined), 'Cell')
})
