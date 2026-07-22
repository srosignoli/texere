<script>
  import Splitter from './Splitter.svelte'
  import ProcessEditor from './ProcessEditor.svelte'
  import NotebookPane from './notebook/NotebookPane.svelte'
  import { RecordPush } from '../../wailsjs/go/main/App'

  export let extractId
  export let onAdvance = null

  let editorRef            // bound ProcessEditor instance
  let notebookId = null
  let filePath = null
  let leftWidth = 520      // px width of the left (editor) pane; user-resizable

  function onReady(e) { notebookId = e.detail.notebookId }
  function onEditorReady(e) { filePath = e.detail?.filePath ?? filePath }

  async function handlePush(text) {
    if (!editorRef) return
    const inserted = await editorRef.insertAndSave(text)
    if (notebookId && filePath && inserted) {
      try { await RecordPush(notebookId, filePath, inserted) } catch { /* provenance is best-effort */ }
    }
  }
</script>

<div class="pv-split">
  <div class="pv-pane pv-left" style="width: {leftWidth}px;">
    <ProcessEditor bind:this={editorRef} {extractId} {onAdvance} on:ready={onEditorReady} />
  </div>
  <Splitter orientation="vertical" size={leftWidth} min={340} max={1000}
            on:resize={(e) => (leftWidth = e.detail)} />
  <div class="pv-pane pv-right">
    <NotebookPane nodeID={extractId} onPush={handlePush} on:ready={onReady} />
  </div>
</div>

<style>
  .pv-split { position: absolute; inset: 0; display: flex; flex-direction: row; }
  .pv-pane { position: relative; height: 100%; }
  .pv-left { flex: 0 0 auto; }
  .pv-right { flex: 1 1 0; min-width: 0; }
</style>
