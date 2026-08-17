<script>
  // Stats panel for the Layered Knowledge Graph. Mounted top-right next to
  // the close button. Glanceable cells: LAYERS / MATCHES / TOTAL NODES /
  // LIMIT / RADIUS / SYSTEM. LIMIT and RADIUS are editable — changing either
  // rewrites the query (\limit / \radius) and re-runs automatically (the
  // parent owns the query + rerun).

  import { createEventDispatcher } from 'svelte'
  import { currentLayout } from '../stores/explorer.js'

  /** Effective result limit currently applied to the query (how MANY matches). */
  export let limit = 100
  /** Upper bound the parent clamps the limit to (render ceiling). */
  export let maxLimit = 500
  /** Effective orbit radius (how FAR each match's neighborhood expands). */
  export let radius = 2
  /** Upper bound the parent clamps the radius to. */
  export let maxRadius = 3

  const dispatch = createEventDispatcher()

  $: layerCount = ($currentLayout?.layers ?? []).length
  $: nodeCount = ($currentLayout?.nodes ?? []).length
  // Direct query matches (seeds). The remaining nodes are neighbors pulled in
  // around each match, which is why "Limit N" can render more than N nodes.
  $: matchCount = $currentLayout?.diagnostics?.matchCount ?? 0

  // Two editable cells share one tiny editor model. Each tracks a local draft
  // that re-syncs from the parent prop unless an edit is in progress, so
  // typing \limit/\radius directly in the query bar updates the cells too.
  let limitDraft = limit
  let limitEditing = false
  $: if (!limitEditing) limitDraft = limit

  let radiusDraft = radius
  let radiusEditing = false
  $: if (!radiusEditing) radiusDraft = radius

  function commitLimit() {
    limitEditing = false
    const n = parseInt(limitDraft, 10)
    if (Number.isFinite(n) && n !== limit) dispatch('applyLimit', n)
    else limitDraft = limit
  }
  function commitRadius() {
    radiusEditing = false
    const n = parseInt(radiusDraft, 10)
    if (Number.isFinite(n) && n !== radius) dispatch('applyRadius', n)
    else radiusDraft = radius
  }

  // Enter commits (via blur); Escape reverts without re-running and must not
  // bubble up to the explorer's close-on-Escape handler.
  function fieldKeydown(e, revert) {
    if (e.key === 'Enter') {
      e.preventDefault()
      e.target.blur()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      revert()
      e.target.blur()
    }
  }
</script>

{#if $currentLayout}
  <div class="stats" role="status" aria-label="Graph statistics">
    <div class="cell">
      <div class="label">Layers</div>
      <div class="value">{layerCount}</div>
    </div>
    <div class="cell">
      <div class="label" title="Nodes that directly matched your query (seeds)">Matches</div>
      <div class="value">{matchCount}</div>
    </div>
    <div class="cell">
      <div class="label" title="Matches plus the neighbors pulled in around each one">Total nodes</div>
      <div class="value">{nodeCount}</div>
    </div>
    <div class="cell cell-edit">
      <div class="label" title="How many matches to anchor the graph on">Limit</div>
      <input
        class="value num-input"
        type="number"
        min="1"
        max={maxLimit}
        bind:value={limitDraft}
        on:focus={() => limitEditing = true}
        on:blur={commitLimit}
        on:keydown={(e) => fieldKeydown(e, () => { limitDraft = limit; limitEditing = false })}
        title="Max results — edit to re-run the query"
        aria-label="Result limit" />
    </div>
    <div class="cell cell-edit">
      <div class="label" title="How far each match expands (1–{maxRadius} hops)">Radius</div>
      <input
        class="value num-input"
        type="number"
        min="1"
        max={maxRadius}
        bind:value={radiusDraft}
        on:focus={() => radiusEditing = true}
        on:blur={commitRadius}
        on:keydown={(e) => fieldKeydown(e, () => { radiusDraft = radius; radiusEditing = false })}
        title="Neighborhood depth — edit to re-run the query"
        aria-label="Orbit radius" />
    </div>
    <div class="cell">
      <div class="label">System</div>
      <div class="value">ISO</div>
    </div>
  </div>
{/if}

<style>
  .stats {
    position: absolute;
    top: 12px;
    right: 92px; /* leave room for legend (?) at 52 and close (✕) at 12 */
    display: flex;
    gap: 18px;
    background: rgba(14, 20, 24, 0.85);
    border: 1px solid #334455;
    border-radius: 6px;
    padding: 6px 12px;
    /* Panel itself is glanceable, but the limit input must be clickable, so
       enable pointer events on the panel and let static cells stay inert. */
    pointer-events: auto;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
    font-family: ui-monospace, SFMono-Regular, monospace;
  }
  .cell {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    line-height: 1;
    pointer-events: none; /* re-enabled on the editable cell below */
  }
  .cell-edit {
    pointer-events: auto;
  }
  .label {
    font-size: 9px;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--explorer-text-muted);
    margin-bottom: 3px;
  }
  .value {
    font-size: 14px;
    font-weight: 600;
    color: #e8eaf0;
  }
  .num-input {
    width: 4ch;
    background: transparent;
    border: none;
    border-bottom: 1px dashed #45607a;
    padding: 0 0 1px;
    font: inherit;
    font-size: 14px;
    font-weight: 600;
    color: #e8eaf0;
    outline: none;
    cursor: text;
  }
  .num-input:hover {
    border-bottom-color: #6b8db0;
  }
  .num-input:focus {
    border-bottom-color: #4a7bd0;
  }
  /* Hide the number spinner for a cleaner look. */
  .num-input::-webkit-inner-spin-button,
  .num-input::-webkit-outer-spin-button {
    -webkit-appearance: none;
    margin: 0;
  }
  .num-input {
    -moz-appearance: textfield;
    appearance: textfield;
  }
</style>
