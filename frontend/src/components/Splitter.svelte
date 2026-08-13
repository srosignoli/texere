<script>
  import { createEventDispatcher } from 'svelte'

  /**
   * Reusable drag handle for resizing a sibling element.
   *
   * Receives the current `size` (read-only) and dispatches a `resize` CustomEvent
   * with the new pixel value on every pointermove and key-nudge. The parent owns
   * the canonical state (typically a writable store) and updates it from the
   * event handler — this avoids the fragile two-way `bind:size` round-trip via
   * reactive proxy variables.
   *
   * - orientation="horizontal" → drags up/down (cursor: ns-resize)
   * - orientation="vertical"   → drags left/right (cursor: ew-resize)
   */
  export let orientation = 'vertical' // 'vertical' | 'horizontal'
  export let size = 0                  // read-only — current value driven by parent
  export let min = 0
  export let max = Infinity

  const dispatch = createEventDispatcher()

  let dragging = false
  let startCoord = 0
  let startSize = 0

  const NUDGE_PX = 8

  function clamp(v) {
    if (v < min) return min
    if (v > max) return max
    return v
  }

  function coordOf(e) {
    return orientation === 'vertical' ? e.clientX : e.clientY
  }

  function emit(next) {
    const clamped = clamp(next)
    if (clamped !== size) {
      dispatch('resize', clamped)
    }
  }

  function onPointerDown(e) {
    if (e.button !== 0) return
    e.preventDefault()
    dragging = true
    startCoord = coordOf(e)
    startSize = size
    // Use window listeners (setPointerCapture was observed to silently drop
    // pointermove events when the cursor left the 1-px handle in embedded
    // webviews). pointercancel covers context-menu / system interrupts.
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerUp)
    document.body.style.userSelect = 'none'
    document.body.style.cursor = orientation === 'vertical' ? 'ew-resize' : 'ns-resize'
  }

  function onPointerMove(e) {
    if (!dragging) return
    const delta = coordOf(e) - startCoord
    emit(startSize + delta)
  }

  function onPointerUp() {
    if (!dragging) return
    dragging = false
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerUp)
    document.body.style.userSelect = ''
    document.body.style.cursor = ''
  }

  function onKeyDown(e) {
    const dec = orientation === 'vertical' ? 'ArrowLeft' : 'ArrowUp'
    const inc = orientation === 'vertical' ? 'ArrowRight' : 'ArrowDown'
    if (e.key === dec) {
      e.preventDefault()
      emit(size - NUDGE_PX)
    } else if (e.key === inc) {
      e.preventDefault()
      emit(size + NUDGE_PX)
    }
  }
</script>

<!-- svelte-ignore a11y-no-noninteractive-tabindex -->
<div
  class="splitter"
  class:vertical={orientation === 'vertical'}
  class:horizontal={orientation === 'horizontal'}
  class:dragging
  role="separator"
  aria-orientation={orientation}
  aria-valuenow={size}
  aria-valuemin={min}
  aria-valuemax={max === Infinity ? undefined : max}
  tabindex="0"
  on:pointerdown={onPointerDown}
  on:keydown={onKeyDown}
>
  <span class="line" />
</div>

<style>
  .splitter {
    position: relative;
    flex-shrink: 0;
    background: transparent;
    z-index: 5;
    touch-action: none;
  }
  .splitter.vertical {
    /* 5-px wide grab zone, with a 1-px divider line painted via .line */
    width: 5px;
    cursor: ew-resize;
    align-self: stretch;
  }
  .splitter.horizontal {
    height: 5px;
    cursor: ns-resize;
    width: 100%;
  }

  /* Always-visible 1-px divider line in the standard panel-border colour.
     Sits centred inside the wider grab area so hit-testing stays generous
     while the visual stays a thin line. */
  .splitter > .line {
    position: absolute;
    background: var(--border);
    transition: background 0.12s ease;
    pointer-events: none;
  }
  .splitter.vertical > .line {
    top: 0;
    bottom: 0;
    left: 2px;
    width: 1px;
  }
  .splitter.horizontal > .line {
    left: 0;
    right: 0;
    top: 2px;
    height: 1px;
  }

  /* Brighten the divider on hover / focus / drag for clear affordance. */
  .splitter:hover > .line,
  .splitter:focus-visible > .line,
  .splitter.dragging > .line {
    background: #4a7aaa;
  }
  .splitter.vertical:hover > .line,
  .splitter.vertical.dragging > .line {
    width: 3px;
    left: 1px;
  }
  .splitter.horizontal:hover > .line,
  .splitter.horizontal.dragging > .line {
    height: 3px;
    top: 1px;
  }

  .splitter:focus-visible {
    outline: none;
  }
</style>
