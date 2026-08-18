<script>
  // Right-side collapsible control panel for the Layered Knowledge
  // Graph explorer. Three sections:
  //   • Layers — checkbox per context (incl. Untagged). Hidden layers
  //     dim their plane, suppress their nodes, and fade edges incident
  //     to them.
  //   • Arrows — checkbox per meta-type (Contains / Leads to /
  //     Properties / Similar to / Unclassified).
  //   • Display — Layer Spacing slider, Plane Opacity slider, Reset
  //     View button.
  //
  // The sidebar is collapsed-by-default to keep the canvas
  // uncluttered; a gear-icon button toggles it. State lives in the
  // shared stores so reanchor / new-query paths can clear it.

  import { createEventDispatcher } from 'svelte'
  import {
    currentLayout,
    layerSpacing,
    planeOpacity,
    hiddenLayers,
    hiddenMetas,
    layoutMode,
  } from '../stores/explorer.js'
  import { getEdgeColor } from './explorer/Theme.js'

  export let open = false
  const dispatch = createEventDispatcher()

  // Edge meta-types ordered to mirror the legend overlay.
  const META_TYPES = [
    { id: 'contains', label: 'Contains', color: hex('contains') },
    { id: 'leadsto', label: 'Leads to', color: hex('leadsto') },
    { id: 'properties', label: 'Properties', color: hex('properties') },
    { id: 'similarto', label: 'Similar to', color: hex('similarto') },
    { id: 'unclassified', label: 'Unclassified', color: hex('unclassified') },
  ]

  $: layers = $currentLayout?.layers ?? []

  // Tri-state for the master "All" toggle.
  $: visibleCount = layers.filter(l => !$hiddenLayers.has(l.contextId)).length
  $: allVisible = layers.length > 0 && visibleCount === layers.length
  $: somePartial = layers.length > 0 && visibleCount > 0 && visibleCount < layers.length

  // Bind for the master checkbox so we can set its `indeterminate`
  // property reactively (DOM-only, not a Svelte attribute).
  let allCheckbox
  $: if (allCheckbox) allCheckbox.indeterminate = somePartial

  // The edge palette is theme-INVARIANT (a shared :root block with no
  // per-theme override), so resolving once at module scope is correct — the
  // swatches cannot drift from what Three.js draws on a theme switch.
  function hex(meta) {
    return '#' + getEdgeColor(meta).toString(16).padStart(6, '0')
  }

  function toggleAllLayers() {
    // If everything is visible → hide all. Otherwise → show all.
    // Matches a tri-state checkbox's natural toggle behaviour.
    if (allVisible) {
      hiddenLayers.set(new Set(layers.map(l => l.contextId)))
    } else {
      hiddenLayers.set(new Set())
    }
  }

  function toggleLayer(contextId) {
    hiddenLayers.update((set) => {
      const next = new Set(set)
      if (next.has(contextId)) next.delete(contextId)
      else next.add(contextId)
      return next
    })
  }

  function toggleMeta(metaId) {
    hiddenMetas.update((set) => {
      const next = new Set(set)
      if (next.has(metaId)) next.delete(metaId)
      else next.add(metaId)
      return next
    })
  }

  function resetView() {
    dispatch('resetView')
  }

  function hideAllExcept(contextId) {
    const all = layers.map(l => l.contextId).filter(c => c !== contextId)
    hiddenLayers.set(new Set(all))
  }
</script>

<!-- Floating gear button — always visible. Clicking it toggles the
     sidebar panel. -->
<button
  class="sidebar-toggle"
  on:click={() => (open = !open)}
  title="Layers & filters"
  aria-label="Toggle explorer controls"
  aria-expanded={open}>
  ☰
</button>

{#if open}
  <aside class="sidebar" aria-label="Explorer controls">
    <div class="sidebar-header">
      <div class="sidebar-title">Controls</div>
      <button
        class="close-btn"
        on:click={() => (open = false)}
        title="Close"
        aria-label="Close controls">✕</button>
    </div>

    <!-- Layers -->
    <section class="section">
      <div class="section-head">
        <span class="section-title">Layers</span>
        <span class="section-badge">{layers.length}</span>
      </div>
      <ul class="list">
        {#if layers.length > 1}
          <li class="row row-all">
            <label class="check">
              <input
                bind:this={allCheckbox}
                type="checkbox"
                checked={allVisible}
                on:change={toggleAllLayers} />
              <span class="row-label all-label">All</span>
              <span class="row-count">{visibleCount}/{layers.length}</span>
            </label>
          </li>
        {/if}
        {#each layers as l (l.contextId)}
          <li class="row">
            <label class="check">
              <input
                type="checkbox"
                checked={!$hiddenLayers.has(l.contextId)}
                on:change={() => toggleLayer(l.contextId)} />
              <span class="row-label">{l.label}</span>
              <span class="row-count">{l.nodeCount}</span>
            </label>
            <button
              class="solo-btn"
              title="Solo this layer"
              aria-label="Solo {l.label}"
              on:click={() => hideAllExcept(l.contextId)}>solo</button>
          </li>
        {/each}
      </ul>
    </section>

    <!-- Arrows -->
    <section class="section">
      <div class="section-head">
        <span class="section-title">Arrows</span>
      </div>
      <ul class="list">
        {#each META_TYPES as m (m.id)}
          <li class="row">
            <label class="check">
              <input
                type="checkbox"
                checked={!$hiddenMetas.has(m.id)}
                on:change={() => toggleMeta(m.id)} />
              <span class="swatch" style="background: {m.color}"></span>
              <span class="row-label">{m.label}</span>
            </label>
          </li>
        {/each}
      </ul>
    </section>

    <!-- Display -->
    <section class="section">
      <div class="section-head">
        <span class="section-title">Display</span>
      </div>
      <div class="mode-row" role="radiogroup" aria-label="Layout mode">
        <span class="mode-label">Layout</span>
        <div class="mode-buttons">
          <label class="mode-btn" class:active={$layoutMode === 'stacked'}>
            <input
              type="radio"
              name="layout-mode"
              value="stacked"
              bind:group={$layoutMode} />
            <span>Stacked</span>
          </label>
          <label class="mode-btn" class:active={$layoutMode === 'extended'}>
            <input
              type="radio"
              name="layout-mode"
              value="extended"
              bind:group={$layoutMode} />
            <span>Extended</span>
          </label>
        </div>
      </div>
      <div class="slider-row">
        <label for="layer-spacing">Layer spacing</label>
        <input
          id="layer-spacing"
          type="range"
          min="3"
          max="14"
          step="0.5"
          bind:value={$layerSpacing} />
        <span class="slider-value">{$layerSpacing.toFixed(1)}</span>
      </div>
      <div class="slider-row">
        <label for="plane-opacity">Plane opacity</label>
        <input
          id="plane-opacity"
          type="range"
          min="0.05"
          max="0.5"
          step="0.01"
          bind:value={$planeOpacity} />
        <span class="slider-value">{$planeOpacity.toFixed(2)}</span>
      </div>
      <button class="reset-btn" on:click={resetView}>Reset view</button>
    </section>
  </aside>
{/if}

<style>
  .sidebar-toggle {
    position: absolute;
    top: 12px;
    left: 12px;
    width: 32px;
    height: 32px;
    background: var(--explorer-scrim);
    color: var(--text);
    border: 1px solid var(--border-strong);
    border-radius: 16px;
    font-size: 14px;
    cursor: pointer;
    display: grid;
    place-items: center;
    z-index: 6;
  }

  .sidebar-toggle:hover {
    background: var(--explorer-scrim-hover);
  }

  .sidebar {
    position: absolute;
    top: 12px;
    left: 52px;
    width: 280px;
    max-height: calc(100vh - 24px);
    overflow-y: auto;
    background: var(--surface-raised);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    padding: 12px 14px;
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.4);
    z-index: 5;
    color: var(--text);
    font-size: 12px;
  }

  .sidebar-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 10px;
  }

  .sidebar-title {
    font-size: 13px;
    font-weight: 600;
  }

  .close-btn {
    width: 22px;
    height: 22px;
    background: transparent;
    color: var(--explorer-hint-text);
    border: 1px solid transparent;
    border-radius: 11px;
    font-size: 12px;
    cursor: pointer;
    display: grid;
    place-items: center;
  }

  .close-btn:hover {
    border-color: var(--border-strong);
    color: var(--text);
  }

  .section {
    margin-bottom: 14px;
  }

  .section:last-child {
    margin-bottom: 0;
  }

  .section-head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    margin-bottom: 6px;
  }

  .section-title {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--explorer-hint-text);
  }

  .section-badge {
    font-size: 11px;
    color: var(--explorer-text-muted);
  }

  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 6px;
  }

  .check {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1 1 auto;
    cursor: pointer;
    user-select: none;
  }

  .check input[type='checkbox'] {
    margin: 0;
    accent-color: var(--explorer-node-default);
  }

  .row-all {
    border-bottom: 1px solid var(--explorer-divider);
    padding-bottom: 4px;
    margin-bottom: 2px;
  }
  .all-label {
    font-weight: 600;
    text-transform: uppercase;
    font-size: 10px;
    letter-spacing: 0.06em;
    color: var(--explorer-hint-text);
  }

  .row-label {
    flex: 1 1 auto;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .row-count {
    color: var(--explorer-text-muted);
    font-variant-numeric: tabular-nums;
    font-size: 11px;
  }

  .swatch {
    width: 12px;
    height: 3px;
    border-radius: 2px;
    flex: 0 0 12px;
  }

  .solo-btn {
    background: transparent;
    color: var(--explorer-text-muted);
    border: 1px solid transparent;
    font-size: 10px;
    cursor: pointer;
    padding: 2px 6px;
    border-radius: 3px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .solo-btn:hover {
    border-color: var(--border-strong);
    color: var(--text);
  }

  .text-btn {
    background: transparent;
    color: var(--explorer-node-default);
    border: none;
    font-size: 11px;
    cursor: pointer;
    padding: 4px 0;
    margin-top: 2px;
  }

  .text-btn:hover {
    text-decoration: underline;
  }

  .slider-row {
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: center;
    column-gap: 8px;
    row-gap: 2px;
    margin-bottom: 8px;
  }

  .slider-row label {
    grid-column: 1;
    grid-row: 1;
    font-size: 11px;
    color: var(--explorer-hint-text);
  }

  .slider-row .slider-value {
    grid-column: 2;
    grid-row: 1;
    font-variant-numeric: tabular-nums;
    font-size: 11px;
    color: var(--text);
  }

  .slider-row input[type='range'] {
    grid-column: 1 / span 2;
    grid-row: 2;
    width: 100%;
    accent-color: var(--explorer-node-default);
  }

  .reset-btn {
    margin-top: 6px;
    width: 100%;
    padding: 6px 8px;
    background: var(--explorer-control);
    color: var(--text);
    border: 1px solid var(--border-strong);
    border-radius: 4px;
    cursor: pointer;
    font-size: 12px;
  }

  .reset-btn:hover {
    background: var(--explorer-control-active);
  }

  /* Layout-mode radio group (Stacked / Extended). Rendered as a
     segmented pill so the choice reads as a binary toggle rather
     than two stray checkboxes. */
  .mode-row {
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: center;
    column-gap: 8px;
    margin-bottom: 10px;
  }
  .mode-label {
    font-size: 11px;
    color: var(--explorer-hint-text);
  }
  .mode-buttons {
    display: inline-flex;
    background: var(--explorer-segment-bg);
    border: 1px solid var(--border-strong);
    border-radius: 4px;
    overflow: hidden;
  }
  .mode-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 4px 10px;
    font-size: 11px;
    color: var(--explorer-hint-text);
    cursor: pointer;
    user-select: none;
    transition: background 0.12s, color 0.12s;
  }
  .mode-btn input[type='radio'] {
    /* Hide the native control; the label itself is the click target. */
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0 0 0 0);
    border: 0;
  }
  .mode-btn.active {
    background: var(--explorer-control-active);
    color: var(--text);
  }
  .mode-btn:hover:not(.active) {
    background: var(--explorer-segment-hover);
    color: var(--explorer-detail-text);
  }
</style>
