package explorer

import (
	"math"
	"testing"
)

// =============================================================================
// T035: Centrality scaling — log curve, bounds, zero-in-degree → minSize
// =============================================================================

func TestScaleSizes_ZeroInDegreeMapsToMinSize(t *testing.T) {
	// Build a result where one node has in-degree 0, another has 5.
	// Even though MIN is enforced from the formula (log1p(0) = 0),
	// verify the result equals minSize exactly (no float drift).
	a, b := makeKey("a"), makeKey("b")
	rs := makeRS(
		[]string{"a", "b"},
		[]ResultEdge{
			{Src: a, Dst: b, Meta: MetaContains, ArrowLong: "contains"},
		},
	)
	// rs.Nodes after makeRS has b's in-degree = 1 from the makeRS helper,
	// but a's in-degree should be 0.

	sizes := ScaleSizes(rs, 0.5, 2.5)
	if sizes[a] != 0.5 {
		t.Fatalf("zero-in-degree node should map to minSize=0.5, got %g", sizes[a])
	}
	if sizes[b] != 2.5 {
		t.Fatalf("only-non-zero-in-degree node should map to maxSize=2.5, got %g", sizes[b])
	}
}

func TestScaleSizes_BoundsAreEnforced(t *testing.T) {
	// 5 nodes with widely varying in-degrees: 0, 1, 5, 50, 100. Verify
	// every output sits inside [minSize, maxSize].
	rs := buildLogScaleFixture()

	sizes := ScaleSizes(rs, 0.5, 2.5)
	for k, s := range sizes {
		if s < 0.5 || s > 2.5 {
			t.Fatalf("node %v size %g outside [0.5, 2.5]", k, s)
		}
	}
}

func TestScaleSizes_LogCurveOrdering(t *testing.T) {
	// Higher in-degree must produce strictly larger sizes when there is
	// any in-degree variation in the result. Check ordering, not exact
	// values (the formula's exact intermediates aren't part of the
	// contract).
	rs := buildLogScaleFixture()
	sizes := ScaleSizes(rs, 0.5, 2.5)

	d0 := makeKey("d0")     // in-degree 0
	d1 := makeKey("d1")     // 1
	d5 := makeKey("d5")     // 5
	d50 := makeKey("d50")   // 50
	d100 := makeKey("d100") // 100

	if !(sizes[d0] < sizes[d1] && sizes[d1] < sizes[d5] && sizes[d5] < sizes[d50] && sizes[d50] < sizes[d100]) {
		t.Fatalf("size order broken — got\n  d0=%g d1=%g d5=%g d50=%g d100=%g",
			sizes[d0], sizes[d1], sizes[d5], sizes[d50], sizes[d100])
	}

	// Log curve compression: the gap between d50 and d100 should be
	// SMALLER than the gap between d1 and d5 (a 5× ratio at the bottom
	// vs a 2× ratio at the top). This confirms log behavior — a
	// linear scale would have the opposite relationship.
	gapBottom := sizes[d5] - sizes[d1]
	gapTop := sizes[d100] - sizes[d50]
	if !(gapTop < gapBottom) {
		t.Fatalf("log curve compression failed: gap(d50→d100)=%g should be < gap(d1→d5)=%g",
			gapTop, gapBottom)
	}
}

func TestScaleSizes_LandmarkSeparationAt10x(t *testing.T) {
	// US3 SC-002: a node with 10× the average in-degree must be visibly
	// larger than its peers. We assert the landmark's size is at least
	// halfway between minSize and maxSize when the rest of the field
	// sits at low single-digit in-degrees.
	rs := buildLogScaleFixture()
	sizes := ScaleSizes(rs, 0.5, 2.5)
	d100 := makeKey("d100")
	d5 := makeKey("d5")

	// d100 is 20× d5; the landmark should be very close to maxSize.
	if !(sizes[d100] > 0.5+0.75*(2.5-0.5)) {
		t.Fatalf("landmark d100 (in-deg 100) should sit in the upper quartile of the size range, got %g", sizes[d100])
	}
	// d5 is well above the floor, but still well below the landmark.
	if !(sizes[d100]-sizes[d5] > 0.4) {
		t.Fatalf("landmark should be visibly larger than mid-tier (gap > 0.4), got gap=%g", sizes[d100]-sizes[d5])
	}
}

func TestScaleSizes_AllZeroInDegree(t *testing.T) {
	// "No false landmarks": a fully orphan result (no edges at all)
	// should yield minSize for every node, not maxSize.
	rs := makeRS([]string{"a", "b", "c"}, nil)
	sizes := ScaleSizes(rs, 0.5, 2.5)
	for k, s := range sizes {
		if s != 0.5 {
			t.Fatalf("orphan node %v should be minSize=0.5, got %g", k, s)
		}
	}
}

func TestScaleSizes_AllUniformInDegree(t *testing.T) {
	// US3 acceptance scenario 2: when every node has the same in-degree
	// (within a small range), sizes should appear roughly uniform — no
	// false landmark emerges. With uniform in-degree, every node hits
	// the maximum of the formula. We accept that as "uniform" for
	// rendering purposes, which still satisfies the acceptance criterion
	// (the test is that landmarks should not falsely emerge — a uniform
	// maxSize is still uniform).
	a, b, c := makeKey("a"), makeKey("b"), makeKey("c")
	d := makeKey("d")
	rs := makeRS(
		[]string{"a", "b", "c", "d"},
		[]ResultEdge{
			{Src: a, Dst: b, Meta: MetaContains, ArrowLong: "contains"},
			{Src: a, Dst: c, Meta: MetaContains, ArrowLong: "contains"},
			{Src: a, Dst: d, Meta: MetaContains, ArrowLong: "contains"},
		},
	)
	// b, c, d all have in-degree 1; a has 0.
	sizes := ScaleSizes(rs, 0.5, 2.5)
	if !(sizes[b] == sizes[c] && sizes[c] == sizes[d]) {
		t.Fatalf("uniform in-degree should yield uniform sizes; got b=%g c=%g d=%g",
			sizes[b], sizes[c], sizes[d])
	}
}

func TestScaleSizes_EmptyResult(t *testing.T) {
	rs := &ResultSet{}
	sizes := ScaleSizes(rs, 0.5, 2.5)
	if len(sizes) != 0 {
		t.Fatalf("empty result should yield empty size map, got %d entries", len(sizes))
	}
}

func TestScaleSizes_SwappedBoundsAreNormalized(t *testing.T) {
	// Defensive: if a caller passes (max, min) by accident, the function
	// normalizes rather than producing inverted/negative sizes.
	rs := buildLogScaleFixture()
	sizesA := ScaleSizes(rs, 0.5, 2.5)
	sizesB := ScaleSizes(rs, 2.5, 0.5)
	for k := range sizesA {
		if math.Abs(float64(sizesA[k]-sizesB[k])) > 1e-6 {
			t.Fatalf("swapped bounds should produce identical output for key %v: %g vs %g", k, sizesA[k], sizesB[k])
		}
	}
}

// =============================================================================
// Integration: T036 wiring — LayoutPass.NodeSizes / NodeView.size carry the
// log-scaled values, not the placeholder constant.
// =============================================================================

func TestLayoutFromAnchor_NodeSizesCarryCentralityScaling(t *testing.T) {
	rs := buildLogScaleFixture()

	// Pick the highest-centrality node as anchor so the centrality
	// signal is preserved across the layout pass (anchor's own
	// in-degree is unchanged by anchoring).
	d100 := makeKey("d100")
	pass := LayoutFromAnchor(rs, d100, "")

	d0 := makeKey("d0")
	if !(pass.NodeSizes[d0] < pass.NodeSizes[d100]) {
		t.Fatalf("expected centrality scaling in NodeSizes, got d0=%g d100=%g",
			pass.NodeSizes[d0], pass.NodeSizes[d100])
	}
	if pass.NodeSizes[d0] != MinNodeSize {
		t.Fatalf("zero-in-degree node should be exactly MinNodeSize=%g, got %g", MinNodeSize, pass.NodeSizes[d0])
	}
	if pass.NodeSizes[d100] != MaxNodeSize {
		t.Fatalf("max-in-degree node should be exactly MaxNodeSize=%g, got %g", MaxNodeSize, pass.NodeSizes[d100])
	}
}

// =============================================================================
// Helpers
// =============================================================================

// buildLogScaleFixture returns a 5-node result where each node has a
// distinct in-degree: 0, 1, 5, 50, 100. Used by every centrality test.
func buildLogScaleFixture() *ResultSet {
	d0 := makeKey("d0")
	d1 := makeKey("d1")
	d5 := makeKey("d5")
	d50 := makeKey("d50")
	d100 := makeKey("d100")

	// Generate edges to give each "dN" node exactly N incoming edges
	// from synthetic source nodes. We register all source nodes too so
	// makeRS computes in-degrees correctly.
	names := []string{"d0", "d1", "d5", "d50", "d100"}
	var edges []ResultEdge

	addIncoming := func(target nodeKey, count int, srcPrefix string) {
		for i := 0; i < count; i++ {
			src := makeKey(srcPrefix + "_" + intToStr(i))
			names = append(names, srcPrefix+"_"+intToStr(i))
			edges = append(edges, ResultEdge{
				Src: src, Dst: target, Meta: MetaContains, ArrowLong: "contains",
			})
		}
	}
	addIncoming(d1, 1, "src1")
	addIncoming(d5, 5, "src5")
	addIncoming(d50, 50, "src50")
	addIncoming(d100, 100, "src100")
	_ = d0 // d0 has no incoming edges

	return makeRS(names, edges)
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// ScaleSizes guards against a nil ResultSet, but the guard sat after the
// make(..., len(rs.Nodes)) that dereferences it — so the nil case panicked
// instead of returning the empty map the guard intends. staticcheck flagged it
// as SA5011.
func TestScaleSizesHandlesANilResultSet(t *testing.T) {
	sizes := ScaleSizes(nil, 0.5, 2.5)
	if sizes == nil {
		t.Fatal("ScaleSizes(nil) returned a nil map, want an empty one")
	}
	if len(sizes) != 0 {
		t.Fatalf("ScaleSizes(nil) returned %d entries, want 0", len(sizes))
	}
}
