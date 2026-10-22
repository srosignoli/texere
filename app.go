package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/collection"
	"texere/internal/content"
	"texere/internal/explorer"
	"texere/internal/media"
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
	curMu         sync.Mutex                    // guards cur
	cur           *collection.Session           // the open Collection; nil when none
	initialSums   []content.ChapterSummary      // summaries loaded at startup from saved config
	startupError  string                        // non-empty when auto-load failed (dir missing)
	quitConfirmed bool                          // set by ConfirmQuit so beforeClose lets the second Quit() through
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
	sums, err := a.switchTo(cfg.CollectionDir)
	if err != nil {
		a.startupError = "Could not load collection: " + err.Error()
		return
	}
	a.initialSums = sums
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
	a.closeSession()
}

// OpenCollection opens dir as the current Collection and returns its chapter
// summaries for Column 1. If dir cannot be opened, the current one stays open.
func (a *App) OpenCollection(dir string) ([]content.ChapterSummary, error) {
	sums, err := a.switchTo(dir)
	if err != nil {
		return nil, err
	}
	// Persist the chosen directory so startup() can auto-load it next launch.
	cfg, _ := loadConfig()
	cfg.CollectionDir = dir
	if err := saveConfig(cfg); err != nil {
		log.Printf("content: saveConfig: %v", err)
	}
	return sums, nil
}

// mediaDir returns <collectionDir>/media, or "" when no collection is open.
// Unexported so it is not surfaced as a Wails binding; read by the asset
// handler (main.go) on the HTTP goroutine. It is safe from that goroutine
// because currentDir takes curMu and a Session's directory never changes.
func (a *App) mediaDir() string {
	dir, err := a.currentDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "media")
}

// collectionRoot is the collection directory, or "" when none is open. It is
// safe from the HTTP asset handler's goroutine because currentDir takes
// curMu and a Session's directory never changes.
func (a *App) collectionRoot() string {
	dir, _ := a.currentDir()
	return dir
}

// LintCollection parses every .n4l file in the open collection and returns
// the aggregated defects and vocabulary for the Problems tab. Returns an
// error when no collection is open.
func (a *App) LintCollection() (content.LintResult, error) {
	root, err := a.currentDir()
	if err != nil {
		return content.LintResult{}, err
	}
	return content.LintCollection(root, a.registry)
}

// ImportImage opens the OS file dialog, copies the chosen image into the
// collection's media/ directory, and returns its AssetRef. Returns (nil, nil)
// when the user cancels, and an error when no collection is open or the file is
// not a supported image.
func (a *App) ImportImage() (*AssetRef, error) {
	_, err := a.currentDir()
	if err != nil {
		return nil, err
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
	return a.importImageFrom(sel)
}

// importImageFrom is the ingest half of ImportImage: everything after a path
// has been chosen. Unexported, so Wails generates no JS binding for it.
func (a *App) importImageFrom(sel string) (*AssetRef, error) {
	dir, err := a.currentDir()
	if err != nil {
		return nil, err
	}
	if sel == "" {
		return nil, fmt.Errorf("importImageFrom: empty path")
	}
	asset, err := media.Import(dir, sel)
	if err != nil {
		return nil, err
	}
	return toAssetRef(asset), nil
}

// ImportAudio opens the OS file dialog (audio filter), copies the chosen file
// into media/, and returns its AssetRef. (nil, nil) on cancel; error if no
// collection is open or the type is unsupported.
func (a *App) ImportAudio() (*AssetRef, error) {
	_, err := a.currentDir()
	if err != nil {
		return nil, err
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
	return a.importAudioFrom(sel)
}

// importAudioFrom is the ingest half of ImportAudio: everything after a path
// has been chosen. Unexported, so Wails generates no JS binding for it.
func (a *App) importAudioFrom(sel string) (*AssetRef, error) {
	dir, err := a.currentDir()
	if err != nil {
		return nil, err
	}
	if sel == "" {
		return nil, fmt.Errorf("importAudioFrom: empty path")
	}
	asset, err := media.ImportAudio(dir, sel)
	if err != nil {
		return nil, err
	}
	return toAssetRef(asset), nil
}

// SaveRecording decodes base64 audio bytes from an in-app recording and stores
// them in media/, returning its AssetRef.
func (a *App) SaveRecording(dataBase64, mime string) (*AssetRef, error) {
	dir, err := a.currentDir()
	if err != nil {
		return nil, err
	}
	data, decErr := base64.StdEncoding.DecodeString(dataBase64)
	if decErr != nil {
		return nil, fmt.Errorf("decode recording: %w", decErr)
	}
	asset, err := media.SaveAudio(dir, data, mime)
	if err != nil {
		return nil, err
	}
	return toAssetRef(asset), nil
}

// ParseFileForUI is a Wails-bound method that parses a .n4l file and returns
// the full structured content for display in the editor view.
// Side-effect: records path as the Session's open file, so its watcher can
// emit file:modified-externally for the correct file.
func (a *App) ParseFileForUI(path string) (*content.ParsedFile, []content.ParseError) {
	if s, err := a.session(); err == nil {
		s.SetOpenFile(path)
	}
	return content.ParseFile(path, a.registry)
}

// =============================================================================
// 2.5D Isometric Graph Explorer (feature 006) — Wails IPC surface.
// See docs/specs/006-isometric-graph-explorer/contracts/layout-ipc.md for the
// full contract: error model, lifecycle, and JSON shapes.
// =============================================================================

// noGraphLayout is the empty layout shown when no collection is open,
// carrying the same hint as the search overlay's Tier 2.
func noGraphLayout() explorer.LayoutResult {
	return explorer.LayoutResult{
		Nodes:       []explorer.NodeView{},
		Edges:       []explorer.EdgeView{},
		Territories: []explorer.TerritoryView{},
		Diagnostics: explorer.Diagnostics{
			GraphError: "Build your graph first (Ctrl+G) to enable graph search.",
		},
	}
}

// ExplorerOpen runs a graph query, materializes the result subgraph, and
// returns the initial layout for the default anchor. User-visible failure
// modes (graph not built, no results) ride in LayoutResult.Diagnostics
// rather than the error return.
func (a *App) ExplorerOpen(query string, mode string) (explorer.LayoutResult, error) {
	lr, err := inCollection(a, func(h collection.Handles) (explorer.LayoutResult, error) {
		return h.Explorer().Open(query, mode)
	})
	if errors.Is(err, collection.ErrNoCollection) {
		return noGraphLayout(), nil
	}
	return lr, err
}

// GraphLayout returns an isometric layout for a query WITHOUT starting/replacing
// the global explorer session — used by the notebook graph cell's graph view.
func (a *App) GraphLayout(query string, mode string) (explorer.LayoutResult, error) {
	lr, err := inCollection(a, func(h collection.Handles) (explorer.LayoutResult, error) {
		return h.Explorer().Layout(query, mode)
	})
	if errors.Is(err, collection.ErrNoCollection) {
		return noGraphLayout(), nil
	}
	return lr, err
}

// ExplorerReanchor runs a new layout pass against the cached ResultSet
// with the supplied anchor. `mode` flips between layout encodings
// (stacked / extended) without re-running the search. Returns
// ErrStaleResultID or ErrAnchorNotInResult for programmer errors;
// never for user-visible states.
func (a *App) ExplorerReanchor(resultID string, anchorID string, mode string) (explorer.LayoutResult, error) {
	lr, err := inCollection(a, func(h collection.Handles) (explorer.LayoutResult, error) {
		return h.Explorer().Reanchor(resultID, anchorID, mode)
	})
	if errors.Is(err, collection.ErrNoCollection) {
		return explorer.LayoutResult{}, explorer.ErrStaleResultID
	}
	return lr, err
}

// ExplorerClose evicts the cached ResultSet. Idempotent.
func (a *App) ExplorerClose() error {
	err := a.withCollection(func(h collection.Handles) error { return h.Explorer().Close() })
	if errors.Is(err, collection.ErrNoCollection) {
		return nil
	}
	return err
}
