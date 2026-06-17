<script>
  // 2.5D Isometric Graph Explorer (feature 006).
  //
  // Skeleton-first reveal (FR-032a/b/c):
  //   1. Layout JSON arrives from ExplorerOpen.
  //   2. Frame 0: ground plane is added and rendered (one frame, ≤16 ms).
  //   3. Next animation frame: all node + edge meshes are built at
  //      opacity 0. They are present in the scene but invisible.
  //   4. Fade-in tweens fire: node N starts at distanceFromAnchor*45 ms
  //      and fades to opacity 1 over 200 ms. Edges fade in alongside
  //      whichever endpoint appears later.
  //
  // Camera lock (FR-003): nothing in this file (or any file we depend
  // on) mutates camera.quaternion. Pan and zoom are the only verbs.
  //
  // The rAF loop runs only while there are active fade-in animations.
  // Pan / zoom outside of animations call fireChange in Scene.js, which
  // routes through onCameraChange so we can rescale labels + picking
  // meshes immediately before the (single, non-rAF) render.

  import { onMount, onDestroy } from 'svelte'
  import { get } from 'svelte/store'
  import * as THREE from 'three'
  import { createScene } from './explorer/Scene.js'
  import { createPicker } from './explorer/Picking.js'
  import { buildNodeMeshes } from './explorer/NodeMeshes.js'
  import { buildEdgeMeshes } from './explorer/EdgeMeshes.js'
  import { buildLayerPlanes } from './explorer/LayerPlanes.js'
  import { createHighlight } from './explorer/Highlight.js'
  import MiniMap from './MiniMap.svelte'
  import ExplorerLegend from './ExplorerLegend.svelte'
  import ExplorerStats from './ExplorerStats.svelte'
  import ExplorerSidebar from './ExplorerSidebar.svelte'
  import { createAnchorHistory } from './explorer/AnchorHistory.js'
  import {
    tweenReanchor,
    computeCameraFrame,
    snapshotCamera,
    snapshotNodePositions,
  } from './explorer/ReanchorAnimator.js'
  import {
    explorerOpen as openExplorer,
    explorerReanchor,
    explorerClose as closeExplorer,
  } from './explorer/Bindings.js'
  import {
    currentLayout,
    currentResultID,
    currentAnchorID,
    explorerQuery,
    anchorHistory,
    pendingReanchorID,
    cameraFrame,
    layerSpacing,
    planeOpacity,
    hiddenLayers,
    hiddenMetas,
    layoutMode,
    resetExplorerState,
    editCardOpen,
    editCardTarget,
  } from '../stores/explorer.js'
  import { appendLog } from '../stores/logPanel.js'
  import { analyzeQuery, getSuggestions } from '../lib/query-grammar.js'
  import { queryHints, queryHelpOpen, ensureQueryHints } from '../stores/queryHints.js'
  import QueryAutocomplete from './QueryAutocomplete.svelte'
  import EditCard from './explorer/EditCard.svelte'
  import { resolveNodeSource } from '../lib/node-source.js'
  import { workbenchStore } from '../stores/workbench.js'
  import { ParseFileForUI, BuildGraph } from '../../wailsjs/go/main/App'

  /** Fired by parent when the user requests close (Esc / button). */
  export let onClose = null

  // Active query is read from the explorerQuery store so the launcher,
  // the omnibar, and future re-anchor sources can all drive re-renders
  // by calling explorerQuery.set(...).
  let lastRunQuery = null
  $: if (sceneCtx && $explorerQuery && $explorerQuery !== lastRunQuery) {
    lastRunQuery = $explorerQuery
    runQuery($explorerQuery)
  }

  // Always-visible query bar. queryDraft is the editable text; it mirrors
  // the explorerQuery store so external launches (MiddleBand "Open in
  // Explorer", Alt-history, etc.) keep the visible text in sync, but local
  // typing is never clobbered because we only re-sync when the store value
  // actually changes from the outside.
  let queryDraft = ''
  let lastSyncedQuery = null
  let queryBarEl
  $: if ($explorerQuery !== lastSyncedQuery) {
    lastSyncedQuery = $explorerQuery
    queryDraft = $explorerQuery
  }

  function submitQuery() {
    const q = queryDraft.trim()
    if (!q) return
    // Same text → force a refresh (the reactive block above no-ops when the
    // store value is unchanged). New text → set the store and let the block
    // run it, so there's a single execution path.
    if (q === get(explorerQuery)) runQuery(q)
    else explorerQuery.set(q)
  }

  // --- Result limit (the editable "filter" value shown in the stats panel) ---
  // The backend injects \limit DEFAULT_QUERY_LIMIT when the query has none
  // (internal/explorer/service.go withDefaultLimit). Mirror that default here
  // so the stats cell shows the effective cap, and let the user edit it to
  // re-run the query with a different limit. Keep DEFAULT_QUERY_LIMIT in sync
  // with the Go DefaultResultLimit and the OmniBar's DEFAULT_LIMIT.
  const DEFAULT_QUERY_LIMIT = 100
  // Match \limit and its backend aliases (\depth/\range/\distance all set the
  // same internal cap), so editing reflects whatever the user typed.
  const LIMIT_RE = /\\(limit|depth|range|distance)\s+(\d+)/i

  function readLimit(q) {
    const m = LIMIT_RE.exec(q || '')
    return m ? parseInt(m[2], 10) : DEFAULT_QUERY_LIMIT
  }

  function writeLimit(q, n) {
    const trimmed = (q || '').trim()
    if (LIMIT_RE.test(trimmed)) {
      // Replace the number in place, preserving whichever keyword was used.
      return trimmed.replace(LIMIT_RE, (_, kw) => `\\${kw} ${n}`)
    }
    return trimmed ? `\\limit ${n} ${trimmed}` : `\\limit ${n}`
  }

  // Effective limit reflected in the stats panel. Clamped to the explorer's
  // render ceiling (MaxNodes server-side) since a larger cap just truncates.
  const MAX_QUERY_LIMIT = 500
  $: effectiveLimit = readLimit(queryDraft)

  function applyLimit(n) {
    let v = Math.floor(Number(n))
    if (!Number.isFinite(v) || v < 1) v = 1
    if (v > MAX_QUERY_LIMIT) v = MAX_QUERY_LIMIT
    const next = writeLimit(queryDraft, v)
    queryDraft = next
    // Setting the store re-runs via the reactive block; the limit change
    // always alters the query text so it never no-ops.
    explorerQuery.set(next)
  }

  // --- Orbit radius (BFS depth of each match's neighborhood) -----------------
  // \radius N is a texere-only directive parsed in the backend's Search()
  // (internal/search/search.go extractOrbitDepth). Default and ceiling mirror
  // the Go DefaultOrbitDepth / MaxOrbitDepth. Distinct from \limit: limit caps
  // how MANY matches, radius how FAR around each one.
  const DEFAULT_QUERY_RADIUS = 2
  const MAX_QUERY_RADIUS = 3
  const RADIUS_RE = /\\radius\s+(\d+)/i

  function readRadius(q) {
    const m = RADIUS_RE.exec(q || '')
    return m ? parseInt(m[1], 10) : DEFAULT_QUERY_RADIUS
  }

  function writeRadius(q, n) {
    const trimmed = (q || '').trim()
    if (RADIUS_RE.test(trimmed)) {
      return trimmed.replace(RADIUS_RE, `\\radius ${n}`)
    }
    return trimmed ? `\\radius ${n} ${trimmed}` : `\\radius ${n}`
  }

  $: effectiveRadius = readRadius(queryDraft)

  function applyRadius(n) {
    let v = Math.floor(Number(n))
    if (!Number.isFinite(v) || v < 1) v = 1
    if (v > MAX_QUERY_RADIUS) v = MAX_QUERY_RADIUS
    const next = writeRadius(queryDraft, v)
    queryDraft = next
    explorerQuery.set(next)
  }

  // --- Query autocomplete + help (shared grammar) -----------------------
  // The dropdown suggests directives after '\' and the user's real
  // chapters/contexts/arrows after the matching directive. Keyboard nav is
  // owned here; QueryAutocomplete is presentational.
  let acItems = []
  let acIndex = 0
  let acOpen = false
  let caretPos = 0

  function recomputeAutocomplete() {
    const analysis = analyzeQuery(queryDraft, caretPos)
    acItems = getSuggestions(analysis, { liveValues: $queryHints })
    acIndex = 0
    acOpen = acItems.length > 0
  }

  function onQueryInput(e) {
    caretPos = e.target.selectionStart ?? queryDraft.length
    recomputeAutocomplete()
  }

  function applySuggestion(item) {
    const analysis = analyzeQuery(queryDraft, caretPos)
    const before = queryDraft.slice(0, analysis.replaceFrom)
    const after = queryDraft.slice(analysis.replaceTo)
    queryDraft = before + item.apply + after
    const newCaret = (before + item.apply).length
    acOpen = false
    // Restore focus + caret, then recompute (e.g. directive → value chain).
    // Defer a microtask so the bound value updates before we set the caret.
    Promise.resolve().then(() => {
      if (!queryBarEl) return
      queryBarEl.focus()
      queryBarEl.setSelectionRange(newCaret, newCaret)
      caretPos = newCaret
      recomputeAutocomplete()
    })
  }

  function onQueryKeydown(e) {
    if (acOpen && acItems.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        acIndex = (acIndex + 1) % acItems.length
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        acIndex = (acIndex - 1 + acItems.length) % acItems.length
        return
      }
      if (e.key === 'Tab' || (e.key === 'Enter' && acIndex >= 0)) {
        // Enter/Tab accepts the highlighted suggestion instead of submitting.
        e.preventDefault()
        applySuggestion(acItems[acIndex])
        return
      }
      if (e.key === 'Escape') {
        // First Escape closes the dropdown; the form/explorer Esc is untouched
        // because we stop propagation only while the dropdown is open.
        e.preventDefault()
        e.stopPropagation()
        acOpen = false
        return
      }
    }
  }

  function openQueryHelp() {
    queryHelpOpen.set(true)
  }

  /** @type {HTMLCanvasElement} */
  let canvas

  // Three.js + helpers
  let sceneCtx = null    // { scene, camera, renderer, render, dispose }
  let picker = null
  let nodeMeshes = null  // registry from NodeMeshes.js
  let edgeMeshes = null  // registry from EdgeMeshes.js
  let layerPlanes = null // registry from LayerPlanes.js (post layered redesign)
  let highlight = null   // hover-emphasis controller from Highlight.js

  // Sidebar open state (toggleable; defaults closed).
  let sidebarOpen = false

  // Tracks the layout mode the current scene was built for. Used by
  // the mode-change reactive block to detect store-vs-scene drift and
  // trigger a rebuild via performModeChange().
  let lastAppliedMode = 'extended'

  // Tween state
  let tweens = []
  let rafID = null

  // UI state
  let loading = false
  let errorMessage = ''
  let truncatedNotice = ''
  let lastDiagnostics = null  // { sourceQuery, nodesCount, edgesCount, layoutMs, graphError }

  // Hover tooltip state (also reused by US5 highlight in a later phase).
  let hoveredNodeID = null
  let hoveredFullLabel = ''
  let hoveredScreen = { x: 0, y: 0 }

  // Edge hover state — set when no node is under the cursor and the
  // edge picker finds an edge instead. Tooltip shows the arrow's long
  // name (or short code / meta if no description is available).
  let hoveredEdge = null
  let hoveredEdgeScreen = { x: 0, y: 0 }

  // Layer hover state — set when neither a node nor an edge is under
  // the cursor and the cursor IS over a layer's tinted plane. Tooltip
  // shows the layer's full Tags list (e.g. "physics, connectivity,
  // path example") so the user can read every authored context tag
  // even though only the primary one appears on the plane label.
  let hoveredLayer = null     // LayerView from $currentLayout.layers
  let hoveredLayerScreen = { x: 0, y: 0 }

  // Freeze-on-click — clicking a node pins the highlight (the
  // node + its neighbours stay bright; the rest stay dimmed) so the
  // user can move the cursor onto other nodes/edges and read their
  // tooltips without losing focus on the pinned subgraph. Clicking
  // the same node again releases the freeze; clicking a different
  // node switches the pin.
  let frozenNodeID = null
  let frozenNodeLabel = ''

  // Lazily-parsed file cache for node->source resolution. Keyed by filePath.
  // Entries are dropped after an edit so the next open re-resolves fresh lines.
  let editFileCache = {}
  // One-shot: restore the freeze pin on this node after a rebuild rebuilds the
  // scene (revealLandscape clears the live pin, so we re-apply once meshes exist).
  let pendingFreezeID = null

  async function ensureEditFileCache() {
    const summaries = get(workbenchStore).summaries ?? []
    for (const s of summaries) {
      if (!s.FilePath || editFileCache[s.FilePath] || s.ScanError) continue
      try {
        const pf = await ParseFileForUI(s.FilePath)
        if (pf) editFileCache[s.FilePath] = pf
      } catch (e) {
        console.warn('edit-card: parse failed', s.FilePath, e)
      }
    }
  }

  async function openEditCard() {
    if (frozenNodeID === null) return
    await ensureEditFileCache()
    const src = resolveNodeSource(frozenNodeID, editFileCache)
    if (!src) {
      appendLog([{ severity: 'warning', source: 'explorer',
        message: `no editable source line for "${frozenNodeLabel}"` }])
      return
    }
    editCardTarget.set({
      filePath: src.filePath, chapterTitle: src.chapterTitle, line: src.line,
      label: frozenNodeLabel, anchorId: frozenNodeID, matchCount: src.matchCount,
    })
    editCardOpen.set(true)
  }

  async function handleCardClose(e) {
    const savedAChange = e.detail?.savedAChange
    const target = get(editCardTarget)
    editCardOpen.set(false)
    editCardTarget.set(null)
    // Invalidate the edited file so a later open re-resolves against new text.
    if (target?.filePath) delete editFileCache[target.filePath]
    if (!savedAChange) return

    // Re-layout only if dirty: rebuild the index, re-run the same query, and
    // re-anchor on the pinned node when it survives the edit.
    loading = true
    try {
      await BuildGraph()
      const q = lastDiagnostics?.sourceQuery
      if (!q || q === '<reanchor>') return
      const layout = await openExplorer(q, get(layoutMode))
      const anchorId = target?.anchorId
      const stillThere = !!anchorId && (layout.nodes ?? []).some((n) => n.id === anchorId)
      if (anchorId) pendingFreezeID = anchorId
      if (stillThere && anchorId !== (layout.anchorId || null)) {
        const relayout = await explorerReanchor(layout.resultId, anchorId, get(layoutMode))
        handleLayout(relayout, q)
      } else {
        handleLayout(layout, q)
        if (!stillThere) {
          pendingFreezeID = null
          appendLog([{ severity: 'info', source: 'explorer',
            message: 'pinned node text changed — showing updated graph' }])
        }
      }
    } catch (err) {
      appendLog([{ severity: 'error', source: 'explorer',
        message: `rebuild after edit failed: ${err?.message ?? err}` }])
    } finally {
      loading = false
    }
  }

  // Reset-view tween (US7, T058). Camera-only animation back to the
  // current anchor's fit-to-result framing.
  let resetTweenRAF = null
  const RESET_DURATION_MS = 600

  // Re-anchor (US6) state
  let history = createAnchorHistory(null)
  // Mirror history snapshot into the global anchorHistory store so
  // sibling components (future Back/Forward toolbar buttons, log
  // breadcrumbs, etc.) can react to it.
  const unsubHistory = history.subscribe((snap) => anchorHistory.set(snap))
  let inFlightTween = null
  // Track resultID across queries so we know when to reset history.
  let lastResultID = null
  // Track which click is intended as a re-anchor — pointerdown captures
  // initial coords, pointerup commits the click only if no significant
  // movement happened (so dragging the camera doesn't accidentally
  // re-anchor).
  let downCoords = null
  const CLICK_DRAG_TOLERANCE_PX = 4

  // Subscribe to omnibar-driven re-anchor requests (Ctrl+Shift+K → Enter
  // on a graph result that's in the current layout, see OmniBar.svelte).
  const unsubPending = pendingReanchorID.subscribe(async (id) => {
    if (!id) return
    pendingReanchorID.set(null)
    await performReanchor(id, { source: 'omnibar' })
  })

  onMount(async () => {
    sceneCtx = createScene(canvas, {
      onCameraChange: () => {
        nodeMeshes?.rescaleLabels()
        picker?.rescaleAll()
        updateHoverScreenPos()
        publishCameraFrame()
      },
    })
    picker = createPicker(sceneCtx.camera, canvas)
    canvas.addEventListener('pointermove', handlePointerMove)
    canvas.addEventListener('pointerleave', clearHover)
    canvas.addEventListener('pointerdown', handlePointerDown)
    canvas.addEventListener('pointerup', handlePointerUp)
    sceneCtx.render()

    // Initial query: prefer whatever the launcher already pushed onto
    // the explorerQuery store; the reactive `$:` block above will pick
    // it up automatically. This onMount no longer issues its own query.
  })

  onDestroy(async () => {
    cancelAnimationFrame(rafID)
    rafID = null
    if (resetTweenRAF !== null) {
      cancelAnimationFrame(resetTweenRAF)
      resetTweenRAF = null
    }
    inFlightTween?.cancel()
    inFlightTween = null
    canvas?.removeEventListener('pointermove', handlePointerMove)
    canvas?.removeEventListener('pointerleave', clearHover)
    canvas?.removeEventListener('pointerdown', handlePointerDown)
    canvas?.removeEventListener('pointerup', handlePointerUp)
    unsubHistory?.()
    unsubPending?.()
    teardownScene()
    try {
      await closeExplorer()
    } catch (_) {
      // Non-fatal; the explorer service is idempotent on close.
    }
    resetExplorerState()
  })

  function teardownScene() {
    nodeMeshes?.dispose()
    edgeMeshes?.dispose()
    layerPlanes?.dispose()
    highlight?.clear()
    picker?.clear()
    sceneCtx?.dispose()
    sceneCtx = null
    picker = null
    nodeMeshes = null
    edgeMeshes = null
    layerPlanes = null
    highlight = null
  }

  async function runQuery(q) {
    loading = true
    errorMessage = ''
    truncatedNotice = ''
    try {
      const layout = await openExplorer(q, get(layoutMode))
      handleLayout(layout, q)
    } catch (err) {
      errorMessage = String(err?.message || err)
      // Wipe any prior scene content so the user doesn't see a stale
      // landscape behind an error overlay.
      teardownLandscapeContent()
    } finally {
      loading = false
    }
  }

  function handleLayout(layout, sourceQuery) {
    currentLayout.set(layout)
    currentResultID.set(layout.resultId || null)
    currentAnchorID.set(layout.anchorId || null)

    // T050 — a new resultID means a fresh ResultSet on the Go side
    // (Open evicts the prior cache). Anchor history is per-result, so
    // we clear it here. Re-anchoring within the same result keeps the
    // resultID stable and history persists.
    if (layout.resultId && layout.resultId !== lastResultID) {
      history.clear(layout.anchorId || null)
      lastResultID = layout.resultId
    }

    lastDiagnostics = {
      sourceQuery,
      nodesCount: layout.nodes?.length ?? 0,
      edgesCount: layout.edges?.length ?? 0,
      territoriesCount: layout.territories?.length ?? 0,
      layoutMs: layout.diagnostics?.layoutMs ?? 0,
      graphError: layout.diagnostics?.graphError ?? '',
    }
    // Surface raw response in the dev console for debugging — short
    // summary always; full payload only when zero nodes so the dev
    // tools log doesn't fill up on healthy queries.
    console.log('[explorer] layout', lastDiagnostics)
    if (lastDiagnostics.nodesCount === 0) {
      console.log('[explorer] full empty response →', layout)
    }
    // Log a breadcrumb to the LogPanel (Ctrl+Shift+M) so the user can
    // diagnose without DevTools open.
    appendLog([{
      severity: lastDiagnostics.graphError ? 'error'
        : (lastDiagnostics.nodesCount === 0 ? 'warning' : 'info'),
      source: 'explorer',
      message: `query="${sourceQuery}" → nodes=${lastDiagnostics.nodesCount} edges=${lastDiagnostics.edgesCount} territories=${lastDiagnostics.territoriesCount} layout=${lastDiagnostics.layoutMs.toFixed(2)}ms`
        + (lastDiagnostics.graphError ? ` graphError="${lastDiagnostics.graphError}"` : ''),
    }])

    if (layout.diagnostics?.graphError) {
      errorMessage = layout.diagnostics.graphError
      teardownLandscapeContent()
      return
    }
    if ((layout.nodes || []).length === 0) {
      errorMessage = 'No graph data for this query.'
      teardownLandscapeContent()
      return
    }
    if (layout.diagnostics?.truncated && layout.diagnostics.truncated > 0) {
      const total = (layout.nodes?.length || 0) + layout.diagnostics.truncated
      truncatedNotice = `Showing ${layout.nodes.length} of ${total} nodes — refine your query.`
    }

    revealLandscape(layout)
  }

  // Drop nodes / edges / ground plane and cancel pending tweens. Called
  // when a query returns empty / errors so the next render (or close)
  // doesn't show stale geometry behind the empty-state overlay.
  function teardownLandscapeContent() {
    cancelAnimationFrame(rafID)
    rafID = null
    tweens = []
    nodeMeshes?.dispose()
    edgeMeshes?.dispose()
    layerPlanes?.dispose()
    highlight?.clear()
    picker?.clear()
    nodeMeshes = null
    edgeMeshes = null
    layerPlanes = null
    highlight = null
    sceneCtx?.render()
  }

  // Skeleton-first reveal. Order is critical: the ground plane MUST be
  // visible in frame 0 (FR-032a). We add it synchronously, render, then
  // schedule heavy mesh creation in requestAnimationFrame so the user
  // sees the empty isometric world during the brief construction window.
  function revealLandscape(layout) {
    // The scene we're about to build reflects the current mode. Keep
    // lastAppliedMode in sync so the mode-change reactive block
    // doesn't re-fire after a fresh reveal.
    lastAppliedMode = get(layoutMode)
    // A fresh layout invalidates any pinned highlight — the node it
    // referenced may no longer be in the result or may have moved.
    frozenNodeID = null
    frozenNodeLabel = ''
    // 1. Replace any prior scene content (re-anchor / new query).
    nodeMeshes?.dispose()
    edgeMeshes?.dispose()
    layerPlanes?.dispose()
    highlight?.clear()
    picker?.clear()
    nodeMeshes = null
    edgeMeshes = null
    layerPlanes = null
    highlight = null

    // 2. Frame 0: layered grid floors, fit camera, render. The floors
    //    are the ONLY surfaces — no opaque ground plane underneath, so
    //    every layer reads as translucent against the workbench
    //    background.
    picker?.clearLayers()
    layerPlanes = buildLayerPlanes(sceneCtx.scene, layout, sceneCtx.camera, canvas, {
      opacity: get(planeOpacity),
      mode: get(layoutMode),
      picker,
    })
    // Apply current sidebar state immediately so re-anchors keep
    // visibility / spacing the user already configured.
    applyHiddenLayers(get(hiddenLayers))
    applySpacing(get(layerSpacing))
    fitCameraToBounds(layout.bounds)
    sceneCtx.render()

    // 3. Next frame: build node + edge meshes at opacity 0; start
    //    fade-in tweens; kick rAF.
    requestAnimationFrame(() => {
      const t0 = performance.now()
      nodeMeshes = buildNodeMeshes(sceneCtx.scene, layout, sceneCtx.camera, canvas, picker, {
        mode: get(layoutMode),
      })
      edgeMeshes = buildEdgeMeshes(sceneCtx.scene, layout, nodeMeshes, picker)
      logBridgesStat(edgeMeshes.bridgesStats)
      // Re-apply sidebar visibility against the freshly built meshes.
      applyHiddenLayers(get(hiddenLayers))
      applyHiddenMetas(get(hiddenMetas))
      highlight = createHighlight(nodeMeshes, edgeMeshes, layout, () => sceneCtx?.render())
      // Re-pin after an edit round-trip rebuilt the scene (FR: node stays pinned).
      if (pendingFreezeID && nodeMeshes?.byId(pendingFreezeID)) {
        frozenNodeID = pendingFreezeID
        frozenNodeLabel = nodeMeshes.fullLabelOf(pendingFreezeID) ?? pendingFreezeID
        highlight.setHovered(pendingFreezeID)
      }
      pendingFreezeID = null
      scheduleRevealTweens(layout, t0)
      ensureRAF()
    })
  }

  // Surface the bridge pass result on the LogPanel. Fires once per
  // edge-rebuild (initial reveal, re-anchor, mode switch, spacing
  // slider).
  function logBridgesStat(stats) {
    if (!stats) return
    const message = stats.skipped
      ? `bridges=skipped (${stats.reason ?? 'too dense'})`
      : `bridges=${stats.bridges}`
    appendLog([{ severity: 'info', source: 'explorer', message }])
  }

  // Per-node fade-in: stagger by distanceFromAnchor so the landscape
  // visually grows outward from the anchor (FR-032 user intuition cue).
  // distanceFromAnchor === -1 means "unreached" (fallback ring) — those
  // nodes appear last, after the connected portion has settled.
  function scheduleRevealTweens(layout, t0) {
    const STAGGER_MS = 45
    const FADE_MS = 200
    const UNREACHED_OFFSET_MS = 120

    // Nodes
    for (const n of layout.nodes) {
      const baseDist = n.distanceFromAnchor === -1
        ? maxFiniteDistance(layout) + 3 + UNREACHED_OFFSET_MS / STAGGER_MS
        : n.distanceFromAnchor
      const startMs = t0 + baseDist * STAGGER_MS
      const r = nodeMeshes.byId(n.id)
      if (!r) continue
      tweens.push({
        startMs, durationMs: FADE_MS, fromOpacity: 0, toOpacity: 1,
        target: r.sphere.material,
      })
      tweens.push({
        startMs, durationMs: FADE_MS, fromOpacity: 0, toOpacity: 1,
        target: r.label.material,
      })
      if (r.cycleGlyph) {
        tweens.push({
          startMs, durationMs: FADE_MS, fromOpacity: 0, toOpacity: 1,
          target: r.cycleGlyph.material,
        })
      }
    }
    // Edges fade with the LATER endpoint so we never see a floating
    // edge with one endpoint already visible (FR-032c). Fade target is
    // each part's userData.targetOpacity (0.5 for edges per the
    // refactor) so the connective tissue stays subordinate to the
    // nodes; nodes fade to 1.
    const distance = new Map(layout.nodes.map(n => [n.id, n.distanceFromAnchor]))
    for (const r of edgeMeshes.all()) {
      const ds = distance.get(r.edge.srcId) ?? 0
      const dd = distance.get(r.edge.dstId) ?? 0
      const later = Math.max(
        ds === -1 ? maxFiniteDistance(layout) + 3 : ds,
        dd === -1 ? maxFiniteDistance(layout) + 3 : dd,
      )
      const startMs = t0 + later * STAGGER_MS + 30
      for (const part of r.parts) {
        const targetOp = part.userData.targetOpacity ?? 1
        tweens.push({
          startMs, durationMs: FADE_MS, fromOpacity: 0, toOpacity: targetOp,
          target: part.material,
        })
      }
    }
  }

  function maxFiniteDistance(layout) {
    let m = 0
    for (const n of layout.nodes) {
      if (n.distanceFromAnchor > m) m = n.distanceFromAnchor
    }
    return m
  }

  // rAF loop runs only while there are pending tweens. Each tick:
  //   1. Advance every active tween's opacity.
  //   2. Drop completed tweens.
  //   3. render() once.
  // When all tweens are done the loop stops; pan / zoom continue to
  // drive renders via Scene.js's fireChange.
  function ensureRAF() {
    if (rafID !== null) return
    rafID = requestAnimationFrame(tick)
  }

  function tick(now) {
    rafID = null
    if (!sceneCtx) return // disposed mid-flight

    let active = 0
    for (const t of tweens) {
      if (t.done) continue
      const elapsed = now - t.startMs
      if (elapsed < 0) {
        active++
        continue
      }
      const u = Math.min(1, elapsed / t.durationMs)
      const eased = easeOutQuad(u)
      t.target.opacity = t.fromOpacity + (t.toOpacity - t.fromOpacity) * eased
      if (u >= 1) t.done = true
      else active++
    }
    if (tweens.some(t => t.done)) {
      tweens = tweens.filter(t => !t.done)
    }

    sceneCtx.render()

    if (active > 0) {
      rafID = requestAnimationFrame(tick)
    }
  }

  function easeOutQuad(u) {
    return 1 - (1 - u) * (1 - u)
  }

  // =============================================================================
  // Layered controls (sidebar reactive bindings)
  // =============================================================================

  // applyHiddenLayers walks every context in the current layout and
  // syncs node groups + plane visibility with the user's hidden-set.
  function applyHiddenLayers(hidden) {
    const layout = get(currentLayout)
    if (!layout?.layers) return
    for (const l of layout.layers) {
      const vis = !hidden.has(l.contextId)
      nodeMeshes?.setLayerVisible(l.contextId, vis)
      layerPlanes?.setLayerVisible(l.contextId, vis)
    }
    edgeMeshes?.refreshEndpointVisibility(
      (id) => nodeMeshes ? nodeMeshes.isVisible(id) : true,
    )
    sceneCtx?.render()
  }

  function applyHiddenMetas(hidden) {
    if (!edgeMeshes) return
    for (const meta of ['contains', 'leadsto', 'properties', 'similarto', 'unclassified']) {
      edgeMeshes.setMetaVisible(meta, !hidden.has(meta))
    }
    sceneCtx?.render()
  }

  // applySpacing repositions every node + edge to honor the current
  // layer-spacing slider. Rebuilds edges from new endpoint positions
  // since their geometry is baked at construction time.
  function applySpacing(units) {
    const layout = get(currentLayout)
    if (!layout?.layers || !nodeMeshes) return

    // contextId -> new Y
    const ctxToY = new Map()
    for (let i = 0; i < layout.layers.length; i++) {
      const l = layout.layers[i]
      const newY = i * units
      ctxToY.set(l.contextId, newY)
      l.y = newY
    }
    // Move every node onto the new Y for its context.
    for (const n of layout.nodes) {
      const r = nodeMeshes.byId(n.id)
      if (!r) continue
      const newY = ctxToY.get(n.contextId ?? '')
      if (newY === undefined) continue
      // Preserve (x, z); only Y changes.
      const x = r.sphere.position.x
      const z = r.sphere.position.z
      n.y = newY
      r.sphere.position.set(x, newY, z)
      const labelDY = r.labelOffsetY ?? 0.55
      r.label.position.set(x, newY + labelDY, z)
      r.pickMesh.position.set(x, newY, z)
      if (r.cycleGlyph) {
        r.cycleGlyph.position.set(x + (r.cycleOffsetX ?? 0), newY + (r.cycleOffsetY ?? labelDY), z)
      }
    }
    // Reposition the planes.
    layerPlanes?.setSpacing(units)
    // Rebuild edges so polylines use the new endpoint Ys.
    if (edgeMeshes) {
      edgeMeshes.dispose()
      picker?.clearEdges()
      edgeMeshes = buildEdgeMeshes(sceneCtx.scene, layout, nodeMeshes, picker)
      logBridgesStat(edgeMeshes.bridgesStats)
      edgeMeshes.setAllOpacity(1)
      applyHiddenMetas(get(hiddenMetas))
      edgeMeshes.refreshEndpointVisibility(
        (id) => nodeMeshes ? nodeMeshes.isVisible(id) : true,
      )
      // Rebuild highlight so its neighbour map references the new
      // edge records (and the legacy reference doesn't leak).
      highlight = createHighlight(nodeMeshes, edgeMeshes, layout, () => sceneCtx?.render())
    }
    sceneCtx?.render()
  }

  // Reactive bindings: sidebar slider / checkbox changes flow through
  // the store, and we react on store change. Guard with sceneCtx so
  // pre-mount values don't fire applyX against a disposed scene.
  $: if (sceneCtx && $hiddenLayers) applyHiddenLayers($hiddenLayers)
  $: if (sceneCtx && $hiddenMetas) applyHiddenMetas($hiddenMetas)
  // Spacing change is heavier (rebuilds edges) — debounce-free for v1
  // because the slider's value changes at trackball speed (≤60 Hz),
  // well within budget. If we ever see jank, throttle to rAF here.
  $: if (sceneCtx && $layerSpacing) applySpacing($layerSpacing)
  $: {
    if (sceneCtx && $planeOpacity !== undefined && layerPlanes) {
      layerPlanes.setOpacity($planeOpacity)
      sceneCtx.render()
    }
  }

  // =============================================================================
  // Hover detection — picks the topmost node under the cursor and feeds
  // the 2D HTML tooltip overlay with the full (untruncated) label. The
  // 3D sprite shows only the truncated label per NodeMeshes.js
  // (truncateLabel) so long .n4l descriptions don't clutter the canvas.
  // =============================================================================

  function handlePointerMove(e) {
    if (!picker || !nodeMeshes) return

    // Nodes always win priority — picker.pick() probes only the node
    // layer (PICK_LAYER) so the edge layer never blocks node selection.
    const id = picker.pick(e.clientX, e.clientY)
    if (id) {
      // Node wins — clear both edge AND layer tooltips so the banner
      // doesn't linger when transitioning from empty grid → node.
      if (hoveredEdge) hoveredEdge = null
      if (hoveredLayer) hoveredLayer = null
      if (id === hoveredNodeID) {
        updateHoverScreenPos()
        return
      }
      hoveredNodeID = id
      hoveredFullLabel = nodeMeshes.fullLabelOf(id) ?? ''
      updateHoverScreenPos()
      // Freeze pin overrides live hover — keep the pinned highlight
      // intact so the user can study the frozen subgraph while
      // still reading other nodes' tooltips.
      if (frozenNodeID === null) {
        highlight?.setHovered(id)
      }
      return
    }

    // No node under cursor → try the edge layer.
    if (hoveredNodeID) {
      // Releasing a node-hover into empty/edge space — drop the node
      // tooltip and (only when NOT frozen) release the highlight.
      hoveredNodeID = null
      hoveredFullLabel = ''
      if (frozenNodeID === null) {
        highlight?.setHovered(null)
      }
    }
    const edgeRef = picker.pickEdge(e.clientX, e.clientY)
    if (edgeRef) {
      hoveredEdge = edgeRef
      hoveredEdgeScreen = { x: e.clientX, y: e.clientY }
      if (hoveredLayer) hoveredLayer = null
      return
    } else if (hoveredEdge) {
      hoveredEdge = null
    }

    // No node or edge → fall back to the layer plane. Surfaces the
    // full authored tag list (e.g. "physics, connectivity, path
    // example") which the plane's single-tag label can't show on its
    // own.
    const ctxId = picker.pickLayer(e.clientX, e.clientY)
    if (ctxId !== null && ctxId !== undefined) {
      const layout = get(currentLayout)
      const layer = (layout?.layers ?? []).find(l => l.contextId === ctxId)
      if (layer) {
        hoveredLayer = layer
        hoveredLayerScreen = { x: e.clientX, y: e.clientY }
        return
      }
    }
    if (hoveredLayer) hoveredLayer = null
  }

  function clearHover() {
    hoveredNodeID = null
    hoveredFullLabel = ''
    hoveredEdge = null
    hoveredLayer = null
    // Freeze pin survives clearHover so the user can sweep the cursor
    // off-canvas without losing focus on the pinned subgraph.
    if (frozenNodeID === null) {
      highlight?.setHovered(null)
    }
  }

  function updateHoverScreenPos() {
    if (!hoveredNodeID || !nodeMeshes) return
    const screen = nodeMeshes.projectToScreen(hoveredNodeID)
    if (screen) hoveredScreen = screen
  }

  // Publish camera state to the Svelte store so overlay components
  // (MiniMap, Compass) can react without polling rAF.
  function publishCameraFrame() {
    if (!sceneCtx) return
    const c = sceneCtx.camera
    cameraFrame.set({
      x: c.position.x,
      z: c.position.z,
      zoom: c.zoom,
      frustumWidth: c.right - c.left,
      frustumHeight: c.top - c.bottom,
    })
  }

  // =============================================================================
  // Camera framing
  // =============================================================================

  // fitCameraToBounds picks an orthographic zoom that frames the entire
  // landscape. Delegates to ReanchorAnimator.computeCameraFrame so the
  // initial fit and any later re-anchor target use byte-identical math —
  // a tween from the initial frame to the same bounds would otherwise
  // produce a tiny visible "snap" at t=1.
  function fitCameraToBounds(bounds) {
    if (!sceneCtx) return
    const target = computeCameraFrame(sceneCtx.camera, bounds)
    sceneCtx.camera.position.copy(target.position)
    sceneCtx.camera.zoom = target.zoom
    sceneCtx.camera.lookAt(target.lookAt)
    sceneCtx.camera.updateProjectionMatrix()
    publishCameraFrame()
  }

  // handleMinimapPan translates the camera so its look-at point lands
  // on the dispatched world (x, z). The camera's quaternion is never
  // touched — Scene.js's pan-zoom controller and our reset/reanchor
  // tweens never call lookAt again after construction, which means
  // the camera carries a constant offset from whatever it's currently
  // looking at. We compute that offset from the current forward
  // vector and translate the camera by the delta between the
  // requested target and the implicit look-at point.
  //
  // Cancels in-flight tweens so the drag isn't fighting the
  // animator.
  function handleMinimapPan(e) {
    if (!sceneCtx) return
    inFlightTween?.cancel()
    inFlightTween = null
    if (resetTweenRAF !== null) {
      cancelAnimationFrame(resetTweenRAF)
      resetTweenRAF = null
    }

    const cam = sceneCtx.camera
    const forward = new THREE.Vector3()
    cam.getWorldDirection(forward)
    // Implicit look-at point under the current iso projection:
    // camera + forward × CAMERA_DISTANCE. We use the same constant
    // ReanchorAnimator.snapshotCamera uses so this stays in lock-step.
    const lookAtX = cam.position.x + forward.x * 30
    const lookAtZ = cam.position.z + forward.z * 30
    cam.position.x += e.detail.x - lookAtX
    cam.position.z += e.detail.z - lookAtZ
    cam.updateProjectionMatrix()
    nodeMeshes?.rescaleLabels()
    picker?.rescaleAll()
    publishCameraFrame()
    sceneCtx.render()
  }

  // =============================================================================
  // Re-anchor (US6) — click / keyboard / mouse-button paths converge
  // here. The ExplorerReanchor IPC returns a fresh LayoutResult; the
  // ReanchorAnimator interpolates positions + camera; edges are
  // disposed immediately and rebuilt from final positions on tween
  // complete (rebuilding per-frame would allocate too aggressively to
  // sustain 60fps for ≥500 edges).
  //
  //   source:
  //     'click'   — pointerup on a node (this canvas)
  //     'omnibar' — pendingReanchorID push from OmniBar.svelte
  //     'history' — Alt+ArrowLeft / Alt+ArrowRight / mouse buttons 4/5
  //                 (history mutation already done by caller; we just
  //                 follow the new current id)
  // =============================================================================

  async function performReanchor(targetID, { source } = { source: 'click' }) {
    if (!targetID) return
    const resultID = get(currentResultID)
    const layout = get(currentLayout)
    if (!resultID || !layout || !sceneCtx || !nodeMeshes) return
    if (targetID === get(currentAnchorID)) return // FR-025a no-op

    // Verify the target is in the current layout. If not, we can't
    // re-anchor — the omnibar caller is responsible for falling back
    // to ExplorerOpen with a fresh query in that case (FR-026i).
    if (!nodeMeshes.byId(targetID)) {
      appendLog([{
        severity: 'warning',
        source: 'explorer',
        message: `re-anchor target "${targetID}" not in current result; ignoring`,
      }])
      return
    }

    // History mutation:
    //   click / omnibar → push the previous anchor onto the back stack.
    //   history → already mutated by caller; nothing to push here.
    if (source === 'click' || source === 'omnibar') {
      const prev = get(currentAnchorID)
      if (prev) history.pushAnchor(targetID)
      else history.setCurrent(targetID)
    } else if (source === 'history') {
      history.setCurrent(targetID)
    }

    // Cancel any in-flight tween — its target is now stale (FR-025b).
    inFlightTween?.cancel()
    inFlightTween = null

    let nextLayout
    try {
      nextLayout = await explorerReanchor(resultID, targetID, get(layoutMode))
    } catch (err) {
      appendLog([{
        severity: 'error',
        source: 'explorer',
        message: `ExplorerReanchor failed: ${err?.message ?? err}`,
      }])
      return
    }

    if (!sceneCtx || !nodeMeshes) return // disposed mid-await
    if (nextLayout.diagnostics?.graphError) {
      // Backend signaled trouble (e.g. stale resultID after eviction).
      // Surface the same empty-state UI the runQuery path uses so the
      // user always lands on a coherent screen.
      handleLayout(nextLayout, '<reanchor>')
      return
    }

    currentLayout.set(nextLayout)
    currentAnchorID.set(nextLayout.anchorId || null)

    // Dispose old edges immediately — leaving them connected to the
    // sphere positions while the spheres tween would produce visible
    // rubber-banding for ~700ms.
    edgeMeshes?.dispose()
    edgeMeshes = null
    // Highlight references the old edges; clear it so a hover landing
    // mid-tween doesn't try to dim disposed materials. It's rebuilt
    // alongside the new edges on tween completion.
    highlight?.clear()
    highlight = null

    const prevPositions = snapshotNodePositions(nodeMeshes)
    const prevCam = snapshotCamera(sceneCtx.camera)
    const nextCam = computeCameraFrame(sceneCtx.camera, nextLayout.bounds)

    // Rebuild the layer planes around the new bounds (re-anchoring
    // usually re-centres the bounds and may add/remove layers).
    layerPlanes?.dispose()
    picker?.clearLayers()
    layerPlanes = buildLayerPlanes(sceneCtx.scene, nextLayout, sceneCtx.camera, canvas, {
      opacity: get(planeOpacity),
      mode: get(layoutMode),
      picker,
    })
    applyHiddenLayers(get(hiddenLayers))
    applySpacing(get(layerSpacing))

    inFlightTween = tweenReanchor({
      camera: sceneCtx.camera,
      render: () => {
        // During the tween: rescale labels + picking meshes whenever
        // camera.zoom moves, otherwise they'd shrink visibly.
        nodeMeshes?.rescaleLabels()
        picker?.rescaleAll()
        publishCameraFrame()
        sceneCtx?.render()
      },
      nodeMeshes,
      prevPositions,
      nextLayout,
      prevCamera: prevCam,
      nextCamera: nextCam,
      onDone: () => {
        // Rebuild edges from final positions, snap to target opacity.
        if (!sceneCtx || !nodeMeshes) return
        // Re-anchor disposes the previous edges; clear stale edge-pick
        // proxies first so the new buildEdgeMeshes can register fresh
        // ones without leaking the old set into the raycast results.
        picker?.clearEdges()
        edgeMeshes = buildEdgeMeshes(sceneCtx.scene, nextLayout, nodeMeshes, picker)
        logBridgesStat(edgeMeshes.bridgesStats)
        edgeMeshes.setAllOpacity(1) // setAllOpacity multiplies by targetOpacity (0.5)
        // Reapply sidebar filters against the new edges/nodes.
        applyHiddenLayers(get(hiddenLayers))
        applyHiddenMetas(get(hiddenMetas))
        highlight = createHighlight(nodeMeshes, edgeMeshes, nextLayout, () => sceneCtx?.render())
        sceneCtx.render()
        inFlightTween = null
      },
      onCancelled: () => {
        // A fresh re-anchor pre-empted us. Edges remain disposed; the
        // new performReanchor call will rebuild them on its own
        // onDone. No work needed here.
      },
    })
  }

  // =============================================================================
  // Layout-mode flip — Stacked ↔ Extended.
  //
  // performReanchor short-circuits when the anchor doesn't change, so
  // mode flips need their own entry point. We bypass the tween path
  // entirely because node sphere colors differ between modes (Extended
  // tints by context; Stacked stays default blue), and the existing
  // tween reuses material objects without recoloring them. A full
  // rebuild via revealLandscape is the cleanest path and runs in
  // ~one frame anyway.
  // =============================================================================

  async function performModeChange() {
    const resultID = get(currentResultID)
    const anchorID = get(currentAnchorID)
    if (!resultID || !anchorID || !sceneCtx) return

    inFlightTween?.cancel()
    inFlightTween = null

    let nextLayout
    try {
      nextLayout = await explorerReanchor(resultID, anchorID, get(layoutMode))
    } catch (err) {
      appendLog([{
        severity: 'error',
        source: 'explorer',
        message: `Layout mode change failed: ${err?.message ?? err}`,
      }])
      return
    }
    if (!sceneCtx) return // disposed mid-await
    if (nextLayout.diagnostics?.graphError) {
      handleLayout(nextLayout, '<mode-change>')
      return
    }

    currentLayout.set(nextLayout)
    currentAnchorID.set(nextLayout.anchorId || null)
    revealLandscape(nextLayout)
  }

  $: {
    if (sceneCtx && nodeMeshes && $layoutMode !== lastAppliedMode) {
      lastAppliedMode = $layoutMode
      performModeChange()
    }
  }

  // =============================================================================
  // Pointer handlers — pointerdown stashes coords; pointerup commits as
  // a click only if the cursor barely moved (so dragging the camera
  // doesn't accidentally re-anchor).
  // =============================================================================

  function handlePointerDown(e) {
    if (e.button === 0) {
      downCoords = { x: e.clientX, y: e.clientY }
      return
    }
    // Mouse buttons 4 (back) / 5 (forward) — PointerEvent.button is
    // 3 / 4 in spec parlance.
    if (e.button === 3) {
      e.preventDefault()
      historyBack()
    } else if (e.button === 4) {
      e.preventDefault()
      historyForward()
    }
  }

  function handlePointerUp(e) {
    if (e.button !== 0 || !downCoords) return
    const dx = Math.abs(e.clientX - downCoords.x)
    const dy = Math.abs(e.clientY - downCoords.y)
    const wasDrag = dx > CLICK_DRAG_TOLERANCE_PX || dy > CLICK_DRAG_TOLERANCE_PX
    downCoords = null
    if (wasDrag) return // pan, not click

    if (!picker) return
    const id = picker.pick(e.clientX, e.clientY)
    if (!id) return
    toggleFreeze(id)
  }

  // toggleFreeze pins the hover highlight on a clicked node so the
  // user can move the cursor onto other nodes/edges and read their
  // tooltips without losing focus on the pinned subgraph. Clicking
  // the same node again releases the freeze; clicking a different
  // node switches the pin.
  function toggleFreeze(nodeId) {
    if (!nodeMeshes) return
    if (frozenNodeID === nodeId) {
      // Release: drop the pin and re-apply live hover (the cursor is
      // currently over `nodeId`, so the highlight just transitions
      // from "pinned" to "live").
      frozenNodeID = null
      frozenNodeLabel = ''
      highlight?.setHovered(nodeId)
    } else {
      // Pin (or switch to the new pin).
      frozenNodeID = nodeId
      frozenNodeLabel = nodeMeshes.fullLabelOf(nodeId) ?? nodeId
      highlight?.setHovered(nodeId)
    }
  }

  function historyBack() {
    const id = history.back()
    if (id) performReanchor(id, { source: 'history' })
  }

  function historyForward() {
    const id = history.forward()
    if (id) performReanchor(id, { source: 'history' })
  }

  // =============================================================================
  // Esc + Alt+Arrow handling.
  // =============================================================================

  function handleKeydown(e) {
    // While the edit card is open it owns the keyboard (its own CodeMirror
    // handles Esc/Ctrl+S). Don't let the explorer act on any key.
    if (get(editCardOpen)) return
    // Ctrl/Cmd+Shift+K focuses the explorer's always-visible query bar.
    // The modal graph omnibar is suppressed while the explorer is open
    // (see OmniBar.svelte), so this shortcut belongs to the bar. Handle it
    // before the editable-target early-return so it works from any focus.
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault()
      e.stopPropagation()
      queryBarEl?.focus()
      queryBarEl?.select()
      return
    }

    // When the user is typing in an editable element (omnibar input,
    // sidebar slider field, etc.) the explorer must NOT intercept
    // plain-letter shortcuts like `R`. Escape stays unhandled here
    // too — the editable element owns its own escape semantics
    // (e.g. close the omnibar without closing the explorer).
    const t = e.target
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) {
      return
    }

    if (e.key === 'Enter') {
      if (frozenNodeID === null) return // nothing pinned — let Enter pass through
      e.preventDefault()
      e.stopPropagation()
      openEditCard()
      return
    }

    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      // Two-step Escape: first release the freeze pin if active,
      // otherwise close the explorer.
      if (frozenNodeID !== null) {
        toggleFreeze(frozenNodeID)
        return
      }
      onClose?.()
      return
    }
    if (e.altKey && e.key === 'ArrowLeft') {
      e.preventDefault()
      e.stopPropagation()
      historyBack()
      return
    }
    if (e.altKey && e.key === 'ArrowRight') {
      e.preventDefault()
      e.stopPropagation()
      historyForward()
      return
    }
    // Reset view to fit-to-result framing (US7, T058). Both R and Home
    // are bound so users with international keyboards have an option.
    // Modifier keys must NOT be held — Ctrl+R / Cmd+R is reload.
    if (!e.ctrlKey && !e.metaKey && !e.altKey && !e.shiftKey &&
        (e.key === 'r' || e.key === 'R' || e.key === 'Home')) {
      e.preventDefault()
      e.stopPropagation()
      resetView()
      return
    }
  }

  // resetView animates the camera to the current LayoutResult's
  // fit-to-bounds framing (FR-029). Camera-only — node positions are
  // unchanged. Reuses computeCameraFrame + snapshotCamera so the
  // target math is byte-identical to the initial fit and any re-anchor
  // tween. Cancels an in-flight reset before starting a new one.
  function resetView() {
    if (!sceneCtx) return
    const layout = get(currentLayout)
    if (!layout?.bounds) return
    if (resetTweenRAF !== null) {
      cancelAnimationFrame(resetTweenRAF)
      resetTweenRAF = null
    }
    const cam = sceneCtx.camera
    const start = snapshotCamera(cam)
    const target = computeCameraFrame(cam, layout.bounds)
    const startMs = performance.now()
    const tmpLook = new THREE.Vector3()

    function step(now) {
      if (!sceneCtx) {
        resetTweenRAF = null
        return
      }
      const u = Math.min(1, (now - startMs) / RESET_DURATION_MS)
      const e = easeOutQuad(u)
      cam.position.lerpVectors(start.position, target.position, e)
      cam.zoom = start.zoom + (target.zoom - start.zoom) * e
      tmpLook.lerpVectors(start.lookAt, target.lookAt, e)
      cam.lookAt(tmpLook)
      cam.updateProjectionMatrix()
      nodeMeshes?.rescaleLabels()
      picker?.rescaleAll()
      publishCameraFrame()
      sceneCtx.render()
      if (u < 1) {
        resetTweenRAF = requestAnimationFrame(step)
      } else {
        cam.position.copy(target.position)
        cam.zoom = target.zoom
        cam.lookAt(target.lookAt)
        cam.updateProjectionMatrix()
        publishCameraFrame()
        sceneCtx.render()
        resetTweenRAF = null
      }
    }
    resetTweenRAF = requestAnimationFrame(step)
  }
</script>

<svelte:window on:keydown|capture={handleKeydown} />

<div class="explorer-root" role="dialog" aria-label="Graph Explorer">
  <canvas bind:this={canvas} class="explorer-canvas" aria-label="Graph Explorer canvas"></canvas>

  <button
    class="explorer-close"
    on:click={() => onClose?.()}
    title="Close (Esc)"
    aria-label="Close Graph Explorer">✕</button>

  <!-- Always-visible query bar. The query result is always rendered as the
       whole graph (ExplorerOpen runs the full query and lays out the entire
       result set). Lives outside the layout-gated overlays below so it stays
       editable on empty / error / loading states. -->
  <form class="explorer-querybar" on:submit|preventDefault={submitQuery}>
    <div class="explorer-querybar-field">
      <input
        bind:this={queryBarEl}
        bind:value={queryDraft}
        type="text"
        spellcheck="false"
        autocomplete="off"
        placeholder={'e.g.  \\notes \\context "differentiate under integral"'}
        aria-label="Graph query"
        on:input={onQueryInput}
        on:keydown={onQueryKeydown}
        on:focus={ensureQueryHints}
        on:blur={() => setTimeout(() => { acOpen = false }, 120)} />
      <QueryAutocomplete
        open={acOpen}
        items={acItems}
        activeIndex={acIndex}
        on:select={(e) => applySuggestion(e.detail)}
        on:hover={(e) => acIndex = e.detail} />
    </div>
    <button type="button" class="explorer-help" title="Query syntax help" aria-label="Query syntax help" on:click={openQueryHelp}>?</button>
    <button type="submit" title="Run query (Enter)" aria-label="Run query">↵</button>
  </form>

  <!-- US7 orientation overlays. Stats / MiniMap / Sidebar suppress
       themselves when there's no layout content; the legend has its
       own toggle affordance and stays available even on empty/error
       states so the user can read up on edge styles before loading
       data. -->
  {#if !errorMessage && !loading && $currentLayout && ($currentLayout.nodes?.length ?? 0) > 0}
    <ExplorerStats
      limit={effectiveLimit} maxLimit={MAX_QUERY_LIMIT} on:applyLimit={(e) => applyLimit(e.detail)}
      radius={effectiveRadius} maxRadius={MAX_QUERY_RADIUS} on:applyRadius={(e) => applyRadius(e.detail)} />
    <MiniMap on:pan={handleMinimapPan} />
    <ExplorerSidebar bind:open={sidebarOpen} on:resetView={resetView} />
  {/if}
  <ExplorerLegend />

  {#if loading}
    <div class="overlay status" role="status">Loading…</div>
  {:else if errorMessage}
    <div class="overlay status" role="status">
      <div class="empty-card">
        <div class="empty-headline">{errorMessage}</div>
        {#if lastDiagnostics}
          <div class="empty-meta">
            <div><span class="empty-label">query</span><code>{lastDiagnostics.sourceQuery}</code></div>
            <div><span class="empty-label">nodes</span>{lastDiagnostics.nodesCount}</div>
            <div><span class="empty-label">edges</span>{lastDiagnostics.edgesCount}</div>
            <div><span class="empty-label">territories</span>{lastDiagnostics.territoriesCount}</div>
            <div><span class="empty-label">layout</span>{lastDiagnostics.layoutMs.toFixed(2)} ms</div>
            {#if lastDiagnostics.graphError}
              <div><span class="empty-label">graph</span>{lastDiagnostics.graphError}</div>
            {/if}
          </div>
        {/if}
        <div class="empty-hint">
          Press <kbd>Ctrl+Shift+K</kbd> to search the graph directly,
          or <kbd>Esc</kbd> to close.
        </div>
      </div>
    </div>
  {/if}

  {#if truncatedNotice}
    <div class="overlay banner" role="status">{truncatedNotice}</div>
  {/if}

  {#if frozenNodeID}
    <button
      class="freeze-pill"
      type="button"
      on:click={() => toggleFreeze(frozenNodeID)}
      title="Click to release the pinned highlight (or press Esc)"
      aria-label="Pinned on {frozenNodeLabel}. Click to release.">
      <svg class="freeze-icon" viewBox="0 0 12 12" width="12" height="12" aria-hidden="true">
        <circle cx="6" cy="6" r="4" fill="currentColor" />
      </svg>
      <span class="freeze-label">Pinned · {frozenNodeLabel}</span>
      <span class="freeze-hint">click to release</span>
    </button>
  {/if}

  {#if $editCardOpen && $editCardTarget}
    <EditCard target={$editCardTarget} on:close={handleCardClose} />
  {/if}

  {#if hoveredNodeID && hoveredFullLabel && hoveredFullLabel.trim().length > 0}
    <div
      class="node-tooltip"
      style="left: {hoveredScreen.x}px; top: {hoveredScreen.y}px;"
      role="tooltip"
      aria-live="polite"
    >
      {hoveredFullLabel}
    </div>
  {/if}

  {#if hoveredEdge}
    <div
      class="edge-tooltip"
      style="left: {hoveredEdgeScreen.x}px; top: {hoveredEdgeScreen.y}px;"
      role="tooltip"
      aria-live="polite"
    >
      <div class="edge-tooltip-title">
        {hoveredEdge.arrowLong || hoveredEdge.arrowCode || hoveredEdge.meta || 'edge'}{hoveredEdge.unknown ? ' (unknown)' : ''}
      </div>
      {#if hoveredEdge.unknown}
        <div class="edge-tooltip-meta">unknown arrow code — not in any SSTconfig</div>
      {:else if hoveredEdge.arrowCode && hoveredEdge.arrowLong}
        <div class="edge-tooltip-meta">{hoveredEdge.arrowCode} · {hoveredEdge.meta}</div>
      {:else if hoveredEdge.meta && hoveredEdge.meta !== hoveredEdge.arrowLong}
        <div class="edge-tooltip-meta">{hoveredEdge.meta}</div>
      {/if}
    </div>
  {/if}

  {#if hoveredLayer}
    <div
      class="layer-tooltip"
      style="left: {hoveredLayerScreen.x}px; top: {hoveredLayerScreen.y}px;"
      role="tooltip"
      aria-live="polite"
    >
      <div class="layer-tooltip-title">{hoveredLayer.label || 'Untagged'}</div>
      {#if hoveredLayer.tags && hoveredLayer.tags.length > 1}
        <div class="layer-tooltip-tags">{hoveredLayer.tags.join(', ')}</div>
      {/if}
      <div class="layer-tooltip-meta">{hoveredLayer.nodeCount} node{hoveredLayer.nodeCount === 1 ? '' : 's'}</div>
    </div>
  {/if}
</div>

<style>
  .explorer-root {
    position: fixed;
    inset: 0;
    z-index: 50;
    background: var(--bg-1, #0e0f12);
    overflow: hidden;
  }

  .explorer-canvas {
    display: block;
    width: 100%;
    height: 100%;
    user-select: none;
    cursor: grab;
  }

  .explorer-canvas:active {
    cursor: grabbing;
  }

  .explorer-close {
    position: absolute;
    top: 12px;
    right: 12px;
    width: 32px;
    height: 32px;
    background: rgba(0, 0, 0, 0.55);
    color: #e8eaf0;
    border: 1px solid #334455;
    border-radius: 16px;
    font-size: 14px;
    cursor: pointer;
    display: grid;
    place-items: center;
  }

  .explorer-close:hover {
    background: rgba(0, 0, 0, 0.8);
  }

  .explorer-querybar {
    position: absolute;
    top: 12px;
    left: 50%;
    transform: translateX(-50%);
    z-index: 10;
    display: flex;
    gap: 6px;
    width: min(680px, calc(100vw - 120px));
    padding: 6px;
    background: rgba(0, 0, 0, 0.6);
    border: 1px solid #334455;
    border-radius: 10px;
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.4);
  }

  .explorer-querybar-field {
    position: relative;
    flex: 1;
    min-width: 0;
    display: flex;
  }

  .explorer-querybar input {
    flex: 1;
    min-width: 0;
    background: rgba(12, 14, 20, 0.9);
    color: #e8eaf0;
    border: 1px solid #2a3a4a;
    border-radius: 6px;
    padding: 7px 10px;
    font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 13px;
    outline: none;
  }

  .explorer-querybar input:focus {
    border-color: #4a7bd0;
  }

  .explorer-querybar button {
    flex: 0 0 auto;
    width: 34px;
    background: rgba(0, 0, 0, 0.55);
    color: #e8eaf0;
    border: 1px solid #334455;
    border-radius: 6px;
    font-size: 14px;
    cursor: pointer;
  }

  .explorer-querybar button:hover {
    background: rgba(74, 123, 208, 0.35);
  }

  .overlay {
    position: absolute;
    pointer-events: none;
    color: var(--fg-2, #d8d8d8);
    font-size: 13px;
  }

  .overlay.status {
    inset: 0;
    display: grid;
    place-items: center;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(2px);
  }

  .empty-card {
    pointer-events: auto;
    max-width: 560px;
    padding: 20px 24px;
    background: rgba(20, 24, 32, 0.92);
    border: 1px solid #334455;
    border-radius: 8px;
    color: #e8eaf0;
    font-size: 13px;
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.4);
    text-align: left;
  }

  .empty-headline {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 12px;
  }

  .empty-meta {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 4px 12px;
    margin-bottom: 14px;
    font-family: ui-monospace, SFMono-Regular, monospace;
    font-size: 12px;
  }

  .empty-meta > div {
    display: contents;
  }

  .empty-meta code {
    overflow-wrap: anywhere;
    color: #a8c4ff;
  }

  .empty-label {
    color: #6b7c95;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-size: 10px;
    line-height: 1.7;
  }

  .empty-hint {
    color: #aab4c2;
    font-size: 12px;
    line-height: 1.5;
  }

  .empty-hint kbd {
    background: #1f2630;
    border: 1px solid #334455;
    border-radius: 3px;
    padding: 1px 5px;
    font-family: ui-monospace, monospace;
    font-size: 11px;
    color: #d8dde8;
  }

  /* 2D HTML tooltip for the hovered node's full (untruncated) text.
     Anchored to the projected screen position of the node sphere with
     a small upward + leftward offset so the tooltip floats above-left
     of the cursor rather than under it. */
  .node-tooltip {
    position: absolute;
    transform: translate(-50%, calc(-100% - 16px));
    max-width: 320px;
    padding: 6px 10px;
    background: rgba(20, 24, 32, 0.94);
    color: #e8eaf0;
    border: 1px solid #334455;
    border-radius: 6px;
    font-size: 12px;
    line-height: 1.45;
    pointer-events: none;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
    white-space: pre-wrap;
    z-index: 5;
  }

  /* Edge hover tooltip — anchored at the cursor position. Larger
     offset than the node tooltip so it doesn't sit under the cursor
     when hovering a horizontal edge. */
  .edge-tooltip {
    position: absolute;
    transform: translate(12px, -100%);
    max-width: 320px;
    padding: 6px 10px;
    background: rgba(20, 24, 32, 0.94);
    color: #e8eaf0;
    border: 1px solid #334455;
    border-radius: 6px;
    font-size: 12px;
    line-height: 1.4;
    pointer-events: none;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
    z-index: 5;
  }
  .edge-tooltip-title {
    font-weight: 600;
  }
  .edge-tooltip-meta {
    margin-top: 3px;
    font-size: 10px;
    color: #aab4c2;
    font-variant-numeric: tabular-nums;
    text-transform: lowercase;
    letter-spacing: 0.04em;
  }

  /* Layer tooltip — surfaces the full authored context tag list when
     the cursor sits on a colored plane region (and not on a node or
     edge). Title = primary tag (matches the plane's visible label);
     tags row = the full comma-joined list. */
  .layer-tooltip {
    position: absolute;
    transform: translate(12px, -100%);
    max-width: 360px;
    padding: 6px 10px;
    background: rgba(20, 24, 32, 0.94);
    color: #e8eaf0;
    border: 1px solid #334455;
    border-radius: 6px;
    font-size: 12px;
    line-height: 1.4;
    pointer-events: none;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
    z-index: 5;
  }
  .layer-tooltip-title {
    font-weight: 600;
  }
  .layer-tooltip-tags {
    margin-top: 3px;
    font-size: 11px;
    color: #d8dde8;
  }
  .layer-tooltip-meta {
    margin-top: 3px;
    font-size: 10px;
    color: #6b7c95;
    font-variant-numeric: tabular-nums;
    text-transform: lowercase;
    letter-spacing: 0.04em;
  }

  .overlay.banner {
    top: 8px;
    left: 50%;
    transform: translateX(-50%);
    background: rgba(0, 0, 0, 0.6);
    padding: 6px 10px;
    border-radius: 6px;
    font-size: 12px;
    pointer-events: auto;
  }

  /* Freeze-pin status pill — appears at the bottom-centre of the
     canvas while a click-pin is active. Clickable to release. */
  .freeze-pill {
    position: absolute;
    bottom: 16px;
    left: 50%;
    transform: translateX(-50%);
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 6px 12px;
    background: rgba(20, 24, 32, 0.92);
    color: #e8eaf0;
    border: 1px solid #6cb4ee;
    border-radius: 14px;
    font-size: 12px;
    cursor: pointer;
    pointer-events: auto;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
    z-index: 5;
  }
  .freeze-pill:hover {
    background: rgba(30, 36, 48, 0.95);
  }
  .freeze-icon {
    color: #6cb4ee;
    flex: 0 0 12px;
  }
  .freeze-label {
    font-weight: 600;
    max-width: 240px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .freeze-hint {
    color: #6b7c95;
    font-size: 10px;
    text-transform: lowercase;
    letter-spacing: 0.04em;
  }
</style>
