package explorer

import (
	"testing"

	"texere/internal/content"
	"texere/internal/search"
)

// An arrow whose code is flagged Unknown in the registry must produce a
// ResultEdge marked Unknown, so the explorer can render it distinctly.
func TestBuildResultSet_UnknownArrowEdge(t *testing.T) {
	reg := &content.SSTRegistry{
		Arrows: map[string]content.ArrowDef{
			"contains": {Code: "contains", LongName: "contains", MetaType: "contains"},
			"affects":  {Code: "affects", LongName: "affects", MetaType: "similarity", Unknown: true},
		},
	}
	sr := &search.SearchResult{
		Notes: []search.FullNote{{
			Chapter: "Editor",
			Blocks: []search.NoteBlock{{
				NodeText: "disableRawMode",
				Arrows: []search.NoteArrow{
					{ArrowCode: "contains", ArrowLong: "contains", Target: "terminal_attributes"},
					{ArrowCode: "affects", ArrowLong: "affects", Target: "terminal_state"},
				},
			}},
		}},
	}

	rs := BuildResultSet(sr, reg, `\notes Editor`)

	var sawUnknown, sawKnown bool
	for _, e := range rs.Edges {
		switch e.ArrowCode {
		case "affects":
			sawUnknown = true
			if !e.Unknown {
				t.Errorf("'affects' edge: Unknown = false; want true")
			}
		case "contains":
			sawKnown = true
			if e.Unknown {
				t.Errorf("'contains' edge: Unknown = true; want false")
			}
		}
	}
	if !sawUnknown {
		t.Errorf("expected an 'affects' edge in the result")
	}
	if !sawKnown {
		t.Errorf("expected a 'contains' edge in the result")
	}
}
