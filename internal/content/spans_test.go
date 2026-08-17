package content

import (
	"os"
	"path/filepath"
	"testing"
)

func spansOf(t *testing.T, src string) []NodeSpan {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.n4l")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := &SSTRegistry{
		Arrows:      map[string]ArrowDef{"means": {Code: "means", MetaType: "test"}},
		Annotations: map[string]AnnotDef{},
	}
	pf, _ := ParseFile(path, reg)
	if pf == nil {
		t.Fatal("ParseFile returned nil")
	}
	return NodeSpans(pf)
}

// Node.ID is the bare short id (see nodeid.go), so a span's ID matches shortID
// exactly.
func findSpan(t *testing.T, spans []NodeSpan, shortID string) NodeSpan {
	t.Helper()
	for _, s := range spans {
		if s.ID == shortID {
			return s
		}
	}
	t.Fatalf("no span with id %q, in %+v", shortID, spans)
	return NodeSpan{}
}

// A node with no "# @id" cannot key a notebook, so it must not produce a span —
// otherwise the caret would resolve to an id of "" and peek nonsense.
func TestNodeSpansOmitsNodesWithoutAnID(t *testing.T) {
	spans := spansOf(t, "- C\n\nplain node\n\"  (means) something\n")
	if len(spans) != 0 {
		t.Fatalf("got %+v, want none", spans)
	}
}

func TestNodeSpansCoversTheNodeLine(t *testing.T) {
	src := "- C\n\n# @id abc\nNewton\n"
	s := findSpan(t, spansOf(t, src), "abc")
	if s.FromLine > 3 || s.ToLine < 4 {
		t.Fatalf("got %+v, want a span covering the marker (3) and node (4) lines", s)
	}
}

// Dittos are the node's continuation lines; a caret on one is inside that node.
func TestNodeSpansExtendsThroughDittos(t *testing.T) {
	src := "- C\n\n# @id abc\nNewton\n\"  (means) a physicist\n\"  (means) also a mathematician\n"
	s := findSpan(t, spansOf(t, src), "abc")
	if s.ToLine < 6 {
		t.Fatalf("ToLine = %d, want at least 6 (the last ditto)", s.ToLine)
	}
}

// The "# @id" line reads as part of the node it labels.
func TestNodeSpansStartsAtTheIDMarkerLine(t *testing.T) {
	src := "- C\n\n# @id abc\nNewton\n"
	s := findSpan(t, spansOf(t, src), "abc")
	if s.FromLine != 3 {
		t.Fatalf("FromLine = %d, want 3 (the # @id line)", s.FromLine)
	}
}

func TestNodeSpansHandlesConsecutiveNodes(t *testing.T) {
	src := "- C\n\n# @id one\nAlpha\n\n# @id two\nBeta\n"
	spans := spansOf(t, src)
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2: %+v", len(spans), spans)
	}
	a, b := findSpan(t, spans, "one"), findSpan(t, spans, "two")
	if a.ToLine >= b.FromLine {
		t.Fatalf("spans overlap: %+v then %+v", a, b)
	}
}

// Both paths must return an empty NON-NIL slice: this crosses into JSON in
// Task 2, where nil marshals to `null` rather than `[]`. The nil-ness is the
// point, so it is asserted separately from the length — a combined
// `got != nil && len(got) != 0` short-circuits to false on a nil return and
// silently passes the regression it exists to catch.
func TestNodeSpansEmptyInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spans []NodeSpan
	}{
		{"nil ParsedFile", NodeSpans(nil)},
		{"file with no nodes", spansOf(t, "- C\n\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.spans == nil {
				t.Fatal("returned a nil slice; want empty non-nil, which marshals to [] not null")
			}
			if len(tc.spans) != 0 {
				t.Fatalf("got %+v, want no spans", tc.spans)
			}
		})
	}
}

// Node.ID (now the bare short id — see nodeid.go) is what GetNotebookByExtract
// and GetExtract are keyed on, so a span must carry it verbatim. Checked
// without going through findSpan's own ID match, so this isn't just
// re-asserting that helper's matching logic.
func TestNodeSpansCarriesNodeIDVerbatim(t *testing.T) {
	spans := spansOf(t, "- C\n\n# @id abc\nNewton\n")
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1: %+v", len(spans), spans)
	}
	if spans[0].ID != "abc" {
		t.Fatalf("ID = %q, want the node's short id %q", spans[0].ID, "abc")
	}
}
