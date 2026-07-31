import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cellPreview, formatDuration } from './cell-preview.js'

test('a text-bearing cell previews its first non-empty line', () => {
  assert.equal(cellPreview({ type: 'n4l', text: 'Ovid (wrote) Metamorphoses' }), 'Ovid (wrote) Metamorphoses')
  assert.equal(cellPreview({ type: 'compute', code: 'print(math.sqrt(16))' }), 'print(math.sqrt(16))')
  assert.equal(cellPreview({ type: 'graph', query: 'Ovid | +2' }), 'Ovid | +2')
})

// A draft that opens with blank lines still has content worth showing.
test('leading blank lines are skipped, and only the first real line is shown', () => {
  assert.equal(cellPreview({ type: 'n4l', text: '\n\n  Ovid\nMetamorphoses' }), 'Ovid')
  assert.equal(cellPreview({ type: 'compute', code: '\n\nprint(1)\nprint(2)' }), 'print(1)')
})

// The count says how much conversation is hidden; the first *user* message says
// what it was about. The first message in the array is always the user's today,
// but preferring by role keeps this honest if that ever changes.
test('an LLM cell previews its opening question and how many messages follow', () => {
  const cell = {
    type: 'llm',
    messages: [
      { role: 'user', content: 'explain the aorist tense' },
      { role: 'assistant', content: 'The aorist is…' },
    ],
  }
  assert.equal(cellPreview(cell), '"explain the aorist tense" · 2 msgs')
})

test('an LLM cell picks the first user message, not the first message', () => {
  const cell = {
    type: 'llm',
    messages: [
      { role: 'system', content: 'you are a tutor' },
      { role: 'user', content: 'decline rosa' },
    ],
  }
  assert.equal(cellPreview(cell), '"decline rosa" · 2 msgs')
})

test('one message is a msg, not msgs', () => {
  const cell = { type: 'llm', messages: [{ role: 'user', content: 'hi' }] }
  assert.equal(cellPreview(cell), '"hi" · 1 msg')
})

test('a sketch previews its stroke count, singular at one', () => {
  assert.equal(cellPreview({ type: 'sketch', strokes: [1, 2, 3] }), '3 strokes')
  assert.equal(cellPreview({ type: 'sketch', strokes: [1] }), '1 stroke')
})

// Assets are content-hashed (internal/media/import.go stores sha256hex+ext and
// discards the original name), so there is no filename to fall back to.
test('an image previews its geometry, since it has no name to show', () => {
  assert.equal(cellPreview({ type: 'image', naturalW: 1024, naturalH: 768 }), '1024×768')
})

test('an image with only one dimension loaded previews as empty', () => {
  assert.equal(cellPreview({ type: 'image', naturalW: 1024, naturalH: 0 }), '')
  assert.equal(cellPreview({ type: 'image', naturalW: 0, naturalH: 768 }), '')
})

test('an audio cell previews its duration', () => {
  assert.equal(cellPreview({ type: 'audio', durationMs: 14000 }), '0:14')
  assert.equal(cellPreview({ type: 'audio', durationMs: 605000 }), '10:05')
})

test('a media cell prefers title, then subject, then query', () => {
  assert.equal(cellPreview({ type: 'media', title: 'T', subject: 'S', query: 'Q' }), 'T')
  assert.equal(cellPreview({ type: 'media', title: '', subject: 'S', query: 'Q' }), 'S')
  assert.equal(cellPreview({ type: 'media', title: '', subject: '', query: 'Q' }), 'Q')
})

// A whitespace-only title is truthy, so a naive `||` chain picks it and only
// trims to '' afterwards — the cell would preview as blank despite having a
// real subject. Each candidate must be clipped before the fallback runs.
test('a media cell falls through a whitespace-only title to the subject', () => {
  assert.equal(cellPreview({ type: 'media', title: '   ', subject: 'S', query: 'Q' }), 'S')
})

// The caller renders no element at all for '', so a cell with nothing in it
// folds to its header alone rather than to a placeholder.
test('a cell with no content previews as the empty string', () => {
  assert.equal(cellPreview({ type: 'n4l', text: '' }), '')
  assert.equal(cellPreview({ type: 'n4l', text: '\n  \n' }), '')
  assert.equal(cellPreview({ type: 'llm', messages: [] }), '')
  assert.equal(cellPreview({ type: 'graph', query: '' }), '')
  assert.equal(cellPreview({ type: 'compute', code: '' }), '')
  assert.equal(cellPreview({ type: 'sketch', strokes: [] }), '')
  assert.equal(cellPreview({ type: 'image', naturalW: 0, naturalH: 0 }), '')
  assert.equal(cellPreview({ type: 'audio', durationMs: 0 }), '')
  assert.equal(cellPreview({ type: 'media', title: '', subject: '', query: '' }), '')
})

// Unlike cellLabel, which falls back to the raw type so a header is never
// blank, there is no sensible content guess for a type this module never saw.
test('an unknown or absent type previews as the empty string', () => {
  assert.equal(cellPreview({ type: 'hologram' }), '')
  assert.equal(cellPreview({}), '')
  assert.equal(cellPreview(null), '')
  assert.equal(cellPreview(undefined), '')
})

test('a shell cell previews its command', () => {
  assert.equal(cellPreview({ type: 'shell', command: 'docker ps' }), 'docker ps')
})

test('a multi-line shell command previews its first non-empty line', () => {
  assert.equal(cellPreview({ type: 'shell', command: '\n\ncd /tmp\nls -la' }), 'cd /tmp')
})

test('a shell cell with no command yet previews as the empty string', () => {
  assert.equal(cellPreview({ type: 'shell', command: '' }), '')
  assert.equal(cellPreview({ type: 'shell' }), '')
})

// Not cosmetic: CSS does the visible ellipsis at the pane's real width, which
// is the only place that width is known. This cap stops a huge draft's first
// line reaching the DOM for a cell folded specifically to stop looking at it.
test('an over-long line is capped at 120 characters', () => {
  const long = 'x'.repeat(500)
  assert.equal(cellPreview({ type: 'n4l', text: long }), 'x'.repeat(120))
  assert.equal(cellPreview({ type: 'graph', query: long }), 'x'.repeat(120))
})

test('a line of exactly 120 characters comes back whole and uncapped', () => {
  const exact = 'x'.repeat(120)
  assert.equal(cellPreview({ type: 'n4l', text: exact }), exact)
  assert.equal(cellPreview({ type: 'graph', query: exact }), exact)
})

test('formatDuration renders m:ss with a padded seconds field', () => {
  assert.equal(formatDuration(0), '0:00')
  assert.equal(formatDuration(7000), '0:07')
  assert.equal(formatDuration(60000), '1:00')
  assert.equal(formatDuration(605000), '10:05')
})

test('formatDuration treats junk as zero rather than rendering NaN', () => {
  assert.equal(formatDuration(undefined), '0:00')
  assert.equal(formatDuration(null), '0:00')
  assert.equal(formatDuration(NaN), '0:00')
  assert.equal(formatDuration(-5), '0:00')
})
