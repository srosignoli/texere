// Filters the sources sidebar by document title and extract text together.
//
// Kept out of the component on purpose: this repo has no Svelte component test
// harness, so anything left in a .svelte file has no tests. The component keeps
// only rendering and click handling.

/**
 * @param {Array<{id: string, title: string}>} readings
 * @param {Record<string, Array<{id: string, text: string, anchor: string}>>} extractsByReading
 * @param {string} query
 * @returns {Array<{reading: object, extracts: Array<object>, autoExpand: boolean}>}
 */
export function filterSources(readings, extractsByReading, query) {
  const list = Array.isArray(readings) ? readings : []
  const byReading = extractsByReading || {}
  const q = String(query ?? '').trim().toLowerCase()

  // No query: every document, trees closed. The caller's own expansion state
  // decides what is open, so nothing is forced here.
  if (!q) return list.map((reading) => ({ reading, extracts: [], autoExpand: false }))

  const out = []
  for (const reading of list) {
    const all = byReading[reading.id] ?? []
    const hits = all.filter((e) => String(e.text ?? '').toLowerCase().includes(q))
    const titleHit = String(reading.title ?? '').toLowerCase().includes(q)
    if (!titleHit && hits.length === 0) continue
    // Expand only when the reason it survived is inside the tree. A title match
    // has nothing to reveal, and forcing it open would bury the other results.
    out.push({ reading, extracts: hits, autoExpand: hits.length > 0 })
  }
  return out
}
