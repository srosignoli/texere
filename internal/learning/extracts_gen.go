package learning

import "texere/internal/content"

// GenerateExtracts finds (from) nodes that are not study-marked and have an id.
func GenerateExtracts(pf *content.ParsedFile) []Extract {
	var out []Extract
	if pf == nil {
		return out
	}
	for _, ch := range pf.Chapters {
		for _, node := range ch.Nodes {
			if node.StudyMeta != nil || node.ID == "" {
				continue
			}
			anchor, ok := fromArrowTarget(node)
			if !ok {
				continue
			}
			out = append(out, Extract{ID: node.ID, FilePath: pf.Path, Text: node.Text, FromAnchor: anchor})
		}
	}
	return out
}

func fromArrowTarget(n content.Node) (string, bool) {
	for _, a := range n.Arrows {
		if a.Code == "from" {
			return a.Target, true
		}
	}
	for _, d := range n.Dittos {
		for _, a := range d.Arrows {
			if a.Code == "from" {
				return a.Target, true
			}
		}
	}
	return "", false
}
