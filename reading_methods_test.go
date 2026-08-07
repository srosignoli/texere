package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"texere/internal/content"
	"texere/internal/learning"
)

func TestCreateExtractAppendsAndAdvances(t *testing.T) {
	dir := t.TempDir()
	// Set up a source + reading element by hand (no dialog).
	os.MkdirAll(filepath.Join(dir, "sources"), 0o755)
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	os.WriteFile(filepath.Join(dir, "sources", "k.txt"), []byte("System 1 is fast and automatic. System 2 is slow."), 0o644)
	notes := filepath.Join(dir, "reading", "k.n4l")
	refID, err := content.CreateReadingNotes(notes, "sources/k.txt", "k")
	if err != nil {
		t.Fatal(err)
	}
	store, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	store.UpsertReading(learning.ReadingElement{ID: refID, RefPath: "sources/k.txt", NotesPath: notes, Title: "k", IntervalDays: 1})

	a := &App{collectionDir: dir, learnStore: store, registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}}
	defer store.Close()

	ev, err := a.CreateExtract(refID, "System 1 is fast", "char=0", true)
	if err != nil || ev == nil || ev.ID == "" {
		t.Fatalf("CreateExtract ev=%+v err=%v", ev, err)
	}
	body, _ := os.ReadFile(notes)
	if !strings.Contains(string(body), `(from) "sources/k.txt char=0"`) {
		t.Fatalf("extract anchor missing:\n%s", body)
	}
	// Reschedule "done" then list reflects it.
	if _, err := a.RescheduleReading(refID, "done"); err != nil {
		t.Fatalf("RescheduleReading: %v", err)
	}
	list, _ := a.GetReadingList()
	if len(list) != 1 || !list[0].Done {
		t.Fatalf("list=%+v want 1 done", list)
	}
}

func TestToReadingViewPdfPercent(t *testing.T) {
	r := learning.ReadingElement{ID: "p1", Medium: "pdf", Position: "page=5", ProjLen: 10}
	v := toReadingView(r)
	if v.Percent != 50 {
		t.Fatalf("PDF percent=%d want 50", v.Percent)
	}
}

func TestImportWebAndCreateExtractPosition(t *testing.T) {
	// Local server with a small article.
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>WT</title></head><body><article><h1>WT</h1>
			<p>A sufficiently long paragraph of article text so readability keeps it as the main content of this page here.</p>
			</article></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	store, _ := learning.Open(dir)
	defer store.Close()
	a := &App{collectionDir: dir, learnStore: store, registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{"from": {Code: "from", LongName: "from"}}}}

	rv, err := a.ImportWebSource(srv.URL + "/a")
	if err != nil || rv == nil {
		t.Fatalf("ImportWebSource rv=%v err=%v", rv, err)
	}
	if rv.Medium != "web" || rv.SourceURL != srv.URL+"/a" {
		t.Fatalf("medium=%q sourceUrl=%q", rv.Medium, rv.SourceURL)
	}
	if rv.Percent < 0 {
		t.Fatalf("Percent should be >= 0, got %d", rv.Percent)
	}
	// The saved html file exists and is script-free.
	html, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rv.RefPath)))
	if strings.Contains(string(html), "<script") {
		t.Fatalf("saved web html not sanitized")
	}
	// CreateExtract with a position string writes that exact anchor.
	if _, err := a.CreateExtract(rv.ID, "A long paragraph", "char=5", true); err != nil {
		t.Fatalf("CreateExtract: %v", err)
	}
	notes, _ := os.ReadFile(rv.NotesPath)
	if !strings.Contains(string(notes), `char=5`) {
		t.Fatalf("extract anchor missing char=5:\n%s", notes)
	}

	// The web pane loads the saved article via GetSourceHTML(refPath). refPath is
	// stored relative to the collection dir, so the method must join collectionDir
	// rather than reading it CWD-relative (regression: bare os.ReadFile fails here).
	got, err := a.GetSourceHTML(rv.RefPath)
	if err != nil {
		t.Fatalf("GetSourceHTML(%q): %v", rv.RefPath, err)
	}
	if !strings.Contains(got, "sufficiently long paragraph") {
		t.Fatalf("GetSourceHTML did not return saved article html:\n%s", got)
	}
}

// readingTreeFixture builds a real collection: a source, a scaffolded notes
// file, and a registered reading. The tree is read from the .n4l, so the tests
// have to write actual N4L rather than seed the extracts table.
func readingTreeFixture(t *testing.T) (*App, *learning.Store, string, string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sources"), 0o755)
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	os.WriteFile(filepath.Join(dir, "sources", "k.txt"), []byte("body text"), 0o644)
	notes := filepath.Join(dir, "reading", "k.n4l")
	refID, err := content.CreateReadingNotes(notes, "sources/k.txt", "k")
	if err != nil {
		t.Fatal(err)
	}
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.UpsertReading(learning.ReadingElement{
		ID: refID, RefPath: "sources/k.txt", NotesPath: notes, Title: "k",
		Position: "char=0", ProjLen: 10000, IntervalDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	a := &App{collectionDir: dir, learnStore: st,
		registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}}
	return a, st, refID, notes
}

func TestGetReadingExtractsOrdersByAnchorNumerically(t *testing.T) {
	a, _, refID, notes := readingTreeFixture(t)
	// Appended late-first, so a pass-through of file order would fail.
	if _, err := content.AppendExtract(notes, "later passage", "sources/k.txt char=100"); err != nil {
		t.Fatal(err)
	}
	if _, err := content.AppendExtract(notes, "early passage", "sources/k.txt char=9"); err != nil {
		t.Fatal(err)
	}

	got, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatal(err)
	}
	list := got[refID]
	if len(list) != 2 {
		t.Fatalf("got %d extracts, want 2: %+v", len(list), list)
	}
	// char=9 before char=100. Sorting the anchor strings would invert these.
	if !strings.HasPrefix(list[0].Text, "early") || !strings.HasPrefix(list[1].Text, "later") {
		t.Fatalf("wrong order: %q then %q — anchors must sort numerically, not lexically",
			list[0].Text, list[1].Text)
	}
	if list[0].Anchor != "sources/k.txt char=9" {
		t.Errorf("anchor = %q, want it passed through verbatim", list[0].Anchor)
	}
	if list[0].Promoted {
		t.Error("a plain extract must not be flagged promoted")
	}
}

// The reason the tree reads the file instead of the extracts table: promoting
// an extract to a card retires its row, and the tree still has to show it —
// it is a passage you took from this document either way.
func TestGetReadingExtractsIncludesPromotedCards(t *testing.T) {
	a, _, refID, notes := readingTreeFixture(t)
	if _, err := content.AppendExtract(notes, "a passage worth a card", "sources/k.txt char=50"); err != nil {
		t.Fatal(err)
	}
	before, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatal(err)
	}
	if len(before[refID]) != 1 {
		t.Fatalf("setup: got %d extracts, want 1", len(before[refID]))
	}

	// Promote it in the file, the way the process view does.
	lines, err := os.ReadFile(notes)
	if err != nil {
		t.Fatal(err)
	}
	nodeLine := 0
	for i, ln := range strings.Split(string(lines), "\n") {
		if strings.Contains(ln, "a passage worth a card") {
			nodeLine = i + 1
		}
	}
	if nodeLine == 0 {
		t.Fatal("setup: could not find the appended node")
	}
	if err := content.SeedBasicCardNode(notes, nodeLine, "a passage worth a card", "sources/k.txt char=50"); err != nil {
		t.Fatal(err)
	}

	got, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatal(err)
	}
	list := got[refID]
	if len(list) != 1 {
		t.Fatalf("promoting removed the passage from the tree: got %d, want 1: %+v", len(list), list)
	}
	if !list[0].Promoted {
		t.Error("a promoted passage must be flagged so the tree can mark it")
	}
	if list[0].Anchor != "sources/k.txt char=50" {
		t.Errorf("anchor = %q, want the promoted node's ditto anchor", list[0].Anchor)
	}
}

// Promotions made before card provenance was preserved stripped the (from)
// line, so the DB row is the only surviving record of the anchor.
func TestGetReadingExtractsFallsBackToTheStoredAnchor(t *testing.T) {
	a, st, refID, notes := readingTreeFixture(t)
	if _, err := content.AppendExtract(notes, "legacy card text", "sources/k.txt char=77"); err != nil {
		t.Fatal(err)
	}
	seen, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatal(err)
	}
	if len(seen[refID]) != 1 {
		t.Fatalf("setup: got %d, want 1", len(seen[refID]))
	}
	nodeID := seen[refID][0].ID
	if _, err := st.UpsertExtract(learning.Extract{
		ID: nodeID, FilePath: notes, Text: "legacy card text",
		FromAnchor: "sources/k.txt char=77", Due: time.Now(), Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Strip the (from) line, as the old promote did.
	body, _ := os.ReadFile(notes)
	var kept []string
	for _, ln := range strings.Split(string(body), "\n") {
		if strings.Contains(ln, "legacy card text") {
			ln = "legacy card text"
		}
		kept = append(kept, ln)
	}
	if err := os.WriteFile(notes, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatal(err)
	}
	if len(got[refID]) != 1 {
		t.Fatalf("a node whose (from) the file lost must fall back to the stored anchor; got %+v", got[refID])
	}
	if got[refID][0].Anchor != "sources/k.txt char=77" {
		t.Errorf("anchor = %q, want the stored one", got[refID][0].Anchor)
	}
}

func TestGetReadingExtractsEmptyIsNotNil(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	a := &App{learnStore: st, collectionDir: dir}
	got, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("returned a nil map; it crosses into JSON, where nil marshals to null")
	}
}

// A peek is a glance at one passage, not a reading position. Extracting during
// one must still create the extract but must not move progress — otherwise
// peeking an early extract and hitting Ctrl+E rewinds a half-read document,
// silently, in both the DB and the # @progress marker.
func peekExtractFixture(t *testing.T) (*App, *learning.Store, string, string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sources"), 0o755)
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	os.WriteFile(filepath.Join(dir, "sources", "k.txt"), []byte("System 1 is fast and automatic. System 2 is slow."), 0o644)
	notes := filepath.Join(dir, "reading", "k.n4l")
	refID, err := content.CreateReadingNotes(notes, "sources/k.txt", "k")
	if err != nil {
		t.Fatal(err)
	}
	store, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.UpsertReading(learning.ReadingElement{
		ID: refID, RefPath: "sources/k.txt", NotesPath: notes, Title: "k",
		Position: "char=5000", ProjLen: 10000, IntervalDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	a := &App{collectionDir: dir, learnStore: store, registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}}
	return a, store, refID, notes
}

func TestCreateExtractDuringAPeekDoesNotAdvanceProgress(t *testing.T) {
	a, store, refID, notes := peekExtractFixture(t)

	v, err := a.CreateExtract(refID, "an early passage", "char=200", false)
	if err != nil {
		t.Fatalf("CreateExtract during a peek failed: %v", err)
	}
	if v == nil || v.ID == "" {
		t.Fatal("the extract itself must still be created during a peek")
	}
	body, _ := os.ReadFile(notes)
	if !strings.Contains(string(body), "an early passage") {
		t.Fatalf("extract was not appended to the notes file:\n%s", body)
	}
	if strings.Contains(string(body), "char=200") && strings.Contains(string(body), "@progress char=200") {
		t.Fatalf("# @progress was advanced during a peek:\n%s", body)
	}

	got, ok, err := store.GetReading(refID)
	if err != nil || !ok {
		t.Fatalf("GetReading: %v, ok=%v", err, ok)
	}
	if got.Position != "char=5000" {
		t.Fatalf("position moved to %q during a peek; want it left at char=5000", got.Position)
	}
}

func TestCreateExtractWhileReadingAdvancesProgress(t *testing.T) {
	a, store, refID, _ := peekExtractFixture(t)

	if _, err := a.CreateExtract(refID, "a later passage", "char=7200", true); err != nil {
		t.Fatalf("CreateExtract: %v", err)
	}
	got, _, err := store.GetReading(refID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Position != "char=7200" {
		t.Fatalf("position = %q, want char=7200 — a normal extract still advances progress", got.Position)
	}
}
