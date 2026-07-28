package mediasearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const archiveSearchFixture = `{
  "response": { "numFound": 256, "docs": [
    { "identifier": "Photosynthesis", "title": "Photosynthesis" },
    { "identifier": "ucberkeley_bio1al_3", "title": "Biology 1AL - Lecture 3" }
  ] }
}`

func TestInternetArchiveNormalizes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(archiveSearchFixture))
	}))
	defer srv.Close()

	got, err := InternetArchive{BaseURL: srv.URL}.Search(context.Background(), "photosynthesis", "video", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	r := got[0]
	if r.Kind != "video" || r.Title != "Photosynthesis" {
		t.Errorf("result = %+v", r)
	}
	if r.SourcePage != "https://archive.org/details/Photosynthesis" {
		t.Errorf("sourcePage = %q, want the details page", r.SourcePage)
	}
	if r.ThumbURL != "https://archive.org/services/img/Photosynthesis" {
		t.Errorf("thumbUrl = %q", r.ThumbURL)
	}
	if r.FullURL != "" {
		t.Errorf("fullUrl = %q, want empty before resolution", r.FullURL)
	}
	// Without this exclusion the TV News Archive drowns everything: verified
	// live at 418 unfiltered hits, all news captures, versus 256 with it.
	if !strings.Contains(gotQuery, "NOT+collection%3Atvarchive") &&
		!strings.Contains(gotQuery, "NOT%20collection%3Atvarchive") {
		t.Errorf("query %q must exclude the TV News Archive", gotQuery)
	}
	if !strings.Contains(gotQuery, "mediatype%3Amovies") {
		t.Errorf("query %q must restrict to movies", gotQuery)
	}
}

func TestInternetArchiveResolvePicksAPlayableFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"files":[
		  {"name":"scan.pdf","format":"Text PDF"},
		  {"name":"clip.mp4","format":"h.264"},
		  {"name":"clip.webm","format":"WebM"}
		]}`))
	}))
	defer srv.Close()

	ia := InternetArchive{BaseURL: "http://unused", MetadataBase: srv.URL}
	got, err := ia.ResolvePlayURL(context.Background(), Result{
		SourcePage: "https://archive.org/details/youtube-Q5K60cL9VcY",
	})
	if err != nil {
		t.Fatalf("ResolvePlayURL: %v", err)
	}
	if got != "https://archive.org/download/youtube-Q5K60cL9VcY/clip.mp4" {
		t.Errorf("got %q, want the h.264 download URL", got)
	}
}

func TestInternetArchiveResolveReportsNoPlayableFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"files":[{"name":"scan.pdf","format":"Text PDF"}]}`))
	}))
	defer srv.Close()

	ia := InternetArchive{BaseURL: "http://unused", MetadataBase: srv.URL}
	_, err := ia.ResolvePlayURL(context.Background(), Result{
		SourcePage: "https://archive.org/details/x",
	})
	if err == nil || !strings.Contains(err.Error(), "no playable file") {
		t.Fatalf("err = %v, want a no-playable-file error", err)
	}
}

func TestInternetArchiveMetadata(t *testing.T) {
	ia := InternetArchive{BaseURL: "http://unused"}
	if ia.ID() != "archive" {
		t.Errorf("ID = %q, want archive", ia.ID())
	}
	if len(ia.Kinds()) != 1 || ia.Kinds()[0] != "video" {
		t.Errorf("Kinds = %v, want [video]", ia.Kinds())
	}
	if _, err := ia.Search(context.Background(), "x", "image", 24); err == nil {
		t.Error("expected an error for kind=image")
	}
}
