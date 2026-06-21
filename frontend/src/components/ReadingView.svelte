<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView, keymap, lineNumbers } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { defaultKeymap, historyKeymap, history } from '@codemirror/commands'
  import { autocompletion } from '@codemirror/autocomplete'
  import { n4lLanguage } from '../lib/n4l-language.js'
  import { n4lHighlighting } from '../lib/n4l-highlight.js'
  import { arrowCompletion } from '../lib/arrow-autocomplete.js'
  import { workbenchStore } from '../stores/workbench.js'
  import { GetSourceProjection, CreateExtract, RescheduleReading, GetFileContent, SaveFile, GetRecentArrows, UpdateRecentArrows } from '../../wailsjs/go/main/App'
  import { activeReading } from '../stores/reading.js'

  export let reading            // ReadingView object
  export let onClose = null

  let arrowDefsRef = []
  let recentCodesRef = []

  $: arrowDefsRef = $workbenchStore.arrowDefs ?? []

  function handleArrowSelected(code) {
    recentCodesRef = [code, ...recentCodesRef.filter((c) => c !== code)].slice(0, 10)
    UpdateRecentArrows(recentCodesRef).catch((e) => console.warn('UpdateRecentArrows failed:', e))
  }

  let projection = ''
  let srcEl                     // the source-pane element (for selection offset)
  let notesContainer
  let notesView
  let savedNotes = ''
  let error = ''

  function charOffset() {
    // Offset of the selection start within the rendered projection text.
    const sel = window.getSelection()
    if (!sel || sel.rangeCount === 0 || !srcEl.contains(sel.anchorNode)) return null
    const range = sel.getRangeAt(0)
    const pre = range.cloneRange()
    pre.selectNodeContents(srcEl)
    pre.setEnd(range.startContainer, range.startOffset)
    return { offset: [...pre.toString()].length, text: sel.toString() }
  }

  async function extract() {
    const sel = charOffset()
    if (!sel || !sel.text.trim()) return
    try {
      await flushNotes() // never lose an unsaved notes edit
      await CreateExtract(reading.id, sel.text, sel.offset)
      await reloadNotes() // pick up the appended extract
    } catch (e) {
      error = String(e?.message ?? e)
    }
  }

  async function flushNotes() {
    if (!notesView) return
    const cur = notesView.state.doc.toString()
    if (cur !== savedNotes) {
      const res = await SaveFile(reading.notesPath, cur)
      savedNotes = res?.finalContent ?? cur
      if (res?.finalContent) setNotes(res.finalContent)
    }
  }

  async function reloadNotes() {
    const text = (await GetFileContent(reading.notesPath)) ?? ''
    setNotes(text)
  }

  function setNotes(text) {
    savedNotes = text
    notesView?.dispatch({ changes: { from: 0, to: notesView.state.doc.length, insert: text } })
  }

  async function done() {
    await RescheduleReading(reading.id, 'done')
    close('done')
  }

  async function close(action) {
    try {
      await flushNotes()
      if (action !== 'done') await RescheduleReading(reading.id, action || 'continue')
    } catch (e) { /* best-effort on close */ }
    activeReading.set(null)
    onClose?.()
  }

  function onKey(e) {
    if (e.key === 'Escape') { close('continue'); return }
    if ((e.ctrlKey || e.metaKey) && (e.key === 'e' || e.key === 'E')) { e.preventDefault(); extract() }
    if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) { e.preventDefault(); flushNotes() }
  }

  onMount(async () => {
    try {
      const saved = await GetRecentArrows()
      if (saved?.length) recentCodesRef = saved
    } catch (e) {
      console.warn('GetRecentArrows failed:', e)
    }
    try {
      projection = (await GetSourceProjection(reading.refPath)) ?? ''
    } catch (e) {
      error = String(e?.message ?? e)
    }
    const text = (await GetFileContent(reading.notesPath)) ?? ''
    savedNotes = text
    notesView = new EditorView({
      parent: notesContainer,
      state: EditorState.create({
        doc: text,
        extensions: [
          lineNumbers(), history(), n4lLanguage, n4lHighlighting,
          autocompletion({ override: [(ctx) => arrowCompletion(arrowDefsRef, recentCodesRef, handleArrowSelected)(ctx)] }),
          keymap.of([...defaultKeymap, ...historyKeymap]),
          EditorView.lineWrapping,
          EditorView.theme({ '&': { height: '100%', background: '#1b2636' },
            '.cm-content': { fontFamily: 'monospace', fontSize: '0.85rem', color: '#e8eaf0' } }),
        ],
      }),
    })
  })

  onDestroy(() => notesView?.destroy())
</script>

<svelte:window on:keydown={onKey} />

<div class="reading-root" role="dialog" aria-label="Reading">
  <div class="reading-bar">
    <span class="reading-title">{reading.title}</span>
    <button class="rb-btn" on:click={extract} title="Extract selection (Ctrl+E)">✂ Extract</button>
    <button class="rb-btn" on:click={() => close('later')}>⏳ Later</button>
    <button class="rb-btn" on:click={done}>✓ Done</button>
    <span class="rb-spacer"></span>
    <button class="rb-close" on:click={() => close('continue')} title="Close (Esc)">×</button>
  </div>
  {#if error}<div class="reading-error">{error}</div>{/if}
  <div class="reading-body">
    <div class="reading-source" bind:this={srcEl}>{projection}</div>
    <div class="reading-notes" bind:this={notesContainer}></div>
  </div>
</div>

<style>
  .reading-root { position: absolute; inset: 0; z-index: 58; background: #131c29; display: flex; flex-direction: column; }
  .reading-bar { display: flex; align-items: center; gap: 8px; padding: 8px 12px;
    border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .reading-title { color: #7dd3fc; font-weight: 600; margin-right: 8px; }
  .rb-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px;
    padding: 4px 10px; cursor: pointer; font-size: 12px; }
  .rb-btn:hover { background: #244a72; }
  .rb-spacer { flex: 1; }
  .rb-close { background: none; border: none; color: #8899aa; font-size: 18px; cursor: pointer; }
  .reading-error { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .reading-body { flex: 1; min-height: 0; display: flex; }
  .reading-source { flex: 1; overflow: auto; padding: 16px 20px; white-space: pre-wrap;
    color: #cdd6e0; font-size: 14px; line-height: 1.6; border-right: 1px solid #2a3a4a;
    user-select: text; }
  .reading-notes { flex: 1; min-width: 0; overflow: hidden; }
</style>
