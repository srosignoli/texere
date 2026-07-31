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
import { sanitizeSubject, sanitizeTarget } from './n4l-escape.js'

const COMMAND_ARROW = 'cmd'
const OUTPUT_ARROW = 'out'

// A promoted entry is a runbook line, not a log dump. 200 lines is generous for
// something a human will read, and it bounds what one click can append to a
// note file.
const MAX_LINES = 200

// buildShellFact returns the N4L for a promoted selection, or '' when there is
// nothing usable — as buildMediaFact returns '' without a subject. The caller
// inserts the result as an N4L draft cell, so the user reads and edits it
// before it ever reaches a file.
export function buildShellFact(input = {}) {
  const { subject, command, lines } = input || {}
  const subj = sanitizeSubject(subject)
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
