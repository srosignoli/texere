package content

// NodeSpans returns the line range of every id-bearing node in a parsed file.
//
// The span starts at the node's "# @id" marker line when it has one: that line
// labels the node, and a caret resting on it is — to the reader — inside that
// node. It ends at the node's last ditto, since dittos are continuation lines of
// the same node rather than nodes of their own.
//
// Nodes without an ID are skipped. They cannot key a notebook, and emitting a
// span with an empty id would make a caret resolve to "" and peek nonsense.
func NodeSpans(pf *ParsedFile) []NodeSpan {
	out := []NodeSpan{}
	if pf == nil {
		return out
	}
	for ci := range pf.Chapters {
		for ni := range pf.Chapters[ci].Nodes {
			n := &pf.Chapters[ci].Nodes[ni]
			if n.ID == "" {
				continue
			}
			from, to := n.Line, n.Line
			if n.IDMarkerLine > 0 && n.IDMarkerLine < from {
				from = n.IDMarkerLine
			}
			for _, d := range n.Dittos {
				if d.Line > to {
					to = d.Line
				}
			}
			out = append(out, NodeSpan{ID: n.ID, FromLine: from, ToLine: to})
		}
	}
	return out
}
