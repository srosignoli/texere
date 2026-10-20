package content

import (
	"os"
	"path/filepath"
	"testing"
)

func parseString(t *testing.T, src string, codes ...string) *ParsedFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "refs.n4l")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := emptyRegistry()
	for _, c := range codes {
		reg.Arrows[c] = ArrowDef{Code: c}
	}
	pf, _ := ParseFile(path, reg)
	if pf == nil {
		t.Fatal("nil ParsedFile")
	}
	return pf
}

// An N4L line is a chain: "A (r1) B (r2) C" is A→B and B→C (upstream N4L).
func TestArrowSourcesChain(t *testing.T) {
	pf := parseString(t, "- ch\n\nalways (eh) 总是 (hp) zǒng shì\n   \" (e.g.) an example (he) its translation\n", "eh", "hp", "e.g.", "he")
	n := pf.Chapters[0].Nodes[0]
	want := [][2]string{{"always", "总是"}, {"总是", "zǒng shì"}}
	for i, a := range n.Arrows {
		if a.Source != want[i][0] || a.Target != want[i][1] {
			t.Errorf("arrow %d: %q -> %q, want %q -> %q", i, a.Source, a.Target, want[i][0], want[i][1])
		}
	}
	d := n.Dittos[0].Arrows
	if d[0].Source != "always" || d[1].Source != "an example" {
		t.Errorf("ditto line chain: %q, %q (a ditto line starts again at its node)", d[0].Source, d[1].Source)
	}
}

// References name items: $label.N (node or ditto line), $PREV.N, $THIS.N.
func TestReferencesResolved(t *testing.T) {
	src := `- poetry

 +:: _sequence_ ::
@title Mary had a little lamb  (note) Had means possessed
 -:: _sequence_ ::

 $title.1 (example of) Nursery rhyme

 Lopez 2025 (claims) quantified outcomes score higher
 $PREV.2 (note) seen in funded proposals
 impact section (needs) three outcomes (note) $THIS.1

 mei
@meirule " (rule) negates past actions (e.g.) ni mei qu
 later (see also) $meirule.2
 $nowhere.1 (note) dangling
`
	pf := parseString(t, src, "note", "example of", "claims", "needs", "rule", "e.g.", "see also")
	texts := map[string]bool{}
	targets := map[string]bool{}
	for _, n := range pf.Chapters[0].Nodes {
		texts[n.Text] = true
		for _, a := range n.Arrows {
			targets[a.Target] = true
		}
	}
	for _, want := range []string{"Mary had a little lamb", "quantified outcomes score higher"} {
		if !texts[want] {
			t.Errorf("node %q missing; nodes %v", want, texts)
		}
	}
	for _, want := range []string{"impact section", "negates past actions"} {
		if !targets[want] {
			t.Errorf("target %q missing; targets %v", want, targets)
		}
	}
	// The chain after a $THIS reference starts at the previous target.
	for _, n := range pf.Chapters[0].Nodes {
		if n.Text == "impact section" && n.Arrows[1].Source != "three outcomes" {
			t.Errorf("source of (note): %q", n.Arrows[1].Source)
		}
	}
	unresolved := 0
	for _, e := range pf.Errors {
		if e.Msg == ErrUnresolvedRef+": $nowhere.1" {
			unresolved++
		} else if len(e.Msg) >= len(ErrUnresolvedRef) && e.Msg[:len(ErrUnresolvedRef)] == ErrUnresolvedRef {
			t.Errorf("resolved reference still reported: %v", e)
		}
	}
	if unresolved != 1 {
		t.Errorf("want $nowhere.1 reported once, errors %v", pf.Errors)
	}
}
