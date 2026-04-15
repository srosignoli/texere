<script>
  import { workbenchStore, contextTags } from '../stores/workbench.js'

  let scrollEl = null
  let _tagScrollTimer = null

  function toggleTag(name) {
    workbenchStore.update((s) => {
      const next = new Set(s.activeContextFilter)
      if (next.has(name)) {
        next.delete(name)
      } else {
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
            on:click={() => toggleTag(tag.name)}
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
    flex: 0 0 160px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    border-right: 1px solid #2a3a4a;
  }

  .col-header {
    display: flex;
    align-items: center;
    padding: 4px 8px;
    border-bottom: 1px solid #2a3a4a;
    flex-shrink: 0;
  }

  .col-title {
    font-size: 0.7rem;
    font-weight: 600;
    color: #5a7a9a;
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
    color: #4a5a6a;
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
    background: #1e2f40;
    border: 1px solid #2a3a4a;
    border-radius: 12px;
    color: #8899aa;
    font-size: 0.78rem;
    cursor: pointer;
    text-align: left;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: background 0.1s, color 0.1s;
  }

  .chip:hover {
    background: #253545;
    color: #c8d0dc;
  }

  .chip.active {
    background: #1e3a5a;
    border-color: #2563eb;
    color: #93c5fd;
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
    border-color: #3a5a7a;
    color: #b0c4d8;
  }
</style>
