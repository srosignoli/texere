<script>
  import { onMount, onDestroy } from 'svelte'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import { GetArrowRegistry, GetInitialSummaries, GetStartupError, ConfirmQuit, ParseFileForUI } from '../wailsjs/go/main/App'
  import { workbenchStore } from './stores/workbench.js'
  import EmptyState from './components/EmptyState.svelte'
  import MiddleBand from './components/MiddleBand.svelte'
  import TopBand from './components/TopBand.svelte'
  import BottomBand from './components/BottomBand.svelte'
  import OmniBar from './components/OmniBar.svelte'

  let cancelCollectionChange = () => {}
  let cancelFileModified = () => {}
  let startupError = ''
  let bottomBandRef = null   // bind:this on BottomBand

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
      } else {
        workbenchStore.update((s) => ({ ...s, isParsing: false }))
      }
    } catch (e) {
      console.error('ParseFileForUI failed:', e)
      workbenchStore.update((s) => ({ ...s, isParsing: false }))
    }
  }

  function handleNodeSelected(e) {
    workbenchStore.update((s) => ({ ...s, selectedNodeLine: e.detail, activeTab: 'note' }))
  }

  onMount(async () => {
    // Load arrow registry once at startup
    try {
      const defs = await GetArrowRegistry()
      workbenchStore.update((s) => ({ ...s, arrowDefs: defs ?? [] }))
    } catch (e) {
      console.warn('GetArrowRegistry failed (degraded mode):', e)
    }

    // Retrieve any auto-loaded collection from startup()
    try {
      const [summaries, errMsg] = await Promise.all([
        GetInitialSummaries(),
        GetStartupError(),
      ])
      startupError = errMsg ?? ''
      if (summaries?.length) {
        // Derive collectionDir from first summary's FilePath (best available)
        const dir = summaries[0]?.FilePath
          ? summaries[0].FilePath.replace(/[\\/][^\\/]+$/, '')
          : null
        workbenchStore.update((s) => ({
          ...s,
          collectionDir: dir,
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
          summaries.push(...ev.Chapters)
        } else if (ev.Kind === 1 /* EventRemoved */) {
          const idx = summaries.findIndex((su) => su.FilePath === ev.Path)
          if (idx !== -1) summaries.splice(idx, 1)
        } else if (ev.Kind === 2 /* EventRenamed */ && ev.Chapters?.length) {
          const idx = summaries.findIndex((su) => su.FilePath === ev.Path)
          if (idx !== -1) summaries[idx] = ev.Chapters[0]
        }
        return { ...s, summaries }
      })
    })

    // Subscribe to external file modification events
    cancelFileModified = EventsOn('file:modified-externally', ({ path }) => {
      workbenchStore.update((s) => ({ ...s, externalModification: path }))
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
  })
</script>

<div class="app">
  {#if collectionOpen}
    <div class="band-top">
      <TopBand on:chapter-selected={handleChapterSelected} on:node-selected={handleNodeSelected} />
    </div>
    <div class="band-middle">
      <MiddleBand />
    </div>
    <div class="band-bottom">
      <BottomBand bind:this={bottomBandRef} />
    </div>
    <OmniBar />
  {:else}
    <EmptyState errorMessage={startupError} />
  {/if}
</div>

<style>
  :global(body) {
    margin: 0;
    background-color: #1b2636;
    color: #e8eaf0;
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
    flex: 0 0 220px;
    display: flex;
    border-bottom: 1px solid #2a3a4a;
    overflow: hidden;
  }

  .band-middle {
    flex: 0 0 44px;
    display: flex;
    align-items: center;
    padding: 0 8px;
    border-bottom: 1px solid #2a3a4a;
    background: #1a2535;
  }

  .band-bottom {
    flex: 1 1 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
</style>
