<script>
  import { onMount, tick, createEventDispatcher } from 'svelte'
  import { GetFileContent, OpenSourceURL, SetReadingProjLen } from '../../wailsjs/go/main/App'

  export let reading
  const dispatch = createEventDispatcher()

  let html = ''
  let articleEl
  let error = ''

  // web char=N offsets are rune offsets into the RENDERED article text (this pane's coordinate
  // space), distinct from text-medium source.Projection offsets; cross-medium relocation is Phase 3.
  function runeOffset() {
    const sel = window.getSelection()
    if (!sel || sel.rangeCount === 0 || !articleEl.contains(sel.anchorNode)) return null
    const range = sel.getRangeAt(0)
    const pre = range.cloneRange()
    pre.selectNodeContents(articleEl)
    pre.setEnd(range.startContainer, range.startOffset)
    return { offset: [...pre.toString()].length, text: sel.toString() }
  }

  export function requestExtract() {
    const sel = runeOffset()
    if (!sel || !sel.text.trim()) return
    dispatch('extract', { text: sel.text, position: 'char=' + sel.offset })
  }

  async function openOriginal() {
    try { await OpenSourceURL(reading.id) } catch (e) { error = String(e?.message ?? e) }
  }

  onMount(async () => {
    try { html = (await GetFileContent(reading.refPath)) ?? '' }
    catch (e) { error = String(e?.message ?? e) }
    await tick()
    if (articleEl) {
      try { await SetReadingProjLen(reading.id, [...(articleEl.textContent ?? '')].length) } catch { /* best-effort */ }
    }
  })
</script>

<div class="web-root">
  <div class="web-bar">
    {#if reading.sourceUrl}<button class="web-open" on:click={openOriginal} title={reading.sourceUrl}>↗ Open original</button>{/if}
  </div>
  {#if error}<div class="web-err">{error}</div>{/if}
  <!-- html is pre-sanitized at import (no script/handlers/remote refs); rendered inert. -->
  <div class="web-article" bind:this={articleEl}>{@html html}</div>
</div>

<style>
  .web-root { flex: 1; display: flex; flex-direction: column; min-width: 0; }
  .web-bar { flex: 0 0 auto; padding: 6px 12px; border-bottom: 1px solid #2a3a4a; }
  .web-open { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .web-err { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .web-article { flex: 1; overflow: auto; padding: 20px 28px; color: #d6dde6; line-height: 1.7; user-select: text; }
  .web-article :global(img) { max-width: 100%; height: auto; }
  .web-article :global(h1), .web-article :global(h2), .web-article :global(h3) { color: #e8eaf0; }
  .web-article :global(a) { color: #7dd3fc; }
  .web-article :global(pre), .web-article :global(code) { background: #161f2e; border-radius: 4px; }
</style>
