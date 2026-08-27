<script>
  import { onMount, tick, createEventDispatcher } from 'svelte'
  import { GetSourceHTML, OpenSourceURL, SetReadingProjLen } from '../../wailsjs/go/main/App'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

  export let reading
  // When set, open here instead of the reading's saved position. Used by the
  // sidebar's extract peek. parseChar reads the char=N out of it, so an anchor
  // still carrying its source path is fine.
  export let startAt = null
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

  // Rune offset of the first character at the top of the scroll viewport, in the
  // same coordinate space as runeOffset()/scrollToRune(). ReadingView pulls this on
  // leave to persist "# @progress char=N" so a web article reopens where you left it.
  export function getProgress() {
    if (!articleEl) return null
    const box = articleEl.getBoundingClientRect()
    const x = box.left + 8
    const y = box.top + 2
    let node = null, offset = 0
    if (document.caretRangeFromPoint) {
      const r = document.caretRangeFromPoint(x, y)
      if (r) { node = r.startContainer; offset = r.startOffset }
    } else if (document.caretPositionFromPoint) {
      const p = document.caretPositionFromPoint(x, y)
      if (p) { node = p.offsetNode; offset = p.offset }
    }
    if (!node || !articleEl.contains(node)) return 'char=0'
    const pre = document.createRange()
    pre.selectNodeContents(articleEl)
    pre.setEnd(node, offset)
    return 'char=' + [...pre.toString()].length
  }

  function parseChar(pos) {
    const m = /char=(\d+)/.exec(pos || '')
    return m ? parseInt(m[1], 10) : 0
  }

  // Scroll the article so the rune at runeIdx (char=N, same coordinate space as
  // runeOffset()) sits at the top of the viewport. The rendered article spans many
  // text nodes, so walk them in document order accumulating rune counts to find the
  // node + UTF-16 offset for runeIdx, then align that range's top to the container's.
  function scrollToRune(runeIdx) {
    if (!articleEl || runeIdx <= 0) return
    const walker = document.createTreeWalker(articleEl, NodeFilter.SHOW_TEXT)
    let acc = 0
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const arr = [...node.textContent]
      if (acc + arr.length > runeIdx) {
        const within = runeIdx - acc
        const u = arr.slice(0, within).join('').length     // UTF-16 start offset
        const end = arr.slice(0, within + 1).join('').length // spans the full rune
        const range = document.createRange()
        range.setStart(node, u)
        range.setEnd(node, end)
        const rects = range.getClientRects()
        const top = (rects.length ? rects[0] : range.getBoundingClientRect()).top
        articleEl.scrollTop += top - articleEl.getBoundingClientRect().top
        return
      }
      acc += arr.length
    }
  }

  async function openOriginal() {
    try { await OpenSourceURL(reading.id) } catch (e) { error = String(e?.message ?? e) }
  }

  // Anchors in the {@html} article would otherwise navigate the whole Wails webview
  // away from the app (no back, and it can strip the window chrome). Intercept clicks
  // and hand http(s) links to the system browser; swallow the rest.
  function onArticleClick(ev) {
    const a = ev.target.closest && ev.target.closest('a[href]')
    if (!a) return
    ev.preventDefault()
    const href = a.getAttribute('href') || ''
    if (/^https?:\/\//i.test(href)) BrowserOpenURL(href)
  }

  onMount(async () => {
    try { html = (await GetSourceHTML(reading.refPath)) ?? '' }
    catch (e) { error = String(e?.message ?? e) }
    await tick()
    if (articleEl) {
      try { await SetReadingProjLen(reading.id, [...(articleEl.textContent ?? '')].length) } catch { /* best-effort */ }
      scrollToRune(parseChar(startAt ?? reading.position))
    }
  })
</script>

<div class="web-root">
  <div class="web-bar">
    {#if reading.sourceUrl}<button class="web-open" on:click={openOriginal} title={reading.sourceUrl}>↗ Open original</button>{/if}
  </div>
  {#if error}<div class="web-err">{error}</div>{/if}
  <!-- html is pre-sanitized at import (no script/handlers/remote refs); rendered inert. -->
  <!-- Not an interactive control: this delegates clicks from the <a> elements
       inside the rendered article so external links open in the browser.
       Keyboard activation already works — Enter on a focused link fires a
       native click that bubbles to here. -->
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <div class="web-article" bind:this={articleEl} on:click={onArticleClick}>{@html html}</div>
</div>

<style>
  .web-root { flex: 1; display: flex; flex-direction: column; min-width: 0; }
  .web-bar { flex: 0 0 auto; padding: 6px 12px; border-bottom: 1px solid var(--border); }
  .web-open { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .web-err { color: var(--danger); padding: 6px 12px; font-size: 12px; }
  .web-article { flex: 1; overflow: auto; padding: 20px 28px; color: var(--text-article); line-height: 1.7; user-select: text; }
  .web-article :global(img) { max-width: 100%; height: auto; }
  .web-article :global(h1), .web-article :global(h2), .web-article :global(h3) { color: var(--text); }
  .web-article :global(a) { color: var(--accent-text); }
  .web-article :global(pre), .web-article :global(code) { background: var(--surface-bar); border-radius: 4px; }
</style>
