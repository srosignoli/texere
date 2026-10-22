# Implementation Plan: N4L File Parser — Layer I Content I/O

**Branch**: `002-n4l-parser` | **Date**: 2026-04-07 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `/docs/superpowers/specs/002-n4l-parser/spec.md`

## Summary

Implement `internal/content` as a fully operational Go package with two complementary paths:
(1) a **decode path** — single-pass state-machine parser producing a typed in-memory AST,
a lightweight chapter-title scanner for fast collection startup, and a filesystem watcher
that keeps Column 1 live; and (2) a **write-back path** — targeted, atomic, line-level
modifications to `.n4l` files for `# @id` injection (stable node identity), reminder
resolution, and `@progress` updates. The package is the sole Layer I component — all
content read by the graph builder (Layer II) and scheduling engine (Layer III) flows through
its decode path; all structured writes to `.n4l` files flow through its write-back path.

## Technical Context

**Language/Version**: Go 1.22 (enforced via `go.mod`)  
**Primary Dependencies**: `github.com/fsnotify/fsnotify` v1.7 (pure Go, cross-platform inotify/ReadDirectoryChanges/kqueue); `crypto/rand` for `# @id` generation; `bufio` for streaming line reads; `os.Rename` for atomic write-back  
**Storage**: None (parser is read-only; SQLite writes happen in `internal/learning` on save events triggered by the caller)  
**Testing**: `go test ./internal/content/...`; round-trip fidelity tests mandatory (constitution); benchmark tests mandatory for parse + context replay path  
**Target Platform**: Linux amd64 + Windows amd64 (cross-compiled)  
**Project Type**: Internal Go library package within Wails v2 desktop app  
**Performance Goals**: Parse + context replay < 50 ms for a 10,000-line file (constitution §Performance Standards); directory scan < 500 ms for 500 files; file-watch event latency < 200 ms  
**Constraints**: Pure Go (no CGo); `fsnotify` uses only OS syscalls — no CGo, compliant; single-pass parser preferred for O(n) time and O(1) lookahead  
**Scale/Scope**: 500 files × 10,000 lines per file as upper performance target

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Source-First & N4L Fidelity | ✅ PASS | Parser is strictly read-only; full N4L syntax surface covered (context ops, sequence, ditto, labels, study markers, reminders, provenance); no proprietary format introduced |
| II. Self-Contained Binary | ✅ PASS | `fsnotify` is pure Go (no CGo); `hash/fnv` is stdlib; no new CGo dependencies |
| III. Separation of Concerns | ✅ PASS | `internal/content` outputs plain Go structs only; it does not touch BadgerDB or SQLite directly; callers in `app.go` route output to Layers II and III |
| IV. Pleasure-Driven Learning | N/A | Parser is infrastructure, not user-facing study UX |
| V. Transparency & Portability | ✅ PASS | Parser never modifies `.n4l` files; errors reported as structured values, not silently swallowed |
| **Performance Standard** | ⚠️ SPEC VIOLATION RESOLVED | Spec SC-002 said "<1 second" but constitution mandates "< 50 ms" for parse + context replay on a 10,000-line collection. **SC-002 updated in spec to < 50 ms.** Plan targets 50 ms hard limit; benchmark test gates CI. |

**Post-Phase 1 re-check**: All gates pass. Data model confirms parser outputs plain structs with no layer boundary violations. `fsnotify` addition logged in go.mod; no CGo introduced.

### Complexity Tracking

| Addition | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| `github.com/fsnotify/fsnotify` | FR-006b: live collection watch; cross-platform (Linux inotify, Windows ReadDirectoryChanges) | `os.Stat` polling loop: battery-unfriendly, 1-second minimum latency, requires background goroutine management |

## Project Structure

### Documentation (this feature)

```text
docs/superpowers/specs/002-n4l-parser/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── parser-api.md    # Phase 1 output — Go package public API contract
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/content/
├── doc.go                  # Package doc (exists — stub)
├── registry.go             # SSTconfig arrow + annotation registry loader
├── scanner.go              # Phase 1 scan: extract chapter titles only (fast startup)
├── parser.go               # Phase 2 parse: full single-pass state machine
├── watcher.go              # fsnotify-based collection directory watcher
├── writeback.go            # Atomic targeted file modifications (# @id, reminders, @progress)
├── nodeid.go               # Stable node ID computation (three-tier: label / @id / none)
├── types.go                # All exported structs (ParsedFile, Node, Arrow, IDInjection, …)
├── errors.go               # ParseError type
└── parser_test.go          # Unit + round-trip fidelity + benchmark + write-back tests

internal/content/testdata/
├── simple.n4l              # Basic chapter, nodes, arrows
├── multi-chapter.n4l       # Multiple chapters in one file
├── contexts.n4l            # :: / +:: / -:: state machine cases
├── sequence.n4l            # _sequence_ tag cases
├── study-markers.n4l       # All study marker variants
├── reminders.n4l           # ALL CAPS reminder threshold cases
├── labels.n4l              # @label / $label.N / $PREV.N
├── errors.n4l              # Intentionally malformed lines
└── large.n4l               # 10,000-line benchmark fixture
```

## Phase 0: Research

See [research.md](research.md) for full decision log. Key decisions:

- **Node identity**: `# @id <shortid>` write-back markers — sole system-managed ID, injected on first save of any study-marked node (labeled or not). `@label` is a cross-reference anchor only and plays no role in identity. Hash-based IDs rejected (break on text edits); label-based IDs rejected (labels can be renamed/removed for cross-referencing reasons, orphaning FSRS history identically to hashes).
- **File watcher**: `fsnotify/fsnotify` v1.7 — pure Go, cross-platform, de-facto standard in Go ecosystem
- **Directory scan**: `os.ReadDir` + `bufio.Scanner` per file; only lines prefixed `"- "` collected; scan terminates full-file read (chapters can be anywhere, so full scan required but cheap)
- **Parser architecture**: Single-pass line-by-line state machine; no backtracking; O(n) time, O(k) memory where k = nodes in current chapter

## Phase 1: Design

See [data-model.md](data-model.md) and [contracts/parser-api.md](contracts/parser-api.md) for full design.

### Key architectural decisions

**Single-pass state machine**: The N4L grammar is regular enough for a single forward pass. Parser state:
- `currentChapter` — pointer to the chapter being built
- `effectiveContext` — `map[string]bool` replayed by `::` / `+::` / `-::`
- `dittoParent` — pointer to the last Node (for ditto resolution)
- `studyBlock` — current block marker state (`none` / `card` / `study` / `cloze`)
- `labelMap` — `map[string]*Node` for `@label` definitions
- `pendingLabels` — slice of `$label.N` forward references resolved in a second pass within the same file

**Two-phase label resolution**: `@label` definitions and `$label.N` references are collected in a first pass; references are resolved in a second pass over the same in-memory node list. Both passes are O(n); the second pass is cheap because all nodes are already parsed.

**Node ID scheme**:
- Labeled: `<filepath>:<chapter-title>:@<label>` — stable across all edits that don't rename the label
- Unlabeled: `<filepath>:<chapter-title>:fnv64a(<nodeText>)` — stable as long as node text is unchanged; breaks on text edit (acceptable tradeoff per clarification Q1)

**Reminder rule**: A line is a reminder iff: (a) after stripping any trailing `# …` comment, (b) every character is uppercase or whitespace, and (c) the result contains ≥ 3 whitespace-separated words.

**Watcher design**: `CollectionWatcher` wraps `fsnotify.Watcher` and emits `CollectionEvent` values (Added / Removed / Renamed / Modified) on a channel. The caller (Wails App struct) consumes events and updates Column 1 accordingly. The watcher performs a lightweight chapter-title re-scan on Add/Rename events to obtain the new file's chapter list before emitting.

**Write-back design** (`writeback.go`): Four exported operations — `InjectMissingIDs`, `InjectNodeID`, `ResolveReminder`, `UpdateProgress`. All use temp-file-then-rename atomicity (FR-022). Multiple ops on the same file are batched into a single rewrite with ops sorted descending by line number so earlier insertions don't shift subsequent targets. Both injection functions use `crypto/rand` for ID generation and verify uniqueness within the file before writing (FR-023).

**Two injection callers, one mechanism**:
- `InjectMissingIDs(parsed)` — called by `app.go` on Ctrl+S; injects IDs for all study-marked nodes without one (items)
- `InjectNodeID(path, lineNumber)` — called by `internal/learning` when a node enters the distillation pipeline (extracts); idempotent if `# @id` already present

A node promoted from extract to item already has a `# @id` from the extract injection; `InjectMissingIDs` skips it (AlreadyHad=true). One ID per node; the SQLite `type` column distinguishes extract vs. item records referencing the same `node_id`.

### Performance verification plan

- Benchmark `ParseFile` on `testdata/large.n4l` (10,000 lines): must pass < 50 ms gate
- Benchmark `ScanChapters` on 500 synthetic files: must pass < 500 ms gate
- Both benchmarks run in CI via `make bench` target with `-benchtime=3s`
- Write-back tests verify byte-for-byte fidelity on unaffected lines (SC-008)
