package explorer

import (
	"math"
	"reflect"
	"testing"
)

// =============================================================================
// T039: Territory polygon tests.
//
// We assert four properties:
//   1. Determinism — repeated calls produce identical output.
//   2. Three contexts → three polygons, sorted by context name.
//   3. Single context → one polygon, larger than the raw hull (padding).
//   4. Non-overlapping output — every pair of polygons is separated by
//      at least TerritoryBufferGap on centroid distance.
// =============================================================================

func TestBuildTerritories_ThreeContextsThreePolygons(t *testing.T) {
	rs, positions := threeContextFixture()
	polys := BuildTerritories(rs, positions)

	if len(polys) != 3 {
		t.Fatalf("expected 3 polygons, got %d", len(polys))
	}

	// Polygons must be ordered by context name (alphabetical).
	wantOrder := []string{"alpha", "beta", "gamma"}
	for i, p := range polys {
		if p.Context != wantOrder[i] {
			t.Fatalf("polygon %d: expected context %q, got %q", i, wantOrder[i], p.Context)
		}
	}

	for _, p := range polys {
		if len(p.Vertices) < 3 {
			t.Fatalf("polygon %q has only %d vertices; expected ≥3", p.Context, len(p.Vertices))
		}
	}
}

func TestBuildTerritories_Deterministic(t *testing.T) {
	rs, positions := threeContextFixture()
	first := BuildTerritories(rs, positions)
	for i := 0; i < 25; i++ {
		next := BuildTerritories(rs, positions)
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("iteration %d: output not deterministic", i)
		}
	}
}

func TestBuildTerritories_VerticesCounterClockwise(t *testing.T) {
	rs, positions := threeContextFixture()
	polys := BuildTerritories(rs, positions)

	for _, p := range polys {
		if !isCounterClockwise(p.Vertices) {
			t.Fatalf("polygon %q vertices are not counter-clockwise: %+v", p.Context, p.Vertices)
		}
		// Anchor vertex is the lowest-Z (ties → lowest-X).
		anchor := p.Vertices[0]
		for _, v := range p.Vertices[1:] {
			if v.Z < anchor.Z || (v.Z == anchor.Z && v.X < anchor.X) {
				t.Fatalf("polygon %q: vertex %+v should precede anchor %+v", p.Context, v, anchor)
			}
		}
	}
}

func TestBuildTerritories_NonOverlappingPairs(t *testing.T) {
	rs, positions := threeContextFixture()
	polys := BuildTerritories(rs, positions)

	for i := 0; i < len(polys); i++ {
		for j := i + 1; j < len(polys); j++ {
			a, b := polys[i], polys[j]
			ca, cb := centroid(a.Vertices), centroid(b.Vertices)
			ra := maxRadius(a.Vertices, ca)
			rb := maxRadius(b.Vertices, cb)
			dx, dz := ca.X-cb.X, ca.Z-cb.Z
			dist := float32(math.Hypot(float64(dx), float64(dz)))
			gap := dist - ra - rb
			if gap < TerritoryBufferGap-1e-3 {
				t.Fatalf("territories %q and %q overlap or touch: gap=%.3f, want ≥%.3f",
					a.Context, b.Context, gap, TerritoryBufferGap)
			}
		}
	}
}

func TestBuildTerritories_SingleContextOnePolygon(t *testing.T) {
	rs := makeRS([]string{"a", "b", "c", "d"}, nil)
	for i := range rs.Nodes {
		rs.Nodes[i].Contexts = []string{"only"}
	}
	positions := map[nodeKey]Vec3{
		rs.Nodes[0].Key: {X: 0, Z: 0},
		rs.Nodes[1].Key: {X: 4, Z: 0},
		rs.Nodes[2].Key: {X: 4, Z: 4},
		rs.Nodes[3].Key: {X: 0, Z: 4},
	}
	polys := BuildTerritories(rs, positions)
	if len(polys) != 1 {
		t.Fatalf("expected 1 polygon for single-context result, got %d", len(polys))
	}
	if polys[0].Context != "only" {
		t.Fatalf("expected context %q, got %q", "only", polys[0].Context)
	}
	// Single-context padding should push at least one vertex past the
	// raw bounding box (0,0)–(4,4).
	pushedOut := false
	for _, v := range polys[0].Vertices {
		if v.X < -0.5 || v.X > 4.5 || v.Z < -0.5 || v.Z > 4.5 {
			pushedOut = true
			break
		}
	}
	if !pushedOut {
		t.Fatalf("single-context padding did not push vertices past the raw hull: %+v", polys[0].Vertices)
	}
}

func TestBuildTerritories_SingleNodeHexDisc(t *testing.T) {
	rs := makeRS([]string{"solo"}, nil)
	rs.Nodes[0].Contexts = []string{"loneliness"}
	positions := map[nodeKey]Vec3{rs.Nodes[0].Key: {X: 2, Z: 3}}

	polys := BuildTerritories(rs, positions)
	if len(polys) != 1 {
		t.Fatalf("expected 1 polygon, got %d", len(polys))
	}
	if len(polys[0].Vertices) != 6 {
		t.Fatalf("expected hex disc with 6 vertices, got %d", len(polys[0].Vertices))
	}
	// All vertices should be approximately SingleNodeDiscRadius from center.
	c := Vec2{X: 2, Z: 3}
	for _, v := range polys[0].Vertices {
		d := float32(math.Hypot(float64(v.X-c.X), float64(v.Z-c.Z)))
		if math.Abs(float64(d-SingleNodeDiscRadius)) > 1e-3 {
			t.Fatalf("hex vertex %+v at distance %.3f from center; expected %.3f", v, d, SingleNodeDiscRadius)
		}
	}
}

func TestBuildTerritories_SkipsContextlessNodes(t *testing.T) {
	rs := makeRS([]string{"tagged", "untagged"}, nil)
	rs.Nodes[0].Contexts = []string{"ctx"}
	rs.Nodes[1].Contexts = nil
	positions := map[nodeKey]Vec3{
		rs.Nodes[0].Key: {X: 0, Z: 0},
		rs.Nodes[1].Key: {X: 5, Z: 5},
	}
	polys := BuildTerritories(rs, positions)
	if len(polys) != 1 {
		t.Fatalf("expected 1 polygon (contextless node skipped), got %d", len(polys))
	}
}

// =============================================================================
// Internal helpers used by tests
// =============================================================================

// threeContextFixture builds a 9-node ResultSet split across three
// contexts with positions clustered far enough apart that the dilated
// hulls do not need shrinking.
func threeContextFixture() (*ResultSet, map[nodeKey]Vec3) {
	rs := makeRS(
		[]string{"a1", "a2", "a3", "b1", "b2", "b3", "g1", "g2", "g3"},
		nil,
	)
	ctxFor := map[string]string{
		"a1": "alpha", "a2": "alpha", "a3": "alpha",
		"b1": "beta", "b2": "beta", "b3": "beta",
		"g1": "gamma", "g2": "gamma", "g3": "gamma",
	}
	for i, n := range rs.Nodes {
		rs.Nodes[i].Contexts = []string{ctxFor[n.Text]}
	}
	positions := map[nodeKey]Vec3{
		makeKey("a1"): {X: -10, Z: -10},
		makeKey("a2"): {X: -8, Z: -10},
		makeKey("a3"): {X: -9, Z: -8},
		makeKey("b1"): {X: 10, Z: -10},
		makeKey("b2"): {X: 12, Z: -10},
		makeKey("b3"): {X: 11, Z: -8},
		makeKey("g1"): {X: 0, Z: 10},
		makeKey("g2"): {X: 2, Z: 10},
		makeKey("g3"): {X: 1, Z: 12},
	}
	return rs, positions
}

// isCounterClockwise returns true if the signed area of the polygon is
// positive under the (x, z)-with-z-up convention used by our renderer.
// We compute the standard shoelace area in 2D and treat z as the
// vertical axis.
func isCounterClockwise(poly []Vec2) bool {
	if len(poly) < 3 {
		return false
	}
	var area float32
	for i := 0; i < len(poly); i++ {
		j := (i + 1) % len(poly)
		area += poly[i].X*poly[j].Z - poly[j].X*poly[i].Z
	}
	return area > 0
}
