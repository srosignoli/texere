package content

import (
	"fmt"
	"os"
	"strings"
)

// CreateReadingNotes writes the per-source notes scaffold for a reading element
// and returns the generated # @id of the reading-element node. The reading
// element is a node (text = title) carrying # @ref/# @source/# @progress and an
// explicit # @id (these nodes are not study-marked, so InjectMissingIDs would
// not key them). Extracts are appended later under the _sequence_ context.
func CreateReadingNotes(notesPath, sourceRel, title string) (string, error) {
	if _, err := os.Stat(notesPath); err == nil {
		return "", fmt.Errorf("content: CreateReadingNotes: file exists: %s", notesPath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("content: CreateReadingNotes: stat: %w", err)
	}
	id, err := GenerateShortID()
	if err != nil {
		return "", err
	}
	lines := []string{
		"- " + title,
		"# @id " + id,
		`# @ref "` + sourceRel + `"`,
		`# @source "` + title + `"`,
		"# @progress char=0",
		title,
		":: " + title + " _sequence_",
		"",
	}
	if err := writeLines(notesPath, lines); err != nil {
		return "", fmt.Errorf("content: CreateReadingNotes: write: %w", err)
	}
	return id, nil
}

// UpdateProgressMarker rewrites the single "# @progress" line in notesPath.
func UpdateProgressMarker(notesPath, newValue string) error {
	lines, err := readLines(notesPath)
	if err != nil {
		return fmt.Errorf("content: UpdateProgressMarker: %w", err)
	}
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "# @progress") {
			lines[i] = "# @progress " + newValue
			if err := writeLines(notesPath, lines); err != nil {
				return fmt.Errorf("content: UpdateProgressMarker: write: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("content: UpdateProgressMarker: no # @progress line in %s", notesPath)
}
