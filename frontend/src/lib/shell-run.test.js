import { test } from 'node:test'
import assert from 'node:assert/strict'
import { appendChunks } from './shell-run.js'

test('chunks are split into lines and appended in order', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\nb\n' }], 100)
  assert.deepEqual(got.map((l) => l.text), ['a', 'b'])
  assert.deepEqual(got.map((l) => l.stream), ['stdout', 'stdout'])
})

// Output arrives mid-line: a chunk boundary is not a line boundary.
test('a partial line is completed by the next chunk', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'hel' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['hel'])
  buf = appendChunks(buf, [{ stream: 'stdout', text: 'lo\n' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['hello'])
})

// A partial stdout line must not swallow a stderr chunk that arrives next.
test('a partial line is only continued by the same stream', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'out-partial' }], 100)
  buf = appendChunks(buf, [{ stream: 'stderr', text: 'err\n' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['out-partial', 'err'])
  assert.deepEqual(buf.map((l) => l.stream), ['stdout', 'stderr'])
})

test('stderr lines keep their stream tag so the UI can tint them', () => {
  const got = appendChunks([], [{ stream: 'stderr', text: 'boom\n' }], 100)
  assert.equal(got[0].stream, 'stderr')
})

test('carriage returns are stripped so CRLF output does not gain blank lines', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\r\nb\r\n' }], 100)
  assert.deepEqual(got.map((l) => l.text), ['a', 'b'])
})

// Without this, `yes` exhausts the webview.
test('the buffer is capped, dropping the oldest lines', () => {
  let buf = []
  for (let i = 0; i < 20; i++) buf = appendChunks(buf, [{ stream: 'stdout', text: `${i}\n` }], 5)
  assert.equal(buf.length, 5)
  assert.deepEqual(buf.map((l) => l.text), ['15', '16', '17', '18', '19'])
})

test('a single oversized chunk is also capped', () => {
  const text = Array.from({ length: 500 }, (_, i) => `l${i}`).join('\n') + '\n'
  const got = appendChunks([], [{ stream: 'stdout', text }], 10)
  assert.equal(got.length, 10)
  assert.equal(got[9].text, 'l499')
})

test('empty and malformed input leaves the buffer alone', () => {
  const buf = [{ stream: 'stdout', text: 'keep' }]
  assert.deepEqual(appendChunks(buf, [], 100), buf)
  assert.deepEqual(appendChunks(buf, null, 100), buf)
  assert.deepEqual(appendChunks(buf, [{ stream: 'stdout', text: '' }], 100), buf)
})

test('blank lines within output are preserved', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\n\nb\n' }], 100)
  assert.deepEqual(got.map((l) => l.text), ['a', '', 'b'])
})
