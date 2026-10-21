package learning

import (
	"fmt"
	"testing"
	"time"
)

// queueFixture is a real Store with a settable clock, and the Queue over it.
type queueFixture struct {
	t    *testing.T
	s    *Store
	q    *Queue
	now  time.Time
	open string // the Source with an open Sitting, "" for none
}

func newQueueFixture(t *testing.T) *queueFixture {
	t.Helper()
	f := &queueFixture{t: t, s: openStore(t), now: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
	f.q = NewQueue(QueueDeps{
		Store:       f.s,
		OpenSitting: func() (string, bool) { return f.open, f.open != "" },
		Now:         func() time.Time { return f.now },
	})
	return f
}

// seed adds n due Study Elements of each kind: Sources, Extracts, Cards.
func (f *queueFixture) seed(sources, extracts, cards int) {
	f.t.Helper()
	for i := 0; i < sources; i++ {
		id := fmt.Sprintf("s%02d", i)
		if err := f.s.UpsertReading(ReadingElement{ID: id, RefPath: "sources/" + id, NotesPath: "reading/" + id + ".n4l", Title: id, IntervalDays: 1}); err != nil {
			f.t.Fatal(err)
		}
	}
	for i := 0; i < extracts; i++ {
		if _, err := f.s.UpsertExtract(Extract{ID: fmt.Sprintf("e%02d", i), FilePath: "reading/x.n4l", Text: "t", Active: true}); err != nil {
			f.t.Fatal(err)
		}
	}
	for i := 0; i < cards; i++ {
		if _, err := f.s.UpsertItem(GeneratedCard{NodeID: fmt.Sprintf("n%02d", i), FilePath: "notes.n4l", CardType: CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"}); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *queueFixture) next() *Turn {
	f.t.Helper()
	turn, err := f.q.Next()
	if err != nil {
		f.t.Fatalf("Next: %v", err)
	}
	return turn
}

func TestNothingDueIsNil(t *testing.T) {
	f := newQueueFixture(t)
	if turn := f.next(); turn != nil {
		t.Errorf("Next() = %+v with nothing due, want nil", turn)
	}
}

// Every kind starts the Session equally far below its share, and ties go to
// the Card: it is the only kind with a deadline.
func TestFirstTurnIsACardOnATie(t *testing.T) {
	f := newQueueFixture(t)
	f.seed(1, 1, 1)
	turn := f.next()
	if turn == nil || turn.Kind != KindCard || turn.Card == nil {
		t.Fatalf("first Turn = %+v, want a Card", turn)
	}
}

// The view re-asks constantly (opening Study, coming back to the Next tab, a
// Source served before its Sitting begins). Until the Turn has ended, re-asking
// returns it and counts nothing.
func TestNextReturnsTheCurrentTurnUntilItEnds(t *testing.T) {
	f := newQueueFixture(t)
	f.seed(3, 3, 3)
	first := f.next()
	for i := 0; i < 3; i++ {
		again := f.next()
		if again == nil || again.Kind != first.Kind || turnID(again) != turnID(first) {
			t.Fatalf("re-ask %d = %+v, want the current Turn %+v", i, again, first)
		}
	}
}

func turnID(t *Turn) string {
	switch t.Kind {
	case KindSource:
		return t.Source.ID
	case KindExtract:
		return t.Extract.ID
	default:
		return t.Card.ID
	}
}

// end ends a Turn the way the app does: grading the Card, deferring the
// Extract, ending the Source's Sitting. Each moves the element's due date out.
func (f *queueFixture) end(turn *Turn) {
	f.t.Helper()
	switch turn.Kind {
	case KindCard:
		prev, _, err := f.s.LoadSched(turn.Card.ID)
		if err != nil {
			f.t.Fatal(err)
		}
		next, err := Schedule(prev, Grade(3), f.now)
		if err != nil {
			f.t.Fatal(err)
		}
		if err := f.s.ApplyReview(turn.Card.ID, next, Grade(3), 1000, prev.State, f.now); err != nil {
			f.t.Fatal(err)
		}
	case KindExtract:
		e, _, err := f.s.GetExtract(turn.Extract.ID)
		if err != nil {
			f.t.Fatal(err)
		}
		if err := f.s.ApplyExtractSchedule(e.ID, ScheduleExtract(e, "later", f.now)); err != nil {
			f.t.Fatal(err)
		}
	case KindSource:
		r, _, err := f.s.GetReading(turn.Source.ID)
		if err != nil {
			f.t.Fatal(err)
		}
		if err := f.s.ApplyReadingSchedule(r.ID, ScheduleReading(r, "continue", f.now)); err != nil {
			f.t.Fatal(err)
		}
	}
}

// run serves and ends n Turns, returning how many of each kind were served.
func (f *queueFixture) run(n int) map[Kind]int {
	f.t.Helper()
	got := map[Kind]int{}
	for i := 0; i < n; i++ {
		turn := f.next()
		if turn == nil {
			f.t.Fatalf("Turn %d: nothing due", i)
		}
		got[turn.Kind]++
		f.end(turn)
	}
	return got
}

// The default Mix is 20 / 15 / 65: over 20 Turns, with plenty of everything
// due, that is 4 Sources, 3 Extracts and 13 Cards.
func TestNextServesTheMixOverTwentyTurns(t *testing.T) {
	f := newQueueFixture(t)
	f.seed(10, 10, 20)
	got := f.run(20)
	if want := (map[Kind]int{KindSource: 4, KindExtract: 3, KindCard: 13}); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("20 Turns served %v, want %v", got, want)
	}
}

// A kind with nothing due gives its share to the others.
func TestKindsWithNothingDueAreSkipped(t *testing.T) {
	f := newQueueFixture(t)
	f.seed(0, 5, 0)
	if got := f.run(5); got[KindExtract] != 5 {
		t.Errorf("served %v, want 5 Extracts", got)
	}
}

// Weight 0 turns a kind off, even when it is the only kind with anything due.
func TestWeightZeroIsNeverServed(t *testing.T) {
	f := newQueueFixture(t)
	f.seed(3, 0, 0)
	if err := f.s.SaveMix(Mix{Source: 0, Extract: 0, Card: 65}); err != nil {
		t.Fatal(err)
	}
	if turn := f.next(); turn != nil {
		t.Errorf("Next() = %+v, want nil: Sources are switched off", turn)
	}
}

// A Source opened from the Read tab is mid-Sitting: Next resumes it before
// the Mix is asked, even though the Mix would pick a Card.
func TestAnOpenSittingResumesFirst(t *testing.T) {
	f := newQueueFixture(t)
	f.seed(3, 3, 3)
	f.open = "s02"
	turn := f.next()
	if turn == nil || turn.Kind != KindSource || turn.Source.ID != "s02" {
		t.Fatalf("Next() = %+v, want the open Source s02", turn)
	}
}

// A sync can retire the current Card (its node was deleted). The Turn has
// ended: Next moves on without an error.
func TestATurnWhoseElementVanishedHasEnded(t *testing.T) {
	f := newQueueFixture(t)
	f.seed(0, 0, 2)
	first := f.next()
	if _, err := f.s.RetireMissing("notes.n4l", map[string]bool{"n01::cloze::0": true}); err != nil {
		t.Fatal(err)
	}
	if first.Card.ID != "n00::cloze::0" {
		t.Fatalf("setup: first Card %q, want n00", first.Card.ID)
	}
	turn := f.next()
	if turn == nil || turn.Card == nil || turn.Card.ID != "n01::cloze::0" {
		t.Fatalf("Next() after retiring the current Card = %+v, want n01", turn)
	}
}

// Changing the Mix applies from the next Turn, against the same tally.
func TestAMixChangeAppliesFromTheNextTurn(t *testing.T) {
	f := newQueueFixture(t)
	f.seed(10, 10, 10)
	f.run(5)
	if err := f.s.SaveMix(Mix{Source: 100}); err != nil {
		t.Fatal(err)
	}
	if got := f.run(5); got[KindSource] != 5 {
		t.Errorf("after the change served %v, want 5 Sources", got)
	}
}

// A Card graded Again can be due again within seconds. Its due date still
// moved, so the Turn ended: once it is due again, it comes back as a new Turn
// chosen by the Mix, not as the current one.
func TestAGradedCardDueAgainAtOnceStillEndsTheTurn(t *testing.T) {
	f := newQueueFixture(t)
	f.seed(5, 0, 1)
	card := f.next()
	if card.Kind != KindCard {
		t.Fatalf("setup: first Turn %+v, want the Card", card)
	}
	prev, _, _ := f.s.LoadSched(card.Card.ID)
	next, err := Schedule(prev, Grade(1), f.now)
	if err != nil {
		t.Fatal(err)
	}
	next.Due = f.now.Add(time.Second)
	if err := f.s.ApplyReview(card.Card.ID, next, Grade(1), 1000, prev.State, f.now); err != nil {
		t.Fatal(err)
	}
	if turn := f.next(); turn == nil || turn.Kind != KindSource {
		t.Fatalf("right after the grade Next() = %+v, want a Source", turn)
	}
}
