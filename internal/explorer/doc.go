// Package explorer implements the layout engine for the 2.5D Isometric Graph
// Explorer (feature 006). It is a read-only consumer of the SSTorytime-KV
// graph populated by feature 004's Build Graph action: it converts a query
// result into an anchor-relative typed subgraph, classifies edges into the
// four SSTorytime relational meta-types (Contains / LeadsTo / Properties /
// SimilarTo) — the "Semantic Compass" — assigns deterministic 3D positions
// along the corresponding spatial axes (Y elevation, X causal flow, ±Z
// depth), groups nodes by primary context into ground-plane territories,
// and emits a JSON LayoutResult consumed by the Three.js frontend.
//
// This package writes nothing to disk and never modifies the SSTorytime-KV
// store. See specs/006-isometric-graph-explorer/ for the spec, plan,
// data model, and IPC contracts.
package explorer
