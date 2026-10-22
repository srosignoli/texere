# Feature Specification: 2.5D Isometric Graph Explorer

**Feature Branch**: `006-isometric-graph-explorer`
**Created**: 2026-05-04
**Status**: Draft
**Input**: User description: "2.5D isometric Graph Explorer that replaces the previously planned 2D node-link diagram. Uses a fixed isometric viewing angle with no perspective distortion. Encodes the four SSTorytime relational meta-types (Contains, Leads to, Properties, Similar to) as four spatial axes — the 'Semantic Compass'. Context tags become colored ground-plane districts. Node centrality drives node size. Camera rotation is locked. Pan, zoom, and hover are the only navigation gestures."

## Dependencies *(critical)*

This feature is a **pure consumer** of the graph backend delivered by feature 004 (`004-unified-search-graph`). It does not query files, parse N4L, build indexes, or write to any data store. Specifically:

- **Graph data source**: All nodes, typed edges, contexts, and centrality information consumed by the explorer come from the **SSTorytime-KV** graph (the `github.com/srosignoli/sstorytime-kv` BadgerDB-backed store) populated by the existing Build Graph action (Ctrl+G). If that graph has not been built or is unavailable, the explorer MUST surface the same "Build your graph first (Ctrl+G) to enable graph search." message convention used by feature 004's Tier 2.
- **Query mechanism**: All result sets consumed by the explorer are produced by feature 004's existing graph-query pipeline (chapter, context, path, neighborhood, etc.). This feature does not introduce new query syntax.
- **Search overlay integration**: Keyboard-driven re-anchoring (FR-026h) is delegated entirely to the existing graph-mode omnibar opened with **Ctrl+Shift+K** (the graph-search shortcut — distinct from Ctrl+K which is the fuzzy file-search shortcut and is unrelated to graph nodes).

If any of these upstream pieces is missing, the explorer renders an empty state with an actionable hint, rather than failing silently.

## Clarifications

### Session 2026-05-04

- Q: How should the explorer render while a result is being laid out (initial open and after re-anchor)? → A: Skeleton-first reveal — ground plane and context territories appear immediately, nodes fade in once positioned.
- Q: How does the user return to a previous anchor after a chain of re-anchors? → A: Browser-style back/forward via keyboard (Alt+Left / Alt+Right) and mouse back/forward buttons, backed by a session-scoped anchor history stack.
- Q: How does the system ensure every node is reliably clickable regardless of its centrality-driven visual size? → A: Invisible hit-area expansion — every node has at least a 24px-diameter clickable region, even when its visual size is smaller; visual minimum is unconstrained.
- Q: How can a keyboard-only user re-anchor to a node they have not visited yet? → A: Reuse the unified search overlay's graph mode (feature 004) — Ctrl+Shift+K → type node name → Enter re-anchors the explorer on that node. (Note: Ctrl+K is the *fuzzy file* mode and is irrelevant here; Ctrl+Shift+K is the *graph* mode that queries the SSTorytime-KV graph.)
- Q: What does the mini-map show alongside the viewport indicator? → A: Context territories plus the viewport rectangle, with no individual node rendering.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Read the shape of a knowledge graph at a glance (Priority: P1)

A user has built a knowledge graph and runs a query (a chapter, a context cluster, a path). Instead of a tangled 2D node-link diagram, the Graph Explorer presents the result as a small isometric landscape. Without reading any labels, the user can already tell:

- which nodes are macro containers (they sit higher above the ground)
- which nodes lead to which (left-to-right flow on the ground plane)
- which nodes are properties of which (clustered along one depth direction)
- which nodes are analogies to which (clustered along the opposite depth direction)
- which nodes are landmarks (they are visibly larger)

**Why this priority**: This is the entire reason the feature exists. Without spatial layout that reflects relationship semantics, the explorer is just decoration. With it, the user reads structure preattentively — before reading any label — which is the core value proposition.

**Independent Test**: Build a graph that includes at least one edge of each of the four relational meta-types. Open the Graph Explorer. Verify a viewer with no prior knowledge of the data can correctly identify which nodes are containers, which sequence into which, and which are properties vs analogies, by spatial position alone.

**Acceptance Scenarios**:

1. **Given** a result with parent-child containment relationships, **When** the explorer renders it, **Then** parent containers appear visibly higher on the vertical axis than their children.
2. **Given** a sequence A → B → C connected by "Leads to" relationships, **When** the explorer renders it, **Then** A, B, and C appear arranged left-to-right along a single horizontal flow direction (consistent across the whole scene).
3. **Given** a node with three property children, **When** the explorer renders it, **Then** the three properties cluster outward from the parent in one depth direction at the same elevation as the parent.
4. **Given** a node with two "Similar to" siblings, **When** the explorer renders it, **Then** the two analogies cluster outward in the *opposite* depth direction from properties, at the same elevation.
5. **Given** the user pans, zooms, or otherwise interacts with the view, **When** these actions occur, **Then** the camera angle never rotates — the four spatial axes remain perfectly oriented regardless of input.

---

### User Story 2 - Read every node label, no matter where the camera is (Priority: P1)

While exploring, the user pans and zooms across the landscape. Every node label they look at is sharp and legible — there is no perspective foreshortening that shrinks distant objects, and labels never appear sideways or rotated relative to the viewer.

**Why this priority**: The 2.5D layout is worthless if the user cannot read what they are looking at. Label legibility is a hard requirement, not a polish item, because the entire purpose of seeing nodes spatially is to anchor that spatial information to the underlying concepts.

**Independent Test**: Open the explorer with a moderately dense query (30+ nodes spread across all four spatial axes). Pan to opposite corners and zoom both in and out. Verify every visible node label is readable at every camera position.

**Acceptance Scenarios**:

1. **Given** the explorer is open and the user zooms out to fit the whole landscape, **When** they look at any node, **Then** its label remains readable (does not pixel-collapse below the legibility threshold).
2. **Given** a node positioned at the far back of the depth axis, **When** the user views it, **Then** its label appears at the same on-screen size as labels of nodes positioned at the front.
3. **Given** any node in the scene, **When** the explorer renders it, **Then** its label appears flat and front-facing to the viewer — never edge-on or rotated away.

---

### User Story 3 - Identify landmark concepts immediately (Priority: P2)

A user opens a complex query result with dozens of nodes and needs to figure out which concepts are central to this neighborhood. Scanning the landscape, they instantly spot two or three nodes that are visibly larger than the rest — these are the most-referenced concepts. They use these landmarks as anchor points to orient themselves before drilling in.

**Why this priority**: Without visual centrality cues, every node looks equally important and the user has no entry point into a complex result. Landmarks reduce orientation from a click-by-click exploration to a 2-second visual scan.

**Independent Test**: Build a graph in which one node is referenced 10× more than the average. Open the explorer. Verify that node renders visibly larger than the rest and is the first thing the eye is drawn to.

**Acceptance Scenarios**:

1. **Given** a node with 10× the in-degree of others in the result, **When** the explorer renders the result, **Then** that node appears proportionally larger on a logarithmic scale — visibly bigger but not screen-dominating.
2. **Given** a result where all nodes have similar in-degree (within a small range), **When** the explorer renders it, **Then** node sizes appear roughly uniform — no false landmarks emerge.
3. **Given** a node with zero incoming references, **When** the explorer renders it, **Then** that node still renders at the minimum (non-zero) size — never invisible.

---

### User Story 4 - See where contexts live in the landscape (Priority: P2)

A user has organized notes into multiple contexts (e.g., "work", "personal", "research"). When they open the explorer, the ground plane is divided into colored zones — one per active context. Their nodes float above or sit on the zone matching their context. Cross-context relationships visibly span the gap between zones, making the cognitive cost of those connections visible at a glance.

**Why this priority**: Context is one of the most valuable organizational primitives in N4L. Surfacing context as physical territory turns it from an invisible tag into a visible region of the landscape, which dramatically improves at-a-glance comprehension of "where does this concept live in my knowledge?".

**Independent Test**: Create a graph with at least 3 contexts and at least one cross-context relationship. Open the explorer. Verify the ground plane shows 3 distinct colored zones, each node sits over its context's zone, and the cross-context edge visibly spans the two zones.

**Acceptance Scenarios**:

1. **Given** a query result spanning 3 contexts, **When** the explorer renders the result, **Then** the ground plane shows 3 distinct, color-coded territorial polygons.
2. **Given** a node tagged with context X, **When** the explorer renders it, **Then** the node sits over the territory of context X.
3. **Given** an edge connecting a node in context X to a node in context Y, **When** the explorer renders that edge, **Then** the edge visibly spans from territory X to territory Y and is distinguishable from same-context edges.
4. **Given** a result with a single active context, **When** the explorer renders the result, **Then** the entire ground plane is rendered as that context's territory (no internal subdivision).

---

### User Story 5 - Trace a specific path through the landscape (Priority: P3)

A user wants to understand a single node's role: what flows into it, what flows out of it, what it is a property of. They hover their cursor over the node. The node is visually emphasized, its connected edges illuminate, and the rest of the landscape dims. The user follows the lit edges visually and traces the node's neighborhood without dragging the camera around.

**Why this priority**: Hover-highlighting is polish that converts a static view into an interactive one. It is not strictly required for an MVP because users can already read structure spatially, but it dramatically improves precision when tracing specific paths in a dense scene.

**Independent Test**: Open the explorer with a moderately dense graph (50+ nodes). Hover over any node. Verify the node is visually emphasized, every directly connected edge is highlighted, and unrelated nodes/edges are dimmed (but still visible enough for context).

**Acceptance Scenarios**:

1. **Given** the explorer is open with multiple nodes visible, **When** the user hovers over a specific node, **Then** that node is visually emphasized (outlined or brightened) within 50ms.
2. **Given** the user is hovering a node, **When** the highlight engages, **Then** every edge directly connected to that node is rendered more prominently than other edges.
3. **Given** the user is hovering a node, **When** the highlight engages, **Then** unrelated nodes and edges are visually dimmed — present for context but not competing for attention.
4. **Given** the user moves the cursor off the node, **When** the hover ends, **Then** the landscape returns to its default rendering within 50ms.

---

### User Story 6 - Pivot exploration around a new anchor (Priority: P2)

A user is reading the landscape around their initial query node and notices an interesting concept several edges away. They want to make *that* concept the new center of attention without typing a new query. They click the node. The landscape smoothly animates as the Semantic Compass re-orients around the clicked node — its containers rise above it, its causal flow extends to its right, its properties cluster on one depth side, its analogies on the other. The compass directions never change; only the node positions do. The user keeps exploring by clicking outward, never losing their bearings.

**Why this priority**: Re-anchoring on click is what turns the explorer from a static visualization into an exploratory map. Without it, every shift of focus requires returning to the search bar and re-running a query. With it, exploration is a continuous spatial activity. It is P2 rather than P1 because the initial query rendering (US1) already delivers significant standalone value; re-anchoring multiplies that value but is not strictly required for the MVP.

**Independent Test**: Open the explorer with a query that returns at least 10 nodes spanning all four meta-types. Click any non-anchor node. Verify the layout recomputes with the clicked node as the new anchor, the camera animates smoothly into the new framing within 1 second, and the compass labels still point to the same meta-types as before.

**Acceptance Scenarios**:

1. **Given** the explorer is rendering a result anchored on node X, **When** the user clicks node Y (where Y ≠ X), **Then** the layout recomputes with Y as the new anchor and the camera animates smoothly to the new fit-to-result framing within 1 second.
2. **Given** the layout has just re-anchored on node Y, **When** the user inspects the spatial axes, **Then** containers of Y appear above Y, causal successors appear toward the same end of the flow axis as before, and the compass element labels are unchanged.
3. **Given** the user clicks the current anchor node, **When** the click event fires, **Then** the system performs no recompute and no camera animation (no-op).
4. **Given** a re-anchor animation is in flight, **When** the user clicks a third node, **Then** the in-flight animation cancels and a new animation begins from the current camera position without visual flicker.
5. **Given** the user has re-anchored several times, **When** they trigger the reset-view action (FR-029), **Then** the camera returns to the framing of the *current* anchor — not the original query anchor.

---

### User Story 7 - Stay oriented in a complex landscape (Priority: P3)

When the landscape is large or the user has panned far from the origin, they need quick ways to recover orientation: a compass element confirming "which way is North", an overview indicator showing where the camera is currently pointed, and a one-key reset that snaps the camera back to fit the entire result.

**Why this priority**: The Semantic Compass only delivers value if users trust its orientation. Permanent orientation aids prevent the "where am I?" failure mode that kills exploration UIs.

**Independent Test**: Open a large result. Pan far off-screen. Verify a compass and mini-map remain visible and accurate, and that a single keystroke or button returns the camera to the default fit-to-result framing.

**Acceptance Scenarios**:

1. **Given** the explorer is open at any pan/zoom position, **When** the user looks at the screen, **Then** a compass element is visible showing the four meta-type axes (N/E/S/W labeled with their meta-types).
2. **Given** the user has panned far from the origin, **When** they look at the mini-map (or overview indicator), **Then** the current camera viewport is shown relative to the full landscape extent.
3. **Given** the user presses the reset-view shortcut, **When** the action triggers, **Then** the camera animates smoothly back to the default fit-to-result framing within 1 second.

---

### Edge Cases

- **No relationships of a given type**: A query result with zero "Contains", "Properties", or "Similar to" edges still renders correctly — the unused axis is simply empty, not malformed.
- **Cycles in containment**: If A contains B and B contains A, the system breaks the cycle for layout purposes by a deterministic tie-breaking rule and surfaces the cycle as a visible warning indicator. The layout engine never enters an infinite loop.
- **Multi-context node**: A node tagged with multiple contexts must sit over exactly one territory; ties are broken deterministically (first context tag in declaration order).
- **Orphan node** (no edges of any meta-type): Sits on the ground plane within the territory of its context (or a neutral zone if no context applies); does not break the layout.
- **Very large result sets**: Results exceeding the documented ceiling (see Assumptions) are either down-sampled, clamped to a maximum, or accompanied by a warning. The UI never freezes.
- **Empty result**: Empty state shows a clear "no results" message in place of an empty landscape.
- **Theme switch (light/dark)**: Edge colors and territory colors remain high-contrast and distinguishable in both themes.
- **Conflicting relationships between the same pair**: If A is connected to B by both "Leads to" and "Contains" simultaneously, both edges render distinctly without overlapping confusingly.
- **Active node has zero in-degree**: Renders at the minimum (non-zero) node size — never invisible.
- **Single-node result**: A result with one node and no edges still renders the node, the compass, and the (single) context territory; unused axes are empty but visible.
- **Re-anchor to a node not in the current result**: Clicking is only enabled on rendered nodes; nodes outside the current query result are not clickable. Bringing in a wholly new anchor still requires a new query.
- **Re-anchor during pan/zoom**: A click that fires after the user has panned or zoomed still re-anchors correctly — the new fit-to-result framing supersedes the user's prior pan/zoom state.
- **Skeleton state visible to user input**: While nodes are still fading in, the user MAY pan, zoom, or click already-rendered nodes; clicking a node that has not yet appeared is ignored (no error toast, no flicker).
- **History at boundaries**: Pressing back when no prior anchor exists, or forward when no forward anchor exists, is a silent no-op (no toast, no animation).
- **History during animation**: Triggering back/forward while a layout transition is in progress cancels the in-flight animation and starts the new one (mirrors FR-025b for clicks).
- **Search-driven re-anchor to in-result node**: Behaves identically to a click on that node — pushes prior anchor onto history, animates the layout, leaves the search overlay closed.
- **Search-driven re-anchor to out-of-result node**: Triggers a new query, replaces the current result set, clears anchor history, and opens the explorer on the selected node as the initial anchor.

## Requirements *(mandatory)*

### Functional Requirements

#### Layout & Camera

- **FR-001**: The system MUST render the active query result as a 2.5D isometric landscape — a 3D scene viewed from a fixed isometric vantage (approximately a 30°–35° downward viewing angle).
- **FR-002**: The system MUST use a projection with no perspective foreshortening — distance from the camera MUST NOT change the apparent size of objects.
- **FR-003**: The system MUST disable camera rotation entirely. The user MUST NOT be able to orbit, tilt, or yaw the camera through any input. The North/South/East/West orientation is permanently fixed.
- **FR-004**: The system MUST support panning the camera laterally across the landscape via primary mouse drag (and trackpad equivalent).
- **FR-005**: The system MUST support zooming in and out via scroll wheel (and pinch-to-zoom equivalent), within sensible minimum and maximum zoom bounds that prevent both label illegibility and infinite zoom-out.

#### Spatial Mapping (the Semantic Compass)

- **FR-006**: For each node in the result, the system MUST position it relative to its anchor node according to the meta-type of the connecting edge:
  - **Containment** ("Contains" / "Belongs to"): vertical elevation. Containers MUST appear higher on the vertical axis than their contained children.
  - **Causality** ("Leads to" / "Follows"): horizontal flow along the ground plane. Antecedents MUST appear toward one end of the flow axis, consequents toward the opposite end, with the direction held consistent across the entire scene.
  - **Properties** ("Properties" / "Expresses"): one direction along the depth axis. Properties MUST cluster on this side of their parent at the same elevation as the parent.
  - **Analogy** ("Similar to" / "Metaphor"): the *opposite* direction along the depth axis from properties, at the same elevation as the source node.
- **FR-007**: The Semantic Compass orientation (which spatial axis maps to which meta-type) MUST be globally consistent across all queries, sessions, and users in v1. It is not user-configurable.
- **FR-008**: The system MUST select a default anchor node when the explorer opens — typically the queried node itself, or the highest-centrality node in the result for queries that do not target a specific node.
- **FR-009**: The system MUST handle nodes that have edges of zero, one, or all four meta-types — an unused axis is simply empty, not malformed.
- **FR-010**: The system MUST detect cycles in containment relationships and break them deterministically for layout purposes, surfacing the cycle as a visible indicator without entering an infinite layout loop.

#### Visual Encoding

- **FR-011**: Each of the four meta-types MUST be visually distinguishable by edge style:
  - Causal ("Leads to"): a solid line of significant thickness with a directional arrowhead.
  - Structural ("Contains"): a solid vertical-pillar-style or thick connector, distinct from the causal style.
  - Property: a dashed line.
  - Analogy: a dotted line.
- **FR-012**: Each meta-type MUST also be assigned a distinct color, with values that maintain WCAG-AA-equivalent contrast against both the dark- and light-theme backgrounds and against any context-territory colors used in the same scene.
- **FR-013**: Edge meaning MUST be distinguishable by line style alone (not relying on color), so colorblind users can still read the four meta-types without color cues.
- **FR-014**: Node size MUST scale logarithmically with each node's in-degree centrality (the count of incoming edges). A node with no incoming links renders at the minimum (non-zero) size; the largest in the scene renders at the maximum size.
- **FR-014a**: Each node MUST expose a clickable / hover-target hit area at least 24 pixels in diameter on screen, regardless of its visual size. When two nodes' hit areas would overlap, the closer-to-cursor node MUST receive the input deterministically.
- **FR-014b**: When hit areas overlap, the visible centrality encoding MUST NOT be inflated to compensate — the visual size remains driven by FR-014. Only the input-handling layer is enlarged.
- **FR-015**: Node labels MUST always face the viewer and MUST remain at a constant on-screen pixel size (or within a narrow legibility range), regardless of zoom level or 3D position.
- **FR-016**: All node labels MUST remain readable (above the documented minimum on-screen pixel size) at every supported zoom level and at every camera position within the pan bounds.

#### Contextual Districts

- **FR-017**: The ground plane (vertical position = 0) MUST be subdivided into colored polygons, one per active context in the current result.
- **FR-018**: Each context territory MUST be rendered as a distinct, visually identifiable region using semi-transparent fill or distinct surface treatment.
- **FR-019**: Each node MUST be positioned over the territory of its primary context.
- **FR-020**: Edges connecting nodes in two different contexts MUST visibly span the gap between the two territories and MUST be visually distinguishable from same-context edges.
- **FR-021**: When the result contains only one active context, the entire ground plane MUST be rendered as that context's territory (no internal subdivision).

#### Interaction

- **FR-022**: Hovering the cursor over a node MUST visually emphasize that node, brighten its directly connected edges, and dim all other nodes and edges to draw attention to the node's neighborhood.
- **FR-023**: Removing the cursor from a node MUST restore the default rendering of all nodes and edges within 50ms.
- **FR-024**: The hover highlight effect MUST engage within 50ms of the cursor entering a node.
- **FR-025**: A primary-click on any node MUST re-anchor the Semantic Compass to that node. The layout MUST recompute with the clicked node as the new anchor, and the camera MUST animate smoothly from the previous framing to the new fit-to-result framing within 1 second. The compass orientation (which axis = which meta-type) MUST NOT change during re-anchoring — only node positions update.
- **FR-025a**: Re-anchoring on the current anchor MUST be a no-op (no recompute, no animation) so accidental clicks on the active anchor do not visually disrupt the user.
- **FR-025b**: If the user clicks a different node while a re-anchor animation is already in progress, the system MUST cancel the in-flight animation and start the new one from the current camera position, without flicker.
- **FR-026**: The system MUST provide an entry point to launch the Graph Explorer from elsewhere in the application (search overlay results, toolbar control, and/or keyboard shortcut), and an exit path that returns the user to their prior view without losing editor state.
- **FR-026a**: The system MUST maintain a session-scoped anchor history. Each re-anchor (FR-025) pushes the previous anchor onto the back stack and clears the forward stack (browser-style semantics).
- **FR-026b**: The user MUST be able to navigate backward through anchor history via keyboard shortcut (Alt+Left) and via the mouse "back" button (where the input device exposes one).
- **FR-026c**: The user MUST be able to navigate forward through anchor history via keyboard shortcut (Alt+Right) and via the mouse "forward" button (where the input device exposes one).
- **FR-026d**: Back and forward navigation MUST trigger the same animated layout transition as a fresh re-anchor (FR-025) — the user's spatial intuition is preserved across history navigation.
- **FR-026e**: When no back history exists, the back action MUST be a no-op; likewise for forward when no forward history exists. These edges MUST NOT produce errors or visual disruption.
- **FR-026f**: A new query (entering the explorer with a different result set, or re-querying from search) MUST clear the anchor history stack — history is per result-set, not per session.
- **FR-026g**: The anchor history stack MUST be reasonably bounded (recommended cap: 50 entries) to prevent unbounded memory growth in long exploration sessions.
- **FR-026h**: The unified search overlay's graph mode (feature 004) MUST be reachable from inside the explorer via its existing keyboard shortcut (**Ctrl+Shift+K** — the graph-search mode that queries the SSTorytime-KV graph; *not* Ctrl+K, which opens fuzzy file search and is unrelated to graph nodes). When opened from the explorer context, confirming a graph-search result with Enter MUST re-anchor the explorer on that node — not navigate the editor — provided the node belongs to the current query result.
- **FR-026i**: When the user confirms a search result whose node is NOT in the current query result, the explorer MUST execute a new query for that node, replacing the current result set; the anchor history stack MUST be cleared as in FR-026f.
- **FR-026j**: Search-driven re-anchoring MUST trigger the same animated layout transition as click-driven re-anchoring (FR-025) so the user's spatial intuition is consistent across input methods.

#### Orientation Aids

- **FR-027**: A compass element MUST be visible at all times, showing the four spatial-axis meta-types (which direction corresponds to Contains / Leads to / Properties / Similar to).
- **FR-028**: An overview indicator (mini-map or equivalent) MUST show the camera's current viewport relative to the full landscape extent when the user has panned beyond the default fit-to-result framing.
- **FR-028a**: The mini-map MUST render the result's context territories (in their relative shapes and colors) and a rectangle representing the camera viewport. Individual nodes and edges MUST NOT be drawn in the mini-map.
- **FR-028b**: The mini-map MUST update its viewport rectangle in real time as the user pans or zooms; territory shapes MUST update only when the underlying result or anchor changes.
- **FR-029**: A reset-view action (keyboard shortcut and/or button) MUST return the camera to the default fit-to-result framing with a smooth animated transition under 1 second.
- **FR-030**: A legend MUST be available (as a permanent or toggle-able UI element) that documents the meaning of each edge style and color.

#### Quality & Robustness

- **FR-031**: The system MUST handle query results from a single isolated node up to the documented maximum (see Assumptions) without UI freezes or visual breakdown.
- **FR-032**: The system MUST gracefully render empty result sets with an explicit "no results" empty state, rather than a blank landscape.
- **FR-032a**: While a result is being laid out (both on initial open and after a re-anchor), the system MUST render the ground plane and context territories immediately (within one frame of the trigger) and fade nodes in as their positions are computed. The user MUST NOT see a blank canvas at any point during the loading window.
- **FR-032b**: Skeleton-state territories MUST already reflect the final result's contexts so the user gains spatial orientation before nodes appear.
- **FR-032c**: Edges MUST appear together with (or fractionally after) their connecting nodes, never as floating lines without endpoints.
- **FR-033**: The system MUST sustain interactive frame rates during pan and zoom up to the documented result-size ceiling on baseline hardware.
- **FR-034**: The system MUST handle results exceeding the documented ceiling by either down-sampling, clamping to a maximum, or warning the user — but MUST NOT freeze or crash.
- **FR-035**: All territory colors and edge colors MUST remain accessible (sufficient contrast, distinguishable by colorblind users via shape/style cues) in both light and dark themes.

### Key Entities

- **Graph Result**: The set of nodes and typed edges returned by a SSTorytime query. Drives what the explorer renders.
- **Anchor Node**: The node around which the Semantic Compass orientation is interpreted. Other nodes' positions are derived relative to this anchor.
- **Anchor History**: A session-scoped, bounded stack of previously visited anchors (back and forward), enabling browser-style navigation through the user's exploration trail.
- **Node**: A single concept in the graph. Visual properties: position (derived from relationships), size (from in-degree centrality), label (always front-facing), context membership.
- **Edge**: A typed relationship between two nodes. Visual properties: line style and color (per meta-type), direction.
- **Meta-type**: One of the four SSTorytime relational categories — Contains, Leads to, Properties, Similar to. Each maps to a spatial axis and an edge style.
- **Context**: A tag that groups nodes. Visual representation: a colored polygon on the ground plane.
- **Context Territory**: The ground-plane region representing a context.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A first-time user, given a one-sentence introduction to the Semantic Compass, can correctly identify the relationship type between two nodes in a sample landscape ≥ 90% of the time, by spatial position alone (no edge or label inspection).
- **SC-002**: A user who has used the explorer at least once can identify the most-referenced ("landmark") node in a 100-node result within 5 seconds of opening it.
- **SC-003**: When a result spans multiple contexts, ≥ 95% of users can correctly identify which nodes belong to which context within 10 seconds of opening the explorer, without clicking or hovering.
- **SC-004**: The explorer renders a 200-node, 500-edge result in under 2 seconds from query trigger to first interactive paint.
- **SC-005**: Pan and zoom interactions sustain ≥ 60 frames per second on baseline hardware (mid-range laptop with integrated GPU) up to the documented result-size ceiling.
- **SC-006**: All node labels remain readable (on-screen pixel size at or above the legibility threshold) across 100% of the supported zoom range and at every camera position within the pan bounds.
- **SC-007**: Hover highlight effects engage and disengage within 50ms on baseline hardware.
- **SC-008**: Camera rotation cannot be triggered through any input device or gesture — disorientation complaints related to "I lost which way is up" trend to zero in user feedback.
- **SC-009**: ≥ 80% of users can return the camera to the default framing within 3 seconds of being lost in a large landscape, using the reset-view affordance.
- **SC-010**: Re-anchoring on click completes (layout recompute + camera animation finished) within 1 second on baseline hardware for results up to the documented size ceiling.

## Assumptions

- **Relationship to feature 004**: This feature replaces the previously planned 2D node-link diagram for the graph explorer surface delivered by feature 004 (`004-unified-search-graph`). It is a deliberate visual-language reset, not an addition.
- **Anchor selection**: When a query targets a specific node, that node is the anchor. For structural or set-returning queries (e.g., chapter, context cluster), the anchor is the highest-centrality node in the result, with ties broken deterministically.
- **Re-anchoring**: User-driven re-anchoring on primary-click is IN scope for v1 (see FR-025 and User Story 6). Re-anchoring is constrained to nodes already in the current query result; bringing a wholly new node into focus still requires a new query.
- **Editing**: The Graph Explorer is a *visual* mode. Editing nodes, edges, contexts, or labels from within the explorer is OUT of scope for v1; the editor remains the source of truth.
- **Result-size ceiling**: The supported ceiling for v1 is approximately 500 nodes and 1500 edges. Results above this size are clamped, down-sampled, or warned about.
- **Multi-context nodes**: A node tagged with multiple contexts sits on the territory of its first declared context (deterministic ordering rule). Refining this rule is OUT of scope for v1.
- **Color palette**: Specific color choices for the four edge meta-types and for context territories are determined by the design layer, conforming to the application's existing dark/light theme constraints. The specification mandates only contrast and distinguishability requirements, not specific hex values.
- **Baseline hardware**: Mid-range laptop with integrated graphics, 1080p display. Devices below this baseline may experience reduced frame rates but should remain functional.
- **Input devices**: Mouse + keyboard (primary) and trackpad (secondary). Touch and mobile input are OUT of scope for v1.
- **Source data**: The graph data source is the **SSTorytime-KV** BadgerDB store populated by feature 004 via the Build Graph action (Ctrl+G). The query mechanism is feature 004's existing graph-query pipeline. This feature consumes their output without modifying them and adds no new persistence or indexing layer. See the Dependencies section for the full contract.
- **Search overlay binding**: The omnibar has two distinct keyboard shortcuts — **Ctrl+K** opens fuzzy file-content search (irrelevant to this feature), **Ctrl+Shift+K** opens graph search against the SSTorytime-KV store. All search-driven re-anchoring (FR-026h) uses the graph-search mode, never the fuzzy mode.
- **Animations**: Smooth transitions (camera reset, hover highlight) are required for usability. Decorative animations (e.g., nodes "rising into place" on initial load) are nice-to-have but not required for v1.
- **Persistence**: Camera position and zoom level are not persisted across sessions in v1. Each query session starts at the default fit-to-result framing.
