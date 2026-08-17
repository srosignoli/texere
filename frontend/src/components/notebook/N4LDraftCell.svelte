<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { workbenchStore } from '../../stores/workbench.js'
  import { n4lBaseExtensions, arrowLint } from '../../lib/n4l-editor.js'

  export let cell
  export let onChange = () => {}
  export let onPush = () => {}
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
    </div>
  {/if}
</div>

<style>
  .draft-cell { background: var(--surface-modal); border: 1px solid var(--border-card); border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .draft-editor { min-height: 48px; }
  .draft-bar { display: flex; gap: 6px; padding: 6px 8px; border-top: 1px solid var(--border-subtle); }
  .push-btn { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .push-btn:hover { background: var(--accent-hover); }
</style>
