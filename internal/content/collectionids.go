package content

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CollectionShortIDs scans every .n4l file under dir for "# @id <shortid>"
// markers and returns the set of short IDs already in use anywhere in the
// collection.
//
// Identity keys are the short ID alone (see [NodeID]), so the file path no
// longer namespaces them: an ID issued in one file collides with the same ID in
// any other. Callers pass the returned set to the injectors as their `taken`
// argument so a newly issued ID is unique collection-wide, not merely
// file-wide.
//
// Directory rules match [ScanCollection] and the collection watcher via
// isExcludedDir: hidden directories at any depth, and sst_data / bleve /
// sources directly under the root, are not descended into. Files that cannot be
// read are skipped rather than failing the scan — an unreadable file's IDs
// cannot be honoured, but that is not a reason to block the save that triggered
// the scan. A missing or unreadable root IS an error: an empty set from a root
// that does not exist would silently license duplicate IDs.
//
// Returns a non-nil map even when empty; callers index into it directly.
func CollectionShortIDs(dir string) (map[string]bool, error) {
	ids := make(map[string]bool)
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err // root missing/unreadable → propagate
			}
			return fs.SkipDir // unreadable subtree → skip it, keep going
		}
		if d.IsDir() {
			if path != dir && isExcludedDir(dir, path) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".n4l") {
			return nil
		}
		fileIDs, readErr := scanShortIDs(path)
		if readErr != nil {
			return nil // unreadable file → skip, keep scanning
		}
		for id := range fileIDs {
			ids[id] = true
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("content: CollectionShortIDs: %w", walkErr)
	}
	return ids, nil
}

// scanShortIDs streams one .n4l file and returns the short IDs its "# @id"
// markers name. It does not parse: the marker is a whole line, so matching it
// costs one prefix test per line and none of the parser's chapter/arrow work.
func scanShortIDs(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ids := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(trimmed, "# @id ") {
			continue
		}
		// Same shape existingIDs matches: "# @id" is fields[0]+[1], id is [2].
		if parts := strings.Fields(trimmed); len(parts) >= 3 {
			ids[parts[2]] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
