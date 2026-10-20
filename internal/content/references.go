package content

// References and chains, as upstream N4L reads them (src/N4L.go: LookupAlias,
// UpdateLastLineCache, AssessGrammarCompletions).
//
// Each line has a list of items in order: the node, then each arrow's target.
// $label.N is the Nth item of the line labelled @label (a node line or a ditto
// line), $PREV.N the Nth item of the previous non-blank line, and $THIS.N the
// Nth item of the current line. A line is a chain: "A (r1) B (r2) C" is A→B
// and B→C.

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var itemRefRe = regexp.MustCompile(`^\$([A-Za-z][A-Za-z0-9_-]*)\.(\d+)$`)

// resolveReferences replaces $label.N, $PREV.N and $THIS.N in node texts and
// arrow targets with the items they refer to, line by line in file order.
// References it cannot resolve are left as written and reported. The label
// check's errors stay, except for references resolved here.
func resolveReferences(pf *ParsedFile) {
	byLine := map[int][]*string{}
	labelLine := map[string]int{}
	for ci := range pf.Chapters {
		ch := &pf.Chapters[ci]
		for ni := range ch.Nodes {
			n := &ch.Nodes[ni]
			if n.Label != "" {
				labelLine[n.Label] = n.Line
			}
			byLine[n.Line] = append(byLine[n.Line], &n.Text)
			for ai := range n.Arrows {
				a := &n.Arrows[ai]
				byLine[a.Line] = append(byLine[a.Line], &a.Target)
			}
			for di := range n.Dittos {
				d := &n.Dittos[di]
				if d.Label != "" {
					labelLine[d.Label] = d.Line
				}
				if _, ok := byLine[d.Line]; !ok {
					byLine[d.Line] = append(byLine[d.Line], &n.Text) // a ditto line's first item is its node
				}
				for ai := range d.Arrows {
					a := &d.Arrows[ai]
					byLine[a.Line] = append(byLine[a.Line], &a.Target)
				}
			}
		}
	}
	lines := make([]int, 0, len(byLine))
	for l := range byLine {
		lines = append(lines, l)
	}
	sort.Ints(lines)

	var unresolved []ParseError
	resolved := map[string]bool{}
	items := map[int][]string{}
	prev := 0
	for _, l := range lines {
		for _, text := range byLine[l] {
			if m := itemRefRe.FindStringSubmatch(*text); m != nil {
				k, _ := strconv.Atoi(m[2])
				var from []string
				switch m[1] {
				case "PREV":
					from = items[prev]
				case "THIS":
					from = items[l]
				default:
					if ll, ok := labelLine[m[1]]; ok {
						from = items[ll]
					}
				}
				if k >= 1 && k <= len(from) {
					resolved[m[0]] = true
					*text = from[k-1]
				} else {
					unresolved = append(unresolved, ParseError{File: pf.Path, Line: l, Msg: fmt.Sprintf("%s: %s", ErrUnresolvedRef, m[0])})
				}
			}
			items[l] = append(items[l], *text)
		}
		prev = l
	}
	kept := pf.Errors[:0]
	seen := map[string]bool{}
	for _, e := range pf.Errors {
		if ref, ok := strings.CutPrefix(e.Msg, ErrUnresolvedRef+": "); ok && resolved[ref] {
			continue
		}
		seen[fmt.Sprintf("%d %s", e.Line, e.Msg)] = true
		kept = append(kept, e)
	}
	for _, e := range unresolved {
		if !seen[fmt.Sprintf("%d %s", e.Line, e.Msg)] {
			kept = append(kept, e)
		}
	}
	pf.Errors = kept
}

// setArrowSources records where each arrow starts: the first arrow of a line
// at the node, each later one at the previous arrow's target.
func setArrowSources(pf *ParsedFile) {
	for ci := range pf.Chapters {
		for ni := range pf.Chapters[ci].Nodes {
			n := &pf.Chapters[ci].Nodes[ni]
			source := n.Text
			for ai := range n.Arrows {
				n.Arrows[ai].Source = source
				source = n.Arrows[ai].Target
			}
			for di := range n.Dittos {
				source = n.Text
				for ai := range n.Dittos[di].Arrows {
					n.Dittos[di].Arrows[ai].Source = source
					source = n.Dittos[di].Arrows[ai].Target
				}
			}
		}
	}
}
