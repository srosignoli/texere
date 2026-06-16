package content

import "sort"

// unknownArrowMetaType is the SST meta-type assigned to synthesized unknown
// arrows. We use "similarity" (NEAR) purely because it maps to a valid STType
// slot that the orbit/cone traversal scans, so the materialized links are
// discoverable. The explorer overrides the *visual* meta to a neutral
// "unknown" style (dashed, no directional axis) via the ArrowDef.Unknown
// flag, so this internal choice never leaks into the spatial layout.
const unknownArrowMetaType = "similarity"

// AugmentWithUnknownArrows scans the parsed files for arrows whose code is not
// declared in any SSTconfig (Arrow.IsRegistered == false) and injects a
// synthetic ArrowDef for each distinct code into reg.Arrows, flagged Unknown.
//
// Both the graph builder and the search service call this BEFORE
// RegisterArrows so the resulting ARROW_DIRECTORY is identical on both sides
// (RegisterArrows sorts by code, so ArrowPtr indices line up deterministically
// across build and search). With the code registered, the link materializes
// like any other edge instead of being dropped — the authored code is used as
// the LongName so it survives all the way to the explorer's edge label.
//
// Already-registered codes are never overwritten. Returns the count of
// distinct unknown codes added (0 when every arrow was registered).
func AugmentWithUnknownArrows(reg *SSTRegistry, files []*ParsedFile) int {
	if reg == nil {
		return 0
	}
	if reg.Arrows == nil {
		reg.Arrows = make(map[string]ArrowDef)
	}

	// Collect distinct unknown codes deterministically.
	unknown := make(map[string]struct{})
	consider := func(a Arrow) {
		if a.Code == "" || a.IsRegistered {
			return
		}
		if _, ok := reg.Arrows[a.Code]; ok {
			return // already registered (or already added on a previous pass)
		}
		unknown[a.Code] = struct{}{}
	}
	for _, pf := range files {
		if pf == nil {
			continue
		}
		for _, ch := range pf.Chapters {
			for _, node := range ch.Nodes {
				for _, a := range node.Arrows {
					consider(a)
				}
				for _, d := range node.Dittos {
					for _, a := range d.Arrows {
						consider(a)
					}
				}
			}
		}
	}

	codes := make([]string, 0, len(unknown))
	for c := range unknown {
		codes = append(codes, c)
	}
	sort.Strings(codes)

	for _, code := range codes {
		reg.Arrows[code] = ArrowDef{
			Code:     code,
			LongName: code, // authored code doubles as the human label
			MetaType: unknownArrowMetaType,
			Unknown:  true,
		}
	}
	return len(codes)
}
