# Sketch cell — Expand (fullscreen) mode + resolution-independent strokes

**Date:** 2026-07-24
**Status:** Approved design
**Feature area:** Processing Notebook (incremental reading) — enhancement to Slice 4 (sketch cell)

## Summary

Add an **Expand** mode to the sketch cell: a per-cell toggle that blows the
drawing canvas up to fill the entire texere window (an in-window CSS overlay),
so a pen tablet's full physical surface maps to the drawing area instead of a
small panel. Because expanding changes the canvas size dramatically, strokes
are made **resolution-independent** — stored in a fixed per-sketch *logical*
coordinate space and rendered with a uniform scale-to-fit (letterbox) transform,
so a sketch renders correctly at any size (inline preview or fullscreen) with no
distortion. This retires the Slice-4 "absolute pixels clip on resize" limitation.

Frontend-only. No new npm/Go dependencies. No backend changes.

## Motivation

On Linux/WebKitGTK (and WSLg), an XP-Pen tablet is delivered to the webview as
`pointerType: "mouse"` with absolute, already-screen-mapped coordinates. The
tablet→screen mapping lives in the driver, below the webview, and the app only
receives events while the cursor is over the canvas — so the app cannot bend the
whole tablet onto one small panel. The reliable in-app fix is the inverse: make
the drawing target as large as the mapped area. Filling the window with the
canvas means the whole tablet maps to the drawing surface.

## Decisions made during brainstorming

- **Expand scope: in-window CSS overlay only.** `position: fixed; inset: 0`
  fills the texere window. No Wails `WindowFullscreen` / OS-fullscreen call — pure
  DOM/CSS is guaranteed reliable everywhere (incl. WSLg). The user maximizes the
  window (and/or maps the tablet to the window/monitor) to get the full surface.
- **Coordinates: resolution-independent (logical space + uniform scale-to-fit).**
  Points are stored in a fixed per-sketch logical canvas; rendering uses a uniform
  scale + centering (letterbox). No distortion; the whole sketch always shows.
- **Logical canvas is set lazily on the first stroke**, from whatever container
  is active at that moment. Expand-then-draw ⇒ logical canvas = the window ⇒ the
  whole tablet is live edge-to-edge. Draw-inline-first ⇒ logical = inline size.
  Fixed thereafter (an existing sketch letterboxes when later expanded).
- **Stroke size stored in logical units** as `sliderPx ÷ drawScale`, so the pen
  feels like the slider value while drawing at any zoom and thickness scales
  proportionally with the view.
- **Existing sketches migrate best-effort** (stamp `logicalW/H` on load, keep
  pixel points as-is); throwaway test sketches may look slightly off — accepted.

## Data model (`cells_json`) — changes

```json
{
  "id": "…",
  "type": "sketch",
  "height": 240,
  "logicalW": 900,
  "logicalH": 240,
  "strokes": [
    { "tool": "pen", "color": "#e8eaf0", "size": 3, "points": [[x, y, pressure], …] }
  ]
}
```

- **New:** `logicalW`, `logicalH` — the sketch's fixed logical canvas dimensions.
  `null`/absent on a fresh cell **and on legacy sketches** (not migrated — see
  below); set on the first *new* stroke to the active container's CSS size.
- `points` — `[x, y, pressure]` are now **logical** coordinates (in the
  `logicalW × logicalH` space), not screen pixels. `pressure` unchanged.
- `size` — now a **logical** width (`sliderPx ÷ drawScale` at draw time), rendered
  as `size × scale`.
- `height` — unchanged; the inline cell's CSS height, persisted. **Never** written
  from the fullscreen container size (guarded — see Interaction).

## Architecture & components

```
frontend/src/
├── lib/sketch-render.js         # MODIFY: add fitTransform(); renderStrokes maps logical→screen
└── components/notebook/SketchCell.svelte   # MODIFY: expand state, overlay, logical coords, Esc
```

No new files. `newSketchCell()` in `lib/notebook.js` is **unchanged** — a fresh
cell has no `logicalW`/`logicalH` keys; they are set on the first stroke. Code
must therefore treat missing/falsy `logicalW/H` as "unset", not assume the keys
exist.

### `sketch-render.js`

- **New pure helper** `fitTransform(containerW, containerH, logicalW, logicalH)`
  → `{ scale, ox, oy }` where
  `scale = min(containerW / logicalW, containerH / logicalH)`,
  `ox = (containerW − logicalW × scale) / 2`, `oy = (containerH − logicalH × scale) / 2`.
  A logical point `(lx, ly)` maps to screen `(ox + lx × scale, oy + ly × scale)`;
  the inverse (for `pointOf`) is `lx = (sx − ox) / scale`, `ly = (sy − oy) / scale`.
  Guards: `logicalW/H` falsy or `≤ 0` ⇒ identity `{ scale: 1, ox: 0, oy: 0 }`.
- **`renderStrokes(ctx, strokes, { width, height, dpr, logicalW, logicalH })`** —
  computes `fitTransform`, and for each stroke maps its logical points to screen
  coordinates and scales `size` by `scale` before calling `getStroke`. Everything
  else (perfect-freehand outline, `Path2D` fill, highlighter `globalAlpha 0.35` +
  `multiply`, dpr backing store, single-point dot, empty-guard) is unchanged.
  When `logicalW/H` are absent (empty sketch), nothing to draw — clears only.

### `SketchCell.svelte`

- **State:** `expanded` (bool). Logical dims read from / written to `cell.logicalW`,
  `cell.logicalH`.
- **Transform helper:** the component computes the same `fitTransform` (imported)
  for the *current* container to convert pointer events to logical coordinates in
  `pointOf`, and passes `logicalW/H` to `renderStrokes`.
- **First-stroke logical init:** on `onDown`, if `cell.logicalW`/`logicalH` are
  unset, set them to the current container CSS size (`wrap.clientWidth`,
  `wrap.clientHeight`) before computing the transform; persist with the stroke.
- **`pointOf(e)`** returns logical `[lx, ly, pressure]` using the inverse transform.
- **Stroke size:** captured as `size ÷ scale` at `onDown` (store logical width).
- **Expand toggle:** a toolbar button sets `expanded = true/false`. On enter, add a
  `keydown` listener (Esc → collapse); on exit/`onDestroy`, remove it. Re-render
  after the DOM updates (the size change also triggers the existing `ResizeObserver`).
- **Resize guard:** `onResize` updates/persists `cell.height` **only when not
  expanded**; it always re-renders.

## Interaction & layout

- **Inline (collapsed):** unchanged layout — toolbar + `.sk-wrap` at `cell.height`,
  `resize: vertical` handle active. The sketch renders letterboxed (scaled to fit
  the inline area), so the whole drawing is visible as a preview.
- **Expanded:** the sketch container becomes a fixed overlay covering the window
  (`position: fixed; inset: 0; z-index` above app chrome), dark backdrop, toolbar
  floating at top, canvas filling the rest. `resize` handle hidden. Drawing uses
  the full window ⇒ the full tablet surface.
- **Exit:** Collapse button or **Esc**. Returns to the inline layout at the saved
  `cell.height`; the drawing is unchanged (same logical strokes, re-fit to inline).

## Rendering flow

1. Container size (CSS px) = `wrap.clientWidth × wrap.clientHeight` (inline height,
   or window size when expanded).
2. `fitTransform(containerW, containerH, cell.logicalW, cell.logicalH)`.
3. `renderStrokes` maps each logical stroke to screen and fills; live redraw stays
   on `requestAnimationFrame`; reopen renders once at mount.

## Persistence

- `logicalW/H`, logical `points`, and logical `size` persist through the existing
  `onChange(cell) → updateCellObj → debounced SaveNotebook(cells_json)` path.
- Persist on completed action only (stroke end, erase, clear, undo/redo, inline
  resize), **never** per `pointermove`, and **never** persist `height` from the
  expanded container. Toggling expand alone does not need to persist (it is
  view-only state; not stored on the cell).

## Migration (existing sketches)

- **No migration.** Legacy sketches (strokes but no `logicalW`/`logicalH`) render
  with the identity transform (`fitTransform` returns `{scale:1, ox:0, oy:0}` when
  logical dims are unset), i.e. their existing absolute-pixel behavior — roughly as
  before at the inline size. No on-load stamping and no point rewriting.
- If the user draws a **new** stroke on a legacy sketch, `logicalW/H` get set from
  the current container at that moment (mixing the old absolute points with the new
  logical space). Accepted: existing sketches are throwaway test drawings.
- A fresh sketch with no strokes leaves `logicalW/H` unset until the first stroke.

## Edge cases / error handling

- **Empty sketch:** no strokes, `logicalW/H` unset ⇒ blank canvas; first stroke
  sets the logical space. No crash.
- **Expand an empty sketch, then draw:** logical space = window size ⇒ whole
  window/tablet live, no dead bands.
- **Expand a wide-short existing sketch:** letterboxes (live strip centered, dead
  bands top/bottom) because its aspect was fixed earlier. Accepted (Expand-first is
  the intended gesture for full-tablet use).
- **`scale ≤ 0` / zero container:** `fitTransform` guards return identity; render
  clears without throwing.
- **Esc while collapsed:** no-op (listener only attached while expanded).
- **Cell destroyed while expanded:** `onDestroy` removes the Esc listener and
  cancels any pending rAF; ResizeObserver disconnected (existing behavior).

## Testing

No JS test runner in this project. Verification is `cd frontend && npm run build`
plus a manual test plan appended to the Slice-4 plan:

- Expand an empty sketch, draw with the pen using the *whole* tablet; confirm the
  cursor reaches all edges of the drawing area and strokes appear where expected.
- Collapse; confirm the whole drawing is visible inline (scaled), not clipped.
- Re-expand; confirm strokes reappear at the same relative positions.
- Draw inline first, then expand; confirm letterbox behavior (no distortion).
- Undo/redo/erase/clear work in both inline and expanded modes.
- Inline resize still persists `cell.height`; expanding does **not** change it.
- Esc exits expand; restart the app and reopen — the sketch re-renders correctly
  (logical coords persisted) and a legacy (no-`logicalW/H`) sketch still shows via
  the identity transform.

## Success criteria

- An **Expand** toggle fills the window with the drawing canvas; Esc/Collapse exits.
- Drawing while expanded uses the full tablet surface (no dead bands when
  Expand-then-draw).
- Strokes render without distortion at any container size; a sketch drawn large is
  fully visible inline (scaled), not clipped.
- `cell.height` is unaffected by expanding; all tools/undo/redo/clear work in both
  modes; sketches persist and re-render across restarts, including migrated legacy
  sketches.

## Dependencies (existing, reused)

Slice 4's `SketchCell` / `sketch-render.js` / `cellRegistry` / `cells_json`
persistence and the `onChange(cell) → updateCellObj → SaveNotebook` path;
CodeMirror-independent; Wails bindings; Vite bundling. No new dependencies.
