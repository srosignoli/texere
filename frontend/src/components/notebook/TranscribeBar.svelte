<script>
  import { onMount, onDestroy } from 'svelte'
  import { GetLLMConfig } from '../../../wailsjs/go/main/App'
  import { streamTurn, TRANSCRIBE_SYSTEM } from '../../lib/llm.js'

  export let cell
  export let bake            // async (cell) => { mediaType, data }
  export let onInsert = () => {} // (text) => insert an N4L draft below
  export let onChange = () => {} // persist the cell (profile choice)
  export let readOnly = false

  let profiles = []
  let busy = false
  let err = ''
  let turn = null

  onMount(async () => { try { const c = await GetLLMConfig(); profiles = c.profiles ?? [] } catch { /* not configured */ } })

  onDestroy(() => turn?.cancel())

  function setProfile(id) { cell.transcribeProfileId = id || null; cell = cell; onChange(cell) }

  async function transcribe() {
    if (busy) return
    busy = true
    err = ''
    let img
    try { img = await bake(cell) } catch (e) { err = String(e?.message ?? e); busy = false; return }
    let acc = ''
    turn = streamTurn({
      profileId: cell.transcribeProfileId,
      system: TRANSCRIBE_SYSTEM,
      messages: [{ role: 'user', content: 'Transcribe this image.', images: [img] }],
      onDelta: (t) => { acc += t },
      onDone: () => { busy = false; turn = null; if (acc.trim()) onInsert(acc) },
      onError: (m) => { err = m; busy = false; turn = null },
    })
  }
</script>

{#if !readOnly}
  <div class="tb">
    <select value={cell.transcribeProfileId || ''} on:change={(e) => setProfile(e.target.value)} disabled={busy || !profiles.length} title="Model for transcription">
      <option value="">active default</option>
      {#each profiles as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <button on:click={transcribe} disabled={busy || !profiles.length}>{busy ? 'Transcribing…' : 'Transcribe'}</button>
    {#if err}<span class="tb-err">{err}</span>{/if}
  </div>
{/if}

<style>
  .tb { display: flex; align-items: center; gap: 6px; padding: 5px 8px; border-top: 1px solid #1c2a3a; }
  .tb select, .tb button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; font-size: 12px; cursor: pointer; }
  .tb button:disabled, .tb select:disabled { opacity: 0.5; cursor: default; }
  .tb-err { color: #ff8080; font-size: 11px; }
</style>
