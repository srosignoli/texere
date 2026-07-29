<script>
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { OpenNotebook, GetNotebook, SaveNotebook, ImportImage } from '../../../wailsjs/go/main/App'
  import { newDraftCell, newLLMCell, newGraphCell, newComputeCell, newSketchCell, newImageCell, newAudioCell, newMediaCell, cellRegistry } from '../../lib/notebook.js'
  import { cellLabel } from '../../lib/cell-label.js'
  import ContextCell from './ContextCell.svelte'
  import LLMSettings from '../LLMSettings.svelte'

  export let nodeID
  export let notebookId = null      // when set, reopen this notebook read-only
  export let readOnly = false
  export let onPush = () => {}

  const dispatch = createEventDispatcher()
  let context = { extractText: '', tags: [], orbit: [], source: { kind: 'none' } }
  let cells = []
  let nbId = null
  let error = ''
  let saveTimer = null
  let showSettings = false

  async function load() {
    error = ''
    try {
      const view = notebookId ? await GetNotebook(notebookId) : await OpenNotebook(nodeID)
      nbId = view.notebookId
      context = view.context ?? context
      cells = Array.isArray(view.cells) ? view.cells : (view.cells ? JSON.parse(view.cells) : [])
      dispatch('ready', { notebookId: nbId })
    } catch (e) { error = String(e?.message ?? e) }
  }
  $: if (nodeID || notebookId) load()

  function scheduleSave() {
    if (readOnly) return
    clearTimeout(saveTimer)
    saveTimer = setTimeout(() => {
      saveTimer = null
      SaveNotebook(nbId, JSON.stringify(cells)).catch(() => {})
    }, 600)
  }
  onDestroy(() => {
    const pending = saveTimer != null
    clearTimeout(saveTimer)
    if (pending && !readOnly && nbId) {
      SaveNotebook(nbId, JSON.stringify(cells)).catch(() => {})
    }
  })

  function addDraft() { cells = [...cells, newDraftCell('')]; scheduleSave() }
  function addLLM() { cells = [...cells, newLLMCell('ask')]; scheduleSave() }
  function addGraph() { cells = [...cells, newGraphCell()]; scheduleSave() }
  function addCompute() { cells = [...cells, newComputeCell()]; scheduleSave() }
  function addSketch() { cells = [...cells, newSketchCell()]; scheduleSave() }
  async function addImage() {
    try {
      const ref = await ImportImage()
      if (!ref) return // cancelled
      cells = [...cells, newImageCell(ref.asset, ref.mime)]
      scheduleSave()
    } catch (e) { error = String(e?.message ?? e) }
  }
  function addAudio() { cells = [...cells, newAudioCell()]; scheduleSave() }
  function addMedia() { cells = [...cells, newMediaCell()]; scheduleSave() }
  function updateCell(i, text) { cells[i].text = text; scheduleSave() }
  function updateCellObj(i, obj) { cells[i] = obj; cells = cells; scheduleSave() }
  function insertAfter(i, newCell) { cells = [...cells.slice(0, i + 1), newCell, ...cells.slice(i + 1)]; scheduleSave() }
  function deleteCell(i) { cells = cells.filter((_, j) => j !== i); scheduleSave() }
  function move(i, dir) {
    const j = i + dir
    if (j < 0 || j >= cells.length) return
    const next = cells.slice(); [next[i], next[j]] = [next[j], next[i]]; cells = next; scheduleSave()
  }
</script>

<div class="nb-pane">
  <div class="nb-head">
    <span class="nb-title">Notebook</span>
    {#if readOnly}<span class="nb-ro">read-only</span>{/if}
    <button class="nb-gear" on:click={() => (showSettings = true)} title="LLM providers">⚙ Models</button>
  </div>
  {#if error}<div class="nb-error">{error}</div>{/if}
  <div class="nb-scroll">
    <ContextCell {context} />
    {#each cells as cell, i (cell.id)}
      <div class="nb-cellwrap">
        <div class="nb-cellhead">
          <span class="nb-cellname">{cellLabel(cell)}</span>
          {#if !readOnly}
            <span class="nb-headspacer"></span>
            <div class="nb-reorder">
              <button on:click={() => move(i, -1)} disabled={i === 0} title="Move up">▲</button>
              <button on:click={() => move(i, 1)} disabled={i === cells.length - 1} title="Move down">▼</button>
            </div>
            <button class="nb-del" on:click={() => deleteCell(i)} title="Delete cell">✕</button>
          {/if}
        </div>
        <svelte:component this={cellRegistry[cell.type]} {cell} {readOnly} {context} cells={cells}
          onChange={(t) => (typeof t === 'string' ? updateCell(i, t) : updateCellObj(i, t))}
          onPush={(t) => onPush(t)}
          onInsert={(text) => insertAfter(i, newDraftCell(text))} />
      </div>
    {/each}
    {#if !readOnly}
      <button class="nb-add" on:click={addDraft}>+ N4L draft cell</button>
      <button class="nb-add" on:click={addLLM}>+ LLM cell</button>
      <button class="nb-add" on:click={addGraph}>+ Graph cell</button>
      <button class="nb-add" on:click={addCompute}>+ Compute cell</button>
      <button class="nb-add" on:click={addSketch}>+ Sketch cell</button>
      <button class="nb-add" on:click={addImage}>+ Scan</button>
      <button class="nb-add" on:click={addAudio}>+ Audio cell</button>
      <button class="nb-add" on:click={addMedia}>+ Media</button>
    {/if}
  </div>
</div>
{#if showSettings}<LLMSettings onClose={() => (showSettings = false)} />{/if}

<style>
  .nb-pane { position: absolute; inset: 0; display: flex; flex-direction: column; background: #0b1220; }
  .nb-head { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #101826; }
  .nb-title { color: #7dd3fc; font-weight: 600; }
  .nb-ro { color: #b58a3a; font-size: 11px; }
  .nb-gear { margin-left: auto; background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
  .nb-error { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .nb-scroll { flex: 1; min-height: 0; overflow-y: auto; padding: 12px; }
  /* Reads as chrome, not content: the same muted, letter-spaced treatment
     AudioCell already used for its own name, dimmed further. Deliberately has
     no background — nine cell types have their own borders and radii, and a
     filled strip would have to match each one to avoid looking detached. The
     reorder and delete controls belong to this wrapper rather than to the cell,
     and sitting outside the cell's border is how that reads honestly. */
  .nb-cellhead { display: flex; align-items: center; font-size: 10px; letter-spacing: .06em; text-transform: uppercase; color: #5d7186; padding: 0 2px 3px; }
  .nb-cellname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .nb-headspacer { flex: 1; }
  .nb-reorder { display: flex; gap: 2px; }
  .nb-reorder button { background: none; border: none; color: #46586a; cursor: pointer; font-size: 9px; padding: 2px 4px; }
  .nb-reorder button:hover:not(:disabled) { color: #7dd3fc; }
  .nb-reorder button:disabled { opacity: .35; cursor: default; }
  /* The gap before ✕ is the whole of the mis-click protection: ▲▼ and delete
     are neighbours for the first time here, and deleteCell has no undo. Keep it
     visible — do not collapse it into the .nb-reorder gap. Hit areas are sized
     via padding (not just the gap) for the same reason: pen input gets no
     hover preview of the delete title to catch an aim error before it lands. */
  .nb-del { margin-left: 14px; background: none; border: none; color: #46586a; cursor: pointer; font-size: 11px; padding: 3px 6px; }
  .nb-del:hover { color: #ff8080; }
  .nb-add { background: #14233a; color: #93c5fd; border: 1px dashed #2a4a6a; border-radius: 6px; padding: 6px 12px; cursor: pointer; font-size: 12px; width: 100%; }
  .nb-add:hover { background: #1a2f4a; }
</style>
