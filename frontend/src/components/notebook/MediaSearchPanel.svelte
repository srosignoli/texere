<script>
  import { onMount } from 'svelte'
  import { ListMediaProviders, SearchMedia, FetchMediaURL } from '../../../wailsjs/go/main/App'

  export let kind = 'image'
  export let query = ''
  export let onQuery = () => {}   // (q) => persist the query on the cell
  export let onPick = () => {}    // (result, {asset, mime}) => void

  const LIMIT = 24

  let providers = []
  let providerId = ''
  let results = []
  let searched = false      // distinguishes "no results" from "not searched yet"
  let busy = false
  let err = ''
  let fetchingURL = ''      // FullURL of the result currently downloading
  let seq = 0               // guards against out-of-order responses

  onMount(async () => {
    try {
      providers = (await ListMediaProviders()).filter((p) => p.kinds?.includes(kind))
      providerId = providers[0]?.id ?? ''
    } catch (e) { err = String(e?.message ?? e) }
  })

  async function search() {
    const q = query.trim()
    if (!q || !providerId) return
    const mine = ++seq
    busy = true; err = ''; results = []
    try {
      const rs = await SearchMedia(providerId, q, kind, LIMIT)
      if (mine !== seq) return       // a newer search superseded this one
      results = rs ?? []
      searched = true
    } catch (e) {
      if (mine !== seq) return
      err = String(e?.message ?? e)
    } finally {
      if (mine === seq) busy = false
    }
  }

  function onKey(e) {
    if (e.key === 'Enter') { e.preventDefault(); search() }
  }

  async function pick(r) {
    if (fetchingURL) return
    fetchingURL = r.fullUrl
    err = ''
    try {
      const ref = await FetchMediaURL(r.fullUrl)
      onPick(r, ref)
    } catch (e) {
      const msg = String(e?.message ?? e)
      // 403 on a direct fetch almost always means hotlink protection: the
      // picture is viewable on its page but not downloadable by URL.
      err = msg.includes('403')
        ? `${msg} — this host blocks direct downloads. Open the source page and use Paste URL with the real image address.`
        : msg
    } finally {
      fetchingURL = ''
    }
  }
</script>

<div class="ms-panel">
  <div class="ms-bar">
    <select bind:value={providerId} disabled={busy || !providers.length}>
      {#each providers as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <input class="ms-q" type="text" placeholder="Search for a picture…"
      bind:value={query} on:input={() => onQuery(query)} on:keydown={onKey} />
    <button class="ms-btn" on:click={search} disabled={busy || !query.trim()}>
      {busy ? 'Searching…' : 'Search'}
    </button>
  </div>

  {#if err}<div class="ms-err">{err}</div>{/if}

  {#if busy}
    <div class="ms-note">Searching {providers.find((p) => p.id === providerId)?.name ?? ''}…</div>
  {:else if searched && !results.length && !err}
    <div class="ms-note">No results for “{query.trim()}”. Try another provider, or Paste URL.</div>
  {:else if results.length}
    <div class="ms-grid">
      {#each results as r (r.fullUrl)}
        <button class="ms-hit" class:ms-loading={fetchingURL === r.fullUrl}
          on:click={() => pick(r)} title={`${r.title}${r.license ? ' — ' + r.license : ''}`}>
          <img src={r.thumbUrl} alt={r.title} loading="lazy" />
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .ms-panel { padding: 8px; }
  .ms-bar { display: flex; gap: 6px; align-items: center; }
  .ms-q { flex: 1 1 auto; background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 4px 8px; }
  .ms-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 4px 10px; cursor: pointer; }
  .ms-btn:disabled { opacity: 0.5; cursor: default; }
  select { background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 4px; }
  .ms-err { margin-top: 8px; padding: 6px 8px; border: 1px solid #6a3a3a; border-radius: 4px; background: #2a1616; color: #f9c; font-size: 12px; }
  .ms-note { margin-top: 8px; color: #8aa; font-size: 12px; }
  .ms-grid { margin-top: 8px; display: grid; grid-template-columns: repeat(auto-fill, minmax(96px, 1fr)); gap: 6px; max-height: 320px; overflow-y: auto; }
  .ms-hit { padding: 0; border: 1px solid #2a3a4a; border-radius: 4px; background: #0b1220; cursor: pointer; overflow: hidden; aspect-ratio: 1; }
  .ms-hit:hover { border-color: #4a7aa8; }
  .ms-hit.ms-loading { opacity: 0.4; cursor: default; }
  .ms-hit img { width: 100%; height: 100%; object-fit: cover; display: block; }
</style>
