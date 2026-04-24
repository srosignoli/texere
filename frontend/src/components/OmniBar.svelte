<script>
  import { onMount, onDestroy } from 'svelte'
  import { omnibarMode, workbenchStore } from '../stores/workbench.js'
  import { ParseFileForUI, SearchGraph } from '../../wailsjs/go/main/App'

  let query = ''
  let selectedIndex = -1
  let inputEl = null
  let panelEl = null
  let loading = false

  // Graph search state
  let graphResults = null
  let graphLoading = false
  let debounceTimer = null

  // ── Mode flags ───────────────────────────────────────────────────────────────
  $: isFuzzyMode = $omnibarMode === 'fuzzy'
  $: isGraphMode = $omnibarMode === 'graph'
  $: isOpen = $omnibarMode !== null

  // ── Global shortcuts ─────────────────────────────────────────────────────────
  // Ctrl+K  → fuzzy search
  // Ctrl+Shift+K → graph search
  function onGlobalKeydown(e) {
    if ((e.ctrlKey || e.metaKey) && !e.shiftKey && e.key === 'k') {
      e.preventDefault()
      omnibarMode.update(m => m === 'fuzzy' ? null : 'fuzzy')
    }
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault()
      omnibarMode.update(m => m === 'graph' ? null : 'graph')
    }
  }
  onMount(()  => document.addEventListener('keydown', onGlobalKeydown))
  onDestroy(() => document.removeEventListener('keydown', onGlobalKeydown))

  // ── Reset query when panel opens ─────────────────────────────────────────────
  let prevMode = null
  $: {
    if ($omnibarMode !== null && prevMode === null) {
      query = ''
      selectedIndex = -1
      graphResults = null
    }
    prevMode = $omnibarMode
  }

  // ── Placeholder text ─────────────────────────────────────────────────────────
  $: placeholder = isFuzzyMode
    ? 'Search chapters, contexts, nodes…'
    : 'Graph query: \\chapter any,  node text \\chapter title,  \\from A \\to B…'

  // ── File cache ───────────────────────────────────────────────────────────────
  let fileCache = {}
  let lastCollectionDir = null

  $: {
    const dir = $workbenchStore.collectionDir
    if (dir !== lastCollectionDir) {
      fileCache = {}
      lastCollectionDir = dir
    }
  }

  // Clear cached graph results when graph is rebuilt
  $: $workbenchStore.graphVersion, (graphResults = null)

  // Keep the currently open file in cache so edits are reflected immediately
  $: {
    const fp = $workbenchStore.selectedChapterKey?.filePath
    const pf = $workbenchStore.parsedFile
    if (fp && pf) fileCache = { ...fileCache, [fp]: pf }
  }

  // Load all uncached collection files when the panel opens
  $: if (isOpen) loadAllFiles()

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
  $: if (isOpen && inputEl) setTimeout(() => inputEl?.focus(), 0)

  // Reset keyboard selection on every keystroke
  $: query, (selectedIndex = -1)

  // ── Fuzzy subsequence scorer ─────────────────────────────────────────────────
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

  // ── Graph search ─────────────────────────────────────────────────────────────
  // Listing queries (\notes …) scan the full DB and must only fire on Enter,
  // not on every keystroke.
  function isListingQuery(q) {
    return /^\\(notes|story|stories|sequence|seq|path|paths|from)(\s|$)/i.test(q)
  }

  function enrichGraphNodes(nodes) {
    return nodes.map(gn => {
      for (const [filePath, pf] of Object.entries(fileCache)) {
        for (const ch of pf.Chapters ?? []) {
          for (const node of ch.Nodes ?? []) {
            if (node.Text === gn.NodeText) {
              return { ...gn, filePath, line: node.Line, chapterTitle: ch.Title }
            }
          }
        }
      }
      return { ...gn, filePath: null, line: null, chapterTitle: gn.Chapter }
    })
  }

  // Dispatch graph search whenever in graph mode with a non-empty query.
  // Listing queries (\notes …) require explicit Enter — they scan the full DB
  // and must not fire on every keystroke.
  $: {
    clearTimeout(debounceTimer)
    const q = query.trim()
    if (q && isGraphMode) {
      if (isListingQuery(q)) {
        // Clear stale results from a previous query; user must press Enter.
        graphResults = null
        graphLoading = false
      } else {
        graphLoading = true
        graphResults = null
        debounceTimer = setTimeout(() => dispatchGraphSearch(q), 300)
      }
    } else if (!isGraphMode) {
      graphResults = null
      graphLoading = false
    }
  }

  async function dispatchGraphSearch(q) {
    try {
      const result = await SearchGraph(q)
      graphResults = result
    } catch (e) {
      console.error('OmniBar: graph search error', e)
    } finally {
      graphLoading = false
    }
  }

  // ── Fuzzy results (only computed in fuzzy mode) ──────────────────────────────
  $: results = (() => {
    const q = query.trim()
    if (!q || !isFuzzyMode) return { chapters: [], contexts: [], nodes: [] }

    const chapters = allFileChapters
      .map(({ filePath, chapter }) => ({ filePath, chapter, s: fuzzyScore(q, chapter.Title) }))
      .filter(x => x.s !== null)
      .sort((a, b) => b.s - a.s)
      .slice(0, 5)

    const ctxMap = new Map()
    for (const { filePath, chapter } of allFileChapters)
      for (const node of chapter.Nodes ?? [])
        for (const tag of node.EffectiveCtx ?? [])
          if (!ctxMap.has(tag)) {
            const s = fuzzyScore(q, tag)
            if (s !== null) ctxMap.set(tag, { tag, filePath, chapterTitle: chapter.Title, s })
          }
    const contexts = [...ctxMap.values()].sort((a, b) => b.s - a.s).slice(0, 6)

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

  // ── Graph result enrichment ──────────────────────────────────────────────────
  $: enrichedGraphNodes = (() => {
    if (!graphResults?.Nodes) return []
    const raw = graphResults.Nodes.slice(0, 10)
    return enrichGraphNodes(raw)
  })()

  // Sequence chains expand into one flat item per step so the user can jump to
  // any node. Path chains are kept as whole chains — one nav entry per path,
  // rendered horizontally with arrow labels between nodes.
  $: flatStructural = (graphResults?.Structural ?? [])
    .filter(chain => (chain.Kind ?? 'sequence') === 'sequence')
    .flatMap((chain, chainIdx) => {
      return (chain.Nodes ?? []).map((step, stepIdx) => {
        const enriched = enrichGraphNodes([
          { NodeText: step.NodeText, Chapter: '', ContextTags: [], Orbit: [] }
        ])[0]
        return {
          type: 'graph-structural',
          chainIdx,
          stepIdx,
          totalSteps: chain.Nodes.length,
          context: chain.Context ?? '',
          chapter: chain.Chapter ?? '',
          nodeText: step.NodeText,
          ...enriched,
        }
      })
    })

  // Full notes — one entry per NoteBlock; clicking navigates to the root node.
  $: flatNotes = (graphResults?.Notes ?? [])
    .flatMap((note, noteIdx) =>
      (note.Blocks ?? []).map((block, blockIdx) => {
        const enriched = enrichGraphNodes([
          { NodeText: block.NodeText, Chapter: note.Chapter, ContextTags: [], Orbit: [] }
        ])[0]
        return {
          type: 'graph-note',
          noteIdx,
          blockIdx,
          chapter: note.Chapter,
          nodeText: block.NodeText,
          line: block.Line,
          context: block.Context ?? '',
          arrows: block.Arrows ?? [],
          ...enriched,
        }
      })
    )

  // Path chains — one entry per chain; clicking navigates to the start node.
  $: flatPaths = (graphResults?.Structural ?? [])
    .filter(chain => chain.Kind === 'path')
    .map((chain, chainIdx) => {
      const startText = chain.Nodes?.[0]?.NodeText ?? ''
      const enriched = enrichGraphNodes([
        { NodeText: startText, Chapter: '', ContextTags: [], Orbit: [] }
      ])[0]
      return {
        type: 'graph-structural',
        chainIdx,
        chapter: chain.Chapter ?? '',
        nodes: chain.Nodes ?? [],
        nodeText: startText,
        ...enriched,
      }
    })

  // ── Combined flat list for keyboard navigation ───────────────────────────────
  $: flatResults = [
    ...(isFuzzyMode ? results.chapters.map(r => ({ type: 'chapter', ...r }))         : []),
    ...(isFuzzyMode ? results.contexts.map(r => ({ type: 'context', ...r }))         : []),
    ...(isFuzzyMode ? results.nodes.map(r => ({ type: 'node', ...r }))               : []),
    ...(isGraphMode ? enrichedGraphNodes.map(r => ({ type: 'graph-node', ...r }))    : []),
    ...(isGraphMode ? flatPaths                                                        : []),
    ...(isGraphMode ? flatStructural                                                   : []),
    ...(isGraphMode ? flatNotes                                                        : []),
    ...(isGraphMode ? (graphResults?.Listings ?? []).map(r => ({ type: 'graph-listing', ...r })) : []),
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
    } else if (item.type === 'graph-node' || item.type === 'graph-structural' || item.type === 'graph-note') {
      if (item.filePath) {
        const gpf = fileCache[item.filePath] ?? null
        workbenchStore.update(s => ({
          ...s,
          selectedChapterKey: { filePath: item.filePath, chapterTitle: item.chapterTitle ?? '' },
          parsedFile: gpf,
          activeContextFilter: new Set(), selectedNodeLine: item.line,
          activeTab: 'note', pendingScrollLine: item.line,
          isDirty: false, isParsing: false,
          nodeListScrollTarget: item.line,
        }))
      }
    } else if (item.type === 'graph-listing') {
      if (item.NodeText) {
        outer: for (const [filePath, lpf] of Object.entries(fileCache)) {
          for (const ch of lpf.Chapters ?? []) {
            for (const node of ch.Nodes ?? []) {
              if (node.Text === item.NodeText) {
                workbenchStore.update(s => ({
                  ...s,
                  selectedChapterKey: { filePath, chapterTitle: ch.Title },
                  parsedFile: lpf,
                  activeContextFilter: new Set(), selectedNodeLine: node.Line,
                  activeTab: 'note', pendingScrollLine: node.Line,
                  isDirty: false, isParsing: false,
                  nodeListScrollTarget: node.Line,
                }))
                break outer
              }
            }
          }
        }
      }
    }
    close()
  }

  function close() {
    omnibarMode.set(null)
    query = ''
    selectedIndex = -1
  }

  // ── Keyboard navigation ──────────────────────────────────────────────────────
  function handleKeydown(e) {
    if (e.key === 'Tab') { e.preventDefault(); return }
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
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (selectedIndex >= 0) {
        navigate(flatResults[selectedIndex])
      } else if (isGraphMode) {
        const q = query.trim()
        if (q) {
          clearTimeout(debounceTimer)
          graphLoading = true
          graphResults = null
          dispatchGraphSearch(q)
        }
      }
    }
  }
</script>

{#if isOpen}
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
      <span class="search-icon" aria-hidden="true">{isFuzzyMode ? '🔍' : '◈'}</span>
      <input
        bind:this={inputEl}
        bind:value={query}
        class="omnibar-input"
        type="text"
        {placeholder}
        autocomplete="off"
        spellcheck="false"
      />
      {#if loading || graphLoading}
        <span class="loading-hint">{graphLoading ? 'searching…' : 'loading…'}</span>
      {/if}
      <span class="mode-badge" class:mode-graph={isGraphMode}>
        {isFuzzyMode ? 'Fuzzy' : 'Graph'}
      </span>
      <button class="omnibar-close" on:click={close} title="Close (Esc)">✕</button>
    </div>

    {#if query.trim()}
      <div class="omnibar-results">

        <!-- ── Fuzzy mode results ── -->
        {#if isFuzzyMode}
          {#if results.chapters.length}
            <div class="result-label">Chapters</div>
            {#each results.chapters as r, i}
              <button
                class="result-item"
                class:selected={selectedIndex === i}
                data-idx={i}
                on:click={() => navigate({ type: 'chapter', ...r })}
                on:mouseenter={() => (selectedIndex = i)}
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
        {/if}

        <!-- ── Graph mode results ── -->
        {#if isGraphMode}
          {#if graphLoading}
            <div class="graph-searching">Searching…</div>
          {:else if isListingQuery(query.trim()) && !graphResults}
            <div class="graph-searching">Press Enter to search…</div>
          {:else if graphResults?.GraphUnavailable}
            <div class="graph-unavailable">Build your graph first (Ctrl+G) to enable graph search.</div>
          {:else if graphResults?.GraphError}
            <div class="result-label">Error</div>
            <div class="graph-error">{graphResults.GraphError}</div>
          {:else if graphResults}
            {#if enrichedGraphNodes.length}
              <div class="result-label">Graph Nodes</div>
              {#each enrichedGraphNodes as r, i}
                <button
                  class="result-item orbit-node-header"
                  class:selected={selectedIndex === i}
                  data-idx={i}
                  on:click={() => navigate({ type: 'graph-node', ...r })}
                  on:mouseenter={() => (selectedIndex = i)}
                >
                  <span class="orbit-node-idx">{i}:</span>
                  <span class="result-text">{r.NodeText}</span>
                  {#if r.chapterTitle ?? r.Chapter}
                    <span class="result-hint">{r.chapterTitle ?? r.Chapter}</span>
                  {/if}
                </button>
                {#if r.Orbit?.length}
                  <div class="orbit-list">
                    {#each r.Orbit as orb}
                      <div class="orbit-entry" style="padding-left: {(orb.Radius - 1) * 18}px">
                        <span class="orbit-dash"> -</span>
                        <span class="orbit-arrow">({orb.ArrowLong})</span>
                        <span class="orbit-neighbor">- {orb.NeighborText}</span>
                        {#if orb.Ctx}
                          <span class="orbit-ctx">.. {orb.Ctx}</span>
                        {/if}
                      </div>
                    {/each}
                  </div>
                {/if}
              {/each}
            {/if}

            {#if flatPaths.length}
              {@const pathsByChapter = flatPaths.reduce((acc, p) => {
                (acc[p.chapter || ''] = acc[p.chapter || ''] ?? []).push(p); return acc
              }, {})}
              {#each Object.entries(pathsByChapter) as [chapter, paths]}
                <div class="sequence-group-header">
                  <span class="sequence-context">:: paths ::</span>
                  {#if chapter}
                    <span class="sequence-chapter">{chapter}</span>
                  {/if}
                </div>
                {#each paths as p}
                  {@const idx = enrichedGraphNodes.length + flatPaths.indexOf(p)}
                  <button
                    class="result-item path-chain"
                    class:selected={selectedIndex === idx}
                    data-idx={idx}
                    on:click={() => navigate(p)}
                    on:mouseenter={() => (selectedIndex = idx)}
                  >
                    {#each p.nodes as step, si}
                      <span class="path-node">{step.NodeText}</span>
                      {#if si < p.nodes.length - 1 && step.ArrowLong}
                        <span class="path-arrow">-({step.ArrowLong})-&gt;</span>
                      {/if}
                    {/each}
                  </button>
                {/each}
              {/each}
            {/if}

            {#if flatStructural.length}
              <div class="result-label">Sequences</div>
              {#each flatStructural as r, i}
                {@const idx = enrichedGraphNodes.length + flatPaths.length + i}
                {@const prev = i > 0 ? flatStructural[i - 1] : null}
                {@const isNewGroup = !prev || prev.context !== r.context || prev.chapter !== r.chapter}
                {@const isNewChain = !prev || prev.chainIdx !== r.chainIdx}
                {@const isLastStep = r.stepIdx === r.totalSteps - 1}

                {#if isNewGroup}
                  <div class="sequence-group-header">
                    {#if r.context}
                      <span class="sequence-context">:: {r.context} ::</span>
                    {/if}
                    {#if r.chapter}
                      <span class="sequence-chapter">{r.chapter}</span>
                    {/if}
                    {#if !r.context && !r.chapter}
                      <span class="sequence-context sequence-context-muted">(uncontextualised)</span>
                    {/if}
                  </div>
                {:else if isNewChain}
                  <div class="sequence-chain-sep" />
                {/if}

                <button
                  class="result-item sequence-step"
                  class:selected={selectedIndex === idx}
                  class:chain-start={r.stepIdx === 0}
                  class:chain-end={isLastStep}
                  data-idx={idx}
                  on:click={() => navigate(r)}
                  on:mouseenter={() => (selectedIndex = idx)}
                >
                  <div class="step-gutter">
                    <span class="step-num">({r.stepIdx + 1})</span>
                    {#if !isLastStep}<div class="step-stem" />{/if}
                  </div>
                  <span class="step-text">{r.nodeText}</span>
                </button>
              {/each}
            {/if}

            {#if flatNotes.length}
              {@const notesByChapter = flatNotes.reduce((acc, n) => {
                (acc[n.chapter || ''] = acc[n.chapter || ''] ?? []).push(n); return acc
              }, {})}
              {#each Object.entries(notesByChapter) as [chapter, blocks]}
                <div class="note-group-header">
                  <span class="note-group-marker">── notes ──</span>
                  {#if chapter}
                    <span class="note-group-chapter">{chapter}</span>
                  {/if}
                </div>
                {#each blocks as n}
                  {@const idx = enrichedGraphNodes.length + flatPaths.length + flatStructural.length + flatNotes.indexOf(n)}
                  <button
                    class="result-item note-block"
                    class:selected={selectedIndex === idx}
                    data-idx={idx}
                    on:click={() => navigate(n)}
                    on:mouseenter={() => (selectedIndex = idx)}
                  >
                    <div class="note-node">
                      <span class="note-node-text">{n.nodeText}</span>
                      {#if n.context}
                        <span class="note-node-context">:: {n.context} ::</span>
                      {/if}
                    </div>
                    {#if n.arrows.length}
                      <div class="note-arrow-list">
                        {#each n.arrows as arrow}
                          <div class="note-arrow-line">
                            <span class="note-arrow">({arrow.ArrowLong})</span>
                            <span class="note-arrow-target">{arrow.Target}</span>
                          </div>
                        {/each}
                      </div>
                    {/if}
                  </button>
                {/each}
              {/each}
            {/if}

            {#if graphResults.Listings?.length}
              {@const chapterItems = graphResults.Listings.filter(l => l.Kind === 'chapter')}
              {@const otherItems   = graphResults.Listings.filter(l => l.Kind !== 'chapter')}

              {#if chapterItems.length}
                <div class="result-label">Chapters</div>
                {#each chapterItems as r, ci}
                  {@const baseIdx = enrichedGraphNodes.length + flatPaths.length + flatStructural.length + flatNotes.length + ci}
                  <button
                    class="result-item listing-chapter-header"
                    class:selected={selectedIndex === baseIdx}
                    data-idx={baseIdx}
                    on:click={() => navigate({ type: 'graph-listing', ...r })}
                    on:mouseenter={() => (selectedIndex = baseIdx)}
                  >
                    <span class="result-icon graph-icon">§</span>
                    <span class="result-text">{r.Title}</span>
                  </button>
                  {#if r.Nodes?.length}
                    <div class="listing-node-list">
                      {#each r.Nodes as nodeText, ni}
                        <div class="listing-node-row">
                          <span class="listing-node-idx">{ni}.</span>
                          <span class="listing-node-text">{nodeText}</span>
                        </div>
                      {/each}
                    </div>
                  {/if}
                {/each}
              {/if}

              {#if otherItems.length}
                {@const kindLabel = { arrow: 'Arrows', context: 'Contexts', 'note-line': 'Notes' }}
                {@const otherByKind = otherItems.reduce((acc, item) => {
                  (acc[item.Kind] = acc[item.Kind] ?? []).push(item); return acc
                }, {})}
                {#each Object.entries(otherByKind) as [kind, items]}
                  <div class="result-label">{kindLabel[kind] ?? kind}</div>
                  {#each items as r, i}
                    {@const baseIdx = enrichedGraphNodes.length + flatPaths.length + flatStructural.length + flatNotes.length + chapterItems.length + i}
                    <button
                      class="result-item"
                      class:selected={selectedIndex === baseIdx}
                      data-idx={baseIdx}
                      on:click={() => navigate({ type: 'graph-listing', ...r })}
                      on:mouseenter={() => (selectedIndex = baseIdx)}
                    >
                      <span class="result-icon graph-icon">{kind === 'arrow' ? '→' : '·'}</span>
                      <span class="result-text">{r.Title}</span>
                      {#if r.Detail}<span class="result-hint">{r.Detail}</span>{/if}
                      {#if kind === 'arrow' && r.MetaType}<span class="arrow-meta">{r.MetaType}</span>{/if}
                      {#if kind === 'arrow' && r.Context}<span class="arrow-context">:: {r.Context} ::</span>{/if}
                    </button>
                  {/each}
                {/each}
              {/if}
            {/if}
          {/if}
        {/if}

        {#if totalCount === 0 && !graphLoading}
          <div class="omnibar-empty">{loading ? 'Loading collection…' : 'No results'}</div>
        {/if}

      </div>
    {:else if isGraphMode}
      <div class="omnibar-hint">
        Type a graph query — e.g. <code>\chapter any</code>, <code>node text \chapter title</code>, <code>\from A \to B</code>
      </div>
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

  .mode-badge {
    flex-shrink: 0;
    font-size: 0.65rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    padding: 1px 6px;
    border-radius: 3px;
    background: #1e3a50;
    color: #5a8aaa;
    border: 1px solid #2a4a6a;
  }

  .mode-badge.mode-graph {
    background: #1a3a2a;
    color: #5a9a7a;
    border-color: #2a5a3a;
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

  .omnibar-hint {
    padding: 12px 16px;
    font-size: 0.8rem;
    color: #4a6a7a;
    font-style: italic;
  }

  .omnibar-hint code {
    font-style: normal;
    color: #6a9aaa;
    background: #0f1e2e;
    padding: 1px 5px;
    border-radius: 3px;
    font-family: monospace;
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

  .graph-icon {
    color: #4a7a6a;
  }

  .arrow-meta {
    font-size: 0.7rem;
    padding: 1px 6px;
    border-radius: 3px;
    background: #e4efe9;
    color: #345244;
    flex-shrink: 0;
  }

  .arrow-context {
    font-size: 0.7rem;
    color: #6a7a7a;
    font-style: italic;
    flex-shrink: 0;
  }

  .graph-searching,
  .graph-unavailable,
  .graph-error {
    padding: 6px 14px;
    font-size: 0.8rem;
    font-style: italic;
    color: #4a6a7a;
  }

  .graph-error {
    color: #8a4a4a;
    font-style: normal;
  }

  .orbit-node-header {
    font-weight: 600;
  }

  .orbit-node-idx {
    flex-shrink: 0;
    width: 22px;
    text-align: right;
    color: #5a7a8a;
    font-size: 0.78rem;
    margin-right: 4px;
  }

  .orbit-list {
    padding: 2px 0 6px 28px;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .orbit-entry {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    font-size: 0.78rem;
    line-height: 1.4;
    align-items: baseline;
  }

  .orbit-dash {
    color: #4a7a6a;
    flex-shrink: 0;
  }

  .orbit-arrow {
    color: #6a9aaa;
    font-style: italic;
    flex-shrink: 0;
  }

  .orbit-neighbor {
    color: #c0ccd8;
  }

  .orbit-ctx {
    color: #7a8a70;
    font-size: 0.72rem;
  }

  .listing-chapter-header {
    font-weight: 600;
  }

  .listing-node-list {
    padding: 2px 0 6px 28px;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .listing-node-row {
    display: flex;
    gap: 6px;
    font-size: 0.78rem;
    color: #8a9aaa;
    padding: 1px 0;
  }

  .listing-node-idx {
    flex-shrink: 0;
    width: 20px;
    text-align: right;
    color: #5a7a8a;
  }

  .listing-node-text {
    color: #a0b4c4;
  }

  /* ── Sequence / \story \sequence groups ────────────────────────────────── */
  .sequence-group-header {
    padding: 8px 14px 4px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    border-top: 1px solid #1a2530;
  }
  .sequence-group-header:first-of-type {
    border-top: none;
  }
  .sequence-context {
    font-size: 0.72rem;
    font-weight: 600;
    color: #c9a96e;
    letter-spacing: 0.02em;
  }
  .sequence-context-muted {
    color: #6a7a8a;
    font-style: italic;
    font-weight: 500;
  }
  .sequence-chapter {
    font-size: 0.65rem;
    color: #5a7a9a;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .sequence-chain-sep {
    height: 1px;
    background: #1a2530;
    margin: 4px 14px;
  }

  .sequence-step {
    padding: 3px 14px;
    align-items: stretch;
    gap: 0;
  }
  .sequence-step .step-gutter {
    display: flex;
    flex-direction: column;
    align-items: center;
    flex-shrink: 0;
    width: 32px;
    padding-top: 4px;
  }
  .sequence-step .step-num {
    font-size: 0.65rem;
    color: #3b82f6;
    font-weight: 700;
    line-height: 1;
    white-space: nowrap;
  }
  .sequence-step .step-stem {
    width: 2px;
    flex: 1;
    background: #1e40af;
    margin-top: 2px;
    min-height: 8px;
  }
  .sequence-step .step-text {
    flex: 1;
    font-size: 0.82rem;
    color: #c8d2dc;
    padding-top: 2px;
    padding-bottom: 2px;
    word-break: break-word;
  }
  .sequence-step.selected .step-text {
    color: #e8eaf0;
  }

  /* ── Path chains (\from \to) ───────────────────────────────────────────── */
  .path-chain {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    padding: 5px 14px;
    font-size: 0.82rem;
  }
  .path-chain .path-node {
    font-weight: 500;
    color: #c8d2dc;
    white-space: nowrap;
  }
  .path-chain .path-arrow {
    color: #6a8a9a;
    font-style: italic;
    font-size: 0.72rem;
    white-space: nowrap;
  }
  .path-chain.selected .path-node {
    color: #e8eaf0;
  }

  /* ── Full notes (\notes) ───────────────────────────────────────────────── */
  .note-group-header {
    padding: 8px 14px 4px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    border-top: 1px solid #1a2530;
  }
  .note-group-header:first-of-type {
    border-top: none;
  }
  .note-group-marker {
    font-size: 0.72rem;
    font-weight: 600;
    color: #8aa8c9;
    letter-spacing: 0.02em;
  }
  .note-group-chapter {
    font-size: 0.65rem;
    color: #5a7a9a;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .note-block {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 2px;
    padding: 4px 14px;
  }
  .note-block .note-node {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 8px;
  }
  .note-block .note-node-text {
    font-weight: 500;
    color: #c8d2dc;
    font-size: 0.86rem;
  }
  .note-block.selected .note-node-text {
    color: #e8eaf0;
  }
  .note-block .note-node-context {
    font-size: 0.68rem;
    color: #c9a96e;
    font-style: italic;
  }
  .note-arrow-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding-left: 18px;
  }
  .note-arrow-line {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    font-size: 0.78rem;
  }
  .note-arrow {
    color: #6a9aaa;
    font-style: italic;
    flex-shrink: 0;
  }
  .note-arrow-target {
    color: #b0bac5;
  }
  .note-block.selected .note-arrow-target {
    color: #d8dae0;
  }
</style>
