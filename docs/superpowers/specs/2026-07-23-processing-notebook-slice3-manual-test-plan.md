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

## TC-4 Compute cell — math.js, value, print
1. Add a Compute cell; enter `return math.sqrt(2) * 10`; Run.
   - EXPECT: `⇒ 14.14…` shown.
2. Replace with `print(math.sqrt(16)); console.log('hi', {a:1})`; Run.
   - EXPECT: a `print` line shows `4` (styled like a value) and the console line renders — no `return` needed.

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
