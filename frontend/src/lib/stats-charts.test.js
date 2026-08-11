import { test } from 'node:test'
import assert from 'node:assert/strict'
import { barConfig, lineConfig } from './stats-charts.js'

const base = { labels: ['Mon', 'Tue'], values: [3, 7], yLabel: 'reviews', unit: 'reviews', color: '#7ecfa0' }

test('barConfig carries the data and the axis title', () => {
  const c = barConfig(base)
  assert.equal(c.type, 'bar')
  assert.deepEqual(c.data.labels, ['Mon', 'Tue'])
  assert.deepEqual(c.data.datasets[0].data, [3, 7])
  assert.equal(c.options.scales.y.title.text, 'reviews')
  assert.equal(c.options.scales.y.title.display, true)
})

test('lineConfig differs from barConfig only in type', () => {
  const b = barConfig(base)
  const l = lineConfig(base)
  assert.equal(l.type, 'line')
  assert.equal(b.type, 'bar')
  assert.deepEqual(l.data.labels, b.data.labels)
  assert.equal(l.options.scales.y.title.text, b.options.scales.y.title.text)
})

// The whole point of the change: hovering a bar must say what it is.
test('the tooltip renders the value with its unit', () => {
  const c = barConfig({ ...base, unit: 'reviews' })
  const label = c.options.plugins.tooltip.callbacks.label
  assert.equal(label({ parsed: { y: 12 } }), '12 reviews')
})

test('a singular value still reads correctly', () => {
  const c = barConfig({ ...base, unit: 'minutes' })
  assert.equal(c.options.plugins.tooltip.callbacks.label({ parsed: { y: 1 } }), '1 minutes')
})

// An empty collection must still produce axes, or the reader sees a blank box
// with no clue what it would have shown.
test('an empty series still produces a valid config with its axis title', () => {
  const c = barConfig({ labels: [], values: [], yLabel: 'cards', unit: 'cards', color: '#7ecfa0' })
  assert.deepEqual(c.data.labels, [])
  assert.deepEqual(c.data.datasets[0].data, [])
  assert.equal(c.options.scales.y.title.text, 'cards')
})

test('missing arrays degrade to empty rather than throwing', () => {
  const c = barConfig({ yLabel: 'cards', unit: 'cards' })
  assert.deepEqual(c.data.labels, [])
  assert.deepEqual(c.data.datasets[0].data, [])
})

// Chart.js defaults to dark-on-light. Without this every label is invisible
// against the panel, which reads as a missing axis rather than a styling bug.
test('the dark palette reaches ticks, grid and axis titles', () => {
  const c = barConfig(base)
  for (const axis of ['x', 'y']) {
    assert.equal(c.options.scales[axis].ticks.color, '#8899aa', `${axis} ticks`)
    assert.equal(c.options.scales[axis].grid.color, '#2a3a4a', `${axis} grid`)
  }
  assert.equal(c.options.scales.y.title.color, '#8899aa', 'y title')
})

test('the y axis starts at zero so bar heights stay comparable', () => {
  assert.equal(barConfig(base).options.scales.y.beginAtZero, true)
})

test('the legend is off — one dataset needs no key', () => {
  assert.equal(barConfig(base).options.plugins.legend.display, false)
})

// A line's only visible mark is its stroke — pointRadius is 0 and fill is off.
// Without a border width it renders as a blank box, which is exactly the
// "I can't tell what this shows" failure this change exists to fix.
test('a line is drawn with a visible stroke and a bar is not', () => {
  assert.ok(lineConfig(base).data.datasets[0].borderWidth > 0, 'a line needs a stroke')
  assert.equal(barConfig(base).data.datasets[0].borderWidth, 0, 'a bar is filled, not stroked')
})

// Hovering must land without pixel-hunting along the stroke.
test('hover targets the whole column, not just the drawn mark', () => {
  const i = lineConfig(base).options.interaction
  assert.equal(i.intersect, false)
  assert.equal(i.mode, 'index')
})

// Chart.js's own autoSkip anchors at index 0, so at realistic card widths it
// drops the LAST tick — the weekly charts labelled a bucket up to three weeks
// before their final bar. The most recent bucket is the one a reader looks at
// first, so labels are thinned from the end instead.
const scale = { getLabelForValue: (v) => `L${v}` }

test('the final x tick is always labelled', () => {
  const cb = barConfig(base).options.scales.x.ticks.callback
  for (const n of [4, 12, 14, 84]) {
    const ticks = new Array(n)
    assert.equal(cb.call(scale, 'last', n - 1, ticks), 'Llast', `${n} ticks`)
  }
})

test('intermediate x ticks are thinned to about six labels', () => {
  const cb = barConfig(base).options.scales.x.ticks.callback
  const ticks = new Array(12)
  const shown = []
  for (let i = 0; i < 12; i++) if (cb.call(scale, i, i, ticks) !== '') shown.push(i)
  assert.ok(shown.length <= 7, `showed ${shown.length} labels, want about six`)
  assert.ok(shown.includes(11), 'the last index must be among them')
})

test('a short series labels every tick', () => {
  const cb = barConfig(base).options.scales.x.ticks.callback
  const ticks = new Array(4)
  for (let i = 0; i < 4; i++) assert.notEqual(cb.call(scale, i, i, ticks), '', `index ${i}`)
})
