# Graph cell — Text/Isometric view + Expand — Manual Test Plan

Prereqs: open a collection; build the graph (Ctrl+G) so queries return results.

## TC-1 Text mode unchanged
1. Add a Graph cell (Text selected by default); run a query.
   - EXPECT: the text/structured result as before.

## TC-2 Graph mode renders the isometric result
1. Toggle to Graph; run a query that matches nodes.
   - EXPECT: the isometric subgraph renders inline with labeled nodes; pan (drag) and zoom (wheel) work.

## TC-3 Expand
1. In Graph mode, click ⛶ Expand.
   - EXPECT: the full isometric explorer opens on the same query (reanchor/sidebar/minimap available).

## TC-4 Persistence + toggle
1. Leave a cell in Graph mode; restart the app, reopen the extract.
   - EXPECT: the cell reopens in Graph mode.
2. Toggle back to Text.
   - EXPECT: the text result shows.

## TC-5 Empty / no graph
1. Run a query with no matches (Graph mode); and try Graph mode before building the graph.
   - EXPECT: a clear "No graph results." / "Build your graph first" message, no crash.

## TC-6 No leak / global explorer intact
1. Add several Graph cells and run them; then open the full explorer from the omnibar and reanchor.
   - EXPECT: cells render independently; the global explorer still works normally (a cell's GraphLayout didn't evict its session).
