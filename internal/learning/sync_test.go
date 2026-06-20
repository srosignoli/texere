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
