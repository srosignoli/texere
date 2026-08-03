<script>
  import Splitter from './Splitter.svelte'
  import ProcessEditor from './ProcessEditor.svelte'
  import NotebookPane from './notebook/NotebookPane.svelte'
  import { RecordPush, PeekNotebook, FileNodeSpans } from '../../wailsjs/go/main/App'
  import { nodeIdAtLine } from '../lib/node-span.js'

  export let extractId
  export let onAdvance = null

  let editorRef            // bound ProcessEditor instance
  let notebookId = null
  let filePath = null
  let leftWidth = 520      // px width of the left (editor) pane; user-resizable

  let spans = []
  let focusedId = extractId   // the node under the caret; starts on the queued one
  let peeked = null           // NotebookView for focusedId, or null when it has none
  let peekedFor = null        // which id `peeked` describes, so a stale reply is ignored
  let caretLine = 0           // last reported caret line, kept so spans arriving later can re-resolve

  // Ids are fully qualified (`<path>:<chapter>:id:<short>`); the path is noise in
  // a one-line header, so show only the short id the user actually recognises.
  const shortId = (id) => (id ? id.slice(id.lastIndexOf(':') + 1) : null)

  async function loadSpans() {
    if (!filePath) return
    try { spans = await FileNodeSpans(filePath) } catch { spans = [] }
    // The editor reports its caret as soon as it is ready, which can beat this
    // fetch — without re-resolving here, opening Process mode would land on the
    // empty state because the map was still [] when the first report arrived.
    if (caretLine) await resolve(caretLine)
  }

  async function onCaret(e) { await resolve(e.detail.line) }

  async function resolve(line) {
    caretLine = line
    const id = nodeIdAtLine(spans, line)
    if (id === focusedId) return
    focusedId = id
    // The queued extract keeps its creating path; nothing else is peeked until
    // it is actually focused.
    if (!id || id === extractId) { peeked = null; peekedFor = id; return }
    try {
      const view = await PeekNotebook(id)
      // A slower reply for a node we have already left must not win.
      if (focusedId !== id) return
      peeked = view
      peekedFor = id
    } catch { peeked = null; peekedFor = id }
  }

  function onReady(e) { notebookId = e.detail.notebookId }
  // Cleared whenever the empty state renders. Nothing can push from it today, so
  // a stale id is unreachable rather than harmless — clear it anyway, because the
  // next person to add an affordance there will not know it was load-bearing.
  $: if (focusedId !== extractId && !(peeked && peekedFor === focusedId)) notebookId = null

  function onEditorReady(e) {
    filePath = e.detail?.filePath ?? filePath
    loadSpans()
  }

  async function handlePush(text) {
    if (!editorRef) return
    const inserted = await editorRef.insertAndSave(text)
    if (notebookId && filePath && inserted) {
      try { await RecordPush(notebookId, filePath, inserted) } catch { /* provenance is best-effort */ }
    }
    await loadSpans()   // the insert moved every line below it
  }
</script>

<div class="pv-split">
  <div class="pv-pane pv-left" style="width: {leftWidth}px;">
    <ProcessEditor bind:this={editorRef} {extractId} {onAdvance}
                   on:ready={onEditorReady} on:caret={onCaret} />
  </div>
  <Splitter orientation="vertical" size={leftWidth} min={340} max={1000}
            on:resize={(e) => (leftWidth = e.detail)} />
  <div class="pv-pane pv-right">
    {#if focusedId !== extractId}
      <div class="pv-focus">
        Processing <b>{shortId(extractId)}</b> · reading <b>{shortId(focusedId) ?? 'a line with no note'}</b>
      </div>
    {/if}

    <div class="pv-notebook">
      {#key focusedId}
        {#if focusedId === extractId}
          <NotebookPane nodeID={extractId} onPush={handlePush} on:ready={onReady} />
        {:else if peeked && peekedFor === focusedId}
          <NotebookPane notebookId={peeked.notebookId} onPush={handlePush} on:ready={onReady} />
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
  .pv-focus { flex: 0 0 auto; padding: 4px 8px; border-bottom: 1px solid #2a3a4a; color: #5d7186; font-size: 11px; }
  .pv-focus b { color: #93c5fd; font-weight: 600; }
  .pv-empty { padding: 16px; color: #5d7186; font-size: 12px; }
</style>
