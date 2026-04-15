# Tasks: Editor Core — Hybrid Band Layout Workbench

**Input**: Design documents from `/specs/003-editor-core/`
**Prerequisites**: plan.md ✅ spec.md ✅ research.md ✅ data-model.md ✅ contracts/wails-bridge.md ✅

**Tests**: Not requested — no test tasks included.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US5)
- All paths are relative to repository root

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Install frontend dependencies and create the skeleton files that every user story builds on.

- [x] T001 Install CodeMirror 6 npm packages in `frontend/package.json` (`@codemirror/state`, `@codemirror/view`, `@codemirror/language`, `@codemirror/autocomplete`, `@codemirror/search`, `@codemirror/commands`, `@lezer/highlight`, `pdfjs-dist`)
- [x] T002 [P] Create `frontend/src/components/`, `frontend/src/lib/`, and `frontend/src/stores/` directories (empty `.gitkeep` files)
- [x] T003 [P] Add `AppConfig` and `SaveFileResult` structs to `app.go` (exact field definitions from `data-model.md §2`)
- [x] T004 [P] Rewrite `frontend/src/App.svelte` as three-band layout scaffold: three empty `<div class="band-top|middle|bottom">` stacked vertically with full-height flex layout; no logic yet

**Checkpoint**: `npm install` succeeds; `make build` compiles with the new structs.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: All new Go backend methods and the Svelte store — must be complete before any user story frontend work.

**⚠️ CRITICAL**: No user story implementation can begin until this phase is complete.

- [x] T005 Add `loadConfig() (*AppConfig, error)` and `saveConfig(cfg *AppConfig) error` helpers to `app.go` (use `os.UserConfigDir()/texere/config.json`; create dir if absent; return empty AppConfig on first run, not error)
- [x] T006 Add `GetFileContent(path string) (string, error)` Wails-bound method to `app.go` (`os.ReadFile` wrapper; returns UTF-8 string)
- [x] T007 Add `GetArrowRegistry() ([]content.ArrowDef, error)` Wails-bound method to `app.go` (returns `a.registry.Arrows` as flat slice sorted by MetaType then Code; returns empty slice in degraded mode)
- [x] T008 Add `ResolveReminder(path string, lineNumber int) (*content.WriteBackResult, error)` Wails-bound method to `app.go` (delegates directly to `content.ResolveReminder`)
- [x] T009 Add `savingPath string` and `openFilePath string` fields to `App` struct in `app.go`; add `SaveFile(path, fileContent string) (*SaveFileResult, error)` Wails-bound method: set `a.savingPath=path`, call `writeLines`+`content.ParseFile`+`content.InjectMissingIDs`, read final on-disk content, clear `a.savingPath`, return `SaveFileResult{Injections, FinalContent}`; also set `a.openFilePath = path` at the **top** of the existing `ParseFileForUI` method (tracking the currently open file for T010 without a separate Wails binding)
- [x] T010 Complete `consumeWatchEvents` in `app.go`: emit `runtime.EventsEmit(a.ctx, "collection:change", ev)` for `EventAdded/Removed/Renamed`; emit `runtime.EventsEmit(a.ctx, "file:modified-externally", map[string]string{"path": ev.Path})` for `EventModified` only when `ev.Path == a.openFilePath && ev.Path != a.savingPath`
- [x] T050 Add `UpdateRecentArrows(codes []string) error` Wails-bound method to `app.go`: load config, replace `RecentArrows` field (add `RecentArrows []string \`json:"recentArrows"\`` to `AppConfig` if not already present from T003), cap at 10 entries, save config *(elevated from Phase 8 — required by T036)*
- [x] T011 [P] Create `frontend/src/stores/workbench.js`: export `workbenchStore` writable with all fields from `data-model.md §3`; export `contextTags` derived store (union of `EffectiveCtx` across selected chapter nodes, each with `{name, isSequence, isActive}`); export `filteredNodes` derived store (nodes where `EffectiveCtx ⊇ activeContextFilter`, sequence-sorted when `_sequence_` active); export `reminderCount` derived store (`parsedFile?.Reminders.length ?? 0`)
- [x] T012 [P] Add `onMount`/`onDestroy` lifecycle in `frontend/src/App.svelte`: call `GetArrowRegistry()` and set `workbenchStore.arrowDefs`; register `EventsOn('collection:change', ...)` to refresh `workbenchStore.summaries`; register `EventsOn('file:modified-externally', ...)` to set `workbenchStore.externalModification`; cancel both listeners in `onDestroy`

**Checkpoint**: `make build` compiles with all new methods; Wails regenerates `frontend/wailsjs/` bindings successfully.

---

## Phase 3: User Story 1 — Collection Config (Priority: P1) 🎯 MVP

**Goal**: App remembers the last-opened collection directory and auto-loads it on restart. First-launch shows an empty state. "Change collection" lets the user switch folders.

**Independent Test**: Open a directory; close the app; relaunch; assert Column 1 is populated without any folder-picker interaction.

- [x] T013 [US1] Wire config auto-load in `app.go` `startup()`: call `loadConfig()`; if `cfg.CollectionDir` is non-empty and the directory exists, call `content.ScanCollection` and `content.WatchCollection`, store results and start `consumeWatchEvents` goroutine; store the loaded summaries so the frontend can retrieve them via a new `GetInitialSummaries() []content.ChapterSummary` method added here
- [x] T014 [US1] Extend `OpenCollection` in `app.go` to call `saveConfig(&AppConfig{CollectionDir: dir})` after a successful scan; set `a.openFilePath = ""` (no file open yet)
- [x] T015 [P] [US1] Create `frontend/src/components/EmptyState.svelte`: centered layout with "Open collection folder" primary button; accepts optional `errorMessage string` prop displayed in amber above the button
- [x] T016 [US1] Wire collection-open flow in `frontend/src/App.svelte`: on mount call `GetInitialSummaries()` and set `workbenchStore.summaries/collectionDir`; render `EmptyState` when `collectionDir` is null, workbench bands otherwise; bind "Open collection folder" button to `runtime.OpenDirectoryDialog` + `OpenCollection` + store update
- [x] T017 [US1] Handle missing-directory startup case in `frontend/src/App.svelte`: detect when app starts with null collectionDir after a non-empty config was attempted (expose a `GetStartupError() string` method in `app.go` that returns a message when auto-load failed); pass error to `EmptyState.errorMessage`
- [x] T018 [P] [US1] Create `frontend/src/components/MiddleBand.svelte` stub with a single "Change collection" button: calls `runtime.OpenDirectoryDialog`, then `OpenCollection`, updates `workbenchStore.collectionDir` and `summaries`; placeholder `<div>` slots for remaining buttons (added in T035)

**Checkpoint**: US1 fully functional and independently testable.

---

## Phase 4: User Story 2 — Browse a Collection and Select a Chapter (Priority: P1)

**Goal**: Column 1 lists chapters; clicking a chapter populates Context Tag chips (Column 2) and Nodes (Column 3) and keeps them live with filesystem changes.

**Independent Test**: Open 3-file collection; assert Column 1 lists all chapters; click a chapter; assert Column 2 shows its tags and Column 3 shows its nodes; add a file to the directory; assert Column 1 updates within 200 ms.

- [x] T019 [P] [US2] Create `frontend/src/components/ColumnChapters.svelte`: renders `workbenchStore.summaries` as a grouped list (filename heading + indented chapter titles); applies modified indicator class when chapter matches `selectedChapterKey` and `isDirty`; emits `chapter-selected` custom event with `{filePath, chapterTitle}` on click
- [x] T020 [US2] Wire chapter selection in `frontend/src/App.svelte`: on `chapter-selected` event call `ParseFileForUI(filePath)`, update `workbenchStore.parsedFile`, `selectedChapterKey`, clear `activeContextFilter` and `selectedNodeLine`, set `isDirty=false`; `a.openFilePath` is updated automatically as a side-effect of `ParseFileForUI` (see T009 — no separate Wails call needed)
- [x] T021 [P] [US2] Create `frontend/src/components/ColumnContextTags.svelte`: renders `contextTags` derived store as toggleable chip buttons; `_sequence_` chip displays a chain icon (⛓ or CSS icon); clicking a chip toggles it in `workbenchStore.activeContextFilter`
- [x] T022 [US2] Create `frontend/src/components/ColumnNodes.svelte`: renders `filteredNodes` derived store as a scrollable list; sequence nodes display with connector lines between them (CSS border-left on adjacent `.sequence-node` items); alphabetical sort toggle button sets a local `sortAlpha` boolean; emits `node-selected` custom event with `node.Line` on click *(superseded by T063 — full triage dashboard rewrite)*
- [x] T023 [US2] Wire `node-selected` event in `frontend/src/App.svelte`: set `workbenchStore.selectedNodeLine = node.Line`
- [x] T024 [US2] Create `frontend/src/components/TopBand.svelte`: horizontal flex container composing `ColumnChapters`, `ColumnContextTags`, `ColumnNodes` side by side; Column 4 placeholder removed in T060
- [x] T025 [US2] Compose TopBand into `frontend/src/App.svelte` top band slot
- [x] T026 [P] [US2] Handle `collection:change` in `workbenchStore` update logic (already subscribed in T012): for `EventAdded` append new `ChapterSummary` to `summaries`; for `EventRemoved` remove matching entry; for `EventRenamed` update the entry's `FilePath`/`FileName` in place

**Checkpoint**: US2 fully functional and independently testable.

---

## Phase 5: User Story 3 — Edit in Source Tab and Save (Priority: P1)

**Goal**: CodeMirror 6 editor with N4L syntax highlighting, arrow autocomplete, Ctrl+S atomic save with `# @id` injection, external modification detection, and button bar actions.

**Independent Test**: Open a file; add a node with `# @card`; press Ctrl+S; assert file on disk contains the node with a new `# @id` line.

- [x] T027 [P] [US3] Create `frontend/src/lib/n4l-language.js`: define `n4lLanguage` using `StreamLanguage.define()` with a `StreamParser<{}>` — tokenize per the token table in `research.md §2`: chapter headers (`tags.heading`), context operators (`tags.keyword`), arrow code+target (`tags.operatorKeyword`/`tags.string`), ditto lines (`tags.meta`), study markers (`tags.annotation`), provenance/ID markers (`tags.comment`), ALL CAPS reminders (`tags.invalid`), `@label` anchors (`tags.labelName`), `#` comments (`tags.lineComment`)
- [x] T028 [P] [US3] Create `frontend/src/lib/n4l-highlight.js`: define `n4lHighlightStyle` using `HighlightStyle.define()` mapping each tag from T027 to a CSS colour; ALL CAPS reminder tag maps to amber (`#f59e0b`); provenance/ID marker maps to muted grey
- [x] T029 [US3] Create `frontend/src/lib/arrow-autocomplete.js`: export `arrowCompletion(arrowDefs)` function returning a CM6 `CompletionSource`; activates on regex `\(\w*$`; returns `CompletionResult` with completions `{label: def.Code, detail: def.LongName, section: def.MetaType}` sorted by MetaType then Code; recently-used completions appear first (read from `workbenchStore.arrowDefs` LRU order)
- [x] T030 [US3] Create `frontend/src/components/SourceTab.svelte`: `bind:this` container div; mount `EditorView` in `onMount` with extensions `[n4lLanguage, n4lHighlightStyle, autocompletion({override: [arrowCompletion(...)]}), search(), keymap.of(defaultKeymap), ...]`; expose `setContent(text)`, `scrollToLine(n)`, and `getContent()` methods; dispatch `content-changed` event on doc updates; destroy view in `onDestroy`
- [x] T031 [US3] Create `frontend/src/components/BottomBand.svelte`: tab bar with Source/Read/Tutor tabs; `activeTab` bound to `workbenchStore.activeTab`; renders `<SourceTab>`, `<ReadTab>` (stub div for now), `<TutorTab>` (stub div) conditionally; Source is default
- [x] T032 [US3] Wire chapter → Source tab content in `frontend/src/App.svelte`: when `selectedChapterKey` changes call `GetFileContent(filePath)` and call `SourceTab.setContent(text)`; when `selectedNodeLine` changes call `SourceTab.scrollToLine(n)`
- [x] T033 [US3] Implement Ctrl+S handler in `frontend/src/components/SourceTab.svelte`: on `keydown` Ctrl+S call `SaveFile(openFilePath, editorContent)`; on success set `workbenchStore.isDirty=false`, call `setContent(result.FinalContent)`, call `ParseFileForUI` to refresh `parsedFile`
- [x] T034 [US3] Set `workbenchStore.isDirty=true` in the `content-changed` event handler wired in `frontend/src/App.svelte`
- [x] T035 [US3] Expand `frontend/src/components/MiddleBand.svelte` with remaining button bar actions: **+node** (dispatch CM6 transaction inserting `\n` + blank node template at cursor, switch to Source tab first), **+context** (insert `::` line at cursor), **search** (dispatch `openSearchPanel` CM6 command), **undo** (dispatch `undo` CM6 command), **save** (trigger Ctrl+S handler on SourceTab ref)
- [x] T036 [P] [US3] Add arrow palette dropdown to `frontend/src/components/MiddleBand.svelte`: **+link** button opens popover listing `workbenchStore.arrowDefs` grouped by `MetaType`; recently-used section at top (from `workbenchStore` LRU); selecting an arrow inserts `(code) ` at cursor and pushes code to front of `recentArrows` list; call `UpdateRecentArrows` Go method (defined in T050, Phase 2) to persist
- [x] T037 [US3] Subscribe to `workbenchStore.externalModification` in `frontend/src/components/SourceTab.svelte`: when non-null show an inline banner "File changed externally — Reload?"; Reload button calls `GetFileContent` and `setContent`; Dismiss button clears `externalModification`; subsequent Ctrl+S proceeds without re-prompting (no second guard needed per clarification)
- [x] T038 [US3] Add unsaved-changes guard in `frontend/src/App.svelte`: before switching `selectedChapterKey` check `workbenchStore.isDirty`; if dirty show a `confirm()` dialog "Unsaved changes — save before switching?"; wire Wails `runtime.WindowBeforeClose` event to the same guard for app-close

**Checkpoint**: US3 fully functional. Source tab edits, saves, and handles external modifications independently.

---

## Phase 6: User Story 4 — Read View (Priority: P2)

**Goal**: Rich read-only card view of the current file with inline media, amber reminders, and sequence connectors.

**Independent Test**: Open a file with an image reference, an ALL CAPS reminder, and a `_sequence_` context; switch to Read tab; assert the image renders inline, the reminder is amber, and sequence nodes are visually connected.

- [x] T039 [P] [US4] Create `frontend/src/components/NodeCard.svelte`: accepts a `Node` prop and `arrowDefs` array; renders `Node.Text` as the card heading; renders each `Arrow` as a `<dl>` entry with `ArrowDef.LongName` as `<dt>` and `Arrow.Target` as `<dd>` (fallback to `Arrow.Code` if not in registry); applies `.reminder` amber class when text is ALL CAPS; applies `.sequence-node` class when `Node.IsSequence`
- [x] T040 [P] [US4] Implement inline media in `frontend/src/components/NodeCard.svelte`: scan `Node.Text` for file-path tokens ending in `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.svg` (render `<img>`), `.mp3`, `.wav`, `.ogg` (render `<audio controls>`); resolve paths relative to `workbenchStore.collectionDir`
- [x] T041 [US4] Create `frontend/src/components/ReadTab.svelte`: maps selected chapter's `Nodes` array to `<NodeCard>` components; wraps sequence runs in a `.sequence-group` div; adds CSS `::before`/`::after` connector lines between adjacent `.sequence-node` cards (vertical line motif)
- [x] T042 [US4] Replace the stub `<ReadTab>` div in `frontend/src/components/BottomBand.svelte` with the real `ReadTab` component
- [x] T043 [US4] Add `.reminder` amber highlight CSS and `.sequence-node` connector CSS to `frontend/src/components/ReadTab.svelte` (amber: `background: #fef3c7; border-left: 3px solid #f59e0b`; connector: `border-left: 2px solid #4b5563` on `.sequence-node + .sequence-node::before`)

**Checkpoint**: US4 functional. Read tab renders any parsed file as cards with media, reminders, and sequence connectors.

---

## Phase 7: User Story 5 — Reminder Counter Badge (Priority: P3)

**Goal**: Amber badge in the button bar shows open reminder count; clicking opens a panel listing each reminder with file + chapter context; resolving a reminder decrements the count.

**Independent Test**: Open a file with 3 ALL CAPS reminder lines; assert badge shows "3"; click Resolve on one; assert badge shows "2".

- [x] T044 [P] [US5] Create `frontend/src/components/ReminderPanel.svelte`: slide-out panel (fixed right-side overlay); lists each `Reminder` from `workbenchStore.parsedFile.Reminders` with: `Reminder.Text`, derived chapter title (find chapter where `chapter.LineStart ≤ reminder.Line`), derived `EffectiveCtx` (find node where `node.Line ≤ reminder.Line`, use its `EffectiveCtx`), and a "Resolve" button
- [x] T045 [US5] Add amber reminder badge to `frontend/src/components/MiddleBand.svelte`: shows `reminderCount` derived store value; hidden when count is 0; clicking toggles a `reminderPanelOpen` boolean that shows/hides `ReminderPanel`
- [x] T046 [US5] Wire "Resolve" in `frontend/src/components/ReminderPanel.svelte`: call `ResolveReminder(reminder.File, reminder.Line)`; on success call `ParseFileForUI` to refresh `parsedFile` (badge auto-decrements via `reminderCount` derived store)
- [x] T047 [US5] Handle the case where `Reminder.File` differs from the currently open file in `ReminderPanel.svelte`: show a "Switch to file" affordance before resolving if the reminder is in a different file than `workbenchStore.selectedChapterKey.filePath`

**Checkpoint**: US5 functional. Badge count reflects open reminders; resolving a reminder updates the count live.

---

## Phase 9: Column 3 — Structural Triage Dashboard

**Purpose**: Replace the flat node list with an interactive triage dashboard that mirrors the N4L AST, remove the Column 4 placeholder, and add bidirectional editor↔Column 3 cursor sync.

**Spec references**: FR-009a, FR-009b, FR-010, FR-011, FR-011a, FR-012a, FR-012b (added 2026-04-11)

- [x] T060 Remove `<div class="col-orbit-placeholder" />` and its CSS from `frontend/src/components/TopBand.svelte`; Column 3 now fills the full top-band width
- [x] T061 Add `editorCursorLine: null` field to `workbenchStore` initial state in `frontend/src/stores/workbench.js`
- [x] T062 Extend `EditorView.updateListener.of()` in `frontend/src/components/SourceTab.svelte`: on `update.selectionSet` write the 1-indexed cursor line to `workbenchStore.editorCursorLine`; dispatch `content-changed` on `update.docChanged` as before
- [x] T063 Rewrite `frontend/src/components/ColumnNodes.svelte` as triage dashboard:
  - Quick filter bar: [All] / [Extracts] / [Structured] buttons; `quickFilter` local state stacks on `filteredNodes` derived store
  - `hasArrows(node)` / `isExtract(node)` / `isStructured(node)` / `allArrows(node)` helpers
  - `expanded: Set<number>` with `toggleExpand(line)` for collapsible structured nodes
  - `seqNumMap`: `{line → 1-based index}` for sequence nodes in display order
  - `cursorMatchedLine`: derived from `editorCursorLine` — searches all chapter nodes for nearest-preceding line ≤ cursor
  - Three rendering paths: A (extract, `"` icon), B (structured, `▶`/`▼` chevron + ditto arrow children with LongName / amber unregistered), C (sequence timeline, numbered + blue vertical stem)
  - `.active` class for clicked selection; `.cursor-at` class for editor-cursor proximity (distinct, subtle background)
  - Hover action stubs: `[ + Card ]` `[ 💬 ]` buttons revealed on `.node-row:hover` via `visibility: hidden → visible`

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Tutor tab placeholder, arrow palette persistence, Go tests, performance validation.

- [x] T048 [P] Create `frontend/src/components/TutorTab.svelte`: centered placeholder text "Knowledge Architect — coming soon"; disabled `<select>` LLM selector with placeholder option "Select LLM model"
- [x] T049 [P] Replace the stub `<TutorTab>` div in `frontend/src/components/BottomBand.svelte` with the real `TutorTab` component
- [x] T051 [P] Write Go unit tests in `app_test.go` for: `loadConfig`/`saveConfig` round-trip; `GetArrowRegistry` returns sorted slice in degraded mode; `SaveFile` with a known file produces expected `FinalContent` and injects `# @id` for a `# @card` node
- [ ] T052 Validate SC-006: generate a 10 000-line `.n4l` file using `internal/content/testdata/gen_large/main.go`; open in Source tab; confirm no perceptible input lag during typing and cursor movement
- [ ] T053 Validate SC-001: open a chapter with 500 nodes; assert Column 2 and Column 3 populate within 500 ms (measure via browser DevTools or manual stopwatch)
- [ ] T054 [P] Validate SC-003: toggle a context chip; assert Column 3 updates within 100 ms (measure via DevTools performance panel)
- [ ] T055 [P] Validate SC-004: in a 10 000-line file add a `# @card` node; press Ctrl+S; assert save + injection completes within 500 ms (measure round-trip time in Wails bridge)
- [ ] T056 [P] Validate SC-005: add a new `.n4l` file to the collection directory; assert Column 1 reflects the addition within 200 ms (observe via manual timing or fsnotify debounce check)
- [ ] T057 [P] Validate SC-007: switch to Read tab on a chapter with 200 nodes; assert render completes within 200 ms (measure via browser DevTools Performance tab)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 — **blocks all user story work**
- **US1 (Phase 3)**: Depends on Phase 2
- **US2 (Phase 4)**: Depends on Phase 2; integrates with US1 (summaries already in store)
- **US3 (Phase 5)**: Depends on Phase 2 + US2 (needs parsedFile and selectedChapterKey); requires T031 (BottomBand) before T042/T049
- **US4 (Phase 6)**: Depends on Phase 2 + US2 (needs parsedFile); T031 stub must exist from US3
- **US5 (Phase 7)**: Depends on Phase 2 + US2 (needs parsedFile); MiddleBand must exist from US1/US3
- **Polish (Phase 8)**: Depends on all user stories

### User Story Dependencies

- **US1**: Independent after Phase 2
- **US2**: Independent after Phase 2 (uses summaries populated by US1 flow, but testable with manual OpenCollection call)
- **US3**: Needs US2 complete (requires parsedFile + selectedChapterKey in store)
- **US4**: Needs US2 complete (requires parsedFile); independent of US3
- **US5**: Needs US2 complete (requires parsedFile); independent of US3 and US4

### Within Each Phase

- All [P]-marked tasks within a phase can be executed concurrently
- Go method tasks (T005–T010) must be sequential (all modify `app.go`)
- T027 and T028 can be parallelised (different files); T029 depends on T027+T028 being available conceptually
- T030 (SourceTab) depends on T027+T028+T029
- T031 (BottomBand stub) can be created before T030 is complete

---

## Parallel Opportunities by Phase

```
Phase 1 parallel group:  T002, T003, T004  (after T001 npm install)
Phase 2 parallel group:  T011, T012        (after T005–T010 Go methods)
Phase 3 parallel group:  T015, T018        (US1 frontend components)
Phase 4 parallel group:  T019, T021, T026  (US2 column components)
Phase 5 parallel group:  T027, T028        (N4L language + highlight)
                         T036, T037        (arrow palette, ext. modification)
Phase 6 parallel group:  T039, T040        (NodeCard + media)
Phase 7 parallel group:  T044              (ReminderPanel)
Phase 8 parallel group:  T048, T049, T051, T053, T054, T055, T056, T057
```

---

## Implementation Strategy

### MVP Scope (User Stories 1 + 2 + 3 only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (**critical — blocks everything**)
3. Complete Phase 3: US1 — collection opens and persists
4. Complete Phase 4: US2 — structural navigation works
5. Complete Phase 5: US3 — Source tab edits and saves
6. **STOP and VALIDATE**: three P1 stories functional end-to-end

### Incremental Delivery

1. Phase 1 + 2 → foundation ready
2. + US1 → app remembers collection (demo-able)
3. + US2 → structural navigation (demo-able)
4. + US3 → full editing workflow (MVP complete)
5. + US4 → Read view (P2 value-add)
6. + US5 → Reminder tracking (P3 quality-of-life)
7. + Polish → Tutor tab, arrow persistence, tests

---

## Notes

- Wails auto-generates `frontend/wailsjs/go/main/App.d.ts` and `frontend/wailsjs/go/main/App.js` from Go method signatures — run `wails generate` or `make dev` after adding new methods to pick up bindings before writing frontend code that calls them
- The `internal/content` package is **read-only** for this feature — do not modify any file under `internal/content/`
- `[P]` tasks within the same phase have no file conflicts and can be executed concurrently by separate agents
- Commit after each phase checkpoint to enable clean rollback if a user story needs rework
