# Graph Explorer — Edit a Pinned Node In Place

**Date:** 2026-06-18
**Status:** Design approved, pending spec review
**Feature area:** `frontend/src/components/explorer`, `frontend/src/stores/explorer.js`

## Problem

While exploring the isometric graph, a user pins a node and studies the paths
from it. They often spot something to fix in the underlying `.n4l` document.
Today the only way to edit is to close the explorer, hunt the file down in the
tree, edit, save, then reopen the explorer and re-run the query — losing their
place. We want an in-place "edit this node's source, then return to the graph"
round-trip that keeps the user anchored on the node they care about.

## Goals

- From a pinned node, open its source `.n4l` at the exact line with one keypress.
- Edit and save without leaving the explorer.
- On return, reflect the edit in the graph **only if something actually changed**,
  re-anchored on the same node where possible.
- Reuse the existing node→editor plumbing and CodeMirror extensions. No new
  backend bindings, no new dependencies (stays within the pure-Go / no-new-deps
  constitution — all work is frontend composition).

## Non-goals

- No live/incremental graph updates while typing. The graph only changes on a
  deliberate save-then-return.
- No multi-node or multi-file batch editing. One pinned node at a time.
- No new editing capabilities beyond what the main `SourceTab` editor already
  offers (same language, highlight, and arrow-autocomplete extensions).

## Decisions (from brainstorming)

| Question | Decision |
|---|---|
| What happens to the graph after save + return? | **Re-layout only if dirty.** Unchanged → instant return. Saved a change → rebuild + re-anchor. |
| Visibility while editing? | **Editor overlays a slice** of the graph, graph dimmed behind. |
| Panel placement / size? | **Centered card, ~70%×70%,** over a heavily-dimmed graph (modal-like). Resizable, remembers last size. |
| Open / save / return gesture? | **Pin → Enter** opens the card; **Ctrl+S** saves; **Esc** closes + returns. Node stays pinned. |
| Node can't be resolved to a source line? | **Toast and don't open.** Pin stays. |
| User edited the pinned node's own text (anchor id no longer exists)? | **Land on the refreshed query** (its default anchor) with a toast explaining the node text changed. |

## Existing infrastructure this builds on

- **GraphExplorer overlay** — rendered as `{#if $explorerOpen}` over the
  workbench; already owns a capture-phase keydown handler and the freeze-pin
  concept (`currentAnchorID`, `frozenNodeLabel`).
- **Node identity** — `NodeView.id` is `Chapter::Text` (or just `Text` when the
  chapter is empty). Carries no `file`/`line`.
- **Node→source resolution** — the omnibar's `enrichGraphNodes` already recovers
  `{filePath, line, chapterTitle}` by matching node text against the in-memory
  parsed-file cache (`ParseFileForUI`). The explorer's chapter-keyed id makes
  this match more precise than the omnibar's pure-text version.
- **Editor navigation** — `navigateToLine({filePath, chapterTitle, line, ...})`
  plus `pendingScrollLine` / `pendingScrollNonce` drive cursor + scroll in the
  main editor.
- **Bindings (all exist, unchanged):** `GetFileContent`, `SaveFile`,
  `BuildGraph`, `ExplorerOpen`, `ExplorerReanchor`. Frontend already tracks
  `currentResultID` and `lastDiagnostics.sourceQuery`.
- **CodeMirror extensions** are standalone in `lib/`: `n4l-language.js`,
  `n4l-highlight.js`, `arrow-autocomplete.js`.

## Architecture

The feature is three small frontend units plus wiring in `GraphExplorer`.

### 1. Node-source resolver — `lib/node-source.js`

Pure function extracted from the omnibar's matching loop:

```
resolveNodeSource(nodeId, fileCache) -> { filePath, chapterTitle, line, matchCount } | null
```

- Split `nodeId` into `{ chapter, text }` (`"Chapter::Text"`, or `text` alone).
- Scan `fileCache` (map of `filePath → ParsedFile`) for a chapter whose title
  matches and a `Node` whose trimmed `Text` matches.
- Return the first hit plus `matchCount` (how many files contained an equal
  hit) so the card can show a "1 of N" hint. Return `null` when nothing matches
  (the unresolvable case → caller toasts and aborts).

The omnibar's `enrichGraphNodes` should be refactored to call this same
function, removing the duplicated loop.

### 2. Edit card — `components/explorer/EditCard.svelte`

A self-contained layer, not a reuse of `SourceTab` (which is welded to the
workbench store and owns the singleton `editorAPI`; a second instance would
fight over it). The card is a smaller, independently testable unit that reuses
the already-extracted `lib/` extensions.

- **Props:** `{ filePath, chapterTitle, line, label, anchorId }`.
- **On mount:** `GetFileContent(filePath)` (fresh from disk) → build an
  `EditorView` with the n4l language + highlight + arrow-autocomplete
  extensions; place cursor + selection on `line` and scroll it into view.
- **Header:** filename, node label, and a "1 of N — others exist" hint when
  `matchCount > 1`.
- **Ctrl+S:** `SaveFile(path, content)` (same write + refresh events as
  `SourceTab`, keeping the file tree and background editor in sync). Sets an
  internal `savedAChange` flag.
- **Emits `close`** with `{ savedAChange }`.
- **Sizing:** ~70%×70% centered; resizable; persists last size to a store/local
  layout key.

### 3. Explorer store additions — `stores/explorer.js`

- `editCardOpen` (writable bool).
- `editCardTarget` (writable `{ filePath, chapterTitle, line, label, anchorId } | null`).

### 4. Wiring in `GraphExplorer.svelte`

**Open (Enter):** in the existing capture-phase keydown handler, add an `Enter`
branch active only when a node is pinned, the card is closed, and the user is
not typing in the omnibar/autocomplete. Resolve via `resolveNodeSource`; on a
hit, set `editCardTarget` + `editCardOpen = true`; on `null`, toast
"no editable source line for this node" and do nothing.

**Render:** `{#if $editCardOpen}<EditCard ... on:close={handleCardClose} />{/if}`
as a child layer above the dimmed canvas.

**Esc ladder** becomes three steps (card-close slots in front of today's
two-step): (1) card open → close card; (2) pin active → release pin; (3) else →
close explorer.

## Data flow — the round-trip

```
pin node ──Enter──► resolveNodeSource ──hit──► open EditCard (cursor on line)
                                       └─null─► toast, abort
EditCard ──Ctrl+S──► SaveFile ──► savedAChange = true
EditCard ──Esc/close──► handleCardClose({ savedAChange })
```

`handleCardClose`:

- `savedAChange === false` → hide card. Instant return; pin intact.
- `savedAChange === true`:
  1. Hide card; show the existing `graph-banner--building` banner.
  2. `await BuildGraph()` — reindex so the edit is reflected.
  3. `await ExplorerOpen(lastDiagnostics.sourceQuery, mode)` — fresh `resultID`
     + layout over the rebuilt graph.
  4. Re-anchor:
     - anchor id still present → `ExplorerReanchor` onto it and re-pin.
     - anchor id gone (the pinned node's own text was edited) → land on the
       refreshed query's default anchor and toast "pinned node text changed —
       showing updated graph."

## Error handling & edge cases

- **Unresolvable node** (target-only node, or text not found): toast + abort,
  pin stays. (Decided.)
- **Ambiguous match** (same text+chapter in multiple files): open the first;
  header shows "1 of N — others exist."
- **Pinned node's text edited** (anchor id gone after rebuild): refreshed query
  + toast. (Decided.)
- **Same file open & dirty in the background editor:** the card loads from disk
  and its save wins; on next background focus, show a one-line "background
  editor had unsaved changes — reloaded from disk" notice. We do not worsen the
  pre-existing multi-editor tension.
- **`sourceQuery` is a listing/special query that can't re-run meaningfully:**
  skip the rebuild, re-anchor on the existing result, and toast that a manual
  Ctrl+G may be needed.
- **`BuildGraph()` fails:** surface via the existing `graph-banner--error`;
  leave the pre-edit layout in place.

## Testing

Follows project convention (Go unit tests + a manual quickstart matrix, as
feature 005 did). No backend change → no Go tests.

- **Unit (pure):** `resolveNodeSource` — exact chapter+text hit; ambiguous
  multi-file (returns first + correct `matchCount`); unresolved target-only
  node returns `null`; empty-chapter id form.
- **Manual quickstart matrix:** pin → Enter opens card on the right line;
  Ctrl+S saves; Esc with no change returns instantly; Esc after a change
  rebuilds and re-anchors; editing the pinned node's text lands on refreshed
  query with toast; unresolvable node toasts without opening; Esc ladder order
  (card → pin → explorer).

## Files touched

- `frontend/src/lib/node-source.js` — new pure resolver.
- `frontend/src/components/explorer/EditCard.svelte` — new card component.
- `frontend/src/stores/explorer.js` — `editCardOpen`, `editCardTarget`.
- `frontend/src/components/GraphExplorer.svelte` — Enter/Esc wiring, render
  card, `handleCardClose` round-trip.
- `frontend/src/components/OmniBar.svelte` — refactor `enrichGraphNodes` to call
  the shared resolver.

No Go files change.
