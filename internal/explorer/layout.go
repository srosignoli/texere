package explorer

import (
	"sort"
	"strings"
	"time"
)

// =============================================================================
// Layout engine — Layered Knowledge Graph.
//
// Each context is rendered as its own translucent grid floor stacked
// vertically along Y. Meta-types (CONTAINS / LEADSTO / PROPERTIES /
// SIMILARTO) no longer steer spatial axes; they are conveyed by edge
// colour + line style in the frontend. Within each layer, nodes are
// placed by concentric BFS distance from the anchor — closer = nearer
// the layer's local origin.
//
// Determinism: contexts are sorted alphabetically (with the synthetic
// Untagged layer pinned at index 0), BFS neighbour lists are pre-
// sorted by key, and angular slots within a depth bucket are assigned
// by stable key sort.
// =============================================================================

const (
	// LayerSpacing is the world-Y distance between adjacent context
	// floors. The frontend exposes a slider that scales node Y after
	// the fact, so this constant is just the canonical reference
	// spacing used when computing LayoutBounds and edge geometry.
	LayerSpacing float32 = 6.0

	// GridStep is the world-units distance between adjacent grid
	// cells on each layer. Nodes within a layer are snapped to
	// (gridX*GridStep, layerY, gridZ*GridStep) so the orthogonal
	// edge router (frontend EdgeMeshes.js) can draw axis-aligned
	// polylines with sharp 90° bends. Larger than MaxNodeSize so a
	// landmark sphere never touches its neighbour's cell.
	GridStep float32 = 3.0

	// MinNodeSize / MaxNodeSize bound the visible node radius. The
	// visible-radius constant on the frontend (RADIUS_SCALE in
	// NodeMeshes.js) preserves the same 3× ratio.
	MinNodeSize float32 = 0.5
	MaxNodeSize float32 = 1.5

	// UntaggedContextID is the synthetic context key used for nodes
	// with no primary tag. The Untagged layer is always pinned to
	// layer 0 (bottom of the stack) so its position is stable even
	// as user-tagged contexts come and go.
	UntaggedContextID = ""

	// UntaggedLayerLabel is the human-readable label rendered on the
	// Untagged plane.
	UntaggedLayerLabel = "Untagged"

	// LayoutModeStacked is the default — each primary context becomes
	// its own translucent grid floor stacked along Y.
	LayoutModeStacked = "stacked"

	// LayoutModeExtended places every node on a single Y=0 plane;
	// contexts are visually distinguished by spatial clustering plus
	// per-context sphere tint on the frontend. The two modes share
	// the same per-context grid placement routine
	// (placeOrthogonalCluster); only the cluster translation and the
	// Y assignment differ.
	LayoutModeExtended = "extended"

	// LayoutModeFlow lays a directed cone/path out left-to-right: every node's
	// X is its directed BFS depth from the cone root(s), so the path reads in
	// authored direction along +X, while siblings/branches fan out along Z.
	// Per-context Y floors are preserved (one grid per context, as in Stacked)
	// so a multi-context cone still separates by grid. Auto-selected for
	// ResultSet.PathLike results; not a user-facing toggle in v1.
	LayoutModeFlow = "flow"

	// ClusterGap (Extended mode) is the extra world-units padding
	// inserted between cluster slots in the centered X-row, on top of
	// each cluster's own grid extent. Keeps cluster boundaries
	// visually obvious without making the landscape unreasonably
	// wide.
	ClusterGap float32 = 2.0 * GridStep
)

// Adjacency holds typed neighbour lists per node. Kept around because
// breakContainmentCycles consumes it and tests still inspect typed
// edges; the layout engine itself now uses a single flat neighbour
// map (allEdges below) because it no longer routes per-meta-type.
type Adjacency struct {
	Children     map[nodeKey][]nodeKey
	Parents      map[nodeKey][]nodeKey
	Successors   map[nodeKey][]nodeKey
	Predecessors map[nodeKey][]nodeKey
	Properties   map[nodeKey][]nodeKey
	Owners       map[nodeKey][]nodeKey
	SimilarTo    map[nodeKey][]nodeKey
}

// buildAdjacency walks the result-set edges and produces typed
// neighbour lists. Containment cycle detection still uses this.
func buildAdjacency(rs *ResultSet) *Adjacency {
	adj := &Adjacency{
		Children:     make(map[nodeKey][]nodeKey),
		Parents:      make(map[nodeKey][]nodeKey),
		Successors:   make(map[nodeKey][]nodeKey),
		Predecessors: make(map[nodeKey][]nodeKey),
		Properties:   make(map[nodeKey][]nodeKey),
		Owners:       make(map[nodeKey][]nodeKey),
		SimilarTo:    make(map[nodeKey][]nodeKey),
	}
	for _, e := range rs.Edges {
		switch e.Meta {
		case MetaContains:
			adj.Children[e.Src] = append(adj.Children[e.Src], e.Dst)
			adj.Parents[e.Dst] = append(adj.Parents[e.Dst], e.Src)
		case MetaLeadsTo:
			adj.Successors[e.Src] = append(adj.Successors[e.Src], e.Dst)
			adj.Predecessors[e.Dst] = append(adj.Predecessors[e.Dst], e.Src)
		case MetaProperties:
			adj.Properties[e.Src] = append(adj.Properties[e.Src], e.Dst)
			adj.Owners[e.Dst] = append(adj.Owners[e.Dst], e.Src)
		case MetaSimilarTo:
			adj.SimilarTo[e.Src] = append(adj.SimilarTo[e.Src], e.Dst)
			adj.SimilarTo[e.Dst] = append(adj.SimilarTo[e.Dst], e.Src)
		}
	}
	for _, m := range []map[nodeKey][]nodeKey{
		adj.Children, adj.Parents, adj.Successors, adj.Predecessors,
		adj.Properties, adj.Owners, adj.SimilarTo,
	} {
		for k, v := range m {
			sort.Slice(v, func(i, j int) bool { return keyLess(v[i], v[j]) })
			m[k] = v
		}
	}
	return adj
}

// buildUndirectedAdjacency returns key → sorted unique neighbour list
// across *all* meta-types. Used by the global BFS distance map.
func buildUndirectedAdjacency(rs *ResultSet) map[nodeKey][]nodeKey {
	tmp := make(map[nodeKey]map[nodeKey]struct{}, len(rs.Nodes))
	for _, n := range rs.Nodes {
		tmp[n.Key] = map[nodeKey]struct{}{}
	}
	for _, e := range rs.Edges {
		if _, ok := tmp[e.Src]; ok {
			tmp[e.Src][e.Dst] = struct{}{}
		}
		if _, ok := tmp[e.Dst]; ok {
			tmp[e.Dst][e.Src] = struct{}{}
		}
	}
	out := make(map[nodeKey][]nodeKey, len(tmp))
	for k, set := range tmp {
		list := make([]nodeKey, 0, len(set))
		for n := range set {
			list = append(list, n)
		}
		sort.Slice(list, func(i, j int) bool { return keyLess(list[i], list[j]) })
		out[k] = list
	}
	return out
}

// LayoutFromAnchor computes a deterministic layered 3D layout for rs
// around the supplied anchor.
//
// Steps:
//  1. Compute global BFS distance from anchor along the undirected
//     graph (all meta-types).
//  2. Group nodes by primary context. Nil-context nodes land in the
//     synthetic Untagged bucket.
//  3. Sort contexts alphabetically (Untagged pinned at the bottom).
//  4. For each layer, pick a layer-local anchor = the member with the
//     smallest global BFS distance (ties broken by key). Place it at
//     (0, layerY, 0).
//  5. Concentric BFS within each layer's intra-layer edge subgraph:
//     depth-1 around radius LayerOriginRadius, depth-d at
//     LayerOriginRadius + (d-1)*LayerRingStep, angular slot from
//     stable key sort. Disconnected intra-layer nodes sit on a
//     fallback ring outside the connected portion.
//
// Determinism: same (rs, anchor, mode) always yields the same LayoutPass.
//
// `mode` selects the spatial encoding of context membership:
//   - LayoutModeStacked (or ""): per-context Y-stacked floors.
//   - LayoutModeExtended: single Y=0 plane with contexts arranged as
//     adjacent clusters in a centered X-row.
//
// In both modes pass.Layers is populated so the frontend's per-context
// checkboxes / labels / colorSlots remain meaningful; only the Y and
// (x, z) placement of cluster members differ between modes.
func LayoutFromAnchor(rs *ResultSet, anchor nodeKey, mode string) *LayoutPass {
	start := time.Now()
	pass := &LayoutPass{
		AnchorKey:   anchor,
		Positions:   make(map[nodeKey]Vec3),
		NodeSizes:   make(map[nodeKey]float32),
		Distances:   make(map[nodeKey]int),
		CycleNodes:  make(map[nodeKey]bool),
		Layers:      nil,
		Territories: nil,
		ComputedAt:  start,
	}
	if rs == nil || len(rs.Nodes) == 0 {
		pass.LayoutMs = elapsedMs(start)
		return pass
	}

	if mode == "" {
		mode = LayoutModeStacked
	}

	// Containment-cycle detection still uses the typed adjacency.
	adj := buildAdjacency(rs)
	pass.CycleNodes = breakContainmentCycles(rs, adj)

	// Global BFS distance map drives per-cluster anchor selection.
	pass.Distances = globalBFSDistances(rs, anchor)

	// Group nodes by primary context (Untagged bucket for nil tags).
	buckets := groupByLayer(rs)

	// Stable cluster order. Stacked: Untagged at the bottom, then
	// alphabetical. Extended: the global anchor's context first
	// (centered at X=0), then alphabetical (Untagged sorted as the
	// empty string).
	contexts := orderedContexts(buckets, rs, anchor, mode)

	switch mode {
	case LayoutModeFlow:
		placeFlow(rs, pass)
	case LayoutModeExtended:
		placeExtended(rs, contexts, buckets, anchor, pass)
	default:
		placeStacked(rs, contexts, buckets, pass)
	}

	// Log-scaled node sizes from in-degree centrality (mode-agnostic).
	pass.NodeSizes = ScaleSizes(rs, MinNodeSize, MaxNodeSize)

	pass.LayoutMs = elapsedMs(start)
	return pass
}

// orderedContexts produces the deterministic context ordering for the
// given mode. The result drives both the cluster traversal order and
// the LayerView indices the frontend uses for color slots.
func orderedContexts(buckets map[string][]nodeKey, rs *ResultSet, anchor nodeKey, mode string) []string {
	contexts := make([]string, 0, len(buckets))
	for ctx := range buckets {
		contexts = append(contexts, ctx)
	}
	if mode == LayoutModeExtended {
		// Find the anchor's own context so it can lead the row.
		anchorCtx := UntaggedContextID
		if idx, ok := rs.NodeIndex[anchor]; ok && idx < len(rs.Nodes) {
			n := rs.Nodes[idx]
			if len(n.Contexts) > 0 && n.Contexts[0] != "" {
				anchorCtx = n.Contexts[0]
			}
		}
		sort.Slice(contexts, func(i, j int) bool {
			a, b := contexts[i], contexts[j]
			if a == anchorCtx {
				return true
			}
			if b == anchorCtx {
				return false
			}
			return a < b
		})
		return contexts
	}
	// Stacked default: Untagged first, then alphabetical.
	sort.Slice(contexts, func(i, j int) bool {
		a, b := contexts[i], contexts[j]
		if a == UntaggedContextID {
			return true
		}
		if b == UntaggedContextID {
			return false
		}
		return a < b
	})
	return contexts
}

// placeStacked is the v1 layout: each context becomes its own grid
// floor stacked along Y at layerIdx × LayerSpacing.
func placeStacked(rs *ResultSet, contexts []string, buckets map[string][]nodeKey, pass *LayoutPass) {
	pass.Layers = make([]LayerPass, 0, len(contexts))
	for layerIdx, ctx := range contexts {
		members := buckets[ctx]
		sort.Slice(members, func(i, j int) bool { return keyLess(members[i], members[j]) })

		y := float32(layerIdx) * LayerSpacing
		label := ctx
		colorSlot := layerIdx
		if ctx == UntaggedContextID {
			label = UntaggedLayerLabel
			colorSlot = -1
		}

		localAnchor := pickClusterAnchor(members, pass.Distances)
		offsets, _ := placeOrthogonalCluster(rs, members, localAnchor)
		for k, c := range offsets {
			pass.Positions[k] = Vec3{
				X: float32(c.X) * GridStep,
				Y: y,
				Z: float32(c.Z) * GridStep,
			}
		}

		cx, cz, ex, ez := clusterBounds(pass.Positions, members)
		pass.Layers = append(pass.Layers, LayerPass{
			Context:   ctx,
			Label:     label,
			Tags:      collectClusterTags(rs, members),
			Chapters:  collectClusterChapters(rs, members),
			Y:         y,
			Members:   members,
			ColorSlot: colorSlot,
			CenterX:   cx,
			CenterZ:   cz,
			ExtentX:   ex,
			ExtentZ:   ez,
		})
	}
}

// FlowComponentGap is the empty-Z padding inserted between adjacent flow
// components so two disconnected cones read as clearly separate rows rather
// than blurring into one band.
const FlowComponentGap float32 = 2.0 * GridStep

// placeFlow lays a directed cone/path out left-to-right, one CONNECTED COMPONENT
// per row. For each component: X encodes a node's directed BFS depth from the
// component's root(s) — depth 0 sits at X=0 so every root lines up on the left
// edge and the path flows along +X. Z lanes within a component come from a
// depth-first walk of the forward edges (trunk stays on one lane, branches peel
// off). Distinct components are stacked along Z with FlowComponentGap between
// them, and each becomes its own LayerPass so the frontend draws a separate grid
// around it — disconnected graphs never share a row.
func placeFlow(rs *ResultSet, pass *LayoutPass) {
	depth := directedFlowDepths(rs)
	fwd := buildDirectedAdjacency(rs)
	components := connectedComponents(rs)

	pass.Layers = make([]LayerPass, 0, len(components))
	var zCursor float32
	for ci, members := range components {
		sort.Slice(members, func(i, j int) bool { return keyLess(members[i], members[j]) })

		lanes := assignFlowLanes(members, fwd, depth)
		maxLane := 0
		for _, l := range lanes {
			if l > maxLane {
				maxLane = l
			}
		}

		for _, k := range members {
			pass.Positions[k] = Vec3{
				X: float32(depth[k]) * GridStep,
				Y: 0,
				Z: zCursor + float32(lanes[k])*GridStep,
			}
		}

		label := flowComponentLabel(rs, members, depth)
		cx, cz, ex, ez := clusterBounds(pass.Positions, members)
		pass.Layers = append(pass.Layers, LayerPass{
			Context:   label,
			Label:     label,
			Tags:      collectClusterTags(rs, members),
			Chapters:  collectClusterChapters(rs, members),
			Y:         0,
			Members:   members,
			ColorSlot: ci,
			CenterX:   cx,
			CenterZ:   cz,
			ExtentX:   ex,
			ExtentZ:   ez,
		})

		// Advance the Z cursor past this component's lane span plus a gap so the
		// next component starts on a fresh row.
		zCursor += float32(maxLane+1)*GridStep + FlowComponentGap
	}
}

// connectedComponents partitions the result's nodes into undirected connected
// components, each returned as a key-sorted member slice. Components are ordered
// largest-first (ties broken by lowest member key) so the biggest cone takes the
// top row deterministically. Isolated nodes form singleton components.
func connectedComponents(rs *ResultSet) [][]nodeKey {
	nbrs := buildUndirectedAdjacency(rs)
	seen := make(map[nodeKey]bool, len(rs.Nodes))
	var comps [][]nodeKey
	for _, n := range rs.Nodes { // rs.Nodes is key-sorted → deterministic seeds
		if seen[n.Key] {
			continue
		}
		queue := []nodeKey{n.Key}
		seen[n.Key] = true
		var members []nodeKey
		for len(queue) > 0 {
			k := queue[0]
			queue = queue[1:]
			members = append(members, k)
			for _, nb := range nbrs[k] {
				if !seen[nb] {
					seen[nb] = true
					queue = append(queue, nb)
				}
			}
		}
		comps = append(comps, members)
	}
	sort.SliceStable(comps, func(i, j int) bool {
		if len(comps[i]) != len(comps[j]) {
			return len(comps[i]) > len(comps[j])
		}
		return keyLess(minKey(comps[i]), minKey(comps[j]))
	})
	return comps
}

// minKey returns the lowest-ordered key in a non-empty member slice.
func minKey(members []nodeKey) nodeKey {
	best := members[0]
	for _, k := range members[1:] {
		if keyLess(k, best) {
			best = k
		}
	}
	return best
}

// flowComponentLabel names a flow component by its root — the shallowest node
// (lowest directed depth), ties broken by key — so each component's grid is
// labelled with the node its cone starts from (e.g. "start").
func flowComponentLabel(rs *ResultSet, members []nodeKey, depth map[nodeKey]int) string {
	best := members[0]
	bestDepth := depth[best]
	for _, k := range members[1:] {
		d := depth[k]
		if d < bestDepth || (d == bestDepth && keyLess(k, best)) {
			best = k
			bestDepth = d
		}
	}
	if idx, ok := rs.NodeIndex[best]; ok {
		return rs.Nodes[idx].Text
	}
	return best.Text
}

// buildDirectedAdjacency returns key → sorted forward neighbour list following
// authored edge direction (Src → Dst). Used by the Flow layout to walk a cone
// in its natural direction.
func buildDirectedAdjacency(rs *ResultSet) map[nodeKey][]nodeKey {
	out := make(map[nodeKey][]nodeKey, len(rs.Nodes))
	seen := make(map[nodeKey]map[nodeKey]struct{}, len(rs.Nodes))
	for _, e := range rs.Edges {
		if _, ok := seen[e.Src]; !ok {
			seen[e.Src] = map[nodeKey]struct{}{}
		}
		if _, dup := seen[e.Src][e.Dst]; dup {
			continue
		}
		seen[e.Src][e.Dst] = struct{}{}
		out[e.Src] = append(out[e.Src], e.Dst)
	}
	for k, list := range out {
		sort.Slice(list, func(i, j int) bool { return keyLess(list[i], list[j]) })
		out[k] = list
	}
	return out
}

// flowRoots returns the cone's source nodes — those with no incoming edge
// inside the result — in deterministic key order. When the result is a pure
// cycle (no zero-in-degree node) it falls back to the lowest-keyed node so the
// depth walk always has a starting point.
func flowRoots(rs *ResultSet) []nodeKey {
	roots := make([]nodeKey, 0)
	for _, n := range rs.Nodes {
		if n.InDegree == 0 {
			roots = append(roots, n.Key)
		}
	}
	if len(roots) == 0 && len(rs.Nodes) > 0 {
		// All nodes have an incoming edge (cyclic). Seed from the lowest key.
		best := rs.Nodes[0].Key
		for _, n := range rs.Nodes[1:] {
			if keyLess(n.Key, best) {
				best = n.Key
			}
		}
		roots = append(roots, best)
	}
	sort.Slice(roots, func(i, j int) bool { return keyLess(roots[i], roots[j]) })
	return roots
}

// directedFlowDepths runs a multi-source directed BFS from every cone root and
// returns each node's hop distance (its X column in the Flow layout). Nodes
// unreachable in the forward direction (e.g. stranded by a cycle) default to
// depth 0 via the caller's map zero-value.
func directedFlowDepths(rs *ResultSet) map[nodeKey]int {
	fwd := buildDirectedAdjacency(rs)
	out := make(map[nodeKey]int, len(rs.Nodes))
	queue := make([]nodeKey, 0, len(rs.Nodes))
	for _, r := range flowRoots(rs) {
		if _, seen := out[r]; seen {
			continue
		}
		out[r] = 0
		queue = append(queue, r)
	}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		d := out[k]
		for _, nb := range fwd[k] {
			if _, seen := out[nb]; seen {
				continue
			}
			out[nb] = d + 1
			queue = append(queue, nb)
		}
	}
	return out
}

// assignFlowLanes assigns each layer member a Z lane. A depth-first walk of the
// forward edges (restricted to this layer) keeps a path's trunk on one lane —
// the first child inherits its parent's lane, later children and disconnected
// members get fresh lanes. Deterministic: members and children are visited in
// key order, roots in (depth, key) order.
func assignFlowLanes(members []nodeKey, fwd map[nodeKey][]nodeKey, depth map[nodeKey]int) map[nodeKey]int {
	memberSet := make(map[nodeKey]struct{}, len(members))
	for _, k := range members {
		memberSet[k] = struct{}{}
	}
	lane := make(map[nodeKey]int, len(members))
	visited := make(map[nodeKey]struct{}, len(members))
	next := 0
	newLane := func() int { l := next; next++; return l }

	var walk func(k nodeKey, l int)
	walk = func(k nodeKey, l int) {
		if _, done := visited[k]; done {
			return
		}
		visited[k] = struct{}{}
		lane[k] = l
		first := true
		for _, c := range fwd[k] {
			if _, ok := memberSet[c]; !ok {
				continue
			}
			if _, done := visited[c]; done {
				continue
			}
			if first {
				walk(c, l)
				first = false
			} else {
				walk(c, newLane())
			}
		}
	}

	// Visit members shallowest-first (then key) so roots anchor the low lanes.
	order := append([]nodeKey(nil), members...)
	sort.Slice(order, func(i, j int) bool {
		di, dj := depth[order[i]], depth[order[j]]
		if di != dj {
			return di < dj
		}
		return keyLess(order[i], order[j])
	})
	for _, k := range order {
		if _, done := visited[k]; !done {
			walk(k, newLane())
		}
	}
	return lane
}

// placeExtended arranges every context's subgraph on a single Y=0
// plane. Clusters are laid out as a 2-D **map** centred on the global
// anchor's context: slot 0 sits at grid cell (0, 0); subsequent
// clusters spiral outward through Chebyshev rings of ever-increasing
// radius (ring r has 8r cells), filled in alphabetical context order.
// This spreads contexts across both X and Z instead of bunching them
// into a single diagonal row when there are many of them. Slot pitch
// is derived from the widest cluster's Chebyshev radius so
// neighbouring clusters keep ClusterGap of breathing room regardless
// of which slot they end up in.
func placeExtended(
	rs *ResultSet,
	contexts []string,
	buckets map[string][]nodeKey,
	anchor nodeKey,
	pass *LayoutPass,
) {
	// First pass: place every cluster locally and capture its radius.
	type clusterPlacement struct {
		ctx       string
		members   []nodeKey
		offsets   map[nodeKey]gridCell
		radius    int
		colorSlot int
	}
	placements := make([]clusterPlacement, 0, len(contexts))
	maxRadius := 1
	for ctxIdx, ctx := range contexts {
		members := buckets[ctx]
		sort.Slice(members, func(i, j int) bool { return keyLess(members[i], members[j]) })
		localAnchor := pickClusterAnchor(members, pass.Distances)
		offsets, radius := placeOrthogonalCluster(rs, members, localAnchor)
		if radius > maxRadius {
			maxRadius = radius
		}
		colorSlot := ctxIdx
		if ctx == UntaggedContextID {
			colorSlot = -1
		}
		placements = append(placements, clusterPlacement{
			ctx:       ctx,
			members:   members,
			offsets:   offsets,
			radius:    radius,
			colorSlot: colorSlot,
		})
	}

	// Slot pitch in world units. We size it by the widest cluster so
	// neighbouring clusters keep at least ClusterGap of breathing room
	// regardless of which slot they end up in.
	slotPitch := float32(2*maxRadius+1)*GridStep + ClusterGap

	pass.Layers = make([]LayerPass, 0, len(placements))
	for i, p := range placements {
		slot := slotForExtendedIndex(i)
		centerX := float32(slot.X) * slotPitch
		centerZ := float32(slot.Z) * slotPitch

		for k, c := range p.offsets {
			pass.Positions[k] = Vec3{
				X: centerX + float32(c.X)*GridStep,
				Y: 0,
				Z: centerZ + float32(c.Z)*GridStep,
			}
		}

		label := p.ctx
		if p.ctx == UntaggedContextID {
			label = UntaggedLayerLabel
		}
		cx, cz, ex, ez := clusterBounds(pass.Positions, p.members)
		pass.Layers = append(pass.Layers, LayerPass{
			Context:   p.ctx,
			Label:     label,
			Tags:      collectClusterTags(rs, p.members),
			Chapters:  collectClusterChapters(rs, p.members),
			Y:         0,
			Members:   p.members,
			ColorSlot: p.colorSlot,
			CenterX:   cx,
			CenterZ:   cz,
			ExtentX:   ex,
			ExtentZ:   ez,
		})
	}
}

// slotForExtendedIndex maps a 0-based cluster index to a (X, Z) grid
// cell on the cluster slot grid in Extended mode. Index 0 → (0, 0)
// (the anchor's cluster). Subsequent indices spiral outward through
// Chebyshev rings:
//
//   slot   1 .. 8   → ring 1 (8 cells)
//   slot   9 .. 24  → ring 2 (16 cells)
//   slot  25 .. 48  → ring 3 (24 cells)
//   ...
//
// Cumulative cells through ring r is `1 + 4*r*(r+1)`, so the smallest
// r containing slot `i` is the one with `4*r*(r+1) >= i`. Within that
// ring, the position is `i - (1 + 4*(r-1)*r)`-th cell of chebyshevRing(r).
//
// Walks the ring in CCW order starting at the (-r, -r) corner — same
// order chebyshevRing already returns.
func slotForExtendedIndex(i int) gridCell {
	if i <= 0 {
		return gridCell{0, 0}
	}
	r := 1
	for 4*r*(r+1) < i {
		r++
	}
	cellsBeforeRing := 4 * (r - 1) * r // cumulative cells in rings 1..r-1 (excluding centre)
	idxInRing := i - cellsBeforeRing - 1
	ring := chebyshevRing(r)
	if idxInRing < 0 || idxInRing >= len(ring) {
		// Defensive: would only fire on int overflow at unrealistic N.
		return ring[len(ring)-1]
	}
	return ring[idxInRing]
}

// collectClusterTags returns the deduped union of every member's
// Contexts slice in first-seen order. Used to populate LayerView.Tags
// so the frontend can surface the full authored context list (e.g.
// `physics, connectivity, path example`) on hover, even though only
// the primary tag drives layer placement.
func collectClusterTags(rs *ResultSet, members []nodeKey) []string {
	seen := map[string]struct{}{}
	tags := []string{}
	for _, k := range members {
		idx, ok := rs.NodeIndex[k]
		if !ok {
			continue
		}
		for _, t := range rs.Nodes[idx].Contexts {
			if t == "" {
				continue
			}
			if _, dup := seen[t]; dup {
				continue
			}
			seen[t] = struct{}{}
			tags = append(tags, t)
		}
	}
	return tags
}

// collectClusterChapters returns the distinct parent chapters across a
// cluster's members, alphabetically sorted. A node's Chapter may itself be a
// comma-separated list (a node authored under several chapters), so we split
// on commas and trim. Used to label a context grid with the chapter(s) it
// spans now that a single context can draw nodes from multiple chapters.
func collectClusterChapters(rs *ResultSet, members []nodeKey) []string {
	seen := map[string]struct{}{}
	chapters := []string{}
	for _, k := range members {
		idx, ok := rs.NodeIndex[k]
		if !ok {
			continue
		}
		for _, c := range strings.Split(rs.Nodes[idx].Chapter, ",") {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			if _, dup := seen[c]; dup {
				continue
			}
			seen[c] = struct{}{}
			chapters = append(chapters, c)
		}
	}
	sort.Strings(chapters)
	return chapters
}

// clusterBounds computes the (x, z) bounding box of a cluster from
// the supplied member positions. CenterX/Z is the box center;
// ExtentX/Z is the half-width. Returns zeros when members is empty.
func clusterBounds(positions map[nodeKey]Vec3, members []nodeKey) (cx, cz, ex, ez float32) {
	if len(members) == 0 {
		return 0, 0, 0, 0
	}
	first := true
	var minX, maxX, minZ, maxZ float32
	for _, k := range members {
		p, ok := positions[k]
		if !ok {
			continue
		}
		if first {
			minX, maxX, minZ, maxZ = p.X, p.X, p.Z, p.Z
			first = false
			continue
		}
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Z < minZ {
			minZ = p.Z
		}
		if p.Z > maxZ {
			maxZ = p.Z
		}
	}
	cx = (minX + maxX) / 2
	cz = (minZ + maxZ) / 2
	ex = (maxX - minX) / 2
	ez = (maxZ - minZ) / 2
	return
}

// pickClusterAnchor picks the member with the smallest global BFS
// distance from the layout's global anchor. Ties go to the lowest
// key (members slice is pre-sorted by key, so a stable scan
// suffices). Falls back to the lowest-keyed member when every
// candidate is unreachable.
func pickClusterAnchor(members []nodeKey, distances map[nodeKey]int) nodeKey {
	var picked nodeKey
	bestDist := int(^uint(0) >> 1) // MaxInt
	for _, k := range members {
		d, ok := distances[k]
		if !ok {
			d = int(^uint(0) >> 1)
		}
		if d < bestDist {
			bestDist = d
			picked = k
		}
	}
	if (picked == nodeKey{}) && len(members) > 0 {
		picked = members[0]
	}
	return picked
}

// =============================================================================
// Helpers
// =============================================================================

// globalBFSDistances runs an undirected BFS from anchor across all
// meta-types and returns distance per node. Unreachable nodes are
// absent from the result map (the caller treats absent as +infinity).
func globalBFSDistances(rs *ResultSet, anchor nodeKey) map[nodeKey]int {
	nbrs := buildUndirectedAdjacency(rs)
	out := map[nodeKey]int{anchor: 0}
	queue := []nodeKey{anchor}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		d := out[k]
		for _, nb := range nbrs[k] {
			if _, seen := out[nb]; seen {
				continue
			}
			out[nb] = d + 1
			queue = append(queue, nb)
		}
	}
	return out
}

// groupByLayer buckets every node by its primary context. Nodes whose
// Contexts slice is empty fall into the synthetic UntaggedContextID
// bucket.
func groupByLayer(rs *ResultSet) map[string][]nodeKey {
	out := make(map[string][]nodeKey)
	for _, n := range rs.Nodes {
		ctx := UntaggedContextID
		if len(n.Contexts) > 0 && n.Contexts[0] != "" {
			ctx = n.Contexts[0]
		}
		out[ctx] = append(out[ctx], n.Key)
	}
	return out
}

// gridCell is an integer (x, z) position on the per-layer grid. Cells
// are translated to world coordinates by multiplying by GridStep.
type gridCell struct {
	X int
	Z int
}

// placeOrthogonalCluster returns the (x, z) grid offsets for every
// member of one context cluster, plus the Chebyshev radius of the
// bounding box (used by callers to size cluster slots in extended
// mode). The cluster anchor lands at (0, 0); other members fan out
// via greedy BFS over intra-cluster edges, snapping each new node to
// the closest free Chebyshev-ring cell to its parent.
//
// Caller responsibilities:
//   - `members` MUST already be key-sorted (so disconnected fallback
//     is deterministic).
//   - World coordinate translation (multiplying by GridStep, picking Y,
//     and translating between cluster slots in extended mode) is up
//     to the caller.
//
// Pure / deterministic.
func placeOrthogonalCluster(rs *ResultSet, members []nodeKey, clusterAnchor nodeKey) (map[nodeKey]gridCell, int) {
	if len(members) == 0 {
		return nil, 0
	}

	memberSet := make(map[nodeKey]struct{}, len(members))
	for _, k := range members {
		memberSet[k] = struct{}{}
	}

	// Intra-cluster adjacency: undirected, restricted to edges whose
	// BOTH endpoints sit in this cluster.
	intra := make(map[nodeKey][]nodeKey, len(members))
	for _, k := range members {
		intra[k] = nil
	}
	for _, e := range rs.Edges {
		_, sOk := memberSet[e.Src]
		_, dOk := memberSet[e.Dst]
		if !sOk || !dOk {
			continue
		}
		intra[e.Src] = append(intra[e.Src], e.Dst)
		intra[e.Dst] = append(intra[e.Dst], e.Src)
	}
	for k, list := range intra {
		uniq := make(map[nodeKey]struct{}, len(list))
		out := list[:0]
		for _, n := range list {
			if _, ok := uniq[n]; ok {
				continue
			}
			uniq[n] = struct{}{}
			out = append(out, n)
		}
		sort.Slice(out, func(i, j int) bool { return keyLess(out[i], out[j]) })
		intra[k] = out
	}

	// Greedy BFS grid placement.
	cellOf := map[nodeKey]gridCell{clusterAnchor: {0, 0}}
	taken := map[gridCell]struct{}{{0, 0}: {}}
	queue := []nodeKey{clusterAnchor}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		pc := cellOf[parent]
		for _, child := range intra[parent] {
			if _, ok := cellOf[child]; ok {
				continue
			}
			cc := findFreeCell(pc, taken)
			cellOf[child] = cc
			taken[cc] = struct{}{}
			queue = append(queue, child)
		}
	}

	// Disconnected members: scan an outward spiral from the cluster
	// origin and grab the first free cells in deterministic key order.
	unreached := []nodeKey{}
	for _, k := range members {
		if _, ok := cellOf[k]; !ok {
			unreached = append(unreached, k)
		}
	}
	if len(unreached) > 0 {
		sort.Slice(unreached, func(i, j int) bool { return keyLess(unreached[i], unreached[j]) })
		for _, k := range unreached {
			c := findFreeCell(gridCell{0, 0}, taken)
			cellOf[k] = c
			taken[c] = struct{}{}
		}
	}

	// Compute the cluster's Chebyshev radius (max |x| or |z| across all cells).
	radius := 0
	for _, c := range cellOf {
		if c.X < 0 {
			if -c.X > radius {
				radius = -c.X
			}
		} else if c.X > radius {
			radius = c.X
		}
		if c.Z < 0 {
			if -c.Z > radius {
				radius = -c.Z
			}
		} else if c.Z > radius {
			radius = c.Z
		}
	}

	return cellOf, radius
}

// findFreeCell walks Chebyshev rings outward from `center` and returns
// the first cell not present in `taken`. Ring r has 8r cells; within a
// ring we visit in a deterministic CCW order starting from (+r, 0).
// Used both to place a child near its parent and to spill orphans
// outward from the global origin.
func findFreeCell(center gridCell, taken map[gridCell]struct{}) gridCell {
	// First try center itself in case it's still free (used only for
	// the very first anchor placement; cheap regardless).
	if _, ok := taken[center]; !ok {
		return center
	}
	for r := 1; r < 1024; r++ {
		for _, off := range chebyshevRing(r) {
			c := gridCell{X: center.X + off.X, Z: center.Z + off.Z}
			if _, ok := taken[c]; !ok {
				return c
			}
		}
	}
	// Practically unreachable for any sane result size (< 500 nodes).
	// Return a sentinel so we never silently overwrite.
	return gridCell{X: 1 << 20, Z: 1 << 20}
}

// chebyshevRing returns the offsets of every cell at Chebyshev
// distance exactly r from the origin, in CCW order starting at (+r, 0).
// Ring r has 8r cells; cached so repeated calls cost O(1).
var ringCache = map[int][]gridCell{}

func chebyshevRing(r int) []gridCell {
	if cached, ok := ringCache[r]; ok {
		return cached
	}
	if r == 0 {
		ringCache[0] = []gridCell{{0, 0}}
		return ringCache[0]
	}
	out := make([]gridCell, 0, 8*r)
	// Walk the square perimeter: bottom edge → right edge → top edge → left edge.
	// CCW order on the (X, Z) plane treating +Z as "up".
	for x := -r; x <= r; x++ { // bottom edge, z = -r
		out = append(out, gridCell{X: x, Z: -r})
	}
	for z := -r + 1; z <= r; z++ { // right edge, x = +r
		out = append(out, gridCell{X: r, Z: z})
	}
	for x := r - 1; x >= -r; x-- { // top edge, z = +r
		out = append(out, gridCell{X: x, Z: r})
	}
	for z := r - 1; z >= -r+1; z-- { // left edge, x = -r
		out = append(out, gridCell{X: -r, Z: z})
	}
	ringCache[r] = out
	return out
}

// computeBounds returns the axis-aligned bounding box of every node's
// position. The frontend uses this for fit-to-result framing.
func computeBounds(pass *LayoutPass) LayoutBounds {
	if len(pass.Positions) == 0 {
		return LayoutBounds{}
	}
	first := true
	var b LayoutBounds
	for _, p := range pass.Positions {
		if first {
			b = LayoutBounds{
				MinX: p.X, MaxX: p.X,
				MinY: p.Y, MaxY: p.Y,
				MinZ: p.Z, MaxZ: p.Z,
			}
			first = false
			continue
		}
		if p.X < b.MinX {
			b.MinX = p.X
		}
		if p.X > b.MaxX {
			b.MaxX = p.X
		}
		if p.Y < b.MinY {
			b.MinY = p.Y
		}
		if p.Y > b.MaxY {
			b.MaxY = p.Y
		}
		if p.Z < b.MinZ {
			b.MinZ = p.Z
		}
		if p.Z > b.MaxZ {
			b.MaxZ = p.Z
		}
	}
	return b
}

func elapsedMs(start time.Time) float32 {
	return float32(time.Since(start).Microseconds()) / 1000.0
}
