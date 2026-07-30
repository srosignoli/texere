// Builds the N4L text promoted from a shell cell: a fact about the command that
// produced the selected output. Pure functions, so they are unit-testable
// without mounting Svelte — the same reasoning media-fact.js records.
//
// Shape, matching buildMediaFact:
//
//   <subject>
//   "  (cmd) <command>
//   "  (out) <line>
//
// Arrows are referenced by short CODE, not long name. Declared in
// SSTconfig/arrows-EP-3.sst:
//   cmd = "has command"   out = "has output"
const COMMAND_ARROW = 'cmd'
const OUTPUT_ARROW = 'out'

// A promoted entry is a runbook line, not a log dump. 200 lines is generous for
// something a human will read, and it bounds what one click can append to a
// note file.
const MAX_LINES = 200

// Long enough for a real `ls -l` row or a stack frame, short enough that one
// runaway line cannot dominate the file.
const MAX_LINE = 400

// sanitizeTarget makes one line of shell output safe to use as the target of a
// ditto line.
//
// It is narrower than it looks. Because every target is preceded by the ditto
// prefix (`"  (out) `), it is never the first thing on a line — so N4L's
// leading-position markers (`#` comment, `"` ditto, `-` chapter, `::` section,
// `@` alias) are all ordinary text here and pass through untouched. This is the
// same argument media-fact.js makes for a leading `@`.
//
// Parentheses are the real hazard: `(...)` is arrow syntax WHEREVER it appears,
// and media-fact.js records a parenthesised timestamp being silently consumed
// by the parser and never reaching the graph. Square brackets keep the shape
// readable, which matters for a stack trace or a `docker ps` row — deliberately
// lossy rather than dropping the characters.
//
// Newlines are not handled here: the caller splits output into lines first, and
// one line is one fact.
export function sanitizeTarget(line) {
  let t = String(line ?? '').replace(/[\r\t\n]/g, ' ').trim()
  if (!t) return ''
  t = t.replace(/\(/g, '[').replace(/\)/g, ']')
  return t.length > MAX_LINE ? t.slice(0, MAX_LINE) : t
}

// buildShellFact returns the N4L for a promoted selection, or '' when there is
// nothing usable — as buildMediaFact returns '' without a subject. The caller
// inserts the result as an N4L draft cell, so the user reads and edits it
// before it ever reaches a file.
export function buildShellFact(input = {}) {
  const { subject, command, lines } = input || {}
  const subj = String(subject ?? '').trim()
  if (!subj) return ''

  const cmd = sanitizeTarget(command)
  const outs = (Array.isArray(lines) ? lines : [])
    .map(sanitizeTarget)
    .filter(Boolean)
    .slice(0, MAX_LINES)

  if (!cmd && outs.length === 0) return ''

  let out = `${subj}\n`
  if (cmd) out += `"  (${COMMAND_ARROW}) ${cmd}\n`
  for (const line of outs) out += `"  (${OUTPUT_ARROW}) ${line}\n`
  return out
}
