package content

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanCollection scans dir for .n4l files and returns a sorted (case-insensitive
// alphabetical by filename) slice of ChapterSummary, each containing the chapter
// titles extracted from that file.
//
// Non-.n4l files and subdirectories are silently ignored.
// If a file cannot be read, its ChapterSummary.ScanError is non-nil; it is still
// included in the returned slice.
// Returns a directory-level error only if dir does not exist or cannot be read.
//
// Performance contract: completes in < 500 ms for ≤ 500 files.
func ScanCollection(dir string) ([]ChapterSummary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("content: ScanCollection: %w", err)
	}

	var results []ChapterSummary
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), ".n4l") {
			continue
		}
		path := filepath.Join(dir, name)
		summary := ChapterSummary{
			FilePath: path,
			FileName: name,
		}
		summary.Chapters, summary.ScanError = scanChapterTitles(path)
		results = append(results, summary)
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].FileName) < strings.ToLower(results[j].FileName)
	})

	return results, nil
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
