# Incremental Reading — Phase 2a (Read & Extract) Design

**Date:** 2026-06-20
**Status:** Design approved, pending spec review
**Parent strategy:** `docs/superpowers/specs/2026-06-20-incremental-reading-design.md` (this is the first slice of that doc's Phase 2)
**Builds on:** Phase 1 recall spine (`internal/learning/`, the Study/Recall overlay) — merged as `008-incremental-reading`.

## 1. Context & slice

Phase 2 (the reading→extract→process→recall loop) is too large for one plan, so it is cut into three shippable slices:

- **2a (this doc) — Read & extract:** the `internal/source/` abstraction + the first adapter (local text/HTML), reading-element registration + scheduling, and the Read face (source pane beside notes; select → verbatim extract). Extracts can already become cards via Phase-1 editor markers + Recall.
- **2b — Process & unify:** the Process face (focused editor + study-action bar), extract scheduling (the deferral formula), and folding Read/Process/Recall into one Study surface with the focus switch + interleaved queue.
- **2c — More media + bridges:** PDF and web-URL adapters; explorer "Study this" bridge.

## 2. Decisions (from brainstorming)

| # | Decision |
|---|---|
| 1 | **First medium:** local **text/HTML** (`.txt`/`.md`/`.html`). Lowest-risk projection + anchor; PDF/web are later adapters. |
| 2 | **Notes layout:** importing auto-creates a **per-source notes file** `reading/<name>.n4l` holding the reading element (`# @ref`/`# @source`/`# @progress`); extracts append under a `_sequence_` context. 1:1 source↔notes. |
| 3 | **Extract capture:** the **verbatim selection** becomes the extract node (with a `(from)` anchor); refined later in Process (2b). |
| 4 | **Surface:** a **standalone overlay** for 2a (a Sources panel + a ReadingView), built as a self-contained component so 2b can embed `ReadingView` as the Study surface's Read tab. |
| 5 | **Scheduling store:** reading elements get their **own `reading_elements` table** in `learning.db` (reading isn't FSRS), separate from `items`. |

## 3. Scope & non-goals

**In scope (2a):** import a `.txt`/`.md`/`.html` source; render its text projection; resume at `# @progress`; select → verbatim extract (`(from)` anchor + auto `# @id`) into the per-source notes file; reading-element scheduling (resurface / Later / Done); a Sources panel listing reading elements with progress + due.

**Out of scope (later slices):** the Process face and extract scheduling (2b); the unified Study surface + focus switch + interleaved queue (2b); PDF and web-URL adapters (2c); rich HTML/article rendering (2a strips HTML to text for uniform char anchors); audio/video (Phase 3).

## 4. The N4L model

```
sources/kahneman.txt            ← the original (copied in on import)
reading/kahneman.n4l            ← auto-created notes file
  # @source "kahneman.txt"
  # @ref "sources/kahneman.txt"
  # @progress char=4180          ← updated as you read
  :: kahneman _sequence_
  # @id <auto>
  System 1 is fast and automatic            ← extract node (verbatim selection)
    (from) "sources/kahneman.txt char=1200"  ← anchor on a ditto line
  # @id <auto>
  anchoring shifts estimates
    (from) "sources/kahneman.txt char=3400"
```

- **Reading element** = the `# @ref` node; **extract** = a verbatim node under `_sequence_` with a `(from)` ditto line + auto `# @id`.
- **Anchor** = `char=<offset>` into the source's **text projection** (for `.txt`/`.md` the file content; for `.html` the stripped article text). Uniform char offsets → the same anchor scheme works for every text medium and clicking it re-locates the spot.
- All writebacks use markers the parser already supports (`# @ref`/`# @source`/`# @progress`/`# @id`/`_sequence_`); `# @ref`/`# @source` are read by `content` into `Provenance` already.
- **`# @id` generation:** reading-element and extract nodes are *not* study-marked, so Phase 1's `InjectMissingIDs` (study-marked nodes only) does **not** assign their ids. The new writeback ops generate ids explicitly via the existing `content.GenerateShortID()` and write the `# @id` line at creation — both the `# @ref` node (in the scaffold) and each appended extract.

## 5. The flow

1. **Import** (`ImportTextSource(path)`): copy the picked file into `<collectionDir>/sources/` (suffix on name collision), auto-create `<collectionDir>/reading/<name>.n4l` with the scaffold in §4, register a reading element in `learning.db` keyed by the `# @ref` node's `# @id`. Returns the notes path + ref node id; the frontend opens the ReadingView.
2. **Read** (ReadingView): the source pane renders the text projection (selectable) beside the notes editor (the reused CodeMirror editor on `reading/<name>.n4l`), scrolled to `# @progress`.
3. **Extract**: select text → `CreateExtract(notesPath, refPath, text, anchorChar)` appends a `_sequence_` node (verbatim text + `(from) "<refPath> char=<anchorChar>"` + auto `# @id`) and bumps `# @progress` to the current read position; the notes editor silently reloads (clean-buffer only, per the Phase-1 invariant).
4. **Leave / Later / Done**: `RescheduleReading(refNodeId, action)` reschedules the reading element (§6); `UpdateReadingProgress` persists position.

## 6. Reading-element scheduling (machinery hidden; sane defaults)

- State per reading element: `position` (anchor string), `due` (unix), `interval_days` (starts 1), `last_read`, `done`.
- **Continue/leave:** `interval_days = min(round(interval_days × 1.5), 30)`; `due = now + interval_days`.
- **Later:** push out one step immediately (apply the growth without reading).
- **Done:** `done = 1`; never resurfaces (but the notes file + extracts remain).
- The Sources panel shows "N due to read" (`due <= now AND done = 0`). This queue stays separate from the Phase-1 item queue until 2b unifies them.

## 7. Architecture / components

| Unit | Responsibility | New/changed |
|---|---|---|
| `internal/source/` | `Projection(path) (text string, err error)` + a per-extension dispatch (`.txt`/`.md` passthrough; `.html` → stripped article text via a pure-Go HTML-to-text). The medium-agnostic seed; PDF/web slot in behind it later. | **new** |
| `internal/learning/` | Schema **v2** migration adding `reading_elements`; reading-element CRUD + `ScheduleReading(prev, action, now)`. Existing `items`/`review_logs` untouched. | extend |
| `internal/content/` | Two writeback ops: **create reading notes file** (scaffold incl. a `GenerateShortID()` `# @id` on the `# @ref` node) and **append extract node** (`_sequence_` node + `(from)` + an explicit `GenerateShortID()` `# @id`). Reuse `UpdateProgress`, `atomicWriteText`, `GenerateShortID`. | extend |
| `reading_methods.go` (repo root) | Wails bindings: `ImportTextSource`, `GetReadingList`, `CreateExtract`, `UpdateReadingProgress`, `RescheduleReading`. `App` opens `reading_elements` via the existing `learnStore` lifecycle. | **new** |
| `frontend` `ReadingView.svelte` | Read face: source pane (render projection, selection→anchor, Extract) + notes editor (reuse existing editor). Self-contained for 2b reuse. | **new** |
| `frontend` `SourcesPanel.svelte` + `stores/reading.js` | Sources list (name · % progress · due) + Import button; opens ReadingView. Toggled like the explorer/study overlays. | **new** |

**Pure-Go constraint:** the `.html`→text step must be pure Go (e.g. `golang.org/x/net/html` tokenizer — already an indirect dep — to strip tags; no CGo). No new CGo deps.

## 8. `learning.db` schema v2

```sql
CREATE TABLE reading_elements (
  id            TEXT PRIMARY KEY,   -- the # @ref node's # @id
  ref_path      TEXT NOT NULL,      -- "sources/x.txt" (relative to collection)
  notes_path    TEXT NOT NULL,      -- absolute path of reading/<name>.n4l
  title         TEXT NOT NULL,      -- # @source value (display)
  position      TEXT,               -- last read anchor, e.g. "char=4180"
  proj_len      INTEGER,            -- projection length (for % progress)
  due           INTEGER,            -- unix seconds; NULL = due now
  interval_days INTEGER NOT NULL DEFAULT 1,
  last_read     INTEGER,
  done          INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL
);
CREATE INDEX idx_reading_due ON reading_elements(due) WHERE done = 0;
```

`schema_meta.version` advances 1→2; the v2 step only `CREATE TABLE IF NOT EXISTS`s the new table (additive, safe on existing DBs).

## 9. Error handling / edge cases

- **Duplicate import name:** suffix the copied source and its notes file (`name-2.txt`, `reading/name-2.n4l`); never overwrite.
- **Source file missing/edited after extracts exist:** anchors are best-effort; a missing `sources/` file renders a "source unavailable" state, notes/extracts remain intact and editable.
- **`# @progress`/extract writeback vs a dirty notes buffer:** reuse the Phase-1 invariant — never clobber unsaved edits; silent-reload only when the buffer is clean, else fall back to the external-modification banner.
- **`.html` with no extractable text:** projection is empty; the reader shows an empty state; import still succeeds.
- **Schema v2 on an existing v1 DB:** additive migration; no data loss; idempotent re-open.
- **Anchor past end of projection** (source shrank): clamp to projection length when resolving/scrolling.

## 10. Testing

- **Go unit:** `source.Projection` (txt passthrough; markdown passthrough; html→stripped text); `reading_elements` schema create/migrate-from-v1, CRUD, and `ScheduleReading` (interval growth + cap, Later, Done, due query); the two content writeback ops (notes-file scaffold; append extract → correct `_sequence_` node, `(from) "...char=N"`, and an auto `# @id`); name-collision suffixing.
- **Manual quickstart matrix:** import a `.txt` → ReadingView opens; select → an extract node with the right `(from)` anchor lands under `_sequence_` and `# @progress` updates; close + reopen resumes at `# @progress`; Later pushes the due date; Done removes it from the due list; import a `.html` → projection is readable stripped text and char anchors resolve.

## 11. Deferred to 2b/2c (recorded)

- Process face + extract scheduling + the unified Study surface with the focus switch and interleaved queue (2b).
- PDF + web-URL adapters; explorer "Study this" bridge (2c).
- Rich HTML/article rendering; relocation by stored text snippet (in addition to char offset) for robustness against source edits.
