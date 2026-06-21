package source

import (
	"os"
	"path/filepath"
	"testing"
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
