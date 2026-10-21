package main

import (
	"texere/internal/collection"
)

// SittingView is a Sitting just begun: its id and its Source as it is now.
type SittingView struct {
	ID     string      `json:"id"`
	Source ReadingView `json:"source"`
}

// SittingEnd is what the view knows when a Sitting ends. Notes is the unsaved
// notes buffer, or nil when it is unchanged.
type SittingEnd struct {
	Position string  `json:"position"`
	Action   string  `json:"action"` // "continue" | "later" | "done"
	Notes    *string `json:"notes"`
}

// BeginSitting opens a Sitting on a Source. peekAnchor is the passage a peek
// opens at, or "" to read from the saved position.
func (a *App) BeginSitting(sourceID, peekAnchor string) (*SittingView, error) {
	return inCollection(a, func(h collection.Handles) (*SittingView, error) {
		id, src, err := h.Sittings().Begin(sourceID, peekAnchor)
		if err != nil {
			return nil, err
		}
		return &SittingView{ID: id, Source: toReadingView(src)}, nil
	})
}

// SittingProgress records a position a pane reports while the user reads.
func (a *App) SittingProgress(sittingID, position string) error {
	return a.withCollection(func(h collection.Handles) error {
		_, err := h.Sittings().Progress(sittingID, position)
		return err
	})
}

// EndSitting ends a Sitting and returns the final notes content when notes
// were sent. Ending one that is not open does nothing.
func (a *App) EndSitting(sittingID string, end SittingEnd) (string, error) {
	var final string
	err := a.withCollection(func(h collection.Handles) error {
		var err error
		final, err = h.Sittings().End(sittingID, end.Position, end.Action, end.Notes)
		return err
	})
	return final, err
}
