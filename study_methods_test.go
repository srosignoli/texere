package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"texere/internal/content"
	"texere/internal/learning"
)

func TestPromoteExtractCreatesItemRetiresExtract(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	p := filepath.Join(dir, "reading", "k.n4l")
	// The (from) arrow must be inline on the same node line — separate-line (from)
	// is parsed as a root node with empty text and cannot produce a cloze card.
	// This matches the format produced by content.AppendExtract.
	os.WriteFile(p, []byte("- K\n# @id e1\nSystem 1 is {{fast}} (from) \"sources/k.txt char=0\"\n"), 0o644)
	store, _ := learning.Open(dir)
	defer store.Close()
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{"from": {Code: "from", LongName: "from"}}}
	a := &App{collectionDir: dir, learnStore: store, registry: reg}

	// Discover the extract.
	if _, err := learning.Sync(store, []string{p}, reg); err != nil {
		t.Fatal(err)
	}
	q, err := a.GetExtractQueue(10)
	if err != nil || len(q) != 1 {
		t.Fatalf("GetExtractQueue len=%d err=%v want 1", len(q), err)
	}
	// Promote to cloze.
	if err := a.PromoteExtract(q[0].ID, "cloze"); err != nil {
		t.Fatalf("PromoteExtract: %v", err)
	}
	body, _ := os.ReadFile(p)
	if !strings.Contains(string(body), "# @cloze") {
		t.Fatalf("marker not written:\n%s", body)
	}
	// Extract retired, an item now exists.
	if n, _ := store.CountDueExtracts(timeNowForTest()); n != 0 {
		t.Fatalf("due extracts after promote=%d want 0", n)
	}
	if st, _ := a.GetStudyCounts(); st.Recall < 1 {
		t.Fatalf("expected a recall item after promote, got %+v", st)
	}
}

func timeNowForTest() time.Time { return time.Now() }
