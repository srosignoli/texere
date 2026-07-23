<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { workbenchStore } from '../../stores/workbench.js'
  import { n4lBaseExtensions, arrowLint } from '../../lib/n4l-editor.js'

  export let cell
  export let onChange = () => {}
  export let onPush = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; present so the shared cell prop set matches
  export let cells = []       // unused

  let container, view
  let arrowDefsRef = []
  $: arrowDefsRef = $workbenchStore.arrowDefs ?? []

  onMount(() => {
    view = new EditorView({
      parent: container,
      state: EditorState.create({
        doc: cell.text ?? '',
        extensions: [
          ...n4lBaseExtensions({ getArrowDefs: () => arrowDefsRef, getRecent: () => [], onArrowSelected: () => {}, fontSize: '0.8rem' }),
          arrowLint(() => arrowDefsRef),
          EditorView.editable.of(!readOnly),
          EditorView.updateListener.of((u) => { if (u.docChanged) onChange(u.state.doc.toString()) }),
        ],
      }),
    })
  })
  onDestroy(() => view?.destroy())

  function push() { if (view) onPush(view.state.doc.toString()) }
</script>

<div class="draft-cell">
  <div class="draft-editor" bind:this={container}></div>
  {#if !readOnly}
    <div class="draft-bar">
      <button class="push-btn" on:click={push} title="Insert this N4L at the editor cursor">→ Push to source</button>
      <button class="del-btn" on:click={onDelete} title="Delete cell">✕</button>
    </div>
  {/if}
</div>

<style>
  .draft-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .draft-editor { min-height: 48px; }
  .draft-bar { display: flex; gap: 6px; padding: 6px 8px; border-top: 1px solid #1c2a3a; }
  .push-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .push-btn:hover { background: #244a72; }
  .del-btn { margin-left: auto; background: none; color: #6b7a8a; border: none; cursor: pointer; font-size: 12px; }
  .del-btn:hover { color: #ff8080; }
</style>
