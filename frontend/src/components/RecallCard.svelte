<script>
  export let card = null      // CardView | null
  export let onGrade = null   // (n:1..4) => void

  let revealed = false
  let lastId = null
  // Reset reveal whenever the card changes.
  $: if (card?.id !== lastId) { lastId = card?.id ?? null; revealed = false }

  const GRADES = [
    { n: 1, label: 'Again' }, { n: 2, label: 'Hard' },
    { n: 3, label: 'Good' }, { n: 4, label: 'Easy' },
  ]

  function onKey(e) {
    if (!card) return
    if (!revealed && (e.key === ' ' || e.key === 'Enter')) { e.preventDefault(); revealed = true; return }
    if (revealed && e.key >= '1' && e.key <= '4') { e.preventDefault(); onGrade?.(parseInt(e.key, 10)) }
  }
</script>

<svelte:window on:keydown={onKey} />

{#if !card}
  <div class="rc-center"><p>Nothing due. 🎉</p></div>
{:else}
  <div class="rc-center">
    <div class="rc-card">
      <div class="rc-front">{card.front}</div>
      {#if revealed}<hr /><div class="rc-back">{card.back}</div>{/if}
    </div>
    {#if !revealed}
      <button class="rc-btn" on:click={() => (revealed = true)}>Show answer (Space)</button>
    {:else}
      <div class="rc-grades">
        {#each GRADES as g}
          <button class="rc-btn" on:click={() => onGrade?.(g.n)}>{g.label} ({g.n})</button>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  .rc-center { flex: 1 1 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 18px; padding: 24px; }
  .rc-card { max-width: 640px; width: 100%; border: 1px solid #2a3a4a; border-radius: 8px; padding: 28px; background: #1b2636; text-align: center; }
  .rc-front { color: #e8eaf0; font-size: 16px; }
  .rc-back { color: #7ecfa0; font-size: 15px; }
  .rc-card hr { border: none; border-top: 1px solid #2a3a4a; margin: 16px 0; }
  .rc-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 8px 14px; cursor: pointer; font-size: 13px; }
  .rc-btn:hover { background: #244a72; }
  .rc-grades { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; }
</style>
