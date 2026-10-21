<script>
  import { onMount, onDestroy } from 'svelte'
  import Splitter from './Splitter.svelte'
  import ProcessEditor from './ProcessEditor.svelte'
  import NotebookPane from './notebook/NotebookPane.svelte'
  import { PeekNotebook, FileNodeSpans } from '../../wailsjs/go/main/App'
  import { EventsOn } from '../../wailsjs/runtime/runtime'
  import { createCaretNotebook } from '../lib/caret-notebook.js'

  export let extractId
  export let onAdvance = null

  let editorRef            // bound ProcessEditor instance
  let filePath = null
  let leftWidth = 520      // px width of the left (editor) pane; user-resizable

  let focusedId = extractId   // the node under the caret; starts on the queued one
  let peeked = null           // NotebookView for focusedId, or null when it has none

  // The queued extract is both the fallback (its file's spans could not be
  // read, so the pane degrades to the pre-feature behaviour instead of pinning
  // the empty state with no error shown) and our own: its Notebook is the
  // creating NotebookPane below, never a peek.
  const caretNb = createCaretNotebook({
    peek: PeekNotebook,
    fallbackId: extractId,
    ownId: extractId,
    onFocus: ({ id, notebook }) => { focusedId = id; peeked = notebook },
  })

  async function loadSpans() {
    if (!filePath) return
    let spans = []
    try { spans = await FileNodeSpans(filePath) } catch { spans = [] }
    // The editor reports its caret as soon as it is ready, which can beat this
    // fetch; setSpans resolves the last reported line again.
    caretNb.setSpans(spans)
  }

  function onCaret(e) { caretNb.caret(e.detail.line) }

  function onEditorReady(e) {
    filePath = e.detail?.filePath ?? filePath
    loadSpans()
  }

  async function handlePush(text) {
    if (!editorRef) return
    await editorRef.insertAndSave(text)
    await loadSpans()   // the insert moved every line below it
  }

  // A Notebook created on the focused node should appear now, not on the next
  // caret move.
  let cancelNotebooksChanged = null
  onMount(() => { cancelNotebooksChanged = EventsOn('notebook:changed', () => caretNb.refresh()) })
  onDestroy(() => { cancelNotebooksChanged?.(); caretNb.destroy() })
</script>

<div class="pv-split">
  <div class="pv-pane pv-left" style="width: {leftWidth}px;">
    <ProcessEditor bind:this={editorRef} {extractId} {onAdvance}
                   on:ready={onEditorReady} on:caret={onCaret} on:saved={loadSpans} />
  </div>
  <Splitter orientation="vertical" size={leftWidth} min={340} max={1000}
            on:resize={(e) => (leftWidth = e.detail)} />
  <div class="pv-pane pv-right">
    {#if focusedId !== extractId}
      <div class="pv-focus">
        Processing <b>{extractId}</b> · reading <b>{focusedId ?? 'a line with no id'}</b>
      </div>
    {/if}

    <div class="pv-notebook">
      {#key focusedId}
        {#if focusedId === extractId}
          <NotebookPane nodeID={extractId} onPush={handlePush} />
        {:else if peeked}
          <NotebookPane notebookId={peeked.notebookId} onPush={handlePush} />
        {:else}
          <div class="pv-empty">No notebook for this line.</div>
        {/if}
      {/key}
    </div>
  </div>
</div>

<style>
  .pv-split { position: absolute; inset: 0; display: flex; flex-direction: row; }
  .pv-pane { position: relative; height: 100%; }
  .pv-left { flex: 0 0 auto; }
  /* .nb-pane is position:absolute; inset:0, so the pane needs its own relatively
     positioned flex child — otherwise it covers the focus header. */
  .pv-right { flex: 1 1 0; min-width: 0; display: flex; flex-direction: column; }
  .pv-notebook { position: relative; flex: 1 1 auto; min-height: 0; }
  .pv-focus { flex: 0 0 auto; padding: 4px 8px; border-bottom: 1px solid var(--border); color: var(--text-faint); font-size: 11px; }
  .pv-focus b { color: var(--accent-text-alt); font-weight: 600; }
  .pv-empty { padding: 16px; color: var(--text-faint); font-size: 12px; }
</style>
