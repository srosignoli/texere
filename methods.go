package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"texere/internal/content"
)

// ---------------------------------------------------------------------------
// Config helpers (T005)
// ---------------------------------------------------------------------------

func configPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("methods: UserConfigDir: %w", err)
	}
	return filepath.Join(base, "texere", "config.json"), nil
}

func loadConfig() (*AppConfig, error) {
	p, err := configPath()
	if err != nil {
		return &AppConfig{}, nil // first-run default
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &AppConfig{}, nil // first-run default
	}
	if err != nil {
		return &AppConfig{}, nil // read error → treat as first run
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &AppConfig{}, nil // corrupt config → treat as first run
	}
	return &cfg, nil
}

func saveConfig(cfg *AppConfig) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("methods: saveConfig: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("methods: saveConfig: marshal: %w", err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("methods: saveConfig: write: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// PickCollectionDir — wraps runtime.OpenDirectoryDialog for the frontend
// ---------------------------------------------------------------------------

// PickCollectionDir opens a native directory-picker dialog and returns the
// selected path. Returns "" (empty string) if the user cancels.
func (a *App) PickCollectionDir() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select collection folder",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// ---------------------------------------------------------------------------
// GetFileContent (T006)
// ---------------------------------------------------------------------------

// GetFileContent returns the raw UTF-8 text of path.
// Called when the Source tab mounts or reloads after an external modification.
func (a *App) GetFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("GetFileContent: %w", err)
	}
	return string(data), nil
}

// ---------------------------------------------------------------------------
// GetArrowRegistry (T007)
// ---------------------------------------------------------------------------

// GetArrowRegistry returns the loaded arrow definitions sorted by MetaType then Code.
// Returns an empty slice in degraded mode (no SSTconfig directory).
func (a *App) GetArrowRegistry() ([]content.ArrowDef, error) {
	if a.registry == nil {
		return []content.ArrowDef{}, nil
	}
	defs := make([]content.ArrowDef, 0, len(a.registry.Arrows))
	for _, def := range a.registry.Arrows {
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].MetaType != defs[j].MetaType {
			return defs[i].MetaType < defs[j].MetaType
		}
		return defs[i].Code < defs[j].Code
	})
	return defs, nil
}

// ---------------------------------------------------------------------------
// ResolveReminder (T008)
// ---------------------------------------------------------------------------

// ResolveReminder lowercases the ALL CAPS reminder text on lineNumber of path.
// Delegates to content.ResolveReminder.
func (a *App) ResolveReminder(path string, lineNumber int) (*content.WriteBackResult, error) {
	return content.ResolveReminder(path, lineNumber)
}

// ---------------------------------------------------------------------------
// SaveFile (T009)
// ---------------------------------------------------------------------------

// atomicWriteText writes text to path via temp-file-then-rename.
func atomicWriteText(path, text string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".texere-save-*")
	if err != nil {
		return fmt.Errorf("atomicWriteText: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	w := bufio.NewWriter(tmp)
	// Normalise CRLF → LF and ensure trailing newline
	normalised := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasSuffix(normalised, "\n") {
		normalised += "\n"
	}
	if _, err := w.WriteString(normalised); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("atomicWriteText: write: %w", err)
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("atomicWriteText: flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("atomicWriteText: close: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("atomicWriteText: rename: %w", err)
	}
	return nil
}

// SaveFile atomically writes fileContent to path, then injects missing # @id
// markers for any # @card nodes. Returns the final on-disk content and the
// list of injections performed.
//
// savingPath is set before any write and cleared after to suppress false
// file:modified-externally events from consumeWatchEvents.
func (a *App) SaveFile(path, fileContent string) (*SaveFileResult, error) {
	a.savingPath = path

	if err := atomicWriteText(path, fileContent); err != nil {
		a.savingPath = ""
		return nil, fmt.Errorf("SaveFile: %w", err)
	}

	// Parse the saved file to find study-marked nodes needing # @id injection
	parsed, _ := content.ParseFile(path, a.registry)
	var injections []content.IDInjection
	if parsed != nil {
		var err error
		injections, err = content.InjectMissingIDs(parsed)
		if err != nil {
			a.savingPath = ""
			return nil, fmt.Errorf("SaveFile: inject IDs: %w", err)
		}
	}

	// Read final on-disk content (may differ if IDs were injected)
	finalData, err := os.ReadFile(path)
	a.savingPath = ""
	if err != nil {
		return nil, fmt.Errorf("SaveFile: read final: %w", err)
	}

	return &SaveFileResult{
		Injections:   injections,
		FinalContent: string(finalData),
	}, nil
}

// ---------------------------------------------------------------------------
// consumeWatchEvents (T010) — replaces the stub in app.go
// ---------------------------------------------------------------------------

// consumeWatchEvents drains the watcher event channel and emits Wails events
// to the frontend for Column 1 updates and external file modification prompts.
func (a *App) consumeWatchEvents() {
	if a.watcher == nil {
		return
	}
	for {
		select {
		case ev, ok := <-a.watcher.Events:
			if !ok {
				return
			}
			switch ev.Kind {
			case content.EventAdded, content.EventRemoved, content.EventRenamed:
				runtime.EventsEmit(a.ctx, "collection:change", ev)
			case content.EventModified:
				if ev.Path == a.openFilePath && ev.Path != a.savingPath {
					runtime.EventsEmit(a.ctx, "file:modified-externally", map[string]string{"path": ev.Path})
				}
			}
		case err, ok := <-a.watcher.Errors:
			if !ok {
				return
			}
			// Log watcher errors but keep running
			_ = err
		}
	}
}

// ---------------------------------------------------------------------------
// UpdateRecentArrows (T050)
// ---------------------------------------------------------------------------

// GetRecentArrows returns the persisted LRU list of recently used arrow codes.
func (a *App) GetRecentArrows() []string {
	cfg, err := loadConfig()
	if err != nil {
		return nil
	}
	return cfg.RecentArrows
}

// UpdateRecentArrows replaces the RecentArrows field in AppConfig with codes
// (caller is responsible for LRU ordering and 10-entry cap) and writes config.
func (a *App) UpdateRecentArrows(codes []string) error {
	cfg, err := loadConfig()
	if err != nil {
		cfg = &AppConfig{}
	}
	if len(codes) > 10 {
		codes = codes[:10]
	}
	cfg.RecentArrows = codes
	return saveConfig(cfg)
}

// ---------------------------------------------------------------------------
// GetInitialSummaries — returns summaries auto-loaded at startup
// ---------------------------------------------------------------------------

// GetInitialSummaries returns the chapter summaries loaded during startup()
// from the persisted CollectionDir. Returns nil if no collection was auto-loaded.
func (a *App) GetInitialSummaries() []content.ChapterSummary {
	return a.initialSums
}

// ---------------------------------------------------------------------------
// GetStartupError — returns error message if startup auto-load failed
// ---------------------------------------------------------------------------

// GetStartupError returns a human-readable error message when startup()
// attempted to auto-load a saved collection directory that no longer exists.
// Returns "" on first launch or successful auto-load.
func (a *App) GetStartupError() string {
	return a.startupError
}
