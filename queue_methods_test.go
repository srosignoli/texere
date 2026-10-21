package main

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/collection"
	"texere/internal/content"
	"texere/internal/learning"
)

// queueApp opens a Collection with one due Source (with real notes on disk)
// and one due Card.
func queueApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sources"), 0o755)
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	os.WriteFile(filepath.Join(dir, "sources", "k.txt"), []byte("text"), 0o644)
	sourceID, err := content.CreateReadingNotes(filepath.Join(dir, "reading", "k.n4l"), "sources/k.txt", "k")
	if err != nil {
		t.Fatal(err)
	}
	a, st := newTestApp(t, dir, nil)
	if err := st.UpsertReading(learning.ReadingElement{ID: sourceID, RefPath: "sources/k.txt", NotesPath: "reading/k.n4l", Title: "k", IntervalDays: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertItem(learning.GeneratedCard{NodeID: "n1", FilePath: "notes.n4l", CardType: learning.CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"}); err != nil {
		t.Fatal(err)
	}
	return a, sourceID
}

func TestNextTurnServesACardFirst(t *testing.T) {
	a, _ := queueApp(t)
	turn, err := a.NextTurn()
	if err != nil {
		t.Fatal(err)
	}
	if turn == nil || turn.Kind != "card" || turn.Card == nil || turn.Card.ID != "n1::cloze::0" {
		t.Fatalf("NextTurn() = %+v, want the Card", turn)
	}
}

// A Source opened from the Read tab is resumed by Next before the Mix, which
// would pick the Card. Goes through the Session's real Sitting Tracker.
func TestNextTurnResumesAnOpenSitting(t *testing.T) {
	a, sourceID := queueApp(t)
	if _, err := a.BeginSitting(sourceID, ""); err != nil {
		t.Fatal(err)
	}
	turn, err := a.NextTurn()
	if err != nil {
		t.Fatal(err)
	}
	if turn == nil || turn.Kind != "source" || turn.Source == nil || turn.Source.ID != sourceID {
		t.Fatalf("NextTurn() = %+v, want the open Source", turn)
	}
	if !filepath.IsAbs(turn.Source.NotesPath) {
		t.Errorf("Source NotesPath = %q, want absolute", turn.Source.NotesPath)
	}
}

func TestStudyMixRoundTripsThroughTheBindings(t *testing.T) {
	a, _ := queueApp(t)
	if m, err := a.GetStudyMix(); err != nil || m != learning.DefaultMix {
		t.Fatalf("GetStudyMix() = %+v, %v; want the default", m, err)
	}
	if err := a.SetStudyMix(learning.Mix{Source: 1, Extract: 2, Card: 3}); err != nil {
		t.Fatal(err)
	}
	if m, _ := a.GetStudyMix(); m != (learning.Mix{Source: 1, Extract: 2, Card: 3}) {
		t.Errorf("GetStudyMix() = %+v after SetStudyMix", m)
	}
	if err := a.SetStudyMix(learning.Mix{Card: -5}); err == nil {
		t.Error("SetStudyMix accepted a negative weight")
	}
}

// The tally belongs to the Session. After a switch, Collection B starts at
// zero (its first Turn is a Card again, the tie rule), and Collection A's
// current Turn is never served from B.
func TestANewSessionStartsANewTally(t *testing.T) {
	a, _ := queueApp(t)
	first, err := a.NextTurn()
	if err != nil || first == nil || first.Kind != "card" {
		t.Fatalf("Collection A first Turn = %+v, %v", first, err)
	}

	dirB := t.TempDir()
	if _, err := a.switchTo(dirB); err != nil {
		t.Fatal(err)
	}
	var st *learning.Store
	a.withCollection(func(h collection.Handles) error { st = h.Learning(); return nil })
	if err := st.UpsertReading(learning.ReadingElement{ID: "b-src", RefPath: "sources/b.txt", NotesPath: "reading/b.n4l", Title: "b", IntervalDays: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertItem(learning.GeneratedCard{NodeID: "b1", FilePath: "b.n4l", CardType: learning.CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"}); err != nil {
		t.Fatal(err)
	}

	turn, err := a.NextTurn()
	if err != nil {
		t.Fatal(err)
	}
	if turn == nil || turn.Kind != "card" || turn.Card.ID != "b1::cloze::0" {
		t.Fatalf("Collection B first Turn = %+v, want B's Card from a fresh tally", turn)
	}
}
