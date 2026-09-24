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
		// Best-effort: a row whose stored path can no longer be resolved under
		// the open collection must not hide the rest of the queue. FilePath is
		// not read for file I/O anywhere the frontend uses this view today, but
		// it is exposed at the Wails boundary alongside ExtractTarget.FilePath
		// (which is), so it is kept in the same absolute form for consistency.
		filePath := e.FilePath
		if abs, rerr := a.learnStore.Resolve(e.FilePath); rerr == nil {
			filePath = abs
		}
		out = append(out, ExtractView{ID: e.ID, Text: e.Text, FromAnchor: e.FromAnchor, FilePath: filePath})
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
	// e.FilePath is collection-relative as stored; the frontend hands this
	// FilePath straight to GetFileContent/SaveFile, which need an absolute
	// path — this is the exact site the reported incident came through.
	filePath, err := a.learnStore.Resolve(e.FilePath)
	if err != nil {
		return nil, fmt.Errorf("GetExtractTarget: %w", err)
	}
	line, _, err := a.nodeByID(filePath, id)
	if err != nil {
		return nil, err
	}
	return &ExtractTarget{ID: e.ID, FilePath: filePath, Line: line, Text: e.Text, FromAnchor: e.FromAnchor}, nil
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
	// e.FilePath is collection-relative as stored; resolve once and reuse the
	// absolute form for every file operation and frontend-facing event below.
	filePath, err := a.learnStore.Resolve(e.FilePath)
	if err != nil {
		return fmt.Errorf("PromoteExtract: %w", err)
	}
	line, nodeText, err := a.nodeByID(filePath, id)
	if err != nil {
		return err
	}
	// A cloze card needs at least one {{deletion}}; promoting without one would
	// retire the extract and generate nothing recallable. Refuse so the author can
	// mark a deletion in the editor first.
	if kind == "cloze" && !content.HasClozeTargets(nodeText) {
		return errors.New("mark a deletion with {{double braces}} before making a cloze")
	}
	wErr := a.selfWrite(filePath, func() error {
		if kind == "card" {
			// Seed a basic card stub "<passage> >> " for the author to complete, and
			// carry the (from) anchor onto a ditto line beneath it. The anchor also
			// lives on the retired extracts row, but the file is this app's durable
			// store — leaving provenance only in the database meant a rebuilt
			// learning.db lost where the passage came from, permanently.
			return content.SeedBasicCardNode(filePath, line, e.Text, e.FromAnchor)
		}
		return content.InsertMarkerAboveNode(filePath, line, "# @cloze")
	})
	if wErr != nil {
		return wErr
	}
	// Re-sync this one file: the node now has StudyMeta → item created, extract retired.
	if _, err := learning.Sync(a.learnStore, []string{filePath}, a.registry); err != nil {
		return err
	}
	// Promotion does not go through rescheduleExtract, so it needs its own log
	// line — without it the statistics would show every dismissal and no
	// promotions, which is precisely backwards.
	// Best-effort: a statistics gap must never fail the user's action.
	_ = a.learnStore.LogActivity("process", id, "promote", 0, time.Now())
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "file:ids-injected", map[string]string{"path": filePath})
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
	if err := a.learnStore.ApplyExtractSchedule(id, learning.ScheduleExtract(e, action, time.Now())); err != nil {
		return err
	}
	// Logged here rather than in the store: ApplyExtractSchedule only sees the
	// resulting row, and `active=0` cannot tell "promoted to a card" from
	// "dismissed". The action string is the only place that distinction exists.
	// Best-effort — a statistics gap must never fail the user's action.
	_ = a.learnStore.LogActivity("process", id, action, 0, time.Now())
	return nil
}
