import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8')

// The study-session path is driven end to end by scripted demo recordings
// (docs/demos/). Those scripts address controls by accessible name, so the
// names below are load-bearing: renaming one breaks a recording silently.
//
// They are also the plain accessibility win — every control here is currently
// identified only by a glyph, a count-suffixed label, or nothing at all.

test('the study tabs are addressable independently of their due-count badge', () => {
  const src = read('../components/StudySurface.svelte')
  // The visible text is `Read` plus a <sup>5</sup> badge, so a text selector
  // matches "Read5" once anything is due. An aria-label is stable.
  assert.match(src, /class="ss-tab"[\s\S]{0,200}aria-label=\{label\}/,
    'ss-tab carries no aria-label derived from its label')
})

test('the three × close buttons have distinct accessible names', () => {
  const cases = [
    ['../components/StudySurface.svelte', 'ss-close'],
    ['../components/StudyMode.svelte', 'study-close'],
    ['../components/ReadingView.svelte', 'rb-close'],
  ]
  const names = []
  for (const [file, cls] of cases) {
    const src = read(file)
    const line = src.split('\n').find((l) => l.includes(`class="${cls}"`))
    assert.ok(line, `${cls} not found in ${file}`)
    const m = line.match(/aria-label="([^"]+)"/)
    assert.ok(m, `${cls} in ${file} has no aria-label — three × buttons are indistinguishable`)
    names.push(m[1])
  }
  assert.equal(new Set(names).size, names.length,
    `close buttons share an accessible name: ${names.join(', ')}`)
})

test('the two Study dialogs do not share one accessible name', () => {
  const a = read('../components/StudySurface.svelte').match(/role="dialog"\s+aria-label="([^"]+)"/)
  const b = read('../components/StudyMode.svelte').match(/role="dialog"\s+aria-label="([^"]+)"/)
  assert.ok(a && b, 'both study roots should keep role="dialog" with an aria-label')
  assert.notEqual(a[1], b[1],
    `both dialogs are named "${a[1]}" — a script cannot tell them apart`)
})

test('the process actions that build a card are addressable', () => {
  const src = read('../components/ProcessEditor.svelte')
  for (const label of ['Create card', 'Make card', 'Cloze', 'Dismiss']) {
    assert.match(src, new RegExp(`aria-label="${label}"`),
      `ProcessEditor has no button with aria-label="${label}"`)
  }
})

test('the recall grade buttons are addressable by grade name', () => {
  const src = read('../components/RecallCard.svelte')
  assert.match(src, /aria-label=\{[^}]*g\.label/,
    'grade buttons carry no aria-label derived from the grade')
  assert.match(src, /aria-label="Show answer"/,
    'the reveal button has no aria-label')
})
