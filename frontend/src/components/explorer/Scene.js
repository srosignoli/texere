// Three.js scene + locked-isometric camera for the 2.5D Isometric Graph
// Explorer (feature 006). This module owns the WebGL renderer, the
// orthographic camera, and the custom pan-only / zoom-only controller.
//
// Hard guarantee: there is NO code path in this module that mutates
// camera.quaternion. Rotation is physically impossible from any input
// (FR-003). All input is interpreted as either pan (X/Z translation) or
// zoom (orthographic camera.zoom property).
//
// The camera looks at the origin from a fixed isometric vantage at roughly
// a 35° downward angle (FR-001). The scene uses Y-up world coordinates
// where:
//   +Y = elevation (containment hierarchy)
//   +X = horizontal flow (causal "leads to")
//   +Z = near depth (properties)
//   -Z = far depth (analogies / "similar to")
//
// Foundational phase: this module sets up the scene shell. Node, edge,
// territory rendering land in later phases (US1+).

import * as THREE from 'three'

// Zoom bounds (FR-005, T032). The orthographic camera's `zoom` property
// scales the visible world *down* as it goes UP — at zoom=2 you see
// half the world width, at zoom=0.5 you see twice. We pick:
//
//   ZOOM_MIN=0.2 — at the lower bound the entire ≤500-node landscape
//     fits comfortably in the viewport without labels (which use
//     constant on-screen pixel size, FR-015) overlapping into illegible
//     soup. Going lower would compress the landscape into a blob.
//   ZOOM_MAX=8.0 — at the upper bound a single landmark node fills
//     roughly a quarter of the viewport. Going higher would put us
//     inside a single sphere with no surrounding context.
const ZOOM_MIN = 0.2
const ZOOM_MAX = 8.0
const ISO_ANGLE_DEG = 35
const CAMERA_DISTANCE = 30 // world units; orthographic so this is geometric only

/**
 * Create the explorer scene and renderer. The caller is responsible for
 * calling dispose() when the canvas is being unmounted.
 *
 * @param {HTMLCanvasElement} canvas
 * @param {{
 *   onCameraChange?: () => void,
 * }} [options]  onCameraChange fires after every pan / zoom / resize so
 *               the host can rescale screen-space-sized objects (label
 *               sprites, picking meshes) before the next render.
 * @returns {{
 *   scene: THREE.Scene,
 *   camera: THREE.OrthographicCamera,
 *   renderer: THREE.WebGLRenderer,
 *   render: () => void,
 *   handleResize: () => void,
 *   dispose: () => void,
 * }}
 */
export function createScene(canvas, options = {}) {
  const onCameraChange = options.onCameraChange

  const scene = new THREE.Scene()
  scene.background = null // transparent — host page provides theme background

  const camera = createIsometricCamera(canvas)
  positionIsometric(camera)

  const renderer = new THREE.WebGLRenderer({
    canvas,
    antialias: true,
    alpha: true,
  })
  renderer.setPixelRatio(window.devicePixelRatio || 1)
  sizeRenderer(renderer, canvas)

  // Pan + zoom controller — strictly NO rotation.
  const controller = installPanZoomController(canvas, camera, fireChange)

  // Resize observer keeps the renderer + camera frustum in sync with the
  // canvas's CSS-rendered size.
  const ro = new ResizeObserver(() => {
    sizeRenderer(renderer, canvas)
    updateCameraFrustum(camera, canvas)
    fireChange()
  })
  ro.observe(canvas)

  // Suppress browser context menu over the canvas — mouse buttons 4/5
  // (history navigation) and right-click are reserved for the explorer
  // (right-click currently no-op in the foundational phase).
  const blockContext = (e) => e.preventDefault()
  canvas.addEventListener('contextmenu', blockContext)

  let frame = 0
  function render() {
    renderer.render(scene, camera)
    frame++
  }

  // fireChange is invoked by pan/zoom/resize. It gives the host a chance
  // to update screen-space-sized objects (e.g. label sprites, picking
  // meshes) BEFORE the next render() so the user never sees a frame
  // with stale sizing. If onCameraChange isn't supplied, fireChange just
  // renders.
  function fireChange() {
    if (onCameraChange) onCameraChange()
    render()
  }

  function dispose() {
    ro.disconnect()
    canvas.removeEventListener('contextmenu', blockContext)
    controller.dispose()
    renderer.dispose()
    // Remove all children + dispose geometries/materials so GC can run.
    scene.traverse((obj) => {
      if (obj.geometry?.dispose) obj.geometry.dispose()
      if (obj.material?.dispose) obj.material.dispose()
    })
  }

  return {
    scene,
    camera,
    renderer,
    render,
    handleResize: () => {
      sizeRenderer(renderer, canvas)
      updateCameraFrustum(camera, canvas)
      fireChange()
    },
    dispose,
  }
}

// =============================================================================
// Helpers (no rotation anywhere — defense in depth against future edits)
// =============================================================================

function createIsometricCamera(canvas) {
  const aspect = (canvas.clientWidth || 1) / (canvas.clientHeight || 1)
  const halfHeight = 10 // world units in view at zoom=1
  const halfWidth = halfHeight * aspect
  const cam = new THREE.OrthographicCamera(
    -halfWidth, halfWidth,
    halfHeight, -halfHeight,
    -100, 100,
  )
  cam.zoom = 1.0
  cam.updateProjectionMatrix()
  return cam
}

function positionIsometric(camera) {
  // Place the camera on a fixed unit-vector ray and look at the origin.
  // The ISO_ANGLE_DEG controls the downward tilt; the azimuth is fixed at
  // 45° so X/Z map to screen-diagonals consistently.
  const angle = (ISO_ANGLE_DEG * Math.PI) / 180
  const azim = Math.PI / 4 // 45° around Y
  const r = CAMERA_DISTANCE
  camera.position.set(
    r * Math.cos(angle) * Math.cos(azim),
    r * Math.sin(angle),
    r * Math.cos(angle) * Math.sin(azim),
  )
  camera.up.set(0, 1, 0)
  camera.lookAt(0, 0, 0) // sets quaternion ONCE at construction; never re-set
}

function sizeRenderer(renderer, canvas) {
  const w = canvas.clientWidth || 1
  const h = canvas.clientHeight || 1
  renderer.setSize(w, h, /* updateStyle */ false)
}

function updateCameraFrustum(camera, canvas) {
  const aspect = (canvas.clientWidth || 1) / (canvas.clientHeight || 1)
  const halfHeight = 10
  const halfWidth = halfHeight * aspect
  camera.left = -halfWidth
  camera.right = halfWidth
  camera.top = halfHeight
  camera.bottom = -halfHeight
  camera.updateProjectionMatrix()
}

// installPanZoomController wires pointer + wheel events to camera position
// and zoom. ZERO rotation behavior.
function installPanZoomController(canvas, camera, onChange) {
  const state = {
    pointerDown: false,
    lastX: 0,
    lastY: 0,
  }

  function onPointerDown(e) {
    // Only buttons 0 (primary) and 1 (middle) initiate pan.
    // Buttons 3/4 (back/forward) are reserved for US6 anchor history.
    if (e.button !== 0 && e.button !== 1) return
    state.pointerDown = true
    state.lastX = e.clientX
    state.lastY = e.clientY
    canvas.setPointerCapture?.(e.pointerId)
  }

  function onPointerUp(e) {
    state.pointerDown = false
    canvas.releasePointerCapture?.(e.pointerId)
  }

  function onPointerMove(e) {
    if (!state.pointerDown) return
    const dx = e.clientX - state.lastX
    const dy = e.clientY - state.lastY
    state.lastX = e.clientX
    state.lastY = e.clientY
    panCamera(camera, dx, dy, canvas)
    onChange()
  }

  function onWheel(e) {
    e.preventDefault()
    // ctrl+wheel and pinch are zoom; plain wheel is also zoom in this
    // explorer (FR-005). Pan is always pointer-driven.
    const factor = Math.exp(-e.deltaY * 0.0015)
    setZoom(camera, camera.zoom * factor)
    onChange()
  }

  canvas.addEventListener('pointerdown', onPointerDown)
  canvas.addEventListener('pointerup', onPointerUp)
  canvas.addEventListener('pointercancel', onPointerUp)
  canvas.addEventListener('pointerleave', onPointerUp)
  canvas.addEventListener('pointermove', onPointerMove)
  canvas.addEventListener('wheel', onWheel, { passive: false })

  return {
    dispose() {
      canvas.removeEventListener('pointerdown', onPointerDown)
      canvas.removeEventListener('pointerup', onPointerUp)
      canvas.removeEventListener('pointercancel', onPointerUp)
      canvas.removeEventListener('pointerleave', onPointerUp)
      canvas.removeEventListener('pointermove', onPointerMove)
      canvas.removeEventListener('wheel', onWheel)
    },
  }
}

// panCamera translates the camera along the world ground plane (X/Z) only.
// Y (elevation) and the camera's quaternion are never touched.
function panCamera(camera, dxScreen, dyScreen, canvas) {
  const w = canvas.clientWidth || 1
  const h = canvas.clientHeight || 1
  const worldPerPxX = (camera.right - camera.left) / camera.zoom / w
  const worldPerPxY = (camera.top - camera.bottom) / camera.zoom / h

  // Build screen-aligned basis vectors that map screen X/Y back to world
  // X/Z (parallel to the ground plane). Because the camera is fixed at
  // azim=45° and tilt=35°, these vectors are constants we could hardcode,
  // but deriving them from the camera matrix keeps the code robust to any
  // future tweak of CAMERA_DISTANCE / ISO_ANGLE_DEG.
  const right = new THREE.Vector3()
  const up = new THREE.Vector3()
  camera.matrixWorld.extractBasis(right, up, new THREE.Vector3())

  // Project right/up onto the ground plane (zero-out Y).
  right.y = 0; right.normalize()
  up.y = 0; up.normalize()

  const dxWorld = dxScreen * worldPerPxX
  const dyWorld = dyScreen * worldPerPxY

  camera.position.x -= right.x * dxWorld
  camera.position.z -= right.z * dxWorld
  camera.position.x += up.x * dyWorld
  camera.position.z += up.z * dyWorld
  // NOTE: position.y is never touched.
  // NOTE: camera.quaternion is never touched.
}

function setZoom(camera, z) {
  camera.zoom = clamp(z, ZOOM_MIN, ZOOM_MAX)
  camera.updateProjectionMatrix()
}

function clamp(v, lo, hi) {
  return Math.min(Math.max(v, lo), hi)
}
