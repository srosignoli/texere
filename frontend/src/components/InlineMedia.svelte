<script>
  // Renders one referenced asset in the Preview tab, or nothing when the path is
  // not media.
  //
  // Shares mediaKind and srcFor with the notes editor's inline media, so the
  // two views agree on what counts as media and on how a path becomes a URL.
  // NodeCard previously had its own extension regexes (no video at all) and
  // built `${collectionDir}/${path}` — a filesystem path, which a webview
  // cannot load.
  import { mediaKind, srcFor } from '../lib/n4l-media.js'

  export let path

  $: kind = mediaKind(path)
  $: src = srcFor(path)
</script>

{#if kind === 'image'}
  <img class="inline-media" {src} alt={path} loading="lazy" />
{:else if kind === 'video'}
  <!-- svelte-ignore a11y-media-has-caption -->
  <video class="inline-media" controls preload="metadata" {src}></video>
{:else if kind === 'audio'}
  <audio class="inline-audio" controls preload="metadata" {src}></audio>
{/if}

<style>
  .inline-media {
    display: block;
    max-width: 100%;
    max-height: 240px;
    border-radius: 4px;
    margin: 6px 0;
  }

  .inline-audio {
    display: block;
    width: 100%;
    max-width: 420px;
    margin: 6px 0;
  }
</style>
