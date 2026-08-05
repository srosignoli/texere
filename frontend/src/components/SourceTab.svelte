<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import { editorAPI } from '../stores/workbench.js'
  import { EditorView } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { undo } from '@codemirror/commands'
  import { startCompletion } from '@codemirror/autocomplete'
  import { search, openSearchPanel, searchKeymap } from '@codemirror/search'
  import { get } from 'svelte/store'
  import { workbenchStore, filteredNodes } from '../stores/workbench.js'
  import { n4lBaseExtensions, pushRecentArrow, notebookDotsGutter, applyDots } from '../lib/n4l-editor.js'
  import { SaveFile, GetFileContent, GetRecentArrows, UpdateRecentArrows, NotebookMarks, FileNodeSpans, PeekNotebook } from '../../wailsjs/go/main/App'
  import { EventsOn } from '../../wailsjs/runtime/runtime'
  import { nodeIdAtLine } from '../lib/node-span.js'
  import NotebookPane from './notebook/NotebookPane.svelte'

  const dispatch = createEventDispatcher()

  let container
  let view
  let externalBanner = null
  let vimMode = false
  let cancelLinksChanged = null
  let cancelNotebooksChanged = null
  let loadedFilePath = null  // tracks which FILE is currently in the editor
  let _prevContextFilter = null
  let savedContent = ''      // text at last save/load — used to derive isDirty

  // Gutter dots: lines whose node has a notebook. Keyed by node id server-side,
  // so a dot survives an edit to the line's text — the text-keyed links this
  // replaced dropped the dot on any edit, including the rewrite that making a
  // card performs.
  let dotMarks = []      // [{line, notebookId}]

  function onDotClick(line) {
    const m = dotMarks.find((x) => x.line === line)
    if (m) openNotebookReadOnly(line)
  }

  async function refreshDots(path) {
    try {
      dotMarks = (await NotebookMarks(path)) ?? []
      if (view) applyDots(view, dotMarks.map((m) => m.line))
    } catch { /* best-effort */ }
  }

  // The overlay is "armed" by a dot click and stays armed until Close. While
  // armed it shows whatever notebook the focused node has, and hides when that
  // node has none — so it can never display a notebook for a note you are not
  // reading. Close is sticky: without that, moving the caret would reopen what
  // you just dismissed.
  let armed = false
  let reopenReadOnly = true
  function openNotebookReadOnly(line) {
    armed = true
    reopenReadOnly = true
    // Move the caret to the clicked line rather than resolving it out of band.
    // The overlay follows the caret, so leaving the caret where it was would
    // show a node the user is not on — and reportCaret's same-line guard would
    // then never fire for their actual line, freezing the overlay there.
    if (view && line >= 1 && line <= view.state.doc.lines) {
      const pos = view.state.doc.line(line).from
      view.dispatch({ selection: { anchor: pos } })
      view.focus()
    }
    resolveCaret(line, true)
  }
  function closeReopen() { armed = false; reopenReadOnly = true }
  function resumeReopen() { reopenReadOnly = false }

  // Caret -> node -> notebook, for the read-only overlay. Same approach as
  // ProcessView: fetch the line map once per file and resolve locally, because
  // caret movement is constant and a round trip per keystroke would be a stream
  // of work for a read that changes nothing.
  let spans = []
  let caretLine = 0
  let focusedId = null
  let focusedNb = null      // NotebookView for focusedId, or null when it has none
  let focusedNbFor = null   // which id focusedNb describes, so a stale reply loses
  let caretTimer = null
  let lastCaretLine = 0

  // Dots and spans come from two INDEPENDENT parses of the same file, so every
  // path that replaces the document must refresh both. Refreshing only spans
  // leaves the gutter marking lines those nodes no longer occupy. Keeping the
  // pair in one call is what stops the next reload path from remembering half.
  async function refreshFileState(path) {
    await refreshDots(path)
    await loadSpans(path)
  }

  async function loadSpans(path) {
    if (!path) return
    try { spans = await FileNodeSpans(path) } catch { spans = [] }
    if (!view) return
    // Resolve from the view's ACTUAL caret, not the last reported line.
    // setContent (every save) and rebuildEditor both reset the caret to the top,
    // and reportCaret's debounce has not fired yet — so caretLine is stale here
    // and would briefly resolve the node the caret just left.
    const line = view.state.doc.lineAt(view.state.selection.main.head).number
    lastCaretLine = line
    clearTimeout(caretTimer)
    await resolveCaret(line)
  }

  async function resolveCaret(line, force = false) {
    caretLine = line
    const id = spans.length ? nodeIdAtLine(spans, line) : null
    if (id === focusedId && !force) return
    focusedId = id
    // Moving to a different node drops back to read-only: "Resume (edit)" was
    // granted for the notebook you were looking at, not the next one.
    reopenReadOnly = true
    // No peek until the overlay is armed. Nothing displays the result before
    // then, and assembleContext behind PeekNotebook re-parses the file, searches
    // the graph and reads the whole source document — far too much to spend on
    // every caret move for a pane that is not on screen.
    if (!id || !armed) { focusedNb = null; focusedNbFor = null; return }
    try {
      const v = await PeekNotebook(id)
      if (focusedId !== id) return   // a slower reply for a node we have left
      focusedNb = v
      focusedNbFor = id
    } catch {
      // Same guard as the success path: a rejection for a node the caret has
      // already left must not wipe the notebook belonging to the node it is on.
      if (focusedId !== id) return
      focusedNb = null
      focusedNbFor = null
    }
  }

  function reportCaret(v) {
    const line = v.state.doc.lineAt(v.state.selection.main.head).number
    if (line === lastCaretLine) return
    lastCaretLine = line
    clearTimeout(caretTimer)
    caretTimer = setTimeout(() => resolveCaret(line), 150)
  }

  // Keep mutable refs that the CM6 completion source can close over
  let arrowDefsRef = []
  let recentCodesRef = []

  // Tracks the last pendingScrollNonce we've already honoured so the nonce
  // reactive fires *exactly once* per navigateToLine() bump — even when the
  // target line is the same as the previous click (Svelte reactivity dedupes
  // identical primitive values, so watching pendingScrollLine alone misses
  // back-to-back navigations to the same line).
  let _scrollNonce = 0

  $: {
    arrowDefsRef = $workbenchStore.arrowDefs
  }

  $: externalMod = $workbenchStore.externalModification
  $: openFilePath = $workbenchStore.selectedChapterKey?.filePath ?? null

  // Load file content only when the selected FILE changes.
  // Chapter changes within the same file only scroll (via pendingScrollLine) without reloading.
  // Plain variable guard avoids re-triggering on unrelated store field changes.
  $: {
    const key = $workbenchStore.selectedChapterKey
    if (key && key.filePath !== loadedFilePath) {
      loadedFilePath = key.filePath
      // Switching files invalidates everything the overlay is built on. Without
      // this, the overlay stays armed and keeps the previous file's focused node
      // until the first caret report of the new one.
      armed = false
      spans = []
      dotMarks = []
      focusedId = null
      focusedNb = null
      focusedNbFor = null
      lastCaretLine = 0
      GetFileContent(key.filePath)
        .then((text) => {
          setContent(text ?? '')
          workbenchStore.update((s) => ({ ...s, isDirty: false }))
          refreshFileState(loadedFilePath)
          // Case A: navigateToLine() already set pendingScrollLine; honour
          // the scroll synchronously after content lands and sync the local
          // _scrollNonce so the Case B reactive doesn't double-fire on the
          // same nonce.
          const w = get(workbenchStore)
          if (w.pendingScrollLine) {
            scrollToLine(w.pendingScrollLine)
            _scrollNonce = w.pendingScrollNonce ?? _scrollNonce
          }
        })
        .catch((e) => console.error('GetFileContent failed:', e))
    }
  }

  // Case B: GetFileContent resolved first → scroll when pendingScrollLine arrives later.
  // Also handles same-file chapter/node switches (no reload, just scroll).
  // Guard: only fire when the target file is already loaded; otherwise Case A
  // (inside GetFileContent.then) will handle it after the async load completes.
  //
  // Watches pendingScrollNonce in addition to the line value so two clicks
  // that target the *same* line still re-trigger the scroll. Without the
  // nonce, Svelte's reactive dedupe of identical numbers swallows the
  // second event.
  $: if ($workbenchStore.pendingScrollNonce !== _scrollNonce
         && $workbenchStore.pendingScrollLine
         && view?.state.doc.length > 0
         && $workbenchStore.selectedChapterKey?.filePath === loadedFilePath) {
    _scrollNonce = $workbenchStore.pendingScrollNonce
    scrollToLine($workbenchStore.pendingScrollLine)
  }

  // Node selected in column 3 → scroll editor to that node's line, then clear so
  // subsequent editor clicks don't re-trigger this reactive and override editorCursorLine.
  $: if ($workbenchStore.selectedNodeLine) {
    scrollToLine($workbenchStore.selectedNodeLine)
    workbenchStore.update((s) => ({ ...s, selectedNodeLine: null }))
  }

  // Context filter changed → scroll to the first visible node (earliest in file order).
  // Object-identity comparison prevents spurious fires on unrelated store updates.
  $: {
    const filter = $workbenchStore.activeContextFilter
    if (filter !== _prevContextFilter) {
      _prevContextFilter = filter
      if (filter.size > 0) {
        const nodes = get(filteredNodes)
        if (nodes.length > 0) scrollToLine(nodes[0].Line)
      }
    }
  }

  // Show external-modification banner when the currently open file is flagged
  $: if (externalMod && externalMod === openFilePath) {
    externalBanner = externalMod
  } else if (!externalMod) {
    externalBanner = null
  }

  // Silent reload when the backend has injected # @id markers into the open
  // file and the buffer was clean (App.svelte already guards the dirty case).
  let _silentReloadNonce = 0
  $: if ($workbenchStore.silentReloadNonce !== _silentReloadNonce && openFilePath) {
    _silentReloadNonce = $workbenchStore.silentReloadNonce
    GetFileContent(openFilePath)
      .then((text) => {
        setContent(text ?? '')
        workbenchStore.update((s) => ({ ...s, isDirty: false }))
        refreshFileState(openFilePath)   // ids were just injected — dots and map are stale
      })
      .catch((e) => console.error('silent reload after # @id injection failed:', e))
  }

  // ----- Public API (called by parent via bind:this) -----

  export function setContent(text) {
    if (!view) return
    savedContent = text   // snapshot before dispatch so updateListener sees clean state
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: text },
      selection: { anchor: 0 },
      effects: EditorView.scrollIntoView(0),
    })
  }

  export function toggleVim() {
    vimMode = !vimMode
    rebuildEditor()
  }

  export function getContent() {
    return view?.state.doc.toString() ?? ''
  }

  export function scrollToLine(lineNumber) {
    if (!view || !lineNumber) return
    const doc = view.state.doc
    if (lineNumber < 1 || lineNumber > doc.lines) return
    const line = doc.line(lineNumber)
    view.dispatch({
      effects: EditorView.scrollIntoView(line.from, { y: 'center' }),
      selection: { anchor: line.from },
    })
    view.focus()
  }

  export function triggerSave() {
    handleSave()
  }

  export function triggerUndo() {
    if (view) undo(view)
  }

  export function triggerSearch() {
    if (view) openSearchPanel(view)
  }

  export function openArrowPicker() {
    if (!view) return
    workbenchStore.update((s) => ({ ...s, activeTab: 'note' }))
    // Insert '(' then immediately open autocomplete — CM6 handles the rest natively
    const sel = view.state.selection.main
    view.dispatch({
      changes: { from: sel.from, to: sel.to, insert: '(' },
      selection: { anchor: sel.from + 1 },
    })
    view.focus()
    startCompletion(view)
  }

  export function insertAtCursor(text) {
    if (!view) return
    const sel = view.state.selection.main
    view.dispatch({
      changes: { from: sel.from, to: sel.to, insert: text },
      selection: { anchor: sel.from + text.length },
    })
    view.focus()
  }

  // Insert text at the end of the current line (ignores intra-line cursor position).
  // Used by +node and +ctx so they always append cleanly rather than splitting text.
  export function insertAtLineEnd(text) {
    if (!view) return
    const line = view.state.doc.lineAt(view.state.selection.main.head)
    view.dispatch({
      changes: { from: line.to, insert: text },
      selection: { anchor: line.to + text.length },
    })
    view.focus()
  }

  // ----- Save -----

  async function handleSave() {
    if (!openFilePath) return
    const content = getContent()
    try {
      const result = await SaveFile(openFilePath, content)
      savedContent = result?.finalContent ?? content  // mark new clean baseline
      workbenchStore.update((s) => ({ ...s, isDirty: false }))
      if (result?.finalContent) {
        setContent(result.finalContent)  // setContent also updates savedContent — fine
      }
      await refreshFileState(loadedFilePath)   // a save can shift every line below an edit
      dispatch('saved', { injections: result?.injections ?? [] })
    } catch (e) {
      console.error('SaveFile failed:', e)
    }
  }

  // ----- External modification banner actions -----

  async function handleReload() {
    if (!openFilePath) return
    try {
      const text = await GetFileContent(openFilePath)
      setContent(text)
      workbenchStore.update((s) => ({ ...s, externalModification: null, isDirty: false }))
      refreshFileState(openFilePath)   // the file changed on disk; line numbers moved
      externalBanner = null
    } catch (e) {
      console.error('GetFileContent reload failed:', e)
    }
  }

  function handleDismiss() {
    workbenchStore.update((s) => ({ ...s, externalModification: null }))
    externalBanner = null
  }

  // ----- CM6 setup -----

  function buildExtensions() {
    return n4lBaseExtensions({
      getArrowDefs: () => arrowDefsRef,
      getRecent: () => recentCodesRef,
      onArrowSelected: handleArrowSelected,
      vim: vimMode,
      fontSize: '0.88rem',
      keymap: [
        { key: 'Mod-s', run: () => { handleSave(); return true }, preventDefault: true },
        ...searchKeymap,
      ],
      extra: [
        notebookDotsGutter(onDotClick),
        search({ top: false }),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            const clean = update.state.doc.toString() === savedContent
            workbenchStore.update((s) => ({ ...s, isDirty: !clean }))
          }
          if (update.selectionSet) {
            const line = update.state.doc.lineAt(update.state.selection.main.head).number
            workbenchStore.update((s) => ({ ...s, editorCursorLine: line }))
          }
        }),
        EditorView.updateListener.of((u) => {
          if (u.selectionSet || u.docChanged) reportCaret(u.view)
        }),
      ],
    })
  }

  function rebuildEditor() {
    if (!view) return
    const doc = view.state.doc.toString()
    view.destroy()
    const state = EditorState.create({ doc, extensions: buildExtensions() })
    view = new EditorView({ state, parent: container })
    reportCaret(view)
    view.focus()
    refreshFileState(loadedFilePath)
  }

  // Called by arrowCompletion when a completion is applied — updates LRU list
  function handleArrowSelected(code) {
    recentCodesRef = pushRecentArrow(recentCodesRef, code)
    UpdateRecentArrows(recentCodesRef).catch((e) => console.warn('UpdateRecentArrows failed:', e))
  }

  onMount(async () => {
    const startState = EditorState.create({
      doc: '',
      extensions: buildExtensions(),
    })
    view = new EditorView({ state: startState, parent: container })
    reportCaret(view)
    editorAPI.set({ triggerSave, triggerUndo, insertAtCursor, insertAtLineEnd, openArrowPicker })

    cancelLinksChanged = EventsOn('notebook:links-changed', (p) => {
      if (p?.path === loadedFilePath) refreshDots(loadedFilePath)
    })
    // A notebook was created elsewhere — in Process mode, which leaves this
    // editor mounted. Without this the gutter keeps its stale marks for the
    // whole session, which is exactly the reported symptom: process an extract,
    // come back here, and the dot never appears.
    cancelNotebooksChanged = EventsOn('notebook:changed', () => {
      if (loadedFilePath) refreshDots(loadedFilePath)
    })

    try {
      const saved = await GetRecentArrows()
      if (saved?.length) recentCodesRef = saved
    } catch (e) {
      console.warn('GetRecentArrows failed:', e)
    }
  })

  onDestroy(() => {
    editorAPI.set(null)
    cancelLinksChanged?.()
    cancelNotebooksChanged?.()
    clearTimeout(caretTimer)
    view?.destroy()
  })
</script>

<div class="source-tab-wrapper">
  <div class="editor-toolbar">
    <button class="vim-btn" class:active={vimMode} on:click={toggleVim} title="Toggle Vim keybindings">VIM</button>
  </div>
  {#if externalBanner}
    <div class="ext-mod-banner">
      <span>File changed externally — Reload?</span>
      <button class="banner-btn reload" on:click={handleReload}>Reload</button>
      <button class="banner-btn dismiss" on:click={handleDismiss}>Dismiss</button>
    </div>
  {/if}
  <div class="editor-container" bind:this={container} />
  {#if armed && focusedNb && focusedNbFor === focusedId}
    <div class="nb-overlay">
      <div class="nb-overlay-bar">
        <span>Notebook</span>
        {#if reopenReadOnly}<button on:click={resumeReopen}>Resume (edit)</button>{/if}
        <button on:click={closeReopen}>✕ Close</button>
      </div>
      <div class="nb-overlay-body">
        {#key focusedNb.notebookId + reopenReadOnly}
          <NotebookPane notebookId={focusedNb.notebookId} readOnly={reopenReadOnly} onPush={() => {}} />
        {/key}
      </div>
    </div>
  {/if}
</div>

<style>
  .source-tab-wrapper {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    position: relative;
  }

  .nb-overlay { position: absolute; top: 0; right: 0; bottom: 0; width: 42%; min-width: 360px; display: flex; flex-direction: column; background: #0b1220; border-left: 1px solid #2a3a4a; z-index: 20; }
  .nb-overlay-bar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #101826; color: #7dd3fc; }
  .nb-overlay-bar button { margin-left: auto; background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .nb-overlay-bar button + button { margin-left: 6px; }
  .nb-overlay-body { position: relative; flex: 1; min-height: 0; }

  .editor-toolbar {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    background: #161f2e;
    border-bottom: 1px solid #2a3a4a;
    flex-shrink: 0;
  }

  .vim-btn {
    padding: 1px 7px;
    background: none;
    border: 1px solid #2a3a4a;
    border-radius: 3px;
    color: #4a5a6a;
    font-size: 0.7rem;
    font-weight: 600;
    cursor: pointer;
    letter-spacing: 0.05em;
  }

  .vim-btn.active {
    border-color: #2563eb;
    color: #93c5fd;
    background: #1e3a5a;
  }

  .ext-mod-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 12px;
    background: #78350f;
    color: #fde68a;
    font-size: 0.82rem;
    flex-shrink: 0;
  }

  .ext-mod-banner span {
    flex: 1;
  }

  .banner-btn {
    padding: 3px 10px;
    border-radius: 3px;
    border: none;
    cursor: pointer;
    font-size: 0.8rem;
  }

  .banner-btn.reload {
    background: #f59e0b;
    color: #1c1100;
  }

  .banner-btn.dismiss {
    background: #44403c;
    color: #e8eaf0;
  }

  .editor-container {
    flex: 1;
    overflow: hidden;
  }

  .editor-container :global(.cm-editor) {
    height: 100%;
  }

  .editor-container :global(.cm-scroller) {
    overflow: auto;
  }
</style>
