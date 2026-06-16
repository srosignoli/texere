package explorer

import "time"

// =============================================================================
// Wire types — exchanged with the frontend over Wails IPC.
// JSON tags match contracts/layout-types.md byte-for-byte.
// =============================================================================

// LayoutResult is the single response shape returned by ExplorerOpen and
// ExplorerReanchor. See contracts/layout-types.md §1.
type LayoutResult struct {
	ResultID    string          `json:"resultId"`
	AnchorID    string          `json:"anchorId"`
	Bounds      LayoutBounds    `json:"bounds"`
	Nodes       []NodeView      `json:"nodes"`
	Edges       []EdgeView      `json:"edges"`
	Layers      []LayerView     `json:"layers"`
	Territories []TerritoryView `json:"territories"` // deprecated: emitted empty post layered redesign; kept on the wire for one release window
	Diagnostics Diagnostics     `json:"diagnostics"`
}

// LayerView is one context floor in the layered knowledge graph. The
// Layered Knowledge Graph redesign stacks each primary context on its
// own translucent grid plane along Y; the frontend uses LayerView to
// draw the planes and their labels.
//
// CenterX / CenterZ / ExtentX / ExtentZ describe the cluster's
// bounding box on the (x, z) ground plane in world units (half-width
// for extents). In Stacked mode every layer's cluster is centered on
// the global anchor (CenterX=CenterZ=0) and its extents span all the
// layer's nodes. In Extended mode CenterX is the cluster's slot
// position in the centered X-row, so the frontend can paint a
// dedicated colored sub-plane for each context on the shared Y=0
// floor.
type LayerView struct {
	ContextID string   `json:"contextId"` // "" for the synthetic Untagged layer
	Label     string   `json:"label"`     // human-readable label rendered on the plane (= ContextID for tagged layers)
	Tags      []string `json:"tags"`      // every authored context tag across the layer's members, first-seen order
	Chapters  []string `json:"chapters"`  // distinct parent chapters across the layer's members, sorted; rendered as a sub-label
	Y         float32  `json:"y"`         // world-Y of this layer's plane
	NodeCount int      `json:"nodeCount"`
	ColorSlot int      `json:"colorSlot"` // index into the frontend's TERRITORY_PALETTE (reused for per-layer tint accents)
	CenterX   float32  `json:"centerX"`
	CenterZ   float32  `json:"centerZ"`
	ExtentX   float32  `json:"extentX"`
	ExtentZ   float32  `json:"extentZ"`
}

// LayoutBounds is the axis-aligned bounding box of all node positions.
// Used by the frontend to compute fit-to-result framing without iterating
// nodes.
type LayoutBounds struct {
	MinX float32 `json:"minX"`
	MaxX float32 `json:"maxX"`
	MinY float32 `json:"minY"`
	MaxY float32 `json:"maxY"`
	MinZ float32 `json:"minZ"`
	MaxZ float32 `json:"maxZ"`
}

// NodeView is one rendered node. See contracts/layout-types.md §2.
type NodeView struct {
	ID                 string  `json:"id"`
	Label              string  `json:"label"`
	X                  float32 `json:"x"`
	Y                  float32 `json:"y"`
	Z                  float32 `json:"z"`
	Size               float32 `json:"size"`
	ContextID          string  `json:"contextId"`
	InDegree           int     `json:"inDegree"`
	CycleWarning       bool    `json:"cycleWarning,omitempty"`
	DistanceFromAnchor int     `json:"distanceFromAnchor"`
}

// EdgeView is one rendered edge. See contracts/layout-types.md §3.
type EdgeView struct {
	SrcID           string `json:"srcId"`
	DstID           string `json:"dstId"`
	Meta            string `json:"meta"` // contains | leadsto | properties | similarto | unclassified | unknown
	ArrowLong       string `json:"arrowLong"`
	CrossesContexts bool   `json:"crossesContexts"`
	// Unknown is true when the arrow code was not declared in any SSTconfig
	// (synthesized at build via AugmentWithUnknownArrows). The frontend draws
	// these with a distinct dashed "unknown arrow code" style; ArrowLong holds
	// the authored code so the label can read e.g. "affects (unknown)".
	Unknown bool `json:"unknown,omitempty"`
}

// TerritoryView is one ground-plane context territory.
// See contracts/layout-types.md §4.
type TerritoryView struct {
	ContextID string  `json:"contextId"`
	Vertices  []Vec2J `json:"vertices"`
	ColorSlot int     `json:"colorSlot"`
}

// Vec2J is a 2D ground-plane coordinate (the Y axis is implicit at 0).
type Vec2J struct {
	X float32 `json:"x"`
	Z float32 `json:"z"`
}

// Diagnostics carries non-error signals to the frontend.
// See contracts/layout-types.md §5.
type Diagnostics struct {
	LayoutMs   float32 `json:"layoutMs"`
	Clamped    bool    `json:"clamped"`
	Truncated  int     `json:"truncated"`
	GraphError string  `json:"graphError,omitempty"`
	// MatchCount is the number of nodes that directly matched the query
	// (the seeds), as opposed to total rendered nodes which also include the
	// neighbors pulled in around each seed. Lets the UI explain why "Limit 10"
	// can render 34 nodes (10 matches + their orbits, deduped).
	MatchCount int `json:"matchCount"`
}

// =============================================================================
// Internal types — never cross the IPC boundary.
// See data-model.md §2.2–2.3.
// =============================================================================

// Vec3 is a 3D world-space coordinate used internally by the layout engine.
type Vec3 struct {
	X float32
	Y float32
	Z float32
}

// Vec2 is a 2D ground-plane coordinate used during territory computation.
type Vec2 struct {
	X float32
	Z float32
}

// nodeKey is the internal identity of a node. We use the node text plus
// chapter as a canonical key because the existing search layer
// (internal/search) returns text-keyed GraphNode records — the underlying
// SST.NodePtr is not exposed on the search-result surface. This is a
// pragmatic v1 choice; refining to NodePtr is a follow-up if we ever need
// to disambiguate identically-named nodes across chapters.
type nodeKey struct {
	Text    string
	Chapter string
}

// String returns the wire-format identifier for this node. The format is
// "<chapter>::<text>" (chapter may be empty); the same string is used as
// NodeView.ID and EdgeView.SrcID/DstID so the frontend can round-trip it
// through the IPC contract without translation.
func (k nodeKey) String() string {
	if k.Chapter == "" {
		return "::" + k.Text
	}
	return k.Chapter + "::" + k.Text
}

// ResultNode is one materialized node within an active ResultSet.
// See data-model.md §2.2.
type ResultNode struct {
	Key         nodeKey
	Text        string
	Chapter     string
	Contexts    []string // all tags; index 0 is the "primary"
	InDegree    int      // count of incoming edges within this ResultSet
}

// ResultEdge is one materialized typed edge.
type ResultEdge struct {
	Src       nodeKey
	Dst       nodeKey
	ArrowCode string
	ArrowLong string
	Meta      MetaType
	Unknown   bool // arrow code not declared in any SSTconfig (synthesized)
}

// ResultSet is the cached subgraph that drives layout passes. One active
// ResultSet exists at a time per the IPC contract (contracts/layout-ipc.md
// §7). All slices are deterministically sorted by node key so layout
// output is reproducible across runs.
type ResultSet struct {
	ID            string
	Nodes         []ResultNode
	Edges         []ResultEdge
	NodeIndex     map[nodeKey]int
	Contexts      []string          // distinct context tags, sorted
	SeedKeys      map[nodeKey]bool  // nodes that directly matched the query (vs. orbit neighbors)
	DefaultAnchor nodeKey
	BuiltAt       time.Time
	SourceQuery   string
	// PathLike is true when the result is a directed cone/path: every
	// structural chain is Kind "path" (a lone \from / \to cone or a \from…\to
	// solve) and no other result tier contributed nodes. The layout engine
	// uses this to auto-select the left-to-right Flow layout (LayoutModeFlow)
	// so the path reads along X in authored direction.
	PathLike bool
}

// LayoutPass is the output of LayoutFromAnchor. See data-model.md §2.3.
type LayoutPass struct {
	AnchorKey   nodeKey
	Positions   map[nodeKey]Vec3
	NodeSizes   map[nodeKey]float32
	Layers      []LayerPass
	Territories []TerritoryPolygon // deprecated, kept empty post layered redesign
	CycleNodes  map[nodeKey]bool
	Distances   map[nodeKey]int
	ComputedAt  time.Time
	LayoutMs    float32
}

// LayerPass is the internal pre-wire shape of one context floor.
type LayerPass struct {
	Context   string // "" for the synthetic Untagged layer
	Label     string
	Tags      []string // deduped union of every member's Contexts (first-seen order)
	Chapters  []string // distinct parent chapters across the layer's members, sorted
	Y         float32
	Members   []nodeKey
	ColorSlot int
	CenterX   float32
	CenterZ   float32
	ExtentX   float32
	ExtentZ   float32
}

// TerritoryPolygon is the internal representation of a context territory
// before it is converted to TerritoryView for the frontend. Deprecated:
// kept so existing tests / serialization paths still compile.
type TerritoryPolygon struct {
	Context  string
	Vertices []Vec2
}
