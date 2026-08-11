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
        x: {
          ticks: { color: TICK, maxRotation: 0, autoSkip: true },
          grid: { color: GRID },
          title: { color: TICK },
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
