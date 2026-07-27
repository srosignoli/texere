package media

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchRemoteStoresAndDedups(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	col := t.TempDir()
	a, err := FetchRemote(col, srv.URL+"/apple.png", "image")
	if err != nil {
		t.Fatalf("FetchRemote: %v", err)
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
	// Same bytes from a different URL → same content-addressed name, one file.
	b, err := FetchRemote(col, srv.URL+"/other-name.png", "image")
	if err != nil || b.Name != a.Name {
		t.Fatalf("dedup failed: %v name=%q vs %q", err, b.Name, a.Name)
	}
	entries, _ := os.ReadDir(filepath.Join(col, "media"))
	if len(entries) != 1 {
		t.Errorf("media dir has %d files, want 1", len(entries))
	}
}

func TestFetchRemoteSendsUserAgent(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("User-Agent")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	if _, err := FetchRemote(t.TempDir(), srv.URL, "image"); err != nil {
		t.Fatalf("FetchRemote: %v", err)
	}
	ua := <-got
	if !strings.Contains(ua, "texere") {
		t.Errorf("User-Agent = %q, want it to identify texere", ua)
	}
}

func TestFetchRemoteRejectsOversize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 2048))
	}))
	defer srv.Close()

	old := remoteMaxBytes
	remoteMaxBytes = 1024
	defer func() { remoteMaxBytes = old }()

	_, err := FetchRemote(t.TempDir(), srv.URL, "image")
	if err == nil {
		t.Fatal("expected error for oversize body, got nil")
	}
	if !strings.Contains(err.Error(), "larger than") {
		t.Errorf("error = %q, want it to mention the size cap", err)
	}
}

func TestFetchRemoteRejectsNonImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png") // lying header; sniffing must win
		_, _ = w.Write([]byte("this is plain text, not an image at all"))
	}))
	defer srv.Close()

	if _, err := FetchRemote(t.TempDir(), srv.URL, "image"); err == nil {
		t.Fatal("expected error for non-image body, got nil")
	}
}

func TestFetchRemoteRejectsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := FetchRemote(t.TempDir(), srv.URL, "image")
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want it to name the status code", err)
	}
}

// A minimal RIFF/WAVE header — enough that it is unmistakably not an image.
var wavBytes = append([]byte("RIFF$\x00\x00\x00WAVEfmt "), make([]byte, 32)...)

// Commons serves Lingua Libre .wav as Content-Type: audio/x-wav, which is not
// in audioMimeByExt. Validating on the header would reject every pronunciation
// file, so the URL's extension decides.
func TestFetchRemoteAudioIgnoresContentTypeHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-wav")
		_, _ = w.Write(wavBytes)
	}))
	defer srv.Close()

	col := t.TempDir()
	a, err := FetchRemote(col, srv.URL+"/LL-Q1321%20(spa)-Rdrg109-manzana.wav", "audio")
	if err != nil {
		t.Fatalf("FetchRemote: %v", err)
	}
	if a.Mime != "audio/wav" {
		t.Errorf("mime = %q, want audio/wav (from the extension, not the header)", a.Mime)
	}
	if !strings.HasSuffix(a.Name, ".wav") {
		t.Errorf("name = %q, want a .wav", a.Name)
	}
	if _, err := os.Stat(filepath.Join(col, "media", a.Name)); err != nil {
		t.Errorf("asset not written: %v", err)
	}
}

func TestFetchRemoteAudioRejectsUnknownExtension(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(wavBytes)
	}))
	defer srv.Close()

	if _, err := FetchRemote(t.TempDir(), srv.URL+"/clip.xyz", "audio"); err == nil {
		t.Fatal("expected an error for an unsupported audio extension")
	}
}

func TestFetchRemoteAudioUsesItsOwnSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	old := audioMaxBytes
	audioMaxBytes = 1024
	defer func() { audioMaxBytes = old }()

	_, err := FetchRemote(t.TempDir(), srv.URL+"/clip.wav", "audio")
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v, want a size-cap error", err)
	}
}

func TestFetchRemoteRejectsUnknownKind(t *testing.T) {
	if _, err := FetchRemote(t.TempDir(), "http://example.invalid/x.wav", "video"); err == nil {
		t.Fatal("expected an error for an unsupported kind")
	}
}
