# Phase 1 Data Model: 2.5D Isometric Graph Explorer

**Date**: 2026-05-04
**Branch**: `006-isometric-graph-explorer`

This document captures the data shapes used by the explorer — both the internal Go types in `internal/explorer/` and the JSON exchanged with the frontend over Wails IPC. It complements [contracts/layout-types.md](./contracts/layout-types.md), which formalizes the JSON contract.

The model has three groups: **inputs** (what the explorer reads from the existing graph), **internals** (what the layout engine builds during a render pass), and **outputs** (what crosses the IPC boundary into the frontend).

---

## 1. Inputs (read-only from feature 004's layer)

These types already exist in the codebase. The explorer is a consumer; it does not own them.

### 1.1 Graph node (existing)

Source: `internal/search/types.go` `GraphNode` and the underlying `SST.NodePtr` from `sstorytime-kv`.

Fields the explorer relies on:

| Field | Type | Source | Used for |
|---|---|---|---|
| `NodePtr` | `SST.NodePtr` | sstorytime-kv | Stable identity, deterministic tie-break (cycle, multi-context) |
| `NodeText` | `string` | sstorytime-kv | Frontend label |
| `Chapter` | `string` | search | Default anchor candidate for chapter queries |
| `ContextTags` | `[]string` | search | Primary-context resolution (first tag wins) |
| `Orbit` | `[]OrbitEntry` | search | Edge enumeration: `(arrowCode, neighborText, radius, ctx)` |

### 1.2 Arrow registry (existing)

Source: `internal/content` `SSTRegistry`. The explorer reads `registry.Arrows[arrowCode].MetaType` to classify each edge into one of the four spatial axes.

| Field | Type | Used for |
|---|---|---|
| `Arrows[code].MetaType` | `string` (one of: `"contains"`, `"leadsto"`, `"properties"`, `"similarto"`) | Edge → Semantic Compass axis mapping |
| `Arrows[code].Long` | `string` | Pass-through to the frontend for legend display |
| `Arrows[code].Inverse` | `bool` | Sign-flip on the assigned axis |

If a `MetaType` is empty or unrecognized, the edge is treated as **unclassified**: it is included in the result for hover-traceability but does not contribute to spatial positioning. (Edge case alignment with FR-009.)

---

## 2. Internals (`internal/explorer/types.go`)

Types built during a layout pass. Not exported beyond the Wails surface.

### 2.1 `MetaType`

```go
type MetaType uint8

const (
    MetaUnclassified MetaType = iota
    MetaContains              // North → +Y elevation
    MetaLeadsTo               // East → +X horizontal flow
    MetaProperties            // South → +Z near depth
    MetaSimilarTo             // West → -Z far depth
)
```

Validation: parsed from registry strings; unknowns map to `MetaUnclassified` and the edge is layout-inert.

### 2.2 `ResultSet` (cached subgraph)

```go
type ResultSet struct {
    ID           string                  // opaque (UUID-like) identifier returned to the frontend
    Nodes        []ResultNode            // materialized subgraph node list
    Edges        []ResultEdge            // materialized subgraph edge list
    NodeIndex    map[SST.NodePtr]int     // NodePtr → index into Nodes (O(1) lookup)
    Contexts     []string                // distinct context tags present, in stable sorted order
    DefaultAnchor SST.NodePtr            // chosen by FR-008 logic
    BuiltAt      time.Time
    SourceQuery  string                  // for debugging only; not surfaced to UI
}

type ResultNode struct {
    Ptr        SST.NodePtr
    Text       string
    Chapter    string
    Contexts   []string                  // all tags; index 0 is the "primary"
    InDegree   int                       // count of incoming edges within this ResultSet
}

type ResultEdge struct {
    Src       SST.NodePtr
    Dst       SST.NodePtr
    ArrowCode string
    ArrowLong string
    Meta      MetaType
}
```

State transitions for `ResultSet`:

```
[non-existent] --ExplorerOpen--> [active]
[active]       --ExplorerOpen--> [active]   (prior result evicted)
[active]       --ExplorerClose-> [non-existent]
```

Only one `ResultSet` is active at a time (R-6 single-result cache).

### 2.3 `LayoutPass` (computed per anchor)

```go
type LayoutPass struct {
    AnchorPtr      SST.NodePtr
    Positions      map[SST.NodePtr]Vec3   // computed (x, y, z) per node
    NodeSizes      map[SST.NodePtr]float32 // log-centrality scaled, in world units
    Territories    []TerritoryPolygon
    CycleNodes     map[SST.NodePtr]bool   // nodes flagged as part of a containment cycle
    ComputedAt     time.Time
}

type Vec3 struct { X, Y, Z float32 }

type TerritoryPolygon struct {
    Context  string
    Vertices []Vec2          // (x, z) on the ground plane
    Color    string          // theme-resolved hex; "" lets frontend pick from palette
}

type Vec2 struct { X, Z float32 }
```

Validation rules:
- `AnchorPtr` MUST be present in `ResultSet.NodeIndex`; otherwise the service returns an error.
- `Positions` MUST contain an entry for every node in the `ResultSet`. Orphan nodes are placed at the ground-plane origin of their context's territory (FR edge case).
- `Vertices` per polygon: ≥3 (single-node groups receive a small disc polygon expanded to a triangulated hexagon to keep ≥3 verts).
- `NodeSizes` are clamped to `[minNodeSize, maxNodeSize]` (world units; the frontend separately ensures ≥24 px hit area).
- `CycleNodes` is non-empty only when at least one containment cycle was detected.

### 2.4 `LayoutResult` (the JSON shape returned to frontend)

```go
type LayoutResult struct {
    ResultID    string         `json:"resultId"`
    AnchorID    string         `json:"anchorId"`
    Bounds      LayoutBounds   `json:"bounds"`
    Nodes       []NodeView     `json:"nodes"`
    Edges       []EdgeView     `json:"edges"`
    Territories []TerritoryView `json:"territories"`
    Diagnostics Diagnostics    `json:"diagnostics"`
}

type LayoutBounds struct {
    MinX, MaxX, MinY, MaxY, MinZ, MaxZ float32 `json:"-"` // Inlined as 6 fields below in JSON
    // (JSON view uses minX/maxX/... — see contracts/layout-types.md)
}

type NodeView struct {
    ID            string   `json:"id"`              // canonical node identifier (stringified NodePtr)
    Label         string   `json:"label"`
    X, Y, Z       float32  `json:"x" "y" "z"`
    Size          float32  `json:"size"`            // world-units diameter
    ContextID     string   `json:"contextId"`
    InDegree      int      `json:"inDegree"`
    CycleWarning  bool     `json:"cycleWarning,omitempty"`
    DistanceFromAnchor int  `json:"distanceFromAnchor"` // BFS depth from anchor (any meta-type), drives reveal staggering
}

type EdgeView struct {
    SrcID    string   `json:"srcId"`
    DstID    string   `json:"dstId"`
    Meta     string   `json:"meta"`     // "contains" | "leadsto" | "properties" | "similarto" | "unclassified"
    ArrowLong string  `json:"arrowLong"`
    CrossesContexts bool `json:"crossesContexts"`
}

type TerritoryView struct {
    ContextID string  `json:"contextId"`     // human-readable tag string
    Vertices  []Vec2J `json:"vertices"`      // (x, z) ground-plane coordinates
    ColorSlot int     `json:"colorSlot"`     // index into the frontend's theme palette
}

type Vec2J struct { X, Z float32 }

type Diagnostics struct {
    LayoutMs     float32 `json:"layoutMs"`
    Clamped      bool    `json:"clamped"`     // true if input exceeded ceiling and was reduced
    Truncated    int     `json:"truncated"`   // count of nodes dropped due to clamping
    GraphError   string  `json:"graphError,omitempty"` // populated only on upstream failure
}
```

Notes:
- `NodeView.ID` is the stringified form of `SST.NodePtr` (decimal). Stable per build.
- `EdgeView.Meta` mirrors the registry's `MetaType` strings — frontend renders edge style and color from this.
- `Diagnostics.GraphError` is set when SSTorytime-KV is unavailable so the frontend can show the "Build your graph first (Ctrl+G)" hint without a separate error path.

### 2.5 Anchor history (frontend-only)

This entity lives entirely in the Svelte store `frontend/src/stores/explorer.js`; no Go counterpart.

```ts
type AnchorHistory = {
  back: string[]    // node IDs, oldest at index 0
  forward: string[] // node IDs, newest at index 0
  current: string   // active anchor node ID
  cap: 50           // FR-026g
}
```

State transitions:

```
on re-anchor(newID):
  if newID === current: no-op (FR-025a)
  else:
    back.push(current); if back.length > cap: back.shift()
    forward = []                                  (FR-026a)
    current = newID

on back():
  if back.empty: no-op (FR-026e)
  else:
    forward.unshift(current)
    current = back.pop()

on forward():
  if forward.empty: no-op (FR-026e)
  else:
    back.push(current); if back.length > cap: back.shift()
    current = forward.shift()

on new query (ExplorerOpen):
  back = []; forward = []; current = result.defaultAnchor   (FR-026f)

on close:
  history is discarded along with the explorer state
```

---

## 3. Relationships at a glance

```
SST.NodePtr (sstorytime-kv)
       │
       ▼
ResultNode  ──────┐
       │           │ (in)
       ▼           ▼
  ResultSet  ◄─── ResultEdge ────► MetaType ──► spatial axis
       │
       │  per anchor
       ▼
  LayoutPass ──► Positions (Vec3 per node)
       │      ──► NodeSizes (log centrality)
       │      ──► Territories (polygons per primary context)
       │      ──► CycleNodes (badged on frontend)
       ▼
  LayoutResult (JSON over Wails IPC)
       │
       ▼
  Frontend store ──► Three.js scene
       │
       ▼
  AnchorHistory (frontend-only, session-scoped)
```

---

## 4. Validation summary

| Constraint | Origin | Enforced by |
|---|---|---|
| Anchor must be in result | FR-025, FR-026 | `service.go` returns error if `anchorID ∉ resultSet.NodeIndex` |
| ≤500 nodes / ≤1500 edges | Spec ceiling, FR-034 | `service.go` clamps before layout; `Diagnostics.Clamped` is true |
| Same input → same output (deterministic) | Implicit (testability) | Stable iteration: sort by `NodePtr` everywhere; no map-iteration order leaks |
| Empty result → empty state | FR-032 | `service.go` returns `LayoutResult` with empty arrays + populated `Diagnostics.GraphError` if applicable |
| Unclassified edges allowed | FR-009 | `EdgeView.Meta == "unclassified"`; frontend renders as a neutral thin line, no axis contribution |
| Cycle must be flagged, never infinite-looped | FR-010 | `cycle.go` breaks deterministically; `CycleNodes` populated; bench test asserts <100 ms even with cycles |
| Multi-context node sits on first tag's territory | Spec assumption | `territory.go` uses `Contexts[0]` for primary assignment |
| All node labels must be readable | FR-015, FR-016 | Frontend: sprites with constant pixel size; not a Go-side concern |
| 24 px hit area | FR-014a | Frontend: separate picking mesh sized in screen space |
| Camera cannot rotate | FR-003 | Frontend: custom controller with no rotation code path |

All validation is checkable in unit tests for the Go side; frontend invariants are checked in [quickstart.md](./quickstart.md) scenarios.
