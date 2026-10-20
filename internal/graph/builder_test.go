package graph_test

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/content"
	"texere/internal/graph"

	SST "github.com/srosignoli/sstorytime-kv/pkg/SSTorytime"
)

// minimalRegistry creates a registry with the few arrows the test fixtures use.
func minimalRegistry() *content.SSTRegistry {
	reg := &content.SSTRegistry{
		Arrows:      make(map[string]content.ArrowDef),
		Annotations: make(map[string]content.AnnotDef),
	}
	reg.Arrows["means"] = content.ArrowDef{Code: "means", LongName: "means", MetaType: "express"}
	reg.Arrows["next"] = content.ArrowDef{Code: "next", LongName: "next", MetaType: "leadsto"}
	return reg
}

func writeN4L(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func resetSSTGlobals() {
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
}

// TestBuildFromCollection_BleveIndexCreated covers T042: a successful build
// must leave both the BadgerDB store (sst_data/) and the Bleve index (bleve/)
// on disk so SearchService.IsReady() can succeed.
func TestBuildFromCollection_BleveIndexCreated(t *testing.T) {
	dir := t.TempDir()
	writeN4L(t, dir, "tiny.n4l", "- Tiny\n\nalpha\n\"  (means) beta\n")

	resetSSTGlobals()
	if _, err := graph.BuildFromCollection(dir, minimalRegistry()); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "sst_data")); err != nil {
		t.Errorf("sst_data not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bleve")); err != nil {
		t.Errorf("bleve not created: %v", err)
	}
}

// TestBuildFromCollection_RebuildClearsStaleNodes covers T043: rebuilding a
// collection after a node is removed from the source must not leave the
// removed node visible in the Bleve index. This locks in the
// os.RemoveAll(blevePath) cleanup at builder.go:41.
func TestBuildFromCollection_RebuildClearsStaleNodes(t *testing.T) {
	dir := t.TempDir()
	reg := minimalRegistry()

	writeN4L(t, dir, "rebuild.n4l", "- Test\n\nwidgetfoo\n\"  (means) gadget\n\nkeeper\n\"  (means) survivor\n")

	resetSSTGlobals()
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("first build: %v", err)
	}

	// Sanity: widgetfoo is searchable after first build.
	idx, err := SST.OpenBleveIndex(filepath.Join(dir, "bleve"))
	if err != nil {
		t.Fatalf("open bleve: %v", err)
	}
	hits, err := idx.SearchByQuery("widgetfoo", 10)
	if err != nil {
		t.Fatalf("search widgetfoo (first build): %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("widgetfoo not found after first build; fixture is wrong")
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close bleve: %v", err)
	}

	// Remove widgetfoo from the source, leaving only keeper.
	writeN4L(t, dir, "rebuild.n4l", "- Test\n\nkeeper\n\"  (means) survivor\n")

	resetSSTGlobals()
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("second build: %v", err)
	}

	idx2, err := SST.OpenBleveIndex(filepath.Join(dir, "bleve"))
	if err != nil {
		t.Fatalf("reopen bleve: %v", err)
	}
	defer idx2.Close()
	hits2, err := idx2.SearchByQuery("widgetfoo", 10)
	if err != nil {
		t.Fatalf("search widgetfoo (second build): %v", err)
	}
	if len(hits2) != 0 {
		t.Errorf("widgetfoo still present after rebuild: %d hits", len(hits2))
	}
	keeperHits, err := idx2.SearchByQuery("keeper", 10)
	if err != nil {
		t.Fatalf("search keeper: %v", err)
	}
	if len(keeperHits) == 0 {
		t.Errorf("keeper missing after rebuild")
	}
}

// TestBuildFromCollection_PartialFailureRecovery covers T057 (FR-019): if a
// previous build was interrupted and left stale state in <dir>/bleve/ or
// <dir>/sst_data/, the next BuildFromCollection must wipe both directories
// cleanly so query results match a fresh build exactly.
//
// We simulate the interruption by writing junk into both directories between
// builds (BuildFromCollection cannot be made to panic mid-pass without a
// test-only injection point in production code, which we did not add).
func TestBuildFromCollection_PartialFailureRecovery(t *testing.T) {
	dir := t.TempDir()
	reg := minimalRegistry()

	body := "- Recovery\n\n" +
		"alphax\n\"  (means) one\n\n" +
		"betax\n\"  (means) two\n\n" +
		"gammax\n\"  (means) three\n\n" +
		"deltax\n\"  (means) four\n\n" +
		"epsilonx\n\"  (means) five\n"
	writeN4L(t, dir, "rec.n4l", body)

	// Build #1 — capture canonical query results.
	resetSSTGlobals()
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("build 1: %v", err)
	}
	expected := captureTokenHitCounts(t, dir, []string{"alphax", "betax", "gammax", "deltax", "epsilonx"})

	// Simulate an interrupted Build #2: corrupt both indexes with stray files
	// the way a panic mid-pass might leave them. The next BuildFromCollection
	// must clear these via os.RemoveAll before writing.
	if err := os.WriteFile(filepath.Join(dir, "bleve", "stale.junk"), []byte("garbage"), 0644); err != nil {
		t.Fatalf("seed stale bleve file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sst_data", "stale.junk"), []byte("garbage"), 0644); err != nil {
		t.Fatalf("seed stale sst_data file: %v", err)
	}

	// Build #3 — clean rebuild from corrupted prior state.
	resetSSTGlobals()
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("build 3: %v", err)
	}
	got := captureTokenHitCounts(t, dir, []string{"alphax", "betax", "gammax", "deltax", "epsilonx"})

	if len(expected) != len(got) {
		t.Fatalf("token-set mismatch: want %d entries, got %d", len(expected), len(got))
	}
	for tok, want := range expected {
		if got[tok] != want {
			t.Errorf("token %q: hit count drift after recovery: want %d, got %d", tok, want, got[tok])
		}
	}

	// Stale junk files must not survive the rebuild.
	if _, err := os.Stat(filepath.Join(dir, "bleve", "stale.junk")); !os.IsNotExist(err) {
		t.Errorf("stale.junk in bleve/ survived rebuild: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sst_data", "stale.junk")); !os.IsNotExist(err) {
		t.Errorf("stale.junk in sst_data/ survived rebuild: %v", err)
	}
}

func captureTokenHitCounts(t *testing.T, dir string, tokens []string) map[string]int {
	t.Helper()
	idx, err := SST.OpenBleveIndex(filepath.Join(dir, "bleve"))
	if err != nil {
		t.Fatalf("open bleve: %v", err)
	}
	defer idx.Close()
	out := make(map[string]int, len(tokens))
	for _, tok := range tokens {
		hits, err := idx.SearchByQuery(tok, 10)
		if err != nil {
			t.Fatalf("search %q: %v", tok, err)
		}
		out[tok] = len(hits)
	}
	return out
}

// TestBuildFromCollection_ChainLinks: "A (next) B (next) C" builds A→B and
// B→C, never A→C (upstream N4L reads a line as a chain).
func TestBuildFromCollection_ChainLinks(t *testing.T) {
	dir := t.TempDir()
	writeN4L(t, dir, "chain.n4l", "- Chain\n\nstep one (next) step two (next) step three\n")

	resetSSTGlobals()
	if _, err := graph.BuildFromCollection(dir, minimalRegistry()); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}
	out := map[string][]string{} // node -> outgoing link targets (forward arrows only)
	for _, mem := range SST.NODE_CACHE {
		n := SST.GetMemoryNodeFromPtr(mem)
		for st, links := range n.I {
			if st <= SST.ST_ZERO {
				continue // inverse readings
			}
			for _, l := range links {
				out[n.S] = append(out[n.S], SST.GetNodeTxtFromPtr(l.Dst))
			}
		}
	}
	has := func(from, to string) bool {
		for _, x := range out[from] {
			if x == to {
				return true
			}
		}
		return false
	}
	if !has("step one", "step two") || !has("step two", "step three") {
		t.Errorf("chain links missing: %v", out)
	}
	if has("step one", "step three") {
		t.Errorf("step one links to step three: a line is a chain, not a star (%v)", out)
	}
}
