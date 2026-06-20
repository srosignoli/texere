package learning

import (
	"strconv"
	"strings"

	"texere/internal/content"
)

// GenerateCards derives Phase-1 study cards (cloze + basic forward arrow) from a
// parsed file. Nodes without a # @id are skipped (they cannot be keyed).
func GenerateCards(pf *content.ParsedFile, reg *content.SSTRegistry) []GeneratedCard {
	var out []GeneratedCard
	if pf == nil {
		return out
	}
	for _, ch := range pf.Chapters {
		for _, node := range ch.Nodes {
			if node.StudyMeta == nil || node.ID == "" {
				continue
			}
			switch node.StudyMeta.CardType {
			case content.CardTypeCloze:
				out = append(out, clozeCards(node.ID, pf.Path, node.Text)...)
			case content.CardTypeBasic:
				out = append(out, basicCards(node.ID, pf.Path, node.Text, node.Arrows, reg)...)
			}
		}
	}
	return out
}

// braceGroups returns the [start,end) byte ranges of each {...} group, in order.
func braceGroups(text string) [][2]int {
	var groups [][2]int
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		if j := strings.IndexByte(text[i:], '}'); j >= 0 {
			groups = append(groups, [2]int{i, i + j + 1})
			i += j
		}
	}
	return groups
}

// stripBraces removes every { and } from text (the "back" rendering).
func stripBraces(text string) string {
	r := strings.NewReplacer("{", "", "}", "")
	return r.Replace(text)
}

func clozeCards(nodeID, filePath, text string) []GeneratedCard {
	groups := braceGroups(text)
	var out []GeneratedCard
	for idx := range groups {
		// Build the front: the chosen group becomes […], others lose their braces.
		var b strings.Builder
		last := 0
		for k, gg := range groups {
			b.WriteString(stripBraces(text[last:gg[0]]))
			if k == idx {
				b.WriteString("[…]")
			} else {
				b.WriteString(stripBraces(text[gg[0]:gg[1]]))
			}
			last = gg[1]
		}
		b.WriteString(stripBraces(text[last:]))
		out = append(out, GeneratedCard{
			NodeID: nodeID, FilePath: filePath, CardType: CardTypeCloze,
			Front: b.String(), Back: stripBraces(text), Discriminator: strconv.Itoa(idx),
		})
	}
	return out
}

func basicCards(nodeID, filePath, text string, arrows []content.Arrow, reg *content.SSTRegistry) []GeneratedCard {
	var out []GeneratedCard
	for i, a := range arrows {
		def, ok := reg.Arrows[a.Code]
		if !ok || !def.Studyable {
			continue
		}
		out = append(out, GeneratedCard{
			NodeID: nodeID, FilePath: filePath, CardType: CardTypeBasic,
			Front: text + " — " + def.LongName + "?", Back: a.Target,
			Discriminator: strconv.Itoa(i),
		})
	}
	return out
}
