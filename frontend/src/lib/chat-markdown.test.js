import { test } from 'node:test'
import assert from 'node:assert/strict'
import { chatToMarkdown, defaultTitle } from './chat-markdown.js'

const D = new Date('2026-08-07T12:00:00Z')

test('one exchange becomes a heading and a body', () => {
  const md = chatToMarkdown('Spaced repetition', [
    { role: 'user', content: 'How does SM-2 pick the next interval?' },
    { role: 'assistant', content: 'It multiplies by an ease factor.' },
  ], ['claude-opus-5'], D)
  assert.match(md, /^# Spaced repetition\n/)
  assert.match(md, /## How does SM-2 pick the next interval\?/)
  assert.match(md, /It multiplies by an ease factor\./)
})

test('several exchanges keep their order', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'first' },
    { role: 'assistant', content: 'answer one' },
    { role: 'user', content: 'second' },
    { role: 'assistant', content: 'answer two' },
  ], [], D)
  assert.ok(md.indexOf('## first') < md.indexOf('## second'), 'questions must stay in order')
  assert.ok(md.indexOf('answer one') < md.indexOf('## second'), 'each answer follows its own question')
})

// The document is a study source, not a record of a conversation in progress.
test('a question still awaiting its answer is dropped', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'answered' },
    { role: 'assistant', content: 'the answer' },
    { role: 'user', content: 'still pending' },
  ], [], D)
  assert.match(md, /## answered/)
  assert.doesNotMatch(md, /still pending/, 'an unanswered question must not become an empty section')
})

test('a multi-line question collapses to a single heading line', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'line one\nline two' },
    { role: 'assistant', content: 'a' },
  ], [], D)
  assert.match(md, /## line one line two/)
  assert.doesNotMatch(md, /## line one\nline two/, 'a newline inside a heading would break the structure')
})

test('markdown inside an answer passes through untouched', () => {
  const body = '- bullet\n\n```js\nconst x = 1\n```\n\n## an inner heading'
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'q' },
    { role: 'assistant', content: body },
  ], [], D)
  assert.ok(md.includes(body), 'answer bodies must not be re-escaped or reflowed')
})

test('the footer lists each model once, in first-use order', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'q' },
    { role: 'assistant', content: 'a' },
  ], ['claude-opus-5', 'gpt-4o', 'claude-opus-5'], D)
  assert.match(md, /\*Generated with claude-opus-5, gpt-4o — 2026-08-07\*/)
})

test('no models means no footer', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'q' },
    { role: 'assistant', content: 'a' },
  ], [], D)
  assert.doesNotMatch(md, /Generated with/)
})

test('an empty conversation still produces a titled document', () => {
  const md = chatToMarkdown('Just a title', [], [], D)
  assert.match(md, /^# Just a title/)
})

test('defaultTitle takes the first line of the first question, trimmed', () => {
  assert.equal(defaultTitle([{ role: 'user', content: 'What is spaced repetition?\nmore' }]),
    'What is spaced repetition?')
  assert.equal(defaultTitle([]), '')
})

test('defaultTitle caps a long question at 60 characters', () => {
  const long = 'x'.repeat(200)
  assert.equal(defaultTitle([{ role: 'user', content: long }]).length, 60)
})
