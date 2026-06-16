package content

import "testing"

func arrow(code string, registered bool) Arrow {
	return Arrow{Code: code, Target: "x", IsRegistered: registered, Line: 1}
}

// AugmentWithUnknownArrows injects a synthetic ArrowDef for each distinct
// unregistered code, leaves registered codes untouched, dedups, and is
// deterministic. Already-present codes are never overwritten.
func TestAugmentWithUnknownArrows(t *testing.T) {
	reg := &SSTRegistry{
		Arrows: map[string]ArrowDef{
			"contains": {Code: "contains", LongName: "contains", MetaType: "contains"},
		},
	}
	files := []*ParsedFile{
		{Chapters: []Chapter{{
			Nodes: []Node{
				{Arrows: []Arrow{arrow("contains", true), arrow("affects", false)}},
				{Arrows: []Arrow{arrow("affects", false)}, // duplicate unknown
					Dittos: []DittoLine{{Arrows: []Arrow{arrow("restores", false)}}}},
			},
		}}},
	}

	added := AugmentWithUnknownArrows(reg, files)
	if added != 2 {
		t.Fatalf("added = %d; want 2 (affects, restores)", added)
	}

	// Registered arrow untouched.
	if d := reg.Arrows["contains"]; d.Unknown {
		t.Errorf("registered 'contains' should not be flagged Unknown")
	}
	// Unknown arrows synthesized with code == long name, flagged.
	for _, code := range []string{"affects", "restores"} {
		d, ok := reg.Arrows[code]
		if !ok {
			t.Fatalf("expected synthesized ArrowDef for %q", code)
		}
		if !d.Unknown {
			t.Errorf("%q: Unknown = false; want true", code)
		}
		if d.LongName != code || d.Code != code {
			t.Errorf("%q: Code/LongName = %q/%q; want both %q", code, d.Code, d.LongName, code)
		}
		if d.MetaType == "" {
			t.Errorf("%q: MetaType empty; want a neutral type", code)
		}
	}

	// Idempotent: a second pass adds nothing.
	if again := AugmentWithUnknownArrows(reg, files); again != 0 {
		t.Errorf("second pass added %d; want 0 (idempotent)", again)
	}
}

// A nil registry is tolerated.
func TestAugmentWithUnknownArrows_NilRegistry(t *testing.T) {
	if n := AugmentWithUnknownArrows(nil, nil); n != 0 {
		t.Errorf("nil registry: got %d; want 0", n)
	}
}
