package explorer

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"texere/internal/content"
	"texere/internal/search"
)

// BuildResultSet converts a feature-004 search result into a deduped,
// key-sorted ResultSet ready for layout. It classifies every edge via
// parseMetaType and computes in-degree per node within the materialized
// subgraph (NOT the global graph — centrality is local to the result).
//
// The caller is responsible for issuing the ResultID and choosing the
// initial anchor; this function returns a ResultSet with both fields
// populated using the heuristic in data-model.md §2.2 (see
// pickDefaultAnchor in service.go).
//
// Two tiers of the search result are consumed:
//
//   - sr.Nodes (graph orbit, Tier 2). Each GraphNode has a NodeText +
//     ContextTags + Orbit; we add the node and emit one ResultEdge per
//     radius-1 orbit entry. This is the path used for plain-text and
//     name-bearing graph queries.
//
//   - sr.Notes (full-note view, the response to `\notes <chapter>`).
//     Each FullNote has a Chapter and Blocks, where each NoteBlock is
//     a root node with its authored arrows attached. We add the root
//     node, the target nodes, and one ResultEdge per arrow. This path
//     is what the launcher uses to render a chapter as an isometric
//     landscape. (`\chapter <name>` with no name term instead routes to a
//     TOC listing, handled by the Listings tier below.)
//
//   - sr.Structural (path + sequence chains, the response to
//     `\from … \to …` and `\sequence`). Each StructuralChain is an
//     ordered list of ChainNodes; we add every node and emit one
//     ResultEdge per consecutive pair, typed by the arrow that leads
//     to the next node. This renders paths and sequences as connected
//     chains in the landscape.
//
//   - sr.Listings (table-of-contents view, the response to `\chapter any`,
//     `\context any`, etc.). A listing has no node-level graph topology, so
//     we synthesize a "collection map": each chapter becomes a hub node and
//     each of its context tags becomes a satellite node connected by a
//     `contains` edge. This mirrors upstream SSTorytime's TOC view (chapters
//     as planets, contexts as moons) but laid out on our isometric grid —
//     each chapter forms its own cluster. The OmniBar keeps rendering the
//     same Listings as a flat list; only the explorer reinterprets them as a
//     graph.
func BuildResultSet(sr *search.SearchResult, registry *content.SSTRegistry, sourceQuery string) *ResultSet {
	rs := &ResultSet{
		ID:          uuid.NewString(),
		Nodes:       []ResultNode{},
		Edges:       []ResultEdge{},
		NodeIndex:   map[nodeKey]int{},
		SeedKeys:    map[nodeKey]bool{},
		BuiltAt:     time.Now(),
		SourceQuery: sourceQuery,
	}
	if sr == nil {
		return rs
	}

	// Pass 1: collect all node keys (parents + orbit neighbors) in stable
	// order. We use insertion order for determinism, then re-sort at the
	// end so the final slice is independent of input order.
	seen := map[nodeKey]struct{}{}
	addNode := func(k nodeKey, text, chapter string, contextTags []string) {
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		rs.Nodes = append(rs.Nodes, ResultNode{
			Key:      k,
			Text:     text,
			Chapter:  chapter,
			Contexts: contextTags,
		})
	}

	// ----- Tier 2: graph orbit results (sr.Nodes) ----------------------
	for _, gn := range sr.Nodes {
		k := nodeKey{Text: gn.NodeText, Chapter: gn.Chapter}
		// The GraphNode itself is a query match (a seed); its orbit entries
		// are neighbors pulled in around it, not matches.
		rs.SeedKeys[k] = true
		addNode(k, gn.NodeText, gn.Chapter, append([]string{}, gn.ContextTags...))
		// Orbit neighbors arrive without explicit chapter; attribute them
		// to the parent's chapter for primary-context grouping. This is a
		// pragmatic v1 choice — the search layer's Orbit data does not
		// carry per-neighbor chapter membership.
		for _, oe := range gn.Orbit {
			nk := nodeKey{Text: oe.NeighborText, Chapter: gn.Chapter}
			tags := splitContextTags(oe.Ctx)
			// Neighbours with no Ctx of their own inherit the source
			// GraphNode's primary context so they don't collapse to
			// the synthetic Untagged layer.
			if len(tags) == 0 && len(gn.ContextTags) > 0 {
				tags = append([]string{}, gn.ContextTags...)
			}
			addNode(nk, oe.NeighborText, gn.Chapter, tags)
		}
	}
	for _, gn := range sr.Nodes {
		seed := nodeKey{Text: gn.NodeText, Chapter: gn.Chapter}
		for _, oe := range gn.Orbit {
			// Edge for EVERY radius. The orbit is a BFS spanning tree where
			// each satellite records the ParentText it was reached from, so a
			// radius-2 node connects to its radius-1 parent (not the seed) and
			// deeper rings stay connected rather than floating. ParentText is
			// empty only for legacy/edge cases — fall back to the seed then.
			src := seed
			if oe.ParentText != "" {
				src = nodeKey{Text: oe.ParentText, Chapter: gn.Chapter}
			}
			dst := nodeKey{Text: oe.NeighborText, Chapter: gn.Chapter}
			meta, arrowCode, arrowLong := classifyArrow(registry, oe.ArrowLong)
			rs.Edges = append(rs.Edges, ResultEdge{
				Src:       src,
				Dst:       dst,
				ArrowCode: arrowCode,
				ArrowLong: arrowLong,
				Meta:      meta,
				Unknown:   isUnknownArrow(registry, arrowCode, arrowLong),
			})
		}
	}

	// ----- Notes tier (sr.Notes — response to `\notes <chapter>`) ------
	// Each NoteBlock is a root node; each NoteArrow is a typed edge to
	// a target node. We walk the slice TWICE so that authored block-
	// root contexts always pre-seed the `seen` set before any arrow
	// target inherits one.
	//
	//   Pass 1 — add every block root with its authored `blk.Context`.
	//   Pass 2 — add every arrow target, falling back to the source
	//            block's context if the target isn't already in `seen`.
	//
	// Iteration over `sr.Notes` / `fn.Blocks` is slice order (file
	// order), so for a target referenced from multiple blocks in
	// different contexts the first block's context wins
	// deterministically.
	for _, fn := range sr.Notes {
		for _, blk := range fn.Blocks {
			rootKey := nodeKey{Text: blk.NodeText, Chapter: fn.Chapter}
			// A note block's root node is an authored match; its arrow targets
			// (added below) are neighbors.
			rs.SeedKeys[rootKey] = true
			addNode(rootKey, blk.NodeText, fn.Chapter, splitContextTags(blk.Context))
		}
	}
	for _, fn := range sr.Notes {
		for _, blk := range fn.Blocks {
			parentTags := splitContextTags(blk.Context)
			for _, na := range blk.Arrows {
				if na.Target == "" {
					continue
				}
				targetKey := nodeKey{Text: na.Target, Chapter: fn.Chapter}
				addNode(targetKey, na.Target, fn.Chapter, parentTags)
			}
		}
	}
	for _, fn := range sr.Notes {
		for _, blk := range fn.Blocks {
			src := nodeKey{Text: blk.NodeText, Chapter: fn.Chapter}
			for _, na := range blk.Arrows {
				if na.Target == "" {
					continue
				}
				dst := nodeKey{Text: na.Target, Chapter: fn.Chapter}
				meta := classifyArrowByCode(registry, na.ArrowCode, na.ArrowLong)
				rs.Edges = append(rs.Edges, ResultEdge{
					Src:       src,
					Dst:       dst,
					ArrowCode: na.ArrowCode,
					ArrowLong: na.ArrowLong,
					Meta:      meta,
					Unknown:   isUnknownArrow(registry, na.ArrowCode, na.ArrowLong),
				})
			}
		}
	}

	// ----- Structural tier (sr.Structural — `\from…\to…`, `\sequence`) -
	// Each chain is an ordered walk of nodes. We add every node (keyed by
	// the chain's chapter) and connect consecutive nodes with the arrow
	// that leads to the NEXT node (ArrowCode/ArrowLong are empty on the
	// last node by contract). The chain's Context label — a sequence name
	// for `\sequence`, often empty for raw paths — is used as the node's
	// single context tag so a labelled chain groups onto its own layer.
	// Detect a pure cone/path result so the layout engine can flow it
	// left-to-right. True only when structural "path" chains are the sole
	// source of nodes — a mixed result (path chains plus orbit/notes/listing
	// nodes) keeps the default layered layout. Sequence chains are excluded:
	// they already render as their own timelines and shouldn't flip the mode.
	hasPathChain := false
	hasNonPathChain := false
	for _, chain := range sr.Structural {
		if chain.Kind == "path" {
			hasPathChain = true
		} else {
			hasNonPathChain = true
		}
	}
	rs.PathLike = hasPathChain && !hasNonPathChain &&
		len(sr.Nodes) == 0 && len(sr.Notes) == 0 && len(sr.Listings) == 0

	for _, chain := range sr.Structural {
		tags := splitContextTags(chain.Context)
		for i, cn := range chain.Nodes {
			if cn.NodeText == "" {
				continue
			}
			src := nodeKey{Text: cn.NodeText, Chapter: chain.Chapter}
			// Every node on a path/sequence chain is part of the matched
			// structure, so the whole chain counts as seeds.
			rs.SeedKeys[src] = true
			addNode(src, cn.NodeText, chain.Chapter, tags)
			if i+1 >= len(chain.Nodes) {
				continue
			}
			next := chain.Nodes[i+1]
			if next.NodeText == "" {
				continue
			}
			dst := nodeKey{Text: next.NodeText, Chapter: chain.Chapter}
			meta := classifyArrowByCode(registry, cn.ArrowCode, cn.ArrowLong)
			rs.Edges = append(rs.Edges, ResultEdge{
				Src:       src,
				Dst:       dst,
				ArrowCode: cn.ArrowCode,
				ArrowLong: cn.ArrowLong,
				Meta:      meta,
				Unknown:   isUnknownArrow(registry, cn.ArrowCode, cn.ArrowLong),
			})
		}
	}

	// ----- Listings tier (sr.Listings — `\chapter any`, `\context any`) -
	// A listing carries no node graph, so we synthesize a collection map:
	//
	//   Kind "chapter": a chapter hub node + one satellite per context tag,
	//     joined by a `contains` edge. Both hub and satellites are tagged with
	//     the chapter name as their primary context, so the layout engine
	//     places each chapter (hub + its contexts) as its own cluster/layer —
	//     the isometric equivalent of upstream's chapter-planet + context-moons.
	//
	//   Kind "context": the dual view (`\context any`) — a context hub node +
	//     one satellite per chapter that uses the tag, same contains edges.
	//
	// Hubs are the query matches (seeds); satellites are pulled-in context.
	// Synthetic arrows have no registry entry, so we label them "contains"
	// directly (MetaContains) rather than going through classifyArrow.
	for _, li := range sr.Listings {
		switch li.Kind {
		case "chapter":
			if li.Title == "" {
				continue
			}
			hub := nodeKey{Text: li.Title, Chapter: li.Title}
			rs.SeedKeys[hub] = true
			addNode(hub, li.Title, li.Title, []string{li.Title})
			for _, ctx := range li.Contexts {
				if ctx == "" {
					continue
				}
				sat := nodeKey{Text: ctx, Chapter: li.Title}
				addNode(sat, ctx, li.Title, []string{li.Title})
				rs.Edges = append(rs.Edges, ResultEdge{
					Src: hub, Dst: sat, ArrowLong: "contains", Meta: MetaContains,
				})
			}
		case "context":
			if li.Title == "" {
				continue
			}
			hub := nodeKey{Text: li.Title, Chapter: ""}
			rs.SeedKeys[hub] = true
			addNode(hub, li.Title, "", []string{li.Title})
			for _, chap := range li.Chapters {
				if chap == "" {
					continue
				}
				sat := nodeKey{Text: chap, Chapter: chap}
				addNode(sat, chap, chap, []string{li.Title})
				rs.Edges = append(rs.Edges, ResultEdge{
					Src: hub, Dst: sat, ArrowLong: "contains", Meta: MetaContains,
				})
			}
		}
	}

	// Dedupe edges by (unordered pair, meta) — collapses both exact
	// duplicates and inverse-arrow pairs (e.g. "contains" ↔ "belongs to")
	// that the SST orbit layer emits twice. The kept representative uses
	// the canonical direction Src.Key < Dst.Key. Authored direction is
	// not preserved in v1 (the registry layer doesn't carry the +/-
	// STType marker after parsing); the layout engine treats every edge
	// as flowing from Src → Dst per the meta-type's natural direction.
	rs.Edges = dedupeEdges(rs.Edges)

	// Sort nodes and edges deterministically by key.
	sort.Slice(rs.Nodes, func(i, j int) bool {
		return keyLess(rs.Nodes[i].Key, rs.Nodes[j].Key)
	})
	sort.Slice(rs.Edges, func(i, j int) bool {
		a, b := rs.Edges[i], rs.Edges[j]
		if a.Src != b.Src {
			return keyLess(a.Src, b.Src)
		}
		if a.Dst != b.Dst {
			return keyLess(a.Dst, b.Dst)
		}
		return a.Meta < b.Meta
	})

	// Build NodeIndex after sorting so indices are stable.
	for i, n := range rs.Nodes {
		rs.NodeIndex[n.Key] = i
	}

	// Compute in-degree by walking edges.
	for _, e := range rs.Edges {
		if idx, ok := rs.NodeIndex[e.Dst]; ok {
			rs.Nodes[idx].InDegree++
		}
	}

	// Collect distinct contexts in stable sorted order.
	ctxSeen := map[string]struct{}{}
	for _, n := range rs.Nodes {
		for _, c := range n.Contexts {
			if c == "" {
				continue
			}
			ctxSeen[c] = struct{}{}
		}
	}
	rs.Contexts = make([]string, 0, len(ctxSeen))
	for c := range ctxSeen {
		rs.Contexts = append(rs.Contexts, c)
	}
	sort.Strings(rs.Contexts)

	return rs
}

// isUnknownArrow reports whether an arrow code/long-name corresponds to a
// synthesized "unknown" ArrowDef (one not declared in any SSTconfig). The
// builder gives such arrows Code == LongName == the authored code, so we match
// on either. Used to flag edges so the frontend can render them distinctly.
func isUnknownArrow(registry *content.SSTRegistry, arrowCode, arrowLong string) bool {
	if registry == nil {
		return false
	}
	if arrowCode != "" {
		if def, ok := registry.Arrows[arrowCode]; ok {
			return def.Unknown
		}
	}
	if arrowLong != "" {
		// Orbit results carry only the long name; for synthesized arrows that
		// equals the code, so a direct registry lookup still resolves it.
		if def, ok := registry.Arrows[arrowLong]; ok {
			return def.Unknown
		}
	}
	return false
}

// classifyArrow looks up an arrow's meta-type from the registry. The
// search layer gives us the arrow's long name, so we scan the registry
// entries for a matching LongName to recover the code and meta-type.
//
// If the arrow is the implicit sequence pair ("then followed by" /
// "follows on from") that feature 004 auto-registers without a registry
// entry, we treat it as MetaLeadsTo.
func classifyArrow(registry *content.SSTRegistry, arrowLong string) (MetaType, string, string) {
	if arrowLong == "" {
		return MetaUnclassified, "", ""
	}
	// Implicit sequence arrows (search.SequenceArrowLong /
	// SequenceInvArrowLong) — match by string to avoid pulling in the
	// search package's constants (which would create an import cycle if
	// search ever depends on explorer).
	if strings.EqualFold(arrowLong, "then followed by") ||
		strings.EqualFold(arrowLong, "follows on from") {
		return MetaLeadsTo, "", arrowLong
	}
	if registry == nil {
		return MetaUnclassified, "", arrowLong
	}
	for code, def := range registry.Arrows {
		if strings.EqualFold(def.LongName, arrowLong) {
			return parseMetaType(def.MetaType), code, def.LongName
		}
	}
	return MetaUnclassified, "", arrowLong
}

// classifyArrowByCode looks up an arrow's meta-type from the registry
// using its short code (e.g. "contain", "next") rather than its long
// name. The Notes tier's NoteArrow carries both, but ArrowCode is the
// faster + more reliable key — the registry is keyed by code. Falls
// back to ArrowLong matching when the code lookup fails so the
// implicit sequence arrow pair (which has no code) still classifies
// correctly.
func classifyArrowByCode(registry *content.SSTRegistry, arrowCode, arrowLong string) MetaType {
	if registry != nil && arrowCode != "" {
		if def, ok := registry.Arrows[arrowCode]; ok {
			return parseMetaType(def.MetaType)
		}
	}
	meta, _, _ := classifyArrow(registry, arrowLong)
	return meta
}

// dedupeEdges collapses exact (Src, Dst, Meta) triples. We deliberately
// do NOT collapse inverse-direction pairs — flipping direction to a
// canonical Src.Key < Dst.Key form was corrupting authored direction
// from the Notes tier (which is direction-faithful: each NoteArrow
// records the exact authored Src → Target pair).
//
// Cost of this choice: the orbit tier (sr.Nodes / OrbitEntry) emits
// both forward and inverse links per relation, so the explorer may
// render two parallel edges for the same authored arrow when an
// inverse arrow is registered for it. That is acceptable visual noise
// for v1 — much better than silently inverting parent/child semantics.
// A future polish task can dedupe inverse pairs by consulting the
// registry's `Reverse` field on each ArrowDef.
func dedupeEdges(edges []ResultEdge) []ResultEdge {
	type edgeKey struct {
		Src, Dst nodeKey
		Meta     MetaType
	}
	seen := make(map[edgeKey]bool, len(edges))
	out := make([]ResultEdge, 0, len(edges))
	for _, e := range edges {
		k := edgeKey{Src: e.Src, Dst: e.Dst, Meta: e.Meta}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}

// keyLess returns whether a < b in the canonical (Chapter, Text) order.
func keyLess(a, b nodeKey) bool {
	if a.Chapter != b.Chapter {
		return a.Chapter < b.Chapter
	}
	return a.Text < b.Text
}

// splitContextTags parses the comma-separated context string used in the
// search layer's OrbitEntry.Ctx field into a clean slice. Empty entries
// are dropped; surrounding whitespace is trimmed.
func splitContextTags(ctx string) []string {
	if ctx == "" {
		return nil
	}
	parts := strings.Split(ctx, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
