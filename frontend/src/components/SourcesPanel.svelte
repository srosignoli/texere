<script>
  import { onMount } from 'svelte'
  import { ImportTextSource, ImportPdfSource, ImportWebSource, GetReadingList } from '../../wailsjs/go/main/App'
  import { activeReading } from '../stores/reading.js'

  export let onClose = null

  let items = []
  let error = ''

  async function load() {
    try {
      items = (await GetReadingList()) ?? []
    } catch (e) {
      error = String(e?.message ?? e)
    }
  }

  async function importSource() {
    error = ''
    try {
      const v = await ImportTextSource()
      if (v) {
        await load()
        activeReading.set(v) // open it immediately
      }
    } catch (e) {
      error = String(e?.message ?? e)
    }
  }

  async function importPdf() {
    error = ''
    try {
      const v = await ImportPdfSource()
      if (v) { await load(); activeReading.set(v) }
    } catch (e) { error = String(e?.message ?? e) }
  }

  async function importWeb() {
    error = ''
    const url = window.prompt('Article URL:')
    if (!url) return
    try {
      const v = await ImportWebSource(url.trim())
      if (v) { await load(); activeReading.set(v) }
    } catch (e) { error = String(e?.message ?? e) }
  }

  function open(v) { activeReading.set(v) }

  // A never-scheduled source has due===0 (NULL); the backend StudyCounts and
  // StudySurface.loadRead both count that as due, so the badge must too.
  const dueNow = (v) => !v.done && (v.due === 0 || v.due * 1000 <= Date.now())

  onMount(load)
</script>

<div class="sources-root" role="dialog" aria-label="Sources">
  <div class="sources-bar">
    <span class="sources-title">Sources</span>
    <button class="sources-import" on:click={importSource}>+ Text</button>
    <button class="sources-import" on:click={importPdf}>+ PDF</button>
    <button class="sources-import" on:click={importWeb}>+ Web</button>
    <button class="sources-close" on:click={() => onClose?.()} title="Close">×</button>
  </div>
  {#if error}<div class="sources-error">{error}</div>{/if}
  {#if items.length === 0}
    <div class="sources-empty">No sources yet. Import a .txt, .md, or .html file to start reading.</div>
  {:else}
    <ul class="sources-list">
      {#each items as v}
        <li class="sources-item" on:click={() => open(v)}>
          <span class="si-title">{v.title}</span>
          <span class="si-pct">{v.percent}%</span>
          {#if v.done}<span class="si-tag done">done</span>
          {:else if dueNow(v)}<span class="si-tag due">due</span>{/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .sources-root { position: absolute; top: 0; right: 0; bottom: 0; width: 340px; z-index: 55;
    background: #131c29; border-left: 1px solid #2a3a4a; display: flex; flex-direction: column; }
  .sources-bar { display: flex; align-items: center; gap: 10px; padding: 8px 12px;
    border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .sources-title { color: #7dd3fc; font-weight: 600; flex: 1; }
  .sources-import, .sources-close { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a;
    border-radius: 6px; padding: 4px 10px; cursor: pointer; font-size: 12px; }
  .sources-close { background: none; border: none; color: #8899aa; font-size: 18px; }
  .sources-error { color: #ff8080; padding: 8px 12px; font-size: 12px; }
  .sources-empty { color: #8aa; padding: 16px 12px; font-size: 13px; }
  .sources-list { list-style: none; margin: 0; padding: 0; overflow: auto; }
  .sources-item { display: flex; align-items: center; gap: 8px; padding: 8px 12px;
    border-bottom: 1px solid #1d2a3a; cursor: pointer; }
  .sources-item:hover { background: #18222f; }
  .si-title { flex: 1; color: #cdd6e0; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .si-pct { color: #7ecfa0; font-size: 12px; }
  .si-tag { font-size: 10px; border-radius: 3px; padding: 1px 5px; }
  .si-tag.due { background: #4a3a1a; color: #e0b000; }
  .si-tag.done { background: #1a3a2a; color: #7ecfa0; }
</style>
