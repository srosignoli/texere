// Splitting a note string into prose and LaTeX.
//
// Delimiters are `\(…\)` for inline and `\[…\]` for display, and nothing else.
// In particular `$…$` is NOT math: `$` introduces a variable in N4L, and every
// one of the 89 `$` occurrences in the real collection is a variable reference
// like `$user1.1`. Supporting the conventional dollar delimiter would corrupt
// all of them.
//
// Dependency-free — it must never import KaTeX — so it runs under `node --test`,
// which is the only automated coverage this feature can have.

const OPENERS = [
  { open: '\\(', close: '\\)', type: 'inline' },
  { open: '\\[', close: '\\]', type: 'display' },
]

/**
 * @param {string} text
 * @returns {{type: 'text'|'inline'|'display', value: string}[]}
 */
export function splitMath(text) {
  const s = String(text ?? '')
  if (!s) return []

  const out = []
  let prose = ''
  let i = 0

  while (i < s.length) {
    const d = OPENERS.find((o) => s.startsWith(o.open, i))
    if (!d) {
      prose += s[i]
      i++
      continue
    }
    const end = s.indexOf(d.close, i + d.open.length)
    if (end < 0) {
      // No closer: the opener is ordinary text. Consuming to end-of-string here
      // would swallow the rest of the note, which is the failure this guards.
      prose += s[i]
      i++
      continue
    }
    if (prose) { out.push({ type: 'text', value: prose }); prose = '' }
    out.push({ type: d.type, value: s.slice(i + d.open.length, end) })
    i = end + d.close.length
  }

  if (prose) out.push({ type: 'text', value: prose })
  return out
}
