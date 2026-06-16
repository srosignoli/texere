package explorer

import "strings"

// MetaType is one of the four SSTorytime relational categories that drive
// spatial layout in the Semantic Compass, plus an Unclassified fallback for
// edges whose registry meta-type is unknown or empty.
//
// The integer values are stable and used in unit tests; the iota ordering
// matches the documentation order in spec.md (Contains / LeadsTo /
// Properties / SimilarTo).
type MetaType uint8

const (
	MetaUnclassified MetaType = iota
	MetaContains              // North → +Y elevation
	MetaLeadsTo               // East → +X horizontal flow
	MetaProperties            // South → +Z near depth
	MetaSimilarTo             // West → -Z far depth
)

// String returns a stable lowercase token for the wire contract
// (matches contracts/layout-types.md EdgeView.meta values).
func (m MetaType) String() string {
	switch m {
	case MetaContains:
		return "contains"
	case MetaLeadsTo:
		return "leadsto"
	case MetaProperties:
		return "properties"
	case MetaSimilarTo:
		return "similarto"
	default:
		return "unclassified"
	}
}

// parseMetaType maps a registry meta-type string to a MetaType. The match
// is case-insensitive and tolerates the synonyms found in existing test
// fixtures and the canonical SSTconfig labels (arrows-CN-2.sst declares
// "contains", arrows-LT-1.sst "leadsto", arrows-EP-3.sst "properties",
// arrows-NR-0.sst "similarity"; some test fixtures use "express",
// "similarto", or "near" instead).
//
// Unrecognized or empty inputs return MetaUnclassified — those edges are
// included in the result set but contribute nothing to spatial layout.
func parseMetaType(raw string) MetaType {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "contains", "contain", "containment":
		return MetaContains
	case "leadsto", "leads-to", "leads_to", "causality":
		return MetaLeadsTo
	case "properties", "property", "express", "expresses", "expression":
		return MetaProperties
	case "similarity", "similar", "similarto", "similar-to", "near", "metaphor":
		return MetaSimilarTo
	default:
		return MetaUnclassified
	}
}
