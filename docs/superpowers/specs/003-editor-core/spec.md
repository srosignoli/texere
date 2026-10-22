# Feature Specification: Editor Core — Hybrid Band Layout Workbench

**Feature Branch**: `003-editor-core`
**Created**: 2026-04-09
**Status**: Draft
**Input**: User description: "003-editor-core — three-band workbench with structural navigation, button bar, and source/read/tutor editor tabs"

---

## 1. Overview

The Texere workbench uses a hybrid layout combining Squeak-style structural navigation with source-first editing. The window is divided into **three horizontal bands**. Column 4 (Orbit Minimap) and the graph build/sync/history pipeline are **out of scope** for this feature; they will be specified in a later feature once SSTorytime-KV integration is ready.

---

## 2. Layout

### 2.1 Top Band — Structural Navigation

Three columns arranged horizontally:

| Col | Name | Contents |
|-----|------|----------|
| 1 | **Chapter / File** | Lists all chapters from open `.n4l` files. Selecting a chapter filters Column 2. |
| 2 | **Context Tags** | All unique context tags from the selected chapter as toggleable chips. The parser replays `::`, `+::`, `-::` operators to compute the effective context set at each line. The `_sequence_` tag receives distinct visual treatment (chain icon). |
| 3 | **Nodes** | Structural triage dashboard. Root nodes matching the active context filter, rendered in three modes: pure-text extracts (no arrows), collapsible structured trees (arrows/dittos), and sequence timelines. A quick filter bar ([All] / [Extracts] / [Structured]) narrows the visible set without altering the source. Column 3 fills the full top-band width. |

Column 4 (Orbit Minimap) was removed — its placeholder div has been deleted from `TopBand.svelte`. It will be reintroduced as a separate feature once SSTorytime-KV graph integration is ready.

### 2.2 Middle Band — Context-Sensitive Button Bar

Core actions: **+node**, **+link**, **+context**, **save** (Ctrl+S), **search**, **undo**.

Additional controls:

- **Reminder counter badge**: Shows count of open ALL CAPS reminders (amber). Clicking opens the reminder panel.
- **Arrow palette** (+link dropdown): Arrows grouped by SST meta-type; recently used arrows at top. Powered by SSTconfig arrow registry.

The following controls are **deferred** to later features: Build graph (Ctrl+G), Build indicator, Sync, History (graph-integration feature); Study, Go Neural (FSRS/MySQL and SSTorytime-KV integration).

### 2.3 Bottom Band — Editor Area (Three Tabs)

| Tab | Default | Contents |
|-----|---------|----------|
| **Source** | Yes | Full N4L editor with CodeMirror 6: syntax highlighting for all native and extended syntax, arrow autocomplete, and inline validation. |
| **Read** | No | Rich read-only rendered view of the current file. Media (images, audio, PDF thumbnails) displayed inline. Nodes rendered as structured cards with arrows resolved into labeled properties. Reminders highlighted as amber alerts. Sequence nodes connected visually. |
| **Tutor** | No | Knowledge Architect AI-powered Socratic tutoring interface with LLM selector. Renders a placeholder for this feature; full LLM integration is deferred. |

---

## 3. User Scenarios & Testing *(mandatory)*

### User Story 1 — Open a Collection and Return to It on Next Launch (Priority: P1)

On first launch the app shows an empty state with a prompt to open a collection folder. The user picks a directory; Column 1 populates with all chapters. On every subsequent launch the app automatically reopens the same directory — no picker required. A "Change collection" action lets the user switch to a different folder at any time.

**Why this priority**: Without collection persistence users must re-navigate to their folder on every launch, making the app unusable as a daily PKM tool.

**Independent Test**: Open a directory with three `.n4l` files; close the app; relaunch; assert Column 1 is already populated with those files without any folder-picker interaction.

**Acceptance Scenarios**:

1. **Given** the app has never been launched, **When** it starts, **Then** it shows an empty state with a visible "Open collection folder" prompt and no columns are populated.
2. **Given** the user picks a directory via the folder picker, **When** the selection is confirmed, **Then** Column 1 populates and the directory is saved to the app config.
3. **Given** a collection directory was previously saved, **When** the app starts, **Then** it automatically loads that directory and populates Column 1 without showing a picker.
4. **Given** a saved collection directory no longer exists, **When** the app starts, **Then** it shows the empty state with an error message and the "Open collection folder" prompt.
5. **Given** a collection is open, **When** the user triggers "Change collection", **Then** a folder picker appears and selecting a new directory replaces the current collection and updates the saved config.

---

### User Story 2 — Browse a Collection and Select a Chapter (Priority: P1)

The user opens Texere with a directory of `.n4l` files. Column 1 lists all chapters. The user clicks a chapter; Column 2 updates to show its context tags as chips; Column 3 shows matching root nodes.

**Why this priority**: Without structural navigation the workbench is unusable. This is the minimum visible product.

**Independent Test**: Open a directory with three `.n4l` files containing chapters; assert Column 1 lists all chapters. Click one chapter; assert Column 2 shows its context tags and Column 3 shows its nodes.

**Acceptance Scenarios**:

1. **Given** a collection is open, **When** Column 1 is inspected, **Then** all chapters are shown grouped by file, sorted alphabetically by filename.
2. **Given** Column 1 is populated, **When** the user clicks a chapter, **Then** Column 2 updates to show only context tags that appear in that chapter.
3. **Given** context tags are shown in Column 2, **When** the user toggles a tag chip, **Then** Column 3 filters to show only nodes matching all active tags.
4. **Given** a chapter contains `_sequence_` contexts, **When** the user activates a `_sequence_` tag, **Then** Column 3 displays nodes in sequence order with connector lines and Column 2 shows the chain icon on that chip.

---

### User Story 3 — Edit in Source Tab and Save (Priority: P1)

The user works in the Source tab (CodeMirror 6 editor). They type new nodes, add arrows via the arrow palette, and save with Ctrl+S. On save: the file is written atomically and missing `# @id` markers are injected for study-marked nodes.

**Why this priority**: Editing is the primary workflow. The Source tab is the default tab.

**Independent Test**: Open a file in Source tab, add a node with `# @card` marker, press Ctrl+S; assert the file on disk contains the new node with an injected `# @id` line.

**Acceptance Scenarios**:

1. **Given** a node is selected in Column 3, **When** the user opens the Source tab, **Then** the editor scrolls to that node's line.
2. **Given** the user types in the Source tab, **When** the change is made, **Then** Column 1 marks the chapter as modified.
3. **Given** unsaved changes exist, **When** the user presses Ctrl+S, **Then** the file is saved atomically, `# @id` markers are injected for study-marked nodes lacking one, and the modified indicator clears.
4. **Given** the user opens the +link dropdown (arrow palette), **When** the dropdown opens, **Then** arrows are grouped by SST meta-type with recently used arrows at top.
5. **Given** the file is modified externally while open, **When** the change is detected, **Then** the app notifies the user and offers to reload without silently overwriting the external edit.
7. **Given** the user dismissed the external-modification reload prompt, **When** they press Ctrl+S, **Then** the save proceeds and overwrites the external changes without a second confirmation.
6. **Given** unsaved changes exist, **When** the user attempts to switch to a different chapter or close the app, **Then** the app prompts to save.

---

### User Story 4 — Read View (Priority: P2)

The user switches to the Read tab. The current file renders as structured cards. Media is displayed inline. Reminders (ALL CAPS lines) are highlighted amber. Sequence nodes are visually connected. Arrows appear as labeled properties within cards.

**Why this priority**: Read view adds significant value for review and recall but the Source tab is fully functional without it.

**Independent Test**: Open a file with an image reference, an ALL CAPS reminder, and a sequence context; switch to Read tab; assert the image renders inline, the reminder is amber, and sequence nodes are connected visually.

**Acceptance Scenarios**:

1. **Given** a file is open, **When** the user switches to the Read tab, **Then** nodes are rendered as structured cards with their arrows resolved into labeled property sections.
2. **Given** a node card references an image file, **When** the Read tab renders, **Then** the image is displayed inline within the card.
3. **Given** a file contains ALL CAPS reminder lines, **When** the Read tab renders, **Then** those reminders are highlighted amber within their cards.
4. **Given** a file contains `_sequence_` nodes, **When** the Read tab renders, **Then** those nodes are rendered in order with a visual connector between them.

---

### User Story 5 — Reminder Counter Badge (Priority: P3)

The button bar shows an amber badge with the count of open ALL CAPS reminders across all open files. Clicking it opens a reminder panel listing every open reminder with its file and context. Resolving a reminder decrements the count.

**Why this priority**: Reminder tracking is a quality-of-life feature; the editor is functional without it.

**Independent Test**: Open a file with three ALL CAPS reminder lines; assert the badge shows "3"; resolve one reminder; assert badge shows "2".

**Acceptance Scenarios**:

1. **Given** open files contain ALL CAPS reminder lines, **When** the button bar renders, **Then** the amber badge shows the total count.
2. **Given** the user clicks the badge, **When** the panel opens, **Then** each reminder is listed with its file, chapter, and effective context at that line.
3. **Given** a reminder is resolved (text lowercased or a response line added), **When** the file is saved, **Then** the badge count decrements.

---

### Edge Cases

- What happens when Column 1 lists a chapter from a file deleted externally between scan time and click time?
- If the user dismisses the external-modification reload prompt and later saves, the save overwrites the external changes (user's edits win; resolved).
- How does the Source tab handle files with parse errors (non-UTF-8, malformed markers)? → File remains fully editable; errors shown as inline markers only (resolved).
- Does the Source tab remain responsive for a 10,000-line file during rapid cursor movement and typing?
- What is the minimum window size before the band layout degrades gracefully?
- What does the Tutor tab show before any LLM is selected?

---

## 4. Requirements *(mandatory)*

### 4.0 Collection Config

- **FR-000**: On first launch, the app MUST display an empty state with an "Open collection folder" prompt; no columns are populated until a directory is chosen.
- **FR-000b**: When the user selects a directory, the app MUST save it to a per-user config file (`collectionDir` field) using `os.UserConfigDir()` for the platform-appropriate location (e.g., `~/.config/texere/config.json` on Linux, `%AppData%\Texere\config.json` on Windows).
- **FR-000c**: On every subsequent launch, the app MUST auto-load the saved `collectionDir` without prompting; if the directory no longer exists, the app MUST show the empty state with an error notice.
- **FR-000d**: A "Change collection" action in the button bar MUST open a folder picker; confirming a new directory MUST replace the current collection and update the saved config.

### 4.1 Top Band — Structural Navigation

**Column 1 — Chapter / File**

- **FR-001**: Column 1 MUST display all chapters from open `.n4l` files, grouped by file, sorted case-insensitively by filename.
- **FR-002**: Column 1 MUST reflect filesystem changes (file added, removed, renamed) within 200 ms without a manual refresh.
- **FR-003**: Clicking a chapter in Column 1 MUST update Column 2 to show only that chapter's context tags and Column 3 to show that chapter's root nodes.
- **FR-004**: Chapters with unsaved edits in the Source tab MUST be visually marked as modified in Column 1.

**Column 2 — Context Tags**

- **FR-005**: Column 2 MUST show all unique context tags from the selected chapter as toggleable chips.
- **FR-006**: The parser MUST compute the effective context set at each line by replaying `::`, `+::`, `-::` operators; chips MUST reflect the full set of tags that appear at any line in the chapter, not just explicit `::` declarations.
- **FR-007**: The `_sequence_` tag chip MUST display a chain icon and receive distinct visual treatment.
- **FR-008**: Toggling chips MUST filter Column 3 to show only nodes present in all active contexts.

**Column 3 — Structural Triage Dashboard**

- **FR-009**: Column 3 MUST display root nodes matching the active context filter from Column 2.
- **FR-009a**: Column 3 MUST include a persistent quick filter bar with three mutually exclusive buttons: **[All]** (default), **[Extracts]** (nodes with no arrows and no StudyMeta), **[Structured]** (nodes with at least one arrow or a StudyMeta marker). The quick filter stacks on top of the Column 2 context filter.
- **FR-009b**: The UI MUST NEVER render artificial grouping nodes, folder labels, or parent entries that do not exist as actual lines in the underlying `.n4l` source. The visual hierarchy MUST mirror the parsed AST exactly.
- **FR-010**: In `_sequence_` contexts Column 3 MUST display nodes as a vertical timeline: each node is numbered `(n)` in file order and connected by a blue vertical stem line. Sequence nodes that also have arrows MUST render a collapsible chevron.
- **FR-011**: In non-sequence contexts Column 3 MUST display nodes in file order with an optional alphabetical sort toggle. Pure-text nodes (extracts) are prefixed with a `"` icon to distinguish them from structured nodes at a glance.
- **FR-011a**: Structured nodes (those with arrows or dittos) MUST render a `▶`/`▼` chevron. Expanding a node reveals its ditto arrows as child entries: arrow LongName (from SSTRegistry) in teal; unregistered arrow codes in amber.
- **FR-012**: Selecting a node in Column 3 MUST scroll the Note editor to that node's line and switch the bottom band to the Note tab.
- **FR-012a**: Moving the cursor in the Note editor MUST highlight the corresponding node in Column 3 with a subtle background (`.cursor-at`) without overriding the explicitly selected node highlight (`.active`). The highlighted node is the one whose source line is the nearest line ≤ the cursor position.
- **FR-012b**: Hovering over any node in Column 3 MUST reveal a secondary action row with stub buttons **[+ Card]** and **[💬]**. These are UI stubs only; backend integration is deferred to later features.

### 4.2 Middle Band — Button Bar

- **FR-013**: The button bar MUST expose the following actions: +node, +link, +context, save (Ctrl+S), search, undo.
- **FR-013a**: The **+node** button MUST insert a blank node line at the current cursor position in the Source tab; if the Source tab is not active it MUST switch to it first.
- **FR-013b**: The **+context** button MUST insert a `::` context operator line at the current cursor position in the Source tab; if the Source tab is not active it MUST switch to it first.
- **FR-013c**: The **search** button MUST open CodeMirror's built-in in-file search panel for the current file (equivalent to Ctrl+F); cross-collection search is out of scope for this feature.
- **FR-014**: The +link button MUST open a dropdown (arrow palette) with arrows grouped by SST meta-type and recently used arrows at top, powered by the SSTconfig arrow registry. The recently used arrow list MUST be persisted in `config.json` (max 10 entries, LRU order) and survive application restarts.
- **FR-015**: An amber reminder counter badge MUST show the count of open ALL CAPS reminders across all open files; clicking it MUST open a reminder panel listing each reminder with its file, chapter, and effective context.

### 4.3 Bottom Band — Editor Tabs

**Source Tab**

- **FR-016**: The Source tab MUST be the default tab when a node or file is selected.
- **FR-017**: The Source tab MUST use CodeMirror 6 with syntax highlighting for all N4L native and extended syntax elements: chapter headers, context operators, arrow declarations, ditto lines, study markers, provenance/ID markers, ALL CAPS reminder lines, and `@label` anchors.
- **FR-018**: The Source tab MUST provide arrow autocomplete (triggered on arrow code prefix) drawing from the SSTconfig arrow registry.
- **FR-019**: The Source tab MUST show inline validation markers for malformed N4L syntax. Files with parse errors MUST remain fully editable; no functionality (save, autocomplete, injection) is blocked by the presence of errors.
- **FR-020**: Pressing Ctrl+S MUST save the file atomically and trigger `# @id` injection for study-marked nodes lacking an ID (delegating to the write-back API from feature 002).
- **FR-021**: The Source tab MUST prompt the user before switching files or closing with unsaved changes.
- **FR-022**: The Source tab MUST remain responsive (no perceptible input lag) for files up to 10,000 lines.

**Read Tab**

- **FR-023**: The Read tab MUST render the current file as read-only structured cards, one card per root node.
- **FR-024**: Arrow declarations MUST be resolved into labeled property sections within each card.
- **FR-025**: Image, audio, and PDF thumbnail media referenced in nodes MUST be displayed inline.
- **FR-026**: ALL CAPS reminder lines MUST be highlighted amber.
- **FR-027**: `_sequence_` nodes MUST be rendered in sequence order with a visual connector between adjacent sequence members.

**Tutor Tab**

- **FR-028**: The Tutor tab MUST render a placeholder UI with a visible LLM selector control. Full LLM integration is deferred to a later feature.

### 4.4 Key Entities

- **AppConfig**: Persisted user configuration (`collectionDir`); stored in the OS user-config directory.
- **Open Collection**: The directory currently loaded; provides all chapters for Column 1.
- **Selected Chapter**: The chapter currently active in Column 1; scopes Columns 2 and 3.
- **Active Context Filter**: The set of toggled chips in Column 2; determines which nodes appear in Column 3.
- **Selected Node**: The node highlighted in Column 3; drives Source tab scroll position.

---

## 5. Success Criteria *(mandatory)*

- **SC-001**: A chapter with 500 nodes opens and populates Columns 2 and 3 within 500 ms of clicking it in Column 1.
- **SC-002**: N4L structural elements in the Source tab (chapter headers, arrows, context operators, markers) are visually distinguishable without reading the text — a reviewer identifies each element type in under 5 seconds per type.
- **SC-003**: Toggling a context tag chip in Column 2 updates Column 3 within 100 ms.
- **SC-004**: Ctrl+S saves the file and completes `# @id` injection within 500 ms for files up to 10,000 lines.
- **SC-005**: Column 1 reflects a file addition or deletion within 200 ms of the filesystem event.
- **SC-006**: The Source tab handles a 10,000-line file with no perceptible input lag during typing or cursor movement.
- **SC-007**: The Read tab renders a chapter of 200 nodes within 200 ms of tab switch (per constitution performance standard).

---

## Clarifications

### Session 2026-04-11

- Q: Column 4 (Orbit) adds no value before SSTorytime-KV; can Column 3 use that space? → A: Yes — Column 4 placeholder removed; Column 3 fills full width
- Q: Column 3 is a flat list; can it become a structural triage dashboard that mirrors the AST? → A: Yes — full rewrite of ColumnNodes; three rendering modes (extract / structured / sequence)
- Q: When multiple context tags are selected and the user scrolls to context, which node should the editor target? → A: The first node in file order that matches any active tag (OR semantics, consistent with the filter itself)
- Q: Should hover actions (Promote to Card, Discuss) be wired to the backend now? → A: No — UI stubs only; backend deferred to FSRS and AI-integration features
- Q: What does bidirectional sync mean for editor cursor → Column 3? → A: Cursor movement sets a `.cursor-at` highlight on the nearest-preceding node; does not override the explicit `.active` selection from a Column 3 click

### Session 2026-04-09

- Q: What do +node and +context do when clicked? → A: Insert template snippets at cursor in Source tab (+node inserts a blank node line; +context inserts `::`)
- Q: What is the scope of the search button? → A: In-file search only — opens CodeMirror's search panel for the current file
- Q: Should "recently used" arrows in the arrow palette be per-session or persisted? → A: Persisted in `config.json` alongside `collectionDir`
- Q: If user dismisses the external-modification reload prompt then presses Ctrl+S, what happens? → A: Save proceeds and overwrites — dismissing the prompt means the user's edits win
- Q: How does the Source tab handle a file with parse errors? → A: File is fully editable; parse errors appear as inline validation markers only; no functionality is blocked

---

## 6. Assumptions

- The collection is a flat directory of `.n4l` files; subdirectory support is out of scope.
- App config is a single JSON file (`config.json`) stored under `os.UserConfigDir()/texere/`; fields for this feature: `collectionDir` and `recentArrows` (LRU list of up to 10 arrow codes).
- `ScanCollection`, `WatchCollection`, `ParseFile`, and `InjectMissingIDs` from feature 002 are used as-is; no Go backend changes are required for this feature beyond wiring them to the new UI events.
- The SSTconfig arrow registry is available as a static configuration file; dynamic registry editing is out of scope.
- Media file paths are referenced in N4L nodes as bare file-path tokens (e.g., `./images/diagram.png`) on their own line within a node or as the target of an arrow. The Read tab detects media by file extension (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.svg` → image; `.mp3`, `.wav`, `.ogg` → audio; `.pdf` → PDF thumbnail via pdf.js). This is the only supported media embedding mechanism.
- Inline rendering of LaTeX is out of scope for both Source and Read tabs.
- Collaborative or multi-window editing is out of scope; single-user, single-window only.
- Column 4 (Orbit Minimap) placeholder has been removed from `TopBand.svelte`; Column 3 now fills the full top-band width. Column 4 will be reintroduced as a separate feature when SSTorytime-KV graph integration is ready.
- Graph build, git auto-commit, Sync, and History are all deferred to a future graph-integration feature pending SSTorytime-KV integration.
- Study and Go Neural are deferred to later features pending FSRS/MySQL and SSTorytime-KV integration.
