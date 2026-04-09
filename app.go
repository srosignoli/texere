package main

import (
	"context"
	"log"
	"path/filepath"

	"texere/internal/content"
)

// App is the main application struct. It holds the Wails runtime context and
// serves as the binding surface between the Go backend and the Svelte frontend.
type App struct {
	ctx      context.Context
	registry *content.SSTRegistry
	watcher  *content.CollectionWatcher
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved so that runtime
// methods (e.g. wails.runtime.EventsEmit) can be called from other methods.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Load the SSTconfig arrow + annotation registry from the directory
	// adjacent to the binary. In development this is the repo root.
	sstconfigDir := filepath.Join(".", "SSTconfig")
	reg, err := content.LoadRegistry(sstconfigDir)
	if err != nil {
		log.Printf("content: LoadRegistry degraded mode: %v", err)
	}
	if len(reg.LoadErrors) > 0 {
		for _, e := range reg.LoadErrors {
			log.Printf("content: registry load warning: %v", e)
		}
	}
	a.registry = reg
}

// domReady is called after the front-end dom has been loaded.
func (a *App) domReady(_ context.Context) {}

// beforeClose is called when the application is about to quit, either by
// clicking the window close button or calling runtime.Quit. Returning true
// will cause the application to continue, false will continue shutdown as normal.
func (a *App) beforeClose(_ context.Context) bool {
	return false
}

// shutdown is called at application termination.
func (a *App) shutdown(_ context.Context) {
	if a.watcher != nil {
		if err := a.watcher.Close(); err != nil {
			log.Printf("content: watcher close: %v", err)
		}
	}
}

// OpenCollection scans a directory of .n4l files and, if successful, starts the
// filesystem watcher. Returns the chapter summaries for Column 1 population.
func (a *App) OpenCollection(dir string) ([]content.ChapterSummary, error) {
	summaries, err := content.ScanCollection(dir)
	if err != nil {
		return nil, err
	}

	// Start watcher (replace any existing one)
	if a.watcher != nil {
		a.watcher.Close()
	}
	watcher, err := content.WatchCollection(dir)
	if err != nil {
		log.Printf("content: WatchCollection: %v", err)
		// Degraded: no live updates, but collection loaded
	} else {
		a.watcher = watcher
		go a.consumeWatchEvents()
	}

	return summaries, nil
}

// consumeWatchEvents drains the watcher event channel. In a full implementation
// this would emit Wails events to the frontend for Column 1 updates.
func (a *App) consumeWatchEvents() {
	if a.watcher == nil {
		return
	}
	for {
		select {
		case _, ok := <-a.watcher.Events:
			if !ok {
				return
			}
			// TODO: emit runtime.EventsEmit(a.ctx, "collection:change", ev)
		case err, ok := <-a.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("content: watcher error: %v", err)
		}
	}
}

// ParseFileForUI is a Wails-bound method that parses a .n4l file and returns
// the full structured content for display in the editor view.
func (a *App) ParseFileForUI(path string) (*content.ParsedFile, []content.ParseError) {
	return content.ParseFile(path, a.registry)
}
