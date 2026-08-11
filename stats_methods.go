package main

import (
	"errors"
	"time"

	"texere/internal/learning"
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
