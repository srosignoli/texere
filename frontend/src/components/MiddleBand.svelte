<script>
  import { createEventDispatcher } from 'svelte'
  import { OpenCollection, PickCollectionDir, CreateNewFile } from '../../wailsjs/go/main/App'
  import { workbenchStore, reminderCount, omnibarMode, editorAPI } from '../stores/workbench.js'
  import { explorerOpen, explorerQuery } from '../stores/explorer.js'
  import { studySurfaceOpen, studyFocus } from '../stores/study.js'
  import ReminderPanel from './ReminderPanel.svelte'
  import SettingsMenu from './SettingsMenu.svelte'
  import Icon from './Icon.svelte'

  const dispatch = createEventDispatcher()
  let reminderPanelOpen = false

  async function handleNewFile() {
    if (!$workbenchStore.collectionDir) return
    const raw = window.prompt('New file name (e.g. notes.n4l):')
    if (raw === null) return // cancelled
    try {
      const summary = await CreateNewFile(raw)
      dispatch('file-created', summary)
    } catch (e) {
      // Wails surfaces Go errors as plain Error objects with .message; fall
      // back to String coercion just in case.
      window.alert(`Couldn't create file: ${e?.message ?? String(e)}`)
    }
  }

  async function handleChangeCollection() {
    const dir = await PickCollectionDir()
    if (!dir) return
    try {
      const summaries = await OpenCollection(dir)
      workbenchStore.update((s) => ({
        ...s,
        collectionDir: dir,
        summaries: summaries ?? [],
        selectedChapterKey: null,
        parsedFile: null,
        activeContextFilter: new Set(),
        selectedNodeLine: null,
        isDirty: false,
      }))
    } catch (e) {
      console.error('OpenCollection failed:', e)
    }
  }

  function handleAddNode() {
    if (!$editorAPI) return
    workbenchStore.update((s) => ({ ...s, activeTab: 'note' }))
    // Append after the current line so we never split an existing node mid-word
    $editorAPI.insertAtLineEnd('\n\n')
  }

  function handleAddContext() {
    if (!$editorAPI) return
    workbenchStore.update((s) => ({ ...s, activeTab: 'note' }))
    // Append a blank context line after the current line; cursor lands after ":: "
    $editorAPI.insertAtLineEnd('\n:: ')
  }

  function handleSearch() {
    // Exactly what Ctrl+K does (OmniBar.svelte:152) — the button's title
    // advertises that shortcut, so it must not diverge from it. omnibarMode is
    // null | 'fuzzy' | 'graph', not a boolean: the previous
    // omnibarOpen.update(v => !v) threw, because omnibarOpen is a
    // subscribe-only alias with no update method, and the throw blanked the
    // whole webview.
    omnibarMode.update(m => m === 'fuzzy' ? null : 'fuzzy')
  }

  function handleUndo() {
    $editorAPI?.triggerUndo()
  }

  function handleSave() {
    $editorAPI?.triggerSave()
  }

  // Open the 2.5D Isometric Graph Explorer (feature 006). Launches with
  // \notes \chapter "<title>" so the search service returns the
  // chapter's nodes and authored arrows (FullNote → NoteBlock →
  // NoteArrow) instead of a TOC listing.
  //
  // Why \chapter and not bare \notes "<title>": SSTorytime's parser
  // strips quotes only from the \chapter argument; bare \notes leaves
  // them in, which produces params.Chapter = "\"foo bar\"" and
  // matches nothing. \notes \chapter "..." is the documented
  // canonical form for multi-word chapter titles. Single-word titles
  // could omit quotes, but always quoting keeps the launcher branch-free.
  $: explorerLaunchQuery = (() => {
    const c = $workbenchStore.selectedChapterKey
    if (!c?.chapterTitle) return ''
    return `\\notes \\chapter "${c.chapterTitle}"`
  })()

  function handleOpenExplorer() {
    if (!explorerLaunchQuery) return
    explorerQuery.set(explorerLaunchQuery)
    explorerOpen.set(true)
  }

  // Open the unified Study surface. The Read focus is where the Sources
  // panel (+ Text / + PDF / + Web import) and the reader live; the Next
  // focus serves the interleaved Read · Process · Recall queue. These
  // mirror the Ctrl+Shift+E / Ctrl+Shift+S shortcuts in App.svelte.
  function handleOpenRead() {
    studyFocus.set('read')
    studySurfaceOpen.set(true)
  }

  function handleOpenStudy() {
    studyFocus.set('next')
    studySurfaceOpen.set(true)
  }
</script>

<div class="middle-band">
  {#if $workbenchStore.activeTab === 'note'}
    <button class="btn" on:click={handleChangeCollection} title="Change collection">⟳ Collection</button>

    <div class="separator" />

    <button class="btn" on:click={handleNewFile} title="New .n4l file in this collection">+file</button>
    <button class="btn" on:click={handleAddNode} title="Insert node (+node)">+node</button>
    <button class="btn" on:click={handleAddContext} title="Insert context line (+context)">+ctx</button>
    <button class="btn" on:click={() => $editorAPI?.openArrowPicker()} title="Insert arrow (+link)">+link</button>

    <div class="separator" />

    <button class="btn" on:click={handleSave} title="Save (Ctrl+S)" aria-label="Save"><Icon name="save" /></button>
    <button class="btn" on:click={handleUndo} title="Undo">↩</button>
    <button class="btn" on:click={handleSearch} title="Search (Ctrl+K / Cmd+K)" aria-label="Search"><Icon name="search" /></button>

    <div class="separator" />

    <button
      class="btn"
      on:click={handleOpenExplorer}
      disabled={!explorerLaunchQuery}
      title={explorerLaunchQuery ? 'Open 2.5D Graph Explorer' : 'Select a chapter to enable'}
      aria-label="Open 2.5D Graph Explorer"
    ><Icon name="globe" /></button>

    <div class="separator" />

    <button class="btn" on:click={handleOpenRead} title="Read & import sources — + Text / PDF / Web (Ctrl+Shift+E)"><Icon name="book-open" /> Read</button>
    <button class="btn" on:click={handleOpenStudy} title="Study — Read · Process · Recall queue (Ctrl+Shift+S)"><Icon name="books" /> Study</button>
  {/if}

  <div class="spacer" />

  <!-- Reminder badge (T045) -->
  {#if $reminderCount > 0}
    <button
      class="btn reminder-badge"
      on:click={() => (reminderPanelOpen = !reminderPanelOpen)}
      title="Open reminders"
    >
      ⚑ {$reminderCount}
    </button>
  {/if}

  <!-- Outside the activeTab guard on purpose: settings are reachable from every
       tab, not only the note. The planned ? help menu sits next to this. -->
  <SettingsMenu />
</div>

<!-- Reminder panel overlay (T044) -->
<ReminderPanel bind:open={reminderPanelOpen} />

<style>
  .middle-band {
    display: flex;
    align-items: center;
    gap: 4px;
    width: 100%;
    height: 100%;
    padding: 0 8px;
  }

  .btn {
    padding: 4px 10px;
    background: var(--control-raised);
    color: var(--text-secondary);
    border: 1px solid var(--border-strong);
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.8rem;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .btn:hover:not(:disabled) {
    background: var(--control-raised-hover);
    color: var(--text);
  }

  .btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .separator {
    width: 1px;
    height: 20px;
    background: var(--border);
    margin: 0 4px;
    flex-shrink: 0;
  }

  .spacer {
    flex: 1;
  }

  .reminder-badge {
    background: #78350f;
    border-color: var(--warn);
    color: #fde68a;
    font-weight: 600;
  }

  .reminder-badge:hover {
    background: #92400e;
  }
</style>
