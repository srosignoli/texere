<script>
  import { onMount, onDestroy } from 'svelte'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import { GetArrowRegistry, GetInitialSummaries, GetInitialCollectionDir, GetStartupError, ConfirmQuit, ParseFileForUI, BuildGraph } from '../wailsjs/go/main/App'
  import { workbenchStore } from './stores/workbench.js'
  import { uiLayout } from './stores/uiLayout.js'
  import { logPanelOpen, appendLog, toggleLogPanel } from './stores/logPanel.js'
  import EmptyState from './components/EmptyState.svelte'
  import MiddleBand from './components/MiddleBand.svelte'
  import TopBand from './components/TopBand.svelte'
  import BottomBand from './components/BottomBand.svelte'
  import OmniBar from './components/OmniBar.svelte'
  import Splitter from './components/Splitter.svelte'
  import LogPanel from './components/LogPanel.svelte'
  import GraphExplorer from './components/GraphExplorer.svelte'
  import QueryHelpPanel from './components/QueryHelpPanel.svelte'
  import StudyMode from './components/StudyMode.svelte'
  import StudySurface from './components/StudySurface.svelte'
  import { explorerOpen } from './stores/explorer.js'
  import { refreshQueryHints } from './stores/queryHints.js'
  import { studyOpen, studySurfaceOpen, studyFocus } from './stores/study.js'
  import { activeReading } from './stores/reading.js'

  // Track viewport height so the top-band drag handle can clamp to a value
  // that always leaves the editor with at least EDITOR_FLOOR_PX of room.
  const TOOLBAR_PX = 44
  const HANDLE_PX = 5
  const EDITOR_FLOOR_PX = 200
  let windowHeight = typeof window !== 'undefined' ? window.innerHeight : 800
  function onResize() { windowHeight = window.innerHeight }
  $: maxTopBandHeight = Math.max(80, windowHeight - TOOLBAR_PX - HANDLE_PX - EDITOR_FLOOR_PX)

  function setTopBandHeight(e) {
    uiLayout.update(s => ({ ...s, topBandHeight: e.detail }))
  }

  let cancelCollectionChange = () => {}
  let cancelFileModified = () => {}
  let cancelIdsInjected = () => {}
  let startupError = ''
  let bottomBandRef = null   // bind:this on BottomBand

  // Graph build state
  let graphBuildState = /** @type {null|'building'|'done'|'error'} */ (null)
  let graphBuildMsg = ''

  async function triggerBuildGraph() {
    if (graphBuildState === 'building') return
    graphBuildState = 'building'
    graphBuildMsg = 'Building graph…'
    try {
      const r = await BuildGraph()
      if (r.error) {
        graphBuildState = 'error'
        graphBuildMsg = `Graph error: ${r.error}`
        appendLog([{ severity: 'error', source: 'build', message: r.error }])
      } else {
        graphBuildState = 'done'
        graphBuildMsg = `Graph ready — ${r.nodesIngested} nodes, ${r.linksIngested} links (${r.durationMs}ms)`
        workbenchStore.update((s) => ({ ...s, graphVersion: (s.graphVersion ?? 0) + 1 }))
        // Refresh query-bar autocomplete hints (chapters/contexts/arrows may
        // have changed with the rebuilt graph).
        refreshQueryHints()
        // Hoisted parse errors from every file in the collection.
        if (r.parseErrors?.length) {
          appendLog(r.parseErrors.map((pe) => ({
            severity: 'warning',
            source: 'parse',
            filePath: pe.File,
            fileName: pe.File ? pe.File.split(/[\\/]/).pop() : undefined,
            line: pe.Line || undefined,
            message: pe.Msg,
          })))
        }
        // Aggregated unknown-arrow summary — one entry per distinct code.
        if (r.unknownArrows) {
          const summary = Object.entries(r.unknownArrows)
          if (summary.length) {
            appendLog(summary.map(([code, count]) => ({
              severity: 'warning',
              source: 'build',
              message: `Unknown arrow code "${code}" — ${count} occurrence${count === 1 ? '' : 's'}. Register in SSTconfig to materialise links.`,
            })))
          }
        }
        if (r.errors?.length) {
          appendLog(r.errors.map((m) => ({ severity: 'error', source: 'build', message: m })))
        }
      }
    } catch (e) {
      graphBuildState = 'error'
      graphBuildMsg = `Graph build failed: ${e}`
      appendLog([{ severity: 'error', source: 'build', message: String(e?.message ?? e) }])
    }
    setTimeout(() => { graphBuildState = null }, 5000)
  }

  function handleKeydown(e) {
    // stopPropagation alongside preventDefault on every match — registered
    // in capture phase, so stopping propagation here prevents the event
    // from continuing down to CodeMirror's defaultKeymap (Ctrl+G →
    // findNext, Ctrl+Shift+K → deleteLine, etc.). Without it, the editor
    // would consume the key first, dirty the file, and only then hand
    // control back to us.
    if (e.key === 'g' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      e.stopPropagation()
      triggerBuildGraph()
    }
    // Ctrl/Cmd+Shift+M toggles the log panel — VS Code's Problems-panel convention.
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'M' || e.key === 'm')) {
      e.preventDefault()
      e.stopPropagation()
      toggleLogPanel()
    }
    // Ctrl/Cmd+Shift+R toggles the Recall (Study) overlay.
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'r' || e.key === 'R')) {
      e.preventDefault()
      e.stopPropagation()
      studyOpen.update((v) => !v)
      return
    }
    // Ctrl/Cmd+Shift+S toggles the unified Study surface (Next queue).
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 's' || e.key === 'S')) {
      e.preventDefault(); e.stopPropagation()
      // Toggling the surface closed must drop the active reading too (mirrors the
      // × onClose); otherwise reopening remounts on a stale reading object.
      if ($studySurfaceOpen) activeReading.set(null)
      studyFocus.set('next'); studySurfaceOpen.update((v) => !v); return
    }
    // Ctrl/Cmd+Shift+E opens the Study surface on the Read focus.
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'e' || e.key === 'E')) {
      e.preventDefault(); e.stopPropagation()
      studyFocus.set('read'); studySurfaceOpen.set(true); return
    }
    // Esc closes the log panel when it's open (and the omnibar isn't already eating Esc).
    if (e.key === 'Escape' && $logPanelOpen) {
      e.preventDefault()
      e.stopPropagation()
      logPanelOpen.set(false)
    }
  }

  // Reactive: show workbench only when a collection is open
  $: collectionOpen = $workbenchStore.collectionDir !== null

  async function handleChapterSelected(e) {
    const { filePath, chapterTitle } = e.detail
    const store = $workbenchStore

    // Guard: unsaved changes in current chapter
    if (store.isDirty) {
      if (!confirm('Unsaved changes — switch anyway?')) return
    }

    workbenchStore.update((s) => ({
      ...s,
      selectedChapterKey: { filePath, chapterTitle },
      activeContextFilter: new Set(),
      selectedNodeLine: null,
      isDirty: false,
      activeTab: 'note',
      isParsing: true,
    }))

    try {
      const result = await ParseFileForUI(filePath)
      if (result && !Array.isArray(result)) {
        const ch = result.Chapters?.find((c) => c.Title === chapterTitle)
        workbenchStore.update((s) => ({
          ...s,
          parsedFile: result,
          pendingScrollLine: ch?.LineStart ?? null,
          isParsing: false,
        }))
        // Surface per-file parse errors (unknown arrows, malformed syntax,
        // …) into the log panel so the user can see them without rebuilding.
        if (result.Errors?.length) {
          const fileName = filePath.split(/[\\/]/).pop()
          appendLog(result.Errors.map((pe) => ({
            severity: 'warning',
            source: 'parse',
            filePath: pe.File ?? filePath,
            fileName,
            line: pe.Line || undefined,
            message: pe.Msg,
          })))
        }
      } else {
        workbenchStore.update((s) => ({ ...s, isParsing: false }))
      }
    } catch (e) {
      console.error('ParseFileForUI failed:', e)
      workbenchStore.update((s) => ({ ...s, isParsing: false }))
      appendLog([{ severity: 'error', source: 'parse', filePath, message: String(e?.message ?? e) }])
    }
  }

  // Log-panel row click → open the file at the offending line. Reuses the
  // chapter-selected pipeline so the same context-filter reset / tab switch
  // semantics apply, then nudges the editor to the specific line via the
  // existing pendingScrollLine + editorAPI hooks.
  async function handleLogEntryClick(e) {
    const entry = e.detail
    if (!entry?.filePath) return
    // Find any chapter whose LineStart precedes the error line so the chapter
    // pipeline opens the right tab; default to '' (whole-file open) when the
    // file's chapters can't be inspected yet.
    let targetChapter = ''
    const summaries = $workbenchStore.summaries ?? []
    const sum = summaries.find((s) => s.FilePath === entry.filePath)
    if (sum?.Chapters?.length) {
      // ChapterSummary only has titles, no line ranges — pick the first as a
      // safe fallback. The handleChapterSelected path will load and scroll
      // via pendingScrollLine below.
      targetChapter = sum.Chapters[0]
    }
    await handleChapterSelected({ detail: { filePath: entry.filePath, chapterTitle: targetChapter } })
    // The chapter pipeline sets pendingScrollLine to the chapter's LineStart;
    // override with the error's actual line so the editor lands on the right
    // row. SourceTab consumes pendingScrollLine via its existing reactive
    // hook (no new editor API needed).
    if (entry.line) {
      workbenchStore.update((s) => ({ ...s, pendingScrollLine: entry.line }))
    }
  }

  function handleNodeSelected(e) {
    workbenchStore.update((s) => ({ ...s, selectedNodeLine: e.detail, activeTab: 'note' }))
  }

  // Triggered when MiddleBand creates a brand-new .n4l file. Optimistically
  // appends the summary so the file appears in the tree before the watcher
  // event arrives (the watcher path is now FilePath-deduped, so the eventual
  // EventAdded is a no-op). Then opens the empty file via the existing
  // chapter-selected pipeline; chapterTitle is "" because a fresh file has
  // no chapters yet.
  function handleFileCreated(e) {
    const summary = e.detail
    if (!summary?.FilePath) return
    workbenchStore.update((s) => {
      const summaries = s.summaries ?? []
      if (summaries.some((x) => x.FilePath === summary.FilePath)) return s
      return { ...s, summaries: [...summaries, summary] }
    })
    handleChapterSelected({ detail: { filePath: summary.FilePath, chapterTitle: '' } })
  }

  onMount(async () => {
    // Capture phase: global shortcuts (Ctrl+Shift+K, Ctrl+K, Ctrl+G,
    // Ctrl+Shift+M, Esc) must fire BEFORE CodeMirror's defaultKeymap, which
    // binds Ctrl+Shift+K to deleteLine and Ctrl+G to findNext. Without
    // capture, the editor would consume the key, dirty the file, and only
    // then bubble the event up to us.
    window.addEventListener('keydown', handleKeydown, true)
    window.addEventListener('resize', onResize)
    // Load arrow registry once at startup
    try {
      const defs = await GetArrowRegistry()
      workbenchStore.update((s) => ({ ...s, arrowDefs: defs ?? [] }))
    } catch (e) {
      console.warn('GetArrowRegistry failed (degraded mode):', e)
    }

    // Retrieve any auto-loaded collection from startup()
    try {
      const [summaries, errMsg, dir] = await Promise.all([
        GetInitialSummaries(),
        GetStartupError(),
        GetInitialCollectionDir(),
      ])
      startupError = errMsg ?? ''
      if (summaries?.length) {
        // Use the backend's real collection root. Deriving it from a summary's
        // FilePath is wrong now that ScanCollection recurses — the first summary
        // (sorted by relative path) may sit in a subdirectory.
        workbenchStore.update((s) => ({
          ...s,
          collectionDir: dir || null,
          summaries: summaries,
        }))
      }
    } catch (e) {
      console.warn('Startup load failed:', e)
    }

    // Subscribe to collection filesystem events
    cancelCollectionChange = EventsOn('collection:change', (ev) => {
      workbenchStore.update((s) => {
        const summaries = [...(s.summaries ?? [])]
        if (ev.Kind === 0 /* EventAdded */ && ev.Chapters?.length) {
          // Upsert by FilePath. atomicWriteText (used by SaveFile and
          // CreateNewFile) writes via temp-then-rename, which fsnotify
          // reports as Create on the destination — even when the file
          // already existed. We therefore must replace any existing
          // summary at this path so chapter add/remove/rename via save
          // is reflected in the tree.
          for (const sum of ev.Chapters) {
            const idx = summaries.findIndex((x) => x.FilePath === sum.FilePath)
            if (idx === -1) summaries.push(sum)
            else summaries[idx] = sum
          }
        } else if (ev.Kind === 1 /* EventRemoved */) {
          const idx = summaries.findIndex((su) => su.FilePath === ev.Path)
          if (idx !== -1) summaries.splice(idx, 1)
        } else if (ev.Kind === 2 /* EventRenamed */ && ev.Chapters?.length) {
          const idx = summaries.findIndex((su) => su.FilePath === ev.Path)
          if (idx !== -1) summaries[idx] = ev.Chapters[0]
        } else if (ev.Kind === 3 /* EventModified */ && ev.Chapters?.length) {
          // Replace the summary in place when a file's chapter set
          // changes (chapters added / removed / renamed via save).
          const idx = summaries.findIndex((su) => su.FilePath === ev.Path)
          if (idx !== -1) summaries[idx] = ev.Chapters[0]
          else summaries.push(ev.Chapters[0])
        }
        return { ...s, summaries }
      })
    })

    // Subscribe to external file modification events
    cancelFileModified = EventsOn('file:modified-externally', ({ path }) => {
      workbenchStore.update((s) => ({ ...s, externalModification: path }))
    })

    // Subscribe to # @id injection events from Build Graph / SyncStudyItems.
    // When the open file had IDs injected by the backend, either silently
    // reload (buffer clean) or fall back to the external-modification banner
    // (buffer dirty — must not clobber unsaved edits).
    cancelIdsInjected = EventsOn('file:ids-injected', ({ path }) => {
      const store = $workbenchStore
      const openPath = store.selectedChapterKey?.filePath ?? null
      if (path !== openPath) return  // not the open file; nothing to do
      if (!store.isDirty) {
        // Buffer is clean: silently reload from disk so the injected # @id
        // lines land in the editor without any user-visible prompt.
        workbenchStore.update((s) => ({ ...s, silentReloadNonce: (s.silentReloadNonce ?? 0) + 1 }))
      } else {
        // Buffer is dirty: fall back to the existing external-modification
        // banner so the user decides whether to reload or keep their edits.
        workbenchStore.update((s) => ({ ...s, externalModification: path }))
      }
    })

    // App close guard: check isDirty before allowing quit (T038)
    EventsOn('app:close-requested', () => {
      const store = $workbenchStore
      if (store.isDirty) {
        if (!confirm('You have unsaved changes. Close anyway?')) return
      }
      ConfirmQuit()
    })
  })

  onDestroy(() => {
    cancelCollectionChange()
    cancelFileModified()
    cancelIdsInjected()
    window.removeEventListener('keydown', handleKeydown, true)
    window.removeEventListener('resize', onResize)
  })
</script>

<div class="app">
  {#if graphBuildState}
    <div class="graph-banner graph-banner--{graphBuildState}">{graphBuildMsg}</div>
  {/if}
  {#if collectionOpen}
    <div class="band-top" style="height: {$uiLayout.topBandHeight}px;">
      <TopBand on:chapter-selected={handleChapterSelected} on:node-selected={handleNodeSelected} />
    </div>
    <Splitter
      orientation="horizontal"
      size={$uiLayout.topBandHeight}
      min={80}
      max={maxTopBandHeight}
      on:resize={setTopBandHeight}
    />
    <div class="band-middle">
      <MiddleBand on:file-created={handleFileCreated} />
    </div>
    <div class="band-bottom">
      <div class="editor-region">
        <BottomBand bind:this={bottomBandRef} />
      </div>
      {#if $logPanelOpen}
        <Splitter
          orientation="horizontal"
          size={$uiLayout.logPanelHeight}
          min={120}
          max={Math.max(180, windowHeight * 0.6)}
          on:resize={(e) => uiLayout.update(s => ({ ...s, logPanelHeight: e.detail }))}
        />
        <div class="log-region" style="height: {$uiLayout.logPanelHeight}px;">
          <LogPanel on:entry-click={handleLogEntryClick} />
        </div>
      {/if}
    </div>
    <OmniBar />
    {#if $explorerOpen}
      <GraphExplorer onClose={() => explorerOpen.set(false)} />
    {/if}
    {#if $studyOpen}
      <StudyMode onClose={() => studyOpen.set(false)} />
    {/if}
    {#if $studySurfaceOpen}
      <StudySurface onClose={() => { studySurfaceOpen.set(false); activeReading.set(null) }} />
    {/if}
    <QueryHelpPanel />
  {:else}
    <EmptyState errorMessage={startupError} />
  {/if}
</div>

<style>
  :global(body) {
    margin: 0;
    background-color: var(--surface);
    color: var(--text);
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto,
                 'Noto Sans', 'Noto Sans CJK SC', 'Noto Sans CJK TC',
                 'Source Han Sans CN', 'Microsoft YaHei', 'SimHei',
                 sans-serif;
    height: 100vh;
    overflow: hidden;
  }

  .app {
    display: flex;
    flex-direction: column;
    height: 100vh;
  }

  .band-top {
    /* height is set inline from the uiLayout store; keep flex-shrink:0 so the
       splitter below is what controls vertical space, not flex pressure. */
    flex: 0 0 auto;
    display: flex;
    border-bottom: 1px solid var(--border);
    overflow: hidden;
  }

  .band-middle {
    flex: 0 0 44px;
    display: flex;
    align-items: center;
    padding: 0 8px;
    border-bottom: 1px solid var(--border);
    background: var(--surface-overlay);
  }

  .band-bottom {
    flex: 1 1 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .editor-region {
    flex: 1 1 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .log-region {
    flex: 0 0 auto;
    min-height: 0;
    overflow: hidden;
    border-top: 1px solid var(--border);
  }

  .graph-banner {
    position: fixed;
    top: 0;
    left: 50%;
    transform: translateX(-50%);
    z-index: 9999;
    padding: 6px 18px;
    border-radius: 0 0 6px 6px;
    font-size: 12px;
    font-weight: 500;
    pointer-events: none;
  }
  .graph-banner--building {
    background: #2a4a6a;
    color: #7ecfff;
    border: 1px solid #3a6a9a;
  }
  .graph-banner--done {
    background: #1a4a2a;
    color: #7ecfa0;
    border: 1px solid #2a7a3a;
  }
  .graph-banner--error {
    background: #4a1a1a;
    color: #ff8080;
    border: 1px solid #7a2a2a;
  }
</style>
