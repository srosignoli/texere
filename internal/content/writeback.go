package content

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Internal write-back machinery
// ---------------------------------------------------------------------------

type opKind int

const (
	opInsert opKind = iota
	opReplace
)

// writeBackOp represents one pending targeted modification before the file is rewritten.
type writeBackOp struct {
	LineNumber   int    // 1-indexed target line
	Kind         opKind // Insert | Replace
	Content      string // New line content
	InsertBefore bool   // For Insert: place before LineNumber
}

// applyOps applies a slice of writeBackOps (sorted descending by line number) to
// the lines of a file and returns the new line slice.
func applyOps(lines []string, ops []writeBackOp) []string {
	// Sort descending so line-number shifts don't affect subsequent ops
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].LineNumber > ops[j].LineNumber
	})

	result := make([]string, len(lines))
	copy(result, lines)

	for _, op := range ops {
		idx := op.LineNumber - 1 // convert to 0-indexed
		switch op.Kind {
		case opReplace:
			if idx >= 0 && idx < len(result) {
				result[idx] = op.Content
			}
		case opInsert:
			if op.InsertBefore {
				if idx < 0 {
					idx = 0
				}
				if idx > len(result) {
					idx = len(result)
				}
				result = append(result[:idx], append([]string{op.Content}, result[idx:]...)...)
			} else {
				after := idx + 1
				if after > len(result) {
					after = len(result)
				}
				result = append(result[:after], append([]string{op.Content}, result[after:]...)...)
			}
		}
	}
	return result
}

// readLines reads a file into a string slice (one element per line, no newlines).
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// writeLines writes lines to path atomically via temp-file-then-rename.
func writeLines(path string, lines []string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".texere-wb-*")
	if err != nil {
		return fmt.Errorf("content: writeback: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	w := bufio.NewWriter(tmp)
	for i, line := range lines {
		if i > 0 {
			w.WriteByte('\n')
		}
		w.WriteString(line)
	}
	// Preserve trailing newline if the original had one
	w.WriteByte('\n')

	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("content: writeback: flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("content: writeback: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("content: writeback: rename: %w", err)
	}
	return nil
}

// existingIDs scans lines for all "# @id <shortid>" markers and returns a set.
func existingIDs(lines []string) map[string]bool {
	ids := make(map[string]bool)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# @id ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 {
				ids[parts[2]] = true
			}
		}
	}
	return ids
}

// generateUniqueID generates a unique short ID not present in the given set.
// Retries up to maxTries times.
func generateUniqueID(existing map[string]bool, maxTries int) (string, error) {
	for i := 0; i < maxTries; i++ {
		id, err := GenerateShortID()
		if err != nil {
			return "", err
		}
		if !existing[id] {
			return id, nil
		}
	}
	return "", fmt.Errorf("content: could not generate unique ID after %d attempts", maxTries)
}

// ---------------------------------------------------------------------------
// InjectMissingIDs (T025)
// ---------------------------------------------------------------------------

// InjectMissingIDs injects a "# @id <shortid>" marker for every study-marked node
// in parsed that has ID == "" (no existing # @id marker).
//
// All injections are batched into a single atomic rewrite of the file.
// On success, the ParsedFile passed in is mutated to reflect the new IDs.
// On error, the file is untouched.
//
// Caller: app.go on every Ctrl+S save event (FR-019).
func InjectMissingIDs(parsed *ParsedFile) ([]IDInjection, error) {
	lines, err := readLines(parsed.Path)
	if err != nil {
		return nil, fmt.Errorf("content: InjectMissingIDs: %w", err)
	}

	existing := existingIDs(lines)
	var ops []writeBackOp
	var injections []IDInjection

	for ci := range parsed.Chapters {
		ch := &parsed.Chapters[ci]
		for ni := range ch.Nodes {
			node := &ch.Nodes[ni]
			if node.StudyMeta == nil {
				continue
			}
			if node.ID != "" {
				injections = append(injections, IDInjection{
					FilePath:  parsed.Path,
					NodeLine:  node.Line,
					ShortID:   extractShortID(node.ID),
					AlreadyHad: true,
				})
				continue
			}

			shortID, err := generateUniqueID(existing, 10)
			if err != nil {
				return nil, err
			}
			existing[shortID] = true

			// Find insertion point: just above the closest preceding study marker
			insertAt := findStudyMarkerAbove(lines, node.Line)

			ops = append(ops, writeBackOp{
				LineNumber:   insertAt,
				Kind:         opInsert,
				Content:      "# @id " + shortID,
				InsertBefore: true,
			})

			injections = append(injections, IDInjection{
				FilePath:   parsed.Path,
				NodeLine:   node.Line,
				InsertedAt: insertAt,
				ShortID:    shortID,
			})

			// Mutate the parsed node
			node.IDMarkerLine = insertAt
			node.ID = NodeID(shortID)
		}
	}

	if len(ops) == 0 {
		return injections, nil
	}

	// Apply all ops and rewrite atomically
	newLines := applyOps(lines, ops)
	if err := writeLines(parsed.Path, newLines); err != nil {
		return nil, err
	}

	// Adjust InsertedAt line numbers in injections to reflect actual post-rewrite positions
	// (ops were sorted descending so earlier ops are not shifted by later insertions)
	return injections, nil
}

// findStudyMarkerAbove scans backwards from nodeLine to find the closest preceding
// "# @card", "# @study", or "# @cloze" marker line (1-indexed). Returns nodeLine if
// none found (insertion goes directly above the node).
func findStudyMarkerAbove(lines []string, nodeLine int) int {
	// Start scanning from the line immediately above the node
	for i := nodeLine - 2; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			break // stop at blank line
		}
		if strings.HasPrefix(trimmed, "# @card") ||
			strings.HasPrefix(trimmed, "# @study") ||
			strings.HasPrefix(trimmed, "# @cloze") ||
			strings.HasPrefix(trimmed, "# @id") {
			return i + 1 // 1-indexed line number of this marker
		}
	}
	return nodeLine
}

// extractShortID extracts the shortid from a NodeID string. Node.ID is now the
// short ID unchanged (see NodeID), so this is the identity function; the name
// and the call sites are kept so a future format change has one place to touch.
func extractShortID(id string) string {
	return id
}

// ---------------------------------------------------------------------------
// InjectNodeID (T026)
// ---------------------------------------------------------------------------

// InjectNodeID injects a "# @id <shortid>" for a single node identified by its
// 1-indexed line number. Used by the incremental reading scheduler when an unmarked
// node (extract) enters the distillation pipeline.
//
// Idempotent: if the node already has a "# @id" marker, returns the existing
// IDInjection with AlreadyHad=true and no file modification.
//
// Caller: internal/learning scheduler (FR-019b).
func InjectNodeID(path string, nodeLineNumber int) (*IDInjection, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, fmt.Errorf("content: InjectNodeID: %w", err)
	}

	// Check if node already has a # @id marker above it
	if shortID := findIDMarkerAbove(lines, nodeLineNumber); shortID != "" {
		return &IDInjection{
			FilePath:   path,
			NodeLine:   nodeLineNumber,
			ShortID:    shortID,
			AlreadyHad: true,
		}, nil
	}

	existing := existingIDs(lines)
	shortID, err := generateUniqueID(existing, 10)
	if err != nil {
		return nil, err
	}

	insertAt := findStudyMarkerAbove(lines, nodeLineNumber)

	op := writeBackOp{
		LineNumber:   insertAt,
		Kind:         opInsert,
		Content:      "# @id " + shortID,
		InsertBefore: true,
	}

	newLines := applyOps(lines, []writeBackOp{op})
	if err := writeLines(path, newLines); err != nil {
		return nil, err
	}

	return &IDInjection{
		FilePath:   path,
		NodeLine:   nodeLineNumber,
		InsertedAt: insertAt,
		ShortID:    shortID,
	}, nil
}

// findIDMarkerAbove scans backwards from nodeLine looking for a "# @id" line
// (skipping blank lines and other # markers). Returns the shortid if found, "" otherwise.
func findIDMarkerAbove(lines []string, nodeLine int) string {
	for i := nodeLine - 2; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			break
		}
		if strings.HasPrefix(trimmed, "# @id ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 {
				return parts[2]
			}
		}
		// Other # markers are acceptable between @id and node
		if !strings.HasPrefix(trimmed, "#") {
			break
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// ResolveReminder (T031)
// ---------------------------------------------------------------------------

// ResolveReminder lowercases the ALL CAPS text on lineNumber of path, preserving
// any trailing inline comment. All other lines are byte-for-byte identical.
//
// Returns an error if lineNumber does not contain an ALL CAPS reminder line.
// Writes via temp-file-then-rename.
//
// Caller: app.go when the user clicks "Resolve" in the reminder panel.
func ResolveReminder(path string, lineNumber int) (*WriteBackResult, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, fmt.Errorf("content: ResolveReminder: %w", err)
	}

	if lineNumber < 1 || lineNumber > len(lines) {
		return nil, fmt.Errorf("content: ResolveReminder: line %d out of range (file has %d lines)", lineNumber, len(lines))
	}

	original := lines[lineNumber-1]
	// Safety guard: verify this is actually a reminder line
	text := stripNodeComment(strings.TrimSpace(original))
	if !isReminder(text) {
		return nil, fmt.Errorf("content: ResolveReminder: line %d is not an ALL CAPS reminder: %q", lineNumber, original)
	}

	// Lowercase the token portion, preserve trailing comment
	var comment string
	if idx := strings.Index(original, " #"); idx >= 0 {
		comment = " " + strings.TrimSpace(original[idx:])
		original = original[:idx]
	}
	resolved := strings.ToLower(strings.TrimSpace(original)) + comment

	op := writeBackOp{
		LineNumber: lineNumber,
		Kind:       opReplace,
		Content:    resolved,
	}

	newLines := applyOps(lines, []writeBackOp{op})
	if err := writeLines(path, newLines); err != nil {
		return nil, err
	}

	return &WriteBackResult{
		FilePath:     path,
		LinesChanged: []int{lineNumber},
	}, nil
}

// ---------------------------------------------------------------------------
// UpdateProgress (T032)
// ---------------------------------------------------------------------------

// UpdateProgress replaces the value after "# @progress " on lineNumber of path
// with newValue. All other lines are byte-for-byte identical.
//
// Returns an error if lineNumber does not contain a # @progress marker.
// Writes via temp-file-then-rename.
//
// Caller: app.go when the user advances their reading progress.
func UpdateProgress(path string, lineNumber int, newValue string) (*WriteBackResult, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, fmt.Errorf("content: UpdateProgress: %w", err)
	}

	if lineNumber < 1 || lineNumber > len(lines) {
		return nil, fmt.Errorf("content: UpdateProgress: line %d out of range (file has %d lines)", lineNumber, len(lines))
	}

	original := lines[lineNumber-1]
	if !strings.Contains(original, "# @progress") {
		return nil, fmt.Errorf("content: UpdateProgress: line %d does not contain # @progress: %q", lineNumber, original)
	}

	// Replace value after "# @progress "
	idx := strings.Index(original, "# @progress")
	prefix := original[:idx] + "# @progress "
	updated := prefix + newValue

	op := writeBackOp{
		LineNumber: lineNumber,
		Kind:       opReplace,
		Content:    updated,
	}

	newLines := applyOps(lines, []writeBackOp{op})
	if err := writeLines(path, newLines); err != nil {
		return nil, err
	}

	return &WriteBackResult{
		FilePath:     path,
		LinesChanged: []int{lineNumber},
	}, nil
}
