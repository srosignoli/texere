package main

import (
	"errors"
	"time"

	"texere/internal/content"
	"texere/internal/learning"
)

var errNoStore = errors.New("no learning store (open a collection first)")

// collectionN4LFiles lists the .n4l files of the open collection.
func (a *App) collectionN4LFiles() []string {
	sums, err := content.ScanCollection(a.collectionDir)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(sums))
	for _, s := range sums {
		if s.FilePath != "" && s.ScanError == nil {
			files = append(files, s.FilePath)
		}
	}
	return files
}

// SyncStudyItems discovers study items across the collection into learning.db.
func (a *App) SyncStudyItems() (*learning.SyncResult, error) {
	if a.learnStore == nil {
		return nil, errNoStore
	}
	res, err := learning.Sync(a.learnStore, a.collectionN4LFiles(), a.registry)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// GetDueQueue returns up to limit due/new cards.
func (a *App) GetDueQueue(limit int) ([]learning.CardView, error) {
	if a.learnStore == nil {
		return nil, errNoStore
	}
	if limit <= 0 {
		limit = 50
	}
	return a.learnStore.DueQueue(time.Now(), limit)
}

// GradeItem applies an FSRS grade to one item and returns the next due card
// (nil when the queue is empty).
func (a *App) GradeItem(itemID string, grade int, elapsedMs int) (*learning.CardView, error) {
	if a.learnStore == nil {
		return nil, errNoStore
	}
	now := time.Now()
	prev, ok, err := a.learnStore.LoadSched(itemID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("item not found: " + itemID)
	}
	next, err := learning.Schedule(prev, learning.Grade(grade), now)
	if err != nil {
		return nil, err
	}
	if err := a.learnStore.ApplyReview(itemID, next, learning.Grade(grade), elapsedMs, prev.State, now); err != nil {
		return nil, err
	}
	q, err := a.learnStore.DueQueue(now, 1)
	if err != nil {
		return nil, err
	}
	if len(q) == 0 {
		return nil, nil
	}
	return &q[0], nil
}

// GetStudyStats returns due/new/total counts for the Study entry badge.
func (a *App) GetStudyStats() (*learning.Stats, error) {
	if a.learnStore == nil {
		return nil, errNoStore
	}
	st, err := a.learnStore.CountStats(time.Now())
	if err != nil {
		return nil, err
	}
	return &st, nil
}
