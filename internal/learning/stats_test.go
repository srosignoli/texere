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

// A review logged without a latency is still a review, but it is not evidence
// about how long reviews take. Counting it as zero would drag the mean down and
// understate the daily-minutes estimate that multiplies by it.
func TestAvgLatencyIgnoresReviewsWithNoRecordedTime(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	seedReview(t, st, now, GradeGood, 2000)
	seedReview(t, st, now, GradeGood, 4000)
	// One row with a NULL elapsed_ms, as an older log or a caller that omitted it.
	if _, err := st.db.Exec(
		`INSERT INTO review_logs (item_id, reviewed_at, grade, elapsed_ms, old_state, new_stability, card_type)
		 VALUES ('i1', ?, 3, NULL, 0, 1.0, 'basic')`, now.Unix()); err != nil {
		t.Fatal(err)
	}

	got, err := st.QualityStatsFor(now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reviews30 != 3 {
		t.Fatalf("reviews = %d, want 3 — an untimed review is still a review", got.Reviews30)
	}
	if got.AvgLatencyMs != 3000 {
		t.Errorf("avg latency = %d, want 3000 — the untimed row must not be in the divisor", got.AvgLatencyMs)
	}
}

// The mean is integer division. A fixture that divides evenly cannot tell
// truncation from rounding, so this one deliberately does not.
func TestAvgLatencyTruncatesRatherThanRounds(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	seedReview(t, st, now, GradeGood, 1000)
	seedReview(t, st, now, GradeGood, 1000)
	seedReview(t, st, now, GradeGood, 1001) // 3001/3 = 1000.33
	got, _ := st.QualityStatsFor(now)
	if got.AvgLatencyMs != 1000 {
		t.Errorf("avg latency = %d, want 1000 (truncated, not rounded)", got.AvgLatencyMs)
	}
}

// The window is the last 30 days INCLUSIVE of today, so the oldest included
// review is 29 days back. An off-by-one here silently shrinks the window.
func TestQualityStatsWindowBoundaryIsExactlyThirtyDays(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	seedReview(t, st, now.AddDate(0, 0, -29), GradeGood, 1000) // in
	seedReview(t, st, now.AddDate(0, 0, -31), GradeGood, 1000) // out
	got, _ := st.QualityStatsFor(now)
	if got.Reviews30 != 1 {
		t.Errorf("reviews = %d, want 1 — day -29 is inside the window and day -31 is outside", got.Reviews30)
	}
}

func seedItem(t *testing.T, s *Store, id string, due, lastReview *time.Time, lapses int, difficulty float64, createdAt time.Time) {
	t.Helper()
	var d, lr interface{}
	if due != nil {
		d = due.Unix()
	}
	if lastReview != nil {
		lr = lastReview.Unix()
	}
	if _, err := s.db.Exec(
		`INSERT INTO items (id, node_id, file_path, card_type, front, back, active,
		 stability, difficulty, due, last_review, reps, lapses, state, created_at)
		 VALUES (?,?,?,?,?,?,1,1.0,?,?,?,1,?,2,?)`,
		id, id, "/c/n.n4l", "basic", "f", "b", difficulty, d, lr, lapses, createdAt.Unix()); err != nil {
		t.Fatal(err)
	}
}

func TestBurdenIsTheSumOfIntervalReciprocals(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	lr := now.AddDate(0, 0, -2)
	due2 := now.AddDate(0, 0, 0) // interval 2 days -> 0.5
	lr2 := now.AddDate(0, 0, -10)
	due10 := now.AddDate(0, 0, 0) // interval 10 days -> 0.1
	seedItem(t, st, "a", &due2, &lr, 0, 5, now)
	seedItem(t, st, "b", &due10, &lr2, 0, 5, now)

	got, err := st.WorkloadStatsFor(now, 10000) // 10s per review
	if err != nil {
		t.Fatal(err)
	}
	if got.Burden < 0.59 || got.Burden > 0.61 {
		t.Errorf("burden = %v, want ~0.6 (1/2 + 1/10)", got.Burden)
	}
	// 0.6 reviews/day * 10s = 6s = 0.1 min
	if got.DailyMinutes < 0.09 || got.DailyMinutes > 0.11 {
		t.Errorf("daily minutes = %v, want ~0.1", got.DailyMinutes)
	}
}

// A never-reviewed item has no interval. Counting it as 1 day would invent a
// daily review that nothing has scheduled.
func TestBurdenExcludesNeverReviewedItems(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	due := now
	seedItem(t, st, "new", &due, nil, 0, 5, now)

	got, _ := st.WorkloadStatsFor(now, 10000)
	if got.Burden != -1 {
		t.Errorf("burden = %v, want -1 — a collection with nothing scheduled has no burden", got.Burden)
	}
	if got.DailyMinutes != -1 {
		t.Errorf("daily minutes = %v, want -1", got.DailyMinutes)
	}
}

func TestForecastBucketsByDayFromToday(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	lr := now.AddDate(0, 0, -1)
	today := now
	inThree := now.AddDate(0, 0, 3)
	seedItem(t, st, "a", &today, &lr, 0, 5, now)
	seedItem(t, st, "b", &inThree, &lr, 0, 5, now)
	seedItem(t, st, "c", &inThree, &lr, 0, 5, now)

	got, _ := st.WorkloadStatsFor(now, 1000)
	if len(got.Forecast) != 14 {
		t.Fatalf("forecast has %d days, want 14", len(got.Forecast))
	}
	if got.Forecast[0] != 1 {
		t.Errorf("today = %d, want 1", got.Forecast[0])
	}
	if got.Forecast[3] != 2 {
		t.Errorf("+3 days = %d, want 2", got.Forecast[3])
	}
	if got.Forecast[1] != 0 {
		t.Errorf("+1 day = %d, want 0", got.Forecast[1])
	}
}

func TestLeechesAndDifficultyBuckets(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	lr := now.AddDate(0, 0, -1)
	due := now
	seedItem(t, st, "easy", &due, &lr, 0, 1.5, now)  // bucket 0 (1-3)
	seedItem(t, st, "mid", &due, &lr, 2, 5.0, now)   // bucket 2 (5-7)
	seedItem(t, st, "leech", &due, &lr, 7, 9.5, now) // bucket 4 (9-10)

	got, _ := st.WorkloadStatsFor(now, 1000)
	if got.Leeches != 1 {
		t.Errorf("leeches = %d, want 1 (lapses >= 5)", got.Leeches)
	}
	if got.Difficulty[0] != 1 || got.Difficulty[2] != 1 || got.Difficulty[4] != 1 {
		t.Errorf("difficulty buckets = %v, want one item in buckets 0, 2 and 4", got.Difficulty)
	}
}

func TestPipelineCountsAndOldestOpenExtract(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	lr := now.AddDate(0, 0, -1)
	due := now
	seedItem(t, st, "card", &due, &lr, 0, 5, now)

	if err := st.UpsertReading(ReadingElement{
		ID: "r1", RefPath: "s/a.txt", NotesPath: "n/a.n4l", Title: "a", IntervalDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// An extract opened 10 days ago and still unprocessed.
	if _, err := st.db.Exec(
		`INSERT INTO extracts (id, file_path, text, from_anchor, due, deferral_count, active, created_at)
		 VALUES ('e1','/c/n.n4l','x','',?,0,1,?)`,
		now.Unix(), now.AddDate(0, 0, -10).Unix()); err != nil {
		t.Fatal(err)
	}

	got, err := st.PipelineStatsFor(now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources != 1 || got.Extracts != 1 || got.ExtractsOpen != 1 || got.Cards != 1 {
		t.Errorf("funnel = %+v, want 1 of each", got)
	}
	if got.OldestOpenDays != 10 {
		t.Errorf("oldest open = %d days, want 10", got.OldestOpenDays)
	}
}

func TestPipelineOldestOpenIsMinusOneWhenNothingIsOpen(t *testing.T) {
	st, _ := Open(t.TempDir())
	got, err := st.PipelineStatsFor(time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.OldestOpenDays != -1 {
		t.Errorf("oldest open = %d, want -1 — an empty queue is not a zero-day-old queue", got.OldestOpenDays)
	}
}
