# Sketch cell — Expand mode + resolution-independent strokes — Manual Test Plan

Prereqs: open an extract for processing; the "mouse/pen" toggle is ON by default.

## TC-1 Expand fills the window
1. Add a Sketch cell; click "⛶ Expand".
   - EXPECT: the sketch fills the entire texere window; toolbar floats at the top; no inline resize handle.

## TC-2 Full-tablet drawing (Expand-first)
1. With an EMPTY sketch, Expand, then draw touching all edges of the drawing area.
   - EXPECT: strokes appear under the cursor across the whole surface (the whole tablet maps to the canvas; no dead bands).

## TC-3 Collapse shows the whole drawing (no clip)
1. After TC-2, press Esc (or click "🗕 Collapse").
   - EXPECT: returns to inline at the saved height; the entire drawing is visible, scaled to fit (letterboxed), not clipped to the top-left.
2. Re-Expand.
   - EXPECT: strokes reappear at the same relative positions.

## TC-4 No distortion (draw inline, then expand)
1. In a fresh sketch, draw a rough circle inline; then Expand.
   - EXPECT: the circle stays a circle (uniform scale, letterboxed) — it does not stretch into an oval.

## TC-5 Tools work in both modes
1. Pen/highlighter/eraser, colors, size slider, undo/redo, clear — try each inline AND expanded.
   - EXPECT: all behave the same; highlighter overlaps darken (multiply); eraser removes whole strokes.

## TC-6 Height is unaffected by expanding
1. Drag the inline resize handle to a specific height; Expand; Collapse.
   - EXPECT: the cell returns to exactly that height. Expanding never changes the saved height.

## TC-7 Persistence across restart
1. Draw (some strokes made inline, some in Expand), Collapse, restart the app, reopen the extract.
   - EXPECT: the sketch re-renders correctly at the inline size and is still editable.

## TC-8 Legacy sketch (if any pre-existing sketch exists)
1. Open a notebook whose sketch was drawn before this change.
   - EXPECT: it still renders (identity transform / old absolute-pixel behavior); no crash.
