# Processing Notebook — Slice 4: Sketch cell

**Date:** 2026-07-23
**Status:** Approved design
**Feature area:** Processing Notebook (incremental reading)
**Slice:** 4 of ~6 (see the Slice 1 design for the full decomposition)

## Summary

Add a **sketch cell**: a resizable canvas for freehand drawing and handwriting
via the Pointer Events API. Pen input draws by default (palm rejection); a
toggle enables mouse drawing for users without a tablet. Strokes are pressure-
enhanced (but never depend on pressure) and rendered with the `perfect-freehand`
library. Tools: pen, highlighter, stroke-eraser, a small color palette,
undo/redo, and clear. Strokes persist as vectors in the notebook's `cells_json`
and re-render on reopen; the cell is fully editable when a notebook is reopened.

This slice is frontend-only, adds one npm dependency (`perfect-freehand`), and
makes no backend changes — the sketch cell persists through the existing
`cellRegistry` / `NotebookPane` / `SaveNotebook` path.

## Decisions made during brainstorming

- **Scope: the sketch cell only.** Attaching a sketch to an LLM cell
  (render→PNG image blocks) and **push-as-image** are Slice 6 (multimodal), NOT
  here.
- **Storage: vector strokes in `cells_json`, no persisted thumbnail.** On
  reopen, strokes re-render onto the cell's canvas. The notebook pane is not
  virtualized, so each canvas stays mounted and scrolling composites natively —
  no PNG thumbnail and no on-disk asset infrastructure (that is Slice 5).
- **Eraser: stroke-erase.** The canvas is a list of vector strokes; the eraser
  deletes whole strokes it passes over (clean undo, perfect re-render). No
  pixel/raster erase.
- **Pressure enhances stroke width but nothing depends on it.** WebKitGTK
  pressure on Linux is unreliable; `perfect-freehand`'s `simulatePressure`
  synthesizes width from velocity when the device reports none.
- **Persist on stroke end, not per point.** Live drawing renders on a
  `requestAnimationFrame` cadence; the completed stroke is committed and
  autosaved on `pointerup`.

## Scope

**In scope**

- A `sketch` cell type in the notebook `cellRegistry`, with a "+ Sketch cell"
  button in `NotebookPane`.
- Pointer-Events drawing: **pen draws by default**, `touch` always ignored (palm
  rejection), a **mouse-draw toggle** additionally allows `mouse`.
- Pressure-enhanced strokes via `perfect-freehand`, with a `simulatePressure`
  fallback so strokes always draw.
- Tools: **pen**, **highlighter**, **stroke-eraser**; a 5-color palette;
  undo/redo; clear.
- A vertically **resizable** canvas whose height persists.
- Vector strokes persisted in `cells_json`; editable and re-rendered on reopen.

**Out of scope (later slices)**

- Attaching a sketch to an LLM cell / rendering the sketch to a PNG image block
  (Slice 6, multimodal).
- Push-as-image: writing the sketch to `media/…png` and inserting an `(image)`
  arrow (Slice 6).
- Shapes, text boxes, layers, infinite canvas, pan/zoom.
- On-disk asset storage / persisted PNG thumbnails (Slice 5 introduces the
  asset directory for images).

## Data model (`cells_json`)

```json
{
  "id": "…",
  "type": "sketch",
  "height": 240,
  "strokes": [
    { "tool": "pen", "color": "#e8eaf0", "size": 3, "points": [[x, y, pressure], …] },
    { "tool": "highlighter", "color": "#f59e0b", "size": 14, "points": [ … ] }
  ]
}
```

- `strokes` — the source of truth (ordered, editable). Each stroke records its
  `tool` (`pen` | `highlighter`), `color`, base `size`, and `points`
  (`[x, y, pressure]` in canvas pixel coordinates). The eraser produces no stroke
  — it removes strokes.
- `height` — the resizable canvas height, persisted so the cell reopens at the
  same size. Canvas width tracks the cell's content width.
- **v1 limitation (accepted):** points are absolute canvas pixels, so a large
  width change between sessions does not reflow existing strokes (they may clip).
  Sketches are session artifacts; normalizing coordinates is deferred.

## Architecture & components

```
frontend/src/
├── components/notebook/
│   └── SketchCell.svelte    # NEW: canvas + toolbar + resize; owns strokes/undo/redo, persists
├── lib/
│   ├── sketch-render.js     # NEW: pure — strokes → canvas (perfect-freehand getStroke + fill)
│   └── notebook.js          # register 'sketch'; newSketchCell()
└── components/notebook/NotebookPane.svelte   # "+ Sketch cell" button
```

- **`sketch-render.js`** is pure and testable-in-isolation: `renderStrokes(ctx,
  strokes, { width, height })` clears the canvas and fills each stroke's
  `perfect-freehand` outline (pen = solid; highlighter = semi-transparent +
  `multiply` composite). Keeping render logic out of the component makes the
  hot path (live redraw during a stroke) easy to reason about.
- **`SketchCell.svelte`** owns interaction (Pointer Events), the undo/redo
  stacks, the toolbar, and persistence; it calls `renderStrokes` for both the
  live redraw and the reopen render.

The cell renders through the existing `cellRegistry`, so `NotebookPane` passes
the same shared props (`cell`, `readOnly`, `context`, `cells`, `onChange`,
`onDelete`) and routes `onChange(cell)` (object) to `updateCellObj`.

## Input — Pointer Events

- Handlers on the canvas: `pointerdown`, `pointermove`, `pointerup`,
  `pointerleave`, `pointercancel`.
- **Which pointers draw:** `pen` always; `mouse` only when the mouse-draw toggle
  is on; `touch` **never** (palm rejection). Determined by `event.pointerType`.
- On `pointerdown` (for an accepted pointer): `setPointerCapture(pointerId)`,
  start a new stroke (pen/highlighter) or begin an erase pass; capture the point.
- On `pointermove`: append `[offsetX, offsetY, event.pressure]`; **schedule a
  redraw on `requestAnimationFrame`** (coalesce multiple moves into one repaint)
  rather than redrawing per event. For the eraser, hit-test strokes near the
  pointer and mark them for removal.
- On `pointerup` / `pointerleave` / `pointercancel`: finalize.
  - pen/highlighter → push the completed stroke; record an `add` op.
  - eraser → remove the hit strokes; record an `erase` op with the removed
    strokes (for undo).
  - Then persist via `onChange(cell)` (once per completed action, not per move).
- **Pressure:** `event.pressure` is 0–1 (often `0.5` when the device has no real
  pressure, `0` on some Linux/WebKitGTK cases). Pass points to `perfect-freehand`
  with `simulatePressure: true` so width is synthesized from velocity when
  pressure is flat, and clamp a minimum width so a stroke always renders.

## Rendering (`sketch-render.js`)

`renderStrokes(ctx, strokes, { width, height, dpr })`:

- Clear the canvas.
- For each stroke: `const outline = getStroke(points, { size, thinning,
  smoothing, streamline, simulatePressure: true })`; build a `Path2D` from the
  outline polygon; `ctx.fillStyle = color`; `ctx.fill(path)`.
- **Highlighter:** `ctx.globalAlpha` ~0.35 and `ctx.globalCompositeOperation =
  'multiply'` for the highlighter strokes (reset to `source-over`/alpha 1 after).
- Handle device-pixel-ratio: size the canvas backing store to `cssWidth*dpr ×
  cssHeight*dpr` and scale the context, so strokes are crisp on HiDPI.
- A single point (tap) yields a small dot — `perfect-freehand` handles the
  degenerate case.

Live drawing re-renders the whole stroke list each `requestAnimationFrame` frame
(fast for typical sketches); on reopen, one render of `cell.strokes`.

## Tools & controls

A compact toolbar above the canvas:

- **Pen** / **Highlighter** / **Eraser** (mutually exclusive tool selection).
- **Colors:** a 5-swatch palette — white `#e8eaf0`, amber `#f59e0b`, cyan
  `#7dd3fc`, red `#ff6b6b`, green `#10b981` (dark-theme friendly).
- **Size:** per-tool defaults (pen `3`, highlighter `14`); a small slider adjusts
  the current tool's size.
- **Undo / Redo:** an operation stack — each op is `add` (a stroke),
  `erase` (removed strokes), or `clear` (all strokes), invertible; redo replays.
  Bounded to a reasonable depth (e.g. 100).
- **Clear:** removes all strokes (recorded as a `clear` op, undoable).
- **Mouse-draw toggle:** allows `mouse` pointers to draw (off by default;
  pen-only).

No shapes, text, layers, or infinite canvas in v1.

## Resize

The canvas sits in a wrapper with CSS `resize: vertical`. A `ResizeObserver` on
the wrapper updates the canvas backing size (accounting for `devicePixelRatio`)
and re-renders `strokes` on height change; the new `height` is written to the
cell and persisted via `onChange(cell)`. Width tracks the cell content width and
also triggers a re-render on change.

## Persistence

- Strokes and `height` live in the cell object; `onChange(cell)` runs on each
  completed action (stroke end, erase, clear, undo/redo, resize) and flows
  through `NotebookPane`'s `updateCellObj` → debounced `SaveNotebook(cells_json)`.
- **Never** persist per `pointermove`.
- No PNG thumbnail, no on-disk assets. On reopen, `SketchCell` renders
  `cell.strokes` to its canvas once at mount.
- Sketches can make a notebook's `cells_json` larger than the text-only target;
  this is expected for a drawing cell and acceptable in SQLite. Autosave writes
  the whole `cells_json` blob (existing Slice-1 behavior); persisting per
  completed action (not per point) keeps the write frequency reasonable.

## Not wired to LLM attach (yet)

Sketch cells are **not** added to the LLM cell's attach picker in this slice —
there is no text `resultText` to send, and multimodal image attachment is Slice
6. Slice 6 will render the sketch canvas to a PNG and send it as an image block.

## Dependencies

New **npm** dependency: **`perfect-freehand`** (MIT, framework-agnostic, pure
JS). No new Go dependencies, no backend changes. `perfect-freehand` exports
`getStroke(points, options)`; the compute worker / Vite bundling from earlier
slices is unaffected.

## Error handling

- Empty/missing `strokes` → a blank canvas, no crash.
- A single-point tap → a dot (handled by `perfect-freehand`).
- No device pressure → `simulatePressure` + a minimum width; strokes always draw.
- Rejected pointer types (`touch`, and `mouse` when the toggle is off) → no-op.
- Resizing smaller doesn't delete strokes (they persist in full; only the visible
  area shrinks).

## Testing

- No JS test runner in this project. Verification is `cd frontend && npm run
  build` plus a manual test plan (Slice 1/2/3 style):
  - draw with the mouse-draw toggle on; confirm pen-only mode ignores mouse and
    touch;
  - pen / highlighter / eraser + each color; eraser removes whole strokes;
  - undo / redo / clear;
  - resize the canvas taller/shorter;
  - **reopen persistence:** restart the app, reopen the extract, confirm the
    sketch re-renders and is still editable.
  - Pen-hardware pressure behavior is a manual check on a tablet (Windows Ink /
    WebView2 shows pressure variation; Linux/WebKitGTK degrades to
    `simulatePressure`).
- `sketch-render.js` is a pure function; if a lightweight check is ever added, it
  can be exercised against a canvas mock, but no test harness is introduced here.

## Success criteria

- Freehand drawing works with the mouse-draw toggle on (and with a pen on a
  tablet); pen-only mode rejects mouse and touch.
- Stroke width varies with pressure where available, and strokes always render
  where it is not (`simulatePressure` fallback).
- Pen, highlighter, and stroke-eraser work; the 5-color palette and size control
  work; undo/redo/clear work.
- The canvas is vertically resizable and its height persists.
- Strokes persist in `cells_json` and re-render, still editable, after an app
  restart.

## Dependencies (existing, reused)

Slice 1's `cellRegistry` / `NotebookPane` / `cells_json` persistence and the
`onChange(cell) → updateCellObj → SaveNotebook` path; Wails bindings; Vite
bundling. New: `perfect-freehand` (npm).
