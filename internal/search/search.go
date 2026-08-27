package search

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	SST "github.com/srosignoli/sstorytime-kv/pkg/SSTorytime"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"texere/internal/content"
)

// orbitDepthRE matches the texere-only \radius N directive. Anchored on a
// word/whitespace boundary on the left so it doesn't trip on, e.g., \radiusx.
var orbitDepthRE = regexp.MustCompile(`(?i)(^|\s)\\radius\s+(\d+)`)

// spaceCollapseRE collapses runs of whitespace left after stripping a
// mid-query directive into a single space.
var spaceCollapseRE = regexp.MustCompile(`\s{2,}`)

// extractOrbitDepth pulls a \radius N directive out of the query (if any),
// returning the clamped depth and the query with the directive removed. When
// absent it returns DefaultOrbitDepth and the query unchanged. Depth is
// clamped to [1, MaxOrbitDepth] so a deep traversal can't blow past the
// explorer's render ceiling.
func extractOrbitDepth(query string) (int, string) {
	m := orbitDepthRE.FindStringSubmatch(query)
	if m == nil {
		return DefaultOrbitDepth, query
	}
	depth, _ := strconv.Atoi(m[2])
	if depth < 1 {
		depth = 1
	}
	if depth > MaxOrbitDepth {
		depth = MaxOrbitDepth
	}
	// Replace the directive (and its leading separator) with a single space,
	// then collapse any resulting double space so a mid-query \radius doesn't
	// leave a gap. Trim the ends for the leading/trailing directive cases.
	cleaned := orbitDepthRE.ReplaceAllString(query, " ")
	cleaned = strings.TrimSpace(spaceCollapseRE.ReplaceAllString(cleaned, " "))
	return depth, cleaned
}

type queryRoute int

const (
	routeNodeSearch queryRoute = iota
	routeStructural
	routeListing
	routeNotes
)

// Orbit expansion tuning.
//
//   - orbitSatelliteCap bounds how many neighbors a single seed contributes,
//     regardless of depth, so a high-degree hub can't flood the result. This
//     is intentionally orthogonal to \radius: radius controls how FAR we
//     traverse, this controls how MANY satellites we keep.
//   - DefaultOrbitDepth is the BFS depth used when the query carries no
//     \radius directive. 2 preserves the historical radius-1+radius-2 orbit.
//   - MaxOrbitDepth clamps \radius so a deep traversal can't explode past the
//     explorer's render ceiling (a hairball that just hits the node clamp).
const (
	orbitSatelliteCap = 8
	DefaultOrbitDepth = 2
	MaxOrbitDepth     = 3
)

// BleveSearcher is the small slice of *SST.Index that the search overlay
// needs. Defined as an interface so tests can substitute a stub that asserts
// (or panics) when called from a route that should bypass Bleve (FR-004).
type BleveSearcher interface {
	SearchByQuery(queryText string, limit int) ([]SST.NodePtr, error)
	Close() error
}

// SearchService wraps the SSTorytime-KV PoSST and manages the BadgerDB lifecycle.
// The store is opened lazily on first Search() call and kept open until Close().
type SearchService struct {
	dbPath        string
	blevePath     string
	collectionDir string
	registry      *content.SSTRegistry
	store         *SST.BadgerKV
	pst           SST.PoSST
	bleveIdx      BleveSearcher
	mu            sync.Mutex
	// nodeContexts maps BadgerDB NodePtr → context tags sourced from the N4L files.
	// Populated during lazy open; used by \notes \context filtering.
	nodeContexts map[SST.NodePtr][]string
	// textToDBPtr maps lower-cased node text → BadgerDB NodePtr. Built once at
	// lazy open so \from/\to endpoint resolution can do O(1) exact-text lookup
	// without issuing a KV read per NODE_CACHE entry.
	textToDBPtr map[string]SST.NodePtr
	// tagChapters inverts the PageMap into "context tag → chapters that use it",
	// powering the inline chapter chips on \context listings. Built lazily on
	// first \context call and reset on store reopen (Close → re-lazy-open).
	tagChapters      map[string][]string
	tagChaptersBuilt bool
	// parsedCache holds the files parsed during augmentUnknownArrows so the
	// immediately-following rebuildMemoryLinks can reuse them instead of
	// re-parsing the whole collection a second time (parsing dominates
	// lazy-open cost on large collections). Cleared after rebuild.
	parsedCache map[string]*content.ParsedFile
}

// NewSearchService creates a SearchService pointed at <collectionDir>/sst_data.
func NewSearchService(collectionDir string, registry *content.SSTRegistry) *SearchService {
	return &SearchService{
		dbPath:        filepath.Join(collectionDir, "sst_data"),
		blevePath:     filepath.Join(collectionDir, "bleve"),
		collectionDir: collectionDir,
		registry:      registry,
	}
}

// IsReady returns true if both sst_data/ and bleve/ directories exist
// (the graph has been built and both indexes are present on disk).
func (s *SearchService) IsReady() bool {
	if _, err := os.Stat(s.dbPath); err != nil {
		return false
	}
	if _, err := os.Stat(s.blevePath); err != nil {
		return false
	}
	return true
}

// Close shuts down the BadgerDB and Bleve connections if open.
func (s *SearchService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		SST.Close(s.pst)
		s.store = nil
	}
	if s.bleveIdx != nil {
		_ = s.bleveIdx.Close()
		s.bleveIdx = nil
	}
}

// Search executes a graph search query. The store is opened lazily on first call
// (after Ctrl+G has built sst_data). All SSTorytime-KV panics are recovered.
func (s *SearchService) Search(query string) (result *SearchResult, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result = NewSearchResult()

	// Lazy open
	if s.store == nil {
		if !s.IsReady() {
			result.GraphUnavailable = true
			return result, nil
		}
		SST.NODE_DIRECTORY = SST.NodeDirectory{}
		SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
		SST.MemoryInit()
		// Synthesize ArrowDefs for any unregistered arrow codes BEFORE
		// RegisterArrows, mirroring what graph.BuildFromCollection does, so
		// the ARROW_DIRECTORY (and thus ArrowPtr indices) match the build's.
		// Without this, unknown-arrow links the build materialized would fail
		// to resolve here and the explorer/orbit would omit them.
		s.augmentUnknownArrows()
		RegisterArrows(s.registry)
		kv, openErr := SST.OpenBadgerStore(s.dbPath)
		if openErr != nil {
			result.GraphError = openErr.Error()
			return result, nil
		}
		s.store = kv
		s.pst = SST.PoSST{KV: kv}
		SST.Configure(s.pst, false)
		bleveIdx, bleveErr := SST.OpenBleveIndex(s.blevePath)
		if bleveErr != nil {
			s.store = nil
			SST.Close(s.pst)
			switch {
			case errors.Is(bleveErr, SST.ErrIndexNotFound):
				result.GraphUnavailable = true
				result.GraphError = "search index missing — press Ctrl+G to build"
			case errors.Is(bleveErr, SST.ErrIndexCorrupt):
				result.GraphUnavailable = true
				result.GraphError = "search index corrupted — press Ctrl+G to rebuild"
			default:
				result.GraphError = fmt.Sprintf("bleve index unavailable: %v — press Ctrl+G to rebuild", bleveErr)
			}
			return result, nil
		}
		s.bleveIdx = bleveIdx
		// Populate NODE_CACHE from BadgerDB, and collect text→DBptr for link rebuild
		// and \from/\to endpoint resolution.
		textToDBPtr := make(map[string]SST.NodePtr)
		s.pst.KV.IterateNodes(func(n SST.Node, ptr SST.NodePtr) bool {
			SST.CacheNode(n)
			textToDBPtr[strings.ToLower(strings.TrimSpace(n.S))] = ptr
			return true
		})
		s.textToDBPtr = textToDBPtr
		// Rebuild in-memory link arrays (NODE_DIRECTORY.I[stIdx]) from the N4L
		// source files. These arrays are only populated during BuildFromCollection
		// and are lost on app restart; re-parsing here avoids a mandatory Ctrl+G
		// after every restart.
		s.rebuildMemoryLinks(textToDBPtr)
		s.tagChapters = nil
		s.tagChaptersBuilt = false
	}

	// Recover from SSTorytime-KV panics so the overlay never crashes the app.
	defer func() {
		if r := recover(); r != nil {
			result = NewSearchResult()
			result.GraphError = fmt.Sprintf("search error: %v", r)
		}
	}()

	// \radius N is a texere-only directive controlling orbit BFS depth. It is
	// NOT understood by SST.DecodeSearchField (which would treat "radius" as a
	// search term), so we extract and strip it before decoding. Both the
	// OmniBar graph search and the explorer route through here, so the same
	// query text produces the same depth on both surfaces — by construction.
	depth, query := extractOrbitDepth(query)

	// Refuse malformed Dirac notation before decoding. Upstream exits the
	// process on it rather than returning an error, so this has to be caught
	// here or not at all — the deferred recover() above cannot see os.Exit.
	if bad := badDiracToken(query); bad != "" {
		result = NewSearchResult()
		result.GraphError = "cannot use " + bad + " — angle brackets are node-path shorthand and must be <a|b> or <a|context|b>"
		return result, nil
	}

	params := SST.DecodeSearchField(query)
	_, maxlimit := SST.MinMaxPolicy(params)
	if maxlimit <= 0 {
		maxlimit = 10
	}
	// Ceiling to bound Bleve candidate pool (limit*10) and graph traversal cost.
	if maxlimit > 10000 {
		maxlimit = 10000
	}

	arrowptrs, sttype := SST.ArrowPtrFromArrowsNames(s.pst, params.Arrows)

	switch s.classifyQuery(params) {
	case routeNodeSearch:
		nodes, searchErr := s.searchNodes(params, arrowptrs, maxlimit, depth)
		if searchErr != nil {
			var pe *SST.ParseError
			if errors.As(searchErr, &pe) {
				result.GraphError = pe.Error()
			} else {
				result.GraphError = searchErr.Error()
			}
			return result, nil
		}
		result.Nodes = nodes
	case routeStructural:
		result.Structural = s.searchStructural(params, arrowptrs, sttype, maxlimit)
	case routeListing:
		result.Listings = s.searchListing(params, arrowptrs, sttype, maxlimit)
	case routeNotes:
		result.Notes = s.searchNotes(params, query)
	}

	return result, nil
}

// classifyQuery maps parsed SearchParameters to a query route.
func (s *SearchService) classifyQuery(params SST.SearchParameters) queryRoute {
	from := len(params.From) > 0
	to := len(params.To) > 0
	seq := params.Sequence
	name := len(params.Name) > 0
	chapter := params.Chapter != ""
	context := len(params.Context) > 0
	pagenr := params.PageNr > 0
	arrows := len(params.Arrows) > 0

	// Structural: closed path (\from + \to), open causal cone (\from XOR \to),
	// or sequence (\sequence). A lone \from is a FORWARD cone and a lone \to a
	// BACKWARD cone — mirroring upstream SSTorytime's Search(), which routes any
	// of (name|from|to) into CausalCones when no closing endpoint is given.
	// This check must precede the arrow-listing check below so `\from X \leadsto`
	// (from set, name empty, arrows set) routes structurally, not to a listing.
	if from || to || seq {
		return routeStructural
	}

	// Listing: chapter/context TOC when no name given
	if (chapter || context) && !name && !pagenr && !from && !to {
		return routeListing
	}

	// Listing: arrow lookup without name
	if arrows && !name {
		return routeListing
	}

	// Notes: \notes [chapter] [\context tag ...]
	if pagenr {
		return routeNotes
	}

	// Default: node text search (plain text, \chapter filter, \context filter with name)
	return routeNodeSearch
}

// normalizeForSearch lowercases and strips diacritics for accent-insensitive matching.
// It also strips surrounding parentheses, which are SST's "unaccent" hint syntax.
func normalizeForSearch(s string) string {
	s = strings.Trim(s, "()")
	s = strings.TrimSpace(strings.ToLower(s))
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}

// sequenceContextLabel returns a human-readable label for the sequence context,
// stripping the `_sequence_` marker that just means "this is a sequence". When
// no descriptive tags remain, returns "" so the frontend knows to group under
// chapter alone.
func sequenceContextLabel(ctx []string) string {
	tags := stripSeqMarkerTags(ctx)
	return strings.Join(tags, ", ")
}

func stripSeqMarkerTags(ctx []string) []string {
	out := make([]string, 0, len(ctx))
	for _, c := range ctx {
		tag := strings.TrimSpace(c)
		if tag == "" || tag == "_sequence_" {
			continue
		}
		out = append(out, tag)
	}
	return out
}

// hasSequenceMarker reports whether a node is inside a `:: _sequence_ ::`
// block. Only such nodes are considered legitimate sequence members — plain
// nodes with outgoing leadsto arrows are regular relationships, not sequences.
func hasSequenceMarker(ctx []string) bool {
	for _, c := range ctx {
		if strings.TrimSpace(c) == "_sequence_" {
			return true
		}
	}
	return false
}

// sameSequenceContextTags reports whether two nodes belong to the same
// `:: _sequence_, <label> ::` block by comparing every context tag except the
// `_sequence_` marker as an unordered set. Mirrors graph.sameSequenceContext.
func sameSequenceContextTags(a, b []string) bool {
	as := stripSeqMarkerTags(a)
	bs := stripSeqMarkerTags(b)
	if len(as) != len(bs) {
		return false
	}
	seen := make(map[string]struct{}, len(as))
	for _, tag := range as {
		seen[tag] = struct{}{}
	}
	for _, tag := range bs {
		if _, ok := seen[tag]; !ok {
			return false
		}
	}
	return true
}

// searchNodes handles plain text, context-scoped, and chapter-scoped queries.
// When params.Name is non-empty the query text is delegated to the Bleve
// overlay (SearchByQuery), which handles stemming, CJK bigrams, accent
// folding, and the operator grammar. The returned NodePtrs are then
// fetched and filtered by chapter / context, preserving Bleve's score
// order (FR-023a).
func (s *SearchService) searchNodes(params SST.SearchParameters, arrowptrs []SST.ArrowPtr, limit, depth int) ([]GraphNode, error) {
	chapLower := normalizeForSearch(params.Chapter)
	isChapFilter := chapLower != "" && chapLower != "%%" && chapLower != "tableofcontents"

	if len(params.Name) == 0 {
		return s.searchNodesWildcard(params, isChapFilter, chapLower, limit, depth), nil
	}

	queryText := joinBleveQueryText(params.Name)
	seeds, err := s.bleveIdx.SearchByQuery(queryText, limit*10)
	if err != nil {
		return nil, err
	}

	nodes := make([]GraphNode, 0, limit)
	for _, ptr := range seeds {
		n := SST.GetDBNodeByNodePtr(s.pst, ptr)
		if n.S == "" {
			continue
		}
		if isChapFilter {
			matched := false
			for _, tok := range strings.Split(n.Chap, ",") {
				if strings.Contains(normalizeForSearch(tok), chapLower) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if len(params.Context) > 0 {
			memPtr, ok := SST.NODE_CACHE[ptr]
			if !ok {
				continue
			}
			memNode := SST.GetMemoryNodeFromPtr(memPtr)
			ctxMatched := false
		outerCtx:
			for st := 0; st < SST.ST_TOP; st++ {
				for _, lnk := range memNode.I[st] {
					if lnk.Ctx != 0 && SST.MatchContexts(params.Context, lnk.Ctx) {
						ctxMatched = true
						break outerCtx
					}
				}
			}
			if !ctxMatched {
				continue
			}
		}
		nodes = append(nodes, GraphNode{
			NodeText:    n.S,
			Chapter:     n.Chap,
			ContextTags: nodeContextTags(ptr),
			Orbit:       s.buildOrbit(ptr, orbitSatelliteCap, depth, params.Context),
		})
		if len(nodes) >= limit {
			break
		}
	}
	return nodes, nil
}

// joinBleveQueryText reconstructs a Bleve query string from the per-token
// names produced by SST.DecodeSearchField. SplitQuotes strips surrounding
// `"…"` and joins on whitespace, so an item with embedded spaces can only
// have come from a quoted phrase atom — re-wrap such items in quotes so the
// phrase semantics survive the round-trip into SearchByQuery.
func joinBleveQueryText(names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		if strings.ContainsAny(n, " \t") {
			parts[i] = `"` + n + `"`
		} else {
			parts[i] = n
		}
	}
	return strings.Join(parts, " ")
}

// searchNodesWildcard preserves the existing IterateNodes path for queries
// with no name term (FR-021): \chapter / \context listings without text.
func (s *SearchService) searchNodesWildcard(params SST.SearchParameters, isChapFilter bool, chapLower string, limit, depth int) []GraphNode {
	nodes := make([]GraphNode, 0, limit)
	s.pst.KV.IterateNodes(func(n SST.Node, ptr SST.NodePtr) bool {
		if n.S == "" {
			return true
		}
		if isChapFilter {
			matched := false
			for _, tok := range strings.Split(n.Chap, ",") {
				if strings.Contains(normalizeForSearch(tok), chapLower) {
					matched = true
					break
				}
			}
			if !matched {
				return true
			}
		}
		if len(params.Context) > 0 {
			memPtr, ok := SST.NODE_CACHE[ptr]
			if !ok {
				return true
			}
			memNode := SST.GetMemoryNodeFromPtr(memPtr)
			ctxMatched := false
		outerCtx:
			for st := 0; st < SST.ST_TOP; st++ {
				for _, lnk := range memNode.I[st] {
					if lnk.Ctx != 0 && SST.MatchContexts(params.Context, lnk.Ctx) {
						ctxMatched = true
						break outerCtx
					}
				}
			}
			if !ctxMatched {
				return true
			}
		}
		nodes = append(nodes, GraphNode{
			NodeText:    n.S,
			Chapter:     n.Chap,
			ContextTags: nodeContextTags(ptr),
			Orbit:       s.buildOrbit(ptr, orbitSatelliteCap, depth, params.Context),
		})
		return len(nodes) < limit
	})
	return nodes
}

// nodeContextTags collects unique context tag strings from all in-memory links
// of the node identified by its BadgerDB NodePtr.
func nodeContextTags(ptr SST.NodePtr) []string {
	memPtr, ok := SST.NODE_CACHE[ptr]
	if !ok {
		return nil
	}
	memNode := SST.GetMemoryNodeFromPtr(memPtr)
	seen := make(map[string]bool)
	var tags []string
	for st := 0; st < SST.ST_TOP; st++ {
		for _, lnk := range memNode.I[st] {
			if lnk.Ctx == 0 {
				continue
			}
			for _, tag := range strings.Split(SST.GetContext(lnk.Ctx), ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" && !seen[tag] {
					seen[tag] = true
					tags = append(tags, tag)
				}
			}
		}
	}
	return tags
}

// chapterContexts returns the distinct context-tag strings of every link
// authored under any node whose Chap field substring-matches chapName
// (case-insensitive, accent-folded — mirrors the chapter filter used in
// searchNodes / searchNodesWildcard). Order is alphabetical for stable
// output. Used by searchListing to render \chapter as a table of contents.
func (s *SearchService) chapterContexts(chapName string) []string {
	normChap := normalizeForSearch(chapName)
	if normChap == "" {
		return nil
	}
	seen := make(map[string]bool)
	out := []string{}
	s.pst.KV.IterateNodes(func(n SST.Node, ptr SST.NodePtr) bool {
		matched := false
		for _, tok := range strings.Split(n.Chap, ",") {
			if strings.Contains(normalizeForSearch(tok), normChap) {
				matched = true
				break
			}
		}
		if !matched {
			return true
		}
		for _, tag := range nodeContextTags(ptr) {
			if !seen[tag] {
				seen[tag] = true
				out = append(out, tag)
			}
		}
		return true
	})
	sort.Strings(out)
	return out
}

// buildOrbit returns the orbit for a node using direct in-memory traversal.
// GetNodeOrbit cannot be used here because it calls GetDBNodeByNodePtr with
// in-memory ptrs (per-class indices) that don't match BadgerDB sequence IDs.
// Instead we walk NODE_DIRECTORY directly via in-memory ptrs, using
// GetMemoryNodeFromPtr for text lookup at every hop.
// nptr is a BadgerDB NodePtr; NODE_CACHE bridges it to the in-memory index.
// orbitSTOrder is the deterministic STtype scan order used when expanding a
// node's links. Express relations first, then containment (inverse), flow,
// and nearness — matching the priority the original radius-1/2 orbit used.
var orbitSTOrder = []int{
	SST.STTypeToSTIndex(SST.EXPRESS),
	SST.STTypeToSTIndex(-SST.EXPRESS),
	SST.STTypeToSTIndex(-SST.CONTAINS),
	SST.STTypeToSTIndex(SST.LEADSTO),
	SST.STTypeToSTIndex(-SST.LEADSTO),
	SST.STTypeToSTIndex(SST.NEAR),
}

// buildOrbit assembles a node's neighborhood as a breadth-first spanning tree
// out to `depth` hops, keeping at most `cap` satellites. This mirrors the
// depth-parametric cone traversal upstream SSTorytime uses (GetFwdConeAsLinks
// is a `for d < depth` BFS) while preserving texere's two additions:
//
//   - \context scoping: a link declared under a non-matching context is
//     skipped, so scoped queries never leak neighbors from sibling contexts.
//   - deterministic STtype ordering + dedup so output is reproducible.
//
// Each node is discovered exactly once, at its shortest radius, and records
// the ParentText it was reached from. That gives every satellite exactly one
// connecting edge at any depth — the explorer can therefore draw edges for
// EVERY radius, not just radius 1, so deeper rings stay connected rather than
// floating. depth=1 yields direct neighbors only; the historical default is 2.
//
// nptr is a BadgerDB NodePtr; NODE_CACHE bridges it to the in-memory index.
func (s *SearchService) buildOrbit(nptr SST.NodePtr, cap, depth int, ctxFilter []string) []OrbitEntry {
	memPtr, ok := SST.NODE_CACHE[nptr]
	if !ok {
		return nil
	}
	if depth < 1 {
		depth = 1
	}

	rootNode := SST.GetMemoryNodeFromPtr(memPtr)

	entries := make([]OrbitEntry, 0, cap)
	// seen tracks node texts already discovered (root included) so each node
	// is emitted once, at its shortest hop distance, and never re-expanded.
	seen := map[string]bool{rootNode.S: true}

	type frame struct {
		memPtr SST.NodePtr
		text   string
		radius int
	}
	queue := []frame{{memPtr: memPtr, text: rootNode.S, radius: 0}}

	for len(queue) > 0 && len(entries) < cap {
		cur := queue[0]
		queue = queue[1:]
		if cur.radius >= depth {
			continue // reached the depth ceiling; don't expand further
		}
		node := SST.GetMemoryNodeFromPtr(cur.memPtr)
		for _, stIdx := range orbitSTOrder {
			if len(entries) >= cap {
				break
			}
			for _, lnk := range node.I[stIdx] {
				if len(entries) >= cap {
					break
				}
				if len(ctxFilter) > 0 && !SST.MatchContexts(ctxFilter, lnk.Ctx) {
					continue
				}
				hop := SST.GetMemoryNodeFromPtr(lnk.Dst)
				if hop.S == "" || seen[hop.S] {
					continue
				}
				seen[hop.S] = true
				arr := SST.GetDBArrowByPtr(s.pst, lnk.Arr)
				entries = append(entries, OrbitEntry{
					ArrowLong:    arr.Long,
					NeighborText: hop.S,
					ParentText:   cur.text,
					Radius:       cur.radius + 1,
					Ctx:          SST.GetContext(lnk.Ctx),
				})
				queue = append(queue, frame{memPtr: lnk.Dst, text: hop.S, radius: cur.radius + 1})
			}
		}
	}
	return entries
}

// resolvePathEndpoints maps \from/\to names to in-memory NodePtrs.
//
// We walk NODE_CACHE first for an exact (case-folded) text match — this keeps
// unambiguous endpoints dominant on real collections where a plain word like
// "start" can be drowned out by thousands of unrelated lines that contain it
// (e.g. Moby-Dick paragraphs). When no exact-text match exists we fall back
// to a Bleve seed lookup so partial / stemmed / accent-folded inputs still
// resolve.
func (s *SearchService) resolvePathEndpoints(names []string, limit int) []SST.NodePtr {
	seen := make(map[SST.NodePtr]bool)
	out := make([]SST.NodePtr, 0, len(names))
	addMem := func(mem SST.NodePtr) {
		if seen[mem] {
			return
		}
		seen[mem] = true
		out = append(out, mem)
	}
	for _, raw := range names {
		isEx, stripped := SST.IsExactMatch(raw)
		clean := raw
		if isEx {
			clean = stripped
		}
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		target := strings.ToLower(clean)
		if dbPtr, ok := s.textToDBPtr[target]; ok {
			if memPtr, ok := SST.NODE_CACHE[dbPtr]; ok {
				addMem(memPtr)
			}
			continue
		}
		// No exact-text match. When the user requested an EXACT match (`!start!`
		// / `|start|`) we must NOT fuzzy-fallback: if that node doesn't exist the
		// endpoint resolves to nothing, rather than to unrelated Bleve hits that
		// would seed a cone/path rooted at the wrong nodes. Honoring `!` is the
		// whole point of the syntax.
		if isEx {
			continue
		}
		// Bleve fallback for non-exact inputs (e.g. \from salm → "grilled salmon").
		// Score-ordered seeds enter the path walk in iteration order; the
		// exact-text preference above keeps an unambiguous endpoint dominant.
		seeds, qerr := s.bleveIdx.SearchByQuery(clean, limit)
		if qerr != nil {
			continue
		}
		for _, dbPtr := range seeds {
			if memPtr, ok := SST.NODE_CACHE[dbPtr]; ok {
				addMem(memPtr)
			}
		}
	}
	return out
}

// searchStructural handles \from+\to path queries and \sequence queries.
func (s *SearchService) searchStructural(params SST.SearchParameters, arrowptrs []SST.ArrowPtr, sttype []int, limit int) []StructuralChain {
	chains := make([]StructuralChain, 0)

	if params.Sequence {
		isWildcard := len(params.Name) == 0 ||
			(len(params.Name) == 1 && params.Name[0] == "%%")

		var nodeptrs []SST.NodePtr
		if isWildcard {
			s.pst.KV.IterateNodes(func(n SST.Node, ptr SST.NodePtr) bool {
				if n.S == "" {
					return true
				}
				if params.Chapter != "" {
					if !strings.Contains(
						normalizeForSearch(n.Chap),
						normalizeForSearch(params.Chapter)) {
						return true
					}
				}
				if !hasSequenceMarker(s.nodeContexts[ptr]) {
					return true
				}
				nodeptrs = append(nodeptrs, ptr)
				return len(nodeptrs) < limit*10
			})
		} else {
			// Bleve seeds the sequence-head candidates: tokens are passed through
			// the same operator grammar used by routeNodeSearch (joinBleveQueryText
			// preserves quoted phrases). Score-ordered seeds are filtered down to
			// nodes that carry the `_sequence_` marker; chain ordering downstream
			// is governed by the longest-fwd-path resort, not Bleve (FR-023a).
			queryText := joinBleveQueryText(params.Name)
			dbPtrs, qerr := s.bleveIdx.SearchByQuery(queryText, limit)
			if qerr != nil {
				return chains
			}
			for _, p := range dbPtrs {
				if hasSequenceMarker(s.nodeContexts[p]) {
					nodeptrs = append(nodeptrs, p)
				}
			}
		}

		if len(nodeptrs) == 0 {
			return chains
		}

		// GetSequenceContainers → GetLongestAxialPath → GetFwdPathsAsLinks reads
		// the in-memory NODE_DIRECTORY which uses in-memory ptrs. Convert DB ptrs
		// to in-memory ptrs via NODE_CACHE before passing them in. Keep the pair
		// so we can recover per-chain context from s.nodeContexts (keyed by DB ptr).
		type candidate struct {
			db  SST.NodePtr
			mem SST.NodePtr
		}
		candidates := make([]candidate, 0, len(nodeptrs))
		for _, dbPtr := range nodeptrs {
			if memPtr, ok := SST.NODE_CACHE[dbPtr]; ok {
				candidates = append(candidates, candidate{db: dbPtr, mem: memPtr})
			}
		}
		if len(candidates) == 0 {
			return chains
		}

		arrows := arrowptrs
		if len(arrows) == 0 {
			// Only leadsto-type arrows form sequences.
			// STAindex for forward leadsto = ST_ZERO + LEADSTO = 4.
			for _, ad := range SST.ARROW_DIRECTORY {
				if ad.STAindex == SST.ST_ZERO+SST.LEADSTO {
					arrows = append(arrows, ad.Ptr)
				}
			}
		}
		if len(arrows) == 0 {
			return chains
		}

		// Pre-compute the longest fwd path for each (candidate, arrow) pair so
		// we can emit chains longest-first. Without this, mid-chain candidates
		// iterated before the sequence head would emit a shorter tail and block
		// the head's longer chain by the seen[] dedup.
		type scored struct {
			cand   candidate
			arrDir SST.ArrowDirectory
			path   []SST.Link
		}
		scoredChains := make([]scored, 0, len(candidates)*len(arrows))
		for _, ap := range arrows {
			arrDir := SST.ARROW_DIRECTORY[ap]
			stypeForArrow := SST.STIndexToSTType(arrDir.STAindex)
			for _, cand := range candidates {
				paths, _ := SST.GetFwdPathsAsLinks(s.pst, cand.mem, stypeForArrow, limit, limit)
				if len(paths) == 0 {
					continue
				}
				var bestPath []SST.Link
				for _, p := range paths {
					var filtered []SST.Link
					for _, lnk := range p {
						if lnk.Arr == ap {
							filtered = append(filtered, lnk)
						} else {
							break
						}
					}
					if len(filtered) > len(bestPath) {
						bestPath = filtered
					}
				}
				if len(bestPath) == 0 {
					continue
				}
				scoredChains = append(scoredChains, scored{
					cand: cand, arrDir: arrDir, path: bestPath,
				})
			}
		}
		sort.SliceStable(scoredChains, func(i, j int) bool {
			return len(scoredChains[i].path) > len(scoredChains[j].path)
		})

		seen := make(map[string]bool)
		for _, sc := range scoredChains {
			startNode := SST.GetMemoryNodeFromPtr(sc.cand.mem)
			key := startNode.S
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			chain := StructuralChain{
				Kind:    "sequence",
				Nodes:   make([]ChainNode, 0, len(sc.path)+1),
				Context: sequenceContextLabel(s.nodeContexts[sc.cand.db]),
				Chapter: startNode.Chap,
			}
			chain.Nodes = append(chain.Nodes, ChainNode{
				NodeText:  startNode.S,
				ArrowCode: sc.arrDir.Short,
				ArrowLong: sc.arrDir.Long,
			})
			for i, lnk := range sc.path {
				dst := SST.GetMemoryNodeFromPtr(lnk.Dst)
				cn := ChainNode{NodeText: dst.S}
				if i < len(sc.path)-1 {
					cn.ArrowCode = sc.arrDir.Short
					cn.ArrowLong = sc.arrDir.Long
				}
				chain.Nodes = append(chain.Nodes, cn)
				// Mark every node the chain covers so mid-chain candidates
				// don't re-emit a shorter tail of the same sequence.
				seen[dst.S] = true
			}
			chains = append(chains, chain)
			if len(chains) >= limit {
				return chains
			}
		}
		return chains
	}

	// Open causal cone: a lone \from (forward) or lone \to (backward), with no
	// closing endpoint. Upstream SSTorytime routes these through CausalCones →
	// GetFwdPathsAsLinks; we do the same and emit each root-to-leaf path as a
	// chain, so the explorer renders the focused cone subgraph reachable from
	// the endpoint rather than dumping the whole graph (the bug a lone \from
	// otherwise fell into via the node-search wildcard path).
	fromOnly := len(params.From) > 0 && len(params.To) == 0
	toOnly := len(params.To) > 0 && len(params.From) == 0
	if fromOnly || toOnly {
		names := params.From
		if toOnly {
			names = params.To
		}
		endpoints := s.resolvePathEndpoints(names, limit)
		return s.searchCone(endpoints, sttype, limit, fromOnly)
	}

	// Path query: \from + \to
	// Resolve endpoints to memory ptrs. SolveNodePtrs does substring matching
	// with an alphabetical limit, so the target node itself can be missed when
	// many unrelated nodes sort before it (e.g. the `start` node is drowned out
	// by Moby-Dick paragraphs containing "start"). Prefer exact-text matches
	// over substring results.
	leftptrs := s.resolvePathEndpoints(params.From, limit)
	rightMem := s.resolvePathEndpoints(params.To, limit)
	if len(leftptrs) == 0 || len(rightMem) == 0 {
		return chains
	}
	rightSet := make(map[SST.NodePtr]bool, len(rightMem))
	for _, mp := range rightMem {
		rightSet[mp] = true
	}

	// Forward BFS over NODE_DIRECTORY. The upstream GetPathsAndSymmetries expands
	// via sst.KV.GetNode which doesn't materialise node.I[] from BadgerDB, so its
	// wave fronts never overlap past depth 1. A direct in-memory walk is both
	// correct and faster for the graph sizes typical to a user's collection.
	allowArrow := func(a SST.ArrowPtr) bool {
		if len(arrowptrs) == 0 {
			return true
		}
		for _, ap := range arrowptrs {
			if ap == a {
				return true
			}
		}
		return false
	}
	type frame struct {
		at   SST.NodePtr
		path []SST.Link
	}
	solutions := make([][]SST.Link, 0)
	seenStart := make(map[SST.NodePtr]bool)
	for _, start := range leftptrs {
		if seenStart[start] {
			continue
		}
		seenStart[start] = true
		queue := []frame{{at: start}}
		for len(queue) > 0 && len(solutions) < limit {
			cur := queue[0]
			queue = queue[1:]
			if len(cur.path) >= limit {
				continue
			}
			node := SST.GetMemoryNodeFromPtr(cur.at)
			for st := SST.ST_ZERO; st < SST.ST_TOP; st++ {
				for _, lnk := range node.I[st] {
					if !allowArrow(lnk.Arr) {
						continue
					}
					cyclic := lnk.Dst == start
					for _, leg := range cur.path {
						if leg.Dst == lnk.Dst {
							cyclic = true
							break
						}
					}
					if cyclic {
						continue
					}
					newPath := make([]SST.Link, len(cur.path)+1)
					copy(newPath, cur.path)
					newPath[len(cur.path)] = lnk
					if rightSet[lnk.Dst] {
						// Prepend a synthetic link carrying the start ptr so the
						// chain builder below can read node 0 uniformly.
						full := make([]SST.Link, 0, len(newPath)+1)
						full = append(full, SST.Link{Dst: start})
						full = append(full, newPath...)
						solutions = append(solutions, full)
						if len(solutions) >= limit {
							break
						}
					}
					queue = append(queue, frame{at: lnk.Dst, path: newPath})
				}
			}
		}
	}
	_ = sttype

	for _, path := range solutions {
		if len(path) == 0 {
			continue
		}
		startNode := SST.GetMemoryNodeFromPtr(path[0].Dst)
		chain := StructuralChain{
			Kind:    "path",
			Nodes:   make([]ChainNode, 0, len(path)),
			Chapter: startNode.Chap,
		}
		for i, link := range path {
			node := SST.GetMemoryNodeFromPtr(link.Dst)
			cn := ChainNode{NodeText: node.S}
			// link.Arr on index i >= 1 describes the edge FROM the previous node
			// TO this one — attach it to the previous chain node.
			if i > 0 && len(chain.Nodes) > 0 {
				arr := SST.GetDBArrowByPtr(s.pst, link.Arr)
				prev := &chain.Nodes[len(chain.Nodes)-1]
				prev.ArrowCode = arr.Short
				prev.ArrowLong = arr.Long
			}
			chain.Nodes = append(chain.Nodes, cn)
		}
		if len(chain.Nodes) > 0 {
			chains = append(chains, chain)
		}
	}
	return chains
}

// coneMaxDepth bounds how many hops a causal cone traverses from its endpoint.
// Generous enough to span a realistic story path while keeping a pathological
// deep chain from blowing past the explorer's node clamp.
const coneMaxDepth = 24

// coneNodeCap / coneEdgeCap bound a cone's BFS so a high-degree hub endpoint
// can't enumerate an unbounded subgraph. Set above the explorer's MaxNodes/
// MaxEdges clamp (500/1500) so the clamp, not these, governs what renders.
const (
	coneNodeCap = 2000
	coneEdgeCap = 6000
)

// coneSTTypes is the set of relation STtypes a cone follows when the query
// carries no explicit arrow constraint. A causal "cone path" follows DIRECTED
// relations — LEADSTO (sequential flow, e.g. the doors `fwd` arrow) and CONTAINS
// (hierarchy) — in their authored direction. The SYMMETRIC relations EXPRESS
// (synonyms/translations such as the chinese `eh`/`hp` arrows) and NEAR
// (proximity) are deliberately excluded: they have no meaningful path direction,
// and following them makes a cone bleed sideways into translations/synonyms of
// the start node (e.g. `start → 开始`), fusing otherwise-unrelated chapters.
// An explicit arrow constraint (`\from X \someArrow`) overrides this default.
var coneSTTypes = []int{SST.LEADSTO, SST.CONTAINS}

// searchCone emits the forward (or backward) causal cone from the endpoints as
// a layered DAG. It runs a multi-source breadth-first walk over the in-memory
// link arrays, keeping ONLY strictly-outward edges (depth(dst) == depth(src)+1).
//
// Walking the explicit BFS — rather than upstream GetFwdPathsAsLinks — fixes two
// problems that surface on real collections where a node is shared across files
// and carries symmetric EXPRESS relations:
//
//   - GetFwdPathsAsLinks never marks the start node visited, so a bidirectional
//     arrow (A→B and B→A) produces start→B→start→… loops that revisit the root
//     and burn the path budget before the genuine forward paths are emitted.
//   - Those back-edges also give the true root a non-zero in-degree, which broke
//     the explorer's "in-degree 0 = cone root" placement.
//
// Keeping outward-only edges yields an acyclic cone whose single source is the
// endpoint, emitted as one 2-node chain per edge (BuildResultSet dedups and
// reconstructs depth from the directed edges). forward=false walks inverse slots
// for a lone \to.
func (s *SearchService) searchCone(endpoints []SST.NodePtr, sttype []int, limit int, forward bool) []StructuralChain {
	chains := make([]StructuralChain, 0)
	if len(endpoints) == 0 {
		return chains
	}

	// STindex slots to follow. When the query constrained arrows
	// (\from X \leadsto) honor only those STtypes; otherwise follow every kind.
	scan := coneSTTypes
	if len(sttype) > 0 {
		scan = sttype
	}
	slots := make([]int, 0, len(scan))
	for _, st := range scan {
		dir := st
		if !forward {
			if st == SST.NEAR {
				continue // NEAR is symmetric — no inverse slot to walk
			}
			dir = -st
		}
		idx := SST.STTypeToSTIndex(dir)
		if idx >= 0 && idx < SST.ST_TOP {
			slots = append(slots, idx)
		}
	}

	// Multi-source BFS. depth records each node's shortest forward distance from
	// any endpoint; the root(s) sit at depth 0 and are marked visited up front so
	// a back-edge can never re-root the cone.
	depth := make(map[SST.NodePtr]int, len(endpoints))
	queue := make([]SST.NodePtr, 0, len(endpoints))
	for _, ep := range endpoints {
		if _, seen := depth[ep]; seen {
			continue
		}
		depth[ep] = 0
		queue = append(queue, ep)
	}

	for len(queue) > 0 && len(chains) < coneEdgeCap && len(depth) < coneNodeCap {
		cur := queue[0]
		queue = queue[1:]
		d := depth[cur]
		if d >= coneMaxDepth {
			continue
		}
		curNode := SST.GetMemoryNodeFromPtr(cur)
		if curNode.S == "" {
			continue
		}
		for _, idx := range slots {
			for _, lnk := range curNode.I[idx] {
				dd, seen := depth[lnk.Dst]
				if !seen {
					depth[lnk.Dst] = d + 1
					queue = append(queue, lnk.Dst)
				} else if dd < d {
					// Destination sits in a SHALLOWER layer — this edge points back
					// toward the root. Skipping it is what drops the start→B→start
					// EXPRESS bounce. Lateral edges (dd == d) and forward edges to an
					// already-discovered node (dd == d+1) are kept so the cone keeps
					// every genuine outward/sideways relationship.
					continue
				}
				dst := SST.GetMemoryNodeFromPtr(lnk.Dst)
				if dst.S == "" || dst.S == curNode.S {
					continue
				}
				arr := SST.GetDBArrowByPtr(s.pst, lnk.Arr)
				// Chapter is intentionally empty. The explorer keys nodes by
				// (text, chapter); a node merged across files (like `start`,
				// authored in both doors.n4l and chinese.n4l) has an ambiguous
				// chapter, so tagging each cone edge with its source's chapter
				// keys the SAME node differently per edge and SHATTERS the cone
				// into disconnected fragments (the bug where `start` vanished into
				// its own stray fragment). Keying cone nodes by text alone matches
				// how the graph store actually merges nodes, so each node is one
				// node everywhere and the cone stays connected.
				chains = append(chains, StructuralChain{
					Kind:    "path",
					Chapter: "",
					Nodes: []ChainNode{
						{NodeText: curNode.S, ArrowCode: arr.Short, ArrowLong: arr.Long},
						{NodeText: dst.S},
					},
				})
				if len(chains) >= coneEdgeCap {
					return chains
				}
			}
		}
	}
	return chains
}

// searchListing handles TOC, notes, arrow, and context listing queries.
func (s *SearchService) searchListing(params SST.SearchParameters, arrowptrs []SST.ArrowPtr, sttype []int, limit int) []ListingItem {
	items := make([]ListingItem, 0)

	chapter := params.Chapter
	context := params.Context
	hasArrows := len(params.Arrows) > 0

	if hasArrows {
		// Arrow lookup: \arrow [term]
		// Only emit arrows whose name/long actually matched the term. Enumerating
		// by sttype would pull in every arrow of a matched arrow's type, which is
		// noisy and confusing to the user.
		emitted := make(map[string]bool)
		for _, ap := range arrowptrs {
			adir := SST.GetDBArrowByPtr(s.pst, ap)
			if adir.Short == "" || emitted[adir.Short] {
				continue
			}
			emitted[adir.Short] = true
			item := ListingItem{
				Kind:     "arrow",
				Title:    adir.Short,
				Detail:   adir.Long,
				MetaType: SST.STTypeName(SST.STIndexToSTType(adir.STAindex)),
			}
			if s.registry != nil {
				if def, ok := s.registry.Arrows[adir.Short]; ok {
					if def.MetaType != "" {
						item.MetaType = def.MetaType
					}
					item.Context = def.Context
				}
			}
			items = append(items, item)
		}
		return items
	}

	// \context modes that emit a flat list of context names rather than a
	// chapter TOC — mirrors upstream `searchN4L context …` output.
	//
	// The N4L parser collapses every token after `\context` into a single
	// space-joined string, so we split the first entry by whitespace and
	// dispatch on its head:
	//
	//   \context any / \context %%       → all unique context tokens
	//   \context <term>     (no chapter) → tokens that co-occur with <term>
	//                                       (the "related contexts" view)
	if chapter == "" && len(context) > 0 {
		first := strings.TrimSpace(context[0])
		parts := strings.Fields(first)
		head := ""
		if len(parts) > 0 {
			head = strings.ToLower(parts[0])
		}

		if head == "any" || head == "%%" {
			return s.contextNamesAsListings(s.listAllContextTokens())
		}

		// Plain `\context <term>` with no chapter filter — return contexts
		// that co-occur with <term>, falling back to substring matches when
		// <term> isn't a known context label. Mirrors upstream behaviour
		// where `searchN4L context restaurant` yields a flat numbered list
		// of related context names.
		related := s.listRelatedContextTokens(first)
		if len(related) > 0 {
			return s.contextNamesAsListings(related)
		}
		matches := s.listContextTokensContaining(first)
		if len(matches) > 0 {
			return s.contextNamesAsListings(matches)
		}

		// Nothing matched. Did-you-mean hint when the input looks like an
		// inverted-syntax search (`\context A B C`) — in texere the working
		// form is `term \context tag`.
		if len(parts) >= 2 {
			ctx := parts[0]
			term := strings.Join(parts[1:], " ")
			return []ListingItem{{
				Kind:   "hint",
				Title:  fmt.Sprintf("Did you mean: %s \\context %s", term, ctx),
				Detail: "\\context filters by tag — put the search term before \\context.",
			}}
		}
		return []ListingItem{}
	}

	// Chapter / context TOC listing.
	// GetChapters uses prefix "chap:<name>:" — exact prefix, case-sensitive.
	// DecodeSearchField maps \chapter any → "%%" and bare \chapter → "TableOfContents";
	// both sentinel values resolve to "" (all chapters).
	// For a non-empty chapter filter we do accent-insensitive substring matching
	// via GetDBChaptersMatchingName, then fetch each matched chapter individually.
	chap := chapter
	if chap == "%%" || chap == "TableOfContents" || chap == "any" {
		chap = ""
	}

	var toc map[string][]string
	if chap != "" {
		normChap := normalizeForSearch(chap)
		matchedChaps := SST.GetDBChaptersMatchingName(s.pst, "")
		toc = make(map[string][]string)
		for _, name := range matchedChaps {
			if strings.Contains(normalizeForSearch(name), normChap) {
				partial := SST.GetChaptersByChapContext(s.pst, name, context, limit)
				for k, v := range partial {
					toc[k] = v
				}
			}
		}
	} else {
		toc = SST.GetChaptersByChapContext(s.pst, "", context, limit)
	}

	// Sort chapter names for stable output (mirrors searchN4L sort.Strings behaviour).
	chapNames := make([]string, 0, len(toc))
	for name := range toc {
		chapNames = append(chapNames, name)
	}
	sort.Strings(chapNames)

	// specificFilter is true when the user typed a chapter name (e.g. \chapter foo)
	// vs. a wildcard (\chapter, \chapter any, \chapter %%). Only the specific
	// case raises an empty-state hint pointing at \notes — the wildcard case is
	// expected to enumerate quietly.
	specificFilter := chap != ""
	for _, chapName := range chapNames {
		item := ListingItem{
			Kind:     "chapter",
			Title:    chapName,
			Contexts: s.chapterContexts(chapName),
		}
		if specificFilter && len(item.Contexts) == 0 {
			item.Detail = fmt.Sprintf(
				"no contexts defined for chapter %s — use \\notes %s to see the chapter's notes",
				chapName, chapName)
		}
		items = append(items, item)
	}

	return items
}

// contextChaptersDisplayCap bounds the per-tag chapter list shipped to the
// frontend. The omnibar caps the visible chip count further (5 + "+N more"),
// so this just keeps the JSON payload predictable for hyper-popular tags.
const contextChaptersDisplayCap = 50

// contextNamesAsListings wraps a sorted list of context names into ListingItem
// rows of Kind "context" — the flat-list form used by \context any, \context
// direct, and \context <term> (related contexts). Each row is annotated with
// the chapters that use the tag so the frontend can render inline chips.
func (s *SearchService) contextNamesAsListings(names []string) []ListingItem {
	items := make([]ListingItem, 0, len(names))
	for _, name := range names {
		items = append(items, ListingItem{
			Kind:     "context",
			Title:    name,
			Chapters: s.chaptersForTag(name, contextChaptersDisplayCap),
		})
	}
	return items
}

// buildTagChapters lazily inverts ListContextChapterPairs into the
// `tag → []chapter` map that drives inline chapter chips on \context
// listings. Each PageMap pair contributes its chapter to every token of its
// combo string. The result is cached on the SearchService and only rebuilt
// after Close → re-lazy-open. Caller must hold s.mu.
func (s *SearchService) buildTagChapters() {
	if s.tagChaptersBuilt {
		return
	}
	pairs := SST.ListContextChapterPairs(s.pst)
	tagSets := make(map[string]map[string]bool)
	for _, p := range pairs {
		for _, tag := range splitContextCombo(p.Combo) {
			set, ok := tagSets[tag]
			if !ok {
				set = make(map[string]bool)
				tagSets[tag] = set
			}
			set[p.Chapter] = true
		}
	}
	out := make(map[string][]string, len(tagSets))
	for tag, set := range tagSets {
		chs := make([]string, 0, len(set))
		for ch := range set {
			chs = append(chs, ch)
		}
		sort.Slice(chs, func(i, j int) bool {
			return strings.ToLower(chs[i]) < strings.ToLower(chs[j])
		})
		out[tag] = chs
	}
	s.tagChapters = out
	s.tagChaptersBuilt = true
}

// chaptersForTag returns the cached chapter list for `tag` (lazy-built on
// first call). Capped at maxChapters so a single hyper-popular tag can't
// blow up the JSON payload — the frontend caps display further at 5.
func (s *SearchService) chaptersForTag(tag string, maxChapters int) []string {
	s.buildTagChapters()
	chs := s.tagChapters[tag]
	if len(chs) > maxChapters {
		chs = chs[:maxChapters]
	}
	return chs
}

// splitContextCombo splits a stored context name (e.g. "restaurant, eating,
// cooking") into its constituent single tokens. Empty entries and the
// internal `_sequence_` marker are dropped. Used by every \context listing
// helper.
func splitContextCombo(combo string) []string {
	parts := strings.Split(combo, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "_sequence_" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// sortContextTokens returns the keys of `seen` alphabetically sorted
// (case-insensitive). Helper to keep the three listing functions terse.
func sortContextTokens(seen map[string]bool) []string {
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

// listAllContextTokens returns every distinct context tag stored in the
// graph backend, alphabetically sorted (case-insensitive). Used by
// \context any. Goes through SST.ListContexts so search reads the graph
// store, not the in-memory N4L re-parse cache (Constitution III).
func (s *SearchService) listAllContextTokens() []string {
	combos := SST.ListContexts(s.pst)
	seen := make(map[string]bool, len(combos))
	for _, combo := range combos {
		for _, tag := range splitContextCombo(combo) {
			seen[tag] = true
		}
	}
	return sortContextTokens(seen)
}

// listContextTokensContaining returns context tags whose label contains
// `term` (case-insensitive substring). Used as the substring-fallback for
// \context <term> when no co-occurrence match exists.
func (s *SearchService) listContextTokensContaining(term string) []string {
	termLower := strings.ToLower(strings.TrimSpace(term))
	if termLower == "" {
		return nil
	}
	combos := SST.ListContexts(s.pst)
	seen := make(map[string]bool)
	for _, combo := range combos {
		for _, tag := range splitContextCombo(combo) {
			if strings.Contains(strings.ToLower(tag), termLower) {
				seen[tag] = true
			}
		}
	}
	return sortContextTokens(seen)
}

// listRelatedContextTokens returns the context tags that co-occur with
// `term` in any stored context combo — the upstream "related contexts"
// view used by \context <term>. A combo "co-occurs" with the term when one
// of its tokens matches `term` (case-insensitive equality or substring of a
// single token); every token from that combo is then collected.
func (s *SearchService) listRelatedContextTokens(term string) []string {
	termLower := strings.ToLower(strings.TrimSpace(term))
	if termLower == "" {
		return nil
	}
	combos := SST.ListContexts(s.pst)
	seen := make(map[string]bool)
	for _, combo := range combos {
		tokens := splitContextCombo(combo)
		matched := false
		for _, tag := range tokens {
			tagLower := strings.ToLower(tag)
			if tagLower == termLower || strings.Contains(tagLower, termLower) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for _, tag := range tokens {
			seen[tag] = true
		}
	}
	return sortContextTokens(seen)
}

// searchNotes returns the authored per-line layout of notes matching the
// \notes query, reading PageMap rows from BadgerDB.  Chapter and context
// filtering are applied at the store level; grouping into FullNote/NoteBlock
// is done in buildFullNotes so the store stays a dumb row source.
// noteAutoPageCap is a hard safety bound on the number of pages
// searchNotes will fetch when no explicit \page is given. The
// upstream GetDBPageMap returns up to pageMapHitsPerPage (60) rows
// per call and iterates the whole pm: keyspace each time, so the
// aggregate cost is O(pages² × pageSize). 100 pages = up to 6000
// page-map rows = enough for any realistic chapter.
const noteAutoPageCap = 100

func (s *SearchService) searchNotes(params SST.SearchParameters, rawQuery string) []FullNote {
	chap := params.Chapter
	if chap == "%%" || chap == "TableOfContents" {
		chap = ""
	}

	// `\notes` ALWAYS sets params.PageNr = 1 upstream (it's a sentinel
	// marking the notes route, not an explicit page request). The only
	// reliable signal that the user typed `\page N` is the literal
	// token in the raw query. When present we honor params.PageNr;
	// otherwise we aggregate every page.
	if hasExplicitPageDirective(rawQuery) {
		rows := SST.GetDBPageMap(s.pst, chap, params.Context, params.PageNr)
		return s.buildFullNotes(rows)
	}

	// No \page: aggregate every page so the explorer sees the
	// whole chapter. The upstream pageMapHitsPerPage cap (60)
	// otherwise silently drops everything past line ~60.
	var rows []SST.PageMap
	for page := 1; page <= noteAutoPageCap; page++ {
		batch := SST.GetDBPageMap(s.pst, chap, params.Context, page)
		if len(batch) == 0 {
			break
		}
		rows = append(rows, batch...)
	}
	return s.buildFullNotes(rows)
}

// hasExplicitPageDirective returns true when the raw query string
// contains `\page` as a standalone N4L directive (whitespace- or
// start-anchored on the left). Used to distinguish a real user-typed
// `\page N` from the implicit PageNr=1 that `\notes` sets as a route
// marker.
func hasExplicitPageDirective(q string) bool {
	lower := strings.ToLower(q)
	idx := strings.Index(lower, "\\page")
	if idx < 0 {
		return false
	}
	// Must be at start or preceded by whitespace.
	if idx > 0 {
		prev := lower[idx-1]
		if prev != ' ' && prev != '\t' && prev != '\n' {
			return false
		}
	}
	// Must be followed by whitespace, end, or the digit immediately
	// (e.g. `\page2` is the same intent as `\page 2`).
	rest := lower[idx+len("\\page"):]
	if len(rest) == 0 {
		return true
	}
	c := rest[0]
	if c == ' ' || c == '\t' || c == '\n' || (c >= '0' && c <= '9') {
		return true
	}
	return false
}

// buildFullNotes collapses per-line PageMap rows into one FullNote per chapter
// and one NoteBlock per root node within that chapter, preserving file order.
// All arrows across the root's own line and its ditto lines land in a single
// block so the frontend can render them as a visual group.
func (s *SearchService) buildFullNotes(rows []SST.PageMap) []FullNote {
	if len(rows) == 0 {
		return []FullNote{}
	}

	type chapterState struct {
		note   *FullNote
		blocks map[SST.NodePtr]int // root NodePtr → index into note.Blocks
	}

	chapters := make(map[string]*chapterState)
	order := make([]string, 0)

	for _, ev := range rows {
		if len(ev.Path) == 0 {
			continue
		}
		rootPtr := ev.Path[0].Dst

		state, ok := chapters[ev.Chapter]
		if !ok {
			state = &chapterState{
				note:   &FullNote{Chapter: ev.Chapter, Blocks: []NoteBlock{}},
				blocks: make(map[SST.NodePtr]int),
			}
			chapters[ev.Chapter] = state
			order = append(order, ev.Chapter)
		}

		idx, seen := state.blocks[rootPtr]
		if !seen {
			rootNode := SST.GetDBNodeByNodePtr(s.pst, rootPtr)
			ctxName, _ := s.pst.KV.GetContextByPtr(ev.Context)
			if ctxName == "any" {
				ctxName = ""
			}
			state.note.Blocks = append(state.note.Blocks, NoteBlock{
				NodeText: rootNode.S,
				Line:     ev.Line,
				Context:  ctxName,
				Arrows:   []NoteArrow{},
			})
			idx = len(state.note.Blocks) - 1
			state.blocks[rootPtr] = idx
		}

		block := &state.note.Blocks[idx]
		for _, link := range ev.Path[1:] {
			arr := SST.GetDBArrowByPtr(s.pst, link.Arr)
			target := SST.GetDBNodeByNodePtr(s.pst, link.Dst)
			block.Arrows = append(block.Arrows, NoteArrow{
				ArrowLong: arr.Long,
				ArrowCode: arr.Short,
				Target:    target.S,
			})
		}
	}

	out := make([]FullNote, 0, len(order))
	for _, chap := range order {
		out = append(out, *chapters[chap].note)
	}
	return out
}

// augmentUnknownArrows scans the collection's N4L files for arrow codes not
// declared in any SSTconfig and injects synthetic ArrowDefs into s.registry,
// matching graph.BuildFromCollection's pre-pass. Called at lazy-open before
// RegisterArrows so the search-side ARROW_DIRECTORY is identical to the build's
// and unknown-arrow links resolve. Caller must hold s.mu.
func (s *SearchService) augmentUnknownArrows() {
	summaries, err := content.ScanCollection(s.collectionDir)
	if err != nil {
		return
	}
	files := make([]*content.ParsedFile, 0, len(summaries))
	cache := make(map[string]*content.ParsedFile, len(summaries))
	for _, summary := range summaries {
		if summary.ScanError != nil {
			continue
		}
		if pf, _ := content.ParseFile(summary.FilePath, s.registry); pf != nil {
			files = append(files, pf)
			cache[summary.FilePath] = pf
		}
	}
	content.AugmentWithUnknownArrows(s.registry, files)
	// Hand the parsed files to rebuildMemoryLinks (next in the lazy-open
	// sequence) so it doesn't re-parse the entire collection.
	s.parsedCache = cache
}

// rebuildMemoryLinks re-populates NODE_DIRECTORY link arrays from N4L source
// files after an app restart (when the in-memory state was lost). textToDBPtr
// maps lower-cased node text → its BadgerDB NodePtr, built during IterateNodes.
// This is a no-op for links whose nodes are already linked (AppendLinkToNode is
// not idempotent, but duplicates are harmless for orbit traversal since seen{}
// deduplicates in buildOrbit).
func (s *SearchService) rebuildMemoryLinks(textToDBPtr map[string]SST.NodePtr) {
	if s.collectionDir == "" {
		return
	}
	summaries, err := content.ScanCollection(s.collectionDir)
	if err != nil {
		return
	}
	nodeContexts := make(map[SST.NodePtr][]string)
	thenArrowPtr, _ := SST.GetDBArrowsWithArrowName(s.pst, SequenceArrowCode)
	// Reuse files parsed moments ago by augmentUnknownArrows when available
	// (the common lazy-open path); only parse on a cache miss. Clear the cache
	// afterward so it can't go stale across rebuilds.
	cache := s.parsedCache
	s.parsedCache = nil
	for _, summary := range summaries {
		if summary.ScanError != nil {
			continue
		}
		pf := cache[summary.FilePath]
		if pf == nil {
			pf, _ = content.ParseFile(summary.FilePath, s.registry)
		}
		if pf == nil {
			continue
		}
		for _, ch := range pf.Chapters {
			var prevSeqDBPtr SST.NodePtr
			var havePrevSeq bool
			var prevSeqCtxPtr SST.ContextPtr
			var prevSeqCtx []string
			for _, node := range ch.Nodes {
				fromKey := strings.ToLower(strings.TrimSpace(node.Text))
				fromDBPtr, okF := textToDBPtr[fromKey]
				if !okF {
					continue
				}
				if len(node.EffectiveCtx) > 0 {
					nodeContexts[fromDBPtr] = node.EffectiveCtx
				}
				ctx := SST.TryContext(s.pst, node.EffectiveCtx)
				processArrows := func(arrows []content.Arrow) {
					for _, arrow := range arrows {
						if arrow.Target == "" || arrow.Code == "" {
							continue
						}
						if _, ok := s.registry.Arrows[arrow.Code]; !ok {
							continue
						}
						toKey := strings.ToLower(strings.TrimSpace(arrow.Target))
						toDBPtr, okT := textToDBPtr[toKey]
						if !okT {
							continue
						}
						if fromDBPtr == toDBPtr {
							continue
						}
						arrowPtr, _ := SST.GetDBArrowsWithArrowName(s.pst, arrow.Code)
						appendMemLink(arrowPtr, fromDBPtr, toDBPtr, ctx)
					}
				}
				processArrows(node.Arrows)
				for _, ditto := range node.Dittos {
					processArrows(ditto.Arrows)
				}

				// Re-materialize implicit `(then)` links between consecutive
				// `:: _sequence_ ::` nodes (mirrors graph.BuildFromCollection).
				// Only chain across nodes that share the same sequence-context
				// label so back-to-back `:: _sequence_, A ::` / `:: _sequence_, B ::`
				// blocks stay separate.
				if node.IsSequence {
					sameCtx := havePrevSeq && sameSequenceContextTags(prevSeqCtx, node.EffectiveCtx)
					if sameCtx && prevSeqDBPtr != fromDBPtr {
						appendMemLink(thenArrowPtr, prevSeqDBPtr, fromDBPtr, prevSeqCtxPtr)
					}
					prevSeqDBPtr = fromDBPtr
					havePrevSeq = true
					prevSeqCtxPtr = ctx
					prevSeqCtx = node.EffectiveCtx
				} else {
					havePrevSeq = false
				}
			}
		}
	}
	s.nodeContexts = nodeContexts
}

// appendMemLink adds a forward (and inverse) in-memory link to NODE_DIRECTORY.
// fromDB and toDB are BadgerDB NodePtrs; NODE_CACHE maps them to in-memory ptrs.
func appendMemLink(arrowPtr SST.ArrowPtr, fromDB, toDB SST.NodePtr, ctx SST.ContextPtr) {
	fromMem, okF := SST.NODE_CACHE[fromDB]
	toMem, okT := SST.NODE_CACHE[toDB]
	if !okF || !okT {
		return
	}
	fwdLink := SST.Link{Arr: arrowPtr, Wgt: 1.0, Dst: toMem, Ctx: ctx}
	SST.AppendLinkToNode(fromMem, fwdLink, toMem)
	invArr := SST.INVERSE_ARROWS[arrowPtr]
	if invArr > 0 {
		invLink := SST.Link{Arr: invArr, Wgt: 1.0, Dst: fromMem, Ctx: ctx}
		SST.AppendLinkToNode(toMem, invLink, fromMem)
	}
}
