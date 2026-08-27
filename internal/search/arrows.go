package search

import (
	"sort"

	SST "github.com/srosignoli/sstorytime-kv/pkg/SSTorytime"

	"texere/internal/content"
)

// Codes for the implicit sequence arrow pair mirrored from upstream N4L.go
// (SEQUENCE_RELN / SEQUENCE_RELN_INV). Auto-registered so `:: _sequence_ ::`
// blocks can be walked as `(then)` chains without requiring users to declare
// the arrow in their SSTconfig.
const (
	SequenceArrowCode    = "then"
	SequenceArrowLong    = "then followed by"
	SequenceInvArrowCode = "from"
	SequenceInvArrowLong = "follows on from"
)

// RegisterArrows populates the global SST ARROW_DIRECTORY from the registry.
// Must be called after SST.MemoryInit() and before any build or search operation.
// Arrows are processed in stable sorted order so ArrowPtrs are assigned
// consistently across sessions (build and search use identical ordering).
func RegisterArrows(registry *content.SSTRegistry) {
	if registry == nil {
		return
	}

	codes := make([]string, 0, len(registry.Arrows))
	for code := range registry.Arrows {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	ptrByCode := make(map[string]SST.ArrowPtr, len(codes))
	for _, code := range codes {
		// The implicit sequence pair is reserved and auto-registered below.
		// Skipping it here is load-bearing: a collection that merely WRITES
		// `A (then) B` as an ordinary relationship gets `then` harvested into the
		// registry by AugmentWithUnknownArrows (which runs first, search.go:176).
		// Registering it here would make the auto-registration below return -1,
		// which skipped InsertInverseArrowDirectory and silently emptied \story
		// and \sequence for the ENTIRE collection — one such arrow in one file
		// was enough, with no error and the sequence nodes still text-searchable.
		if code == SequenceArrowCode || code == SequenceInvArrowCode {
			continue
		}
		def := registry.Arrows[code]
		mt := def.MetaType
		if mt == "" {
			mt = "leadsto"
		}
		ptr := SST.InsertArrowDirectory(mt, def.Code, def.LongName, "+")
		if ptr >= 0 {
			ptrByCode[code] = ptr
		} else {
			// Already registered in this session — look up existing ptr.
			ptrByCode[code] = SST.GetDBArrowByName(SST.PoSST{}, def.Code)
		}
	}

	// Wire inverse pairs so IdempDBAddLink can add reverse links automatically.
	for _, code := range codes {
		def := registry.Arrows[code]
		if def.Reverse == "" {
			continue
		}
		fwdPtr, hasFwd := ptrByCode[code]
		bwdPtr, hasBwd := ptrByCode[def.Reverse]
		if hasFwd && hasBwd {
			SST.InsertInverseArrowDirectory(fwdPtr, bwdPtr)
		}
	}

	// Auto-register the implicit sequence arrow pair (upstream N4L.go does
	// the same at line 1388). Used by the builder to wire consecutive nodes
	// in a `:: _sequence_ ::` block so `\story` / `\sequence` can walk them.
	// Resolve rather than give up when the insert reports "already present".
	// Within one process RegisterArrows runs on every lazy open, so the second
	// call legitimately finds the pair registered — and abandoning the inverse
	// wiring in that case is the same silent failure as above, just reached by
	// a different route.
	fwdSeq := SST.InsertArrowDirectory("leadsto", SequenceArrowCode, SequenceArrowLong, "+")
	if fwdSeq < 0 {
		fwdSeq = SST.GetDBArrowByName(SST.PoSST{}, SequenceArrowCode)
	}
	invSeq := SST.InsertArrowDirectory("leadsto", SequenceInvArrowCode, SequenceInvArrowLong, "-")
	if invSeq < 0 {
		invSeq = SST.GetDBArrowByName(SST.PoSST{}, SequenceInvArrowCode)
	}
	if fwdSeq >= 0 && invSeq >= 0 {
		SST.InsertInverseArrowDirectory(fwdSeq, invSeq)
	}
}
