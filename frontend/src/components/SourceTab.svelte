<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import { editorAPI } from '../stores/workbench.js'
  import { EditorView, keymap, lineNumbers, drawSelection, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { defaultKeymap, historyKeymap, history, undo } from '@codemirror/commands'
  import { autocompletion, startCompletion } from '@codemirror/autocomplete'
  import { search, openSearchPanel, searchKeymap } from '@codemirror/search'
  import { vim } from '@replit/codemirror-vim'
  import { get } from 'svelte/store'
  import { workbenchStore, filteredNodes } from '../stores/workbench.js'
  import { n4lLanguage } from '../lib/n4l-language.js'
  import { n4lHighlighting } from '../lib/n4l-highlight.js'
  import { arrowCompletion } from '../lib/arrow-autocomplete.js'
  import { SaveFile, GetFileContent, GetRecentArrows, UpdateRecentArrows } from '../../wailsjs/go/main/App'

  const dispatch = createEventDispatcher()

  let container
  let view
  let externalBanner = null
  let vimMode = false
  let loadedFilePath = null  // tracks which FILE is currently in the editor
  let _prevContextFilter = null
  let savedContent = ''      // text at last save/load — used to derive isDirty

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
      GetFileContent(key.filePath)
        .then((text) => {
          setContent(text ?? '')
          workbenchStore.update((s) => ({ ...s, isDirty: false }))
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
    return [
      ...(vimMode ? [vim()] : []),
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
      search({ top: false }),
      keymap.of([
        {
          key: 'Mod-s',
          run: () => { handleSave(); return true },
          preventDefault: true,
        },
        ...defaultKeymap,
        ...historyKeymap,
        ...searchKeymap,
      ]),
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
      EditorView.theme({
        '&': { height: '100%', background: '#1b2636' },
        '.cm-content': { fontFamily: 'monospace', fontSize: '0.88rem', color: '#e8eaf0', textAlign: 'left' },
        '.cm-gutters': { background: '#161f2e', borderRight: '1px solid #2a3a4a', color: '#4a5a6a' },
        '.cm-cursor': { borderLeftColor: '#7dd3fc' },
        '.cm-selectionBackground': { background: '#1e3a5a !important' },
        '.cm-activeLineGutter': { background: '#1d2e42' },
        '.cm-activeLine': { background: '#1d2e42' },
        '.cm-searchMatch': { background: '#854d0e', borderRadius: '2px' },
        '.cm-vim-panel': { background: '#1a2535', color: '#e8eaf0', borderTop: '1px solid #2a3a4a' },
        '.cm-vim-panel input': { background: 'transparent', color: '#e8eaf0', outline: 'none' },
        // Autocomplete tooltip
        '.cm-tooltip.cm-tooltip-autocomplete': { background: '#1a2535', border: '1px solid #2a3a4a', borderRadius: '6px', boxShadow: '0 6px 24px rgba(0,0,0,0.55)' },
        '.cm-tooltip-autocomplete ul': { fontFamily: 'monospace', fontSize: '0.82rem' },
        '.cm-tooltip-autocomplete ul li': { color: '#b0bec5', padding: '3px 10px' },
        '.cm-tooltip-autocomplete ul li[aria-selected]': { background: '#1e3a5a !important', color: '#e8eaf0' },
        '.cm-completionLabel': { color: '#67e8f9' },
        '.cm-completionDetail': { color: '#8899aa', fontStyle: 'normal', marginLeft: '8px' },
        '.cm-completionSectionHeader': { color: '#5a7a9a', fontSize: '0.68rem', fontWeight: '600', textTransform: 'uppercase', letterSpacing: '0.06em', padding: '4px 10px 2px', background: '#141d2b' },
      }),
    ]
  }

  function rebuildEditor() {
    if (!view) return
    const doc = view.state.doc.toString()
    view.destroy()
    const state = EditorState.create({ doc, extensions: buildExtensions() })
    view = new EditorView({ state, parent: container })
    view.focus()
  }

  // Called by arrowCompletion when a completion is applied — updates LRU list
  function handleArrowSelected(code) {
    recentCodesRef = [code, ...recentCodesRef.filter((c) => c !== code)].slice(0, 10)
    UpdateRecentArrows(recentCodesRef).catch((e) => console.warn('UpdateRecentArrows failed:', e))
  }

  onMount(async () => {
    const startState = EditorState.create({
      doc: '',
      extensions: buildExtensions(),
    })
    view = new EditorView({ state: startState, parent: container })
    editorAPI.set({ triggerSave, triggerUndo, insertAtCursor, insertAtLineEnd, openArrowPicker })

    try {
      const saved = await GetRecentArrows()
      if (saved?.length) recentCodesRef = saved
    } catch (e) {
      console.warn('GetRecentArrows failed:', e)
    }
  })

  onDestroy(() => {
    editorAPI.set(null)
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
</div>

<style>
  .source-tab-wrapper {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }

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
