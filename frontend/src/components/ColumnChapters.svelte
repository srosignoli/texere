<script>
  import { createEventDispatcher } from 'svelte'
  import { workbenchStore } from '../stores/workbench.js'

  const dispatch = createEventDispatcher()

  let scrollEl = null

  // Set of file paths that are collapsed; all expanded by default
  let collapsed = new Set()

  $: summaries = $workbenchStore.summaries
  $: selectedKey = $workbenchStore.selectedChapterKey
  $: isDirty = $workbenchStore.isDirty

  function isSelected(filePath, chapterTitle) {
    return selectedKey?.filePath === filePath && selectedKey?.chapterTitle === chapterTitle
  }

  function handleClick(filePath, chapterTitle) {
    dispatch('chapter-selected', { filePath, chapterTitle })
  }

  function toggleCollapse(filePath) {
    collapsed = collapsed.has(filePath)
      ? (collapsed.delete(filePath), new Set(collapsed))
      : new Set(collapsed).add(filePath)
  }

  // Scroll the active chapter into view whenever the selection changes.
  // Auto-expands the file tree if it was collapsed so the chapter is in the DOM.
  $: if (selectedKey && scrollEl) {
    const key = `${selectedKey.filePath}::${selectedKey.chapterTitle}`
    if (collapsed.has(selectedKey.filePath)) {
      collapsed = new Set([...collapsed].filter(p => p !== selectedKey.filePath))
    }
    setTimeout(() => {
      scrollEl.querySelector(`[data-key="${CSS.escape(key)}"]`)
        ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }, 0)
  }
</script>

<div class="col-chapters">
  <div class="col-header">
    <span class="col-title">Files / Chapters</span>
  </div>
  <div class="col-scroll" bind:this={scrollEl}>
    {#if summaries.length === 0}
      <div class="empty-hint">No files</div>
    {:else}
      {#each summaries as summary (summary.FilePath)}
        {@const isCollapsed = collapsed.has(summary.FilePath)}
        <div class="file-group">
          <!-- File root row -->
          <button
            class="file-row"
            title={summary.FilePath}
            on:click={() => toggleCollapse(summary.FilePath)}
          >
            <span class="chevron">{isCollapsed ? '▶' : '▼'}</span>
            <span class="file-name">{summary.RelPath?.includes('/') ? summary.RelPath : summary.FileName}</span>
          </button>

          <!-- Chapter children -->
          {#if !isCollapsed}
            {#if summary.ScanError}
              <div class="scan-error">⚠ {summary.ScanError}</div>
            {:else}
              {#each (summary.Chapters ?? []) as title, i}
                {@const last = i === (summary.Chapters?.length ?? 0) - 1}
                <button
                  class="chapter-item"
                  data-key="{summary.FilePath}::{title}"
                  class:active={isSelected(summary.FilePath, title)}
                  class:dirty={isSelected(summary.FilePath, title) && isDirty}
                  on:click={() => handleClick(summary.FilePath, title)}
                >
                  <span class="tree-line">{last ? '└' : '├'}</span>
                  <span class="chapter-title">{title}</span>
                  {#if isSelected(summary.FilePath, title) && isDirty}
                    <span class="dirty-dot" title="Unsaved changes">●</span>
                  {/if}
                </button>
              {/each}
            {/if}
          {/if}
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .col-chapters {
    /* width is owned by the parent slot in TopBand (uiLayout.chaptersWidth);
       the right-hand divider line is owned by the Splitter that follows. */
    display: flex;
    flex-direction: column;
    overflow: hidden;
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
    padding: 4px 0;
  }

  .empty-hint {
    padding: 12px 8px;
    color: #4a5a6a;
    font-size: 0.8rem;
  }

  .file-group {
    margin-bottom: 2px;
  }

  /* File root row */
  .file-row {
    display: flex;
    align-items: center;
    gap: 5px;
    width: 100%;
    padding: 4px 6px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
  }

  .file-row:hover .file-name {
    color: #c8d0dc;
  }

  .chevron {
    font-size: 0.55rem;
    color: #4a6a7a;
    flex-shrink: 0;
    width: 10px;
    text-align: center;
  }

  .file-name {
    font-size: 0.78rem;
    font-weight: 600;
    color: #7a9ab5;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Chapter child rows */
  .chapter-item {
    display: flex;
    align-items: center;
    width: 100%;
    padding: 3px 6px 3px 8px;
    background: none;
    border: none;
    color: #b0bec5;
    font-size: 0.8rem;
    text-align: left;
    cursor: pointer;
    gap: 4px;
  }

  .chapter-item:hover {
    background: #1e2f40;
    color: #e8eaf0;
  }

  .chapter-item.active {
    background: #1e3a5a;
    color: #e8eaf0;
  }

  .tree-line {
    font-size: 0.7rem;
    color: #2e4a5a;
    flex-shrink: 0;
    font-family: monospace;
    width: 12px;
  }

  .chapter-title {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .scan-error {
    padding: 2px 8px 2px 20px;
    color: #f59e0b;
    font-size: 0.75rem;
  }

  .dirty-dot {
    color: #f59e0b;
    font-size: 0.6rem;
    flex-shrink: 0;
  }
</style>
