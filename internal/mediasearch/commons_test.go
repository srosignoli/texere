package mediasearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const commonsFixture = `{
  "query": {
    "pages": [
      {
        "pageid": 1,
        "title": "File:Red Apple.jpg",
        "imageinfo": [{
          "thumburl": "https://upload.wikimedia.org/thumb/red-apple.jpg",
          "url": "https://upload.wikimedia.org/red-apple.jpg",
          "descriptionurl": "https://commons.wikimedia.org/wiki/File:Red_Apple.jpg",
          "extmetadata": { "LicenseShortName": { "value": "CC BY-SA 4.0" } }
        }]
      },
      {
        "pageid": 2,
        "title": "File:No Info.jpg"
      }
    ]
  }
}`

func TestCommonsSearchNormalizes(t *testing.T) {
	var gotQuery, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(commonsFixture))
	}))
	defer srv.Close()

	got, err := Commons{BaseURL: srv.URL}.Search(context.Background(), "apple", "image", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The second page has no imageinfo and must be skipped, not panic.
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	want := Result{
		Kind:       "image",
		ThumbURL:   "https://upload.wikimedia.org/thumb/red-apple.jpg",
		FullURL:    "https://upload.wikimedia.org/red-apple.jpg",
		Title:      "Red Apple.jpg",
		SourcePage: "https://commons.wikimedia.org/wiki/File:Red_Apple.jpg",
		License:    "CC BY-SA 4.0",
	}
	if got[0] != want {
		t.Errorf("result =\n %+v\nwant\n %+v", got[0], want)
	}
	for _, sub := range []string{"gsrsearch=apple", "gsrnamespace=6", "gsrlimit=24", "formatversion=2"} {
		if !strings.Contains(gotQuery, sub) {
			t.Errorf("query %q missing %q", gotQuery, sub)
		}
	}
	if !strings.Contains(gotUA, "texere") {
		t.Errorf("User-Agent = %q, want it to identify texere", gotUA)
	}
}

func TestCommonsSearchEmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"query":{"pages":[]}}`))
	}))
	defer srv.Close()

	got, err := Commons{BaseURL: srv.URL}.Search(context.Background(), "zzzz", "image", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d results, want 0", len(got))
	}
}

func TestCommonsSearchServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := (Commons{BaseURL: srv.URL}).Search(context.Background(), "apple", "image", 24); err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestCommonsRejectsUnsupportedKind(t *testing.T) {
	if _, err := (Commons{BaseURL: "http://unused"}).Search(context.Background(), "apple", "audio", 24); err == nil {
		t.Fatal("expected error for kind=audio, got nil")
	}
}
