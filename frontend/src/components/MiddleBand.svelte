<script>
  import { createEventDispatcher } from 'svelte'
  import { OpenCollection, PickCollectionDir, CreateNewFile } from '../../wailsjs/go/main/App'
  import { workbenchStore, reminderCount, omnibarOpen, editorAPI } from '../stores/workbench.js'
  import ReminderPanel from './ReminderPanel.svelte'

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
    omnibarOpen.update(v => !v)
  }

  function handleUndo() {
    $editorAPI?.triggerUndo()
  }

  function handleSave() {
    $editorAPI?.triggerSave()
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

    <button class="btn" on:click={handleSearch} title="Search (Ctrl+K / Cmd+K)">🔍</button>
    <button class="btn" on:click={handleUndo} title="Undo">↩</button>
    <button class="btn" on:click={handleSave} title="Save (Ctrl+S)">💾</button>
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
    background: #263547;
    color: #c8d0dc;
    border: 1px solid #334455;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.8rem;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .btn:hover {
    background: #2e4060;
    color: #e8eaf0;
  }

  .separator {
    width: 1px;
    height: 20px;
    background: #2a3a4a;
    margin: 0 4px;
    flex-shrink: 0;
  }

  .spacer {
    flex: 1;
  }

  .reminder-badge {
    background: #78350f;
    border-color: #f59e0b;
    color: #fde68a;
    font-weight: 600;
  }

  .reminder-badge:hover {
    background: #92400e;
  }
</style>
