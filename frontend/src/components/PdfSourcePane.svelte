<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import * as pdfjsLib from 'pdfjs-dist'
  import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
  import { GetSourceBytes, SetReadingProjLen } from '../../wailsjs/go/main/App'

  pdfjsLib.GlobalWorkerOptions.workerSrc = workerUrl

  export let reading
  const dispatch = createEventDispatcher()

  let pdf = null
  let pageNum = 1
  let total = 0
  let error = ''
  let canvas
  let textLayerEl

  function parsePage(pos) {
    const m = /page=(\d+)/.exec(pos || '')
    return m ? parseInt(m[1], 10) : 1
  }

  function base64ToBytes(b64) {
    const bin = atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
    return arr
  }

  async function renderPage(n) {
    if (!pdf || n < 1 || n > total) return
    pageNum = n
    const page = await pdf.getPage(n)
    const viewport = page.getViewport({ scale: 1.4 })
    canvas.width = viewport.width
    canvas.height = viewport.height
    const ctx = canvas.getContext('2d')
    await page.render({ canvasContext: ctx, viewport }).promise
    // Selectable text layer aligned over the canvas.
    textLayerEl.innerHTML = ''
    textLayerEl.style.width = viewport.width + 'px'
    textLayerEl.style.height = viewport.height + 'px'
    const textContent = await page.getTextContent()
    const tl = new pdfjsLib.TextLayer({ textContentSource: textContent, container: textLayerEl, viewport })
    await tl.render()
    dispatch('progress', { position: 'page=' + pageNum })
  }

  export function requestExtract() {
    const sel = window.getSelection()
    if (!sel || !textLayerEl.contains(sel.anchorNode)) return
    const text = sel.toString()
    if (!text.trim()) return
    dispatch('extract', { text, position: 'page=' + pageNum })
  }

  function prev() { renderPage(pageNum - 1) }
  function next() { renderPage(pageNum + 1) }

  onMount(async () => {
    try {
      const b64 = await GetSourceBytes(reading.refPath)
      pdf = await pdfjsLib.getDocument({ data: base64ToBytes(b64) }).promise
      total = pdf.numPages
      try { await SetReadingProjLen(reading.id, total) } catch { /* best-effort */ }
      const start = Math.min(Math.max(1, parsePage(reading.position)), total)
      await renderPage(start)
    } catch (e) { error = String(e?.message ?? e) }
  })

  onDestroy(() => { pdf?.destroy?.() })
</script>

<div class="pdf-root">
  <div class="pdf-bar">
    <button class="pdf-nav" on:click={prev} disabled={pageNum <= 1}>‹</button>
    <span class="pdf-page">{pageNum} / {total || '–'}</span>
    <button class="pdf-nav" on:click={next} disabled={pageNum >= total}>›</button>
  </div>
  {#if error}<div class="pdf-err">{error}</div>{/if}
  <div class="pdf-scroll">
    <div class="pdf-page-wrap">
      <canvas bind:this={canvas}></canvas>
      <div class="pdf-textlayer" bind:this={textLayerEl}></div>
    </div>
  </div>
</div>

<style>
  .pdf-root { flex: 1; display: flex; flex-direction: column; min-width: 0; background: #1b2230; }
  .pdf-bar { flex: 0 0 auto; display: flex; align-items: center; gap: 10px; justify-content: center; padding: 6px; border-bottom: 1px solid #2a3a4a; }
  .pdf-nav { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 10px; cursor: pointer; }
  .pdf-nav:disabled { opacity: .4; cursor: default; }
  .pdf-page { color: #8aa; font-size: 12px; }
  .pdf-err { color: #ff8080; padding: 8px 12px; font-size: 12px; }
  .pdf-scroll { flex: 1; overflow: auto; display: flex; justify-content: center; padding: 12px; }
  .pdf-page-wrap { position: relative; }
  .pdf-textlayer { position: absolute; top: 0; left: 0; overflow: hidden; opacity: 0.25; line-height: 1; }
  .pdf-textlayer :global(span) { color: transparent; position: absolute; white-space: pre; cursor: text; }
  .pdf-textlayer :global(::selection) { background: #7dd3fc; }
</style>
