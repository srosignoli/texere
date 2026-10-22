import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  newDraftCell, newLLMCell, newGraphCell, newComputeCell, newSketchCell,
  newImageCell, newAudioCell, newMediaCell, newShellCell, pickMediaAsset,
} from './cell-schema.js'

test('each factory makes a cell of its own type with a unique id', () => {
  const made = {
    n4l: newDraftCell(), llm: newLLMCell(), graph: newGraphCell(), compute: newComputeCell(),
    sketch: newSketchCell(), image: newImageCell('a.png', 'image/png'), audio: newAudioCell(),
    media: newMediaCell(), shell: newShellCell(),
  }
  for (const [type, cell] of Object.entries(made)) assert.equal(cell.type, type)
  const ids = new Set(Array.from({ length: 20 }, () => newDraftCell().id))
  assert.equal(ids.size, 20)
})

// The factory and the pick are the two places a media cell's fields are
// written. If a pick writes a field the factory lacks, cells that never had a
// pick are missing it.
test('a new media cell already has every field a pick writes', () => {
  const picked = {}
  pickMediaAsset(picked, { kind: 'video', provider: 'p', embedURL: 'e', playerKind: 'iframe', title: 't', durationMs: 5 }, { asset: 'a', mime: 'm' })
  const fresh = newMediaCell()
  for (const key of Object.keys(picked)) assert.ok(key in fresh, `newMediaCell lacks ${key}`)
})

test('a pick swaps the asset and forgets the old framing', () => {
  const cell = { ...newMediaCell(), asset: 'old.png', crop: { x: 0.1, y: 0.1, w: 0.5, h: 0.5 }, naturalW: 800, naturalH: 600,
    title: 'old video', startSec: 42, embedURL: 'old', playerKind: 'iframe' }
  pickMediaAsset(cell, { kind: 'image' }, { asset: 'new.jpg', mime: 'image/jpeg' })
  assert.equal(cell.asset, 'new.jpg')
  assert.equal(cell.mime, 'image/jpeg')
  assert.equal(cell.kind, 'image')
  assert.equal(cell.crop, null, 'a crop of the old frame would mis-crop the new picture')
  assert.equal(cell.naturalW, 0)
  assert.equal(cell.naturalH, 0)
  assert.equal(cell.startSec, 0)
  assert.equal(cell.title, '')
  assert.equal(cell.embedURL, '')
  assert.equal(cell.playerKind, '')
})

test('a pick takes the subject from the query only when there is none', () => {
  const empty = { ...newMediaCell(), query: '  apple tree ' }
  pickMediaAsset(empty, {}, { asset: 'a', mime: 'm' })
  assert.equal(empty.subject, 'apple tree')

  const named = { ...newMediaCell(), query: 'apple tree', subject: 'Malus domestica' }
  pickMediaAsset(named, {}, { asset: 'a', mime: 'm' })
  assert.equal(named.subject, 'Malus domestica')
})

test('a pick keeps the provider when the result names none', () => {
  const cell = { ...newMediaCell(), provider: 'commons' }
  pickMediaAsset(cell, {}, { asset: 'a', mime: 'm' })
  assert.equal(cell.provider, 'commons')
  pickMediaAsset(cell, { provider: 'openverse' }, { asset: 'a', mime: 'm' })
  assert.equal(cell.provider, 'openverse')
})
