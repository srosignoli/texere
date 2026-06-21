<script>
  import { onMount, createEventDispatcher } from 'svelte'
  import { GetSourceProjection } from '../../wailsjs/go/main/App'

  export let reading
  const dispatch = createEventDispatcher()

  let projection = ''
  let srcEl
  let error = ''

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

  onMount(async () => {
    try { projection = (await GetSourceProjection(reading.refPath)) ?? '' }
    catch (e) { error = String(e?.message ?? e) }
  })
</script>

<div class="tsp" bind:this={srcEl}>{#if error}<span class="tsp-err">{error}</span>{:else}{projection}{/if}</div>

<style>
  .tsp { flex: 1; overflow: auto; padding: 16px 20px; white-space: pre-wrap; color: #cdd6e0;
    font-size: 14px; line-height: 1.6; user-select: text; }
  .tsp-err { color: #ff8080; }
</style>
