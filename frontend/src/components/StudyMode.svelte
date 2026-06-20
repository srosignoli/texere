<script>
  import { onMount } from 'svelte'
  import { SyncStudyItems, GetDueQueue, GradeItem } from '../../wailsjs/go/main/App'

  export let onClose = null

  let card = null        // current CardView or null
  let revealed = false
  let loading = true
  let error = ''
  let remaining = 0
  let shownAt = 0

  const GRADES = [
    { n: 1, label: 'Again' },
    { n: 2, label: 'Hard' },
    { n: 3, label: 'Good' },
    { n: 4, label: 'Easy' },
  ]

  async function load() {
    loading = true
    error = ''
    try {
      await SyncStudyItems()
      const q = await GetDueQueue(50)
      remaining = q?.length ?? 0
      card = q && q.length ? q[0] : null
    } catch (e) {
      error = String(e?.message ?? e)
    } finally {
      loading = false
      revealed = false
      shownAt = Date.now()
    }
  }

  async function grade(n) {
    if (!card) return
    const elapsed = Date.now() - shownAt
    try {
      const next = await GradeItem(card.id, n, elapsed)
      card = next ?? null
      remaining = card ? Math.max(0, remaining - 1) : 0
    } catch (e) {
      error = String(e?.message ?? e)
    }
    revealed = false
    shownAt = Date.now()
  }

  function onKey(e) {
    if (e.key === 'Escape') { onClose?.(); return }
    if (!card) return
    if (!revealed && (e.key === ' ' || e.key === 'Enter')) {
      e.preventDefault(); revealed = true; return
    }
    if (revealed && e.key >= '1' && e.key <= '4') {
      e.preventDefault(); grade(parseInt(e.key, 10))
    }
  }

  onMount(load)
</script>

<svelte:window on:keydown={onKey} />

<div class="study-root" role="dialog" aria-label="Study">
  <div class="study-bar">
    <span class="study-title">Recall</span>
    <span class="study-count">{remaining} due</span>
    <button class="study-close" on:click={() => onClose?.()} title="Close (Esc)">×</button>
  </div>

  {#if loading}
    <div class="study-center"><p>Loading…</p></div>
  {:else if error}
    <div class="study-center"><p class="study-error">{error}</p></div>
  {:else if !card}
    <div class="study-center"><p>Nothing due. 🎉</p></div>
  {:else}
    <div class="study-center">
      <div class="study-card">
        <div class="study-front">{card.front}</div>
        {#if revealed}
          <hr />
          <div class="study-back">{card.back}</div>
        {/if}
      </div>
      {#if !revealed}
        <button class="study-reveal" on:click={() => (revealed = true)}>Show answer (Space)</button>
      {:else}
        <div class="study-grades">
          {#each GRADES as g}
            <button class="study-grade" on:click={() => grade(g.n)}>{g.label} ({g.n})</button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .study-root {
    position: absolute; inset: 0; z-index: 60;
    display: flex; flex-direction: column;
    background: #131c29;
  }
  .study-bar {
    flex: 0 0 auto; display: flex; align-items: center; gap: 12px;
    padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #161f2e;
  }
  .study-title { color: #7dd3fc; font-weight: 600; }
  .study-count { color: #8aa; font-size: 12px; }
  .study-close {
    margin-left: auto; background: none; border: none; color: #8899aa;
    font-size: 20px; line-height: 1; cursor: pointer;
  }
  .study-close:hover { color: #e8eaf0; }
  .study-center { flex: 1 1 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 18px; padding: 24px; }
  .study-card { max-width: 640px; width: 100%; border: 1px solid #2a3a4a; border-radius: 8px; padding: 28px; background: #1b2636; text-align: center; }
  .study-front { color: #e8eaf0; font-size: 16px; }
  .study-back { color: #7ecfa0; font-size: 15px; }
  .study-card hr { border: none; border-top: 1px solid #2a3a4a; margin: 16px 0; }
  .study-reveal, .study-grade {
    background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a;
    border-radius: 6px; padding: 8px 14px; cursor: pointer; font-size: 13px;
  }
  .study-reveal:hover, .study-grade:hover { background: #244a72; }
  .study-grades { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; }
  .study-error { color: #ff8080; }
</style>
