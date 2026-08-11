// Readout formatting for the Stats focus.
//
// Dependency-free and out of the component because this repo has no Svelte
// component test harness — anything left in a .svelte file has no tests.

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
