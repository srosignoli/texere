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
