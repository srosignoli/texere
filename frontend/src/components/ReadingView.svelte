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
  import { CreateExtract, RescheduleReading, GetFileContent, SaveFile, GetRecentArrows, UpdateRecentArrows, UpdateReadingProgress } from '../../wailsjs/go/main/App'
  import { activeReading } from '../stores/reading.js'
  import TextSourcePane from './TextSourcePane.svelte'
  import PdfSourcePane from './PdfSourcePane.svelte'
  import WebSourcePane from './WebSourcePane.svelte'

  export let reading            // ReadingView object
  export let onClose = null

  let arrowDefsRef = []
  let recentCodesRef = []

  $: arrowDefsRef = $workbenchStore.arrowDefs ?? []

  function handleArrowSelected(code) {
    recentCodesRef = [code, ...recentCodesRef.filter((c) => c !== code)].slice(0, 10)
    UpdateRecentArrows(recentCodesRef).catch((e) => console.warn('UpdateRecentArrows failed:', e))
  }

  let pane
  let notesContainer
  let notesView
  let savedNotes = ''
  let error = ''

  async function onPaneExtract(e) {
    const { text, position } = e.detail
    try {
      await flushNotes()
      await CreateExtract(reading.id, text, position)
      await reloadNotes()
    } catch (err) { error = String(err?.message ?? err) }
  }

  async function onPaneProgress(e) {
    try { await UpdateReadingProgress(reading.id, e.detail.position) } catch { /* best-effort */ }
  }

  function triggerExtract() { pane?.requestExtract?.() }

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
      // Persist reading progress on leave for panes that report it on demand
      // (text/markdown). PDF/web push progress as the user navigates.
      const pos = pane?.getProgress?.()
      if (pos) await UpdateReadingProgress(reading.id, pos)
      await flushNotes()
      if (action !== 'done') await RescheduleReading(reading.id, action || 'continue')
    } catch (e) { /* best-effort on close */ }
    activeReading.set(null)
    onClose?.()
  }

  function onKey(e) {
    if (e.key === 'Escape') { close('continue'); return }
    if ((e.ctrlKey || e.metaKey) && (e.key === 'e' || e.key === 'E')) { e.preventDefault(); triggerExtract() }
    if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) { e.preventDefault(); flushNotes() }
  }

  onMount(async () => {
    try {
      const saved = await GetRecentArrows()
      if (saved?.length) recentCodesRef = saved
    } catch (e) {
      console.warn('GetRecentArrows failed:', e)
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
    <button class="rb-btn" on:click={triggerExtract} title="Extract selection (Ctrl+E)">✂ Extract</button>
    <button class="rb-btn" on:click={() => close('later')}>⏳ Later</button>
    <button class="rb-btn" on:click={done}>✓ Done</button>
    <span class="rb-spacer"></span>
    <button class="rb-close" on:click={() => close('continue')} title="Close (Esc)">×</button>
  </div>
  {#if error}<div class="reading-error">{error}</div>{/if}
  <div class="reading-body">
    <div class="reading-source">
      {#if reading.medium === 'pdf'}
        <PdfSourcePane bind:this={pane} {reading} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {:else if reading.medium === 'web'}
        <WebSourcePane bind:this={pane} {reading} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {:else}
        <TextSourcePane bind:this={pane} {reading} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {/if}
    </div>
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
  .reading-source { flex: 1; min-width: 0; display: flex; border-right: 1px solid #2a3a4a; }
  .reading-notes { flex: 1; min-width: 0; overflow: hidden; }
</style>
