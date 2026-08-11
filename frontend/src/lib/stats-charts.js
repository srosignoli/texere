// Chart.js config builders for the Stats focus.
//
// Deliberately does NOT import Chart.js: a config is a plain object, and
// keeping the import out is what lets these run under `node --test`. This repo
// has no Svelte component harness, so a config built inside the component would
// have no tests at all.

const TICK = '#8899aa'
const GRID = '#2a3a4a'

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

function config(type, { labels, values, yLabel, unit, color = '#7ecfa0' }) {
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
          ticks: { color: TICK, maxRotation: 0, autoSkip: false, callback: thinFromEnd },
          grid: { color: GRID },
        },
        y: {
          beginAtZero: true,
          ticks: { color: TICK, precision: 0 },
          grid: { color: GRID },
          title: { display: true, text: yLabel, color: TICK },
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
