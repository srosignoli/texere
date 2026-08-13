<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { n4lBaseExtensions, pushRecentArrow } from '../lib/n4l-editor.js'
  import { workbenchStore } from '../stores/workbench.js'
  import { CreateExtract, RescheduleReading, GetFileContent, SaveFile, GetRecentArrows, UpdateRecentArrows, UpdateReadingProgress, LogReadingSession } from '../../wailsjs/go/main/App'
  import { activeReading } from '../stores/reading.js'
  import TextSourcePane from './TextSourcePane.svelte'
  import PdfSourcePane from './PdfSourcePane.svelte'
  import WebSourcePane from './WebSourcePane.svelte'

  export let reading            // ReadingView object
  export let onClose = null
  // An anchor to open at instead of saved progress, or null for a normal read.
  // While peeking, progress is NOT persisted: you came to look at one passage,
  // and closing must leave you where you had actually read up to.
  export let peekAt = null

  let arrowDefsRef = []
  let recentCodesRef = []

  $: arrowDefsRef = $workbenchStore.arrowDefs ?? []

  function handleArrowSelected(code) {
    recentCodesRef = pushRecentArrow(recentCodesRef, code)
    UpdateRecentArrows(recentCodesRef).catch((e) => console.warn('UpdateRecentArrows failed:', e))
  }

  let pane
  let notesContainer
  let notesView
  let savedNotes = ''
  let error = ''
  let closed = false   // set once close()/onDestroy has persisted, to avoid double-saving

  // Session start, for the activity log. Wall-clock rather than a timer: the
  // number only needs to be right to the second.
  const openedAt = Date.now()

  async function onPaneExtract(e) {
    const { text, position } = e.detail
    try {
      await flushNotes()
      // A peek must not advance progress — extracting during one would rewind
      // the document to the peeked passage. The extract itself is still made.
      await CreateExtract(reading.id, text, position, !peekAt)
      await reloadNotes()
    } catch (err) { error = String(err?.message ?? err) }
  }

  async function onPaneProgress(e) {
    if (peekAt) return   // PDF/web push as the user navigates; a peek must not record
    try { await UpdateReadingProgress(reading.id, e.detail.position) } catch { /* best-effort */ }
  }

  function triggerExtract() { pane?.requestExtract?.() }

  async function flushNotes() {
    if (!notesView) return
    const cur = notesView.state.doc.toString()
    if (cur !== savedNotes) {
      const res = await SaveFile(reading.notesPath, cur)
      savedNotes = res?.finalContent ?? cur
      // Only rewrite the buffer when the backend actually normalized/injected
      // something; rewriting on every save reset the caret to the top.
      if (res?.finalContent && res.finalContent !== cur) setNotes(res.finalContent)
    }
  }

  async function reloadNotes() {
    const text = (await GetFileContent(reading.notesPath)) ?? ''
    setNotes(text)
  }

  function setNotes(text) {
    savedNotes = text
    if (!notesView) return
    // Preserve the caret (clamped to the new length) across a full-document
    // replace so a writeback/reload doesn't jump the cursor to offset 0.
    const head = Math.min(notesView.state.selection.main.head, text.length)
    notesView.dispatch({ changes: { from: 0, to: notesView.state.doc.length, insert: text }, selection: { anchor: head } })
  }

  async function done() {
    await RescheduleReading(reading.id, 'done')
    close('done')
  }

  // Persist the way close() does, but leave the surface alone. StudySurface
  // awaits this before switching documents: onDestroy's write is
  // fire-and-forget, so a re-read of this reading would otherwise race it and
  // see the stale position. Sets `closed` so onDestroy does not write again.
  export async function flush() {
    if (closed) return
    closed = true
    try {
      const pos = peekAt ? null : pane?.getProgress?.()
      if (pos) await UpdateReadingProgress(reading.id, pos)
      if (!peekAt) LogReadingSession(reading.id, Math.round((Date.now() - openedAt) / 1000)).catch(() => {})
      await flushNotes()
      await RescheduleReading(reading.id, 'continue')
    } catch (e) { /* best-effort */ }
  }

  async function close(action) {
    // flush() may already have persisted this session (StudySurface awaits it
    // before switching documents, leaving this instance mounted and clickable
    // meanwhile). Re-persisting would log the same reading time twice.
    const alreadyPersisted = closed
    closed = true
    try {
      if (!alreadyPersisted) {
        // Persist reading progress on leave for panes that report it on demand
        // (text/markdown). PDF/web push progress as the user navigates.
        const pos = peekAt ? null : pane?.getProgress?.()
        if (pos) await UpdateReadingProgress(reading.id, pos)
        if (!peekAt) LogReadingSession(reading.id, Math.round((Date.now() - openedAt) / 1000)).catch(() => {})
        await flushNotes()
      }
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
        extensions: n4lBaseExtensions({
          getArrowDefs: () => arrowDefsRef,
          getRecent: () => recentCodesRef,
          onArrowSelected: handleArrowSelected,
        }),
      }),
    })
  })

  // If the component is torn down without close() running — e.g. the Study
  // surface is dismissed via its own × or the Ctrl+Shift+S toggle, which unmount
  // ReadingView directly — persist notes + progress before the editor is gone.
  // Capture the doc synchronously and fire the writes best-effort (no writeback
  // dispatch, since notesView is about to be destroyed).
  onDestroy(() => {
    if (!closed) {
      const cur = notesView?.state.doc.toString()
      if (cur != null && cur !== savedNotes) SaveFile(reading.notesPath, cur).catch(() => {})
      const pos = peekAt ? null : pane?.getProgress?.()
      if (pos) UpdateReadingProgress(reading.id, pos).catch(() => {})
      if (!peekAt) LogReadingSession(reading.id, Math.round((Date.now() - openedAt) / 1000)).catch(() => {})
      RescheduleReading(reading.id, 'continue').catch(() => {})
    }
    notesView?.destroy()
  })
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
        <PdfSourcePane bind:this={pane} {reading} startAt={peekAt} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {:else if reading.medium === 'web'}
        <WebSourcePane bind:this={pane} {reading} startAt={peekAt} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {:else}
        <TextSourcePane bind:this={pane} {reading} startAt={peekAt} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {/if}
    </div>
    <div class="reading-notes" bind:this={notesContainer}></div>
  </div>
</div>

<style>
  .reading-root { position: absolute; inset: 0; z-index: 58; background: var(--surface-study); display: flex; flex-direction: column; }
  .reading-bar { display: flex; align-items: center; gap: 8px; padding: 8px 12px;
    border-bottom: 1px solid var(--border); background: var(--surface-bar); }
  .reading-title { color: var(--accent-text); font-weight: 600; margin-right: 8px; }
  .rb-btn { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px;
    padding: 4px 10px; cursor: pointer; font-size: 12px; }
  .rb-btn:hover { background: var(--accent-hover); }
  .rb-spacer { flex: 1; }
  .rb-close { background: none; border: none; color: var(--text-muted); font-size: 18px; cursor: pointer; }
  .reading-error { color: var(--danger); padding: 6px 12px; font-size: 12px; }
  .reading-body { flex: 1; min-height: 0; display: flex; }
  .reading-source { flex: 1; min-width: 0; display: flex; border-right: 1px solid var(--border); }
  .reading-notes { flex: 1; min-width: 0; overflow: hidden; }
</style>
