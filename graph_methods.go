package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"texere/internal/content"
	"texere/internal/explorer"
	"texere/internal/graph"
	"texere/internal/search"
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
	if a.collectionDir == "" {
		result.Error = "No collection open"
		return result
	}

	// Release BadgerDB lock before rebuilding.
	if a.searchSvc != nil {
		a.searchSvc.Close()
		a.searchSvc = nil
	}

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "graph:build-started", nil)
	}

	br, err := graph.BuildFromCollection(a.collectionDir, a.registry)
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

	// Reopen so searches work immediately after build.
	a.searchSvc = search.NewSearchService(a.collectionDir, a.registry)
	// Re-point the explorer at the new search service. Without this the
	// explorer keeps a reference to the now-closed pre-build service, so the
	// graph stays empty until the app is restarted (OpenCollection already
	// does this via SetSearchService; BuildGraph must too).
	if a.explorerSvc != nil {
		a.explorerSvc.SetSearchService(a.searchSvc)
	} else {
		a.explorerSvc = explorer.NewExplorerService(a.searchSvc, a.registry)
	}

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "graph:build-complete", result)
	}
	return result
}
