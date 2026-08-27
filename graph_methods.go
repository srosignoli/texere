package main

import (
	"log"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/content"
	"texere/internal/explorer"
	"texere/internal/graph"
	"texere/internal/learning"
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

	if a.learnStore != nil {
		// Suppress spurious file:modified-externally for the open file during
		// sync writeback (mirrors how SaveFile brackets self-writes).
		a.beginSelfWrite(a.openFilePath)
		syncRes, err := learning.Sync(a.learnStore, a.collectionN4LFiles(), a.registry)
		a.endSelfWrite(a.openFilePath)
		if err != nil {
			log.Printf("learning: sync after build: %v", err)
		}
		// If the open file had # @id markers injected, notify the frontend so it
		// can silently reload the buffer and pick up the new IDs.
		if a.ctx != nil && a.openFilePath != "" {
			for _, injPath := range syncRes.InjectedPaths {
				if injPath == a.openFilePath {
					runtime.EventsEmit(a.ctx, "file:ids-injected", map[string]string{"path": a.openFilePath})
					break
				}
			}
		}
	}

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "graph:build-complete", result)
	}
	return result
}
