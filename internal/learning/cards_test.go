package learning

import (
	"os"
	"path/filepath"
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
	return p, &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}
}

func findCard(cards []GeneratedCard, nodeID, cardType, disc string) *GeneratedCard {
	for i := range cards {
		if cards[i].NodeID == nodeID &&
			cards[i].CardType == cardType && cards[i].Discriminator == disc {
			return &cards[i]
		}
	}
	return nil
}

func TestGenerateBasicAndClozeCards(t *testing.T) {
	body := "- Ch\n" +
		"# @id n1\n# @cloze\nSystem 1 is {{fast}} and {{automatic}}\n" +
		"# @id n2\n# @card\nWhat is System 2? >> effortful\n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	got := GenerateCards(pf)

	if len(got) != 3 {
		t.Fatalf("got %d cards want 3: %+v", len(got), got)
	}
	cloze0 := findCard(got, "n1", CardTypeCloze, "0")
	if cloze0 == nil || cloze0.Front != "System 1 is […] and automatic" ||
		cloze0.Back != "System 1 is fast and automatic" {
		t.Fatalf("cloze0=%+v", cloze0)
	}
	basic := findCard(got, "n2", CardTypeBasic, "0")
	if basic == nil || basic.Front != "What is System 2?" || basic.Back != "effortful" {
		t.Fatalf("basic=%+v", basic)
	}
}

func TestGenerateReverseEmitsTwoCards(t *testing.T) {
	body := "- Ch\n# @id n1\n# @card reverse\ncapital of France >> Paris\n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	got := GenerateCards(pf)

	if len(got) != 2 {
		t.Fatalf("got %d cards want 2: %+v", len(got), got)
	}
	fwd := findCard(got, "n1", CardTypeBasic, "0")
	rev := findCard(got, "n1", CardTypeBasic, "1")
	if fwd == nil || fwd.Front != "capital of France" || fwd.Back != "Paris" {
		t.Fatalf("fwd=%+v", fwd)
	}
	if rev == nil || rev.Front != "Paris" || rev.Back != "capital of France" {
		t.Fatalf("rev=%+v", rev)
	}
}

func TestBasicNoSeparatorYieldsNoCard(t *testing.T) {
	body := "- Ch\n# @id n1\n# @card\njust a statement with no separator\n" +
		"# @id n2\n# @card\nempty back >> \n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	if got := GenerateCards(pf); len(got) != 0 {
		t.Fatalf("got %d cards want 0: %+v", len(got), got)
	}
}

func TestClozeSingleBraceIsNotABlank(t *testing.T) {
	body := "- Ch\n# @id n1\n# @cloze\nThe capital is {Paris}\n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	if got := GenerateCards(pf); len(got) != 0 {
		t.Fatalf("single-brace cloze should yield no card, got %+v", got)
	}
}

// Regression for the original TC-1.1 failure: with the REAL SSTconfig registry,
// a # @card node with >> must produce exactly one item (the old arrow/Studyable
// path produced zero because ArrowDef.Studyable was never populated).
func TestGenerateBasicWithRealRegistry(t *testing.T) {
	reg, err := content.LoadRegistry(filepath.Join("..", "..", "SSTconfig"))
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "card.n4l")
	if err := os.WriteFile(p, []byte("- Ch\n# @id k1\n# @card\ncapital of France >> Paris\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, _ := content.ParseFile(p, reg)
	got := GenerateCards(pf)
	if len(got) != 1 || got[0].Front != "capital of France" || got[0].Back != "Paris" {
		t.Fatalf("real-registry basic: got %+v want 1 card Q='capital of France' A='Paris'", got)
	}
}
