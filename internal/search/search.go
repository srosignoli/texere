package search

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	SST "github.com/srosignoli/sstorytime-kv/pkg/SSTorytime"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"texere/internal/content"
)

type queryRoute int

const (
	routeNodeSearch queryRoute = iota
	routeStructural
	routeListing
	routeNotes
)

// SearchService wraps the SSTorytime-KV PoSST and manages the BadgerDB lifecycle.
// The store is opened lazily on first Search() call and kept open until Close().
type SearchService struct {
	dbPath        string
	collectionDir string
	registry      *content.SSTRegistry
	store         *SST.BadgerKV
	pst           SST.PoSST
	mu            sync.Mutex
	// nodeContexts maps BadgerDB NodePtr → context tags sourced from the N4L files.
	// Populated during lazy open; used by \notes \context filtering.
	nodeContexts map[SST.NodePtr][]string
	// textToDBPtr maps lower-cased node text → BadgerDB NodePtr. Built once at
	// lazy open so \from/\to endpoint resolution can do O(1) exact-text lookup
	// without issuing a KV read per NODE_CACHE entry.
	textToDBPtr map[string]SST.NodePtr
}

// NewSearchService creates a SearchService pointed at <collectionDir>/sst_data.
func NewSearchService(collectionDir string, registry *content.SSTRegistry) *SearchService {
	return &SearchService{
		dbPath:        filepath.Join(collectionDir, "sst_data"),
		collectionDir: collectionDir,
		registry:      registry,
	}
}

// IsReady returns true if the sst_data directory exists (graph has been built).
func (s *SearchService) IsReady() bool {
	_, err := os.Stat(s.dbPath)
	return err == nil
}

// Close shuts down the BadgerDB connection if open.
func (s *SearchService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		SST.Close(s.pst)
		s.store = nil
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
		RegisterArrows(s.registry)
		kv, openErr := SST.OpenBadgerStore(s.dbPath)
		if openErr != nil {
			result.GraphError = openErr.Error()
			return result, nil
		}
		s.store = kv
		s.pst = SST.PoSST{KV: kv}
		SST.Configure(s.pst, false)
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
	}

	// Recover from SSTorytime-KV panics so the overlay never crashes the app.
	defer func() {
		if r := recover(); r != nil {
			result = NewSearchResult()
			result.GraphError = fmt.Sprintf("search error: %v", r)
		}
	}()

	params := SST.DecodeSearchField(query)
	_, maxlimit := SST.MinMaxPolicy(params)
	if maxlimit <= 0 {
		maxlimit = 10
	}

	arrowptrs, sttype := SST.ArrowPtrFromArrowsNames(s.pst, params.Arrows)

	switch s.classifyQuery(params) {
	case routeNodeSearch:
		result.Nodes = s.searchNodes(params, arrowptrs, maxlimit)
	case routeStructural:
		result.Structural = s.searchStructural(params, arrowptrs, sttype, maxlimit)
	case routeListing:
		result.Listings = s.searchListing(params, arrowptrs, sttype, maxlimit)
	case routeNotes:
		result.Notes = s.searchNotes(params)
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

	// Structural: path queries (\from + \to) or sequence queries (\sequence)
	if (from && to) || seq {
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
// Uses IterateNodes with accent-insensitive matching so that queries like
// "(fangzi) \chapter chinese" correctly match accented node texts like "fángzi".
func (s *SearchService) searchNodes(params SST.SearchParameters, arrowptrs []SST.ArrowPtr, limit int) []GraphNode {
	// Build per-term match specs. Terms wrapped in !…! or |…| use exact (equality)
	// matching; all other terms use accent-insensitive substring matching.
	type matchTerm struct {
		text  string
		exact bool
	}
	matchTerms := make([]matchTerm, 0, len(params.Name))
	for _, raw := range params.Name {
		if isEx, cleaned := SST.IsExactMatch(raw); isEx {
			if n := normalizeForSearch(cleaned); n != "" {
				matchTerms = append(matchTerms, matchTerm{n, true})
			}
		} else {
			if n := normalizeForSearch(raw); n != "" {
				matchTerms = append(matchTerms, matchTerm{n, false})
			}
		}
	}

	chapLower := normalizeForSearch(params.Chapter)
	isChapFilter := chapLower != "" && chapLower != "%%" && chapLower != "tableofcontents"

	nodes := make([]GraphNode, 0, limit)

	s.pst.KV.IterateNodes(func(n SST.Node, ptr SST.NodePtr) bool {
		if n.S == "" {
			return true
		}
		// Chapter filter: case-insensitive substring match against each chapter
		// token (n.Chap may be comma-separated when a node belongs to multiple chapters).
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
		// Name filter: OR across all terms — node passes if ANY term matches.
		// Exact terms use equality; plain terms use accent-insensitive substring.
		if len(matchTerms) > 0 {
			nodeNorm := normalizeForSearch(n.S)
			matched := false
			for _, mt := range matchTerms {
				if mt.exact {
					if nodeNorm == mt.text {
						matched = true
						break
					}
				} else {
					if strings.Contains(nodeNorm, mt.text) {
						matched = true
						break
					}
				}
			}
			if !matched {
				return true
			}
		}
		// Context filter: node passes only if it has at least one in-memory link
		// whose stored context matches the query filter (OR semantics).
		// Links with Ctx==0 carry no context and are skipped.
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
			Orbit:       s.buildOrbit(ptr, 8),
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

// buildOrbit returns the orbit for a node using direct in-memory traversal.
// GetNodeOrbit cannot be used here because it calls GetDBNodeByNodePtr with
// in-memory ptrs (per-class indices) that don't match BadgerDB sequence IDs.
// Instead we walk NODE_DIRECTORY directly via in-memory ptrs, using
// GetMemoryNodeFromPtr for text lookup at every hop.
// nptr is a BadgerDB NodePtr; NODE_CACHE bridges it to the in-memory index.
func (s *SearchService) buildOrbit(nptr SST.NodePtr, cap int) []OrbitEntry {
	memPtr, ok := SST.NODE_CACHE[nptr]
	if !ok {
		return nil
	}

	stOrder := []int{
		SST.STTypeToSTIndex(SST.EXPRESS),
		SST.STTypeToSTIndex(-SST.EXPRESS),
		SST.STTypeToSTIndex(-SST.CONTAINS),
		SST.STTypeToSTIndex(SST.LEADSTO),
		SST.STTypeToSTIndex(-SST.LEADSTO),
		SST.STTypeToSTIndex(SST.NEAR),
	}

	entries := make([]OrbitEntry, 0, cap)
	seen := make(map[string]bool)

	rootNode := SST.GetMemoryNodeFromPtr(memPtr)

	for _, stIdx := range stOrder {
		if len(entries) >= cap {
			break
		}
		for _, lnk1 := range rootNode.I[stIdx] {
			if len(entries) >= cap {
				break
			}
			hop1 := SST.GetMemoryNodeFromPtr(lnk1.Dst)
			if hop1.S == "" || hop1.S == rootNode.S {
				continue
			}
			arr1 := SST.GetDBArrowByPtr(s.pst, lnk1.Arr)
			key1 := hop1.S + "|" + arr1.Long
			if seen[key1] {
				continue
			}
			seen[key1] = true
			entries = append(entries, OrbitEntry{
				ArrowLong:    arr1.Long,
				NeighborText: hop1.S,
				Radius:       1,
				Ctx:          SST.GetContext(lnk1.Ctx),
			})
			// Radius-2: follow same-STtype links from hop1
			for _, lnk2 := range hop1.I[stIdx] {
				if len(entries) >= cap {
					break
				}
				hop2 := SST.GetMemoryNodeFromPtr(lnk2.Dst)
				if hop2.S == "" || hop2.S == rootNode.S || hop2.S == hop1.S {
					continue
				}
				arr2 := SST.GetDBArrowByPtr(s.pst, lnk2.Arr)
				key2 := hop2.S + "|" + arr2.Long
				if seen[key2] {
					continue
				}
				seen[key2] = true
				entries = append(entries, OrbitEntry{
					ArrowLong:    arr2.Long,
					NeighborText: hop2.S,
					Radius:       2,
					Ctx:          SST.GetContext(lnk2.Ctx),
				})
			}
		}
	}
	return entries
}

// resolvePathEndpoints maps \from/\to names to in-memory NodePtrs.
//
// Why: SolveNodePtrs → SearchNodesByName iterates the BadgerDB `text:` index
// alphabetically and applies a substring filter only up to `limit` hits. On a
// real collection the plain "start" node is drowned out by thousands of
// unrelated lines that contain "start" and sort before `text:start` (e.g.
// Moby-Dick paragraphs), so the true node is never returned. We therefore
// walk NODE_CACHE directly for an exact (case-folded) text match before
// falling back to the substring scan — NODE_CACHE has one entry per node,
// so this is O(N) over the whole graph (~14k nodes in practice).
func (s *SearchService) resolvePathEndpoints(names []string, params SST.SearchParameters, arrowptrs []SST.ArrowPtr, limit int) []SST.NodePtr {
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
		clean := raw
		if isEx, stripped := SST.IsExactMatch(raw); isEx {
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
				continue
			}
		}
		for _, dbPtr := range SST.SolveNodePtrs(s.pst, []string{raw}, params, arrowptrs, limit) {
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
			// Join name tokens to form the full node text for an exact substring
			// match. DecodeSearchField splits multi-word names into individual
			// tokens, but SolveNodePtrs ORs them, returning too many candidates.
			joined := strings.Join(params.Name, " ")
			dbPtrs := s.pst.KV.SearchNodesByName(joined, limit)
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

	// Path query: \from + \to
	// Resolve endpoints to memory ptrs. SolveNodePtrs does substring matching
	// with an alphabetical limit, so the target node itself can be missed when
	// many unrelated nodes sort before it (e.g. the `start` node is drowned out
	// by Moby-Dick paragraphs containing "start"). Prefer exact-text matches
	// over substring results.
	leftptrs := s.resolvePathEndpoints(params.From, params, arrowptrs, limit)
	rightMem := s.resolvePathEndpoints(params.To, params, arrowptrs, limit)
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

	for _, chapName := range chapNames {
		items = append(items, ListingItem{
			Kind:  "chapter",
			Title: chapName,
		})
	}

	// If context filter produced no chapter results, list contexts directly
	if len(items) == 0 && len(context) > 0 {
		for _, ctx := range context {
			items = append(items, ListingItem{
				Kind:  "context",
				Title: ctx,
			})
		}
	}

	return items
}

// searchNotes returns the authored per-line layout of notes matching the
// \notes query, reading PageMap rows from BadgerDB.  Chapter and context
// filtering are applied at the store level; grouping into FullNote/NoteBlock
// is done in buildFullNotes so the store stays a dumb row source.
func (s *SearchService) searchNotes(params SST.SearchParameters) []FullNote {
	chap := params.Chapter
	if chap == "%%" || chap == "TableOfContents" {
		chap = ""
	}
	page := params.PageNr
	if page <= 0 {
		page = 1
	}
	rows := SST.GetDBPageMap(s.pst, chap, params.Context, page)
	return s.buildFullNotes(rows)
}

// buildFullNotes collapses per-line PageMap rows into one FullNote per chapter
// and one NoteBlock per root node within that chapter, preserving file order.
// All arrows across the root's own line and its ditto lines land in a single
// block so the frontend can render them as a visual group.
func (s *SearchService) buildFullNotes(rows []SST.PageMap) []FullNote {
	if len(rows) == 0 {
		return []FullNote{}
	}

	type blockKey struct {
		chap string
		root SST.NodePtr
	}
	type chapterState struct {
		note   *FullNote
		blocks map[SST.NodePtr]int // root NodePtr → index into note.Blocks
	}

	chapters := make(map[string]*chapterState)
	order := make([]string, 0)
	_ = blockKey{}

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
	for _, summary := range summaries {
		if summary.ScanError != nil {
			continue
		}
		pf, _ := content.ParseFile(summary.FilePath, s.registry)
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
