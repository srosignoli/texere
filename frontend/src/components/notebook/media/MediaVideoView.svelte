<script>
  import { mediaErrorMessage } from '../../../lib/media-error.js'

  export let cell
  export let onStart = () => {}
  export let cropping = false  // unused; images only
  export let onCrop = () => {} // unused; images only
  export let onMeta = () => {} // unused; video reports no intrinsic size

  let videoEl
  let playErr = ''

  $: isFile = cell.playerKind !== 'iframe'
  // youtube-nocookie avoids setting tracking cookies until playback starts.
  // Build the embed URL only when an id was actually extracted. Returning the
  // bare prefix would keep this string truthy and make the "not recognised"
  // branch below unreachable, silently rendering an iframe with no video.
  $: ytId = cell.playerKind === 'iframe' ? youtubeId(cell.embedURL) : ''
  $: embedSrc = ytId
    ? `https://www.youtube-nocookie.com/embed/${ytId}${cell.startSec ? `?start=${cell.startSec}` : ''}`
    : ''

  // Accepts both watch?v=<id> and youtu.be/<id> forms.
  function youtubeId(url) {
    const m = String(url ?? '').match(/(?:youtube\.com\/.*[?&]v=|youtu\.be\/)([A-Za-z0-9_-]{6,})/)
    return m ? m[1] : ''
  }

  // Resume at the marked moment, so reopening a cell lands where it left off.
  function onLoaded() {
    playErr = ''
    if (cell.startSec && videoEl && Math.abs(videoEl.currentTime - cell.startSec) > 1) {
      videoEl.currentTime = cell.startSec
    }
  }

  function onVideoError(e) {
    // Referencing rather than storing means a removed upstream video is a real
    // outcome; say so rather than showing an empty frame.
    playErr = mediaErrorMessage(e.target?.error, 'Video')
  }

  function setStartHere() {
    if (videoEl) onStart(videoEl.currentTime)
  }
</script>

<div class="vv-wrap">
  {#if playErr}
    <div class="vv-err">{playErr}</div>
  {/if}

  {#if isFile}
    <video bind:this={videoEl} class="vv-player" src={cell.embedURL} controls preload="metadata"
      on:loadedmetadata={onLoaded} on:error={onVideoError}></video>
  {:else if embedSrc}
    <iframe class="vv-player" src={embedSrc} title={cell.title || 'video'}
      allow="fullscreen; encrypted-media" frameborder="0"></iframe>
  {:else}
    <div class="vv-err">Not a recognised video link: {cell.embedURL}</div>
  {/if}

  <div class="vv-tools">
    {#if isFile}
      <button class="vv-btn" on:click={setStartHere} title="Mark the moment currently playing">
        Set start here
      </button>
    {/if}
    <span class="vv-start">start {cell.startSec || 0}s</span>
  </div>
</div>

<style>
  .vv-wrap { padding: 8px; }
  .vv-player { width: 100%; aspect-ratio: 16 / 9; background: #000; border: 0; border-radius: 4px; display: block; }
  .vv-tools { display: flex; gap: 8px; align-items: center; margin-top: 6px; }
  .vv-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 2px 8px; cursor: pointer; }
  .vv-btn:hover { background: #24334a; }
  .vv-start { font-size: 11px; color: #8aa; }
  .vv-err { padding: 8px 0; color: #d88; font-size: 13px; }
</style>
