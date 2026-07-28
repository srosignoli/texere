package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"texere/internal/mediasearch"
)

func TestListMediaProvidersReturnsBoth(t *testing.T) {
	got := (&App{}).ListMediaProviders()
	if len(got) != 7 {
		t.Fatalf("got %d providers, want 7", len(got))
	}
	if got[0].ID == "" || got[0].Name == "" || len(got[0].Kinds) == 0 {
		t.Errorf("provider info incomplete: %+v", got[0])
	}
}

func TestResolveVideoPlayURLUnknownProvider(t *testing.T) {
	if _, err := (&App{}).ResolveVideoPlayURL("nope", mediasearch.Result{}); err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}

// An image provider cannot resolve a play URL; asking must fail loudly rather
// than returning an empty string the frontend would treat as a valid URL.
func TestResolveVideoPlayURLNonVideoProvider(t *testing.T) {
	if _, err := (&App{}).ResolveVideoPlayURL("commons", mediasearch.Result{}); err == nil {
		t.Fatal("expected an error for a provider that is not a VideoResolver")
	}
}

func TestSearchMediaUnknownProvider(t *testing.T) {
	_, err := (&App{}).SearchMedia("nope", "apple", "image", 24)
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestListLanguagesIsPopulated(t *testing.T) {
	got := (&App{}).ListLanguages()
	if len(got) < 25 {
		t.Fatalf("got %d languages, want at least 25", len(got))
	}
	if got[0].Name == "" || got[0].Code == "" {
		t.Errorf("incomplete first entry: %+v", got[0])
	}
}

func TestFetchMediaURLRequiresCollection(t *testing.T) {
	_, err := (&App{}).FetchMediaURL("https://example.org/a.png", "image")
	if err == nil {
		t.Fatal("expected error when no collection is open, got nil")
	}
}

// 1x1 PNG, same bytes as internal/media's test fixture.
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestFetchMediaURLStoresAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	got, err := (&App{collectionDir: dir}).FetchMediaURL(srv.URL+"/a.png", "image")
	if err != nil {
		t.Fatalf("FetchMediaURL: %v", err)
	}
	name, _ := got["asset"].(string)
	if name == "" {
		t.Fatalf("no asset in %+v", got)
	}
	if got["mime"] != "image/png" {
		t.Errorf("mime = %v, want image/png", got["mime"])
	}
	if _, err := os.Stat(filepath.Join(dir, "media", name)); err != nil {
		t.Errorf("asset not on disk: %v", err)
	}
}

func TestFetchMediaURLPassesKindThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(append([]byte("RIFF$\x00\x00\x00WAVEfmt "), make([]byte, 32)...))
	}))
	defer srv.Close()

	dir := t.TempDir()
	got, err := (&App{collectionDir: dir}).FetchMediaURL(srv.URL+"/manzana.wav", "audio")
	if err != nil {
		t.Fatalf("FetchMediaURL: %v", err)
	}
	if got["mime"] != "audio/wav" {
		t.Errorf("mime = %v, want audio/wav", got["mime"])
	}
}
