<script>
  import { imageStyles } from '../../../lib/image-view.js'

  export let cell
  export let cropping = false
  export let onCrop = () => {}   // ({x,y,w,h}) => void
  export let onMeta = () => {}   // (w, h) => record and persist the intrinsic size

  let missing = false
  let imgEl
  let sel = null
  let dragging = false
  let start = null

  $: styles = imageStyles(cell)
  $: src = `/media/${cell.asset}`
  // While cropping, show the uncropped frame so a new selection always covers
  // the whole image rather than the previous crop.
  $: cropStyles = imageStyles({ ...cell, crop: null })

  function onLoad(e) {
    missing = false
    if (!cell.naturalW || !cell.naturalH) {
      onMeta(e.target.naturalWidth, e.target.naturalHeight)
    }
  }

  function frac(e) {
    const r = imgEl.getBoundingClientRect()
    return {
      x: Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)),
      y: Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)),
    }
  }
  function down(e) { dragging = true; start = frac(e); sel = null; e.target.setPointerCapture?.(e.pointerId) }
  function move(e) {
    if (!dragging) return
    const p = frac(e)
    sel = { x: Math.min(start.x, p.x), y: Math.min(start.y, p.y), w: Math.abs(p.x - start.x), h: Math.abs(p.y - start.y) }
  }
  function up() {
    if (!dragging) return
    dragging = false
    if (sel && sel.w > 0.02 && sel.h > 0.02) onCrop({ ...sel })
    sel = null
  }
</script>

{#if missing}
  <div class="mv-missing">image not found: {cell.asset}</div>
{:else if cropping}
  <div class="mv-cropwrap">
    <div class="mv-view" style={cropStyles.viewport}>
      <div style={cropStyles.pan}>
        <img bind:this={imgEl} {src} style={cropStyles.img} alt=""
          on:pointerdown={down} on:pointermove={move} on:pointerup={up} on:pointercancel={up} draggable="false" />
      </div>
      {#if sel}
        <div class="mv-sel" style="left:{sel.x * 100}%;top:{sel.y * 100}%;width:{sel.w * 100}%;height:{sel.h * 100}%;"></div>
      {/if}
    </div>
    <div class="mv-hint">Drag to select a crop.</div>
  </div>
{:else}
  <div class="mv-view" style={styles.viewport}>
    <div style={styles.pan}>
      <img {src} style={styles.img} alt={cell.subject || cell.query || ''}
        on:load={onLoad} on:error={() => (missing = true)} draggable="false" />
    </div>
  </div>
{/if}

<style>
  .mv-view { position: relative; overflow: hidden; margin: 8px; }
  .mv-missing { padding: 12px; color: #d88; font-size: 13px; }
  .mv-cropwrap { position: relative; }
  .mv-sel { position: absolute; border: 1px solid #6ad; background: rgba(102, 170, 221, 0.2); pointer-events: none; }
  .mv-hint { padding: 0 8px 8px; color: #8aa; font-size: 11px; }
</style>
