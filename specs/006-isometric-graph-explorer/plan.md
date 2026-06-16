# Implementation Plan: 2.5D Isometric Graph Explorer

**Branch**: `006-isometric-graph-explorer` | **Date**: 2026-05-04 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/006-isometric-graph-explorer/spec.md`

## Summary

Replace the previously planned 2D node-link diagram (feature 004) with a 2.5D isometric Graph Explorer driven by the SSTorytime-KV graph. A Go layout engine computes deterministic node positions from typed edges relative to an anchor node (the "Semantic Compass": containment → vertical Y, causality → horizontal X, properties → +Z, analogies → -Z), groups nodes by primary context into ground-plane territories, and emits a JSON scene description over Wails IPC. The Svelte/Three.js frontend renders an orthographic 3D scene with a locked isometric camera (no rotation), pan/zoom, hover-highlight, primary-click-to-re-anchor with browser-style history (Alt+←/→), keyboard re-anchor via the existing `Ctrl+Shift+K` graph-search omnibar, billboarded labels, log-centrality node sizing, a permanent compass element, a context-only mini-map, and a skeleton-first reveal so territories paint immediately while nodes fade in.

## Technical Context

**Language/Version**: Go 1.22 (backend, layout engine + Wails bindings); JavaScript/Svelte 3.49 (frontend); pinned via existing `go.mod` and `frontend/package.json`.
**Primary Dependencies**:
- Backend (Go, no new deps): existing `github.com/srosignoli/sstorytime-kv` (read-only consumer), `github.com/wailsapp/wails/v2`. Layout engine uses only stdlib (`sort`, `math`, `container/heap`).
- Frontend (new dep): **Three.js** (`three` ≈ r160) for the 3D scene + orthographic camera. CodeMirror, Svelte already present and unchanged.

**Storage**: None added. Read-only consumer of the SSTorytime-KV BadgerDB store (`<collectionDir>/sst_data/`) populated by feature 004's Build Graph (Ctrl+G). No new files, no new tables, no persisted layout state.

**Testing**:
- Go unit tests for the layout engine: determinism, axis assignment per meta-type, log-centrality scaling, cycle breaking, territory grouping, anchor-relative positioning.
- Go benchmark (`testing.B`) covering layout for 200 / 500 / 1500 nodes to verify <100ms layout budget on baseline hardware.
- Manual quickstart scenarios cover all P1/P2 user stories on actual user data — frontend rendering and interaction behaviors (camera lock, hover, click, history, mini-map) are validated by hand.

**Target Platform**: Wails v2 desktop app on Linux amd64 (primary) and Windows amd64 (cross-compiled). Mid-range integrated GPU baseline at 1080p. Touch and mobile out of scope.

**Project Type**: Desktop application (Go backend bound to Svelte/Vite frontend via Wails).

**Performance Goals**:
- Layout compute (Go): <100ms for ≤500 nodes / ≤1500 edges.
- First interactive paint (skeleton — ground + territories visible): <16ms (one frame) after layout JSON arrives.
- Full landscape (all nodes faded in): <2s for a 200-node / 500-edge result (SC-004).
- Re-anchor (layout + animated camera + node fade): <1s end-to-end (SC-010).
- Pan / zoom: ≥60 fps (SC-005).
- Hover highlight engage/disengage: <50ms (SC-007).

**Constraints**:
- Result-size ceiling: ~500 nodes / ~1500 edges; above the ceiling the system must clamp/down-sample/warn (FR-034).
- Pure Go for backend dependencies (no CGo); per-feature exceptions go in Complexity Tracking. This feature adds none.
- Camera rotation must be physically impossible to trigger from any input (FR-003).
- Read-only access to SSTorytime-KV; no writes or schema changes.

**Scale/Scope**: Single-user desktop tool; ≤500 nodes per render; session-scoped state only (no persistence across app restart).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Principle I — Source-First & Full N4L Fidelity
**PASS.** This feature is a pure read-only consumer of derived state (BadgerDB). It never reads or writes `.n4l` files; round-trip fidelity is unaffected.

### Principle II — Self-Contained Binary
**PASS (Go side).** No new Go dependencies; existing SSTorytime-KV is already pure Go. Cross-compilation unaffected.
**Note (Frontend):** Three.js (`three`) is added as an npm dev/runtime dependency. Frontend assets are bundled by Vite and embedded by Wails into the single binary; no external runtime services are introduced. This is consistent with how CodeMirror, Svelte, and pdfjs-dist are already shipped.

### Principle III — Separation of Concerns
**PASS.** The explorer is a visualization layer over the knowledge graph. It does not store canonical knowledge, does not write to BadgerDB, and does not touch SQLite. Layout state is computed on demand, never persisted.

### Principle IV — Pleasure-Driven Learning
**N/A.** This is an exploration feature, not a study/scheduling feature. Constitution principle IV gates study-facing features.

### Principle V — Transparency & Portability
**PASS.** No automatic decisions to surface (the Semantic Compass mapping is fixed and documented). The mini-map, compass, legend, and history affordances all keep the user informed about the current view state. No data is held hostage; the explorer is a view over BadgerDB which is itself rebuildable from `.n4l` source.

### Performance Standards Compliance
**DEVIATION (justified — see Complexity Tracking).** The constitution lists `Graph explorer initial render < 500 ms | Cytoscape.js layout`. This budget was set for a 2D Cytoscape rendering plan that the spec explicitly replaces. The new contract is split into:
- **Skeleton paint** (ground + context territories, the primary spatial-orientation cue) within one frame (≤16 ms) — *exceeds* the spirit of the 500 ms budget.
- **Full landscape** (all nodes settled) within 2 s — *exceeds* the literal 500 ms but in a different render model where the user already has a usable, oriented landscape long before completion.

### Technology Stack Compliance
**DEVIATION (justified — see Complexity Tracking).** The constitution names Cytoscape.js as the canonical graph-visualization tech. This feature uses Three.js because Cytoscape.js is 2D-only and physically cannot render the 2.5D landscape, vertical-elevation containment encoding, or billboarded labels mandated by the spec.

### Testing Discipline
**PASS.** Layout engine (the public Go API of the new package) has unit-test coverage for all four axis mappings, cycle handling, centrality scaling, and territory grouping; benchmark tests guard the <100 ms budget.

### Result of Constitution Check
**Conditional pass.** Two deviations (Three.js, perf-budget reframing) are recorded in Complexity Tracking with justifications. Both deviations require a constitution amendment in a follow-up PR (Section IX of the constitution: "Changes to the canonical technology choices below MUST be documented as a constitution amendment with migration rationale"). This plan proceeds; the amendment is treated as a parallel governance task that must land before merge.

## Project Structure

### Documentation (this feature)

```text
specs/006-isometric-graph-explorer/
├── plan.md                 # This file
├── research.md             # Phase 0: tech decisions + rationale
├── data-model.md           # Phase 1: types & relationships
├── quickstart.md           # Phase 1: manual verification of every user story
├── contracts/
│   ├── layout-ipc.md       # Wails method contracts (Open / Reanchor / Close)
│   └── layout-types.md     # JSON shapes exchanged with the frontend
├── checklists/
│   └── requirements.md     # From /speckit.specify + /speckit.clarify
├── spec.md
└── tasks.md                # Created later by /speckit.tasks
```

### Source Code (repository root)

```text
texere/
├── app.go                                           # +Wails methods: ExplorerOpen, ExplorerReanchor, ExplorerClose
├── internal/
│   └── explorer/                                    # NEW package
│       ├── doc.go                                   # Package overview
│       ├── types.go                                 # LayoutResult, NodeView, EdgeView, Territory, MetaType
│       ├── classify.go                              # Edge → meta-type classification (uses registry.Arrows[code].MetaType)
│       ├── neighborhood.go                          # Anchor-relative typed BFS over the result subgraph
│       ├── cycle.go                                 # Containment cycle detection + deterministic break
│       ├── centrality.go                            # In-degree count + log scaling helper
│       ├── layout.go                                # Position assignment per Semantic Compass axes
│       ├── territory.go                             # Context grouping + ground-plane polygon computation
│       ├── service.go                               # Stateful façade with result-set cache (for re-anchor)
│       ├── layout_test.go                           # Determinism + axis-mapping unit tests
│       ├── cycle_test.go                            # Cycle-break tests
│       ├── territory_test.go                        # Territory polygon tests
│       └── layout_bench_test.go                     # Layout perf benchmarks (200 / 500 / 1500 nodes)
└── frontend/
    ├── package.json                                 # +three dependency
    └── src/
        ├── components/
        │   ├── GraphExplorer.svelte                 # NEW: top-level explorer container (canvas + overlays)
        │   ├── explorer/                            # NEW subdir
        │   │   ├── Scene.js                         # Three.js scene/camera/renderer setup, pan+zoom, lock rotation
        │   │   ├── NodeMeshes.js                    # Builds + tweens node meshes; billboarded label sprites
        │   │   ├── EdgeMeshes.js                    # Per-meta-type line styles (solid/pillar/dashed/dotted)
        │   │   ├── Territories.js                   # Ground-plane polygon meshes
        │   │   ├── Picking.js                       # Raycasting + 24px hit-area expansion
        │   │   ├── Highlight.js                     # Hover highlight + dim non-neighbors
        │   │   ├── ReanchorAnimator.js              # Position/camera tween between layouts; cancellable
        │   │   ├── AnchorHistory.js                 # Back/forward stack manager (Alt+←/→, mouse 4/5)
        │   │   └── Skeleton.js                      # First-frame ground + territory paint
        │   ├── Compass.svelte                       # Always-visible compass overlay
        │   ├── MiniMap.svelte                       # Territory + viewport-rectangle overlay
        │   └── ExplorerLegend.svelte                # Edge-style/color legend (toggle)
        ├── stores/
        │   └── explorer.js                          # Svelte stores: open/closed, current layout, history
        └── (existing) OmniBar.svelte                # MODIFIED: when explorer is open, graph-mode Enter dispatches re-anchor
```

**Structure Decision**: Single project layout with one new Go package (`internal/explorer/`) and one new frontend feature directory (`frontend/src/components/explorer/` + `GraphExplorer.svelte` + minor companion components). No backend/frontend split (we already have a Wails desktop app structure). The explorer's launch entry point and Ctrl+Shift+K integration touch existing components (`MiddleBand.svelte`, `OmniBar.svelte`) rather than introducing new global plumbing.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Use **Three.js** for graph visualization (vs constitution-mandated Cytoscape.js) | Cytoscape.js is 2D-only and cannot render the 2.5D isometric landscape, vertical elevation, billboarded labels, or orthographic camera that drive the entire spec (FR-001 through FR-005, FR-015). The Semantic Compass needs three independent spatial axes. | Cytoscape.js with x/y plus visual-encoding tricks (icon size, color) was considered and rejected: it cannot represent containment as elevation, properties vs analogies as opposite depth axes, or context territories as 3D ground polygons. The spec explicitly replaces the 2D approach. |
| **Performance budget reframe**: skeleton paint <16 ms + full landscape <2 s, vs constitution's flat `<500 ms` for graph explorer initial render | The constitution budget was set for a 2D Cytoscape pipeline. The new render model is two-phase by design (FR-032a/b: skeleton-first reveal). Skeleton paint in one frame *exceeds* the spirit of the old budget; full settle at <2 s is a different metric measured under a different rendering style. | A literal <500 ms full settle was rejected because Three.js mesh creation for ≤500 nodes (≤500 sprite labels + ≤1500 edge geometries) cannot reliably finish in 500 ms on integrated-GPU baseline hardware, and forcing it would compromise the skeleton-first UX that the spec mandates. |

Both violations require a follow-up constitution amendment. The amendment scope:
- Replace `Cytoscape.js` with `Three.js (graph explorer) + Cytoscape.js (any future 2D needs)` in the Development Workflow tech-stack table — or remove Cytoscape.js entirely if no other feature uses it.
- Replace the single `Graph explorer initial render < 500 ms` row with the two-phase budget (`Skeleton render < 16 ms / one frame` + `Full landscape settle < 2 s for ≤500 nodes`) in the Performance Standards table.

## Phase 0: Outline & Research

See [research.md](./research.md) for full Decision/Rationale/Alternatives writeup. Summary of resolved unknowns:
- **3D library**: Three.js r160. (Rationale: spec demands; mature; small footprint when tree-shaken.)
- **Layout location**: Go (deterministic, unit-testable, one IPC round-trip per re-anchor). Frontend renders only.
- **Edge meta-type classification**: Use `registry.Arrows[code].MetaType` already populated by feature 004's `RegisterArrows`.
- **Cycle handling for containment**: Lowest NodePtr "wins" (deterministic tie-break); cycle is rendered as a visible warning badge on the cycle's nodes.
- **Territory polygon shape**: Convex hull of node ground-plane positions per context, dilated by a fixed margin and clipped to the world bounds.
- **Re-anchor caching**: Result-set cache keyed by an opaque `resultID` issued by `ExplorerOpen`. Re-anchor calls only pass `(resultID, newAnchorID)` to avoid re-querying SSTorytime-KV.
- **Frontend test framework**: None added. Layout determinism is tested in Go; UI behavior is verified by quickstart scenarios.

## Phase 1: Design & Contracts

**Outputs:**
- [data-model.md](./data-model.md) — entity definitions, types, validation rules, state transitions.
- [contracts/layout-ipc.md](./contracts/layout-ipc.md) — Wails method signatures and semantics.
- [contracts/layout-types.md](./contracts/layout-types.md) — JSON shapes exchanged between Go and the frontend.
- [quickstart.md](./quickstart.md) — step-by-step manual verification of every user story.
- Agent context update via `.specify/scripts/bash/update-agent-context.sh claude`.

### Re-evaluated Constitution Check (post-design)

After working through the data model and contracts:
- No new dependencies surfaced beyond Three.js.
- The IPC contract holds only opaque IDs and view-projection data — no canonical knowledge crosses the boundary in either direction.
- Layout determinism is preserved by relying only on stable inputs (NodePtrs, registry-assigned MetaTypes, result-set membership). Same input always yields same positions.

The two flagged deviations (Three.js, perf-budget reframe) remain the only outstanding governance items. Both are documented above and require a parallel constitution amendment PR.
