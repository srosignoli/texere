package explorer

import (
	"testing"

	"texere/internal/content"
	"texere/internal/search"
)

// TestBuildResultSet_FromNotesTier verifies that `\notes <chapter>` data
// (the FullNote → NoteBlock → NoteArrow tree) is converted into
// ResultNodes + ResultEdges by BuildResultSet. This is the launcher's
// primary data path: `\chapter` alone produces a TOC listing with no
// graph topology, but `\notes` returns block-level edges we can lay out.
func TestBuildResultSet_FromNotesTier(t *testing.T) {
	reg := &content.SSTRegistry{
		Arrows: map[string]content.ArrowDef{
			"contain": {Code: "contain", LongName: "contains", MetaType: "contains"},
			"freq":    {Code: "freq", LongName: "has frequency", MetaType: "properties"},
			"next":    {Code: "next", LongName: "leads to", MetaType: "leadsto"},
		},
	}
	sr := &search.SearchResult{
		Notes: []search.FullNote{
			{
				Chapter: "brain",
				Blocks: []search.NoteBlock{
					{
						NodeText: "neuron",
						Line:     5,
						Context:  "biology, brain",
						Arrows: []search.NoteArrow{
							{ArrowLong: "contains", ArrowCode: "contain", Target: "axon"},
							{ArrowLong: "has frequency", ArrowCode: "freq", Target: "10Hz"},
							{ArrowLong: "leads to", ArrowCode: "next", Target: "synapse"},
						},
					},
					{
						NodeText: "synapse",
						Line:     12,
						Context:  "biology",
						Arrows: []search.NoteArrow{
							{ArrowLong: "contains", ArrowCode: "contain", Target: "vesicle"},
						},
					},
				},
			},
		},
	}

	rs := BuildResultSet(sr, reg, "\\notes brain")

	// All authored nodes (root + arrow targets, deduped) appear in the
	// result set.
	wantNodes := []string{"10Hz", "axon", "neuron", "synapse", "vesicle"}
	if len(rs.Nodes) != len(wantNodes) {
		t.Fatalf("expected %d nodes, got %d (%+v)", len(wantNodes), len(rs.Nodes), nodeTexts(rs))
	}
	for i, want := range wantNodes {
		if rs.Nodes[i].Text != want {
			t.Fatalf("node[%d] text mismatch — got %q, want %q (full: %+v)",
				i, rs.Nodes[i].Text, want, nodeTexts(rs))
		}
	}

	// Edges: 4 authored arrows (3 from neuron + 1 from synapse), each
	// with the correct meta-type. Dedupe should NOT collapse any since
	// every edge connects a distinct (Src, Dst, meta) triple.
	if len(rs.Edges) != 4 {
		t.Fatalf("expected 4 edges, got %d (%+v)", len(rs.Edges), rs.Edges)
	}

	metaCount := map[MetaType]int{}
	for _, e := range rs.Edges {
		metaCount[e.Meta]++
	}
	if metaCount[MetaContains] != 2 {
		t.Fatalf("expected 2 contains edges, got %d", metaCount[MetaContains])
	}
	if metaCount[MetaProperties] != 1 {
		t.Fatalf("expected 1 properties edge, got %d", metaCount[MetaProperties])
	}
	if metaCount[MetaLeadsTo] != 1 {
		t.Fatalf("expected 1 leadsto edge, got %d", metaCount[MetaLeadsTo])
	}

	// In-degree: axon, vesicle, 10Hz, synapse each have exactly 1
	// incoming edge; neuron has 0.
	for _, want := range []struct {
		text string
		in   int
	}{
		{"neuron", 0},
		{"axon", 1},
		{"10Hz", 1},
		{"synapse", 1},
		{"vesicle", 1},
	} {
		idx := -1
		for i, n := range rs.Nodes {
			if n.Text == want.text {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.Fatalf("node %q not found", want.text)
		}
		if rs.Nodes[idx].InDegree != want.in {
			t.Fatalf("node %q in-degree = %d, want %d", want.text, rs.Nodes[idx].InDegree, want.in)
		}
	}

	// Context tags: neuron should carry "biology, brain" parsed into a
	// 2-element slice.
	for _, n := range rs.Nodes {
		if n.Text != "neuron" {
			continue
		}
		if len(n.Contexts) != 2 || n.Contexts[0] != "biology" || n.Contexts[1] != "brain" {
			t.Fatalf("neuron contexts = %+v, want [biology brain]", n.Contexts)
		}
	}
}

// TestBuildResultSet_NotesAndNodesTiersCombine verifies that when the
// search result contains BOTH tiers (e.g. a chapter notes view that
// also returns related orbit nodes), BuildResultSet merges them
// without losing data and without duplicating shared nodes.
func TestBuildResultSet_NotesAndNodesTiersCombine(t *testing.T) {
	reg := &content.SSTRegistry{
		Arrows: map[string]content.ArrowDef{
			"contain": {Code: "contain", LongName: "contains", MetaType: "contains"},
		},
	}
	sr := &search.SearchResult{
		Nodes: []search.GraphNode{
			{
				NodeText:    "neuron",
				Chapter:     "brain",
				ContextTags: []string{"biology"},
				Orbit: []search.OrbitEntry{
					{ArrowLong: "contains", NeighborText: "axon", Radius: 1, Ctx: ""},
				},
			},
		},
		Notes: []search.FullNote{
			{
				Chapter: "brain",
				Blocks: []search.NoteBlock{
					{
						NodeText: "neuron",
						Line:     1,
						Context:  "biology",
						Arrows: []search.NoteArrow{
							{ArrowLong: "contains", ArrowCode: "contain", Target: "dendrite"},
						},
					},
				},
			},
		},
	}

	rs := BuildResultSet(sr, reg, "")

	// Three distinct nodes despite "neuron" appearing in both tiers.
	if len(rs.Nodes) != 3 {
		t.Fatalf("expected 3 nodes (neuron, axon, dendrite), got %d (%+v)", len(rs.Nodes), nodeTexts(rs))
	}
	// Two edges (neuron→axon from orbit, neuron→dendrite from notes).
	// Both have the same Src+meta but different Dst, so dedupe leaves
	// them alone.
	if len(rs.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d (%+v)", len(rs.Edges), rs.Edges)
	}
}

func nodeTexts(rs *ResultSet) []string {
	out := make([]string, 0, len(rs.Nodes))
	for _, n := range rs.Nodes {
		out = append(out, n.Text)
	}
	return out
}

// =============================================================================
// Context inheritance (Notes tier) — target nodes get the source block's
// context when they don't have an authored one of their own.
// =============================================================================

// findNode returns the result node with the given text, or nil if missing.
func findNode(rs *ResultSet, text string) *ResultNode {
	for i := range rs.Nodes {
		if rs.Nodes[i].Text == text {
			return &rs.Nodes[i]
		}
	}
	return nil
}

func TestBuildResultSet_TargetInheritsSourceBlockContext(t *testing.T) {
	// "alpha waves" sits in `brain waves`. The arrow `(freq) 5-15 Hz`
	// puts "5-15 Hz" in the graph with no authored context. Per the
	// inheritance rule, the target should join `brain waves` too.
	reg := &content.SSTRegistry{
		Arrows: map[string]content.ArrowDef{
			"freq": {Code: "freq", LongName: "has frequency", MetaType: "properties"},
		},
	}
	sr := &search.SearchResult{
		Notes: []search.FullNote{
			{
				Chapter: "brain",
				Blocks: []search.NoteBlock{
					{
						NodeText: "alpha waves",
						Context:  "brain waves",
						Arrows: []search.NoteArrow{
							{ArrowLong: "has frequency", ArrowCode: "freq", Target: "5-15 Hz"},
						},
					},
				},
			},
		},
	}

	rs := BuildResultSet(sr, reg, "")
	target := findNode(rs, "5-15 Hz")
	if target == nil {
		t.Fatalf("expected 5-15 Hz to appear in result, got %+v", nodeTexts(rs))
	}
	if len(target.Contexts) != 1 || target.Contexts[0] != "brain waves" {
		t.Fatalf("expected 5-15 Hz to inherit `brain waves`, got %+v", target.Contexts)
	}
}

func TestBuildResultSet_TargetAsBlockRootKeepsOwnContext(t *testing.T) {
	// "5-15 Hz" appears BOTH as an arrow target under `brain waves` AND
	// as a block root in its own `:: frequencies ::` section. The
	// authored root context must win regardless of which block iterates
	// first.
	reg := &content.SSTRegistry{
		Arrows: map[string]content.ArrowDef{
			"freq":    {Code: "freq", LongName: "has frequency", MetaType: "properties"},
			"contain": {Code: "contain", LongName: "contains", MetaType: "contains"},
		},
	}
	// Order A: target appears as arrow tail first, then as block root.
	srA := &search.SearchResult{
		Notes: []search.FullNote{
			{
				Chapter: "brain",
				Blocks: []search.NoteBlock{
					{
						NodeText: "alpha waves",
						Context:  "brain waves",
						Arrows: []search.NoteArrow{
							{ArrowLong: "has frequency", ArrowCode: "freq", Target: "5-15 Hz"},
						},
					},
					{
						NodeText: "5-15 Hz",
						Context:  "frequencies",
						Arrows: []search.NoteArrow{
							{ArrowLong: "contains", ArrowCode: "contain", Target: "13 Hz"},
						},
					},
				},
			},
		},
	}
	rs := BuildResultSet(srA, reg, "")
	target := findNode(rs, "5-15 Hz")
	if target == nil {
		t.Fatalf("5-15 Hz missing (order A): %+v", nodeTexts(rs))
	}
	if len(target.Contexts) != 1 || target.Contexts[0] != "frequencies" {
		t.Fatalf("order A: expected `frequencies` from block-root, got %+v", target.Contexts)
	}

	// Order B: target appears as block root first, then as arrow tail.
	srB := &search.SearchResult{
		Notes: []search.FullNote{
			{
				Chapter: "brain",
				Blocks: []search.NoteBlock{
					{
						NodeText: "5-15 Hz",
						Context:  "frequencies",
						Arrows: []search.NoteArrow{
							{ArrowLong: "contains", ArrowCode: "contain", Target: "13 Hz"},
						},
					},
					{
						NodeText: "alpha waves",
						Context:  "brain waves",
						Arrows: []search.NoteArrow{
							{ArrowLong: "has frequency", ArrowCode: "freq", Target: "5-15 Hz"},
						},
					},
				},
			},
		},
	}
	rs = BuildResultSet(srB, reg, "")
	target = findNode(rs, "5-15 Hz")
	if target == nil {
		t.Fatalf("5-15 Hz missing (order B): %+v", nodeTexts(rs))
	}
	if len(target.Contexts) != 1 || target.Contexts[0] != "frequencies" {
		t.Fatalf("order B: expected `frequencies` from block-root, got %+v", target.Contexts)
	}
}

func TestBuildResultSet_MultiSourceTargetFollowsFileOrder(t *testing.T) {
	// "5-15 Hz" is a target of both `alpha waves` (in `brain waves`)
	// and `power line` (in `electrical`). The first source in file
	// order wins.
	reg := &content.SSTRegistry{
		Arrows: map[string]content.ArrowDef{
			"freq": {Code: "freq", LongName: "has frequency", MetaType: "properties"},
		},
	}
	sr := &search.SearchResult{
		Notes: []search.FullNote{
			{
				Chapter: "brain",
				Blocks: []search.NoteBlock{
					{
						NodeText: "alpha waves",
						Context:  "brain waves",
						Arrows: []search.NoteArrow{
							{ArrowLong: "has frequency", ArrowCode: "freq", Target: "5-15 Hz"},
						},
					},
					{
						NodeText: "power line",
						Context:  "electrical",
						Arrows: []search.NoteArrow{
							{ArrowLong: "has frequency", ArrowCode: "freq", Target: "5-15 Hz"},
						},
					},
				},
			},
		},
	}
	rs := BuildResultSet(sr, reg, "")
	target := findNode(rs, "5-15 Hz")
	if target == nil {
		t.Fatalf("5-15 Hz missing: %+v", nodeTexts(rs))
	}
	if len(target.Contexts) != 1 || target.Contexts[0] != "brain waves" {
		t.Fatalf("first-source context expected `brain waves`, got %+v", target.Contexts)
	}
}

func TestBuildResultSet_ContextlessBlockStaysUntagged(t *testing.T) {
	// Block with no authored `:: context ::`: root and arrow target
	// both still land in Untagged (empty Contexts slice). Verifies the
	// inheritance fallback never invents a context out of thin air.
	reg := &content.SSTRegistry{
		Arrows: map[string]content.ArrowDef{
			"contain": {Code: "contain", LongName: "contains", MetaType: "contains"},
		},
	}
	sr := &search.SearchResult{
		Notes: []search.FullNote{
			{
				Chapter: "brain",
				Blocks: []search.NoteBlock{
					{
						NodeText: "lone root",
						Context:  "", // no :: context ::
						Arrows: []search.NoteArrow{
							{ArrowLong: "contains", ArrowCode: "contain", Target: "lone target"},
						},
					},
				},
			},
		},
	}
	rs := BuildResultSet(sr, reg, "")
	for _, name := range []string{"lone root", "lone target"} {
		n := findNode(rs, name)
		if n == nil {
			t.Fatalf("%q missing", name)
		}
		if len(n.Contexts) != 0 {
			t.Fatalf("%q expected empty contexts (Untagged), got %+v", name, n.Contexts)
		}
	}
}

// =============================================================================
// Context inheritance (Orbit tier) — empty OrbitEntry.Ctx inherits from
// the source GraphNode's primary tag.
// =============================================================================

func TestBuildResultSet_OrbitNeighborInheritsCtxOnEmpty(t *testing.T) {
	reg := &content.SSTRegistry{
		Arrows: map[string]content.ArrowDef{
			"contain": {Code: "contain", LongName: "contains", MetaType: "contains"},
		},
	}
	sr := &search.SearchResult{
		Nodes: []search.GraphNode{
			{
				NodeText:    "alpha waves",
				Chapter:     "brain",
				ContextTags: []string{"brain waves"},
				Orbit: []search.OrbitEntry{
					{ArrowLong: "contains", NeighborText: "5-15 Hz", Radius: 1, Ctx: ""},
					{ArrowLong: "contains", NeighborText: "spike", Radius: 1, Ctx: "neural"},
				},
			},
		},
	}
	rs := BuildResultSet(sr, reg, "")

	// 5-15 Hz had empty Ctx → inherits brain waves.
	hz := findNode(rs, "5-15 Hz")
	if hz == nil {
		t.Fatalf("5-15 Hz missing: %+v", nodeTexts(rs))
	}
	if len(hz.Contexts) != 1 || hz.Contexts[0] != "brain waves" {
		t.Fatalf("5-15 Hz expected inherited `brain waves`, got %+v", hz.Contexts)
	}

	// spike had its own Ctx → keeps `neural`.
	spike := findNode(rs, "spike")
	if spike == nil {
		t.Fatalf("spike missing: %+v", nodeTexts(rs))
	}
	if len(spike.Contexts) != 1 || spike.Contexts[0] != "neural" {
		t.Fatalf("spike expected its own `neural`, got %+v", spike.Contexts)
	}
}
