<script>
  import { SearchGraph, GraphLayout } from '../../../wailsjs/go/main/App'
  import { analyzeQuery, getSuggestions } from '../../lib/query-grammar.js'
  import AskBox from '../AskBox.svelte'
  import { queryHints, ensureQueryHints } from '../../stores/queryHints.js'
  import GraphResults from './GraphResults.svelte'
  import IsoGraphView from '../IsoGraphView.svelte'
  import { explorerOpen, explorerQuery } from '../../stores/explorer.js'

  export let cell
  export let onChange = () => {}
  export let readOnly = false
  export let context = null // unused; shared cell prop set
  export let cells = []      // unused
  export let onPush = () => {} // unused; shared cell prop set

  let result = null
  let layout = null
  let running = false
  let error = ''
  let caretPos = 0
  let suggestions = []
  let showSug = false

  function onInput(e) {
    cell.query = e.target.value
    caretPos = e.target.selectionStart ?? cell.query.length
    const analysis = analyzeQuery(cell.query, caretPos)
    suggestions = getSuggestions(analysis, { liveValues: $queryHints })
    showSug = suggestions.length > 0
    onChange(cell)
  }
  function applySug(item) {
    // Replace the token under the caret using the grammar's analysis.
    // query-grammar.js's analyzeQuery() returns {replaceFrom, replaceTo}, and
    // getSuggestions() items carry the full substitution text in `apply`
    // (already trailing-spaced) — see OmniBar.svelte's applySuggestion() for
    // the same pattern.
    const analysis = analyzeQuery(cell.query, caretPos)
    const from = analysis?.replaceFrom ?? caretPos
    const to = analysis?.replaceTo ?? caretPos
    const insert = item.apply ?? item.label ?? ''
    cell.query = cell.query.slice(0, from) + insert + cell.query.slice(to)
    cell = cell
    showSug = false
    onChange(cell)
  }

  // The translated query fills the input and stops there. This cell already
  // requires an explicit Run (or Enter), and a generated \notes is a full-DB
  // scan — it should not fire because a question was asked.
  function onTranslated(e) {
    cell.query = e.detail
    cell = cell
    showSug = false
    onChange(cell)
  }

  function setView(v) {
    if (cell.view === v) return
    cell.view = v
    onChange(cell)
    if (v === 'graph' && cell.query.trim()) run()
  }
  function expand() { explorerQuery.set(cell.query); explorerOpen.set(true) }

  async function run() {
    if (running) return
    running = true
    error = ''
    showSug = false
    try {
      if (cell.view === 'graph') {
        layout = await GraphLayout(cell.query, 'extended')
        // Keep LLM-attach working: derive a bounded text summary from node labels.
        const labels = (layout?.nodes ?? []).map((n) => n.label ?? n.text ?? n.name ?? '').filter(Boolean)
        cell.resultText = labels.join('\n').slice(0, 4000)
        onChange(cell)
      } else {
        result = await SearchGraph(cell.query)
        cell.resultText = summarize(result)
        onChange(cell)
      }
    } catch (e) { error = String(e?.message ?? e); result = null; layout = null; cell.resultText = ''; onChange(cell) }
    finally { running = false }
  }

  // Bounded plain-text summary for LLM attach (~4KB cap).
  function summarize(r) {
    if (!r) return ''
    const out = []
    for (const n of r.Nodes ?? []) {
      out.push(n.NodeText)
      for (const o of n.Orbit ?? []) out.push(`  ${o.ArrowLong} ${o.NeighborText}`)
    }
    for (const c of r.Structural ?? []) out.push((c.Nodes ?? []).map((x) => x.NodeText).join(c.Kind === 'sequence' ? ' > ' : ' -> '))
    for (const it of r.Listings ?? []) out.push(`${it.Title}${it.Detail ? ' — ' + it.Detail : ''}`)
    for (const note of r.Notes ?? []) for (const b of note.Blocks ?? []) {
      out.push(b.NodeText)
      for (const a of b.Arrows ?? []) out.push(`  ${a.ArrowLong} ${a.Target}`)
    }
    return out.join('\n').slice(0, 4000)
  }

  function onKey(e) { if (e.key === 'Enter') { e.preventDefault(); run() } else if (e.key === 'Escape') showSug = false }
</script>

<div class="gq-cell">
  {#if !readOnly}
    <div class="gq-ask"><AskBox on:translated={onTranslated} /></div>
  {/if}
  <div class="gq-bar">
    <div class="gq-input-wrap">
      <input class="gq-input" value={cell.query} on:input={onInput} on:keydown={onKey} on:focus={() => ensureQueryHints()}
        placeholder="searchN4L query — e.g.  eat \radius 2   or   \from A \to B" disabled={readOnly} autocomplete="off" />
      {#if showSug && !readOnly}
        <ul class="gq-sug">
          {#each suggestions.slice(0, 8) as s}
            <li on:mousedown|preventDefault={() => applySug(s)}>{s.label}{#if s.detail}<span class="s-sum"> — {s.detail}</span>{/if}</li>
          {/each}
        </ul>
      {/if}
    </div>
    <div class="gq-view">
      <button class:sel={(cell.view ?? 'text') === 'text'} on:click={() => setView('text')} disabled={readOnly && false}>Text</button>
      <button class:sel={(cell.view ?? 'text') === 'graph'} on:click={() => setView('graph')}>Graph</button>
    </div>
    {#if !readOnly}
      <button class="gq-run" on:click={run} disabled={running}>{running ? '…' : 'Run'}</button>
    {/if}
  </div>
  {#if error}<div class="gq-error">{error}</div>{/if}
  {#if (cell.view ?? 'text') === 'graph'}
    <div class="gq-graph">
      <div class="gq-graphbar"><button class="gq-expand" on:click={expand} title="Open in full explorer">⛶ Expand</button></div>
      <IsoGraphView {layout} height={320} />
    </div>
  {:else if result}
    <div class="gq-results"><GraphResults {result} /></div>
  {/if}
</div>

<style>
  .gq-cell { background: var(--surface-modal); border: 1px solid var(--border-card); border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .gq-ask { border-bottom: 1px solid var(--border); }
  .gq-bar { display: flex; align-items: flex-start; gap: 6px; padding: 6px 8px; }
  .gq-input-wrap { position: relative; flex: 1; min-width: 0; }
  .gq-input { box-sizing: border-box; width: 100%; background: var(--surface-sunken); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-size: 13px; padding: 5px 8px; font-family: monospace; }
  .gq-sug { position: absolute; z-index: 5; left: 0; right: 0; margin: 2px 0 0; padding: 4px; list-style: none; background: var(--surface-inset); border: 1px solid var(--border); border-radius: 6px; max-height: 200px; overflow-y: auto; }
  .gq-sug li { padding: 3px 6px; cursor: pointer; font-size: 12px; color: var(--text-modal); border-radius: 4px; }
  .gq-sug li:hover { background: var(--surface-inset-hover); }
  .s-sum { color: var(--accent-dim); }
  .gq-run { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px; padding: 5px 12px; cursor: pointer; font-size: 12px; }
  .gq-error { color: var(--danger); font-size: 12px; padding: 0 8px 6px; }
  .gq-results { padding: 6px 8px 8px; border-top: 1px solid var(--border-subtle); }
  .gq-view { display: inline-flex; gap: 2px; }
  .gq-view button { background: var(--control); color: var(--text-tab-muted); border: 1px solid var(--border); border-radius: 6px; padding: 4px 8px; cursor: pointer; font-size: 12px; }
  .gq-view button.sel { color: var(--text); background: var(--accent-hover); }
  .gq-graph { border-top: 1px solid var(--border-subtle); padding: 6px 8px 8px; }
  .gq-graphbar { display: flex; justify-content: flex-end; margin-bottom: 4px; }
  .gq-expand { background: var(--control); color: var(--accent-text-alt); border: 1px solid var(--border); border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
</style>
