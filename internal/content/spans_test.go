package content

import (
	"os"
	"path/filepath"
	"strings"
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

// Node IDs are fully qualified (`<path>:<chapter>:id:<short>`) and the path is a
// per-test temp dir, so match on the short id's suffix rather than hardcoding it.
// The leading colon prevents `:abc` matching a node whose short id ends in "abc".
func findSpan(t *testing.T, spans []NodeSpan, shortID string) NodeSpan {
	t.Helper()
	for _, s := range spans {
		if strings.HasSuffix(s.ID, ":"+shortID) {
			return s
		}
	}
	t.Fatalf("no span whose id ends in %q, in %+v", ":"+shortID, spans)
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

func TestNodeSpansEmptyInputs(t *testing.T) {
	if got := NodeSpans(nil); got != nil && len(got) != 0 {
		t.Fatalf("NodeSpans(nil) = %+v, want empty", got)
	}
	if got := spansOf(t, "- C\n\n"); len(got) != 0 {
		t.Fatalf("got %+v, want none for a file with no nodes", got)
	}
}

// The qualified id is what GetNotebookByExtract and GetExtract are keyed on, so a
// span must carry it verbatim. A short id here would make every peek miss and the
// pane would show the empty state forever.
func TestNodeSpansCarriesTheFullyQualifiedID(t *testing.T) {
	s := findSpan(t, spansOf(t, "- C\n\n# @id abc\nNewton\n"), "abc")
	if s.ID == "abc" {
		t.Fatal("span carries the short id; it must carry the fully-qualified Node.ID")
	}
	if !strings.Contains(s.ID, ":id:abc") {
		t.Fatalf("ID = %q, want the qualified form <path>:<chapter>:id:abc", s.ID)
	}
}
