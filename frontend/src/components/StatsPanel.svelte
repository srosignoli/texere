<script>
  // Four panels over one payload, loaded when the tab is entered. The whole
  // Stats focus is a single GetStatistics call so the panels can never disagree
  // about the moment they describe.
  import { onMount } from 'svelte'
  import { GetStatistics } from '../../wailsjs/go/main/App'
  import { humanDuration, fiBand } from '../lib/stats-format.js'
  import Chart from 'chart.js/auto'
  import { barConfig, lineConfig } from '../lib/stats-charts.js'

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

  // Same shape as ComputeCell's: create on mount, destroy on teardown. No
  // update() needed — this panel renders its charts inside {#if s}, so the
  // action runs once with final data, and leaving the tab destroys the whole
  // component.
  function chart(node, config) {
    const c = new Chart(node, config)
    return { destroy() { c.destroy() } }
  }

  const weekLabels = (series) => (series ?? []).map((w) => w.weekStart)
  const weekValues = (series) => (series ?? []).map((w) => w.count)
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
          <div class="sp-stat">
            <span class="sp-label">Forgetting index</span>
            <span class="sp-value" class:ok={fiBand(s.quality.forgettingIndex) === 'ok'}
                  class:low={fiBand(s.quality.forgettingIndex) === 'low'}
                  class:high={fiBand(s.quality.forgettingIndex) === 'high'}>{pct(s.quality.forgettingIndex)}</span>
            <span class="sp-meaning">Share of reviews you failed. SuperMemo's healthy range is 7–13%.</span>
          </div>
        {:else}
          <div class="sp-stat">
            <span class="sp-label">Forgetting index</span>
            <span class="sp-value">Not enough data yet</span>
            <span class="sp-meaning">Share of reviews you failed. SuperMemo's healthy range is 7–13%.</span>
          </div>
        {/if}
        {#if has(s.quality.retention)}
          <div class="sp-stat">
            <span class="sp-label">Retention</span>
            <span class="sp-value">{pct(s.quality.retention)}</span>
            <span class="sp-meaning">Share you recalled successfully — the other side of the forgetting index.</span>
          </div>
        {:else}
          <div class="sp-stat">
            <span class="sp-label">Retention</span>
            <span class="sp-value">Not enough data yet</span>
            <span class="sp-meaning">Share you recalled successfully — the other side of the forgetting index.</span>
          </div>
        {/if}
        <div class="sp-chartbox"><canvas use:chart={barConfig({
          labels: ['Again', 'Hard', 'Good', 'Easy'], values: s.quality.gradeMix,
          yLabel: 'reviews', unit: 'reviews', color: '#7dd3fc' })}></canvas></div>
        <div class="sp-stat">
          <span class="sp-label">Leeches</span>
          <span class="sp-value">{s.workload.leeches} cards</span>
          <span class="sp-meaning">Cards you have failed five times or more.</span>
        </div>
        <div class="sp-chartbox"><canvas use:chart={barConfig({
          labels: ['1–3', '3–5', '5–7', '7–9', '9–10'], values: s.workload.difficulty,
          yLabel: 'cards', unit: 'cards', color: '#b9a3e3' })}></canvas></div>
      </section>

      <section class="sp-card">
        <h3>Workload</h3>
        {#if has(s.workload.burden)}
          <div class="sp-stat">
            <span class="sp-label">Burden</span>
            <span class="sp-value">{s.workload.burden.toFixed(1)} reviews/day</span>
            <span class="sp-meaning">Estimated steady-state load once every card settles into its interval.</span>
          </div>
        {:else}
          <div class="sp-stat">
            <span class="sp-label">Burden</span>
            <span class="sp-value">Not enough data yet</span>
            <span class="sp-meaning">Estimated steady-state load once every card settles into its interval.</span>
          </div>
        {/if}
        {#if has(s.workload.dailyMinutes)}
          <div class="sp-stat">
            <span class="sp-label">Daily time</span>
            <span class="sp-value">{humanDuration(s.workload.dailyMinutes)}</span>
            <span class="sp-meaning">Burden multiplied by your average time per review.</span>
          </div>
        {:else}
          <div class="sp-stat">
            <span class="sp-label">Daily time</span>
            <span class="sp-value">Not enough data yet</span>
            <span class="sp-meaning">Burden multiplied by your average time per review.</span>
          </div>
        {/if}
        <div class="sp-chartbox"><canvas use:chart={barConfig({
          labels: s.workload.forecast.map((_, i) => (i === 0 ? 'today' : `+${i}d`)),
          values: s.workload.forecast,
          yLabel: 'cards due', unit: 'cards', color: '#e0b000' })}></canvas></div>
        <div class="sp-stat">
          <span class="sp-label">Backlog</span>
          <span class="sp-value">{s.workload.backlog.read} read · {s.workload.backlog.process} process · {s.workload.backlog.recall} recall</span>
          <span class="sp-meaning">Waiting for you right now.</span>
        </div>
      </section>

      <section class="sp-card">
        <h3>Habit</h3>
        <div class="sp-chartbox"><canvas use:chart={lineConfig({
          labels: (s.habit.reviewsPerDay ?? []).map((d) => d.day),
          values: (s.habit.reviewsPerDay ?? []).map((d) => d.count),
          yLabel: 'reviews', unit: 'reviews', color: '#7ecfa0' })}></canvas></div>
        <div class="sp-stat">
          <span class="sp-label">Current streak</span>
          <span class="sp-value">{s.habit.currentStreak} days</span>
          <span class="sp-meaning">Consecutive days with at least one review.</span>
        </div>
        <div class="sp-stat">
          <span class="sp-label">Longest streak</span>
          <span class="sp-value">{s.habit.longestStreak} days</span>
          <span class="sp-meaning">Your best run so far.</span>
        </div>
        <div class="sp-stat">
          <span class="sp-label">Memorized</span>
          <span class="sp-value">{s.workload.memorized} cards</span>
          <span class="sp-meaning">Cards that have been reviewed at least once.</span>
        </div>
        <div class="sp-chartbox"><canvas use:chart={barConfig({
          labels: weekLabels(s.cardsPerWeek), values: weekValues(s.cardsPerWeek),
          yLabel: 'cards', unit: 'cards', color: '#7ecfa0' })}></canvas></div>
        {#if settled(s.readSince)}
          <div class="sp-chartbox"><canvas use:chart={barConfig({
            labels: weekLabels(s.readMinsWeek), values: weekValues(s.readMinsWeek),
            yLabel: 'minutes', unit: 'minutes', color: '#7dd3fc' })}></canvas></div>
        {:else}
          <p class="sp-sub sp-nodata">
            {s.readSince ? `Collecting reading time since ${sinceDate(s.readSince)}` : 'Collecting reading time since today'}
          </p>
        {/if}
      </section>

      <section class="sp-card">
        <h3>Pipeline health</h3>
        <div class="sp-stat">
          <span class="sp-label">Sources finished</span>
          <span class="sp-value">{s.pipeline.sourcesDone} of {s.pipeline.sources}</span>
          <span class="sp-meaning">Documents you have marked done.</span>
        </div>
        <div class="sp-stat">
          <span class="sp-label">Extracts</span>
          <span class="sp-value">{s.pipeline.extractsOpen} open of {s.pipeline.extracts}</span>
          <span class="sp-meaning">Passages taken from your reading, and how many still await processing.</span>
        </div>
        <div class="sp-stat">
          <span class="sp-label">Cards</span>
          <span class="sp-value">{s.pipeline.cards}</span>
          <span class="sp-meaning">Active cards in the collection.</span>
        </div>
        {#if has(s.pipeline.oldestOpenDays)}
          <div class="sp-stat">
            <span class="sp-label">Oldest unprocessed extract</span>
            <span class="sp-value">{s.pipeline.oldestOpenDays} days</span>
            <span class="sp-meaning">How long the oldest untouched passage has waited.</span>
          </div>
        {:else}
          <div class="sp-stat">
            <span class="sp-label">Oldest unprocessed extract</span>
            <span class="sp-value">Not enough data yet</span>
            <span class="sp-meaning">How long the oldest untouched passage has waited.</span>
          </div>
        {/if}
        <div class="sp-stat">
          <span class="sp-label">Last 30 days</span>
          <span class="sp-value">{s.pipeline.extractsMade30} extracts · {s.pipeline.cardsMade30} cards</span>
          <span class="sp-meaning">What you have created this month.</span>
        </div>
        {#if settled(s.procSince)}
          <div class="sp-chartbox"><canvas use:chart={barConfig({
            labels: weekLabels(s.procPerWeek), values: weekValues(s.procPerWeek),
            yLabel: 'extracts', unit: 'extracts', color: '#b9a3e3' })}></canvas></div>
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
  .sp-sub { color: #e8eaf0; font-size: 12px; margin: 4px 0; }
  .sp-nodata { color: #8899aa; font-weight: normal; font-style: italic; }
  .sp-stat { margin: 8px 0; }
  .sp-label { display: block; color: #8899aa; font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; }
  .sp-value { display: block; color: #e8eaf0; font-size: 15px; font-weight: 600; }
  .sp-value.ok { color: #7ecfa0; }
  .sp-value.low { color: #7dd3fc; }
  .sp-value.high { color: #e0b000; }
  .sp-meaning { display: block; color: #5d7186; font-size: 11px; line-height: 1.4; }
  .sp-chartbox { position: relative; height: 140px; margin: 8px 0; }
  .sp-loading, .sp-error { color: #8aa; padding: 20px; }
  .sp-error { color: #ff8080; }
</style>
