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
		pf, _ := content.ParseFile(summary.FilePath, registry)
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

	// Commit Pass 1's index writes before Pass 2 starts so that any reader
	// running between the two passes sees a consistent view.
	if err := bleveIdx.ApplyBatch(bleveBatch); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("bleve apply batch (pass 1): %v", err))
	}

	// Pass 2 — add links and populate in-memory link arrays for orbit queries.
	for _, summary := range summaries {
		if summary.ScanError != nil {
			continue
		}
		pf, _ := content.ParseFile(summary.FilePath, registry)
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
					if _, ok := registry.Arrows[arrow.Code]; !ok {
						result.UnknownArrows[arrow.Code]++
						continue
					}
					toNode := ensureNode(sst, bleveIdx, arrow.Target, ch.Title, sourceFile, nodeIndex, result)
					if fromNode.NPtr == toNode.NPtr {
						continue
					}
					arrowPtr, _ := SST.Edge(sst, fromNode, arrow.Code, toNode, node.EffectiveCtx, 1.0)
					appendMemoryLink(arrowPtr, fromNode.NPtr, toNode.NPtr, ctxPtrForNode)
					appendArrowLeg(arrow, toNode, arrowPtr)
					result.LinksIngested++
				}
				for _, ditto := range node.Dittos {
					for _, arrow := range ditto.Arrows {
						if arrow.Target == "" || arrow.Code == "" {
							continue
						}
						if _, ok := registry.Arrows[arrow.Code]; !ok {
							continue
						}
						toNode := ensureNode(sst, bleveIdx, arrow.Target, ch.Title, sourceFile, nodeIndex, result)
						if fromNode.NPtr == toNode.NPtr {
							continue
						}
						arrowPtr, _ := SST.Edge(sst, fromNode, arrow.Code, toNode, node.EffectiveCtx, 1.0)
						appendMemoryLink(arrowPtr, fromNode.NPtr, toNode.NPtr, ctxPtrForNode)
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
// New nodes created in Pass 2 are also indexed into the Bleve overlay
// immediately (Pass 1's batch has already been committed by the caller).
// These are typically rare arrow targets that weren't chapter roots.
func ensureNode(sst SST.PoSST, bleveIdx *SST.Index, text, chap, sourceFile string, index map[string]SST.Node, result *BuildResult) SST.Node {
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
	if err := bleveIdx.AddNode(n.NPtr, n.S, chap, sourceFile); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("bleve add node %q: %v", text, err))
	}
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
