# Processing Notebook — Slice 1 Manual Test Plan

Prereqs: a collection with at least one reading source that has produced an
extract in the Process queue; run Build Graph (Ctrl+G) once for orbit coverage.

## TC-1 Split view + context cell
1. Open an extract for processing.
   - EXPECT: left = N4L editor scrolled to the extract; right = notebook.
   - EXPECT: context cell shows Extract text; Tags/Neighbors if the graph is
     built; Source passage for a text/web source (or "From: <anchor>" / PDF note).
   - EXPECT: the split appears in well under a second (target < 300ms).

## TC-2 Draft cells CRUD + persistence
1. Click "+ N4L draft cell"; type `Newton (invented) calculus`.
2. Add a second cell; reorder with ▲/▼; delete one.
3. Restart the app; reopen the same extract.
   - EXPECT: the remaining draft cell and its text are still there.

## TC-3 Push to source + provenance dot
1. In a draft cell type `Newton (is a) physicist`; click "Push to source".
   - EXPECT: the line is inserted at the editor cursor and persists on disk.
   - EXPECT: a purple dot appears in the gutter on the pushed node's line.
2. Open the same file in the Source tab.
   - EXPECT: the purple dot appears there too.

## TC-4 Reopen read-only + Resume
1. In the Source tab, click the purple dot.
   - EXPECT: the notebook opens read-only in the overlay (cells not editable).
2. Click "Resume (edit)".
   - EXPECT: cells become editable; edits autosave.
3. Close and reopen the dot.
   - EXPECT: the edit persisted.

## TC-5 Graph-unbuilt degradation
1. On a fresh collection with no graph built, open an extract.
   - EXPECT: Neighbors shows "build the graph" hint, no error.
