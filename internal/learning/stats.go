package learning

import (
	"database/sql"
	"time"
)

// DayCount is one day of a dense series — every day in the window is present,
// quiet ones as zero, so a chart can index by position without gaps.
type DayCount struct {
	Day   string `json:"day"` // "2006-01-02", local
	Count int    `json:"count"`
}

// WeekCount is one bucket of a weekly series. WeekStart labels it with the local
// date of the bucket's first day, so a chart axis and the bucket it describes
// come from the same arithmetic and cannot drift apart.
type WeekCount struct {
	WeekStart string `json:"weekStart"`
	Count     int    `json:"count"`
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
	totalMs, timed := 0, 0
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
		// A review whose latency was never recorded is counted as a review but
		// not as a timed one. Folding it in as zero would drag the mean toward
		// nothing and understate the workload estimate that divides by it.
		if elapsed.Valid {
			totalMs += int(elapsed.Int64)
			timed++
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if out.Reviews30 == 0 {
		return out, nil // sentinels stand
	}
	out.ForgettingIndex = float64(out.GradeMix[0]) / float64(out.Reviews30) * 100
	out.Retention = 100 - out.ForgettingIndex
	if timed > 0 {
		out.AvgLatencyMs = totalMs / timed
	}
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
		// A never-reviewed card has a NULL due and is available NOW — StudyCounts
		// (queue.go), DueQueue and CountStats.New all treat it that way. Dropping
		// it here printed fourteen empty bars beside a non-zero Recall backlog,
		// answering "nothing is coming" when a hundred new cards were waiting.
		if !due.Valid {
			out.Forecast[0]++
		} else {
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

	// Named `query`, not `sql`: shadowing the imported package name inside this
	// function would hide database/sql from anything added below it later.
	q := func(query string, args []interface{}, dst *int) error {
		return s.db.QueryRow(query, args...).Scan(dst)
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

// CardsPerWeek buckets item creation, oldest first, the last entry being the
// week containing now. Read from items.created_at rather than the activity log
// so creation has exactly one source of truth.
func (s *Store) CardsPerWeek(weeks int, now time.Time) ([]WeekCount, error) {
	if weeks <= 0 {
		return []WeekCount{}, nil // guard before make: a negative length panics
	}
	out := make([]WeekCount, weeks)
	start := midnight(now).AddDate(0, 0, -7*(weeks-1))
	for i := range out {
		out[i].WeekStart = start.AddDate(0, 0, 7*i).Format("2006-01-02")
	}
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
			out[idx].Count++
		}
	}
	return out, rows.Err()
}
