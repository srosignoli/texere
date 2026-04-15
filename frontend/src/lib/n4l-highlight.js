import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'

/**
 * N4L highlight style — maps StreamLanguage token names (via @lezer/highlight tags)
 * to CSS colour values.
 *
 * Colour palette (dark background #1b2636):
 *   heading          #7dd3fc  — light blue
 *   keyword          #c084fc  — purple (context operator ::)
 *   operatorKeyword  #67e8f9  — cyan  (arrow code)
 *   string           #86efac  — green (arrow target)
 *   meta             #94a3b8  — slate (ditto =)
 *   annotation       #fde68a  — pale amber (study markers)
 *   comment          #6b7280  — muted grey (provenance)
 *   labelName        #a78bfa  — violet (@label)
 *   lineComment      #4b5563  — dark grey (#comment)
 *   invalid          #f59e0b  — amber (ALL CAPS reminders)
 */
const n4lHighlightStyle = HighlightStyle.define([
  { tag: tags.heading,         color: '#7dd3fc', fontWeight: 'bold' },
  { tag: tags.keyword,         color: '#c084fc' },
  { tag: tags.operatorKeyword, color: '#67e8f9' },
  { tag: tags.string,          color: '#86efac' },
  { tag: tags.meta,            color: '#94a3b8' },
  { tag: tags.annotation,      color: '#fde68a' },
  { tag: tags.comment,         color: '#6b7280', fontStyle: 'italic' },
  { tag: tags.labelName,       color: '#a78bfa' },
  { tag: tags.lineComment,     color: '#4b5563', fontStyle: 'italic' },
  { tag: tags.invalid,         color: '#f59e0b', fontWeight: '600' },
])

export const n4lHighlighting = syntaxHighlighting(n4lHighlightStyle)
