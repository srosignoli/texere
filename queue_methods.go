package main

import (
	"texere/internal/collection"
	"texere/internal/learning"
)

// TurnView is one Turn of the Study Queue: Kind names which of Source,
// Extract and Card is set.
type TurnView struct {
	Kind    string             `json:"kind"`
	Source  *ReadingView       `json:"source,omitempty"`
	Extract *ExtractView       `json:"extract,omitempty"`
	Card    *learning.CardView `json:"card,omitempty"`
}

// NextTurn returns the Turn to study now, or nil when nothing is due. Asking
// again before the Turn has ended returns the same Turn.
func (a *App) NextTurn() (*TurnView, error) {
	return inCollection(a, func(h collection.Handles) (*TurnView, error) {
		t, err := h.Queue().Next()
		if err != nil || t == nil {
			return nil, err
		}
		v := &TurnView{Kind: string(t.Kind)}
		switch t.Kind {
		case learning.KindSource:
			rv := toReadingView(*t.Source)
			v.Source = &rv
		case learning.KindExtract:
			e := t.Extract
			v.Extract = &ExtractView{ID: e.ID, Text: e.Text, FromAnchor: e.FromAnchor, FilePath: e.FilePath}
		case learning.KindCard:
			v.Card = t.Card
		}
		return v, nil
	})
}

// GetStudyMix returns the open Collection's Mix.
func (a *App) GetStudyMix() (learning.Mix, error) {
	var m learning.Mix
	err := a.withCollection(func(h collection.Handles) error {
		var err error
		m, err = h.Learning().LoadMix()
		return err
	})
	return m, err
}

// SetStudyMix stores the open Collection's Mix. It applies from the next Turn.
func (a *App) SetStudyMix(m learning.Mix) error {
	return a.withCollection(func(h collection.Handles) error { return h.Learning().SaveMix(m) })
}
