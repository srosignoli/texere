// Bridge effect for intra-layer line crossings.
//
// When two axis-aligned edges share a Y plane and their segments
// cross at right angles (one X-segment, one Z-segment), draw a small
// vertical "hop" on the lower-priority edge so the crossing reads as
// one passing under the other — the standard graph-drawing trick to
// disambiguate intersections.
//
// Priority order (lower index → stays continuous on top):
//   leadsto > contains > properties > similarto > unclassified
//
// Hop shape: a square bridge.
//
//   …— p0 ─ HOP_HALF →─┐                          ┌─→ HOP_HALF — p1 —…
//                      │   HUMP                   │
//                      └─────────────────────────—┘     (lifted +Y)
//
// HOP_HALF and HUMP are small enough that the bridge reads as a
// localised bump (rather than reshaping the whole edge).
//
// Performance gate: skip the entire pass when there are more than
// MAX_BRIDGE_EDGES edges. The scan is O(E²) over segments, so 400
// edges (~1200 segments) takes <5 ms; beyond that the cost grows
// quadratically with diminishing visual return on dense scenes.

import * as THREE from 'three'

const MAX_BRIDGE_EDGES = 400
const HOP_HALF = 0.4
const HUMP = 0.6

const PRIORITY = {
  leadsto: 0,
  contains: 1,
  properties: 2,
  similarto: 3,
  unclassified: 4,
}

/**
 * Apply bridge effects to a set of edge records in place.
 *
 * @param {Array<{ edge: any, line: THREE.Line, path: THREE.Vector3[] }>} records
 * @returns {{ bridges: number, skipped: boolean, reason?: string }}
 */
export function applyBridges(records) {
  if (records.length > MAX_BRIDGE_EDGES) {
    return { bridges: 0, skipped: true, reason: 'too dense' }
  }

  // Collect candidate segments. Cross-layer edges are excluded.
  // Each segment carries its parent record + index so we can rewrite
  // the polyline later.
  const segs = []
  for (let i = 0; i < records.length; i++) {
    const r = records[i]
    if (!isIntraLayer(r.path)) continue
    for (let s = 0; s < r.path.length - 1; s++) {
      const a = r.path[s]
      const b = r.path[s + 1]
      // Skip zero-length segments.
      if (a.x === b.x && a.z === b.z) continue
      // Snap Y to a coarse key so floats from different paths group
      // together cleanly.
      const yKey = Math.round(a.y * 100) / 100
      const axis = (a.x === b.x) ? 'Z' : 'X'
      segs.push({
        recIdx: i,
        segIdx: s,
        y: yKey,
        axis,
        // Sort the endpoints so p0 < p1 along the segment's axis;
        // simplifies the in-range check.
        x0: Math.min(a.x, b.x),
        x1: Math.max(a.x, b.x),
        z0: Math.min(a.z, b.z),
        z1: Math.max(a.z, b.z),
        // Cache the original axis-fixed coordinate.
        fixed: axis === 'X' ? a.z : a.x, // X-axis seg: constant Z; Z-axis seg: constant X
      })
    }
  }

  // Group by Y so we only compare segments that could actually cross.
  const byY = new Map()
  for (const s of segs) {
    if (!byY.has(s.y)) byY.set(s.y, [])
    byY.get(s.y).push(s)
  }

  // Each affected record gets a list of hop points so we can rewrite
  // its polyline once at the end (a single segment may carry multiple
  // hops if many edges cross it).
  const hopsByRec = new Map()
  let bridgeCount = 0

  for (const yGroup of byY.values()) {
    if (yGroup.length < 2) continue
    // Split by axis. Only X × Z pairs can cross transversally.
    const xSegs = yGroup.filter(s => s.axis === 'X')
    const zSegs = yGroup.filter(s => s.axis === 'Z')
    if (xSegs.length === 0 || zSegs.length === 0) continue

    for (const xs of xSegs) {
      for (const zs of zSegs) {
        // X-segment: constant Z = xs.fixed, varying X in [xs.x0, xs.x1].
        // Z-segment: constant X = zs.fixed, varying Z in [zs.z0, zs.z1].
        // They cross at (zs.fixed, xs.fixed) iff the cross-point lies
        // strictly inside both ranges.
        const cx = zs.fixed
        const cz = xs.fixed
        if (cx <= xs.x0 || cx >= xs.x1) continue
        if (cz <= zs.z0 || cz >= zs.z1) continue
        if (xs.recIdx === zs.recIdx) continue // self-crossing (shouldn't happen)

        // Decide which edge ducks. Lower priority hops over (drawn on
        // top with a bump); higher priority stays continuous.
        const ea = records[xs.recIdx].edge
        const eb = records[zs.recIdx].edge
        const pa = PRIORITY[ea.meta] ?? PRIORITY.unclassified
        const pb = PRIORITY[eb.meta] ?? PRIORITY.unclassified
        let hopper
        if (pa === pb) {
          // Same priority: break the tie by edge identifier so the
          // result is deterministic across runs.
          hopper = (cmpEdgeId(ea, eb) < 0) ? xs : zs
        } else {
          // Lower-priority value = HIGHER priority; the other hops.
          hopper = (pa < pb) ? zs : xs
        }
        const hopAxis = hopper.axis
        addHop(hopsByRec, hopper.recIdx, hopper.segIdx, cx, cz, hopper.y, hopAxis)
        bridgeCount++
      }
    }
  }

  // Rewrite affected polylines and rebuild line geometries.
  for (const [recIdx, hops] of hopsByRec) {
    const r = records[recIdx]
    r.path = rewritePolylineWithHops(r.path, hops)
    r.line.geometry.dispose()
    r.line.geometry = new THREE.BufferGeometry().setFromPoints(r.path)
    // Dashed lines (unknown-arrow edges) cache per-vertex distances; recompute
    // after the geometry swap or the dash pattern renders stale.
    if (r.line.material?.isLineDashedMaterial) {
      r.line.computeLineDistances()
    }
  }

  return { bridges: bridgeCount, skipped: false }
}

function isIntraLayer(path) {
  if (!path || path.length < 2) return false
  const y0 = path[0].y
  for (let i = 1; i < path.length; i++) {
    if (Math.abs(path[i].y - y0) > 1e-3) return false
  }
  return true
}

function cmpEdgeId(a, b) {
  // (srcId, dstId, meta) lex order. Stable across runs.
  if (a.srcId !== b.srcId) return a.srcId < b.srcId ? -1 : 1
  if (a.dstId !== b.dstId) return a.dstId < b.dstId ? -1 : 1
  return (a.meta || '') < (b.meta || '') ? -1 : 1
}

function addHop(hopsByRec, recIdx, segIdx, cx, cz, y, axis) {
  if (!hopsByRec.has(recIdx)) hopsByRec.set(recIdx, [])
  hopsByRec.get(recIdx).push({ segIdx, cx, cz, y, axis })
}

// rewritePolylineWithHops splices a 4-point square hop into the
// polyline at each crossing point on the hopper edge. Multiple hops
// on the same segment are sorted along the segment's axis so the
// inserted points appear in path order.
function rewritePolylineWithHops(path, hops) {
  // Group hops by segIdx and sort along the segment's axis.
  const bySeg = new Map()
  for (const h of hops) {
    if (!bySeg.has(h.segIdx)) bySeg.set(h.segIdx, [])
    bySeg.get(h.segIdx).push(h)
  }

  const out = []
  for (let i = 0; i < path.length - 1; i++) {
    const a = path[i]
    const b = path[i + 1]
    out.push(a)
    const segHops = bySeg.get(i)
    if (!segHops || segHops.length === 0) continue

    // Sort hops along the direction from a → b.
    const isXSeg = a.z === b.z
    if (isXSeg) {
      const ascending = b.x > a.x
      segHops.sort((p, q) => ascending ? (p.cx - q.cx) : (q.cx - p.cx))
    } else {
      const ascending = b.z > a.z
      segHops.sort((p, q) => ascending ? (p.cz - q.cz) : (q.cz - p.cz))
    }

    for (const h of segHops) {
      const y = h.y
      const hump = y + HUMP
      if (isXSeg) {
        // X-axis segment: hop in X with Y hump. Z is constant.
        const dir = b.x > a.x ? 1 : -1
        const x0 = h.cx - HOP_HALF * dir
        const x1 = h.cx + HOP_HALF * dir
        out.push(new THREE.Vector3(x0, y, h.cz))
        out.push(new THREE.Vector3(x0, hump, h.cz))
        out.push(new THREE.Vector3(x1, hump, h.cz))
        out.push(new THREE.Vector3(x1, y, h.cz))
      } else {
        // Z-axis segment: hop in Z with Y hump. X is constant.
        const dir = b.z > a.z ? 1 : -1
        const z0 = h.cz - HOP_HALF * dir
        const z1 = h.cz + HOP_HALF * dir
        out.push(new THREE.Vector3(h.cx, y, z0))
        out.push(new THREE.Vector3(h.cx, hump, z0))
        out.push(new THREE.Vector3(h.cx, hump, z1))
        out.push(new THREE.Vector3(h.cx, y, z1))
      }
    }
  }
  out.push(path[path.length - 1])
  return out
}
