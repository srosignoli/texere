# Processing Notebook — Slice 1: Shell + Produce-N4L Loop

**Date:** 2026-07-22
**Status:** Approved design
**Feature area:** Incremental reading / extract processing
**Slice:** 1 of ~6 (see "Decomposition" below)

## Summary

When an extract surfaces from the processing queue, the goal is not just to make
cards — it is to *reason* about the extract, connect it to existing knowledge,
and formulate proper N4L notes with relationship arrows. This feature adds a
split-screen Process view: the existing N4L source editor on the left, and a
notebook-style reasoning workspace on the right. **The notebook is working
memory: everything in it exists to produce N4L that gets pushed into the
editor.**

Slice 1 delivers the skeleton that makes every later cell type a pluggable
component, and it delivers standalone value on its own: a persistent reasoning
workspace that produces N4L with provenance, carrying zero LLM / canvas / file-
watcher risk.

## Decomposition (context for this slice)

The full "Processing Notebook" feature is too large for one spec. It is cut into
~6 vertical slices, each with its own spec → plan → implementation cycle:

1. **Notebook shell + produce-N4L loop** — *this spec*.
2. LLM cell (text): Anthropic API + local endpoints, streaming, ask + tutor
   (Feynman / Wozniak) modes; retires the `TutorTab` placeholder.
3. Graph query cell + Compute cell (sandboxed JS Web Worker, math.js, Chart.js,
   `graph.query` / `graph.orbit` bindings).
4. Sketch cell (perfect-freehand, Pointer Events, pressure, palm rejection).
5. Image cell + watched inbox (fsnotify + debounce, rotate/crop, on-disk assets).
6. Multimodal + glue (attach sketch/image to LLM, transcription, push-as-image,
   feed-forward leech reformulation, distillation nudge, `\notebooks` search).

Slices 2–6 are explicitly **out of scope** here.

## Decisions made during brainstorming

These framed the slice and should not be silently reopened during
implementation:

- **Disposition model is unchanged.** The left pane keeps today's editor and its
  `Make card` / `Cloze` / `Later` / `Dismiss` buttons and the `PromoteExtract`
  flow exactly as they are. The notebook is a *reasoning + push-to-source*
  surface bolted onto the right. The queue-bar / "done" redesign is deferred to
  a later slice.
- **Context cell is full except PDF page-text.** It shows extract text, effective
  context tags, radius-1 orbit, and the source passage for text/web sources. PDF
  sources show the `(from)` anchor plus an "open source" affordance but no
  extracted page text — Go-side PDF text extraction does not exist yet and
  becomes its own later slice (needs a pure-Go PDF text library plus a
  Complexity Tracking entry).
- **Reopen reuses the split view; manual "process any node" is deferred.**
  Clicking a provenance dot opens the same Process split surface (left editor
  scrolled to the node, right pane showing the saved notebook **read-only** with
  a **Resume** button). New notebooks are created only via the extract queue in
  this slice.

## Scope

**In scope**

- Split Process view: left = today's editor (unchanged), right = notebook.
- SQLite `notebooks` + `notebook_links` tables (schema v5).
- Debounced autosave (no save button).
- **Context cell** (auto-generated, pinned, read-only): extract text, effective
  context tags, radius-1 orbit, text/web source passage.
- **N4L draft cell**: CodeMirror 6 with N4L highlighting + live validation
  against the SSTconfig arrow registry (unregistered arrows get an amber
  underline with a quick-fix).
- **Push-to-source**: insert cell content at the left editor's cursor and record
  a provenance link.
- **Purple gutter dots** on nodes with linked notebooks (main SourceTab editor
  and the Process editor), clicking reopens the notebook read-only + Resume.

**Out of scope (later slices)**

- All other cell types: LLM, graph query, compute, sketch, image.
- Queue-bar "done" redesign (existing disposition buttons stay).
- PDF page-text extraction.
- Manual "process any node" entry point.
- `\notebooks` search scope, feed-forward leech reformulation, push-as-image,
  distillation nudge.

## Architecture & component boundaries

Refactor `ProcessView.svelte` into a thin **container** (splitter + shared
state) hosting two children:

```
ProcessView (container: Splitter, holds left-editor `view` ref + extractId + mode)
├── ProcessEditor   (existing left pane, lifted out ~verbatim; exposes insertAtCursor(text))
└── NotebookPane    (right pane; owns notebook state + autosave)
    ├── ContextCell     (pinned index 0, generated, read-only)
    └── N4LDraftCell[]  (CodeMirror via shared n4l-editor.js; Push button)
```

**Why a container** (approach chosen): the two panes must share the left
editor's CodeMirror `view` handle, because push-to-source targets its cursor. A
container makes that handle an explicit prop rather than a global. The
alternative — keep `ProcessView` monolithic and mount the notebook as a sibling
overlay — was rejected for hiding that coupling behind shared module state.

Cell rendering goes through a small `cellRegistry` (cell `type` → Svelte
component) so later slices add a cell type without editing `NotebookPane`. In
Slice 1 the registry holds `context` and `n4l`.

Reused pieces: `Splitter.svelte`, `n4lBaseExtensions` / shared `n4l-editor.js`
setup, `workbenchStore.arrowDefs`.

## Data model — `learning.db` schema v5

Follows the existing versioned-DDL migration pattern in
`internal/learning/db.go` (`ddlV5`, `schemaVersion = 5`).

```sql
CREATE TABLE IF NOT EXISTS notebooks (
  notebook_id     TEXT PRIMARY KEY,
  extract_node_id TEXT NOT NULL,           -- node processed when created
  chapter         TEXT NOT NULL DEFAULT '',
  context_tags    TEXT NOT NULL DEFAULT '', -- JSON array of tag strings
  cells_json      TEXT NOT NULL DEFAULT '[]',
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notebooks_node ON notebooks(extract_node_id);

CREATE TABLE IF NOT EXISTS notebook_links (
  node_id     TEXT NOT NULL,               -- a node the notebook pushed
  notebook_id TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  PRIMARY KEY (node_id, notebook_id)
);
CREATE INDEX IF NOT EXISTS idx_nblinks_node ON notebook_links(node_id);
```

**Lifecycle:** one notebook per `extract_node_id`. Opening an extract loads its
existing notebook or creates a fresh one, so a deferred-then-resumed extract
returns to the same notebook.

**What `cells_json` persists:** only *user* cells (draft cells) — a JSON array of
`{ "id": "...", "type": "n4l", "text": "..." }`. The context cell is **derived**
and recomputed on every open (keeps orbit/source fresh and keeps the row small).
It is always rendered as index 0 and never serialized.

**Notebooks are lab notebooks, not knowledge.** They are not in the knowledge
graph, not scheduled, not studied, and (like the rest of `learning.db`) not
committed to git.

## Backend methods (Wails bindings, `app.go` / a new `notebook_methods.go`)

- `OpenNotebook(nodeID string) → (*NotebookView, error)` — loads or creates the
  row and computes the fresh context payload (see below). Target < 300ms.
- `SaveNotebook(notebookID, cellsJSON string) → error` — upsert `cells_json` +
  `updated_at`. Called debounced from the pane.
- `RecordPush(notebookID, filePath string, fromLine, toLine int) → ([]string, error)`
  — after the frontend inserts text and `SaveFile`s, re-parse that line range,
  resolve the node ID(s) it declares (matching arrow targets + dittos, per the
  explorer's node→line rule — not just subject nodes), insert `notebook_links`
  rows, and emit `notebook:links-changed{filePath}`.
- `GetNotebookLinks(filePath string) → (map[string][]string, error)` — node ID →
  notebook IDs, for gutter rendering.
- `GetNotebook(notebookID string) → (*NotebookView, error)` — for read-only
  reopen.

`NotebookView` carries the persisted user cells plus the freshly-computed
context payload (extract text, tags, orbit entries, resolved source passage or
anchor).

## Context cell assembly & the 300ms budget

On open:

1. Reuse `GetExtractTarget(nodeID)` — already yields file path, line, extract
   text, and `(from)` anchor.
2. Run `SearchGraph` on the node text to get the **radius-1 orbit** (typed
   neighbors). If the BadgerDB graph is unbuilt, the orbit is empty and the cell
   shows a quiet "Build graph for neighbors" hint — never an error.
3. If a `(from)` anchor exists and the source is text/web, resolve the passage
   via `source.Projection` + anchor match. For PDF sources, show the anchor and
   an "open source" link only (no page text in this slice).

Rendered as read-only labeled subsections: **Extract / Tags / Orbit / Source**.
Every input degrades gracefully when missing.

## N4L draft cell & push-to-source

- Editor: CodeMirror 6 via `n4lBaseExtensions` (highlighting + arrow
  autocomplete), plus a **lint** pass that flags unregistered arrows (checked
  against `workbenchStore.arrowDefs`) with an amber underline and a quick-fix
  (register the arrow, or replace it).
- **Push** button: the frontend inserts the cell text at the left editor's
  cursor, records the inserted line range, calls the existing `SaveFile`, then
  calls `RecordPush(notebookID, filePath, fromLine, toLine)` to write provenance
  rows and trigger a gutter refresh.
- Draft cell content **remains** in the notebook after pushing (it is a lab
  record, not consumed).

## Persistence / autosave

Notebook state is component-owned in `NotebookPane`. A **debounced** (~600ms)
`SaveNotebook` fires on any cell edit / insert / delete / reorder — there is no
save button. Cell insertion in Slice 1 is limited to draft cells (an "add draft
cell" button; the `/ \ ``` ~` keystroke inserts arrive with their cell types in
later slices). Reorder is an array move; delete removes the cell. The pinned
context cell (index 0) is never reordered, deleted, or serialized.

Autosave writes plain JSON text; the rAF stroke-batching concern noted for the
sketch slice does not apply here.

## Provenance dots & reopen

- A CodeMirror **gutter** extension added to the shared `n4l-editor.js` setup —
  so both the main SourceTab editor and the Process editor get it — renders a
  purple dot on any line whose node has ≥1 link. Data comes from
  `GetNotebookLinks(filePath)`, refreshed on the `notebook:links-changed` event.
- Clicking a dot opens the Process split for that file/node with the notebook
  **read-only**. A **Resume** button flips it editable. When reopened outside the
  live queue flow, the surface is a pure viewer — the disposition buttons are not
  wired for non-queue reopen in this slice.

## Error handling & edge cases

- **Graph unbuilt** → orbit section shows a quiet "Build graph for neighbors"
  hint, not an error.
- **Source file missing/moved** → passage section falls back to the raw anchor.
- **Node text changed after promotion** (fnv-hashed ID shifts when
  `SeedBasicCardNode` rewrites the line and drops the `(from)` anchor) → the old
  notebook orphans gracefully: still reachable through its `notebook_links`; dot
  resolution simply finds no matching line and skips it.
- **Concurrent external file edit during push** → covered by the existing
  self-write guard (`beginSelfWrite` / `endSelfWrite`).

## Testing

- **Go unit tests:** CRUD for `notebooks` and `notebook_links`; `RecordPush`
  node-ID resolution including arrow-target and ditto cases; context assembly
  with graph-built vs not, and text / web / PDF / no-source variants.
- **Frontend:** a manual test plan document in the project's established style
  (the incremental-reading manual test plan is the template). No new e2e harness.

## Success criteria (Slice 1)

- Opening an extract for processing shows the split view with the context cell
  populated (extract + orbit + text/web source passage) in **under 300ms**.
- Draft cells can be created, edited, reordered, and deleted; notebook state
  survives app restart.
- "Push to source" inserts valid N4L at the editor cursor and records
  `notebook_links` rows.
- Purple gutter dots appear on linked nodes and reopen the correct notebook
  read-only, with Resume making it editable.
- Notebook JSON for a typical text-only Slice-1 session stays well under 20KB in
  SQLite.

## Dependencies

Existing: extract queue (`GetExtractQueue` / `GetExtractTarget`), N4L editor
(`n4l-editor.js`, `Splitter.svelte`), SSTorytime-KV search API (`SearchGraph`),
SSTconfig arrow registry (`workbenchStore.arrowDefs`), SQLite (`learning.db`),
Wails bindings, self-write guard. No new third-party dependencies in this slice.
