<script>
  import { onMount, onDestroy } from 'svelte'
  import { omnibarMode, workbenchStore, navigateToLine } from '../stores/workbench.js'
  import { ParseFileForUI, SearchGraph } from '../../wailsjs/go/main/App'
  import { appendLog } from '../stores/logPanel.js'
  import VirtualList from './VirtualList.svelte'

  let query = ''
  let selectedIndex = -1
  let inputEl = null
  let virtualListEl = null
  let loading = false

  // Graph search state
  let graphResults = null
  let graphLoading = false
  let debounceTimer = null

  // Limit handling — if no \limit is in the query, default to 100.
  // Backend ceiling enforced server-side at 10000.
  const DEFAULT_LIMIT = 100
  function extractUserLimit(q) {
    const m = /\\limit\s+(\d+)/i.exec(q)
    if (m) {
      return { hasLimit: true, limit: parseInt(m[1], 10), cleaned: q.replace(/\\limit\s+\d+/i, '').trim() }
    }
    return { hasLimit: false, limit: DEFAULT_LIMIT, cleaned: q.trim() }
  }

  // \notes is paginated by the backend at 60 rows per call. The user advances
  // pages with `\page N`. Extract the current page so we can render the next
  // hint after the last note row.
  function extractPageNr(q) {
    const m = /\\page\s+(\d+)/i.exec(q)
    return m ? Math.max(1, parseInt(m[1], 10)) : 1
  }

  // Per-tag expansion state for inline chapter chips on \context rows.
  // When a tag is in this Set, all its chapters are shown; otherwise the
  // first 5 plus a "+N more" toggle.
  const CONTEXT_CHIP_LIMIT = 5
  let expandedContextTags = new Set()
  function toggleContextExpand(tag) {
    if (expandedContextTags.has(tag)) expandedContextTags.delete(tag)
    else expandedContextTags.add(tag)
    expandedContextTags = expandedContextTags
  }

  // Pivot the omnibar query to `\notes \context "<tag>"` so the user sees
  // every note tagged with that context across the collection. Quoting is
  // applied when the tag has whitespace or commas to keep the parser happy.
  function pivotToNotesContext(tag) {
    const needsQuote = /[\s,]/.test(tag)
    const escaped = needsQuote ? `"${tag}"` : tag
    query = `\\notes \\context ${escaped}`
    selectedIndex = -1
    clearTimeout(debounceTimer)
    graphLoading = true
    graphResults = null
    dispatchGraphSearch(query.trim())
  }

  // Resolve a chapter title to its (filePath, chapter) and navigate to its
  // first node — same pattern as the fuzzy-chapter row's navigate() branch.
  function navigateToChapterByTitle(chapterTitle) {
    for (const [filePath, pf] of Object.entries(fileCache)) {
      for (const ch of pf.Chapters ?? []) {
        if (ch.Title === chapterTitle) {
          navigateToLine({
            filePath,
            chapterTitle: ch.Title,
            line: ch.LineStart ?? null,
            parsedFile: pf,
          })
          close()
          return
        }
      }
    }
  }

  // ── Mode flags ───────────────────────────────────────────────────────────────
  $: isFuzzyMode = $omnibarMode === 'fuzzy'
  $: isGraphMode = $omnibarMode === 'graph'
  $: isOpen = $omnibarMode !== null

  // ── Global shortcuts ─────────────────────────────────────────────────────────
  // Ctrl+K  → fuzzy search
  // Ctrl+Shift+K → graph search
  function onGlobalKeydown(e) {
    if ((e.ctrlKey || e.metaKey) && !e.shiftKey && e.key === 'k') {
      // stopPropagation in addition to preventDefault — capture-phase
      // means we fire BEFORE CodeMirror's keymap, but only stopPropagation
      // prevents the event from continuing down to the editor element
      // (where CodeMirror would map it to deleteLine for Ctrl+Shift+K or
      // similar collisions for other Ctrl/Cmd shortcuts).
      e.preventDefault()
      e.stopPropagation()
      omnibarMode.update(m => m === 'fuzzy' ? null : 'fuzzy')
    }
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault()
      e.stopPropagation()
      omnibarMode.update(m => m === 'graph' ? null : 'graph')
    }
  }
  // Capture phase: must run before CodeMirror's defaultKeymap so
  // Ctrl+Shift+K doesn't trigger deleteLine on the focused editor before
  // toggling graph mode (which would dirty the file).
  onMount(()  => document.addEventListener('keydown', onGlobalKeydown, true))
  onDestroy(() => document.removeEventListener('keydown', onGlobalKeydown, true))

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
    const stripped = q.replace(/^\\limit\s+\d+\s*/i, '')
    return /^\\(notes|story|stories|sequence|seq|path|paths|from)(\s|$)/i.test(stripped)
  }

  // Reads `cache` from the second arg (rather than the closed-over `fileCache`)
  // so reactive callers can declare cache as a dependency — see `$: enrichedGraphNodes`,
  // `flatStructural`, `flatNotes`, `flatPaths` — and re-run when loadAllFiles
  // finishes. Otherwise, results that arrived before the cache filled stay
  // stranded with filePath=null and silent-skip on click forever.
  //
  // Match is whitespace-tolerant: the parser preserves trailing whitespace in
  // some node strings while the Bleve indexer trims, so an exact string-equal
  // would miss perfectly valid targets.
  // Whitespace-tolerant node lookup against the in-memory file cache.
  // Returns true if any cached file has a node whose text equals `text`
  // (after trim). Used by the virtualItems builder to decide whether a
  // listing row is navigable.
  function lookupNodeInCache(text) {
    const target = String(text ?? '').trim()
    if (!target) return false
    for (const pf of Object.values(fileCache)) {
      for (const ch of pf.Chapters ?? []) {
        for (const node of ch.Nodes ?? []) {
          if (String(node.Text ?? '').trim() === target) return true
        }
      }
    }
    return false
  }

  function enrichGraphNodes(nodes, cache) {
    const pairs = Object.entries(cache ?? {})
    return nodes.map(gn => {
      const target = String(gn.NodeText ?? '').trim()
      for (const [filePath, pf] of pairs) {
        for (const ch of pf.Chapters ?? []) {
          for (const node of ch.Nodes ?? []) {
            if (String(node.Text ?? '').trim() === target) {
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
      // Ensure the backend gets at least DEFAULT_LIMIT results when the user
      // didn't specify \limit. The frontend virtual scroller renders all of
      // them; the user-facing `\limit` directive is left untouched if present.
      const { hasLimit } = extractUserLimit(q)
      const queryToSend = hasLimit ? q : `\\limit ${DEFAULT_LIMIT} ${q}`
      const result = await SearchGraph(queryToSend)
      graphResults = result
      // Mirror inline graph errors into the log panel so the user keeps a
      // history across queries — the inline message still shows as today.
      if (result?.GraphError) {
        appendLog([{ severity: 'error', source: 'search', message: `${queryToSend}: ${result.GraphError}` }])
      }
    } catch (e) {
      console.error('OmniBar: graph search error', e)
      appendLog([{ severity: 'error', source: 'search', message: `${queryToSend}: ${String(e?.message ?? e)}` }])
    } finally {
      graphLoading = false
    }
  }

  // ── Fuzzy results (only computed in fuzzy mode) ──────────────────────────────
  $: results = (() => {
    if (!isFuzzyMode) return { chapters: [], contexts: [], nodes: [] }
    const { limit, cleaned } = extractUserLimit(query)
    if (!cleaned) return { chapters: [], contexts: [], nodes: [] }

    const chapters = allFileChapters
      .map(({ filePath, chapter }) => ({ filePath, chapter, s: fuzzyScore(cleaned, chapter.Title) }))
      .filter(x => x.s !== null)
      .sort((a, b) => b.s - a.s)
      .slice(0, limit)

    const ctxMap = new Map()
    for (const { filePath, chapter } of allFileChapters)
      for (const node of chapter.Nodes ?? [])
        for (const tag of node.EffectiveCtx ?? [])
          if (!ctxMap.has(tag)) {
            const s = fuzzyScore(cleaned, tag)
            if (s !== null) ctxMap.set(tag, { tag, filePath, chapterTitle: chapter.Title, s })
          }
    const contexts = [...ctxMap.values()].sort((a, b) => b.s - a.s).slice(0, limit)

    const nodes = []
    for (const { filePath, chapter } of allFileChapters)
      for (const node of chapter.Nodes ?? []) {
        const s = fuzzyScore(cleaned, node.Text ?? '')
        if (s !== null) nodes.push({ filePath, chapterTitle: chapter.Title, node, s })
      }
    nodes.sort((a, b) => b.s - a.s)
    nodes.splice(limit)

    return { chapters, contexts, nodes }
  })()

  // ── Graph result enrichment ──────────────────────────────────────────────────
  // All four derivations explicitly read `fileCache` so Svelte's reactive system
  // re-runs them when loadAllFiles() finishes. Without that dependency, results
  // that arrive before the cache is populated stay stranded with filePath:null
  // and silent-skip on click forever.
  $: enrichedGraphNodes = graphResults?.Nodes ? enrichGraphNodes(graphResults.Nodes, fileCache) : []

  // Sequence chains expand into one flat item per step so the user can jump to
  // any node. Path chains are kept as whole chains — one nav entry per path,
  // rendered horizontally with arrow labels between nodes.
  $: flatStructural = (() => {
    void fileCache // re-run when cache fills (re-enrichment)
    return (graphResults?.Structural ?? [])
      .filter(chain => (chain.Kind ?? 'sequence') === 'sequence')
      .flatMap((chain, chainIdx) => {
        return (chain.Nodes ?? []).map((step, stepIdx) => {
          const enriched = enrichGraphNodes(
            [{ NodeText: step.NodeText, Chapter: '', ContextTags: [], Orbit: [] }],
            fileCache,
          )[0]
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
  })()

  // Full notes — one entry per NoteBlock; clicking navigates to the root node.
  $: flatNotes = (() => {
    void fileCache
    return (graphResults?.Notes ?? [])
      .flatMap((note, noteIdx) =>
        (note.Blocks ?? []).map((block, blockIdx) => {
          const enriched = enrichGraphNodes(
            [{ NodeText: block.NodeText, Chapter: note.Chapter, ContextTags: [], Orbit: [] }],
            fileCache,
          )[0]
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
  })()

  // Path chains — one entry per chain; clicking navigates to the start node.
  $: flatPaths = (() => {
    void fileCache
    return (graphResults?.Structural ?? [])
      .filter(chain => chain.Kind === 'path')
      .map((chain, chainIdx) => {
        const startText = chain.Nodes?.[0]?.NodeText ?? ''
        const enriched = enrichGraphNodes(
          [{ NodeText: startText, Chapter: '', ContextTags: [], Orbit: [] }],
          fileCache,
        )[0]
        return {
          type: 'graph-structural',
          chainIdx,
          chapter: chain.Chapter ?? '',
          nodes: chain.Nodes ?? [],
          nodeText: startText,
          ...enriched,
        }
      })
  })()

  // ── Unified virtualized list ─────────────────────────────────────────────────
  // virtualItems is the full render order (labels, group headers, separators,
  // and actionable rows). flatResults is the actionable subset, indexed by
  // selectedIndex for keyboard navigation. actionableToVirtual maps
  // selectedIndex → virtualItems index for scroll-to-row math.
  $: virtualItems = (() => {
    const out = []
    let aIdx = 0
    const pushAction = (kind, data, extra = {}) => {
      out.push({ kind, data, actionableIdx: aIdx, ...extra })
      aIdx += 1
    }

    if (isFuzzyMode) {
      if (results.chapters.length) {
        out.push({ kind: 'label', text: 'Chapters' })
        for (const r of results.chapters) pushAction('fuzzy-chapter', { type: 'chapter', ...r })
      }
      if (results.contexts.length) {
        out.push({ kind: 'label', text: 'Contexts' })
        for (const r of results.contexts) pushAction('fuzzy-context', { type: 'context', ...r })
      }
      if (results.nodes.length) {
        out.push({ kind: 'label', text: 'Nodes' })
        for (const r of results.nodes) pushAction('fuzzy-node', { type: 'node', ...r })
      }
    }

    if (isGraphMode && graphResults && !graphResults.GraphUnavailable && !graphResults.GraphError) {
      if (enrichedGraphNodes.length) {
        out.push({ kind: 'label', text: 'Graph Nodes' })
        for (let i = 0; i < enrichedGraphNodes.length; i++) {
          pushAction('graph-node', { type: 'graph-node', ...enrichedGraphNodes[i] }, { displayIdx: i })
        }
      }

      if (flatPaths.length) {
        const pathsByChapter = new Map()
        for (const p of flatPaths) {
          const k = p.chapter || ''
          if (!pathsByChapter.has(k)) pathsByChapter.set(k, [])
          pathsByChapter.get(k).push(p)
        }
        for (const [chapter, paths] of pathsByChapter) {
          out.push({ kind: 'path-group-header', chapter })
          for (const p of paths) pushAction('path', p)
        }
      }

      if (flatStructural.length) {
        out.push({ kind: 'label', text: 'Sequences' })
        let prev = null
        for (const r of flatStructural) {
          const isNewGroup = !prev || prev.context !== r.context || prev.chapter !== r.chapter
          const isNewChain = !prev || prev.chainIdx !== r.chainIdx
          if (isNewGroup) {
            out.push({ kind: 'sequence-group-header', context: r.context, chapter: r.chapter })
          } else if (isNewChain) {
            out.push({ kind: 'sequence-chain-sep' })
          }
          const isLastStep = r.stepIdx === r.totalSteps - 1
          pushAction('sequence-step', r, { isLastStep })
          prev = r
        }
      }

      if (flatNotes.length) {
        const notesByChapter = new Map()
        for (const n of flatNotes) {
          const k = n.chapter || ''
          if (!notesByChapter.has(k)) notesByChapter.set(k, [])
          notesByChapter.get(k).push(n)
        }
        for (const [chapter, blocks] of notesByChapter) {
          out.push({ kind: 'note-group-header', chapter })
          for (const n of blocks) {
            pushAction('note-block-header', n)
            for (const arrow of n.arrows) {
              out.push({ kind: 'note-block-arrow', arrow })
            }
          }
        }
        // \notes is paginated server-side (60 PageMap rows per call). Append a
        // marker so the user knows where the page ends and how to load more.
        const pageNr = extractPageNr(query)
        out.push({ kind: 'notes-end-of-page', pageNr })
      }

      if (graphResults.Listings?.length) {
        const chapterItems = graphResults.Listings.filter(l => l.Kind === 'chapter')
        const hintItems = graphResults.Listings.filter(l => l.Kind === 'hint')
        const contextItems = graphResults.Listings.filter(l => l.Kind === 'context')
        const otherItems = graphResults.Listings.filter(l =>
          l.Kind !== 'chapter' && l.Kind !== 'hint' && l.Kind !== 'context')

        // Hints render at the top so the user sees the syntax suggestion
        // before any (possibly noisy) result list.
        if (hintItems.length) {
          for (const r of hintItems) {
            out.push({ kind: 'listing-hint', title: r.Title, detail: r.Detail })
          }
        }

        if (chapterItems.length) {
          out.push({ kind: 'label', text: 'Chapters' })
          for (const r of chapterItems) {
            const data = { type: 'graph-listing', ...r }
            pushAction('listing-chapter-header', data)
            if (r.Contexts?.length) {
              out.push({ kind: 'listing-chapter-contexts', contexts: r.Contexts })
            } else if (r.Detail) {
              out.push({ kind: 'listing-chapter-detail', detail: r.Detail })
            }
            if (r.Nodes?.length) {
              for (let ni = 0; ni < r.Nodes.length; ni++) {
                out.push({ kind: 'listing-chapter-node', nodeText: r.Nodes[ni], idx: ni })
              }
            }
          }
        }

        if (contextItems.length) {
          out.push({ kind: 'label', text: 'Contexts' })
          for (const r of contextItems) {
            pushAction('listing-context-row', { type: 'graph-listing', ...r })
          }
        }

        if (otherItems.length) {
          const kindLabel = { arrow: 'Arrows', 'note-line': 'Notes' }
          const otherByKind = new Map()
          for (const item of otherItems) {
            if (!otherByKind.has(item.Kind)) otherByKind.set(item.Kind, [])
            otherByKind.get(item.Kind).push(item)
          }
          for (const [kind, items] of otherByKind) {
            out.push({ kind: 'label', text: kindLabel[kind] ?? kind })
            for (const r of items) {
              // navigate() looks up r.NodeText in fileCache; if it's empty
              // or doesn't resolve, the click would silently no-op. Render
              // such rows as inert (visually muted, no actionableIdx, not
              // reachable via keyboard) so the user knows the row carries
              // no jump target.
              const navigable = !!r.NodeText && lookupNodeInCache(r.NodeText)
              if (navigable) {
                pushAction('listing-other', { type: 'graph-listing', ...r }, { listingKind: kind })
              } else {
                out.push({ kind: 'listing-other-inert', data: { type: 'graph-listing', ...r }, listingKind: kind })
              }
            }
          }
        }
      }
    }

    return out
  })()

  $: flatResults = virtualItems.filter(v => v.actionableIdx !== undefined).map(v => v.data)

  $: actionableToVirtual = (() => {
    const m = new Map()
    for (let vi = 0; vi < virtualItems.length; vi++) {
      const v = virtualItems[vi]
      if (v.actionableIdx !== undefined) m.set(v.actionableIdx, vi)
    }
    return m
  })()

  $: totalCount = flatResults.length

  // ── Result count summary ─────────────────────────────────────────────────────
  // Per-bucket counts joined with ' · ', e.g. "12 chapters · 247 nodes · 8 contexts".
  // Helps users see what came back at a glance and that there's more to scroll.
  function plural(n, singular, pluralForm) {
    return `${n} ${n === 1 ? singular : (pluralForm ?? singular + 's')}`
  }

  $: resultSummary = (() => {
    const parts = []
    if (isFuzzyMode) {
      if (results.chapters.length) parts.push(plural(results.chapters.length, 'chapter'))
      if (results.contexts.length) parts.push(plural(results.contexts.length, 'context', 'contexts'))
      if (results.nodes.length) parts.push(plural(results.nodes.length, 'node'))
      return parts.join(' · ')
    }
    if (!isGraphMode || !graphResults || graphResults.GraphUnavailable || graphResults.GraphError) {
      return ''
    }
    if (enrichedGraphNodes.length) parts.push(plural(enrichedGraphNodes.length, 'node'))
    if (flatPaths.length) parts.push(plural(flatPaths.length, 'path'))
    const sequenceChainCount = (graphResults.Structural ?? []).filter(c => (c.Kind ?? 'sequence') === 'sequence').length
    if (sequenceChainCount) {
      parts.push(plural(sequenceChainCount, 'sequence'))
      if (flatStructural.length !== sequenceChainCount) {
        parts.push(plural(flatStructural.length, 'step'))
      }
    }
    if (flatNotes.length) {
      parts.push(plural(flatNotes.length, 'note'))
      const arrowTotal = flatNotes.reduce((a, n) => a + (n.arrows?.length ?? 0), 0)
      if (arrowTotal) parts.push(plural(arrowTotal, 'arrow'))
    }
    const listings = graphResults.Listings ?? []
    const chapterListings = listings.filter(l => l.Kind === 'chapter')
    if (chapterListings.length) {
      parts.push(plural(chapterListings.length, 'chapter'))
      const nodeTotal = chapterListings.reduce((a, l) => a + (l.Nodes?.length ?? 0), 0)
      if (nodeTotal) parts.push(plural(nodeTotal, 'node'))
    }
    const ctxListings = listings.filter(l => l.Kind === 'context').length
    if (ctxListings) parts.push(plural(ctxListings, 'context', 'contexts'))
    const arrowListings = listings.filter(l => l.Kind === 'arrow').length
    if (arrowListings) parts.push(plural(arrowListings, 'arrow'))
    const noteLineListings = listings.filter(l => l.Kind === 'note-line').length
    if (noteLineListings) parts.push(plural(noteLineListings, 'note line'))
    return parts.join(' · ')
  })()

  // ── Navigation ───────────────────────────────────────────────────────────────
  // trace() emits a one-line breadcrumb to the log panel (Ctrl+Shift+M) for
  // every navigate() decision so click failures stop being silent. severity
  // info → hidden by default in the panel's `errors`/`warnings` filters.
  function trace(action, item, detail = {}) {
    appendLog([{
      severity: 'info',
      source: 'search',
      filePath: item?.filePath,
      line: item?.line,
      message: `navigate(${item?.type ?? '?'}) → ${action}${detail.reason ? `: ${detail.reason}` : ''}`,
    }])
  }

  function navigate(item) {
    trace('enter', item)
    const pf = fileCache[item.filePath] ?? null
    if (item.type === 'chapter') {
      navigateToLine({
        filePath: item.filePath,
        chapterTitle: item.chapter.Title,
        line: item.chapter.LineStart ?? null,
        parsedFile: pf,
      })
      trace('opened', item)
    } else if (item.type === 'context') {
      const ch = pf?.Chapters?.find(c => c.Title === item.chapterTitle)
      navigateToLine({
        filePath: item.filePath,
        chapterTitle: item.chapterTitle,
        line: ch?.LineStart ?? null,
        parsedFile: pf,
        contextFilter: new Set([item.tag]),
      })
      trace('opened', item, { reason: `context ${item.tag}` })
    } else if (item.type === 'node') {
      navigateToLine({
        filePath: item.filePath,
        chapterTitle: item.chapterTitle,
        line: item.node.Line,
        parsedFile: pf,
      })
      trace('opened', item, { reason: `line ${item.node.Line}` })
    } else if (item.type === 'graph-node' || item.type === 'graph-structural' || item.type === 'graph-note') {
      if (!item.filePath) {
        trace('skipped', item, { reason: 'filePath null (NodeText not in fileCache; graph result enriched before files loaded?)' })
        close()
        return
      }
      const gpf = fileCache[item.filePath] ?? null
      navigateToLine({
        filePath: item.filePath,
        chapterTitle: item.chapterTitle ?? '',
        line: item.line,
        parsedFile: gpf,
      })
      trace('opened', item, { reason: `line ${item.line}` })
    } else if (item.type === 'graph-listing') {
      // Enter on a \context row pivots the query to `\notes \context "<tag>"`
      // so the user gets every note tagged with that context — keeps the
      // omnibar open instead of closing to navigate.
      if (item.Kind === 'context') {
        pivotToNotesContext(item.Title)
        trace('pivoted', item, { reason: `\\notes \\context "${item.Title}"` })
        return
      }
      if (!item.NodeText) {
        trace('skipped', item, { reason: 'listing has no NodeText' })
        close()
        return
      }
      const target = String(item.NodeText).trim()
      let found = false
      outer: for (const [filePath, lpf] of Object.entries(fileCache)) {
        for (const ch of lpf.Chapters ?? []) {
          for (const node of ch.Nodes ?? []) {
            if (String(node.Text ?? '').trim() === target) {
              navigateToLine({
                filePath,
                chapterTitle: ch.Title,
                line: node.Line,
                parsedFile: lpf,
              })
              trace('opened', item, { reason: `${ch.Title}:${node.Line}` })
              found = true
              break outer
            }
          }
        }
      }
      if (!found) {
        trace('skipped', item, { reason: `NodeText "${target}" not found in fileCache` })
      }
    }
    close()
  }

  function close() {
    omnibarMode.set(null)
    query = ''
    selectedIndex = -1
  }

  function scrollSelectedIntoView() {
    if (selectedIndex < 0 || !virtualListEl) return
    const vi = actionableToVirtual.get(selectedIndex)
    if (vi !== undefined) virtualListEl.scrollToIndex(vi)
  }

  // ── Keyboard navigation ──────────────────────────────────────────────────────
  function handleKeydown(e) {
    if (e.key === 'Tab') { e.preventDefault(); return }
    if (e.key === 'Escape') { close(); return }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      selectedIndex = Math.min(selectedIndex + 1, flatResults.length - 1)
      scrollSelectedIntoView()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (selectedIndex <= 0) {
        selectedIndex = -1
        inputEl?.focus()
      } else {
        selectedIndex--
        scrollSelectedIntoView()
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
      {#if isGraphMode && graphLoading}
        <div class="omnibar-status"><div class="graph-searching">Searching…</div></div>
      {:else if isGraphMode && isListingQuery(query.trim()) && !graphResults}
        <div class="omnibar-status"><div class="graph-searching">Press Enter to search…</div></div>
      {:else if isGraphMode && graphResults?.GraphUnavailable}
        <div class="omnibar-status">
          <div class="graph-unavailable">
            {graphResults.GraphError || 'Build your graph first (Ctrl+G) to enable graph search.'}
          </div>
        </div>
      {:else if isGraphMode && graphResults?.GraphError}
        <div class="omnibar-status">
          <div class="result-label">Error</div>
          <div class="graph-error">{graphResults.GraphError}</div>
        </div>
      {:else if totalCount === 0}
        <div class="omnibar-status">
          <div class="omnibar-empty">{loading ? 'Loading collection…' : 'No results'}</div>
        </div>
      {:else}
        {#if resultSummary}
          <div class="omnibar-summary" title="Scroll the list for the full content">
            <span class="omnibar-summary-counts">{resultSummary}</span>
            <span class="omnibar-summary-hint">↓ scroll for more</span>
          </div>
        {/if}
        <div class="omnibar-results">
          <VirtualList
            bind:this={virtualListEl}
            items={virtualItems}
            estimatedHeight={28}
            overscan={5}
            let:item
            let:index
          >
            {#if item.kind === 'label'}
              <div class="result-label" data-vidx={index}>{item.text}</div>

            {:else if item.kind === 'fuzzy-chapter'}
              <button
                class="result-item"
                class:selected={selectedIndex === item.actionableIdx}
                data-idx={item.actionableIdx}
                on:click={() => navigate(item.data)}
                on:mouseenter={() => (selectedIndex = item.actionableIdx)}
              >
                <span class="result-icon">§</span>
                <span class="result-text">{item.data.chapter.Title}</span>
                <span class="result-hint">{item.data.filePath.split('/').pop()}</span>
              </button>

            {:else if item.kind === 'fuzzy-context'}
              <button
                class="result-item"
                class:selected={selectedIndex === item.actionableIdx}
                data-idx={item.actionableIdx}
                on:click={() => navigate(item.data)}
                on:mouseenter={() => (selectedIndex = item.actionableIdx)}
              >
                <span class="result-icon">::</span>
                <span class="result-text">{item.data.tag}</span>
                <span class="result-hint">{item.data.chapterTitle}</span>
              </button>

            {:else if item.kind === 'fuzzy-node'}
              <button
                class="result-item"
                class:selected={selectedIndex === item.actionableIdx}
                data-idx={item.actionableIdx}
                on:click={() => navigate(item.data)}
                on:mouseenter={() => (selectedIndex = item.actionableIdx)}
              >
                <span class="result-icon">¶</span>
                <span class="result-text">{item.data.node.Text}</span>
                <span class="result-hint">{item.data.chapterTitle}</span>
              </button>

            {:else if item.kind === 'graph-node'}
              <div class="vrow-graph-node">
                <button
                  class="result-item orbit-node-header"
                  class:selected={selectedIndex === item.actionableIdx}
                  data-idx={item.actionableIdx}
                  on:click={() => navigate(item.data)}
                  on:mouseenter={() => (selectedIndex = item.actionableIdx)}
                >
                  <span class="orbit-node-idx">{item.displayIdx}:</span>
                  <span class="result-text">{item.data.NodeText}</span>
                  {#if item.data.chapterTitle ?? item.data.Chapter}
                    <span class="result-hint">{item.data.chapterTitle ?? item.data.Chapter}</span>
                  {/if}
                </button>
                {#if item.data.Orbit?.length}
                  <div class="orbit-list">
                    {#each item.data.Orbit as orb}
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
              </div>

            {:else if item.kind === 'path-group-header'}
              <div class="sequence-group-header">
                <span class="sequence-context">:: paths ::</span>
                {#if item.chapter}
                  <span class="sequence-chapter">{item.chapter}</span>
                {/if}
              </div>

            {:else if item.kind === 'path'}
              <button
                class="result-item path-chain"
                class:selected={selectedIndex === item.actionableIdx}
                data-idx={item.actionableIdx}
                on:click={() => navigate(item.data)}
                on:mouseenter={() => (selectedIndex = item.actionableIdx)}
              >
                {#each item.data.nodes as step, si}
                  <span class="path-node">{step.NodeText}</span>
                  {#if si < item.data.nodes.length - 1 && step.ArrowLong}
                    <span class="path-arrow">-({step.ArrowLong})-&gt;</span>
                  {/if}
                {/each}
              </button>

            {:else if item.kind === 'sequence-group-header'}
              <div class="sequence-group-header">
                {#if item.context}
                  <span class="sequence-context">:: {item.context} ::</span>
                {/if}
                {#if item.chapter}
                  <span class="sequence-chapter">{item.chapter}</span>
                {/if}
                {#if !item.context && !item.chapter}
                  <span class="sequence-context sequence-context-muted">(uncontextualised)</span>
                {/if}
              </div>

            {:else if item.kind === 'sequence-chain-sep'}
              <div class="sequence-chain-sep" />

            {:else if item.kind === 'sequence-step'}
              <button
                class="result-item sequence-step"
                class:selected={selectedIndex === item.actionableIdx}
                class:chain-start={item.data.stepIdx === 0}
                class:chain-end={item.isLastStep}
                data-idx={item.actionableIdx}
                on:click={() => navigate(item.data)}
                on:mouseenter={() => (selectedIndex = item.actionableIdx)}
              >
                <div class="step-gutter">
                  <span class="step-num">({item.data.stepIdx + 1})</span>
                  {#if !item.isLastStep}<div class="step-stem" />{/if}
                </div>
                <span class="step-text">{item.data.nodeText}</span>
              </button>

            {:else if item.kind === 'note-group-header'}
              <div class="note-group-header">
                <span class="note-group-marker">── notes ──</span>
                {#if item.chapter}
                  <span class="note-group-chapter">{item.chapter}</span>
                {/if}
              </div>

            {:else if item.kind === 'note-block-header'}
              <button
                class="result-item note-block-header"
                class:selected={selectedIndex === item.actionableIdx}
                data-idx={item.actionableIdx}
                on:click={() => navigate(item.data)}
                on:mouseenter={() => (selectedIndex = item.actionableIdx)}
              >
                <span class="note-node-text">{item.data.nodeText}</span>
                {#if item.data.context}
                  <span class="note-node-context">:: {item.data.context} ::</span>
                {/if}
              </button>

            {:else if item.kind === 'note-block-arrow'}
              <div class="note-arrow-line">
                <span class="note-arrow">({item.arrow.ArrowLong})</span>
                <span class="note-arrow-target">{item.arrow.Target}</span>
              </div>

            {:else if item.kind === 'notes-end-of-page'}
              <div class="notes-end-of-page">
                <span class="notes-end-rule">──── end of page {item.pageNr} ────</span>
                <span class="notes-end-hint">type <code>\page {item.pageNr + 1}</code> for more · empty result means you've reached the end</span>
              </div>

            {:else if item.kind === 'listing-chapter-header'}
              <button
                class="result-item listing-chapter-header"
                class:selected={selectedIndex === item.actionableIdx}
                data-idx={item.actionableIdx}
                on:click={() => navigate(item.data)}
                on:mouseenter={() => (selectedIndex = item.actionableIdx)}
                title="Open chapter “{item.data.Title}”"
              >
                <span class="result-icon graph-icon">§</span>
                <span class="result-text">{item.data.Title}</span>
              </button>

            {:else if item.kind === 'listing-chapter-contexts'}
              <div class="listing-context-chips">
                {#each item.contexts as ctx}
                  <button
                    class="listing-context-chip"
                    on:click|stopPropagation={() => pivotToNotesContext(ctx)}
                    title="Show all notes tagged with :: {ctx} ::"
                  >{ctx}</button>
                {/each}
              </div>

            {:else if item.kind === 'listing-chapter-detail'}
              <div class="result-hint listing-empty-hint">{item.detail}</div>

            {:else if item.kind === 'listing-chapter-node'}
              <div class="listing-node-row">
                <span class="listing-node-idx">{item.idx}.</span>
                <span class="listing-node-text">{item.nodeText}</span>
              </div>

            {:else if item.kind === 'listing-hint'}
              <div class="listing-hint">
                <span class="listing-hint-title">{item.title}</span>
                {#if item.detail}<span class="listing-hint-detail">{item.detail}</span>{/if}
              </div>

            {:else if item.kind === 'listing-context-row'}
              <div class="listing-context-row">
                <button
                  class="result-item listing-context-tag"
                  class:selected={selectedIndex === item.actionableIdx}
                  data-idx={item.actionableIdx}
                  on:click={() => pivotToNotesContext(item.data.Title)}
                  on:mouseenter={() => (selectedIndex = item.actionableIdx)}
                  title="Show all notes tagged with this context"
                >
                  <span class="result-icon graph-icon">::</span>
                  <span class="result-text">{item.data.Title}</span>
                </button>
                {#if item.data.Chapters?.length}
                  {@const chs = item.data.Chapters}
                  {@const expanded = expandedContextTags.has(item.data.Title)}
                  {@const visible = expanded ? chs : chs.slice(0, CONTEXT_CHIP_LIMIT)}
                  <div class="listing-context-chips">
                    {#each visible as chapName}
                      <button
                        class="listing-context-chip"
                        on:click|stopPropagation={() => navigateToChapterByTitle(chapName)}
                        title="Open chapter “{chapName}”"
                      >{chapName}</button>
                    {/each}
                    {#if !expanded && chs.length > CONTEXT_CHIP_LIMIT}
                      <button
                        class="listing-context-more"
                        on:click|stopPropagation={() => toggleContextExpand(item.data.Title)}
                      >+{chs.length - CONTEXT_CHIP_LIMIT} more</button>
                    {/if}
                  </div>
                {/if}
              </div>

            {:else if item.kind === 'listing-other'}
              <button
                class="result-item"
                class:selected={selectedIndex === item.actionableIdx}
                data-idx={item.actionableIdx}
                on:click={() => navigate(item.data)}
                on:mouseenter={() => (selectedIndex = item.actionableIdx)}
              >
                <span class="result-icon graph-icon">{item.listingKind === 'arrow' ? '→' : '·'}</span>
                <span class="result-text">{item.data.Title}</span>
                {#if item.data.Detail}<span class="result-hint">{item.data.Detail}</span>{/if}
                {#if item.listingKind === 'arrow' && item.data.MetaType}<span class="arrow-meta">{item.data.MetaType}</span>{/if}
                {#if item.listingKind === 'arrow' && item.data.Context}<span class="arrow-context">:: {item.data.Context} ::</span>{/if}
              </button>

            {:else if item.kind === 'listing-other-inert'}
              <div class="result-item result-item-inert" title="No navigation target — this listing has no associated node">
                <span class="result-icon graph-icon">{item.listingKind === 'arrow' ? '→' : '·'}</span>
                <span class="result-text">{item.data.Title}</span>
                {#if item.data.Detail}<span class="result-hint">{item.data.Detail}</span>{/if}
                {#if item.listingKind === 'arrow' && item.data.MetaType}<span class="arrow-meta">{item.data.MetaType}</span>{/if}
                {#if item.listingKind === 'arrow' && item.data.Context}<span class="arrow-context">:: {item.data.Context} ::</span>{/if}
              </div>
            {/if}
          </VirtualList>
        </div>
      {/if}
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
    flex: 1;
    min-height: 0;
    padding: 4px 0 8px;
    display: flex;
    flex-direction: column;
    position: relative;
  }

  .omnibar-results :global(.vlist-viewport) {
    flex: 1;
    min-height: 0;
  }

  /* Static fade hint that more content is below the viewport. */
  .omnibar-results::after {
    content: "";
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 24px;
    pointer-events: none;
    background: linear-gradient(to bottom, rgba(26, 37, 53, 0) 0%, #1a2535 100%);
  }

  .omnibar-summary {
    flex-shrink: 0;
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
    padding: 4px 14px 4px;
    border-bottom: 1px solid #1a2530;
    font-size: 0.72rem;
    color: #6a8aa0;
  }

  .omnibar-summary-counts {
    font-variant-numeric: tabular-nums;
    letter-spacing: 0.02em;
  }

  .omnibar-summary-hint {
    color: #4a6a7a;
    font-style: italic;
    flex-shrink: 0;
  }

  .omnibar-status {
    overflow-y: auto;
    padding: 4px 0 8px;
  }

  .vrow-graph-node {
    display: flex;
    flex-direction: column;
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

  /* Inert listing rows — Kind:'arrow' / 'note-line' that have no navigation
     target (no NodeText, or NodeText not present in the in-memory file
     cache). Rendered as plain <div>s with no click handler so the user
     gets no false expectation. */
  .result-item-inert {
    cursor: default;
    color: #6a8aa0;
  }
  .result-item-inert:hover {
    background: none;
    color: #6a8aa0;
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

  .listing-empty-hint {
    padding: 2px 0 6px 28px;
    font-size: 0.78rem;
    font-style: italic;
    color: #8a9aaa;
  }

  .listing-node-row {
    display: flex;
    gap: 6px;
    font-size: 0.78rem;
    color: #8a9aaa;
    padding: 1px 14px 1px 42px;
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

  /* \context listing row — tag button + inline chapter chips */
  .listing-context-row {
    display: flex;
    flex-direction: column;
  }
  .listing-context-tag {
    /* uses .result-item base; nothing extra needed but kept for selector specificity */
  }
  .listing-context-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 6px;
    padding: 0 14px 6px 42px;
  }
  .listing-context-chip,
  .listing-context-more {
    background: #1e2f40;
    border: 1px solid #2a4258;
    border-radius: 10px;
    padding: 1px 8px;
    color: #a0b4c4;
    font-size: 0.72rem;
    font-family: inherit;
    cursor: pointer;
    line-height: 1.4;
    transition: background 0.08s, color 0.08s, border-color 0.08s;
  }
  .listing-context-chip:hover {
    background: #2a4258;
    color: #e8eaf0;
    border-color: #3a5a7a;
  }
  .listing-context-more {
    color: #6a8aa0;
    font-style: italic;
  }
  .listing-context-more:hover {
    color: #c8d2dc;
    border-color: #3a5a7a;
  }

  /* Did-you-mean hints surfaced by the search backend */
  .listing-hint {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin: 6px 14px;
    padding: 8px 12px;
    background: #1e3242;
    border-left: 2px solid #c9a96e;
    border-radius: 3px;
    font-size: 0.78rem;
  }
  .listing-hint-title {
    color: #d8c590;
    font-weight: 500;
  }
  .listing-hint-detail {
    color: #8aa0b0;
    font-style: italic;
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
  .note-block-header {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 8px;
    padding: 4px 14px;
  }
  .note-block-header .note-node-text {
    font-weight: 500;
    color: #c8d2dc;
    font-size: 0.86rem;
  }
  .note-block-header.selected .note-node-text {
    color: #e8eaf0;
  }
  .note-block-header .note-node-context {
    font-size: 0.68rem;
    color: #c9a96e;
    font-style: italic;
  }
  .note-arrow-line {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    font-size: 0.78rem;
    padding: 1px 14px 1px 32px;
  }
  .note-arrow {
    color: #6a9aaa;
    font-style: italic;
    flex-shrink: 0;
  }
  .note-arrow-target {
    color: #b0bac5;
  }

  .notes-end-of-page {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    padding: 10px 14px 14px;
    color: #6a8aa0;
    font-size: 0.72rem;
    text-align: center;
  }
  .notes-end-rule {
    letter-spacing: 0.04em;
    color: #4a6a7a;
  }
  .notes-end-hint {
    color: #6a8aa0;
    font-style: italic;
  }
  .notes-end-hint code {
    font-style: normal;
    background: #0f1e2e;
    color: #8aa8c9;
    padding: 0 4px;
    border-radius: 3px;
    font-family: monospace;
  }
</style>
