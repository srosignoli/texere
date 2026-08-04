<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import { EditorView } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { workbenchStore } from '../stores/workbench.js'
  import { n4lBaseExtensions, pushRecentArrow, notebookDotsGutter, applyDots } from '../lib/n4l-editor.js'
  import { GetExtractTarget, PromoteExtract, DeferExtract, DismissExtract, GetFileContent, SaveFile, SyncStudyItems, GetRecentArrows, UpdateRecentArrows, GetNotebookLinks } from '../../wailsjs/go/main/App'

  export let extractId
  export let onAdvance = null

  const dispatch = createEventDispatcher()

  let target = null
  let container
  let view
  let savedContent = ''
  let error = ''
  let seeding = false   // true after "Make card": author is filling in the back
  let vimMode = false
  let arrowDefsRef = []
  let recentCodesRef = []
  $: arrowDefsRef = $workbenchStore.arrowDefs ?? []

  // Caret reporting is debounced because it fires on every keystroke and arrow
  // key, and the consumer does a backend peek. Resting on a line should report
  // once; scrolling through fifty should report once at the end.
  let caretTimer = null
  let lastCaretLine = 0

  function reportCaret(view) {
    const line = view.state.doc.lineAt(view.state.selection.main.head).number
    if (line === lastCaretLine) return
    lastCaretLine = line
    clearTimeout(caretTimer)
    caretTimer = setTimeout(() => dispatch('caret', { line }), 150)
  }

  function handleArrowSelected(code) {
    recentCodesRef = pushRecentArrow(recentCodesRef, code)
    UpdateRecentArrows(recentCodesRef).catch(() => {})
  }

  async function refreshDots() {
    if (!view || !target) return
    try {
      const marks = await GetNotebookLinks(target.filePath)
      applyDots(view, (marks ?? []).map((m) => m.line))
    } catch { /* best-effort */ }
  }

  async function flush() {
    if (!view || !target) return
    const cur = view.state.doc.toString()
    if (cur !== savedContent) {
      const res = await SaveFile(target.filePath, cur)
      savedContent = res?.finalContent ?? cur
      // A save can shift every line below an edit, so any line->node map a
      // consumer holds is now stale. Ctrl+S, Make card and the action bar all
      // reach the file through here, and none of them dispatched anything
      // before, so a caret->notebook map had no way to learn a save had
      // happened and stayed wrong for the rest of the session.
      dispatch('saved')
    }
  }

  // Insert text as its own block at the cursor, persist, and return the inserted
  // text so the caller can record provenance. Handles SaveFile's # @id injection
  // by reloading the editor to the final on-disk content.
  export async function insertAndSave(text) {
    if (!view || !target) return ''
    let block = text.endsWith('\n') ? text : text + '\n'
    const sel = view.state.selection.main
    const line = view.state.doc.lineAt(sel.head)
    const at = line.to               // end of the cursor's line
    if (view.state.doc.sliceString(at - 1, at) !== '\n' && at !== 0) block = '\n' + block
    view.dispatch({ changes: { from: at, insert: block }, selection: { anchor: at + block.length } })
    const res = await SaveFile(target.filePath, view.state.doc.toString())
    const final = res?.finalContent ?? view.state.doc.toString()
    if (final !== view.state.doc.toString()) {
      savedContent = final
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: final } })
    } else {
      savedContent = final
    }
    await refreshDots()
    return block
  }

  async function act(fn) {
    error = ''
    try {
      await flush()                 // never lose a refine edit
      await fn()
      onAdvance?.()
    } catch (e) { error = String(e?.message ?? e) }
  }

  async function makeCard() {
    error = ''
    try {
      await flush()
      await PromoteExtract(extractId, 'card')   // seeds "<passage> >> " + # @card
      const text = (await GetFileContent(target.filePath)) ?? ''
      savedContent = text
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } })
      seeding = true
      focusSeedLine()
    } catch (e) { error = String(e?.message ?? e) }
  }

  // Place the cursor at the end of the seeded "... >> " line so the author types the back.
  function focusSeedLine() {
    if (!view) return
    const doc = view.state.doc
    for (let n = 1; n <= doc.lines; n++) {
      const line = doc.line(n)
      if (line.text.trimEnd().endsWith('>>')) {
        view.dispatch({ selection: { anchor: line.to }, effects: EditorView.scrollIntoView(line.to, { y: 'center' }) })
        view.focus()
        return
      }
    }
    view.focus()
  }

  async function createCard() {
    error = ''
    try {
      await flush()             // persist the filled-in back
      await SyncStudyItems()    // SaveFile does not sync; materialize the card now
      seeding = false
      onAdvance?.()
    } catch (e) { error = String(e?.message ?? e) }
  }

  const makeCloze = () => act(() => PromoteExtract(extractId, 'cloze'))
  const later = () => act(() => DeferExtract(extractId))
  const dismiss = () => act(() => DismissExtract(extractId))

  function focusLine(n) {
    if (!view || !n) return
    const doc = view.state.doc
    if (n < 1 || n > doc.lines) { view.focus(); return }
    const line = doc.line(n)
    view.dispatch({ selection: { anchor: line.from }, effects: EditorView.scrollIntoView(line.from, { y: 'center' }) })
    view.focus()
  }

  // Select the extract's passage inside the note file so it's clear which text this
  // Process item is about. On disk the passage is whitespace-collapsed onto target.line
  // (the "<passage> (from) \"anchor\"" node line), so match it there and select just the
  // passage. If it can't be found (note edited since), fall back to focusing the line.
  function selectExtract(t) {
    if (!view || !t?.line) { focusLine(t?.line); return }
    const doc = view.state.doc
    if (t.line < 1 || t.line > doc.lines) { view.focus(); return }
    const line = doc.line(t.line)
    const passage = (t.text ?? '').split(/\s+/).filter(Boolean).join(' ')
    const col = passage ? line.text.indexOf(passage) : -1
    if (col < 0) { focusLine(t.line); return }
    const from = line.from + col
    const to = from + passage.length
    view.dispatch({ selection: { anchor: from, head: to }, effects: EditorView.scrollIntoView(from, { y: 'center' }) })
    view.focus()
  }

  function buildExtensions() {
    return n4lBaseExtensions({
      getArrowDefs: () => arrowDefsRef,
      getRecent: () => recentCodesRef,
      onArrowSelected: handleArrowSelected,
      vim: vimMode,
      keymap: [{ key: 'Mod-s', run: () => { flush().catch(() => {}); return true }, preventDefault: true }],
      extra: [
        notebookDotsGutter(() => {}),
        EditorView.updateListener.of((u) => {
          if (u.selectionSet || u.docChanged) reportCaret(u.view)
        }),
      ],
    })
  }

  // Recreate the view with the current extension set (preserving unsaved content),
  // e.g. after toggling Vim. Cursor resets to the top, matching the main editor.
  function rebuildEditor() {
    if (!view) return
    const doc = view.state.doc.toString()
    view.destroy()
    view = new EditorView({ parent: container, state: EditorState.create({ doc, extensions: buildExtensions() }) })
    view.focus()
    // Constructing a view does not fire updateListener (only view.dispatch does),
    // and the cursor resets to the top here — so without an explicit report the
    // pane would keep showing the pre-toggle node's notebook while the caret sits
    // on line 1.
    reportCaret(view)
    refreshDots()
  }

  function toggleVim() {
    vimMode = !vimMode
    rebuildEditor()
  }

  onMount(async () => {
    try {
      target = await GetExtractTarget(extractId)
      dispatch('ready', { filePath: target.filePath })
      try { const s = await GetRecentArrows(); if (s?.length) recentCodesRef = s } catch { /* best-effort */ }
      const text = (await GetFileContent(target.filePath)) ?? ''
      savedContent = text
      view = new EditorView({
        parent: container,
        state: EditorState.create({ doc: text, extensions: buildExtensions() }),
      })
      selectExtract(target)
      // Report the initial caret explicitly. selectExtract fires the update
      // listener on its success path, but its fallback branches (line out of
      // range, or no line at all) only call view.focus(), which dispatches
      // nothing — the pane would then never learn where the caret started and
      // would sit on the empty state until the user moved it by hand.
      // Harmless on the common path: reportCaret's same-line guard collapses
      // the duplicate.
      reportCaret(view)
      await refreshDots()
    } catch (e) { error = String(e?.message ?? e) }
  })

  // Persist any unsaved edits (e.g. a card back typed after "Make card") before
  // teardown, so dismissing the Study surface without Create card / Later doesn't
  // drop them. flush() captures the doc synchronously and is a no-op when clean.
  onDestroy(() => {
    flush().catch(() => {})
    clearTimeout(caretTimer)
    view?.destroy()
  })
</script>

<div class="pv-root">
  <div class="pv-bar">
    <span class="pv-title">Process</span>
    {#if target}<span class="pv-anchor" title={target.fromAnchor}>▸ {target.fromAnchor}</span>{/if}
    <button class="vim-btn" class:active={vimMode} on:click={toggleVim} title="Toggle Vim keybindings">VIM</button>
    <span class="pv-spacer"></span>
    {#if seeding}
      <span class="pv-anchor">Fill in the back after "&gt;&gt;", then:</span>
      <button class="pv-btn" on:click={createCard}>✓ Create card</button>
    {:else}
      <button class="pv-btn" on:click={makeCard}>→ Make card</button>
      <button class="pv-btn" on:click={makeCloze}>{'{ }'} Cloze</button>
      <button class="pv-btn" on:click={later}>⏳ Later</button>
      <button class="pv-btn" on:click={dismiss}>✕ Dismiss</button>
    {/if}
  </div>
  {#if error}<div class="pv-error">{error}</div>{/if}
  <div class="pv-editor" bind:this={container}></div>
</div>

<style>
  .pv-root { position: absolute; inset: 0; display: flex; flex-direction: column; background: #131c29; }
  .pv-bar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .pv-title { color: #7dd3fc; font-weight: 600; }
  .pv-anchor { color: #5a7a9a; font-size: 11px; }
  .pv-spacer { flex: 1; }
  .pv-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 4px 10px; cursor: pointer; font-size: 12px; }
  .pv-btn:hover { background: #244a72; }
  .vim-btn { padding: 1px 7px; background: none; border: 1px solid #2a3a4a; border-radius: 3px;
    color: #4a5a6a; font-size: 0.7rem; font-weight: 600; cursor: pointer; letter-spacing: 0.05em; }
  .vim-btn.active { border-color: #2563eb; color: #93c5fd; background: #1e3a5a; }
  .pv-error { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .pv-editor { flex: 1; min-height: 0; overflow: hidden; }
</style>
