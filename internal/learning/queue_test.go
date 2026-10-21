package learning

import (
	"testing"
	"time"
)

func TestStudyCountsAndDueReading(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	now := time.Unix(2_000_000, 0)

	// One due reading element, one due extract, one new item.
	s.UpsertReading(ReadingElement{ID: "r1", RefPath: "sources/x.txt", NotesPath: "reading/x.n4l", Title: "X", IntervalDays: 1})
	s.UpsertExtract(Extract{ID: "x1", FilePath: "reading/x.n4l", Text: "t", Active: true})
	s.UpsertItem(GeneratedCard{NodeID: "n1", FilePath: "f.n4l", CardType: CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"})

	c, err := s.StudyCounts(now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Read != 1 || c.Process != 1 || c.Recall != 1 {
		t.Fatalf("counts=%+v want 1/1/1", c)
	}
	dr, err := s.DueReading(now, 10)
	if err != nil || len(dr) != 1 {
		t.Fatalf("DueReading len=%d err=%v want 1", len(dr), err)
	}
}
