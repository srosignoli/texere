<script>
  import MediaSearchPanel from './MediaSearchPanel.svelte'
  import MediaImageView from './media/MediaImageView.svelte'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused
  export let onInsert = () => {}

  // kind -> render component. Audio and video slot in here in later slices
  // without MediaCell changing.
  const views = { image: MediaImageView }

  function persist() { onChange(cell) }

  function onPick(result, ref) {
    cell.asset = ref.asset
    cell.mime = ref.mime
    cell.kind = result.kind || 'image'
    cell.provider = result.provider ?? cell.provider
    cell.sourcePage = result.sourcePage ?? ''
    cell.license = result.license ?? ''
    // The thing you searched for is the thing the fact will be about.
    if (!cell.subject) cell.subject = cell.query.trim()
    cell.naturalW = 0
    cell.naturalH = 0
    cell = cell
    persist()
  }

  function onQuery(q) { cell.query = q; persist() }

  function onMeta(w, h) {
    cell.naturalW = w
    cell.naturalH = h
    cell = cell
    persist()
  }
</script>

<div class="me-cell">
  <div class="me-bar">
    <span class="me-label">Media</span>
    {#if cell.license}<span class="me-lic">{cell.license}</span>{/if}
    <span class="me-spacer"></span>
    {#if !readOnly}<button class="me-btn" on:click={onDelete} title="Delete cell">✕</button>{/if}
  </div>

  {#if !cell.asset}
    {#if readOnly}
      <div class="me-empty">No asset.</div>
    {:else}
      <MediaSearchPanel kind={cell.kind} query={cell.query} {onQuery} {onPick} />
    {/if}
  {:else}
    <svelte:component this={views[cell.kind]} {cell} {onMeta} />
    {#if cell.sourcePage}
      <div class="me-src"><a href={cell.sourcePage} target="_blank" rel="noreferrer">source</a></div>
    {/if}
  {/if}
</div>

<style>
  .me-cell { border: 1px solid #2a3a4a; border-radius: 6px; background: #101826; }
  .me-bar { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid #2a3a4a; }
  .me-label { font-size: 12px; color: #8aa; }
  .me-lic { font-size: 11px; color: #678; }
  .me-spacer { flex: 1 1 auto; }
  .me-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 2px 8px; cursor: pointer; }
  .me-btn:hover { background: #24334a; }
  .me-empty { padding: 12px; color: #8aa; font-size: 13px; }
  .me-src { padding: 0 8px 8px; font-size: 11px; }
  .me-src a { color: #6ad; }
</style>
