# Study Statistics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A fourth Study focus showing learning quality, workload, habit and pipeline health, plus an activity log for the two events the database cannot currently reconstruct.

**Architecture:** A new `activity_log` table (schema v6) with two write-points. All computation lives in `internal/learning/stats.go` behind one binding. Chart arithmetic lives in a dependency-free JS module; the panel renders inline SVG.

**Tech Stack:** Go 1.25, SQLite (`modernc.org/sqlite`, no CGo), Wails v2, Svelte 3.59, `node --test`.

## Global Constraints

- **The activity log records only what is unrecoverable** — `read` sessions and `process` outcomes. Cards created is already `items.created_at` and extracts created is already `extracts.created_at`; logging those would create a second, disagreeing source of truth.
- **A peek logs nothing.** `ReadingView`'s `peekAt` guard applies to the reading-session write exactly as it does to the three progress-persist paths. Without it, glancing at an extract from the sidebar counts as time spent reading.
- **`process` detail values are the scheduler's own strings** — `later`, `dismiss`, `promote` — so the log cannot drift from `ScheduleExtract`'s vocabulary.
- **No charting library.** Inline SVG only. The arithmetic lives in `frontend/src/lib/stats-format.js`, not in the component.
- **Every panel survives an empty collection.** No `NaN`, no divide-by-zero, no panic. A metric with no data reports the sentinel `-1`, and the UI renders "Not enough data yet" — never `0%`.
- **Day boundaries are local, computed in Go**, never `at/86400` in SQL. A UTC day boundary puts an evening review on the wrong day and silently breaks streaks for anyone west of Greenwich.
- Go: `gofmt`, imports grouped stdlib / external / internal. JS/Svelte: 2-space indent, **no semicolons**, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.
- `wails` is not on PATH; it is at `/home/simone/go/bin/wails`.
- `git add frontend/wailsjs` as a bare directory exits non-zero because `runtime/` is gitignored. Add explicit file paths and confirm with `git show --stat`.
- `TestWatchCollectionAdded` in `internal/content` is a **known pre-existing fsnotify flake**. Ignore it; never "fix" it.

## MANDATORY: negative-control every test you write

This project has shipped tests that passed against the exact bug they were written to catch, and a feature whose primary control did nothing. Before committing, for each behaviour asserted: break the production code in the specific way the test exists to catch, run the test, confirm it **fails with a message naming the real problem**, then restore and re-run green. Report the actual failure output for the most load-bearing assertion.

Three traps that have produced a *silent* pass here before:
- A sabotage that does not compile is **not** a control. If removing the code breaks the build, change the sabotage (e.g. compare against a key that cannot match) until it runs and the test genuinely fails.
- A fixture that cannot distinguish the bug is not a test. If a control does not fail, say so plainly — the fixture is the defect, not the control.
- Seeding a fixture with values that are all identical (all the same day, all the same grade) makes ordering and grouping bugs invisible. Vary them.

## File Structure

- `internal/learning/db.go` — schema v6.
- `internal/learning/activity.go` — the log's writer and its two aggregate reads.
- `internal/learning/activity_test.go`
- `internal/learning/stats.go` — every statistic, one struct.
- `internal/learning/stats_test.go`
- `study_methods.go` — the `process` write-point.
- `stats_methods.go` — `GetStatistics` and `LogReadingSession` bindings.
- `stats_methods_test.go`
- `frontend/src/lib/stats-format.js` — chart arithmetic, dependency-free.
- `frontend/src/lib/stats-format.test.js`
- `frontend/src/components/StatsPanel.svelte` — the four panels.
- `frontend/src/components/ReadingView.svelte` — the `read` write-point.
- `frontend/src/components/StudySurface.svelte` — the tab.
- `docs/superpowers/specs/2026-08-11-study-statistics-manual-test-plan.md`

---

### Task 1: Schema v6 and the activity log

**Files:**
- Modify: `internal/learning/db.go`
- Create: `internal/learning/activity.go`
- Create: `internal/learning/activity_test.go`

**Interfaces:**
- Consumes: the existing `Store` and its `db` field.
- Produces:
  - `func (s *Store) LogActivity(kind, refID, detail string, amount int, at time.Time) error`
  - `func (s *Store) ActivityWeeks(kind string, weeks int, now time.Time) ([]int, error)`
  - `func (s *Store) ActivitySince() (time.Time, bool, error)`

`ActivityWeeks` returns exactly `weeks` entries, oldest first, the last being the week containing `now`. For `kind == "read"` each entry is **minutes** (summed `amount` seconds ÷ 60); for any other kind it is a **row count**.

- [ ] **Step 1: Write the failing test**

Create `internal/learning/activity_test.go`:

```go
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
	got, ok, err := st.ActivitySince()
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
	_, ok, err := st.ActivitySince()
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
	if got[3] != 10 {
		t.Errorf("this week = %d minutes, want 10 — the last entry must be the current week", got[3])
	}
	if got[1] != 20 {
		t.Errorf("two weeks ago = %d minutes, want 20", got[1])
	}
	if got[2] != 0 {
		t.Errorf("last week = %d, want 0 — a quiet week must be a zero, not a missing entry", got[2])
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
	if got[1] != 2 {
		t.Errorf("this week = %d, want 2 rows counted", got[1])
	}
}

func TestActivityWeeksIgnoresOtherKinds(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	st.LogActivity("read", "r1", "", 600, now)
	got, _ := st.ActivityWeeks("process", 2, now)
	if got[1] != 0 {
		t.Errorf("process week = %d, want 0 — a read row must not be counted as a process row", got[1])
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/learning/ -run 'Activity|LogActivity' -v`
Expected: FAIL — `st.LogActivity undefined`.

- [ ] **Step 3: Implement the migration**

In `internal/learning/db.go`, change `const schemaVersion = 5` to `6`, add a `case 5:` arm to the migration switch mirroring the existing arms:

```go
		case 5:
			if _, err := s.db.Exec(ddlV6); err != nil {
				return fmt.Errorf("learning: apply v6: %w", err)
			}
```

and add the DDL beside the others:

```go
const ddlV6 = `
CREATE TABLE IF NOT EXISTS activity_log (
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  kind   TEXT NOT NULL,
  ref_id TEXT NOT NULL DEFAULT '',
  at     INTEGER NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  amount INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_activity_at ON activity_log(at);
`
```

- [ ] **Step 4: Implement the store methods**

Create `internal/learning/activity.go`:

```go
package learning

import (
	"database/sql"
	"time"
)

// LogActivity records one event the schema cannot otherwise reconstruct.
//
// Only two kinds exist: "read" (a reading session, amount = seconds) and
// "process" (an extract processed, detail = the scheduler's own action string).
// Card and extract *creation* are deliberately absent — items.created_at and
// extracts.created_at already record those, and a second copy could disagree.
func (s *Store) LogActivity(kind, refID, detail string, amount int, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO activity_log (kind, ref_id, at, detail, amount) VALUES (?,?,?,?,?)`,
		kind, refID, at.Unix(), detail, amount)
	return err
}

// ActivitySince returns the timestamp of the earliest logged event.
//
// ok=false means the log is empty. The caller needs that distinct from a zero
// time, which a UI would happily render as 1970.
func (s *Store) ActivitySince() (time.Time, bool, error) {
	var at sql.NullInt64
	if err := s.db.QueryRow(`SELECT MIN(at) FROM activity_log`).Scan(&at); err != nil {
		return time.Time{}, false, err
	}
	if !at.Valid {
		return time.Time{}, false, nil
	}
	return time.Unix(at.Int64, 0), true, nil
}

// ActivityWeeks returns exactly `weeks` buckets, oldest first, the last being
// the week containing now. "read" sums amount as minutes; every other kind
// counts rows.
//
// Bucketed in Go rather than SQL because week boundaries have to be local:
// dividing a unix timestamp by 604800 buckets by UTC and drops an evening
// session into the wrong week for anyone west of Greenwich.
func (s *Store) ActivityWeeks(kind string, weeks int, now time.Time) ([]int, error) {
	out := make([]int, weeks)
	if weeks <= 0 {
		return out, nil
	}
	// Start of the bucket window: midnight local, (weeks-1) weeks back.
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -7*(weeks-1))

	rows, err := s.db.Query(
		`SELECT at, amount FROM activity_log WHERE kind=? AND at >= ?`, kind, start.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var at int64
		var amount int
		if err := rows.Scan(&at, &amount); err != nil {
			return nil, err
		}
		idx := int(time.Unix(at, 0).Sub(start).Hours() / (24 * 7))
		if idx < 0 || idx >= weeks {
			continue
		}
		if kind == "read" {
			out[idx] += amount
		} else {
			out[idx]++
		}
	}
	if kind == "read" {
		for i := range out {
			out[i] /= 60 // seconds -> whole minutes
		}
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/learning/ -run 'Activity|LogActivity' -v` then `make test`
Expected: PASS (5 tests).

- [ ] **Step 6: Negative controls (report both)**

1. In `ActivityWeeks`, change `out[idx] += amount` to `out[0] += amount`. `TestActivityWeeksBucketsReadMinutesOldestFirst` must FAIL naming the wrong week. Restore.
2. Invert the kind filter — `WHERE kind=?` becomes `WHERE kind!=?`. This keeps the placeholder count and argument order intact, so it runs rather than erroring on arity; a sabotage that fails to execute proves nothing. `TestActivityWeeksIgnoresOtherKinds` must FAIL. Restore.

- [ ] **Step 7: Commit**

```bash
git add internal/learning/db.go internal/learning/activity.go internal/learning/activity_test.go
git commit -m "feat(learning): activity log for reading sessions and extract processing"
```

---

### Task 2: The two write-points

**Files:**
- Modify: `study_methods.go`
- Create: `stats_methods.go`
- Create: `stats_methods_test.go`
- Modify: `frontend/src/components/ReadingView.svelte`
- Regenerate: `frontend/wailsjs/go/main/App.d.ts`, `App.js`

**Interfaces:**
- Consumes: `LogActivity` (Task 1).
- Produces: `func (a *App) LogReadingSession(id string, seconds int) error`.

- [ ] **Step 1: Write the failing test**

Create `stats_methods_test.go`:

```go
package main

import (
	"testing"
	"time"

	"texere/internal/learning"
)

func TestLogReadingSessionRecordsMinutes(t *testing.T) {
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := &App{collectionDir: dir, learnStore: st}

	if err := a.LogReadingSession("r1", 600); err != nil {
		t.Fatalf("LogReadingSession: %v", err)
	}
	got, err := st.ActivityWeeks("read", 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 10 {
		t.Errorf("logged %d minutes, want 10", got[0])
	}
}

// A zero- or negative-length session is noise, not data: it would inflate the
// session count in the pipeline panel while contributing no time.
func TestLogReadingSessionIgnoresEmptySessions(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	t.Cleanup(func() { st.Close() })
	a := &App{collectionDir: dir, learnStore: st}

	if err := a.LogReadingSession("r1", 0); err != nil {
		t.Fatalf("a zero session must be ignored, not an error: %v", err)
	}
	if _, ok, _ := st.ActivitySince(); ok {
		t.Error("a zero-second session was written to the log")
	}
}

func TestRescheduleExtractLogsItsAction(t *testing.T) {
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.UpsertExtract(learning.Extract{
		ID: "e1", FilePath: "/c/n.n4l", Text: "x", Due: time.Now(), Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	a := &App{collectionDir: dir, learnStore: st}

	if err := a.DismissExtract("e1"); err != nil {
		t.Fatalf("DismissExtract: %v", err)
	}
	got, err := st.ActivityWeeks("process", 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 1 {
		t.Errorf("processed count = %d, want 1 — processing must be logged", got[0])
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test . -run 'LogReadingSession|RescheduleExtractLogs' -v`
Expected: FAIL — `a.LogReadingSession undefined`.

- [ ] **Step 3: Implement the binding**

Create `stats_methods.go`:

```go
package main

import (
	"errors"
	"time"
)

// LogReadingSession records time spent reading one document.
//
// Called by ReadingView when it leaves a document, and only then: a peek is
// suppressed at the call site alongside the three progress writes, so glancing
// at an extract never counts as reading.
func (a *App) LogReadingSession(id string, seconds int) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	if seconds <= 0 {
		return nil // opening and immediately leaving is not a session
	}
	return a.learnStore.LogActivity("read", id, "", seconds, time.Now())
}
```

- [ ] **Step 4: Implement the process write-point**

In `study_methods.go`, `rescheduleExtract` currently ends:

```go
	return a.learnStore.ApplyExtractSchedule(id, learning.ScheduleExtract(e, action, time.Now()))
```

Replace with:

```go
	if err := a.learnStore.ApplyExtractSchedule(id, learning.ScheduleExtract(e, action, time.Now())); err != nil {
		return err
	}
	// Logged here rather than in the store: ApplyExtractSchedule only sees the
	// resulting row, and `active=0` cannot tell "promoted to a card" from
	// "dismissed". The action string is the only place that distinction exists.
	// Best-effort — a statistics gap must never fail the user's action.
	_ = a.learnStore.LogActivity("process", id, action, 0, time.Now())
	return nil
```

- [ ] **Step 5: Implement the reading write-point**

In `ReadingView.svelte`, add `LogReadingSession` to the existing `wailsjs` import, and record the mount time beside the other state:

```js
  // Session start, for the activity log. Wall-clock rather than a timer: the
  // number only needs to be right to the second.
  const openedAt = Date.now()
```

Then in **`flush()`** and in **`close(action)`**, immediately after the existing
`if (pos) await UpdateReadingProgress(...)` line, add:

```js
      if (!peekAt) LogReadingSession(reading.id, Math.round((Date.now() - openedAt) / 1000)).catch(() => {})
```

and in **`onDestroy`**, inside the existing `if (!closed) {` block, after its
progress write:

```js
      if (!peekAt) LogReadingSession(reading.id, Math.round((Date.now() - openedAt) / 1000)).catch(() => {})
```

All three sites are guarded by `peekAt` for the same reason the progress writes
are: a peek is a glance, not a reading session.

- [ ] **Step 6: Verify**

Run: `go test . -run 'LogReadingSession|RescheduleExtractLogs' -v`, then `make test`, `make test-js`, `cd frontend && npx vite build`.

Then: `grep -c "LogReadingSession" frontend/src/components/ReadingView.svelte`
Expected: **4** — the import plus the three guarded call sites. Fewer means a leave-path logs nothing; the symptom is reading time that silently under-counts depending on how you left the document.

- [ ] **Step 7: Negative controls (report both)**

1. Delete the `_ = a.learnStore.LogActivity("process", ...)` line. `TestRescheduleExtractLogsItsAction` must FAIL. Restore.
2. Change `if seconds <= 0` to `if seconds < 0`. `TestLogReadingSessionIgnoresEmptySessions` must FAIL. Restore.

- [ ] **Step 8: Regenerate bindings and commit**

```bash
/home/simone/go/bin/wails generate module
git add study_methods.go stats_methods.go stats_methods_test.go \
  frontend/src/components/ReadingView.svelte \
  frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js
git commit -m "feat(stats): log reading sessions and extract processing"
git show --stat HEAD
```

---

### Task 3: Review-derived statistics

**Files:**
- Create: `internal/learning/stats.go`
- Create: `internal/learning/stats_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces, in `internal/learning/stats.go`:

```go
type DayCount struct {
	Day   string `json:"day"`   // "2006-01-02", local
	Count int    `json:"count"`
}

type QualityStats struct {
	Reviews30       int     `json:"reviews30"`
	ForgettingIndex float64 `json:"forgettingIndex"` // percent; -1 when no data
	Retention       float64 `json:"retention"`       // percent; -1 when no data
	GradeMix        [4]int  `json:"gradeMix"`        // 0=Again 1=Hard 2=Good 3=Easy
	AvgLatencyMs    int     `json:"avgLatencyMs"`
}

type HabitStats struct {
	ReviewsPerDay []DayCount `json:"reviewsPerDay"` // 84 entries, oldest first
	CurrentStreak int        `json:"currentStreak"`
	LongestStreak int        `json:"longestStreak"`
}

func (s *Store) QualityStatsFor(now time.Time) (QualityStats, error)
func (s *Store) HabitStatsFor(now time.Time) (HabitStats, error)
func streaks(days []DayCount) (current, longest int)
```

`-1` is the "not enough data" sentinel. Returning `0` would render as a perfect
0% forgetting index on a collection that has never been reviewed.

Grades are `GradeAgain = 1` through `GradeEasy = 4` (`internal/learning/types.go`).

- [ ] **Step 1: Write the failing test**

Create `internal/learning/stats_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/learning/ -run 'QualityStats|HabitStats|Streaks' -v`
Expected: FAIL — undefined types and methods.

- [ ] **Step 3: Implement**

Create `internal/learning/stats.go`:

```go
package learning

import "time"

// DayCount is one day of a dense series — every day in the window is present,
// quiet ones as zero, so a chart can index by position without gaps.
type DayCount struct {
	Day   string `json:"day"` // "2006-01-02", local
	Count int    `json:"count"`
}

// QualityStats answers "is the learning working?".
//
// ForgettingIndex and Retention use -1 as a "no data" sentinel. Zero would be
// indistinguishable from a flawless collection, and a fresh install would
// proudly report a 0% forgetting index having never been reviewed.
type QualityStats struct {
	Reviews30       int     `json:"reviews30"`
	ForgettingIndex float64 `json:"forgettingIndex"`
	Retention       float64 `json:"retention"`
	GradeMix        [4]int  `json:"gradeMix"` // 0=Again 1=Hard 2=Good 3=Easy
	AvgLatencyMs    int     `json:"avgLatencyMs"`
}

type HabitStats struct {
	ReviewsPerDay []DayCount `json:"reviewsPerDay"`
	CurrentStreak int        `json:"currentStreak"`
	LongestStreak int        `json:"longestStreak"`
}

const statsWindowDays = 30
const habitWindowDays = 84 // 12 weeks

func (s *Store) QualityStatsFor(now time.Time) (QualityStats, error) {
	var out QualityStats
	out.ForgettingIndex = -1
	out.Retention = -1

	since := midnight(now).AddDate(0, 0, -(statsWindowDays - 1)).Unix()
	rows, err := s.db.Query(
		`SELECT grade, elapsed_ms FROM review_logs WHERE reviewed_at >= ?`, since)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	totalMs := 0
	for rows.Next() {
		var grade int
		var elapsed sql.NullInt64
		if err := rows.Scan(&grade, &elapsed); err != nil {
			return out, err
		}
		if grade >= 1 && grade <= 4 {
			out.GradeMix[grade-1]++
		}
		out.Reviews30++
		totalMs += int(elapsed.Int64)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if out.Reviews30 == 0 {
		return out, nil // sentinels stand
	}
	out.ForgettingIndex = float64(out.GradeMix[0]) / float64(out.Reviews30) * 100
	out.Retention = 100 - out.ForgettingIndex
	out.AvgLatencyMs = totalMs / out.Reviews30
	return out, nil
}

func (s *Store) HabitStatsFor(now time.Time) (HabitStats, error) {
	var out HabitStats
	start := midnight(now).AddDate(0, 0, -(habitWindowDays - 1))

	// Dense skeleton first, then fill: a chart indexes by position, and a
	// sparse result would silently shift every bar left by the quiet days.
	byDay := make(map[string]int, habitWindowDays)
	rows, err := s.db.Query(`SELECT reviewed_at FROM review_logs WHERE reviewed_at >= ?`, start.Unix())
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var at int64
		if err := rows.Scan(&at); err != nil {
			return out, err
		}
		byDay[time.Unix(at, 0).Format("2006-01-02")]++
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	out.ReviewsPerDay = make([]DayCount, habitWindowDays)
	for i := 0; i < habitWindowDays; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		out.ReviewsPerDay[i] = DayCount{Day: day, Count: byDay[day]}
	}
	out.CurrentStreak, out.LongestStreak = streaks(out.ReviewsPerDay)
	return out, nil
}

// streaks counts consecutive active days in a dense, oldest-first series.
// current is the run ending on the final entry — zero if that day is quiet,
// because a streak you have already broken is not a current streak.
func streaks(days []DayCount) (current, longest int) {
	run := 0
	for _, d := range days {
		if d.Count > 0 {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	return run, longest
}

// midnight is the local start of now's day. Day boundaries must be local:
// bucketing on at/86400 is UTC and drops an evening review into the wrong day.
func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
```

Add `"database/sql"` to the imports.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/learning/ -run 'QualityStats|HabitStats|Streaks' -v` then `make test`
Expected: PASS (7 tests).

- [ ] **Step 5: Negative controls (report all three)**

1. Change `out.ForgettingIndex = -1` / `out.Retention = -1` to `0`. `TestQualityStatsReportsMinusOneWithNoReviews` must FAIL. Restore.
2. In `HabitStatsFor`, replace the dense fill with `out.ReviewsPerDay = append(out.ReviewsPerDay, DayCount{day, n})` for present days only. `TestHabitStatsReviewsPerDayIsDenseAndOldestFirst` must FAIL on the length. Restore.
3. In `streaks`, `return longest, longest`. `TestStreaksCurrentIsZeroWhenTheLastDayIsQuiet` must FAIL. Restore.

- [ ] **Step 6: Commit**

```bash
git add internal/learning/stats.go internal/learning/stats_test.go
git commit -m "feat(learning): forgetting index, grade mix and review streaks"
```

---

### Task 4: Item, workload and pipeline statistics

**Files:**
- Modify: `internal/learning/stats.go`
- Modify: `internal/learning/stats_test.go`

**Interfaces:**
- Consumes: `midnight` and `statsWindowDays` from Task 3; the existing `StudyCounts(now)` returning `StudyStats3{Read, Process, Recall}`.
- Produces:

```go
type WorkloadStats struct {
	Burden       float64     `json:"burden"`       // reviews/day; -1 when no scheduled items
	DailyMinutes float64     `json:"dailyMinutes"` // -1 when burden or latency unknown
	Forecast     []int       `json:"forecast"`     // 14 entries, index 0 = today
	Backlog      StudyStats3 `json:"backlog"`
	Leeches      int         `json:"leeches"`
	Difficulty   [5]int      `json:"difficulty"`
	Memorized    int         `json:"memorized"`
}

type PipelineStats struct {
	Sources        int `json:"sources"`
	SourcesDone    int `json:"sourcesDone"`
	Extracts       int `json:"extracts"`
	ExtractsOpen   int `json:"extractsOpen"`
	Cards          int `json:"cards"`
	OldestOpenDays int `json:"oldestOpenDays"` // -1 when nothing is open
	ExtractsMade30 int `json:"extractsMade30"`
	CardsMade30    int `json:"cardsMade30"`
}

func (s *Store) WorkloadStatsFor(now time.Time, avgLatencyMs int) (WorkloadStats, error)
func (s *Store) PipelineStatsFor(now time.Time) (PipelineStats, error)
```

Burden is SuperMemo's Σ 1/interval, with `interval = max(1, (due − last_review) in days)`. An item that has never been reviewed has a NULL `last_review` and is **excluded** — counting it as interval 0 would divide by zero, and clamping it to 1 would claim a daily review that is not scheduled.

- [ ] **Step 1: Write the failing test**

Append to `internal/learning/stats_test.go`:

```go
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
	due2 := now.AddDate(0, 0, 0)   // interval 2 days -> 0.5
	lr2 := now.AddDate(0, 0, -10)
	due10 := now.AddDate(0, 0, 0)  // interval 10 days -> 0.1
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/learning/ -run 'Burden|Forecast|Leeches|Pipeline' -v`
Expected: FAIL — `st.WorkloadStatsFor undefined`.

- [ ] **Step 3: Implement**

Append to `internal/learning/stats.go`:

```go
const forecastDays = 14
const leechLapses = 5

type WorkloadStats struct {
	Burden       float64     `json:"burden"`
	DailyMinutes float64     `json:"dailyMinutes"`
	Forecast     []int       `json:"forecast"`
	Backlog      StudyStats3 `json:"backlog"`
	Leeches      int         `json:"leeches"`
	Difficulty   [5]int      `json:"difficulty"`
	Memorized    int         `json:"memorized"`
}

type PipelineStats struct {
	Sources        int `json:"sources"`
	SourcesDone    int `json:"sourcesDone"`
	Extracts       int `json:"extracts"`
	ExtractsOpen   int `json:"extractsOpen"`
	Cards          int `json:"cards"`
	OldestOpenDays int `json:"oldestOpenDays"`
	ExtractsMade30 int `json:"extractsMade30"`
	CardsMade30    int `json:"cardsMade30"`
}

// WorkloadStatsFor answers "what is coming, and can I sustain it?".
//
// Burden is SuperMemo's sum of interval reciprocals: an item on a 100-day
// interval costs 1/100 of a review per day, so the sum estimates reviews per
// day across the collection. avgLatencyMs comes from QualityStats — the caller
// passes it rather than this method re-querying review_logs.
func (s *Store) WorkloadStatsFor(now time.Time, avgLatencyMs int) (WorkloadStats, error) {
	var out WorkloadStats
	out.Burden = -1
	out.DailyMinutes = -1
	out.Forecast = make([]int, forecastDays)

	start := midnight(now)
	rows, err := s.db.Query(
		`SELECT due, last_review, lapses, difficulty, reps FROM items WHERE active=1`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	burden := 0.0
	scheduled := 0
	for rows.Next() {
		var due, lastReview sql.NullInt64
		var lapses, reps int
		var difficulty sql.NullFloat64
		if err := rows.Scan(&due, &lastReview, &lapses, &difficulty, &reps); err != nil {
			return out, err
		}
		if reps > 0 {
			out.Memorized++
		}
		if lapses >= leechLapses {
			out.Leeches++
		}
		if difficulty.Valid {
			b := int((difficulty.Float64 - 1) / 2) // 1-10 -> five buckets of two
			if b < 0 {
				b = 0
			}
			if b > 4 {
				b = 4
			}
			out.Difficulty[b]++
		}
		// An item never reviewed has no interval; counting it would invent a
		// review the scheduler has not planned.
		if due.Valid && lastReview.Valid {
			days := float64(due.Int64-lastReview.Int64) / 86400
			if days < 1 {
				days = 1
			}
			burden += 1 / days
			scheduled++
		}
		if due.Valid {
			idx := int(time.Unix(due.Int64, 0).Sub(start).Hours() / 24)
			if idx < 0 {
				idx = 0 // overdue items are due today
			}
			if idx < forecastDays {
				out.Forecast[idx]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if scheduled > 0 {
		out.Burden = burden
		if avgLatencyMs > 0 {
			out.DailyMinutes = burden * float64(avgLatencyMs) / 1000 / 60
		}
	}
	if out.Backlog, err = s.StudyCounts(now); err != nil {
		return out, err
	}
	return out, nil
}

// PipelineStatsFor answers "is material flowing, or piling up?".
func (s *Store) PipelineStatsFor(now time.Time) (PipelineStats, error) {
	var out PipelineStats
	out.OldestOpenDays = -1
	since := midnight(now).AddDate(0, 0, -(statsWindowDays - 1)).Unix()

	q := func(sql string, args []interface{}, dst *int) error {
		return s.db.QueryRow(sql, args...).Scan(dst)
	}
	if err := q(`SELECT COUNT(*) FROM reading_elements`, nil, &out.Sources); err != nil {
		return out, err
	}
	if err := q(`SELECT COUNT(*) FROM reading_elements WHERE done=1`, nil, &out.SourcesDone); err != nil {
		return out, err
	}
	if err := q(`SELECT COUNT(*) FROM extracts`, nil, &out.Extracts); err != nil {
		return out, err
	}
	if err := q(`SELECT COUNT(*) FROM extracts WHERE active=1`, nil, &out.ExtractsOpen); err != nil {
		return out, err
	}
	if err := q(`SELECT COUNT(*) FROM items WHERE active=1`, nil, &out.Cards); err != nil {
		return out, err
	}
	if err := q(`SELECT COUNT(*) FROM extracts WHERE created_at >= ?`,
		[]interface{}{since}, &out.ExtractsMade30); err != nil {
		return out, err
	}
	if err := q(`SELECT COUNT(*) FROM items WHERE created_at >= ?`,
		[]interface{}{since}, &out.CardsMade30); err != nil {
		return out, err
	}

	var oldest sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT MIN(created_at) FROM extracts WHERE active=1`).Scan(&oldest); err != nil {
		return out, err
	}
	if oldest.Valid {
		out.OldestOpenDays = int(now.Sub(time.Unix(oldest.Int64, 0)).Hours() / 24)
	}
	return out, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/learning/ -run 'Burden|Forecast|Leeches|Pipeline' -v` then `make test`
Expected: PASS (6 new tests, all earlier ones still green).

- [ ] **Step 5: Negative controls (report all three)**

1. Remove the `&& lastReview.Valid` condition (use `if due.Valid {` for the burden block, with `days` computed from `lastReview.Int64` which is 0 when NULL). `TestBurdenExcludesNeverReviewedItems` must FAIL. Restore.
2. Change `if idx < 0 { idx = 0 }` to `continue`. `TestForecastBucketsByDayFromToday` still passes — **this control does not distinguish the bug**, so instead change the forecast index to `idx := int(time.Unix(due.Int64, 0).Sub(start).Hours() / 24 / 7)`. `TestForecastBucketsByDayFromToday` must FAIL on the `+3 days` assertion. Restore.
3. Change `out.OldestOpenDays = -1` to `= 0`. `TestPipelineOldestOpenIsMinusOneWhenNothingIsOpen` must FAIL. Restore.

- [ ] **Step 6: Commit**

```bash
git add internal/learning/stats.go internal/learning/stats_test.go
git commit -m "feat(learning): burden, due forecast, leeches and pipeline counts"
```

---

### Task 5: The `GetStatistics` binding

**Files:**
- Modify: `stats_methods.go`
- Modify: `stats_methods_test.go`
- Regenerate: `frontend/wailsjs/go/main/App.d.ts`, `App.js`, `models.ts`

**Interfaces:**
- Consumes: `QualityStatsFor`, `HabitStatsFor`, `WorkloadStatsFor`, `PipelineStatsFor` (Tasks 3–4); `ActivityWeeks`, `ActivitySince` (Task 1).
- Produces:

```go
type StatsView struct {
	Quality      learning.QualityStats  `json:"quality"`
	Workload     learning.WorkloadStats `json:"workload"`
	Habit        learning.HabitStats    `json:"habit"`
	Pipeline     learning.PipelineStats `json:"pipeline"`
	CardsPerWeek []int                  `json:"cardsPerWeek"`
	ReadMinsWeek []int                  `json:"readMinsWeek"`
	ProcPerWeek  []int                  `json:"procPerWeek"`
	LoggingSince int64                  `json:"loggingSince"` // unix; 0 = never
}

func (a *App) GetStatistics() (*StatsView, error)
```

`cardsPerWeek` comes from `items.created_at` rather than the activity log, per the constraint that creation is never double-recorded. Add `CardsPerWeek(weeks int, now time.Time) ([]int, error)` to `internal/learning/stats.go` using the same bucketing shape as `ActivityWeeks`.

- [ ] **Step 1: Write the failing test**

Append to `stats_methods_test.go`:

```go
func TestGetStatisticsAssemblesEveryPanel(t *testing.T) {
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := &App{collectionDir: dir, learnStore: st}

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
	if got.LoggingSince != 0 {
		t.Errorf("loggingSince = %d, want 0 for a never-written log", got.LoggingSince)
	}
	// Dense series must be present and correctly sized even with no data, or
	// the charts index past the end of an array.
	if len(got.Habit.ReviewsPerDay) != 84 {
		t.Errorf("reviewsPerDay = %d entries, want 84", len(got.Habit.ReviewsPerDay))
	}
	if len(got.Workload.Forecast) != 14 {
		t.Errorf("forecast = %d entries, want 14", len(got.Workload.Forecast))
	}
	for name, series := range map[string][]int{
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test . -run TestGetStatistics -v`
Expected: FAIL — `a.GetStatistics undefined`.

- [ ] **Step 3: Implement `CardsPerWeek`**

Append to `internal/learning/stats.go`:

```go
// CardsPerWeek buckets item creation, oldest first, the last entry being the
// week containing now. Read from items.created_at rather than the activity log
// so creation has exactly one source of truth.
func (s *Store) CardsPerWeek(weeks int, now time.Time) ([]int, error) {
	if weeks <= 0 {
		return []int{}, nil // guard before make: a negative length panics
	}
	out := make([]int, weeks)
	start := midnight(now).AddDate(0, 0, -7*(weeks-1))
	rows, err := s.db.Query(`SELECT created_at FROM items WHERE created_at >= ?`, start.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var at int64
		if err := rows.Scan(&at); err != nil {
			return nil, err
		}
		idx := int(time.Unix(at, 0).Sub(start).Hours() / (24 * 7))
		if idx >= 0 && idx < weeks {
			out[idx]++
		}
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Implement the binding**

Append to `stats_methods.go`:

```go
const statsWeeks = 12

// StatsView is the whole Stats focus in one payload. Assembled in a single call
// so the four panels can never disagree about the moment they describe.
type StatsView struct {
	Quality      learning.QualityStats  `json:"quality"`
	Workload     learning.WorkloadStats `json:"workload"`
	Habit        learning.HabitStats    `json:"habit"`
	Pipeline     learning.PipelineStats `json:"pipeline"`
	CardsPerWeek []int                  `json:"cardsPerWeek"`
	ReadMinsWeek []int                  `json:"readMinsWeek"`
	ProcPerWeek  []int                  `json:"procPerWeek"`
	LoggingSince int64                  `json:"loggingSince"`
}

func (a *App) GetStatistics() (*StatsView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	now := time.Now()
	var v StatsView
	var err error

	if v.Quality, err = a.learnStore.QualityStatsFor(now); err != nil {
		return nil, err
	}
	// Latency is passed through rather than re-queried: two queries over the
	// same window could disagree if a review lands between them.
	if v.Workload, err = a.learnStore.WorkloadStatsFor(now, v.Quality.AvgLatencyMs); err != nil {
		return nil, err
	}
	if v.Habit, err = a.learnStore.HabitStatsFor(now); err != nil {
		return nil, err
	}
	if v.Pipeline, err = a.learnStore.PipelineStatsFor(now); err != nil {
		return nil, err
	}
	if v.CardsPerWeek, err = a.learnStore.CardsPerWeek(statsWeeks, now); err != nil {
		return nil, err
	}
	if v.ReadMinsWeek, err = a.learnStore.ActivityWeeks("read", statsWeeks, now); err != nil {
		return nil, err
	}
	if v.ProcPerWeek, err = a.learnStore.ActivityWeeks("process", statsWeeks, now); err != nil {
		return nil, err
	}
	if since, ok, err := a.learnStore.ActivitySince(); err != nil {
		return nil, err
	} else if ok {
		v.LoggingSince = since.Unix()
	}
	return &v, nil
}
```

Add `"texere/internal/learning"` to the imports.

- [ ] **Step 5: Run to verify it passes**

Run: `go test . -run TestGetStatistics -v` then `make test`
Expected: PASS.

- [ ] **Step 6: Report a known coverage gap — do not invent a control for it**

The tests in this task run against an empty collection, where burden is already
`-1`. So changing `WorkloadStatsFor(now, v.Quality.AvgLatencyMs)` to
`WorkloadStatsFor(now, 0)` breaks nothing: **the latency hand-off between
`QualityStatsFor` and `WorkloadStatsFor` is not covered by any test.**

Do not paper over this by weakening a test until it passes. Verify by reading
that with a zero latency `DailyMinutes` keeps its `-1` sentinel even when burden
is known, and state the gap plainly in your report. It belongs in the manual
plan (Task 8, case 5) instead, where a real collection has both numbers.

- [ ] **Step 7: Regenerate bindings and commit**

```bash
/home/simone/go/bin/wails generate module
git add stats_methods.go stats_methods_test.go internal/learning/stats.go \
  frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js frontend/wailsjs/go/models.ts
git commit -m "feat(stats): assemble every panel in one binding"
git show --stat HEAD
```

---

### Task 6: Chart arithmetic

**Files:**
- Create: `frontend/src/lib/stats-format.js`
- Create: `frontend/src/lib/stats-format.test.js`

**Interfaces:**
- Consumes: nothing — dependency-free by design.
- Produces:
  - `barHeights(values, height) -> number[]` — each value scaled to `height`, tallest exactly `height`; an all-zero or empty series gives all zeros (never `NaN`).
  - `sparkPath(values, width, height) -> string` — an SVG `path` `d`; `''` for fewer than two points.
  - `humanDuration(minutes) -> string` — `'0m'`, `'45m'`, `'2h 5m'`.
  - `fiBand(fi) -> 'low'|'ok'|'high'|'unknown'` — SuperMemo's 7–13% target; `-1` is `'unknown'`.

Look at `frontend/src/lib/sources-filter.js` for house style before writing.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/stats-format.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { barHeights, sparkPath, humanDuration, fiBand } from './stats-format.js'

test('bars scale so the tallest fills the height', () => {
  assert.deepEqual(barHeights([1, 2, 4], 100), [25, 50, 100])
})

// An empty collection must render flat bars, not a chart full of NaN paths.
test('an all-zero series gives zero-height bars, not NaN', () => {
  assert.deepEqual(barHeights([0, 0, 0], 100), [0, 0, 0])
  assert.deepEqual(barHeights([], 100), [])
})

test('a single point still scales', () => {
  assert.deepEqual(barHeights([7], 50), [50])
})

test('sparkPath spans the full width and starts with a move', () => {
  const d = sparkPath([0, 10, 5], 100, 20)
  assert.match(d, /^M0[ ,]/)
  assert.ok(d.includes('100'), 'the last point must reach the right edge')
  assert.ok(!d.includes('NaN'), 'a NaN in a path silently renders nothing')
})

test('sparkPath needs at least two points', () => {
  assert.equal(sparkPath([5], 100, 20), '')
  assert.equal(sparkPath([], 100, 20), '')
})

test('humanDuration reads naturally at each scale', () => {
  assert.equal(humanDuration(0), '0m')
  assert.equal(humanDuration(45), '45m')
  assert.equal(humanDuration(125), '2h 5m')
  assert.equal(humanDuration(120), '2h')
})

// The band is the whole point of showing a forgetting index — a bare number
// does not tell you whether it is healthy.
test('fiBand classifies against the 7-13% target', () => {
  assert.equal(fiBand(-1), 'unknown')
  assert.equal(fiBand(4), 'low')
  assert.equal(fiBand(7), 'ok')
  assert.equal(fiBand(10), 'ok')
  assert.equal(fiBand(13), 'ok')
  assert.equal(fiBand(20), 'high')
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/stats-format.test.js`
Expected: FAIL — cannot resolve `./stats-format.js`.

- [ ] **Step 3: Implement**

Create `frontend/src/lib/stats-format.js`:

```js
// Chart arithmetic for the Stats focus.
//
// Dependency-free and out of the component because this repo has no Svelte
// component test harness — anything left in a .svelte file has no tests. It
// also means every NaN guard below is actually covered.

/** Scale values so the tallest is exactly `height`. All-zero gives all zeros. */
export function barHeights(values, height) {
  const list = Array.isArray(values) ? values : []
  const max = Math.max(0, ...list)
  // Guard the divide rather than the render: a NaN height makes an SVG rect
  // vanish silently, which reads as "no data" instead of "bug".
  if (max <= 0) return list.map(() => 0)
  return list.map((v) => (Math.max(0, v) / max) * height)
}

/** An SVG path spanning the full width; '' for fewer than two points. */
export function sparkPath(values, width, height) {
  const list = Array.isArray(values) ? values : []
  if (list.length < 2) return ''
  const max = Math.max(0, ...list)
  const step = width / (list.length - 1)
  return list
    .map((v, i) => {
      const y = max > 0 ? height - (Math.max(0, v) / max) * height : height
      return `${i === 0 ? 'M' : 'L'}${round(i * step)} ${round(y)}`
    })
    .join(' ')
}

export function humanDuration(minutes) {
  const m = Math.max(0, Math.round(Number(minutes) || 0))
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  const rest = m % 60
  return rest ? `${h}h ${rest}m` : `${h}h`
}

/**
 * SuperMemo's target forgetting index is 7–13%: below it you are reviewing more
 * than you need to, above it you are losing material.
 */
export function fiBand(fi) {
  const v = Number(fi)
  if (!Number.isFinite(v) || v < 0) return 'unknown'
  if (v < 7) return 'low'
  if (v > 13) return 'high'
  return 'ok'
}

function round(n) {
  return Math.round(n * 100) / 100
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `node --test frontend/src/lib/stats-format.test.js` then `make test-js`
Expected: all PASS.

- [ ] **Step 5: Negative controls (report both)**

1. Remove the `if (max <= 0) return list.map(() => 0)` guard in `barHeights`. The all-zero test must FAIL with `NaN`. Restore.
2. Change `if (v > 13) return 'high'` to `if (v > 14)`. The `fiBand` test must FAIL at the boundary. Restore.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/stats-format.js frontend/src/lib/stats-format.test.js
git commit -m "feat(stats): chart arithmetic with guarded empty states"
```

---

### Task 7: The Stats panel and its tab

**Files:**
- Create: `frontend/src/components/StatsPanel.svelte`
- Modify: `frontend/src/components/StudySurface.svelte`

**Interfaces:**
- Consumes: `GetStatistics` (Task 5) and every export of `stats-format.js` (Task 6).
- Produces: no new exports.

Three hazards, each of which has already cost this codebase a commit:

1. **`.ss-body` is `position: relative` with absolutely-positioned children.** The panel's root must be `position: absolute; inset: 0` with its own scrolling, or it renders under the other surfaces and is invisible.
2. **Svelte instruments an assignment only when the template references the variable.** Every piece of state this panel mutates must be read in the markup, or the update schedules no re-render.
3. **A `-1` sentinel must never reach the screen.** Every metric that can be `-1` needs an explicit branch showing "Not enough data yet".

- [ ] **Step 1: Create the component**

Create `frontend/src/components/StatsPanel.svelte` with four cards in a grid.

Structure it as:

```svelte
<script>
  // Four panels over one payload, loaded when the tab is entered. The whole
  // Stats focus is a single GetStatistics call so the panels can never disagree
  // about the moment they describe.
  import { onMount } from 'svelte'
  import { GetStatistics } from '../../wailsjs/go/main/App'
  import { barHeights, sparkPath, humanDuration, fiBand } from '../lib/stats-format.js'

  let s = null
  let error = ''

  onMount(load)
  export function refresh() { return load() }

  async function load() {
    error = ''
    try { s = await GetStatistics() } catch (e) { error = String(e?.message ?? e) }
  }

  // -1 is the "never measured" sentinel from the Go side. It must never be
  // rendered: 0% would read as a flawless collection nobody has studied.
  const has = (v) => typeof v === 'number' && v >= 0
  const pct = (v) => `${Math.round(v)}%`
  const fmtDate = (unix) => new Date(unix * 1000).toLocaleDateString()
</script>
```

Then the markup: `{#if error}` an error line; `{:else if !s}` a loading line; `{:else}` a `.sp-grid` containing four `.sp-card` sections:

- **Learning quality** — the forgetting index with `class:ok / class:low / class:high` from `fiBand`, or "Not enough data yet" when `!has(s.quality.forgettingIndex)`; retention; a four-segment grade-mix bar built with `barHeights(s.quality.gradeMix, 60)`; the leech count from `s.workload.leeches`; the difficulty histogram from `barHeights(s.workload.difficulty, 40)`.
- **Workload** — burden to one decimal or "Not enough data yet"; `humanDuration(s.workload.dailyMinutes)`; a 14-bar forecast from `barHeights(s.workload.forecast, 60)`; the read/process/recall backlog.
- **Habit** — an SVG `<path d={sparkPath(s.habit.reviewsPerDay.map((d) => d.count), 260, 40)} />`; current and longest streak; memorized; a 12-bar cards-per-week chart; and, when `s.loggingSince` is non-zero, read minutes per week — otherwise the line "Collecting reading time since today".
- **Pipeline** — the funnel counts, `s.pipeline.oldestOpenDays` guarded by `has()`, the 30-day creation rates, and processed-per-week under the same `loggingSince` guard.

Every card is a `<section class="sp-card">` with an `<h3>`. Charts are inline `<svg>` with `<rect>` or `<path>` — no library, no external fetch.

Styles follow the existing dark palette used by `SourcesSidebar.svelte`: `#131c29` card background, `#2a3a4a` borders, `#e8eaf0` text, `#7dd3fc` headings, `#7ecfa0` for good values and `#e0b000` for warnings. Root:

```css
  .sp-root { position: absolute; inset: 0; overflow: auto; padding: 12px; }
  .sp-grid { display: grid; gap: 12px; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); }
```

- [ ] **Step 2: Add the tab**

In `StudySurface.svelte`, add the import:

```js
  import StatsPanel from './StatsPanel.svelte'
```

Extend the tab list at `:165` — it currently reads:

```svelte
    {#each [['next','Next'],['read','Read'],['process','Process'],['recall','Recall']] as [k, label]}
```

Add `['stats','Stats']` as a fifth pair.

Add a final branch in `.ss-body`, after the `recall` branch and before its closing `{/if}`:

```svelte
    {:else if $studyFocus === 'stats'}
      <StatsPanel />
```

**Do not add `stats` to `nextKind()`.** That function picks what the Next focus rotates to; statistics are not a queue and must never be something the app sends you to.

- [ ] **Step 3: Verify**

Run: `make test`, `make test-js`, and `cd frontend && npx vite build`.
Expected: all pass, and **no unused-CSS-selector warning naming any `.sp-*` selector** — such a warning means the markup and styles disagree.

Then: `grep -n "stats" frontend/src/components/StudySurface.svelte`
Expected: the tab pair and the branch, and **nothing inside `nextKind`**.

- [ ] **Step 4: Report the symptoms**

State what a user would see if each were wrong: the `position: absolute; inset: 0` root, the `has()` guard on a `-1`, and `stats` leaking into `nextKind()`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/StatsPanel.svelte frontend/src/components/StudySurface.svelte
git commit -m "feat(stats): the Stats focus"
```

---

### Task 8: Manual test plan

**Files:**
- Create: `docs/superpowers/specs/2026-08-11-study-statistics-manual-test-plan.md`

Match the house format of `docs/superpowers/specs/2026-08-07-llm-source-manual-test-plan.md`: a preamble stating what is automated and what is not, the deliberate behaviours, prereqs, numbered cases with observable results, `**[DB]**` on anything invisible, and a closing "what I most want to hear about".

Read the four panels as built before writing, so the cases match what shipped.

- [ ] **Step 1: Write the document**

Cover at least:

1. The Stats tab appears beside Next/Read/Process/Recall and opens the four panels.
2. On a **fresh collection**, every panel reads "Not enough data yet" — no `0%` forgetting index, no `NaN`, no empty chart area that looks broken.
3. Forgetting index matches a hand-count: grade a few cards, note how many you pressed Again, and check the percentage. **[DB]** `SELECT grade, COUNT(*) FROM review_logs GROUP BY grade;`
4. The band colouring changes as the index crosses 7% and 13%.
5. Burden and estimated daily minutes are plausible against the due forecast.
6. The forecast's first bar equals today's due count, matching the Recall tab's badge.
7. Streak increments after studying on a new day and shows zero after a missed day.
8. **Reading a document logs a session; peeking an extract does not.** Read a document for a minute and close it; check the reading-time figure. Then peek an extract from the sidebar, close, and confirm the figure is unchanged. **[DB]** `SELECT kind, amount, detail FROM activity_log ORDER BY at DESC LIMIT 5;`
9. Processing an extract — promote, dismiss and defer — each adds a `process` row with the right `detail`.
10. The funnel counts match the sidebar and the Process badge.
11. The log-backed charts show "collecting since …" until they have data, and start filling afterwards.
12. Leaving Stats and returning reloads the numbers rather than showing a stale snapshot.

State explicitly, so a tester does not file them as bugs: statistics are a snapshot taken when the tab is entered, not live; reading history before this feature does not exist and cannot be backfilled; and `Next` never rotates to Stats.

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-08-11-study-statistics-manual-test-plan.md
git commit -m "docs(test-plan): study statistics"
```

---

## Self-Review

**Spec coverage.** Schema v6 and the two write-points → Tasks 1–2, including the `peekAt` guard. All 19 metrics in the spec's table → Tasks 3–5: quality and habit in Task 3, workload and pipeline in Task 4, the weekly series and `loggingSince` in Task 5. Inline-SVG charts with no library → Tasks 6–7. Empty-collection safety → the `-1` sentinel, tested in Tasks 3, 4 and 5 and rendered in Task 7. The fourth tab → Task 7. Manual coverage → Task 8.

**Placeholder scan.** None: every code step carries literal code and every test its assertions. Task 7 is specified structurally rather than as one long literal component — the card contents are enumerated field by field with the exact expressions to use, which is the level at which a reviewer can reject it.

**Type consistency.** `QualityStats`, `WorkloadStats`, `HabitStats`, `PipelineStats` and `DayCount` are defined in Tasks 3–4 and embedded unchanged in Task 5's `StatsView`. The JSON names used in Task 7 (`s.quality.forgettingIndex`, `s.workload.forecast`, `s.habit.reviewsPerDay`, `s.pipeline.oldestOpenDays`, `s.cardsPerWeek`, `s.readMinsWeek`, `s.procPerWeek`, `s.loggingSince`) match those struct tags exactly. `barHeights`, `sparkPath`, `humanDuration` and `fiBand` are defined in Task 6 and called with those names in Task 7. `LogActivity(kind, refID, detail string, amount int, at time.Time)` is defined in Task 1 and called with that argument order in Task 2 and nowhere else.

**One control that does not work, called out rather than hidden.** Task 4's second negative control has an obvious form that fails to distinguish the bug — clamping overdue items to today is unobservable in that fixture. The plan says so and substitutes a control that does fail. Task 5 likewise has no test covering the latency hand-off between `QualityStatsFor` and `WorkloadStatsFor`; the plan instructs the implementer to report that gap rather than invent a passing control. Both are places this project has previously shipped a green test over a real bug.
