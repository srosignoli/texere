package source

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchAndExtract(t *testing.T) {
	// A 1x1 transparent GIF the article references.
	gif := []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 1, 0, 1, 0, 0x80, 0, 0,
		0, 0, 0, 0xff, 0xff, 0xff, 0x21, 0xf9, 4, 1, 0, 0, 0, 0,
		0x2c, 0, 0, 0, 0, 1, 0, 1, 0, 0, 2, 2, 0x44, 1, 0, 0x3b}
	mux := http.NewServeMux()
	mux.HandleFunc("/pixel.gif", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		w.Write(gif)
	})
	mux.HandleFunc("/article", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>The Real Title</title></head><body>
			<nav>nav junk</nav>
			<article><h1>The Real Title</h1>
			<p>First substantial paragraph of the article body that readability should keep, long enough to count as content for the extraction heuristics to fire properly here.</p>
			<p>Second substantial paragraph, also long enough, with an inline image: <img src="/pixel.gif" alt="px"></p>
			<script>window.evil = 1</script>
			</article></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	art, err := FetchAndExtract(srv.URL + "/article")
	if err != nil {
		t.Fatalf("FetchAndExtract: %v", err)
	}
	if !strings.Contains(art.Title, "The Real Title") {
		t.Fatalf("title=%q", art.Title)
	}
	if art.URL != srv.URL+"/article" {
		t.Fatalf("URL=%q", art.URL)
	}
	if strings.Contains(art.HTML, "<script") || strings.Contains(art.HTML, "window.evil") {
		t.Fatalf("sanitized HTML still has a script:\n%s", art.HTML)
	}
	if strings.Contains(art.HTML, "/pixel.gif") {
		t.Fatalf("image not inlined (remote src remains):\n%s", art.HTML)
	}
	if !strings.Contains(art.HTML, "data:image/gif;base64,") {
		t.Fatalf("image not inlined as data URI:\n%s", art.HTML)
	}
	if !strings.Contains(art.HTML, "substantial paragraph") {
		t.Fatalf("article body missing:\n%s", art.HTML)
	}
}

// Wikipedia and many other sites reject the default Go http User-Agent with a
// 403. FetchAndExtract must send a real User-Agent so these fetches succeed.
func TestFetchAndExtractSendsUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua == "" || strings.HasPrefix(ua, "Go-http-client") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>UA Title</title></head><body><article><h1>UA Title</h1>
			<p>A sufficiently long article paragraph so readability keeps it as the main content of this page here now.</p>
			</article></body></html>`))
	}))
	defer srv.Close()
	art, err := FetchAndExtract(srv.URL + "/article")
	if err != nil {
		t.Fatalf("FetchAndExtract with UA-gating server: %v", err)
	}
	if !strings.Contains(art.Title, "UA Title") {
		t.Fatalf("title=%q", art.Title)
	}
}

func TestFetchAndExtractDropsFailedImageKeepsArticle(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/img404", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/article", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>T</title></head><body><article><h1>T</h1>
			<p>A long article paragraph that readability keeps as the main content of this page for sure here now.</p>
			<p>More body text with a broken image: <img src="/img404" alt="x"></p>
			</article></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	art, err := FetchAndExtract(srv.URL + "/article")
	if err != nil {
		t.Fatalf("FetchAndExtract: %v", err)
	}
	if !strings.Contains(art.HTML, "More body text") {
		t.Fatalf("article body lost when image failed:\n%s", art.HTML)
	}
	if strings.Contains(art.HTML, "/img404") {
		t.Fatalf("failed image's remote src should have been dropped:\n%s", art.HTML)
	}
}

func TestFetchAndExtractNon2xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	defer srv.Close()
	if _, err := FetchAndExtract(srv.URL); err == nil {
		t.Fatal("expected an error for a non-2xx article fetch")
	}
}
