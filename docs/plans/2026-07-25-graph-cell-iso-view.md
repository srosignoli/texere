# Graph cell — Text / Isometric result view + Expand — Implementation Plan

**Goal:** Let the graph cell render its query result as text (current) or as the 2.5D isometric graph, with an Expand button that opens the full explorer.

**Architecture:** Extract the layout pipeline inside `ExplorerService.Open` into a stateless helper; add a `Layout(query, mode)` method + `GraphLayout` binding that never touches the single active session. A new embeddable `IsoGraphView.svelte` composes the existing `explorer/*.js` render modules from a `layout` prop. The graph cell gets a Text|Graph toggle and an Expand that opens the existing explorer via the `explorerQuery`/`explorerOpen` stores.

**Tech Stack:** Go (internal/explorer); Svelte 3.49 + `three` (existing); reuses `explorer/*.js` render modules + the `explorer` layout pipeline. No new deps, no CGo.

## Global Constraints

- No new npm/Go dependencies; no CGo.
- **`GraphLayout` must not mutate `s.active`** — it cannot disturb a live global-explorer session. `Open`'s behavior stays identical (still stores `s.active`).
- `IsoGraphView` reads/writes **no Svelte stores**; it must **dispose** its three.js resources on destroy (multiple graph cells must not leak GPU memory).
- First cut of the inline graph is **pan/zoom + visible labels** (interactive node-pick/reanchor is via Expand → the full explorer).
- Text mode, the global explorer, and `ExplorerOpen`/`Reanchor` behavior are unchanged (regression).
- Frontend has no JS test runner; frontend tasks verify with `cd frontend && npm run build` + a manual test plan. Go uses `go test ./...`.

---

## File Structure

- `internal/explorer/service.go` — MODIFY: extract `layoutForQuery`; add `Layout`.
- `internal/explorer/service_test.go` — MODIFY: add `Layout` tests.
- `app.go` — MODIFY: add `GraphLayout(query, mode)` binding.
- `frontend/src/components/IsoGraphView.svelte` — CREATE.
- `frontend/src/components/notebook/GraphQueryCell.svelte` — MODIFY: Text|Graph toggle + graph render + Expand.
- `docs/specs/2026-07-25-graph-cell-iso-view-manual-test-plan.md` — CREATE (Task 4).

---

## Task 1: Backend — stateless `Layout` + `GraphLayout` binding (TDD)

**Files:**
- Modify: `internal/explorer/service.go`, `internal/explorer/service_test.go`, `app.go`

**Interfaces:**
- Produces: `(*ExplorerService).Layout(query, mode string) (LayoutResult, error)` and the `GraphLayout(query, mode)` Wails binding. Consumed by the graph cell (Task 3). `make build` regenerates the JS binding.

- [ ] **Step 1: Write the failing test**

Read the existing `service_test.go` to reuse its service/fixture setup (how it builds an `ExplorerService` with a ready search service and asserts `Open`). Add a test mirroring that harness:

```go
func TestLayoutMatchesOpenAndDoesNotEvictSession(t *testing.T) {
	svc := newTestServiceWithGraph(t) // reuse the existing harness in service_test.go
	const q = "…a query the fixture matches…"

	// Establish a session via Open, capture its result.
	openOut, err := svc.Open(q, "stacked")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Layout returns an equivalent result (same node/edge counts) …
	layoutOut, err := svc.Layout(q, "stacked")
	if err != nil {
		t.Fatalf("Layout: %v", err)
	}
	if len(layoutOut.Nodes) != len(openOut.Nodes) || len(layoutOut.Edges) != len(openOut.Edges) {
		t.Errorf("Layout nodes/edges = %d/%d, want %d/%d",
			len(layoutOut.Nodes), len(layoutOut.Edges), len(openOut.Nodes), len(openOut.Edges))
	}

	// … and Layout did NOT evict the active session: a Reanchor against the
	// Open result still works.
	if openOut.AnchorID != "" {
		if _, err := svc.Reanchor(openOut.ResultID, openOut.AnchorID, "stacked"); err != nil {
			t.Errorf("Reanchor after interleaved Layout failed (session evicted): %v", err)
		}
	}
}
```

(Adjust the fixture query/helper name to whatever `service_test.go` already provides. If the harness helper differs, use it — the point is: a fixture graph, `Open` then `Layout` then `Reanchor`.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/explorer/ -run TestLayoutMatchesOpen`
Expected: FAIL (`Layout` undefined).

- [ ] **Step 3: Extract `layoutForQuery` and add `Layout`**

In `internal/explorer/service.go`, refactor `Open` to delegate to a new helper (assumes the caller holds `s.mu`), and add `Layout`. Replace the body of `Open` (from `s.mu.Lock()` onward) and add both functions:

```go
func (s *ExplorerService) Open(query, mode string) (LayoutResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs, out, err := s.layoutForQuery(query, mode)
	if rs != nil {
		s.active = rs // keep the session for Reanchor (unchanged behavior)
	}
	return out, err
}

// Layout computes a layout for a query WITHOUT storing it as the active
// session, so notebook graph cells can render a query without evicting the
// global explorer's active ResultSet.
func (s *ExplorerService) Layout(query, mode string) (LayoutResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, out, err := s.layoutForQuery(query, mode)
	return out, err
}

// layoutForQuery is the stateless core of Open: search → build → clamp →
// layout. It never touches s.active; the caller decides whether to store the
// returned ResultSet. Assumes s.mu is held.
func (s *ExplorerService) layoutForQuery(query, mode string) (*ResultSet, LayoutResult, error) {
	if isBlank(query) {
		return nil, LayoutResult{}, ErrEmptyQuery
	}
	if s.search == nil || !s.search.IsReady() {
		return nil, graphUnavailableResult(), nil
	}
	sr, err := s.search.Search(withDefaultLimit(query))
	if err != nil {
		return nil, graphUnavailableResult(), nil
	}
	if sr.GraphUnavailable {
		msg := sr.GraphError
		if msg == "" {
			msg = "Build your graph first (Ctrl+G) to enable graph search."
		}
		out := emptyLayoutResult()
		out.Diagnostics.GraphError = msg
		return nil, out, nil
	}
	rs := BuildResultSet(sr, s.registry, query)
	if len(rs.Nodes) == 0 {
		return rs, emptyLayoutResult(), nil
	}
	truncated := clampResultSet(rs)
	rs.DefaultAnchor = pickDefaultAnchor(rs)
	pass := LayoutFromAnchor(rs, rs.DefaultAnchor, effectiveMode(rs, mode))
	out := s.toLayoutResult(rs, pass)
	out.Diagnostics.MatchCount = matchCount(rs)
	if truncated > 0 {
		out.Diagnostics.Clamped = true
		out.Diagnostics.Truncated = truncated
	}
	return rs, out, nil
}
```

This preserves `Open` exactly: the empty-nodes case still sets `s.active = rs` (rs non-nil), and the graph-unavailable/blank cases still don't (rs nil).

- [ ] **Step 4: Add the `GraphLayout` binding**

In `app.go`, near `ExplorerOpen`, add (mirroring `ExplorerOpen`'s nil-guard):

```go
// GraphLayout returns an isometric layout for a query WITHOUT starting/replacing
// the global explorer session — used by the notebook graph cell's graph view.
func (a *App) GraphLayout(query string, mode string) (explorer.LayoutResult, error) {
	if a.explorerSvc == nil {
		return explorer.LayoutResult{
			Nodes:       []explorer.NodeView{},
			Edges:       []explorer.EdgeView{},
			Territories: []explorer.TerritoryView{},
			Diagnostics: explorer.Diagnostics{GraphError: "Build your graph first (Ctrl+G) to enable graph search."},
		}, nil
	}
	return a.explorerSvc.Layout(query, mode)
}
```

(If `ExplorerOpen`'s nil-guard literal differs, copy its exact `Diagnostics` shape.)

- [ ] **Step 5: Run tests + build (regenerates bindings)**

Run: `go test ./internal/explorer/`
Expected: PASS (new test + existing explorer suite — `Open`/`Reanchor` unchanged).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`; regenerates `frontend/wailsjs/go/main/App.{js,d.ts}` with `GraphLayout`.

- [ ] **Step 6: Confirm the binding**

Run: `grep -n "GraphLayout" frontend/wailsjs/go/main/App.d.ts`
Expected: `export function GraphLayout(...)`.

- [ ] **Step 7: Commit**

```bash
git add internal/explorer/ app.go frontend/wailsjs/
git commit -m "feat(explorer): stateless GraphLayout(query,mode) that doesn't evict the session"
```

---

## Task 2: `IsoGraphView.svelte` — embeddable isometric view

**Files:**
- Create: `frontend/src/components/IsoGraphView.svelte`

**Interfaces:**
- Consumes: the `explorer/*.js` render modules; a `LayoutResult` via the `layout` prop.

**Implementation note:** model this on `GraphExplorer.svelte`'s `onMount` (scene/picker setup), its layout-render block (`buildLayerPlanes` → fit camera → rAF `buildNodeMeshes`/`buildEdgeMeshes`), and `teardownScene` (dispose). Cross-check the exact module call signatures and the `nodeMeshes/edgeMeshes/layerPlanes.dispose()` / `picker.clear()/clearLayers()` / `sceneCtx.dispose()` names against that file; adjust the code below to match if a module differs. Do NOT wire hover/click picking or reanchor (deferred to Expand).

- [ ] **Step 1: Create the component**

Create `frontend/src/components/IsoGraphView.svelte`:

```svelte
<script>
  import { onMount, onDestroy } from 'svelte'
  import { createScene } from './explorer/Scene.js'
  import { createPicker } from './explorer/Picking.js'
  import { buildNodeMeshes } from './explorer/NodeMeshes.js'
  import { buildEdgeMeshes } from './explorer/EdgeMeshes.js'
  import { buildLayerPlanes } from './explorer/LayerPlanes.js'
  import { computeCameraFrame } from './explorer/ReanchorAnimator.js'

  export let layout = null
  export let height = 320

  let wrap, canvas
  let sceneCtx = null
  let picker = null
  let nodeMeshes = null
  let edgeMeshes = null
  let layerPlanes = null
  let ro = null
  let rafId = 0

  $: message = !layout ? '' : (layout.diagnostics && layout.diagnostics.graphError)
    ? layout.diagnostics.graphError
    : (!layout.nodes || layout.nodes.length === 0) ? 'No graph results.' : ''

  function disposeMeshes() {
    nodeMeshes?.dispose(); edgeMeshes?.dispose(); layerPlanes?.dispose()
    picker?.clear?.(); picker?.clearLayers?.()
    nodeMeshes = null; edgeMeshes = null; layerPlanes = null
  }

  function renderLayout(l) {
    if (!sceneCtx || !l || message) return
    disposeMeshes()
    layerPlanes = buildLayerPlanes(sceneCtx.scene, l, sceneCtx.camera, canvas, { opacity: 0.12, mode: 'stacked', picker })
    const t = computeCameraFrame(sceneCtx.camera, l.bounds)
    sceneCtx.camera.position.copy(t.position)
    sceneCtx.camera.zoom = t.zoom
    sceneCtx.camera.lookAt(t.lookAt)
    sceneCtx.camera.updateProjectionMatrix()
    sceneCtx.render()
    if (rafId) cancelAnimationFrame(rafId)
    rafId = requestAnimationFrame(() => {
      rafId = 0
      nodeMeshes = buildNodeMeshes(sceneCtx.scene, l, sceneCtx.camera, canvas, picker, { mode: 'stacked' })
      edgeMeshes = buildEdgeMeshes(sceneCtx.scene, l, nodeMeshes, picker)
      sceneCtx.render()
    })
  }

  // Re-render whenever the layout changes (and once sceneCtx exists after mount).
  $: if (sceneCtx) renderLayout(layout)

  onMount(() => {
    sceneCtx = createScene(canvas, {
      onCameraChange: () => { nodeMeshes?.rescaleLabels?.(); picker?.rescaleAll?.() },
    })
    picker = createPicker(sceneCtx.camera, canvas)
    ro = new ResizeObserver(() => sceneCtx?.render())
    ro.observe(wrap)
  })

  onDestroy(() => {
    if (rafId) cancelAnimationFrame(rafId)
    ro?.disconnect()
    disposeMeshes()
    sceneCtx?.dispose()
    sceneCtx = null; picker = null
  })
</script>

<div class="iso-wrap" bind:this={wrap} style="height:{height}px">
  <canvas bind:this={canvas}></canvas>
  {#if message}<div class="iso-msg">{message}</div>{/if}
</div>

<style>
  .iso-wrap { position: relative; width: 100%; background: #0b1220; border-radius: 6px; overflow: hidden; }
  .iso-wrap canvas { display: block; width: 100%; height: 100%; touch-action: none; }
  .iso-msg { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; color: #7a8ba0; font-size: 13px; padding: 12px; text-align: center; }
</style>
```

- [ ] **Step 2: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds (imports resolve; the module is tree-shaken until Task 3 uses it, but must compile).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/IsoGraphView.svelte
git commit -m "feat(explorer): embeddable IsoGraphView (pan/zoom + labels)"
```

---

## Task 3: Graph cell — Text|Graph toggle + graph render + Expand

**Files:**
- Modify: `frontend/src/components/notebook/GraphQueryCell.svelte`

**Interfaces:**
- Consumes: `GraphLayout` (Task 1), `IsoGraphView` (Task 2), the `explorerOpen`/`explorerQuery` stores.

- [ ] **Step 1: Imports + state**

Add to the imports:

```js
  import { SearchGraph, GraphLayout } from '../../../wailsjs/go/main/App'
  import IsoGraphView from '../IsoGraphView.svelte'
  import { explorerOpen, explorerQuery } from '../../stores/explorer.js'
```

(Merge `GraphLayout` into the existing `SearchGraph` import line.)

Add state near the other `let`s:

```js
  let layout = null
```

- [ ] **Step 2: View toggle + Expand handlers**

Add functions (near `run`):

```js
  function setView(v) {
    if (cell.view === v) return
    cell.view = v
    onChange(cell)
    if (v === 'graph' && cell.query.trim()) run()
  }
  function expand() { explorerQuery.set(cell.query); explorerOpen.set(true) }
```

- [ ] **Step 3: Branch `run()` on the view**

Replace `run()` with:

```js
  async function run() {
    if (running) return
    running = true
    error = ''
    showSug = false
    try {
      if (cell.view === 'graph') {
        layout = await GraphLayout(cell.query, 'stacked')
        // Keep LLM-attach working: derive a bounded text summary from node labels.
        const labels = (layout?.nodes ?? []).map((n) => n.label ?? n.text ?? n.name ?? '').filter(Boolean)
        cell.resultText = labels.join('\n').slice(0, 4000)
        onChange(cell)
      } else {
        result = await SearchGraph(cell.query)
        cell.resultText = summarize(result)
        onChange(cell)
      }
    } catch (e) { error = String(e?.message ?? e); result = null; layout = null; cell.resultText = ''; onChange(cell) }
    finally { running = false }
  }
```

(If `NodeView`'s label field is not `label`/`text`/`name`, check its shape in `internal/explorer/types.go` and use the right field.)

- [ ] **Step 4: Default `cell.view` + markup**

Ensure `cell.view` defaults to text where read (the factory in `notebook.js` doesn't set it; treat absent as text). Add the toggle to the bar (before the Run button) and the Expand button; add the graph result region.

In the `.gq-bar`, before `<button class="gq-run" …>`, add:

```svelte
      <div class="gq-view">
        <button class:sel={(cell.view ?? 'text') === 'text'} on:click={() => setView('text')} disabled={readOnly && false}>Text</button>
        <button class:sel={(cell.view ?? 'text') === 'graph'} on:click={() => setView('graph')}>Graph</button>
      </div>
```

Replace the results block:

```svelte
  {#if error}<div class="gq-error">{error}</div>{/if}
  {#if (cell.view ?? 'text') === 'graph'}
    <div class="gq-graph">
      <div class="gq-graphbar"><button class="gq-expand" on:click={expand} title="Open in full explorer">⛶ Expand</button></div>
      <IsoGraphView {layout} height={320} />
    </div>
  {:else if result}
    <div class="gq-results"><GraphResults {result} /></div>
  {/if}
```

- [ ] **Step 5: Styles**

Add to the `<style>` block:

```css
  .gq-view { display: inline-flex; gap: 2px; }
  .gq-view button { background: #14233a; color: #6b7a8a; border: 1px solid #2a3a4a; border-radius: 6px; padding: 4px 8px; cursor: pointer; font-size: 12px; }
  .gq-view button.sel { color: #e8eaf0; background: #244a72; }
  .gq-graph { border-top: 1px solid #1c2a3a; padding: 6px 8px 8px; }
  .gq-graphbar { display: flex; justify-content: flex-end; margin-bottom: 4px; }
  .gq-expand { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
```

- [ ] **Step 6: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds (pulls `IsoGraphView` into the bundle).

- [ ] **Step 7: Manual smoke**

`PATH="$PATH:$(go env GOPATH)/bin" make build` then `./build/bin/texere`: build the graph (Ctrl+G) if needed; in a Graph cell, switch to Graph, run a query → the isometric render appears; pan/zoom works; Expand opens the full explorer on the same query; switch back to Text → text result.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/notebook/GraphQueryCell.svelte
git commit -m "feat(notebook): graph cell text/graph view toggle + Expand"
```

---

## Task 4: Manual test plan + full verification

**Files:**
- Create: `docs/specs/2026-07-25-graph-cell-iso-view-manual-test-plan.md`

- [ ] **Step 1: Write the manual test plan**

Create the file:

```markdown
# Graph cell — Text/Isometric view + Expand — Manual Test Plan

Prereqs: open a collection; build the graph (Ctrl+G) so queries return results.

## TC-1 Text mode unchanged
1. Add a Graph cell (Text selected by default); run a query.
   - EXPECT: the text/structured result as before.

## TC-2 Graph mode renders the isometric result
1. Toggle to Graph; run a query that matches nodes.
   - EXPECT: the isometric subgraph renders inline with labeled nodes; pan (drag) and zoom (wheel) work.

## TC-3 Expand
1. In Graph mode, click ⛶ Expand.
   - EXPECT: the full isometric explorer opens on the same query (reanchor/sidebar/minimap available).

## TC-4 Persistence + toggle
1. Leave a cell in Graph mode; restart the app, reopen the extract.
   - EXPECT: the cell reopens in Graph mode.
2. Toggle back to Text.
   - EXPECT: the text result shows.

## TC-5 Empty / no graph
1. Run a query with no matches (Graph mode); and try Graph mode before building the graph.
   - EXPECT: a clear "No graph results." / "Build your graph first" message, no crash.

## TC-6 No leak / global explorer intact
1. Add several Graph cells and run them; then open the full explorer from the omnibar and reanchor.
   - EXPECT: cells render independently; the global explorer still works normally (a cell's GraphLayout didn't evict its session).
```

- [ ] **Step 2: Backend suite + full build**

Run: `go test ./...`
Expected: all PASS (includes the new explorer `Layout` test).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`.

- [ ] **Step 3: Execute the manual test plan**

Walk TC-1 … TC-6 in `./build/bin/texere`.

- [ ] **Step 4: Commit**

```bash
git add docs/specs/2026-07-25-graph-cell-iso-view-manual-test-plan.md
git commit -m "docs(test-plan): graph cell text/isometric view manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** stateless `Layout` + `GraphLayout` binding not evicting `s.active` (T1); embeddable `IsoGraphView` composing the existing render modules with dispose-on-destroy (T2); Text|Graph toggle + graph render + Expand + graph-mode `resultText` (T3); manual plan + verification (T4). Deferred per spec: in-cell reanchor/sidebar/minimap (Expand → full explorer).
- **Scope trim vs spec:** the spec listed "pan/zoom + node pick"; this cut ships **pan/zoom + visible labels** and defers interactive node-pick to Expand (the picker's hover/click resolution is intricate to embed safely). Flagged for the human at handoff.
- **Placeholder scan:** no TBD/TODO; every code step complete; the `layoutForQuery` extraction preserves `Open` exactly; T2 instructs cross-checking module signatures against `GraphExplorer.svelte`.
- **Type consistency:** `GraphLayout(query, mode) → explorer.LayoutResult` → `IsoGraphView`'s `layout` prop (`{nodes, edges, layers, bounds, diagnostics}`); `resultText` derived from `layout.nodes[].label`; `cell.view ∈ {text, graph}` (absent = text). `explorerQuery.set`/`explorerOpen.set` is the same open path `MiddleBand` uses.
- **Compile-sequencing:** `GraphLayout` JS binding generated by T1's `make build` before T3 imports it; `IsoGraphView` (T2) tree-shaken until T3 mounts it; T3's build is the real frontend gate.
```
