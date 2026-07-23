# Processing Notebook — Slice 3 (Graph + Compute cells) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a graph query cell (full searchN4L parity with the Ctrl+Shift+K search, results rendered inline) and a compute cell (sandboxed Web Worker JS with math.js, an async graph bridge, and Chart.js plots) to the processing notebook.

**Architecture:** Both cells slot into Slice 1's `cellRegistry`. The graph cell reuses `SearchGraph` + `query-grammar.js` and renders the full `SearchResult` via a presentational `GraphResults.svelte`. The compute cell runs user JS in a per-run Web Worker; a main-thread host (`compute-host.js`) bridges `graph.query`/`graph.orbit` to `SearchGraph`, enforces a timeout, and collects console/value/chart output. Results are ephemeral; a bounded text summary persists for LLM attach.

**Tech Stack:** Svelte 3.49, Vite 3 (native Web Worker bundling), math.js + Chart.js (new npm deps), existing `SearchGraph` Wails binding. No new Go dependencies.

## Global Constraints

- Frontend has no JS test runner; frontend tasks verify with `cd frontend && npm run build` plus a manual test plan.
- New **npm** deps allowed this slice: `mathjs`, `chart.js` (frontend only; the pure-Go/no-CGo rule governs Go deps, not npm). No new Go dependencies.
- Cells persist via Slice 1 autosave; graph/compute cells call `onChange(cell)` with the whole object (NotebookPane routes object→`updateCellObj`, string→`updateCell`).
- Graph cell has **full searchN4L parity** with the Ctrl+Shift+K unified search (any directive/`\radius N`/path/sequence/context/notes) — no radius-1 limit; it renders the complete `SearchResult`.
- Results are ephemeral; the query text / source code persists, plus a truncated `resultText` summary for LLM attach.
- Compute worker: terminable, 5s timeout, math.js + async `graph.query(q)`/`graph.orbit(node, radius=1)` + Chart.js via a main-thread render bridge; kernel-labelled (`language:"js"`, JS only).
- Run `wails`/`make` via `PATH="$PATH:$(go env GOPATH)/bin"`.

## Deviation from the spec (confirm at handoff)

The spec says the compute cell shows "the last expression's value." Implicit last-expression capture is incompatible with the top-level-`await` async wrapper without an AST transform (out of scope). This plan instead shows the value the user code **`return`s** from the async wrapper (top-level `await` supported), alongside captured `console` output. This is the one behavioral deviation; everything else matches the spec.

---

## File Structure

**Frontend (Svelte/JS)**
- `frontend/src/components/notebook/GraphResults.svelte` — create: presentational renderer for a `SearchResult` (nodes+orbits, Structural chains, Listings, Notes, error/unavailable).
- `frontend/src/components/notebook/GraphQueryCell.svelte` — create: query input + autocomplete + Run + inline `GraphResults` + `resultText`.
- `frontend/src/components/notebook/ComputeCell.svelte` — create: code textarea + Run/Stop + console/value log + Chart.js canvases + `resultText`.
- `frontend/src/lib/compute-worker.js` — create: the JS-kernel Web Worker (math.js, graph RPC, chart/console shims, async eval).
- `frontend/src/lib/compute-host.js` — create: main-thread runner (spawn worker, bridge graph RPC → `SearchGraph`, timeout, cancel).
- `frontend/src/lib/notebook.js` — modify: register `graph`/`compute` in `cellRegistry`; add `newGraphCell`/`newComputeCell`.
- `frontend/src/lib/llm.js` — modify: `buildSystem` includes attached `graph`/`compute` cells' `resultText`.
- `frontend/src/components/notebook/NotebookPane.svelte` — modify: "+ Graph cell" / "+ Compute cell" buttons.
- `frontend/package.json` — modify: add `mathjs`, `chart.js`.
- `docs/superpowers/specs/2026-07-23-processing-notebook-slice3-manual-test-plan.md` — create.

---

## Phase 3a — Graph query cell

## Task 1: GraphResults presentational renderer

**Files:**
- Create: `frontend/src/components/notebook/GraphResults.svelte`

**Interfaces:**
- Consumes: a `SearchResult` object (fields: `GraphUnavailable`, `GraphError`, `Nodes[]{NodeText,Chapter,ContextTags,Orbit[]{ArrowLong,NeighborText,Radius}}`, `Structural[]{Kind,Context,Chapter,Nodes[]{NodeText,ArrowLong}}`, `Listings[]{Kind,Title,Detail,Nodes[]}`, `Notes[]{Chapter,Blocks[]{NodeText,Context,Arrows[]{ArrowLong,Target}}}`).
- Produces: `GraphResults` component, prop `result`.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/notebook/GraphResults.svelte`:

```svelte
<script>
  export let result = null
  $: r = result ?? {}
  $: empty = r && !r.GraphUnavailable && !r.GraphError &&
    !(r.Nodes?.length || r.Structural?.length || r.Listings?.length || r.Notes?.length)
</script>

<div class="gr">
  {#if r.GraphUnavailable}
    <p class="gr-hint">Graph not built — run Build Graph (Ctrl+G) first.</p>
  {:else if r.GraphError}
    <p class="gr-error">{r.GraphError}</p>
  {:else if empty}
    <p class="gr-hint">No results.</p>
  {:else}
    {#if r.Nodes?.length}
      <section>
        {#each r.Nodes as n}
          <div class="node">
            <div class="node-head">{n.NodeText}{#if n.Chapter}<span class="chap">· {n.Chapter}</span>{/if}</div>
            {#if n.ContextTags?.length}<div class="tags">{#each n.ContextTags as t}<span class="tag">{t}</span>{/each}</div>{/if}
            {#if n.Orbit?.length}
              <ul class="orbit">
                {#each n.Orbit as o}<li><span class="arrow">{o.ArrowLong}</span> {o.NeighborText}{#if o.Radius > 1}<span class="rad"> r{o.Radius}</span>{/if}</li>{/each}
              </ul>
            {/if}
          </div>
        {/each}
      </section>
    {/if}

    {#if r.Structural?.length}
      <section class="chains">
        {#each r.Structural as chain}
          <div class="chain" class:seq={chain.Kind === 'sequence'}>
            {#if chain.Context}<div class="chain-label">{chain.Context}{#if chain.Chapter}<span class="chap"> · {chain.Chapter}</span>{/if}</div>{/if}
            <div class="chain-nodes">
              {#each chain.Nodes as cn, i}
                <span class="cn">{cn.NodeText}</span>{#if cn.ArrowLong}<span class="cn-arrow"> —{cn.ArrowLong}→ </span>{/if}
              {/each}
            </div>
          </div>
        {/each}
      </section>
    {/if}

    {#if r.Listings?.length}
      <section class="listings">
        {#each r.Listings as it}
          <div class="listing"><b>{it.Title}</b>{#if it.Detail}<span class="detail"> — {it.Detail}</span>{/if}
            {#if it.Nodes?.length}<div class="listing-nodes">{it.Nodes.join(', ')}</div>{/if}
          </div>
        {/each}
      </section>
    {/if}

    {#if r.Notes?.length}
      <section class="notes">
        {#each r.Notes as note}
          <div class="note-chap">{note.Chapter}</div>
          {#each note.Blocks as b}
            <div class="nb"><div class="nb-head">{b.NodeText}{#if b.Context}<span class="chap"> · {b.Context}</span>{/if}</div>
              {#if b.Arrows?.length}<ul class="orbit">{#each b.Arrows as a}<li><span class="arrow">{a.ArrowLong}</span> {a.Target}</li>{/each}</ul>{/if}
            </div>
          {/each}
        {/each}
      </section>
    {/if}
  {/if}
</div>

<style>
  .gr { font-size: 12px; color: #cdd6e2; display: flex; flex-direction: column; gap: 10px; }
  .gr-hint { color: #5a7a9a; margin: 0; }
  .gr-error { color: #ff8080; margin: 0; }
  section { display: flex; flex-direction: column; gap: 6px; }
  .node, .nb { background: #101826; border: 1px solid #26374a; border-radius: 6px; padding: 6px 8px; }
  .node-head, .nb-head { color: #e8eaf0; font-weight: 600; }
  .chap { color: #5a7a9a; font-weight: 400; font-size: 11px; }
  .tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 3px; }
  .tag { background: #1e3a5a; color: #93c5fd; border-radius: 4px; padding: 0 5px; font-size: 11px; }
  .orbit { margin: 4px 0 0; padding-left: 14px; color: #b9c6d6; }
  .arrow { color: #7dd3fc; }
  .rad { color: #5a7a9a; font-size: 10px; }
  .chain { background: #101826; border: 1px solid #26374a; border-radius: 6px; padding: 6px 8px; }
  .chain-label { color: #93c5fd; font-size: 11px; margin-bottom: 3px; }
  .cn { color: #e8eaf0; }
  .cn-arrow { color: #7dd3fc; }
  .chain.seq .chain-nodes { display: flex; flex-direction: column; }
  .listing { padding: 2px 0; }
  .detail { color: #b9c6d6; }
  .listing-nodes { color: #8aa0b6; font-size: 11px; }
  .note-chap { color: #7dd3fc; font-size: 11px; margin-top: 4px; }
</style>
```

- [ ] **Step 2: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/notebook/GraphResults.svelte
git commit -m "feat(notebook): GraphResults renderer for full SearchResult"
```

---

## Task 2: GraphQueryCell (query editor + Run)

**Files:**
- Create: `frontend/src/components/notebook/GraphQueryCell.svelte`

**Interfaces:**
- Consumes: `SearchGraph` from `../../../wailsjs/go/main/App`; `analyzeQuery`, `getSuggestions` from `../../lib/query-grammar.js`; `queryHints` store from `../../stores/queryHints.js` (live values for autocomplete; optional — pass `{}` if unavailable); `GraphResults` (Task 1).
- Props: `cell` (`{id,type:'graph',query,resultText}`), `onChange(cell)`, `onDelete()`, `readOnly`, plus ignored `context`/`cells` (shared cell prop set).
- Produces: `GraphQueryCell`. On Run, sets `cell.resultText` (bounded summary) and calls `onChange(cell)`.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/notebook/GraphQueryCell.svelte`:

```svelte
<script>
  import { SearchGraph } from '../../../wailsjs/go/main/App'
  import { analyzeQuery, getSuggestions } from '../../lib/query-grammar.js'
  import { queryHints } from '../../stores/queryHints.js'
  import GraphResults from './GraphResults.svelte'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null // unused; shared cell prop set
  export let cells = []      // unused

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
    const analysis = analyzeQuery(cell.query, caretPos)
    const from = analysis?.replaceFrom ?? caretPos
    const to = analysis?.replaceTo ?? caretPos
    const insert = item.insert ?? item.value ?? item.token ?? item.label ?? ''
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
    } catch (e) { error = String(e?.message ?? e); result = null }
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
      <input class="gq-input" value={cell.query} on:input={onInput} on:keydown={onKey}
        placeholder="searchN4L query — e.g.  eat \radius 2   or   \from A \to B" disabled={readOnly} autocomplete="off" />
      {#if showSug && !readOnly}
        <ul class="gq-sug">
          {#each suggestions.slice(0, 8) as s}
            <li on:mousedown|preventDefault={() => applySug(s)}>{s.label ?? s.token ?? s.value}{#if s.summary}<span class="s-sum"> — {s.summary}</span>{/if}</li>
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
  .gq-input { width: 100%; background: #0b1220; border: 1px solid #2a3a4a; border-radius: 6px; color: #e8eaf0; font-size: 13px; padding: 5px 8px; font-family: monospace; }
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
```

> Note on `analyzeQuery`'s return shape: this uses `analysis.replaceFrom`/`replaceTo` and `item.insert`/`value`/`token`/`label`/`summary` defensively (falling back through several field names) because `query-grammar.js`'s exact suggestion object shape is owned by that module. Before finishing, open `frontend/src/lib/query-grammar.js` and confirm the real field names for `getSuggestions` items and `analyzeQuery`'s replace range; adjust `applySug` to the actual names (keep the fallback chain if unsure). This does not change the Run/render behavior, only autocomplete insertion.

- [ ] **Step 2: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/notebook/GraphQueryCell.svelte
git commit -m "feat(notebook): graph query cell (searchN4L editor + Run + inline results)"
```

---

## Task 3: Register graph cell + wire NotebookPane + LLM attach

**Files:**
- Modify: `frontend/src/lib/notebook.js`, `frontend/src/components/notebook/NotebookPane.svelte`, `frontend/src/lib/llm.js`

**Interfaces:**
- Consumes: `GraphQueryCell` (Task 2).
- Produces: `newGraphCell()`; `cellRegistry.graph`; a "+ Graph cell" button; `buildSystem` handling of attached `graph`/`compute` cells.

- [ ] **Step 1: Register the cell type**

In `frontend/src/lib/notebook.js`, add the import, factory, and registry entry:

```js
import GraphQueryCell from '../components/notebook/GraphQueryCell.svelte'
```

```js
export function newGraphCell() {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'graph', query: '', resultText: '' }
}
```

```js
export const cellRegistry = {
  context: ContextCell,
  n4l: N4LDraftCell,
  llm: LLMCell,
  graph: GraphQueryCell,
}
```

- [ ] **Step 2: Add the "+ Graph cell" button to NotebookPane**

In `frontend/src/components/notebook/NotebookPane.svelte`:
- Extend the import: `import { newDraftCell, newLLMCell, newGraphCell, cellRegistry } from '../../lib/notebook.js'`
- Add a handler beside `addLLM`: `function addGraph() { cells = [...cells, newGraphCell()]; scheduleSave() }`
- Add the button next to the others: `<button class="nb-add" on:click={addGraph}>+ Graph cell</button>`

- [ ] **Step 3: Extend buildSystem for graph/compute attach**

In `frontend/src/lib/llm.js`, inside `buildSystem`'s attached-cells loop, add branches so attached graph/compute cells contribute their `resultText`. Locate the existing loop:

```js
  for (const c of attachedCells ?? []) {
    if (c.type === 'n4l' && c.text) ctx.push(`# Attached N4L draft\n${c.text}`)
    else if (c.type === 'llm' && c.messages?.length) {
      ctx.push('# Attached conversation\n' + c.messages.map((m) => `${m.role}: ${m.content}`).join('\n'))
    }
  }
```

Add two branches before the closing brace:

```js
    else if (c.type === 'graph' && c.resultText) ctx.push(`# Attached graph result\n${c.resultText}`)
    else if (c.type === 'compute' && c.resultText) ctx.push(`# Attached compute output\n${c.resultText}`)
```

- [ ] **Step 4: Include graph/compute cells in the LLM attach picker**

In `frontend/src/components/notebook/LLMCell.svelte`, the attach control filters sibling cells to `n4l`/`llm`. Broaden it to also offer `graph` and `compute`. Find both occurrences of:

```js
c.type === 'n4l' || c.type === 'llm'
```

and change each to:

```js
c.type === 'n4l' || c.type === 'llm' || c.type === 'graph' || c.type === 'compute'
```

Also update the attach label to name the type — the existing template does `{c.type === 'n4l' ? 'draft' : 'llm'}`; change it to:

```svelte
{c.type === 'n4l' ? 'draft' : c.type === 'graph' ? 'graph' : c.type === 'compute' ? 'compute' : 'llm'}
```

- [ ] **Step 5: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 6: Manual smoke test**

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build && ./build/bin/texere`
Verify: a "+ Graph cell" appears; adding it shows a query input; typing a query + Run (or Enter) renders results; an LLM cell's attach list now includes the graph cell.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/notebook.js frontend/src/components/notebook/NotebookPane.svelte frontend/src/lib/llm.js frontend/src/components/notebook/LLMCell.svelte
git commit -m "feat(notebook): register graph cell; attach graph/compute results to LLM cells"
```

---

## Phase 3b — Compute cell

## Task 4: Add math.js + Chart.js dependencies

**Files:**
- Modify: `frontend/package.json` (+ lockfile)

- [ ] **Step 1: Install**

Run: `cd frontend && npm install mathjs chart.js`
Expected: `dependencies` gains `mathjs` and `chart.js`; `package-lock.json` updated.

- [ ] **Step 2: Verify the build still succeeds**

Run: `cd frontend && npm run build`
Expected: build succeeds (deps resolve).

- [ ] **Step 3: Commit**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "chore(deps): add math.js + Chart.js for the compute cell"
```

---

## Task 5: Compute engine — worker + host

**Files:**
- Create: `frontend/src/lib/compute-worker.js`, `frontend/src/lib/compute-host.js`

**Interfaces:**
- Consumes: `mathjs` (in the worker); `SearchGraph` from `../../wailsjs/go/main/App` (in the host).
- Produces:
  - `compute-worker.js` — a Web Worker module; message protocol below.
  - `compute-host.js` exports `runCompute(code, { onConsole, onValue, onChart, onError, onDone, timeoutMs })` → `{ cancel() }`.
- Message protocol (host ⇄ worker):
  - host→worker: `{type:'run', code}`, `{type:'graph-res', id, result|error}`
  - worker→host: `{type:'console', level, text}`, `{type:'chart', config}`, `{type:'value', text}`, `{type:'error', text}`, `{type:'done'}`, `{type:'graph-req', id, query}`

- [ ] **Step 1: Create the worker**

Create `frontend/src/lib/compute-worker.js`:

```js
// JS-kernel Web Worker for the compute cell. Runs the user's code as the body
// of an async function with `math`, `graph`, `chart`, and `console` in scope.
import { create, all } from 'mathjs'

const math = create(all)
let reqId = 0
const pending = new Map()

function post(msg) { self.postMessage(msg) }
function fmt(v) {
  if (typeof v === 'string') return v
  try { return JSON.stringify(v) } catch { return String(v) }
}

function rpc(query) {
  const id = ++reqId
  post({ type: 'graph-req', id, query })
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }))
}
const graph = {
  query: (q) => rpc(String(q)),
  orbit: (node, radius = 1) => rpc(String(node) + ' \\radius ' + radius),
}
const chart = (config) => post({ type: 'chart', config })
const consoleShim = {
  log: (...a) => post({ type: 'console', level: 'log', text: a.map(fmt).join(' ') }),
  info: (...a) => post({ type: 'console', level: 'info', text: a.map(fmt).join(' ') }),
  warn: (...a) => post({ type: 'console', level: 'warn', text: a.map(fmt).join(' ') }),
  error: (...a) => post({ type: 'console', level: 'error', text: a.map(fmt).join(' ') }),
}

self.onmessage = async (e) => {
  const m = e.data
  if (m.type === 'graph-res') {
    const p = pending.get(m.id)
    if (!p) return
    pending.delete(m.id)
    if (m.error) p.reject(new Error(m.error))
    else p.resolve(m.result)
    return
  }
  if (m.type === 'run') {
    try {
      // eslint-disable-next-line no-new-func
      const fn = new Function('math', 'graph', 'chart', 'console',
        '"use strict"; return (async () => {\n' + m.code + '\n})();')
      const value = await fn(math, graph, chart, consoleShim)
      if (value !== undefined) post({ type: 'value', text: fmt(value) })
      post({ type: 'done' })
    } catch (err) {
      post({ type: 'error', text: String((err && err.message) || err) })
    }
  }
}
```

- [ ] **Step 2: Create the host**

Create `frontend/src/lib/compute-host.js`:

```js
import { SearchGraph } from '../../wailsjs/go/main/App'

// runCompute spawns a fresh worker, bridges graph.query/orbit to SearchGraph,
// enforces a timeout, and streams output via callbacks. Returns { cancel }.
export function runCompute(code, { onConsole = () => {}, onValue = () => {}, onChart = () => {}, onError = () => {}, onDone = () => {}, timeoutMs = 5000 } = {}) {
  const worker = new Worker(new URL('./compute-worker.js', import.meta.url), { type: 'module' })
  let finished = false

  const timer = setTimeout(() => finish('timeout'), timeoutMs)

  function finish(reason) {
    if (finished) return
    finished = true
    clearTimeout(timer)
    worker.terminate()
    if (reason === 'timeout') onError(`Timed out after ${timeoutMs}ms (terminated).`)
    onDone()
  }

  worker.onmessage = async (e) => {
    const m = e.data
    if (finished) return
    switch (m.type) {
      case 'console': onConsole(m.level, m.text); break
      case 'chart': onChart(m.config); break
      case 'value': onValue(m.text); break
      case 'error': onError(m.text); finish('error'); break
      case 'done': finish('done'); break
      case 'graph-req': {
        try {
          const result = await SearchGraph(m.query)
          if (!finished) worker.postMessage({ type: 'graph-res', id: m.id, result })
        } catch (err) {
          if (!finished) worker.postMessage({ type: 'graph-res', id: m.id, error: String((err && err.message) || err) })
        }
        break
      }
    }
  }
  worker.onerror = (e) => { onError(e.message || 'compute worker failed'); finish('error') }

  worker.postMessage({ type: 'run', code })
  return { cancel: () => finish('cancel') }
}
```

- [ ] **Step 3: Verify the frontend compiles (and the worker chunk is emitted)**

Run: `cd frontend && npm run build`
Expected: build succeeds; Vite emits a separate worker chunk (look for an `assets/compute-worker-*.js` entry in the build output). If the build errors on the `new URL(...)` worker import, confirm Vite is version 3+ (it is) — no config change is needed.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/compute-worker.js frontend/src/lib/compute-host.js
git commit -m "feat(notebook): compute engine — worker (math.js + graph RPC) + host (timeout/cancel/bridge)"
```

---

## Task 6: ComputeCell component

**Files:**
- Create: `frontend/src/components/notebook/ComputeCell.svelte`

**Interfaces:**
- Consumes: `runCompute` from `../../lib/compute-host.js`; `Chart` from `chart.js/auto`.
- Props: `cell` (`{id,type:'compute',language:'js',code,resultText}`), `onChange(cell)`, `onDelete()`, `readOnly`, ignored `context`/`cells`.
- Produces: `ComputeCell`. Persists `code` + a bounded `resultText` (console+value text) on completion.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/notebook/ComputeCell.svelte`:

```svelte
<script>
  import { onDestroy } from 'svelte'
  import { runCompute } from '../../lib/compute-host.js'
  import Chart from 'chart.js/auto'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null // unused; shared cell prop set
  export let cells = []      // unused

  let running = false
  let logs = []        // { level, text }
  let value = ''
  let error = ''
  let chartConfigs = []
  let run = null

  onDestroy(() => run?.cancel())

  function persist() {
    const text = [...logs.map((l) => l.text), value].filter(Boolean).join('\n').slice(0, 4000)
    cell.resultText = text
    onChange(cell)
  }

  function start() {
    if (running) return
    logs = []; value = ''; error = ''; chartConfigs = []
    running = true
    run = runCompute(cell.code ?? '', {
      onConsole: (level, text) => { logs = [...logs, { level, text }] },
      onValue: (text) => { value = text },
      onChart: (config) => { chartConfigs = [...chartConfigs, config] },
      onError: (text) => { error = text },
      onDone: () => { running = false; run = null; persist() },
    })
  }
  function stop() { run?.cancel(); running = false; run = null }
  function onCodeInput(e) { cell.code = e.target.value; onChange(cell) }

  // Svelte action: render a Chart.js chart into the canvas, destroy on teardown.
  function chartAction(node, config) {
    const c = new Chart(node, config)
    return { destroy() { c.destroy() } }
  }
</script>

<div class="cc-cell">
  <div class="cc-bar">
    <span class="cc-lang">js</span>
    <span class="cc-spacer"></span>
    {#if !readOnly}
      {#if running}<button class="cc-stop" on:click={stop}>Stop</button>{:else}<button class="cc-run" on:click={start}>Run</button>{/if}
      <button class="cc-del" on:click={onDelete} title="Delete cell">✕</button>
    {/if}
  </div>
  <textarea class="cc-code" value={cell.code ?? ''} on:input={onCodeInput} disabled={readOnly} spellcheck="false"
    placeholder={"// math.js as `math`, async `graph.query(q)` / `graph.orbit(node)`, `chart(config)`.\n// `return` a value to display it.\nreturn math.sqrt(16)"} rows="5"></textarea>

  {#if logs.length || value || error || chartConfigs.length}
    <div class="cc-out">
      {#each logs as l}<div class="cc-log {l.level}">{l.text}</div>{/each}
      {#if value}<div class="cc-value">⇒ {value}</div>{/if}
      {#if error}<div class="cc-error">{error}</div>{/if}
      {#each chartConfigs as cfg (cfg)}<div class="cc-chart"><canvas use:chartAction={cfg}></canvas></div>{/each}
    </div>
  {/if}
</div>

<style>
  .cc-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .cc-bar { display: flex; align-items: center; gap: 8px; padding: 5px 8px; border-bottom: 1px solid #1c2a3a; }
  .cc-lang { color: #7dd3fc; font-size: 11px; font-family: monospace; }
  .cc-spacer { flex: 1; }
  .cc-run { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 3px 12px; cursor: pointer; font-size: 12px; }
  .cc-stop { background: #5a1e1e; color: #ffd7d7; border: 1px solid #2a3a4a; border-radius: 6px; padding: 3px 12px; cursor: pointer; font-size: 12px; }
  .cc-del { background: none; border: none; color: #6b7a8a; cursor: pointer; }
  .cc-del:hover { color: #ff8080; }
  .cc-code { width: 100%; box-sizing: border-box; background: #0b1220; border: none; color: #e8eaf0; font-family: monospace; font-size: 13px; padding: 8px; resize: vertical; }
  .cc-out { padding: 8px; border-top: 1px solid #1c2a3a; display: flex; flex-direction: column; gap: 4px; }
  .cc-log { font-family: monospace; font-size: 12px; color: #b9c6d6; white-space: pre-wrap; }
  .cc-log.warn { color: #e0b050; }
  .cc-log.error { color: #ff8080; }
  .cc-value { font-family: monospace; font-size: 13px; color: #7dd3fc; white-space: pre-wrap; }
  .cc-error { color: #ff8080; font-size: 12px; white-space: pre-wrap; }
  .cc-chart { background: #0b1220; border: 1px solid #1c2a3a; border-radius: 6px; padding: 6px; margin-top: 4px; }
</style>
```

> The `{#each chartConfigs as cfg (cfg)}` keyed block plus the `chartAction` (create on mount / `destroy()` on teardown) ensures each Run's charts are torn down and rebuilt cleanly — no Chart.js "canvas already in use" leaks. `chart.js/auto` auto-registers all controllers so any config type works.

- [ ] **Step 2: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/notebook/ComputeCell.svelte
git commit -m "feat(notebook): compute cell UI (code editor + Run/Stop + output + Chart.js)"
```

---

## Task 7: Register compute cell + NotebookPane button

**Files:**
- Modify: `frontend/src/lib/notebook.js`, `frontend/src/components/notebook/NotebookPane.svelte`

**Interfaces:**
- Consumes: `ComputeCell` (Task 6).
- Produces: `newComputeCell()`; `cellRegistry.compute`; a "+ Compute cell" button.

- [ ] **Step 1: Register the cell type**

In `frontend/src/lib/notebook.js`:

```js
import ComputeCell from '../components/notebook/ComputeCell.svelte'
```

```js
export function newComputeCell() {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'compute', language: 'js', code: '', resultText: '' }
}
```

Add to `cellRegistry`:

```js
  compute: ComputeCell,
```

- [ ] **Step 2: Add the "+ Compute cell" button**

In `NotebookPane.svelte`:
- Extend the import to include `newComputeCell`.
- Add: `function addCompute() { cells = [...cells, newComputeCell()]; scheduleSave() }`
- Add the button: `<button class="nb-add" on:click={addCompute}>+ Compute cell</button>`

- [ ] **Step 3: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/notebook.js frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): register compute cell + toolbar button"
```

---

## Task 8: Manual test plan + full verification

**Files:**
- Create: `docs/superpowers/specs/2026-07-23-processing-notebook-slice3-manual-test-plan.md`

- [ ] **Step 1: Write the manual test plan**

Create the file:

```markdown
# Processing Notebook — Slice 3 Manual Test Plan

Prereqs: a collection with a built graph (Ctrl+G). Open an extract for processing.

## TC-1 Graph cell — text query
1. Add a Graph cell; type `<some node text> \radius 2`; Run (or Enter).
   - EXPECT: matched node(s) with orbit entries at radius 1 AND 2 render inline.

## TC-2 Graph cell — path query
1. In a Graph cell run a `\from <A> \to <B>` query.
   - EXPECT: path chain(s) render as arrow sequences (same results as the Ctrl+Shift+K search).

## TC-3 Graph cell — parity + errors
1. Run a `\notes` or `\chapter` query; confirm listings/notes render.
2. On a collection with no graph built, run any query.
   - EXPECT: "Graph not built (Ctrl+G)" hint, no crash.

## TC-4 Compute cell — math.js + value
1. Add a Compute cell; enter `return math.sqrt(2) * 10`; Run.
   - EXPECT: `⇒ 14.14…` shown.
2. Add `console.log('hi', {a:1})` above the return; Run.
   - EXPECT: the console line renders.

## TC-5 Compute cell — graph bridge
1. In a Compute cell: `const r = await graph.query('<node> \\radius 1'); return r.Nodes.length`; Run.
   - EXPECT: the node count from the live graph is returned.

## TC-6 Compute cell — Chart.js
1. `chart({ type:'bar', data:{ labels:['a','b'], datasets:[{ data:[3,7] }] } }); return 'done'`; Run.
   - EXPECT: a bar chart renders below the cell.

## TC-7 Compute cell — Stop + timeout
1. `while(true){}`; Run.
   - EXPECT: after ~5s the cell reports a timeout (terminated); the UI never freezes.
2. Run a slow loop and click Stop.
   - EXPECT: it halts immediately.

## TC-8 Attach to LLM
1. Run a Graph cell, then in an LLM cell attach it and ask a question about the result.
   - EXPECT: the reply reflects the graph result.

## TC-9 Persistence
1. Add a graph query and compute code, restart the app, reopen the extract.
   - EXPECT: the query text and code persisted (results/output are recomputed on Run).
```

- [ ] **Step 2: Backend suite + full build**

Run: `go test ./...`
Expected: all PASS (no Go changes this slice; confirms nothing broke).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`.

- [ ] **Step 3: Verify the compute worker runs in the PRODUCTION build (not just dev)**

Run: `./build/bin/texere`, add a Compute cell, run `return 1+1`.
Expected: `⇒ 2`. (This confirms Vite's `new Worker(new URL(...))` chunk resolves under the Wails asset server, the one real integration risk in this slice.)

- [ ] **Step 4: Execute the manual test plan**

Walk TC-1 … TC-9.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-07-23-processing-notebook-slice3-manual-test-plan.md
git commit -m "docs(test-plan): Processing Notebook Slice 3 manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** graph cell full-searchN4L-parity editor + inline full-SearchResult render (T1–T2), register + NotebookPane + LLM attach (T3), math.js/Chart.js deps (T4), sandboxed terminable worker + async graph bridge + timeout/cancel (T5), compute UI with console/value/charts (T6), register + button (T7), manual plan + production-worker verification (T8). Deferred (sketch/image/multimodal/`\notebooks`/feed-forward/extra kernels/isometric-in-cell) are out of scope by design.
- **Deviation (flagged, confirm at handoff):** compute value display uses the async wrapper's `return` value (top-level await supported), not implicit last-expression capture (which needs an AST transform). Documented in the header.
- **Known integration risk (has a verification step):** Vite worker chunk resolving under the Wails production asset server — T5 Step 3 and T8 Step 3 explicitly verify it in `make build`, not just dev.
- **Autocomplete field-name caveat:** T2 uses defensive field access for `query-grammar.js`'s suggestion/analysis shape with an instruction to confirm the real names; Run/render behavior does not depend on it.
- **Type consistency:** cell objects (`{type:'graph',query,resultText}`, `{type:'compute',language,code,resultText}`) match `newGraphCell`/`newComputeCell`, the `onChange(cell)`→`updateCellObj` path, and `buildSystem`'s `resultText` reads; the worker⇄host message protocol types (`run`/`graph-req`/`graph-res`/`console`/`chart`/`value`/`error`/`done`) are used identically on both sides.
```
