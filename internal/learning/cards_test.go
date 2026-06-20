package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"texere/internal/content"
)

func writeFixture(t *testing.T, body string) (string, *content.SSTRegistry) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{
		"means": {Code: "means", LongName: "means", Studyable: true, MetaType: "properties"},
		"note":  {Code: "note", LongName: "note", Studyable: false},
	}}
	return p, reg
}

func TestGenerateClozeAndBasicCards(t *testing.T) {
	// Cloze node + a basic node with one studyable and one non-studyable arrow.
	body := "- Ch\n" +
		"# @id n1\n" +
		"# @cloze\n" +
		"System 1 is {fast} and {automatic}\n" +
		"# @id n2\n" +
		"# @card\n" +
		"System 2 (means) effortful (note) hint\n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	got := GenerateCards(pf, reg)

	// Expect 2 cloze cards (fast, automatic) + 1 basic card (means -> effortful).
	if len(got) != 3 {
		t.Fatalf("got %d cards, want 3: %+v", len(got), got)
	}

	// The parser computes node.ID as filepath+":"+chapterTitle+":id:"+shortid,
	// so we match by suffix ":id:n1" / ":id:n2".
	var cloze0 *GeneratedCard
	var basic *GeneratedCard
	for i := range got {
		if strings.HasSuffix(got[i].NodeID, ":id:n1") && got[i].Discriminator == "0" {
			cloze0 = &got[i]
		}
		if strings.HasSuffix(got[i].NodeID, ":id:n2") && got[i].CardType == CardTypeBasic {
			basic = &got[i]
		}
	}
	if cloze0 == nil {
		t.Fatalf("missing cloze card 0")
	}
	if cloze0.Front != "System 1 is […] and automatic" || cloze0.Back != "System 1 is fast and automatic" {
		t.Fatalf("cloze0 front=%q back=%q", cloze0.Front, cloze0.Back)
	}
	if basic == nil {
		t.Fatalf("missing basic card")
	}
	if basic.Front != "System 2 — means?" || basic.Back != "effortful" {
		t.Fatalf("basic front=%q back=%q", basic.Front, basic.Back)
	}
}
