---
description: "Tasks for 006-isometric-graph-explorer"
---

# Tasks: 2.5D Isometric Graph Explorer

**Input**: Design documents from `/docs/superpowers/specs/006-isometric-graph-explorer/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are REQUIRED for the Go layout engine per the project Constitution (§Development Workflow: "All public APIs and scheduling algorithms MUST have unit tests"). They are NOT generated for the frontend; UI behavior is verified by [quickstart.md](./quickstart.md) scenarios.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested independently. Phases run in priority order from spec.md.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story this task belongs to (US1, US2, …) — required for story phases only
- All paths are absolute or repo-root-relative; the Go module root is `/home/simone/projects/texere/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and skeleton directories.

- [X] T001 Add `three` (≈r160, see research.md R-1) to `frontend/package.json` as a runtime dependency, then run `npm install --prefix frontend` to update lockfile.
- [X] T002 Create the Go package directory `internal/explorer/` with a `doc.go` containing a one-paragraph package summary referencing spec.md.
- [X] T003 Create the frontend directory `frontend/src/components/explorer/` (will hold the Three.js modules).
- [X] T004 [P] Update `frontend/src/components/explorer/.gitkeep` (empty placeholder so the dir is tracked) — and confirm the existing top-level `Makefile` `test` target picks up `./internal/explorer/...`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Types, IPC stubs, and frontend shell that every user story needs. No user story work can start until this phase is complete.

- [X] T005 Define `MetaType` enum (`MetaUnclassified`, `MetaContains`, `MetaLeadsTo`, `MetaProperties`, `MetaSimilarTo`) and `parseMetaType(string) MetaType` in `internal/explorer/classify.go`. The `parseMetaType` body classifies registry meta-type strings into the enum (rationale in research.md R-3).
- [X] T006 Define wire types (`LayoutResult`, `NodeView`, `EdgeView`, `TerritoryView`, `Vec2J`, `Diagnostics`, `LayoutBounds`) in `internal/explorer/types.go`. JSON tags MUST match `contracts/layout-types.md` byte-for-byte.
- [X] T007 In the same `internal/explorer/types.go`, add the internal types (`ResultSet`, `ResultNode`, `ResultEdge`, `LayoutPass`, `Vec3`, `TerritoryPolygon`) per data-model.md §2.2–2.3. Keep these unexported; only the wire types from T006 cross the IPC boundary.
- [X] T008 Implement `BuildResultSet(searchOutput *search.SearchResult, registry *content.SSTRegistry) (*ResultSet, error)` in `internal/explorer/neighborhood.go`. The function turns feature 004's existing search output into a deduped, NodePtr-keyed subgraph, classifies every edge via T005's `parseMetaType`, and computes in-degree per node. Stable iteration: sort by `NodePtr` everywhere.
- [X] T009 Implement `ExplorerService` in `internal/explorer/service.go` with a mutex-protected single-result cache (one active `ResultSet`). Add constructor `NewExplorerService(searchSvc *search.SearchService, registry *content.SSTRegistry)` and lifecycle methods that begin as stubs (`Open`, `Reanchor`, `Close`) returning empty `LayoutResult`s.
- [X] T010 Add Wails-bound methods `ExplorerOpen(query string) (LayoutResult, error)`, `ExplorerReanchor(resultID string, anchorID string) (LayoutResult, error)`, `ExplorerClose() error` on `App` in `app.go`. They delegate to the `ExplorerService` instance held on `App` (instantiate alongside the existing `searchSvc` initialization). Honor the error model in `contracts/layout-ipc.md` §4.
- [X] T011 Create `frontend/src/stores/explorer.js` exporting Svelte stores: `explorerOpen` (boolean), `currentLayout` (LayoutResult | null), `currentResultID` (string | null), `currentAnchorID` (string | null), and an `anchorHistory` store with `{ back: [], forward: [], cap: 50 }` per data-model.md §2.5. No history mutation logic yet — that lands in US6.
- [X] T012 Create `frontend/src/components/GraphExplorer.svelte` — a top-level component that mounts a `<canvas>` of full container size, owns a Three.js Scene/Camera/Renderer via T013, and reacts to `currentLayout` to render. For now it shows an empty canvas with a "no result loaded" status text.
- [X] T013 [P] Create `frontend/src/components/explorer/Scene.js` exporting `createScene(canvas)` which sets up an orthographic camera at the fixed 35° isometric vantage (research.md R-9), a custom pan-only pointer controller, scroll-wheel zoom with `zoomMin=0.25` / `zoomMax=4.0` placeholder bounds, and an explicit guarantee that no code path mutates `camera.quaternion`. Pinch and ctrl+wheel trigger zoom (not rotation). Returns `{ scene, camera, renderer, dispose }`.
- [X] T014 [P] Create `frontend/src/components/explorer/Picking.js` exporting `createPicker(camera)` and `addPickTarget(pickingScene, nodeID, worldPosition, screenDiameter=24)`. Uses a dedicated `THREE.Layer` for raycasting; the picking mesh is invisible and sized in screen space (recompute on `camera.zoom` change). No node-rendering logic — that's US1.
- [X] T015 Wire `GraphExplorer.svelte` to call `window.go.main.App.ExplorerOpen` on mount with the current chapter as the query (placeholder until launch UX in US1 T028) and to call `ExplorerClose` on destroy. The result is stored in `currentLayout` for downstream phases to consume.

**Checkpoint**: Foundation ready — IPC contract is callable end-to-end, Three.js scene renders an empty world with a locked camera. User story phases can begin.

---

## Phase 3: User Story 1 — Read the shape of a knowledge graph at a glance (Priority: P1) 🎯 MVP

**Goal**: Open the explorer with a query and see nodes positioned along the four Semantic Compass axes (containers above, causal flow on X, properties +Z, analogies -Z), with the camera locked at the isometric angle and pan/zoom working.

**Independent Test**: With a test collection containing all four meta-types loaded, open the explorer; verify by spatial position alone (no labels needed) which nodes are containers, sequence successors, properties, and analogies. Verify the camera cannot rotate.

### Layout engine (Go)

- [X] T016 [P] [US1] Flesh out edge meta-type classification in `internal/explorer/classify.go`: read `registry.Arrows[arrowCode].MetaType`, normalize case/whitespace, and return the enum from T005. Inverse arrows flip the sign of their assigned axis (research.md R-3).
- [X] T017 [P] [US1] Implement anchor-relative typed BFS in `internal/explorer/neighborhood.go`: `computeDistances(rs *ResultSet, anchor SST.NodePtr) map[SST.NodePtr]int` and per-axis adjacency lists (parents, children, causal forward, causal back, properties, analogies). Stable order via NodePtr sort.
- [X] T018 [P] [US1] Implement containment cycle detection in `internal/explorer/cycle.go`: `breakCycles(parents, children map[SST.NodePtr][]SST.NodePtr) (cycles map[SST.NodePtr]bool)`. Algorithm: DFS; on a back-edge to an ancestor, demote the higher NodePtr in the cycle to a child position (lowest-NodePtr wins, research.md R-4). Returns the set of cycle-participating NodePtrs for `LayoutPass.CycleNodes`.
- [X] T019 [US1] Implement Semantic Compass position assignment in `internal/explorer/layout.go` (`LayoutFromAnchor(rs *ResultSet, anchor SST.NodePtr) *LayoutPass`). Steps: anchor at origin; Y from containment depth (parents +Y, children -Y, integer levels with 1.5 unit spacing); X from causal BFS direction; +Z for properties, -Z for analogies; sibling spread along the unused axes to avoid colocation. Handle unclassified edges by leaving the node at its current axial position (no contribution). Return populated `LayoutPass` (Positions, NodeSizes default 1.0 for now, Territories empty for now, CycleNodes from T018, ComputedAt set).
- [X] T020 [US1] Implement default anchor selection in `internal/explorer/service.go` (`pickDefaultAnchor(rs *ResultSet) SST.NodePtr`). Rule: if the originating query had a target node, use it; otherwise pick the highest-in-degree node, breaking ties by lowest NodePtr (FR-008).
- [X] T021 [US1] Wire the full `ExplorerOpen` path in `internal/explorer/service.go` and `app.go`: delegate the query to the existing `searchSvc.Search`, build the `ResultSet` via T008, evict any prior cache, run `LayoutFromAnchor` for the default anchor, and emit the `LayoutResult` per `contracts/layout-types.md`. Populate `Diagnostics.LayoutMs`. If `searchSvc` is unavailable or returns `GraphUnavailable`, emit an empty `LayoutResult` with `Diagnostics.GraphError = "Build your graph first (Ctrl+G) to enable graph search."` (no IPC error).

### Layout engine tests (Go — required by Constitution)

- [X] T022 [P] [US1] Table-driven axis assignment test in `internal/explorer/layout_test.go`: a fixture graph with one edge of each meta-type from a single anchor; assert containment yields +Y or -Y, causal yields ±X, property +Z, analogy -Z, with stable spacing.
- [X] T023 [P] [US1] Determinism / snapshot test in `internal/explorer/layout_test.go`: build a 25-node fixture, call `LayoutFromAnchor` 100 times, assert all 100 outputs are identical (use `reflect.DeepEqual` over the `LayoutPass`).
- [X] T024 [P] [US1] Cycle-break test in `internal/explorer/cycle_test.go`: feed a 3-cycle (A→B→C→A via "contains") and assert no infinite loop, completion within 100 ms, lowest NodePtr emerges as the cycle root, and `CycleNodes` contains all three.
- [X] T025 [P] [US1] Edge-case test in `internal/explorer/layout_test.go`: results with zero edges of one or more meta-types still produce valid `LayoutPass` with empty axes (FR-009) and the anchor at the origin.

### Frontend rendering (Three.js + Svelte)

- [X] T026 [P] [US1] Create `frontend/src/components/explorer/NodeMeshes.js` exporting `buildNodeMeshes(scene, layoutResult)`. For each `NodeView`, create a `THREE.Mesh` with a sphere geometry of radius `0.4 * layoutResult.nodes[i].size` (uniform 1.0 in MVP), positioned at `(x, y, z)`. Attach a child `THREE.Sprite` for the label using a canvas texture rendering `node.label` in the project's existing UI font; placeholder constant world size for now (proper billboarding lands in US2).
- [X] T027 [P] [US1] Create `frontend/src/components/explorer/EdgeMeshes.js` exporting `buildEdgeMeshes(scene, layoutResult)`. For each `EdgeView`, build a line between `nodes[srcId]` and `nodes[dstId]` using one of four `THREE.LineBasicMaterial` / `THREE.LineDashedMaterial` styles per `meta`: `leadsto` solid+thick+arrow head, `contains` solid+pillar-thickness, `properties` dashed, `similarto` dotted. Unclassified edges render as a thin neutral gray line. Edge endpoints are looked up by `srcId` / `dstId`.
- [X] T028 [US1] Implement skeleton-first reveal in `GraphExplorer.svelte`: on receiving a `LayoutResult`, immediately add a ground plane `THREE.PlaneGeometry` sized to `bounds` and a single neutral territory placeholder (replaced by US4); then schedule node + edge mesh creation in batches keeping each frame ≤8 ms (research.md R-7). Stagger node fade-ins by `distanceFromAnchor` with a 200 ms tween. Edges build alongside their target nodes (FR-032c).
- [X] T029 [US1] Add an "Open Explorer" launch entry point in `frontend/src/components/MiddleBand.svelte`: a button that sets `explorerOpen = true` and calls `ExplorerOpen` with the current chapter as the query. Esc closes the explorer (`explorerOpen = false`, `ExplorerClose`). Verify that no input device or gesture rotates the camera (right-click drag, two-finger trackpad twist, etc. — all should be no-ops or pan).
- [ ] T030 [US1] Run the US1 quickstart scenarios manually against a test collection. Capture any regressions; fix or open issues.

**Checkpoint**: P1 MVP — Open the explorer, see a 4-axis isometric layout with locked camera, basic spheres and styled edges. Story 1 is fully testable independently.

---

## Phase 4: User Story 2 — Read every node label, no matter where the camera is (Priority: P1)

**Goal**: Labels remain sharp and front-facing at every camera position and zoom level.

**Independent Test**: Open a 30+ node result, pan to corners, zoom from 25% to 400%; verify every label stays legible and never appears edge-on.

- [X] T031 [US2] Replace placeholder label rendering in `frontend/src/components/explorer/NodeMeshes.js` with proper billboarded `THREE.Sprite` labels using a higher-resolution canvas texture and constant on-screen pixel size (recompute sprite scale on zoom change in `Scene.js`'s zoom handler).
- [X] T032 [US2] Tighten zoom bounds in `frontend/src/components/explorer/Scene.js`: choose `zoomMin` and `zoomMax` so labels never collapse below ~12 px on-screen and never balloon disproportionately. Add brief comments explaining the bounds.
- [ ] T033 [US2] Manually verify US2 quickstart at extreme pan/zoom positions on the test collection.

**Checkpoint**: P1 MVP+labels — fully readable at all zoom levels.

---

## Phase 5: User Story 3 — Identify landmark concepts immediately (Priority: P2)

**Goal**: High-centrality nodes appear as visibly larger landmarks; uniform centrality produces uniform sizes.

**Independent Test**: With a graph where one node has 10× the average in-degree, open the explorer and verify that node visibly dominates without being screen-filling; with a uniform graph, sizes look equal.

- [X] T034 [P] [US3] Implement log-scale centrality helper `ScaleSizes(rs *ResultSet, minSize, maxSize float32) map[SST.NodePtr]float32` in `internal/explorer/centrality.go`. Formula: `size = minSize + (maxSize - minSize) * log1p(inDegree) / log1p(maxInDegree)`; floor at `minSize` (>0) so zero-in-degree nodes are visible.
- [X] T035 [P] [US3] Unit test in `internal/explorer/centrality_test.go`: assert log curve produces visible separation at 10× ratios, both bounds enforced, and zero-in-degree → exactly `minSize`.
- [X] T036 [US3] Wire `ScaleSizes` into `LayoutPass.NodeSizes` and emit each `NodeView.size` accordingly in `internal/explorer/layout.go` / `service.go`.
- [X] T037 [US3] Update `frontend/src/components/explorer/NodeMeshes.js` to scale the visible sphere radius from `NodeView.size`. Picking-mesh size from T014 must remain exactly 24 px screen-space (FR-014b).

---

## Phase 6: User Story 4 — See where contexts live in the landscape (Priority: P2)

**Goal**: The ground plane is divided into colored territories per primary context; cross-context relationships visibly bridge zones.

**Independent Test**: Query a result spanning 3 contexts with at least one cross-context edge; verify three distinct ground-plane territories, nodes float over their context, and the cross-context edge spans the gap distinctly. Single-context queries show one full territory.

- [X] T038 [US4] Implement convex-hull territory generation in `internal/explorer/territory.go`: `BuildTerritories(rs *ResultSet, positions map[SST.NodePtr]Vec3) []TerritoryPolygon`. Group nodes by primary context (first declared tag), Graham-scan their (x,z) positions, dilate by 0.5 world units, shrink overlapping pairs to maintain a buffer gap (research.md R-5). Single-node groups produce a hex disc; single-context results produce one large polygon spanning bounds (FR-021).
- [X] T039 [P] [US4] Unit test in `internal/explorer/territory_test.go`: deterministic vertex order, three contexts → three polygons, single context → one polygon, non-overlapping output.
- [X] T040 [US4] Wire `BuildTerritories` into `LayoutPass.Territories`; emit `TerritoryView` with `colorSlot` (palette index 0..N-1, sorted contexts) and counterclockwise vertices in `LayoutResult` (research.md R-5).
- [X] T041 [P] [US4] Create `frontend/src/components/explorer/Territories.js` exporting `buildTerritoryMeshes(scene, layoutResult, theme)`. For each `TerritoryView`, build a `THREE.ShapeGeometry` from vertices, apply a semi-transparent fill from the theme's territory palette indexed by `colorSlot`. Replace the neutral placeholder territory from T028.
- [X] T042 [US4] Update `frontend/src/components/explorer/EdgeMeshes.js` to render edges with `EdgeView.crossesContexts === true` distinctly (e.g., elevated arc or extra glow) so cross-context bridges are visually identifiable (FR-020).

---

## Phase 7: User Story 6 — Pivot exploration around a new anchor (Priority: P2)

**Goal**: Click any node to re-anchor; smooth animated layout transition; browser-style history with Alt+←/→ and mouse buttons 4/5; search-driven re-anchor via Ctrl+Shift+K.

**Independent Test**: With a result loaded, click a non-anchor node — the layout reorganizes around it within 1 s. Press Alt+ArrowLeft — prior anchor returns. Press Alt+ArrowRight — forward anchor returns. Press Ctrl+Shift+K, type a node name in the current result, Enter — explorer re-anchors. Same with an out-of-result name — full new query, history cleared.

### Backend (Go)

- [X] T043 [US6] Implement `ExplorerReanchor(resultID, anchorID string) (LayoutResult, error)` in `internal/explorer/service.go`: validate `resultID == cache.ID`, validate `anchorID` is in `cache.NodeIndex`, short-circuit identical-anchor with byte-identical positions (FR-025a), otherwise run `LayoutFromAnchor` for the new anchor and return the new `LayoutResult`. Errors per `contracts/layout-ipc.md`: `"stale resultId"`, `"anchor not in result"`.
- [X] T044 [P] [US6] Service tests in `internal/explorer/service_test.go`: stale-resultID error path, anchor-not-in-result error path, same-anchor short-circuit produces deterministic output, different-anchor produces stable output across repeated calls.

### Frontend (Three.js + Svelte)

- [X] T045 [P] [US6] Create `frontend/src/components/explorer/ReanchorAnimator.js` exporting `tweenLayout(scene, prevLayout, nextLayout, durationMs=900)`. Tweens node positions, edge endpoints, and camera framing from `prevLayout` to `nextLayout`. The tween is cancellable; calling again before completion picks up from the current interpolation values (FR-025b).
- [X] T046 [P] [US6] Create `frontend/src/components/explorer/AnchorHistory.js` implementing the state machine in data-model.md §2.5 (`pushAnchor`, `back`, `forward`, `clear`, capped at 50). Exposes a Svelte-compatible store; mutations are pure on the `{back, forward, current}` triple.
- [X] T047 [US6] Wire primary-click on a node to re-anchor in `frontend/src/components/GraphExplorer.svelte`: raycast through `Picking.js` → if a node is hit and `nodeID !== currentAnchorID`, push the current anchor to `AnchorHistory`, call `ExplorerReanchor(resultID, nodeID)`, run `tweenLayout` with the response. Ignore clicks on the current anchor (FR-025a) and clicks on still-fading-in nodes (edge case).
- [X] T048 [US6] Bind `Alt+ArrowLeft` / `Alt+ArrowRight` and `pointerdown` `button === 3` / `button === 4` (mouse back/forward) in `GraphExplorer.svelte` to `AnchorHistory.back` / `forward`, then call `ExplorerReanchor` and `tweenLayout` with the new anchor. No-op at boundaries (FR-026e); cancel/restart animation if mid-flight (FR-026d).
- [X] T049 [US6] Modify `frontend/src/components/OmniBar.svelte` so that when `explorerOpen` is `true` and the omnibar is in graph mode (`Ctrl+Shift+K`), Enter on a result dispatches: if the selected node ID is in the current `LayoutResult` node set, call `ExplorerReanchor` and push to history; otherwise call `ExplorerOpen(newQuery)` and clear `AnchorHistory` (FR-026h–j). The standard editor-navigation Enter path must remain when the explorer is closed.
- [X] T050 [US6] Clear `AnchorHistory` in `frontend/src/stores/explorer.js` whenever `ExplorerOpen` returns a new `resultID` (FR-026f).
- [ ] T051 [US6] Walk the US6 quickstart scenarios manually: click re-anchor with animation, history back/forward to limits, mid-animation cancel/restart, search-driven in-result re-anchor, search-driven out-of-result new query.

---

## Phase 8: User Story 5 — Trace a specific path through the landscape (Priority: P3)

**Goal**: Hover a node to emphasize it, brighten its connected edges, and dim the rest within 50 ms. Releasing hover restores defaults within 50 ms.

**Independent Test**: With a 50+ node result, hover any node and verify highlight engages within 50 ms; verify connected edges are emphasized and unrelated nodes/edges are dimmed but still visible. Hovering small (low-centrality) nodes works thanks to the 24 px hit area from T014.

- [X] T052 [US5] Create `frontend/src/components/explorer/Highlight.js` exporting `setHovered(scene, hoveredNodeID)`. Implementation: maintain a single set of "neighbor-of-hover" IDs derived from the current `LayoutResult.edges`; mutate node-mesh material `opacity` (or shader uniform) for dimmed vs prominent states; never allocate per frame.
- [X] T053 [US5] Wire hover detection in `frontend/src/components/GraphExplorer.svelte` via the `Picking.js` raycaster (24 px screen-space hit area from T014). On `pointermove`, call `setHovered(...)` with the topmost picked node ID or `null`. Engage and disengage MUST complete within 50 ms (verify via DevTools Performance tab during quickstart).
- [ ] T054 [US5] Manually verify US5 quickstart on a dense (≥50-node) result; ensure smallest log-scaled nodes are still effortless to hover (24 px hit area).

---

## Phase 9: User Story 7 — Stay oriented in a complex landscape (Priority: P3)

**Goal**: A permanent compass shows axis labels; a mini-map shows territories + viewport; reset-view returns the camera to fit-to-result framing.

**Independent Test**: Open a 100+ node result, pan far away, verify compass and mini-map are visible and accurate, and the reset-view shortcut returns within 1 s. After re-anchoring several times, reset returns to the *current* anchor's framing.

- [X] T055 [P] [US7] Create `frontend/src/components/Compass.svelte` — a small floating overlay (e.g., bottom-right) showing the four meta-type axes with their labels (Contains / Leads to / Properties / Similar to). Always visible.
- [X] T056 [P] [US7] Create `frontend/src/components/MiniMap.svelte` — a small floating overlay rendering ONLY territory polygons (no nodes, FR-028a) plus a rectangle showing the camera's current viewport relative to `LayoutResult.bounds`. Update the rectangle on every pan/zoom (subscribe to camera changes); rebuild territory shapes only when the underlying `LayoutResult` changes (FR-028b).
- [X] T057 [P] [US7] Create `frontend/src/components/ExplorerLegend.svelte` — a toggleable overlay (e.g., "?" hotkey or button) listing the four edge styles, their colors (theme-resolved), and a one-line description per meta-type.
- [X] T058 [US7] Add a reset-view keybind (default `R` or `Home`) in `GraphExplorer.svelte` that animates the camera to fit-to-result framing for the *current* anchor within 1 s (FR-029, US6 acceptance scenario 5).

---

## Phase 10: Polish & Cross-Cutting Concerns

- [X] T059 Implement result-size clamping in `internal/explorer/service.go` (FR-034): when `len(rs.Nodes) > 500` or `len(rs.Edges) > 1500`, deterministically drop the highest-NodePtr nodes (and their edges) until under the cap; populate `Diagnostics.Clamped = true` and `Diagnostics.Truncated = droppedCount`.
- [X] T060 [P] Add a layout benchmark in `internal/explorer/layout_bench_test.go`: `BenchmarkLayout_200`, `_500`, `_1500` over fixture graphs. Add a guard `func TestLayoutBudget500(t *testing.T)` asserting <100 ms wall-clock for 500 nodes on the test runner.
- [X] T061 [P] Empty-state and "graph not built" UI in `frontend/src/components/GraphExplorer.svelte`: when `LayoutResult.diagnostics.graphError` is non-empty, render the message as a centered banner. When the result is empty (no error, zero nodes), render a "no results" empty state. When `Diagnostics.Truncated > 0`, show a small "Showing 500 of N nodes — refine your query" badge.
- [X] T062 [P] Cycle-warning visual in `frontend/src/components/explorer/NodeMeshes.js`: for nodes where `NodeView.cycleWarning === true`, attach a small warning glyph sprite adjacent to the node mesh.
- [X] T063 [P] Theme palette wiring: define a `territoryPalette` and `edgePalette` in `frontend/src/stores/explorer.js` that resolve from CSS variables / the existing app theme; verify contrast in both light and dark themes (FR-012, FR-035) by manual inspection.
- [X] T064 Constitution amendment PR (governance task): author a separate commit/PR amending `.specify/memory/constitution.md` to (a) replace `Cytoscape.js` with `Three.js` in the Development Workflow tech-stack table, (b) replace the `Graph explorer initial render < 500 ms` row with the two-phase budget (skeleton ≤16 ms, full settle ≤2 s), per Complexity Tracking in plan.md.
- [ ] T065 Final manual quickstart pass against ALL 7 user stories on a real test collection; capture regressions, file follow-ups.

---

## Dependencies

### Phase ordering

- Phase 1 (Setup) → Phase 2 (Foundational) → all user-story phases.
- **Phase 3 (US1)** is the MVP slice. Phases 4–9 each layer on top.
- Phase 4 (US2 labels) **must follow Phase 3** — it modifies the same `NodeMeshes.js` and `Scene.js` files.
- Phase 5 (US3 centrality) — independent of Phases 4, 6, 7, 8, 9. Could land in any order after Phase 3, but file overlap with `NodeMeshes.js` makes it serial after Phase 4 if both ship in the same release.
- Phase 6 (US4 territories) — independent of Phases 4, 5, 7, 8, 9. Modifies `Territories.js` (new) and `EdgeMeshes.js` (small change). Can run parallel with Phase 5 across two devs.
- Phase 7 (US6 re-anchor) — depends on Phase 3's `NodeMeshes`/`EdgeMeshes` plus the `Picking.js` foundational layer. Independent of Phases 4, 5, 6, 8, 9.
- Phase 8 (US5 hover) — depends on the `Picking.js` foundational layer. Independent of all other story phases.
- Phase 9 (US7 orientation) — independent overlay components; depends only on `LayoutResult` shape from Phase 3.
- Phase 10 (Polish) — runs after the relevant story phase that creates the surface being polished.

### Within-phase dependencies (notable)

- T019 (layout) needs T016 + T017 + T018.
- T021 (full ExplorerOpen path) needs T019 + T020.
- T028 (skeleton-first reveal) needs T026 + T027 + T013.
- T031 (billboarded labels) modifies code authored by T026.
- T037 visual node sizing needs T026 + T036.
- T040 emit territories needs T038.
- T041 territory render needs T040.
- T043 ExplorerReanchor needs T021's cache to be active.
- T047 click-to-reanchor needs T043 + T045 + T046 + T014.
- T049 omnibar Enter dispatch needs T046 + T050 (history clearing on new query).
- T053 hover wiring needs T052 + T014.

### Parallel opportunities

- **Phase 2 (Foundational)** — T013 and T014 are in different files and can run in parallel.
- **Phase 3 (US1)** — Backend trio (T016/T017/T018) all parallel. Frontend pair (T026/T027) parallel. Tests (T022/T023/T024/T025) all parallel after their implementations land.
- **Phase 5 (US3)** — T034 and T035 parallel.
- **Phase 6 (US4)** — T039 (test) and T041 (territory render) parallel after T040.
- **Phase 7 (US6)** — T044 (service test), T045 (animator), T046 (history) all parallel; wiring tasks (T047–T050) serialize after them.
- **Phase 9 (US7)** — T055/T056/T057 are three independent overlay components, all parallel.
- **Phase 10** — T060/T061/T062/T063 are independent concerns and can land in parallel; T064 is a separate PR; T065 is the final sweep.

---

## Implementation Strategy

### MVP — ship Phases 1+2+3+4

That gets you: opens, renders the four-axis isometric layout, labels stay readable, camera locked, pan/zoom works. Stop here, demo, gather feedback.

### Increment 2 — Phase 5 + 6 (centrality + territories)

These two are independent and can be parallelized across devs. Together they deliver the visual "landmark" and "district" experience that defines the explorer's character.

### Increment 3 — Phase 7 (re-anchor + history)

The exploration verb. Demoable as soon as it lands.

### Increment 4 — Phase 8 + 9 (hover + orientation aids)

P3 polish that makes dense graphs comfortable to navigate.

### Increment 5 — Phase 10 (cross-cutting polish + constitution amendment)

Includes T064 (the constitution amendment PR), which is a merge gate for the whole feature regardless of incremental shipping.

---

## Format Validation

Every task above:

- ✅ Starts with `- [ ]` (markdown checkbox)
- ✅ Has a sequential Task ID (T001–T065)
- ✅ Includes a `[P]` marker only when the task is in a different file from any incomplete prior task in the same phase
- ✅ Includes a `[US#]` story label on every task in Phases 3–9; no story label on Phases 1, 2, 10
- ✅ Includes a concrete file path (`internal/explorer/...`, `frontend/src/components/...`, `app.go`, etc.)
- ✅ Description is specific enough that an LLM can complete it without re-reading the spec or plan from scratch
