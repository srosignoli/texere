package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"texere/internal/content"
	"texere/internal/search"

	SST "github.com/srosignoli/sstorytime-kv/pkg/SSTorytime"
)

// BuildResult summarises a completed graph build.
type BuildResult struct {
	NodesIngested  int
	LinksIngested  int
	FilesProcessed int
	Errors         []string
	// UnknownArrows aggregates arrow codes encountered in the source files
	// that aren't registered in the SSTRegistry. Key = arrow code text,
	// value = number of occurrences across the collection. The corresponding
	// links and target nodes are NOT created (existing builder behaviour);
	// surfaced in the log panel so authors know what to register.
	UnknownArrows map[string]int
	// ParseErrors hoists every per-file content.ParseError encountered
	// during the build, so the frontend can list them in one place rather
	// than waiting for the user to navigate to each file.
	ParseErrors []content.ParseError
	DurationMs  int64
}

// BuildFromCollection ingests all .n4l files from collectionDir into a fresh
// BadgerDB store at <collectionDir>/sst_data. The caller must close any open
// SearchService before calling this (BadgerDB does not allow two writers).
func BuildFromCollection(collectionDir string, registry *content.SSTRegistry) (*BuildResult, error) {
	start := time.Now()
	result := &BuildResult{
		UnknownArrows: make(map[string]int),
	}

	summaries, err := content.ScanCollection(collectionDir)
	if err != nil {
		return nil, fmt.Errorf("graph: scan collection: %w", err)
	}

	dbPath := filepath.Join(collectionDir, "sst_data")
	if err := os.RemoveAll(dbPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("graph: clear sst_data: %w", err)
	}
	blevePath := filepath.Join(collectionDir, "bleve")
	if err := os.RemoveAll(blevePath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("graph: clear bleve: %w", err)
	}

	// Parse every file ONCE up front and reuse the result across both build
	// passes (parsing is the build's dominant cost — re-parsing per pass on a
	// large collection is what made Ctrl+G feel like it hung). Pass 1/2 look
	// up arrow codes in the live registry map, not the parsed IsRegistered
	// flag, so files parsed before the registry is augmented stay valid.
	//
	// The pre-parse also discovers arrow codes not declared in any SSTconfig
	// and synthesizes ArrowDefs for them (so their links materialize, rendered
	// "unknown", instead of being dropped). AugmentWithUnknownArrows MUST run
	// before RegisterArrows so the synthesized arrows land in ARROW_DIRECTORY;
	// the search service runs the identical augmentation at lazy-open so
	// ArrowPtr indices line up across build and query.
	parsedByPath := make(map[string]*content.ParsedFile, len(summaries))
	preParsed := make([]*content.ParsedFile, 0, len(summaries))
	for _, summary := range summaries {
		if summary.ScanError != nil {
			continue
		}
		if pf, _ := content.ParseFile(summary.FilePath, registry); pf != nil {
			parsedByPath[summary.FilePath] = pf
			preParsed = append(preParsed, pf)
		}
	}
	content.AugmentWithUnknownArrows(registry, preParsed)

	// Reset in-memory node directory and cache so indices stay in sync with
	// the freshly created BadgerDB (which resets its sequence to 0).
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)

	SST.MemoryInit()
	search.RegisterArrows(registry)

	kv, err := SST.OpenBadgerStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("graph: open store: %w", err)
	}
	sst := SST.PoSST{KV: kv}
	defer SST.Close(sst)
	SST.Configure(sst, false)

	bleveIdx, err := SST.NewBleveIndex(blevePath)
	if err != nil {
		return nil, fmt.Errorf("graph: open bleve index: %w", err)
	}
	defer bleveIdx.Close()
	// One batch for the WHOLE build (both passes). Bleve's scorch backend turns
	// every individual Index() call into its own segment plus a persist/merge
	// cycle, so per-node writes in Pass 2 — one per unique arrow target — were
	// the dominant cost after full-text indexing was added. Staging every node
	// into a single batch and committing once collapses that to one segment
	// build. Safe because BuildGraph closes the search service first, so no
	// reader observes the index mid-build.
	bleveBatch := bleveIdx.NewBatch()

	// nodeIndex maps normalised text → SST.Node with NPtr populated.
	nodeIndex := make(map[string]SST.Node)

	// Pass 1 — add all nodes.
	// CacheNode must be called here so that the per-class in-memory directory
	// indices (stored in NODE_CACHE) stay aligned with BadgerDB sequence IDs.
	for _, summary := range summaries {
		if summary.ScanError != nil {
			result.Errors = append(result.Errors, summary.ScanError.Error())
			continue
		}
		pf := parsedByPath[summary.FilePath]
		if pf == nil {
			continue
		}
		result.FilesProcessed++
		// Hoist per-file parse errors into the build summary so the log
		// panel can render every problem from one Ctrl+G batch.
		if len(pf.Errors) > 0 {
			result.ParseErrors = append(result.ParseErrors, pf.Errors...)
		}
		for _, ch := range pf.Chapters {
			for _, node := range ch.Nodes {
				key := normalizeText(node.Text)
				if _, exists := nodeIndex[key]; exists {
					continue
				}
				n := SST.Node{S: node.Text, Chap: ch.Title}
				n = SST.IdempDBAddNode(sst, n)
				SST.CacheNode(n) // register in NODE_DIRECTORY and NODE_CACHE
				nodeIndex[key] = n
				bleveBatch.Add(n.NPtr, n.S, ch.Title, summary.FilePath)
				result.NodesIngested++
			}
		}
	}

	// Pass 2 — add links and populate in-memory link arrays for orbit queries.
	// Arrow-target nodes discovered here stage into the same bleveBatch (see
	// ensureNode); the single commit happens after this loop.
	for _, summary := range summaries {
		if summary.ScanError != nil {
			continue
		}
		pf := parsedByPath[summary.FilePath]
		if pf == nil {
			continue
		}
		sourceFile := summary.FilePath
		for _, ch := range pf.Chapters {
			var prevSeq *SST.Node
			var prevCtx []string
			for i := range ch.Nodes {
				node := ch.Nodes[i]
				fromNode, ok := nodeIndex[normalizeText(node.Text)]
				if !ok {
					continue
				}

				// linePaths accumulates one Path per source line covered by this
				// root node: its own line, plus one entry per ditto line.  Each
				// path starts with {Dst: root} — dittos inherit the root as the
				// effective "from" — and has arrow legs appended as we process
				// the line's arrows below.
				linePaths := map[int][]SST.Link{
					node.Line: {{Dst: fromNode.NPtr}},
				}
				for _, ditto := range node.Dittos {
					if _, ok := linePaths[ditto.Line]; !ok {
						linePaths[ditto.Line] = []SST.Link{{Dst: fromNode.NPtr}}
					}
				}
				ctxPtrForNode := SST.TryContext(sst, node.EffectiveCtx)

				appendArrowLeg := func(arrow content.Arrow, toNode SST.Node, arrowPtr SST.ArrowPtr) {
					line := arrow.Line
					if _, ok := linePaths[line]; !ok {
						linePaths[line] = []SST.Link{{Dst: fromNode.NPtr}}
					}
					linePaths[line] = append(linePaths[line], SST.Link{
						Arr: arrowPtr,
						Ctx: ctxPtrForNode,
						Dst: toNode.NPtr,
						Wgt: 1.0,
					})
				}

				for _, arrow := range node.Arrows {
					if arrow.Target == "" || arrow.Code == "" {
						continue
					}
					def, ok := registry.Arrows[arrow.Code]
					if !ok {
						// Defensive: AugmentWithUnknownArrows should have added
						// every code already. If one slipped through, drop it
						// (old behaviour) and report it.
						result.UnknownArrows[arrow.Code]++
						continue
					}
					// Unknown (synthesized) arrows still materialize as links —
					// rendered with the "unknown" style — but are also counted
					// so the log panel keeps nudging authors to register them.
					if def.Unknown {
						result.UnknownArrows[arrow.Code]++
					}
					toNode := ensureNode(sst, bleveBatch, arrow.Target, ch.Title, sourceFile, nodeIndex, result)
					srcNode := arrowSource(sst, bleveBatch, arrow, fromNode, ch.Title, sourceFile, nodeIndex, result)
					if srcNode.NPtr == toNode.NPtr {
						continue
					}
					arrowPtr, _ := SST.Edge(sst, srcNode, arrow.Code, toNode, node.EffectiveCtx, 1.0)
					appendMemoryLink(arrowPtr, srcNode.NPtr, toNode.NPtr, ctxPtrForNode)
					appendArrowLeg(arrow, toNode, arrowPtr)
					result.LinksIngested++
				}
				for _, ditto := range node.Dittos {
					for _, arrow := range ditto.Arrows {
						if arrow.Target == "" || arrow.Code == "" {
							continue
						}
						def, ok := registry.Arrows[arrow.Code]
						if !ok {
							result.UnknownArrows[arrow.Code]++
							continue
						}
						if def.Unknown {
							result.UnknownArrows[arrow.Code]++
						}
						toNode := ensureNode(sst, bleveBatch, arrow.Target, ch.Title, sourceFile, nodeIndex, result)
						srcNode := arrowSource(sst, bleveBatch, arrow, fromNode, ch.Title, sourceFile, nodeIndex, result)
						if srcNode.NPtr == toNode.NPtr {
							continue
						}
						arrowPtr, _ := SST.Edge(sst, srcNode, arrow.Code, toNode, node.EffectiveCtx, 1.0)
						appendMemoryLink(arrowPtr, srcNode.NPtr, toNode.NPtr, ctxPtrForNode)
						appendArrowLeg(arrow, toNode, arrowPtr)
						result.LinksIngested++
					}
				}

				// Persist one PageMap event per source line so \notes can
				// reconstruct the authored layout verbatim.
				for line, path := range linePaths {
					_ = sst.KV.SavePageMap(SST.PageMap{
						Chapter: ch.Title,
						Line:    line,
						Context: ctxPtrForNode,
						Path:    path,
					})
				}

				// Wire consecutive `:: _sequence_ ::` nodes with an implicit
				// `(then)` arrow so `\story` / `\sequence` queries can walk
				// sequences that don't declare explicit (next)-style arrows.
				// Only link across nodes that share the same sequence-context
				// label — otherwise two back-to-back `:: _sequence_, X ::` and
				// `:: _sequence_, Y ::` blocks would fuse into one chain.
				if node.IsSequence {
					sameCtx := prevSeq != nil && sameSequenceContext(prevCtx, node.EffectiveCtx)
					if sameCtx && prevSeq.NPtr != fromNode.NPtr {
						ctxPtr := SST.TryContext(sst, prevCtx)
						arrowPtr, _ := SST.Edge(sst, *prevSeq, search.SequenceArrowCode, fromNode, prevCtx, 1.0)
						appendMemoryLink(arrowPtr, prevSeq.NPtr, fromNode.NPtr, ctxPtr)
						result.LinksIngested++
					}
					prev := fromNode
					prevSeq = &prev
					prevCtx = node.EffectiveCtx
				} else {
					prevSeq = nil
					prevCtx = nil
				}
			}
		}
	}

	// Commit every staged node (Pass 1 chapter roots + Pass 2 arrow targets) in
	// a single batch.
	if err := bleveIdx.ApplyBatch(bleveBatch); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("bleve apply batch: %v", err))
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// appendMemoryLink populates the in-memory NODE_DIRECTORY link arrays for both
// the forward and inverse directions so that GetNodeOrbit works within the
// same process session (enumeratePaths reads node.I[st], not BadgerDB).
// Both node ptrs must already be registered in NODE_CACHE via CacheNode.
func appendMemoryLink(arrowPtr SST.ArrowPtr, fromDB, toDB SST.NodePtr, ctx SST.ContextPtr) {
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

// ensureNode returns the SST.Node for text, creating it in BadgerDB if absent.
// chap is assigned when the node is first created; if the node already exists
// with an empty Chap it is updated so arrow-target nodes inherit chapter info.
//
// New nodes created in Pass 2 are staged into the shared Bleve batch (committed
// once after Pass 2 completes) rather than indexed individually — per-node
// commits were the dominant build cost. These are typically arrow targets that
// weren't chapter roots.
// arrowSource is the node an arrow starts from: the line's node for the first
// arrow, the previous arrow's target for the next ones ("A (r1) B (r2) C" is
// A→B and B→C, as upstream N4L reads it).
func arrowSource(sst SST.PoSST, bleveBatch *SST.IndexBatch, arrow content.Arrow, lineNode SST.Node, chap, sourceFile string, index map[string]SST.Node, result *BuildResult) SST.Node {
	if arrow.Source == "" || normalizeText(arrow.Source) == normalizeText(lineNode.S) {
		return lineNode
	}
	return ensureNode(sst, bleveBatch, arrow.Source, chap, sourceFile, index, result)
}

func ensureNode(sst SST.PoSST, bleveBatch *SST.IndexBatch, text, chap, sourceFile string, index map[string]SST.Node, result *BuildResult) SST.Node {
	key := normalizeText(text)
	if n, ok := index[key]; ok {
		if n.Chap == "" && chap != "" {
			updated := SST.Node{S: n.S, Chap: chap, NPtr: n.NPtr}
			SST.IdempDBAddNode(sst, updated)
			n.Chap = chap
			index[key] = n
		}
		return n
	}
	n := SST.Node{S: text, Chap: chap}
	n = SST.IdempDBAddNode(sst, n)
	SST.CacheNode(n) // register target node in NODE_DIRECTORY and NODE_CACHE
	index[key] = n
	bleveBatch.Add(n.NPtr, n.S, chap, sourceFile)
	result.NodesIngested++
	return n
}

func normalizeText(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// sameSequenceContext reports whether two nodes belong to the same
// `:: _sequence_, <label> ::` block by comparing every context tag except the
// `_sequence_` marker itself. Used to stop the implicit-then linker from fusing
// back-to-back sequences with different labels.
func sameSequenceContext(a, b []string) bool {
	as := stripSeqMarker(a)
	bs := stripSeqMarker(b)
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

func stripSeqMarker(ctx []string) []string {
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
