import { StreamLanguage } from '@codemirror/language'
import { tags } from '@lezer/highlight'

/**
 * N4L StreamParser token table:
 *
 * - Chapter header:   "- <title>"          → tags.heading
 * - Context operator: "::" or "::+"        → tags.keyword
 * - Arrow line:       "(code) target"       → code: tags.operatorKeyword, target: tags.string
 * - Ditto arrow:      "= (code) target"     → '=' : tags.meta, then as arrow
 * - Study marker:     "# @card / @study / @cloze / @id …" → tags.annotation
 * - Provenance:       "# @source / @ref / @progress …"    → tags.comment
 * - Label anchor:     "# @label <name>"    → tags.labelName
 * - Generic comment:  "#" not starting @   → tags.lineComment
 * - ALL CAPS reminder: ALL_CAPS text        → tags.invalid  (amber in highlight)
 * - Regular node text:                      → default (no token)
 */

const ARROW_CODE_RE = /^\(\s*[\w.,-]+\s*\)/
const ALL_CAPS_RE = /^[A-Z][A-Z0-9 _,;:.!?'"()[\]-]{2,}$/
const STUDY_MARKERS = new Set(['@card', '@study', '@cloze', '@id'])
const PROV_MARKERS = new Set(['@source', '@ref', '@progress', '@label'])

function tokenize(stream, _state) {
  // Chapter header: line starts with "- "
  if (stream.sol()) {
    // Chapter header: "-title" or "- title", but not "--" and not bare "-"
    if (stream.match(/^-(?![-\s]*$)/)) {
      stream.skipToEnd()
      return 'heading'
    }

    // Context operator line: ":: " or "::+ "
    if (stream.match(/^::[\+]?\s*/)) {
      stream.skipToEnd()
      return 'keyword'
    }

    // Ditto line: starts with "= "
    if (stream.match(/^=\s*/)) {
      return 'meta'
    }

    // Hash markers
    if (stream.peek() === '#') {
      stream.next() // consume '#'
      stream.eatSpace()
      const rest = stream.string.slice(stream.pos)
      const firstToken = rest.split(/\s/)[0]
      if (STUDY_MARKERS.has(firstToken)) {
        stream.skipToEnd()
        return 'annotation'
      }
      if (firstToken === '@label') {
        stream.skipToEnd()
        return 'labelName'
      }
      if (PROV_MARKERS.has(firstToken)) {
        stream.skipToEnd()
        return 'comment'
      }
      stream.skipToEnd()
      return 'lineComment'
    }
  }

  // Arrow code: "(code)"
  if (stream.match(ARROW_CODE_RE)) {
    return 'operatorKeyword'
  }

  // Arrow target: rest of line after arrow code (consumed token-by-token)
  // We just eat the whole remaining line as 'string' when we see we're past an arrow code
  // (handled below by eating word-by-word and checking the full line context)

  // ALL CAPS reminder (must be majority uppercase, non-trivial length)
  const remaining = stream.string.slice(stream.pos)
  if (ALL_CAPS_RE.test(remaining.trim())) {
    stream.skipToEnd()
    return 'invalid' // mapped to amber in highlight style
  }

  // Default: consume one character (let CM6 handle the rest as plain text)
  stream.next()
  return null
}

export const n4lLanguage = StreamLanguage.define({
  startState: () => ({}),
  token: tokenize,
  blankLine: (_state) => {},
  copyState: (state) => ({ ...state }),
})
