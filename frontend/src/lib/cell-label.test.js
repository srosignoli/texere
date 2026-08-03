import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cellLabel } from './cell-label.js'

test('each stacked cell type gets the name of the button that creates it', () => {
  assert.equal(cellLabel({ type: 'n4l' }), 'N4L draft')
  assert.equal(cellLabel({ type: 'graph' }), 'Graph')
  assert.equal(cellLabel({ type: 'sketch' }), 'Sketch')
  assert.equal(cellLabel({ type: 'audio' }), 'Audio')
  // The button reads "+ Scan", so the header must too — not "Image".
  assert.equal(cellLabel({ type: 'image' }), 'Scan')
})

test('a detail is appended where the type has meaningful variants', () => {
  // llm resolves mode through skillIdForCell, not by echoing it raw — see the
  // dedicated legacy-cell tests below for why.
  assert.equal(cellLabel({ type: 'llm', mode: 'tutor' }), 'LLM · socratic-tutor')
  assert.equal(cellLabel({ type: 'llm', mode: 'ask' }), 'LLM · thinking-partner')
  assert.equal(cellLabel({ type: 'compute', language: 'js' }), 'Compute · js')
})

// The whole point: three stacked Media cells must not read identically.
test('a media cell names what it actually holds', () => {
  assert.equal(cellLabel({ type: 'media', kind: 'image' }), 'Media · image')
  assert.equal(cellLabel({ type: 'media', kind: 'video' }), 'Media · video')
  assert.equal(cellLabel({ type: 'media', kind: 'audio' }), 'Media · audio')
})

// "pronunciation" is the word a language learner scans for; "audio" is not.
test('a pronunciation is named as one, not as audio', () => {
  assert.equal(
    cellLabel({ type: 'media', kind: 'audio', provider: 'pronunciation' }),
    'Media · pronunciation')
  assert.equal(
    cellLabel({ type: 'media', kind: 'audio', provider: 'commons-audio' }),
    'Media · audio')
})

// Media cells saved before the audio slice have no `kind`. A dangling
// separator would look like a rendering bug.
//
// llm is excluded here: skillIdForCell always resolves to a real skill id
// (falling back to DEFAULT_SKILL_ID), so an llm cell always has a detail —
// see the dedicated tests below for that behaviour.
test('a missing detail appends nothing, not a dangling separator', () => {
  assert.equal(cellLabel({ type: 'media' }), 'Media')
  assert.equal(cellLabel({ type: 'compute' }), 'Compute')
})

test('an unknown type stays identifiable rather than blank', () => {
  assert.equal(cellLabel({ type: 'widget' }), 'widget')
  // context is vestigial in cellRegistry and never stacked, but if one ever
  // appeared it must still render something.
  assert.equal(cellLabel({ type: 'context' }), 'context')
})

test('a shell cell is named, with the chosen shell as its detail', () => {
  assert.equal(cellLabel({ type: 'shell', shellLabel: 'Ubuntu (WSL)' }), 'Shell · Ubuntu (WSL)')
  assert.equal(cellLabel({ type: 'shell', shellLabel: 'bash' }), 'Shell · bash')
})

// A cell created before a shell was picked must still name itself.
test('a shell cell with no shell chosen shows no dangling separator', () => {
  assert.equal(cellLabel({ type: 'shell' }), 'Shell')
  assert.equal(cellLabel({ type: 'shell', shellLabel: '' }), 'Shell')
})

// Modern cell: skillName is persisted at pick time (see LLMCell.svelte's
// selectSkill), so it is used as-is — the friendliest text available.
test('an LLM cell is labelled with its skill name', () => {
  assert.equal(cellLabel({ type: 'llm', skillName: 'Socratic tutor' }), 'LLM · Socratic tutor')
})

// A notebook saved before skills existed carries `mode`, not `skill`/
// `skillName` — and opening it must not write to it (no migration on read).
// LLMCell.svelte's picker resolves the cell's skill via skillIdForCell and
// shows the resolved skill's friendly name (e.g. "Socratic tutor" for
// mode: 'tutor'). The header must resolve through the SAME function, so it
// can never show a different skill than the one the picker has selected.
// Echoing the raw legacy `mode` string instead — the previous behaviour —
// produced "LLM · tutor" beside a picker reading "Socratic tutor": two labels
// for one cell, permanently disagreeing.
test('a legacy LLM cell resolves its mode through skillIdForCell, matching the picker', () => {
  assert.equal(cellLabel({ type: 'llm', mode: 'tutor' }), 'LLM · socratic-tutor')
  assert.equal(cellLabel({ type: 'llm', mode: 'ask' }), 'LLM · thinking-partner')
})

// A cell with neither skillName nor mode still resolves through
// skillIdForCell's own default (DEFAULT_SKILL_ID), which is exactly what the
// picker's activeSkill would resolve to for the same cell — so even this
// corner case cannot disagree with the picker.
test('an LLM cell with neither falls back to skillIdForCell\'s default', () => {
  assert.equal(cellLabel({ type: 'llm' }), 'LLM · thinking-partner')
})

test('the header is never empty', () => {
  assert.equal(cellLabel({}), 'Cell')
  assert.equal(cellLabel({ type: '' }), 'Cell')
  assert.equal(cellLabel(null), 'Cell')
  assert.equal(cellLabel(undefined), 'Cell')
})
