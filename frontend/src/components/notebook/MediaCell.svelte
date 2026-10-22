<script>
  import { pickMediaAsset } from '../../lib/cell-schema.js'
  import MediaSearchPanel from './MediaSearchPanel.svelte'
  import MediaImageView from './media/MediaImageView.svelte'
  import MediaAudioView from './media/MediaAudioView.svelte'
  import MediaVideoView from './media/MediaVideoView.svelte'
  import { buildMediaFact, mediaArrow, hasContent } from '../../lib/media-fact.js'
  import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'

  export let cell
  export let onChange = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused
  export let onInsert = () => {}

  // kind -> render component.
  const views = { image: MediaImageView, audio: MediaAudioView, video: MediaVideoView }

  let replacing = false
  let cropping = false

  function replace() { replacing = true; cropping = false }

  function persist() { onChange(cell) }

  function applyCrop(crop) { cell.crop = crop; cell = cell; persist(); cropping = false }
  function clearCrop() { cell.crop = null; cell = cell; persist() }

  function onPick(result, ref) {
    pickMediaAsset(cell, result, ref)
    cell = cell
    replacing = false
    cropping = false
    persist()
  }

  function onQuery(q) { cell.query = q; persist() }
  function onLang(l) { cell.lang = l; persist() }
  function onKind(k) { cell.kind = k; persist() }

  // cell.sourcePage is verbatim provider JSON (Commons' descriptionurl,
  // Openverse's foreign_landing_url) — remote-controlled text, not a trusted
  // href. A raw <a target="_blank"> also does nothing under Wails' WebKitGTK
  // frontend on Linux, which registers no handler for it. Validate the scheme
  // and hand off to the OS browser instead, matching WebSourcePane.svelte.
  function openSource() {
    if (/^https?:\/\//i.test(cell.sourcePage)) BrowserOpenURL(cell.sourcePage)
  }

  function setSubject(e) { cell.subject = e.target.value; persist() }

  function promote() {
    const text = buildMediaFact(cell)
    if (!text) return
    onInsert(text)
  }

  function onMeta(w, h) {
    cell.naturalW = w
    cell.naturalH = h
    cell = cell
    persist()
  }

  function onStart(sec) {
    cell.startSec = Math.max(0, Math.round(sec || 0))
    cell = cell
    persist()
  }
</script>

<div class="me-cell">
  {#if cell.license || (!readOnly && hasContent(cell))}
    <div class="me-bar">
      {#if cell.license}<span class="me-lic">{cell.license}</span>{/if}
      <span class="me-spacer"></span>
      {#if !readOnly && hasContent(cell) && !replacing}
        <button class="me-btn" on:click={replace} title="Pick a different asset">Replace</button>
      {/if}
      {#if !readOnly && replacing && hasContent(cell)}
        <button class="me-btn" on:click={() => (replacing = false)} title="Keep the current asset">Cancel</button>
      {/if}
    </div>
  {/if}

  {#if !hasContent(cell) || replacing}
    {#if readOnly}
      <div class="me-empty">No asset.</div>
    {:else}
      <MediaSearchPanel kind={cell.kind} query={cell.query} lang={cell.lang} {onQuery} {onLang} {onKind} {onPick} />
    {/if}
  {:else}
    <svelte:component this={views[cell.kind]} {cell} {cropping} {onMeta} {onStart} onCrop={applyCrop} />
    {#if !readOnly}
      <div class="me-tools">
        {#if cell.kind === 'image'}
          {#if cropping}
            <button class="me-btn" on:click={() => (cropping = false)}>Cancel crop</button>
          {:else}
            <button class="me-btn" on:click={() => (cropping = true)}>Crop</button>
          {/if}
          {#if cell.crop}<button class="me-btn" on:click={clearCrop}>Clear crop</button>{/if}
        {/if}
        <span class="me-spacer"></span>
        <input class="me-subj" type="text" placeholder="subject" value={cell.subject}
          on:input={setSubject} title="The N4L node this asset is about" />
        <button class="me-btn" on:click={promote} disabled={!cell.subject.trim() || !hasContent(cell)}
          title={`Insert an N4L draft using the (${mediaArrow(cell)}) arrow`}>
          Promote to N4L ({mediaArrow(cell)})
        </button>
      </div>
    {/if}
    {#if cell.sourcePage}
      <div class="me-src"><button class="me-src-btn" on:click={openSource}>source</button></div>
    {/if}
  {/if}
</div>

<style>
  .me-cell { border: 1px solid var(--border); border-radius: 6px; background: var(--surface-inset); }
  .me-bar { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid var(--border); }
  .me-lic { font-size: 11px; color: var(--text-license); }
  .me-spacer { flex: 1 1 auto; }
  .me-btn { background: var(--surface); color: var(--text-media); border: 1px solid var(--border); border-radius: 4px; padding: 2px 8px; cursor: pointer; }
  .me-btn:hover { background: var(--surface-hover); }
  .me-empty { padding: 12px; color: var(--text-quiet); font-size: 13px; }
  .me-src { padding: 0 8px 8px; font-size: 11px; }
  .me-src-btn { background: none; border: none; padding: 0; color: var(--link-text); cursor: pointer; font-size: 11px; text-decoration: underline; }
  .me-tools { display: flex; gap: 6px; padding: 0 8px 8px; }
  .me-subj { width: 140px; background: var(--surface-sunken); color: var(--text-media); border: 1px solid var(--border); border-radius: 4px; padding: 2px 6px; }
  .me-btn:disabled { opacity: 0.5; cursor: default; }
</style>
