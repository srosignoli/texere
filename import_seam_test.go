package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"texere/internal/content"
	"texere/internal/learning"
)

// seamFixture builds an empty collection plus an App wired to it, mirroring the
// setup TestImportWebAndCreateExtractPosition uses for the pathless web import.
func seamFixture(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	a := &App{collectionDir: dir, learnStore: store,
		registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}}
	return a, dir
}

// The ingest half of ImportTextSource must work from a caller-supplied path, so
// the dialog is the only thing the exported method adds.
func TestImportTextFromIngestsAPathWithoutADialog(t *testing.T) {
	a, dir := seamFixture(t)

	outside := filepath.Join(t.TempDir(), "essay.md")
	if err := os.WriteFile(outside, []byte("# Essay\n\nSome body text."), 0o644); err != nil {
		t.Fatal(err)
	}

	rv, err := a.importTextFrom(outside)
	if err != nil || rv == nil {
		t.Fatalf("importTextFrom rv=%v err=%v", rv, err)
	}
	if rv.Title != "essay" {
		t.Fatalf("title = %q, want %q", rv.Title, "essay")
	}
	// The source is copied into the collection, not merely referenced.
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rv.RefPath))); err != nil {
		t.Fatalf("source not copied into collection: %v", err)
	}
	// Notes are scaffolded and point back at the copied source.
	notes, err := os.ReadFile(rv.NotesPath)
	if err != nil {
		t.Fatalf("notes not scaffolded: %v", err)
	}
	if !strings.Contains(string(notes), rv.RefPath) {
		t.Fatalf("notes do not reference %q:\n%s", rv.RefPath, notes)
	}
	// The reading is registered in the store.
	if _, ok, err := a.learnStore.GetReading(rv.ID); err != nil || !ok {
		t.Fatalf("reading %q not registered: ok=%v err=%v", rv.ID, ok, err)
	}
}

// An empty path is a cancelled dialog; the ingest helper must reject it rather
// than scaffolding an empty-named reading.
func TestImportTextFromRejectsAnEmptyPath(t *testing.T) {
	a, _ := seamFixture(t)
	if _, err := a.importTextFrom(""); err == nil {
		t.Fatal("importTextFrom(\"\") = nil error, want an error")
	}
}

// The ingest half of ImportPdfSource must work from a caller-supplied path.
func TestImportPdfFromIngestsAPathWithoutADialog(t *testing.T) {
	a, dir := seamFixture(t)

	outside := filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(outside, []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rv, err := a.importPdfFrom(outside)
	if err != nil || rv == nil {
		t.Fatalf("importPdfFrom rv=%v err=%v", rv, err)
	}
	if rv.Medium != "pdf" {
		t.Fatalf("medium = %q, want %q", rv.Medium, "pdf")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rv.RefPath))); err != nil {
		t.Fatalf("pdf not copied into collection: %v", err)
	}
	if _, ok, err := a.learnStore.GetReading(rv.ID); err != nil || !ok {
		t.Fatalf("reading %q not registered: ok=%v err=%v", rv.ID, ok, err)
	}
}

func TestImportPdfFromRejectsAnEmptyPath(t *testing.T) {
	a, _ := seamFixture(t)
	if _, err := a.importPdfFrom(""); err == nil {
		t.Fatal("importPdfFrom(\"\") = nil error, want an error")
	}
}

// writePNG writes a real 1x1 PNG, since media.Import sniffs the MIME type.
func writePNG(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
}

func TestImportImageFromIngestsAPathWithoutADialog(t *testing.T) {
	a, dir := seamFixture(t)

	outside := filepath.Join(t.TempDir(), "diagram.png")
	writePNG(t, outside)

	got, err := a.importImageFrom(outside)
	if err != nil || got == nil {
		t.Fatalf("importImageFrom got=%v err=%v", got, err)
	}
	if got["mime"] != "image/png" {
		t.Fatalf("mime = %v, want image/png", got["mime"])
	}
	asset, _ := got["asset"].(string)
	if asset == "" {
		t.Fatal("asset name empty")
	}
	if _, err := os.Stat(filepath.Join(dir, "media", asset)); err != nil {
		t.Fatalf("image not stored under media/: %v", err)
	}
}

func TestImportImageFromRejectsANonImage(t *testing.T) {
	a, _ := seamFixture(t)
	outside := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(outside, []byte("plain text, not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.importImageFrom(outside); err == nil {
		t.Fatal("importImageFrom(text file) = nil error, want rejection")
	}
}

func TestImportAudioFromIngestsAPathWithoutADialog(t *testing.T) {
	a, dir := seamFixture(t)

	outside := filepath.Join(t.TempDir(), "lecture.mp3")
	if err := os.WriteFile(outside, []byte("ID3 fake audio payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := a.importAudioFrom(outside)
	if err != nil || got == nil {
		t.Fatalf("importAudioFrom got=%v err=%v", got, err)
	}
	asset, _ := got["asset"].(string)
	if asset == "" {
		t.Fatal("asset name empty")
	}
	if _, err := os.Stat(filepath.Join(dir, "media", asset)); err != nil {
		t.Fatalf("audio not stored under media/: %v", err)
	}
}

func TestImportAudioFromRejectsAnUnsupportedExtension(t *testing.T) {
	a, _ := seamFixture(t)
	outside := filepath.Join(t.TempDir(), "clip.xyz")
	if err := os.WriteFile(outside, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.importAudioFrom(outside); err == nil {
		t.Fatal("importAudioFrom(.xyz) = nil error, want rejection")
	}
}
