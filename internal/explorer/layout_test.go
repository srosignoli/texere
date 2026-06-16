package explorer

import (
	"math"
	"reflect"
	"sort"
	"testing"
	"time"

	"texere/internal/search"
)

// =============================================================================
// Test fixtures
// =============================================================================

// makeKey builds a deterministic nodeKey for tests.
func makeKey(name string) nodeKey {
	return nodeKey{Chapter: "t", Text: name}
}

// makeRS constructs a ResultSet directly from explicit nodes and edges.
// Nodes default to nil Contexts (Untagged). Set rs.Nodes[i].Contexts in
// the test if you want them on a specific layer.
func makeRS(nodes []string, edges []ResultEdge) *ResultSet {
	rs := &ResultSet{
		ID:        "test",
		Nodes:     make([]ResultNode, 0, len(nodes)),
		Edges:     edges,
		NodeIndex: make(map[nodeKey]int),
		BuiltAt:   time.Unix(0, 0),
	}
	for _, name := range nodes {
		k := makeKey(name)
		rs.Nodes = append(rs.Nodes, ResultNode{Key: k, Text: name, Chapter: "t"})
	}
	sort.Slice(rs.Nodes, func(i, j int) bool { return keyLess(rs.Nodes[i].Key, rs.Nodes[j].Key) })
	for i, n := range rs.Nodes {
		rs.NodeIndex[n.Key] = i
	}
	for _, e := range edges {
		if idx, ok := rs.NodeIndex[e.Dst]; ok {
			rs.Nodes[idx].InDegree++
		}
	}
	return rs
}

// setContexts assigns context tags to nodes by name. Nodes not listed
// keep their default (nil → Untagged layer).
func setContexts(rs *ResultSet, byName map[string][]string) {
	for i, n := range rs.Nodes {
		if ctxs, ok := byName[n.Text]; ok {
			rs.Nodes[i].Contexts = ctxs
		}
	}
}

// =============================================================================
// Layered layout: same-context nodes share Y
// =============================================================================

func TestLayoutFromAnchor_SameContextSameY(t *testing.T) {
	anchor := makeKey("a")
	rs := makeRS(
		[]string{"a", "b", "c", "d"},
		[]ResultEdge{
			{Src: anchor, Dst: makeKey("b"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: anchor, Dst: makeKey("c"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: anchor, Dst: makeKey("d"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
		},
	)
	// a, b on layer "alpha"; c, d on layer "beta".
	setContexts(rs, map[string][]string{
		"a": {"alpha"},
		"b": {"alpha"},
		"c": {"beta"},
		"d": {"beta"},
	})

	pass := LayoutFromAnchor(rs, anchor, "")

	yA := pass.Positions[makeKey("a")].Y
	yB := pass.Positions[makeKey("b")].Y
	yC := pass.Positions[makeKey("c")].Y
	yD := pass.Positions[makeKey("d")].Y
	if !approxEqual(yA, yB) {
		t.Fatalf("a and b share context alpha — Y must match: %g vs %g", yA, yB)
	}
	if !approxEqual(yC, yD) {
		t.Fatalf("c and d share context beta — Y must match: %g vs %g", yC, yD)
	}
	if approxEqual(yA, yC) {
		t.Fatalf("alpha and beta are different contexts — Y must differ: alpha=%g beta=%g", yA, yC)
	}
}

// =============================================================================
// Layered layout: Untagged layer pinned at Y=0
// =============================================================================

func TestLayoutFromAnchor_UntaggedLayerAtBottom(t *testing.T) {
	anchor := makeKey("a")
	rs := makeRS(
		[]string{"a", "b", "c"},
		[]ResultEdge{
			{Src: anchor, Dst: makeKey("b"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: anchor, Dst: makeKey("c"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
		},
	)
	// a, c tagged; b untagged.
	setContexts(rs, map[string][]string{
		"a": {"zeta"}, // alphabetically last
		"c": {"alpha"},
	})

	pass := LayoutFromAnchor(rs, anchor, "")

	// Untagged ("") is at layer 0 → Y = 0.
	if !approxEqual(pass.Positions[makeKey("b")].Y, 0) {
		t.Fatalf("untagged node b must be at Y=0, got %g", pass.Positions[makeKey("b")].Y)
	}
	// "alpha" at layer 1, "zeta" at layer 2 in alphabetical order
	// (untagged pinned at 0).
	if !approxEqual(pass.Positions[makeKey("c")].Y, LayerSpacing) {
		t.Fatalf("alpha node c expected Y=%g, got %g", LayerSpacing, pass.Positions[makeKey("c")].Y)
	}
	if !approxEqual(pass.Positions[makeKey("a")].Y, 2*LayerSpacing) {
		t.Fatalf("zeta node a expected Y=%g, got %g", 2*LayerSpacing, pass.Positions[makeKey("a")].Y)
	}

	// Layers slice mirrors the same order.
	if len(pass.Layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(pass.Layers))
	}
	if pass.Layers[0].Context != UntaggedContextID {
		t.Fatalf("layer 0 must be Untagged, got %q", pass.Layers[0].Context)
	}
	if pass.Layers[0].Label != UntaggedLayerLabel {
		t.Fatalf("layer 0 label must be %q, got %q", UntaggedLayerLabel, pass.Layers[0].Label)
	}
	if pass.Layers[1].Context != "alpha" {
		t.Fatalf("layer 1 must be alpha, got %q", pass.Layers[1].Context)
	}
	if pass.Layers[2].Context != "zeta" {
		t.Fatalf("layer 2 must be zeta, got %q", pass.Layers[2].Context)
	}
}

// =============================================================================
// Layered layout: anchor at its layer's local origin
// =============================================================================

func TestLayoutFromAnchor_AnchorAtLayerOrigin(t *testing.T) {
	anchor := makeKey("a")
	rs := makeRS([]string{"a", "b"}, nil)
	setContexts(rs, map[string][]string{
		"a": {"foo"},
		"b": {"bar"},
	})

	pass := LayoutFromAnchor(rs, anchor, "")

	// Anchor sits at (0, layerY, 0) — only Y differs from origin.
	p := pass.Positions[anchor]
	if !approxEqual(p.X, 0) || !approxEqual(p.Z, 0) {
		t.Fatalf("anchor must be at (0, y, 0) on its layer, got %+v", p)
	}
}

// =============================================================================
// Determinism — repeated calls produce byte-identical output
// =============================================================================

func TestLayoutFromAnchor_Determinism(t *testing.T) {
	rs := buildSampleResultSet()
	anchor := rs.Nodes[0].Key

	first := LayoutFromAnchor(rs, anchor, "")
	for i := 0; i < 100; i++ {
		got := LayoutFromAnchor(rs, anchor, "")
		if !reflect.DeepEqual(got.Positions, first.Positions) {
			t.Fatalf("iteration %d produced a different position map", i)
		}
		if !reflect.DeepEqual(got.Distances, first.Distances) {
			t.Fatalf("iteration %d produced a different distances map", i)
		}
		if !reflect.DeepEqual(got.CycleNodes, first.CycleNodes) {
			t.Fatalf("iteration %d produced a different cycle set", i)
		}
		if !reflect.DeepEqual(layerCtxs(got), layerCtxs(first)) {
			t.Fatalf("iteration %d produced a different layer order", i)
		}
	}
}

func layerCtxs(p *LayoutPass) []string {
	out := make([]string, len(p.Layers))
	for i, l := range p.Layers {
		out[i] = l.Context
	}
	return out
}

// buildSampleResultSet returns a 25-node fixture spanning four contexts
// and all four meta-types. Used by the determinism + bounds tests.
func buildSampleResultSet() *ResultSet {
	names := []string{"anchor"}
	edges := []ResultEdge{}
	anchor := makeKey("anchor")

	for i := 0; i < 5; i++ {
		c := makeKey(fmtName("c", i))
		names = append(names, fmtName("c", i))
		edges = append(edges, ResultEdge{Src: anchor, Dst: c, Meta: MetaContains, ArrowLong: "contains"})
		for j := 0; j < 2; j++ {
			gc := makeKey(fmtName("gc", i*10+j))
			names = append(names, fmtName("gc", i*10+j))
			edges = append(edges, ResultEdge{Src: c, Dst: gc, Meta: MetaContains, ArrowLong: "contains"})
		}
	}
	prev := anchor
	for i := 0; i < 4; i++ {
		s := makeKey(fmtName("s", i))
		names = append(names, fmtName("s", i))
		edges = append(edges, ResultEdge{Src: prev, Dst: s, Meta: MetaLeadsTo, ArrowLong: "leads to"})
		prev = s
	}
	for i := 0; i < 3; i++ {
		p := makeKey(fmtName("p", i))
		names = append(names, fmtName("p", i))
		edges = append(edges, ResultEdge{Src: anchor, Dst: p, Meta: MetaProperties, ArrowLong: "has"})
	}
	for i := 0; i < 2; i++ {
		a := makeKey(fmtName("a", i))
		names = append(names, fmtName("a", i))
		edges = append(edges, ResultEdge{Src: anchor, Dst: a, Meta: MetaSimilarTo, ArrowLong: "similar to"})
	}
	rs := makeRS(names, edges)

	// Distribute the 25 nodes across four contexts so the determinism
	// test exercises multi-layer layout.
	contexts := []string{"alpha", "beta", "gamma", "delta"}
	for i := range rs.Nodes {
		rs.Nodes[i].Contexts = []string{contexts[i%len(contexts)]}
	}
	return rs
}

func fmtName(prefix string, i int) string {
	if i < 10 {
		return prefix + "0" + string(rune('0'+i))
	}
	return prefix + string(rune('0'+i/10)) + string(rune('0'+i%10))
}

// =============================================================================
// Cross-layer edges have endpoints on different Ys
// =============================================================================

func TestLayoutFromAnchor_CrossLayerEdgeEndpointsDifferentY(t *testing.T) {
	anchor := makeKey("a")
	rs := makeRS(
		[]string{"a", "b"},
		[]ResultEdge{
			{Src: anchor, Dst: makeKey("b"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
		},
	)
	setContexts(rs, map[string][]string{
		"a": {"alpha"},
		"b": {"beta"},
	})
	pass := LayoutFromAnchor(rs, anchor, "")
	yA := pass.Positions[anchor].Y
	yB := pass.Positions[makeKey("b")].Y
	if approxEqual(yA, yB) {
		t.Fatalf("cross-layer edge endpoints must have different Y, got %g == %g", yA, yB)
	}
}

// =============================================================================
// Edge cases
// =============================================================================

func TestLayoutFromAnchor_EmptyResultSet(t *testing.T) {
	rs := &ResultSet{ID: "empty", NodeIndex: map[nodeKey]int{}}
	pass := LayoutFromAnchor(rs, makeKey("missing"), "")
	if len(pass.Positions) != 0 {
		t.Fatalf("expected no positions, got %d", len(pass.Positions))
	}
}

func TestLayoutFromAnchor_SingleNodeResult(t *testing.T) {
	anchor := makeKey("solo")
	rs := makeRS([]string{"solo"}, nil)

	pass := LayoutFromAnchor(rs, anchor, "")
	// Solo node is untagged → layer 0 → Y = 0.
	if pass.Positions[anchor] != (Vec3{0, 0, 0}) {
		t.Fatalf("solo anchor must be at origin, got %+v", pass.Positions[anchor])
	}
	if d, ok := pass.Distances[anchor]; !ok || d != 0 {
		t.Fatalf("anchor distance must be 0, got %v", pass.Distances)
	}
}

func TestLayoutFromAnchor_DisconnectedNodeOnSameLayerOnGrid(t *testing.T) {
	anchor := makeKey("a")
	rs := makeRS(
		[]string{"a", "b", "orphan"},
		[]ResultEdge{
			{Src: anchor, Dst: makeKey("b"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
			// "orphan" has no edges
		},
	)
	setContexts(rs, map[string][]string{
		"a":      {"shared"},
		"b":      {"shared"},
		"orphan": {"shared"},
	})

	pass := LayoutFromAnchor(rs, anchor, "")

	pA := pass.Positions[anchor]
	pB := pass.Positions[makeKey("b")]
	pO := pass.Positions[makeKey("orphan")]

	// All three nodes share Y (same layer).
	if !approxEqual(pA.Y, pB.Y) || !approxEqual(pA.Y, pO.Y) {
		t.Fatalf("members of one context share Y: a=%g b=%g orphan=%g", pA.Y, pB.Y, pO.Y)
	}

	// Each node lands on a distinct grid cell — orthogonal placement
	// guarantees no overlap.
	if pA == pB || pA == pO || pB == pO {
		t.Fatalf("nodes must occupy distinct grid cells, got a=%+v b=%+v orphan=%+v", pA, pB, pO)
	}

	// Every node sits on the GridStep grid (X and Z are integer multiples).
	for name, p := range map[string]Vec3{"a": pA, "b": pB, "orphan": pO} {
		if math.Abs(math.Mod(float64(p.X)/float64(GridStep), 1)) > 1e-3 {
			t.Fatalf("%s.X (%g) not on GridStep grid", name, p.X)
		}
		if math.Abs(math.Mod(float64(p.Z)/float64(GridStep), 1)) > 1e-3 {
			t.Fatalf("%s.Z (%g) not on GridStep grid", name, p.Z)
		}
	}
}

// =============================================================================
// Extended-mode layout: every node at Y=0, contexts in clusters
// =============================================================================

func TestLayoutFromAnchor_Extended_AllNodesAtY0(t *testing.T) {
	anchor := makeKey("a")
	rs := makeRS(
		[]string{"a", "b", "c", "d"},
		[]ResultEdge{
			{Src: anchor, Dst: makeKey("b"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: anchor, Dst: makeKey("c"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
		},
	)
	setContexts(rs, map[string][]string{
		"a": {"foo"},
		"b": {"bar"},
		"c": {"baz"},
		// d untagged → its own cluster
	})

	pass := LayoutFromAnchor(rs, anchor, LayoutModeExtended)

	for k, p := range pass.Positions {
		if !approxEqual(p.Y, 0) {
			t.Fatalf("extended mode: %v at non-zero Y %g", k, p.Y)
		}
	}
}

func TestLayoutFromAnchor_Extended_AnchorAtMapCenter(t *testing.T) {
	// Post-2D spiral redesign: the anchor's cluster sits at the map
	// centre (0, 0) and other clusters radiate outward through
	// Chebyshev rings. With N=3 contexts, bar and baz fill the first
	// two cells of ring 1 (at distinct (X, Z) cells, not on the same
	// row).
	anchor := makeKey("a")
	rs := makeRS([]string{"a", "b", "c"}, nil)
	setContexts(rs, map[string][]string{
		"a": {"foo"},
		"b": {"bar"},
		"c": {"baz"},
	})

	pass := LayoutFromAnchor(rs, anchor, LayoutModeExtended)
	pA := pass.Positions[anchor]
	if !approxEqual(pA.X, 0) || !approxEqual(pA.Z, 0) {
		t.Fatalf("anchor's cluster must be at map centre (0, 0), got %+v", pA)
	}
	pB := pass.Positions[makeKey("b")]
	pC := pass.Positions[makeKey("c")]
	// Non-anchor clusters must NOT be at the map centre (either X or
	// Z non-zero — they live in ring 1 or later).
	if approxEqual(pB.X, 0) && approxEqual(pB.Z, 0) {
		t.Fatalf("non-anchor cluster b must be off-centre, got %+v", pB)
	}
	if approxEqual(pC.X, 0) && approxEqual(pC.Z, 0) {
		t.Fatalf("non-anchor cluster c must be off-centre, got %+v", pC)
	}
	// b and c land in distinct cells (slot 1 vs slot 2 of the spiral).
	if approxEqual(pB.X, pC.X) && approxEqual(pB.Z, pC.Z) {
		t.Fatalf("clusters b and c must occupy distinct slots, both at %+v", pB)
	}
}

func TestLayoutFromAnchor_Extended_SlotsSpread2D(t *testing.T) {
	// With 9 contexts (anchor + 8 = first full ring), the cluster
	// slots must span BOTH X and Z. This guards against any future
	// regression that collapses the slot row to a single axis.
	anchor := makeKey("a0")
	names := []string{"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"}
	rs := makeRS(names, nil)
	ctxFor := map[string]string{}
	for i, n := range names {
		ctxFor[n] = "ctx" + n
		_ = i
	}
	setContexts(rs, map[string][]string{
		"a0": {"anchor"},
		"a1": {"c1"}, "a2": {"c2"}, "a3": {"c3"}, "a4": {"c4"},
		"a5": {"c5"}, "a6": {"c6"}, "a7": {"c7"}, "a8": {"c8"},
	})

	pass := LayoutFromAnchor(rs, anchor, LayoutModeExtended)

	var (
		anyNonZeroX = false
		anyNonZeroZ = false
		seen        = map[[2]float32]nodeKey{}
	)
	for _, n := range rs.Nodes {
		p := pass.Positions[n.Key]
		if !approxEqual(p.X, 0) {
			anyNonZeroX = true
		}
		if !approxEqual(p.Z, 0) {
			anyNonZeroZ = true
		}
		key := [2]float32{p.X, p.Z}
		if prev, dup := seen[key]; dup {
			t.Fatalf("nodes %v and %v collide at %+v", prev, n.Key, p)
		}
		seen[key] = n.Key
	}
	if !anyNonZeroX {
		t.Fatalf("extended slots collapsed to Z axis — no node has X != 0")
	}
	if !anyNonZeroZ {
		t.Fatalf("extended slots collapsed to X axis — no node has Z != 0")
	}
}

func TestLayoutFromAnchor_Extended_DistinctCellsAcrossContexts(t *testing.T) {
	// Two contexts, each with the same number of nodes. After clustering,
	// no two nodes should share a (x, z) cell — the slot pitch is sized
	// to keep clusters apart.
	anchor := makeKey("a")
	rs := makeRS([]string{"a", "b", "c", "d"}, nil)
	setContexts(rs, map[string][]string{
		"a": {"alpha"},
		"b": {"alpha"},
		"c": {"beta"},
		"d": {"beta"},
	})

	pass := LayoutFromAnchor(rs, anchor, LayoutModeExtended)
	seen := map[[2]float32]nodeKey{}
	for k, p := range pass.Positions {
		key := [2]float32{p.X, p.Z}
		if prev, ok := seen[key]; ok {
			t.Fatalf("extended mode collision at %+v: %v and %v", p, prev, k)
		}
		seen[key] = k
	}
}

func TestLayoutFromAnchor_Extended_Determinism(t *testing.T) {
	rs := buildSampleResultSet()
	anchor := rs.Nodes[0].Key

	first := LayoutFromAnchor(rs, anchor, LayoutModeExtended)
	for i := 0; i < 50; i++ {
		got := LayoutFromAnchor(rs, anchor, LayoutModeExtended)
		if !reflect.DeepEqual(got.Positions, first.Positions) {
			t.Fatalf("extended mode iteration %d produced a different position map", i)
		}
	}
}

// =============================================================================
// Bounds + helpers
// =============================================================================

// =============================================================================
// Flow layout: directed cone reads left-to-right
// =============================================================================

// TestLayoutFromAnchor_Flow_DepthDrivesX verifies that in Flow mode a node's X
// is its directed hop distance from the root: a straight chain a→b→c→d marches
// monotonically along +X, one GridStep per hop, all on a single Z lane.
func TestLayoutFromAnchor_Flow_DepthDrivesX(t *testing.T) {
	root := makeKey("a")
	rs := makeRS(
		[]string{"a", "b", "c", "d"},
		[]ResultEdge{
			{Src: makeKey("a"), Dst: makeKey("b"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: makeKey("b"), Dst: makeKey("c"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: makeKey("c"), Dst: makeKey("d"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
		},
	)

	pass := LayoutFromAnchor(rs, root, LayoutModeFlow)

	xs := []float32{
		pass.Positions[makeKey("a")].X,
		pass.Positions[makeKey("b")].X,
		pass.Positions[makeKey("c")].X,
		pass.Positions[makeKey("d")].X,
	}
	for i := 1; i < len(xs); i++ {
		if xs[i]-xs[i-1] != GridStep {
			t.Fatalf("hop %d: X must advance one GridStep (%g); got %g→%g", i, GridStep, xs[i-1], xs[i])
		}
	}
	// A single unbranched chain stays on one Z lane.
	z0 := pass.Positions[makeKey("a")].Z
	for _, name := range []string{"b", "c", "d"} {
		if pass.Positions[makeKey(name)].Z != z0 {
			t.Errorf("node %q: straight chain must share one Z lane (%g); got %g", name, z0, pass.Positions[makeKey(name)].Z)
		}
	}
}

// TestLayoutFromAnchor_Flow_BranchesGetSeparateLanes verifies that when the
// root forks, the two children land in the same X column (same depth) but on
// distinct Z lanes so the branches don't overlap.
func TestLayoutFromAnchor_Flow_BranchesGetSeparateLanes(t *testing.T) {
	root := makeKey("a")
	rs := makeRS(
		[]string{"a", "b", "c"},
		[]ResultEdge{
			{Src: makeKey("a"), Dst: makeKey("b"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: makeKey("a"), Dst: makeKey("c"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
		},
	)

	pass := LayoutFromAnchor(rs, root, LayoutModeFlow)

	if pass.Positions[makeKey("b")].X != pass.Positions[makeKey("c")].X {
		t.Errorf("siblings b,c must share depth column X; got %g vs %g",
			pass.Positions[makeKey("b")].X, pass.Positions[makeKey("c")].X)
	}
	if pass.Positions[makeKey("b")].Z == pass.Positions[makeKey("c")].Z {
		t.Errorf("siblings b,c must occupy distinct Z lanes; both at %g", pass.Positions[makeKey("b")].Z)
	}
}

// TestLayoutFromAnchor_Flow_DisconnectedComponentsSeparateRows verifies that
// two disconnected cones (a→b and c→d) each get their root on the left (X=0) and
// occupy non-overlapping Z bands, so they read as separate rows rather than
// blurring onto one line.
func TestLayoutFromAnchor_Flow_DisconnectedComponentsSeparateRows(t *testing.T) {
	rs := makeRS(
		[]string{"a", "b", "c", "d"},
		[]ResultEdge{
			{Src: makeKey("a"), Dst: makeKey("b"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: makeKey("c"), Dst: makeKey("d"), Meta: MetaLeadsTo, ArrowLong: "leads to"},
		},
	)

	pass := LayoutFromAnchor(rs, makeKey("a"), LayoutModeFlow)

	// Both roots line up on the left edge.
	if x := pass.Positions[makeKey("a")].X; x != 0 {
		t.Errorf("root a must be at X=0; got %g", x)
	}
	if x := pass.Positions[makeKey("c")].X; x != 0 {
		t.Errorf("root c must be at X=0; got %g", x)
	}

	// The two components must not overlap in Z. Component {a,b} and {c,d} should
	// occupy disjoint Z ranges.
	zAB := []float32{pass.Positions[makeKey("a")].Z, pass.Positions[makeKey("b")].Z}
	zCD := []float32{pass.Positions[makeKey("c")].Z, pass.Positions[makeKey("d")].Z}
	maxAB := maxF(zAB)
	minCD := minF(zCD)
	maxCD := maxF(zCD)
	minAB := minF(zAB)
	overlap := maxAB >= minCD && maxCD >= minAB
	if overlap {
		t.Errorf("disconnected components overlap in Z: {a,b}=%v {c,d}=%v", zAB, zCD)
	}
}

func maxF(xs []float32) float32 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func minF(xs []float32) float32 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

// TestBuildResultSet_PathLikeDetection asserts a pure cone (only "path" chains)
// is flagged PathLike, while a result mixing path chains with orbit nodes is
// not (so it keeps the default layered layout).
func TestBuildResultSet_PathLikeDetection(t *testing.T) {
	coneOnly := &search.SearchResult{
		Structural: []search.StructuralChain{{
			Kind: "path",
			Nodes: []search.ChainNode{
				{NodeText: "start", ArrowCode: "lt", ArrowLong: "leads to"},
				{NodeText: "goal"},
			},
		}},
	}
	if rs := BuildResultSet(coneOnly, nil, `\from start`); !rs.PathLike {
		t.Errorf("pure path cone must be PathLike")
	}

	mixed := &search.SearchResult{
		Nodes: []search.GraphNode{{NodeText: "x"}},
		Structural: []search.StructuralChain{{
			Kind:  "path",
			Nodes: []search.ChainNode{{NodeText: "start"}, {NodeText: "goal"}},
		}},
	}
	if rs := BuildResultSet(mixed, nil, `mixed`); rs.PathLike {
		t.Errorf("mixed orbit+path result must NOT be PathLike")
	}

	seq := &search.SearchResult{
		Structural: []search.StructuralChain{{
			Kind:  "sequence",
			Nodes: []search.ChainNode{{NodeText: "s1"}, {NodeText: "s2"}},
		}},
	}
	if rs := BuildResultSet(seq, nil, `\sequence`); rs.PathLike {
		t.Errorf("sequence result must NOT be PathLike (renders as timeline)")
	}
}

func TestComputeBounds_SingleNodeAtOrigin(t *testing.T) {
	pass := &LayoutPass{
		Positions: map[nodeKey]Vec3{
			makeKey("a"): {0, 0, 0},
		},
	}
	b := computeBounds(pass)
	want := LayoutBounds{}
	if b != want {
		t.Fatalf("expected zero bounds for single node at origin, got %+v", b)
	}
}

func TestComputeBounds_RectangularSpread(t *testing.T) {
	pass := &LayoutPass{
		Positions: map[nodeKey]Vec3{
			makeKey("a"): {-2, 0, -1},
			makeKey("b"): {3, 5, 4},
			makeKey("c"): {0, -1, 0},
		},
	}
	b := computeBounds(pass)
	want := LayoutBounds{MinX: -2, MaxX: 3, MinY: -1, MaxY: 5, MinZ: -1, MaxZ: 4}
	if b != want {
		t.Fatalf("bounds mismatch\n  got: %+v\n  want: %+v", b, want)
	}
}

func approxEqual(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-3
}
