package explorer

import (
	"errors"
	"fmt"
	"regexp"
	"sync"

	"texere/internal/content"
	"texere/internal/search"
)

// MaxNodes is the hard ceiling on the number of nodes shipped to the
// frontend per result (FR-034). Beyond this the layout engine's wall-
// clock budget (T060 / TestLayoutBudget500) stops being defensible.
const MaxNodes = 500

// MaxEdges is the hard ceiling on edges shipped to the frontend. We
// allow 3× MaxNodes so a fully-connected radius-1 orbit at the cap is
// still rendered without truncation (each anchor on average carries
// ~3 typed neighbors in real .n4l data).
const MaxEdges = 1500

// DefaultResultLimit is the result cap the explorer injects when the user's
// query carries no explicit \limit. Without it the backend's MinMaxPolicy
// defaults a short search term to as few as 5 nodes, which makes the explorer
// show a tiny slice of one chapter while the OmniBar graph search (which
// injects the same default) shows the full breadth. Keep this in sync with
// the OmniBar's DEFAULT_LIMIT so the two surfaces agree. The MaxNodes clamp
// still bounds the rendered subgraph.
const DefaultResultLimit = 100

// limitDirectiveRE matches an explicit \limit directive (the canonical cap
// directive; \depth/\range/\distance are its aliases and all set the same
// internal Range). Mirrors the OmniBar's extractUserLimit regex.
var limitDirectiveRE = regexp.MustCompile(`(?i)\\(limit|depth|range|distance)\s+\d+`)

// ErrStaleResultID is returned by Reanchor when the supplied resultID does
// not match the active cache. The frontend treats this as a signal to call
// Open afresh. See contracts/layout-ipc.md §2.
var ErrStaleResultID = errors.New("stale resultId")

// ErrAnchorNotInResult is returned by Reanchor when the supplied anchorID
// is not a node in the cached ResultSet. Defense-in-depth — the frontend
// should never send this in normal flow because click targets are
// restricted to rendered nodes.
var ErrAnchorNotInResult = errors.New("anchor not in result")

// ErrEmptyQuery is returned by Open when the query is empty/whitespace.
// This is a programmer error, not a user-visible state.
var ErrEmptyQuery = errors.New("empty query")

// ExplorerService is the stateful façade that backs the Wails-bound
// ExplorerOpen / ExplorerReanchor / ExplorerClose methods on App. It
// owns a single-result cache (one active ResultSet at a time) protected
// by a mutex so concurrent IPC calls are serialized at the service level.
type ExplorerService struct {
	mu       sync.Mutex
	search   *search.SearchService
	registry *content.SSTRegistry

	active *ResultSet // nil when no result is loaded
}

// NewExplorerService constructs a service. The search service may be nil
// when no collection is loaded; in that case Open returns a LayoutResult
// with Diagnostics.GraphError populated so the frontend can render the
// "Build your graph first (Ctrl+G)" hint.
func NewExplorerService(searchSvc *search.SearchService, registry *content.SSTRegistry) *ExplorerService {
	return &ExplorerService{
		search:   searchSvc,
		registry: registry,
	}
}

// SetSearchService swaps in a new search service (called when the user
// opens a different collection). Any active ResultSet is evicted because
// it refers to the previous collection.
func (s *ExplorerService) SetSearchService(searchSvc *search.SearchService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.search = searchSvc
	s.active = nil
}

// Open runs the query against the search service, caches the materialized
// ResultSet, and returns the initial layout for the default anchor.
//
// User-visible failure modes (graph not built, empty result) are signaled
// via Diagnostics, not Go errors. Programmer errors (empty query) are
// signaled via Go errors per contracts/layout-ipc.md §4.
// Open runs the query, caches the materialized ResultSet, and returns
// an initial layout for the default anchor. `mode` selects the spatial
// encoding (LayoutModeStacked / LayoutModeExtended); empty defaults to
// stacked.
func (s *ExplorerService) Open(query, mode string) (LayoutResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs, out, err := s.layoutForQuery(query, mode)
	if rs != nil {
		s.active = rs // keep the session for Reanchor (unchanged behavior)
	}
	return out, err
}

// Layout computes a layout for a query WITHOUT storing it as the active
// session, so notebook graph cells can render a query without evicting the
// global explorer's active ResultSet.
func (s *ExplorerService) Layout(query, mode string) (LayoutResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, out, err := s.layoutForQuery(query, mode)
	return out, err
}

// layoutForQuery is the stateless core of Open: search → build → clamp →
// layout. It never touches s.active; the caller decides whether to store the
// returned ResultSet. Assumes s.mu is held.
func (s *ExplorerService) layoutForQuery(query, mode string) (*ResultSet, LayoutResult, error) {
	if isBlank(query) {
		return nil, LayoutResult{}, ErrEmptyQuery
	}
	if s.search == nil || !s.search.IsReady() {
		return nil, graphUnavailableResult(), nil
	}
	sr, err := s.search.Search(withDefaultLimit(query))
	if err != nil {
		return nil, graphUnavailableResult(), nil
	}
	if sr.GraphUnavailable {
		msg := sr.GraphError
		if msg == "" {
			msg = "Build your graph first (Ctrl+G) to enable graph search."
		}
		out := emptyLayoutResult()
		out.Diagnostics.GraphError = msg
		return nil, out, nil
	}
	rs := BuildResultSet(sr, s.registry, query)
	if len(rs.Nodes) == 0 {
		return rs, emptyLayoutResult(), nil
	}
	// Result-size clamping (FR-034). Drop the highest-keyed nodes (and
	// their incident edges) until under both caps. Keying is stable
	// across runs, so the same query always clamps to the same subset
	// — a property the frontend relies on for "Showing X of N" UX.
	truncated := clampResultSet(rs)
	rs.DefaultAnchor = pickDefaultAnchor(rs)
	pass := LayoutFromAnchor(rs, rs.DefaultAnchor, effectiveMode(rs, mode))
	out := s.toLayoutResult(rs, pass)
	out.Diagnostics.MatchCount = matchCount(rs)
	if truncated > 0 {
		out.Diagnostics.Clamped = true
		out.Diagnostics.Truncated = truncated
	}
	return rs, out, nil
}

// matchCount returns how many of the rendered nodes were direct query
// matches (seeds), as opposed to neighbors pulled in around them. Seeds
// dropped by clampResultSet no longer appear in NodeIndex and are excluded,
// so the count reflects what's actually on screen.
func matchCount(rs *ResultSet) int {
	if rs == nil {
		return 0
	}
	n := 0
	for k := range rs.SeedKeys {
		if _, ok := rs.NodeIndex[k]; ok {
			n++
		}
	}
	return n
}

// Reanchor runs a new layout pass against the cached ResultSet. The
// ResultSet itself is unchanged; only the layout transform is
// recomputed. `mode` lets the caller flip between stacked / extended
// without re-running the search.
func (s *ExplorerService) Reanchor(resultID, anchorID, mode string) (LayoutResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active == nil || s.active.ID != resultID {
		return LayoutResult{}, ErrStaleResultID
	}
	anchorKey, ok := keyFromString(s.active, anchorID)
	if !ok {
		return LayoutResult{}, ErrAnchorNotInResult
	}
	pass := LayoutFromAnchor(s.active, anchorKey, effectiveMode(s.active, mode))
	return s.toLayoutResult(s.active, pass), nil
}

// effectiveMode overrides the caller-requested layout mode with Flow when the
// result is a directed cone/path (ResultSet.PathLike). Cone results always read
// best left-to-right, so we pin them to Flow regardless of the frontend's
// stacked/extended toggle in v1; every other result honors the requested mode.
func effectiveMode(rs *ResultSet, requested string) string {
	if rs != nil && rs.PathLike {
		return LayoutModeFlow
	}
	return requested
}

// Close evicts the active ResultSet. No-op if none is loaded.
func (s *ExplorerService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = nil
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// clampResultSet enforces MaxNodes and MaxEdges in place. Nodes are
// deterministically ordered (rs.Nodes is key-sorted by BuildResultSet);
// we drop from the high-keyed tail until under both caps. Edges
// pointing at a dropped node are removed; NodeIndex is rebuilt; per-
// node in-degree is recomputed against the surviving edges so
// centrality reflects the rendered subgraph (FR-014).
//
// Returns the number of nodes actually dropped — this is what surfaces
// in Diagnostics.Truncated. Same query → same drop set, so the
// "Showing X of N" hint is stable across re-anchors of the same
// result.
func clampResultSet(rs *ResultSet) int {
	if rs == nil {
		return 0
	}
	originalNodes := len(rs.Nodes)
	if originalNodes <= MaxNodes && len(rs.Edges) <= MaxEdges {
		return 0
	}

	// Step 1: cap node count. rs.Nodes is already key-sorted ascending
	// by BuildResultSet, so trimming the tail drops the highest keys
	// deterministically (FR-034 rationale: stable across queries).
	if len(rs.Nodes) > MaxNodes {
		dropped := rs.Nodes[MaxNodes:]
		rs.Nodes = rs.Nodes[:MaxNodes]
		// Drop edges that point at any pruned node.
		droppedSet := make(map[nodeKey]struct{}, len(dropped))
		for _, n := range dropped {
			droppedSet[n.Key] = struct{}{}
		}
		kept := rs.Edges[:0]
		for _, e := range rs.Edges {
			if _, gone := droppedSet[e.Src]; gone {
				continue
			}
			if _, gone := droppedSet[e.Dst]; gone {
				continue
			}
			kept = append(kept, e)
		}
		rs.Edges = kept
	}

	// Step 2: cap edge count. Surviving edges are key-sorted; trim the
	// tail. We don't prune the nodes referenced by trimmed edges —
	// orphan nodes are a valid layout state (placeFallback handles them).
	if len(rs.Edges) > MaxEdges {
		rs.Edges = rs.Edges[:MaxEdges]
	}

	// Rebuild NodeIndex and recompute in-degree against the surviving
	// edges so centrality and color slot maps remain coherent.
	rs.NodeIndex = make(map[nodeKey]int, len(rs.Nodes))
	for i, n := range rs.Nodes {
		rs.Nodes[i].InDegree = 0
		rs.NodeIndex[n.Key] = i
	}
	for _, e := range rs.Edges {
		if idx, ok := rs.NodeIndex[e.Dst]; ok {
			rs.Nodes[idx].InDegree++
		}
	}

	return originalNodes - len(rs.Nodes)
}

// pickDefaultAnchor implements FR-008: prefer the highest-in-degree node;
// break ties by node-key ordering for determinism.
//
// A future refinement (out of scope for v1 foundational) is to honor the
// originating query's target node when present.
func pickDefaultAnchor(rs *ResultSet) nodeKey {
	if len(rs.Nodes) == 0 {
		return nodeKey{}
	}
	// Cone/path results anchor on the source (the \from endpoint), not the
	// busiest node: the root has in-degree 0, so the highest-in-degree
	// heuristic below would otherwise pick a leaf and read the path backwards.
	// rs.Nodes is key-sorted, so the first zero-in-degree node is deterministic.
	if rs.PathLike {
		for _, n := range rs.Nodes {
			if n.InDegree == 0 {
				return n.Key
			}
		}
	}
	best := rs.Nodes[0]
	for _, n := range rs.Nodes[1:] {
		if n.InDegree > best.InDegree {
			best = n
		}
	}
	return best.Key
}

// toLayoutResult converts a LayoutPass into the wire LayoutResult that
// crosses the IPC boundary. Sorts nodes by ID and edges by (srcId, dstId,
// meta) so the JSON is deterministic across runs.
func (s *ExplorerService) toLayoutResult(rs *ResultSet, pass *LayoutPass) LayoutResult {
	out := LayoutResult{
		ResultID:    rs.ID,
		AnchorID:    pass.AnchorKey.String(),
		Bounds:      computeBounds(pass),
		Nodes:       make([]NodeView, 0, len(rs.Nodes)),
		Edges:       make([]EdgeView, 0, len(rs.Edges)),
		Layers:      layerViews(pass),
		Territories: []TerritoryView{}, // deprecated, kept empty so old wire consumers still parse
	}

	for _, n := range rs.Nodes {
		ctxID := ""
		if len(n.Contexts) > 0 {
			ctxID = n.Contexts[0]
		}
		pos := pass.Positions[n.Key]
		size := pass.NodeSizes[n.Key]
		if size == 0 {
			size = MinNodeSize
		}
		dist, hasDist := pass.Distances[n.Key]
		if !hasDist {
			dist = -1
		}
		out.Nodes = append(out.Nodes, NodeView{
			ID:                 n.Key.String(),
			Label:              n.Text,
			X:                  pos.X,
			Y:                  pos.Y,
			Z:                  pos.Z,
			Size:               size,
			ContextID:          ctxID,
			InDegree:           n.InDegree,
			CycleWarning:       pass.CycleNodes[n.Key],
			DistanceFromAnchor: dist,
		})
	}
	for _, e := range rs.Edges {
		srcCtx, dstCtx := primaryContextOf(rs, e.Src), primaryContextOf(rs, e.Dst)
		// Unknown arrows override the spatial meta with a neutral "unknown"
		// token so the frontend draws them dashed, off the Compass axes — the
		// authored code rides along in ArrowLong for the "code (unknown)" label.
		meta := e.Meta.String()
		if e.Unknown {
			meta = "unknown"
		}
		out.Edges = append(out.Edges, EdgeView{
			SrcID:           e.Src.String(),
			DstID:           e.Dst.String(),
			Meta:            meta,
			ArrowLong:       e.ArrowLong,
			CrossesContexts: srcCtx != dstCtx,
			Unknown:         e.Unknown,
		})
	}
	out.Diagnostics.LayoutMs = pass.LayoutMs
	return out
}

// layerViews converts LayoutPass.Layers into the wire LayerView slice.
// Order preserved (Untagged first, then alphabetical), so the frontend
// builds checkboxes in a stable order.
func layerViews(pass *LayoutPass) []LayerView {
	if len(pass.Layers) == 0 {
		return []LayerView{}
	}
	out := make([]LayerView, 0, len(pass.Layers))
	for _, lp := range pass.Layers {
		tags := lp.Tags
		if tags == nil {
			tags = []string{}
		}
		chapters := lp.Chapters
		if chapters == nil {
			chapters = []string{}
		}
		out = append(out, LayerView{
			ContextID: lp.Context,
			Label:     lp.Label,
			Tags:      tags,
			Chapters:  chapters,
			Y:         lp.Y,
			NodeCount: len(lp.Members),
			ColorSlot: lp.ColorSlot,
			CenterX:   lp.CenterX,
			CenterZ:   lp.CenterZ,
			ExtentX:   lp.ExtentX,
			ExtentZ:   lp.ExtentZ,
		})
	}
	return out
}

// primaryContextOf returns the first context tag of a node in the result
// set, or empty string if the node has no contexts (or is missing — the
// latter shouldn't happen in well-formed input).
func primaryContextOf(rs *ResultSet, k nodeKey) string {
	idx, ok := rs.NodeIndex[k]
	if !ok {
		return ""
	}
	if len(rs.Nodes[idx].Contexts) == 0 {
		return ""
	}
	return rs.Nodes[idx].Contexts[0]
}

// keyFromString reverses nodeKey.String() — the wire ID is "<chapter>::<text>".
// We split on the first "::" because chapter names may contain colons but
// the format is documented to use "::" as the canonical separator.
func keyFromString(rs *ResultSet, id string) (nodeKey, bool) {
	for k := range rs.NodeIndex {
		if k.String() == id {
			return k, true
		}
	}
	return nodeKey{}, false
}

// emptyLayoutResult returns a zero-content LayoutResult with non-nil
// slice fields so the frontend can iterate without null checks.
func emptyLayoutResult() LayoutResult {
	return LayoutResult{
		Nodes:       []NodeView{},
		Edges:       []EdgeView{},
		Layers:      []LayerView{},
		Territories: []TerritoryView{},
	}
}

// graphUnavailableResult is the response returned when no search service
// is available (no collection loaded, search not ready).
func graphUnavailableResult() LayoutResult {
	out := emptyLayoutResult()
	out.Diagnostics.GraphError = "Build your graph first (Ctrl+G) to enable graph search."
	return out
}

// isBlank reports whether s is empty or contains only whitespace.
// withDefaultLimit prepends a default \limit when the query specifies none,
// so the explorer asks the backend for the same breadth of results the
// OmniBar graph search does. A user-supplied \limit (or alias) is left
// untouched. Listing/notes queries are unaffected — \limit only widens the
// candidate pool; it never narrows an explicit result.
func withDefaultLimit(query string) string {
	if limitDirectiveRE.MatchString(query) {
		return query
	}
	return fmt.Sprintf("\\limit %d %s", DefaultResultLimit, query)
}

func isBlank(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}
