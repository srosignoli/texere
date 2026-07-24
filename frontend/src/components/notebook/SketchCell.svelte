<script>
  import { onMount, onDestroy, tick } from 'svelte'
  import { renderStrokes, fitTransform } from '../../lib/sketch-render.js'

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
  // On WebKitGTK (Linux) a pen tablet reports pointerType 'mouse', so it is
  // indistinguishable from a mouse; default this ON so the pen/mouse draw out of
  // the box. Turn OFF for pen-only palm rejection on platforms that report
  // 'pen'/'touch' correctly (e.g. Windows/WebView2 touchscreens).
  let mouseDraw = true

  let expanded = false // in-window fullscreen overlay for drawing with the whole tablet

  // Live stroke list; undo/redo hold invertible ops.
  let strokes = (cell.strokes ?? []).map((s) => ({ ...s, points: s.points.slice() }))
  let undoStack = []
  let redoStack = []

  let drawing = null   // in-progress pen/highlighter stroke
  let erasing = null   // { removed: [] } during an erase pass
  let activePointerId = null // the one pointer that owns the in-progress action
  let rafPending = false
  let rafId = 0

  onMount(() => {
    ctx = canvas.getContext('2d')
    dpr = window.devicePixelRatio || 1
    wrap.style.height = (cell.height ?? 240) + 'px'
    ro = new ResizeObserver(onResize)
    ro.observe(wrap)
    render()
  })
  onDestroy(() => {
    ro?.disconnect()
    if (rafPending) cancelAnimationFrame(rafId)
    window.removeEventListener('keydown', onKey)
  })

  function render() {
    if (!ctx) return
    const width = wrap.clientWidth
    const height = wrap.clientHeight
    renderStrokes(ctx, drawing ? [...strokes, drawing] : strokes, {
      width, height, dpr, logicalW: cell.logicalW, logicalH: cell.logicalH,
    })
  }
  function scheduleRender() {
    if (rafPending) return
    rafPending = true
    rafId = requestAnimationFrame(() => { rafPending = false; render() })
  }
  function onResize() {
    // Never persist the fullscreen container height as the cell's inline height.
    if (!expanded) {
      const h = Math.round(wrap.clientHeight)
      if (h && h !== cell.height) { cell.height = h; onChange(cell) }
    }
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
    const { scale, ox, oy } = fitTransform(wrap.clientWidth, wrap.clientHeight, cell.logicalW, cell.logicalH)
    return [((e.clientX - r.left) - ox) / scale, ((e.clientY - r.top) - oy) / scale, e.pressure || 0.5]
  }

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
  function onMove(e) {
    if (readOnly || e.pointerId !== activePointerId) return // only the owning pointer draws (palm rejection)
    if (drawing) { drawing.points.push(pointOf(e)); scheduleRender() }
    else if (erasing) { eraseAt(pointOf(e)); scheduleRender() }
  }
  function onUp(e) {
    if (readOnly || e.pointerId !== activePointerId) return
    activePointerId = null
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

  function onKey(e) { if (e.key === 'Escape' && expanded) toggleExpand() }
  async function toggleExpand() {
    expanded = !expanded
    if (expanded) window.addEventListener('keydown', onKey)
    else window.removeEventListener('keydown', onKey)
    await tick() // let the overlay resize the wrap before we re-fit
    render()
  }
</script>

<div class="sk-cell" class:expanded>
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
      <label class="sk-mouse" title="Allow mouse/pen to draw. Turn off for pen-only palm rejection (only effective where the platform reports a real pen — not Linux/WebKitGTK)."><input type="checkbox" bind:checked={mouseDraw} /> mouse/pen</label>
      <button class="sk-expand" on:click={toggleExpand} title="Expand to fill the window (Esc to exit)">{expanded ? '🗕 Collapse' : '⛶ Expand'}</button>
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
  .sk-cell.expanded { position: fixed; inset: 0; z-index: 1000; margin: 0; border-radius: 0; display: flex; flex-direction: column; }
  .sk-cell.expanded .sk-wrap { flex: 1; height: auto !important; resize: none; }
</style>
