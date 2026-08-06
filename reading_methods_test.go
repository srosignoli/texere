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

func TestGetReadingExtractsGroupsByNotesPath(t *testing.T) {
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{learnStore: st, collectionDir: dir}

	now := time.Now()
	if err := st.UpsertReading(learning.ReadingElement{
		ID: "r1", RefPath: "sources/x.txt", NotesPath: "notes/x.n4l",
		Title: "X", Position: "char=0", ProjLen: 100, Due: now, IntervalDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []learning.Extract{
		{ID: "e-late", FilePath: "notes/x.n4l", Text: "later passage", FromAnchor: "sources/x.txt char=100", Due: now, Active: true},
		{ID: "e-early", FilePath: "notes/x.n4l", Text: "early passage", FromAnchor: "sources/x.txt char=9", Due: now, Active: true},
		{ID: "e-elsewhere", FilePath: "notes/other.n4l", Text: "different doc", FromAnchor: "sources/y.txt char=1", Due: now, Active: true},
	} {
		if _, err := st.UpsertExtract(e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatal(err)
	}
	list := got["r1"]
	if len(list) != 2 {
		t.Fatalf("got %d extracts for r1, want 2 (grouping is by notes path): %+v", len(list), list)
	}
	// Document order, numerically: char=9 comes before char=100. Sorting the
	// anchors as strings would put "char=100" first and the tree would read out
	// of order for any document past its hundredth character.
	if list[0].ID != "e-early" || list[1].ID != "e-late" {
		t.Fatalf("wrong order: %s then %s — anchors must sort numerically, not lexically", list[0].ID, list[1].ID)
	}
	if list[0].Anchor != "sources/x.txt char=9" {
		t.Errorf("anchor = %q, want it passed through verbatim", list[0].Anchor)
	}
}

// PDF anchors use page=N. Nothing may assume char=.
func TestGetReadingExtractsPassesPageAnchorsThrough(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	a := &App{learnStore: st, collectionDir: dir}
	now := time.Now()
	st.UpsertReading(learning.ReadingElement{
		ID: "r-pdf", RefPath: "sources/p.pdf", NotesPath: "notes/p.n4l",
		Title: "P", Position: "page=1", ProjLen: 10, Due: now, IntervalDays: 1, Medium: "pdf",
	})
	st.UpsertExtract(learning.Extract{
		ID: "e-p", FilePath: "notes/p.n4l", Text: "from page 12",
		FromAnchor: "sources/p.pdf page=12", Due: now, Active: true,
	})

	got, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatal(err)
	}
	if len(got["r-pdf"]) != 1 || got["r-pdf"][0].Anchor != "sources/p.pdf page=12" {
		t.Fatalf("page anchor not passed through: %+v", got["r-pdf"])
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
