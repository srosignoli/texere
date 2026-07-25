<script>
  import { imageStyles } from '../../../lib/image-view.js'

  export let cell
  export let onMeta = () => {}   // (w, h) => record and persist the intrinsic size

  let missing = false

  $: styles = imageStyles(cell)
  $: src = `/media/${cell.asset}`

  function onLoad(e) {
    missing = false
    // Persisted so imageStyles is correct on first paint after a reload, rather
    // than only once the image has loaded. Same behaviour as ImageCell.
    if (!cell.naturalW || !cell.naturalH) {
      onMeta(e.target.naturalWidth, e.target.naturalHeight)
    }
  }
</script>

{#if missing}
  <div class="mv-missing">image not found: {cell.asset}</div>
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
</style>
