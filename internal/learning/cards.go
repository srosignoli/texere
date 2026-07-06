package learning

import (
	"strconv"
	"strings"

	"texere/internal/content"
)

// GenerateCards derives Phase-1 study cards from a parsed file. Basic and reverse
// cards split the node's literal text on the first ">>"; cloze cards blank each
// {{double-brace}} group. Nodes without a # @id are skipped (they cannot be keyed).
func GenerateCards(pf *content.ParsedFile) []GeneratedCard {
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
				out = append(out, basicCards(node.ID, pf.Path, node.Text, false)...)
			case content.CardTypeReverse:
				out = append(out, basicCards(node.ID, pf.Path, node.Text, true)...)
			}
		}
	}
	return out
}

// basicCards splits text on the first ">>" into front/back. A line without ">>"
// (or with an empty side after trimming) yields no card. With reverse, the swapped
// card is emitted too (Discriminator "1").
func basicCards(nodeID, filePath, text string, reverse bool) []GeneratedCard {
	front, back, ok := strings.Cut(text, ">>")
	if !ok {
		return nil
	}
	front = strings.TrimSpace(front)
	back = strings.TrimSpace(back)
	if front == "" || back == "" {
		return nil
	}
	out := []GeneratedCard{{
		NodeID: nodeID, FilePath: filePath, CardType: CardTypeBasic,
		Front: front, Back: back, Discriminator: "0",
	}}
	if reverse {
		out = append(out, GeneratedCard{
			NodeID: nodeID, FilePath: filePath, CardType: CardTypeBasic,
			Front: back, Back: front, Discriminator: "1",
		})
	}
	return out
}

// braceGroups returns the [start,end) byte ranges of each {{...}} group, in order.
func braceGroups(text string) [][2]int {
	var groups [][2]int
	i := 0
	for i+1 < len(text) {
		if text[i] == '{' && text[i+1] == '{' {
			if j := strings.Index(text[i+2:], "}}"); j >= 0 {
				end := i + 2 + j + 2
				groups = append(groups, [2]int{i, end})
				i = end
				continue
			}
			break
		}
		i++
	}
	return groups
}

// stripBraces removes every {{ and }} from text (the "back" rendering).
func stripBraces(text string) string {
	r := strings.NewReplacer("{{", "", "}}", "")
	return r.Replace(text)
}

func clozeCards(nodeID, filePath, text string) []GeneratedCard {
	groups := braceGroups(text)
	var out []GeneratedCard
	for idx := range groups {
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
