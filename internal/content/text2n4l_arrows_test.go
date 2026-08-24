package content

import (
	"path/filepath"
	"testing"
)

// The two arrows SSTorytime's text2N4L writes into every document it
// fractionates must resolve to real contains-type arrows, not be synthesized
// as "unknown" by AugmentWithUnknownArrows.
//
// They are not a minor case: in a collection containing text2N4L output they
// are the two most-used arrows by a wide margin — 16,906 of 17,485 undeclared
// occurrences in the sample collection before they were declared. Left
// undeclared, every one of those relations lands in the graph as an untyped
// "unknown" edge.
//
// Codes and long names come from SSTorytime's own constants in
// pkg/SSTorytime/globals.go (CONT_FINDS / INV_CONT_FOUND_IN and
// CONT_FRAG / INV_CONT_FRAG_IN); their partners are asserted here so the
// reverse direction keeps resolving too.
func TestText2N4LArrowsAreDeclared(t *testing.T) {
	reg, err := LoadRegistry(filepath.Join("..", "..", "SSTconfig"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if len(reg.LoadErrors) > 0 {
		t.Fatalf("registry load errors: %v", reg.LoadErrors)
	}

	for _, want := range []struct{ code, reverse string }{
		{"extract-fr", "has-extract"}, // written by text2N4L
		{"has-frag", "charct-in"},     // written by text2N4L
		{"has-extract", "extract-fr"},
		{"charct-in", "has-frag"},
	} {
		def, ok := reg.Arrows[want.code]
		if !ok {
			t.Errorf("%q is not declared in SSTconfig", want.code)
			continue
		}
		if def.Unknown {
			t.Errorf("%q resolved as an unknown/synthesized arrow", want.code)
		}
		if def.MetaType != "contains" {
			t.Errorf("%q meta-type = %q, want %q", want.code, def.MetaType, "contains")
		}
		if def.Reverse != want.reverse {
			t.Errorf("%q reverse = %q, want %q", want.code, def.Reverse, want.reverse)
		}
		if def.LongName == "" || def.LongName == want.code {
			t.Errorf("%q has no human long name (got %q)", want.code, def.LongName)
		}
	}
}
