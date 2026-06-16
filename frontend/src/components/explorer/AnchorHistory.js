// Browser-style anchor-history state machine for the 2.5D Isometric
// Graph Explorer (US6 / FR-026a–g).
//
// Three operations:
//
//   pushAnchor(newID)
//     - Push the current anchor onto the back stack.
//     - Clear the forward stack (browser-style: navigating creates a
//       fresh forward branch).
//     - Set current = newID.
//     - No-op when newID === current (FR-025a).
//
//   back() / forward()
//     - Move the cursor through history; returns the new current ID
//     - or null at boundaries (FR-026e).
//
//   clear() — wipe the stacks. Called when a new query loads (FR-026f).
//
// The history is bounded at HISTORY_CAP entries (FR-026g) — older
// entries fall off the back stack when the cap is exceeded.
//
// State is a Svelte writable so any component can subscribe to changes
// (e.g. to enable/disable a back/forward button when canGoBack /
// canGoForward flips).

import { writable, derived, get } from 'svelte/store'

export const HISTORY_CAP = 50

/**
 * Create a fresh anchor-history controller bound to a Svelte store.
 *
 * @param {string|null} initialAnchor
 */
export function createAnchorHistory(initialAnchor = null) {
  /** @type {{back:string[],forward:string[],current:string|null}} */
  const initial = { back: [], forward: [], current: initialAnchor }
  const store = writable(initial)

  const canGoBack = derived(store, $h => $h.back.length > 0)
  const canGoForward = derived(store, $h => $h.forward.length > 0)

  /** Push a new anchor; returns true when the state actually changed. */
  function pushAnchor(newID) {
    if (!newID) return false
    let changed = false
    store.update(h => {
      if (h.current === newID) return h
      const back = h.current ? [...h.back, h.current] : [...h.back]
      // Cap the back stack — drop the oldest entries if we exceed.
      while (back.length > HISTORY_CAP) back.shift()
      changed = true
      return { back, forward: [], current: newID }
    })
    return changed
  }

  /** Move cursor backward; returns the new current ID, or null at the boundary. */
  function back() {
    let next = null
    store.update(h => {
      if (h.back.length === 0) return h
      const back = [...h.back]
      const prior = back.pop()
      const forward = h.current ? [h.current, ...h.forward] : [...h.forward]
      while (forward.length > HISTORY_CAP) forward.pop()
      next = prior
      return { back, forward, current: prior }
    })
    return next
  }

  /** Move cursor forward; returns the new current ID, or null at the boundary. */
  function forward() {
    let next = null
    store.update(h => {
      if (h.forward.length === 0) return h
      const forward = [...h.forward]
      const ahead = forward.shift()
      const back = h.current ? [...h.back, h.current] : [...h.back]
      while (back.length > HISTORY_CAP) back.shift()
      next = ahead
      return { back, forward, current: ahead }
    })
    return next
  }

  function clear(currentAfterClear = null) {
    store.set({ back: [], forward: [], current: currentAfterClear })
  }

  /** Replace the current anchor without affecting history (e.g. after
   *  Open returns a different default anchor than expected). */
  function setCurrent(id) {
    store.update(h => ({ ...h, current: id }))
  }

  return {
    subscribe: store.subscribe,
    canGoBack,
    canGoForward,
    pushAnchor,
    back,
    forward,
    clear,
    setCurrent,
    /** Read-only snapshot — handy for debugging / tests. */
    snapshot() { return get(store) },
  }
}
