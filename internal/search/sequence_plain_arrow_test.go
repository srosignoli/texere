package search_test

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/graph"
	"texere/internal/search"
)

// buildSequencePlusPlainArrow builds a collection holding BOTH a real
// `:: _sequence_ ::` block and an ordinary leadsto arrow written outside any
// sequence context — `A (next) B`, which is a perfectly normal relationship.
func buildSequencePlusPlainArrow(t *testing.T) *search.SearchService {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"sequence.n4l", "plain_then.n4l"} {
		data, err := os.ReadFile(filepath.Join(testFixturesDir(t), name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	reg := testRegistry()
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("build: %v", err)
	}
	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)
	return svc
}

// A leadsto arrow written outside a `:: _sequence_ ::` block is an ordinary
// relationship, and hasSequenceMarker's own comment says so: "plain nodes with
// outgoing leadsto arrows are regular relationships, not sequences". Writing
// one must not disturb real sequences elsewhere in the collection.
//
// It did. A single `(then)` arrow anywhere in any file silently emptied \story
// and \sequence for the WHOLE collection — no error, no warning, and the
// sequence nodes still findable by text search. Found against the real
// sstorytime-kv examples: one such arrow in chinese.n4l, three in
// music-catalogue.n4l, each independently killing every sequence.
func TestSequenceSurvivesAPlainLeadstoArrowElsewhere(t *testing.T) {
	svc := buildSequencePlusPlainArrow(t)

	res, err := svc.Search(`\story step 1`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\story step 1: no chains — a plain (then) arrow in an unrelated ` +
			`file has emptied sequence search for the whole collection`)
	}
}

func TestSequenceAnySurvivesAPlainLeadstoArrowElsewhere(t *testing.T) {
	svc := buildSequencePlusPlainArrow(t)

	res, err := svc.Search(`\story any`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\story any: no chains — the Tutorial and Recipes sequences still ` +
			`exist in the graph but the route returns nothing`)
	}
}

// The sequence data must still be present either way; if this fails the
// problem is ingestion, not the sequence route.
func TestSequenceNodesAreStillIngested(t *testing.T) {
	svc := buildSequencePlusPlainArrow(t)

	res, err := svc.Search("boil water")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Fatal("sequence member text is missing from the graph — this is an ingestion failure")
	}
}
