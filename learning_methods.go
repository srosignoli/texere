package main

import (
	"errors"
	"time"

	"texere/internal/collection"
	"texere/internal/learning"
)

// SyncStudyItems discovers study items across the collection into learning.db.
func (a *App) SyncStudyItems() (*learning.SyncResult, error) {
	s, err := a.session()
	if err != nil {
		return nil, err
	}
	res, err := s.Sync()
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// GetDueQueue returns up to limit due/new cards.
func (a *App) GetDueQueue(limit int) ([]learning.CardView, error) {
	return inCollection(a, func(h collection.Handles) ([]learning.CardView, error) {
		if limit <= 0 {
			limit = 50
		}
		q, err := h.Learning().DueQueue(time.Now(), limit)
		if err != nil {
			return nil, err
		}
		return q, nil
	})
}

// GradeItem applies an FSRS grade to one item and returns the next due card
// (nil when the queue is empty).
func (a *App) GradeItem(itemID string, grade int, elapsedMs int) (*learning.CardView, error) {
	return inCollection(a, func(h collection.Handles) (*learning.CardView, error) {
		now := time.Now()
		prev, ok, err := h.Learning().LoadSched(itemID)
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
		if err := h.Learning().ApplyReview(itemID, next, learning.Grade(grade), elapsedMs, prev.State, now); err != nil {
			return nil, err
		}
		q, err := h.Learning().DueQueue(now, 1)
		if err != nil {
			return nil, err
		}
		if len(q) == 0 {
			return nil, nil
		}
		return &q[0], nil
	})
}

// GetStudyStats returns due/new/total counts for the Study entry badge.
func (a *App) GetStudyStats() (*learning.Stats, error) {
	return inCollection(a, func(h collection.Handles) (*learning.Stats, error) {
		st, err := h.Learning().CountStats(time.Now())
		if err != nil {
			return nil, err
		}
		return &st, nil
	})
}
