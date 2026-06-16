package search

// NewSearchResult returns a SearchResult with all slice fields initialized
// as non-nil empty slices so the frontend can safely iterate without null checks.
func NewSearchResult() *SearchResult {
	return &SearchResult{
		Nodes:      []GraphNode{},
		Listings:   []ListingItem{},
		Structural: []StructuralChain{},
		Notes:      []FullNote{},
	}
}

// SearchResult is the top-level return value from the SearchGraph Wails binding.
type SearchResult struct {
	GraphUnavailable bool              `json:"GraphUnavailable"`
	GraphError       string            `json:"GraphError"`
	Nodes            []GraphNode       `json:"Nodes"`
	Listings         []ListingItem     `json:"Listings"`
	Structural       []StructuralChain `json:"Structural"`
	Notes            []FullNote        `json:"Notes"`
}

// FullNote groups the per-line PageMap rows of a single chapter into the
// authored note blocks that \notes renders.  Chapters appear in the order
// their first row is returned by the backend (currently Chap-alphabetical).
type FullNote struct {
	Chapter string      `json:"Chapter"`
	Blocks  []NoteBlock `json:"Blocks"`
}

// NoteBlock is one root node together with every arrow authored under it
// (including arrows on its ditto lines).  Arrows are kept in file order.
type NoteBlock struct {
	NodeText string      `json:"NodeText"`
	Line     int         `json:"Line"`    // 1-indexed source line of the root node
	Context  string      `json:"Context"` // Comma-separated context tag string for display
	Arrows   []NoteArrow `json:"Arrows"`
}

// NoteArrow renders one arrow leg of a note block.  ArrowLong is the human
// name ("has frequency"); ArrowCode is the short code ("freq") kept for
// future click-to-filter UX; Target is the resolved node text.
type NoteArrow struct {
	ArrowLong string `json:"ArrowLong"`
	ArrowCode string `json:"ArrowCode"`
	Target    string `json:"Target"`
}

// GraphNode is a single Tier 2 graph result card.
type GraphNode struct {
	NodeText    string      `json:"NodeText"`
	Chapter     string      `json:"Chapter"`
	ContextTags []string    `json:"ContextTags"`
	Orbit       []OrbitEntry `json:"Orbit"`
}

// OrbitEntry is one orbit satellite of a graph node.
// Radius mirrors SST.Orbit.Radius: 1 = immediate neighbor, 2 = 2nd degree, etc.
// Ctx is a comma-separated context tag string (empty when no context).
// ParentText is the node this satellite hangs off (the seed for Radius 1,
// otherwise the Radius-1 hop, etc.) so the explorer can draw an edge for
// EVERY radius rather than only radius 1. Empty parent means the seed itself.
type OrbitEntry struct {
	ArrowLong    string `json:"ArrowLong"`
	NeighborText string `json:"NeighborText"`
	ParentText   string `json:"ParentText"`
	Radius       int    `json:"Radius"`
	Ctx          string `json:"Ctx"`
}

// StructuralChain is the output of a path (\from\to, \paths) or sequence (\sequence) query.
// Kind distinguishes "path" chains (rendered as horizontal arrow-labeled rows) from
// "sequence" chains (rendered as numbered vertical timelines). For \sequence results,
// Context is the sequence label (e.g. "pod lifecycle") and Chapter is the owning
// chapter title — the frontend groups chains by these so mixed sequences render as
// separate numbered timelines.
type StructuralChain struct {
	Kind    string      `json:"Kind"`
	Nodes   []ChainNode `json:"Nodes"`
	Context string      `json:"Context"`
	Chapter string      `json:"Chapter"`
}

// ChainNode is a single node in a structural chain.
// ArrowCode and ArrowLong are the arrow leading to the NEXT node (empty for the last node).
type ChainNode struct {
	NodeText  string `json:"NodeText"`
	ArrowCode string `json:"ArrowCode"`
	ArrowLong string `json:"ArrowLong"`
}

// ListingItem is the output of a listing command (\chapter any, \context, \arrow, \notes).
type ListingItem struct {
	Kind     string   `json:"Kind"`     // "chapter" | "context" | "arrow" | "note-line" | "hint"
	Title    string   `json:"Title"`
	Detail   string   `json:"Detail"`
	NodeText string   `json:"NodeText"` // non-empty if navigable (first node for navigation)
	Nodes    []string `json:"Nodes"`    // all node texts for chapter listings
	Contexts []string `json:"Contexts"` // distinct contexts within a chapter (TOC view)
	Chapters []string `json:"Chapters"` // for Kind:"context" — chapters that use this tag (capped server-side)
	MetaType string   `json:"MetaType"` // arrow meta-type (e.g. "contains", "properties")
	Context  string   `json:"Context"`  // section header where the arrow was defined, e.g. "identity orientation"
}
