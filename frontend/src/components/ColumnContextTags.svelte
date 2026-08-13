<script>
  import { workbenchStore, contextTags } from '../stores/workbench.js'

  let scrollEl = null
  let _tagScrollTimer = null

  // Default click is single-select: the clicked chip becomes the only
  // selection, releasing any previously active chips. Ctrl/Cmd+click is
  // additive — toggles this chip in/out of the existing set. Re-clicking the
  // sole selected chip (without modifier) clears the filter, mirroring the
  // toggle-off feel users expect.
  function handleChipClick(event, name) {
    const additive = event.ctrlKey || event.metaKey
    workbenchStore.update((s) => {
      const next = new Set(additive ? s.activeContextFilter : [])
      if (additive) {
        if (next.has(name)) next.delete(name)
        else next.add(name)
      } else if (s.activeContextFilter.size === 1 && s.activeContextFilter.has(name)) {
        // Already the sole selection — clear the filter.
        next.clear()
      } else {
        next.clear()
        next.add(name)
      }
      return { ...s, activeContextFilter: next }
    })
  }

  // Context tags active at the current editor cursor position.
  // Uses the same "largest node.Line ≤ cursor" algorithm as ColumnNodes.svelte.
  $: cursorContextTags = (() => {
    const line = $workbenchStore.editorCursorLine
    if (!line) return new Set()
    const ch = $workbenchStore.parsedFile?.Chapters?.find(
      (c) => c.Title === $workbenchStore.selectedChapterKey?.chapterTitle
    )
    if (!ch) return new Set()
    let match = null
    for (const n of (ch.Nodes ?? []))
      if (n.Line <= line) match = n
    return new Set(match?.EffectiveCtx ?? [])
  })()

  // Scroll the first filter-active chip into view when activeContextFilter changes.
  $: {
    const filter = $workbenchStore.activeContextFilter
    if (filter.size > 0 && scrollEl) {
      const first = [...filter][0]
      setTimeout(() => {
        scrollEl.querySelector(`[data-tag="${CSS.escape(first)}"]`)
          ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
      }, 0)
    }
  }

  // Scroll the first cursor-active chip into view (debounced, block:'nearest'
  // avoids scroll when the chip is already visible).
  $: if (cursorContextTags.size > 0 && scrollEl) {
    clearTimeout(_tagScrollTimer)
    _tagScrollTimer = setTimeout(() => {
      const first = [...cursorContextTags][0]
      const el = scrollEl.querySelector(`[data-tag="${CSS.escape(first)}"]`)
      el?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }, 120)
  }
</script>

<div class="col-context-tags">
  <div class="col-header">
    <span class="col-title">Contexts / Sequences</span>
  </div>
  <div class="col-scroll" bind:this={scrollEl}>
    {#if $contextTags.length === 0}
      <div class="empty-hint">No tags</div>
    {:else}
      <div class="chips">
        {#each $contextTags as tag (tag.name)}
          <button
            class="chip"
            data-tag={tag.name}
            class:active={tag.isActive}
            class:sequence={tag.isSequence}
            class:cursor-at={cursorContextTags.has(tag.name)}
            on:click={(e) => handleChipClick(e, tag.name)}
            title={tag.name}
          >
            {#if tag.isSequence}
              <span class="seq-icon" aria-hidden="true">⛓</span>
            {/if}
            {tag.name}
          </button>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .col-context-tags {
    /* width is owned by the parent slot in TopBand (uiLayout.contextTagsWidth);
       the right-hand divider line is owned by the Splitter that follows. */
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .col-header {
    display: flex;
    align-items: center;
    padding: 4px 8px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .col-title {
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--accent-dim);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .col-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 6px 4px;
  }

  .empty-hint {
    padding: 12px 8px;
    color: var(--text-dim);
    font-size: 0.8rem;
  }

  .chips {
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 0 2px;
  }

  .chip {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 8px;
    background: var(--surface-raised);
    border: 1px solid var(--border);
    border-radius: 12px;
    color: var(--text-muted);
    font-size: 0.78rem;
    cursor: pointer;
    text-align: left;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: background 0.1s, color 0.1s;
  }

  .chip:hover {
    background: var(--chip-hover);
    color: var(--text-secondary);
  }

  .chip.active {
    background: var(--accent);
    border-color: var(--accent-primary);
    color: var(--accent-text-alt);
  }

  .chip.sequence {
    border-style: dashed;
  }

  .chip.sequence.active {
    background: #1a3020;
    border-color: #4ade80;
    color: #86efac;
  }

  .seq-icon {
    font-size: 0.7rem;
  }

  .chip.cursor-at:not(.active) {
    background: #1e3550;
    border-color: var(--accent-faint);
    color: #b0c4d8;
  }
</style>
