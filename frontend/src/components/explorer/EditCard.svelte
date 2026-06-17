<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import { get } from 'svelte/store'
  import { EditorView, keymap, lineNumbers, drawSelection, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { defaultKeymap, historyKeymap, history } from '@codemirror/commands'
  import { autocompletion } from '@codemirror/autocomplete'
  import { workbenchStore } from '../../stores/workbench.js'
  import { n4lLanguage } from '../../lib/n4l-language.js'
  import { n4lHighlighting } from '../../lib/n4l-highlight.js'
  import { arrowCompletion } from '../../lib/arrow-autocomplete.js'
  import { GetFileContent, SaveFile, GetRecentArrows, UpdateRecentArrows } from '../../../wailsjs/go/main/App'

  /** @type {{ filePath: string, chapterTitle: string, line: number, label: string, matchCount: number }} */
  export let target

  const dispatch = createEventDispatcher()

  let container
  let view
  let initialContent = ''
  let savedAChange = false
  let arrowDefsRef = []
  let recentCodesRef = []

  $: arrowDefsRef = $workbenchStore.arrowDefs ?? []

  const fileName = (target?.filePath ?? '').replace(/^.*[\\/]/, '')

  function handleArrowSelected(code) {
    recentCodesRef = [code, ...recentCodesRef.filter((c) => c !== code)].slice(0, 10)
    UpdateRecentArrows(recentCodesRef).catch((e) => console.warn('UpdateRecentArrows failed:', e))
  }

  function buildExtensions() {
    return [
      lineNumbers(),
      drawSelection(),
      highlightActiveLine(),
      highlightActiveLineGutter(),
      EditorView.lineWrapping,
      history(),
      n4lLanguage,
      n4lHighlighting,
      autocompletion({
        override: [(ctx) => arrowCompletion(arrowDefsRef, recentCodesRef, handleArrowSelected)(ctx)],
      }),
      keymap.of([
        { key: 'Mod-s', run: () => { handleSave(); return true }, preventDefault: true },
        { key: 'Escape', run: () => { requestClose(); return true } },
        ...defaultKeymap,
        ...historyKeymap,
      ]),
      EditorView.theme({
        '&': { height: '100%', background: '#1b2636' },
        '.cm-content': { fontFamily: 'monospace', fontSize: '0.88rem', color: '#e8eaf0', textAlign: 'left' },
        '.cm-gutters': { background: '#161f2e', borderRight: '1px solid #2a3a4a', color: '#4a5a6a' },
        '.cm-cursor': { borderLeftColor: '#7dd3fc' },
        '.cm-activeLine': { background: '#1d2e42' },
        '.cm-activeLineGutter': { background: '#1d2e42' },
        '.cm-tooltip.cm-tooltip-autocomplete': { background: '#1a2535', border: '1px solid #2a3a4a', borderRadius: '6px', boxShadow: '0 6px 24px rgba(0,0,0,0.55)' },
      }),
    ]
  }

  function placeCursorOnLine(lineNumber) {
    if (!view || !lineNumber) return
    const doc = view.state.doc
    if (lineNumber < 1 || lineNumber > doc.lines) { view.focus(); return }
    const line = doc.line(lineNumber)
    view.dispatch({
      selection: { anchor: line.from },
      effects: EditorView.scrollIntoView(line.from, { y: 'center' }),
    })
    view.focus()
  }

  async function handleSave() {
    if (!view || !target?.filePath) return
    const content = view.state.doc.toString()
    if (content === initialContent) return // nothing changed; keep savedAChange false
    try {
      const result = await SaveFile(target.filePath, content)
      const finalContent = result?.finalContent ?? content
      if (finalContent !== content) {
        view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: finalContent } })
      }
      initialContent = finalContent
      savedAChange = true
    } catch (e) {
      console.error('EditCard SaveFile failed:', e)
    }
  }

  function requestClose() {
    dispatch('close', { savedAChange })
  }

  onMount(async () => {
    try {
      const saved = await GetRecentArrows()
      if (saved?.length) recentCodesRef = saved
    } catch (e) {
      console.warn('GetRecentArrows failed:', e)
    }
    let text = ''
    try {
      text = (await GetFileContent(target.filePath)) ?? ''
    } catch (e) {
      console.error('EditCard GetFileContent failed:', e)
    }
    initialContent = text
    const state = EditorState.create({ doc: text, extensions: buildExtensions() })
    view = new EditorView({ state, parent: container })
    placeCursorOnLine(target.line)
  })

  onDestroy(() => { view?.destroy() })
</script>

<div class="editcard-backdrop" on:click|self={requestClose}>
  <div class="editcard" role="dialog" aria-label="Edit node source">
    <header class="editcard-head">
      <span class="editcard-file">{fileName}</span>
      <span class="editcard-node" title={target?.label}>{target?.label}</span>
      {#if target?.matchCount > 1}
        <span class="editcard-ambiguous">1 of {target.matchCount} — others exist</span>
      {/if}
      <span class="editcard-spacer"></span>
      <span class="editcard-hint">Ctrl+S save · Esc close</span>
      <button class="editcard-close" title="Close (Esc)" on:click={requestClose}>×</button>
    </header>
    <div class="editcard-editor" bind:this={container}></div>
  </div>
</div>

<style>
  .editcard-backdrop {
    position: absolute; inset: 0; z-index: 50;
    display: flex; align-items: center; justify-content: center;
    background: rgba(10, 16, 26, 0.72);
  }
  .editcard {
    width: 70%; height: 70%;
    min-width: 360px; min-height: 240px;
    display: flex; flex-direction: column;
    background: #1b2636; border: 1px solid #2a3a4a; border-radius: 8px;
    box-shadow: 0 18px 60px rgba(0, 0, 0, 0.6);
    overflow: hidden; resize: both;
  }
  .editcard-head {
    flex: 0 0 auto; display: flex; align-items: center; gap: 10px;
    padding: 6px 10px; border-bottom: 1px solid #2a3a4a;
    background: #161f2e; font-size: 12px;
  }
  .editcard-file { color: #7dd3fc; font-weight: 600; }
  .editcard-node { color: #b0bec5; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 40%; }
  .editcard-ambiguous { color: #e0b000; font-size: 11px; }
  .editcard-spacer { flex: 1 1 auto; }
  .editcard-hint { color: #5a7a9a; font-size: 11px; }
  .editcard-close {
    background: none; border: none; color: #8899aa; font-size: 18px;
    line-height: 1; cursor: pointer; padding: 0 4px;
  }
  .editcard-close:hover { color: #e8eaf0; }
  .editcard-editor { flex: 1 1 0; min-height: 0; overflow: hidden; }
</style>
