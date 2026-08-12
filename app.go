package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"texere/internal/content"
	"texere/internal/explorer"
	"texere/internal/learning"
	"texere/internal/media"
	"texere/internal/search"
)

// AppConfig is the persisted user configuration.
// Stored at: os.UserConfigDir()/texere/config.json
type AppConfig struct {
	CollectionDir string    `json:"collectionDir"` // abs path; "" = no collection saved yet
	RecentArrows  []string  `json:"recentArrows"`  // LRU list of arrow codes; max 10 entries
	LLM           LLMConfig `json:"llm"`
	STT           LLMConfig `json:"stt"`
	// QueryLLMProfileID selects which LLM profile translates a natural-language
	// question into a graph query. Empty means "the active profile". It lives on
	// AppConfig rather than LLMConfig because LLMConfig is reused verbatim for
	// STT, where this would be meaningless.
	QueryLLMProfileID string `json:"queryLlmProfileId"`
	// Theme is the selected appearance: "blue", "dark" or "light". Empty means
	// blue — that is the first-run and the upgraded-config path, and the
	// frontend's normalizeTheme resolves it the same way.
	Theme string `json:"theme"`
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
	dirMu         sync.RWMutex             // guards collectionDir for the /media asset handler (HTTP goroutine)
	savingMu      sync.Mutex               // guards savingPaths
	savingPaths   map[string]int           // ref-counted set of paths the app is writing; suppresses false external-mod events
	openFilePath  string                   // path of the file currently open in the Source tab
	initialSums   []content.ChapterSummary // summaries loaded at startup from saved config
	startupError  string                   // non-empty when auto-load failed (dir missing)
	watcher       *content.CollectionWatcher
	quitConfirmed bool                          // set by ConfirmQuit so beforeClose lets the second Quit() through
	searchSvc     *search.SearchService         // graph search; nil until a collection is loaded
	explorerSvc   *explorer.ExplorerService     // 2.5D isometric explorer; lifetime mirrors searchSvc
	learnStore    *learning.Store               // spaced-repetition store; nil until a collection is loaded
	llmStreams    map[string]context.CancelFunc // active LLM turns, keyed by streamId
	llmMu         sync.Mutex                    // guards llmStreams
	shellRuns     map[string]context.CancelFunc // active shell runs, keyed by runId
	shellMu       sync.Mutex                    // guards shellRuns
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved so that runtime
// methods (e.g. wails.runtime.EventsEmit) can be called from other methods.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.llmStreams = map[string]context.CancelFunc{}
	a.shellRuns = map[string]context.CancelFunc{}

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
	a.dirMu.Lock()
	a.collectionDir = cfg.CollectionDir
	a.dirMu.Unlock()
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
	a.shellMu.Lock()
	for _, cancel := range a.shellRuns {
		cancel()
	}
	a.shellMu.Unlock()
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
	a.dirMu.Lock()
	a.collectionDir = dir
	a.dirMu.Unlock()

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

// mediaDir returns <collectionDir>/media, or "" when no collection is open.
// Unexported so it is not surfaced as a Wails binding; read by the asset handler
// (main.go) on the HTTP goroutine, hence the mutex.
func (a *App) mediaDir() string {
	a.dirMu.RLock()
	defer a.dirMu.RUnlock()
	if a.collectionDir == "" {
		return ""
	}
	return filepath.Join(a.collectionDir, "media")
}

// collectionRoot is the collection directory, or "" when none is open. Read
// under dirMu because the HTTP asset handler calls it from its own goroutine,
// exactly as mediaDir does.
func (a *App) collectionRoot() string {
	a.dirMu.RLock()
	defer a.dirMu.RUnlock()
	return a.collectionDir
}

// ImportImage opens the OS file dialog, copies the chosen image into the
// collection's media/ directory, and returns {asset, mime}. Returns (nil, nil)
// when the user cancels, and an error when no collection is open or the file is
// not a supported image.
func (a *App) ImportImage() (map[string]any, error) {
	a.dirMu.RLock()
	dir := a.collectionDir
	a.dirMu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("no collection open")
	}
	sel, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Add image",
		Filters: []runtime.FileFilter{
			{DisplayName: "Images (*.jpg, *.png, *.gif, *.webp)", Pattern: "*.jpg;*.jpeg;*.png;*.gif;*.webp"},
		},
	})
	if err != nil {
		return nil, err
	}
	if sel == "" {
		return nil, nil // cancelled
	}
	asset, err := media.Import(dir, sel)
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": asset.Name, "mime": asset.Mime}, nil
}

// ImportAudio opens the OS file dialog (audio filter), copies the chosen file
// into media/, and returns {asset, mime}. (nil, nil) on cancel; error if no
// collection is open or the type is unsupported.
func (a *App) ImportAudio() (map[string]any, error) {
	a.dirMu.RLock()
	dir := a.collectionDir
	a.dirMu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("no collection open")
	}
	sel, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Add audio",
		Filters: []runtime.FileFilter{
			{DisplayName: "Audio (*.webm, *.ogg, *.wav, *.mp3, *.m4a, *.aac, *.flac)", Pattern: "*.webm;*.ogg;*.oga;*.wav;*.mp3;*.m4a;*.mp4;*.aac;*.flac"},
		},
	})
	if err != nil {
		return nil, err
	}
	if sel == "" {
		return nil, nil
	}
	asset, err := media.ImportAudio(dir, sel)
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": asset.Name, "mime": asset.Mime}, nil
}

// SaveRecording decodes base64 audio bytes from an in-app recording and stores
// them in media/, returning {asset, mime}.
func (a *App) SaveRecording(dataBase64, mime string) (map[string]any, error) {
	a.dirMu.RLock()
	dir := a.collectionDir
	a.dirMu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("no collection open")
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return nil, fmt.Errorf("decode recording: %w", err)
	}
	asset, err := media.SaveAudio(dir, data, mime)
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": asset.Name, "mime": asset.Mime}, nil
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

// GraphLayout returns an isometric layout for a query WITHOUT starting/replacing
// the global explorer session — used by the notebook graph cell's graph view.
func (a *App) GraphLayout(query string, mode string) (explorer.LayoutResult, error) {
	if a.explorerSvc == nil {
		return explorer.LayoutResult{
			Nodes:       []explorer.NodeView{},
			Edges:       []explorer.EdgeView{},
			Territories: []explorer.TerritoryView{},
			Diagnostics: explorer.Diagnostics{
				GraphError: "Build your graph first (Ctrl+G) to enable graph search.",
			},
		}, nil
	}
	return a.explorerSvc.Layout(query, mode)
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
