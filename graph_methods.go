package main

import (
	"texere/internal/content"
)

// BuildGraphResult is returned by the BuildGraph Wails-bound method.
type BuildGraphResult struct {
	NodesIngested  int                  `json:"nodesIngested"`
	LinksIngested  int                  `json:"linksIngested"`
	FilesProcessed int                  `json:"filesProcessed"`
	DurationMs     int64                `json:"durationMs"`
	Error          string               `json:"error,omitempty"`
	Errors         []string             `json:"errors,omitempty"`        // bleve / IO errors collected during build
	UnknownArrows  map[string]int       `json:"unknownArrows,omitempty"` // arrow code → occurrences (frontend log panel)
	ParseErrors    []content.ParseError `json:"parseErrors,omitempty"`   // hoisted per-file parse errors
}

// BuildGraph ingests the open collection into the SSTorytime-KV BadgerDB store,
// rebuilding sst_data from scratch. Emits graph:build-started and
// graph:build-complete events so the frontend can show progress.
func (a *App) BuildGraph() *BuildGraphResult {
	result := &BuildGraphResult{}
	s, err := a.session()
	if err != nil {
		result.Error = "No collection open"
		return result
	}
	a.emit("graph:build-started", nil)
	br, err := s.RebuildGraph()
	if err != nil {
		result.Error = err.Error()
	} else {
		result.NodesIngested = br.NodesIngested
		result.LinksIngested = br.LinksIngested
		result.FilesProcessed = br.FilesProcessed
		result.DurationMs = br.DurationMs
		result.Errors = br.Errors
		result.UnknownArrows = br.UnknownArrows
		result.ParseErrors = br.ParseErrors
	}
	a.emit("graph:build-complete", result)
	return result
}
