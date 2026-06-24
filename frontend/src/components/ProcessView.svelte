<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView, keymap, lineNumbers } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { defaultKeymap, historyKeymap, history } from '@codemirror/commands'
  import { autocompletion } from '@codemirror/autocomplete'
  import { workbenchStore } from '../stores/workbench.js'
  import { n4lLanguage } from '../lib/n4l-language.js'
  import { n4lHighlighting } from '../lib/n4l-highlight.js'
  import { arrowCompletion } from '../lib/arrow-autocomplete.js'
  import { GetExtractTarget, PromoteExtract, DeferExtract, DismissExtract, GetFileContent, SaveFile, SyncStudyItems, GetRecentArrows, UpdateRecentArrows } from '../../wailsjs/go/main/App'

  export let extractId
  export let onAdvance = null

  let target = null
  let container
  let view
  let savedContent = ''
  let error = ''
  let seeding = false   // true after "Make card": author is filling in the back
  let arrowDefsRef = []
  let recentCodesRef = []
  $: arrowDefsRef = $workbenchStore.arrowDefs ?? []

  function handleArrowSelected(code) {
    recentCodesRef = [code, ...recentCodesRef.filter((c) => c !== code)].slice(0, 10)
    UpdateRecentArrows(recentCodesRef).catch(() => {})
  }

  async function flush() {
    if (!view || !target) return
    const cur = view.state.doc.toString()
    if (cur !== savedContent) {
      const res = await SaveFile(target.filePath, cur)
      savedContent = res?.finalContent ?? cur
    }
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

  onMount(async () => {
    try {
      target = await GetExtractTarget(extractId)
      try { const s = await GetRecentArrows(); if (s?.length) recentCodesRef = s } catch { /* best-effort */ }
      const text = (await GetFileContent(target.filePath)) ?? ''
      savedContent = text
      view = new EditorView({
        parent: container,
        state: EditorState.create({
          doc: text,
          extensions: [
            lineNumbers(), history(), n4lLanguage, n4lHighlighting,
            autocompletion({ override: [(ctx) => arrowCompletion(arrowDefsRef, recentCodesRef, handleArrowSelected)(ctx)] }),
            keymap.of([{ key: 'Mod-s', run: () => { flush().catch(() => {}); return true }, preventDefault: true }, ...defaultKeymap, ...historyKeymap]),
            EditorView.lineWrapping,
            EditorView.theme({ '&': { height: '100%', background: '#1b2636' }, '.cm-content': { fontFamily: 'monospace', fontSize: '0.85rem', color: '#e8eaf0' } }),
          ],
        }),
      })
      focusLine(target.line)
    } catch (e) { error = String(e?.message ?? e) }
  })

  onDestroy(() => view?.destroy())
</script>

<div class="pv-root">
  <div class="pv-bar">
    <span class="pv-title">Process</span>
    {#if target}<span class="pv-anchor" title={target.fromAnchor}>▸ {target.fromAnchor}</span>{/if}
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
  .pv-error { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .pv-editor { flex: 1; min-height: 0; overflow: hidden; }
</style>
