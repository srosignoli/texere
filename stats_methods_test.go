package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"texere/internal/content"
	"texere/internal/learning"
)

func TestRescheduleExtractLogsItsAction(t *testing.T) {
	dir := t.TempDir()
	a, st := newTestApp(t, dir, nil)
	if _, err := st.UpsertExtract(learning.Extract{
		ID: "e1", FilePath: "n.n4l", Text: "x", Due: time.Now(), Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := a.DismissExtract("e1"); err != nil {
		t.Fatalf("DismissExtract: %v", err)
	}
	got, err := st.ActivityWeeks("process", 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Count != 1 {
		t.Errorf("processed count = %d, want 1 — processing must be logged", got[0].Count)
	}
}

// PromoteExtract does not go through rescheduleExtract — it has its own path
// (SeedBasicCardNode / InsertMarkerAboveNode + learning.Sync) and needs its own
// log line. Without it, promotion — the most meaningful processing outcome —
// would never show up in the pipeline panel.
func TestPromoteExtractLogsItsAction(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	p := filepath.Join(dir, "reading", "k.n4l")
	// The (from) arrow must be inline on the same node line — separate-line (from)
	// is parsed as a root node with empty text and cannot produce a card.
	// This matches the format produced by content.AppendExtract.
	os.WriteFile(p, []byte("- K\n# @id e1\nSystem 1 is fast (from) \"sources/k.txt char=0\"\n"), 0o644)
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{"from": {Code: "from", LongName: "from"}}}
	a, st := newTestApp(t, dir, reg)

	if _, err := learning.Sync(st, []string{p}, reg); err != nil {
		t.Fatal(err)
	}
	q, err := a.GetExtractQueue(10)
	if err != nil || len(q) != 1 {
		t.Fatalf("GetExtractQueue len=%d err=%v want 1", len(q), err)
	}

	if err := a.PromoteExtract(q[0].ID, "card"); err != nil {
		t.Fatalf("PromoteExtract: %v", err)
	}
	got, err := st.ActivityWeeks("process", 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Count != 1 {
		t.Errorf("processed count = %d, want 1 — promotion must be logged", got[0].Count)
	}
}

func TestGetStatisticsAssemblesEveryPanel(t *testing.T) {
	dir := t.TempDir()
	a, _ := newTestApp(t, dir, nil)

	got, err := a.GetStatistics()
	if err != nil {
		t.Fatalf("GetStatistics on an empty collection must not error: %v", err)
	}
	// The whole point: a fresh collection reports "no data", not perfection.
	if got.Quality.ForgettingIndex != -1 {
		t.Errorf("fi = %v, want -1", got.Quality.ForgettingIndex)
	}
	if got.Workload.Burden != -1 {
		t.Errorf("burden = %v, want -1", got.Workload.Burden)
	}
	if got.Pipeline.OldestOpenDays != -1 {
		t.Errorf("oldest open = %d, want -1", got.Pipeline.OldestOpenDays)
	}
	if got.ReadSince != 0 || got.ProcSince != 0 {
		t.Errorf("readSince=%d procSince=%d, want 0 for a never-written log", got.ReadSince, got.ProcSince)
	}
	// Dense series must be present and correctly sized even with no data, or
	// the charts index past the end of an array.
	if len(got.Habit.ReviewsPerDay) != 84 {
		t.Errorf("reviewsPerDay = %d entries, want 84", len(got.Habit.ReviewsPerDay))
	}
	if len(got.Workload.Forecast) != 14 {
		t.Errorf("forecast = %d entries, want 14", len(got.Workload.Forecast))
	}
	for name, series := range map[string][]learning.WeekCount{
		"cardsPerWeek": got.CardsPerWeek, "readMinsWeek": got.ReadMinsWeek, "procPerWeek": got.ProcPerWeek,
	} {
		if len(series) != 12 {
			t.Errorf("%s = %d entries, want 12", name, len(series))
		}
	}
}

func TestGetStatisticsWithoutACollection(t *testing.T) {
	a := &App{}
	if _, err := a.GetStatistics(); err == nil {
		t.Error("want an error when no collection is open")
	}
}

// One shared flag let a single dismissed extract switch the READING chart out
// of its "still collecting" state, so a log one second old would assert twelve
// weeks of zero reading. The two kinds must age independently.
func TestGetStatisticsTracksEachActivityKindSeparately(t *testing.T) {
	dir := t.TempDir()
	a, st := newTestApp(t, dir, nil)

	if err := st.LogActivity("process", "e1", "dismiss", 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := a.GetStatistics()
	if err != nil {
		t.Fatal(err)
	}
	if got.ProcSince == 0 {
		t.Error("procSince = 0 after logging a process event")
	}
	if got.ReadSince != 0 {
		t.Errorf("readSince = %d, want 0 — processing an extract says nothing about reading", got.ReadSince)
	}
}

// The latency hand-off from QualityStatsFor into WorkloadStatsFor: every other
// fixture here is an empty collection where burden is already -1, so nothing
// distinguishes passing the real average from passing zero. This one mints a
// real graded card so both numbers exist and the product must be non-zero.
func TestGetStatisticsPassesRealLatencyIntoWorkload(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	notes := filepath.Join(dir, "reading", "cards.n4l")
	if err := os.WriteFile(notes, []byte("- cards\n\n# @card\nSystem 1 is fast >> yes\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}
	a, st := newTestApp(t, dir, reg)

	if _, err := learning.Sync(st, []string{notes}, a.registry); err != nil {
		t.Fatal(err)
	}
	q, err := a.GetDueQueue(10)
	if err != nil || len(q) == 0 {
		t.Fatalf("setup: no gradeable card (err=%v, n=%d)", err, len(q))
	}
	if _, err := a.GradeItem(q[0].ID, 3, 5000); err != nil {
		t.Fatal(err)
	}

	got, err := a.GetStatistics()
	if err != nil {
		t.Fatal(err)
	}
	if got.Quality.AvgLatencyMs != 5000 {
		t.Fatalf("avgLatencyMs = %d, want 5000 — fixture is wrong", got.Quality.AvgLatencyMs)
	}
	if got.Workload.Burden <= 0 {
		t.Fatalf("burden = %v, want > 0 — fixture is wrong", got.Workload.Burden)
	}
	// Passing 0 instead of the measured average leaves this at the -1 sentinel.
	if got.Workload.DailyMinutes <= 0 {
		t.Errorf("dailyMinutes = %v, want > 0 — the measured latency never reached WorkloadStatsFor",
			got.Workload.DailyMinutes)
	}
}
