// Chart.js config builders for the Stats focus.
//
// Deliberately does NOT import Chart.js: a config is a plain object, and
// keeping the import out is what lets these run under `node --test`. This repo
// has no Svelte component harness, so a config built inside the component would
// have no tests at all.

import { cssVar } from './css-var.js'

// Fallbacks for when getComputedStyle returns nothing (node --test has no real
// document) — same reasoning as css-var.js's own fallback constants. Also the
// values these two chart-chrome roles carry in Blue, so a themeless test
// environment renders exactly what Blue would.
const TICK_FALLBACK = '#8899aa'
const GRID_FALLBACK = '#2a3a4a'

// Fallbacks for the four chart-series tokens (final-fix review, feature 052
// — Important 1), same reasoning as TICK_FALLBACK/GRID_FALLBACK above: each
// is Blue's own literal for that series, and what cssVar() falls back to
// with no real document. Exported so StatsPanel.svelte's call sites can pass
// them to cssVar too, rather than duplicating the literal there.
export const CHART_BLUE_FALLBACK = '#7dd3fc'
export const CHART_PURPLE_FALLBACK = '#b9a3e3'
export const CHART_AMBER_FALLBACK = '#e0b000'
export const CHART_GREEN_FALLBACK = '#7ecfa0'

/**
 * @param {{labels?: string[], values?: number[], yLabel: string, unit: string, color?: string}} o
 */
export function barConfig(o) {
  return config('bar', o)
}

/** Same as barConfig but drawn as a line — used for the 84-day review series. */
export function lineConfig(o) {
  return config('line', o)
}

function config(type, { labels, values, yLabel, unit, color = cssVar('--chart-green', CHART_GREEN_FALLBACK) }) {
  // Read at call-time (not module load), same as Theme.js / css-var.js: a
  // config is built fresh on every mount (see StatsPanel.svelte's chart()
  // action comment), so this always reflects whichever theme is live when the
  // panel opens. TICK/GRID are chrome — axis ticks and gridlines, byte-
  // identical to --text-muted/--border's own Blue values, so reusing those
  // existing tokens is a visual no-op in Blue and a real fix in Light (a
  // #2a3a4a gridline is nearly black on a light canvas). `color` IS data — it
  // identifies which series this is — but the literal itself still needed a
  // Light override (final-fix review, Important 1): see style.css's
  // --chart-* comment for the contrast numbers and the solve method.
  const tick = cssVar('--text-muted', TICK_FALLBACK)
  const grid = cssVar('--border', GRID_FALLBACK)
  return {
    type,
    data: {
      labels: Array.isArray(labels) ? labels : [],
      datasets: [{
        data: Array.isArray(values) ? values : [],
        backgroundColor: color,
        borderColor: color,
        borderWidth: type === 'line' ? 1.5 : 0,
        pointRadius: 0,
        fill: false,
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: false,
      // Without this a line chart is almost unhoverable: pointRadius is 0 and
      // Chart.js defaults to intersect:true, so the hit target collapses onto
      // the stroke itself. Showing the value on hover is the whole point.
      interaction: { mode: 'index', intersect: false },
      plugins: {
        // One dataset per chart, so a legend would only repeat the card's title.
        legend: { display: false },
        tooltip: {
          callbacks: {
            // The reason this feature exists: a bar must say what it is worth.
            label: (ctx) => `${ctx.parsed.y} ${unit}`,
          },
        },
      },
      scales: {
        // No x title: the x labels are self-describing (dates, grade names),
        // and a title with no display and no text was dead config.
        x: {
          // Chart.js's autoSkip anchors at index 0 and steps forward, so at
          // realistic card widths it drops the FINAL tick — the weekly charts
          // labelled a week up to three buckets before their last bar, and the
          // forecast stopped at +12d with a fourteenth bar drawn past it.
          // Thinning from the end instead keeps the most recent bucket labelled,
          // which is the one a reader looks at first.
          ticks: { color: tick, maxRotation: 0, autoSkip: false, callback: thinFromEnd },
          grid: { color: grid },
        },
        y: {
          beginAtZero: true,
          ticks: { color: tick, precision: 0 },
          grid: { color: grid },
          title: { display: true, text: yLabel, color: tick },
        },
      },
    },
  }
}

// Show roughly six labels, counting back from the last so it is always one of
// them. `this` is the Chart.js scale, hence a function expression rather than
// an arrow.
function thinFromEnd(value, index, ticks) {
  const step = Math.max(1, Math.ceil(ticks.length / 6))
  return (ticks.length - 1 - index) % step === 0 ? this.getLabelForValue(value) : ''
}
