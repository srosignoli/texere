package explorer

import (
	"math"
	"sort"
)

// =============================================================================
// Territory polygon computation. Per research.md R-5: convex hull per
// primary context, dilated by a fixed margin, with pairwise overlap
// shrinking so cross-context bridge edges (FR-020) have a visible gap.
//
// Output is deterministic: contexts sorted alphabetically, vertices in
// counter-clockwise order starting at the lowest-Z / lowest-X point.
// =============================================================================

const (
	// TerritoryDilation is the outward margin applied to each context's
	// convex hull, in world units (research.md R-5).
	TerritoryDilation float32 = 0.5

	// TerritoryBufferGap is the minimum gap between two dilated hulls.
	// Overlapping pairs are shrunk symmetrically until this gap is met.
	TerritoryBufferGap float32 = 0.2

	// SingleNodeDiscRadius is the world-units radius of the hex disc
	// emitted for single-node context groups.
	SingleNodeDiscRadius float32 = TerritoryDilation + 0.8

	// SingleContextPadding is the extra dilation applied when the
	// ResultSet has only one context — the polygon spans the whole
	// landscape so the user reads "this is one zone" at a glance
	// (FR-021).
	SingleContextPadding float32 = 1.5
)

// BuildTerritories groups nodes by primary context, produces a convex
// polygon per group on the (x, z) ground plane, then resolves overlaps
// between dilated polygons. The y coordinate of positions is ignored.
//
// Nodes whose Contexts slice is empty are skipped — they belong to no
// territory and the frontend renders them on the neutral ground plane.
func BuildTerritories(rs *ResultSet, positions map[nodeKey]Vec3) []TerritoryPolygon {
	if rs == nil || len(rs.Nodes) == 0 {
		return nil
	}

	groups := groupByPrimaryContext(rs, positions)
	if len(groups) == 0 {
		return nil
	}

	contexts := make([]string, 0, len(groups))
	for ctx := range groups {
		contexts = append(contexts, ctx)
	}
	sort.Strings(contexts)

	singleContext := len(contexts) == 1

	polys := make([]TerritoryPolygon, 0, len(contexts))
	for _, ctx := range contexts {
		pts := groups[ctx]
		var verts []Vec2
		switch {
		case len(pts) == 1:
			verts = hexDisc(pts[0], SingleNodeDiscRadius)
		case singleContext:
			verts = dilateAround(convexHull(pts), TerritoryDilation+SingleContextPadding)
		default:
			verts = dilateAround(convexHull(pts), TerritoryDilation)
		}
		polys = append(polys, TerritoryPolygon{Context: ctx, Vertices: verts})
	}

	if !singleContext {
		resolveOverlaps(polys)
	}

	for i := range polys {
		polys[i].Vertices = canonicalizeCCW(polys[i].Vertices)
	}
	return polys
}

// groupByPrimaryContext returns the ground-plane positions of each
// node, grouped by its first context tag. Nodes with no contexts are
// excluded.
func groupByPrimaryContext(rs *ResultSet, positions map[nodeKey]Vec3) map[string][]Vec2 {
	out := make(map[string][]Vec2)
	for _, n := range rs.Nodes {
		if len(n.Contexts) == 0 {
			continue
		}
		p, ok := positions[n.Key]
		if !ok {
			continue
		}
		ctx := n.Contexts[0]
		out[ctx] = append(out[ctx], Vec2{X: p.X, Z: p.Z})
	}
	return out
}

// =============================================================================
// Convex hull (Graham scan)
// =============================================================================

// convexHull returns the convex hull of pts in counter-clockwise order
// on the (x, z) plane, starting at the point with the lowest Z (ties
// broken by lowest X). The input slice is not mutated.
//
// Determinism: duplicates are removed and points are sorted before the
// scan so the output is stable across runs.
func convexHull(pts []Vec2) []Vec2 {
	uniq := dedupeAndSort(pts)
	if len(uniq) <= 2 {
		return uniq
	}

	// Find pivot: lowest Z, then lowest X.
	pivotIdx := 0
	for i, p := range uniq {
		if p.Z < uniq[pivotIdx].Z || (p.Z == uniq[pivotIdx].Z && p.X < uniq[pivotIdx].X) {
			pivotIdx = i
		}
	}
	pivot := uniq[pivotIdx]

	rest := make([]Vec2, 0, len(uniq)-1)
	for i, p := range uniq {
		if i != pivotIdx {
			rest = append(rest, p)
		}
	}

	// Sort by polar angle from pivot ascending (CCW from the +X
	// direction); collinear points sorted by distance (closer first) so
	// the scan pops the intermediates.
	sort.SliceStable(rest, func(i, j int) bool {
		ax, az := rest[i].X-pivot.X, rest[i].Z-pivot.Z
		bx, bz := rest[j].X-pivot.X, rest[j].Z-pivot.Z
		// cross > 0 means b has the larger polar angle (a is earlier in
		// CCW order); cross < 0 means b is earlier and must come first.
		cross := ax*bz - bx*az
		if cross != 0 {
			return cross > 0
		}
		// Collinear: closer first.
		return ax*ax+az*az < bx*bx+bz*bz
	})

	stack := []Vec2{pivot}
	for _, p := range rest {
		for len(stack) >= 2 && cross2D(stack[len(stack)-2], stack[len(stack)-1], p) <= 0 {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, p)
	}
	return stack
}

// cross2D returns the z-component of the cross product (b-a)×(c-a) on
// the (x, z) plane. Positive = counter-clockwise turn at b.
func cross2D(a, b, c Vec2) float32 {
	return (b.X-a.X)*(c.Z-a.Z) - (b.Z-a.Z)*(c.X-a.X)
}

func dedupeAndSort(pts []Vec2) []Vec2 {
	seen := make(map[Vec2]bool, len(pts))
	out := make([]Vec2, 0, len(pts))
	for _, p := range pts {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].X != out[j].X {
			return out[i].X < out[j].X
		}
		return out[i].Z < out[j].Z
	})
	return out
}

// =============================================================================
// Dilation and overlap resolution
// =============================================================================

// dilateAround pushes each polygon vertex away from the centroid by
// `margin` world units. This is a cheap approximation of a Minkowski
// dilation; it preserves convexity and is exact at vertices that lie on
// a circle around the centroid. Good enough at this scale (research.md
// R-5: "convex hulls … look organic at this scale").
func dilateAround(poly []Vec2, margin float32) []Vec2 {
	if len(poly) == 0 || margin == 0 {
		return poly
	}
	c := centroid(poly)
	out := make([]Vec2, len(poly))
	for i, p := range poly {
		dx, dz := p.X-c.X, p.Z-c.Z
		d := float32(math.Hypot(float64(dx), float64(dz)))
		if d == 0 {
			out[i] = p
			continue
		}
		scale := (d + margin) / d
		out[i] = Vec2{X: c.X + dx*scale, Z: c.Z + dz*scale}
	}
	return out
}

func centroid(poly []Vec2) Vec2 {
	if len(poly) == 0 {
		return Vec2{}
	}
	var sx, sz float32
	for _, p := range poly {
		sx += p.X
		sz += p.Z
	}
	n := float32(len(poly))
	return Vec2{X: sx / n, Z: sz / n}
}

// hexDisc returns a regular hexagon centered at c with the supplied
// radius. Used for single-node context groups so the territory polygon
// always has ≥3 vertices (data-model.md §2.3 validation rule).
func hexDisc(c Vec2, radius float32) []Vec2 {
	out := make([]Vec2, 6)
	for i := 0; i < 6; i++ {
		theta := math.Pi/6 + float64(i)*math.Pi/3
		out[i] = Vec2{
			X: c.X + radius*float32(math.Cos(theta)),
			Z: c.Z + radius*float32(math.Sin(theta)),
		}
	}
	return out
}

// resolveOverlaps is intentionally a no-op in v1.
//
// The original design (research.md R-5 step 5) called for shrinking
// overlapping pairs by their bounding-circle overlap so the territories
// kept a visible buffer gap. In practice this collapses single-node
// hex discs whenever two context-tagged root nodes land near each other
// in the BFS layout — common in real chapters because tagged roots
// cluster around the anchor. A pair of radius-1.3 hex discs with
// centroids closer than ~2.4 wu shrinks to zero area, rendering both
// territories invisible.
//
// Cross-context bridge edges (T042) now arch upward in +Y, which is the
// principal visual cue for "this edge spans territories" — the buffer
// gap on the ground plane is no longer load-bearing. Letting polygons
// overlap is the lesser evil: semi-transparent fills blend additively
// where they meet, which honestly communicates that a node belongs to
// the intersection of two contexts.
//
// T063 (polish) can revisit with proper SAT-based polygon overlap and
// bounded shrinking. Until then, prefer visible-and-overlapping over
// invisible-with-buffer.
func resolveOverlaps(polys []TerritoryPolygon) {
	_ = polys
}

func maxRadius(poly []Vec2, c Vec2) float32 {
	var max float32
	for _, p := range poly {
		dx, dz := p.X-c.X, p.Z-c.Z
		d := float32(math.Hypot(float64(dx), float64(dz)))
		if d > max {
			max = d
		}
	}
	return max
}

// =============================================================================
// Canonical orientation
// =============================================================================

// canonicalizeCCW rotates a polygon so its first vertex is the one with
// the lowest Z (ties broken by lowest X), and ensures the winding order
// is counter-clockwise on the (x, z) plane. Required by T040 / the
// contract in layout-types.md.
func canonicalizeCCW(poly []Vec2) []Vec2 {
	if len(poly) < 3 {
		return poly
	}
	// Find anchor vertex.
	start := 0
	for i, p := range poly {
		if p.Z < poly[start].Z || (p.Z == poly[start].Z && p.X < poly[start].X) {
			start = i
		}
	}
	rotated := make([]Vec2, len(poly))
	for i := range poly {
		rotated[i] = poly[(start+i)%len(poly)]
	}
	// Ensure CCW by summing the signed area via the shoelace formula.
	var area float32
	for i := 0; i < len(rotated); i++ {
		j := (i + 1) % len(rotated)
		area += (rotated[j].X - rotated[i].X) * (rotated[j].Z + rotated[i].Z)
	}
	// area > 0 → clockwise on (x, z) with z growing up → reverse.
	if area > 0 {
		// Reverse everything except the anchor at index 0.
		for i, j := 1, len(rotated)-1; i < j; i, j = i+1, j-1 {
			rotated[i], rotated[j] = rotated[j], rotated[i]
		}
	}
	return rotated
}
