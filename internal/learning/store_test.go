package learning

import (
	"testing"
	"time"
)

func newCard(id string) GeneratedCard {
	return GeneratedCard{NodeID: id, FilePath: "x.n4l", CardType: CardTypeCloze,
		Front: "f-" + id, Back: "b-" + id, Discriminator: "0"}
}

func TestUpsertRetireQueueReview(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_000_000, 0)

	// Upsert two cards.
	for _, id := range []string{"a", "b"} {
		added, err := s.UpsertItem(newCard(id))
		if err != nil || !added {
			t.Fatalf("UpsertItem(%s) added=%v err=%v", id, added, err)
		}
	}
	// Re-upsert "a" with new front: not added, front updated, still active.
	g := newCard("a")
	g.Front = "updated"
	if added, err := s.UpsertItem(g); err != nil || added {
		t.Fatalf("re-upsert added=%v err=%v want added=false", added, err)
	}

	// Both new cards are due (due IS NULL).
	q, err := s.DueQueue(now, 10)
	if err != nil || len(q) != 2 {
		t.Fatalf("DueQueue len=%d err=%v want 2", len(q), err)
	}

	// Retire "b" (no longer generated for /x.n4l).
	keep := map[string]bool{newCard("a").ItemID(): true}
	retired, err := s.RetireMissing("x.n4l", keep)
	if err != nil || retired != 1 {
		t.Fatalf("RetireMissing retired=%d err=%v want 1", retired, err)
	}
	if q, _ := s.DueQueue(now, 10); len(q) != 1 {
		t.Fatalf("DueQueue after retire len=%d want 1", len(q))
	}

	// Review "a": schedule it into the future, it leaves the due queue.
	itemID := newCard("a").ItemID()
	st := SchedState{Stability: 5, Difficulty: 5, Due: now.Add(48 * time.Hour),
		LastReview: now, Reps: 1, State: 2}
	if err := s.ApplyReview(itemID, st, GradeGood, 1200, 0, now); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}
	if q, _ := s.DueQueue(now, 10); len(q) != 0 {
		t.Fatalf("DueQueue after review len=%d want 0", len(q))
	}
	// Loaded sched round-trips.
	got, ok, err := s.LoadSched(itemID)
	if err != nil || !ok || got.Reps != 1 || got.Due.Unix() != st.Due.Unix() {
		t.Fatalf("LoadSched got=%+v ok=%v err=%v", got, ok, err)
	}
	// Stats: 1 due (the future-due 'a' is not due; 'b' retired) -> due=0, total active=1.
	stats, err := s.CountStats(now)
	if err != nil || stats.Total != 1 || stats.Due != 0 {
		t.Fatalf("CountStats=%+v err=%v want Total=1 Due=0", stats, err)
	}
}
