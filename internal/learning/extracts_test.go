package learning

import (
	"testing"
	"time"
)

func TestExtractScheduleAndCRUD(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_000_000, 0)

	e := Extract{ID: "f.n4l:Ch:id:a", FilePath: "/c/reading/f.n4l", Text: "System 1 is fast",
		FromAnchor: "sources/f.txt char=10", Active: true}
	added, err := s.UpsertExtract(e)
	if err != nil || !added {
		t.Fatalf("UpsertExtract added=%v err=%v", added, err)
	}
	// New extract is due (due NULL).
	due, err := s.DueExtracts(now, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("DueExtracts len=%d err=%v want 1", len(due), err)
	}

	// Later → deferral 1, due 1.5 days out, leaves the due list.
	got, _, _ := s.GetExtract(e.ID)
	sched := ScheduleExtract(got, "later", now)
	if sched.DeferralCount != 1 {
		t.Fatalf("deferral=%d want 1", sched.DeferralCount)
	}
	wantDue := now.Add(time.Duration(1.5*24) * time.Hour)
	if sched.Due.Unix() != wantDue.Unix() {
		t.Fatalf("due=%v want %v", sched.Due, wantDue)
	}
	if err := s.ApplyExtractSchedule(e.ID, sched); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.DueExtracts(now, 10); len(d) != 0 {
		t.Fatalf("after later, due len=%d want 0", len(d))
	}

	// Cap at 90 days.
	big := ScheduleExtract(Extract{DeferralCount: 30, Active: true}, "later", now)
	if big.Due.Unix() != now.Add(90*24*time.Hour).Unix() {
		t.Fatalf("cap due=%v want now+90d", big.Due)
	}

	// Dismiss → inactive.
	dm := ScheduleExtract(got, "dismiss", now)
	if dm.Active {
		t.Fatal("dismiss should set Active=false")
	}

	// Re-upsert preserves schedule (does not reset deferral/due).
	got2, _, _ := s.GetExtract(e.ID)
	got2.Text = "edited"
	if a, _ := s.UpsertExtract(got2); a {
		t.Fatal("re-upsert should not be 'added'")
	}
	after, _, _ := s.GetExtract(e.ID)
	if after.DeferralCount != 1 || after.Text != "edited" {
		t.Fatalf("after re-upsert deferral=%d text=%q (want 1, edited)", after.DeferralCount, after.Text)
	}

	// RetireMissingExtracts retires ids not in keep.
	n, err := s.RetireMissingExtracts(e.FilePath, map[string]bool{})
	if err != nil || n != 1 {
		t.Fatalf("RetireMissingExtracts n=%d err=%v want 1", n, err)
	}
}

// A dismissed extract whose (from) node is still in the .n4l must not be
// resurrected when the file is re-synced (GenerateExtracts → UpsertExtract).
func TestUpsertExtractDoesNotResurrectDismissed(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_000_000, 0)

	e := Extract{ID: "f.n4l:Ch:id:z", FilePath: "/c/reading/f.n4l", Text: "passage",
		FromAnchor: "sources/f.txt char=0", Active: true}
	if _, err := s.UpsertExtract(e); err != nil {
		t.Fatal(err)
	}
	// User dismisses it.
	got, _, _ := s.GetExtract(e.ID)
	if err := s.ApplyExtractSchedule(e.ID, ScheduleExtract(got, "dismiss", now)); err != nil {
		t.Fatal(err)
	}
	// Re-sync re-finds the still-present (from) node and upserts it again.
	if _, err := s.UpsertExtract(e); err != nil {
		t.Fatal(err)
	}
	after, _, _ := s.GetExtract(e.ID)
	if after.Active {
		t.Fatal("re-upsert resurrected a dismissed extract (active flipped back to true)")
	}
	if d, _ := s.DueExtracts(now, 10); len(d) != 0 {
		t.Fatalf("dismissed extract reappeared in due queue: len=%d want 0", len(d))
	}
}

func TestExtractAnchorsIncludesRetiredRows(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, e := range []Extract{
		{ID: "/c/n.n4l:C:id:a", FilePath: "/c/n.n4l", Text: "kept", FromAnchor: "sources/x.txt char=10", Due: now, Active: true},
		{ID: "/c/o.n4l:C:id:c", FilePath: "/c/o.n4l", Text: "promoted away", FromAnchor: "sources/y.txt char=5", Due: now, Active: true},
	} {
		if _, err := st.UpsertExtract(e); err != nil {
			t.Fatal(err)
		}
	}
	// Retiring must NOT drop it from this map. A promotion made before card
	// provenance was preserved stripped the (from) line from the .n4l, leaving
	// this row as the only record of where the passage came from.
	if _, err := st.RetireMissingExtracts("/c/o.n4l", map[string]bool{}); err != nil {
		t.Fatal(err)
	}

	got, err := st.ExtractAnchors()
	if err != nil {
		t.Fatal(err)
	}
	if got["/c/o.n4l:C:id:c"] != "sources/y.txt char=5" {
		t.Fatalf("retired extract lost its anchor: %+v", got)
	}
	if got["/c/n.n4l:C:id:a"] != "sources/x.txt char=10" {
		t.Fatalf("active extract missing: %+v", got)
	}
}

func TestExtractAnchorsEmptyIsNotNil(t *testing.T) {
	st, _ := Open(t.TempDir())
	got, err := st.ExtractAnchors()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("returned nil; callers index into this directly")
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
}
