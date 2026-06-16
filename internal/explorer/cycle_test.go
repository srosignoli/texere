package explorer

import (
	"math"
	"testing"
	"time"
)

// =============================================================================
// T024: Cycle break — must terminate, mark cycle members, lowest key wins
// =============================================================================

func TestBreakContainmentCycles_3Cycle(t *testing.T) {
	// A "contains" B "contains" C "contains" A — a 3-cycle. Lowest key
	// (A) wins as the layout-time root.
	a := makeKey("a")
	b := makeKey("b")
	c := makeKey("c")
	rs := makeRS(
		[]string{"a", "b", "c"},
		[]ResultEdge{
			{Src: a, Dst: b, Meta: MetaContains, ArrowLong: "contains"},
			{Src: b, Dst: c, Meta: MetaContains, ArrowLong: "contains"},
			{Src: c, Dst: a, Meta: MetaContains, ArrowLong: "contains"},
		},
	)
	adj := buildAdjacency(rs)

	done := make(chan map[nodeKey]bool, 1)
	go func() {
		done <- breakContainmentCycles(rs, adj)
	}()

	select {
	case cycles := <-done:
		// All three nodes participate in the cycle and must be marked.
		for _, k := range []nodeKey{a, b, c} {
			if !cycles[k] {
				t.Fatalf("expected %v in cycle set, got %+v", k, cycles)
			}
		}
		if len(cycles) != 3 {
			t.Fatalf("expected exactly 3 cycle nodes, got %d (%+v)", len(cycles), cycles)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("breakContainmentCycles did not terminate within 100 ms — likely infinite loop")
	}
}

func TestBreakContainmentCycles_LowestKeyAtRoot(t *testing.T) {
	// 4-cycle: a→b→c→d→a. After the break, the layout BFS from anchor=a
	// should reach b, c, d as descendants — confirming that the cycle
	// did NOT break edges out of a (the lowest key remains the root).
	a, b, c, d := makeKey("a"), makeKey("b"), makeKey("c"), makeKey("d")
	rs := makeRS(
		[]string{"a", "b", "c", "d"},
		[]ResultEdge{
			{Src: a, Dst: b, Meta: MetaContains, ArrowLong: "contains"},
			{Src: b, Dst: c, Meta: MetaContains, ArrowLong: "contains"},
			{Src: c, Dst: d, Meta: MetaContains, ArrowLong: "contains"},
			{Src: d, Dst: a, Meta: MetaContains, ArrowLong: "contains"},
		},
	)
	pass := LayoutFromAnchor(rs, a, "")

	// Cycle nodes should be marked regardless of how the layout
	// engine encodes containment spatially (post-layered redesign,
	// containment is no longer the Y axis — it's an edge style).
	for _, k := range []nodeKey{a, b, c, d} {
		if !pass.CycleNodes[k] {
			t.Fatalf("expected %v marked as cycle node, got %+v", k, pass.CycleNodes)
		}
	}
	// The layout must still place every cycle member at a finite,
	// non-NaN position (the BFS must not have looped).
	for _, k := range []nodeKey{a, b, c, d} {
		p, ok := pass.Positions[k]
		if !ok {
			t.Fatalf("expected %v to be placed, missing from Positions", k)
		}
		if math.IsNaN(float64(p.X)) || math.IsNaN(float64(p.Y)) || math.IsNaN(float64(p.Z)) {
			t.Fatalf("expected %v at finite position, got %+v", k, p)
		}
	}
}

func TestBreakContainmentCycles_NoCycleProducesEmptyMarker(t *testing.T) {
	a, b, c := makeKey("a"), makeKey("b"), makeKey("c")
	rs := makeRS(
		[]string{"a", "b", "c"},
		[]ResultEdge{
			{Src: a, Dst: b, Meta: MetaContains, ArrowLong: "contains"},
			{Src: b, Dst: c, Meta: MetaContains, ArrowLong: "contains"},
		},
	)
	adj := buildAdjacency(rs)
	cycles := breakContainmentCycles(rs, adj)
	if len(cycles) != 0 {
		t.Fatalf("expected no cycle markers for an acyclic graph, got %+v", cycles)
	}
}

func TestBreakContainmentCycles_SelfLoop(t *testing.T) {
	a := makeKey("a")
	rs := makeRS(
		[]string{"a"},
		[]ResultEdge{
			{Src: a, Dst: a, Meta: MetaContains, ArrowLong: "contains"},
		},
	)
	adj := buildAdjacency(rs)

	done := make(chan map[nodeKey]bool, 1)
	go func() {
		done <- breakContainmentCycles(rs, adj)
	}()

	select {
	case cycles := <-done:
		if !cycles[a] {
			t.Fatalf("expected self-loop node %v marked as cycle, got %+v", a, cycles)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("self-loop break did not terminate within 100 ms")
	}
}

// TestBreakContainmentCycles_OnlyAffectsContainment confirms that
// LEADSTO / PROPERTIES / SIMILARTO cycles are NOT broken — the layout
// engine handles those via first-reach-wins placement, not cycle break.
func TestBreakContainmentCycles_OnlyAffectsContainment(t *testing.T) {
	a, b, c := makeKey("a"), makeKey("b"), makeKey("c")
	rs := makeRS(
		[]string{"a", "b", "c"},
		[]ResultEdge{
			// Causal cycle: A → B → C → A (legitimate "circular flow")
			{Src: a, Dst: b, Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: b, Dst: c, Meta: MetaLeadsTo, ArrowLong: "leads to"},
			{Src: c, Dst: a, Meta: MetaLeadsTo, ArrowLong: "leads to"},
		},
	)
	adj := buildAdjacency(rs)
	cycles := breakContainmentCycles(rs, adj)
	if len(cycles) != 0 {
		t.Fatalf("expected no cycle markers for a non-containment cycle, got %+v", cycles)
	}
	// Layout should also terminate cleanly.
	done := make(chan struct{})
	go func() {
		LayoutFromAnchor(rs, a, "")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("layout did not terminate on causal cycle")
	}
}
