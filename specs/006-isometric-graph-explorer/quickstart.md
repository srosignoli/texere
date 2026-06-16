# Quickstart: 2.5D Isometric Graph Explorer

**Date**: 2026-05-04
**Branch**: `006-isometric-graph-explorer`

This document walks through manual verification of every user story in [spec.md](./spec.md). It assumes a developer has built and launched Texere with the explorer feature merged, and has loaded a collection that exercises all four meta-types.

---

## Prerequisites

1. A Texere build that includes this branch.
2. A test collection with:
   - At least 3 contexts (e.g., `work`, `personal`, `research`).
   - At least one edge of each meta-type: `contains`, `leadsto`, `properties`, `similarto`.
   - At least one cross-context relationship.
   - One node referenced by ≥10× the average node (for landmark testing).
3. The graph has been built (Ctrl+G has been pressed and completed without errors).

If the SSTorytime-KV graph has not been built, the explorer should display the message **"Build your graph first (Ctrl+G) to enable graph search."** and not crash. (Defensive check; not part of any user story.)

---

## US1 — Read the shape of a knowledge graph at a glance (P1)

**Action**: Open the Graph Explorer with a query that returns a node having parents, children, properties, and analogies (e.g., a domain root concept).

**Expected**:
1. Within ~2 seconds, the explorer canvas is populated (skeleton appears in <16 ms; nodes fade in over the next ~500 ms).
2. The anchor node sits at the origin.
3. Containers (`contains` edges with the anchor as child) sit visibly higher on Y than the anchor.
4. Containment children sit visibly lower than the anchor.
5. `Leads to` successors are arranged toward one end of the X axis; `from`/predecessors at the opposite end.
6. Properties cluster on the +Z side of the anchor.
7. Analogies (`Similar to`) cluster on the -Z side at the same elevation as the anchor.
8. Drag-pan and scroll-zoom both work; the camera does not rotate.
9. Try to rotate (right-click drag, two-finger trackpad twist, etc.) — nothing should rotate the camera.

**Acceptance**: A sighted observer who has been told only the four-axis convention should be able to point at any node and correctly identify its relationship to the anchor.

---

## US2 — Read every node label, no matter where the camera is (P1)

**Action**: With a moderately dense result (≥30 nodes spread across all four axes), pan to opposite corners and zoom from 25% to 400%.

**Expected**:
1. Every node label remains readable at every camera position.
2. Labels are flat to the screen (never edge-on or rotated).
3. Labels at the far back of +Z are the same on-screen size as labels at the front (-Z).

**Acceptance**: Take a screenshot at maximum zoom-out. Open it in an image viewer. Every label is legible without zooming in.

---

## US3 — Identify landmark concepts immediately (P2)

**Action**: Open the explorer with a result that contains a node with 10× the in-degree of the median node.

**Expected**:
1. The high-centrality node is visibly larger than its neighbors.
2. The size scaling is logarithmic — the landmark dominates but does not crowd out its neighbors.
3. Other nodes within the same in-degree band have similar sizes.
4. A node with zero incoming references is rendered at the minimum size, but is still visible (not invisible).

**Acceptance**: Without reading any labels, a fresh observer can point to the landmark within 5 seconds.

---

## US4 — See where contexts live in the landscape (P2)

**Action**: Open the explorer with a result that spans 3 contexts and contains at least one cross-context relationship.

**Expected**:
1. The ground plane shows 3 distinct, color-coded territory polygons.
2. Each node sits over the territory of its first declared context.
3. The cross-context edge visibly spans the gap between two territories and is rendered distinctly from same-context edges (e.g., dotted bridge style).
4. Now run a single-context query — verify the entire ground plane is one large territory (no internal subdivision).
5. Switch the app theme between light and dark — territory colors and edge colors stay distinguishable.

**Acceptance**: Without inspecting any node, the observer can correctly partition nodes into their contexts within 10 seconds.

---

## US5 — Trace a specific path through the landscape (P3)

**Action**: With a moderately dense graph (≥50 nodes), hover over any node.

**Expected**:
1. Within 50 ms the hovered node is visually emphasized (outline / brightened).
2. All edges directly connected to that node are rendered prominently.
3. Unrelated nodes and edges are dimmed but still visible.
4. Move the cursor off the node — landscape returns to default within 50 ms.
5. Verify the hover highlight engages reliably even on the smallest log-scaled nodes — the 24 px hit area should make them effortless to target.

**Acceptance**: A user can trace a 3-hop path purely by hovering each node and visually following the highlighted edges.

---

## US6 — Pivot exploration around a new anchor (P2)

**Action**: With a result loaded, click any non-anchor node.

**Expected**:
1. Layout recomputes with the clicked node as the new anchor.
2. The camera animates smoothly to the new fit-to-result framing.
3. The total time from click to settled framing is ≤ 1 second.
4. The compass element (US7) labels are unchanged — only positions move.
5. Click the *current* anchor — nothing happens (no flicker, no animation).
6. Click a different node mid-animation — the in-flight tween cancels and a new one starts from the current camera position without flicker.

**Action 2**: Press `Alt+ArrowLeft`. Verify the previous anchor is restored with a smooth animation. Press `Alt+ArrowRight` — the forward anchor returns. Press `Alt+ArrowLeft` repeatedly past the start — silent no-op (no toast, no glitch).

**Action 3**: If your mouse has back/forward buttons, the same back/forward bindings apply.

**Action 4**: Run a new query. Verify the anchor history is cleared (Alt+ArrowLeft is now a no-op until you re-anchor).

**Acceptance**: All five acceptance scenarios in spec User Story 6 are observed.

---

## US7 — Stay oriented in a complex landscape (P3)

**Action**: Open a large (≥100-node) result.

**Expected**:
1. A compass element is visible somewhere on the canvas (likely a corner) showing the four spatial-axis labels.
2. Pan far from the origin. A mini-map appears or remains visible, showing context territories (without individual nodes) and a viewport rectangle.
3. The viewport rectangle updates in real time as you pan and zoom.
4. The territory shapes in the mini-map only update when the underlying result or anchor changes (re-anchor recomputes territories).
5. Press the reset-view shortcut. The camera animates smoothly back to the fit-to-result framing within 1 second. After re-anchoring several times, reset returns to the *current* anchor's framing — not the original query anchor.

**Acceptance**: A user reports never feeling lost during exploration of a 100-node result.

---

## Search-driven re-anchor (FR-026h–j)

**Action 1**: With the explorer open, press `Ctrl+Shift+K`. The omnibar opens in graph mode.

**Action 2**: Type the name of a node that is in the current result. Press Enter.

**Expected**: The omnibar closes; the explorer re-anchors on that node with the standard animation; the prior anchor is pushed onto history (Alt+ArrowLeft works).

**Action 3**: Type the name of a node that is *not* in the current result. Press Enter.

**Expected**: The current result is replaced (a new query is run); the anchor history is cleared; the explorer re-renders with the selected node as the initial anchor.

**Note**: `Ctrl+K` (without shift) opens the *fuzzy file* search and is unrelated to the explorer. Verify it still navigates the editor as before — it should not affect explorer state.

---

## Edge cases (sample — see [spec.md §Edge Cases](./spec.md#edge-cases) for full list)

- **Empty result**: Query a non-existent term. Verify the explorer shows a "no results" empty state, not a blank canvas.
- **Single-node result**: Query a node with zero edges. The node renders at minimum size at the origin; the compass and a single-context territory are visible.
- **Cycle in containment**: If your test collection contains a contained-by cycle, verify both nodes show a small cycle-warning badge and the layout completes (no freeze).
- **Result-size ceiling**: Run a query that exceeds 500 nodes. Verify a "Showing 500 of N nodes" banner appears; the explorer renders the clamped subset.

---

## Performance verification

The Go-side performance budget is enforced by `go test -bench=. ./internal/explorer/`. The frontend-side budgets (60 fps pan/zoom; <50 ms hover) are checked manually:

1. Open a 200-node result. Drag-pan continuously for 5 seconds. Verify no visible stutter; if dev tools are available, the frame time should stay ≤16 ms.
2. Hover-then-unhover 20 different nodes in rapid succession. Verify the highlight transitions feel instant (no perceptible delay).
3. Re-anchor 10 times in a row. Verify each re-anchor settles within 1 second.

---

## Smoke test for IPC contract

In the Wails dev console:

```js
const r = await window.go.main.App.ExplorerOpen("chapter brain")
// r.resultId is non-empty, r.nodes.length > 0, r.diagnostics.graphError is empty
const r2 = await window.go.main.App.ExplorerReanchor(r.resultId, r.nodes[1].id)
// r2.anchorId === r.nodes[1].id
await window.go.main.App.ExplorerClose()
// returns null/undefined; no exception
```

Failure modes worth eyeballing:
- `ExplorerReanchor("bogus", "1")` should reject with `"stale resultId"`.
- `ExplorerOpen("")` should reject with `"empty query"`.
- `ExplorerOpen` while no graph is built returns a `LayoutResult` with `diagnostics.graphError` set — not an exception.

---

## Done definition

This feature is verifiable as complete when:

- [ ] All 7 user stories pass their acceptance criteria above.
- [ ] All listed edge cases produce the expected behavior.
- [ ] Go unit tests for `internal/explorer/` pass and benchmark stays within the <100 ms layout budget at 500 nodes.
- [ ] Manual perf checks show 60 fps pan/zoom and <50 ms hover at 200 nodes.
- [ ] Light/dark theme switch preserves edge and territory contrast.
- [ ] Constitution amendment PR for Three.js + perf-budget reframe has landed (or has been explicitly deferred with merge-time approval).
