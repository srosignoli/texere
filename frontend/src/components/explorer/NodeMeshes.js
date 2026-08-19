// Node + label rendering for the 2.5D Isometric Graph Explorer.
//
// Per node we create three Three.js objects:
//   1. A sphere mesh sized by NodeView.size (FR-014: log-centrality
//      driven; the visible encoding is what changes here).
//   2. A canvas-textured Sprite holding the node label. Sprites in
//      Three.js always face the camera (billboarding for free) and we
//      keep on-screen pixel size constant by recomputing sprite.scale
//      whenever camera.zoom changes (FR-015 / FR-016).
//   3. A transparent picking mesh registered with the Picker so the
//      24 px screen-space hit area is preserved regardless of visual
//      size (FR-014a/b).
//
// Returned: a registry that exposes:
//   - byId(id)        -> { group, sphere, label, pickMesh }
//   - rescaleLabels()  - call when camera.zoom or canvas size changes
//   - setOpacity(id, t) - linearly fade a node group; used by the
//                          skeleton-first reveal in GraphExplorer.svelte
//   - retheme()        - re-resolve every CSS-palette colour and redraw the
//                          label textures in place (theme switch)
//   - dispose()        - frees all GPU resources + removes from scene

import * as THREE from 'three'
import {
  getNodeColor,
  getLabelColor,
  getLabelHaloColor,
  getTerritoryPalette,
  getCycleGlyphColor,
  getCycleGlyphHaloColor,
} from './Theme.js'

// Visible-radius scale factor applied to NodeView.size. NodeView.size
// arrives in world units in roughly [0.5, 1.5] (Go side caps the
// centrality range at 3× — see internal/explorer/layout.go).
// RADIUS_SCALE preserves that ratio in the visible sphere.
const RADIUS_SCALE = 0.4
const RADIUS_FLOOR = 0.18

// On-screen label height (pixels). Set high enough to remain readable at
// minimum zoom (FR-016). Tuned in tandem with Scene.js's zoom bounds.
const LABEL_PIXEL_HEIGHT = 14

// Vertical offset of the label sprite above its sphere, in world units.
// Keeps the label visually attached to the node without overlapping it.
const LABEL_OFFSET = 0.55

// Cycle-warning glyph (T062, FR-007 / data-model §2.3). Rendered as a
// small amber triangle sprite tucked beside the node sphere; only
// shown for nodes where NodeView.cycleWarning === true (the Go layout
// engine flags every member of a containment cycle).
const CYCLE_GLYPH_PIXEL_HEIGHT = 14

// Label truncation limits. The 3D scene shows only the first words of
// long node texts; the full text is surfaced in a 2D HTML tooltip on
// hover (see GraphExplorer.svelte). Without truncation, .n4l nodes
// authored as full sentences become walls of text in the canvas.
const LABEL_MAX_WORDS = 3
const LABEL_MAX_CHARS = 24

/**
 * Build all node meshes for a layout result.
 *
 * @param {THREE.Scene} scene
 * @param {{nodes: any[], anchorId: string, layers?: any[]}} layoutResult
 * @param {THREE.OrthographicCamera} camera
 * @param {HTMLCanvasElement} canvas
 * @param {ReturnType<typeof import('./Picking.js').createPicker>} picker
 * @param {{mode?: 'stacked' | 'extended'}} [options]
 */
export function buildNodeMeshes(scene, layoutResult, camera, canvas, picker, options = {}) {
  /** @type {Map<string, NodeRecord>} */
  const records = new Map()
  const sphereGeo = new THREE.SphereGeometry(1, 16, 12)
  const mode = options.mode ?? 'stacked'

  // contextId → palette color. Rebuilt by resolveContextColors() on every
  // build and on every retheme, so the palette is never pinned to whichever
  // theme happened to be active earlier. Used only in extended mode, where
  // contexts have no per-floor differentiation and the sphere tint becomes
  // the dominant context cue.
  function resolveContextColors() {
    const m = new Map()
    if (mode !== 'extended') return m
    const palette = getTerritoryPalette()
    for (const l of layoutResult.layers ?? []) {
      const slot = l.colorSlot
      if (slot === undefined || slot === null || slot < 0) continue
      m.set(l.contextId, palette[slot % palette.length])
    }
    return m
  }

  // sphereColorFor resolves a node's sphere colour against the palette in
  // `byContext`. Called at build time and again on retheme.
  function sphereColorFor(n, byContext) {
    if (n.id === layoutResult.anchorId) return getNodeColor(true)
    if (mode === 'extended' && byContext.has(n.contextId)) return byContext.get(n.contextId)
    return getNodeColor(false)
  }

  let colorByContext = resolveContextColors()

  for (const n of layoutResult.nodes) {
    const radius = Math.max(RADIUS_FLOOR, (n.size ?? 1) * RADIUS_SCALE)

    const sphereColor = sphereColorFor(n, colorByContext)
    const sphereMat = new THREE.MeshBasicMaterial({
      color: sphereColor,
      transparent: true,
      opacity: 0,
    })
    const sphere = new THREE.Mesh(sphereGeo, sphereMat)
    sphere.position.set(n.x, n.y, n.z)
    sphere.scale.setScalar(radius)

    const fullLabel = n.label ?? ''
    const shortLabel = truncateLabel(fullLabel)
    const label = createLabelSprite(shortLabel)
    label.position.set(n.x, n.y + radius + LABEL_OFFSET, n.z)
    label.material.opacity = 0

    const pickMesh = picker.makePickMesh(n.id, sphere.position)

    const group = new THREE.Group()
    group.userData.nodeID = n.id
    group.userData.contextId = n.contextId ?? ''
    group.add(sphere)
    group.add(label)
    group.add(pickMesh)
    scene.add(group)

    // Cycle-warning glyph (T062): only created when the layout engine
    // flagged this node as part of a containment cycle. Sized to a
    // constant on-screen pixel height like the label, and offset just
    // above and to the right of the sphere so the warning is visible
    // without clipping the label.
    let cycleGlyph = null
    if (n.cycleWarning) {
      cycleGlyph = createCycleGlyphSprite()
      cycleGlyph.position.set(n.x + radius * 0.85, n.y + radius + LABEL_OFFSET * 1.3, n.z)
      cycleGlyph.material.opacity = 0
      group.add(cycleGlyph)
    }

    // labelOffsetY: vertical world-units offset of the label sprite
    // above the sphere centre. The re-anchor animator (ReanchorAnimator)
    // reads this to keep labels glued to their spheres during position
    // tweens.
    const labelOffsetY = radius + LABEL_OFFSET
    const cycleOffsetY = radius + LABEL_OFFSET * 1.3
    const cycleOffsetX = radius * 0.85
    records.set(n.id, { node: n, group, sphere, label, pickMesh, fullLabel, shortLabel, labelOffsetY, cycleGlyph, cycleOffsetY, cycleOffsetX })
  }

  // Initial sprite sizing for the current zoom.
  rescaleLabels(records, camera, canvas)

  return {
    byId(id) { return records.get(id) },
    all() { return records.values() },
    size: records.size,

    rescaleLabels: () => rescaleLabels(records, camera, canvas),

    /**
     * Re-resolve every colour this module reads from the CSS palette and
     * repaint in place. Called when the user switches theme with the
     * explorer already open.
     *
     * Label text lives in a canvas texture, so a material colour swap is not
     * enough — the texture has to be redrawn with the new fill/halo. We swap
     * `material.map` and dispose the old one rather than rebuilding the
     * sprite, which keeps every sprite's position, scale, opacity (mid
     * reveal-tween included) and visibility exactly as the user left it.
     * Geometry and camera are never touched.
     */
    retheme() {
      colorByContext = resolveContextColors()
      for (const r of records.values()) {
        r.sphere.material.color.setHex(sphereColorFor(r.node, colorByContext))
        const old = r.label.material.map
        r.label.material.map = makeLabelTexture(r.shortLabel)
        r.label.material.needsUpdate = true
        old?.dispose()
      }
      // Text and font are unchanged, so the canvas aspect is too — but
      // rescale anyway so a retheme can never leave a stale sprite size.
      rescaleLabels(records, camera, canvas)
    },

    setOpacity(id, t) {
      const r = records.get(id)
      if (!r) return
      r.sphere.material.opacity = t
      r.label.material.opacity = t
      if (r.cycleGlyph) r.cycleGlyph.material.opacity = t
    },

    setAllOpacity(t) {
      for (const r of records.values()) {
        r.sphere.material.opacity = t
        r.label.material.opacity = t
        if (r.cycleGlyph) r.cycleGlyph.material.opacity = t
      }
    },

    /**
     * Toggle every node whose primary context matches `contextId`.
     * Drives the sidebar's per-layer visibility checkboxes (post
     * layered redesign).
     */
    setLayerVisible(contextId, visible) {
      for (const r of records.values()) {
        if ((r.group.userData.contextId ?? '') === (contextId ?? '')) {
          r.group.visible = visible
        }
      }
    },

    /**
     * Tell callers whether a node id is currently visible (its
     * containing layer is on). Used by EdgeMeshes.setNodeVisible
     * to suppress edges whose endpoints are hidden.
     */
    isVisible(id) {
      const r = records.get(id)
      return r ? r.group.visible : false
    },

    /** Keys for callers that need to iterate in stable order. */
    nodeIDs: Array.from(records.keys()),

    /**
     * Project a registered node's world position to screen-space pixels.
     * Used by the hover tooltip overlay to anchor the HTML element.
     * @param {string} id
     * @returns {{x:number,y:number}|null}
     */
    projectToScreen(id) {
      const r = records.get(id)
      if (!r) return null
      const v = r.sphere.position.clone().project(camera)
      const w = canvas.clientWidth || 1
      const h = canvas.clientHeight || 1
      return {
        x: ((v.x + 1) / 2) * w,
        y: ((-v.y + 1) / 2) * h,
      }
    },

    fullLabelOf(id) {
      return records.get(id)?.fullLabel ?? null
    },

    dispose() {
      for (const r of records.values()) {
        scene.remove(r.group)
        r.sphere.geometry.dispose()
        r.sphere.material.dispose()
        r.label.material.map?.dispose()
        r.label.material.dispose()
        if (r.cycleGlyph) {
          r.cycleGlyph.material.map?.dispose()
          r.cycleGlyph.material.dispose()
        }
      }
      records.clear()
      // sphereGeo is shared across all node meshes; dispose once after
      // every per-node material/geometry is freed.
      sphereGeo.dispose()
    },
  }
}

/**
 * Recompute every label sprite's scale so it occupies LABEL_PIXEL_HEIGHT
 * pixels on screen regardless of camera.zoom or canvas resize.
 * @param {Map<string, NodeRecord>} records
 * @param {THREE.OrthographicCamera} camera
 * @param {HTMLCanvasElement} canvas
 */
function rescaleLabels(records, camera, canvas) {
  const h = canvas.clientHeight || 1
  // worldPerPixel along the screen height (orthographic camera).
  const worldPerPx = (camera.top - camera.bottom) / camera.zoom / h
  for (const r of records.values()) {
    const tex = r.label.material.map
    const aspect = (tex?.image?.width ?? 1) / (tex?.image?.height ?? 1)
    const worldHeight = LABEL_PIXEL_HEIGHT * worldPerPx
    r.label.scale.set(worldHeight * aspect, worldHeight, 1)

    if (r.cycleGlyph) {
      const gtex = r.cycleGlyph.material.map
      const gaspect = (gtex?.image?.width ?? 1) / (gtex?.image?.height ?? 1)
      const gh = CYCLE_GLYPH_PIXEL_HEIGHT * worldPerPx
      r.cycleGlyph.scale.set(gh * gaspect, gh, 1)
    }
  }
}

/**
 * Truncate a label so the 3D scene stays readable. Returns the
 * trimmed string (with a trailing ellipsis when truncated). The
 * full original text is preserved on the node record so the hover
 * tooltip can show it.
 *
 * Rules: keep at most LABEL_MAX_WORDS words; if the result is
 * still longer than LABEL_MAX_CHARS characters, trim and append "…".
 *
 * @param {string} text
 * @returns {string}
 */
function truncateLabel(text) {
  if (!text) return ''
  const trimmed = text.trim()
  const words = trimmed.split(/\s+/)
  const tooManyWords = words.length > LABEL_MAX_WORDS
  let out = words.slice(0, LABEL_MAX_WORDS).join(' ')
  if (out.length > LABEL_MAX_CHARS) {
    out = out.slice(0, LABEL_MAX_CHARS - 1).trimEnd() + '…'
  } else if (tooManyWords) {
    out += '…'
  }
  return out
}

/**
 * Draw a node label into a high-DPR canvas texture, resolving the label
 * fill and halo from the CSS palette AT CALL TIME. Split out from
 * createLabelSprite so retheme() can redraw the texture of an existing
 * sprite without disturbing anything else about it.
 *
 * @param {string} text
 * @returns {THREE.CanvasTexture}
 */
function makeLabelTexture(text) {
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  const fontSize = 28 // canvas-space px
  const padding = 8

  // Measure first to size the canvas snugly to the text.
  const measureCanvas = document.createElement('canvas')
  const measureCtx = measureCanvas.getContext('2d')
  measureCtx.font = `600 ${fontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
  const metrics = measureCtx.measureText(text || ' ')
  const textWidth = Math.ceil(metrics.width)

  const canvas = document.createElement('canvas')
  canvas.width = (textWidth + padding * 2) * dpr
  canvas.height = (fontSize + padding * 2) * dpr
  const ctx = canvas.getContext('2d')
  ctx.scale(dpr, dpr)
  ctx.font = `600 ${fontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
  ctx.textBaseline = 'middle'
  ctx.textAlign = 'left'

  // Halo: stroke + fill so the label remains legible against any
  // territory color or when overlapping an edge.
  ctx.lineWidth = 4
  ctx.strokeStyle = getLabelHaloColor()
  ctx.strokeText(text, padding, fontSize / 2 + padding)
  ctx.fillStyle = getLabelColor()
  ctx.fillText(text, padding, fontSize / 2 + padding)

  const tex = new THREE.CanvasTexture(canvas)
  tex.minFilter = THREE.LinearFilter
  tex.magFilter = THREE.LinearFilter
  tex.anisotropy = 4
  return tex
}

/**
 * Build a billboarded label sprite from text. Uses a high-DPR canvas
 * texture so the label stays sharp at zoom-in.
 * @param {string} text
 * @returns {THREE.Sprite}
 */
function createLabelSprite(text) {
  const mat = new THREE.SpriteMaterial({
    map: makeLabelTexture(text),
    transparent: true,
    depthWrite: false,
    depthTest: true,
  })
  const sprite = new THREE.Sprite(mat)
  // Per-sprite center is (0.5, 0.5) — the sprite's anchor is its center,
  // so positioning sprite at (x, y, z) places its midpoint there.
  return sprite
}

/**
 * Build the cycle-warning glyph sprite — a small amber triangle with a
 * dark halo. Reused for every cycle-flagged node; sized to a constant
 * pixel height like the label so it stays visible at every zoom level.
 *
 * @returns {THREE.Sprite}
 */
function createCycleGlyphSprite() {
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  const SIZE = 32 // canvas-space px (pre-DPR)
  const canvas = document.createElement('canvas')
  canvas.width = SIZE * dpr
  canvas.height = SIZE * dpr
  const ctx = canvas.getContext('2d')
  ctx.scale(dpr, dpr)

  // Equilateral triangle pointing up, inscribed in a SIZE×SIZE box
  // with a small margin so the stroke doesn't clip.
  const cx = SIZE / 2
  const top = 4
  const bottom = SIZE - 4
  const halfW = (bottom - top) * (Math.sqrt(3) / 2) * 0.5
  ctx.beginPath()
  ctx.moveTo(cx, top)
  ctx.lineTo(cx + halfW, bottom)
  ctx.lineTo(cx - halfW, bottom)
  ctx.closePath()
  ctx.lineJoin = 'round'
  ctx.lineWidth = 4
  ctx.strokeStyle = getCycleGlyphHaloColor()
  ctx.stroke()
  ctx.fillStyle = getCycleGlyphColor()
  ctx.fill()

  // Centered "!" character so the glyph reads as a warning.
  ctx.fillStyle = getCycleGlyphHaloColor()
  ctx.font = `700 ${SIZE * 0.5}px -apple-system, "Segoe UI", Roboto, sans-serif`
  ctx.textBaseline = 'middle'
  ctx.textAlign = 'center'
  ctx.fillText('!', cx, SIZE * 0.6)

  const tex = new THREE.CanvasTexture(canvas)
  tex.minFilter = THREE.LinearFilter
  tex.magFilter = THREE.LinearFilter
  tex.anisotropy = 4

  const mat = new THREE.SpriteMaterial({
    map: tex,
    transparent: true,
    depthWrite: false,
    depthTest: true,
  })
  return new THREE.Sprite(mat)
}

/** @typedef {{
 *   node: any,
 *   group: THREE.Group,
 *   sphere: THREE.Mesh,
 *   label: THREE.Sprite,
 *   pickMesh: THREE.Mesh,
 *   cycleGlyph?: THREE.Sprite,
 * }} NodeRecord */
