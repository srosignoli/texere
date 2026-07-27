package mediasearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Shaped like a real formatversion=2 response for `LL-Q1860 pie`, including the
// compounds that genuinely outrank the bare word.
const pieFixture = `{
  "query": { "pages": [
    { "title": "File:LL-Q1860 (eng)-I learned some phrases-pie-eyed.wav",
      "imageinfo": [{ "url": "https://upload.wikimedia.org/pie-eyed.wav",
                      "descriptionurl": "https://commons.wikimedia.org/wiki/File:pie-eyed.wav",
                      "mime": "audio/wav", "duration": 1.5 }] },
    { "title": "File:LL-Q1860 (eng)-Neøn-pie chart.wav",
      "imageinfo": [{ "url": "https://upload.wikimedia.org/pie-chart.wav",
                      "descriptionurl": "https://commons.wikimedia.org/wiki/File:pie-chart.wav",
                      "mime": "audio/wav", "duration": 1.1 }] },
    { "title": "File:LL-Q1860 (eng)-ManjiroLL-pie.wav",
      "imageinfo": [{ "url": "https://upload.wikimedia.org/pie.wav",
                      "descriptionurl": "https://commons.wikimedia.org/wiki/File:pie.wav",
                      "mime": "audio/wav", "duration": 0.9 }] }
  ] }
}`

func TestPronunciationFiltersToTheExactWord(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(pieFixture))
	}))
	defer srv.Close()

	got, err := Pronunciation{BaseURL: srv.URL}.Search(context.Background(), "English|pie", "audio", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want only the bare word: %+v", len(got), got)
	}
	r := got[0]
	if r.Kind != "audio" || r.Title != "pie" || r.Lang != "eng" || r.Speaker != "ManjiroLL" {
		t.Errorf("result = %+v, want audio/pie/eng/ManjiroLL", r)
	}
	if r.DurationMs != 900 {
		t.Errorf("durationMs = %d, want 900", r.DurationMs)
	}
	if r.Mime != "audio/wav" {
		t.Errorf("mime = %q, want audio/wav", r.Mime)
	}
	if r.FullURL != "https://upload.wikimedia.org/pie.wav" {
		t.Errorf("fullUrl = %q", r.FullURL)
	}
	if !strings.Contains(gotQuery, "LL-Q1860+pie") && !strings.Contains(gotQuery, "LL-Q1860%20pie") {
		t.Errorf("query %q should search LL-Q1860 pie", gotQuery)
	}
	if !strings.Contains(gotQuery, "gsrnamespace=6") {
		t.Errorf("query %q should restrict to the File namespace", gotQuery)
	}
	// Without this the fallback path returns any file in the File namespace —
	// PDFs and images included — which render as playable rows that then fail
	// to download.
	if !strings.Contains(gotQuery, "filetype%3Aaudio") {
		t.Errorf("query %q should restrict to audio", gotQuery)
	}
}

// When nothing matches exactly the unfiltered set is returned, so a word with no
// exact recording still shows near-misses instead of an empty grid.
func TestPronunciationFallsBackWhenNothingMatchesExactly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(pieFixture))
	}))
	defer srv.Close()

	got, err := Pronunciation{BaseURL: srv.URL}.Search(context.Background(), "English|zzzz", "audio", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want all 3 as fallback", len(got))
	}
	// No speaker is guessed on a non-exact name.
	if got[0].Speaker != "" {
		t.Errorf("speaker = %q, want empty on a fallback row", got[0].Speaker)
	}
}

func TestPronunciationAcceptsRawQCode(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"query":{"pages":[]}}`))
	}))
	defer srv.Close()

	if _, err := (Pronunciation{BaseURL: srv.URL}).Search(context.Background(), "Q1321|manzana", "audio", 24); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(gotQuery, "LL-Q1321") {
		t.Errorf("query %q should use the raw Q-code", gotQuery)
	}
}

func TestPronunciationRejectsBadInput(t *testing.T) {
	p := Pronunciation{BaseURL: "http://unused"}
	if _, err := p.Search(context.Background(), "English|apple", "image", 24); err == nil {
		t.Error("expected an error for kind=image")
	}
	if _, err := p.Search(context.Background(), "manzana", "audio", 24); err == nil {
		t.Error("expected an error for a query with no language separator")
	}
	if _, err := p.Search(context.Background(), "Klingon|apple", "audio", 24); err == nil {
		t.Error("expected an error for an unknown language")
	}
}

func TestPronunciationMetadata(t *testing.T) {
	p := Pronunciation{BaseURL: "http://unused"}
	if p.ID() != "pronunciation" {
		t.Errorf("ID = %q, want pronunciation (persisted on cells and used to pick the N4L arrow)", p.ID())
	}
	if len(p.Kinds()) != 1 || p.Kinds()[0] != "audio" {
		t.Errorf("Kinds = %v, want [audio]", p.Kinds())
	}
	if p.Name() == "" {
		t.Error("Name must not be empty")
	}
}
