# Feature Specification: N4L File Parser — Layer I Content I/O

**Feature Branch**: `002-n4l-parser`  
**Created**: 2026-04-07  
**Status**: Draft  
**Input**: User description: "001-n4l-parser"

---

## Clarifications

### Session 2026-04-07

- Q: What stable identifier should the parser assign to each node for persistence in the scheduling database? → A (final): Sole system-managed scheme — `# @id <shortid>` written into the `.n4l` file by the system on first save of any study-marked node. `@label` is a cross-reference anchor only and plays no role in node identity. Hash-based IDs were rejected (fragile on text edits); label-based IDs were rejected (labels can be renamed/removed by the user for cross-reference reasons, breaking FSRS history identically to hashes).
- Q: Should the application automatically detect filesystem changes (files added/removed/renamed) in the collection directory while running? → A: Yes — automatic directory watching; Column 1 updates without a restart.
- Q: What is the minimum threshold for an ALL CAPS line to be treated as a reminder (to avoid false positives from acronyms and short tokens)? → A: Minimum 3 words, all uppercase (excluding trailing inline comment).
- Q: In what order should files appear in Column 1? → A: Alphabetical by filename (case-insensitive).
- Q: What should Columns 2 and 3 display while an on-demand parse is in progress? → A: Show a loading indicator (spinner or skeleton state); replace with parsed content when done.

---

## Loading Model — Performance & UX Rationale

The application works with an entire **collection** — a directory of `.n4l` files — not a single file at a time. Column 1 of the structural browser lists every file/chapter in the collection. This raises a performance question: how much parsing happens at startup vs. on demand?

The two-phase model is the right answer, and it is fully feasible:

**Phase 1 — Directory scan (at startup, ~milliseconds):** The application scans the collection directory for `.n4l` files and reads only the chapter-title lines (`- title`) from each to populate Column 1. A typical collection of 200 files scans in well under one second because only a handful of bytes per file are needed. Column 1 is ready before the user can interact.

**Phase 2 — On-demand full parse (on file/chapter selection, <1 second):** When the user selects a file or chapter in Column 1, the selected file is fully parsed to populate Columns 2 and 3 (context tags and nodes). Parsing a file of up to 10,000 lines completes in well under a second, which is imperceptible.

**Why no bulk background parse is needed at startup:** The reminder panel badge and study scheduling queue read from the embedded database (SQLite), which is populated incrementally as the user saves files. On first run with an empty database, the counts start at zero and populate naturally as files are saved. Re-parsing the entire collection in the background at startup would add complexity and resource cost for no user-visible benefit.

**Multi-chapter files:** The primary convention is one file = one chapter. Files with multiple `- title` lines are valid but exceptional. In Column 1, a single-chapter file is shown as one flat entry (filename or chapter title). A multi-chapter file is shown as a collapsible group with the filename as the parent entry and each chapter title as a child entry.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Load a Collection and Browse Its Files (Priority: P1)

A user points the application at their notes directory. The application immediately displays all `.n4l` files (and their chapters) in Column 1, sorted and grouped so the user can navigate the collection at a glance. No delay is perceptible — the collection appears before the user has finished looking up.

**Why this priority**: This is the entry point to every other feature. If the collection cannot be loaded and displayed quickly, the application is unusable regardless of how well individual files parse.

**Independent Test**: Point the application at a directory of 50+ `.n4l` files and verify Column 1 is populated with all files and chapters in under 2 seconds, with no full parse having occurred yet.

**Acceptance Scenarios**:

1. **Given** a directory containing N `.n4l` files, **When** the application starts with that directory as the collection, **Then** all N files appear in Column 1 within 2 seconds.
2. **Given** a file with a single `- title` line, **When** the directory scan completes, **Then** the file appears as a single flat entry in Column 1 labelled with that chapter title (or filename if no title line is present).
3. **Given** a file with multiple `- title` lines, **When** the directory scan completes, **Then** the file appears in Column 1 as a collapsible group: the filename as parent, each chapter title as a child entry.
4. **Given** a directory containing non-`.n4l` files, **When** the scan completes, **Then** only `.n4l` files appear in Column 1; other files are silently ignored.
5. **Given** a `.n4l` file that cannot be read (permissions error, encoding error), **When** the scan completes, **Then** the file still appears in Column 1 with an error indicator, and the rest of the collection loads normally.

---

### User Story 2 — Select a File and See Its Content (Priority: P1)

A user clicks a file or chapter in Column 1. The application parses that file on demand and immediately populates Column 2 with its unique context tags and Column 3 with its nodes. The structural browser reflects the file content exactly — every node, arrow, and context the user sees corresponds to what is written in the source.

**Why this priority**: Tied with US1 as the core interactive loop. All editing, navigation, and review flows begin here.

**Independent Test**: Select any file in Column 1 and verify Columns 2 and 3 are populated correctly and completely within 1 second of the click.

**Acceptance Scenarios**:

1. **Given** a file selected in Column 1, **When** the on-demand parse completes, **Then** Column 2 shows all unique context tags from that file's active chapter, and Column 3 shows all root nodes matching the default context filter.
2. **Given** a chapter with context operators (`::`, `+::`, `-::`), **When** the file is parsed, **Then** each node is tagged with the effective context set at its position (computed by replaying all operators from chapter start).
3. **Given** a chapter with `_sequence_` in the active context, **When** the file is parsed, **Then** consecutive nodes in that context are identified as an ordered sequence.
4. **Given** a node followed by ditto lines (`"  (arrow) target`), **When** the file is parsed, **Then** ditto lines inherit the parent node's identity and their arrows are correctly attributed.
5. **Given** a file containing `@label` and `$label.N` / `$PREV.N` references, **When** the file is parsed, **Then** references are resolved to the correct node or ditto position within that file.
6. **Given** a multi-chapter file, **When** a child chapter entry is selected in Column 1, **Then** only that chapter's content is shown in Columns 2 and 3, not the content of other chapters in the same file.

---

### User Story 3 — Study Markers Are Recognised and Mapped to Nodes (Priority: P2)

A user annotates nodes with study markers (`# @card`, `# @cloze`, `# @study` blocks, `# @end`). When the user saves a file, every marked node is identified for scheduling — and only those nodes. The correct card type (basic, reverse, cloze, sequence) is assigned per the marker rules and persisted for the scheduling layer.

**Why this priority**: Study scheduling cannot create or update any entries without an accurate map of which nodes carry which study markers. This unlocks the core learning loop.

**Independent Test**: Annotate a set of nodes with various study markers, save the file, and verify the scheduling layer receives the correct list of studyable nodes with the right card types and no false positives.

**Acceptance Scenarios**:

1. **Given** a `# @card` marker above a node, **When** the file is saved, **Then** that node is flagged as a basic card (reverse direction determined by arrow pair definition).
2. **Given** a `# @study` / `# @end` block, **When** the file is saved, **Then** every node within the block is flagged as studyable; nodes outside the block are not.
3. **Given** `# @cloze` above a node with `{braces}`, **When** the file is saved, **Then** that node is flagged as a cloze card with the braced text identified as the deletion target.
4. **Given** a `# @study sequence` block in a `_sequence_` context, **When** the file is saved, **Then** sequence card variants (forward, gap, order) are generated for nodes in the block.
5. **Given** individual markers inside a study block, **When** the file is saved, **Then** individual markers take precedence over block defaults for those nodes.

---

### User Story 4 — Reminders and Priority Markers Are Surfaced (Priority: P3)

A user writes ALL CAPS lines (reminders) and `# @priority N` markers in their notes. On save, reminders are persisted so the reminder panel can show them across the entire collection. Priority values are available to the scheduling layer — without the user doing anything beyond their normal note-writing workflow.

**Why this priority**: Reminders and priority markers are quality-of-life features that don't block the core loop, but they are important for knowledge-debt tracking and curation workflows.

**Independent Test**: Write an ALL CAPS reminder and a `# @priority 10` marker, save the file, and verify the reminder appears in the reminder panel and the priority value is available to the scheduling layer.

**Acceptance Scenarios**:

1. **Given** a line in ALL CAPS (with or without trailing comment), **When** the file is saved, **Then** it is identified as a reminder with its file path and line number recorded.
2. **Given** a `# @priority N` marker (0–100), **When** the file is parsed, **Then** the priority value is attached to the node or block it applies to.
3. **Given** `# @source`, `# @ref`, and `# @progress` markers, **When** the file is parsed, **Then** provenance metadata is captured and associated with the relevant nodes.

---

### Edge Cases

- What happens when the collection directory is empty (no `.n4l` files)? Column 1 shows an empty state with a prompt to add `.n4l` files to the directory.
- If a user clicks a second file while the first is still parsing, the in-flight parse is cancelled and the new file's parse begins immediately with its own loading indicator.
- What happens when a `.n4l` file contains no `- title` line (no chapter declaration)?
- What happens when a `::` operator appears with no tags — does it reset to an empty context set?
- How does the parser handle a `# @study` block that is never closed (reaches EOF or a `::` reset)?
- What happens when a `$label.N` reference points to a label that does not exist in the file?
- How does the parser treat a ditto line that appears without a preceding node (orphan ditto)?
- What happens when `# @priority` is given a value outside the 0–100 range?
- What happens when a malformed arrow (unmatched parentheses or unknown short code) appears in a node line?
- When the collection directory changes while running (files added, renamed, or deleted), Column 1 updates automatically via filesystem watching (FR-006b). In-flight parses of a deleted file are discarded gracefully.
- What happens if a write-back operation (FR-019–021) fails mid-write (e.g., disk full)? The temporary file is discarded; the original file is untouched; an error is reported to the caller.
- What if two study-marked nodes in the same file receive the same generated `# @id`? FR-023 mandates uniqueness checking before write; regeneration is attempted up to 10 times before the error is surfaced to the user.
- What if an extract's `# @id` is already present when the user adds a study marker (promoting it to an item)? FR-018d: no new ID is injected; the existing `# @id` is reused. The FSRS scheduler creates a new item record with the same `node_id`; the extract record is marked as promoted.
- What if the editor is open at an extract node when the scheduler injects a `# @id` for it? The injection writes via temp-file-then-rename; the editor must reload the file (or the Wails bridge notifies the frontend of the external change). File reload is handled by the editor integration feature, not by this feature.

## Requirements *(mandatory)*

### Functional Requirements

**Collection loading (Phase 1 — directory scan)**

- **FR-001**: The application MUST accept a collection directory path at startup and scan it for all `.n4l` files (non-recursive; subdirectories are ignored unless specified as a separate future capability).
- **FR-002**: The directory scan MUST read only the chapter-title lines (`- title`) from each file to populate Column 1; it MUST NOT perform a full parse during the scan.
- **FR-003**: The scan MUST complete and Column 1 MUST be populated within 2 seconds for a collection of up to 500 files on a typical desktop machine.
- **FR-004**: For each file with a single chapter, Column 1 MUST display one entry using the chapter title (falling back to the filename if no title line is present). For each file with multiple chapters, Column 1 MUST display a collapsible group with the filename as parent and each chapter title as a child entry. Files MUST be sorted case-insensitively alphabetical by filename; within a multi-chapter group, chapters appear in file order.
- **FR-005**: Files that cannot be read (permission errors, unrecognised encoding) MUST appear in Column 1 with an error indicator; they MUST NOT prevent the rest of the collection from loading.
- **FR-006**: Non-`.n4l` files and subdirectories encountered during the scan MUST be silently ignored.
- **FR-006b**: The application MUST watch the collection directory for filesystem changes (files added, removed, or renamed) while it is running. When a change is detected, Column 1 MUST update automatically — new files appear, removed files disappear, renamed files update their entry — without requiring a restart or manual refresh. Changes to file content on disk (outside the application) do not trigger an automatic re-parse; re-parse occurs only on the next selection of that file.

**On-demand full parse (Phase 2 — file selection)**

- **FR-007**: When a file or chapter is selected in Column 1, Columns 2 and 3 MUST immediately show a loading indicator (spinner or skeleton state). The application MUST complete the full parse and replace the loading state with parsed content within 50 ms for files up to 10,000 lines (constitution performance standard).
- **FR-008**: The parser MUST compute the effective context set at each node position by replaying all context operators (`::`, `+::`, `-::`) from the chapter start; each node MUST store a snapshot of its effective context.
- **FR-009**: The parser MUST identify `_sequence_` as a special context tag and mark consecutive nodes in that context as an ordered sequence.
- **FR-010**: The parser MUST resolve ditto lines (`"`) to their parent node and correctly attribute their arrows to that parent node.
- **FR-011**: The parser MUST resolve `@label` definitions and `$label.N` / `$PREV.N` reference expressions to the corresponding node or ditto position within the same file.
- **FR-012**: The parser MUST handle annotation symbols (`%`, `=`, `**`, `>>`) defined in SSTconfig as inline arrow markers within node text.
- **FR-013**: The parser MUST read `# @id <shortid>` lines (immediately above a node, before any study marker) as the sole system-assigned node identifier. The stable key is `<filepath>:<chapter>:id:<shortid>`. If no `# @id` line is present, `Node.ID` is empty — the node is parsed and returned but cannot be tracked by either scheduler until an ID is assigned. `@label` values are cross-reference anchors only and MUST NOT be used as node identifiers. A `# @id` marker is shared across all scheduling record types that reference the same node (extract, item) — the scheduler distinguishes record types via the `type` column in SQLite, not via separate identifiers.
- **FR-013b**: Re-parsing the same file content MUST always produce identical node identifiers; no non-determinism from ordering or timestamps.

**Save-triggered parse (study markers, reminders, provenance)**

- **FR-014**: When a file is saved (Ctrl+S), the parser MUST extract all study markers — individual (`# @card`, `# @card reverse`, `# @card no-reverse`, `# @cloze`) and block (`# @study`, `# @study reverse`, `# @study sequence`, `# @end`) — and map them to their annotated nodes per the specification rules.
- **FR-015**: When a file is saved, the parser MUST identify a line as a reminder if and only if its text (after stripping any trailing `# …` inline comment) consists entirely of uppercase characters and contains at least 3 whitespace-separated words. Single-word or two-word all-caps tokens (acronyms, short codes) are NOT reminders. Identified reminders are recorded with their file path and line number for persistence.
- **FR-016**: When a file is saved, the parser MUST extract `# @priority N` values (0–100) and associate them with the node or block they annotate.
- **FR-017**: When a file is saved, the parser MUST extract `# @source`, `# @ref`, and `# @progress` provenance markers and associate them with their annotated nodes.

**Error handling (all phases)**

- **FR-018**: The parser MUST handle parse errors on individual lines without aborting — it MUST return all successfully parsed content alongside a list of per-line errors, so partial files remain usable.

**Schedulable object types and identity rules**

The scheduling layer works with three distinct types of objects. The parser's responsibility is to expose the raw material for all three; identity assignment rules differ by type.

- **FR-018a**: The parser MUST expose all nodes (marked and unmarked alike) with their line numbers in `ParsedFile`, so that both the FSRS scheduler and the incremental reading scheduler can identify specific nodes for ID injection and state tracking.
- **FR-018b**: Topics (a chapter, or a chapter + context-tag set) MUST be identified structurally as `<filepath>:<chapter-title>` or `<filepath>:<chapter-title>:<sorted-context-tags>`. No `# @id` injection into the file is required or permitted for topic identity — the chapter title is the stable anchor. The parser already produces the data needed to construct this key.
- **FR-018c**: Extracts (unmarked nodes that have entered the incremental reading distillation pipeline) MUST receive a `# @id` marker injected by the incremental reading scheduler at the moment a node enters the pipeline — not on file save. The injection uses the same write-back mechanism and format as items (FR-019), but is triggered by `internal/learning`, not by `app.go`.
- **FR-018d**: A `# @id` marker is the node's identity, not a record type marker. When an extract is promoted to an item (user adds a study marker to a node that already has `# @id`), no new `# @id` is injected. The incremental reading scheduler marks the extract record as promoted in SQLite; the FSRS scheduler creates a new item record referencing the same `node_id`. One `# @id` per node, regardless of how many scheduling records reference it.

**Write-back operations (targeted file modifications)**

- **FR-019**: When a file is saved (Ctrl+S) and a study-marked node has no existing `# @id` marker, the system MUST generate a unique 8-character base-36 identifier and insert a `# @id <shortid>` line immediately above that node (before any existing study marker). This applies regardless of whether the node has an `@label`. The inserted line MUST be the only change to the file — all other lines MUST remain byte-for-byte identical.
- **FR-019b**: The write-back API MUST also expose a single-node injection function callable by the incremental reading scheduler for extract nodes. Given a file path and a node's line number, it generates and injects a `# @id` for that specific node. The same atomicity, uniqueness, and fidelity guarantees as FR-019 apply.
- **FR-020**: The system MUST provide a write-back operation to resolve a reminder: given a file path and line number of an ALL CAPS reminder, it lowercases that line's text while preserving any trailing inline comment. All other lines MUST remain byte-for-byte identical.
- **FR-021**: The system MUST provide a write-back operation to update a `# @progress` marker: given a file path, line number, and new value, it replaces the value after `# @progress ` on that line. All other lines MUST remain byte-for-byte identical.
- **FR-022**: All write-back operations MUST be atomic with respect to the filesystem: the new content MUST be written to a temporary file in the same directory, then renamed over the original. A crash mid-write MUST leave the original file intact.
- **FR-023**: The `# @id` values generated by FR-019 MUST be unique within a file; before writing, the system MUST verify the generated ID does not already appear in the file and regenerate if necessary.

### Key Entities

- **Collection**: The set of all `.n4l` files in the configured directory; the top-level unit the application works with.
- **File**: A single `.n4l` file within the collection; identified by its path relative to the collection root; contains one or more chapters.
- **Chapter**: A named section begun with a `- title` line; the primary navigable unit shown in Column 1; contains context operators and nodes.
- **ContextState**: The effective set of context tags active at a given position, computed by replaying all operators from chapter start.
- **Node**: A root-level statement; carries text, its effective context snapshot, zero or more outgoing arrows, optional study/reminder/priority metadata, and a stable system-assigned identifier (`# @id <shortid>`) if one has been written into the file. Nodes without a `# @id` marker have no stable ID and cannot be tracked by the scheduling layer until the system assigns one on next save.
- **IDMarker**: A `# @id <shortid>` comment line immediately above a node; written by the system either at save time (for study-marked items, via `app.go`) or on demand (for extracts entering the distillation pipeline, via `internal/learning`). Once written, it is the node's sole stable identity for all schedulers. A node that has been promoted from extract to item carries one `# @id` that both the incremental reading record and the FSRS item record reference via `node_id` in SQLite. `@label` values serve a different purpose (cross-referencing) and are never used as identity tokens.
- **Topic**: A schedulable unit for the incremental reading scheduler; defined as a chapter (`<filepath>:<chapter-title>`) or a chapter filtered by a context-tag set. Identity is structural — no `# @id` is written to the file for topics.
- **Extract**: An unmarked node in the incremental reading distillation pipeline. The scheduler assigns it a `# @id` when it first enters the pipeline; from that point the node is tracked persistently. Promoting an extract (adding a study marker) does not change or duplicate its `# @id`.
- **Item**: A study-marked node (`# @card`, `# @cloze`, or within a `# @study` block) tracked by the FSRS scheduler. Receives a `# @id` on first save with a study marker.
- **Arrow**: A typed relationship from a node (or ditto) to a target string; identified by its short code (e.g., `(means)`, `(e.g.)`).
- **DittoLine**: A continuation of the immediately preceding node (`"`); inherits the parent's identity; carries its own arrows.
- **Label**: A named anchor (`@label`) attached to a node or ditto; referenced via `$label.N` or `$PREV.N` within the same file.
- **StudyMarker**: Metadata attached to a node or block specifying card type and directionality for the learning layer.
- **Reminder**: An ALL CAPS line representing an open knowledge-debt item; carries file path, line number, and raw text.
- **ProvenanceMarker**: A `# @source` / `# @ref` / `# @progress` annotation recording the origin of knowledge.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A collection of up to 500 files is scanned and Column 1 is populated within 2 seconds of the application starting, with no full parse having occurred.
- **SC-002**: Selecting a file or chapter in Column 1 populates Columns 2 and 3 in under 50 ms for files up to 10,000 lines (constitution §Performance Standards: "N4L parse + context replay < 50 ms"). A loading indicator is shown immediately on selection so the user never perceives a frozen UI.
- **SC-003**: All nodes, arrows, chapters, and context states present in a correctly written `.n4l` file are represented in the parsed output — zero omissions, zero spurious additions.
- **SC-004**: Re-parsing the same unchanged file always produces identical output (determinism), ensuring no spurious diffs or state corruption on repeated select/save cycles.
- **SC-005**: A file containing errors on some lines is still returned with all valid content intact; the user sees a specific, actionable error message for each problematic line rather than a generic failure.
- **SC-006**: 100% of study markers in well-formed files are correctly attributed to their target nodes on save, ensuring the learning layer never misses a studyable item or schedules a non-studyable one.
- **SC-007**: All lines qualifying as reminders (all-uppercase text, at least 3 words, excluding trailing comment) are recorded on save with correct file path and line number — zero false negatives and zero false positives against a reference test corpus. Single-word and two-word all-caps tokens are correctly excluded.
- **SC-008**: After any write-back operation (ID injection, reminder resolution, `@progress` update), every line in the file not targeted by the operation is byte-for-byte identical to the pre-write version — verified by a diff of unaffected lines in tests.

## Assumptions

- The collection is a flat directory of `.n4l` files; recursive subdirectory scanning is out of scope for this feature.
- Arrow types referenced in `.n4l` files are defined in the SSTconfig directory; the parser validates short codes against this registry and flags unknown codes as errors.
- Annotation symbols (`%`, `=`, `**`, `>>`) used as inline markers are also defined in SSTconfig; the parser reads this registry at startup.
- If the SSTconfig directory is absent, the parser operates in degraded mode (no arrow validation) and reports a startup warning.
- `.n4l` files are UTF-8 encoded; other encodings are out of scope.
- Cross-file references (e.g., `$label` spanning multiple files) are out of scope for this layer and are handled by the graph layer (Layer II).
- Files up to ~10,000 lines are the primary performance target for on-demand parse; very large files (>50,000 lines) are not a goal for this iteration.
- The reminder panel badge and study scheduling queue read from the embedded database (SQLite), which is populated on file save — not from re-parsing the entire collection at startup. On first run with an empty database, reminder counts and study state start at zero and populate as files are saved.
- The `internal/content` package contains both a decode path (parser, strictly read-only) and a write-back path for targeted, structured modifications (`# @id` injection, reminder resolution, `@progress` update). The write-back path never modifies any line it was not explicitly asked to change. Full buffer replacement (user edits) is the responsibility of the editor integration feature and is out of scope here.
