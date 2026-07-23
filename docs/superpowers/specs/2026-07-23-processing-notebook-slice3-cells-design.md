# Processing Notebook — Slice 3: Graph query cell + Compute cell

**Date:** 2026-07-23
**Status:** Approved design
**Feature area:** Processing Notebook (incremental reading)
**Slice:** 3 of ~6 (see the Slice 1 design for the full decomposition)

## Summary

Add two new notebook cell types:

- **Graph query cell** — a searchN4L query editor that runs against the
  SSTorytime-KV graph (via the existing `SearchGraph`) and renders results
  inline: matched nodes with their radius-1 orbit for text queries, path chains
  for `\from`/`\to`/`\paths`/`\sequence` queries, and listings. Results attach
  into LLM cells as context.
- **Compute cell** — user JavaScript evaluated in a sandboxed, terminable Web
  Worker with **math.js** available, an async **`graph.query`/`graph.orbit`**
  bridge back to the graph, captured console/value output, and **Chart.js**
  plots. Cells are labelled with a kernel `language` (`js` only in v1) so
  additional kernels can be added later without reworking the cell framework.

Both cells slot into the Slice 1 `cellRegistry`; neither adds Go dependencies.

## Decisions made during brainstorming

- **One spec, two build phases:** 3a (graph query cell) first — small and
  reuse-heavy — then 3b (compute cell) — the net-new worker/libs/bridge work.
- **Full compute cell in v1:** math.js + async graph bridge + console/value
  output + Chart.js plots.
- **Manual Run for both cells** (not auto-run on open/edit): graph queries and
  arbitrary JS can be expensive, so execution is explicit.
- **Results are ephemeral, with a bounded text summary persisted for attach:**
  the query text / source code persists in `cells_json`; rendered results and
  chart output are recomputed on Run; a truncated `resultText` summary is
  persisted so attach-to-LLM works across restarts without bloating the row.

## Scope

**In scope (two phases):**

- **3a — Graph query cell**
  - searchN4L query editor reusing `frontend/src/lib/query-grammar.js`
    (`analyzeQuery`/`getSuggestions`) for autocomplete/help, so grammar never
    drifts from the unified search bar.
  - **Run** → inline render of `SearchResult`: nodes + radius-1 orbit
    (text queries), `Structural` path chains (`\from`/`\to`/`\paths`/`\sequence`),
    and `Listings`.
  - Attachable to LLM cells as context.
- **3b — Compute cell**
  - Fresh Web Worker per Run (`type: "module"`), math.js imported inside it.
  - Async `graph.query(q)` / `graph.orbit(node)` bridge (worker → main → Wails
    `SearchGraph` → back).
  - Captured `console.*` output + last-expression value; Chart.js plots via a
    worker-emitted chart spec rendered on the main thread.
  - **Stop** button + execution **timeout** (default 5s) → `worker.terminate()`.
  - Kernel-labelled architecture (`language: "js"`; JS kernel only in v1).

**Out of scope (later slices):** sketch cell (4); image cell + watched inbox
(5); multimodal LLM attachment, push-as-image, feed-forward leech, `\notebooks`
search (6); additional compute kernels; persisting full results or chart images.

## Architecture & component boundaries

```
frontend/src/
├── components/notebook/
│   ├── GraphQueryCell.svelte   # NEW: query editor + Run + inline results
│   ├── GraphResults.svelte     # NEW: renders nodes/orbits/Structural/Listings
│   └── ComputeCell.svelte      # NEW: code editor + Run/Stop + output + charts
├── lib/
│   ├── notebook.js             # register 'graph'/'compute'; newGraphCell/newComputeCell
│   ├── compute-worker.js       # NEW: the JS kernel Web Worker (math.js + graph RPC + chart/console shims)
│   └── compute-host.js         # NEW: main-thread runner — spawns worker, bridges graph.query, collects output, timeout
└── lib/llm.js                  # extend buildSystem to include graph/compute cells' resultText
```

- **`compute-host.js`** owns the worker lifecycle: `run(code, { onConsole, onValue,
  onChart, onError, onDone })` → spawns a worker, wires the graph RPC to
  `SearchGraph`, enforces the timeout, and returns a `cancel()` that terminates
  the worker. This keeps `ComputeCell.svelte` thin (UI only) and the worker
  protocol testable in isolation.
- **`compute-worker.js`** runs inside the worker: imports math.js, installs a
  `graph` object whose methods post RPC requests and await replies, a `chart()`
  that posts chart specs, and a `console` shim that posts log lines; evaluates
  the user code (async) and posts the final value / errors.
- **`GraphResults.svelte`** is a pure presentational component (props: a
  `SearchResult`) reused by the graph cell.

The cells render through the existing `cellRegistry` (Slice 1), so `NotebookPane`
needs no special-casing beyond passing the same `context`/`cells` props it
already passes and adding "+ Graph cell" / "+ Compute cell" buttons.

## Backend (Wails)

Read-only, minimal — the graph is already exposed:

- Reuse the existing `SearchGraph(query string) → (*search.SearchResult, error)`
  for both the graph cell and the compute bridge. `graph.orbit(node)` is a
  frontend convenience that calls `SearchGraph(node + " \\radius 1")` (the
  `\radius` directive already exists in the query grammar) — no new backend
  method.
- No new BadgerDB access, no new Go dependency. (If a dedicated
  `GraphQuery`/`GraphOrbit` binding proves cleaner during implementation, it is a
  thin pass-through to `SearchGraph`; the plan will decide.)

## Graph query cell (3a)

Cell object in `cells_json`:

```json
{ "id": "...", "type": "graph", "query": "", "resultText": "" }
```

- **Editor:** a query input using `analyzeQuery`/`getSuggestions` from
  `query-grammar.js` for the `\directive` autocomplete/help dropdown, matching
  the OmniBar UX.
- **Run:** calls `SearchGraph(query)`; renders via `GraphResults.svelte`:
  - text queries → each matched node with its radius-1 orbit (`arrowLong →
    neighborText`);
  - `\from`/`\to`/`\paths`/`\sequence` → `Structural` chains as arrow sequences;
  - `Listings` → a simple list.
- **Persistence:** `query` persists; results are ephemeral. On Run, a bounded
  plain-text summary of the results is written to `resultText` (persisted,
  truncated to a few KB) so an LLM cell can attach it after a restart without
  re-running.
- **Errors:** `SearchResult.GraphUnavailable` → "Build the graph (Ctrl+G)" hint;
  `SearchResult.GraphError` → shown inline. No crash on empty/invalid queries.

## Compute cell (3b)

Cell object in `cells_json`:

```json
{ "id": "...", "type": "compute", "language": "js", "code": "", "resultText": "" }
```

### Execution model

- **Run** spawns a fresh worker (clean global state each run) via
  `compute-host.js`. **Stop** and a **5s timeout** both call `worker.terminate()`
  and report "terminated". Manual Run only.
- The worker evaluates the user code as the body of an `async` function, so
  top-level `await` works (`await graph.query(...)`).

### Worker API surface (installed in `compute-worker.js`)

- `math` — the math.js namespace.
- `graph.query(q)` → `Promise<SearchResult>`; `graph.orbit(node)` →
  `Promise<SearchResult>`. Each posts `{type:"graph-req", id, kind, arg}`; the
  host calls `SearchGraph` and posts `{type:"graph-res", id, result|error}`; the
  worker resolves/rejects the pending Promise.
- `chart(config)` — posts `{type:"chart", config}`; the host renders it (below).
- `console.log/info/warn/error` — shimmed to post `{type:"console", level, text}`.
- The final expression value is posted as `{type:"value", text}` (stringified).

### Output rendering (main thread, in `ComputeCell.svelte`)

- Console lines and the final value render as a text log.
- Each `chart` spec renders into its own `<canvas>` via `new Chart(canvas,
  config)` (Chart.js). Chart.js `config` is JSON-serialisable, so it crosses
  `postMessage` intact. Charts are destroyed/recreated on each Run.
- **Persistence:** `code` persists; console/value/chart output is ephemeral. A
  bounded text summary of the console+value output is written to `resultText`
  (persisted, truncated) for attach-to-LLM.

### Kernel architecture (minimal)

The cell stores `language: "js"`. `compute-host.js` maps `language` → a worker
module (`js` → `compute-worker.js`). Adding a kernel later is a new worker module
plus a registry entry — no plugin framework, no dynamic loading in v1.

### Sandbox note

A Web Worker has no DOM access and is terminable, which is the isolation this
needs: it protects the main thread/UI from runaway or hanging user code and
keeps compute off the render thread. It is not a security boundary against the
user's own code (the code is the user's, running in their own app); math.js
`evaluate` and arbitrary JS are expected. Untrusted third-party code is not a
concern for this single-user desktop tool.

## Attach to LLM cells

Extend `buildSystem` in `frontend/src/lib/llm.js` to handle attached cells of
type `graph` and `compute`: append the cell's persisted `resultText` under a
labelled header (`# Attached graph result` / `# Attached compute output`), using
the same attachment mechanism Slice 2 built for `n4l`/`llm` cells. Charts are not
sent (text-only in this slice; multimodal is Slice 6). The LLM cell's
attach-context multiselect already lists sibling cells; graph/compute cells with
a non-empty `resultText` become selectable.

## Dependencies

New **npm** dependencies (frontend only; the pure-Go/no-CGo constraint governs Go
dependencies, not npm — and the original feature spec names both):

- **math.js** — symbolic/numeric math in the compute worker.
- **Chart.js** — plotting on the main thread.

The compute worker is bundled by Vite via `new Worker(new
URL('./compute-worker.js', import.meta.url), { type: 'module' })`. No new Go
dependencies; the graph is reached through the existing `SearchGraph` binding.

## Error handling

- **Graph cell:** query error / unbuilt graph surfaces inline via
  `SearchResult.GraphError`/`GraphUnavailable`; malformed queries return an empty
  result set, not a crash.
- **Compute cell:** a thrown error or syntax error renders as an error block with
  the message; an infinite loop hits the 5s timeout → terminate; a rejected
  `graph.query` (backend error) surfaces in the error block. Worker crash →
  "compute worker failed" with the message.
- Neither cell ever writes to `.n4l` files or the graph — both are read-only
  consumers.

## Testing

- **Go:** no substantial new tests — both cells reuse the already-tested
  `SearchGraph`. If a thin `GraphQuery`/`GraphOrbit` binding is added, a
  pass-through test covers it.
- **Frontend** (`npm run build` + a manual test plan in the Slice 1/2 style):
  run a text query (nodes+orbits) and a `\from…\to` query (path chains); a
  compute cell doing a math.js computation, an `await graph.query('…')` returning
  live results, and a Chart.js plot; a Stop and a timeout on a runaway loop; and
  attaching a graph cell's result into an LLM cell. `compute-host.js`'s worker
  protocol (graph RPC round-trip, timeout, cancel) is exercised via the manual
  plan; no JS test runner exists in this project.

## Success criteria

- A text query renders matched nodes with their orbit; a `\from…\to` query
  renders path chains.
- A compute cell computes with math.js, `await graph.query('…')` returns live
  graph results, and a `chart(...)` call renders a Chart.js plot.
- Stop and the 5s timeout both kill a runaway compute cell without freezing the
  UI.
- A graph cell's result attaches into an LLM cell's context and influences the
  reply.
- Query text and compute code persist across app restart.

## Dependencies (existing, reused)

`SearchGraph` (`internal/search`), `query-grammar.js`, Slice 1's
`cellRegistry`/`NotebookPane`/persistence, Slice 2's LLM-cell attach mechanism
(`llm.js` `buildSystem`), Wails bindings, Vite worker bundling.
