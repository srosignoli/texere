<script>
  // Embeddable 2.5D isometric view (feature 021, Task 2). Composes the
  // existing explorer render modules against a `layout` prop (a
  // LayoutResult) without any Svelte store reads/writes, hover/click
  // picking, or reanchor. Pan/zoom come free from Scene.js's built-in
  // controller. Multiple instances may be mounted simultaneously, so
  // every three.js resource created here must be disposed on destroy.
  import { onMount, onDestroy } from 'svelte'
  import { createScene } from './explorer/Scene.js'
  import { createPicker } from './explorer/Picking.js'
  import { buildNodeMeshes } from './explorer/NodeMeshes.js'
  import { buildEdgeMeshes } from './explorer/EdgeMeshes.js'
  import { buildLayerPlanes } from './explorer/LayerPlanes.js'
  import { computeCameraFrame } from './explorer/ReanchorAnimator.js'

  export let layout = null
  export let height = 320

  let wrap, canvas
  let sceneCtx = null
  let picker = null
  let nodeMeshes = null
  let edgeMeshes = null
  let layerPlanes = null
  let ro = null
  let rafId = 0

  $: message = !layout ? '' : (layout.diagnostics && layout.diagnostics.graphError)
    ? layout.diagnostics.graphError
    : (!layout.nodes || layout.nodes.length === 0) ? 'No graph results.' : ''

  function disposeMeshes() {
    nodeMeshes?.dispose(); edgeMeshes?.dispose(); layerPlanes?.dispose()
    picker?.clear?.(); picker?.clearLayers?.()
    nodeMeshes = null; edgeMeshes = null; layerPlanes = null
  }

  function renderLayout(l) {
    if (!sceneCtx) return
    if (rafId) { cancelAnimationFrame(rafId); rafId = 0 }
    const invalid = !l || (l.diagnostics && l.diagnostics.graphError) || !l.nodes || l.nodes.length === 0
    if (invalid) { disposeMeshes(); sceneCtx.render(); return }
    disposeMeshes()
    layerPlanes = buildLayerPlanes(sceneCtx.scene, l, sceneCtx.camera, canvas, { opacity: 0.12, mode: 'stacked', picker })
    const t = computeCameraFrame(sceneCtx.camera, l.bounds)
    sceneCtx.camera.position.copy(t.position)
    sceneCtx.camera.zoom = t.zoom
    sceneCtx.camera.lookAt(t.lookAt)
    sceneCtx.camera.updateProjectionMatrix()
    sceneCtx.render()
    rafId = requestAnimationFrame(() => {
      rafId = 0
      nodeMeshes = buildNodeMeshes(sceneCtx.scene, l, sceneCtx.camera, canvas, picker, { mode: 'stacked' })
      edgeMeshes = buildEdgeMeshes(sceneCtx.scene, l, nodeMeshes, picker)
      nodeMeshes.setAllOpacity(1)
      edgeMeshes.setAllOpacity(1)
      sceneCtx.render()
    })
  }

  // Re-render whenever the layout changes (and once sceneCtx exists after mount).
  $: if (sceneCtx) renderLayout(layout)

  onMount(() => {
    sceneCtx = createScene(canvas, {
      onCameraChange: () => { nodeMeshes?.rescaleLabels?.(); picker?.rescaleAll?.() },
    })
    picker = createPicker(sceneCtx.camera, canvas)
    ro = new ResizeObserver(() => sceneCtx?.render())
    ro.observe(wrap)
    sceneCtx.render()
  })

  onDestroy(() => {
    if (rafId) cancelAnimationFrame(rafId)
    ro?.disconnect()
    disposeMeshes()
    sceneCtx?.dispose()
    sceneCtx = null; picker = null
  })
</script>

<div class="iso-wrap" bind:this={wrap} style="height:{height}px">
  <canvas bind:this={canvas}></canvas>
  {#if message}<div class="iso-msg">{message}</div>{/if}
</div>

<style>
  .iso-wrap { position: relative; width: 100%; background: #0b1220; border-radius: 6px; overflow: hidden; }
  .iso-wrap canvas { display: block; width: 100%; height: 100%; touch-action: none; }
  .iso-msg { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; color: #7a8ba0; font-size: 13px; padding: 12px; text-align: center; background: #0b1220; }
</style>
