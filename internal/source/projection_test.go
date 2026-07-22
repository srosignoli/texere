package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProjectionPlainTextPassthrough(t *testing.T) {
	p := write(t, "a.txt", "System 1 is fast.\nSystem 2 is slow.")
	got, err := Projection(p)
	if err != nil || got != "System 1 is fast.\nSystem 2 is slow." {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestProjectionMarkdownPassthrough(t *testing.T) {
	p := write(t, "a.md", "# Title\n\nbody **bold**")
	got, err := Projection(p)
	if err != nil || got != "# Title\n\nbody **bold**" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestProjectionHTMLStripsToText(t *testing.T) {
	p := write(t, "a.html", "<html><head><style>x{}</style></head><body><p>Hello <b>world</b></p><script>ignore()</script></body></html>")
	got, err := Projection(p)
	if err != nil || got != "Hello world" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestExcerpt(t *testing.T) {
	full := "Intro paragraph. " + strings.Repeat("x ", 400) +
		"The quick brown fox jumps. " + strings.Repeat("y ", 400) + "End."
	got := Excerpt(full, "quick   brown\nfox")
	if !strings.Contains(got, "quick brown fox") {
		t.Fatalf("excerpt should contain the needle window, got: %q", got)
	}
	if len(got) > 800 {
		t.Fatalf("excerpt too long: %d", len(got))
	}
	if Excerpt(full, "") != "" {
		t.Fatalf("empty needle must yield empty excerpt")
	}
	if Excerpt(full, "not-present-anywhere") != "" {
		t.Fatalf("missing needle must yield empty excerpt")
	}

	// Multi-byte runes on both sides of the needle: a raw byte-offset window can
	// land mid-rune, so the result must be snapped to rune boundaries. The
	// "é€€" unit is 8 bytes and window/2 (300) ≡ 4 mod 8, so start/end fall on a
	// continuation byte here — producing invalid UTF-8 without the boundary snap.
	utf8Full := strings.Repeat("é€€", 100) + "quick brown fox" + strings.Repeat("é€€", 100)
	utf8Got := Excerpt(utf8Full, "quick brown fox")
	if !utf8.ValidString(utf8Got) {
		t.Fatalf("excerpt must be valid UTF-8, got: %q", utf8Got)
	}
	if !strings.Contains(utf8Got, "quick brown fox") {
		t.Fatalf("utf8 excerpt should contain the needle window, got: %q", utf8Got)
	}
}
