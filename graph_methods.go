package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"texere/internal/graph"
	"texere/internal/search"
)

// BuildGraphResult is returned by the BuildGraph Wails-bound method.
type BuildGraphResult struct {
	NodesIngested  int    `json:"nodesIngested"`
	LinksIngested  int    `json:"linksIngested"`
	FilesProcessed int    `json:"filesProcessed"`
	DurationMs     int64  `json:"durationMs"`
	Error          string `json:"error,omitempty"`
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

	runtime.EventsEmit(a.ctx, "graph:build-started", nil)

	br, err := graph.BuildFromCollection(a.collectionDir, a.registry)
	if err != nil {
		result.Error = err.Error()
	} else {
		result.NodesIngested = br.NodesIngested
		result.LinksIngested = br.LinksIngested
		result.FilesProcessed = br.FilesProcessed
		result.DurationMs = br.DurationMs
	}

	// Reopen so searches work immediately after build.
	a.searchSvc = search.NewSearchService(a.collectionDir, a.registry)

	runtime.EventsEmit(a.ctx, "graph:build-complete", result)
	return result
}
