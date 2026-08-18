<script>
  // Mini-map overlay for the 2.5D Isometric Graph Explorer.
  //
  // Renders a top-down (XZ) overview of every context cluster plus a
  // viewport rectangle showing the camera's current visible region.
  // Works in BOTH stacked and extended modes — the rectangles are
  // derived from each LayerView's (CenterX, CenterZ, ExtentX, ExtentZ)
  // emitted by the Go layout engine. In Stacked mode rectangles
  // overlap (clusters share the same XZ footprint); in Extended mode
  // they're spread across the spiral slot grid.
  //
  // Hover any rectangle (or its legend chip) to highlight it and see
  // the layer's full tag list, node count, and color. The highlight
  // is purely cosmetic on the minimap — it doesn't drive the main
  // scene's Highlight.js (which is for node neighbour emphasis).

  import { createEventDispatcher } from 'svelte'
  import { currentLayout, cameraFrame } from '../stores/explorer.js'
  import { getTerritoryPalette } from './explorer/Theme.js'

  const dispatch = createEventDispatcher()

  // Mini-map sizes (CSS px).
  const MAP_SIZE = 150
  const LEGEND_MAX_ROWS = 8

  // World-units padding around the cluster footprint when drawing its
  // rectangle. Matches LayerPlanes.js's PLANE_PADDING so the minimap
  // rectangles line up visually with the actual rendered planes.
  const PLANE_PADDING = 2.4
  const VIEW_PADDING = 3

  let hoveredCtxId = null
  let svgEl
  let dragging = false

  // Click or drag on the minimap to pan the main camera. We use the
  // SVG's getScreenCTM to convert client coords → SVG coords; because
  // our viewBox is in world (x, z) units, the result IS world coords
  // (x maps to world X, y maps to world Z).
  function clientToWorld(clientX, clientY) {
    if (!svgEl) return null
    const pt = svgEl.createSVGPoint()
    pt.x = clientX
    pt.y = clientY
    const m = svgEl.getScreenCTM()
    if (!m) return null
    const wp = pt.matrixTransform(m.inverse())
    return { x: wp.x, z: wp.y }
  }

  function onPointerDown(e) {
    if (e.button !== 0) return
    dragging = true
    try { e.target.setPointerCapture?.(e.pointerId) } catch (_) {}
    const wp = clientToWorld(e.clientX, e.clientY)
    if (wp) dispatch('pan', wp)
    e.preventDefault()
  }
  function onPointerMove(e) {
    if (!dragging) return
    const wp = clientToWorld(e.clientX, e.clientY)
    if (wp) dispatch('pan', wp)
  }
  function onPointerUp(e) {
    if (!dragging) return
    dragging = false
    try { e.target.releasePointerCapture?.(e.pointerId) } catch (_) {}
  }

  $: layers = $currentLayout?.layers ?? []
  $: bounds = $currentLayout?.bounds ?? null

  $: worldBox = bounds ? {
    minX: bounds.minX - VIEW_PADDING,
    maxX: bounds.maxX + VIEW_PADDING,
    minZ: bounds.minZ - VIEW_PADDING,
    maxZ: bounds.maxZ + VIEW_PADDING,
  } : null

  $: viewport = ($cameraFrame && worldBox) ? viewportFor($cameraFrame) : null

  $: hoveredLayer = layers.find(l => l.contextId === hoveredCtxId) ?? null

  function viewportFor(cam) {
    const visibleW = cam.frustumWidth / cam.zoom
    // Camera's iso tilt compresses ground depth on screen — back-
    // project the on-screen height through sin(35°) to get the true
    // ground span the viewer sees.
    const visibleD = cam.frustumHeight / (cam.zoom * Math.sin((35 * Math.PI) / 180))
    return {
      x: cam.x - visibleW / 2,
      z: cam.z - visibleD / 2,
      w: visibleW,
      d: visibleD,
    }
  }

  function rectFor(layer) {
    // Match the visible plane size in LayerPlanes.js.
    const w = Math.max(8, layer.extentX * 2 + PLANE_PADDING * 2)
    const d = Math.max(8, layer.extentZ * 2 + PLANE_PADDING * 2)
    return {
      x: layer.centerX - w / 2,
      z: layer.centerZ - d / 2,
      w,
      d,
    }
  }

  function colorFor(slot) {
    if (slot === undefined || slot === null || slot < 0) return 'var(--explorer-territory-none)'
    const palette = getTerritoryPalette()
    const c = palette[slot % palette.length]
    return '#' + c.toString(16).padStart(6, '0')
  }
</script>

{#if worldBox && layers.length > 0}
  <div class="minimap" role="region" aria-label="Graph mini-map">
    {#if hoveredLayer}
      <div class="hover-bar">
        <span class="hover-swatch" style="background: {colorFor(hoveredLayer.colorSlot)}"></span>
        <span class="hover-name">{hoveredLayer.label || 'Untagged'}</span>
        <span class="hover-count">{hoveredLayer.nodeCount}</span>
      </div>
      {#if hoveredLayer.tags && hoveredLayer.tags.length > 1}
        <div class="hover-tags">{hoveredLayer.tags.join(', ')}</div>
      {/if}
    {/if}
    <svg
      class="map"
      class:dragging
      bind:this={svgEl}
      viewBox="{worldBox.minX} {worldBox.minZ} {worldBox.maxX - worldBox.minX} {worldBox.maxZ - worldBox.minZ}"
      preserveAspectRatio="xMidYMid meet"
      width={MAP_SIZE}
      height={MAP_SIZE}
      on:pointerdown={onPointerDown}
      on:pointermove={onPointerMove}
      on:pointerup={onPointerUp}
      on:pointercancel={onPointerUp}>
      <!-- Backdrop. Colours set via `style` (not bare fill/stroke attributes)
           so the browser resolves the var() the same way it does for any
           other CSS property — presentation-attribute var() support is less
           consistent across WebKit versions. -->
      <rect
        x={worldBox.minX}
        y={worldBox.minZ}
        width={worldBox.maxX - worldBox.minX}
        height={worldBox.maxZ - worldBox.minZ}
        style="fill: var(--explorer-minimap-canvas-bg); stroke: var(--border-strong);"
        stroke-width="0.3"
        vector-effect="non-scaling-stroke" />
      {#each layers as l (l.contextId)}
        {@const r = rectFor(l)}
        {@const color = colorFor(l.colorSlot)}
        {@const isActive = hoveredCtxId === l.contextId}
        <rect
          x={r.x} y={r.z}
          width={r.w} height={r.d}
          style="fill: {color}; stroke: {color};"
          fill-opacity={isActive ? 0.75 : 0.4}
          stroke-width={isActive ? 1.6 : 0.8}
          stroke-opacity={isActive ? 1 : 0.7}
          vector-effect="non-scaling-stroke"
          on:mouseenter={() => (hoveredCtxId = l.contextId)}
          on:mouseleave={() => (hoveredCtxId = null)} />
      {/each}
      {#if viewport}
        <rect
          x={viewport.x} y={viewport.z}
          width={viewport.w} height={viewport.d}
          style="fill: var(--explorer-viewport-fill); stroke: var(--text);"
          stroke-width="1"
          vector-effect="non-scaling-stroke"
          pointer-events="none" />
      {/if}
    </svg>
    <ul class="legend">
      {#each layers.slice(0, LEGEND_MAX_ROWS) as l (l.contextId)}
        <li
          class="row"
          class:active={hoveredCtxId === l.contextId}
          on:mouseenter={() => (hoveredCtxId = l.contextId)}
          on:mouseleave={() => (hoveredCtxId = null)}>
          <span class="swatch" style="background: {colorFor(l.colorSlot)}"></span>
          <span class="row-name">{l.label || 'Untagged'}</span>
          <span class="row-count">{l.nodeCount}</span>
        </li>
      {/each}
      {#if layers.length > LEGEND_MAX_ROWS}
        <li class="row more">+{layers.length - LEGEND_MAX_ROWS} more</li>
      {/if}
    </ul>
  </div>
{/if}

<style>
  .minimap {
    position: absolute;
    left: 12px;
    bottom: 12px;
    width: 200px;
    background: var(--surface-raised);
    border: 1px solid var(--border-strong);
    border-radius: 6px;
    padding: 6px;
    pointer-events: auto;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
    color: var(--text);
    font-size: 11px;
    z-index: 4;
  }
  .hover-bar {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 2px 4px 4px;
    border-bottom: 1px solid var(--explorer-divider);
    margin-bottom: 4px;
  }
  .hover-swatch {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 2px;
    flex: 0 0 10px;
  }
  .hover-name {
    flex: 1 1 auto;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .hover-count {
    color: var(--explorer-text-muted);
    font-variant-numeric: tabular-nums;
  }
  .hover-tags {
    font-size: 10px;
    color: var(--explorer-hint-text);
    padding: 0 4px 4px;
    line-height: 1.35;
  }
  .map {
    display: block;
    margin: 0 auto;
    cursor: grab;
    touch-action: none;
  }
  .map.dragging {
    cursor: grabbing;
  }
  .legend {
    list-style: none;
    margin: 6px 0 0;
    padding: 0;
    max-height: 140px;
    overflow-y: auto;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 2px 4px;
    border-radius: 3px;
    cursor: default;
    user-select: none;
  }
  .row:hover,
  .row.active {
    background: var(--explorer-row-hover);
  }
  .swatch {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 2px;
    flex: 0 0 10px;
  }
  .row-name {
    flex: 1 1 auto;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row-count {
    color: var(--explorer-text-muted);
    font-variant-numeric: tabular-nums;
    font-size: 10px;
  }
  .row.more {
    color: var(--explorer-text-muted);
    font-style: italic;
    cursor: default;
  }
</style>
