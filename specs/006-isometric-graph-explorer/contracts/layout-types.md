# Contract: JSON Shapes — Explorer Layout

**Date**: 2026-05-04
**Branch**: `006-isometric-graph-explorer`

This document is the canonical specification for the JSON exchanged between the Go backend (`internal/explorer/`) and the Svelte/Three.js frontend over Wails IPC. It is referenced by [layout-ipc.md](./layout-ipc.md).

All shapes use camelCase per the existing project convention (`internal/search/types.go` is similarly camelCased on the wire).

---

## 1. Top-level: `LayoutResult`

The single response shape returned by both `ExplorerOpen` and `ExplorerReanchor`.

```jsonc
{
  "resultId": "string (opaque, UUID-like)",
  "anchorId": "string (stringified NodePtr)",
  "bounds": {
    "minX": 0, "maxX": 0,
    "minY": 0, "maxY": 0,
    "minZ": 0, "maxZ": 0
  },
  "nodes": [ /* NodeView[]   — see §2 */ ],
  "edges": [ /* EdgeView[]   — see §3 */ ],
  "territories": [ /* TerritoryView[] — see §4 */ ],
  "diagnostics": { /* Diagnostics — see §5 */ }
}
```

Field semantics:
- `resultId` is opaque; the frontend stores it and passes it back on every `ExplorerReanchor`. Never displayed to the user.
- `anchorId` is the active anchor at time of return. Used by the frontend to highlight the anchor and pre-populate `AnchorHistory.current`.
- `bounds` is the axis-aligned bounding box of all node positions; used by the frontend to compute fit-to-result framing without iterating nodes.
- `nodes`, `edges`, `territories` are all empty when the result is empty (no results, graph unavailable). The frontend uses `diagnostics` to decide which empty-state message to show.

---

## 2. `NodeView`

```jsonc
{
  "id": "string (stringified NodePtr; stable per build)",
  "label": "string (NodeText)",
  "x": 0, "y": 0, "z": 0,
  "size": 1.0,
  "contextId": "string (primary context tag, or empty for context-less nodes)",
  "inDegree": 0,
  "cycleWarning": false,
  "distanceFromAnchor": 0
}
```

- `x` / `y` / `z` are world-space, with the anchor at the origin. Y is up (containment elevation), X is causal flow, Z is depth (properties = +Z, analogies = -Z).
- `size` is in world units (the visual diameter); the frontend separately ensures a 24 px hit area regardless of this value (FR-014a).
- `contextId` is the *first declared* tag (multi-context tie-break). Empty string is allowed for nodes with no context tags; such nodes sit on a neutral zone.
- `cycleWarning` is omitted (default `false`) for non-cycle nodes; present and `true` for nodes that participate in a containment cycle.
- `distanceFromAnchor` is BFS depth across all meta-types. Used by the frontend to stagger fade-in (closer nodes appear first).

Ordering: nodes are sorted by `id` ascending (deterministic across runs). The anchor is included in the array — its `x = y = z = 0` and `distanceFromAnchor = 0`.

---

## 3. `EdgeView`

```jsonc
{
  "srcId": "string",
  "dstId": "string",
  "meta": "contains | leadsto | properties | similarto | unclassified",
  "arrowLong": "string (human-readable arrow name)",
  "crossesContexts": false
}
```

- `meta` drives both the line-style mapping (FR-011) and the spatial-axis assignment used during layout.
- `arrowLong` is pass-through for the legend tooltip ("contains" might display its specific subtype, e.g., "is part of").
- `crossesContexts` is `true` when `nodes[srcId].contextId !== nodes[dstId].contextId`. Renders as a "bridge" edge (FR-020).

Ordering: edges are sorted by `(srcId, dstId, meta)` for determinism. Duplicate edges (same triple) are collapsed at the Go layer.

---

## 4. `TerritoryView`

```jsonc
{
  "contextId": "string (the context tag this territory represents)",
  "vertices": [ { "x": 0, "z": 0 }, /* ... ≥3 vertices ... */ ],
  "colorSlot": 0
}
```

- `vertices` are 2D ground-plane coordinates (y is implicitly 0). Ordering is counterclockwise (right-hand rule, y-up).
- `colorSlot` is an integer index into the frontend's theme palette. The Go side does not pick hex colors — the frontend resolves the slot against the active light/dark theme (FR-012, FR-035).
- One territory per distinct context in the result. Single-context results emit one large territory covering the entire bounds (FR-021).
- Context-less nodes do not produce a territory; they sit on the bare ground plane.

---

## 5. `Diagnostics`

```jsonc
{
  "layoutMs": 0.0,
  "clamped": false,
  "truncated": 0,
  "graphError": ""
}
```

- `layoutMs` is the wall-clock layout-pass duration in Go (excludes IPC overhead).
- `clamped` is `true` when the input result exceeded the ceiling and was reduced.
- `truncated` is the number of nodes dropped due to clamping. Frontend displays a small badge ("Showing 500 of 723 nodes — refine your query") when nonzero.
- `graphError` is empty for normal results; populated with a user-facing message when the SSTorytime-KV graph is unavailable. The frontend treats this as the empty-state message.

---

## 6. Stability and ordering

The Go layer guarantees:

1. **Deterministic ordering** — `nodes` sorted by `id`, `edges` sorted by `(srcId, dstId, meta)`, `territories` sorted by `contextId`, `vertices` per territory in counterclockwise order.
2. **Stable IDs** — `id` is the decimal string form of `SST.NodePtr`, which is stable as long as the underlying graph build has not been redone. Across a single explorer session, IDs do not change.
3. **Idempotent layout** — `ExplorerReanchor` with the same `(resultId, anchorId)` returns byte-identical JSON (subject to `layoutMs` jitter, which the frontend ignores).

These guarantees make snapshot tests (Go side) and screenshot tests (frontend, future) feasible.

---

## 7. Example payload (small, for clarity)

A 4-node result anchored on `42`:

```json
{
  "resultId": "8e7b1d0a-4f93-49a8-9e3f-2c9c1f01fa12",
  "anchorId": "42",
  "bounds": { "minX": -2.0, "maxX": 2.0, "minY": 0.0, "maxY": 1.5, "minZ": -1.5, "maxZ": 1.5 },
  "nodes": [
    { "id": "13", "label": "Vehicle", "x": 0.0, "y": 1.5, "z": 0.0, "size": 1.4, "contextId": "transport", "inDegree": 5, "cycleWarning": false, "distanceFromAnchor": 1 },
    { "id": "42", "label": "Car",     "x": 0.0, "y": 0.0, "z": 0.0, "size": 1.0, "contextId": "transport", "inDegree": 2, "cycleWarning": false, "distanceFromAnchor": 0 },
    { "id": "57", "label": "Wheel",   "x": 0.0, "y": 0.0, "z": 1.0, "size": 0.7, "contextId": "transport", "inDegree": 1, "cycleWarning": false, "distanceFromAnchor": 1 },
    { "id": "71", "label": "Bicycle", "x": 0.0, "y": 0.0, "z": -1.2, "size": 0.9, "contextId": "transport", "inDegree": 3, "cycleWarning": false, "distanceFromAnchor": 1 }
  ],
  "edges": [
    { "srcId": "13", "dstId": "42", "meta": "contains",   "arrowLong": "is a kind of", "crossesContexts": false },
    { "srcId": "42", "dstId": "57", "meta": "properties", "arrowLong": "has",          "crossesContexts": false },
    { "srcId": "42", "dstId": "71", "meta": "similarto",  "arrowLong": "similar to",   "crossesContexts": false }
  ],
  "territories": [
    { "contextId": "transport", "vertices": [{"x":-2.5,"z":-2.0},{"x":2.5,"z":-2.0},{"x":2.5,"z":2.0},{"x":-2.5,"z":2.0}], "colorSlot": 0 }
  ],
  "diagnostics": { "layoutMs": 1.2, "clamped": false, "truncated": 0, "graphError": "" }
}
```

This payload renders as: `Car` at the origin, `Vehicle` floating above it (containment, +Y), `Wheel` to one side at the same elevation (property, +Z), `Bicycle` to the opposite side (analogy, -Z), all on a single "transport" territory.
