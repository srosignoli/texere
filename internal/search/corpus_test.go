package search_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"texere/internal/content"
	"texere/internal/graph"
	"texere/internal/search"
)

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
		src := filepath.Join(testFixturesDir(t), "corpus")
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
		reg, _ := content.LoadRegistry(filepath.Join(testFixturesDir(t), "corpus_sstconfig"))
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
