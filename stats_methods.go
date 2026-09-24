package main

import (
	"time"

	"texere/internal/collection"
	"texere/internal/learning"
)

// LogReadingSession records time spent reading one document.
//
// Called by ReadingView when it leaves a document, and only then: a peek is
// suppressed at the call site alongside the three progress writes, so glancing
// at an extract never counts as reading.
func (a *App) LogReadingSession(id string, seconds int) error {
	return a.withCollection(func(h collection.Handles) error {
		if seconds <= 0 {
			return nil // opening and immediately leaving is not a session
		}
		return h.Learning().LogActivity("read", id, "", seconds, time.Now())
	})
}

const statsWeeks = 12

// StatsView is the whole Stats focus in one payload. Assembled in a single call
// so the four panels can never disagree about the moment they describe.
type StatsView struct {
	Quality      learning.QualityStats  `json:"quality"`
	Workload     learning.WorkloadStats `json:"workload"`
	Habit        learning.HabitStats    `json:"habit"`
	Pipeline     learning.PipelineStats `json:"pipeline"`
	CardsPerWeek []learning.WeekCount   `json:"cardsPerWeek"`
	ReadMinsWeek []learning.WeekCount   `json:"readMinsWeek"`
	ProcPerWeek  []learning.WeekCount   `json:"procPerWeek"`
	// Per-kind, not one shared flag: a single dismissed extract must not switch
	// the reading chart out of its "still collecting" state and have it claim
	// twelve weeks of zero reading. 0 means nothing of that kind is logged.
	ReadSince int64 `json:"readSince"`
	ProcSince int64 `json:"procSince"`
}

func (a *App) GetStatistics() (*StatsView, error) {
	return inCollection(a, func(h collection.Handles) (*StatsView, error) {
		now := time.Now()
		var v StatsView
		var err error

		if v.Quality, err = h.Learning().QualityStatsFor(now); err != nil {
			return nil, err
		}
		// Latency is passed through rather than re-queried: two queries over the
		// same window could disagree if a review lands between them.
		if v.Workload, err = h.Learning().WorkloadStatsFor(now, v.Quality.AvgLatencyMs); err != nil {
			return nil, err
		}
		if v.Habit, err = h.Learning().HabitStatsFor(now); err != nil {
			return nil, err
		}
		if v.Pipeline, err = h.Learning().PipelineStatsFor(now); err != nil {
			return nil, err
		}
		if v.CardsPerWeek, err = h.Learning().CardsPerWeek(statsWeeks, now); err != nil {
			return nil, err
		}
		if v.ReadMinsWeek, err = h.Learning().ActivityWeeks("read", statsWeeks, now); err != nil {
			return nil, err
		}
		if v.ProcPerWeek, err = h.Learning().ActivityWeeks("process", statsWeeks, now); err != nil {
			return nil, err
		}
		if since, ok, err := h.Learning().ActivitySince("read"); err != nil {
			return nil, err
		} else if ok {
			v.ReadSince = since.Unix()
		}
		if since, ok, err := h.Learning().ActivitySince("process"); err != nil {
			return nil, err
		} else if ok {
			v.ProcSince = since.Unix()
		}
		return &v, nil
	})
}
