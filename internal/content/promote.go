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

// SeedBasicCardNode rewrites the node at the 1-indexed nodeLine into a seeded
// basic card: the line becomes "<front> >> " (back left empty for the author),
// preserving the original line's leading indentation, and a "# @card" marker is
// inserted directly above it — in a single atomic rewrite. Used when promoting an
// extract to a basic card; the caller passes the extract's passage text as front
// and the original (from) anchor is intentionally dropped from the file.
func SeedBasicCardNode(path string, nodeLine int, front string) error {
	lines, err := readLines(path)
	if err != nil {
		return fmt.Errorf("content: SeedBasicCardNode: %w", err)
	}
	if nodeLine < 1 || nodeLine > len(lines) {
		return fmt.Errorf("content: SeedBasicCardNode: line %d out of range (1..%d)", nodeLine, len(lines))
	}
	idx := nodeLine - 1
	indent := lines[idx][:len(lines[idx])-len(strings.TrimLeft(lines[idx], " \t"))]
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, "# @card")
	out = append(out, indent+front+" >> ")
	out = append(out, lines[idx+1:]...)
	if err := writeLines(path, out); err != nil {
		return fmt.Errorf("content: SeedBasicCardNode: write: %w", err)
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
//
// Note: after injection the in-memory parsed nodes' line numbers (Line /
// IDMarkerLine) are NOT shifted to reflect the inserted "# @id" lines. Callers
// that need line-accurate positions after the rewrite should re-parse the file.
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

	// Assign IDs to all pending nodes (forward order, top-to-bottom).
	type assigned struct {
		p        pending
		shortID  string
		insertAt int
	}
	assigns := make([]assigned, len(todo))
	for i := 0; i < len(todo); i++ {
		p := todo[i]
		n := p.node
		shortID, err := generateUniqueID(existing, 10)
		if err != nil {
			return nil, err
		}
		existing[shortID] = true
		assigns[i] = assigned{p: p, shortID: shortID, insertAt: n.Line}
	}

	// Build insertion ops bottom-up so earlier line numbers remain valid after each insert.
	var ops []writeBackOp
	for i := len(assigns) - 1; i >= 0; i-- {
		a := assigns[i]
		n := a.p.node
		ops = append(ops, writeBackOp{
			LineNumber:   a.insertAt,
			Kind:         opInsert,
			Content:      "# @id " + a.shortID,
			InsertBefore: true,
		})
		n.IDMarkerLine = a.insertAt
		n.ID = NodeID(parsed.Path, a.p.chTitle, a.shortID)
	}

	newLines := applyOps(lines, ops)
	if err := writeLines(parsed.Path, newLines); err != nil {
		return nil, err
	}

	// Build the returned slice in forward (top-to-bottom) document order.
	injections := make([]IDInjection, len(assigns))
	for i, a := range assigns {
		injections[i] = IDInjection{
			FilePath:   parsed.Path,
			NodeLine:   a.p.node.Line,
			InsertedAt: a.insertAt,
			ShortID:    a.shortID,
		}
	}

	return injections, nil
}
