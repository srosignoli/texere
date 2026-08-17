import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'

/**
 * N4L highlight style — maps StreamLanguage token names (via @lezer/highlight tags)
 * to CSS custom properties (`--n4l-syntax-*`, defined per-theme in style.css).
 *
 * The editor is themed through the CSS cascade, not JS: HighlightStyle.define
 * emits CSS text and passes var(...) straight through, so switching
 * `data-theme` on <html> retones every rule below with no rebuild here.
 *
 * Tag → role mapping (note the trap: CodeMirror's `comment` tag is N4L's
 * *provenance*, and `lineComment` is N4L's `# comment` — swap them and two
 * greys trade places with nothing looking obviously broken):
 *   heading          → --n4l-syntax-heading       (section heading)
 *   keyword          → --n4l-syntax-keyword       (context operator ::)
 *   operatorKeyword  → --n4l-syntax-arrow-code    (arrow code)
 *   string           → --n4l-syntax-arrow-target  (arrow target)
 *   meta             → --n4l-syntax-ditto         (ditto =)
 *   annotation       → --n4l-syntax-marker        (study markers)
 *   comment          → --n4l-syntax-provenance    (provenance)
 *   labelName        → --n4l-syntax-label         (@label)
 *   lineComment      → --n4l-syntax-comment       (# comment)
 *   invalid          → --n4l-syntax-invalid       (ALL CAPS reminders)
 */
const n4lHighlightStyle = HighlightStyle.define([
  { tag: tags.heading,         color: 'var(--n4l-syntax-heading)', fontWeight: 'bold' },
  { tag: tags.keyword,         color: 'var(--n4l-syntax-keyword)' },
  { tag: tags.operatorKeyword, color: 'var(--n4l-syntax-arrow-code)' },
  { tag: tags.string,          color: 'var(--n4l-syntax-arrow-target)' },
  { tag: tags.meta,            color: 'var(--n4l-syntax-ditto)' },
  { tag: tags.annotation,      color: 'var(--n4l-syntax-marker)' },
  { tag: tags.comment,         color: 'var(--n4l-syntax-provenance)', fontStyle: 'italic' },
  { tag: tags.labelName,       color: 'var(--n4l-syntax-label)' },
  { tag: tags.lineComment,     color: 'var(--n4l-syntax-comment)', fontStyle: 'italic' },
  { tag: tags.invalid,         color: 'var(--n4l-syntax-invalid)', fontWeight: '600' },
])

export { n4lHighlightStyle }
export const n4lHighlighting = syntaxHighlighting(n4lHighlightStyle)
