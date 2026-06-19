# Graph Explorer Edit Card — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user edit a pinned graph node's `.n4l` source in a centered card over the dimmed explorer, save, and return — re-laying-out the graph only when something actually changed.

**Architecture:** Frontend-only composition over existing Wails bindings. A pure resolver maps an explorer node id (`Chapter::Text`) to `{filePath, line}`; a self-contained `EditCard.svelte` hosts its own CodeMirror; `GraphExplorer.svelte` wires Enter→open, the card owns the keyboard while open, and on close it rebuilds + re-anchors only if a change was saved.

**Tech Stack:** Svelte 3.49, CodeMirror 6, Wails v2 bindings (`GetFileContent`, `SaveFile`, `BuildGraph`, `ExplorerOpen`, `ExplorerReanchor`), Node 22 `node:test` for the pure unit. No new dependencies. No Go changes.

## Global Constraints

- No new dependencies (frontend or Go); pure composition over existing modules. (`CLAUDE.md` code style + constitution.)
- No Go files change in this feature.
- Reuse the existing CodeMirror extensions in `frontend/src/lib/` (`n4l-language.js`, `n4l-highlight.js`, `arrow-autocomplete.js`).
- Notifications use the existing `appendLog([{ severity, source, message }])` log-panel channel — there is no separate toast component.
- Explorer node id format is `Chapter::Text`, or `Text` when the chapter is empty (matches `internal/explorer/types.go` `nodeKey.id()`).
- Frontend is ESM (`"type": "module"`); the pure resolver is unit-tested with `node --test` (no test framework added). Svelte components are verified with `npm run build` (compile) + a manual quickstart matrix, per project convention.

---

### Task 1: Pure node→source resolver

**Files:**
- Create: `frontend/src/lib/node-source.js`
- Test: `frontend/src/lib/node-source.test.js`

**Interfaces:**
- Consumes: nothing (pure).
- Produces: `resolveNodeSource(nodeId: string, fileCache: Record<string, ParsedFile>) => { filePath: string, chapterTitle: string, line: number, matchCount: number } | null`. `ParsedFile` here is the shape returned by the `ParseFileForUI` binding: `{ Chapters?: Array<{ Title: string, Nodes?: Array<{ Text: string, Line: number }> }> }`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/node-source.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { resolveNodeSource } from './node-source.js'

const cache = {
  '/c/a.n4l': { Chapters: [
    { Title: 'Animals', Nodes: [ { Text: 'cat', Line: 5 }, { Text: 'dog', Line: 7 } ] },
  ] },
  '/c/b.n4l': { Chapters: [
    { Title: 'Pets', Nodes: [ { Text: 'cat', Line: 3 } ] },
  ] },
}

test('exact chapter::text match returns that file/line', () => {
  assert.deepEqual(resolveNodeSource('Animals::cat', cache),
    { filePath: '/c/a.n4l', chapterTitle: 'Animals', line: 5, matchCount: 2 })
})

test('chapter disambiguates when the text appears in multiple files', () => {
  assert.deepEqual(resolveNodeSource('Pets::cat', cache),
    { filePath: '/c/b.n4l', chapterTitle: 'Pets', line: 3, matchCount: 2 })
})

test('plain text id (no chapter) returns the first match', () => {
  assert.deepEqual(resolveNodeSource('dog', cache),
    { filePath: '/c/a.n4l', chapterTitle: 'Animals', line: 7, matchCount: 1 })
})

test('chapter present but only the text matches elsewhere falls back to first text match', () => {
  assert.deepEqual(resolveNodeSource('Birds::dog', cache),
    { filePath: '/c/a.n4l', chapterTitle: 'Animals', line: 7, matchCount: 1 })
})

test('unresolved text returns null', () => {
  assert.equal(resolveNodeSource('Animals::elephant', cache), null)
})

test('null / empty inputs return null', () => {
  assert.equal(resolveNodeSource('', cache), null)
  assert.equal(resolveNodeSource('cat', null), null)
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend && node --test src/lib/node-source.test.js`
Expected: FAIL — `Cannot find module '.../node-source.js'` (the module does not exist yet).

- [ ] **Step 3: Write the minimal implementation**

Create `frontend/src/lib/node-source.js`:

```js
// resolveNodeSource maps a graph-explorer node identity back to the concrete
// .n4l source location the user can edit. Explorer node ids are "Chapter::Text"
// (or just "Text" when the node has no chapter), matching internal/explorer
// types.go nodeKey.id(). We recover { filePath, chapterTitle, line } by matching
// node text (and, when present, chapter) against parsed files. The chapter is a
// preference, not a filter: if no chapter+text hit exists we fall back to the
// first text-only hit, so callers that pass a bare text id keep first-match
// semantics. Shared by the explorer edit-card and the omnibar.

/**
 * @param {string} nodeId  "Chapter::Text" or "Text".
 * @param {Record<string, {Chapters?: Array<{Title?: string, Nodes?: Array<{Text?: string, Line: number}>}>}>} fileCache
 *        map of filePath -> ParsedFile (as returned by the ParseFileForUI binding).
 * @returns {{ filePath: string, chapterTitle: string, line: number, matchCount: number } | null}
 */
export function resolveNodeSource(nodeId, fileCache) {
  if (!nodeId || !fileCache) return null
  const sep = nodeId.indexOf('::')
  const chapter = sep === -1 ? '' : nodeId.slice(0, sep)
  const text = sep === -1 ? nodeId : nodeId.slice(sep + 2)
  const wanted = String(text).trim()
  if (!wanted) return null

  const matches = []
  let chapterMatch = null
  for (const [filePath, pf] of Object.entries(fileCache)) {
    for (const ch of pf?.Chapters ?? []) {
      for (const node of ch?.Nodes ?? []) {
        if (String(node?.Text ?? '').trim() !== wanted) continue
        const hit = { filePath, chapterTitle: ch.Title ?? '', line: node.Line }
        matches.push(hit)
        if (chapter && chapterMatch === null && (ch.Title ?? '') === chapter) {
          chapterMatch = hit
        }
      }
    }
  }
  if (matches.length === 0) return null
  const chosen = chapterMatch ?? matches[0]
  return { ...chosen, matchCount: matches.length }
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend && node --test src/lib/node-source.test.js`
Expected: PASS — `# pass 6  # fail 0`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/node-source.js frontend/src/lib/node-source.test.js
git commit -m "feat(explorer): pure node->source resolver for the edit card"
```

---

### Task 2: Explorer store fields for the edit card

**Files:**
- Modify: `frontend/src/stores/explorer.js`

**Interfaces:**
- Consumes: nothing.
- Produces: two writables — `editCardOpen` (boolean, default `false`) and `editCardTarget` (`{ filePath, chapterTitle, line, label, anchorId, matchCount } | null`, default `null`). Both reset to defaults inside the existing `resetExplorerState()`.

- [ ] **Step 1: Add the writables**

In `frontend/src/stores/explorer.js`, after the existing `pendingReanchorID` export (around line 61), add:

```js
/**
 * editCardOpen — true while the in-explorer source edit card is mounted.
 * While true, GraphExplorer hands all keyboard input to the card.
 */
export const editCardOpen = writable(false)

/**
 * editCardTarget — the resolved source location the card opens on, or null.
 * Shape: { filePath, chapterTitle, line, label, anchorId, matchCount }.
 * `anchorId` is the explorer node id the user pinned; `label` is its display
 * label; `matchCount` drives the card's "1 of N" header hint.
 */
export const editCardTarget = writable(/** @type {null | {
  filePath: string, chapterTitle: string, line: number,
  label: string, anchorId: string, matchCount: number
}} */(null))
```

- [ ] **Step 2: Reset them with the rest of explorer state**

Find `resetExplorerState()` (the function that calls `anchorHistory.set(...)` / `pendingReanchorID.set(null)` around line 132) and add these two lines inside it:

```js
  editCardOpen.set(false)
  editCardTarget.set(null)
```

- [ ] **Step 3: Verify it compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds (exit 0), no import/reference errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/stores/explorer.js
git commit -m "feat(explorer): edit-card open/target stores"
```

---

### Task 3: The EditCard component

**Files:**
- Create: `frontend/src/components/explorer/EditCard.svelte`

**Interfaces:**
- Consumes: `editCardTarget` shape from Task 2 (`{ filePath, chapterTitle, line, label, matchCount }`); bindings `GetFileContent(path) => string`, `SaveFile(path, content) => { finalContent?: string, injections?: any[] }`, `GetRecentArrows() => string[]`, `UpdateRecentArrows(codes) => void`; `$workbenchStore.arrowDefs`; lib extensions `n4lLanguage`, `n4lHighlighting`, `arrowCompletion`.
- Produces: a Svelte component with prop `target` and a single dispatched event `close` carrying `{ savedAChange: boolean }`.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/explorer/EditCard.svelte`:

```svelte
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
```

- [ ] **Step 2: Verify it compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds (exit 0). The component is not yet mounted anywhere — this step only proves it compiles.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/explorer/EditCard.svelte
git commit -m "feat(explorer): EditCard component (CodeMirror over a node's source)"
```

---

### Task 4: Wire the card into GraphExplorer (open, keyboard, round-trip)

**Files:**
- Modify: `frontend/src/components/GraphExplorer.svelte`

**Interfaces:**
- Consumes: `resolveNodeSource` (Task 1); `editCardOpen`, `editCardTarget` (Task 2); `EditCard` (Task 3); existing locals `frozenNodeID`, `frozenNodeLabel`, `lastDiagnostics`, `loading`, `nodeMeshes`, `highlight`, helpers `openExplorer`, `explorerReanchor`, `handleLayout`, `appendLog`, `layoutMode`; bindings `ParseFileForUI`, `BuildGraph`; `workbenchStore`.
- Produces: no new exports; adds the Enter→open gesture, the "card owns keyboard while open" guard, `handleCardClose`, a `pendingFreezeID` reveal-restore hook, and renders `<EditCard>`.

- [ ] **Step 1: Add imports**

In the `<script>` import block of `frontend/src/components/GraphExplorer.svelte`:

- Add `editCardOpen, editCardTarget` to the existing destructured import from `'../stores/explorer.js'`.
- Add these new imports near the other component/lib/binding imports:

```js
  import EditCard from './explorer/EditCard.svelte'
  import { resolveNodeSource } from '../lib/node-source.js'
  import { workbenchStore } from '../stores/workbench.js'
  import { ParseFileForUI, BuildGraph } from '../../wailsjs/go/main/App'
```

- [ ] **Step 2: Add edit-card state and the file-cache loader**

Near the other `let` declarations (e.g. just below `let frozenNodeLabel = ''` around line 302), add:

```js
  // Lazily-parsed file cache for node->source resolution. Keyed by filePath.
  // Entries are dropped after an edit so the next open re-resolves fresh lines.
  let editFileCache = {}
  // One-shot: restore the freeze pin on this node after a rebuild rebuilds the
  // scene (revealLandscape clears the live pin, so we re-apply once meshes exist).
  let pendingFreezeID = null

  async function ensureEditFileCache() {
    const summaries = get(workbenchStore).summaries ?? []
    for (const s of summaries) {
      if (!s.FilePath || editFileCache[s.FilePath] || s.ScanError) continue
      try {
        const pf = await ParseFileForUI(s.FilePath)
        if (pf) editFileCache[s.FilePath] = pf
      } catch (e) {
        console.warn('edit-card: parse failed', s.FilePath, e)
      }
    }
  }

  async function openEditCard() {
    if (frozenNodeID === null) return
    await ensureEditFileCache()
    const src = resolveNodeSource(frozenNodeID, editFileCache)
    if (!src) {
      appendLog([{ severity: 'warning', source: 'explorer',
        message: `no editable source line for "${frozenNodeLabel}"` }])
      return
    }
    editCardTarget.set({
      filePath: src.filePath, chapterTitle: src.chapterTitle, line: src.line,
      label: frozenNodeLabel, anchorId: frozenNodeID, matchCount: src.matchCount,
    })
    editCardOpen.set(true)
  }
```

- [ ] **Step 3: Add the round-trip close handler**

Add below `openEditCard`:

```js
  async function handleCardClose(e) {
    const savedAChange = e.detail?.savedAChange
    const target = get(editCardTarget)
    editCardOpen.set(false)
    editCardTarget.set(null)
    // Invalidate the edited file so a later open re-resolves against new text.
    if (target?.filePath) delete editFileCache[target.filePath]
    if (!savedAChange) return

    // Re-layout only if dirty: rebuild the index, re-run the same query, and
    // re-anchor on the pinned node when it survives the edit.
    loading = true
    try {
      await BuildGraph()
      const q = lastDiagnostics?.sourceQuery
      if (!q || q === '<reanchor>') return
      const layout = await openExplorer(q, get(layoutMode))
      const anchorId = target?.anchorId
      const stillThere = !!anchorId && (layout.nodes ?? []).some((n) => n.id === anchorId)
      if (anchorId) pendingFreezeID = anchorId
      if (stillThere && anchorId !== (layout.anchorId || null)) {
        const relayout = await explorerReanchor(layout.resultId, anchorId, get(layoutMode))
        handleLayout(relayout, q)
      } else {
        handleLayout(layout, q)
        if (!stillThere) {
          pendingFreezeID = null
          appendLog([{ severity: 'info', source: 'explorer',
            message: 'pinned node text changed — showing updated graph' }])
        }
      }
    } catch (err) {
      appendLog([{ severity: 'error', source: 'explorer',
        message: `rebuild after edit failed: ${err?.message ?? err}` }])
    } finally {
      loading = false
    }
  }
```

- [ ] **Step 4: Restore the freeze pin after the scene rebuilds**

In `revealLandscape`, inside the `requestAnimationFrame` callback, immediately after the line `highlight = createHighlight(nodeMeshes, edgeMeshes, layout, () => sceneCtx?.render())` (around line 539), add:

```js
      // Re-pin after an edit round-trip rebuilt the scene (FR: node stays pinned).
      if (pendingFreezeID && nodeMeshes?.byId(pendingFreezeID)) {
        frozenNodeID = pendingFreezeID
        frozenNodeLabel = nodeMeshes.fullLabelOf(pendingFreezeID) ?? pendingFreezeID
        highlight.setHovered(pendingFreezeID)
      }
      pendingFreezeID = null
```

- [ ] **Step 5: Let the card own the keyboard, and add Enter→open**

In `handleKeydown(e)`, add this as the very first statement (before the Ctrl+Shift+K block at line ~1187) so the explorer ignores all keys while the card is up:

```js
    // While the edit card is open it owns the keyboard (its own CodeMirror
    // handles Esc/Ctrl+S). Don't let the explorer act on any key.
    if (get(editCardOpen)) return
```

Then, immediately after the editable-target early-return block (the `if (t && (t.tagName === 'INPUT' || ...)) return` at lines ~1200-1203), add the Enter gesture:

```js
    if (e.key === 'Enter') {
      if (frozenNodeID === null) return // nothing pinned — let Enter pass through
      e.preventDefault()
      e.stopPropagation()
      openEditCard()
      return
    }
```

- [ ] **Step 6: Render the card**

In the template, inside the `.explorer-root` dialog (e.g. right after the `{#if frozenNodeID}` pinned-badge block near line 1376), add:

```svelte
  {#if $editCardOpen && $editCardTarget}
    <EditCard target={$editCardTarget} on:close={handleCardClose} />
  {/if}
```

- [ ] **Step 7: Verify it compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds (exit 0), no unresolved imports/symbols.

- [ ] **Step 8: Manual verification (quickstart)**

Run: `make dev` (Wails hot-reload). With a collection open:
1. Press Ctrl+G to build the graph, run a query in the explorer.
2. Click a node → it pins ("Pinned · …" badge). Press **Enter** → the centered card opens with the cursor on that node's source line.
3. Edit nothing, press **Esc** → returns instantly to the same graph; node still pinned. (No rebuild banner.)
4. Press **Enter** again, change the node's text on its line, **Ctrl+S**, then **Esc** → the loading state shows briefly (rebuild), the graph re-renders, and you land on the refreshed query with an info log "pinned node text changed".
5. Press **Enter**, edit a *different* line (not the node's own text), **Ctrl+S**, **Esc** → graph rebuilds and re-anchors on the still-present pinned node, pin restored.
6. Pin a node that is only ever an arrow *target* (never a left-hand line), press **Enter** → a warning log "no editable source line" appears and no card opens.

Expected: all six behave as described.

- [ ] **Step 9: Commit**

```bash
git add frontend/src/components/GraphExplorer.svelte
git commit -m "feat(explorer): pin -> Enter edit card with re-layout-if-dirty round-trip"
```

---

### Task 5: Point the omnibar at the shared resolver (DRY)

**Files:**
- Modify: `frontend/src/components/OmniBar.svelte`

**Interfaces:**
- Consumes: `resolveNodeSource` (Task 1).
- Produces: no API change. `enrichGraphNodes` returns the same shape as before (`{ ...gn, filePath, line, chapterTitle }`), delegating its matching loop to the shared resolver. Behavior is a superset of today's first-text-match (passing a bare text id preserves first-match order).

- [ ] **Step 1: Import the resolver**

In `frontend/src/components/OmniBar.svelte`, add to the imports:

```js
  import { resolveNodeSource } from '../lib/node-source.js'
```

- [ ] **Step 2: Replace the matching loop in `enrichGraphNodes`**

Replace the existing `enrichGraphNodes` body (the nested `for` loops at lines ~286-301) with:

```js
  function enrichGraphNodes(nodes, cache) {
    return nodes.map((gn) => {
      const r = resolveNodeSource(String(gn.NodeText ?? '').trim(), cache ?? {})
      if (r) return { ...gn, filePath: r.filePath, line: r.line, chapterTitle: r.chapterTitle }
      return { ...gn, filePath: null, line: null, chapterTitle: gn.Chapter }
    })
  }
```

Note: a bare-text id (no `::`) preserves today's first-text-match semantics; `cache` is already the `{ filePath: ParsedFile }` map `resolveNodeSource` expects.

- [ ] **Step 3: Verify it compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds (exit 0).

- [ ] **Step 4: Manual verification (quickstart)**

Run: `make dev`. Open the omnibar in graph mode (Ctrl+K → graph), run a query that returns nodes, click a result row that maps to a known node.
Expected: it still navigates to the correct file + line in the editor (unchanged behavior).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/OmniBar.svelte
git commit -m "refactor(omnibar): reuse shared resolveNodeSource for node enrichment"
```

---

### Task 6: Quickstart matrix entry

**Files:**
- Modify: `docs/quickstart.md`

**Interfaces:** none (docs only).

- [ ] **Step 1: Append the edit-card matrix**

Add a section to `docs/quickstart.md` documenting the manual matrix from Task 4 Step 8 (pin → Enter → edit → Ctrl+S → Esc; no-change instant return; node-text edit lands on refreshed query with info log; other-line edit re-anchors with pin restored; target-only node warns without opening; Esc inside the card closes the card, not the explorer).

Use the same checklist style as the existing quickstart entries (one row per behavior with the steps and the expected result).

- [ ] **Step 2: Commit**

```bash
git add docs/quickstart.md
git commit -m "docs: quickstart matrix for the graph explorer edit card"
```

---

## Self-Review

**Spec coverage:**
- Pin → Enter opens card on the node's line → Task 4 (Step 5 Enter, Step 2 `openEditCard`, Task 3 `placeCursorOnLine`). ✅
- Centered card ~70%×70% over dimmed graph, resizable → Task 3 styles (`.editcard` 70%/70%, `resize: both`, `.editcard-backdrop`). ✅
- Ctrl+S saves; Esc closes/returns → Task 3 keymap (`Mod-s`, `Escape`). ✅
- Node→source resolution, chapter-aware, ambiguity count → Task 1 + Task 3 header hint. ✅
- Unresolvable node → toast + don't open → Task 4 `openEditCard` warning branch. ✅
- Re-layout only if dirty → Task 4 `handleCardClose` (`if (!savedAChange) return`) + Task 3 `savedAChange` only set on a content-changing save. ✅
- Rebuild + re-anchor on save; pinned-text-changed lands on refreshed query with toast → Task 4 `handleCardClose` + `pendingFreezeID` restore. ✅
- DRY the omnibar resolver → Task 5. ✅
- No backend/dep changes → all tasks frontend-only; resolver tested with built-in `node:test`. ✅
- Testing convention (Go unit + manual matrix) → Task 1 unit test, Task 6 matrix. ✅

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. ✅

**Type consistency:** `resolveNodeSource` returns `{ filePath, chapterTitle, line, matchCount }` in Task 1 and is consumed with exactly those fields in Task 4 (`src.filePath/chapterTitle/line/matchCount`) and Task 5 (`r.filePath/line/chapterTitle`). `editCardTarget` shape (`filePath, chapterTitle, line, label, anchorId, matchCount`) is defined in Task 2, set in Task 4, and read in Task 3 (`target.filePath/line/label/matchCount`). `close` event payload `{ savedAChange }` dispatched in Task 3, read in Task 4 (`e.detail?.savedAChange`). Consistent. ✅
