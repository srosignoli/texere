<script>
  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused
  export let onInsert = () => {}
</script>

<div class="me-cell">
  <div class="me-bar">
    <span class="me-label">Media</span>
    <span class="me-spacer"></span>
    {#if !readOnly}<button class="me-btn" on:click={onDelete} title="Delete cell">✕</button>{/if}
  </div>
  {#if !cell.asset}
    <div class="me-empty">No asset yet.</div>
  {:else}
    <div class="me-empty">asset: {cell.asset}</div>
  {/if}
</div>

<style>
  .me-cell { border: 1px solid #2a3a4a; border-radius: 6px; background: #101826; }
  .me-bar { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid #2a3a4a; }
  .me-label { font-size: 12px; color: #8aa; }
  .me-spacer { flex: 1 1 auto; }
  .me-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 2px 8px; cursor: pointer; }
  .me-btn:hover { background: #24334a; }
  .me-empty { padding: 12px; color: #8aa; font-size: 13px; }
</style>
