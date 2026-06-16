// Raycasting + hit-area expansion for the 2.5D Isometric Graph Explorer
// (feature 006).
//
// FR-014a/b: every node MUST expose a clickable / hover-target hit area at
// least 24 pixels in diameter regardless of its visual size. The visual
// mesh remains exactly the centrality-driven size (FR-014b — visual
// encoding is not inflated to compensate for input ergonomics).
//
// Implementation: a separate transparent picking mesh per node lives on a
// dedicated raycast layer (PICK_LAYER). The raycaster ignores the visible
// scene meshes and tests only against the picking layer. Picking-mesh
// scale is recomputed in screen space, so 24 px on screen translates to
// the right world-units radius at the current camera.zoom.
//
// Foundational phase: this module exposes the API but does not yet add
// real picking meshes to the scene — node creation and pick-mesh
// registration land in US1 (T026) and US5 (T053) respectively.

import * as THREE from 'three'

export const PICK_LAYER = 1
export const EDGE_PICK_LAYER = 2
export const LAYER_PICK_LAYER = 3
export const HIT_TARGET_PIXELS = 24
export const EDGE_HIT_TARGET_PIXELS = 14

/**
 * Create a Picker bound to a camera + canvas. Each picker owns a
 * raycaster restricted to PICK_LAYER and a registry mapping THREE.Object3D
 * UUIDs back to wire-format node IDs (so callers receive `nodeID`
 * strings, not Three.js objects).
 *
 * @param {THREE.OrthographicCamera} camera
 * @param {HTMLCanvasElement} canvas
 */
export function createPicker(camera, canvas) {
  const nodeRaycaster = new THREE.Raycaster()
  nodeRaycaster.layers.set(PICK_LAYER)
  const edgeRaycaster = new THREE.Raycaster()
  edgeRaycaster.layers.set(EDGE_PICK_LAYER)
  const layerRaycaster = new THREE.Raycaster()
  layerRaycaster.layers.set(LAYER_PICK_LAYER)
  /** @type {Map<string, string>} uuid → nodeID */
  const nodeIDByUUID = new Map()
  /** @type {Map<string, any>} uuid → edge record (caller-supplied ref) */
  const edgeRefByUUID = new Map()
  /** @type {Map<string, string>} uuid → layer contextId (the actual mesh raycasted) */
  const layerCtxByUUID = new Map()
  /** @type {THREE.Mesh[]} picking meshes registered in the scene */
  const pickMeshes = []
  /** @type {THREE.Mesh[]} edge pick proxies (one cylinder per polyline segment) */
  const edgePickMeshes = []
  /** @type {THREE.Mesh[]} layer plane meshes (raycasted directly — no proxy) */
  const layerPickMeshes = []
  const tmpVec = new THREE.Vector2()

  /**
   * Attach a transparent picking mesh next to a node mesh. Caller is
   * responsible for adding the picking mesh to the scene (we don't add it
   * here so the caller controls grouping).
   *
   * @param {string} nodeID         Wire ID from NodeView.id.
   * @param {THREE.Vector3} worldPosition
   * @returns {THREE.Mesh}
   */
  function makePickMesh(nodeID, worldPosition) {
    // 1-unit radius sphere; we set scale based on screen-space pixel
    // size on every camera change.
    const geom = new THREE.SphereGeometry(1, 8, 8)
    const mat = new THREE.MeshBasicMaterial({
      transparent: true,
      opacity: 0,
      depthWrite: false,
    })
    const mesh = new THREE.Mesh(geom, mat)
    mesh.position.copy(worldPosition)
    mesh.layers.set(PICK_LAYER)
    nodeIDByUUID.set(mesh.uuid, nodeID)
    pickMeshes.push(mesh)
    rescaleOne(mesh)
    return mesh
  }

  /**
   * Attach invisible cylinder pick proxies along an edge's polyline
   * waypoints. One cylinder per segment between consecutive points.
   * Returns a THREE.Group that the caller adds to the scene; the
   * group's children are the per-segment proxies, each tagged with
   * `edgeRef` so the raycaster hit→ref lookup is O(1).
   *
   * @param {any} edgeRef             Caller-supplied identity passed back from pickEdge.
   * @param {THREE.Vector3[]} points  Polyline waypoints (≥2).
   * @returns {THREE.Group}
   */
  function makeEdgePickMesh(edgeRef, points) {
    const group = new THREE.Group()
    if (!points || points.length < 2) return group
    for (let i = 0; i < points.length - 1; i++) {
      const a = points[i]
      const b = points[i + 1]
      const len = a.distanceTo(b)
      if (len < 1e-6) continue
      // Cylinder of unit radius; world-radius is set via scale.x / .z
      // on every camera change so the hit target stays at a fixed
      // screen-space size.
      const geom = new THREE.CylinderGeometry(1, 1, len, 6, 1, true)
      const mat = new THREE.MeshBasicMaterial({
        transparent: true,
        opacity: 0,
        depthWrite: false,
        side: THREE.DoubleSide,
      })
      const cyl = new THREE.Mesh(geom, mat)
      cyl.position.copy(a).add(b).multiplyScalar(0.5)
      // CylinderGeometry's local axis is +Y. Align it with (b - a).
      const dir = new THREE.Vector3().subVectors(b, a).normalize()
      cyl.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), dir)
      cyl.layers.set(EDGE_PICK_LAYER)
      cyl.userData.segmentLength = len
      edgeRefByUUID.set(cyl.uuid, edgeRef)
      edgePickMeshes.push(cyl)
      group.add(cyl)
      rescaleEdgeOne(cyl)
    }
    return group
  }

  /**
   * Register an existing plane mesh (the visible cluster grid plane)
   * as a raycast target for layer-hover detection. The mesh's `layers`
   * are augmented so it stays rendered on the default layer AND is
   * raycastable on LAYER_PICK_LAYER. Caller owns the mesh's lifetime
   * (we don't dispose its geometry/material here).
   *
   * @param {string} contextId   The layer's wire ContextID.
   * @param {THREE.Mesh} planeMesh
   */
  function makeLayerPickMesh(contextId, planeMesh) {
    if (!planeMesh) return
    planeMesh.layers.enable(LAYER_PICK_LAYER)
    layerCtxByUUID.set(planeMesh.uuid, contextId)
    layerPickMeshes.push(planeMesh)
  }

  /** Remove every registered picking mesh + free GPU resources. */
  function clear() {
    for (const m of pickMeshes) {
      m.geometry.dispose()
      m.material.dispose()
      m.parent?.remove(m)
    }
    pickMeshes.length = 0
    nodeIDByUUID.clear()
    clearEdges()
    clearLayers()
  }

  function clearEdges() {
    for (const m of edgePickMeshes) {
      m.geometry.dispose()
      m.material.dispose()
      m.parent?.remove(m)
    }
    edgePickMeshes.length = 0
    edgeRefByUUID.clear()
  }

  function clearLayers() {
    // Layer pick meshes are the visible plane meshes; LayerPlanes.js
    // owns their lifetime. Here we just drop the references and
    // disable our layer bit so the raycaster doesn't keep matching
    // them after dispose.
    for (const m of layerPickMeshes) {
      m.layers.disable(LAYER_PICK_LAYER)
    }
    layerPickMeshes.length = 0
    layerCtxByUUID.clear()
  }

  /**
   * Recompute screen-space scale on every picking mesh. Call after
   * camera.zoom changes or canvas resize.
   */
  function rescaleAll() {
    for (const m of pickMeshes) rescaleOne(m)
    for (const m of edgePickMeshes) rescaleEdgeOne(m)
  }

  function rescaleOne(mesh) {
    // For an orthographic camera, world-units-per-pixel along the screen
    // height = (top - bottom) / zoom / canvasHeight.
    const h = canvas.clientHeight || 1
    const worldPerPx = (camera.top - camera.bottom) / camera.zoom / h
    const radiusWorld = (HIT_TARGET_PIXELS / 2) * worldPerPx
    mesh.scale.setScalar(radiusWorld)
  }

  function rescaleEdgeOne(mesh) {
    const h = canvas.clientHeight || 1
    const worldPerPx = (camera.top - camera.bottom) / camera.zoom / h
    const radiusWorld = (EDGE_HIT_TARGET_PIXELS / 2) * worldPerPx
    // Y axis is the cylinder's length axis — leave it at 1 so the
    // segment-length geometry isn't stretched. Only the perpendicular
    // axes (X, Z in local space) get the screen-space radius scale.
    mesh.scale.set(radiusWorld, 1, radiusWorld)
  }

  /**
   * Pick the topmost node at a screen-space pointer position.
   * @param {number} clientX
   * @param {number} clientY
   * @returns {string|null} nodeID or null if nothing was hit
   */
  function pick(clientX, clientY) {
    const rect = canvas.getBoundingClientRect()
    tmpVec.x = ((clientX - rect.left) / rect.width) * 2 - 1
    tmpVec.y = -((clientY - rect.top) / rect.height) * 2 + 1
    nodeRaycaster.setFromCamera(tmpVec, camera)
    if (pickMeshes.length === 0) return null
    const hits = nodeRaycaster.intersectObjects(pickMeshes, /* recursive */ false)
    if (hits.length === 0) return null
    return nodeIDByUUID.get(hits[0].object.uuid) ?? null
  }

  /**
   * Pick the topmost layer plane at a screen-space pointer position.
   * Returns the layer's contextId, or null if no plane is under the
   * cursor. Used to surface the full context tag list on hover.
   * @param {number} clientX
   * @param {number} clientY
   * @returns {string|null}
   */
  function pickLayer(clientX, clientY) {
    const rect = canvas.getBoundingClientRect()
    tmpVec.x = ((clientX - rect.left) / rect.width) * 2 - 1
    tmpVec.y = -((clientY - rect.top) / rect.height) * 2 + 1
    layerRaycaster.setFromCamera(tmpVec, camera)
    if (layerPickMeshes.length === 0) return null
    const hits = layerRaycaster.intersectObjects(layerPickMeshes, /* recursive */ false)
    if (hits.length === 0) return null
    return layerCtxByUUID.get(hits[0].object.uuid) ?? null
  }

  /**
   * Pick the topmost edge at a screen-space pointer position. Returns
   * the caller-supplied edgeRef (typically an EdgeView) or null.
   * @param {number} clientX
   * @param {number} clientY
   * @returns {any|null}
   */
  function pickEdge(clientX, clientY) {
    const rect = canvas.getBoundingClientRect()
    tmpVec.x = ((clientX - rect.left) / rect.width) * 2 - 1
    tmpVec.y = -((clientY - rect.top) / rect.height) * 2 + 1
    edgeRaycaster.setFromCamera(tmpVec, camera)
    if (edgePickMeshes.length === 0) return null
    const hits = edgeRaycaster.intersectObjects(edgePickMeshes, /* recursive */ false)
    if (hits.length === 0) return null
    return edgeRefByUUID.get(hits[0].object.uuid) ?? null
  }

  return {
    makePickMesh,
    makeEdgePickMesh,
    makeLayerPickMesh,
    clear,
    clearEdges,
    clearLayers,
    rescaleAll,
    pick,
    pickEdge,
    pickLayer,
    /** Test-only access to the registered count. */
    get registeredCount() { return pickMeshes.length },
    get edgeRegisteredCount() { return edgePickMeshes.length },
    get layerRegisteredCount() { return layerPickMeshes.length },
  }
}
