package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"texere/internal/content"
)

func TestSyncAssignsIDsUpsertsAndRetires(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	// A cloze node with NO # @id — Sync must inject one and create a card.
	body := "- Ch\n# @cloze\nSystem 1 is {fast}\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	res, err := Sync(store, []string{p}, reg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Added != 1 || res.Total != 1 {
		t.Fatalf("res=%+v want Added=1 Total=1", res)
	}
	// The file now contains an injected "# @id".
	after, _ := os.ReadFile(p)
	if !strings.Contains(string(after), "# @id ") {
		t.Fatalf("expected an injected # @id in file:\n%s", after)
	}
	// InjectedPaths must list the file on the first sync.
	if len(res.InjectedPaths) != 1 || res.InjectedPaths[0] != p {
		t.Fatalf("res.InjectedPaths=%v want [%s]", res.InjectedPaths, p)
	}
	// Re-syncing the same file adds nothing.
	res2, _ := Sync(store, []string{p}, reg)
	if res2.Added != 0 || res2.Total != 1 {
		t.Fatalf("res2=%+v want Added=0 Total=1", res2)
	}
	// InjectedPaths must be empty on re-sync (ids already present).
	if len(res2.InjectedPaths) != 0 {
		t.Fatalf("res2.InjectedPaths=%v want empty", res2.InjectedPaths)
	}
	// Removing the cloze marker retires the card on next sync.
	if err := os.WriteFile(p, []byte("- Ch\nSystem 1 is fast\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res3, _ := Sync(store, []string{p}, reg)
	if res3.Retired != 1 {
		t.Fatalf("res3=%+v want Retired=1", res3)
	}
	if st, _ := store.CountStats(time.Now()); st.Total != 0 {
		t.Fatalf("active total=%d want 0 after retire", st.Total)
	}
}

func TestSyncDiscoversAndRetiresExtracts(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	// A (from) extract node WITH an id, no card markers.
	// The (from) arrow is inline on the same node line so GenerateExtracts
	// finds a node with ID != "", StudyMeta == nil, and a from-arrow.
	body := "- Ch\n# @id e1\nSystem 1 is fast (from) \"sources/f.txt char=0\"\n"
	os.WriteFile(p, []byte(body), 0o644)
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{"from": {Code: "from", LongName: "from"}}}
	store, _ := Open(dir)
	defer store.Close()

	res, err := Sync(store, []string{p}, reg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.ExtractsAdded != 1 {
		t.Fatalf("ExtractsAdded=%d want 1", res.ExtractsAdded)
	}
	if n, _ := store.CountDueExtracts(time.Now()); n != 1 {
		t.Fatalf("due extracts=%d want 1", n)
	}

	// Promote it: add # @card → on next sync the extract retires and an item appears.
	body2 := "- Ch\n# @id e1\n# @card\nSystem 1 is fast (from) \"sources/f.txt char=0\"\n"
	os.WriteFile(p, []byte(body2), 0o644)
	res2, _ := Sync(store, []string{p}, reg)
	if res2.ExtractsRetired != 1 {
		t.Fatalf("ExtractsRetired=%d want 1", res2.ExtractsRetired)
	}
	if n, _ := store.CountDueExtracts(time.Now()); n != 0 {
		t.Fatalf("due extracts after promote=%d want 0", n)
	}
}

func TestSyncMixedCardAndExtractFileIdempotent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mix.n4l")
	// An un-ID'd # @card node ABOVE an un-ID'd (from) extract node.
	body := "- Ch\n# @card\nitem front (means) \"item back\"\nextracted sentence (from) \"sources/x.txt char=0\"\n"
	os.WriteFile(p, []byte(body), 0o644)
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{
		"means": {Code: "means", LongName: "means", Studyable: true},
		"from":  {Code: "from", LongName: "from"},
	}}
	store, _ := Open(dir)
	defer store.Close()

	if _, err := Sync(store, []string{p}, reg); err != nil {
		t.Fatal(err)
	}
	// Exactly two # @id lines (one per node), no double-IDs.
	after1, _ := os.ReadFile(p)
	if n := strings.Count(string(after1), "# @id "); n != 2 {
		t.Fatalf("after sync 1: %d '# @id' lines want 2:\n%s", n, after1)
	}
	// Second sync is idempotent: no new ids added, no new extract.
	res2, err := Sync(store, []string{p}, reg)
	if err != nil {
		t.Fatal(err)
	}
	after2, _ := os.ReadFile(p)
	if string(after1) != string(after2) {
		t.Fatalf("Sync not idempotent — file changed on 2nd sync:\n--- after1 ---\n%s\n--- after2 ---\n%s", after1, after2)
	}
	if res2.ExtractsAdded != 0 {
		t.Fatalf("2nd sync ExtractsAdded=%d want 0", res2.ExtractsAdded)
	}
}

func TestSyncDiscoversRealAppendExtract(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "k.n4l")
	if _, err := content.CreateReadingNotes(notes, "sources/k.txt", "K"); err != nil {
		t.Fatal(err)
	}
	if _, err := content.AppendExtract(notes, "System 1 is fast and automatic", "sources/k.txt char=10"); err != nil {
		t.Fatal(err)
	}
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{"from": {Code: "from", LongName: "from"}}}
	store, _ := Open(dir)
	defer store.Close()
	res, err := Sync(store, []string{notes}, reg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.ExtractsAdded != 1 {
		t.Fatalf("ExtractsAdded=%d want 1 (real AppendExtract output must be discoverable)", res.ExtractsAdded)
	}
	if n, _ := store.CountDueExtracts(time.Now()); n != 1 {
		t.Fatalf("due extracts=%d want 1", n)
	}
}
