# Notebook Cell Chrome Consolidation — Manual Test Plan

Prereqs: open a collection with a processing notebook (extract) that can be
edited; be able to reopen the same notebook **read-only** (e.g. via history or
a search result opened read-only) for TC-6 and TC-12. No automated coverage
exists for this slice — it moved markup and CSS, added no pure logic — so this
plan is the verification gate.

**Status: all twelve cases below are deferred to human verification** — this
agent has no GUI and cannot exercise `make dev`. Every EXPECT was instead
checked directly against the committed source: `frontend/src/components/
notebook/NotebookPane.svelte`, `ComputeCell.svelte`, `AudioCell.svelte`,
`MediaCell.svelte`, `ImageCell.svelte`, `SketchCell.svelte`, and
`frontend/src/lib/cell-label.js`.

## TC-1 Header layout, all eight types

1. Add one cell of each of `+ N4L draft cell`, `+ LLM cell`, `+ Graph cell`,
   `+ Compute cell`, `+ Sketch cell`, `+ Scan`, `+ Audio cell`, `+ Media`
   (the eight `.nb-add` buttons in `NotebookPane.svelte`).
   - EXPECT: every cell's header (`.nb-cellhead`) shows its uppercase type
     name on the left (`.nb-cellname`, e.g. "N4L DRAFT", "SKETCH", "SCAN" —
     `.nb-cellhead`'s `text-transform: uppercase` applied to `cellLabel(cell)`
     from `frontend/src/lib/cell-label.js`), then `▲ ▼` (`.nb-reorder`), then a
     visible gap, then `✕` (`.nb-del`), all inside the same flex row.
   - EXPECT: the gap between the `.nb-reorder` block and `.nb-del` (14px
     `margin-left` on `.nb-del`) is clearly wider than the 2px gap between `▲`
     and `▼` inside `.nb-reorder` — this gap is the sole mis-click guard
     between reorder and delete.
   - EXPECT: no cell has a gutter column to its left — `.nb-reorder` no longer
     exists as a separate left-hand column; there is nothing between the pane
     edge and the header/cell content.

## TC-2 Cells are wider

1. Compare a notebook on this branch against the same notebook on `main`.
   - EXPECT: each cell's content (code textarea, sketch canvas, transcript,
     etc.) starts flush with the header's left edge — the old `.nb-reorder`
     gutter column and the old flex `.nb-cellwrap`/`.nb-cell` split are gone,
     so cells reclaim that horizontal space rather than being indented past a
     left-hand button column.

## TC-3 End-of-list buttons

1. With three or more cells, look at the first cell's header.
   - EXPECT: `▲` is dimmed (`.nb-reorder button:disabled { opacity: .35 }`)
     and does nothing when clicked (`disabled={i === 0}` in
     `NotebookPane.svelte`).
2. Look at the last cell's header.
   - EXPECT: `▼` is dimmed and inert (`disabled={i === cells.length - 1}`).
3. Delete down to a single cell.
   - EXPECT: both `▲` and `▼` are dimmed and inert on that one remaining cell.

## TC-4 Reorder

1. With at least three cells including a Sketch cell with some strokes drawn
   and an LLM cell with a transcript, click `▲` on a middle cell, then `▼` to
   move it back.
   - EXPECT: the cell moves position; its header name (`.nb-cellname`) moves
     with it — the header lives inside the same per-cell `{#each cells as
     cell, i (cell.id)}` block as the cell body, keyed on `cell.id`, so header
     and content never separate.
   - EXPECT: the sketch's strokes and the LLM's transcript are still present
     after the move — `move(i, dir)` swaps entries in the `cells` array itself
     rather than remounting components by index, so content is preserved, not
     reset to empty.
2. Then click `✕` on the moved cell.
   - EXPECT: the moved cell is the one removed, not the cell now occupying its
     old position — Svelte's keyed `{#each cells as cell, i (cell.id)}` block
     re-derives `i` when `move()` reassigns `cells`, so the `deleteCell(i)`
     bound to that header always targets the cell it is currently drawn on,
     not the index it was created with.

## TC-5 Delete, all eight types

1. For each of the eight cell types from TC-1, click that cell's header `✕`.
   - EXPECT: the cell is removed (`deleteCell(i)` filters it out of `cells`).
2. Before deleting, inspect each cell's own body/toolbar.
   - EXPECT: no cell shows a second `✕` delete control anywhere in its own
     bar — the per-cell delete buttons and their `onDelete` props were removed
     from all eight components in a prior task. (`ImageCell`'s `✕ Cancel` crop
     button is a distinct control, covered separately in TC-10.)

## TC-6 Read-only notebook

1. Reopen a notebook read-only (all eight cell types present).
   - EXPECT: every header shows only the cell name — no `▲ ▼` and no `✕` —
     because the whole reorder/delete block in `NotebookPane.svelte` sits
     behind a single `{#if !readOnly}`.
2. Look specifically at `N4LDraftCell`, `ImageCell` and `SketchCell`.
   - EXPECT: each of these three also hides its own in-cell toolbar entirely
     in read-only — `N4LDraftCell`'s `.draft-bar` (the "→ Push to source"
     button; the `.draft-editor` CodeMirror mount itself stays rendered but
     becomes non-editable via `EditorView.editable.of(!readOnly)`, rather than
     being hidden), `ImageCell`'s `.im-bar`, and `SketchCell`'s `.sk-bar`, are
     each behind their own `{#if !readOnly}` — so neither the header nor any
     cell's own toolbar offers a control, even though the draft cell's text
     area remains visible (read-only) underneath.

## TC-7 Empty-bar suppression, Audio

1. Add `+ Audio cell` and do not record anything.
   - EXPECT: no `.au-bar` and no horizontal rule appears between the header
     and the record/import controls — `.au-bar` is gated on `{#if
     cell.asset}` in `AudioCell.svelte`, and an unrecorded cell has no asset.
2. Record (or import) audio.
   - EXPECT: `.au-bar` now appears, showing the duration (`.au-dur`) on the
     left and, in an editable notebook, "Replace" right-aligned after
     `.au-spacer`.

## TC-8 Empty-bar suppression, Media

1. Add `+ Media` and do not pick anything.
   - EXPECT: no `.me-bar` appears above the search panel — `.me-bar` is gated
     on `{#if cell.license || (!readOnly && hasContent(cell))}` in
     `MediaCell.svelte`, and a fresh cell has neither.
2. Pick a licensed Wikimedia Commons asset.
   - EXPECT: `.me-bar` now appears with the licence text (`.me-lic`) on the
     left and "Replace" right-aligned after `.me-spacer`.
3. Reopen the same notebook read-only.
   - EXPECT: `.me-bar` still shows, with the licence text still present but no
     "Replace" button — `hasContent(cell) && !readOnly` gates Replace, but
     `cell.license` alone keeps the bar itself visible.

## TC-9 Compute label and bar (editable)

1. Add `+ Compute cell` in an editable notebook.
   - EXPECT: the header reads `COMPUTE · JS` (uppercased `cellLabel`, which
     appends `cell.language` as the detail for a compute cell) — `js` appears
     exactly once; `ComputeCell.svelte` no longer has its own `.cc-lang` span.
   - EXPECT: `.cc-bar` is present above the code textarea, and "Run" (or
     "Stop" while running) is right-aligned after `.cc-spacer` — the bar
     itself is not doubled or removed, only the redundant language label was.

## TC-10 Image crop cancel survives

1. Add `+ Scan`, import an image, click "⛶ Crop" to enter crop mode.
   - EXPECT: the button that was "⛶ Crop" now reads "✕ Cancel" and clicking
     it exits crop mode (`cancelCrop`) — this button lives in `ImageCell.svelte`'s
     `.im-bar` and was deliberately left alone by the delete-button sweep,
     which only touched each cell's dedicated delete/`onDelete` wiring, not
     this crop-specific control.

## TC-11 Persistence

1. Reorder two cells and delete one, wait past the 600ms save debounce
   (`scheduleSave` in `NotebookPane.svelte`), then close and reopen the
   notebook (or restart the app).
   - EXPECT: the new order and the deletion both persisted — reordering and
     deleting write into the same `cells` array that `scheduleSave` debounces
     and writes via `SaveNotebook`.

## TC-12 Read-only Compute cell — no bar at all

Task 3's review caught that removing `.cc-lang` would have left `.cc-bar`
rendering nothing but an empty `.cc-spacer` in a read-only notebook — the same
kind of stub artifact this whole slice removes elsewhere. The fix gates the
entire bar on `{#if !readOnly}` in `ComputeCell.svelte`.

1. Add a Compute cell, then reopen the notebook read-only.
   - EXPECT: **no bar at all** appears above the code area — no `.cc-bar`
     element, no padding strip, no horizontal rule; the code textarea is the
     first thing rendered in `.cc-cell`.
2. Reopen the same notebook editable (or look at an editable Compute cell).
   - EXPECT: `.cc-bar` is present and "Run" (or "Stop") is right-aligned after
     `.cc-spacer`, exactly as in TC-9 — the bar is suppressed only in
     read-only, not removed altogether.
