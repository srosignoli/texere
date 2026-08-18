<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import { EditorView, keymap, lineNumbers, drawSelection, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { defaultKeymap, historyKeymap, history } from '@codemirror/commands'
  import { autocompletion, completionStatus } from '@codemirror/autocomplete'
  import { workbenchStore } from '../../stores/workbench.js'
  import { appendLog } from '../../stores/logPanel.js'
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
        { key: 'Escape', run: (view) => {
            if (completionStatus(view.state) !== null) return false // let autocompletion close its dropdown first
            requestClose(); return true
        } },
        ...defaultKeymap,
        ...historyKeymap,
      ]),
      EditorView.theme({
        '&': { height: '100%', background: 'var(--surface)' },
        '.cm-content': { fontFamily: 'monospace', fontSize: '0.88rem', color: 'var(--text)', textAlign: 'left' },
        '.cm-gutters': { background: 'var(--surface-bar)', borderRight: '1px solid var(--border)', color: 'var(--text-dim)' },
        '.cm-cursor': { borderLeftColor: 'var(--accent-text)' },
        '.cm-activeLine': { background: 'var(--editor-active-line)' },
        '.cm-activeLineGutter': { background: 'var(--editor-active-line)' },
        '.cm-tooltip.cm-tooltip-autocomplete': { background: 'var(--surface-overlay)', border: '1px solid var(--border)', borderRadius: '6px', boxShadow: '0 6px 24px rgba(0,0,0,0.55)' },
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
      appendLog([{ severity: 'error', source: 'editor',
        message: `save failed: ${e?.message ?? e}` }])
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
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px;
    box-shadow: 0 18px 60px rgba(0, 0, 0, 0.6);
    overflow: hidden; resize: both;
  }
  .editcard-head {
    flex: 0 0 auto; display: flex; align-items: center; gap: 10px;
    padding: 6px 10px; border-bottom: 1px solid var(--border);
    background: var(--surface-bar); font-size: 12px;
  }
  .editcard-file { color: var(--accent-text); font-weight: 600; }
  .editcard-node { color: var(--text-row); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 40%; }
  /* Bare 11px text directly on var(--surface-bar) — chrome, not data (unlike
     SourcesSidebar's self-contained due-tag pair below), so it needed a real
     Light value rather than staying literal: #e0b000 measures 1.77:1 against
     --surface-bar's Light fill. Reuses var(--stat-high) rather than minting
     a fresh token — same amber "notice" hue StatsPanel already solved for
     Light (4.69:1 here against --surface-bar; that token's own comment
     records 4.52:1 against --surface-study), so this ambiguous-match notice
     keeps reading as the same amber notice everywhere in the app, which was
     the whole point of sharing the hex in the first place. */
  .editcard-ambiguous { color: var(--stat-high); font-size: 11px; }
  .editcard-spacer { flex: 1 1 auto; }
  .editcard-hint { color: var(--accent-dim); font-size: 11px; }
  .editcard-close {
    background: none; border: none; color: var(--text-muted); font-size: 18px;
    line-height: 1; cursor: pointer; padding: 0 4px;
  }
  .editcard-close:hover { color: var(--text); }
  .editcard-editor { flex: 1 1 0; min-height: 0; overflow: hidden; }
</style>
