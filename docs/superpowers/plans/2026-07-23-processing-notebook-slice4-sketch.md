# Processing Notebook — Slice 4 (Sketch cell) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a sketch cell to the processing notebook — a resizable canvas for freehand pen/mouse drawing with pen/highlighter/stroke-eraser tools, undo/redo, and vector strokes persisted in `cells_json`.

**Architecture:** A pure `sketch-render.js` turns vector strokes into filled canvas paths via `perfect-freehand`; `SketchCell.svelte` owns Pointer-Events input, the undo/redo stacks, the toolbar, and resize/persistence. The cell slots into the existing `cellRegistry` and persists through the Slice-1 `onChange(cell) → updateCellObj → SaveNotebook` path. No backend changes.

**Tech Stack:** Svelte 3.49, Vite 3, `perfect-freehand` (new npm dep), Canvas 2D + Pointer Events + ResizeObserver.

## Global Constraints

- Frontend has no JS test runner; frontend tasks verify with `cd frontend && npm run build` plus a manual test plan.
- New **npm** dep allowed this slice: `perfect-freehand` (MIT). No new Go deps, no backend changes.
- Pen draws by default; `touch` is always ignored (palm rejection); a mouse-draw toggle additionally allows `mouse`.
- Pressure enhances stroke width but nothing depends on it — use `perfect-freehand`'s `simulatePressure: true` + a pressure floor so strokes always render.
- Persist on stroke end (`pointerup`) / erase / clear / undo / redo / resize — **never** per `pointermove`. Live drawing redraws on `requestAnimationFrame`.
- Eraser removes **whole strokes** (stroke-erase); no pixel/raster model.
- Vector strokes + `height` persist in `cells_json`; no PNG thumbnail, no on-disk assets. On reopen, re-render `cell.strokes`.
- Sketch cells are NOT added to the LLM attach picker (Slice 6 wires sketch→multimodal).
- Palette (dark-theme): `#e8eaf0` (white), `#f59e0b` (amber), `#7dd3fc` (cyan), `#ff6b6b` (red), `#10b981` (green). Default sizes: pen `3`, highlighter `14`.
- Run `wails`/`make` via `PATH="$PATH:$(go env GOPATH)/bin"`.

---

## File Structure

**Frontend (Svelte/JS)**
- `frontend/src/lib/sketch-render.js` — create: pure `renderStrokes(ctx, strokes, {width,height,dpr})`.
- `frontend/src/components/notebook/SketchCell.svelte` — create: canvas + toolbar + Pointer Events + undo/redo + resize + persistence.
- `frontend/src/lib/notebook.js` — modify: register `sketch` in `cellRegistry`; add `newSketchCell()`.
- `frontend/src/components/notebook/NotebookPane.svelte` — modify: "+ Sketch cell" button.
- `frontend/package.json` — modify: add `perfect-freehand`.
- `docs/superpowers/specs/2026-07-23-processing-notebook-slice4-manual-test-plan.md` — create.

Cell object shape (`cells_json`): `{ id, type:'sketch', height, strokes:[ {tool,color,size,points:[[x,y,pressure],…]} ] }`.

---

## Task 1: Add the perfect-freehand dependency

**Files:**
- Modify: `frontend/package.json` (+ lockfile)

- [ ] **Step 1: Install**

Run: `cd frontend && npm install perfect-freehand`
Expected: `dependencies` gains `perfect-freehand`; `package-lock.json` updated.

- [ ] **Step 2: Verify the build still succeeds**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "chore(deps): add perfect-freehand for the sketch cell"
```

---

## Task 2: sketch-render.js — strokes → canvas

**Files:**
- Create: `frontend/src/lib/sketch-render.js`

**Interfaces:**
- Consumes: `getStroke` from `perfect-freehand`.
- Produces: `renderStrokes(ctx, strokes, { width, height, dpr })` — sizes the canvas backing store to `width*dpr × height*dpr`, scales the context by `dpr`, clears, and fills each stroke's `perfect-freehand` outline (pen = solid; highlighter = alpha 0.35 + `multiply`). `strokes` items are `{tool,color,size,points:[[x,y,pressure],…]}`.

- [ ] **Step 1: Create the module**

Create `frontend/src/lib/sketch-render.js`:

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

// renderStrokes clears the canvas and fills each stroke. width/height are CSS
// pixels; dpr scales the backing store so strokes are crisp on HiDPI displays.
export function renderStrokes(ctx, strokes, { width, height, dpr = 1 }) {
  const canvas = ctx.canvas
  const bw = Math.max(1, Math.round(width * dpr))
  const bh = Math.max(1, Math.round(height * dpr))
  if (canvas.width !== bw) canvas.width = bw
  if (canvas.height !== bh) canvas.height = bh
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, width, height)
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

- [ ] **Step 2: Verify the frontend builds**

Run: `cd frontend && npm run build`
Expected: build succeeds. (Note: nothing imports `sketch-render.js` yet, so Vite tree-shakes it — this build confirms no syntax break but the module is actually compiled into the bundle only once `SketchCell.svelte` imports it in Task 4's build. Task 3 authors the importer; Task 4's build is the real compile check.)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/sketch-render.js
git commit -m "feat(notebook): sketch-render (perfect-freehand strokes → canvas)"
```

---

## Task 3: SketchCell component

**Files:**
- Create: `frontend/src/components/notebook/SketchCell.svelte`

**Interfaces:**
- Consumes: `renderStrokes` from `../../lib/sketch-render.js`.
- Props: `cell` (`{id,type:'sketch',height,strokes}`), `onChange(cell)`, `onDelete()`, `readOnly`, plus ignored `context`/`cells`/`onPush` (shared cell prop set).
- Produces: `SketchCell`. Persists `cell.strokes` + `cell.height` via `onChange(cell)` on each completed action.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/notebook/SketchCell.svelte`:

```svelte
<script>
  import { onMount, onDestroy } from 'svelte'
  import { renderStrokes } from '../../lib/sketch-render.js'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused; shared cell prop set

  const COLORS = ['#e8eaf0', '#f59e0b', '#7dd3fc', '#ff6b6b', '#10b981']
  const SIZES = { pen: 3, highlighter: 14 }

  let wrap, canvas, ctx, ro
  let dpr = 1
  let tool = 'pen'
  let color = COLORS[0]
  let size = SIZES.pen
  let mouseDraw = false

  // Live stroke list; undo/redo hold invertible ops.
  let strokes = (cell.strokes ?? []).map((s) => ({ ...s, points: s.points.slice() }))
  let undoStack = []
  let redoStack = []

  let drawing = null   // in-progress pen/highlighter stroke
  let erasing = null   // { removed: [] } during an erase pass
  let rafPending = false

  onMount(() => {
    ctx = canvas.getContext('2d')
    dpr = window.devicePixelRatio || 1
    wrap.style.height = (cell.height ?? 240) + 'px'
    ro = new ResizeObserver(onResize)
    ro.observe(wrap)
    render()
  })
  onDestroy(() => ro?.disconnect())

  function render() {
    if (!ctx) return
    const width = wrap.clientWidth
    const height = wrap.clientHeight
    renderStrokes(ctx, drawing ? [...strokes, drawing] : strokes, { width, height, dpr })
  }
  function scheduleRender() {
    if (rafPending) return
    rafPending = true
    requestAnimationFrame(() => { rafPending = false; render() })
  }
  function onResize() {
    const h = Math.round(wrap.clientHeight)
    if (h && h !== cell.height) { cell.height = h; onChange(cell) }
    render()
  }

  function persist() { cell.strokes = strokes; onChange(cell) }
  function pushOp(op) { undoStack = [...undoStack.slice(-99), op]; redoStack = [] }

  function accept(e) {
    if (e.pointerType === 'pen') return true
    if (e.pointerType === 'mouse') return mouseDraw
    return false // touch → palm rejection
  }
  function pointOf(e) {
    const r = canvas.getBoundingClientRect()
    return [e.clientX - r.left, e.clientY - r.top, e.pressure || 0.5]
  }

  function eraseAt(p) {
    const th = 10
    const keep = []
    for (const s of strokes) {
      if (s.points.some((q) => Math.hypot(q[0] - p[0], q[1] - p[1]) < th)) erasing.removed.push(s)
      else keep.push(s)
    }
    if (keep.length !== strokes.length) strokes = keep
  }

  function onDown(e) {
    if (readOnly || !accept(e)) return
    canvas.setPointerCapture(e.pointerId)
    if (tool === 'eraser') { erasing = { removed: [] }; eraseAt(pointOf(e)) }
    else drawing = { tool, color, size, points: [pointOf(e)] }
    scheduleRender()
  }
  function onMove(e) {
    if (readOnly) return
    if (drawing) { drawing.points.push(pointOf(e)); scheduleRender() }
    else if (erasing) { eraseAt(pointOf(e)); scheduleRender() }
  }
  function onUp() {
    if (readOnly) return
    if (drawing) {
      if (drawing.points.length) { strokes = [...strokes, drawing]; pushOp({ type: 'add', stroke: drawing }); persist() }
      drawing = null
    } else if (erasing) {
      if (erasing.removed.length) { pushOp({ type: 'erase', removed: erasing.removed }); persist() }
      erasing = null
    }
    render()
  }

  function undo() {
    if (!undoStack.length) return
    const op = undoStack[undoStack.length - 1]
    undoStack = undoStack.slice(0, -1)
    if (op.type === 'add') strokes = strokes.filter((s) => s !== op.stroke)
    else if (op.type === 'erase') strokes = [...strokes, ...op.removed]
    else if (op.type === 'clear') strokes = op.removed
    redoStack = [...redoStack, op]
    persist(); render()
  }
  function redo() {
    if (!redoStack.length) return
    const op = redoStack[redoStack.length - 1]
    redoStack = redoStack.slice(0, -1)
    if (op.type === 'add') strokes = [...strokes, op.stroke]
    else if (op.type === 'erase') strokes = strokes.filter((s) => !op.removed.includes(s))
    else if (op.type === 'clear') strokes = []
    undoStack = [...undoStack, op]
    persist(); render()
  }
  function clearAll() {
    if (!strokes.length) return
    pushOp({ type: 'clear', removed: strokes })
    strokes = []
    persist(); render()
  }

  function setTool(t) { tool = t; if (SIZES[t]) size = SIZES[t] }
</script>

<div class="sk-cell">
  {#if !readOnly}
    <div class="sk-bar">
      <button class:active={tool === 'pen'} on:click={() => setTool('pen')}>✎ Pen</button>
      <button class:active={tool === 'highlighter'} on:click={() => setTool('highlighter')}>▮ Highlight</button>
      <button class:active={tool === 'eraser'} on:click={() => setTool('eraser')}>⌫ Erase</button>
      <span class="sk-colors">
        {#each COLORS as c}<button class="sw" class:sel={color === c} style="background:{c}" on:click={() => (color = c)} aria-label="color"></button>{/each}
      </span>
      <input class="sk-size" type="range" min="1" max="30" bind:value={size} title="Size" />
      <button on:click={undo} disabled={!undoStack.length} title="Undo">↶</button>
      <button on:click={redo} disabled={!redoStack.length} title="Redo">↷</button>
      <button on:click={clearAll} title="Clear">Clear</button>
      <label class="sk-mouse"><input type="checkbox" bind:checked={mouseDraw} /> mouse</label>
      <span class="sk-spacer"></span>
      <button class="sk-del" on:click={onDelete} title="Delete cell">✕</button>
    </div>
  {/if}
  <div class="sk-wrap" bind:this={wrap}>
    <canvas bind:this={canvas}
      on:pointerdown={onDown} on:pointermove={onMove} on:pointerup={onUp} on:pointercancel={onUp}></canvas>
  </div>
</div>

<style>
  .sk-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .sk-bar { display: flex; align-items: center; gap: 6px; padding: 5px 8px; border-bottom: 1px solid #1c2a3a; flex-wrap: wrap; }
  .sk-bar button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
  .sk-bar button.active { background: #244a72; color: #e8eaf0; }
  .sk-bar button:disabled { opacity: 0.4; cursor: default; }
  .sk-colors { display: flex; gap: 3px; }
  .sw { width: 16px; height: 16px; padding: 0 !important; border-radius: 50% !important; border: 1px solid #2a3a4a; cursor: pointer; }
  .sw.sel { outline: 2px solid #7dd3fc; }
  .sk-size { width: 80px; }
  .sk-mouse { color: #93c5fd; font-size: 11px; display: flex; align-items: center; gap: 3px; }
  .sk-spacer { flex: 1; }
  .sk-del { background: none !important; border: none !important; color: #6b7a8a !important; }
  .sk-del:hover { color: #ff8080 !important; }
  .sk-wrap { position: relative; height: 240px; min-height: 80px; resize: vertical; overflow: hidden; background: #0b1220; }
  .sk-wrap canvas { display: block; width: 100%; height: 100%; touch-action: none; cursor: crosshair; }
</style>
```

> Notes: `touch-action: none` on the canvas stops touch/pen from scrolling the pane while drawing. The wrapper's initial height is set imperatively in `onMount` (not via a reactive inline style) so `resize: vertical` drags aren't overwritten on re-render. Erase/undo/redo track strokes by object identity, so re-adding on undo restores exact content. Points store `e.pressure || 0.5` (a floor); `perfect-freehand`'s `simulatePressure` handles flat-pressure devices.

- [ ] **Step 2: Verify the frontend builds**

Run: `cd frontend && npm run build`
Expected: build succeeds. (Still not wired into `cellRegistry`, so — like `ComputeCell` in Slice 3 — the real Vite compile happens in Task 4. This build confirms the component's own syntax via `svelte()` only if reachable; to be safe, Task 4's build is the gate.)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/notebook/SketchCell.svelte
git commit -m "feat(notebook): sketch cell (canvas + pen/highlighter/eraser + undo/redo + resize)"
```

---

## Task 4: Register sketch cell + NotebookPane button

**Files:**
- Modify: `frontend/src/lib/notebook.js`, `frontend/src/components/notebook/NotebookPane.svelte`

**Interfaces:**
- Consumes: `SketchCell` (Task 3).
- Produces: `newSketchCell()`; `cellRegistry.sketch`; a "+ Sketch cell" button.

- [ ] **Step 1: Register the cell type**

In `frontend/src/lib/notebook.js`:

```js
import SketchCell from '../components/notebook/SketchCell.svelte'
```

```js
export function newSketchCell() {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'sketch', height: 240, strokes: [] }
}
```

Add to `cellRegistry`:

```js
  sketch: SketchCell,
```

- [ ] **Step 2: Add the "+ Sketch cell" button to NotebookPane**

In `frontend/src/components/notebook/NotebookPane.svelte`:
- Extend the import to include `newSketchCell`:
  `import { newDraftCell, newLLMCell, newGraphCell, newComputeCell, newSketchCell, cellRegistry } from '../../lib/notebook.js'`
- Add the handler beside `addCompute`:
  `function addSketch() { cells = [...cells, newSketchCell()]; scheduleSave() }`
- Add the button after "+ Compute cell" (inside the `{#if !readOnly}` add-cell block):
  `<button class="nb-add" on:click={addSketch}>+ Sketch cell</button>`

- [ ] **Step 3: Verify the frontend builds (this is the real compile of SketchCell + sketch-render + perfect-freehand)**

Run: `cd frontend && npm run build`
Expected: build succeeds; `perfect-freehand` is now pulled into the bundle (module count rises).

- [ ] **Step 4: Manual smoke test**

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build && ./build/bin/texere`
Verify: "+ Sketch cell" appears; adding one shows the toolbar + canvas; with the "mouse" toggle on, dragging draws; pen/highlighter/eraser + colors work; undo/redo/clear work; the canvas resizes.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/notebook.js frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): register sketch cell + toolbar button"
```

---

## Task 5: Manual test plan + full verification

**Files:**
- Create: `docs/superpowers/specs/2026-07-23-processing-notebook-slice4-manual-test-plan.md`

- [ ] **Step 1: Write the manual test plan**

Create the file:

```markdown
# Processing Notebook — Slice 4 (Sketch cell) Manual Test Plan

Prereqs: open an extract for processing.

## TC-1 Draw (mouse toggle)
1. Add a Sketch cell; turn ON the "mouse" toggle; drag on the canvas.
   - EXPECT: a smooth pen stroke appears; width varies slightly (simulated pressure).

## TC-2 Palm rejection / pen-only
1. Turn the "mouse" toggle OFF; try to draw with the mouse.
   - EXPECT: nothing draws (pen-only). (With a pen tablet, pen still draws; touch never draws.)

## TC-3 Tools + colors
1. Pick Highlight + amber; draw over an existing pen stroke.
   - EXPECT: a translucent amber highlight that darkens where it overlaps (multiply).
2. Switch pen colors and draw.
   - EXPECT: strokes render in the selected color; the size slider changes width.

## TC-4 Eraser (stroke-erase)
1. Select Erase; drag across a stroke.
   - EXPECT: the whole stroke it touches disappears (not a partial rub-out).

## TC-5 Undo / Redo / Clear
1. Draw 3 strokes; Undo twice; Redo once.
   - EXPECT: strokes remove/re-add in order.
2. Erase a stroke, then Undo.
   - EXPECT: the erased stroke returns.
3. Clear, then Undo.
   - EXPECT: all strokes return.

## TC-6 Resize
1. Drag the bottom edge of the canvas taller, then shorter.
   - EXPECT: the canvas resizes; existing strokes stay put.

## TC-7 Persistence
1. Draw a sketch, restart the app, reopen the extract.
   - EXPECT: the sketch re-renders at the same height and is still editable (draw more, undo works).

## TC-8 Pressure (tablet, optional)
1. On a pen tablet (Windows Ink / WebView2), vary pen pressure.
   - EXPECT: stroke width tracks pressure. On Linux/WebKitGTK, width is velocity-simulated but strokes always draw.
```

- [ ] **Step 2: Backend suite + full build**

Run: `go test ./...`
Expected: all PASS (no Go changes this slice; confirms nothing broke).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`.

- [ ] **Step 3: Execute the manual test plan**

Walk TC-1 … TC-8 in `./build/bin/texere`.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs/2026-07-23-processing-notebook-slice4-manual-test-plan.md
git commit -m "docs(test-plan): Processing Notebook Slice 4 manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** perfect-freehand dep (T1), pure render module incl. highlighter multiply + dpr (T2), SketchCell with Pointer-Events pen-default+mouse-toggle+palm-rejection, pressure floor + simulatePressure, rAF live render, persist-on-end, pen/highlighter/stroke-eraser + palette + size + undo/redo/clear, resizable canvas persisting height (T3), register + button (T4), manual plan + verification (T5). Deferred (LLM attach, push-as-image, shapes/text/layers/pan-zoom, disk assets/thumbnail) are out of scope by design.
- **Placeholder scan:** no TBD/TODO; every code step has complete code.
- **Type consistency:** the cell object `{type:'sketch',height,strokes:[{tool,color,size,points}]}` from `newSketchCell` matches what `SketchCell` reads/writes and what `renderStrokes` consumes (`s.tool/color/size/points`); `renderStrokes(ctx, strokes, {width,height,dpr})` signature is identical in T2 and T3; `onChange(cell)` (object) routes to `updateCellObj` in NotebookPane.
- **Known compile-sequencing note:** `sketch-render.js` (T2) and `SketchCell.svelte` (T3) are tree-shaken until Task 4 wires the cell into `cellRegistry`; T4's `npm run build` (module count rises with perfect-freehand) is the real compile gate, and T5's `make build` embeds it — same pattern used for the compute cell in Slice 3.
```
