import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  hasArrows, isText, isLinked, isExtract, isCard, hasDetail, NODE_FILTERS,
} from './node-filters.js'

// Node shapes mirroring the parser's output (Go field names, via Wails).
const plain      = { ID: '', Text: 'just a line', Arrows: [], Dittos: [], StudyMeta: null }
const bare       = { ID: '', Text: 'no collections at all' }
const rootArrow  = { ID: '', Text: 'a', Arrows: [{ Code: 'is', Target: 'b' }], Dittos: [], StudyMeta: null }
const dittoArrow = { ID: '', Text: 'a', Arrows: [], Dittos: [{ Text: '', Arrows: [{ Code: 'is', Target: 'b' }] }], StudyMeta: null }
const dittoOnly  = { ID: '', Text: 'a', Arrows: [], Dittos: [{ Text: 'more prose', Arrows: [] }], StudyMeta: null }
const extract    = { ID: 'e1', Text: 'an extracted sentence', Arrows: [], Dittos: [], StudyMeta: null }
const extractArr = { ID: 'e2', Text: 'an extract that grew an arrow', Arrows: [{ Code: 'is', Target: 'b' }], Dittos: [], StudyMeta: null }
const card       = { ID: 'c1', Text: 'front >> back', Arrows: [], Dittos: [], StudyMeta: { CardType: 'basic' } }
const cardNoID   = { ID: '', Text: 'hand written >> card', Arrows: [], Dittos: [], StudyMeta: { CardType: 'basic' } }
const cardArrows = { ID: 'c2', Text: 'front >> back', Arrows: [{ Code: 'is', Target: 'b' }], Dittos: [], StudyMeta: { CardType: 'cloze' } }

test('hasArrows sees arrows on the node and on a ditto', () => {
  assert.equal(hasArrows(rootArrow), true)
  assert.equal(hasArrows(dittoArrow), true)
  assert.equal(hasArrows(plain), false)
  assert.equal(hasArrows(dittoOnly), false)
})

// A node arriving without Arrows/Dittos at all must not throw.
test('hasArrows tolerates missing collections', () => {
  assert.equal(hasArrows(bare), false)
})

test('isText is prose with no arrows and no study marker', () => {
  assert.equal(isText(plain), true)
  assert.equal(isText(dittoOnly), true)
  assert.equal(isText(rootArrow), false)
  assert.equal(isText(card), false)
})

// An extract is structurally prose. Overlapping lenses: it is BOTH.
test('isText includes an extract that carries no arrows', () => {
  assert.equal(isText(extract), true)
  assert.equal(isExtract(extract), true)
})

test('isLinked is any node carrying an arrow', () => {
  assert.equal(isLinked(rootArrow), true)
  assert.equal(isLinked(dittoArrow), true)
  assert.equal(isLinked(plain), false)
})

// The overlap the spec calls out: a card with arrows must appear under Linked,
// which is the one place someone hunting for arrows would look.
test('isLinked includes a card that carries arrows', () => {
  assert.equal(isLinked(cardArrows), true)
  assert.equal(isCard(cardArrows), true)
})

test('isExtract needs an @id and no study marker', () => {
  assert.equal(isExtract(extract), true)
  assert.equal(isExtract(extractArr), true)
  assert.equal(isExtract(plain), false)
  assert.equal(isExtract(card), false)
})

test('isCard is any study-marked node, with or without an @id', () => {
  assert.equal(isCard(card), true)
  assert.equal(isCard(cardNoID), true)
  assert.equal(isCard(cardArrows), true)
  assert.equal(isCard(extract), false)
  assert.equal(isCard(plain), false)
})

// A hand-written card with no @id must be reachable somewhere. Under the
// literal "@id AND @card" rule it would appear in no filter at all.
test('a study marker without an @id is still a card and is not an extract', () => {
  assert.equal(isCard(cardNoID), true)
  assert.equal(isExtract(cardNoID), false)
})

// hasDetail is the RENDER switch, not a filter. It must keep accepting exactly
// what the old isStructured accepted: arrows OR a study marker. If it ever
// narrows to hasArrows, a card with no arrows silently loses its collapsible
// row.
test('hasDetail accepts arrows or a study marker', () => {
  assert.equal(hasDetail(rootArrow), true)
  assert.equal(hasDetail(dittoArrow), true)
  assert.equal(hasDetail(card), true)
  assert.equal(hasDetail(cardNoID), true)
  assert.equal(hasDetail(plain), false)
  assert.equal(hasDetail(extract), false)
})

test('NODE_FILTERS is the bar, in order, with unique ids', () => {
  assert.deepEqual(NODE_FILTERS.map((f) => f.id), ['all', 'text', 'linked', 'extracts', 'cards'])
  assert.deepEqual(NODE_FILTERS.map((f) => f.label), ['All', 'Text', 'Linked', 'Extracts', 'Cards'])
  assert.equal(new Set(NODE_FILTERS.map((f) => f.id)).size, NODE_FILTERS.length)
})

// 'all' carries a real match so the caller filters unconditionally and never
// special-cases it.
test('every filter has a callable match, and all matches everything', () => {
  for (const f of NODE_FILTERS) assert.equal(typeof f.match, 'function', f.id)
  const all = NODE_FILTERS.find((f) => f.id === 'all')
  for (const n of [plain, rootArrow, extract, card, cardNoID, bare]) {
    assert.equal(all.match(n), true)
  }
})

test('each filter entry applies its own predicate', () => {
  const match = (id) => NODE_FILTERS.find((f) => f.id === id).match
  assert.equal(match('text')(plain), true)
  assert.equal(match('text')(rootArrow), false)
  assert.equal(match('linked')(rootArrow), true)
  assert.equal(match('linked')(plain), false)
  assert.equal(match('extracts')(extract), true)
  assert.equal(match('extracts')(card), false)
  assert.equal(match('cards')(card), true)
  assert.equal(match('cards')(extract), false)
})
