package explorer

import "math"

// ScaleSizes maps each node's in-degree to a visual size in world units
// using a logarithmic curve. The result is bounded by [minSize, maxSize].
//
// Formula (per tasks.md T034 and US3 acceptance criteria):
//
//   size(n) = minSize + (maxSize - minSize) * log1p(inDegree(n)) / log1p(maxInDegree)
//
// Properties:
//   - A node with zero incoming edges renders at exactly minSize (FR-014
//     requires it to be the minimum non-zero size).
//   - The most-referenced node renders at exactly maxSize.
//   - log1p compresses the high end so a single 100×-referenced node
//     doesn't visually overwhelm a 10×-referenced one.
//   - When every node has identical in-degree (or the result is empty /
//     all-zero), every node receives minSize. This is the "no false
//     landmarks" behavior US3 calls for: when there is no relative
//     dominance signal, no node visually dominates.
//
// The picking mesh size (FR-014a, 24 px screen-space) is computed
// separately on the frontend and is unaffected by this function — only
// the visible centrality encoding changes here.
func ScaleSizes(rs *ResultSet, minSize, maxSize float32) map[nodeKey]float32 {
	out := make(map[nodeKey]float32, len(rs.Nodes))
	if rs == nil || len(rs.Nodes) == 0 {
		return out
	}
	if maxSize < minSize {
		minSize, maxSize = maxSize, minSize
	}

	// Find the maximum in-degree in this result (NOT the global graph —
	// centrality is local to the materialized subgraph per data-model.md).
	maxIn := 0
	for _, n := range rs.Nodes {
		if n.InDegree > maxIn {
			maxIn = n.InDegree
		}
	}

	// Degenerate case: no in-degree variation (everyone is equal-or-zero).
	// Map every node to minSize so the user sees "flat hierarchy" rather
	// than a false uniform landmark band at maxSize.
	if maxIn == 0 {
		for _, n := range rs.Nodes {
			out[n.Key] = minSize
		}
		return out
	}

	denom := math.Log1p(float64(maxIn))
	span := float64(maxSize - minSize)
	for _, n := range rs.Nodes {
		t := math.Log1p(float64(n.InDegree)) / denom
		out[n.Key] = float32(float64(minSize) + span*t)
	}
	return out
}
