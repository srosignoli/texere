<script>
  import { imageStyles } from '../../lib/image-view.js'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused

  let missing = false
  let cropping = false
  let sel = null       // { x, y, w, h } in fractions of the crop-mode image, during a drag
  let imgEl           // the crop-mode <img>, for rect math
  let dragging = false
  let start = null

  $: styles = imageStyles(cell)
  $: src = `/media/${cell.asset}`
  // In crop mode we show the FULL rotated image (ignoring the current crop) so a
  // new crop is always selected over the whole rotated frame.
  $: cropStyles = imageStyles({ ...cell, crop: null })

  function persist() { onChange(cell) }

  function onLoad(e) {
    missing = false
    if (!cell.naturalW || !cell.naturalH) {
      cell.naturalW = e.target.naturalWidth
      cell.naturalH = e.target.naturalHeight
      persist()
    }
  }
  function rotate(delta) {
    cell.rotation = (((cell.rotation || 0) + delta) % 360 + 360) % 360
    cell.crop = null // rotating invalidates a crop taken in the old frame
    persist()
  }
  function reset() {
    cell.rotation = 0
    cell.crop = null
    persist()
  }
  function enterCrop() { cropping = true; sel = null }
  function cancelCrop() { cropping = false; sel = null; dragging = false }
  function applyCrop() {
    if (sel && sel.w > 0.02 && sel.h > 0.02) { cell.crop = { ...sel }; persist() }
    cancelCrop()
  }

  function frac(e) {
    const r = imgEl.getBoundingClientRect()
    return {
      x: Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)),
      y: Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)),
    }
  }
  function down(e) { if (readOnly) return; dragging = true; start = frac(e); sel = { x: start.x, y: start.y, w: 0, h: 0 } }
  function move(e) {
    if (!dragging) return
    const p = frac(e)
    sel = {
      x: Math.min(start.x, p.x), y: Math.min(start.y, p.y),
      w: Math.abs(p.x - start.x), h: Math.abs(p.y - start.y),
    }
  }
  function up() { dragging = false }
</script>

<div class="im-cell">
  {#if !readOnly}
    <div class="im-bar">
      <button on:click={() => rotate(-90)} title="Rotate left">↶</button>
      <button on:click={() => rotate(90)} title="Rotate right">↷</button>
      {#if cropping}
        <button on:click={applyCrop} title="Apply crop">✓ Apply</button>
        <button on:click={cancelCrop} title="Cancel crop">✕ Cancel</button>
      {:else}
        <button on:click={enterCrop} title="Crop">⛶ Crop</button>
      {/if}
      <button on:click={reset} title="Reset rotation and crop">Reset</button>
      <span class="im-spacer"></span>
      <button class="im-del" on:click={onDelete} title="Delete cell">✕</button>
    </div>
  {/if}

  {#if missing}
    <div class="im-missing">image not found: {cell.asset}</div>
  {:else if cropping}
    <div class="im-cropwrap">
      <div style={cropStyles.viewport}>
        <div style={cropStyles.pan}>
          <img bind:this={imgEl} {src} style={cropStyles.img} alt=""
            on:pointerdown={down} on:pointermove={move} on:pointerup={up} on:pointercancel={up} draggable="false" />
        </div>
      </div>
      {#if sel}
        <div class="im-sel" style="left:{sel.x * 100}%;top:{sel.y * 100}%;width:{sel.w * 100}%;height:{sel.h * 100}%;"></div>
      {/if}
    </div>
  {:else}
    <div style={styles.viewport}>
      <div style={styles.pan}>
        <img {src} style={styles.img} alt="" on:load={onLoad} on:error={() => (missing = true)} draggable="false" />
      </div>
    </div>
  {/if}
</div>

<style>
  .im-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .im-bar { display: flex; align-items: center; gap: 6px; padding: 5px 8px; border-bottom: 1px solid #1c2a3a; flex-wrap: wrap; }
  .im-bar button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
  .im-spacer { flex: 1; }
  .im-del { background: none !important; border: none !important; color: #6b7a8a !important; }
  .im-del:hover { color: #ff8080 !important; }
  .im-cell > div:not(.im-bar):not(.im-cropwrap) { max-height: 70vh; }
  .im-cell img { max-width: 100%; }
  .im-cropwrap { position: relative; max-height: 70vh; touch-action: none; cursor: crosshair; }
  .im-sel { position: absolute; border: 1.5px solid #7dd3fc; background: rgba(125, 211, 252, 0.15); pointer-events: none; }
  .im-missing { padding: 20px; color: #ff8080; font-size: 13px; }
</style>
