<script>
  // Four panels over one payload, loaded when the tab is entered. The whole
  // Stats focus is a single GetStatistics call so the panels can never disagree
  // about the moment they describe.
  import { onMount } from 'svelte'
  import { GetStatistics } from '../../wailsjs/go/main/App'
  import { barHeights, sparkPath, humanDuration, fiBand } from '../lib/stats-format.js'

  let s = null
  let error = ''

  // No refresh() export: switching focus destroys and recreates this component,
  // so onMount re-runs and the numbers are fresh on every return to the tab.
  // An exported refresh nobody calls is just a claim that something reloads.
  onMount(load)

  async function load() {
    error = ''
    try { s = await GetStatistics() } catch (e) { error = String(e?.message ?? e) }
  }

  // -1 is the "never measured" sentinel from the Go side. It must never be
  // rendered: 0% would read as a flawless collection nobody has studied.
  const has = (v) => typeof v === 'number' && v >= 0
  // A chart over a log that started an hour ago would draw twelve weeks of
  // zeros and read as "you did nothing", when the truth is "we only just began
  // recording". Below two days it says so, and names the day it started.
  const settled = (unix) => unix > 0 && Date.now() / 1000 - unix >= 2 * 86400
  const sinceDate = (unix) => new Date(unix * 1000).toLocaleDateString()
  const pct = (v) => `${Math.round(v)}%`

  const gradeLabels = ['Again', 'Hard', 'Good', 'Easy']
</script>

<div class="sp-root">
  {#if error}
    <div class="sp-error">{error}</div>
  {:else if !s}
    <div class="sp-loading">Loading statistics…</div>
  {:else}
    <div class="sp-grid">
      <section class="sp-card">
        <h3>Learning quality</h3>
        {#if has(s.quality.forgettingIndex)}
          <p class="sp-metric" class:ok={fiBand(s.quality.forgettingIndex) === 'ok'}
             class:low={fiBand(s.quality.forgettingIndex) === 'low'}
             class:high={fiBand(s.quality.forgettingIndex) === 'high'}>
            Forgetting index: {pct(s.quality.forgettingIndex)}
          </p>
        {:else}
          <p class="sp-metric sp-nodata">Forgetting index: Not enough data yet</p>
        {/if}
        {#if has(s.quality.retention)}
          <p class="sp-sub">Retention: {pct(s.quality.retention)}</p>
        {:else}
          <p class="sp-sub sp-nodata">Retention: Not enough data yet</p>
        {/if}
        <div class="sp-chartrow">
          <svg width="80" height="60" viewBox="0 0 80 60" class="sp-chart">
            {#each barHeights(s.quality.gradeMix, 60) as h, i}
              <rect x={i * 20 + 2} y={60 - h} width="14" height={h} fill="#7dd3fc" />
            {/each}
          </svg>
          <div class="sp-chartlabels">
            {#each gradeLabels as label}<span>{label}</span>{/each}
          </div>
        </div>
        <p class="sp-sub">Leeches: {s.workload.leeches}</p>
        <svg width="100" height="40" viewBox="0 0 100 40" class="sp-chart">
          {#each barHeights(s.workload.difficulty, 40) as h, i}
            <rect x={i * 20 + 2} y={40 - h} width="14" height={h} fill="#e0b000" />
          {/each}
        </svg>
      </section>

      <section class="sp-card">
        <h3>Workload</h3>
        {#if has(s.workload.burden)}
          <p class="sp-metric">Burden: {s.workload.burden.toFixed(1)}</p>
        {:else}
          <p class="sp-metric sp-nodata">Burden: Not enough data yet</p>
        {/if}
        {#if has(s.workload.dailyMinutes)}
          <p class="sp-sub">Daily time: {humanDuration(s.workload.dailyMinutes)}</p>
        {:else}
          <p class="sp-sub sp-nodata">Daily time: Not enough data yet</p>
        {/if}
        <svg width="120" height="60" viewBox="0 0 120 60" class="sp-chart">
          {#each barHeights(s.workload.forecast, 60) as h, i}
            <rect x={i * 8 + 1} y={60 - h} width="6" height={h} fill="#7dd3fc" />
          {/each}
        </svg>
        <p class="sp-sub">Backlog — Read: {s.workload.backlog.read}, Process: {s.workload.backlog.process}, Recall: {s.workload.backlog.recall}</p>
      </section>

      <section class="sp-card">
        <h3>Habit</h3>
        <svg width="260" height="40" viewBox="0 0 260 40" class="sp-chart">
          <path d={sparkPath((s.habit.reviewsPerDay ?? []).map((d) => d?.count ?? 0), 260, 40)}
                fill="none" stroke="#7ecfa0" stroke-width="1.5" />
        </svg>
        <p class="sp-sub">Current streak: {s.habit.currentStreak} · Longest streak: {s.habit.longestStreak}</p>
        <p class="sp-sub">Memorized: {s.workload.memorized}</p>
        <svg width="120" height="40" viewBox="0 0 120 40" class="sp-chart">
          {#each barHeights(s.cardsPerWeek, 40) as h, i}
            <rect x={i * 10 + 1} y={40 - h} width="7" height={h} fill="#7dd3fc" />
          {/each}
        </svg>
        {#if settled(s.readSince)}
          <svg width="120" height="40" viewBox="0 0 120 40" class="sp-chart">
            {#each barHeights(s.readMinsWeek, 40) as h, i}
              <rect x={i * 10 + 1} y={40 - h} width="7" height={h} fill="#7ecfa0" />
            {/each}
          </svg>
        {:else}
          <p class="sp-sub sp-nodata">
            {s.readSince ? `Collecting reading time since ${sinceDate(s.readSince)}` : 'Collecting reading time since today'}
          </p>
        {/if}
      </section>

      <section class="sp-card">
        <h3>Pipeline health</h3>
        <p class="sp-sub">Sources: {s.pipeline.sourcesDone} / {s.pipeline.sources}</p>
        <p class="sp-sub">Extracts: {s.pipeline.extractsOpen} open / {s.pipeline.extracts} total</p>
        <p class="sp-sub">Cards: {s.pipeline.cards}</p>
        {#if has(s.pipeline.oldestOpenDays)}
          <p class="sp-sub">Oldest open extract: {s.pipeline.oldestOpenDays}d</p>
        {:else}
          <p class="sp-sub sp-nodata">Oldest open extract: Not enough data yet</p>
        {/if}
        <p class="sp-sub">Last 30 days: {s.pipeline.extractsMade30} extracts, {s.pipeline.cardsMade30} cards</p>
        {#if settled(s.procSince)}
          <svg width="120" height="40" viewBox="0 0 120 40" class="sp-chart">
            {#each barHeights(s.procPerWeek, 40) as h, i}
              <rect x={i * 10 + 1} y={40 - h} width="7" height={h} fill="#7dd3fc" />
            {/each}
          </svg>
        {:else}
          <p class="sp-sub sp-nodata">
            {s.procSince ? `Collecting processing history since ${sinceDate(s.procSince)}` : 'Collecting processing history since today'}
          </p>
        {/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .sp-root { position: absolute; inset: 0; overflow: auto; padding: 12px; background: #131c29; }
  .sp-grid { display: grid; gap: 12px; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); }
  .sp-card { background: #131c29; border: 1px solid #2a3a4a; border-radius: 8px; padding: 12px; }
  .sp-card h3 { margin: 0 0 8px; color: #7dd3fc; font-size: 13px; }
  .sp-metric { color: #e8eaf0; font-size: 14px; margin: 4px 0; font-weight: 600; }
  .sp-sub { color: #e8eaf0; font-size: 12px; margin: 4px 0; }
  .sp-nodata { color: #8899aa; font-weight: normal; font-style: italic; }
  .sp-metric.ok { color: #7ecfa0; }
  .sp-metric.low { color: #7dd3fc; }
  .sp-metric.high { color: #e0b000; }
  .sp-chartrow { display: flex; flex-direction: column; gap: 2px; }
  .sp-chart { display: block; margin: 6px 0; }
  .sp-chartlabels { display: flex; gap: 6px; font-size: 9px; color: #8899aa; }
  .sp-chartlabels span { width: 14px; text-align: center; }
  .sp-loading, .sp-error { color: #8aa; padding: 20px; }
  .sp-error { color: #ff8080; }
</style>
