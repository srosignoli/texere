package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInsertMarkerAboveNode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	body := "- Ch\n# @id a1\nSystem 1 is fast\n    (from) \"sources/f.txt char=0\"\n"
	os.WriteFile(p, []byte(body), 0o644)
	// node "System 1 is fast" is line 3 (1-indexed).
	if err := InsertMarkerAboveNode(p, 3, "# @cloze"); err != nil {
		t.Fatalf("InsertMarkerAboveNode: %v", err)
	}
	out, _ := os.ReadFile(p)
	lines := strings.Split(string(out), "\n")
	if strings.TrimSpace(lines[2]) != "# @cloze" || strings.TrimSpace(lines[3]) != "System 1 is fast" {
		t.Fatalf("marker not directly above node:\n%s", out)
	}
	if err := InsertMarkerAboveNode(p, 999, "# @card"); err == nil {
		t.Fatal("expected out-of-range error")
	}
	if err := InsertMarkerAboveNode(p, 3, "not-a-marker"); err == nil {
		t.Fatal("expected invalid-marker error")
	}
}

func TestInjectMissingExtractIDs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	// A (from) node with NO # @id and no card markers.
	body := "- Ch\nanchoring shifts estimates\n    (from) \"sources/f.txt char=20\"\n"
	os.WriteFile(p, []byte(body), 0o644)
	reg := &SSTRegistry{Arrows: map[string]ArrowDef{"from": {Code: "from", LongName: "from"}}}
	pf, _ := ParseFile(p, reg)
	injs, err := InjectMissingExtractIDs(pf)
	if err != nil || len(injs) != 1 {
		t.Fatalf("InjectMissingExtractIDs n=%d err=%v want 1", len(injs), err)
	}
	out, _ := os.ReadFile(p)
	if !strings.Contains(string(out), "# @id ") {
		t.Fatalf("expected an injected # @id:\n%s", out)
	}
}
