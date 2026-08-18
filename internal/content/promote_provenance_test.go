package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No anchor to preserve (a hand-written node promoted to a card) must produce
// exactly the old output — no stray ditto line.
func TestSeedBasicCardNodeWithoutAnchorIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ex.n4l")
	if err := os.WriteFile(p, []byte("- Ch\n# @id e1\n  a hand written note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SeedBasicCardNode(p, 3, "a hand written note", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want := "- Ch\n# @id e1\n# @card\n  a hand written note >> \n"
	if string(got) != want {
		t.Fatalf("seeded file:\n%q\nwant:\n%q", got, want)
	}
}

// The point of putting the anchor on a ditto rather than the node line: the card
// text must stay clean. If the anchor leaked into node.Text it would end up in
// the card's back, since basicCards splits node.Text on the first ">>".
func TestSeededCardTextExcludesTheProvenanceDitto(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ex.n4l")
	body := "- Ch\n# @id e1\n  extracted sentence (from) \"sources/x.txt char=0\"\n"
	os.WriteFile(p, []byte(body), 0o644)
	if err := SeedBasicCardNode(p, 3, "extracted sentence", "sources/x.txt char=0"); err != nil {
		t.Fatal(err)
	}
	// Author fills in the back, as they would in the editor.
	raw, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(raw), ">> \n", ">> the answer\n", 1)), 0o644)

	reg, _ := LoadRegistry(filepath.Join(dir, "none"))
	pf, _ := ParseFile(p, reg)
	if pf == nil {
		t.Fatal("nil parse")
	}
	var n *Node
	for ci := range pf.Chapters {
		for ni := range pf.Chapters[ci].Nodes {
			if pf.Chapters[ci].Nodes[ni].ID == "e1" {
				n = &pf.Chapters[ci].Nodes[ni]
			}
		}
	}
	if n == nil {
		t.Fatal("seeded node not found after reparse")
	}
	if strings.Contains(n.Text, "(from)") || strings.Contains(n.Text, "sources/") {
		t.Errorf("provenance leaked into node text %q — it would land in the card's back", n.Text)
	}
	if n.StudyMeta == nil {
		t.Error("seeded node lost its # @card StudyMeta")
	}
	if !nodeHasFromArrow(n) {
		t.Error("provenance is no longer reachable from the node: nodeHasFromArrow is false")
	}
}
