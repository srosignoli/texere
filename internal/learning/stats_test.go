package learning

import (
	"testing"
	"time"
)

// seedReview writes a review_logs row directly. The public path needs a full
// item and an FSRS pass; this task only reads the log back.
func seedReview(t *testing.T, s *Store, at time.Time, grade Grade, elapsedMs int) {
	t.Helper()
	if _, err := s.db.Exec(
		`INSERT INTO review_logs (item_id, reviewed_at, grade, elapsed_ms, old_state, new_stability, card_type)
		 VALUES (?,?,?,?,0,1.0,'basic')`,
		"i1", at.Unix(), int(grade), elapsedMs); err != nil {
		t.Fatal(err)
	}
}

func TestQualityStatsForgettingIndexFromAKnownMix(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	// 2 Again out of 10 reviews = 20%.
	for i := 0; i < 2; i++ {
		seedReview(t, st, now.AddDate(0, 0, -1), GradeAgain, 1000)
	}
	for i := 0; i < 5; i++ {
		seedReview(t, st, now.AddDate(0, 0, -2), GradeGood, 2000)
	}
	for i := 0; i < 3; i++ {
		seedReview(t, st, now.AddDate(0, 0, -3), GradeEasy, 3000)
	}

	got, err := st.QualityStatsFor(now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reviews30 != 10 {
		t.Fatalf("reviews = %d, want 10", got.Reviews30)
	}
	if got.ForgettingIndex != 20 {
		t.Errorf("forgetting index = %v, want 20", got.ForgettingIndex)
	}
	if got.Retention != 80 {
		t.Errorf("retention = %v, want 80", got.Retention)
	}
	if got.GradeMix != [4]int{2, 0, 5, 3} {
		t.Errorf("grade mix = %v, want [2 0 5 3]", got.GradeMix)
	}
	// (2*1000 + 5*2000 + 3*3000) / 10 = 2100
	if got.AvgLatencyMs != 2100 {
		t.Errorf("avg latency = %d, want 2100", got.AvgLatencyMs)
	}
}

// The whole point of the sentinel: a collection nobody has reviewed must not
// report a flawless 0% forgetting index.
func TestQualityStatsReportsMinusOneWithNoReviews(t *testing.T) {
	st, _ := Open(t.TempDir())
	got, err := st.QualityStatsFor(time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ForgettingIndex != -1 || got.Retention != -1 {
		t.Errorf("got fi=%v retention=%v, want -1 for both", got.ForgettingIndex, got.Retention)
	}
	if got.Reviews30 != 0 {
		t.Errorf("reviews = %d, want 0", got.Reviews30)
	}
}

func TestQualityStatsExcludesReviewsOlderThan30Days(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	seedReview(t, st, now.AddDate(0, 0, -5), GradeGood, 1000)
	seedReview(t, st, now.AddDate(0, 0, -45), GradeAgain, 1000)

	got, _ := st.QualityStatsFor(now)
	if got.Reviews30 != 1 {
		t.Fatalf("reviews = %d, want 1 — a 45-day-old review must fall outside the window", got.Reviews30)
	}
	if got.ForgettingIndex != 0 {
		t.Errorf("fi = %v, want 0 — the old Again must not count", got.ForgettingIndex)
	}
}

func TestHabitStatsReviewsPerDayIsDenseAndOldestFirst(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	seedReview(t, st, now, GradeGood, 1000)
	seedReview(t, st, now, GradeGood, 1000)
	seedReview(t, st, now.AddDate(0, 0, -2), GradeGood, 1000)

	got, err := st.HabitStatsFor(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ReviewsPerDay) != 84 {
		t.Fatalf("got %d days, want exactly 84 — quiet days must be present as zeros", len(got.ReviewsPerDay))
	}
	last := got.ReviewsPerDay[83]
	if last.Day != now.Format("2006-01-02") || last.Count != 2 {
		t.Errorf("last day = %+v, want today with 2", last)
	}
	if got.ReviewsPerDay[82].Count != 0 {
		t.Errorf("yesterday = %d, want 0", got.ReviewsPerDay[82].Count)
	}
	if got.ReviewsPerDay[81].Count != 1 {
		t.Errorf("two days ago = %d, want 1", got.ReviewsPerDay[81].Count)
	}
}

// Streaks are pure arithmetic over the dense day list, so they are tested
// directly rather than through a seeded database.
func TestStreaksAcrossAGap(t *testing.T) {
	days := []DayCount{
		{"2026-08-01", 3}, {"2026-08-02", 1}, {"2026-08-03", 4}, // 3-day run
		{"2026-08-04", 0},
		{"2026-08-05", 2}, {"2026-08-06", 1}, // current run of 2, ending today
	}
	cur, longest := streaks(days)
	if longest != 3 {
		t.Errorf("longest = %d, want 3", longest)
	}
	if cur != 2 {
		t.Errorf("current = %d, want 2", cur)
	}
}

// A streak you already broke is not a current streak.
func TestStreaksCurrentIsZeroWhenTheLastDayIsQuiet(t *testing.T) {
	days := []DayCount{{"2026-08-01", 5}, {"2026-08-02", 5}, {"2026-08-03", 0}}
	cur, longest := streaks(days)
	if cur != 0 {
		t.Errorf("current = %d, want 0 — the run ended before today", cur)
	}
	if longest != 2 {
		t.Errorf("longest = %d, want 2", longest)
	}
}

func TestStreaksOnAnEmptyList(t *testing.T) {
	cur, longest := streaks(nil)
	if cur != 0 || longest != 0 {
		t.Errorf("got %d/%d, want 0/0", cur, longest)
	}
}
