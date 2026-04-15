<script>
  import { onMount, onDestroy } from 'svelte'
  import { omnibarOpen, workbenchStore } from '../stores/workbench.js'
  import { ParseFileForUI } from '../../wailsjs/go/main/App'

  let query = ''
  let selectedIndex = -1
  let inputEl = null
  let panelEl = null
  let loading = false

  // ── Global shortcut: Ctrl+K (Win/Linux) / Cmd+K (Mac) ───────────────────────
  function onGlobalKeydown(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
      e.preventDefault()
      omnibarOpen.update(v => !v)
    }
  }
  onMount(()  => document.addEventListener('keydown', onGlobalKeydown))
  onDestroy(() => document.removeEventListener('keydown', onGlobalKeydown))

  // fileCache: filePath → ParsedFile (plain object for Svelte reactivity on reassign)
  let fileCache = {}
  let lastCollectionDir = null

  // Clear cache when the collection changes
  $: {
    const dir = $workbenchStore.collectionDir
    if (dir !== lastCollectionDir) {
      fileCache = {}
      lastCollectionDir = dir
    }
  }

  // Always keep the currently open file's live parsedFile in the cache so edits
  // are reflected immediately without a stale re-parse.
  $: {
    const fp  = $workbenchStore.selectedChapterKey?.filePath
    const pf  = $workbenchStore.parsedFile
    if (fp && pf) fileCache = { ...fileCache, [fp]: pf }
  }

  // Load all uncached collection files when the omnibar opens
  $: if ($omnibarOpen) loadAllFiles()

  async function loadAllFiles() {
    const summaries = $workbenchStore.summaries ?? []
    const toLoad = summaries.filter(s => !fileCache[s.FilePath] && !s.ScanError)
    if (toLoad.length === 0) return
    loading = true
    await Promise.all(toLoad.map(async s => {
      try {
        const pf = await ParseFileForUI(s.FilePath)
        if (pf) fileCache = { ...fileCache, [s.FilePath]: pf }
      } catch (e) {
        console.error('OmniBar: failed to parse', s.FilePath, e)
      }
    }))
    loading = false
  }

  // Flat list of { filePath, chapter } across the entire collection
  $: allFileChapters = (() => {
    const out = []
    for (const [filePath, pf] of Object.entries(fileCache))
      for (const ch of pf.Chapters ?? [])
        out.push({ filePath, chapter: ch })
    return out
  })()

  // Autofocus input when panel opens
  $: if ($omnibarOpen && inputEl) setTimeout(() => inputEl?.focus(), 0)

  // Reset keyboard selection on every keystroke
  $: query, (selectedIndex = -1)

  // ── Fuzzy subsequence scorer ─────────────────────────────────────────────────
  // Returns null if needle is not a subsequence of haystack (case-insensitive).
  // Score bonuses: +2 consecutive chars, +1 word-boundary match, +3 prefix match.
  function fuzzyScore(needle, haystack) {
    const n = needle.toLowerCase()
    const h = haystack.toLowerCase()
    let ni = 0, score = 0, lastMatchIdx = -2, firstMatch = true
    for (let hi = 0; hi < h.length && ni < n.length; hi++) {
      if (h[hi] !== n[ni]) continue
      if (hi === lastMatchIdx + 1) score += 2
      const prev = hi === 0 ? ' ' : h[hi - 1]
      if (prev === ' ' || prev === '_' || prev === '-') score += 1
      if (firstMatch && hi === 0) score += 3
      firstMatch = false
      lastMatchIdx = hi
      ni++
    }
    return ni < n.length ? null : score
  }

  // ── Results ──────────────────────────────────────────────────────────────────
  $: results = (() => {
    const q = query.trim()
    if (!q) return { chapters: [], contexts: [], nodes: [] }

    // Chapters — up to 5
    const chapters = allFileChapters
      .map(({ filePath, chapter }) => ({ filePath, chapter, s: fuzzyScore(q, chapter.Title) }))
      .filter(x => x.s !== null)
      .sort((a, b) => b.s - a.s)
      .slice(0, 5)

    // Contexts — deduplicated by tag name across entire collection, up to 6
    const ctxMap = new Map()
    for (const { filePath, chapter } of allFileChapters)
      for (const node of chapter.Nodes ?? [])
        for (const tag of node.EffectiveCtx ?? [])
          if (!ctxMap.has(tag)) {
            const s = fuzzyScore(q, tag)
            if (s !== null) ctxMap.set(tag, { tag, filePath, chapterTitle: chapter.Title, s })
          }
    const contexts = [...ctxMap.values()]
      .sort((a, b) => b.s - a.s).slice(0, 6)

    // Nodes — up to 10
    const nodes = []
    for (const { filePath, chapter } of allFileChapters)
      for (const node of chapter.Nodes ?? []) {
        const s = fuzzyScore(q, node.Text ?? '')
        if (s !== null) nodes.push({ filePath, chapterTitle: chapter.Title, node, s })
      }
    nodes.sort((a, b) => b.s - a.s)
    nodes.splice(10)

    return { chapters, contexts, nodes }
  })()

  $: flatResults = [
    ...results.chapters.map(r => ({ type: 'chapter', ...r })),
    ...results.contexts.map(r => ({ type: 'context', ...r })),
    ...results.nodes.map(r    => ({ type: 'node',    ...r })),
  ]
  $: totalCount = flatResults.length

  // ── Navigation ───────────────────────────────────────────────────────────────
  function navigate(item) {
    const pf = fileCache[item.filePath] ?? null
    if (item.type === 'chapter') {
      workbenchStore.update(s => ({
        ...s,
        selectedChapterKey: { filePath: item.filePath, chapterTitle: item.chapter.Title },
        parsedFile: pf,
        activeContextFilter: new Set(), selectedNodeLine: null,
        activeTab: 'note', pendingScrollLine: item.chapter.LineStart ?? null,
        isDirty: false, isParsing: false,
      }))
    } else if (item.type === 'context') {
      const ch = pf?.Chapters?.find(c => c.Title === item.chapterTitle)
      workbenchStore.update(s => ({
        ...s,
        selectedChapterKey: { filePath: item.filePath, chapterTitle: item.chapterTitle },
        parsedFile: pf,
        activeContextFilter: new Set([item.tag]), selectedNodeLine: null,
        activeTab: 'note', pendingScrollLine: ch?.LineStart ?? null,
        isDirty: false, isParsing: false,
      }))
    } else if (item.type === 'node') {
      workbenchStore.update(s => ({
        ...s,
        selectedChapterKey: { filePath: item.filePath, chapterTitle: item.chapterTitle },
        parsedFile: pf,
        activeContextFilter: new Set(), selectedNodeLine: item.node.Line,
        activeTab: 'note', pendingScrollLine: item.node.Line,
        isDirty: false, isParsing: false,
        nodeListScrollTarget: item.node.Line,
      }))
    }
    close()
  }

  function close() {
    omnibarOpen.set(false)
    query = ''
    selectedIndex = -1
  }

  // ── Keyboard navigation ──────────────────────────────────────────────────────
  function handleKeydown(e) {
    if (e.key === 'Escape') { close(); return }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      selectedIndex = Math.min(selectedIndex + 1, flatResults.length - 1)
      setTimeout(() => {
        panelEl?.querySelector(`[data-idx="${selectedIndex}"]`)
          ?.scrollIntoView({ block: 'nearest' })
      }, 0)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (selectedIndex <= 0) {
        selectedIndex = -1
        inputEl?.focus()
      } else {
        selectedIndex--
      }
    } else if (e.key === 'Enter' && selectedIndex >= 0) {
      e.preventDefault()
      navigate(flatResults[selectedIndex])
    }
  }
</script>

{#if $omnibarOpen}
  <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <div class="omnibar-backdrop" on:click={close} role="presentation" />

  <div
    class="omnibar-panel"
    bind:this={panelEl}
    on:keydown={handleKeydown}
    role="dialog"
    aria-label="Search"
  >
    <div class="omnibar-input-row">
      <span class="search-icon" aria-hidden="true">🔍</span>
      <input
        bind:this={inputEl}
        bind:value={query}
        class="omnibar-input"
        type="text"
        placeholder="Search chapters, contexts, nodes…"
        autocomplete="off"
        spellcheck="false"
      />
      {#if loading}
        <span class="loading-hint">loading…</span>
      {/if}
      <button class="omnibar-close" on:click={close} title="Close (Esc)">✕</button>
    </div>

    {#if query.trim()}
      {#if totalCount === 0}
        <div class="omnibar-empty">{loading ? 'Loading collection…' : 'No results'}</div>
      {:else}
        <div class="omnibar-results">

          {#if results.chapters.length}
            <div class="result-label">Chapters</div>
            {#each results.chapters as r, i}
              {@const idx = i}
              <button
                class="result-item"
                class:selected={selectedIndex === idx}
                data-idx={idx}
                on:click={() => navigate({ type: 'chapter', ...r })}
                on:mouseenter={() => (selectedIndex = idx)}
              >
                <span class="result-icon">§</span>
                <span class="result-text">{r.chapter.Title}</span>
                <span class="result-hint">{r.filePath.split('/').pop()}</span>
              </button>
            {/each}
          {/if}

          {#if results.contexts.length}
            <div class="result-label">Contexts</div>
            {#each results.contexts as r, i}
              {@const idx = results.chapters.length + i}
              <button
                class="result-item"
                class:selected={selectedIndex === idx}
                data-idx={idx}
                on:click={() => navigate({ type: 'context', ...r })}
                on:mouseenter={() => (selectedIndex = idx)}
              >
                <span class="result-icon">::</span>
                <span class="result-text">{r.tag}</span>
                <span class="result-hint">{r.chapterTitle}</span>
              </button>
            {/each}
          {/if}

          {#if results.nodes.length}
            <div class="result-label">Nodes</div>
            {#each results.nodes as r, i}
              {@const idx = results.chapters.length + results.contexts.length + i}
              <button
                class="result-item"
                class:selected={selectedIndex === idx}
                data-idx={idx}
                on:click={() => navigate({ type: 'node', ...r })}
                on:mouseenter={() => (selectedIndex = idx)}
              >
                <span class="result-icon">¶</span>
                <span class="result-text">{r.node.Text}</span>
                <span class="result-hint">{r.chapterTitle}</span>
              </button>
            {/each}
          {/if}

        </div>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .omnibar-backdrop {
    position: fixed;
    inset: 0;
    z-index: 299;
    background: rgba(0, 0, 0, 0.35);
  }

  .omnibar-panel {
    position: fixed;
    top: 220px;
    left: 0;
    right: 0;
    z-index: 300;
    background: #1a2535;
    border-bottom: 1px solid #2a3a4a;
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.55);
    display: flex;
    flex-direction: column;
    max-height: 60vh;
  }

  .omnibar-input-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 12px;
    border-bottom: 1px solid #2a3a4a;
    flex-shrink: 0;
  }

  .search-icon {
    font-size: 0.9rem;
    color: #5a7a9a;
    flex-shrink: 0;
  }

  .omnibar-input {
    flex: 1;
    background: transparent;
    border: none;
    outline: none;
    color: #e8eaf0;
    font-size: 0.95rem;
    font-family: inherit;
    caret-color: #7dd3fc;
  }

  .omnibar-input::placeholder {
    color: #3a5a7a;
  }

  .loading-hint {
    font-size: 0.75rem;
    color: #4a6a7a;
    flex-shrink: 0;
    font-style: italic;
  }

  .omnibar-close {
    background: none;
    border: none;
    color: #5a7a9a;
    cursor: pointer;
    font-size: 0.9rem;
    padding: 2px 6px;
    flex-shrink: 0;
  }

  .omnibar-close:hover {
    color: #e8eaf0;
  }

  .omnibar-results {
    overflow-y: auto;
    padding: 4px 0 8px;
  }

  .omnibar-empty {
    padding: 16px;
    text-align: center;
    color: #4a5a6a;
    font-size: 0.84rem;
  }

  .result-label {
    padding: 6px 14px 2px;
    font-size: 0.68rem;
    font-weight: 600;
    color: #5a7a9a;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .result-item {
    display: flex;
    align-items: baseline;
    gap: 8px;
    width: 100%;
    padding: 6px 14px;
    background: none;
    border: none;
    color: #c8d0dc;
    font-size: 0.84rem;
    text-align: left;
    cursor: pointer;
    transition: background 0.08s;
  }

  .result-item:hover,
  .result-item.selected {
    background: #1e2f40;
    color: #e8eaf0;
  }

  .result-item.selected {
    border-left: 2px solid #2563eb;
    padding-left: 12px;
  }

  .result-icon {
    color: #5a7a9a;
    font-family: monospace;
    font-size: 0.78rem;
    flex-shrink: 0;
    width: 18px;
  }

  .result-text {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .result-hint {
    font-size: 0.75rem;
    color: #4a6a7a;
    flex-shrink: 0;
    white-space: nowrap;
    max-width: 30%;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
