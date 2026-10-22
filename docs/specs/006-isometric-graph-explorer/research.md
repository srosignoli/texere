# Phase 0 Research: 2.5D Isometric Graph Explorer

**Date**: 2026-05-04
**Branch**: `006-isometric-graph-explorer`

This document records the technology and design decisions that resolve the Technical Context unknowns in [plan.md](./plan.md), with rationale and rejected alternatives.

---

## R-1. 3D rendering library

**Decision**: **Three.js r160** (`three` npm package).

**Rationale**:
- Spec mandates a 3D scene (orthographic camera, billboarded labels, vertical Z-axis hierarchy) — Cytoscape.js cannot deliver this.
- Three.js is the de facto standard for browser 3D, has a mature `OrthographicCamera`, billboard helpers (`Sprite`), instanced-mesh primitives (for ≤500 nodes this isn't strictly required but is a known scale path), and excellent tree-shaking with Vite.
- Bundle cost (tree-shaken core + camera + sprite + line geometry) is ~150–200 kB gzipped — acceptable in a desktop-bundled app.
- Pinned at r160; the API surface this feature uses is stable across recent r1xx releases.

**Alternatives considered**:
- **Babylon.js** — heavier, more game-engine-oriented; we don't need PBR or physics.
- **regl / pixi 3D shims** — too low-level; we'd reimplement camera, picking, and sprite billboarding.
- **Cytoscape.js (constitution default)** — 2D only; physically cannot satisfy FR-001..FR-005, FR-015.
- **Custom WebGL** — 4–6 weeks of engine work for capabilities Three.js gives us out of the box.

---

## R-2. Where the layout lives (Go vs JavaScript)

**Decision**: **Layout engine in Go** (`internal/explorer/`). Frontend receives ready-to-render JSON and is responsible only for rendering, animation, and interaction.

**Rationale**:
- Determinism is testable in Go with table-driven unit tests; positions for a fixed input must be byte-identical across runs (a snapshot test).
- Cycle detection, log centrality, and territory polygon computation are pure functions — naturally suited to Go and naturally hard to verify in a Three.js scene.
- One Wails IPC round-trip per anchor change keeps the surface tiny.
- Aligns with feature 004's pattern: graph queries run in Go and emit JSON for the frontend.
- Hot-path budget (≤100 ms for 500 nodes) is easily met in Go; Wails IPC adds <5 ms.

**Alternatives considered**:
- **Layout in JavaScript on the frontend**: rejected because layout determinism cannot be guaranteed across browser engines and JS sort stability differences, unit testing the layout would require a frontend test framework we don't otherwise need, and layout perf in JS is harder to bound predictably.
- **Layout in WebAssembly** (compile Go to WASM): rejected — extra build complexity, no benefit since we already have a Go process via Wails.

---

## R-3. Edge meta-type classification

**Decision**: Use the `MetaType` field already populated on each arrow in `internal/content` (`SSTRegistry.Arrows[code].MetaType`) by feature 002's parser and consumed by feature 004's `RegisterArrows` (see `internal/search/arrows.go`).

The four canonical meta-types map to spatial axes (FR-006):
| MetaType (registry value) | Direction (Semantic Compass) | Spatial axis |
|---|---|---|
| Containment / "Belongs to" | North | +Y (vertical, parents above) |
| Causality / "Leads to" | East | +X (horizontal flow) |
| Properties / "Expresses" | South | +Z (depth, near side) |
| Similarity / "Similar to" | West | -Z (depth, far side) |

For inverse arrows (e.g., `then` ↔ `from`), the explorer projects both onto the same axis pair; the inverse direction simply flips the sign on that axis.

**Rationale**:
- Re-uses existing well-tested classification — no new parser logic.
- The arrow registry is the canonical source for STType/meta-type resolution; using it keeps a single source of truth.

**Alternatives considered**:
- **Arrow-name heuristics** (matching strings like "contains", "leads to"): rejected — registry already does this work and is i18n-aware.
- **Per-edge user override**: out of scope for v1 (FR-007 makes the compass non-configurable).

---

## R-4. Cycle handling for containment

**Decision**: **Lowest `NodePtr` wins.** When a containment cycle is detected during the BFS used for vertical positioning, the system marks the node with the lowest `SST.NodePtr` value as the layout-time root of the cycle. Other nodes in the cycle are placed at child elevations relative to that root. Every node participating in the cycle receives a `cycleWarning: true` flag in the layout JSON, which the frontend renders as a small badge (e.g., a circular warning glyph adjacent to the node).

**Rationale**:
- Determinism: `NodePtr` is stable per-build. Same graph → same break.
- "Lowest wins" is trivial to explain in an error/help text and easy to unit-test.
- Surfacing the cycle visually keeps the user informed (Constitution V — Transparency).

**Alternatives considered**:
- **Largest in-degree wins** — also deterministic, but in-degree is a derived quantity that introduces ordering dependency.
- **First-in-result-set wins** — depends on result-set ordering, which is not guaranteed stable.
- **Rendering the cycle as edges with both endpoints at the same elevation** (no break) — rejected because it produces inconsistent vertical hierarchy and breaks FR-006.

---

## R-5. Territory polygon shape and computation

**Decision**: **Convex hull per context**, dilated by a fixed margin (default: 0.5 world units), with a global "buffer" zone separating distinct territories so the bridge edges (cross-context, FR-020) have a visible gap to span.

Algorithm (Go side, in `internal/explorer/territory.go`):
1. Group nodes by primary context (first declared tag — Assumption documented in spec).
2. For each context group, project node ground-plane positions (x, z) — y is ignored.
3. Compute the convex hull (Graham scan; ≤500 points so naive complexity is fine).
4. Dilate the hull outward by a fixed margin.
5. If two dilated hulls overlap, shrink each by the overlap to maintain the buffer gap.
6. For single-node groups, emit a small disc polygon around the node position.
7. For the single-context special case (FR-021), emit one large hull spanning the entire result.

Output: For each context, an array of (x, z) polygon vertices and a color slot index (assigned by the frontend's theme palette).

**Rationale**:
- Convex hulls are simple, deterministic, and look organic at this scale.
- The fixed-margin buffer mirrors the city-block aesthetic the spec invokes (SimCity 2000 districts).
- O(n log n) per group — well within budget for 500 nodes total.

**Alternatives considered**:
- **Voronoi-style territories** — visually striking but creates jagged borders and is harder to compute deterministically near collinear node arrangements.
- **Axis-aligned bounding boxes** — too rigid; produces ugly grid-like districts that ignore the actual node positions.
- **Concave hulls** — complex tuning for the alpha parameter; deterministic only with a fixed alpha.

---

## R-6. Re-anchor caching strategy

**Decision**: **Result-set cache** in `internal/explorer/service.go`. `ExplorerOpen(query)` runs the query (delegating to feature 004's existing graph-search path), stores the materialized subgraph (nodes + edges + context membership) in memory, and returns an opaque `resultID` along with the initial layout. `ExplorerReanchor(resultID, anchorID)` re-runs only the layout step against the cached subgraph — no BadgerDB hit.

Cache lifecycle:
- One active result per session. A new `ExplorerOpen` evicts the prior cache.
- `ExplorerClose()` evicts.
- Cache is process-memory only; no disk persistence.
- Cache size bounded by the spec's 500-node ceiling.

**Rationale**:
- Re-anchor budget is <1 s end-to-end (SC-010); avoiding the BadgerDB query keeps the layout-only path well under 100 ms.
- Single-result cache is sufficient because the explorer's history (Alt+←/→) navigates anchors *within* the current result; new queries always go through `ExplorerOpen`.

**Alternatives considered**:
- **No cache, re-query on every anchor change** — rejected: the BadgerDB orbit query for 500 nodes can take 50–200 ms; combined with layout this risks blowing the 1 s budget on slower disks.
- **Multi-result LRU cache** — rejected: anchor history within a single result is the only navigation pattern; there is no use case for keeping older results around since `ExplorerOpen` for a new query always clears history (FR-026f).

---

## R-7. Skeleton-first reveal mechanics

**Decision**: The Wails layout response is shipped as a single JSON payload, but the **frontend** stages the render across three phases:

1. **Frame 0 (≤16 ms)** — Build and add to the scene: ground plane, all context territory polygons, the compass overlay, the mini-map. Camera framing is set to fit-to-result.
2. **Frame 1–N (next ~500 ms)** — Build node meshes + sprite labels in batches sized to keep each frame under 8 ms; use a CSS-style fade-in opacity tween (200 ms each) staggered by distance from anchor (closer nodes appear first).
3. **Edge build** — Edges are constructed alongside their target nodes in the same batch and tween in together (FR-032c: never floating without endpoints).

**Rationale**:
- Single IPC keeps the contract simple; the frontend has full layout data immediately and can stage rendering as it sees fit.
- The user perceives the landscape "growing out" from the anchor — a UX touch that aligns with the exploration metaphor.
- Per-batch <8 ms keeps pan/zoom interactive even during initial reveal.

**Alternatives considered**:
- **Two-phase IPC** (territories first, then nodes): rejected — adds a round-trip with no benefit since layout is already <100 ms.
- **One-shot full reveal** (no fade-in): rejected — violates FR-032a and feels jarring.

---

## R-8. Frontend testing framework

**Decision**: **None added.** Frontend behaviors are verified through manual quickstart scenarios.

**Rationale**:
- The constitution mandates unit tests for "public APIs and scheduling algorithms". The layout engine (the public Go API) has full unit + benchmark coverage; the frontend renders deterministic input deterministically.
- The application has no existing frontend test framework (no Vitest, no Playwright). Adding one for this feature alone introduces tooling overhead disproportionate to the marginal coverage gain.
- 3D scene assertions are notoriously fragile in headless runners.

**Alternatives considered**:
- **Vitest + JSDOM** — rejected for this feature; can be added later as a shared concern if frontend test coverage becomes a project-wide priority.
- **Playwright in Wails dev mode** — rejected; complex setup, flaky for animation timing assertions.

---

## R-9. Camera rotation lock

**Decision**: At the Three.js layer:
- Use `OrthographicCamera`. Position it at a fixed offset (`{x: cos(35°), y: sin(35°), z: cos(35°)}` scaled) looking at the origin with `up = (0, 1, 0)`.
- Disable any built-in `OrbitControls` rotation. Custom pan-only controller listens to `pointermove` while pointer is down and moves the camera/world along the (x, z) plane.
- Zoom adjusts `camera.zoom` (orthographic), bounded `[zoomMin, zoomMax]` (initial values: `[0.25, 4.0]`; refined during dev based on label legibility).
- Trackpad two-finger gesture: pan; pinch: zoom. (Wheel events with `ctrlKey` set are pinch on macOS.)
- Browser default right-click menu is suppressed inside the explorer canvas; mouse buttons 4/5 are mapped to history back/forward.

**Rationale**:
- A custom controller is simpler than OrbitControls + restrictions, and it makes rotation physically impossible (FR-003) — there is no code path that mutates camera quaternion.

**Alternatives considered**:
- **MapControls + lock pitch/yaw** — rejected: still allows pinch-rotation via two-finger trackpad; locking it requires intercepting the same events we're already handling.

---

## R-10. Hit-area expansion (FR-014a/b)

**Decision**: A **separate transparent picking mesh** per node, sized to a constant 24-pixel screen-space diameter (using `camera.zoom`-aware world-space conversion). Picking meshes are added to a dedicated raycast layer; the visible mesh remains exactly the centrality-driven size. Hover/click events fire from picking-mesh hits.

**Rationale**:
- Three.js `Raycaster` supports per-layer testing; separating visual and pick targets is the standard pattern.
- Screen-space sizing is recomputed on zoom changes (rare event) — cheap.
- When picking meshes overlap, the raycaster's distance-sorted hit list gives "closest to cursor" determinism (FR-014a).

**Alternatives considered**:
- **Single mesh sized to max(24px, visualDiameter)**: rejected — would inflate the visible centrality encoding (forbidden by FR-014b).
- **CPU-side AABB picking on screen-space coordinates**: rejected — duplicates raycaster logic; harder to maintain.

---

## R-11. Anchor history integration with browser-style shortcuts

**Decision**: Two layers:

- **Frontend store** (`stores/explorer.js`) holds the in-memory `back: NodeID[]`, `forward: NodeID[]`, capped at 50 each (FR-026g).
- **Wails layer** is unchanged: the frontend simply calls `ExplorerReanchor(resultID, prevAnchor)` on back, and the Go-side layout cache replies in <100 ms.

Keyboard / mouse bindings (frontend, captured at the explorer-canvas root):
- `Alt+ArrowLeft` → back; `Alt+ArrowRight` → forward.
- Pointer event with `button === 3` (mouse-back) → back; `button === 4` (mouse-forward) → forward.
- All bindings are no-ops at history boundaries (FR-026e).

**Rationale**:
- Keeps the IPC contract minimal; history is purely a frontend concern.
- 50-entry cap prevents memory growth in pathological cases (FR-026g).

**Alternatives considered**:
- **Persist history on the Go side**: rejected — no benefit; complicates the contract for a session-scoped, ephemeral feature.

---

## R-12. Search-driven re-anchor (Ctrl+Shift+K integration)

**Decision**: When the explorer is open and the omnibar is in graph mode, pressing Enter on a result invokes a new frontend pathway:
- If the selected node is in the current `resultID`'s subgraph: call `ExplorerReanchor(resultID, selectedNodeID)` and close the omnibar (FR-026h).
- Otherwise: call `ExplorerOpen(newQueryForSelectedNode)` which evicts the current result and clears history (FR-026i).

The check ("is this node in the current result?") is performed entirely on the frontend by querying the current layout state, avoiding any Go round-trip for the common case. The omnibar already knows its result mode, so the only modification is to its Enter handler when an `explorerOpen` Svelte store is true.

**Rationale**:
- Reuses feature 004 plumbing; the only change is the Enter-key dispatch site in `OmniBar.svelte`.
- The "node in current result?" check is O(1) against a frontend `Set<nodeID>`.

**Alternatives considered**:
- **New dedicated keybind for search-from-explorer**: rejected — adds a binding when Ctrl+Shift+K already does the right thing semantically.

---

## Summary

All NEEDS-CLARIFICATION items from Phase 0 are resolved. No further clarification round is required before Phase 1.
