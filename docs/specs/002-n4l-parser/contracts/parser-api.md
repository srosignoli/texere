# Contract: `internal/content` Package API

**Feature**: `002-n4l-parser` | **Date**: 2026-04-07  
**Package path**: `texere/internal/content`

This document defines the public Go API that the `internal/content` package exposes to its callers (`app.go`, `internal/graph`, `internal/learning`). Callers MUST depend only on the signatures defined here — internal implementation types (e.g., `parserState`) are not part of this contract.

---

## 1. Registry

### `LoadRegistry(sstconfigDir string) (*SSTRegistry, error)`

Reads the SSTconfig directory and builds the arrow + annotation registry from `.sst` files. Must be called once at application startup before any parse calls.

The SSTconfig directory contains files with the `.sst` extension. File roles by name:
- `arrows-*.sst` — arrow pair definitions; first line is `- meta-type`; pairs are `+ forward name (code)` optionally followed by `- reverse name (revcode)`; `(img)` and `(url)` are reserved N4L relations
- `annotations.sst` — annotation symbol mappings of the form `symbol (code)` (symbols: `***`, `**`, `=`, `>>`, `%>`, `%<`)
- `closures.sst` — arrow transitivity rules `(a) + (b) => (c)`; parsed but not enforced here (graph layer responsibility)

Both `//` (line-level) and `#` (inline) comment styles are stripped before parsing.

- If `sstconfigDir` does not exist: returns a non-nil `*SSTRegistry` with empty maps and an error. Caller SHOULD log a degraded-mode warning but MUST NOT crash.
- If individual `.sst` files cannot be parsed: errors are accumulated in `SSTRegistry.LoadErrors`; partial registry is returned.

**Caller**: `app.go` on startup.

---

## 2. Directory Scan (Phase 1)

### `ScanCollection(dir string) ([]ChapterSummary, error)`

Scans `dir` for `.n4l` files and returns a sorted (case-insensitive alphabetical by filename) slice of `ChapterSummary`. Non-`.n4l` files and subdirectories are silently ignored.

- If a file cannot be read, its `ChapterSummary.ScanError` is non-nil; it is still included in the returned slice.
- Returns an error only for directory-level failures (e.g., `dir` does not exist or is not a directory).
- **Performance contract**: completes in < 500 ms for ≤ 500 files.

**Caller**: `app.go` at collection open time; also called by `CollectionWatcher` internally on `Added`/`Renamed` events.

---

## 3. Full Parse (Phase 2, on-demand)

### `ParseFile(path string, registry *SSTRegistry) (*ParsedFile, []ParseError)`

Parses the `.n4l` file at `path` and returns the complete in-memory representation.

- Returns a non-nil `*ParsedFile` even if `ParseError` entries are present — partial results are always returned.
- Returns `nil, []ParseError{{Msg: "encoding error", …}}` only if the file cannot be read at all.
- **Determinism contract**: calling `ParseFile` on the same file content with the same registry always returns structurally identical output (same node IDs, same slice ordering).
- **Performance contract**: completes in < 50 ms for a 10,000-line file (constitution §Performance Standards).

**Caller**: `app.go` when the user selects a file/chapter in Column 1.

---

## 4. Collection Watcher (Phase 3)

### `WatchCollection(dir string) (*CollectionWatcher, error)`

Starts a filesystem watcher on `dir`. Returns a `*CollectionWatcher` whose `Events` channel emits `CollectionEvent` values as files are added, removed, renamed, or modified.

- The watcher performs a `ScanCollection` internally on `Added`/`Renamed` events and attaches the resulting `ChapterSummary` to the emitted event.
- `Modified` events are emitted for content changes to existing files; no automatic re-scan of chapter titles is performed (callers may choose to re-scan if needed).
- Calling `CollectionWatcher.Close()` stops the watcher and closes both channels.
- Returns an error if `dir` does not exist or the OS watcher cannot be initialised.

**Caller**: `app.go` after `ScanCollection` completes; caller owns the goroutine that reads from `Events`.

---

## 5. Stable Node ID (utility)

### `NodeID(filepath, chapterTitle, shortID string) string`

Computes the stable node identifier:
- If `shortID != ""`: returns `filepath + ":" + chapterTitle + ":id:" + shortID`
- Else: returns `""` (node is untracked)

`@label` values are **not accepted** by this function — labels are cross-reference anchors, not identity tokens. The `shortID` parameter corresponds to the value on the node's `# @id` marker line, accessible via `Node.ID` (which stores the full computed key) or by parsing the raw `IDMarkerLine` content.

**Exposed as a utility** so that callers (`internal/learning`, `internal/graph`) can reconstruct IDs for database lookup without depending on parsed output.

---

## 6. Write-Back Operations

The `# @id` marker is the node's identity, shared across all scheduling record types (extract, item) that reference it. Two functions expose ID injection: a batch function for save-time item injection and a single-node function for scheduler-triggered extract injection.

---

### `InjectMissingIDs(parsed *ParsedFile) ([]IDInjection, error)`

For every study-marked node in `parsed` that has `ID == ""` (no `# @id` marker), generates a unique 8-character base-36 ID and inserts a `# @id <shortid>` line immediately above the node's closest preceding study marker (or directly above the node if no marker line precedes it).

- Nodes that already have `ID != ""` are skipped — no duplicate injection.
- Uniqueness is verified by scanning all existing `# @id` values in the file; regenerates up to 10 times on collision before returning an error.
- Writes via temp-file-then-rename (atomic per FR-022). All injections for a file are batched into a single rewrite.
- On success, the `ParsedFile` passed in is mutated to reflect the new IDs — callers do not need to re-parse.
- On error, the file is untouched.

**Caller**: `app.go` on every `Ctrl+S` save event, after the full parse completes (FR-019).

---

### `InjectNodeID(path string, nodeLineNumber int) (*IDInjection, error)`

Injects a `# @id <shortid>` for a single specific node identified by its line number. Used by the incremental reading scheduler when an unmarked node (extract) enters the distillation pipeline.

- `nodeLineNumber` is the 1-indexed line number of the node text line (as reported in `Node.Line` from a prior `ParseFile` call).
- If the node at that line already has a `# @id` marker, returns the existing `IDInjection` with no file modification — idempotent.
- The inserted line is placed immediately above the node line (or above any `# @card`/`# @study` marker if one exists directly above the node).
- Same uniqueness verification and atomic write guarantees as `InjectMissingIDs`.
- On success, returns the `IDInjection` describing the inserted marker and its generated `ShortID`.
- On error, the file is untouched.

**Caller**: `internal/learning` (incremental reading scheduler) when a node is picked for the distillation pipeline for the first time (FR-019b).

---

### `ResolveReminder(path string, lineNumber int) (*WriteBackResult, error)`

Lowercases the ALL CAPS text on `lineNumber` of `path`, preserving any trailing `# …` inline comment. All other lines are byte-for-byte identical.

- Returns an error if `lineNumber` does not contain an ALL CAPS line (safety guard against stale line numbers).
- Writes via temp-file-then-rename.

**Caller**: `app.go` when the user clicks "Resolve" in the reminder panel.

---

### `UpdateProgress(path string, lineNumber int, newValue string) (*WriteBackResult, error)`

Replaces the value after `# @progress ` on `lineNumber` of `path` with `newValue`. All other lines are byte-for-byte identical.

- Returns an error if `lineNumber` does not contain a `# @progress` marker.
- Writes via temp-file-then-rename.

**Caller**: `app.go` when the user advances their reading progress.

---

## 7. Error Handling Conventions

- All functions that return `error` (not `[]ParseError`) use standard Go errors; wrap with `fmt.Errorf("content: %w", err)` for stack context.
- `[]ParseError` is always returned alongside the parsed result — callers MUST NOT treat a non-nil/non-empty `[]ParseError` as a fatal failure.
- `ParseError.Line` is 1-indexed. `ParseError.Line == 0` means the error is file-level (e.g., encoding error on the entire file).

---

## 8. Thread Safety

- `SSTRegistry` is read-only after construction — safe for concurrent use.
- `ParseFile` and `ScanCollection` are stateless — safe to call concurrently.
- `CollectionWatcher` is safe to call `Close()` from any goroutine; `Events` and `Errors` channels MUST be read from a single goroutine (or with external synchronisation).

---

## 9. Versioning

Breaking changes to this contract (added required parameters, removed functions, changed return types) constitute a `MAJOR` version bump per the constitution. Additive changes (new optional fields on structs, new utility functions) are `MINOR`. Bug fixes with identical signatures are `PATCH`.
