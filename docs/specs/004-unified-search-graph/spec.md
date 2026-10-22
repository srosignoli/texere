# Feature Specification: Unified Search Bar with SSTorytime-KV Graph Integration

**Feature Branch**: `004-unified-search-graph`  
**Created**: 2026-04-17  
**Status**: Draft  
**Input**: User description: "Unified search bar with SSTorytime-KV graph integration"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Quick Navigation via Fuzzy File Search (Priority: P1)

A user is editing an N4L document and wants to jump to a specific node or file without leaving the editor. They press Ctrl+K, type a few characters, and the overlay instantly shows fuzzy-matched results from file content. They press Enter and the editor jumps to the matching line in the correct file.

**Why this priority**: This is the core navigation flow — it delivers immediate value with zero graph dependency and works even before the user has built their graph. It is the fastest path to improving daily editor usability.

**Independent Test**: Can be fully tested by opening any N4L file, pressing Ctrl+K, typing a search term, and pressing Enter to confirm the editor jumps to the correct line. No graph or BadgerDB required.

**Acceptance Scenarios**:

1. **Given** the editor is open with an N4L file loaded, **When** the user presses Ctrl+K, **Then** the search overlay appears in under 50ms with focus on the text input.
2. **Given** the overlay is open and the user types a search term, **When** characters are entered, **Then** fuzzy-matched results from file content appear immediately (Tier 1) without any perceptible delay.
3. **Given** results are displayed and the user presses Enter on a result, **Then** the overlay closes and the editor scrolls to and highlights the matching node's source line in under 50ms.
4. **Given** the overlay is open, **When** the user presses Escape, **Then** the overlay closes and editor focus is restored.

---

### User Story 2 - Graph-Powered Node Search (Priority: P2)

A user wants to find nodes across their knowledge graph by concept, context tag, or chapter membership — richer than a text search. They press Ctrl+K, type a query (possibly with a command prefix like `\context` or `\chapter`), and after a brief pause see graph results showing the matching node, its immediate typed neighbors (orbit), its chapter, and its context tags. They press Enter to jump to that node in the file.

**Why this priority**: Graph search is the feature's main differentiator. It enables semantic navigation across the knowledge graph, not just text matching. It depends on a built graph (Ctrl+G must have run), so it is slightly higher friction than fuzzy search — but it is the reason for this feature's existence.

**Independent Test**: Can be fully tested by building a graph (Ctrl+G), opening the overlay, running a text query or command-prefix query, and verifying that graph results appear below fuzzy results with orbit neighbors and metadata shown. Enter navigates to the node.

**Acceptance Scenarios**:

1. **Given** the graph has been built (Ctrl+G completed), **When** the user types a simple text query (e.g., `supermarket`), **Then** Tier 2 results appear within 100ms of the typing pause, showing node text, immediate neighbors with arrow types, chapter, and context tags.
2. **Given** the user types a context-scoped query (e.g., `notes about chinese context restaurant`), **When** the debounce fires, **Then** only nodes matching the context filter are shown in Tier 2.
3. **Given** the user types a chapter-scoped query (e.g., `\chapter brain`), **When** the debounce fires, **Then** results are filtered to nodes in that chapter.
4. **Given** the graph has NOT been built, **When** a query would produce graph results, **Then** Tier 2 shows the message "Build your graph first (Ctrl+G) to enable graph search."
5. **Given** a result is displayed, **When** the user presses Enter, **Then** the overlay closes and the editor navigates to that node's source line.

---

### User Story 3 - Structural & Listing Queries (Priority: P3)

A user wants to see a chapter table of contents (`\chapter any`), explore a context cluster (`\context restaurant`), look up arrow definitions (`\arrow ph`), view a full note page (`\notes brain`), or run a path query (`\from A \to B`). These queries return structured lists or chains rather than single nodes.

**Why this priority**: These queries cover specialist use cases that go beyond simple lookup. They are valuable but lower frequency than plain search. Path queries anticipate a future graph explorer feature.

**Independent Test**: Can be fully tested by running each command-prefix query in the overlay and verifying that the result panel shows the appropriate structured output (table of contents list, context cluster, arrow definitions, note page, or path chain as plain text).

**Acceptance Scenarios**:

1. **Given** the overlay is open, **When** the user types `\chapter any`, **Then** the result panel shows a table of contents listing all chapters.
2. **Given** the overlay is open, **When** the user types `\context restaurant`, **Then** context clusters related to "restaurant" are shown.
3. **Given** the overlay is open, **When** the user types `\arrow ph`, **Then** arrow definitions matching "ph" are displayed.
4. **Given** the overlay is open, **When** the user types `\notes brain`, **Then** the full note-form page view for the "brain" chapter is displayed.
5. **Given** the user types a structural query containing `\from` and `\to`, `\paths`, or `\sequence`, **When** the debounce fires, **Then** the result chain is displayed as formatted plain text in the overlay.
6. **Given** a structural/listing query result is shown, **When** the user presses Enter on a node-like result, **Then** the editor navigates to that node's source line (if applicable); for non-navigable items, Enter is a no-op.

---

### User Story 4 - Result Limiting and Exact/Accent-Insensitive Matching (Priority: P3)

A user needs fine-grained control over search: limiting the number of results (`\limit 3`), requiring exact matches (`!A!` or `|A|`), or performing accent-insensitive matching (`(fangzi)` matching fángzi). These search modifiers must be correctly parsed and passed to the graph search backend.

**Why this priority**: These are power-user modifiers that improve precision but are not required for the core navigation loop. They can be added incrementally after the main tiers work.

**Independent Test**: Can be fully tested by running queries with each modifier and verifying result counts, exact match filtering, and accent-insensitive hit matching.

**Acceptance Scenarios**:

1. **Given** a query `supermarket \limit 3`, **When** the debounce fires, **Then** at most 3 graph results are shown.
2. **Given** a query `!supermarket!`, **When** evaluated, **Then** only nodes with the exact string "supermarket" (not substrings) are returned.
3. **Given** a query `(fangzi)`, **When** evaluated, **Then** nodes containing accented variants such as "fángzi" are included in results.

---

### Edge Cases

- What happens when the graph DB is corrupted or unreadable? → Tier 2 shows an error message; Tier 1 continues working normally.
- What happens when a graph result's node text matches multiple source locations in the in-memory index (duplicate node text across files)? → All matching locations are listed as sub-results; the user selects which file/line to navigate to.
- What happens when the in-memory index has not finished building at application start when the user opens the overlay? → Tier 2 is disabled with a "Indexing files, please wait…" message until the index is ready.
- What happens when a query produces zero results in either tier? → Each tier section shows "No results" rather than being hidden, so the user knows the query was processed.
- What happens when both tiers return results for the same node? → The same node may appear in both tiers; no deduplication is required in v1.
- What happens when the N4L file containing a result node has been deleted or renamed? → Navigating to that result shows an error toast; the overlay remains open.
- What happens when the user types very quickly before the debounce fires? → Only the final debounced query is sent to the graph backend; intermediate keystrokes do not produce redundant requests.
- What happens when a `\limit` modifier is combined with a prefix command? → The limit is applied to the combined result set returned by the backend.
- What happens when Tab is pressed on a result? → For now, Tab is a no-op (graph explorer is a future feature). The action is acknowledged without closing the overlay.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST open a search overlay panel when the user presses Ctrl+K or activates the search icon in the toolbar, within 50ms of the trigger.
- **FR-002**: The overlay MUST appear over the editor without navigating away from or unmounting the editor view.
- **FR-003**: The overlay MUST contain a text input that receives keyboard focus automatically on open.
- **FR-004**: As the user types, the overlay MUST display Tier 1 (fuzzy file content) results immediately using the existing fuzzy search implementation.
- **FR-005**: After a debounce pause, the overlay MUST dispatch the query to the graph search backend and display Tier 2 (graph-powered) results below Tier 1 results, within 100ms of the debounce firing. While the query is in-flight, Tier 2 MUST display a static "Searching…" text label.
- **FR-006**: Tier 2 results MUST display: matching node text, immediate orbit neighbors with arrow types, the chapter the node belongs to, and its context tags.
- **FR-007**: The system MUST parse and correctly route all searchN4L query prefixes: `\chapter`, `\in`, `\notes`, `\context`, `\arrow`, `\limit`, `\from`, `\to`, `\paths`, `\sequence`, and the exact-match (`!…!`, `|…|`) and accent-insensitive (`(…)`) syntax.
- **FR-008**: Pressing Enter on any navigable result MUST close the overlay and scroll the editor to the corresponding source line in the correct file, within 50ms.
- **FR-009**: Pressing Escape OR clicking anywhere outside the overlay MUST close the overlay and restore editor focus without any navigation action.
- **FR-010**: Arrow keys MUST move selection among results; the selected result MUST be visually highlighted.
- **FR-011**: Pressing Tab on a result MUST be accepted without closing the overlay (no-op for now, reserved for future graph explorer).
- **FR-012**: If the graph has not been built (no BadgerDB data available), Tier 2 MUST display the message: "Build your graph first (Ctrl+G) to enable graph search."
- **FR-013**: Structural queries (`\from`/`\to`, `\paths`, `\sequence`) MUST display their results as formatted plain text chains in the overlay.
- **FR-014**: Listing queries (`\chapter any`, `\context [term]`, `\arrow [term]`, `\notes [chapter]`) MUST display their respective structured output in the overlay result panel.
- **FR-015**: The Go backend MUST expose a Wails binding that accepts the parsed query and returns typed result structs to the frontend.
- **FR-016**: The Go backend MUST call the SSTorytime-KV searchN4L API with the user's query and return results including node text, orbit neighbors (with arrow types), chapter, and context tags. After receiving graph results, the backend MUST enrich each result with source location (file path, line number) by looking up node text in the in-memory index built at application start.
- **FR-018**: At application start, the system MUST parse all N4L files in the active collection and build an in-memory index mapping node text to source file path and line number, so that graph results can be linked to their source location without re-scanning files at query time.
- **FR-017**: If Tier 1 or Tier 2 returns no results, each tier MUST display a "No results" indicator rather than collapsing the section.
- **FR-019**: Each tier MUST display a maximum of 10 results by default. The user MAY override this cap using the `\limit N` modifier, which applies to the Tier 2 graph results.

### Key Entities

- **SearchQuery**: The raw text entered by the user, plus a parsed representation of any command prefix, modifiers, and the core search term.
- **FuzzyResult**: A Tier 1 result — file path, matching line number, matching text snippet.
- **GraphResult**: A Tier 2 result — node text, chapter, context tags, list of orbit neighbors (each with arrow type and neighbor node text), source file path and line number (resolved from the startup-built in-memory index, not from BadgerDB).
- **OrbitNeighbor**: A single immediate neighbor of a node — arrow type label, neighbor node text, direction (inbound/outbound).
- **StructuralResult**: Output of a path or chain query — an ordered list of node texts connected by arrow types, displayed as plain text.
- **ListingResult**: Output of a listing command — a structured list (table of contents, context cluster, arrow definitions, or note page content).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The search overlay opens in under 50ms after Ctrl+K is pressed, as measured from keypress to overlay visible with input focused.
- **SC-002**: Tier 1 (fuzzy) results appear as the user types with no perceptible delay (under 16ms per keystroke, maintaining 60fps UI responsiveness).
- **SC-003**: Tier 2 (graph) results appear within 100ms of the debounce pause completing after the user stops typing.
- **SC-004**: Pressing Enter on a result navigates the editor to the correct source line in under 50ms.
- **SC-005**: All nine searchN4L command prefixes (`\chapter`, `\in`, `\notes`, `\context`, `\arrow`, `\limit`, `\from \to`, `\paths`, `\sequence`) and the three match modifiers (exact `!…!`/`|…|`, accent-insensitive `(…)`) are correctly parsed and produce non-error results when the graph is built.
- **SC-006**: Tier 2 results for a node always include at least the node text, chapter, and orbit (neighbors with arrow types) when that data exists in the graph.
- **SC-007**: The overlay is fully operable via keyboard alone (open, navigate results, confirm, dismiss) without requiring mouse interaction.

## Clarifications

### Session 2026-04-17

- Q: Does SSTorytime-KV BadgerDB store source file path and line number per node? → A: Neither is stored in BadgerDB. All collection files are parsed at application start; a node-text → (file path, line number) in-memory index is built from this and used to resolve source location for graph results at query time.
- Q: What should the Tier 2 area show while a graph query is in-flight? → A: A static "Searching…" text label in the Tier 2 section.
- Q: How is the SSTorytime-KV Go module referenced in this project? → A: Published Go module, imported by module path (github.com/srosignoli/sstorytime-kv).
- Q: What is the default maximum results shown per tier when no `\limit` is specified? → A: 10 per tier (10 fuzzy + 10 graph = 20 total maximum displayed).
- Q: Should clicking outside the search overlay close it? → A: Yes — clicking anywhere outside the overlay closes it and restores editor focus.

## Assumptions

- The existing fuzzy search implementation in the frontend is reusable as-is for Tier 1 results; no significant refactoring is needed.
- The SSTorytime-KV Go module is a published module imported as `github.com/srosignoli/sstorytime-kv` and exposes a `searchN4L` function compatible with the query syntax described in this spec.
- The BadgerDB populated by Ctrl+G (Build graph) is the same database that SSTorytime-KV reads for graph search; no additional indexing step is required.
- The Wails binding layer already exists and can be extended with new bound methods without architectural changes.
- Source file path and line number are NOT stored in the graph. Instead, all N4L files in the collection are parsed at application start and an in-memory index (node text → file path + line number) is maintained. Graph results are enriched with source location by looking up node text in this index at query time.
- Debounce duration for graph queries is set to approximately 200ms — long enough to avoid spamming BadgerDB on fast typing, short enough to feel responsive.
- The graph explorer (Cytoscape.js) is a future feature; Tab on a result is a no-op in this version.
- `graph_report` analysis (centrality, loops, stats dashboard) is explicitly out of scope for this feature.
- The overlay does not replace or unmount the editor; it is rendered as an absolute-positioned or modal layer on top of the existing editor DOM.
- Mobile and touch-screen support is out of scope for v1; keyboard-only navigation is the primary interaction model.
