# Tasks: N4L File Parser — Layer I Content I/O

**Input**: Design documents from `/docs/specs/002-n4l-parser/`  
**Branch**: `002-n4l-parser`  
**Package**: `texere/internal/content`

**Tests**: Included — mandatory per constitution ("N4L parser changes MUST include round-trip
fidelity tests" and "performance-sensitive paths MUST include benchmark tests that can be run
in CI").

**Note on plan.md**: The Key Architectural Decisions section in plan.md still references the
old hash-based / label-based node ID scheme. All tasks below follow the current spec: sole
`# @id` system-managed marker, no label or hash involvement.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Parallelisable — operates on a different file from concurrent tasks in the same phase
- **[Story]**: Maps task to user story for traceability

---

## Phase 1: Setup

**Purpose**: Add new dependency, create file skeletons, prepare testdata directory.

- [x] T001 Add `github.com/fsnotify/fsnotify v1.7` to `go.mod` and `go.sum` via `go get github.com/fsnotify/fsnotify@v1.7.0` from the repo root
- [x] T002 [P] Create `internal/content/testdata/` directory and write `testdata/simple.n4l`: one chapter (`- basic notes`), three nodes with inline arrows using `(means)` and `(e.g.)`, one ditto line, and one `@label` anchor
- [x] T003 [P] Update `internal/content/doc.go` with package-level documentation: purpose (Layer I — sole content read/write path), three exported capabilities (ScanCollection, ParseFile, WatchCollection), write-back path summary, and layer boundary rules (no imports of `internal/graph` or `internal/learning`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core types, error types, and the SSTconfig registry — shared by every user story.
No user story work can begin until this phase is complete.

**⚠️ CRITICAL**: Complete T004–T008 before any Phase 3+ task.

- [x] T004 Implement all exported struct types in `internal/content/types.go`: `ParsedFile`, `Chapter`, `Node`, `Arrow`, `DittoLine`, `StudyMeta` (with `CardType` and `BlockType` enums), `Provenance`, `Reminder`, `ChapterSummary`, `SSTRegistry`, `ArrowDef`, `AnnotDef`, `CollectionWatcher`, `CollectionEvent` (with `EventKind` enum), `IDInjection` (including `AlreadyHad bool`), `WriteBackResult`. Follow field definitions in `docs/specs/002-n4l-parser/data-model.md` exactly.
- [x] T005 [P] Implement `ParseError` type and error-category constants in `internal/content/errors.go`: struct with `File string`, `Line int`, `Msg string`; exported string constants for each category (`ErrUnknownArrow`, `ErrOrphanDitto`, `ErrUnresolvedRef`, `ErrInvalidPriority`, `ErrMalformedMarker`, `ErrEncoding`)
- [x] T006 [P] Implement `NodeID(filepath, chapterTitle, shortID string) string` in `internal/content/nodeid.go`: returns `filepath+":"+chapterTitle+":id:"+shortID` when `shortID != ""`; returns `""` otherwise. `@label` values must NOT be accepted as parameters — add a doc comment stating this explicitly. Include a `GenerateShortID() string` helper that produces an 8-character base-36 string using `crypto/rand`.
- [x] T007 Implement `LoadRegistry(sstconfigDir string) (*SSTRegistry, error)` in `internal/content/registry.go`: walk `sstconfigDir` for files with the `.sst` extension (case-insensitive); for each file strip `//` line comments (lines where `//` appears — trim from that point) and `#` inline comments (trim from `#` onward), then process the remaining content: (a) **`arrows-*.sst`** — the first non-blank line is `- meta-type-name` (the `MetaType` for all pairs in the file); `:: label ::` lines are section headers (ignored for registry building); `+ forward name (code)` lines optionally followed by `- reverse name (revcode)` on the next line define one `ArrowDef` per pair; lines starting with `# RESERVED` flag the arrow as a reserved N4L relation (note: `(img)` and `(url)` are reserved in the default SSTconfig); (b) **`annotations.sst`** — lines of the form `symbol (code)` where symbol is one of `***`, `**`, `=`, `>>`, `%>`, `%<` define an `AnnotDef`; (c) **`closures.sst`** — lines of the form `(a) + (b) => (c)` define transitivity rules; load into `SSTRegistry.LoadErrors` for now (transitivity enforcement is the graph layer's responsibility); if `sstconfigDir` does not exist return a non-nil `*SSTRegistry` with empty maps and a non-nil error (degraded mode); accumulate per-file parse errors into `SSTRegistry.LoadErrors` without aborting
- [x] T008 [P] Generate `internal/content/testdata/large.n4l`: a valid N4L file with exactly 10,000 lines — one chapter, 2,500 nodes each with one ditto arrow line, interspersed with `::` context operators every 100 nodes, and one `_sequence_` context block of 20 nodes. Use a Go program or shell script to generate it; commit the generated file.

**Checkpoint**: `go build ./internal/content/...` passes. Foundation ready.

---

## Phase 3: User Story 1 — Load a Collection and Browse Its Files (Priority: P1) 🎯 MVP

**Goal**: `ScanCollection` enumerates all `.n4l` files and their chapter titles; `WatchCollection`
keeps the list current as the directory changes.

**Independent Test**: Call `ScanCollection` on a directory of 5 mixed `.n4l` files; assert
returned slice is sorted case-insensitively by filename, each entry has correct `Chapters []string`
from the file, and a file with a read error has `ScanError != nil` but does not abort the slice.

### Implementation

- [x] T009 [P] [US1] Create additional testdata fixtures in `internal/content/testdata/`: `multi-chapter.n4l` (one file with three `- title` lines and nodes in each chapter) and `errors.n4l` (a file with an orphan ditto on line 3, an unknown arrow code on line 7, and a `$label.N` reference to a non-existent label on line 12)
- [x] T010 [US1] Implement `ScanCollection(dir string) ([]ChapterSummary, error)` in `internal/content/scanner.go`: use `os.ReadDir` for the file listing; iterate only `.n4l` files (case-insensitive suffix check); for each file open with `bufio.Scanner` and collect all lines starting with `"- "` as chapter titles; sort the returned slice case-insensitively by `FileName`; if a file cannot be opened set `ScanError` and include the entry; return a directory-level error only if `os.ReadDir` fails
- [x] T011 [US1] Write tests for `ScanCollection` in `internal/content/parser_test.go`: `TestScanCollectionEmpty` (empty dir → empty slice, nil error), `TestScanCollectionSorted` (5 files in non-alphabetical order → slice sorted by filename), `TestScanCollectionMultiChapter` (multi-chapter.n4l → Chapters has 3 titles in file order), `TestScanCollectionUnreadable` (chmod 000 file → entry with ScanError, others unaffected); `BenchmarkScanCollection` using 50 copies of `simple.n4l` in a temp dir (target: linear scaling)
- [x] T012 [US1] Implement `WatchCollection(dir string) (*CollectionWatcher, error)` in `internal/content/watcher.go`: wrap `fsnotify.NewWatcher`; call `watcher.Add(dir)`; start a goroutine that translates `fsnotify.Event` values into `CollectionEvent` values — `Create` → `Added` (call `ScanCollection` on the new file path, attach `ChapterSummary` to event), `Remove` → `Removed`, `Rename` → `Renamed` (re-scan new path), `Write` → `Modified`; filter non-`.n4l` events; expose `Events <-chan CollectionEvent` and `Errors <-chan error`; implement `Close() error`
- [x] T013 [US1] Write tests for `WatchCollection` in `internal/content/parser_test.go`: `TestWatchCollectionAdded` (write a new `.n4l` file → receive `Added` event with correct `ChapterSummary`), `TestWatchCollectionRemoved` (delete a file → receive `Removed` event with correct `Path`), `TestWatchCollectionIgnoresNonN4L` (write a `.txt` file → no event emitted); use `t.TempDir()` and a 500 ms event timeout

**Checkpoint**: `go test ./internal/content/... -run TestScan -run TestWatch` passes.

---

## Phase 4: User Story 2 — Select a File and See Its Content (Priority: P1)

**Goal**: `ParseFile` converts any valid `.n4l` file into a complete, correct `ParsedFile` struct
in under 50 ms for 10,000-line files.

**Independent Test**: Call `ParseFile` on `testdata/contexts.n4l`; assert every node's
`EffectiveCtx` matches the expected context set computed by hand for that position; total
parse time (benchmarked) is under 50 ms.

### Implementation

- [x] T014 [P] [US2] Create remaining testdata fixtures: `internal/content/testdata/contexts.n4l` (file with `::`, `+::`, `-::` operators creating 6 distinct effective-context states across 20 nodes), `internal/content/testdata/sequence.n4l` (chapter with `_sequence_` context holding 8 nodes, one non-sequence node interspersed), `internal/content/testdata/labels.n4l` (3 `@label` anchors, 4 `$label.N` references including one `$PREV.3`, one forward reference resolved in second pass)
- [x] T015 [US2] Implement the `ParseFile` skeleton in `internal/content/parser.go`: open file with `bufio.Scanner`; line-by-line loop maintaining `parserState` struct (fields: `currentChapter *Chapter`, `effectiveCtx map[string]struct{}`, `dittoParent *Node`, `pendingIDMarker string`, `labelMap map[string]*Node`, `pendingRefs []pendingRef`); detect `"- "` chapter lines → close current chapter, open new one; detect bare node lines → append `Node` to current chapter; collect `ParseError` for each malformed line without aborting; return `(*ParsedFile, []ParseError)` with all chapters and errors populated
- [x] T016 [US2] Implement context state machine in `internal/content/parser.go`: detect `:: tag1, tag2` (replace — clear map, add tags), `+:: tag3 ::` (union — add tags to existing), `-:: tag3 :` (subtract — remove tag); after each operator update, snapshot `effectiveCtx` as sorted `[]string` onto each subsequent node's `EffectiveCtx` field; handle empty `::` (clear to empty set); set `Node.IsSequence = true` when `_sequence_` is in the effective context
- [x] T017 [US2] Implement ditto line resolution in `internal/content/parser.go`: detect lines starting with `"  ` (double-quote + spaces); attribute to `dittoParent`; if `dittoParent == nil` emit `ParseError{Msg: ErrOrphanDitto}`; parse arrows on the ditto line and append to `dittoParent.Dittos` as `DittoLine{Line, Arrows}`
- [x] T018 [US2] Implement label definition and two-pass reference resolution in `internal/content/parser.go`: detect `@labelname` prefix on node lines → store in `parserState.labelMap`; detect `$labelname.N` and `$PREV.N` tokens on any line → record as `pendingRef{refStr, targetNode, fieldIdx}`; after the chapter is complete, iterate `pendingRefs` and resolve each to the correct node or ditto; emit `ParseError{Msg: ErrUnresolvedRef}` for any reference that cannot be resolved
- [x] T019 [US2] Implement arrow parsing and annotation symbol recognition in `internal/content/parser.go`: detect `(code) target` patterns on node and ditto lines using a simple string scanner (no regex — find first `(`, scan to `)`, extract code and remainder as target); look up code in `SSTRegistry.Arrows`; set `Arrow.IsRegistered = false` and emit `ParseError{Msg: ErrUnknownArrow}` for unknown codes; detect annotation symbols (`>>`, `%`, `=`, `**`) from `SSTRegistry.Annotations` as inline markers within node text; set `Arrow.IsDitto` correctly
- [x] T020 [US2] Implement `# @id` marker reading in `internal/content/parser.go`: detect lines matching `# @id <shortid>`; store `shortid` in `parserState.pendingIDMarker`; on the next node line, set `Node.IDMarkerLine` to the `# @id` line number and compute `Node.ID` via `NodeID(filepath, chapterTitle, shortid)`; clear `pendingIDMarker` after assignment
- [x] T021 [US2] Write round-trip fidelity and benchmark tests in `internal/content/parser_test.go`: `TestParseFidelitySimple` (parse `simple.n4l` → verify chapter title, node count, arrow targets, ditto attribution), `TestParseContextReplay` (parse `contexts.n4l` → assert each node's `EffectiveCtx` matches expected set), `TestParseSequence` (parse `sequence.n4l` → assert `IsSequence` on correct nodes), `TestParseLabelResolution` (parse `labels.n4l` → assert all references resolved, forward ref resolved), `TestParseErrorCollection` (parse `errors.n4l` → assert `len(errors) == 3` and file still returns all valid nodes), `TestParseDeterminism` (call `ParseFile` twice on same file → assert identical output), `BenchmarkParseFile10k` on `large.n4l` with `b.ReportAllocs()`; add `TestMain` that fails the build if `BenchmarkParseFile10k` median exceeds 50 ms when run with `-bench -benchtime=3s`

**Checkpoint**: `go test ./internal/content/... -run TestParse` passes; `go test -bench=BenchmarkParseFile10k ./internal/content/...` completes under 50 ms.

---

## Phase 5: User Story 3 — Study Markers and Node ID Injection (Priority: P2)

**Goal**: Study markers on all variants are extracted on parse; `# @id` markers are injected
atomically for items (save-triggered) and extracts (scheduler-triggered); promotion from extract
to item reuses the existing ID without a second injection.

**Independent Test**: Parse `testdata/study-markers.n4l`; assert each node has the correct
`StudyMeta.CardType`; call `InjectMissingIDs`; assert `# @id` lines are present above each
study-marked node and no other lines changed; call `InjectMissingIDs` again → all
`IDInjection.AlreadyHad == true`.

### Implementation

- [x] T022 [P] [US3] Create `internal/content/testdata/study-markers.n4l`: one chapter with all study marker variants — individual `# @card`, `# @card reverse`, `# @card no-reverse`, `# @cloze` with `{braces}`, `# @study` block (3 nodes), `# @study reverse` block (2 nodes), `# @study sequence` block inside a `_sequence_` context (4 nodes), `# @end`, an individual marker overriding the block default on one block node; two nodes without any marker (must not appear in study output)
- [x] T023 [US3] Implement study marker extraction in `internal/content/parser.go`: detect `# @card`, `# @card reverse`, `# @card no-reverse` → set `parserState.pendingStudy` with correct `CardType` and flags; detect `# @cloze` → set `pendingStudy.CardType = Cloze`; detect `# @study`, `# @study reverse`, `# @study sequence` → open study block, set `parserState.studyBlock`; detect `# @end` → close study block; apply block defaults to nodes within the block; individual markers on a node override block defaults (`FromBlock = false`); extract `{braces}` content into `StudyMeta.ClozeTargets`; close study block on new `::` replacement or EOF; ditto lines inherit parent's `StudyMeta`; arrows with `studyable: false` in registry become hints, not tested
- [x] T024 [US3] Implement schedulable-type identity in `internal/content/parser.go` per FR-018a–d: the parser exposes all nodes (marked and unmarked) with `Line` set; `Node.ID` is computed only from `# @id` marker (T020, already done); no special handling for topics (structural identity is computed by callers from `filepath + chapter`); document in code comments that `@label` plays no role in `Node.ID`
- [x] T025 [US3] Implement `InjectMissingIDs(parsed *ParsedFile) ([]IDInjection, error)` in `internal/content/writeback.go`: iterate all chapters and nodes in `parsed`; for each node with `StudyMeta != nil` and `ID == ""`: call `GenerateShortID()`, verify uniqueness by scanning all existing `# @id` values in the file (re-read file once at start, build a `map[string]bool`), regenerate up to 10 times on collision; determine insertion line (just above the first `# @card`/`# @study`/`# @cloze` line preceding the node, or directly above the node if none); collect all insertions as `writeBackOp{Kind: Insert}`; sort ops descending by `LineNumber`; apply all to the file content in memory; write to a temp file in the same directory then `os.Rename` atomically; mutate `parsed` nodes to reflect new `IDMarkerLine` and `ID` values; return `[]IDInjection` with one entry per node (including `AlreadyHad: true` entries for already-IDed nodes)
- [x] T026 [US3] Implement `InjectNodeID(path string, nodeLineNumber int) (*IDInjection, error)` in `internal/content/writeback.go`: read the file; check if the node at `nodeLineNumber` already has a `# @id` immediately above it (scan backwards from `nodeLineNumber` skipping blank lines and other `#` markers); if already has `# @id` return `&IDInjection{AlreadyHad: true}` with no write; otherwise generate ID, verify uniqueness, insert `# @id <shortid>` above the node (or above its closest study marker), write atomically via temp-rename; return the injection details
- [x] T027 [US3] Write tests for study markers and write-back in `internal/content/parser_test.go`: `TestStudyMarkerExtraction` (parse `study-markers.n4l` → assert each node has correct `CardType`, `ForceReverse`, `FromBlock`; assert unmarked nodes have `StudyMeta == nil`; assert sequence block generates sequence card types), `TestBlockClosedByContextReset` (study block open → `::` operator → block closes, subsequent nodes have no `StudyMeta`), `TestInjectMissingIDs` (inject on `study-markers.n4l` copy → assert `# @id` lines present, unaffected lines byte-identical via `diff`), `TestInjectMissingIDsIdempotent` (inject twice → all `AlreadyHad == true`, file unchanged), `TestInjectNodeIDIdempotent` (call `InjectNodeID` on already-IDed node → `AlreadyHad == true`, no write), `TestExtractToItemPromotion` (inject node ID as extract via `InjectNodeID`; then add `# @card` and call `InjectMissingIDs` → same ID preserved, `AlreadyHad == true`)

**Checkpoint**: `go test ./internal/content/... -run TestStudy -run TestInject` passes.

---

## Phase 6: User Story 4 — Reminders and Priority Markers (Priority: P3)

**Goal**: ALL CAPS reminder lines (≥ 3 words) are detected and collected; `# @priority`,
`# @source`, `# @ref`, `# @progress` are extracted and attached to nodes; `ResolveReminder`
and `UpdateProgress` write-back operations modify only the targeted line.

**Independent Test**: Parse `testdata/reminders.n4l`; assert `ParsedFile.Reminders` contains
exactly the expected set (correct line numbers, no single-word or two-word tokens, no mixed-case
lines); call `ResolveReminder` on one reminder line; assert that line is lowercased and all
other lines are byte-identical.

### Implementation

- [x] T028 [P] [US4] Create `internal/content/testdata/reminders.n4l`: one chapter with — 3 genuine ALL CAPS reminders (≥ 3 words, e.g. `FINISH THIS SECTION`); 2 false-positive candidates (one single word `API`, one two-word `TODO NOW`); 1 reminder with a trailing inline comment (`CHECK THIS REFERENCE # added 2026`); 2 mixed-case lines (should not be reminders); 3 nodes with `# @priority N` (one out-of-range at 150); 2 nodes with `# @source` / `# @ref` / `# @progress` provenance markers
- [x] T029 [US4] Implement reminder detection in `internal/content/parser.go`: for each non-comment, non-marker line strip trailing `\s*#.*$`; split on whitespace; check every token against `[A-Z0-9\-_']+`; if all tokens match and `len(tokens) >= 3`, create `Reminder{File, Line, Text}` and append to `parsedFile.Reminders`; also set a flag on the corresponding `Node` if the reminder is a node line
- [x] T030 [US4] Implement `# @priority`, `# @source`, `# @ref`, `# @progress` extraction in `internal/content/parser.go`: detect `# @priority N` → parse N as int; if outside 0–100 emit `ParseError{Msg: ErrInvalidPriority}` and skip; otherwise store as `*int` in `parserState.lastPriority`, apply to next node's `Node.Priority`; detect `# @source`, `# @ref`, `# @progress` → accumulate into `parserState.pendingProvenance`; apply accumulated `Provenance` to next node and clear
- [x] T031 [US4] Implement `ResolveReminder(path string, lineNumber int) (*WriteBackResult, error)` in `internal/content/writeback.go`: read file; verify that line `lineNumber` passes the ALL CAPS reminder rule (safety guard — return error if not); lowercase the token portion of the line, preserving any trailing `# …` comment verbatim; apply via temp-rename; return `WriteBackResult{LinesChanged: []int{lineNumber}}`
- [x] T032 [US4] Implement `UpdateProgress(path string, lineNumber int, newValue string) (*WriteBackResult, error)` in `internal/content/writeback.go`: read file; verify line `lineNumber` starts with `# @progress `; replace the value after `# @progress ` with `newValue`; apply via temp-rename; return `WriteBackResult{LinesChanged: []int{lineNumber}}`
- [x] T033 [US4] Write tests in `internal/content/parser_test.go`: `TestReminderDetection` (parse `reminders.n4l` → assert `Reminders` has exactly 3 entries with correct lines, `API` and `TODO NOW` excluded, mixed-case excluded), `TestReminderWithComment` (reminder with trailing comment → `Text` excludes comment), `TestPriorityOutOfRange` (parse node with `# @priority 150` → `ParseError`, `Node.Priority == nil`), `TestProvenanceAttachment` (parse node with `# @source` / `# @ref` / `# @progress` → `Node.Provenance` populated), `TestResolveReminder` (call on reminder line → line lowercased, all other lines byte-identical), `TestResolveReminderGuard` (call on non-reminder line → error, file untouched), `TestUpdateProgress` (update `# @progress` value → only that line changes), `BenchmarkScanCollection500` using 500 copies of `simple.n4l` in a temp dir; add gate: fails if median exceeds 500 ms

**Checkpoint**: `go test ./internal/content/...` passes all tests. `go test -bench=. ./internal/content/...` shows BenchmarkParseFile10k < 50 ms and BenchmarkScanCollection500 < 500 ms.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: CI gate, lint, Wails app wiring stub.

- [x] T034 Add `bench` target to `Makefile`: `go test -bench=. -benchtime=3s ./internal/content/... | tee bench.txt`; add a check script `scripts/check-bench.sh` that parses `bench.txt` and exits non-zero if `BenchmarkParseFile10k` > 50 ms or `BenchmarkScanCollection500` > 500 ms; wire into `make ci`
- [x] T035 [P] Run `make lint` (`golangci-lint run ./internal/content/...`) and fix all reported issues; ensure `gofmt` formatting on all new files; verify no `//nolint` directives have been added without a comment explaining why
- [x] T036 [P] Add stub wiring in `app.go`: add `registry *content.SSTRegistry`, `watcher *content.CollectionWatcher` fields to the `App` struct; in `startup()` call `content.LoadRegistry(...)` and `content.ScanCollection(...)` and `content.WatchCollection(...)`; expose a `ParseFileForUI(path string) (*content.ParsedFile, []content.ParseError)` method as a Wails binding; these stubs do not need full UI integration — they just verify the package compiles and the API is callable from `app.go`
- [x] T037 [P] Verify cross-compilation: run `make build-windows` (cross-compile to Windows amd64) and confirm the binary builds without CGo errors; `fsnotify` must compile cleanly for `GOOS=windows`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — **blocks all user stories**
- **Phase 3 (US1)**: Depends on Phase 2 — can begin as soon as T004–T008 complete
- **Phase 4 (US2)**: Depends on Phase 2 — can begin in parallel with Phase 3
- **Phase 5 (US3)**: Depends on Phase 4 (needs full parser for study marker extraction)
- **Phase 6 (US4)**: Depends on Phase 4 (needs full parser) — can run in parallel with Phase 5
- **Phase 7 (Polish)**: Depends on Phases 3–6 complete

### User Story Dependencies

- **US1 (P1)**: Needs T004 (types), T005 (errors), T007 (registry) from Foundational
- **US2 (P1)**: Needs T004, T005, T006 (nodeid), T007 from Foundational; US2 and US1 can proceed in parallel after Phase 2
- **US3 (P2)**: Needs US2 complete (study markers build on the parser from T015–T020)
- **US4 (P3)**: Needs US2 complete (reminder detection extends parser.go); can run in parallel with US3

### Within Each Phase

- Testdata fixture tasks `[P]` → can run alongside implementation tasks
- Tests must be written after the implementation task they cover (or in parallel as TDD stubs)
- `parser.go` tasks T015–T020, T023–T024, T029–T030 are sequential (same file)
- `writeback.go` tasks T025–T026, T031–T032 are sequential (same file)

---

## Parallel Opportunities

### Phase 2 — Foundational

```
T004 types.go  ──┐
T005 errors.go ──┤ all in parallel
T006 nodeid.go ──┤
T008 large.n4l ──┘
T007 registry.go (after T004)
```

### Phase 3 + Phase 4 — US1 and US2 in parallel (after Phase 2)

```
T009 testdata [P]  → T010 scanner → T011 scanner tests → T012 watcher → T013 watcher tests
T014 testdata [P]  → T015 parser skeleton → T016 context → T017 ditto → T018 labels → T019 arrows → T020 @id reading → T021 tests
```

### Phase 5 + Phase 6 — US3 and US4 in parallel (after Phase 4)

```
T022 testdata [P]  → T023 study markers → T024 ID identity → T025 InjectMissingIDs → T026 InjectNodeID → T027 tests
T028 testdata [P]  → T029 reminders → T030 priority/provenance → T031 ResolveReminder → T032 UpdateProgress → T033 tests
```

---

## Implementation Strategy

### MVP (User Stories 1 + 2 only)

1. Phase 1: Setup
2. Phase 2: Foundational (T004–T008)
3. Phase 3: US1 — collection loading (T009–T013)
4. Phase 4: US2 — full parse (T014–T021)
5. **STOP and VALIDATE**: `ScanCollection` populates a list from a real notes directory; `ParseFile` on any file returns correct chapters, nodes, and context states; benchmark < 50 ms
6. The application can open a collection and display its structure — independently usable

### Incremental Delivery

1. MVP (US1 + US2) → collection browser functional
2. Add US3 (study markers + write-back) → FSRS scheduling can begin
3. Add US4 (reminders + priority) → reminder panel and curation priority functional
4. Phase 7 (polish) → CI-gated, lint-clean, Wails-wired

---

## Notes

- All `parser.go` changes (T015–T020, T023–T024, T029–T030) are sequential — they extend the same state machine
- All `writeback.go` changes (T025–T026, T031–T032) are sequential — they extend the same atomic-write infrastructure
- Benchmark gates (< 50 ms parse, < 500 ms scan) are constitutional requirements, not suggestions
- Write-back tests MUST verify byte-for-byte fidelity on unaffected lines (SC-008)
- `# @id` is the sole node identity mechanism — do not introduce any label-based or hash-based ID path
- `internal/content` must not import `internal/graph` or `internal/learning` at any point
