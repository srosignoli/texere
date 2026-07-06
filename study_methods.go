package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/content"
	"texere/internal/learning"
)

// ExtractView is a due extract for the Process queue.
type ExtractView struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	FromAnchor string `json:"fromAnchor"`
	FilePath   string `json:"filePath"`
}

// ExtractTarget locates an extract's node line for the Process editor.
type ExtractTarget struct {
	ID         string `json:"id"`
	FilePath   string `json:"filePath"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
	FromAnchor string `json:"fromAnchor"`
}

func (a *App) GetStudyCounts() (*learning.StudyStats3, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	c, err := a.learnStore.StudyCounts(time.Now())
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (a *App) GetExtractQueue(limit int) ([]ExtractView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	if limit <= 0 {
		limit = 50
	}
	exs, err := a.learnStore.DueExtracts(time.Now(), limit)
	if err != nil {
		return nil, err
	}
	out := make([]ExtractView, 0, len(exs))
	for _, e := range exs {
		out = append(out, ExtractView{ID: e.ID, Text: e.Text, FromAnchor: e.FromAnchor, FilePath: e.FilePath})
	}
	return out, nil
}

// nodeByID parses file and returns the 1-indexed line and raw text of the node
// whose ID==id.
func (a *App) nodeByID(filePath, id string) (int, string, error) {
	pf, _ := content.ParseFile(filePath, a.registry)
	if pf == nil {
		return 0, "", fmt.Errorf("parse %s failed", filePath)
	}
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.ID == id {
				return n.Line, n.Text, nil
			}
		}
	}
	return 0, "", fmt.Errorf("node %q not found in %s", id, filePath)
}

func (a *App) GetExtractTarget(id string) (*ExtractTarget, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	e, ok, err := a.learnStore.GetExtract(id)
	if err != nil || !ok {
		return nil, fmt.Errorf("GetExtractTarget: %q not found", id)
	}
	line, _, err := a.nodeByID(e.FilePath, id)
	if err != nil {
		return nil, err
	}
	return &ExtractTarget{ID: e.ID, FilePath: e.FilePath, Line: line, Text: e.Text, FromAnchor: e.FromAnchor}, nil
}

func (a *App) PromoteExtract(id, kind string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	if kind != "card" && kind != "cloze" {
		return fmt.Errorf("PromoteExtract: bad kind %q", kind)
	}
	e, ok, err := a.learnStore.GetExtract(id)
	if err != nil || !ok {
		return fmt.Errorf("PromoteExtract: %q not found", id)
	}
	line, nodeText, err := a.nodeByID(e.FilePath, id)
	if err != nil {
		return err
	}
	// A cloze card needs at least one {{deletion}}; promoting without one would
	// retire the extract and generate nothing recallable. Refuse so the author can
	// mark a deletion in the editor first.
	if kind == "cloze" && !content.HasClozeTargets(nodeText) {
		return errors.New("mark a deletion with {{double braces}} before making a cloze")
	}
	a.beginSelfWrite(e.FilePath)
	var wErr error
	if kind == "card" {
		// Seed a basic card stub "<passage> >> " for the author to complete; the
		// (from) anchor is dropped (provenance stays in the retired extract row).
		wErr = content.SeedBasicCardNode(e.FilePath, line, e.Text)
	} else {
		wErr = content.InsertMarkerAboveNode(e.FilePath, line, "# @cloze")
	}
	a.endSelfWrite(e.FilePath)
	if wErr != nil {
		return wErr
	}
	// Re-sync this one file: the node now has StudyMeta → item created, extract retired.
	if _, err := learning.Sync(a.learnStore, []string{e.FilePath}, a.registry); err != nil {
		return err
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "file:ids-injected", map[string]string{"path": e.FilePath})
	}
	return nil
}

func (a *App) DeferExtract(id string) error   { return a.rescheduleExtract(id, "later") }
func (a *App) DismissExtract(id string) error { return a.rescheduleExtract(id, "dismiss") }

func (a *App) rescheduleExtract(id, action string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	e, ok, err := a.learnStore.GetExtract(id)
	if err != nil || !ok {
		return fmt.Errorf("rescheduleExtract: %q not found", id)
	}
	return a.learnStore.ApplyExtractSchedule(id, learning.ScheduleExtract(e, action, time.Now()))
}
