package mediasearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const commonsAudioFixture = `{
  "query": { "pages": [
    { "title": "File:Tango de Manzana (ISRC USUAN1100404).mp3",
      "imageinfo": [{ "url": "https://upload.wikimedia.org/tango.mp3",
                      "descriptionurl": "https://commons.wikimedia.org/wiki/File:tango.mp3",
                      "mime": "audio/mpeg", "duration": 132.4 }] },
    { "title": "File:Venta callejera de manzana de oferta.ogg",
      "imageinfo": [{ "url": "https://upload.wikimedia.org/venta.ogg",
                      "descriptionurl": "https://commons.wikimedia.org/wiki/File:venta.ogg",
                      "mime": "application/ogg", "duration": 8.0 }] }
  ] }
}`

func TestCommonsAudioReturnsEverythingUnfiltered(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(commonsAudioFixture))
	}))
	defer srv.Close()

	got, err := CommonsAudio{BaseURL: srv.URL}.Search(context.Background(), "manzana", "audio", 24)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2 (no filtering)", len(got))
	}
	if got[0].Kind != "audio" || got[0].Mime != "audio/mpeg" || got[0].DurationMs != 132400 {
		t.Errorf("result = %+v", got[0])
	}
	// Not a Lingua Libre name: no language or speaker is invented.
	if got[0].Lang != "" || got[0].Speaker != "" {
		t.Errorf("lang/speaker = %q/%q, want empty", got[0].Lang, got[0].Speaker)
	}
	if got[0].Title != "Tango de Manzana (ISRC USUAN1100404)" {
		t.Errorf("title = %q", got[0].Title)
	}
	if !strings.Contains(gotQuery, "filetype%3Aaudio") {
		t.Errorf("query %q should restrict to audio", gotQuery)
	}
}

func TestCommonsAudioRejectsUnsupportedKind(t *testing.T) {
	if _, err := (CommonsAudio{BaseURL: "http://unused"}).Search(context.Background(), "a", "image", 24); err == nil {
		t.Error("expected an error for kind=image")
	}
}

func TestCommonsAudioMetadata(t *testing.T) {
	c := CommonsAudio{BaseURL: "http://unused"}
	if c.ID() != "commons-audio" {
		t.Errorf("ID = %q, want commons-audio", c.ID())
	}
	if len(c.Kinds()) != 1 || c.Kinds()[0] != "audio" {
		t.Errorf("Kinds = %v, want [audio]", c.Kinds())
	}
}
