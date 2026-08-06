// Builds the system prompt that turns a natural-language question into a
// searchN4L query, and checks what comes back.
//
// The grammar is NOT restated here — it is rendered from query-grammar.js,
// which already drives the autocomplete and the help panel and is grounded in
// sstorytime-kv source. A hand-written copy would be a third description of the
// same language, drifting from the other two without anything failing.
//
// Dependency-free on purpose: this repo has no Svelte component harness, and a
// module importing wailsjs cannot be loaded by Node at all, so logic left in a
// .svelte file has no tests.

import { DIRECTIVES, MATCHING, COMBINING } from './query-grammar.js'

// How many live values of each kind to show. Bounded because a large collection
// has hundreds of context tags and the prompt would dwarf the question.
const HINT_CAP = 40

const KNOWN = new Set(DIRECTIVES.flatMap((d) => [d.token, ...d.aliases]))

function hintBlock(label, values) {
  if (!Array.isArray(values) || values.length === 0) return ''
  const shown = values.slice(0, HINT_CAP)
  // Say so when truncated: otherwise the model infers that an unlisted chapter
  // does not exist and avoids a perfectly good one.
  const head = values.length > shown.length
    ? `Some of the ${label} in this collection:`
    : `The ${label} in this collection:`
  return `${head}\n${shown.map((v) => `  ${v}`).join('\n')}\n\n`
}

// Mirrors hintBlock: omit the heading entirely rather than print "Worked
// examples:" over nothing, which happens whenever ListQueryExamples rejects.
function examplesBlock(examples) {
  if (!Array.isArray(examples) || examples.length === 0) return ''
  const worked = examples
    .map((e) => `  Q: ${e.question}\n  A: ${e.query}`)
    .join('\n')
  return `Worked examples:\n${worked}\n\n`
}

export function buildQueryPrompt(examples, hints) {
  const h = hints || {}
  const directives = DIRECTIVES.map((d) => {
    const alias = d.aliases.length ? ` (also ${d.aliases.join(', ')})` : ''
    const arg = d.arg ? ` ${d.arg}` : ''
    return `  ${d.token}${arg}${alias}\n    ${d.summary}\n    e.g. ${d.example}`
  }).join('\n')

  const matching = MATCHING.map((m) => `  ${m.title}: ${m.body}`).join('\n')

  return [
    'You translate a natural-language question into a single search query for an',
    'N4L/SSTorytime knowledge graph.',
    '',
    'These are the ONLY directives that work. Anything else is silently ignored',
    'and returns nothing, so never invent one:',
    directives,
    '',
    'How text matches:',
    matching,
    '',
    `How they combine: ${COMBINING.rule}`,
    `  e.g. ${COMBINING.example} — ${COMBINING.explanation}`,
    '',
    examplesBlock(examples) +
    hintBlock('chapters', h.chapter) +
    hintBlock('context tags', h.context) +
    hintBlock('arrow codes', h.arrow) +
    'Prefer a name from those lists over inventing one.',
    '',
    'Reply with the query string and nothing else — no explanation, no quotes',
    'around the whole query, no code fences.',
  ].join('\n')
}

// Models wrap output in fences or prefix it with a label despite instructions.
// Without this the "query only" contract holds exactly as long as the model
// chooses to cooperate.
export function stripToQuery(raw) {
  let s = String(raw ?? '').trim()
  const fence = s.match(/^```[a-zA-Z]*\n([\s\S]*?)\n?```$/)
  if (fence) s = fence[1].trim()
  if (s.startsWith('`') && s.endsWith('`') && s.length > 1) s = s.slice(1, -1).trim()
  s = s.replace(/^query:\s*/i, '')
  return s.trim()
}

// Checks vocabulary, not arity: it will not catch an argument passed to a flag
// that takes none. It exists to stop upstream-only syntax reaching the bar.
//
// Backslashes inside a quoted phrase are skipped. A directive is never inside
// quotes, and search text legitimately contains them — integral.n4l in the
// corpus holds `"Evaluate \(I(\nu) = \int_0^\infty\)"`, which would otherwise be
// refused with \nu and \int reported as unknown directives. Refusing a working
// query is worse than letting an odd token through: a bad query costs a retry,
// a false refusal blocks the user and tells them something untrue.
export function validateQuery(text) {
  const s = String(text ?? '')
  // Blank out double-quoted spans, preserving length so nothing else shifts.
  const outsideQuotes = s.replace(/"[^"]*"/g, (m) => ' '.repeat(m.length))
  const unknown = []
  for (const m of outsideQuotes.matchAll(/\\[a-zA-Z]+/g)) {
    if (!KNOWN.has(m[0]) && !unknown.includes(m[0])) unknown.push(m[0])
  }
  return { ok: unknown.length === 0, unknown }
}
