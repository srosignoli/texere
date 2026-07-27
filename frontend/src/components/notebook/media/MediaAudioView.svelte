<script>
  import { mediaErrorMessage } from '../../../lib/media-error.js'

  export let cell
  export let cropping = false  // unused; images only
  export let onCrop = () => {} // unused; images only
  export let onMeta = () => {} // unused; audio has no intrinsic size

  let missing = false
  let playErr = ''

  $: src = `/media/${cell.asset}`

  // Shared with the search preview so the wording cannot drift. It separates a
  // decode failure (WebKitGTK/WSLg often lacks .ogg codecs) from a load
  // failure, because "not found" would send the user after the wrong problem.
  function onAudioError(e) {
    missing = true
    playErr = mediaErrorMessage(e.target?.error)
  }
</script>

{#if missing}
  <div class="av-err">{playErr || `audio not found: ${cell.asset}`}</div>
{:else}
  <div class="av-wrap">
    <audio class="av-player" {src} controls on:error={onAudioError}></audio>
  </div>
{/if}

<style>
  .av-wrap { padding: 8px; }
  .av-player { width: 100%; }
  .av-err { padding: 12px; color: #d88; font-size: 13px; }
</style>
