package media

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 1x1 PNG.
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestImportHashesAndDedups(t *testing.T) {
	col := t.TempDir()
	src := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(src, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Import(col, src)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if a.Mime != "image/png" {
		t.Errorf("mime = %q, want image/png", a.Mime)
	}
	if !strings.HasSuffix(a.Name, ".png") || len(a.Name) != 64+4 {
		t.Errorf("name = %q, want <64 hex>.png", a.Name)
	}
	if _, err := os.Stat(filepath.Join(col, "media", a.Name)); err != nil {
		t.Errorf("file not written: %v", err)
	}
	// Re-import identical bytes → same name, still one file.
	b, err := Import(col, src)
	if err != nil || b.Name != a.Name {
		t.Fatalf("dedup failed: %v name=%q vs %q", err, b.Name, a.Name)
	}
	entries, _ := os.ReadDir(filepath.Join(col, "media"))
	if len(entries) != 1 {
		t.Errorf("media dir has %d files, want 1", len(entries))
	}
}

func TestImportRejectsNonImage(t *testing.T) {
	col := t.TempDir()
	src := filepath.Join(t.TempDir(), "note.txt")
	_ = os.WriteFile(src, []byte("this is plain text, not an image"), 0o644)
	if _, err := Import(col, src); err == nil {
		t.Fatal("expected error for non-image, got nil")
	}
}

func TestFileServerServesAndBlocksTraversal(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "ok.png"), pngBytes, 0o644)
	h := FileServer(func() string { return dir })

	// Serves a real file.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/media/ok.png", nil))
	if rr.Code != http.StatusOK || rr.Body.Len() != len(pngBytes) {
		t.Errorf("serve: code=%d len=%d", rr.Code, rr.Body.Len())
	}
	// Traversal collapses to a base name and does not escape.
	for _, p := range []string{"/media/../server.go", "/media/..%2f..%2fx", "/other/x"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code == http.StatusOK {
			t.Errorf("path %q unexpectedly served (code %d)", p, rr.Code)
		}
	}
	// No collection open → 404.
	h2 := FileServer(func() string { return "" })
	rr2 := httptest.NewRecorder()
	h2.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/media/ok.png", nil))
	if rr2.Code != http.StatusNotFound {
		t.Errorf("empty dir: code=%d, want 404", rr2.Code)
	}
}

func TestImportAudioByExtension(t *testing.T) {
	col := t.TempDir()
	src := filepath.Join(t.TempDir(), "memo.MP3") // mixed case
	if err := os.WriteFile(src, []byte("not really mp3 but ext decides"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := ImportAudio(col, src)
	if err != nil {
		t.Fatalf("ImportAudio: %v", err)
	}
	if a.Mime != "audio/mpeg" || !strings.HasSuffix(a.Name, ".mp3") {
		t.Errorf("got name=%q mime=%q", a.Name, a.Mime)
	}
	if _, err := os.Stat(filepath.Join(col, "media", a.Name)); err != nil {
		t.Errorf("not written: %v", err)
	}
	// dedup
	b, _ := ImportAudio(col, src)
	if b.Name != a.Name {
		t.Errorf("dedup failed: %q vs %q", b.Name, a.Name)
	}
}

func TestImportAudioRejectsNonAudioExt(t *testing.T) {
	col := t.TempDir()
	src := filepath.Join(t.TempDir(), "note.txt")
	_ = os.WriteFile(src, []byte("x"), 0o644)
	if _, err := ImportAudio(col, src); err == nil {
		t.Fatal("expected rejection for .txt")
	}
}

func TestSaveAudioStripsCodecs(t *testing.T) {
	col := t.TempDir()
	a, err := SaveAudio(col, []byte("opusbytes"), "audio/webm;codecs=opus")
	if err != nil {
		t.Fatalf("SaveAudio: %v", err)
	}
	if a.Mime != "audio/webm" || !strings.HasSuffix(a.Name, ".webm") {
		t.Errorf("got name=%q mime=%q", a.Name, a.Mime)
	}
}

func TestSaveAudioRejectsUnknownMime(t *testing.T) {
	col := t.TempDir()
	if _, err := SaveAudio(col, []byte("x"), "audio/weird"); err == nil {
		t.Fatal("expected rejection for audio/weird")
	}
}

func TestCollectionServerServesNestedPaths(t *testing.T) {
	dir := t.TempDir()
	// Both folders matter: conflating "media/" with some blessed assets folder
	// is the bug that prompted this route.
	for _, rel := range []string{"media/a.png", "assets/b.png", "deep/nest/c.png"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), pngBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := CollectionServer(func() string { return dir })

	for _, rel := range []string{"media/a.png", "assets/b.png", "deep/nest/c.png"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/collection/"+rel, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("%s: code=%d, want 200", rel, rr.Code)
		}
		if rr.Body.Len() != len(pngBytes) {
			t.Errorf("%s: body=%d bytes, want %d", rel, rr.Body.Len(), len(pngBytes))
		}
		if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("%s: Content-Type=%q, want image/png — a webview needs a usable type", rel, ct)
		}
	}
}

// This tightens behaviour deliberately: GetSourceBytes cleans ".." and reads
// outside the collection. This route must not.
func TestCollectionServerBlocksEscapes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media", "in.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	// A real file one level ABOVE the collection: the thing an escape would reach.
	outside := filepath.Join(filepath.Dir(dir), "escaped-"+filepath.Base(dir)+".png")
	if err := os.WriteFile(outside, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(outside) })

	h := CollectionServer(func() string { return dir })
	for _, p := range []string{
		"/collection/../" + filepath.Base(outside),
		"/collection/media/../../" + filepath.Base(outside),
		"/collection/..%2f" + filepath.Base(outside),
		"/other/media/in.png",
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code == http.StatusOK {
			t.Errorf("path %q was served (code %d) — it escapes the collection", p, rr.Code)
		}
	}
	// The control: the same handler DOES serve something legitimate, so a
	// blanket 404 cannot be mistaken for containment working.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/collection/media/in.png", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("in-collection file not served (code %d) — the fixture proves nothing", rr.Code)
	}
}

func TestCollectionServerWithNoCollectionOpen(t *testing.T) {
	h := CollectionServer(func() string { return "" })
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/collection/media/a.png", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("code=%d, want 404", rr.Code)
	}
}

// /media/ only ever holds imported audio, so it labels .mp4 and .webm audio/*.
// /collection/ serves whatever a note references, and both are VIDEO there — an
// audio Content-Type on a <video> body is wrong on every file of that kind.
func TestCollectionServerTypesVideoAsVideo(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"clip.mp4", "clip.webm", "voice.mp3", "voice.wav"} {
		if err := os.WriteFile(filepath.Join(dir, name), pngBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := CollectionServer(func() string { return dir })
	for name, want := range map[string]string{
		"clip.mp4":  "video/mp4",
		"clip.webm": "video/webm",
		"voice.mp3": "audio/mpeg",
		"voice.wav": "audio/wav",
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/collection/"+name, nil))
		if got := rr.Header().Get("Content-Type"); got != want {
			t.Errorf("%s: Content-Type=%q, want %q", name, got, want)
		}
	}
}
