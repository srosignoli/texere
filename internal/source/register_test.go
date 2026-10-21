package source

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/learning"
)

// collection is a temporary Collection with a real learning Store.
func collection(t *testing.T) (string, *learning.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return dir, st
}

// picked writes a file outside the Collection, as a file dialog would return.
func picked(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRegisterCopiesAPickedTextFile(t *testing.T) {
	dir, st := collection(t)
	from := picked(t, "k.txt", "some text")

	r, err := Register(dir, st, Import{Medium: "text", FileName: "k.txt", CopyFrom: from})
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "sources", "k.txt")); err != nil || string(b) != "some text" {
		t.Errorf("sources/k.txt = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "reading", "k.n4l")); err != nil {
		t.Errorf("reading/k.n4l missing: %v", err)
	}
	want := learning.ReadingElement{ID: r.ID, RefPath: "sources/k.txt", NotesPath: filepath.Join(dir, "reading", "k.n4l"),
		Title: "k", Position: "char=0", ProjLen: 9, IntervalDays: 1, Medium: "text"}
	if r != want {
		t.Errorf("Register = %+v\nwant       %+v", r, want)
	}
	if got, ok, err := st.GetReading(r.ID); err != nil || !ok || got.NotesPath != want.NotesPath {
		t.Errorf("stored row = %+v, ok=%v, %v", got, ok, err)
	}
}

// A PDF opens at page 1; its length is unknown until the pane reports it.
func TestRegisterTakesThePdfDefaults(t *testing.T) {
	dir, st := collection(t)
	r, err := Register(dir, st, Import{Medium: "pdf", FileName: "paper.pdf", CopyFrom: picked(t, "paper.pdf", "%PDF-1.4 binary")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Position != "page=1" || r.ProjLen != 0 || r.Medium != "pdf" {
		t.Errorf("Register = %+v, want Position page=1, ProjLen 0, medium pdf", r)
	}
}

// Generated content keeps its human title; the caller picks the file names.
func TestRegisterWritesGeneratedContent(t *testing.T) {
	dir, st := collection(t)
	md := "# Héllo\n\nSome notes."
	r, err := Register(dir, st, Import{Medium: "llm", Title: "My Notes", FileName: "my-notes.md", NotesName: "my-notes.n4l", Data: []byte(md)})
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "sources", "my-notes.md")); err != nil || string(b) != md {
		t.Errorf("sources/my-notes.md = %q, %v", b, err)
	}
	if r.Title != "My Notes" || r.RefPath != "sources/my-notes.md" || r.NotesPath != filepath.Join(dir, "reading", "my-notes.n4l") {
		t.Errorf("Register = %+v", r)
	}
	if want := len([]rune(md)); r.ProjLen != want {
		t.Errorf("ProjLen = %d, want %d (runes, not bytes)", r.ProjLen, want)
	}
}

// A web article is measured by the text a reader sees, not its markup.
func TestRegisterMeasuresAWebArticleByItsText(t *testing.T) {
	dir, st := collection(t)
	page := `<html><body><script>var x = 1;</script><p>Read me</p></body></html>`
	r, err := Register(dir, st, Import{Medium: "web", Title: "Article", FileName: "article.html", NotesName: "article.n4l",
		Data: []byte(page), SourceURL: "https://example.org/a"})
	if err != nil {
		t.Fatal(err)
	}
	if want := len([]rune(TextFromHTML(page))); r.ProjLen != want || want == len([]rune(page)) {
		t.Errorf("ProjLen = %d, want %d (the text, without markup)", r.ProjLen, want)
	}
	if r.SourceURL != "https://example.org/a" {
		t.Errorf("SourceURL = %q", r.SourceURL)
	}
}

// A second k.txt becomes k-2, and a picked file's title follows the name it was
// placed under.
func TestRegisterSuffixesCollidingNames(t *testing.T) {
	dir, st := collection(t)
	from := picked(t, "k.txt", "text")
	if _, err := Register(dir, st, Import{Medium: "text", FileName: "k.txt", CopyFrom: from}); err != nil {
		t.Fatal(err)
	}
	r, err := Register(dir, st, Import{Medium: "text", FileName: "k.txt", CopyFrom: from})
	if err != nil {
		t.Fatal(err)
	}
	if r.RefPath != "sources/k-2.txt" || r.NotesPath != filepath.Join(dir, "reading", "k-2.n4l") || r.Title != "k-2" {
		t.Errorf("second import = RefPath %q, NotesPath %q, Title %q; want sources/k-2.txt, reading/k-2.n4l, k-2", r.RefPath, r.NotesPath, r.Title)
	}
}

// files lists every file under dir/sub, or nothing when sub does not exist.
func files(t *testing.T, dir, sub string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, sub))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// A failed import leaves the Collection as it was: a notes file left in
// reading/ would be picked up by the graph and the linter as a stray note.
func TestAFailedRegisterLeavesNoFiles(t *testing.T) {
	dir, st := collection(t)
	st.Close() // storing the row will fail
	if _, err := Register(dir, st, Import{Medium: "text", FileName: "k.txt", CopyFrom: picked(t, "k.txt", "text")}); err == nil {
		t.Fatal("Register succeeded with a closed Store")
	}
	if got := files(t, dir, "sources"); len(got) != 0 {
		t.Errorf("sources/ holds %v after a failed import", got)
	}
	if got := files(t, dir, "reading"); len(got) != 0 {
		t.Errorf("reading/ holds %v after a failed import", got)
	}
}

// With neither Data nor CopyFrom there is nothing to import: refused before
// anything is written, rather than an empty Source.
func TestRegisterNeedsContent(t *testing.T) {
	dir, st := collection(t)
	if _, err := Register(dir, st, Import{Medium: "text", FileName: "k.txt"}); err == nil {
		t.Error("Register accepted an import with no content")
	}
	if got := files(t, dir, "sources"); len(got) != 0 {
		t.Errorf("sources/ holds %v after a refused import", got)
	}
}
