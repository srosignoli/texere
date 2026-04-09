# Research: N4L File Parser — Layer I Content I/O

**Feature**: `002-n4l-parser` | **Date**: 2026-04-07

---

## Decision 1: Stable Node Identity Scheme

**Question**: How should unlabeled nodes receive stable identifiers that survive text edits?

**Decision**: `# @id <shortid>` write-back markers — a system-assigned 8-character base-36 opaque ID injected into the `.n4l` file on first save of a study-marked unlabeled node. Full three-tier precedence:
1. `@label` (user-assigned) → `<filepath>:<chapter>:@<label>`
2. `# @id <shortid>` (system-assigned) → `<filepath>:<chapter>:id:<shortid>`
3. No ID — node is parsed but not schedulable until an ID exists

**Rationale**:
- Text-hash IDs (the prior approach) break on any edit to the node's content. Since notes are designed for continuous improvement, hash-based IDs would progressively orphan FSRS history — making the learning state unreliable over time.
- Writing the ID into the source file aligns with Principle I (source-first): all stable identity lives in the `.n4l` file, not in a derived store.
- The approach is transparent: users can see `# @id k7mpq9rx` in their file, understand it, and even transfer it manually if they restructure their notes.
- Base-36 (digits + lowercase letters): 36^8 ≈ 2.8 trillion combinations; collision probability is negligible for any realistic collection. Compact, human-readable, URL-safe.
- IDs are injected above the closest study marker (or directly above the node if no marker), so the file remains readable: `# @id k7mpq9rx` / `# @card` / `node text`.

**Alternatives considered**:
- FNV-64a hash of node text: zero write-back needed, but breaks on every text edit — rejected as fundamentally incompatible with the continuous-improvement workflow.
- `@label` as ID (highest-priority tier): rejected. Labels are cross-reference anchors whose primary function is letting other nodes point to `$label.N`. Users rename and remove labels for cross-referencing reasons, which would silently orphan FSRS history — identical fragility to hashes, just less frequent. Conflating cross-reference identity with scheduling identity creates two failure modes instead of one. `@label` is excluded from ID computation entirely.
- UUID v4: globally unique, but 36 characters is verbose in a plain-text file; base-36 at 8 chars is sufficient and less visually intrusive.
- User-required labeling: forcing users to add `@label` to every studyable node creates friction and diverges from how N4L is written in practice — rejected.
- Sequential counter per file (`# @id 001`): predictable but means adding a node anywhere in the file renumbers all subsequent counters — rejected.

---

## Decision 2: Filesystem Watcher Library

**Question**: Which library should implement the live collection directory watch (FR-006b)?

**Decision**: `github.com/fsnotify/fsnotify` v1.7.x.

**Rationale**:
- Pure Go — no CGo, compliant with Principle II.
- Cross-platform: Linux (inotify), Windows (ReadDirectoryChangesW), macOS (FSEvents), BSD (kqueue).
- De-facto standard: used by Viper, Hugo, Cobra, and Wails itself internally — battle-tested, well-maintained.
- Minimal API: `Watcher.Add(dir)` + event channel — straightforward to wrap.

**Alternatives considered**:
- `os.Stat` polling loop: no dependency, but battery-unfriendly (constant syscalls), minimum 1-second latency, requires manual goroutine lifecycle management.
- `golang.org/x/exp/watch`: experimental, not stable API.
- Raw inotify/kqueue syscalls: platform-specific, requires build tags, duplicates what fsnotify already provides.

**Note**: `fsnotify` is already an indirect dependency of `github.com/wailsapp/wails/v2` (Wails uses it for frontend hot-reload in dev mode). Adding it as a direct dependency does not increase binary size.

---

## Decision 3: Directory Scan Strategy

**Question**: How should the Phase 1 directory scan efficiently extract chapter titles from all files?

**Decision**: `os.ReadDir` for file listing + `bufio.Scanner` per file reading all lines but collecting only lines with the prefix `"- "` (chapter title marker).

**Rationale**:
- `os.ReadDir` returns a sorted `[]DirEntry` (alphabetical) in a single syscall — no shell invocation, O(1) per entry for name/type metadata.
- `bufio.Scanner` with default 64KB line buffer handles all practical N4L line lengths.
- Full-file scan (not just first N lines) is required because N4L permits multiple chapters anywhere in a file; however, reading only `- ` prefixed lines from a 10,000-line file takes < 1 ms — well within the 500 ms budget for 500 files.
- Returning an alphabetically sorted list from `os.ReadDir` directly satisfies FR-004 (alphabetical by filename, case-insensitive) with one `strings.ToLower` comparison pass.

**Alternatives considered**:
- Read only first 100 lines: misses late chapters in multi-chapter files — incorrect.
- Memory-map files: overhead not justified for a sequential line scan; complicates Windows cross-compilation.
- Parallel fan-out (one goroutine per file): marginal benefit for 500 files; adds synchronisation complexity; defer to benchmarking if profiling shows scan is a bottleneck.

---

## Decision 4: Parser Architecture

**Question**: What parsing strategy is appropriate for the N4L grammar?

**Decision**: Single-pass, line-by-line state machine with a second in-memory pass for label resolution.

**Rationale**:
- The N4L grammar is largely sequential: each line's interpretation depends only on parser state accumulated from previous lines (chapter, effective context set, ditto parent, study block). No lookahead is needed.
- A state machine processes each line in O(1) time; total parse is O(n) — easily under 50 ms for 10,000 lines.
- A second pass for label resolution (`$label.N`, `$PREV.N`) is necessary because labels may be defined after they are referenced. The second pass is O(r) where r = number of references, which is small.
- No external parser generator needed; the grammar is simple enough for hand-written code, which is easier to maintain and produces better error messages.

**Alternatives considered**:
- ANTLR or PEG grammar: adds tooling dependency, generated code, and a build step — significant complexity for a simple line-oriented grammar.
- Two-pass from the start: unnecessary; a single pass with deferred label resolution is cleaner.
- AST-rewriting parser: overkill; N4L output is a direct structural model, not a general-purpose AST.

---

## Decision 5: Performance Target Reconciliation

**Question**: The spec (SC-002) stated "<1 second" for parsing 10,000 lines, but the constitution mandates "<50 ms." Which governs?

**Decision**: The constitution governs. SC-002 has been updated to "<50 ms" in the spec. The plan targets 50 ms as a hard CI gate.

**Rationale**: The constitution §Performance Standards is the canonical source of latency budgets for all interactive operations, and it explicitly states "N4L parse + context replay < 50 ms" for a 10,000-line collection. The spec's "<1 second" was an oversight. A single-pass state machine in Go is capable of processing 10,000 lines in < 5 ms in practice; the 50 ms budget is generous.

---

## Decision 6: Reminder Detection Algorithm

**Question**: How should the reminder threshold (≥3 all-caps words) be implemented?

**Decision**: After stripping a trailing `# …` comment (regex: `\s*#.*$`), split remaining text on whitespace. If every resulting token matches `[A-Z0-9\-_']+` (all uppercase letters, digits, hyphen, underscore, apostrophe) and the token count is ≥ 3, the line is a reminder.

**Rationale**:
- Allows common technical patterns like `CHECK THIS BEFORE MERGE` or `FINISH SECTION THREE` while excluding `API`, `TODO`, `HTTP GET`.
- Hyphen and underscore are included to handle reminders like `RE-READ CHAPTER FIVE` without false negatives.
- Digits are included so reminders like `READ CHAPTER 3 NOW` are caught.

**Alternatives considered**:
- Pure `strings.ToUpper(line) == line`: too broad; catches any line that happens to have no lowercase letters (e.g., a line with only numbers or punctuation).
- NLP verb detection (Option D from clarification): disproportionately complex and language-dependent.

---

## Decision 7: Write-Back Atomicity Strategy

**Question**: How should write-back operations (FR-019–022) ensure the original file is never corrupted by a partial write?

**Decision**: Write-to-temp-then-rename. The new file content is written to a temporary file in the same directory (same filesystem), then `os.Rename` atomics the swap. On POSIX systems `rename(2)` is guaranteed atomic by the kernel. On Windows, `os.Rename` is not guaranteed atomic but is effectively atomic for same-volume renames on NTFS; the failure window is microseconds and the worst outcome is a leftover temp file, not a corrupted original.

**Rationale**:
- The original file is never touched until the new content is fully written and flushed.
- A crash between `Write` and `Rename` leaves the original intact and a temp file behind; the temp file is cleaned up on next startup.
- Same-directory placement ensures the temp file is on the same filesystem as the original, making `Rename` a metadata-only operation (no data copy).

**Alternatives considered**:
- In-place overwrite: simplest, but a crash mid-write corrupts the original — unacceptable for user-owned knowledge files.
- Write to system temp dir then move: risks a cross-filesystem `rename` that falls back to copy+delete — loses atomicity guarantee.
- Backup original before write: redundant complexity; the rename approach already provides the safety guarantee.
