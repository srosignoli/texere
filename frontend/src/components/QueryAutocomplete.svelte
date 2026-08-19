<script>
  // Shared suggestion dropdown for the graph query bars. Presentational only:
  // the host bar owns the query state, computes `items` (via the
  // query-grammar module), tracks `activeIndex`, and handles keyboard
  // navigation. This component renders the list, groups items by section,
  // highlights the active row, and emits `select` on click / `close` on
  // outside-click.
  //
  // Items are { label, detail, section, apply }. `activeIndex` is a flat
  // index into `items` (section grouping is presentational only).

  import { createEventDispatcher, tick } from 'svelte'

  export let items = []
  export let activeIndex = 0
  /** Anchor open/closed from the host; the host hides this when not needed. */
  export let open = false

  const dispatch = createEventDispatcher()

  let listEl

  // Build display groups (preserve first-seen section order, items already
  // arrive section-clustered from getSuggestions).
  $: groups = (() => {
    const out = []
    let cur = null
    items.forEach((it, i) => {
      if (!cur || cur.section !== it.section) {
        cur = { section: it.section, rows: [] }
        out.push(cur)
      }
      cur.rows.push({ it, i })
    })
    return out
  })()

  // Keep the active row scrolled into view when the host moves activeIndex.
  $: if (open && listEl && activeIndex >= 0) scrollActiveIntoView(activeIndex)
  async function scrollActiveIntoView(idx) {
    await tick()
    const el = listEl?.querySelector(`[data-idx="${idx}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  }
</script>

{#if open && items.length > 0}
  <div class="qac" bind:this={listEl} role="listbox" aria-label="Query suggestions">
    {#each groups as g}
      <div class="qac-section">{g.section}</div>
      {#each g.rows as { it, i } (it.section + ':' + it.label)}
        <button
          type="button"
          class="qac-row"
          class:active={i === activeIndex}
          data-idx={i}
          role="option"
          aria-selected={i === activeIndex}
          on:mousedown|preventDefault={() => dispatch('select', it)}
          on:mouseenter={() => dispatch('hover', i)}
        >
          <span class="qac-label">{it.label}</span>
          {#if it.detail}<span class="qac-detail">{it.detail}</span>{/if}
        </button>
      {/each}
    {/each}
  </div>
{/if}

<style>
  .qac {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    z-index: 20;
    max-height: 320px;
    overflow-y: auto;
    background: var(--surface-overlay);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.55);
    padding: 4px;
  }

  .qac-section {
    padding: 6px 8px 2px;
    font-size: 0.68rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--accent-dim);
  }

  .qac-row {
    display: flex;
    align-items: baseline;
    gap: 8px;
    width: 100%;
    text-align: left;
    padding: 5px 8px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--text);
    font: inherit;
    cursor: pointer;
  }

  .qac-row.active,
  .qac-row:hover {
    background: var(--accent);
  }

  .qac-label {
    flex: 0 0 auto;
    font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    color: var(--qac-label-text);
    white-space: nowrap;
  }

  .qac-detail {
    flex: 1 1 auto;
    min-width: 0;
    font-size: 0.8rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
