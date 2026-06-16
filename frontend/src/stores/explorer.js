// Svelte stores backing the 2.5D Isometric Graph Explorer (feature 006).
//
// Boundaries:
//   - explorerOpen / currentLayout / currentResultID / currentAnchorID — the
//     current explorer session state. Cleared on close.
//   - anchorHistory — browser-style back/forward stack for the active result.
//     Implementation lives here (foundational); the back/forward state machine
//     defined in data-model.md §2.5 is wired up in US6 (T046).
//
// Wails bindings: the foundational phase calls window.go.main.App.ExplorerOpen
// at runtime instead of importing from frontend/wailsjs/go/main/App.js. Wails
// regenerates the bindings file on the next `wails dev` / `wails build`, but
// the runtime-injected global is available regardless of regeneration state.

import { writable, derived } from 'svelte/store'

/** explorerOpen — true while the explorer surface is mounted. */
export const explorerOpen = writable(false)

/**
 * explorerQuery — the active query string passed to ExplorerOpen.
 * Anyone can update it (launcher, omnibar Enter, search-driven re-anchor)
 * and the GraphExplorer component re-renders.
 *
 * @type {import('svelte/store').Writable<string>}
 */
export const explorerQuery = writable('')

/**
 * currentLayout — the most recent LayoutResult returned by ExplorerOpen or
 * ExplorerReanchor. Shape matches contracts/layout-types.md.
 * @type {import('svelte/store').Writable<any|null>}
 */
export const currentLayout = writable(null)

/** currentResultID — opaque ID of the active ResultSet (Wails-side cache key). */
export const currentResultID = writable(/** @type {string|null} */(null))

/** currentAnchorID — wire ID of the active anchor node. */
export const currentAnchorID = writable(/** @type {string|null} */(null))

/**
 * anchorHistory — session-scoped browser-style stack. The state machine
 * lives in components/explorer/AnchorHistory.js; this writable mirrors
 * its current snapshot for components that need to react to its state
 * (e.g. enabling/disabling Back/Forward affordances). GraphExplorer
 * pushes snapshots into here whenever the controller mutates.
 *
 * @type {import('svelte/store').Writable<{back: string[], forward: string[], current: string|null}>}
 */
export const anchorHistory = writable({ back: [], forward: [], current: null })

/**
 * pendingReanchorID — one-shot signal from external sources (e.g. the
 * omnibar's in-result Enter dispatch) asking the explorer to re-anchor
 * to a specific node ID without going through ExplorerOpen. The
 * GraphExplorer component consumes the value and resets it to null.
 *
 * @type {import('svelte/store').Writable<string|null>}
 */
export const pendingReanchorID = writable(/** @type {string|null} */(null))

/**
 * layoutMode — 'extended' (default; all nodes share a single Y=0 plane
 * with contexts arranged as clusters in a centered X-row) or 'stacked'
 * (per-context floors stacked along Y). Changes go through ExplorerReanchor
 * so the search isn't re-run.
 * @type {import('svelte/store').Writable<'stacked' | 'extended'>}
 */
export const layoutMode = writable('extended')

/**
 * layerSpacing — world-Y distance between adjacent context floors.
 * Bound to the Layer Spacing slider in ExplorerSidebar.svelte.
 * @type {import('svelte/store').Writable<number>}
 */
export const layerSpacing = writable(6)

/**
 * planeOpacity — alpha of each context's translucent grid plane.
 * Bound to the Plane Opacity slider in ExplorerSidebar.svelte.
 * Glass-clear at 0.05; ~half-tint at 0.5.
 * @type {import('svelte/store').Writable<number>}
 */
export const planeOpacity = writable(0.15)

/**
 * hiddenLayers — set of contextIds the user has toggled off via the
 * sidebar's Layers section. Nodes on hidden layers (and edges
 * incident to them) are made invisible; the plane itself fades out.
 * @type {import('svelte/store').Writable<Set<string>>}
 */
export const hiddenLayers = writable(new Set())

/**
 * hiddenMetas — set of edge meta-type strings the user has toggled off
 * via the sidebar's Arrows section. Values are the wire strings:
 * "contains" | "leadsto" | "properties" | "similarto" | "unclassified".
 * @type {import('svelte/store').Writable<Set<string>>}
 */
export const hiddenMetas = writable(new Set())

/**
 * cameraFrame — a snapshot of the explorer camera updated by Scene.js's
 * onCameraChange hook (every pan / zoom / resize). Lets overlay
 * components (MiniMap, Compass) react to viewport changes without
 * polling rAF. Camera position.y is not tracked because the camera's
 * downward tilt is fixed and never changes (FR-003).
 *
 * @type {import('svelte/store').Writable<{
 *   x: number, z: number,            // camera position projected to ground plane
 *   zoom: number,
 *   frustumWidth: number,            // camera.right - camera.left, in world units
 *   frustumHeight: number,           // camera.top  - camera.bottom, in world units
 * } | null>}
 */
export const cameraFrame = writable(null)

/** canGoBack — derived from anchorHistory.back.length. */
export const canGoBack = derived(anchorHistory, $h => $h.back.length > 0)

/** canGoForward — derived from anchorHistory.forward.length. */
export const canGoForward = derived(anchorHistory, $h => $h.forward.length > 0)

/**
 * Reset all explorer state. Called on close and on a new query (FR-026f).
 */
export function resetExplorerState() {
  currentLayout.set(null)
  currentResultID.set(null)
  currentAnchorID.set(null)
  anchorHistory.set({ back: [], forward: [], current: null })
  pendingReanchorID.set(null)
  explorerQuery.set('')
  cameraFrame.set(null)
  hiddenLayers.set(new Set())
  hiddenMetas.set(new Set())
  layoutMode.set('extended')
  // layerSpacing and planeOpacity persist across closes (user preference).
}
