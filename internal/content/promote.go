package content

import (
	"fmt"
	"strings"
)

// InsertMarkerAboveNode inserts marker as its own line directly above the
// 1-indexed nodeLine. marker must be a "# @..." study/metadata marker.
func InsertMarkerAboveNode(path string, nodeLine int, marker string) error {
	if !strings.HasPrefix(strings.TrimSpace(marker), "# @") {
		return fmt.Errorf("content: InsertMarkerAboveNode: not a marker: %q", marker)
	}
	lines, err := readLines(path)
	if err != nil {
		return fmt.Errorf("content: InsertMarkerAboveNode: %w", err)
	}
	if nodeLine < 1 || nodeLine > len(lines) {
		return fmt.Errorf("content: InsertMarkerAboveNode: line %d out of range (1..%d)", nodeLine, len(lines))
	}
	idx := nodeLine - 1
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, marker)
	out = append(out, lines[idx:]...)
	if err := writeLines(path, out); err != nil {
		return fmt.Errorf("content: InsertMarkerAboveNode: write: %w", err)
	}
	return nil
}

// nodeHasFromArrow reports whether the node (or one of its ditto lines) carries
// a (from) arrow.
func nodeHasFromArrow(n *Node) bool {
	for _, a := range n.Arrows {
		if a.Code == "from" {
			return true
		}
	}
	for _, d := range n.Dittos {
		for _, a := range d.Arrows {
			if a.Code == "from" {
				return true
			}
		}
	}
	return false
}

// InjectMissingExtractIDs injects a "# @id <shortid>" for every non-study-marked
// node that carries a (from) arrow and lacks an id. Mutates parsed in place via a
// single atomic rewrite (mirrors InjectMissingIDs for extract nodes).
func InjectMissingExtractIDs(parsed *ParsedFile) ([]IDInjection, error) {
	lines, err := readLines(parsed.Path)
	if err != nil {
		return nil, fmt.Errorf("content: InjectMissingExtractIDs: %w", err)
	}

	existing := existingIDs(lines)

	type pending struct {
		chTitle string
		node    *Node
	}
	var todo []pending

	for ci := range parsed.Chapters {
		ch := &parsed.Chapters[ci]
		for ni := range ch.Nodes {
			n := &ch.Nodes[ni]
			if n.StudyMeta != nil || n.ID != "" {
				continue
			}
			if !nodeHasFromArrow(n) {
				continue
			}
			todo = append(todo, pending{chTitle: ch.Title, node: n})
		}
	}

	if len(todo) == 0 {
		return nil, nil
	}

	// Build ops and injections; process bottom-up so earlier line numbers stay valid.
	var ops []writeBackOp
	var injections []IDInjection

	for i := len(todo) - 1; i >= 0; i-- {
		p := todo[i]
		n := p.node

		shortID, err := generateUniqueID(existing, 10)
		if err != nil {
			return nil, err
		}
		existing[shortID] = true

		insertAt := n.Line // insert # @id directly above the node

		ops = append(ops, writeBackOp{
			LineNumber:   insertAt,
			Kind:         opInsert,
			Content:      "# @id " + shortID,
			InsertBefore: true,
		})

		n.IDMarkerLine = insertAt
		n.ID = NodeID(parsed.Path, p.chTitle, shortID)

		injections = append(injections, IDInjection{
			FilePath:   parsed.Path,
			NodeLine:   n.Line,
			InsertedAt: insertAt,
			ShortID:    shortID,
		})
	}

	newLines := applyOps(lines, ops)
	if err := writeLines(parsed.Path, newLines); err != nil {
		return nil, err
	}

	return injections, nil
}
