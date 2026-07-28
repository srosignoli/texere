package mediasearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const commonsVideoFixture = `{
  "query": { "pages": [
    { "title": "File:NASA - Seeing Photosynthesis from Space.webm",
      "imageinfo": [{
        "url": "https://upload.wikimedia.org/wikipedia/commons/1/12/NASA.webm",
        "descriptionurl": "https://commons.wikimedia.org/wiki/File:NASA.webm",
        "thumburl": "https://upload.wikimedia.org/wikipedia/commons/thumb/1/12/NASA.webm/240px.jpg",
        "mime": "video/webm", "duration": 174.0 }] },
    { "title": "File:No Info.webm" }
  ] }
}`

func TestCommonsVideoNormalizes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(commonsVideoFixture))
	}))
	defer srv.Close()

	got, err := CommonsVideo{BaseURL: srv.URL}.Search(context.Background(), "photosynthesis", "video", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The second page has no imageinfo and must be skipped, not panic.
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	r := got[0]
	if r.Kind != "video" {
		t.Errorf("kind = %q, want video", r.Kind)
	}
	if r.Title != "NASA - Seeing Photosynthesis from Space.webm" {
		t.Errorf("title = %q", r.Title)
	}
	if r.FullURL != "https://upload.wikimedia.org/wikipedia/commons/1/12/NASA.webm" {
		t.Errorf("fullUrl = %q", r.FullURL)
	}
	if r.SourcePage != "https://commons.wikimedia.org/wiki/File:NASA.webm" {
		t.Errorf("sourcePage = %q", r.SourcePage)
	}
	if r.ThumbURL == "" {
		t.Error("thumbUrl is empty; the results grid shows tiles and needs it")
	}
	if r.DurationMs != 174000 {
		t.Errorf("durationMs = %d, want 174000", r.DurationMs)
	}
	if !strings.Contains(gotQuery, "filetype%3Avideo") {
		t.Errorf("query %q should restrict to video", gotQuery)
	}
	if !strings.Contains(gotQuery, "iiurlwidth=") {
		t.Errorf("query %q must ask for a thumbnail width, or thumburl is absent", gotQuery)
	}
}

// Commons already knows the play URL, so resolving is a no-op. It implements
// the interface anyway so the call site does not special-case it.
func TestCommonsVideoResolveIsIdentity(t *testing.T) {
	in := Result{FullURL: "https://upload.wikimedia.org/x.webm"}
	got, err := CommonsVideo{BaseURL: "http://unused"}.ResolvePlayURL(context.Background(), in)
	if err != nil {
		t.Fatalf("ResolvePlayURL: %v", err)
	}
	if got != in.FullURL {
		t.Errorf("got %q, want the unchanged FullURL", got)
	}
}

func TestCommonsVideoMetadata(t *testing.T) {
	c := CommonsVideo{BaseURL: "http://unused"}
	if c.ID() != "commons-video" {
		t.Errorf("ID = %q, want commons-video (persisted; buildVideoURL branches on it)", c.ID())
	}
	if len(c.Kinds()) != 1 || c.Kinds()[0] != "video" {
		t.Errorf("Kinds = %v, want [video]", c.Kinds())
	}
	if _, err := c.Search(context.Background(), "x", "image", 24); err == nil {
		t.Error("expected an error for kind=image")
	}
}
