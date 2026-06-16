// Layer planes for the Layered Knowledge Graph (post-territory redesign).
//
// Each context becomes a translucent grid floor on its own Y plane. The
// plane consists of:
//   1. A THREE.GridHelper drawing the grid lines.
//   2. A THREE.PlaneGeometry tinted underneath the grid so the surface
//      reads as a distinct floor even at low opacity.
//   3. A THREE.Sprite billboard label showing the context name, anchored
//      at the +X corner of the plane.
//
// The returned registry exposes hooks the sidebar controls drive:
//   setOpacity(t)             — global plane opacity (slider)
//   setSpacing(units)         — re-Y each plane to layerIdx * spacing
//   setLayerVisible(ctx, vis) — toggle a single plane on/off
//   dispose()                 — free GPU resources
//
// The planes hover at TERRITORY_Y above each layer's Y so the spheres
// sitting on the plane (at exactly layerY) don't z-fight with the grid.

import * as THREE from 'three'
import {
  COLOR_PLANE_GRID,
  COLOR_PLANE_TINT,
  COLOR_EXTENDED_PLANE_GRID,
  COLOR_EXTENDED_PLANE_TINT,
  COLOR_LABEL,
  COLOR_LABEL_HALO,
  PLANE_OPACITY_DEFAULT,
  TERRITORY_PALETTE,
} from './Theme.js'

// Padding around the (x, z) bounds when sizing each plane so nodes on
// the outer ring don't visually overflow the floor's edge.
const PLANE_PADDING = 2.4

// Dim fill for the chapter sub-label so it reads as secondary to the
// context name on the same label texture.
const COLOR_LABEL_SUB = 'rgba(200, 214, 230, 0.7)'

// Max chapter names shown in a grid's chapter sub-label before collapsing
// the rest into a "+N more" suffix (a context can span many chapters).
const CHAPTER_LABEL_CAP = 3

// formatChapters renders a layer's parent chapters as a compact sub-label
// string, e.g. "Cooking, Restaurants, Travel +2 more". Returns '' when the
// layer has no chapter attribution (e.g. the synthetic Untagged layer with
// orphan nodes), so callers can skip the sub-line entirely.
function formatChapters(chapters) {
  if (!Array.isArray(chapters) || chapters.length === 0) return ''
  if (chapters.length <= CHAPTER_LABEL_CAP) return chapters.join(', ')
  const shown = chapters.slice(0, CHAPTER_LABEL_CAP).join(', ')
  return `${shown} +${chapters.length - CHAPTER_LABEL_CAP} more`
}

// World-units height of the plane label sprite (recomputed against
// camera zoom every rescale tick).
const LABEL_PIXEL_HEIGHT = 18

// Z-fight offset: planes sit slightly below the nodes' Y so spheres
// look like they're resting ON the floor rather than floating through it.
const PLANE_Y_OFFSET = -0.05

/**
 * Build layer planes for a LayoutResult.
 *
 * Stacked mode (default): one translucent grid floor per context,
 * stacked along Y. Each plane is tinted by its context's palette
 * color and labelled with the context name.
 *
 * Extended mode: a single, larger grid plane covering all nodes on
 * Y=0. No per-context labels (context is conveyed via node tint on
 * the frontend NodeMeshes side). The plane uses a distinct neutral
 * tint so the user can tell at a glance which mode is active.
 *
 * @param {THREE.Scene} scene
 * @param {{layers: Array<{contextId: string, label: string, y: number, nodeCount: number, colorSlot: number}>, bounds: {minX: number, maxX: number, minZ: number, maxZ: number}}} layoutResult
 * @param {THREE.OrthographicCamera} camera
 * @param {HTMLCanvasElement} canvas
 * @param {{opacity?: number, mode?: 'stacked' | 'extended', picker?: ReturnType<typeof import('./Picking.js').createPicker>}} [options]
 * @returns {{
 *   all(): Array<LayerRecord>,
 *   setOpacity(t: number): void,
 *   setSpacing(units: number): void,
 *   setLayerVisible(contextId: string, visible: boolean): void,
 *   rescaleLabels(): void,
 *   dispose(): void,
 * }}
 */
export function buildLayerPlanes(scene, layoutResult, camera, canvas, options = {}) {
  const layers = layoutResult.layers ?? []
  const bounds = layoutResult.bounds ?? { minX: -8, maxX: 8, minZ: -8, maxZ: 8 }
  const mode = options.mode ?? 'stacked'
  const picker = options.picker ?? null

  // Plane size: cover the (x, z) extent of every node + a generous
  // padding so the floor reads as enclosing the cluster.
  const width = Math.max(8, (bounds.maxX - bounds.minX) + PLANE_PADDING * 2)
  const depth = Math.max(8, (bounds.maxZ - bounds.minZ) + PLANE_PADDING * 2)
  const cx = (bounds.maxX + bounds.minX) / 2
  const cz = (bounds.maxZ + bounds.minZ) / 2

  // Grid division: roughly one cell per world unit, capped so dense
  // results stay legible.
  const gridDivisions = Math.max(8, Math.min(40, Math.round(Math.max(width, depth) / 1.5)))
  const gridSize = Math.max(width, depth)

  /** @type {Array<LayerRecord>} */
  const records = []
  let opacity = options.opacity ?? PLANE_OPACITY_DEFAULT

  // Extended mode: per-cluster colored sub-planes on the shared Y=0
  // floor. Each cluster gets its own tinted patch sized to the
  // cluster's bounding box (centerX/Z + extentX/Z from the wire), the
  // same palette colour its stacked floor would have used, and a
  // corner label sprite with the context name — matching the
  // stacked-mode visual language while keeping every cluster on the
  // same plane.
  if (mode === 'extended') {
    for (const layer of layers) {
      const subWidth = Math.max(6, layer.extentX * 2 + PLANE_PADDING * 2)
      const subDepth = Math.max(6, layer.extentZ * 2 + PLANE_PADDING * 2)
      const subDivisions = Math.max(4, Math.min(24, Math.round(Math.max(subWidth, subDepth) / 1.2)))
      const subGridSize = Math.max(subWidth, subDepth)

      const group = new THREE.Group()
      group.userData.contextId = layer.contextId
      group.position.set(layer.centerX, PLANE_Y_OFFSET, layer.centerZ)

      const tintColor = paletteColor(layer.colorSlot)
      const gridColor = brighten(tintColor, 0.55)

      const planeGeo = new THREE.PlaneGeometry(subWidth, subDepth)
      const planeMat = new THREE.MeshBasicMaterial({
        color: tintColor,
        transparent: true,
        opacity: opacity * 0.6,
        side: THREE.DoubleSide,
        depthWrite: false,
      })
      const plane = new THREE.Mesh(planeGeo, planeMat)
      plane.rotation.x = -Math.PI / 2
      group.add(plane)
      picker?.makeLayerPickMesh(layer.contextId, plane)

      const grid = new THREE.GridHelper(subGridSize, subDivisions, gridColor, gridColor)
      grid.material.transparent = true
      grid.material.opacity = Math.min(1, opacity * 1.8)
      grid.material.depthWrite = false
      group.add(grid)

      // Flat label aligned with the cluster's FRONT (+Z) edge — the
      // one that projects to the bottom of the screen at the iso
      // vantage and is therefore the most visible / least occluded
      // by the cluster's own nodes. Text runs along +X so it reads
      // left-to-right (with a mild diagonal tilt from the iso
      // projection), top of letters facing away from the camera.
      const labelHeightWu = Math.min(1.6, Math.max(0.9, subWidth * 0.14))
      const labelMesh = createFlatPlaneLabel(layer.label, labelHeightWu, formatChapters(layer.chapters))
      labelMesh.position.set(0, 0.05, subDepth / 2 + labelHeightWu * 0.7)
      group.add(labelMesh)

      scene.add(group)
      records.push({ layer, group, plane, grid, labelSprite: labelMesh })
    }
    return makeRegistry(records, scene, camera, canvas, mode, (t) => (opacity = t))
  }

  for (const layer of layers) {
    const group = new THREE.Group()
    group.userData.contextId = layer.contextId
    group.position.set(cx, layer.y + PLANE_Y_OFFSET, cz)

    // Per-layer colour. The Go layer assigns colorSlot from the sorted
    // contexts list (Untagged sentinel is -1). Tinted floors give each
    // context a clear visual identity even when several layers stack
    // close together. Untagged falls back to the neutral tint.
    const tintColor = paletteColor(layer.colorSlot)
    const gridColor = brighten(tintColor, 0.55)

    // Tint plane — translucent fill so the floor has a recognizable
    // surface area even when the grid lines fade out.
    const planeGeo = new THREE.PlaneGeometry(width, depth)
    const planeMat = new THREE.MeshBasicMaterial({
      color: tintColor,
      transparent: true,
      opacity: opacity * 0.6,
      side: THREE.DoubleSide,
      depthWrite: false,
    })
    const plane = new THREE.Mesh(planeGeo, planeMat)
    plane.rotation.x = -Math.PI / 2
    group.add(plane)
    picker?.makeLayerPickMesh(layer.contextId, plane)

    // Grid lines — the dominant visual cue. Lighter shade of the
    // layer's tint so the colour identity carries through.
    const grid = new THREE.GridHelper(gridSize, gridDivisions, gridColor, gridColor)
    grid.material.transparent = true
    grid.material.opacity = Math.min(1, opacity * 1.8)
    grid.material.depthWrite = false
    group.add(grid)

    // Context label sprite — corner of the plane so it doesn't sit
    // over node spheres. Constant on-screen height per rescaleLabels.
    const labelSprite = createPlaneLabelSprite(layer.label, formatChapters(layer.chapters))
    labelSprite.position.set(
      (width / 2) - PLANE_PADDING * 0.8,
      0.08,
      -(depth / 2) + PLANE_PADDING * 0.8,
    )
    group.add(labelSprite)

    scene.add(group)
    records.push({ layer, group, plane, grid, labelSprite })
  }

  return makeRegistry(records, scene, camera, canvas, mode, (t) => (opacity = t))
}

// makeRegistry builds the lifecycle API returned to callers. Shared
// between stacked and extended modes so opacity / visibility / dispose
// behave uniformly regardless of how many planes there are.
function makeRegistry(records, scene, camera, canvas, mode, setOpacityRef) {
  rescaleLabels(records, camera, canvas)
  return {
    all() {
      return records
    },
    setOpacity(t) {
      setOpacityRef(t)
      for (const r of records) {
        r.plane.material.opacity = t * 0.6
        r.grid.material.opacity = Math.min(1, t * 1.8)
      }
    },
    setSpacing(units) {
      // Extended mode has a single shared plane at Y=0 regardless of
      // spacing — the slider is meaningful only in stacked mode.
      if (mode === 'extended') return
      for (let i = 0; i < records.length; i++) {
        const r = records[i]
        const newY = i * units + PLANE_Y_OFFSET
        r.group.position.y = newY
        r.layer.y = i * units
      }
    },
    setLayerVisible(contextId, visible) {
      // Both modes now have per-context groups (stacked → per-floor;
      // extended → per-cluster sub-plane), so hiding a layer hides
      // both its nodes (via NodeMeshes) AND its colored region here.
      for (const r of records) {
        if (r.layer.contextId === contextId) {
          r.group.visible = visible
        }
      }
    },
    rescaleLabels: () => rescaleLabels(records, camera, canvas),
    dispose() {
      for (const r of records) {
        scene.remove(r.group)
        r.plane.geometry.dispose()
        r.plane.material.dispose()
        r.grid.geometry.dispose()
        r.grid.material.dispose()
        if (r.labelSprite) {
          r.labelSprite.material.map?.dispose()
          r.labelSprite.material.dispose()
        }
      }
      records.length = 0
    },
  }
}

function rescaleLabels(records, camera, canvas) {
  if (!camera || !canvas) return
  const h = canvas.clientHeight || 1
  const worldPerPx = (camera.top - camera.bottom) / camera.zoom / h
  for (const r of records) {
    if (!r.labelSprite) continue
    // Flat plane labels are sized in world units and should NOT
    // rescale with zoom (that's the point — they sit on the grid).
    if (!r.labelSprite.isSprite) continue
    const tex = r.labelSprite.material.map
    const aspect = (tex?.image?.width ?? 1) / (tex?.image?.height ?? 1)
    // Scale per text line so a two-line (context + chapters) label keeps the
    // primary line at the same on-screen size as a single-line label.
    const lines = r.labelSprite.userData.labelLines ?? 1
    const worldHeight = LABEL_PIXEL_HEIGHT * lines * worldPerPx
    r.labelSprite.scale.set(worldHeight * aspect, worldHeight, 1)
  }
}

// paletteColor returns the layer's tint hex. Untagged (colorSlot < 0)
// uses the neutral fallback so the synthetic "no context" layer reads
// as a plain grey floor — visually distinct from any user-tagged
// context which gets a saturated palette colour.
function paletteColor(slot) {
  if (slot === undefined || slot === null || slot < 0) return COLOR_PLANE_TINT
  return TERRITORY_PALETTE[slot % TERRITORY_PALETTE.length]
}

// brighten lerps a 24-bit hex colour toward white by `amount` (0..1).
// Used so the grid lines on each plane are a lighter shade of the
// plane's tint, giving the floor a clear coloured identity without
// overpowering the nodes.
function brighten(hex, amount) {
  const r = (hex >> 16) & 0xff
  const g = (hex >> 8) & 0xff
  const b = hex & 0xff
  const lr = Math.min(255, Math.round(r + (255 - r) * amount))
  const lg = Math.min(255, Math.round(g + (255 - g) * amount))
  const lb = Math.min(255, Math.round(b + (255 - b) * amount))
  return (lr << 16) | (lg << 8) | lb
}

// createFlatPlaneLabel builds a textured plane that lays flat on the
// ground (XZ) plane with text running along +Z. Used in Extended mode
// to label each cluster's grid region parallel to its right edge —
// matches the orientation of an axis label on a chart.
//
// The plane's world size is anchored to `heightWu` (text height) so
// the label stays at a fixed world size as the camera zooms (unlike
// the per-floor sprite labels which scale to a constant pixel size).
function createFlatPlaneLabel(text, heightWu, subText = '') {
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  const fontSize = 64
  const subFontSize = 40 // chapter sub-line, smaller than the context name
  const lineGap = 8
  const padding = 14
  const hasSub = !!subText

  const measureCanvas = document.createElement('canvas')
  const measureCtx = measureCanvas.getContext('2d')
  measureCtx.font = `700 ${fontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
  const textWidth = Math.ceil(measureCtx.measureText(text || ' ').width)
  let subWidth = 0
  if (hasSub) {
    measureCtx.font = `500 ${subFontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
    subWidth = Math.ceil(measureCtx.measureText(subText).width)
  }

  const canvasWidth = Math.max(textWidth, subWidth) + padding * 2
  const canvasHeight = fontSize + (hasSub ? lineGap + subFontSize : 0) + padding * 2
  const canvas = document.createElement('canvas')
  canvas.width = canvasWidth * dpr
  canvas.height = canvasHeight * dpr
  const ctx = canvas.getContext('2d')
  ctx.scale(dpr, dpr)
  ctx.textBaseline = 'middle'
  ctx.textAlign = 'left'

  // Context name (primary line).
  ctx.font = `700 ${fontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
  const titleY = fontSize / 2 + padding
  ctx.lineWidth = 6
  ctx.strokeStyle = COLOR_LABEL_HALO
  ctx.strokeText(text, padding, titleY)
  ctx.fillStyle = COLOR_LABEL
  ctx.fillText(text, padding, titleY)

  // Chapter sub-line (secondary, dimmer).
  if (hasSub) {
    ctx.font = `500 ${subFontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
    const subY = fontSize + lineGap + subFontSize / 2 + padding
    ctx.lineWidth = 5
    ctx.strokeStyle = COLOR_LABEL_HALO
    ctx.strokeText(subText, padding, subY)
    ctx.fillStyle = COLOR_LABEL_SUB
    ctx.fillText(subText, padding, subY)
  }

  const tex = new THREE.CanvasTexture(canvas)
  tex.minFilter = THREE.LinearFilter
  tex.magFilter = THREE.LinearFilter
  tex.anisotropy = 4

  // World-unit sizing: text rendered at fontSize px maps to heightWu
  // world units in plane height. Width follows canvas aspect.
  const aspect = canvasWidth / canvasHeight
  const worldHeight = heightWu
  const worldWidth = worldHeight * aspect

  const geom = new THREE.PlaneGeometry(worldWidth, worldHeight)
  const mat = new THREE.MeshBasicMaterial({
    map: tex,
    transparent: true,
    depthWrite: false,
    side: THREE.DoubleSide,
  })
  const mesh = new THREE.Mesh(geom, mat)

  // Lay flat on the XZ ground plane. After the -π/2 rotation around
  // world X: canvas-X stays aligned with world +X (text reads
  // left-to-right along the X axis), canvas-Y maps to world -Z (top
  // of letters points to -Z = away from the iso camera, so the text
  // appears right-side-up to a viewer looking down from +X+Y+Z).
  // The plane's normal (originally +Z in local) ends up at world +Y,
  // i.e. facing the camera.
  mesh.rotation.x = -Math.PI / 2
  return mesh
}

function createPlaneLabelSprite(text, subText = '') {
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  const fontSize = 36
  const subFontSize = 24 // chapter sub-line
  const lineGap = 5
  const padding = 12
  const hasSub = !!subText

  const measureCanvas = document.createElement('canvas')
  const measureCtx = measureCanvas.getContext('2d')
  measureCtx.font = `700 ${fontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
  const textWidth = Math.ceil(measureCtx.measureText(text || ' ').width)
  let subWidth = 0
  if (hasSub) {
    measureCtx.font = `500 ${subFontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
    subWidth = Math.ceil(measureCtx.measureText(subText).width)
  }

  const canvasW = Math.max(textWidth, subWidth) + padding * 2
  const canvasH = fontSize + (hasSub ? lineGap + subFontSize : 0) + padding * 2
  const canvas = document.createElement('canvas')
  canvas.width = canvasW * dpr
  canvas.height = canvasH * dpr
  const ctx = canvas.getContext('2d')
  ctx.scale(dpr, dpr)
  ctx.textBaseline = 'middle'
  ctx.textAlign = 'left'

  ctx.font = `700 ${fontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
  const titleY = fontSize / 2 + padding
  ctx.lineWidth = 5
  ctx.strokeStyle = COLOR_LABEL_HALO
  ctx.strokeText(text, padding, titleY)
  ctx.fillStyle = COLOR_LABEL
  ctx.fillText(text, padding, titleY)

  if (hasSub) {
    ctx.font = `500 ${subFontSize}px -apple-system, "Segoe UI", Roboto, sans-serif`
    const subY = fontSize + lineGap + subFontSize / 2 + padding
    ctx.lineWidth = 4
    ctx.strokeStyle = COLOR_LABEL_HALO
    ctx.strokeText(subText, padding, subY)
    ctx.fillStyle = COLOR_LABEL_SUB
    ctx.fillText(subText, padding, subY)
  }

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
  const sprite = new THREE.Sprite(mat)
  // Two-line labels need extra on-screen height so the primary line doesn't
  // shrink — rescaleLabels reads this to scale the taller canvas up.
  sprite.userData.labelLines = hasSub ? 2 : 1
  return sprite
}

/** @typedef {{
 *   layer: {contextId: string, label: string, y: number, nodeCount: number, colorSlot: number},
 *   group: THREE.Group,
 *   plane: THREE.Mesh,
 *   grid: THREE.GridHelper,
 *   labelSprite: THREE.Sprite,
 * }} LayerRecord */
