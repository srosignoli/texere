package explorer

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// T060: Layout performance benchmarks + 500-node wall-clock guard.
//
// The constitution's amended budget for a graph explorer query is a
// two-phase reveal: skeleton ≤16 ms (frame 0 — ground + territories +
// camera frame) and full settle ≤2 s. The Go-side layout pass is one
// component of frame 0, and is expected to run under ~100 ms even at
// the result-size cap (MaxNodes = 500). Beyond that the WebGL bring-up
// dominates, so widening the layout budget gains nothing.
//
// TestLayoutBudget500 enforces the 100 ms ceiling on CI so a future
// algorithm change (e.g. swapping linear sibling spread for a tree
// optimizer) can't quietly bust the budget. Benchmarks at 200 / 500 /
// 1500 nodes give us a profile across realistic sizes.
// =============================================================================

const layoutBudget500ms = 100 // wall-clock budget for 500-node layout

// buildBenchFixture constructs an n-node, edge-dense ResultSet with a
// mix of all four meta-types so the per-axis BFS, cycle detection,
// centrality, and territory passes all do real work. The structure is
// deterministic so repeat runs benchmark identical input.
//
//   - Node 0 is the anchor; every other node attaches via a rotating
//     meta-type so all four axes get populated.
//   - Each node n>0 also gets a back-edge to floor(n/3), producing a
//     dense connected graph (no orphans, no fallback-grid expansion).
//   - Every fifth node is tagged with one of three contexts so the
//     territory pass groups them realistically.
func buildBenchFixture(n int) *ResultSet {
	nodes := make([]string, n)
	for i := 0; i < n; i++ {
		nodes[i] = fmt.Sprintf("n%06d", i)
	}

	metas := []MetaType{MetaContains, MetaLeadsTo, MetaProperties, MetaSimilarTo}
	edges := make([]ResultEdge, 0, n*3)
	keyOf := func(i int) nodeKey { return makeKey(nodes[i]) }
	for i := 1; i < n; i++ {
		// Anchor-spoke edge: rotating meta-type so all four axes fire.
		edges = append(edges, ResultEdge{
			Src:       keyOf(0),
			Dst:       keyOf(i),
			Meta:      metas[i%4],
			ArrowLong: "bench",
		})
		// Back-edge to floor(i/3) so the graph is densely connected and
		// the cycle detector sees a non-trivial input.
		parent := i / 3
		if parent != 0 && parent != i {
			edges = append(edges, ResultEdge{
				Src:       keyOf(parent),
				Dst:       keyOf(i),
				Meta:      MetaContains,
				ArrowLong: "bench",
			})
		}
	}

	rs := makeRS(nodes, edges)
	contexts := []string{"alpha", "beta", "gamma"}
	for i := range rs.Nodes {
		if i%5 == 0 {
			rs.Nodes[i].Contexts = []string{contexts[i%3]}
		}
	}
	return rs
}

func BenchmarkLayout_200(b *testing.B) {
	rs := buildBenchFixture(200)
	anchor := rs.Nodes[0].Key
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LayoutFromAnchor(rs, anchor, "")
	}
}

func BenchmarkLayout_500(b *testing.B) {
	rs := buildBenchFixture(500)
	anchor := rs.Nodes[0].Key
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LayoutFromAnchor(rs, anchor, "")
	}
}

func BenchmarkLayout_1500(b *testing.B) {
	// 1500-node fixture exercises the algorithm well past the MaxNodes
	// cap (500) — the production code never sees inputs this large
	// because Open clamps first, but the benchmark records the
	// algorithmic curve so future changes can be evaluated against the
	// hot path's scaling behavior.
	rs := buildBenchFixture(1500)
	anchor := rs.Nodes[0].Key
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LayoutFromAnchor(rs, anchor, "")
	}
}

// TestLayoutBudget500 is the hard wall-clock guard: a 500-node layout
// MUST complete in under 100 ms on the CI test runner. If this fires,
// the algorithm has regressed and the frontend's "skeleton ≤16 ms"
// frame-0 budget is at risk. We run three trials and assert against
// the MEDIAN to absorb GC pauses / OS scheduling jitter without
// hiding a real regression.
func TestLayoutBudget500(t *testing.T) {
	rs := buildBenchFixture(500)
	anchor := rs.Nodes[0].Key

	const trials = 3
	durs := make([]time.Duration, trials)
	for i := 0; i < trials; i++ {
		start := time.Now()
		LayoutFromAnchor(rs, anchor, "")
		durs[i] = time.Since(start)
	}

	// median of 3 = the middle value after sorting.
	for i := 0; i < trials; i++ {
		for j := i + 1; j < trials; j++ {
			if durs[j] < durs[i] {
				durs[i], durs[j] = durs[j], durs[i]
			}
		}
	}
	median := durs[trials/2]

	if median > time.Duration(layoutBudget500ms)*time.Millisecond {
		t.Fatalf("LayoutFromAnchor(500 nodes) median=%s exceeds %dms budget (samples=%v)",
			median, layoutBudget500ms, durs)
	}
	t.Logf("LayoutFromAnchor(500) median=%s (samples=%v)", median, durs)
}
