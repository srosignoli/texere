// Hover-driven emphasis for the 2.5D Isometric Graph Explorer (US5).
//
// Engages on pointermove via the Picking.js raycaster (24 px screen-
// space hit area so the smallest log-centrality node remains effortless
// to hover, FR-014b). When a node is hovered, that node plus its
// directly connected neighbors stay at full opacity; every other node
// and edge dims toward DIM_NODE / DIM_EDGE so the eye is drawn along
// the highlighted subgraph. Releasing the hover restores defaults.
//
// Performance contract (T053): engage and disengage MUST complete in
// under 50 ms. We hit that by:
//   1. Precomputing nodeID → neighbor-set once at construction.
//   2. Mutating existing material.opacity in place — never allocating.
//   3. Iterating the existing nodeMeshes / edgeMeshes registries.
//
// Interaction with the skeleton-first reveal: this module writes
// material.opacity directly, so a hover applied mid-reveal will be
// stomped on the next fade-in tween tick. Acceptable: reveals complete
// in ≤2 s and any new pointermove re-applies the highlight.

const DIM_NODE_OPACITY = 0.18
const DIM_EDGE_FACTOR = 0.35   // multiplier on each edge's targetOpacity
const PROMINENT_NODE_OPACITY = 1.0
const PROMINENT_EDGE_BOOST = 1.6 // multiplier on each edge's targetOpacity (capped at 1)

/**
 * Build a highlight controller over the existing node / edge registries
 * for the active LayoutResult. Discard and rebuild whenever the layout
 * changes (re-anchor / new query), since the neighbor map is derived
 * from layout.edges.
 *
 * @param {ReturnType<typeof import('./NodeMeshes.js').buildNodeMeshes>} nodeMeshes
 * @param {ReturnType<typeof import('./EdgeMeshes.js').buildEdgeMeshes>} edgeMeshes
 * @param {{edges: Array<{srcId: string, dstId: string}>}} layout
 * @param {() => void} [onChange] - invoked after each setHovered transition; pass a scene.render hook
 */
export function createHighlight(nodeMeshes, edgeMeshes, layout, onChange) {
  // Adjacency map: nodeID → Set<neighborID>. Built once; O(1) lookup
  // per hover transition.
  const neighbors = new Map()
  for (const e of layout.edges || []) {
    let s = neighbors.get(e.srcId)
    if (!s) { s = new Set(); neighbors.set(e.srcId, s) }
    s.add(e.dstId)
    let d = neighbors.get(e.dstId)
    if (!d) { d = new Set(); neighbors.set(e.dstId, d) }
    d.add(e.srcId)
  }

  let currentID = null

  function apply() {
    if (currentID === null) {
      restore()
      onChange?.()
      return
    }
    // Prominent set is { currentID } ∪ neighbors(currentID).
    const adj = neighbors.get(currentID)
    for (const r of nodeMeshes.all()) {
      const id = r.group.userData.nodeID
      const isPromo = id === currentID || (adj && adj.has(id))
      const op = isPromo ? PROMINENT_NODE_OPACITY : DIM_NODE_OPACITY
      r.sphere.material.opacity = op
      r.label.material.opacity = op
      if (r.cycleGlyph) r.cycleGlyph.material.opacity = op
    }
    for (const er of edgeMeshes.all()) {
      const incident = er.edge.srcId === currentID || er.edge.dstId === currentID
      for (const p of er.parts) {
        const base = p.userData.targetOpacity ?? 0.5
        p.material.opacity = incident
          ? Math.min(1, base * PROMINENT_EDGE_BOOST)
          : base * DIM_EDGE_FACTOR
      }
    }
    onChange?.()
  }

  function restore() {
    for (const r of nodeMeshes.all()) {
      r.sphere.material.opacity = PROMINENT_NODE_OPACITY
      r.label.material.opacity = PROMINENT_NODE_OPACITY
      if (r.cycleGlyph) r.cycleGlyph.material.opacity = PROMINENT_NODE_OPACITY
    }
    for (const er of edgeMeshes.all()) {
      for (const p of er.parts) {
        p.material.opacity = p.userData.targetOpacity ?? 0.5
      }
    }
  }

  return {
    /**
     * Engage hover emphasis on the given node ID. Pass null to clear.
     * Idempotent — passing the current ID twice is a no-op (no render,
     * no allocation).
     */
    setHovered(id) {
      if (id === currentID) return
      currentID = id
      apply()
    },
    /** Forcibly clear emphasis (e.g. on layout teardown). */
    clear() {
      if (currentID === null) return
      currentID = null
      restore()
      onChange?.()
    },
  }
}
