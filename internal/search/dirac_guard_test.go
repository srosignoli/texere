package search_test

import (
	"strings"
	"testing"
)

// A malformed <...> token used to reach upstream's DiracNotation, which prints
// "Bad Dirac notation" and calls os.Exit(-1) — killing the whole process. In the
// app that means the window vanishes and unsaved editor buffers are lost.
//
// These tests are unusual: if the guard is missing they do not merely fail, they
// terminate the test binary. A crashed run IS the failure signal.
func TestMalformedDiracDoesNotKillTheProcess(t *testing.T) {
	svc := buildSequenceGraph(t)

	for _, q := range []string{
		"step <-> 1", // proximity, which query-grammar.js documents
		"a <2> b",    // the <N> proximity form
		"<justone>",  // no pipes at all
		"<a|b|c|d>",  // four parts
		"<>",         // empty
	} {
		t.Run(q, func(t *testing.T) {
			res, err := svc.Search(q)
			if err != nil {
				t.Fatalf("Search returned a Go error: %v", err)
			}
			if res == nil {
				t.Fatal("nil result")
			}
			if res.GraphError == "" {
				t.Fatalf("expected a GraphError explaining the bad token; got none (nodes=%d)", len(res.Nodes))
			}
			if !strings.Contains(strings.ToLower(res.GraphError), "<") {
				t.Errorf("GraphError %q does not mention the offending syntax", res.GraphError)
			}
		})
	}
}

// The two forms that ARE valid Dirac notation must keep working — the grammar
// documents them as shorthand for \from X \to Y.
func TestValidDiracStillRuns(t *testing.T) {
	svc := buildSequenceGraph(t)

	for _, q := range []string{"<step 1|step 3>", "<step 1|tutorial|step 3>"} {
		t.Run(q, func(t *testing.T) {
			res, err := svc.Search(q)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if res.GraphError != "" {
				t.Fatalf("valid Dirac notation was rejected: %q", res.GraphError)
			}
		})
	}
}
