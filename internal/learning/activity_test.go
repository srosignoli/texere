package learning

import (
	"testing"
	"time"
)

func TestLogActivityRoundTrips(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.Local)
	if err := st.LogActivity("process", "e1", "promote", 0, now); err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	got, ok, err := st.ActivitySince("")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ActivitySince reported no data after a write")
	}
	if got.Unix() != now.Unix() {
		t.Errorf("since = %v, want %v", got, now)
	}
}

func TestActivitySinceOnAnEmptyLog(t *testing.T) {
	st, _ := Open(t.TempDir())
	_, ok, err := st.ActivitySince("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("an empty log must report ok=false, not a zero time the UI would render as 1970")
	}
}

// Reading is summed as minutes; everything else is counted as rows. Seeding
// several weeks with different values is what makes a bucketing bug visible —
// identical weeks would pass even if every row landed in one bucket.
func TestActivityWeeksBucketsReadMinutesOldestFirst(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	// 600s this week, 1200s two weeks ago, nothing in between.
	st.LogActivity("read", "r1", "", 600, now)
	st.LogActivity("read", "r1", "", 1200, now.AddDate(0, 0, -14))

	got, err := st.ActivityWeeks("read", 4, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d weeks, want exactly 4", len(got))
	}
	if got[3].Count != 10 {
		t.Errorf("this week = %d minutes, want 10 — the last entry must be the current week", got[3].Count)
	}
	if got[1].Count != 20 {
		t.Errorf("two weeks ago = %d minutes, want 20", got[1].Count)
	}
	if got[2].Count != 0 {
		t.Errorf("last week = %d, want 0 — a quiet week must be a zero, not a missing entry", got[2].Count)
	}
	// The label is what the chart's x-axis shows. If it drifts from the bucket
	// it names, every bar is captioned with the wrong week.
	wantLast := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	if got[3].WeekStart != wantLast {
		t.Errorf("last bucket starts %q, want %q — the final bucket is the week containing now", got[3].WeekStart, wantLast)
	}
	wantFirst := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local).AddDate(0, 0, -21).Format("2006-01-02")
	if got[0].WeekStart != wantFirst {
		t.Errorf("first bucket starts %q, want %q", got[0].WeekStart, wantFirst)
	}
	// Labels must be strictly increasing: two buckets sharing a label would make
	// the axis unreadable and hide a bucketing collapse.
	for i := 1; i < len(got); i++ {
		if got[i].WeekStart <= got[i-1].WeekStart {
			t.Fatalf("bucket %d starts %q, not after %q", i, got[i].WeekStart, got[i-1].WeekStart)
		}
	}
}

func TestActivityWeeksCountsRowsForNonReadKinds(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	st.LogActivity("process", "e1", "promote", 0, now)
	st.LogActivity("process", "e2", "dismiss", 0, now)

	got, err := st.ActivityWeeks("process", 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if got[1].Count != 2 {
		t.Errorf("this week = %d, want 2 rows counted", got[1].Count)
	}
}

func TestActivityWeeksIgnoresOtherKinds(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	st.LogActivity("read", "r1", "", 600, now)
	got, _ := st.ActivityWeeks("process", 2, now)
	if got[1].Count != 0 {
		t.Errorf("process week = %d, want 0 — a read row must not be counted as a process row", got[1].Count)
	}
}
