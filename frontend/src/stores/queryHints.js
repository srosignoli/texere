// Live-value cache + help-panel toggle for the graph query bars.
//
// queryHints holds the user's real chapters / context tags / arrow codes so
// the autocomplete dropdown can suggest them after the matching directive.
// The lists are fetched lazily (first time a bar is focused) and refreshed
// after a graph rebuild (Ctrl+G). On any binding error the lists stay empty
// and the dropdown silently falls back to keyword-only suggestions.
//
// queryHelpOpen is the shared open/closed state for the single QueryHelpPanel
// instance mounted in App.svelte; either query bar's "?" button toggles it.

import { writable, get } from 'svelte/store'
import { ListChapters, ListContextTags, ListArrowCodes } from '../../wailsjs/go/main/App'

/**
 * queryHints — { chapter: string[], context: string[], arrow: string[] }.
 * Keyed by the grammar module's `valueKind` so getSuggestions() can index it
 * directly via liveValues[kind].
 * @type {import('svelte/store').Writable<{chapter:string[], context:string[], arrow:string[]}>}
 */
export const queryHints = writable({ chapter: [], context: [], arrow: [] })

/** queryHelpOpen — true while the shared query help panel is visible. */
export const queryHelpOpen = writable(false)

// Guards so concurrent focus events don't fire overlapping fetches.
let loaded = false
let inFlight = null

/**
 * refreshQueryHints — (re)fetch all three live-value lists from the backend.
 * Safe to call repeatedly; coalesces concurrent calls. Resolves when done.
 */
export async function refreshQueryHints() {
  if (inFlight) return inFlight
  inFlight = (async () => {
    try {
      const [chapter, context, arrow] = await Promise.all([
        ListChapters().catch(() => []),
        ListContextTags().catch(() => []),
        ListArrowCodes().catch(() => []),
      ])
      queryHints.set({
        chapter: Array.isArray(chapter) ? chapter : [],
        context: Array.isArray(context) ? context : [],
        arrow: Array.isArray(arrow) ? arrow : [],
      })
      loaded = true
    } catch (_) {
      // Leave whatever we had; dropdown degrades to keywords-only.
    } finally {
      inFlight = null
    }
  })()
  return inFlight
}

/**
 * ensureQueryHints — fetch once on first use (e.g. a query bar gaining focus).
 * No-op after the first successful load; use refreshQueryHints() to force a
 * reload after the graph changes.
 */
export function ensureQueryHints() {
  if (loaded || inFlight) return
  refreshQueryHints()
}

/** Current snapshot of the live values, for non-reactive callers. */
export function currentQueryHints() {
  return get(queryHints)
}
