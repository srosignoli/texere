import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildSystem, skillIdForCell } from './llm-prompt.js'

test('the system text leads, and stands alone when there is no context', () => {
  assert.equal(buildSystem('You are a tutor.', null, []), 'You are a tutor.')
})

test('an extract, tags, neighbours and source each become their own block', () => {
  const got = buildSystem('S', {
    extractText: 'the extract',
    tags: ['latin', 'grammar'],
    orbit: [{ arrowLong: 'means', neighborText: 'a thing' }],
    source: { passage: 'the passage' },
  }, [])
  assert.ok(got.startsWith('S\n\n--- Context ---\n'), got)
  assert.ok(got.includes('# Extract\nthe extract'), got)
  assert.ok(got.includes('# Tags\nlatin, grammar'), got)
  assert.ok(got.includes('# Neighbours\n- means a thing'), got)
  assert.ok(got.includes('# Source passage\nthe passage'), got)
})

test('a context block is absent when its data is missing', () => {
  const got = buildSystem('S', { extractText: 'x' }, [])
  assert.ok(got.includes('# Extract'), got)
  assert.ok(!got.includes('# Tags'), got)
  assert.ok(!got.includes('# Neighbours'), got)
  assert.ok(!got.includes('# Source passage'), got)
})

test('an attached N4L draft is included', () => {
  const got = buildSystem('S', null, [{ type: 'n4l', text: 'a (means) b' }])
  assert.ok(got.includes('# Attached N4L draft\na (means) b'), got)
})

test('an attached conversation is flattened to role: content lines', () => {
  const got = buildSystem('S', null, [{
    type: 'llm', messages: [{ role: 'user', content: 'q' }, { role: 'assistant', content: 'a' }],
  }])
  assert.ok(got.includes('# Attached conversation\nuser: q\nassistant: a'), got)
})

test('an attached graph result is included', () => {
  const got = buildSystem('S', null, [{ type: 'graph', resultText: 'node -> node' }])
  assert.ok(got.includes('# Attached graph result\nnode -> node'), got)
})

test('an attached compute cell shows its code fenced and its output', () => {
  const got = buildSystem('S', null, [{ type: 'compute', code: 'print(1)', resultText: '1' }])
  assert.ok(got.includes('```js\nprint(1)\n```'), got)
  assert.ok(got.includes('Output:\n1'), got)
})

test('an empty attachment contributes nothing', () => {
  assert.equal(buildSystem('S', null, [{ type: 'n4l', text: '' }]), 'S')
  assert.equal(buildSystem('S', null, [{ type: 'llm', messages: [] }]), 'S')
})

test('a missing attachment list is tolerated', () => {
  assert.equal(buildSystem('S', null, undefined), 'S')
})

// --- migration ---
// cell.mode is what notebooks saved before this change carry. Mapping it at
// READ time means no rewrite pass and no risk to existing files.

test('a cell with an explicit skill uses it', () => {
  assert.equal(skillIdForCell({ skill: 'exam-drill', mode: 'tutor' }), 'exam-drill')
})

test('a legacy mode maps to the skill that replaced it', () => {
  assert.equal(skillIdForCell({ mode: 'tutor' }), 'socratic-tutor')
  assert.equal(skillIdForCell({ mode: 'ask' }), 'thinking-partner')
})

test('an unknown or absent mode falls back to the default skill', () => {
  assert.equal(skillIdForCell({ mode: 'wat' }), 'thinking-partner')
  assert.equal(skillIdForCell({}), 'thinking-partner')
  assert.equal(skillIdForCell(null), 'thinking-partner')
  assert.equal(skillIdForCell(undefined), 'thinking-partner')
})
