import { test } from 'node:test'
import assert from 'node:assert/strict'
import { filterSources } from './sources-filter.js'

const readings = [
  { id: 'r1', title: 'Incremental learning' },
  { id: 'r2', title: 'Darwin notes' },
]
const byReading = {
  r1: [
    { id: 'e1', text: 'ensures 95% recall of top-priority material', anchor: 'char=10' },
    { id: 'e2', text: 'the fastest avenue towards knowledge', anchor: 'char=90' },
  ],
  r2: [{ id: 'e3', text: 'natural selection acts only by recall of variation', anchor: 'char=5' }],
}

test('an empty query returns every reading, nothing expanded', () => {
  const got = filterSources(readings, byReading, '')
  assert.equal(got.length, 2)
  assert.ok(got.every((r) => r.autoExpand === false))
  assert.ok(got.every((r) => r.extracts.length === 0))
})

test('a title match keeps the reading without expanding it', () => {
  const got = filterSources(readings, byReading, 'darwin')
  assert.equal(got.length, 1)
  assert.equal(got[0].reading.id, 'r2')
  assert.equal(got[0].autoExpand, false, 'a title match should not force the tree open')
})

// The case the search box exists for: finding a passage when you have
// forgotten which document it came from.
test('an extract match keeps the reading and expands to the matches only', () => {
  const got = filterSources(readings, byReading, 'fastest avenue')
  assert.equal(got.length, 1)
  assert.equal(got[0].reading.id, 'r1')
  assert.equal(got[0].autoExpand, true)
  assert.deepEqual(got[0].extracts.map((e) => e.id), ['e2'], 'only matching extracts are shown')
})

test('a query matching extracts in several documents keeps them all', () => {
  const got = filterSources(readings, byReading, 'recall')
  assert.deepEqual(got.map((r) => r.reading.id), ['r1', 'r2'])
  assert.deepEqual(got[0].extracts.map((e) => e.id), ['e1'])
  assert.deepEqual(got[1].extracts.map((e) => e.id), ['e3'])
})

test('matching is case-insensitive and trims the query', () => {
  assert.equal(filterSources(readings, byReading, '  DARWIN  ').length, 1)
  assert.equal(filterSources(readings, byReading, 'RECALL').length, 2)
})

test('no match returns an empty list', () => {
  assert.deepEqual(filterSources(readings, byReading, 'zzzznotathing'), [])
})

test('a reading with no extracts is handled', () => {
  const got = filterSources([{ id: 'r9', title: 'Lonely' }], {}, 'lonely')
  assert.equal(got.length, 1)
  assert.deepEqual(got[0].extracts, [])
})

test('order follows the input reading list', () => {
  const got = filterSources(readings, byReading, '')
  assert.deepEqual(got.map((r) => r.reading.id), ['r1', 'r2'])
})
