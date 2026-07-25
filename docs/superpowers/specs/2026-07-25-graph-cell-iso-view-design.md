# Graph cell — Text / Isometric-graph result view + Expand

**Date:** 2026-07-25
**Status:** Approved design
**Feature area:** Processing Notebook (incremental reading) — graph query cell

## Summary

Let the graph query cell render its result as **text** (current) or as the
**2.5D isometric graph**, chosen by a Text | Graph toggle. The graph view reuses
the existing isometric render modules (`explorer/*.js`) in a new embeddable
`IsoGraphView` component fed by a new **stateless** `GraphLayout(query, mode)`
binding (so it doesn't clobber the single-session global explorer). An **Expand**
button opens the full isometric explorer (the existing surface) seeded with the
cell's query, for full-power exploration (reanchor, sidebar, minimap, modes).

## Motivation / grounding

- The query engine is shared: the cell's `SearchGraph` and the explorer's query
  bar both call `search.SearchService.Search(query)`; the explorer additionally
  applies a default `\limit` and builds a laid-out subgraph. So a graph view of
  the cell's query is consistent with the text view and the full explorer by
  construction.
- `ExplorerService` is **single-session** — it caches one active `ResultSet`
  (`s.active`) shared with the global explorer. A cell must therefore use a
  **stateless** layout path that does not touch `s.active`.
- `GraphExplorer.svelte` is bound to singleton stores (reanchor history, sidebar,
  minimap), so it is not directly embeddable — but its rendering is already
  factored into pure modules (`createScene`, `buildNodeMeshes`, `buildEdgeMeshes`,
  `buildLayerPlanes`, `createPicker`) parametrized by `(scene, layoutResult,
  camera, canvas)`. `IsoGraphView` composes those from a `layout` prop.

## Decisions (from brainstorming)

- **Text | Graph toggle** on the graph cell (`cell.view`, persisted; default text).
- **Graph view = an embeddable `IsoGraphView`** built from the existing render
  modules — **pan/zoom + node pick**, no reanchor/sidebar/minimap.
- **Stateless `GraphLayout(query, mode)`** binding (no `s.active` mutation).
- **Expand opens the existing full explorer** (`explorerQuery.set(cell.query);
  explorerOpen.set(true)`) — full features live there.
- **No in-cell reanchor** (single-session server); recentre-by-click is an
  explorer-only capability for this cut.
- **`resultText` for LLM-attach** in graph mode is derived from the layout's node
  labels (bounded), avoiding a second search.

## Scope

**In scope**

- `internal/explorer`: extract the stateless layout computation from `Open`; add
  `Layout(query, mode) (LayoutResult, error)` (no `s.active` write).
- `app.go`: `GraphLayout(query, mode) (explorer.LayoutResult, error)` binding.
- `frontend/src/components/IsoGraphView.svelte` (new): embeddable isometric view
  from a `layout` prop; pan/zoom + node pick; per-instance three.js lifecycle
  (resize, DPR, dispose on destroy).
- `GraphQueryCell.svelte`: Text | Graph toggle (`cell.view`); graph view fetches
  `GraphLayout` and renders `IsoGraphView`; an **⛶ Expand** button opens the full
  explorer with the cell's query; derive `resultText` from layout labels in graph
  mode.

**Out of scope**

- In-cell reanchor / sidebar / minimap / layout-mode switching (use Expand →
  full explorer).
- Refactoring `GraphExplorer.svelte` to be embeddable, or multi-session in
  `ExplorerService`.
- Persisting a rendered graph thumbnail; animating skeleton reveal in-cell
  (a single static-on-load render is fine).

## Data model (`cells_json`)

The graph cell gains one field:

```json
{ "id":"…", "type":"graph", "query":"…", "resultText":"…", "view":"text" }
```

- `view` — `"text"` (default/absent) or `"graph"`; persisted so the cell reopens
  in the chosen mode. `query`/`resultText` unchanged.

## Architecture & components

```
internal/explorer/service.go   # extract stateless layout core; add Layout(query, mode)
app.go                         # GraphLayout(query, mode) binding (near ExplorerOpen)
frontend/src/components/
  ├── IsoGraphView.svelte      # NEW: embeddable isometric view from a `layout` prop
  └── notebook/GraphQueryCell.svelte  # Text|Graph toggle + graph render + Expand
```

### Backend — stateless layout

- Extract a helper, e.g. `layoutForQuery(query, mode) (*ResultSet, LayoutResult)`,
  holding the existing pipeline: `Search(withDefaultLimit(query))` → graph-
  unavailable / empty handling → `BuildResultSet` → `clampResultSet` →
  `pickDefaultAnchor` → `LayoutFromAnchor` → `toLayoutResult` (+ diagnostics).
- `Open` calls it under `s.mu`, then assigns `s.active = rs` (unchanged behavior
  for the global explorer's reanchor).
- New `Layout(query, mode) (LayoutResult, error)` calls the same helper under
  `s.mu` and **discards the ResultSet** — no `s.active` write, so it cannot
  disturb a live global-explorer session. Returns the same graph-unavailable /
  empty diagnostics shape.
- Binding `GraphLayout(query, mode)` → `a.explorerSvc.Layout(query, mode)`
  (guards `explorerSvc == nil` like `SearchGraph`/`ExplorerOpen`).

### Frontend — `IsoGraphView.svelte`

- Props: `layout` (a `LayoutResult`); optional `height`.
- On mount / on `layout` change: `createScene(canvas)`, `createPicker(camera,
  canvas)`, `buildLayerPlanes(scene, layout, camera, canvas)`, `buildNodeMeshes(
  scene, layout, camera, canvas, picker)`, `buildEdgeMeshes(scene, layout,
  nodeMeshes, picker)`; render. Pan/zoom via the same Scene camera controls the
  explorer uses; node pick highlights/shows the node label (no reanchor).
- Handles an empty / graph-unavailable layout by showing the diagnostics message
  (e.g. "Build your graph first") instead of an empty canvas.
- `ResizeObserver` + DPR-aware sizing; on `onDestroy`, dispose the renderer,
  geometries, materials, and the picker (multiple graph cells must not leak GPU
  resources). No Svelte stores are read or written.

### Graph cell — `GraphQueryCell.svelte`

- Add `cell.view` (default `'text'`). A small **Text | Graph** toggle in the bar,
  plus an **⛶ Expand** button (shown in graph view).
- `run()`:
  - **text view:** unchanged — `SearchGraph(cell.query)` → `result` → `GraphResults`;
    `cell.resultText = summarize(result)`.
  - **graph view:** `layout = await GraphLayout(cell.query, mode)` → render
    `<IsoGraphView {layout} />`; set `cell.resultText` from the layout node labels
    (bounded ~4 KB) so LLM-attach still has content.
  - Switching the toggle re-runs for the newly selected view (or renders the
    already-fetched data if present).
- **Expand:** `import { explorerOpen, explorerQuery } from stores/explorer.js`;
  on click `explorerQuery.set(cell.query); explorerOpen.set(true)` — the existing
  `GraphExplorer` (mounted in `App.svelte`) takes over full-screen.
- `mode` defaults to `'stacked'` (matching the explorer default).

## Interaction

- Toggle Text|Graph switches the result region; the chosen view persists on the
  cell. Run (Enter / Run button) fetches for the active view.
- Graph view shows the inline isometric render (pan/zoom, click a node to
  highlight). Expand hands off to the full explorer for reanchor etc.
- Read-only notebooks: the toggle/Expand still work for viewing; no query edit.

## Error handling

- Graph not built / unavailable → `GraphLayout` returns a diagnostics message;
  `IsoGraphView` shows it (mirrors the explorer's "Build your graph first").
- Empty result → an empty-state message in the graph region.
- Query/binding error → the existing `error` line in the cell.
- `explorerSvc == nil` (no collection) → `GraphLayout` returns a graph-unavailable
  result, same as `SearchGraph`.

## Testing

- **Go (`internal/explorer`):** a test that `Layout(query, mode)` returns the same
  `LayoutResult` (nodes/edges/diagnostics) as `Open(query, mode)` for a fixture
  graph, **and** that `Layout` does **not** mutate `s.active` (a subsequent
  `Reanchor` against a prior `Open`'s result still works — i.e. an interleaved
  `Layout` call didn't evict the session). Graph-unavailable path covered.
- **Frontend:** no JS test runner — `cd frontend && npm run build` + a manual test
  plan: toggle a graph cell to Graph, Run a query → the isometric render appears;
  pan/zoom/pick work; Expand opens the full explorer on the same query; toggling
  back to Text shows the text result; a cell in Graph mode persists across restart;
  no leak/slowdown with several graph cells.
- Full build: `go test ./...` and `make build`.

## Success criteria

- A graph cell can show its result as text or as the isometric graph, persisted.
- The inline graph reuses the existing render modules; pan/zoom + node pick work;
  several graph cells coexist without leaking or clobbering the global explorer.
- Expand opens the full explorer on the cell's query.
- Text mode and the global explorer are unchanged; no new dependencies; no CGo.

## Dependencies

- **New:** none. Backend reuses the existing `explorer` pipeline; the frontend
  reuses `three` (already a dep) and the `explorer/*.js` render modules.
- **Reused:** `search.SearchService.Search`; the `explorer` layout pipeline
  (`BuildResultSet`/`clampResultSet`/`LayoutFromAnchor`/`toLayoutResult`); the
  `explorer/*.js` render modules; the `explorerOpen`/`explorerQuery` stores; the
  `cellRegistry`/`cells_json` persistence path.
