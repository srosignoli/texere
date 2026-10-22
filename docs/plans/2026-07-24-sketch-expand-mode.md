# Sketch Expand Mode + Resolution-Independent Strokes Implementation Plan

**Goal:** Add an in-window Expand (fullscreen) mode to the sketch cell so a pen tablet's full surface maps to the drawing area, and make strokes resolution-independent (fixed logical space + uniform scale-to-fit) so they render correctly at any size.

**Architecture:** Strokes are stored in a fixed per-sketch logical coordinate space (`cell.logicalW × cell.logicalH`, set lazily on the first stroke). A pure `fitTransform()` maps logical→container (uniform scale + letterbox); it's baked into the canvas transform for rendering and inverted for pointer input. Expand is a per-cell CSS overlay (`position: fixed; inset: 0`) — no OS/Wails calls, no new deps.

**Tech Stack:** Svelte 3.49, Canvas 2D, Pointer Events, `perfect-freehand` (existing). Frontend-only; no backend, no new dependencies.

## Global Constraints

- Frontend has no JS test runner; every task verifies with `cd frontend && npm run build` plus a manual test plan. Do NOT add JS unit tests.
- No new npm or Go dependencies. No backend changes. No changes outside `frontend/src/lib/sketch-render.js`, `frontend/src/components/notebook/SketchCell.svelte`, and the new manual-test-plan doc.
- Persist on completed action only (stroke end, erase, clear, undo/redo, inline resize) — NEVER per `pointermove`.
- NEVER write `cell.height` from the expanded container size (guard on `!expanded`).
- Stroke `points` and `size` are stored in LOGICAL units. `size` stored = `sliderPx ÷ drawScale`; rendered = `size × scale`.
- `logicalW`/`logicalH` are set lazily on the first stroke from the active container CSS size; absent on fresh cells and legacy sketches. Treat missing/falsy logical dims as "unset".
- Legacy sketches (no `logicalW/H`) MUST still render — `fitTransform` returns identity `{scale:1, ox:0, oy:0}` when logical dims are unset, preserving old absolute-pixel behavior. No migration.
- Expand is an in-window CSS overlay only (`position: fixed; inset: 0`); Esc or a Collapse button exits. No Wails `WindowFullscreen`.
- Palette, tool set, sizes (pen 3, highlighter 14), highlighter `globalAlpha 0.35` + `multiply`, dpr backing store, undo/redo bound 100 — all unchanged.

---

## File Structure

- `frontend/src/lib/sketch-render.js` — MODIFY: add `fitTransform()`; make `renderStrokes` logical-aware (bake the fit transform).
- `frontend/src/components/notebook/SketchCell.svelte` — MODIFY: logical coordinates (Task 2) then Expand mode (Task 3).
- `docs/specs/2026-07-24-sketch-expand-mode-manual-test-plan.md` — CREATE (Task 4).

`frontend/src/lib/notebook.js` `newSketchCell()` is intentionally **unchanged** — a fresh cell has no `logicalW/H`.

---

## Task 1: `sketch-render.js` — `fitTransform` + logical-aware rendering

**Files:**
- Modify: `frontend/src/lib/sketch-render.js`

**Interfaces:**
- Produces: `fitTransform(containerW, containerH, logicalW, logicalH) → { scale, ox, oy }` and an extended `renderStrokes(ctx, strokes, { width, height, dpr, logicalW, logicalH })`. Both consumed by `SketchCell.svelte` in Tasks 2–3.

- [ ] **Step 1: Replace the file contents**

Replace the entire file with:

```js
import { getStroke } from 'perfect-freehand'

const STROKE_OPTS = { thinning: 0.6, smoothing: 0.5, streamline: 0.5, simulatePressure: true }

// Build a filled Path2D from a perfect-freehand outline polygon.
function pathFromOutline(outline) {
  const p = new Path2D()
  if (!outline.length) return p
  p.moveTo(outline[0][0], outline[0][1])
  for (let i = 1; i < outline.length; i++) p.lineTo(outline[i][0], outline[i][1])
  p.closePath()
  return p
}

// fitTransform computes a uniform scale-to-fit (letterbox) mapping from a
// sketch's fixed logical space (logicalW × logicalH) into a container of
// containerW × containerH CSS px. Returns { scale, ox, oy } so a logical point
// (lx, ly) maps to CSS (ox + lx*scale, oy + ly*scale). When logical dims are
// unset/invalid, returns identity so legacy absolute-pixel sketches render
// unchanged.
export function fitTransform(containerW, containerH, logicalW, logicalH) {
  if (!logicalW || !logicalH || logicalW <= 0 || logicalH <= 0) return { scale: 1, ox: 0, oy: 0 }
  const scale = Math.min(containerW / logicalW, containerH / logicalH)
  const ox = (containerW - logicalW * scale) / 2
  const oy = (containerH - logicalH * scale) / 2
  return { scale, ox, oy }
}

// renderStrokes clears the canvas and fills each stroke. width/height are CSS
// pixels; dpr scales the backing store so strokes are crisp on HiDPI displays.
// logicalW/logicalH define the sketch's logical space; the fit transform (scale
// + letterbox offset) is baked into the canvas transform so logical stroke
// coordinates AND sizes map into the container. Unset logical dims → identity
// (legacy absolute-pixel strokes render unchanged).
export function renderStrokes(ctx, strokes, { width, height, dpr = 1, logicalW = 0, logicalH = 0 }) {
  const canvas = ctx.canvas
  const bw = Math.max(1, Math.round(width * dpr))
  const bh = Math.max(1, Math.round(height * dpr))
  if (canvas.width !== bw) canvas.width = bw
  if (canvas.height !== bh) canvas.height = bh
  // Clear in device space, then apply the fit transform baked with dpr.
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.clearRect(0, 0, bw, bh)
  const { scale, ox, oy } = fitTransform(width, height, logicalW, logicalH)
  ctx.setTransform(dpr * scale, 0, 0, dpr * scale, dpr * ox, dpr * oy)
  for (const s of strokes ?? []) {
    if (!s || !s.points || !s.points.length) continue
    const outline = getStroke(s.points, { size: s.size ?? 3, ...STROKE_OPTS })
    const path = pathFromOutline(outline)
    ctx.save()
    if (s.tool === 'highlighter') {
      ctx.globalAlpha = 0.35
      ctx.globalCompositeOperation = 'multiply'
    }
    ctx.fillStyle = s.color ?? '#e8eaf0'
    ctx.fill(path)
    ctx.restore()
  }
}
```

- [ ] **Step 2: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds (this module is already imported by `SketchCell.svelte`; the new `renderStrokes` params default, so the existing call site — which does not yet pass `logicalW/H` — still works: unset dims → identity → current inline behavior unchanged).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/sketch-render.js
git commit -m "feat(sketch): fitTransform + logical-aware renderStrokes"
```

---

## Task 2: `SketchCell.svelte` — logical coordinates

**Files:**
- Modify: `frontend/src/components/notebook/SketchCell.svelte`

**Interfaces:**
- Consumes: `fitTransform` and the extended `renderStrokes` from Task 1.
- Produces: strokes with logical `points`/`size` and `cell.logicalW`/`cell.logicalH`, consumed by Task 3's expand rendering and by persistence.

- [ ] **Step 1: Import `fitTransform`**

Change the render import line:

```js
  import { renderStrokes } from '../../lib/sketch-render.js'
```

to:

```js
  import { renderStrokes, fitTransform } from '../../lib/sketch-render.js'
```

- [ ] **Step 2: Pass logical dims to `renderStrokes`**

Replace `render()`:

```js
  function render() {
    if (!ctx) return
    const width = wrap.clientWidth
    const height = wrap.clientHeight
    renderStrokes(ctx, drawing ? [...strokes, drawing] : strokes, { width, height, dpr })
  }
```

with:

```js
  function render() {
    if (!ctx) return
    const width = wrap.clientWidth
    const height = wrap.clientHeight
    renderStrokes(ctx, drawing ? [...strokes, drawing] : strokes, {
      width, height, dpr, logicalW: cell.logicalW, logicalH: cell.logicalH,
    })
  }
```

- [ ] **Step 3: Invert the transform in `pointOf`**

Replace `pointOf`:

```js
  function pointOf(e) {
    const r = canvas.getBoundingClientRect()
    return [e.clientX - r.left, e.clientY - r.top, e.pressure || 0.5]
  }
```

with:

```js
  function pointOf(e) {
    const r = canvas.getBoundingClientRect()
    const { scale, ox, oy } = fitTransform(wrap.clientWidth, wrap.clientHeight, cell.logicalW, cell.logicalH)
    return [((e.clientX - r.left) - ox) / scale, ((e.clientY - r.top) - oy) / scale, e.pressure || 0.5]
  }
```

- [ ] **Step 4: Keep the eraser threshold ~constant in screen px**

Replace `eraseAt`:

```js
  function eraseAt(p) {
    const th = 10
    const keep = []
    for (const s of strokes) {
      if (s.points.some((q) => Math.hypot(q[0] - p[0], q[1] - p[1]) < th)) erasing.removed.push(s)
      else keep.push(s)
    }
    if (keep.length !== strokes.length) strokes = keep
  }
```

with:

```js
  function eraseAt(p) {
    // p is in logical units; scale a ~10px screen threshold into logical units.
    const { scale } = fitTransform(wrap.clientWidth, wrap.clientHeight, cell.logicalW, cell.logicalH)
    const th = 10 / scale
    const keep = []
    for (const s of strokes) {
      if (s.points.some((q) => Math.hypot(q[0] - p[0], q[1] - p[1]) < th)) erasing.removed.push(s)
      else keep.push(s)
    }
    if (keep.length !== strokes.length) strokes = keep
  }
```

- [ ] **Step 5: Set the logical canvas on first stroke and store logical size**

Replace `onDown`:

```js
  function onDown(e) {
    if (readOnly || !accept(e)) return
    if (activePointerId !== null) return // ignore a second pointer while one action is in progress
    activePointerId = e.pointerId
    canvas.setPointerCapture(e.pointerId)
    if (tool === 'eraser') { erasing = { removed: [] }; eraseAt(pointOf(e)) }
    else drawing = { tool, color, size, points: [pointOf(e)] }
    scheduleRender()
  }
```

with:

```js
  function onDown(e) {
    if (readOnly || !accept(e)) return
    if (activePointerId !== null) return // ignore a second pointer while one action is in progress
    activePointerId = e.pointerId
    canvas.setPointerCapture(e.pointerId)
    // Lazily fix the sketch's logical canvas to the current container on the
    // first stroke (Expand-first ⇒ whole window/tablet becomes the logical space).
    if (!cell.logicalW || !cell.logicalH) {
      cell.logicalW = wrap.clientWidth
      cell.logicalH = wrap.clientHeight
    }
    const { scale } = fitTransform(wrap.clientWidth, wrap.clientHeight, cell.logicalW, cell.logicalH)
    if (tool === 'eraser') { erasing = { removed: [] }; eraseAt(pointOf(e)) }
    else drawing = { tool, color, size: size / scale, points: [pointOf(e)] } // store logical width
    scheduleRender()
  }
```

- [ ] **Step 6: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds. (At inline size the first stroke sets `logicalW/H` = inline size, so `scale = 1` and behavior is identical to before; logical coords/size are now persisted.)

- [ ] **Step 7: Manual smoke test**

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build` then `./build/bin/texere`. Add a Sketch cell, draw a few strokes inline, erase one, undo/redo. Confirm normal behavior (this task adds no visible change inline; it lays the logical-coordinate groundwork).

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/notebook/SketchCell.svelte
git commit -m "feat(sketch): store strokes in a resolution-independent logical space"
```

---

## Task 3: `SketchCell.svelte` — Expand (in-window overlay) mode

**Files:**
- Modify: `frontend/src/components/notebook/SketchCell.svelte`

**Interfaces:**
- Consumes: the logical-coordinate rendering from Task 2 (expanding changes the container size; strokes re-fit automatically).

- [ ] **Step 1: Import `tick`**

Change:

```js
  import { onMount, onDestroy } from 'svelte'
```

to:

```js
  import { onMount, onDestroy, tick } from 'svelte'
```

- [ ] **Step 2: Add `expanded` state**

Add after `let mouseDraw = true` (and its comment block):

```js
  let expanded = false // in-window fullscreen overlay for drawing with the whole tablet
```

- [ ] **Step 3: Add the expand toggle + Esc handler**

Add these functions immediately after `setTool` (near the end of the `<script>`):

```js
  function onKey(e) { if (e.key === 'Escape' && expanded) toggleExpand() }
  async function toggleExpand() {
    expanded = !expanded
    if (expanded) window.addEventListener('keydown', onKey)
    else window.removeEventListener('keydown', onKey)
    await tick() // let the overlay resize the wrap before we re-fit
    render()
  }
```

- [ ] **Step 4: Guard `cell.height` against the expanded size**

Replace `onResize`:

```js
  function onResize() {
    const h = Math.round(wrap.clientHeight)
    if (h && h !== cell.height) { cell.height = h; onChange(cell) }
    render()
  }
```

with:

```js
  function onResize() {
    // Never persist the fullscreen container height as the cell's inline height.
    if (!expanded) {
      const h = Math.round(wrap.clientHeight)
      if (h && h !== cell.height) { cell.height = h; onChange(cell) }
    }
    render()
  }
```

- [ ] **Step 5: Remove the Esc listener on destroy**

Replace `onDestroy`:

```js
  onDestroy(() => { ro?.disconnect(); if (rafPending) cancelAnimationFrame(rafId) })
```

with:

```js
  onDestroy(() => {
    ro?.disconnect()
    if (rafPending) cancelAnimationFrame(rafId)
    window.removeEventListener('keydown', onKey)
  })
```

- [ ] **Step 6: Add the `expanded` class and the toggle button in the markup**

Change the cell wrapper:

```svelte
<div class="sk-cell">
```

to:

```svelte
<div class="sk-cell" class:expanded>
```

And in the toolbar, insert the Expand button immediately before `<span class="sk-spacer"></span>`:

```svelte
      <button class="sk-expand" on:click={toggleExpand} title="Expand to fill the window (Esc to exit)">{expanded ? '🗕 Collapse' : '⛶ Expand'}</button>
      <span class="sk-spacer"></span>
```

- [ ] **Step 7: Add the overlay styles**

Add these rules to the `<style>` block (after the `.sk-wrap canvas` rule):

```css
  .sk-cell.expanded { position: fixed; inset: 0; z-index: 1000; margin: 0; border-radius: 0; display: flex; flex-direction: column; }
  .sk-cell.expanded .sk-wrap { flex: 1; height: auto !important; resize: none; }
```

- [ ] **Step 8: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 9: Manual smoke test**

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build` then `./build/bin/texere`.
- Add a Sketch cell; click **⛶ Expand** → it fills the whole window; the toolbar stays at top.
- Draw with the pen/mouse using the full area; confirm strokes appear under the cursor across the whole surface.
- Press **Esc** (or **Collapse**) → returns to inline at the same height; the drawing is fully visible (scaled), not clipped.
- Re-expand → strokes reappear at the same relative positions.
- Drag the inline resize handle taller/shorter → height persists; expanding does not change the saved height.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/components/notebook/SketchCell.svelte
git commit -m "feat(sketch): in-window Expand mode (fill the window for full-tablet drawing)"
```

---

## Task 4: Manual test plan + full verification

**Files:**
- Create: `docs/specs/2026-07-24-sketch-expand-mode-manual-test-plan.md`

- [ ] **Step 1: Write the manual test plan**

Create the file:

```markdown
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
```

- [ ] **Step 2: Backend suite + full build**

Run: `go test ./...`
Expected: all PASS (no Go changes; confirms nothing broke).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`.

- [ ] **Step 3: Execute the manual test plan**

Walk TC-1 … TC-8 in `./build/bin/texere`.

- [ ] **Step 4: Commit**

```bash
git add docs/specs/2026-07-24-sketch-expand-mode-manual-test-plan.md
git commit -m "docs(test-plan): sketch expand mode manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** `fitTransform` + logical-aware `renderStrokes` (T1); logical `points`/`size`, first-stroke logical init, inverted `pointOf`, scale-aware eraser threshold (T2); in-window overlay Expand/Collapse + Esc, `cell.height` guard, hidden inline resize while expanded, Esc-listener cleanup (T3); manual plan + full verification (T4). Out of scope (OS fullscreen, PNG export, pan/zoom, aspect-lock, migration) omitted by design.
- **Placeholder scan:** no TBD/TODO; every code step shows complete code (full-file replacement for T1; exact old→new blocks for T2–T3).
- **Type/interface consistency:** `fitTransform(containerW, containerH, logicalW, logicalH) → {scale, ox, oy}` is used identically in `renderStrokes`, `render`, `pointOf`, `eraseAt`, and `onDown`. `renderStrokes(ctx, strokes, {width,height,dpr,logicalW,logicalH})` matches the T2 call site. Stored stroke `size = sliderPx/scale` (logical) pairs with the transform baking `× scale` at render, so drawn thickness equals the slider value at draw scale. Legacy/unset logical dims resolve to identity everywhere via the single `fitTransform` guard.
- **Reliability:** Expand uses `position: fixed`; verified the notebook pane has no transformed/filtered ancestor, so `fixed` resolves to the viewport. `height: auto !important` beats the imperative inline `wrap.style.height` while expanded and yields back to it on collapse.
```
