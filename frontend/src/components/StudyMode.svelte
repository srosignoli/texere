<script>
  import { onMount } from 'svelte'
  import { SyncStudyItems, GetDueQueue, GradeItem } from '../../wailsjs/go/main/App'
  import RecallCard from './RecallCard.svelte'

  export let onClose = null

  let card = null
  let loading = true
  let error = ''
  let remaining = 0
  let shownAt = 0

  async function load() {
    loading = true; error = ''
    try {
      await SyncStudyItems()
      const q = await GetDueQueue(50)
      remaining = q?.length ?? 0
      card = q && q.length ? q[0] : null
    } catch (e) { error = String(e?.message ?? e) }
    finally { loading = false }
    if (card) shownAt = Date.now()
  }

  async function grade(n) {
    if (!card) return
    const elapsed = Date.now() - shownAt
    try {
      const next = await GradeItem(card.id, n, elapsed)
      card = next ?? null
      remaining = card ? Math.max(0, remaining - 1) : 0
    } catch (e) { error = String(e?.message ?? e) }
    if (card) shownAt = Date.now()
  }

  function onKey(e) { if (e.key === 'Escape') onClose?.() }
  onMount(load)
</script>

<svelte:window on:keydown={onKey} />

<div class="study-root" role="dialog" aria-label="Study">
  <div class="study-bar">
    <span class="study-title">Recall</span>
    <span class="study-count">{remaining} due</span>
    <button class="study-close" on:click={() => onClose?.()} title="Close">×</button>
  </div>
  {#if loading}
    <div class="study-center"><p>Loading…</p></div>
  {:else if error}
    <div class="study-center"><p class="study-error">{error}</p></div>
  {:else}
    <RecallCard {card} onGrade={grade} />
  {/if}
</div>

<style>
  .study-root { position: absolute; inset: 0; z-index: 60; display: flex; flex-direction: column; background: var(--surface-study); }
  .study-bar { flex: 0 0 auto; display: flex; align-items: center; gap: 12px; padding: 8px 12px; border-bottom: 1px solid var(--border); background: var(--surface-bar); }
  .study-title { color: var(--accent-text); font-weight: 600; }
  .study-count { color: var(--text-quiet); font-size: 12px; }
  .study-close { margin-left: auto; background: none; border: none; color: var(--text-muted); font-size: 20px; line-height: 1; cursor: pointer; }
  .study-close:hover { color: var(--text); }
  .study-center { flex: 1 1 0; display: flex; align-items: center; justify-content: center; }
  .study-error { color: var(--danger); }
</style>
