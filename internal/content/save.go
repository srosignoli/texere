package content

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// SaveResult is what Save leaves behind: the # @id markers it injected and the
// file's final on-disk content.
type SaveResult struct {
	Injections   []IDInjection
	FinalContent string
}

// WriteFileAtomic writes text to path via temp-file-then-rename, normalising
// CRLF to LF and ensuring a trailing newline.
func WriteFileAtomic(path, text string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".texere-save-*")
	if err != nil {
		return fmt.Errorf("WriteFileAtomic: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	w := bufio.NewWriter(tmp)
	normalised := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasSuffix(normalised, "\n") {
		normalised += "\n"
	}
	if _, err := w.WriteString(normalised); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("WriteFileAtomic: write: %w", err)
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("WriteFileAtomic: flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("WriteFileAtomic: close: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("WriteFileAtomic: rename: %w", err)
	}
	return nil
}

// Save atomically writes text to path, then injects missing # @id markers for
// any study-marked nodes. collectionRoot is the open Collection's directory,
// or "" for a bare file with no Collection to be unique against.
//
// The caller brackets the call as a self-write when a watcher is running.
func Save(path, text string, reg *SSTRegistry, collectionRoot string) (SaveResult, error) {
	if err := WriteFileAtomic(path, text); err != nil {
		return SaveResult{}, fmt.Errorf("SaveFile: %w", err)
	}

	// Parse the saved file to find study-marked nodes needing # @id injection
	parsed, _ := ParseFile(path, reg)
	var injections []IDInjection
	if parsed != nil {
		// Short IDs are the whole identity key, so uniqueness is collection-wide;
		// the set of IDs already in use has to come from the whole collection, not
		// from this file. When no collection is open (a bare file opened directly)
		// there is no collection to be unique against and taken stays nil, which
		// falls back to file-scoped uniqueness. A scan error is not fatal for the
		// same reason a save must not be: it degrades to file-scoped uniqueness
		// and is logged.
		var taken map[string]bool
		if collectionRoot != "" {
			var scanErr error
			taken, scanErr = CollectionShortIDs(collectionRoot)
			if scanErr != nil {
				log.Printf("SaveFile: collection id scan: %v", scanErr)
			}
		}
		var err error
		injections, err = InjectMissingIDs(parsed, taken)
		if err != nil {
			return SaveResult{}, fmt.Errorf("SaveFile: inject IDs: %w", err)
		}
	}

	// Read final on-disk content (may differ if IDs were injected)
	finalData, err := os.ReadFile(path)
	if err != nil {
		return SaveResult{}, fmt.Errorf("SaveFile: read final: %w", err)
	}
	return SaveResult{Injections: injections, FinalContent: string(finalData)}, nil
}
