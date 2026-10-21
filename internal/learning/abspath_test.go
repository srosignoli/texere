package learning

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pathStore is a real Store whose rows are seeded the way callers do today,
// with paths relative to the Collection.
func pathStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func TestGetReadingReturnsAnAbsoluteNotesPath(t *testing.T) {
	s, dir := pathStore(t)
	if err := s.UpsertReading(ReadingElement{ID: "r1", RefPath: "sources/x.txt", NotesPath: "reading/x.n4l", Title: "x", IntervalDays: 1}); err != nil {
		t.Fatal(err)
	}
	r, ok, err := s.GetReading("r1")
	if err != nil || !ok {
		t.Fatalf("GetReading: ok=%v err=%v", ok, err)
	}
	if want := filepath.Join(dir, "reading", "x.n4l"); r.NotesPath != want {
		t.Errorf("NotesPath = %q, want %q", r.NotesPath, want)
	}
	if r.RefPath != "sources/x.txt" {
		t.Errorf("RefPath = %q, want it left relative", r.RefPath)
	}
}

func TestListAndDueReadingReturnAbsoluteNotesPaths(t *testing.T) {
	s, dir := pathStore(t)
	if err := s.UpsertReading(ReadingElement{ID: "r1", RefPath: "sources/x.txt", NotesPath: "reading/x.n4l", Title: "x", IntervalDays: 1}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "reading", "x.n4l")
	list, err := s.ListReading()
	if err != nil || len(list) != 1 || list[0].NotesPath != want {
		t.Errorf("ListReading = %+v, %v; want one row at %q", list, err, want)
	}
	due, err := s.DueReading(time.Now(), 5)
	if err != nil || len(due) != 1 || due[0].NotesPath != want {
		t.Errorf("DueReading = %+v, %v; want one row at %q", due, err, want)
	}
}

func TestExtractReadsReturnAbsoluteFilePaths(t *testing.T) {
	s, dir := pathStore(t)
	if _, err := s.UpsertExtract(Extract{ID: "e1", FilePath: "reading/x.n4l", Text: "t", Active: true}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "reading", "x.n4l")
	e, ok, err := s.GetExtract("e1")
	if err != nil || !ok || e.FilePath != want {
		t.Errorf("GetExtract = %+v, ok=%v, %v; want FilePath %q", e, ok, err, want)
	}
	due, err := s.DueExtracts(time.Now(), 5)
	if err != nil || len(due) != 1 || due[0].FilePath != want {
		t.Errorf("DueExtracts = %+v, %v; want one row at %q", due, err, want)
	}
}

// rawColumn reads a stored value directly, as the migrations do.
func rawColumn(t *testing.T, s *Store, query, id string) string {
	t.Helper()
	var v string
	if err := s.db.QueryRow(query, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// The columns stay relative so a Collection survives being moved; an absolute
// path written in is stored relative.
func TestUpsertReadingStoresAnAbsoluteNotesPathRelative(t *testing.T) {
	s, dir := pathStore(t)
	abs := filepath.Join(dir, "reading", "x.n4l")
	if err := s.UpsertReading(ReadingElement{ID: "r1", RefPath: "sources/x.txt", NotesPath: abs, Title: "x", IntervalDays: 1}); err != nil {
		t.Fatal(err)
	}
	if got := rawColumn(t, s, `SELECT notes_path FROM reading_elements WHERE id=?`, "r1"); got != "reading/x.n4l" {
		t.Errorf("stored notes_path = %q, want reading/x.n4l", got)
	}
}

func TestUpsertItemAndExtractStoreRelative(t *testing.T) {
	s, dir := pathStore(t)
	abs := filepath.Join(dir, "notes", "n.n4l")
	if _, err := s.UpsertItem(GeneratedCard{NodeID: "n1", FilePath: abs, CardType: CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertExtract(Extract{ID: "e1", FilePath: abs, Text: "t", Active: true}); err != nil {
		t.Fatal(err)
	}
	if got := rawColumn(t, s, `SELECT file_path FROM items WHERE id=?`, "n1::cloze::0"); got != "notes/n.n4l" {
		t.Errorf("stored items.file_path = %q, want notes/n.n4l", got)
	}
	if got := rawColumn(t, s, `SELECT file_path FROM extracts WHERE id=?`, "e1"); got != "notes/n.n4l" {
		t.Errorf("stored extracts.file_path = %q, want notes/n.n4l", got)
	}
}

// Sync passes the absolute path of the file it just parsed.
func TestRetireMissingTakesAnAbsolutePath(t *testing.T) {
	s, dir := pathStore(t)
	abs := filepath.Join(dir, "notes", "n.n4l")
	for _, n := range []string{"keep", "gone"} {
		if _, err := s.UpsertItem(GeneratedCard{NodeID: n, FilePath: abs, CardType: CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpsertExtract(Extract{ID: n, FilePath: abs, Text: "t", Active: true}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.RetireMissing(abs, map[string]bool{"keep::cloze::0": true}); err != nil || n != 1 {
		t.Errorf("RetireMissing retired %d, err %v; want 1", n, err)
	}
	if n, err := s.RetireMissingExtracts(abs, map[string]bool{"keep": true}); err != nil || n != 1 {
		t.Errorf("RetireMissingExtracts retired %d, err %v; want 1", n, err)
	}
}

// The Store only ever holds paths inside its Collection.
func TestAWriteOutsideTheCollectionIsRefused(t *testing.T) {
	s, _ := pathStore(t)
	outside := filepath.Join(t.TempDir(), "elsewhere.n4l")
	if err := s.UpsertReading(ReadingElement{ID: "r1", RefPath: "sources/x.txt", NotesPath: outside, Title: "x", IntervalDays: 1}); err == nil {
		t.Error("UpsertReading accepted a notes path outside the Collection")
	}
	if _, ok, _ := s.GetReading("r1"); ok {
		t.Error("the refused Source was stored anyway")
	}
}

func TestCardReadsReturnAbsoluteFilePaths(t *testing.T) {
	s, dir := pathStore(t)
	if _, err := s.UpsertItem(GeneratedCard{NodeID: "n1", FilePath: "notes/n.n4l", CardType: CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "notes", "n.n4l")
	c, ok, err := s.GetCardView("n1::cloze::0")
	if err != nil || !ok || c.FilePath != want {
		t.Errorf("GetCardView = %+v, ok=%v, %v; want FilePath %q", c, ok, err, want)
	}
	due, err := s.DueQueue(time.Now(), 5)
	if err != nil || len(due) != 1 || due[0].FilePath != want {
		t.Errorf("DueQueue = %+v, %v; want one card at %q", due, err, want)
	}
}

// seedEscaping writes one row of each kind whose stored path lies outside the
// Collection: the legacy shape a copied Collection can still hold.
func seedEscaping(t *testing.T, s *Store, id string) {
	t.Helper()
	outside := "/elsewhere/" + id + ".n4l"
	for _, q := range []string{
		`INSERT INTO reading_elements (id, ref_path, notes_path, title, interval_days, done, created_at) VALUES (?, 'sources/x.txt', ?, 't', 1, 0, 0)`,
		`INSERT INTO extracts (id, file_path, text, active, created_at) VALUES (?, ?, 't', 1, 0)`,
	} {
		if _, err := s.db.Exec(q, id, outside); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`INSERT INTO items (id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES (?, ?, ?, 'cloze', 'f', 'b', 1, 0, 0, 0, 0)`, id, id, outside); err != nil {
		t.Fatal(err)
	}
}

func TestASingleRowThatEscapesTheCollectionIsAnError(t *testing.T) {
	s, _ := pathStore(t)
	seedEscaping(t, s, "bad")
	if _, _, err := s.GetReading("bad"); err == nil || !strings.Contains(err.Error(), `"bad"`) {
		t.Errorf("GetReading err = %v, want an error naming the row", err)
	}
	if _, _, err := s.GetExtract("bad"); err == nil || !strings.Contains(err.Error(), `"bad"`) {
		t.Errorf("GetExtract err = %v, want an error naming the row", err)
	}
	if _, _, err := s.GetCardView("bad"); err == nil || !strings.Contains(err.Error(), `"bad"`) {
		t.Errorf("GetCardView err = %v, want an error naming the row", err)
	}
}

// One stale row must not empty the library or the queue: lists skip it.
func TestManyRowReadsSkipARowThatEscapes(t *testing.T) {
	s, _ := pathStore(t)
	seedEscaping(t, s, "bad")
	if err := s.UpsertReading(ReadingElement{ID: "good", RefPath: "sources/g.txt", NotesPath: "reading/g.n4l", Title: "g", IntervalDays: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertExtract(Extract{ID: "good", FilePath: "reading/g.n4l", Text: "t", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertItem(GeneratedCard{NodeID: "good", FilePath: "notes.n4l", CardType: CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if l, err := s.ListReading(); err != nil || len(l) != 1 || l[0].ID != "good" {
		t.Errorf("ListReading = %+v, %v; want only good", l, err)
	}
	if l, err := s.DueReading(now, 10); err != nil || len(l) != 1 || l[0].ID != "good" {
		t.Errorf("DueReading = %+v, %v; want only good", l, err)
	}
	if l, err := s.DueExtracts(now, 10); err != nil || len(l) != 1 || l[0].ID != "good" {
		t.Errorf("DueExtracts = %+v, %v; want only good", l, err)
	}
	if l, err := s.DueQueue(now, 10); err != nil || len(l) != 1 || l[0].ID != "good::cloze::0" {
		t.Errorf("DueQueue = %+v, %v; want only good", l, err)
	}
}

// The Study Queue asks each due query for one row. If the first-due row escapes,
// skipping it must not leave the answer empty.
func TestADueQueryFillsItsLimitPastAnEscapingRow(t *testing.T) {
	s, _ := pathStore(t)
	seedEscaping(t, s, "bad")
	// Due long ago, so "bad" is first in due order in every table.
	for _, q := range []string{`UPDATE reading_elements SET due=1 WHERE id='bad'`, `UPDATE extracts SET due=1 WHERE id='bad'`, `UPDATE items SET due=1 WHERE id='bad'`} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpsertReading(ReadingElement{ID: "good", RefPath: "sources/g.txt", NotesPath: "reading/g.n4l", Title: "g", IntervalDays: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertExtract(Extract{ID: "good", FilePath: "reading/g.n4l", Text: "t", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertItem(GeneratedCard{NodeID: "good", FilePath: "notes.n4l", CardType: CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if l, err := s.DueReading(now, 1); err != nil || len(l) != 1 || l[0].ID != "good" {
		t.Errorf("DueReading(1) = %+v, %v; want good", l, err)
	}
	if l, err := s.DueExtracts(now, 1); err != nil || len(l) != 1 || l[0].ID != "good" {
		t.Errorf("DueExtracts(1) = %+v, %v; want good", l, err)
	}
	if l, err := s.DueQueue(now, 1); err != nil || len(l) != 1 || l[0].ID != "good::cloze::0" {
		t.Errorf("DueQueue(1) = %+v, %v; want good", l, err)
	}
}
