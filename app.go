package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"texere/internal/content"
	"texere/internal/explorer"
	"texere/internal/learning"
	"texere/internal/search"
)

// AppConfig is the persisted user configuration.
// Stored at: os.UserConfigDir()/texere/config.json
type AppConfig struct {
	CollectionDir string    `json:"collectionDir"` // abs path; "" = no collection saved yet
	RecentArrows  []string  `json:"recentArrows"`  // LRU list of arrow codes; max 10 entries
	LLM           LLMConfig `json:"llm"`
}

// SaveFileResult is returned by the SaveFile Wails-bound method.
type SaveFileResult struct {
	Injections   []content.IDInjection `json:"injections"`
	FinalContent string                `json:"finalContent"` // on-disk content after @id injection
}

// App is the main application struct. It holds the Wails runtime context and
// serves as the binding surface between the Go backend and the Svelte frontend.
type App struct {
	ctx           context.Context
	registry      *content.SSTRegistry
	collectionDir string                   // abs path of the open collection; "" when none
	savingMu      sync.Mutex               // guards savingPaths
	savingPaths   map[string]int           // ref-counted set of paths the app is writing; suppresses false external-mod events
	openFilePath  string                   // path of the file currently open in the Source tab
	initialSums   []content.ChapterSummary // summaries loaded at startup from saved config
	startupError  string                   // non-empty when auto-load failed (dir missing)
	watcher       *content.CollectionWatcher
	quitConfirmed bool                      // set by ConfirmQuit so beforeClose lets the second Quit() through
	searchSvc     *search.SearchService     // graph search; nil until a collection is loaded
	explorerSvc   *explorer.ExplorerService // 2.5D isometric explorer; lifetime mirrors searchSvc
	learnStore    *learning.Store           // spaced-repetition store; nil until a collection is loaded
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

	// Auto-load the last-used collection from persisted config.
	cfg, _ := loadConfig()
	if cfg.CollectionDir == "" {
		return // first launch — nothing to load
	}
	info, err := os.Stat(cfg.CollectionDir)
	if err != nil || !info.IsDir() {
		a.startupError = "Last collection directory not found: " + cfg.CollectionDir
		return
	}
	summaries, err := content.ScanCollection(cfg.CollectionDir)
	if err != nil {
		a.startupError = "Could not load collection: " + err.Error()
		return
	}
	a.collectionDir = cfg.CollectionDir
	a.initialSums = summaries
	a.searchSvc = search.NewSearchService(cfg.CollectionDir, a.registry)
	a.explorerSvc = explorer.NewExplorerService(a.searchSvc, a.registry)

	if a.learnStore != nil {
		a.learnStore.Close()
		a.learnStore = nil
	}
	if a.collectionDir != "" {
		if ls, err := learning.Open(a.collectionDir); err != nil {
			log.Printf("learning: open store: %v", err)
		} else {
			a.learnStore = ls
		}
	}

	watcher, err := content.WatchCollection(cfg.CollectionDir)
	if err != nil {
		log.Printf("content: WatchCollection at startup: %v", err)
	} else {
		a.watcher = watcher
		go a.consumeWatchEvents()
	}
}

// domReady is called after the front-end dom has been loaded.
func (a *App) domReady(_ context.Context) {}

// beforeClose is called when the application is about to quit, either by
// clicking the window close button or calling runtime.Quit. Returning true
// will cause the application to continue (keeps window open), false will
// continue shutdown as normal.
//
// On the first call (X button / Ctrl+C) we emit "app:close-requested" so the
// frontend can prompt for unsaved changes, then return true to keep the window
// open. The frontend calls ConfirmQuit() which sets quitConfirmed and calls
// runtime.Quit() again; on that second call we return false to allow quit.
func (a *App) beforeClose(ctx context.Context) bool {
	if a.quitConfirmed {
		return false // allow quit
	}
	runtime.EventsEmit(ctx, "app:close-requested")
	return true // keep window open; frontend decides
}

// ConfirmQuit is called by the frontend after the user has confirmed they want
// to quit (e.g. dismissed the unsaved-changes dialog). It sets the quit flag
// and triggers the actual application shutdown.
func (a *App) ConfirmQuit() {
	a.quitConfirmed = true
	runtime.Quit(a.ctx)
}

// shutdown is called at application termination.
func (a *App) shutdown(_ context.Context) {
	if a.watcher != nil {
		if err := a.watcher.Close(); err != nil {
			log.Printf("content: watcher close: %v", err)
		}
	}
	if a.searchSvc != nil {
		a.searchSvc.Close()
	}
	if a.learnStore != nil {
		a.learnStore.Close()
	}
}

// OpenCollection scans a directory of .n4l files, starts the filesystem watcher,
// persists the directory to config, and returns the chapter summaries for Column 1.
func (a *App) OpenCollection(dir string) ([]content.ChapterSummary, error) {
	summaries, err := content.ScanCollection(dir)
	if err != nil {
		return nil, err
	}

	// Persist the chosen directory so startup() can auto-load it next launch.
	cfg, _ := loadConfig()
	cfg.CollectionDir = dir
	if err := saveConfig(cfg); err != nil {
		log.Printf("content: saveConfig: %v", err)
	}

	// No file is open after switching collections.
	a.openFilePath = ""
	a.collectionDir = dir

	// Re-create the search service for the new collection directory.
	if a.searchSvc != nil {
		a.searchSvc.Close()
	}
	a.searchSvc = search.NewSearchService(dir, a.registry)
	if a.explorerSvc == nil {
		a.explorerSvc = explorer.NewExplorerService(a.searchSvc, a.registry)
	} else {
		a.explorerSvc.SetSearchService(a.searchSvc)
	}

	if a.learnStore != nil {
		a.learnStore.Close()
		a.learnStore = nil
	}
	if dir != "" {
		if ls, err := learning.Open(dir); err != nil {
			log.Printf("learning: open store: %v", err)
		} else {
			a.learnStore = ls
		}
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

// ParseFileForUI is a Wails-bound method that parses a .n4l file and returns
// the full structured content for display in the editor view.
// Side-effect: sets a.openFilePath so consumeWatchEvents can emit
// file:modified-externally for the correct file.
func (a *App) ParseFileForUI(path string) (*content.ParsedFile, []content.ParseError) {
	a.openFilePath = path
	return content.ParseFile(path, a.registry)
}

// =============================================================================
// 2.5D Isometric Graph Explorer (feature 006) — Wails IPC surface.
// See specs/006-isometric-graph-explorer/contracts/layout-ipc.md for the
// full contract: error model, lifecycle, and JSON shapes.
// =============================================================================

// ExplorerOpen runs a graph query, materializes the result subgraph, and
// returns the initial layout for the default anchor. User-visible failure
// modes (graph not built, no results) ride in LayoutResult.Diagnostics
// rather than the error return.
func (a *App) ExplorerOpen(query string, mode string) (explorer.LayoutResult, error) {
	if a.explorerSvc == nil {
		// No collection loaded yet — surface the same hint the search
		// overlay's Tier 2 uses so the frontend can render a useful
		// empty state.
		return explorer.LayoutResult{
			Nodes:       []explorer.NodeView{},
			Edges:       []explorer.EdgeView{},
			Territories: []explorer.TerritoryView{},
			Diagnostics: explorer.Diagnostics{
				GraphError: "Build your graph first (Ctrl+G) to enable graph search.",
			},
		}, nil
	}
	return a.explorerSvc.Open(query, mode)
}

// ExplorerReanchor runs a new layout pass against the cached ResultSet
// with the supplied anchor. `mode` flips between layout encodings
// (stacked / extended) without re-running the search. Returns
// ErrStaleResultID or ErrAnchorNotInResult for programmer errors;
// never for user-visible states.
func (a *App) ExplorerReanchor(resultID string, anchorID string, mode string) (explorer.LayoutResult, error) {
	if a.explorerSvc == nil {
		return explorer.LayoutResult{}, explorer.ErrStaleResultID
	}
	return a.explorerSvc.Reanchor(resultID, anchorID, mode)
}

// ExplorerClose evicts the cached ResultSet. Idempotent.
func (a *App) ExplorerClose() error {
	if a.explorerSvc == nil {
		return nil
	}
	return a.explorerSvc.Close()
}
