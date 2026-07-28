<script>
  import { mediaErrorMessage } from '../../../lib/media-error.js'
  import { buildVideoURL, parseClock, formatClock } from '../../../lib/media-fact.js'
  import { BrowserOpenURL } from '../../../../wailsjs/runtime/runtime'

  export let cell
  export let onStart = () => {}
  export let cropping = false  // unused; images only
  export let onCrop = () => {} // unused; images only
  export let onMeta = () => {} // unused; video reports no intrinsic size

  let videoEl
  let playErr = ''

  $: isFile = cell.playerKind !== 'iframe'

  // YouTube is NOT embedded. Its player refuses to configure itself in this
  // window ("Error 153"): it validates the embedding origin, and this window's
  // origin is the wails:// custom scheme, so there is no https origin to
  // present. A Referrer-Policy cannot help — that governs how much referrer is
  // sent, not what it is. So a YouTube link is offered as a link instead of a
  // broken player, opened in the real browser where it works.
  //
  // The same button is offered for ordinary video files, because fullscreen
  // renders black in this webview; opening in a browser is the way to watch
  // one full-screen. See the design doc's known-limitations section.
  $: watchURL = buildVideoURL(cell)
  $: canWatch = /^https?:\/\//i.test(watchURL)

  function openWatch() {
    if (canWatch) BrowserOpenURL(watchURL)
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
    //
    // MediaError 4 covers several causes this component cannot tell apart — an
    // unsupported codec (H.265 and AV1 commonly fail in this webview), a server
    // that does not support range requests, or a resource that would not load.
    // The remedy is the same for all of them, and it is one button away, so
    // point at it rather than leaving the user with a dead frame.
    playErr = mediaErrorMessage(e.target?.error, 'Video')
    if (canWatch) playErr += ' Try "Open in browser".'
  }

  function setStartHere() {
    if (videoEl) onStart(videoEl.currentTime)
  }

  // One-way value binding, so re-deriving on cell.startSec cannot fight the
  // keystrokes: typing changes only the input, and the field is refreshed from
  // the cell after a commit or after "Set start here".
  $: startText = formatClock(cell.startSec || 0)

  // The typed field is the ONLY way to mark a moment on a YouTube cell, whose
  // position cannot be read back from the embed — and a precise override for
  // ordinary videos. Unusable input restores the previous value rather than
  // resetting the marked moment to zero.
  function commitStart(e) {
    const sec = parseClock(e.target.value)
    if (sec === null) {
      e.target.value = formatClock(cell.startSec || 0)
      return
    }
    onStart(sec)
  }
</script>

<div class="vv-wrap">
  {#if playErr}
    <div class="vv-err">{playErr}</div>
  {/if}

  {#if isFile}
    <video bind:this={videoEl} class="vv-player" src={cell.embedURL} controls preload="metadata"
      on:loadedmetadata={onLoaded} on:error={onVideoError}></video>
  {:else if canWatch}
    <div class="vv-ext">
      <div class="vv-exttitle">{cell.title || 'YouTube video'}</div>
      <div class="vv-extnote">YouTube cannot be embedded in this window; it opens in your browser.</div>
    </div>
  {:else}
    <div class="vv-err">Not a recognised video link: {cell.embedURL}</div>
  {/if}

  <div class="vv-tools">
    {#if isFile}
      <button class="vv-btn" on:click={setStartHere} title="Mark the moment currently playing">
        Set start here
      </button>
    {/if}
    {#if canWatch}
      <button class="vv-btn" on:click={openWatch}
        title={isFile ? 'Open in your browser, at the marked start — also the way to watch fullscreen' : 'Open on YouTube at the marked start'}>
        {isFile ? 'Open in browser' : 'Watch on YouTube'}
      </button>
    {/if}
    <span class="vv-start">start</span>
    <input class="vv-time" type="text" value={startText} title="Start time — m:ss, h:mm:ss, or seconds"
      on:change={commitStart}
      on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); e.target.blur() } }} />
  </div>
</div>

<style>
  .vv-wrap { padding: 8px; }
  .vv-player { width: 100%; aspect-ratio: 16 / 9; background: #000; border: 0; border-radius: 4px; display: block; }
  /* Correct fullscreen sizing: the browser sizes the element itself, so
     width:100% plus a fixed aspect-ratio would fight it. NOTE this did NOT fix
     the black fullscreen observed in the Wails window on Linux/WSLg — that is
     an open platform issue recorded in the slice 3 design doc, and "Open in
     browser" above is the working way to watch fullscreen. The rule is kept
     because it is correct regardless. */
  .vv-player:fullscreen { width: 100%; height: 100%; aspect-ratio: auto; border-radius: 0; }
  .vv-tools { display: flex; gap: 8px; align-items: center; margin-top: 6px; }
  .vv-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 2px 8px; cursor: pointer; }
  .vv-btn:hover { background: #24334a; }
  .vv-start { font-size: 11px; color: #8aa; }
  .vv-time { width: 72px; background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 2px 6px; font-size: 12px; }
  .vv-err { padding: 8px 0; color: #d88; font-size: 13px; }
  .vv-ext { padding: 14px 12px; background: #0b1220; border: 1px solid #2a3a4a; border-radius: 4px; }
  .vv-exttitle { color: #cde; font-size: 13px; }
  .vv-extnote { color: #8aa; font-size: 11px; margin-top: 4px; }
</style>
