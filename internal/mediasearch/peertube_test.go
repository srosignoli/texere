package mediasearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const peertubeFixture = `{
  "total": 105,
  "data": [
    { "uuid": "81840642-e10d-4a74-9c02-621061fccbc1",
      "name": "Can We Hack Photosynthesis to Feed the World?",
      "duration": 671,
      "thumbnailPath": "/lazy-static/thumbnails/010965a4.jpg",
      "url": "https://diler.tube/videos/watch/81840642-e10d-4a74-9c02-621061fccbc1",
      "account": { "host": "diler.tube" } },
    { "uuid": "03ba40d1-5928-4c16-9f38-7472181b2b1f",
      "name": "Photosynthesis | The Dr. Binocs Show",
      "duration": 221,
      "thumbnailPath": "/lazy-static/thumbnails/abc.jpg",
      "url": "https://tube-sciences-technologies.apps.education.fr/videos/watch/03ba40d1",
      "account": { "host": "tube-sciences-technologies.apps.education.fr" } }
  ]
}`

func TestPeerTubeNormalizes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(peertubeFixture))
	}))
	defer srv.Close()

	got, err := PeerTube{BaseURL: srv.URL}.Search(context.Background(), "photosynthesis", "video", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	r := got[0]
	if r.Kind != "video" || r.Title != "Can We Hack Photosynthesis to Feed the World?" {
		t.Errorf("result = %+v", r)
	}
	if r.SourcePage != "https://diler.tube/videos/watch/81840642-e10d-4a74-9c02-621061fccbc1" {
		t.Errorf("sourcePage = %q, want the watch page", r.SourcePage)
	}
	// duration is SECONDS in this API, unlike Commons which reports a float.
	if r.DurationMs != 671000 {
		t.Errorf("durationMs = %d, want 671000", r.DurationMs)
	}
	// thumbnailPath is relative; left unjoined it would resolve against the
	// app's own origin and silently render nothing.
	if r.ThumbURL != "https://diler.tube/lazy-static/thumbnails/010965a4.jpg" {
		t.Errorf("thumbUrl = %q, want it joined with the instance host", r.ThumbURL)
	}
	if got[1].ThumbURL != "https://tube-sciences-technologies.apps.education.fr/lazy-static/thumbnails/abc.jpg" {
		t.Errorf("second thumbUrl = %q, want its own instance host", got[1].ThumbURL)
	}
	// FullURL is deliberately empty until ResolvePlayURL runs.
	if r.FullURL != "" {
		t.Errorf("fullUrl = %q, want empty before resolution", r.FullURL)
	}
	for _, sub := range []string{"search=photosynthesis", "count=24"} {
		if !strings.Contains(gotQuery, sub) {
			t.Errorf("query %q missing %q", gotQuery, sub)
		}
	}
}

func TestPeerTubeSkipsEntriesWithoutAHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"total":1,"data":[{"uuid":"x","name":"No host","duration":10,
		  "thumbnailPath":"/t.jpg","url":"https://example.org/videos/watch/x"}]}`))
	}))
	defer srv.Close()

	got, err := PeerTube{BaseURL: srv.URL}.Search(context.Background(), "x", "video", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	// No host to join against: leave the thumbnail empty rather than emitting a
	// relative URL the webview would resolve against its own origin.
	if got[0].ThumbURL != "" {
		t.Errorf("thumbUrl = %q, want empty when the host is unknown", got[0].ThumbURL)
	}
}

func TestPeerTubeMetadata(t *testing.T) {
	p := PeerTube{BaseURL: "http://unused"}
	if p.ID() != "peertube" {
		t.Errorf("ID = %q, want peertube", p.ID())
	}
	if len(p.Kinds()) != 1 || p.Kinds()[0] != "video" {
		t.Errorf("Kinds = %v, want [video]", p.Kinds())
	}
	if _, err := p.Search(context.Background(), "x", "audio", 24); err == nil {
		t.Error("expected an error for kind=audio")
	}
}
