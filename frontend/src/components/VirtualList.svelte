<script>
  import { onMount, tick } from 'svelte'

  export let items = []
  export let estimatedHeight = 28
  export let overscan = 5
  export let getKey = (item, idx) => idx

  let viewportEl = null
  let scrollTop = 0
  let viewportH = 0
  let heights = new Map() // idx → measured px
  let itemsRef = items    // reference identity tracker for reset

  $: if (items !== itemsRef) {
    heights = new Map()
    itemsRef = items
  }

  $: rowOffsets = (() => {
    const out = new Array(items.length + 1)
    out[0] = 0
    for (let i = 0; i < items.length; i++) {
      out[i + 1] = out[i] + (heights.get(i) ?? estimatedHeight)
    }
    return out
  })()

  $: totalHeight = items.length ? rowOffsets[items.length] : 0

  $: startIdx = (() => {
    if (items.length === 0) return 0
    let lo = 0, hi = items.length
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (rowOffsets[mid + 1] <= scrollTop) lo = mid + 1
      else hi = mid
    }
    return Math.max(0, lo - overscan)
  })()

  $: endIdx = (() => {
    if (items.length === 0) return 0
    const target = scrollTop + viewportH
    let lo = 0, hi = items.length
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (rowOffsets[mid] < target) lo = mid + 1
      else hi = mid
    }
    return Math.min(items.length, lo + overscan)
  })()

  $: visibleSlice = (() => {
    const out = []
    for (let i = startIdx; i < endIdx; i++) {
      out.push({ index: i, item: items[i], top: rowOffsets[i] })
    }
    return out
  })()

  function onScroll(e) {
    scrollTop = e.target.scrollTop
  }

  let pendingFlush = false
  function flushHeights(updates) {
    let changed = false
    for (const [idx, h] of updates) {
      if (heights.get(idx) !== h) {
        heights.set(idx, h)
        changed = true
      }
    }
    if (changed) heights = heights // trigger reactivity
    pendingFlush = false
  }

  function measure(node, idx) {
    let currentIdx = idx
    let pending = null
    const ro = new ResizeObserver(() => {
      const h = node.getBoundingClientRect().height
      if (h > 0) {
        if (!pending) pending = new Map()
        pending.set(currentIdx, h)
        if (!pendingFlush) {
          pendingFlush = true
          requestAnimationFrame(() => {
            const updates = pending
            pending = null
            flushHeights(updates)
          })
        }
      }
    })
    ro.observe(node)
    // immediate first measurement
    const h0 = node.getBoundingClientRect().height
    if (h0 > 0) {
      heights.set(currentIdx, h0)
      heights = heights
    }
    return {
      update(newIdx) { currentIdx = newIdx },
      destroy() { ro.disconnect() },
    }
  }

  function updateViewportH() {
    if (viewportEl) viewportH = viewportEl.clientHeight
  }

  onMount(() => {
    updateViewportH()
    const ro = new ResizeObserver(updateViewportH)
    if (viewportEl) ro.observe(viewportEl)
    return () => ro.disconnect()
  })

  export async function scrollToIndex(i) {
    if (i < 0 || i >= items.length || !viewportEl) return
    await tick()
    const top = rowOffsets[i]
    const h = heights.get(i) ?? estimatedHeight
    const bottom = top + h
    const vh = viewportEl.clientHeight
    const cur = viewportEl.scrollTop
    if (top < cur) {
      viewportEl.scrollTop = top
    } else if (bottom > cur + vh) {
      viewportEl.scrollTop = bottom - vh
    }
  }
</script>

<div
  class="vlist-viewport"
  bind:this={viewportEl}
  on:scroll={onScroll}
>
  <div class="vlist-spacer" style="height: {totalHeight}px;">
    {#each visibleSlice as v (getKey(v.item, v.index))}
      <div
        class="vlist-row"
        style="top: {v.top}px;"
        use:measure={v.index}
      >
        <slot item={v.item} index={v.index} />
      </div>
    {/each}
  </div>
</div>

<style>
  .vlist-viewport {
    overflow-y: auto;
    overflow-x: hidden;
    height: 100%;
    width: 100%;
    position: relative;
  }

  .vlist-spacer {
    position: relative;
    width: 100%;
  }

  .vlist-row {
    position: absolute;
    left: 0;
    right: 0;
  }
</style>
