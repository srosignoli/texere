# Data Model: Unified Search Bar with SSTorytime-KV Graph Integration

**Branch**: `004-unified-search-graph` | **Date**: 2026-04-17

---

## Go Backend Types (`internal/search/types.go`)

### SearchResult

Top-level return value from the `SearchGraph` Wails binding.

```go
type SearchResult struct {
    GraphUnavailable bool            // true when sst_data/ does not exist
    GraphError       string          // non-empty on open/query error
    Nodes            []GraphNode     // Tier 2 node results (plain text + structural)
    Listings         []ListingItem   // results from listing commands (\chapter any, \arrow, etc.)
    Structural       []StructuralChain // results from path/sequence commands
}
```

**Constraints**:
- Exactly one of `GraphUnavailable`, `GraphError`, or result slices is meaningful per call.
- All slices are non-nil empty slices (never null) so the frontend can iterate safely.
- `Nodes` length ≤ default limit (10) unless the user specifies `\limit N`.

---

### GraphNode

A single graph result node (Tier 2 card).

```go
type GraphNode struct {
    NodeText    string        // Node's text from the graph (Node.S in SSTorytime-KV)
    Chapter     string        // Chapter the node belongs to (Node.Chap)
    ContextTags []string      // Context tags active at this node
    Orbit       []OrbitEntry  // Immediate neighbors (radius=1 only)
}
```

**Notes**:
- `NodeText` is the join key used by the frontend to resolve `(filePath, line)` from `fileCache`.
- `ContextTags` are derived from the orbit/context stored in the graph; may differ from parsed N4L if the graph is stale.
- `Orbit` is capped at the first 8 entries per node to keep result cards compact.

---

### OrbitEntry

One immediate neighbor of a graph node.

```go
type OrbitEntry struct {
    ArrowCode    string // Short arrow code, e.g., "means", "e.g."
    ArrowLong    string // Human-readable arrow name
    Direction    string // "outbound" | "inbound"
    NeighborText string // Text of the neighbor node (Orbit.Text in SSTorytime-KV)
}
```

**Source**: Derived from `GetNodeOrbit` → `[ST_TOP][]Orbit`. STindex < ST_TOP/2 → outbound; ≥ ST_TOP/2 → inbound (convention).

---

### StructuralChain

Output of a path (`\from\to`, `\paths`) or sequence (`\sequence`) query.

```go
type StructuralChain struct {
    Nodes []ChainNode // Ordered chain of nodes
}

type ChainNode struct {
    NodeText  string // Node text
    ArrowCode string // Arrow leading to the NEXT node (empty for last node)
    ArrowLong string // Human-readable arrow name
}
```

**Display format**: `A → [arrowCode] → B → [arrowCode] → C`

**Navigability**: Each `ChainNode` is independently navigable by Enter if its `NodeText` resolves in `fileCache`.

---

### ListingItem

Output of listing commands (`\chapter any`, `\context [term]`, `\arrow [term]`, `\notes [chapter]`).

```go
type ListingItem struct {
    Kind    string // "chapter" | "context" | "arrow" | "note-line"
    Title   string // Chapter name, context tag, arrow code, or note line text
    Detail  string // Subtitle or secondary info (e.g., arrow long name, chapter for note-line)
    NodeText string // Non-empty if navigable (e.g., first node of a chapter)
}
```

**Navigability**: Items with non-empty `NodeText` are navigable via Enter.

---

## Frontend Types (Svelte / JS)

These are the in-memory representations used by `OmniBar.svelte`. They are derived from `SearchResult` after frontend enrichment.

### EnrichedGraphNode

```js
{
  nodeText: string,       // from GraphNode.NodeText
  chapter: string,        // from GraphNode.Chapter
  contextTags: string[],  // from GraphNode.ContextTags
  orbit: OrbitEntry[],    // from GraphNode.Orbit
  filePath: string|null,  // resolved from fileCache (null if not found)
  line: number|null,      // resolved from fileCache (null if not found)
  chapterTitle: string,   // resolved from fileCache
}
```

### FlatResult (unified navigation list)

The existing `flatResults` array in OmniBar is extended:

```js
{
  type: 'chapter' | 'context' | 'node'       // existing Tier 1 types
       | 'graph-node'                          // new: Tier 2 enriched node
       | 'graph-structural'                    // new: path/sequence chain node
       | 'graph-listing'                       // new: listing item
       | 'graph-unavailable'                   // new: sentinel (not navigable)
       | 'graph-error'                         // new: sentinel (not navigable)
       | 'graph-loading'                       // new: sentinel (not navigable)
  // ... type-specific fields
}
```

---

## State Additions to OmniBar.svelte

| Variable | Type | Purpose |
|----------|------|---------|
| `graphResults` | `SearchResult \| null` | Latest response from `SearchGraph` |
| `graphLoading` | `boolean` | True while Tier 2 query is in-flight |
| `debounceTimer` | `number \| null` | setTimeout handle for 200ms debounce |

---

## Go Backend State (`App` struct additions)

| Field | Type | Purpose |
|-------|------|---------|
| `searchSvc` | `*search.SearchService` | Holds open PoSST for the active collection |

Lifecycle:
- Created in `OpenCollection` (after graph path is known)
- Re-created when collection changes
- Closed in `shutdown()` via `searchSvc.Close()`
- Nil before first collection is opened → `SearchGraph` returns `GraphUnavailable: true`
