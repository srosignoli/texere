# Notebook cell folding — manual test plan

**Design:** `2026-07-29-notebook-cell-folding-design.md`
**Build:** `make dev`

Unit coverage: `frontend/src/lib/cell-preview.test.js` via `make test-js`
covers `cellPreview` and `formatDuration` exhaustively. Everything below is what
that cannot reach.

## TC-1 Every stacked type folds and previews

For each of the eight stacked types — N4L draft, LLM, Graph, Compute, Sketch,
Scan, Audio, Media — add a cell, give it content, and fold it.

**Expect:** the header reads `▸ NAME`, the body disappears, and one muted line
appears beneath the header showing that cell's content: the draft's first line,
the LLM's opening question and message count, the graph query, the first line of
code, the stroke count, the image's pixel dimensions, the recording's duration,
the media title/subject/query. Clicking the chevron **or the name** unfolds it.

## TC-2 An empty cell folds to its header alone

Add a fresh cell of each type and fold it **without** giving it content.

**Expect:** header only. No preview line, no blank row, no `(empty)` placeholder.

## TC-3 Fold survives reopening and restart

Fold two cells in a notebook of five. Close it, reopen it. Then quit the app,
relaunch, and reopen it.

**Expect:** the same two cells are folded both times; the other three are not.

## TC-4 A notebook saved before this change opens unfolded

Open a notebook last saved by a build without folding.

**Expect:** every cell is unfolded and every header shows `▾`. No error toast.

## TC-5 Read-only

Open a notebook read-only (the reopen-by-`notebookId` path).

**Expect:** headers show `▸`/`▾` and the name, and **no** `▲▼` and **no** `✕`.
Folding and Fold all both work. Fold a cell, close, reopen read-only — it is
unfolded again, because read-only does not save.

## TC-6 Folding never destroys work

This is the case the design chose CSS-hiding over unmounting for. All four must
pass.

1. **Compute:** run a cell whose code calls `print(...)` and `chart(...)`. With
   output and a chart visible, fold the cell, then unfold it.
   **Expect:** logs, the `⇒` value and the rendered chart are all still there.
   *(Unmounting would have shown an empty output pane — `chartConfigs` is never
   written to the cell.)*
2. **LLM mid-stream:** send a question and fold the cell **while the answer is
   still streaming**. Wait, then unfold.
   **Expect:** the answer completed and is in the transcript. It was not
   truncated or cancelled.
3. **LLM unsent input:** type into an LLM cell's textarea without pressing Send.
   Fold, unfold.
   **Expect:** the typed text is still in the textarea.
4. **Audio mid-recording:** start recording, fold the cell, wait ~5s, unfold,
   stop.
   **Expect:** still recording when unfolded; the saved clip is the full length,
   and the duration chip agrees with the header preview.

## TC-7 A folded sketch keeps its height

Draw in a sketch cell, note its height, fold it, unfold it.

**Expect:** the drawing returns at its original height and is fully visible, not
collapsed to a sliver. Restart the app, reopen: still the original height.

*(A folded cell is `display: none`, so `SketchCell.onResize` measures
`clientHeight` as 0. The `h &&` truthiness guard is what stops that zero being
saved — this case exists to catch anyone removing it.)*

## TC-8 Fold all

1. In a notebook with cells, click **Fold all**.
   **Expect:** every cell folds; the button becomes **Unfold all**.
2. Click **Unfold all**.
   **Expect:** every cell unfolds; the button becomes **Fold all**.
3. Fold every cell but one by hand, then fold that last one.
   **Expect:** the button flips to **Unfold all** by itself.
4. Reopen a notebook whose cells were all folded.
   **Expect:** the button reads **Unfold all** immediately.
5. Open a notebook with no cells.
   **Expect:** no Fold all button, and `⚙ Models` still sits at the right edge.

## TC-9 The other header controls still work

On a folded cell: click `▲` and `▼`, and click `✕`.

**Expect:** reorder moves the folded cell and it stays folded; delete removes it.
`▲` is still disabled on the first cell and `▼` on the last. The gap between the
`▲▼` pair and `✕` is unchanged.

## TC-10 Audio duration is reported identically in both places

Record a clip of ~1:05 and fold the cell, then unfold it.

**Expect:** the folded preview and the `.au-dur` chip in the unfolded bar show
the same string. *(They now share `formatDuration`.)*
