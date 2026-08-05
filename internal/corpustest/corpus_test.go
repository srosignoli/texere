package corpustest

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"texere/internal/content"
	"texere/internal/graph"
	"texere/internal/search"
)

// This lives in its OWN PACKAGE on purpose. sstorytime-kv keeps process-global
// state (SST.NODE_DIRECTORY, NODE_CACHE, MemoryInit) that every SearchService
// resets when it opens, so a service held open across tests corrupts every
// other service in the same process. Held inside internal/search this fixture
// broke five unrelated tests — TestPath_ByName, TestPath_WithDepth,
// TestNotes_DittoFlatten, TestSearchService_ResolvePathEndpoints_Bleve and
// TestSearchService_ForwardCone — which are green on main. Go runs each package
// in its own process, which is the only isolation available.
//
// The corpus is nine files from sstorytime-kv/examples, chosen for domain
// spread and route coverage rather than size. It exists so the few-shot
// examples fed to the LLM are proven to RETURN RESULTS, not merely to parse: a
// query can be syntactically valid, semantically sensible, and still return
// nothing for pure data reasons, and only running it catches that.
//
// Built once per package run — ~0.9s — and shared, because rebuilding per test
// would dominate the suite.
var (
	corpusOnce sync.Once
	corpusSvc  *search.SearchService
	corpusErr  error
)

func corpus(t *testing.T) *search.SearchService {
	t.Helper()
	corpusOnce.Do(func() {
		dir, err := os.MkdirTemp("", "corpus")
		if err != nil {
			corpusErr = err
			return
		}
		src := "corpus"
		entries, err := os.ReadDir(src)
		if err != nil {
			corpusErr = err
			return
		}
		for _, e := range entries {
			b, rerr := os.ReadFile(filepath.Join(src, e.Name()))
			if rerr != nil {
				corpusErr = rerr
				return
			}
			if werr := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); werr != nil {
				corpusErr = werr
				return
			}
		}
		reg, _ := content.LoadRegistry("corpus_sstconfig")
		if _, berr := graph.BuildFromCollection(dir, reg); berr != nil {
			corpusErr = berr
			return
		}
		corpusSvc = search.NewSearchService(dir, reg)
	})
	if corpusErr != nil {
		t.Fatalf("corpus build: %v", corpusErr)
	}
	return corpusSvc
}

func TestCorpusBuilds(t *testing.T) {
	svc := corpus(t)
	res, err := svc.Search(`\chapter any`)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Listings) == 0 {
		t.Fatal(`\chapter any returned no listings — the corpus did not ingest`)
	}
	t.Logf("corpus chapters: %d", len(res.Listings))
}

// The corpus legitimately references arrows absent from SSTconfig
// (extract-fr, has-frag, "consists of"). Those produce parse errors without
// blocking ingestion, so the build must NOT fail on them — failing would make
// this test hostage to upstream's authoring choices.
func TestCorpusToleratesUnknownArrows(t *testing.T) {
	svc := corpus(t)
	res, err := svc.Search("pod")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Fatal(`"pod" returned nothing; ingestion is broken despite unknown arrows`)
	}
}

// Every query the prompt teaches must return something on the corpus. The
// expected kind is asserted too: a query that starts returning listings where
// it used to return nodes has changed meaning, and the prompt's question would
// no longer describe it.
func TestQueryExamplesReturnResults(t *testing.T) {
	svc := corpus(t)
	const (
		nodes = "nodes"
		list  = "listings"
		notes = "notes"
		chain = "structural"
	)
	for _, tc := range []struct{ query, want string }{
		{"pod", nodes},
		{`pod \chapter Kubernetes`, nodes},
		{`\notes \chapter neuroscience`, notes},
		{`waves \context oscillations`, nodes},
		{`\context oscillations`, list},
		{`\notes \chapter neuroscience \context memory`, notes},
		{`\from start \to target 1`, chain},
		{`\from door`, chain},
		{`"thank you"`, nodes},
		{`brain&!wave`, nodes},
		{`neuro:*`, nodes},
		{`!pod!`, nodes},
		{`\chapter any`, list},
		{`\arrow any`, list},
		{`pod \radius 1`, nodes},
		{`\story Mary`, chain},
		{`\sequence \chapter astronomy`, chain},
		{`\story any`, chain},
		{`\notes \context "differentiate under integral"`, notes},
	} {
		t.Run(tc.query, func(t *testing.T) {
			res, err := svc.Search(tc.query)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			got := map[string]int{
				nodes: len(res.Nodes), list: len(res.Listings),
				notes: len(res.Notes), chain: len(res.Structural),
			}
			if got[tc.want] == 0 {
				t.Fatalf("expected %s, got nodes=%d listings=%d notes=%d structural=%d — "+
					"this example teaches the model a query that returns nothing",
					tc.want, got[nodes], got[list], got[notes], got[chain])
			}
		})
	}
}
