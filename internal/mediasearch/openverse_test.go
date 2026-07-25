package mediasearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const openverseFixture = `{
  "result_count": 2,
  "results": [
    {
      "title": "Green apple",
      "url": "https://live.staticflickr.com/green-apple.jpg",
      "thumbnail": "https://api.openverse.org/v1/images/abc/thumb/",
      "foreign_landing_url": "https://www.flickr.com/photos/x/123",
      "license": "by-sa",
      "license_version": "4.0"
    },
    {
      "title": "Apple, no licence version",
      "url": "https://example.org/apple2.jpg",
      "thumbnail": "https://example.org/apple2-thumb.jpg",
      "foreign_landing_url": "https://example.org/page",
      "license": "cc0",
      "license_version": ""
    }
  ]
}`

func TestOpenverseSearchNormalizes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(openverseFixture))
	}))
	defer srv.Close()

	got, err := Openverse{BaseURL: srv.URL}.Search(context.Background(), "apple", "image", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	want := Result{
		Kind:       "image",
		ThumbURL:   "https://api.openverse.org/v1/images/abc/thumb/",
		FullURL:    "https://live.staticflickr.com/green-apple.jpg",
		Title:      "Green apple",
		SourcePage: "https://www.flickr.com/photos/x/123",
		License:    "CC BY-SA 4.0",
	}
	if got[0] != want {
		t.Errorf("result =\n %+v\nwant\n %+v", got[0], want)
	}
	// Missing license_version must not leave a trailing space.
	if got[1].License != "CC CC0" {
		t.Errorf("license = %q, want %q", got[1].License, "CC CC0")
	}
	for _, sub := range []string{"q=apple", "page_size=24"} {
		if !strings.Contains(gotQuery, sub) {
			t.Errorf("query %q missing %q", gotQuery, sub)
		}
	}
}

func TestOpenverseRejectsUnsupportedKind(t *testing.T) {
	if _, err := (Openverse{BaseURL: "http://unused"}).Search(context.Background(), "a", "video", 24); err == nil {
		t.Fatal("expected error for kind=video, got nil")
	}
}
