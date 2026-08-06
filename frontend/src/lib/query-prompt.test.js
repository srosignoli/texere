import { test } from 'node:test'
import assert from 'node:assert/strict'
import { DIRECTIVES } from './query-grammar.js'
import { buildQueryPrompt, stripToQuery, validateQuery } from './query-prompt.js'

const EX = [{ question: 'what about pods?', query: 'pod' }]
const HINTS = { chapter: ['Kubernetes notes'], context: ['pods'], arrow: ['leadsto'] }

test('the prompt teaches every directive we execute', () => {
  const p = buildQueryPrompt(EX, HINTS)
  for (const d of DIRECTIVES) assert.ok(p.includes(d.token), `missing ${d.token}`)
})

// The whole point of confining the model: upstream syntax we do not route must
// never appear, or it emits queries that parse, return nothing, and look
// exactly like "no results".
test('the prompt never mentions syntax texere cannot execute', () => {
  const p = buildQueryPrompt(EX, HINTS)
  for (const bad of ['\\theme', '\\bookmarks', '\\dna', '\\terms', '\\concepts']) {
    assert.ok(!p.includes(bad), `prompt leaks ${bad}`)
  }
})

test('the prompt carries the examples and the live vocabulary', () => {
  const p = buildQueryPrompt(EX, HINTS)
  assert.ok(p.includes('what about pods?'))
  assert.ok(p.includes('Kubernetes notes'))
  assert.ok(p.includes('pods'))
  assert.ok(p.includes('leadsto'))
})

test('long hint lists are capped and the truncation is stated', () => {
  const many = Array.from({ length: 100 }, (_, i) => `chapter-${i}`)
  const p = buildQueryPrompt(EX, { chapter: many, context: [], arrow: [] })
  assert.ok(p.includes('chapter-0'))
  assert.ok(!p.includes('chapter-99'), 'list was not capped')
  assert.match(p, /some of/i, 'truncation must be stated or the model infers the rest do not exist')
})

test('empty hint lists are omitted, not rendered as bare headings', () => {
  const p = buildQueryPrompt(EX, { chapter: [], context: [], arrow: [] })
  assert.ok(!/chapters:\s*\n\s*\n/i.test(p))
})

// Happens whenever ListQueryExamples rejects: examples comes back [] and the
// prompt must not tell the model to study examples that are not there.
test('an empty example list omits the Worked examples heading', () => {
  const p = buildQueryPrompt([], HINTS)
  assert.ok(!p.includes('Worked examples:'))
})

test('stripToQuery unwraps what models actually return', () => {
  assert.equal(stripToQuery('pod \\chapter Kubernetes'), 'pod \\chapter Kubernetes')
  assert.equal(stripToQuery('```\npod\n```'), 'pod')
  assert.equal(stripToQuery('```text\npod\n```'), 'pod')
  assert.equal(stripToQuery('`pod`'), 'pod')
  assert.equal(stripToQuery('Query: pod'), 'pod')
  assert.equal(stripToQuery('   pod   \n'), 'pod')
})

test('validateQuery accepts every canonical token and alias', () => {
  for (const d of DIRECTIVES) {
    assert.ok(validateQuery(`${d.token} x`).ok, `rejected ${d.token}`)
    for (const a of d.aliases) assert.ok(validateQuery(`${a} x`).ok, `rejected alias ${a}`)
  }
})

test('validateQuery rejects upstream-only directives and names them', () => {
  const r = validateQuery('\\dna \\chapter NDA')
  assert.equal(r.ok, false)
  assert.deepEqual(r.unknown, ['\\dna'])
  assert.deepEqual(validateQuery('\\theme dark \\bookmarks').unknown, ['\\theme', '\\bookmarks'])
})

test('a query with no directives at all is valid', () => {
  assert.ok(validateQuery('recipe fish soup').ok)
  assert.ok(validateQuery('"thank you"').ok)
})

// integral.n4l in the corpus contains exactly this. Refusing it would block a
// working query and blame LaTeX for being a directive.
test('backslashes inside a quoted phrase are not directives', () => {
  const r = validateQuery('"Evaluate \\(I(\\nu) = \\int_0^\\infty\\)"')
  assert.ok(r.ok, `falsely refused: ${JSON.stringify(r.unknown)}`)
})

test('an unsupported directive is still caught outside quotes', () => {
  assert.deepEqual(validateQuery('\\dna \\chapter "a \\nu b"').unknown, ['\\dna'])
})

test('a directive before a quoted phrase is still validated', () => {
  assert.ok(validateQuery('\\notes \\context "differentiate under integral"').ok)
  assert.deepEqual(validateQuery('\\theme dark "quoted \\dna"').unknown, ['\\theme'])
})

// A model wrapping its whole reply in quotes would otherwise slip anything past
// the check, since quoted spans are blanked before scanning. The prompt's own
// closing line asks the model not to, which is why this is worth guarding.
test('a whole-reply-quoted answer is still validated', () => {
  const r = validateQuery('"\\dna \\chapter NDA"')
  assert.equal(r.ok, false)
  assert.deepEqual(r.unknown, ['\\dna'])
})

// ...while a legitimate whole-query phrase stays valid. It has no backslash, so
// looking inside it costs nothing.
test('a whole-query phrase is unaffected', () => {
  assert.ok(validateQuery('"thank you"').ok)
  assert.ok(validateQuery('"differentiate under integral"').ok)
})

// The LaTeX case must keep working: the quotes there are not the whole reply.
test('quoted LaTeX inside a larger query is still allowed', () => {
  assert.ok(validateQuery('\\notes \\context "Evaluate \\(I(\\nu)\\)"').ok)
})
