<script>
  import { onMount } from 'svelte'
  import { arrowPaletteOpen, workbenchStore, editorAPI } from '../stores/workbench.js'
  import { UpdateRecentArrows, GetRecentArrows } from '../../wailsjs/go/main/App'

  let arrowFilter = ''
  let arrowHighlightIdx = 0
  let inputEl = null
  let panelEl = null
  let recentCodes = []

  $: arrowDefs = $workbenchStore.arrowDefs

  onMount(async () => {
    try {
      const saved = await GetRecentArrows()
      if (saved?.length) recentCodes = saved
    } catch (e) {
      console.warn('GetRecentArrows failed:', e)
    }
  })

  // Autofocus + reset when palette opens
  $: if ($arrowPaletteOpen && inputEl) {
    arrowFilter = ''
    arrowHighlightIdx = 0
    setTimeout(() => inputEl?.focus(), 0)
  }

  // Reset highlight when filter changes
  $: arrowFilter, (arrowHighlightIdx = 0)

  // Flat filtered list — only used while arrowFilter is non-empty
  $: filteredArrows = arrowFilter.trim()
    ? arrowDefs.filter((d) => {
        const q = arrowFilter.toLowerCase()
        return d.Code.toLowerCase().includes(q) || d.LongName.toLowerCase().includes(q)
      })
    : []

  // Grouped view (used when filter is empty)
  $: groupedArrows = (() => {
    const groups = new Map()
    const recentDefs = recentCodes
      .map((c) => arrowDefs.find((d) => d.Code === c))
      .filter(Boolean)
    if (recentDefs.length) groups.set('Recent', recentDefs)
    for (const def of arrowDefs) {
      const key = def.MetaType || 'Other'
      if (!groups.has(key)) groups.set(key, [])
      groups.get(key).push(def)
    }
    return groups
  })()

  function close() {
    arrowPaletteOpen.set(false)
    arrowFilter = ''
    arrowHighlightIdx = 0
  }

  async function selectArrow(code) {
    close()
    if (!$editorAPI) return
    workbenchStore.update((s) => ({ ...s, activeTab: 'note' }))
    $editorAPI.insertAtCursor(`(${code}) `)

    recentCodes = [code, ...recentCodes.filter((c) => c !== code)].slice(0, 10)
    try {
      await UpdateRecentArrows(recentCodes)
    } catch (e) {
      console.warn('UpdateRecentArrows failed:', e)
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') { close(); return }
    const items = arrowFilter.trim() ? filteredArrows : []
    if (!items.length) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      arrowHighlightIdx = Math.min(arrowHighlightIdx + 1, items.length - 1)
      setTimeout(() => {
        panelEl?.querySelector(`[data-idx="${arrowHighlightIdx}"]`)
          ?.scrollIntoView({ block: 'nearest' })
      }, 0)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      arrowHighlightIdx = Math.max(arrowHighlightIdx - 1, 0)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      selectArrow(items[arrowHighlightIdx].Code)
    }
  }
</script>

{#if $arrowPaletteOpen}
  <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <div class="palette-backdrop" on:click={close} role="presentation" />

  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div
    class="palette-panel"
    bind:this={panelEl}
    on:keydown={handleKeydown}
    role="dialog"
    aria-label="Insert arrow"
  >
    <div class="palette-header">
      <input
        bind:this={inputEl}
        bind:value={arrowFilter}
        class="palette-filter"
        type="text"
        placeholder="Filter arrows…"
        autocomplete="off"
        spellcheck="false"
      />
      <button class="palette-close" on:click={close} title="Close (Esc)">✕</button>
    </div>

    <div class="palette-results">
      {#if arrowFilter.trim()}
        {#if filteredArrows.length === 0}
          <div class="palette-empty">No matches</div>
        {:else}
          {#each filteredArrows as def, i}
            <button
              class="palette-item"
              class:highlighted={i === arrowHighlightIdx}
              data-idx={i}
              on:click={() => selectArrow(def.Code)}
              on:mouseenter={() => (arrowHighlightIdx = i)}
            >
              <span class="arrow-code">({def.Code})</span>
              <span class="arrow-name">{def.LongName}</span>
            </button>
          {/each}
        {/if}
      {:else}
        {#each [...groupedArrows.entries()] as [group, defs]}
          <div class="palette-group-label">{group}</div>
          {#each defs as def}
            <button class="palette-item" on:click={() => selectArrow(def.Code)}>
              <span class="arrow-code">({def.Code})</span>
              <span class="arrow-name">{def.LongName}</span>
            </button>
          {/each}
        {/each}
        {#if arrowDefs.length === 0}
          <div class="palette-empty">No arrows loaded</div>
        {/if}
      {/if}
    </div>
  </div>
{/if}

<style>
  .palette-backdrop {
    position: fixed;
    inset: 0;
    z-index: 299;
    background: rgba(0, 0, 0, 0.35);
  }

  .palette-panel {
    position: fixed;
    top: 264px; /* below band-top (220px) + band-middle (44px) */
    left: 8px;
    z-index: 300;
    background: #1a2535;
    border: 1px solid #2a3a4a;
    border-radius: 6px;
    width: 280px;
    max-height: 420px;
    display: flex;
    flex-direction: column;
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.55);
  }

  .palette-header {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 8px;
    border-bottom: 1px solid #2a3a4a;
    flex-shrink: 0;
  }

  .palette-filter {
    flex: 1;
    background: #0f1a27;
    border: 1px solid #2a3a4a;
    border-radius: 4px;
    color: #e8eaf0;
    font-size: 0.82rem;
    font-family: inherit;
    padding: 4px 8px;
    outline: none;
  }

  .palette-filter::placeholder {
    color: #3a5a7a;
  }

  .palette-close {
    background: none;
    border: none;
    color: #5a7a9a;
    cursor: pointer;
    font-size: 0.9rem;
    padding: 2px 6px;
    flex-shrink: 0;
  }

  .palette-close:hover {
    color: #e8eaf0;
  }

  .palette-results {
    overflow-y: auto;
    flex: 1;
    padding: 4px 0;
  }

  .palette-group-label {
    padding: 4px 10px 2px;
    font-size: 0.68rem;
    font-weight: 600;
    color: #5a7a9a;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .palette-item {
    display: flex;
    align-items: baseline;
    gap: 8px;
    width: 100%;
    padding: 5px 10px;
    background: none;
    border: none;
    color: #b0bec5;
    font-size: 0.82rem;
    text-align: left;
    cursor: pointer;
  }

  .palette-item:hover,
  .palette-item.highlighted {
    background: #1e2f40;
    color: #e8eaf0;
  }

  .arrow-code {
    color: #67e8f9;
    font-family: monospace;
    flex-shrink: 0;
  }

  .arrow-name {
    color: #8899aa;
    font-size: 0.78rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .palette-empty {
    padding: 8px 10px;
    color: #4a5a6a;
    font-size: 0.8rem;
  }
</style>
