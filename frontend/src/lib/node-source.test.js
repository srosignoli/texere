import { test } from 'node:test'
import assert from 'node:assert/strict'
import { resolveNodeSource } from './node-source.js'

const cache = {
  '/c/a.n4l': { Chapters: [
    { Title: 'Animals', Nodes: [ { Text: 'cat', Line: 5 }, { Text: 'dog', Line: 7 } ] },
  ] },
  '/c/b.n4l': { Chapters: [
    { Title: 'Pets', Nodes: [ { Text: 'cat', Line: 3 } ] },
  ] },
}

test('exact chapter::text match returns that file/line', () => {
  assert.deepEqual(resolveNodeSource('Animals::cat', cache),
    { filePath: '/c/a.n4l', chapterTitle: 'Animals', line: 5, matchCount: 2 })
})

test('chapter disambiguates when the text appears in multiple files', () => {
  assert.deepEqual(resolveNodeSource('Pets::cat', cache),
    { filePath: '/c/b.n4l', chapterTitle: 'Pets', line: 3, matchCount: 2 })
})

test('plain text id (no chapter) returns the first match', () => {
  assert.deepEqual(resolveNodeSource('dog', cache),
    { filePath: '/c/a.n4l', chapterTitle: 'Animals', line: 7, matchCount: 1 })
})

test('chapter present but only the text matches elsewhere falls back to first text match', () => {
  assert.deepEqual(resolveNodeSource('Birds::dog', cache),
    { filePath: '/c/a.n4l', chapterTitle: 'Animals', line: 7, matchCount: 1 })
})

test('unresolved text returns null', () => {
  assert.equal(resolveNodeSource('Animals::elephant', cache), null)
})

test('null / empty inputs return null', () => {
  assert.equal(resolveNodeSource('', cache), null)
  assert.equal(resolveNodeSource('cat', null), null)
})

// Target/ditto resolution: many graph nodes are arrow *targets* (objects) that
// never start a line, so they live in Node.Arrows[].Target / Dittos, not in
// Chapter.Nodes[].Text. They still have an editable source line — the line that
// references them.
const arrowsCache = {
  '/c/x.n4l': { Chapters: [
    { Title: 'Bio', Nodes: [
      {
        Text: 'whale', Line: 10,
        Arrows: [ { Code: 'eats', Target: 'krill', Line: 10 } ],
        Dittos: [ { Text: 'mammal', Line: 11, Arrows: [ { Code: 'isa', Target: 'animal', Line: 11 } ] } ],
      },
      { Text: 'krill', Line: 20 },
    ] },
  ] },
}

test('a target-only node resolves to the line that references it', () => {
  assert.deepEqual(resolveNodeSource('Bio::animal', arrowsCache),
    { filePath: '/c/x.n4l', chapterTitle: 'Bio', line: 11, matchCount: 1 })
})

test('a subject line is preferred over a reference line when both exist', () => {
  // "krill" is referenced as an arrow target on line 10 but is its own subject
  // on line 20; editing its own line is preferred.
  assert.deepEqual(resolveNodeSource('Bio::krill', arrowsCache),
    { filePath: '/c/x.n4l', chapterTitle: 'Bio', line: 20, matchCount: 1 })
})

test('a ditto continuation text resolves as a subject', () => {
  assert.deepEqual(resolveNodeSource('Bio::mammal', arrowsCache),
    { filePath: '/c/x.n4l', chapterTitle: 'Bio', line: 11, matchCount: 1 })
})
