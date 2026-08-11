// Chart arithmetic for the Stats focus.
//
// Dependency-free and out of the component because this repo has no Svelte
// component test harness — anything left in a .svelte file has no tests. It
// also means every NaN guard below is actually covered.

/** Scale values so the tallest is exactly `height`. All-zero gives all zeros. */
export function barHeights(values, height) {
  const list = Array.isArray(values) ? values : []
  const max = Math.max(0, ...list)
  // Guard the divide rather than the render: a NaN height makes an SVG rect
  // vanish silently, which reads as "no data" instead of "bug".
  if (max <= 0) return list.map(() => 0)
  return list.map((v) => (Math.max(0, v) / max) * height)
}

/** An SVG path spanning the full width; '' for fewer than two points. */
export function sparkPath(values, width, height) {
  const list = Array.isArray(values) ? values : []
  if (list.length < 2) return ''
  const max = Math.max(0, ...list)
  const step = width / (list.length - 1)
  return list
    .map((v, i) => {
      const y = max > 0 ? height - (Math.max(0, v) / max) * height : height
      return `${i === 0 ? 'M' : 'L'}${round(i * step)} ${round(y)}`
    })
    .join(' ')
}

export function humanDuration(minutes) {
  const m = Math.max(0, Math.round(Number(minutes) || 0))
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  const rest = m % 60
  return rest ? `${h}h ${rest}m` : `${h}h`
}

/**
 * SuperMemo's target forgetting index is 7–13%: below it you are reviewing more
 * than you need to, above it you are losing material.
 */
export function fiBand(fi) {
  const v = Number(fi)
  if (!Number.isFinite(v) || v < 0) return 'unknown'
  if (v < 7) return 'low'
  if (v > 13) return 'high'
  return 'ok'
}

function round(n) {
  return Math.round(n * 100) / 100
}
