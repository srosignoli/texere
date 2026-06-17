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
