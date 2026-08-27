package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
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
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("methods: saveConfig: write: %w", err)
	}
	if err := os.Chmod(p, 0o600); err != nil {
		return fmt.Errorf("methods: saveConfig: chmod: %w", err)
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

// beginSelfWrite marks path as being written by the app so the collection
// watcher does not surface a false "modified externally" toast. It is
// reference-counted and mutex-guarded so concurrent or nested self-writes
// (SaveFile, sync writeback, reading/study operations) don't clobber each
// other's suppression. Pair every call with endSelfWrite (via defer).
func (a *App) beginSelfWrite(path string) {
	a.savingMu.Lock()
	if a.savingPaths == nil {
		a.savingPaths = make(map[string]int)
	}
	a.savingPaths[path]++
	a.savingMu.Unlock()
}

// endSelfWrite releases one self-write hold on path (see beginSelfWrite).
func (a *App) endSelfWrite(path string) {
	a.savingMu.Lock()
	if n := a.savingPaths[path]; n > 1 {
		a.savingPaths[path] = n - 1
	} else {
		delete(a.savingPaths, path)
	}
	a.savingMu.Unlock()
}

// isSelfWriting reports whether the app currently holds a self-write on path.
func (a *App) isSelfWriting(path string) bool {
	a.savingMu.Lock()
	_, ok := a.savingPaths[path]
	a.savingMu.Unlock()
	return ok
}

// SaveFile atomically writes fileContent to path, then injects missing # @id
// markers for any # @card nodes. Returns the final on-disk content and the
// list of injections performed.
//
// The path is registered as a self-write for the duration to suppress false
// file:modified-externally events from consumeWatchEvents.
func (a *App) SaveFile(path, fileContent string) (*SaveFileResult, error) {
	a.beginSelfWrite(path)
	defer a.endSelfWrite(path)

	if err := atomicWriteText(path, fileContent); err != nil {
		return nil, fmt.Errorf("SaveFile: %w", err)
	}

	// Parse the saved file to find study-marked nodes needing # @id injection
	parsed, _ := content.ParseFile(path, a.registry)
	var injections []content.IDInjection
	if parsed != nil {
		// Short IDs are the whole identity key, so uniqueness is collection-wide;
		// the set of IDs already in use has to come from the whole collection, not
		// from this file. When no collection is open (a bare file opened directly)
		// there is no collection to be unique against and taken stays nil, which
		// falls back to file-scoped uniqueness. A scan error is not fatal for the
		// same reason a save must not be: it degrades to file-scoped uniqueness
		// and is logged.
		var taken map[string]bool
		if root := a.collectionRoot(); root != "" {
			var scanErr error
			taken, scanErr = content.CollectionShortIDs(root)
			if scanErr != nil {
				log.Printf("SaveFile: collection id scan: %v", scanErr)
			}
		}
		var err error
		injections, err = content.InjectMissingIDs(parsed, taken)
		if err != nil {
			return nil, fmt.Errorf("SaveFile: inject IDs: %w", err)
		}
	}

	// Read final on-disk content (may differ if IDs were injected)
	finalData, err := os.ReadFile(path)
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
				// Always notify the file tree so chapter add/remove/rename
				// inside a file is reflected, regardless of whether the
				// change came from us (SaveFile) or an external editor.
				runtime.EventsEmit(a.ctx, "collection:change", ev)
				// Only surface the "external mod" toast when *another*
				// process changed the currently-open file.
				if ev.Path == a.openFilePath && !a.isSelfWriting(ev.Path) {
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

// validThemes is the closed set SetTheme accepts. The value reaches the DOM as
// a data-theme attribute, so an arbitrary string must never be storable.
var validThemes = map[string]bool{"blue": true, "dark": true, "light": true}

// GetTheme returns the persisted theme, defaulting to "blue" for a config that
// has none — first run, or an upgrade from before this field existed.
func (a *App) GetTheme() string {
	cfg, err := loadConfig()
	if err != nil || cfg.Theme == "" || !validThemes[cfg.Theme] {
		return "blue"
	}
	return cfg.Theme
}

// SetTheme persists the theme. An unrecognised value is rejected and the stored
// theme is left untouched.
func (a *App) SetTheme(theme string) error {
	if !validThemes[theme] {
		return fmt.Errorf("unknown theme %q", theme)
	}
	cfg, err := loadConfig()
	if err != nil {
		cfg = &AppConfig{}
	}
	cfg.Theme = theme
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

// GetInitialCollectionDir returns the absolute path of the collection auto-loaded
// at startup, or "" if none. The frontend must use this rather than deriving the
// root from a summary's FilePath: ScanCollection is recursive, so the first
// summary (sorted by collection-relative path) can live in a subdirectory.
func (a *App) GetInitialCollectionDir() string {
	return a.collectionDir
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

// CreateNewFile creates an empty `.n4l` file in the currently open
// collection and returns the same ChapterSummary shape the watcher would
// emit. The frontend can append the result optimistically; the watcher's
// EventAdded handler is idempotent (dedupes by FilePath).
//
// Validation: rejects empty / path-separator-bearing names, auto-suffixes
// `.n4l` when missing, and refuses to overwrite an existing file.
func (a *App) CreateNewFile(name string) (*content.ChapterSummary, error) {
	if a.collectionDir == "" {
		return nil, fmt.Errorf("CreateNewFile: no collection open")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("CreateNewFile: filename is empty")
	}
	if strings.ContainsAny(name, `/\`) {
		return nil, fmt.Errorf("CreateNewFile: filename cannot contain path separators")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".n4l") {
		name += ".n4l"
	}
	path := filepath.Join(a.collectionDir, name)

	switch _, err := os.Stat(path); {
	case err == nil:
		return nil, fmt.Errorf("CreateNewFile: file already exists: %s", name)
	case !os.IsNotExist(err):
		return nil, fmt.Errorf("CreateNewFile: stat: %w", err)
	}

	a.beginSelfWrite(path)
	err := atomicWriteText(path, "")
	a.endSelfWrite(path)
	if err != nil {
		return nil, fmt.Errorf("CreateNewFile: %w", err)
	}

	return &content.ChapterSummary{
		FilePath: path,
		FileName: name,
		Chapters: nil,
	}, nil
}
