// Edge rendering for the 2.5D Isometric Graph Explorer.
//
// Orthogonal layout: nodes sit on integer grid cells (see layout.go's
// placeLayer) so we route every edge as an axis-aligned polyline with
// 90° bends. This matches the orthogonal-graph-drawing aesthetic
// requested in the redesign — and, projected through the locked
// isometric camera, gives the classic "blueprint stack" look (see
// the reference images in the redesign brief).
//
// Routing rule:
//   intra-layer (same Y):    [start → (end.x, _, start.z) → end]    (X then Z, 1 bend)
//   cross-layer (different Y): [start → (start.x, end.y, start.z) → (end.x, end.y, start.z) → end]   (Y → X → Z, 2 bends)
//
// Each meta-type renders with a distinct color. ALL edges are solid
// THREE.Line polylines — dashed/dotted styling was dropped because
// colour alone (amber / cyan / green / purple / gray) is already
// easy to spot at the canvas zoom levels we target, and dashed dashes
// disappear at low zoom anyway. The arrowhead cone on `leadsto` is
// the only per-meta geometric extra.
//
// Opacity defaults to 0.5 so edges read as connective tissue rather
// than competing with nodes for attention.

import * as THREE from 'three'
import { getEdgeColor } from './Theme.js'
import { applyBridges } from './EdgeBridges.js'

// Per-meta marker dimensions. Sizes are tuned so markers stay legible
// at min zoom (≥ ~6-8 px on screen) without overwhelming the node
// spheres they sit beside.
const LEADSTO_CONE_R = 0.35
const LEADSTO_CONE_H = 0.9
const CONTAINS_DIAMOND_R = 0.35
const PROPERTIES_DISC_R = 0.25
const SIMILARTO_CONE_R = 0.22
const SIMILARTO_CONE_H = 0.55

const DEFAULT_EDGE_OPACITY = 0.5

/**
 * Build all edge meshes for a layout result. Endpoints are looked up
 * through the supplied nodeMeshes registry — endpoints missing from the
 * registry produce a console warning and the edge is skipped.
 *
 * If `picker` is supplied, an invisible pick proxy is registered for
 * each edge so hover detection can surface the arrow description.
 *
 * @param {THREE.Scene} scene
 * @param {{edges: any[]}} layoutResult
 * @param {ReturnType<typeof import('./NodeMeshes.js').buildNodeMeshes>} nodeMeshes
 * @param {ReturnType<typeof import('./Picking.js').createPicker>} [picker]
 */
export function buildEdgeMeshes(scene, layoutResult, nodeMeshes, picker) {
  /** @type {EdgeRecord[]} */
  const records = []

  for (const e of layoutResult.edges) {
    const src = nodeMeshes.byId(e.srcId)
    const dst = nodeMeshes.byId(e.dstId)
    if (!src || !dst) continue

    const start = src.sphere.position
    const end = dst.sphere.position
    const color = getEdgeColor(e.meta)
    const path = buildOrthogonalPath(start, end)

    // Unknown-arrow edges (code not declared in any SSTconfig) render dashed
    // so they read as provisional/unregistered; everything else is solid.
    const line = e.meta === 'unknown'
      ? makeDashedLine(path, color, { size: 0.5, gap: 0.3 })
      : makeLine(path, color)
    const markers = makeMarkersForEdge(e.meta, path, color)
    const parts = [line, ...markers]

    for (const p of parts) {
      p.material.transparent = true
      p.material.opacity = 0
      p.material.depthWrite = false
      p.userData.targetOpacity = DEFAULT_EDGE_OPACITY
      scene.add(p)
    }

    // metaHidden / endpointHidden are independent reasons an edge can
    // be suppressed. Both default to false; visibility helpers below
    // flip them and we apply `parts[i].visible = !metaHidden && !endpointHidden`.
    records.push({
      edge: e,
      line,
      parts,
      pickGroup: null,
      path,           // current polyline (bridges may rewrite this)
      metaHidden: false,
      endpointHidden: false,
    })
  }

  // Bridge effect: lower-priority edges hop over higher-priority ones
  // at intra-layer crossings. Rewrites `record.path` and rebuilds the
  // affected line's BufferGeometry in place. Markers are at endpoints
  // and untouched. Must run BEFORE pick proxies are registered so the
  // proxies match the final polylines.
  const bridgesStats = applyBridges(records)

  // Pick proxies — invisible cylinders along each (possibly-rewritten)
  // polyline segment so a mouse hover snaps to the edge with a
  // generous hit area.
  if (picker) {
    for (const r of records) {
      r.pickGroup = picker.makeEdgePickMesh(r.edge, r.path)
      scene.add(r.pickGroup)
    }
  }

  function applyVisibility(r) {
    const vis = !r.metaHidden && !r.endpointHidden
    for (const p of r.parts) p.visible = vis
    if (r.pickGroup) r.pickGroup.visible = vis
  }

  return {
    all() { return records },
    size: records.length,
    /** Diagnostic — surfaced to the LogPanel. */
    bridgesStats,

    setOpacity(record, t) {
      // Tween targets the per-record DEFAULT_EDGE_OPACITY rather than 1
      // so a fully faded-in edge sits at 0.5 (the spec opacity, FR-011).
      const target = record.parts[0]?.userData.targetOpacity ?? DEFAULT_EDGE_OPACITY
      for (const p of record.parts) {
        p.material.opacity = t * target
      }
    },

    setAllOpacity(t) {
      for (const r of records) {
        const target = r.parts[0]?.userData.targetOpacity ?? DEFAULT_EDGE_OPACITY
        for (const p of r.parts) {
          p.material.opacity = t * target
        }
      }
    },

    forEdgesAt(nodeID, fn) {
      for (const r of records) {
        if (r.edge.dstId === nodeID || r.edge.srcId === nodeID) {
          fn(r)
        }
      }
    },

    /**
     * Toggle every edge of a given meta-type (visible state stacks
     * with endpoint-hidden). Drives the sidebar's per-arrow-type
     * checkboxes.
     */
    setMetaVisible(meta, visible) {
      for (const r of records) {
        if ((r.edge.meta || 'unclassified') !== meta) continue
        r.metaHidden = !visible
        applyVisibility(r)
      }
    },

    /**
     * Recompute endpoint-visibility for every edge given an
     * `isVisible(nodeId)` predicate (typically nodeMeshes.isVisible).
     * An edge is endpoint-hidden when either endpoint is currently
     * hidden by a layer toggle.
     */
    refreshEndpointVisibility(isVisibleFn) {
      for (const r of records) {
        const srcVis = isVisibleFn(r.edge.srcId)
        const dstVis = isVisibleFn(r.edge.dstId)
        r.endpointHidden = !(srcVis && dstVis)
        applyVisibility(r)
      }
    },

    dispose() {
      for (const r of records) {
        for (const p of r.parts) {
          scene.remove(p)
          if (p.geometry?.dispose) p.geometry.dispose()
          if (p.material?.dispose) p.material.dispose()
        }
        if (r.pickGroup) {
          // Pick proxy children are owned by Picking.js's registry;
          // calling picker.clearEdges() (done by the caller on full
          // teardown / re-anchor) frees their geometries/materials.
          // Here we just detach the group from the scene.
          scene.remove(r.pickGroup)
        }
      }
      records.length = 0
    },
  }
}

// =============================================================================
// Orthogonal path construction
// =============================================================================

// buildOrthogonalPath returns the polyline waypoints that route from
// start to end with axis-aligned 90° segments. Intra-layer (same Y)
// produces 4 waypoints (two bends, X → Z → X around the X midpoint);
// cross-layer produces 4 waypoints (Y → X → Z, two bends).
//
// The intra-layer route bends at the X MIDPOINT rather than at the
// destination's column. The earlier "X then Z" route turned at
// (end.x, start.z) — the destination column — which in the left-to-right
// Flow layout coincides with a sibling node's grid cell, so a fan-out
// (door → passage / road / river) visually threaded THROUGH the sibling
// and read as a chain (door → passage → road …). Turning at the midpoint
// (an empty half-cell, since nodes sit on integer GridStep columns) keeps
// every bend off the node grid, so sibling edges fan out cleanly instead
// of overlapping a neighbour.
//
// The router is intentionally deterministic — no randomization, no
// edge-crossing minimization.
function buildOrthogonalPath(start, end) {
  const sameLayer = Math.abs(start.y - end.y) < 1e-3
  if (sameLayer) {
    // Intra-layer: X → Z → X, bending at the X midpoint (an empty half-cell)
    // so the perpendicular run never passes through a node's column.
    const midX = (start.x + end.x) / 2
    return [
      new THREE.Vector3(start.x, start.y, start.z),
      new THREE.Vector3(midX, start.y, start.z),
      new THREE.Vector3(midX, start.y, end.z),
      new THREE.Vector3(end.x, start.y, end.z),
    ]
  }
  // Cross-layer: Y → X → Z. Two bends.
  return [
    new THREE.Vector3(start.x, start.y, start.z),
    new THREE.Vector3(start.x, end.y, start.z),
    new THREE.Vector3(end.x, end.y, start.z),
    new THREE.Vector3(end.x, end.y, end.z),
  ]
}

// =============================================================================
// Geometry constructors
// =============================================================================

function makeLine(points, color) {
  const geom = new THREE.BufferGeometry().setFromPoints(points)
  const mat = new THREE.LineBasicMaterial({ color })
  return new THREE.Line(geom, mat)
}

function makeDashedLine(points, color, dash) {
  const geom = new THREE.BufferGeometry().setFromPoints(points)
  const mat = new THREE.LineDashedMaterial({
    color,
    dashSize: dash.size,
    gapSize: dash.gap,
  })
  const line = new THREE.Line(geom, mat)
  line.computeLineDistances() // required for LineDashedMaterial
  return line
}

// makeMarkersForEdge returns 0–2 marker meshes for an edge's polyline,
// shape determined by the meta-type:
//
//   leadsto    → filled cone at destination (causal arrowhead)
//   contains   → filled diamond at source (UML composition)
//   properties → small filled disc at destination (attribute pin)
//   similarto  → small cones at BOTH endpoints (symmetric convergence)
//   unclassified / unknown → no marker
//
// All markers share the edge's color and a MeshBasicMaterial — they
// fade in alongside the line in the existing reveal tween and dim with
// it under hover-highlight.
function makeMarkersForEdge(meta, points, color) {
  switch (meta) {
    case 'leadsto':
      return filterNull([makeConeAt(points, color, true, LEADSTO_CONE_R, LEADSTO_CONE_H)])
    case 'contains':
      return filterNull([makeDiamondAt(points, color, false, CONTAINS_DIAMOND_R)])
    case 'properties':
      return filterNull([makeDiscAt(points, color, true, PROPERTIES_DISC_R)])
    case 'similarto':
      // Two small cones, each pointing AT its own endpoint (the
      // tangent inside the helper already points toward the picked
      // end, so both cones face their respective node).
      return filterNull([
        makeConeAt(points, color, true, SIMILARTO_CONE_R, SIMILARTO_CONE_H),
        makeConeAt(points, color, false, SIMILARTO_CONE_R, SIMILARTO_CONE_H),
      ])
    default:
      return []
  }
}

function filterNull(arr) {
  return arr.filter(Boolean)
}

// makeConeAt builds a cone marker at one end of the polyline.
//   atEnd=true  → cone at points[last], oriented along final segment
//   atEnd=false → cone at points[0],   oriented along first segment
// The cone's tip lands exactly on the endpoint (pulled back by H/2).
function makeConeAt(points, color, atEnd, radius, height) {
  if (points.length < 2) return null
  let tip, prev
  if (atEnd) {
    tip = points[points.length - 1]
    prev = points[points.length - 2]
  } else {
    tip = points[0]
    prev = points[1]
  }
  const tangent = new THREE.Vector3().subVectors(tip, prev)
  const len = tangent.length()
  if (len < 1e-6) return null
  tangent.divideScalar(len)

  const cone = new THREE.Mesh(
    new THREE.ConeGeometry(radius, height, 10),
    new THREE.MeshBasicMaterial({ color }),
  )
  cone.position.copy(tip).addScaledVector(tangent, -height / 2)
  cone.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), tangent)
  return cone
}

// makeDiamondAt places a regular octahedron (filled diamond) at one
// end of the polyline. atEnd=false → at source. Oriented along the
// adjacent segment so the diamond's long axis aligns with the edge.
function makeDiamondAt(points, color, atEnd, radius) {
  if (points.length < 2) return null
  let pos, neighbour
  if (atEnd) {
    pos = points[points.length - 1]
    neighbour = points[points.length - 2]
  } else {
    pos = points[0]
    neighbour = points[1]
  }
  const tangent = new THREE.Vector3().subVectors(neighbour, pos)
  const len = tangent.length()
  if (len < 1e-6) return null
  tangent.divideScalar(len)

  const diamond = new THREE.Mesh(
    new THREE.OctahedronGeometry(radius, 0),
    new THREE.MeshBasicMaterial({ color }),
  )
  // Push the diamond slightly off the endpoint along the segment so
  // its centre sits between the node sphere and the edge proper.
  diamond.position.copy(pos).addScaledVector(tangent, radius * 1.6)
  // OctahedronGeometry's local Y is the long axis; align with the
  // segment so the diamond's points stretch along the edge.
  diamond.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), tangent)
  return diamond
}

// makeDiscAt places a small sphere at one end of the polyline.
// Used as a "property pin" for properties-meta edges.
function makeDiscAt(points, color, atEnd, radius) {
  if (points.length < 2) return null
  let pos, neighbour
  if (atEnd) {
    pos = points[points.length - 1]
    neighbour = points[points.length - 2]
  } else {
    pos = points[0]
    neighbour = points[1]
  }
  const tangent = new THREE.Vector3().subVectors(pos, neighbour)
  const len = tangent.length()
  if (len < 1e-6) return null
  tangent.divideScalar(len)

  const disc = new THREE.Mesh(
    new THREE.SphereGeometry(radius, 12, 8),
    new THREE.MeshBasicMaterial({ color }),
  )
  // Pull the disc back slightly along the segment so it doesn't get
  // buried inside the destination sphere.
  disc.position.copy(pos).addScaledVector(tangent, -radius * 1.6)
  return disc
}

/** @typedef {{
 *   edge: any,
 *   line: THREE.Line,
 *   parts: THREE.Object3D[],
 *   pickGroup: THREE.Group | null,
 *   path: THREE.Vector3[],
 *   metaHidden: boolean,
 *   endpointHidden: boolean,
 * }} EdgeRecord */
