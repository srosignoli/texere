<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { n4lBaseExtensions, pushRecentArrow } from '../lib/n4l-editor.js'
  import { workbenchStore } from '../stores/workbench.js'
  import { CreateExtract, BeginSitting, SittingProgress, EndSitting, GetFileContent, SaveFile, GetRecentArrows, UpdateRecentArrows } from '../../wailsjs/go/main/App'
  import { askConfirm } from '../lib/dialog.js'
  import { runLint } from '../stores/lint.js'
  import { activeReading } from '../stores/reading.js'
  import TextSourcePane from './TextSourcePane.svelte'
  import PdfSourcePane from './PdfSourcePane.svelte'
  import WebSourcePane from './WebSourcePane.svelte'

  export let reading            // ReadingView object; the Sitting's Source replaces it once begun
  export let onClose = null
  // An anchor to open at instead of saved progress, or null for a normal read.
  // A peek is a Sitting that records nothing; the backend knows, so nothing
  // here has to guard for it.
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
  let endFailed = false  // the last end failed to save the notes; the next close may discard them
  // The Sitting this view is. The Source comes back from BeginSitting as it
  // is now, so the panes open at the saved position, not a cached one.
  let sittingId = null
  let source = null
  let destroyed = false

  async function onPaneExtract(e) {
    const { text, position } = e.detail
    try {
      await flushNotes()
      await CreateExtract(sittingId, text, position)
      await reloadNotes()
    } catch (err) { error = String(err?.message ?? err) }
  }

  async function onPaneProgress(e) {
    if (!sittingId) return
    try { await SittingProgress(sittingId, e.detail.position) } catch { /* best-effort */ }
  }

  function triggerExtract() { pane?.requestExtract?.() }

  async function flushNotes() {
    if (!notesView) return
    const cur = notesView.state.doc.toString()
    if (cur !== savedNotes) {
      const res = await SaveFile(reading.notesPath, cur)
      savedNotes = res?.finalContent ?? cur
      runLint()   // a save can fix (or introduce) a defect; Problems must reflect the new state
      // Only rewrite the buffer when the backend actually normalized/injected
      // something; rewriting on every save reset the caret to the top.
      if (res?.finalContent && res.finalContent !== cur) setNotes(res.finalContent)
    }
  }

  async function reloadNotes() {
    // Failing here must not discard what's in the buffer: only touch
    // savedNotes/notesView (via setNotes) once the fetch has actually
    // succeeded, and surface the failure instead of throwing so a caller
    // that isn't itself wrapped in try/catch can't turn this into an
    // unhandled rejection.
    try {
      const text = (await GetFileContent(reading.notesPath)) ?? ''
      setNotes(text)
    } catch (err) {
      error = String(err?.message ?? err)
    }
  }

  function setNotes(text) {
    savedNotes = text
    if (!notesView) return
    // Preserve the caret (clamped to the new length) across a full-document
    // replace so a writeback/reload doesn't jump the cursor to offset 0.
    const head = Math.min(notesView.state.selection.main.head, text.length)
    notesView.dispatch({ changes: { from: 0, to: notesView.state.doc.length, insert: text }, selection: { anchor: head } })
  }

  // The unsaved notes buffer, or null when it matches what was last saved.
  function unsavedNotes() {
    const cur = notesView?.state.doc.toString()
    return cur != null && cur !== savedNotes ? cur : null
  }

  // End this Sitting. The backend owns the order and the peek rule; it keeps
  // the Sitting open when the notes cannot be saved, and then so do we.
  async function end(action, notes = unsavedNotes()) {
    if (!sittingId) return true
    try {
      let final = await EndSitting(sittingId, { position: pane?.getProgress?.() ?? '', action, notes })
      // "" means the Sitting had already ended (a flush, or another Sitting
      // began) and these notes were not saved. Save them on their own: notes
      // typed after the end must not be lost.
      if (notes != null && !final) final = (await SaveFile(reading.notesPath, notes))?.finalContent ?? notes
      if (notes != null) {
        savedNotes = final
        runLint()   // a save can fix (or introduce) a defect; Problems must reflect the new state
        if (final !== notes) setNotes(final)
      }
      endFailed = false
      return true
    } catch (e) {
      error = String(e?.message ?? e)
      endFailed = true
      return false
    }
  }

  // End the Sitting but leave the surface alone. StudySurface awaits this
  // before switching documents; the next Sitting cannot begin before it ends.
  export async function flush() { await end('continue') }

  async function close(action) {
    // After a failed save, a notes file that keeps refusing must not trap the
    // reader: closing again offers to give the notes up.
    const discard = endFailed && (await askConfirm('Close without saving notes?', { confirmLabel: 'Close' }))
    if (!(await end(action, discard ? null : unsavedNotes()))) return
    activeReading.set(null)
    onClose?.()
  }

  const done = () => close('done')

  function onKey(e) {
    if (e.key === 'Escape') { close('continue'); return }
    if ((e.ctrlKey || e.metaKey) && (e.key === 'e' || e.key === 'E')) { e.preventDefault(); triggerExtract() }
    if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) { e.preventDefault(); flushNotes() }
  }

  onMount(async () => {
    try {
      const v = await BeginSitting(reading.id, peekAt ?? '')
      if (destroyed) {
        // Torn down before the Sitting began: end it, or it stays open.
        EndSitting(v.id, { position: '', action: 'continue', notes: null }).catch(() => {})
        return
      }
      sittingId = v.id
      source = v.source
    } catch (err) {
      error = String(err?.message ?? err)
      return
    }
    try {
      const saved = await GetRecentArrows()
      if (saved?.length) recentCodesRef = saved
    } catch (e) {
      console.warn('GetRecentArrows failed:', e)
    }
    // On failure (e.g. the stored notesPath no longer resolves inside the
    // collection), skip building the editor entirely rather than opening one
    // on empty content — an empty buffer would be one accidental Ctrl+S away
    // from overwriting real notes at whatever path the backend does resolve
    // next. notesView stays undefined; every other use of it in this
    // component already guards with `if (!notesView) return` or `?.`, so a
    // half-initialised state doesn't leak into flushNotes/setNotes/onDestroy.
    try {
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
    } catch (err) {
      error = String(err?.message ?? err)
    }
  })

  // If the component is torn down without close() running — e.g. the Study
  // surface is dismissed via its own × or the Ctrl+Shift+S toggle, which unmount
  // ReadingView directly — end the Sitting with the notes buffer before the
  // editor is gone. Fire-and-forget: onDestroy can't be async. Ending a Sitting
  // that already ended does nothing.
  onDestroy(() => {
    destroyed = true
    if (sittingId) {
      const notes = unsavedNotes()
      EndSitting(sittingId, { position: pane?.getProgress?.() ?? '', action: 'continue', notes })
        .then((final) => { if (notes != null && !final) return SaveFile(reading.notesPath, notes) })
        .then(() => { if (notes != null) runLint() }).catch(() => {})
    }
    notesView?.destroy()
  })
</script>

<svelte:window on:keydown={onKey} />

<div class="reading-root" role="dialog" aria-label="Reading">
  <div class="reading-bar">
    <span class="reading-title">{source?.title ?? reading.title}</span>
    <button class="rb-btn" on:click={triggerExtract} title="Extract selection (Ctrl+E)" aria-label="Extract selection">✂ Extract</button>
    <button class="rb-btn" on:click={() => close('later')}>⏳ Later</button>
    <button class="rb-btn" on:click={done}>✓ Done</button>
    <span class="rb-spacer"></span>
    <button class="rb-close" on:click={() => close('continue')} title="Close (Esc)" aria-label="Close reader">×</button>
  </div>
  {#if error}<div class="reading-error">{error}</div>{/if}
  <div class="reading-body">
    <div class="reading-source">
      {#if !source}
        <!-- the Sitting has not begun yet -->
      {:else if source.medium === 'pdf'}
        <PdfSourcePane bind:this={pane} reading={source} startAt={peekAt} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {:else if source.medium === 'web'}
        <WebSourcePane bind:this={pane} reading={source} startAt={peekAt} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {:else}
        <TextSourcePane bind:this={pane} reading={source} startAt={peekAt} on:extract={onPaneExtract} on:progress={onPaneProgress} />
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
