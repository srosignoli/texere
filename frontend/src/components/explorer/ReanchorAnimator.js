// Animated transition between two LayoutResults. Used by the click-to-
// re-anchor and Alt+←/→ history flows (FR-025, FR-026d).
//
// Visual contract per FR-025:
//   - Layout MUST recompute with the clicked node as the new anchor.
//   - Camera MUST animate smoothly to the new fit-to-result framing
//     within 1 second.
//   - Compass orientation (which axis = which meta-type) MUST NOT
//     change — only node positions update.
//
// Strategy (cancellable rAF loop):
//
//   t = 0
//     Capture every existing node's start position (sphere world coords)
//     and the start camera state. Fire onEdgeFadeOut so the host can
//     fade existing edges to opacity 0 in parallel.
//
//   t = 0 → 0.5 (edge fade-out + node tween, parallel)
//     Interpolate sphere/label/pickMesh positions toward the new layout
//     using easeOutCubic. Camera position + zoom interpolate likewise.
//     Edges should reach opacity 0 by t≈0.4 (host-driven, see callback).
//
//   t = 0.5
//     Fire onEdgesRebuild — host disposes old edge geometry and builds
//     fresh edges from the current (mid-tween) node positions. They
//     start at opacity 0.
//
//   t = 0.5 → 1.0 (node tween + edge fade-in)
//     Continue interpolating positions; new edges fade in to their
//     target opacity (0.5 default, set by EdgeMeshes).
//
//   t = 1.0
//     Snap to exact target positions (avoid float drift), call onDone.
//
// Cancellation: a fresh tween started while one is in flight cancels
// the prior tween and starts the new one from the CURRENT interpolated
// positions (FR-025b / FR-026d). The host's onEdgesRebuild can be
// safely re-fired against partially-tweened geometry.

import * as THREE from 'three'

const DEFAULT_DURATION_MS = 700
const EDGE_REBUILD_T = 0.5
const EDGE_FADEOUT_END_T = 0.4

// Camera fit constants — kept identical to fitCameraToBounds in
// GraphExplorer.svelte's initial framing so tween targets and initial
// targets coincide perfectly. Any drift between the two would produce
// a visible "snap" at t=1.
const ISO_ANGLE_DEG = 35
const CAMERA_DISTANCE = 30
const FIT_PADDING = 1.4

/**
 * Compute the camera state that frames a LayoutResult's bounds with
 * the same isometric vantage as the initial Scene.js camera. Returns
 * a snapshot suitable for tweening.
 *
 * @param {THREE.OrthographicCamera} camera
 * @param {{minX,maxX,minY,maxY,minZ,maxZ}} bounds
 * @returns {{position: THREE.Vector3, zoom: number, lookAt: THREE.Vector3}}
 */
export function computeCameraFrame(camera, bounds) {
  const cx = (bounds.maxX + bounds.minX) / 2
  const cy = (bounds.maxY + bounds.minY) / 2
  const cz = (bounds.maxZ + bounds.minZ) / 2

  const angle = (ISO_ANGLE_DEG * Math.PI) / 180
  const azim = Math.PI / 4 // 45°
  const r = CAMERA_DISTANCE

  const position = new THREE.Vector3(
    Math.cos(angle) * Math.cos(azim) * r + cx,
    Math.sin(angle) * r + cy,
    Math.cos(angle) * Math.sin(azim) * r + cz,
  )

  const widthWorld = bounds.maxX - bounds.minX
  const depthWorld = bounds.maxZ - bounds.minZ
  const heightWorld = bounds.maxY - bounds.minY
  // Post layered redesign: Y now encodes context layer index and can be
  // much taller than width/depth (e.g. 5 layers × 6 wu = 30 wu). Drop
  // the legacy `heightWorld * 1.5` Y-bias and treat all three axes
  // symmetrically — the iso projection compresses Y on screen anyway,
  // so we pad height a touch (1.2×) to keep the top and bottom layers
  // off the viewport edges.
  const span = Math.max(widthWorld, depthWorld, heightWorld * 1.2, 4) * FIT_PADDING

  // halfHeight here is the world-units height of the orthographic
  // frustum — i.e. (top - bottom). The initial camera in Scene.js
  // has top=10, bottom=-10 so this is 20.
  const halfHeight = camera.top - camera.bottom
  let zoom = halfHeight / span
  if (!Number.isFinite(zoom) || zoom <= 0) zoom = 1
  zoom = Math.min(8, Math.max(0.2, zoom))

  return {
    position,
    zoom,
    lookAt: new THREE.Vector3(cx, cy, cz),
  }
}

/**
 * Snapshot the current camera state so we can interpolate from it.
 * @param {THREE.OrthographicCamera} camera
 */
export function snapshotCamera(camera) {
  // Snapshot the look-at point as the current position projected toward
  // the world origin along the camera's view direction. We can't read
  // the actual lookAt back from a camera, so we approximate by walking
  // along -camera.forward by CAMERA_DISTANCE.
  const forward = new THREE.Vector3()
  camera.getWorldDirection(forward)
  return {
    position: camera.position.clone(),
    zoom: camera.zoom,
    lookAt: camera.position.clone().addScaledVector(forward, CAMERA_DISTANCE),
  }
}

/**
 * Snapshot every node's current world position. Used as the start
 * frame for the position tween.
 * @param {ReturnType<typeof import('./NodeMeshes.js').buildNodeMeshes>} nodeMeshes
 * @returns {Map<string, {x:number,y:number,z:number}>}
 */
export function snapshotNodePositions(nodeMeshes) {
  const out = new Map()
  for (const id of nodeMeshes.nodeIDs) {
    const r = nodeMeshes.byId(id)
    if (!r) continue
    const p = r.sphere.position
    out.set(id, { x: p.x, y: p.y, z: p.z })
  }
  return out
}

/**
 * Run a re-anchor tween. Returns a controller with `.cancel()`.
 *
 * @param {{
 *   camera: THREE.OrthographicCamera,
 *   render: () => void,
 *   nodeMeshes: ReturnType<typeof import('./NodeMeshes.js').buildNodeMeshes>,
 *   prevPositions: Map<string,{x:number,y:number,z:number}>,
 *   nextLayout: any,                         // LayoutResult
 *   prevCamera: ReturnType<typeof snapshotCamera>,
 *   nextCamera: ReturnType<typeof computeCameraFrame>,
 *   durationMs?: number,
 *   onEdgeFadeProgress?: (t: number) => void,  // t in [0,1] over 0..EDGE_FADEOUT_END_T
 *   onEdgesRebuild?: () => void,               // fired exactly once at EDGE_REBUILD_T
 *   onEdgeFadeIn?: (t: number) => void,        // t in [0,1] over EDGE_REBUILD_T..1
 *   onDone?: () => void,
 *   onCancelled?: () => void,
 * }} opts
 */
export function tweenReanchor(opts) {
  const {
    camera, render, nodeMeshes, prevPositions, nextLayout,
    prevCamera, nextCamera,
    durationMs = DEFAULT_DURATION_MS,
    onEdgeFadeProgress, onEdgesRebuild, onEdgeFadeIn, onDone, onCancelled,
  } = opts

  let rafID = null
  let cancelled = false
  let edgesRebuilt = false
  const startMs = performance.now()
  const lookAtTmp = new THREE.Vector3()

  function cancel() {
    if (cancelled) return
    cancelled = true
    if (rafID !== null) {
      cancelAnimationFrame(rafID)
      rafID = null
    }
    onCancelled?.()
  }

  function step(now) {
    if (cancelled) return
    const u = Math.min(1, (now - startMs) / durationMs)
    const e = easeOutCubic(u)

    // Node positions
    for (const n of nextLayout.nodes) {
      const r = nodeMeshes.byId(n.id)
      if (!r) continue
      const start = prevPositions.get(n.id) ?? { x: n.x, y: n.y, z: n.z }
      const x = lerp(start.x, n.x, e)
      const y = lerp(start.y, n.y, e)
      const z = lerp(start.z, n.z, e)
      r.sphere.position.set(x, y, z)
      // Label sits at sphere position + (0, labelOffsetY, 0). We only
      // know labelOffsetY indirectly via the existing label.position.y
      // before the tween — capture once at start. For simplicity and
      // determinism, recompute the offset every frame from the
      // record's stored labelOffsetY (NodeMeshes attaches this).
      const labelDY = r.labelOffsetY ?? 0.55
      r.label.position.set(x, y + labelDY, z)
      r.pickMesh.position.set(x, y, z)
      if (r.cycleGlyph) {
        r.cycleGlyph.position.set(x + (r.cycleOffsetX ?? 0), y + (r.cycleOffsetY ?? labelDY), z)
      }
    }

    // Camera
    camera.position.lerpVectors(prevCamera.position, nextCamera.position, e)
    camera.zoom = lerp(prevCamera.zoom, nextCamera.zoom, e)
    lookAtTmp.lerpVectors(prevCamera.lookAt, nextCamera.lookAt, e)
    camera.lookAt(lookAtTmp)
    camera.updateProjectionMatrix()

    // Edge phasing
    if (u < EDGE_FADEOUT_END_T) {
      onEdgeFadeProgress?.(Math.min(1, u / EDGE_FADEOUT_END_T))
    } else if (!edgesRebuilt && u >= EDGE_REBUILD_T) {
      edgesRebuilt = true
      onEdgesRebuild?.()
    } else if (edgesRebuilt) {
      const remaining = (u - EDGE_REBUILD_T) / (1 - EDGE_REBUILD_T)
      onEdgeFadeIn?.(Math.min(1, remaining))
    }

    render()

    if (u < 1) {
      rafID = requestAnimationFrame(step)
    } else {
      // Snap exactly to target so tiny float drift doesn't accumulate.
      for (const n of nextLayout.nodes) {
        const r = nodeMeshes.byId(n.id)
        if (!r) continue
        r.sphere.position.set(n.x, n.y, n.z)
        const labelDY = r.labelOffsetY ?? 0.55
        r.label.position.set(n.x, n.y + labelDY, n.z)
        r.pickMesh.position.set(n.x, n.y, n.z)
        if (r.cycleGlyph) {
          r.cycleGlyph.position.set(n.x + (r.cycleOffsetX ?? 0), n.y + (r.cycleOffsetY ?? labelDY), n.z)
        }
      }
      camera.position.copy(nextCamera.position)
      camera.zoom = nextCamera.zoom
      camera.lookAt(nextCamera.lookAt)
      camera.updateProjectionMatrix()
      render()
      // Ensure edges are rebuilt even on very short tweens that may
      // skip the EDGE_REBUILD_T threshold (durationMs ≪ a single frame).
      if (!edgesRebuilt) {
        onEdgesRebuild?.()
        edgesRebuilt = true
      }
      onEdgeFadeIn?.(1)
      rafID = null
      onDone?.()
    }
  }

  rafID = requestAnimationFrame(step)
  return { cancel }
}

// =============================================================================
// Helpers
// =============================================================================

function lerp(a, b, t) {
  return a + (b - a) * t
}

function easeOutCubic(u) {
  const x = 1 - u
  return 1 - x * x * x
}
