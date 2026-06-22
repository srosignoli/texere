package content

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanCollection recursively scans dir for .n4l files and returns a sorted
// (case-insensitive by collection-relative path) slice of ChapterSummary, each
// containing the chapter titles extracted from that file.
//
// Non-.n4l files are ignored. Directories named sst_data, bleve, sources, or
// beginning with "." are not descended into (see isExcludedDir). The collection
// root itself is always scanned.
// If a file cannot be read, its ChapterSummary.ScanError is non-nil; it is still
// included. Returns a directory-level error only if dir does not exist or cannot
// be read.
//
// Performance contract: completes in < 500 ms for ≤ 500 files.
func ScanCollection(dir string) ([]ChapterSummary, error) {
	var results []ChapterSummary
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err // root missing/unreadable → propagate
			}
			return fs.SkipDir // unreadable subtree → skip it, keep going
		}
		if d.IsDir() {
			if path != dir && isExcludedDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".n4l") {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = d.Name()
		}
		summary := ChapterSummary{
			FilePath: path,
			FileName: d.Name(),
			RelPath:  filepath.ToSlash(rel),
		}
		summary.Chapters, summary.ScanError = scanChapterTitles(path)
		results = append(results, summary)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("content: ScanCollection: %w", walkErr)
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].RelPath) < strings.ToLower(results[j].RelPath)
	})

	return results, nil
}

// isExcludedDir reports whether a directory (by base name) is a non-content
// directory that the scan and watch must not descend into: the BadgerDB graph
// store, the full-text index, the imported-binary assets dir, or any hidden
// directory. Shared by ScanCollection and the collection watcher.
func isExcludedDir(name string) bool {
	switch name {
	case "sst_data", "bleve", "sources":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// scanChapterTitles reads a single .n4l file and returns its chapter titles.
// Lines starting with "- " are treated as chapter title lines.
func scanChapterTitles(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	titles := []string{} // never nil so JS receives [] not null
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// Trim leading whitespace — N4L files may indent chapter lines.
		line := strings.TrimSpace(scanner.Text())
		// Chapter header: starts with "-" (with or without trailing space),
		// but not "--" (code/comment convention) and must have non-blank content.
		if len(line) > 1 && line[0] == '-' && line[1] != '-' && line[1] != ':' {
			title := strings.TrimSpace(line[1:])
			if title != "" {
				titles = append(titles, title)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return titles, err
	}
	return titles, nil
}
