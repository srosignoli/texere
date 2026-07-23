<script>
  import { SearchGraph } from '../../../wailsjs/go/main/App'
  import { analyzeQuery, getSuggestions } from '../../lib/query-grammar.js'
  import { queryHints, ensureQueryHints } from '../../stores/queryHints.js'
  import GraphResults from './GraphResults.svelte'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null // unused; shared cell prop set
  export let cells = []      // unused
  export let onPush = () => {} // unused; shared cell prop set

  let result = null
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

  async function run() {
    if (running) return
    running = true
    error = ''
    showSug = false
    try {
      result = await SearchGraph(cell.query)
      cell.resultText = summarize(result)
      onChange(cell)
    } catch (e) { error = String(e?.message ?? e); result = null; cell.resultText = ''; onChange(cell) }
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
    {#if !readOnly}
      <button class="gq-run" on:click={run} disabled={running}>{running ? '…' : 'Run'}</button>
      <button class="gq-del" on:click={onDelete} title="Delete cell">✕</button>
    {/if}
  </div>
  {#if error}<div class="gq-error">{error}</div>{/if}
  {#if result}<div class="gq-results"><GraphResults {result} /></div>{/if}
</div>

<style>
  .gq-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .gq-bar { display: flex; align-items: flex-start; gap: 6px; padding: 6px 8px; }
  .gq-input-wrap { position: relative; flex: 1; min-width: 0; }
  .gq-input { box-sizing: border-box; width: 100%; background: #0b1220; border: 1px solid #2a3a4a; border-radius: 6px; color: #e8eaf0; font-size: 13px; padding: 5px 8px; font-family: monospace; }
  .gq-sug { position: absolute; z-index: 5; left: 0; right: 0; margin: 2px 0 0; padding: 4px; list-style: none; background: #101826; border: 1px solid #2a3a4a; border-radius: 6px; max-height: 200px; overflow-y: auto; }
  .gq-sug li { padding: 3px 6px; cursor: pointer; font-size: 12px; color: #cdd6e2; border-radius: 4px; }
  .gq-sug li:hover { background: #24374d; }
  .s-sum { color: #5a7a9a; }
  .gq-run { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 5px 12px; cursor: pointer; font-size: 12px; }
  .gq-del { background: none; border: none; color: #6b7a8a; cursor: pointer; }
  .gq-del:hover { color: #ff8080; }
  .gq-error { color: #ff8080; font-size: 12px; padding: 0 8px 6px; }
  .gq-results { padding: 6px 8px 8px; border-top: 1px solid #1c2a3a; }
</style>
