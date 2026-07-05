<script>
  import { onMount, tick, createEventDispatcher } from 'svelte'
  import { GetSourceProjection } from '../../wailsjs/go/main/App'

  export let reading
  const dispatch = createEventDispatcher()

  let projection = ''
  let srcEl
  let error = ''

  function parseChar(pos) {
    const m = /char=(\d+)/.exec(pos || '')
    return m ? parseInt(m[1], 10) : 0
  }

  // The projection renders as a single text node; find it (skipping the Svelte
  // {#if} comment anchors around it).
  function textNode() {
    if (!srcEl) return null
    for (const n of srcEl.childNodes) {
      if (n.nodeType === 3 && n.textContent.length) return n // Node.TEXT_NODE
    }
    return null
  }

  // Visual top (viewport px) of the rune at runeIdx. Range offsets are UTF-16
  // code units, but our char=N anchors are rune offsets, so convert first.
  function runeTop(node, arr, runeIdx) {
    const clamped = Math.max(0, Math.min(runeIdx, arr.length))
    let u = arr.slice(0, clamped).join('').length
    let end = u + 1
    if (end > node.length) { end = node.length; u = Math.max(0, end - 1) }
    const range = document.createRange()
    range.setStart(node, u)
    range.setEnd(node, end)
    const rects = range.getClientRects()
    return (rects.length ? rects[0] : range.getBoundingClientRect()).top
  }

  function charOffset() {
    const sel = window.getSelection()
    if (!sel || sel.rangeCount === 0 || !srcEl.contains(sel.anchorNode)) return null
    const range = sel.getRangeAt(0)
    const pre = range.cloneRange()
    pre.selectNodeContents(srcEl)
    pre.setEnd(range.startContainer, range.startOffset)
    return { offset: [...pre.toString()].length, text: sel.toString() }
  }

  export function requestExtract() {
    const sel = charOffset()
    if (!sel || !sel.text.trim()) return
    dispatch('extract', { text: sel.text, position: 'char=' + sel.offset })
  }

  // Rune offset of the first character at the top of the scroll viewport. The
  // parent pulls this on leave to persist "# @progress char=N".
  export function getProgress() {
    const node = textNode()
    if (!node) return null
    const arr = [...node.textContent]
    if (arr.length === 0) return 'char=0'
    const top = srcEl.getBoundingClientRect().top
    // Largest offset whose visual top is still at/above the viewport top edge.
    let lo = 0, hi = arr.length
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1
      if (runeTop(node, arr, mid) <= top + 1) lo = mid
      else hi = mid - 1
    }
    return 'char=' + lo
  }

  function scrollToOffset(runeIdx) {
    const node = textNode()
    if (!node || runeIdx <= 0) return
    const arr = [...node.textContent]
    const containerTop = srcEl.getBoundingClientRect().top
    srcEl.scrollTop += runeTop(node, arr, runeIdx) - containerTop
  }

  onMount(async () => {
    try { projection = (await GetSourceProjection(reading.refPath)) ?? '' }
    catch (e) { error = String(e?.message ?? e) }
    await tick()
    scrollToOffset(parseChar(reading.position))
  })
</script>

<div class="tsp" bind:this={srcEl}>{#if error}<span class="tsp-err">{error}</span>{:else}{projection}{/if}</div>

<style>
  .tsp { flex: 1; overflow: auto; padding: 16px 20px; white-space: pre-wrap; color: #cdd6e0;
    font-size: 14px; line-height: 1.6; user-select: text; }
  .tsp-err { color: #ff8080; }
</style>
