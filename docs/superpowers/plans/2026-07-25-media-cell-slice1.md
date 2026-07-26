# Media Cell Slice 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Media cell to the processing notebook that searches the web for a picture, stores the chosen image in the collection's `media/` directory, and promotes it to an N4L `has picture` fact.

**Architecture:** A new provider-agnostic `internal/mediasearch` package returns normalized `Result` values from two keyless providers (Wikimedia Commons, Openverse). A new `media.FetchRemote` downloads a chosen URL through the same guards `internal/source/web.go` already uses and stores it via the existing content-addressed `store()`. On the frontend, a new `MediaCell` renders an inline acquisition panel when it has no asset and the image plus a toolbar when it does; promotion reuses the notebook's existing `onInsert` path.

**Tech Stack:** Go 1.22+ (stdlib only — `net/http`, `encoding/json`), Svelte 3.49, Wails v2.12, `node:test` for JS unit tests.

## Global Constraints

- **Pure Go only.** No CGo, no new external dependencies. Everything in this plan uses the standard library.
- **Go formatting:** `gofmt`; imports grouped stdlib / external / internal.
- **Persisted cell `type` strings must not change.** The existing image cell stays `"image"` on disk; only its button label changes to `+ Scan`. The new cell is `"media"`. Changing a persisted `type` breaks `cellRegistry` lookup for every saved notebook.
- **`ImageCell.svelte`, `bake.js`, and `TranscribeBar.svelte` are not modified by any task in this plan.** `MediaCell` must never mount `TranscribeBar`.
- **Descriptive User-Agent on every outbound request.** Go's default `Go-http-client/...` is rejected with 403 by Wikimedia and others; Commons' API policy requires an identifying UA.
- **Search results are never persisted.** Cells are serialized wholesale into `cellsJSON` by `SaveNotebook`.
- **Provenance (`sourcePage`, `license`) lives in the cell, never in the generated N4L text.**
- Run `make test` (Go) and `make test-js` (JS, added in Task 9) before each commit that touches the corresponding language.

---

### Task 1: `media.FetchRemote`

Downloads a remote image and stores it with the same content-addressing and dedup as `Import`.

**Files:**
- Create: `internal/media/fetch.go`
- Create: `internal/media/fetch_test.go`

**Interfaces:**
- Consumes: existing unexported `store(collectionDir, data []byte, ext string) (string, error)` and `extByMime` map, both in `internal/media/import.go`.
- Produces: `media.FetchRemote(collectionDir, rawURL string) (Asset, error)`. `Asset` is the existing `{Name, Mime string}`.

- [ ] **Step 1: Write the failing tests**

Create `internal/media/fetch_test.go`. Note it reuses `pngBytes`, already declared in `media_test.go` in this same package.

```go
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
	a, err := FetchRemote(col, srv.URL+"/apple.png")
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
	b, err := FetchRemote(col, srv.URL+"/other-name.png")
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

	if _, err := FetchRemote(t.TempDir(), srv.URL); err != nil {
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

	_, err := FetchRemote(t.TempDir(), srv.URL)
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

	if _, err := FetchRemote(t.TempDir(), srv.URL); err == nil {
		t.Fatal("expected error for non-image body, got nil")
	}
}

func TestFetchRemoteRejectsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := FetchRemote(t.TempDir(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want it to name the status code", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/media/ -run TestFetchRemote -v`
Expected: FAIL — `undefined: FetchRemote`, `undefined: remoteMaxBytes`.

- [ ] **Step 3: Write the implementation**

Create `internal/media/fetch.go`:

```go
package media

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// remoteMaxBytes caps a downloaded image. A var, not a const, so tests can
// shrink it rather than transferring megabytes.
var remoteMaxBytes = 8 << 20 // 8 MB

const (
	remoteFetchTimeout = 15 * time.Second
	// remoteUserAgent identifies texere to remote servers. Go's default
	// ("Go-http-client/...") is rejected with 403 by Wikimedia and many other
	// hosts, and Commons' API policy requires an identifying agent.
	remoteUserAgent = "texere/1.0 (+https://github.com/srosignoli/texere) media-fetch"
)

var remoteClient = &http.Client{Timeout: remoteFetchTimeout}

// FetchRemote downloads rawURL, verifies it really is a supported image, and
// stores it under <collectionDir>/media/ content-addressed — so the same picture
// fetched twice, from any URL, is stored once. The URL itself is not persisted
// here; the caller keeps provenance in the notebook cell.
func FetchRemote(collectionDir, rawURL string) (Asset, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return Asset{}, fmt.Errorf("media: bad url: %w", err)
	}
	req.Header.Set("User-Agent", remoteUserAgent)
	resp, err := remoteClient.Do(req)
	if err != nil {
		return Asset{}, fmt.Errorf("media: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Asset{}, fmt.Errorf("media: fetch: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(remoteMaxBytes)+1))
	if err != nil {
		return Asset{}, fmt.Errorf("media: read: %w", err)
	}
	if len(data) > remoteMaxBytes {
		return Asset{}, fmt.Errorf("media: image larger than %d MB", remoteMaxBytes>>20)
	}
	// Sniff rather than trust Content-Type: hotlink guards and CDNs frequently
	// mislabel, and Import() already establishes sniffing as the house rule.
	mime := http.DetectContentType(data)
	ext, ok := extByMime[mime]
	if !ok {
		return Asset{}, fmt.Errorf("media: unsupported type %q", mime)
	}
	name, err := store(collectionDir, data, ext)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: name, Mime: mime}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/media/ -v`
Expected: PASS — all five new `TestFetchRemote*` tests plus the pre-existing `TestImport*` / audio tests.

- [ ] **Step 5: Commit**

```bash
git add internal/media/fetch.go internal/media/fetch_test.go
git commit -m "feat(media): FetchRemote downloads and stores a remote image"
```

---

### Task 2: `mediasearch` package — types and the Wikimedia Commons provider

**Files:**
- Create: `internal/mediasearch/mediasearch.go` (types + shared HTTP helper)
- Create: `internal/mediasearch/commons.go`
- Create: `internal/mediasearch/commons_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `mediasearch.Result{Kind, ThumbURL, FullURL, Title, SourcePage, License string}` with JSON tags `kind`, `thumbUrl`, `fullUrl`, `title`, `sourcePage`, `license`.
  - `mediasearch.Provider` interface: `ID() string`, `Name() string`, `Kinds() []string`, `Search(ctx context.Context, query, kind string, limit int) ([]Result, error)`.
  - `mediasearch.Commons{BaseURL string}` implementing `Provider`; `BaseURL` is injectable so tests point it at an `httptest` server.
  - Unexported `getJSON(ctx context.Context, rawURL string, out any) error`.

- [ ] **Step 1: Write the failing test**

Create `internal/mediasearch/commons_test.go`. The fixture is a real-shaped `formatversion=2` MediaWiki response.

```go
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

	if _, err := Commons{BaseURL: srv.URL}.Search(context.Background(), "apple", "image", 24); err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestCommonsRejectsUnsupportedKind(t *testing.T) {
	if _, err := Commons{BaseURL: "http://unused"}.Search(context.Background(), "apple", "audio", 24); err == nil {
		t.Fatal("expected error for kind=audio, got nil")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mediasearch/ -v`
Expected: FAIL — the package does not exist yet (`no Go files in .../internal/mediasearch`).

- [ ] **Step 3: Write the types and HTTP helper**

Create `internal/mediasearch/mediasearch.go`:

```go
// Package mediasearch searches the web for study assets (images now; audio and
// video later) and normalizes every provider's response into a common Result.
// It knows nothing about collections or storage — the caller downloads a chosen
// FullURL via media.FetchRemote. Pure standard library, no CGo.
package mediasearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Result is one search hit, normalized across providers. Field tags match the
// camelCase the Svelte frontend expects from the Wails binding.
type Result struct {
	Kind       string `json:"kind"`       // "image" (later "audio", "video")
	ThumbURL   string `json:"thumbUrl"`   // small preview for the results grid
	FullURL    string `json:"fullUrl"`    // what gets downloaded on pick
	Title      string `json:"title"`
	SourcePage string `json:"sourcePage"` // human-facing page, for attribution
	License    string `json:"license"`
}

// Provider is one searchable source. Implementations must be safe for
// concurrent use and must honour ctx cancellation, because the UI cancels a
// search as soon as the query changes.
type Provider interface {
	ID() string        // stable identifier persisted in the cell
	Name() string      // display label
	Kinds() []string   // kinds this provider can serve
	Search(ctx context.Context, query, kind string, limit int) ([]Result, error)
}

const (
	searchTimeout  = 15 * time.Second
	searchMaxBytes = 4 << 20 // 4 MB of JSON is far beyond any sane response
	// searchUserAgent identifies texere to remote APIs. Go's default is 403'd
	// by Wikimedia, whose API policy requires an identifying agent.
	searchUserAgent = "texere/1.0 (+https://github.com/srosignoli/texere) media-search"
)

var searchClient = &http.Client{Timeout: searchTimeout}

// getJSON performs a GET with the texere User-Agent and decodes the response
// into out. Non-2xx responses are errors.
func getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("mediasearch: bad url: %w", err)
	}
	req.Header.Set("User-Agent", searchUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := searchClient.Do(req)
	if err != nil {
		return fmt.Errorf("mediasearch: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mediasearch: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, searchMaxBytes))
	if err != nil {
		return fmt.Errorf("mediasearch: read: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("mediasearch: decode: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Write the Commons provider**

Create `internal/mediasearch/commons.go`:

```go
package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// commonsAPI is the live MediaWiki endpoint. Commons is strongest for concrete
// nouns — good for vocabulary, thin for abstract words.
const commonsAPI = "https://commons.wikimedia.org/w/api.php"

// Commons searches Wikimedia Commons. BaseURL is injectable for tests; the zero
// value is not usable — construct via Providers() or with BaseURL set.
type Commons struct{ BaseURL string }

func (c Commons) ID() string      { return "commons" }
func (c Commons) Name() string    { return "Wikimedia Commons" }
func (c Commons) Kinds() []string { return []string{"image"} }

// commonsResponse is the formatversion=2 shape, where "pages" is a JSON array
// rather than the pageid-keyed object of formatversion=1.
type commonsResponse struct {
	Query struct {
		Pages []struct {
			Title     string `json:"title"`
			ImageInfo []struct {
				ThumbURL       string `json:"thumburl"`
				URL            string `json:"url"`
				DescriptionURL string `json:"descriptionurl"`
				ExtMetadata    struct {
					LicenseShortName struct {
						Value string `json:"value"`
					} `json:"LicenseShortName"`
				} `json:"extmetadata"`
			} `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
}

func (c Commons) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "image" {
		return nil, fmt.Errorf("mediasearch: commons: unsupported kind %q", kind)
	}
	q := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"generator":     {"search"},
		"gsrsearch":     {query},
		"gsrnamespace":  {"6"}, // File:
		"gsrlimit":      {strconv.Itoa(limit)},
		"prop":          {"imageinfo"},
		"iiprop":        {"url|extmetadata"},
		"iiurlwidth":    {"240"}, // asks for a thumburl at this width
	}
	var body commonsResponse
	if err := getJSON(ctx, c.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Query.Pages))
	for _, p := range body.Query.Pages {
		if len(p.ImageInfo) == 0 {
			continue // search can return pages whose imageinfo is unavailable
		}
		ii := p.ImageInfo[0]
		out = append(out, Result{
			Kind:       "image",
			ThumbURL:   ii.ThumbURL,
			FullURL:    ii.URL,
			Title:      strings.TrimPrefix(p.Title, "File:"),
			SourcePage: ii.DescriptionURL,
			License:    ii.ExtMetadata.LicenseShortName.Value,
		})
	}
	return out, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/mediasearch/ -v`
Expected: PASS — four tests.

- [ ] **Step 6: Commit**

```bash
git add internal/mediasearch/
git commit -m "feat(mediasearch): Provider interface + Wikimedia Commons image search"
```

---

### Task 3: Openverse provider and the registry

**Files:**
- Create: `internal/mediasearch/openverse.go`
- Create: `internal/mediasearch/openverse_test.go`
- Create: `internal/mediasearch/registry.go`
- Create: `internal/mediasearch/registry_test.go`

**Interfaces:**
- Consumes: `Result`, `Provider`, `getJSON` from Task 2.
- Produces:
  - `mediasearch.Openverse{BaseURL string}` implementing `Provider`.
  - `mediasearch.Providers() []Provider` — live providers in display order.
  - `mediasearch.Find(id string) (Provider, bool)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/mediasearch/openverse_test.go`:

```go
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
```

Create `internal/mediasearch/registry_test.go`:

```go
package mediasearch

import "testing"

func TestProvidersIncludesBothAndAllServeImages(t *testing.T) {
	ps := Providers()
	if len(ps) != 2 {
		t.Fatalf("got %d providers, want 2", len(ps))
	}
	seen := map[string]bool{}
	for _, p := range ps {
		seen[p.ID()] = true
		if p.Name() == "" {
			t.Errorf("provider %q has an empty Name", p.ID())
		}
		if len(p.Kinds()) == 0 || p.Kinds()[0] != "image" {
			t.Errorf("provider %q Kinds = %v, want image first", p.ID(), p.Kinds())
		}
	}
	if !seen["commons"] || !seen["openverse"] {
		t.Errorf("providers = %v, want commons and openverse", seen)
	}
}

func TestFindKnownAndUnknown(t *testing.T) {
	p, ok := Find("commons")
	if !ok || p.ID() != "commons" {
		t.Errorf("Find(commons) = %v, %v", p, ok)
	}
	if _, ok := Find("nope"); ok {
		t.Error("Find(nope) reported ok, want false")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/mediasearch/ -run 'TestOpenverse|TestProviders|TestFind' -v`
Expected: FAIL — `undefined: Openverse`, `undefined: Providers`, `undefined: Find`.

- [ ] **Step 3: Write the Openverse provider**

Create `internal/mediasearch/openverse.go`:

```go
package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// openverseAPI works anonymously (rate-limited); registering a key raises the
// limits and is a later slice's concern.
const openverseAPI = "https://api.openverse.org/v1/images/"

// Openverse aggregates openly-licensed images from Flickr and similar sources.
// Broader everyday coverage than Commons. BaseURL is injectable for tests.
type Openverse struct{ BaseURL string }

func (o Openverse) ID() string      { return "openverse" }
func (o Openverse) Name() string    { return "Openverse" }
func (o Openverse) Kinds() []string { return []string{"image"} }

type openverseResponse struct {
	Results []struct {
		Title             string `json:"title"`
		URL               string `json:"url"`
		Thumbnail         string `json:"thumbnail"`
		ForeignLandingURL string `json:"foreign_landing_url"`
		License           string `json:"license"`
		LicenseVersion    string `json:"license_version"`
	} `json:"results"`
}

func (o Openverse) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "image" {
		return nil, fmt.Errorf("mediasearch: openverse: unsupported kind %q", kind)
	}
	q := url.Values{
		"q":         {query},
		"page_size": {strconv.Itoa(limit)},
	}
	var body openverseResponse
	if err := getJSON(ctx, o.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Results))
	for _, r := range body.Results {
		out = append(out, Result{
			Kind:       "image",
			ThumbURL:   r.Thumbnail,
			FullURL:    r.URL,
			Title:      r.Title,
			SourcePage: r.ForeignLandingURL,
			License:    openverseLicense(r.License, r.LicenseVersion),
		})
	}
	return out, nil
}

// openverseLicense renders Openverse's ("by-sa", "4.0") into "CC BY-SA 4.0",
// omitting the version when absent so no trailing space is produced.
func openverseLicense(code, version string) string {
	if code == "" {
		return ""
	}
	s := "CC " + strings.ToUpper(code)
	if version != "" {
		s += " " + version
	}
	return s
}
```

- [ ] **Step 4: Write the registry**

Create `internal/mediasearch/registry.go`:

```go
package mediasearch

// Providers returns the live providers in display order. Both are keyless;
// keyed providers (Pexels, YouTube, Google CSE) arrive in a later slice and
// will be configured through the LLMProfile pattern rather than hardcoded here.
func Providers() []Provider {
	return []Provider{
		Commons{BaseURL: commonsAPI},
		Openverse{BaseURL: openverseAPI},
	}
}

// Find returns the provider with the given ID.
func Find(id string) (Provider, bool) {
	for _, p := range Providers() {
		if p.ID() == id {
			return p, true
		}
	}
	return nil, false
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/mediasearch/ -v`
Expected: PASS — all eight tests across both providers and the registry.

- [ ] **Step 6: Verify the live API shapes match the fixtures**

The fixtures above are hand-written. A shape mismatch would pass tests and fail in the app, so check once against the real endpoints:

```bash
curl -s -A "texere/1.0 media-search" \
  'https://commons.wikimedia.org/w/api.php?action=query&format=json&formatversion=2&generator=search&gsrsearch=apple&gsrnamespace=6&gsrlimit=2&prop=imageinfo&iiprop=url|extmetadata&iiurlwidth=240' \
  | head -c 1200
curl -s -A "texere/1.0 media-search" \
  'https://api.openverse.org/v1/images/?q=apple&page_size=2' | head -c 1200
```

Confirm `query.pages[]` carries `imageinfo[0].thumburl` / `url` / `descriptionurl`, and that Openverse results carry `url`, `thumbnail`, `foreign_landing_url`, `license`. If a field name differs, fix both the struct tag and the fixture, then re-run Step 5.

- [ ] **Step 7: Commit**

```bash
git add internal/mediasearch/
git commit -m "feat(mediasearch): Openverse provider + provider registry"
```

---

### Task 4: Wails bindings

**Files:**
- Create: `media_methods.go`
- Create: `media_methods_test.go`

**Interfaces:**
- Consumes: `mediasearch.Providers()`, `mediasearch.Find()`, `mediasearch.Result` (Tasks 2–3); `media.FetchRemote` (Task 1); the existing `App` fields `collectionDir` and `dirMu`.
- Produces, callable from the frontend as `wailsjs/go/main/App`:
  - `ListMediaProviders() []ProviderInfo` where `ProviderInfo{ID, Name string; Kinds []string}` has JSON tags `id`, `name`, `kinds`.
  - `SearchMedia(providerID, query, kind string, limit int) ([]mediasearch.Result, error)`
  - `FetchMediaURL(rawURL string) (map[string]any, error)` → `{"asset": string, "mime": string}`.

- [ ] **Step 1: Write the failing tests**

Create `media_methods_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestListMediaProvidersReturnsBoth(t *testing.T) {
	got := (&App{}).ListMediaProviders()
	if len(got) != 2 {
		t.Fatalf("got %d providers, want 2", len(got))
	}
	if got[0].ID == "" || got[0].Name == "" || len(got[0].Kinds) == 0 {
		t.Errorf("provider info incomplete: %+v", got[0])
	}
}

func TestSearchMediaUnknownProvider(t *testing.T) {
	_, err := (&App{}).SearchMedia("nope", "apple", "image", 24)
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestFetchMediaURLRequiresCollection(t *testing.T) {
	_, err := (&App{}).FetchMediaURL("https://example.org/a.png")
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
	got, err := (&App{collectionDir: dir}).FetchMediaURL(srv.URL + "/a.png")
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestListMediaProviders|TestSearchMedia|TestFetchMediaURL' -v`
Expected: FAIL — `undefined: (*App).ListMediaProviders` and friends.

- [ ] **Step 3: Write the bindings**

Create `media_methods.go`:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"texere/internal/media"
	"texere/internal/mediasearch"
)

// searchCallTimeout bounds a single SearchMedia call. Wails bindings take no
// context, so the deadline is created here.
const searchCallTimeout = 20 * time.Second

// ProviderInfo describes one search provider to the frontend. Kinds lets the
// panel hide providers that cannot serve the requested kind once audio and
// video arrive.
type ProviderInfo struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Kinds []string `json:"kinds"`
}

// ListMediaProviders returns the available search providers in display order.
func (a *App) ListMediaProviders() []ProviderInfo {
	ps := mediasearch.Providers()
	out := make([]ProviderInfo, 0, len(ps))
	for _, p := range ps {
		out = append(out, ProviderInfo{ID: p.ID(), Name: p.Name(), Kinds: p.Kinds()})
	}
	return out
}

// SearchMedia runs a provider search. It needs no open collection — nothing is
// written until the user picks a result.
func (a *App) SearchMedia(providerID, query, kind string, limit int) ([]mediasearch.Result, error) {
	p, ok := mediasearch.Find(providerID)
	if !ok {
		return nil, fmt.Errorf("unknown media provider %q", providerID)
	}
	if limit <= 0 || limit > 50 {
		limit = 24
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchCallTimeout)
	defer cancel()
	return p.Search(ctx, query, kind, limit)
}

// FetchMediaURL downloads rawURL into the collection's media/ directory and
// returns {asset, mime}. Serves both "clicked a search result" and "pasted a
// URL" — they are the same operation.
func (a *App) FetchMediaURL(rawURL string) (map[string]any, error) {
	a.dirMu.RLock()
	dir := a.collectionDir
	a.dirMu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("no collection open")
	}
	asset, err := media.FetchRemote(dir, rawURL)
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": asset.Name, "mime": asset.Mime}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -v -run 'TestListMediaProviders|TestSearchMedia|TestFetchMediaURL'`
Expected: PASS — four tests.

If `pngBytes` collides with an existing declaration in package `main`, rename the one in `media_methods_test.go` to `mediaPNGBytes` and update its three uses in that file.

- [ ] **Step 5: Run the whole Go suite and the linter**

Run: `make test && make lint`
Expected: all packages PASS, no lint findings.

- [ ] **Step 6: Regenerate the Wails bindings**

Run: `wails generate module`
Expected: `frontend/wailsjs/go/main/App.js` and `App.d.ts` gain `ListMediaProviders`, `SearchMedia`, `FetchMediaURL`; `frontend/wailsjs/go/models.ts` gains `mediasearch.Result`.

- [ ] **Step 7: Commit**

```bash
git add media_methods.go media_methods_test.go frontend/wailsjs/
git commit -m "feat(media): ListMediaProviders/SearchMedia/FetchMediaURL bindings"
```

---

### Task 5: Media cell type, registry entry, and notebook buttons

Ends with: `+ Media` adds a cell that persists across a reload, and the old button reads `+ Scan`.

**Files:**
- Modify: `frontend/src/lib/notebook.js`
- Create: `frontend/src/components/notebook/MediaCell.svelte`
- Modify: `frontend/src/components/notebook/NotebookPane.svelte:105-111` (the `nb-add` button block) and its `addMedia` handler

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `newMediaCell()` in `lib/notebook.js` returning the cell shape below.
  - `cellRegistry.media = MediaCell`.
  - `MediaCell.svelte` accepting the standard cell prop set: `cell`, `onChange`, `onDelete`, `readOnly`, `context`, `cells`, `onPush`, `onInsert`.

- [ ] **Step 1: Add the cell factory and registry entry**

In `frontend/src/lib/notebook.js`, add the import beside the others at the top:

```js
import MediaCell from '../components/notebook/MediaCell.svelte'
```

Add the factory after `newAudioCell`:

```js
// Media cell: a study asset (picture now; pronunciation and video later) found
// by web search, pasted URL, or file import. `kind` discriminates the render;
// `embedURL` is reserved for video, which is referenced rather than stored, so
// adding it later needs no migration of saved notebooks.
export function newMediaCell() {
  seq += 1
  return {
    id: `cell-${Date.now()}-${seq}`, type: 'media',
    kind: 'image',
    asset: '', mime: '', embedURL: '',
    crop: null, naturalW: 0, naturalH: 0,
    query: '', subject: '',
    provider: '', sourcePage: '', license: '',
  }
}
```

Add to `cellRegistry`:

```js
  media: MediaCell,
```

- [ ] **Step 2: Create the placeholder MediaCell**

Create `frontend/src/components/notebook/MediaCell.svelte`. Tasks 6–9 fill in the two real states; this establishes the shell and the delete control.

```svelte
<script>
  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused
  export let onInsert = () => {}
</script>

<div class="me-cell">
  <div class="me-bar">
    <span class="me-label">Media</span>
    <span class="me-spacer"></span>
    {#if !readOnly}<button class="me-btn" on:click={onDelete} title="Delete cell">✕</button>{/if}
  </div>
  {#if !cell.asset}
    <div class="me-empty">No asset yet.</div>
  {:else}
    <div class="me-empty">asset: {cell.asset}</div>
  {/if}
</div>

<style>
  .me-cell { border: 1px solid #2a3a4a; border-radius: 6px; background: #101826; }
  .me-bar { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid #2a3a4a; }
  .me-label { font-size: 12px; color: #8aa; }
  .me-spacer { flex: 1 1 auto; }
  .me-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 2px 8px; cursor: pointer; }
  .me-btn:hover { background: #24334a; }
  .me-empty { padding: 12px; color: #8aa; font-size: 13px; }
</style>
```

- [ ] **Step 3: Wire the buttons in NotebookPane**

In `frontend/src/components/notebook/NotebookPane.svelte`, extend the import on line 4 with `newMediaCell`:

```js
  import { newDraftCell, newLLMCell, newGraphCell, newComputeCell, newSketchCell, newImageCell, newAudioCell, newMediaCell, cellRegistry } from '../../lib/notebook.js'
```

Add the handler next to `addAudio`:

```js
  function addMedia() { cells = [...cells, newMediaCell()]; scheduleSave() }
```

In the `{#if !readOnly}` button block, relabel the image button and add the media one. `addImage` and the `"image"` cell type are unchanged — only the label moves from "Image cell" to "Scan":

```svelte
      <button class="nb-add" on:click={addImage}>+ Scan</button>
      <button class="nb-add" on:click={addAudio}>+ Audio cell</button>
      <button class="nb-add" on:click={addMedia}>+ Media</button>
```

- [ ] **Step 4: Verify in the running app**

Run: `make dev`

Check, in an open collection with a notebook:
1. The button row reads `+ Scan` where it used to read `+ Image cell`, and a `+ Media` button follows `+ Audio cell`.
2. `+ Scan` still opens the OS file dialog and produces a working image cell with its Transcribe bar.
3. `+ Media` adds a cell reading "No asset yet."
4. Reload (Ctrl+R). The media cell is still there — proving `type: "media"` round-trips through `SaveNotebook`.
5. Any pre-existing image cells in saved notebooks still render.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/notebook.js frontend/src/components/notebook/MediaCell.svelte frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): media cell type + '+ Media' button; relabel image cell '+ Scan'"
```

---

### Task 6: Search panel and image render — search, pick, display

Ends with: type a word, pick a picture, see it in the cell.

**Files:**
- Create: `frontend/src/components/notebook/MediaSearchPanel.svelte`
- Create: `frontend/src/components/notebook/media/MediaImageView.svelte`
- Modify: `frontend/src/components/notebook/MediaCell.svelte`

**Interfaces:**
- Consumes: `ListMediaProviders`, `SearchMedia`, `FetchMediaURL` from `wailsjs/go/main/App` (Task 4); `imageStyles` from `lib/image-view.js` (existing, signature `imageStyles({naturalW, naturalH, rotation = 0, crop = null})` → `{viewport, pan, img}` CSS strings).
- Produces:
  - `MediaSearchPanel.svelte` with props `kind` (string), `query` (string, two-way via `onQuery`), `onPick` (`(result, {asset, mime}) => void`), `onQuery` (`(q) => void`), and `busy` handled internally.
  - `MediaImageView.svelte` with prop `cell`.

- [ ] **Step 1: Create the image view**

Create `frontend/src/components/notebook/media/MediaImageView.svelte`:

```svelte
<script>
  import { imageStyles } from '../../../lib/image-view.js'

  export let cell
  export let onMeta = () => {}   // (w, h) => record and persist the intrinsic size

  let missing = false

  $: styles = imageStyles(cell)
  $: src = `/media/${cell.asset}`

  function onLoad(e) {
    missing = false
    // Persisted so imageStyles is correct on first paint after a reload, rather
    // than only once the image has loaded. Same behaviour as ImageCell.
    if (!cell.naturalW || !cell.naturalH) {
      onMeta(e.target.naturalWidth, e.target.naturalHeight)
    }
  }
</script>

{#if missing}
  <div class="mv-missing">image not found: {cell.asset}</div>
{:else}
  <div class="mv-view" style={styles.viewport}>
    <div style={styles.pan}>
      <img {src} style={styles.img} alt={cell.subject || cell.query || ''}
        on:load={onLoad} on:error={() => (missing = true)} draggable="false" />
    </div>
  </div>
{/if}

<style>
  .mv-view { position: relative; overflow: hidden; margin: 8px; }
  .mv-missing { padding: 12px; color: #d88; font-size: 13px; }
</style>
```

- [ ] **Step 2: Create the search panel**

Create `frontend/src/components/notebook/MediaSearchPanel.svelte`. Note the stale-result guard: a `seq` counter, incremented per search, so a slow earlier response cannot overwrite a newer one.

```svelte
<script>
  import { onMount } from 'svelte'
  import { ListMediaProviders, SearchMedia, FetchMediaURL } from '../../../wailsjs/go/main/App'

  export let kind = 'image'
  export let query = ''
  export let onQuery = () => {}   // (q) => persist the query on the cell
  export let onPick = () => {}    // (result, {asset, mime}) => void

  const LIMIT = 24

  let providers = []
  let providerId = ''
  let results = []
  let searched = false      // distinguishes "no results" from "not searched yet"
  let busy = false
  let err = ''
  let fetchingURL = ''      // FullURL of the result currently downloading
  let seq = 0               // guards against out-of-order responses

  onMount(async () => {
    try {
      providers = (await ListMediaProviders()).filter((p) => p.kinds?.includes(kind))
      providerId = providers[0]?.id ?? ''
    } catch (e) { err = String(e?.message ?? e) }
  })

  async function search() {
    const q = query.trim()
    if (!q || !providerId) return
    const mine = ++seq
    busy = true; err = ''; results = []
    try {
      const rs = await SearchMedia(providerId, q, kind, LIMIT)
      if (mine !== seq) return       // a newer search superseded this one
      results = rs ?? []
      searched = true
    } catch (e) {
      if (mine !== seq) return
      err = String(e?.message ?? e)
    } finally {
      if (mine === seq) busy = false
    }
  }

  function onKey(e) {
    if (e.key === 'Enter') { e.preventDefault(); search() }
  }

  async function pick(r) {
    if (fetchingURL) return
    fetchingURL = r.fullUrl
    err = ''
    try {
      const ref = await FetchMediaURL(r.fullUrl)
      onPick(r, ref)
    } catch (e) {
      const msg = String(e?.message ?? e)
      // 403 on a direct fetch almost always means hotlink protection: the
      // picture is viewable on its page but not downloadable by URL.
      err = msg.includes('403')
        ? `${msg} — this host blocks direct downloads. Open the source page and use Paste URL with the real image address.`
        : msg
    } finally {
      fetchingURL = ''
    }
  }
</script>

<div class="ms-panel">
  <div class="ms-bar">
    <select bind:value={providerId} disabled={busy || !providers.length}>
      {#each providers as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <input class="ms-q" type="text" placeholder="Search for a picture…"
      bind:value={query} on:input={() => onQuery(query)} on:keydown={onKey} />
    <button class="ms-btn" on:click={search} disabled={busy || !query.trim()}>
      {busy ? 'Searching…' : 'Search'}
    </button>
  </div>

  {#if err}<div class="ms-err">{err}</div>{/if}

  {#if busy}
    <div class="ms-note">Searching {providers.find((p) => p.id === providerId)?.name ?? ''}…</div>
  {:else if searched && !results.length && !err}
    <div class="ms-note">No results for “{query.trim()}”. Try another provider, or Paste URL.</div>
  {:else if results.length}
    <div class="ms-grid">
      {#each results as r (r.fullUrl)}
        <button class="ms-hit" class:ms-loading={fetchingURL === r.fullUrl}
          on:click={() => pick(r)} title={`${r.title}${r.license ? ' — ' + r.license : ''}`}>
          <img src={r.thumbUrl} alt={r.title} loading="lazy" />
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .ms-panel { padding: 8px; }
  .ms-bar { display: flex; gap: 6px; align-items: center; }
  .ms-q { flex: 1 1 auto; background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 4px 8px; }
  .ms-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 4px 10px; cursor: pointer; }
  .ms-btn:disabled { opacity: 0.5; cursor: default; }
  select { background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 4px; }
  .ms-err { margin-top: 8px; padding: 6px 8px; border: 1px solid #6a3a3a; border-radius: 4px; background: #2a1616; color: #f9c; font-size: 12px; }
  .ms-note { margin-top: 8px; color: #8aa; font-size: 12px; }
  .ms-grid { margin-top: 8px; display: grid; grid-template-columns: repeat(auto-fill, minmax(96px, 1fr)); gap: 6px; max-height: 320px; overflow-y: auto; }
  .ms-hit { padding: 0; border: 1px solid #2a3a4a; border-radius: 4px; background: #0b1220; cursor: pointer; overflow: hidden; aspect-ratio: 1; }
  .ms-hit:hover { border-color: #4a7aa8; }
  .ms-hit.ms-loading { opacity: 0.4; cursor: default; }
  .ms-hit img { width: 100%; height: 100%; object-fit: cover; display: block; }
</style>
```

- [ ] **Step 3: Wire both into MediaCell**

Replace the body of `frontend/src/components/notebook/MediaCell.svelte`:

```svelte
<script>
  import MediaSearchPanel from './MediaSearchPanel.svelte'
  import MediaImageView from './media/MediaImageView.svelte'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused
  export let onInsert = () => {}

  // kind -> render component. Audio and video slot in here in later slices
  // without MediaCell changing.
  const views = { image: MediaImageView }

  function persist() { onChange(cell) }

  function onPick(result, ref) {
    cell.asset = ref.asset
    cell.mime = ref.mime
    cell.kind = result.kind || 'image'
    cell.provider = result.provider ?? cell.provider
    cell.sourcePage = result.sourcePage ?? ''
    cell.license = result.license ?? ''
    // The thing you searched for is the thing the fact will be about.
    if (!cell.subject) cell.subject = cell.query.trim()
    cell.naturalW = 0
    cell.naturalH = 0
    cell = cell
    persist()
  }

  function onQuery(q) { cell.query = q; persist() }

  function onMeta(w, h) {
    cell.naturalW = w
    cell.naturalH = h
    cell = cell
    persist()
  }
</script>

<div class="me-cell">
  <div class="me-bar">
    <span class="me-label">Media</span>
    {#if cell.license}<span class="me-lic">{cell.license}</span>{/if}
    <span class="me-spacer"></span>
    {#if !readOnly}<button class="me-btn" on:click={onDelete} title="Delete cell">✕</button>{/if}
  </div>

  {#if !cell.asset}
    {#if readOnly}
      <div class="me-empty">No asset.</div>
    {:else}
      <MediaSearchPanel kind={cell.kind} query={cell.query} {onQuery} {onPick} />
    {/if}
  {:else}
    <svelte:component this={views[cell.kind]} {cell} {onMeta} />
    {#if cell.sourcePage}
      <div class="me-src"><a href={cell.sourcePage} target="_blank" rel="noreferrer">source</a></div>
    {/if}
  {/if}
</div>

<style>
  .me-cell { border: 1px solid #2a3a4a; border-radius: 6px; background: #101826; }
  .me-bar { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid #2a3a4a; }
  .me-label { font-size: 12px; color: #8aa; }
  .me-lic { font-size: 11px; color: #678; }
  .me-spacer { flex: 1 1 auto; }
  .me-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 2px 8px; cursor: pointer; }
  .me-btn:hover { background: #24334a; }
  .me-empty { padding: 12px; color: #8aa; font-size: 13px; }
  .me-src { padding: 0 8px 8px; font-size: 11px; }
  .me-src a { color: #6ad; }
</style>
```

Note `result.provider` is not a field on `mediasearch.Result`; the `??` keeps `cell.provider` unchanged for now, and Task 7 sets it from the panel's selected provider.

- [ ] **Step 4: Verify in the running app**

Run: `make dev`

1. `+ Media`, type `apple`, press Enter → a grid of thumbnails appears.
2. Switch the provider to Openverse and search again → a different grid.
3. Click a thumbnail → the cell replaces the panel with the picture, and a `source` link appears.
4. Reload → the picture is still there (served from `/media/<hash>.jpg`).
5. Search something with no hits (`qwertyuiopasdfgh`) → "No results for …", *not* an error.
6. Type a query, press Enter, and immediately change the query and press Enter again → the grid shows the second query's results; the first does not overwrite them.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/notebook/MediaSearchPanel.svelte frontend/src/components/notebook/media/ frontend/src/components/notebook/MediaCell.svelte
git commit -m "feat(notebook): media cell web image search, pick, and render"
```

---

### Task 7: Import file, Paste URL, Replace, and provider provenance

**Files:**
- Modify: `frontend/src/components/notebook/MediaSearchPanel.svelte`
- Modify: `frontend/src/components/notebook/MediaCell.svelte`

**Interfaces:**
- Consumes: `ImportImage` from `wailsjs/go/main/App` (existing, returns `{asset, mime}` or `null` on cancel); `FetchMediaURL` (Task 4); `onPick` from Task 6.
- Produces: `onPick(result, ref)` is now always called with a `result` carrying `kind`, `title`, `sourcePage`, `license`, and `provider`, whatever the acquisition path.

- [ ] **Step 1: Add the alternate paths to the panel**

In `MediaSearchPanel.svelte`, extend the import:

```js
  import { ListMediaProviders, SearchMedia, FetchMediaURL, ImportImage } from '../../../wailsjs/go/main/App'
```

Add state beside the others:

```js
  let pasteURL = ''
```

Set the provider on picked search results — replace the existing `pick` body's `onPick` call so provenance carries the provider id. The full replacement for `pick`:

```js
  async function pick(r) {
    if (fetchingURL) return
    fetchingURL = r.fullUrl
    err = ''
    try {
      const ref = await FetchMediaURL(r.fullUrl)
      onPick({ ...r, provider: providerId }, ref)
    } catch (e) {
      const msg = String(e?.message ?? e)
      err = msg.includes('403')
        ? `${msg} — this host blocks direct downloads. Open the source page and use Paste URL with the real image address.`
        : msg
    } finally {
      fetchingURL = ''
    }
  }
```

Add the two alternate acquisition functions:

```js
  async function importFile() {
    err = ''
    try {
      const ref = await ImportImage()
      if (!ref) return // cancelled
      onPick({ kind, title: '', sourcePage: '', license: '', provider: 'file' }, ref)
    } catch (e) { err = String(e?.message ?? e) }
  }

  async function fetchPasted() {
    const u = pasteURL.trim()
    if (!u) return
    err = ''
    busy = true
    try {
      const ref = await FetchMediaURL(u)
      onPick({ kind, title: '', sourcePage: u, license: '', provider: 'url' }, ref)
    } catch (e) {
      err = String(e?.message ?? e)
    } finally { busy = false }
  }
```

Add the row to the markup, immediately after the closing `</div>` of `.ms-bar`:

```svelte
  <div class="ms-alt">
    <button class="ms-btn" on:click={importFile} disabled={busy}>Import file…</button>
    <input class="ms-q" type="text" placeholder="…or paste an image URL"
      bind:value={pasteURL} on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); fetchPasted() } }} />
    <button class="ms-btn" on:click={fetchPasted} disabled={busy || !pasteURL.trim()}>Fetch</button>
  </div>
```

Add the style:

```css
  .ms-alt { display: flex; gap: 6px; align-items: center; margin-top: 6px; }
```

- [ ] **Step 2: Add Replace to the filled state**

In `MediaCell.svelte`, add state and a handler in the `<script>`:

```js
  let replacing = false

  function replace() { replacing = true }
```

Extend `onPick` so picking while replacing returns to the filled view — add as its last statement, before `persist()`:

```js
    replacing = false
```

Change the state test in the markup from `{#if !cell.asset}` to `{#if !cell.asset || replacing}`, and add a Replace button to the bar, before the delete button:

```svelte
    {#if !readOnly && cell.asset && !replacing}
      <button class="me-btn" on:click={replace} title="Pick a different asset">Replace</button>
    {/if}
    {#if !readOnly && replacing && cell.asset}
      <button class="me-btn" on:click={() => (replacing = false)} title="Keep the current asset">Cancel</button>
    {/if}
```

Picking a new asset while `replacing` overwrites the old fields wholesale in `onPick`, so no separate clear step is needed.

- [ ] **Step 3: Verify in the running app**

Run: `make dev`

1. `+ Media` → *Import file…* → pick a local JPEG → it renders, with no source link.
2. `+ Media` → paste a direct image URL (e.g. any `https://upload.wikimedia.org/...jpg`) → *Fetch* → it renders and the source link points at the pasted URL.
3. Paste a URL that is not an image (a plain HTML page) → an error naming the unsupported type; the cell keeps its previous state.
4. Paste a URL returning 404 → an error naming the status.
5. On a filled cell, *Replace* → the panel reappears with the previous query intact → pick a different picture → the cell shows the new one. *Cancel* returns to the existing picture.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/notebook/MediaSearchPanel.svelte frontend/src/components/notebook/MediaCell.svelte
git commit -m "feat(notebook): media cell file import, paste-URL, and replace"
```

---

### Task 8: Crop

**Files:**
- Modify: `frontend/src/components/notebook/media/MediaImageView.svelte`
- Modify: `frontend/src/components/notebook/MediaCell.svelte`

**Interfaces:**
- Consumes: `imageStyles` from `lib/image-view.js`; `cell.crop` as `{x, y, w, h}` in fractions of the frame, the same shape `ImageCell` persists.
- Produces: a `cropping` prop on `MediaImageView` and an `onCrop(crop)` callback.

Rotation is deliberately **not** implemented — scans need it, illustrations do not, and its absence keeps the two cells' toolbars visibly different.

- [ ] **Step 1: Add crop-drag to the image view**

Replace `frontend/src/components/notebook/media/MediaImageView.svelte`:

```svelte
<script>
  import { imageStyles } from '../../../lib/image-view.js'

  export let cell
  export let cropping = false
  export let onCrop = () => {}   // ({x,y,w,h}) => void
  export let onMeta = () => {}   // (w, h) => record and persist the intrinsic size

  let missing = false
  let imgEl
  let sel = null
  let dragging = false
  let start = null

  $: styles = imageStyles(cell)
  $: src = `/media/${cell.asset}`
  // While cropping, show the uncropped frame so a new selection always covers
  // the whole image rather than the previous crop.
  $: cropStyles = imageStyles({ ...cell, crop: null })

  function onLoad(e) {
    missing = false
    if (!cell.naturalW || !cell.naturalH) {
      onMeta(e.target.naturalWidth, e.target.naturalHeight)
    }
  }

  function frac(e) {
    const r = imgEl.getBoundingClientRect()
    return {
      x: Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)),
      y: Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)),
    }
  }
  function down(e) { dragging = true; start = frac(e); sel = null; e.target.setPointerCapture?.(e.pointerId) }
  function move(e) {
    if (!dragging) return
    const p = frac(e)
    sel = { x: Math.min(start.x, p.x), y: Math.min(start.y, p.y), w: Math.abs(p.x - start.x), h: Math.abs(p.y - start.y) }
  }
  function up() {
    if (!dragging) return
    dragging = false
    if (sel && sel.w > 0.02 && sel.h > 0.02) onCrop({ ...sel })
    sel = null
  }
</script>

{#if missing}
  <div class="mv-missing">image not found: {cell.asset}</div>
{:else if cropping}
  <div class="mv-cropwrap">
    <div class="mv-view" style={cropStyles.viewport}>
      <div style={cropStyles.pan}>
        <img bind:this={imgEl} {src} style={cropStyles.img} alt=""
          on:pointerdown={down} on:pointermove={move} on:pointerup={up} on:pointercancel={up} draggable="false" />
      </div>
      {#if sel}
        <div class="mv-sel" style="left:{sel.x * 100}%;top:{sel.y * 100}%;width:{sel.w * 100}%;height:{sel.h * 100}%;"></div>
      {/if}
    </div>
    <div class="mv-hint">Drag to select a crop.</div>
  </div>
{:else}
  <div class="mv-view" style={styles.viewport}>
    <div style={styles.pan}>
      <img {src} style={styles.img} alt={cell.subject || cell.query || ''}
        on:load={onLoad} on:error={() => (missing = true)} draggable="false" />
    </div>
  </div>
{/if}

<style>
  .mv-view { position: relative; overflow: hidden; margin: 8px; }
  .mv-missing { padding: 12px; color: #d88; font-size: 13px; }
  .mv-cropwrap { position: relative; }
  .mv-sel { position: absolute; border: 1px solid #6ad; background: rgba(102, 170, 221, 0.2); pointer-events: none; }
  .mv-hint { padding: 0 8px 8px; color: #8aa; font-size: 11px; }
</style>
```

- [ ] **Step 2: Add the crop toolbar to MediaCell**

In `MediaCell.svelte`, add state and handlers:

```js
  let cropping = false

  function applyCrop(crop) { cell.crop = crop; cell = cell; persist(); cropping = false }
  function clearCrop() { cell.crop = null; cell = cell; persist() }
```

Pass the props through to the view — replace the `<svelte:component>` line:

```svelte
    <svelte:component this={views[cell.kind]} {cell} {cropping} {onMeta} onCrop={applyCrop} />
```

Add the toolbar immediately after that line:

```svelte
    {#if !readOnly}
      <div class="me-tools">
        {#if cropping}
          <button class="me-btn" on:click={() => (cropping = false)}>Cancel crop</button>
        {:else}
          <button class="me-btn" on:click={() => (cropping = true)}>Crop</button>
        {/if}
        {#if cell.crop}<button class="me-btn" on:click={clearCrop}>Clear crop</button>{/if}
      </div>
    {/if}
```

Add the style:

```css
  .me-tools { display: flex; gap: 6px; padding: 0 8px 8px; }
```

`cropping` must reset when the asset changes — add `cropping = false` alongside `replacing = false` in `onPick`.

- [ ] **Step 3: Verify in the running app**

Run: `make dev`

1. On a filled media cell, *Crop* → the full image appears with the drag hint.
2. Drag a rectangle → on release, the cell shows just that region and the toolbar gains *Clear crop*.
3. Reload → the crop persists.
4. *Clear crop* → the full image returns.
5. *Cancel crop* leaves the existing crop untouched.
6. A drag smaller than 2% in either axis is ignored (guards against a stray click wiping the view).
7. Confirm the Scan cell still has *Rotate* and the Media cell does not.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/notebook/media/MediaImageView.svelte frontend/src/components/notebook/MediaCell.svelte
git commit -m "feat(notebook): crop for the media cell"
```

---

### Task 9: `buildMediaFact` and Promote to N4L

**Files:**
- Create: `frontend/src/lib/media-fact.js`
- Create: `frontend/src/lib/media-fact.test.js`
- Modify: `frontend/src/components/notebook/MediaCell.svelte`
- Modify: `Makefile` (add a `test-js` target)

**Interfaces:**
- Consumes: `onInsert(text)`, already wired at `NotebookPane.svelte:96` to `insertAfter(i, newDraftCell(text))`.
- Produces: `buildMediaFact({subject, asset})` → N4L text string, exported from `lib/media-fact.js`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/media-fact.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildMediaFact } from './media-fact.js'

test('builds a has-picture fact from subject and asset', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'a3f9c1.jpg' }),
    'manzana\n"  (pic) media/a3f9c1.jpg\n')
})

test('a multi-word subject is preserved verbatim', () => {
  assert.equal(
    buildMediaFact({ subject: 'red delicious apple', asset: 'b7.png' }),
    'red delicious apple\n"  (pic) media/b7.png\n')
})

test('subject is trimmed', () => {
  assert.equal(
    buildMediaFact({ subject: '  manzana  ', asset: 'a3.jpg' }),
    'manzana\n"  (pic) media/a3.jpg\n')
})

test('an empty subject yields an empty string, not a malformed fact', () => {
  assert.equal(buildMediaFact({ subject: '', asset: 'a3.jpg' }), '')
  assert.equal(buildMediaFact({ subject: '   ', asset: 'a3.jpg' }), '')
})

test('a missing asset yields an empty string', () => {
  assert.equal(buildMediaFact({ subject: 'manzana', asset: '' }), '')
})
```

- [ ] **Step 2: Add the `test-js` target and run to verify failure**

Add to `Makefile`, after the `test` target, and add `test-js` to the `.PHONY` line:

```makefile
## test-js: run frontend unit tests (node:test)
test-js:
	node --test frontend/src/lib/
```

Update the `.PHONY` line to:

```makefile
.PHONY: build build-windows test test-js bench ci lint dev clean deps
```

Run: `make test-js`
Expected: FAIL — `Cannot find module '.../frontend/src/lib/media-fact.js'`. The pre-existing `node-source.test.js` should pass.

- [ ] **Step 3: Write the implementation**

Create `frontend/src/lib/media-fact.js`:

```js
// Builds the N4L text promoted from a media cell. Kept as a pure function so it
// is unit-testable without mounting Svelte.
//
// The asset path is collection-relative (`media/<hash>.<ext>`), not absolute and
// not a file:// URL, so a collection stays portable when moved or shared.

// N4L references arrows by their short CODE, not their long name — the code
// is the `(pic)` token in SSTconfig/arrows-EP-3.sst, which declares long name
// "has picture" / reverse "is a picture of" for it.
const PICTURE_ARROW = 'pic'

export function buildMediaFact({ subject, asset }) {
  const s = (subject ?? '').trim()
  if (!s || !asset) return ''
  return `${s}\n"  (${PICTURE_ARROW}) media/${asset}\n`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `make test-js`
Expected: PASS — five `media-fact` tests plus the existing `node-source` tests.

- [ ] **Step 5: Add the Promote control to MediaCell**

In `MediaCell.svelte`, extend the script:

```js
  import { buildMediaFact } from '../../lib/media-fact.js'
```

```js
  function setSubject(e) { cell.subject = e.target.value; persist() }

  function promote() {
    const text = buildMediaFact(cell)
    if (!text) return
    onInsert(text)
  }
```

Add to the `.me-tools` toolbar, after the crop buttons:

```svelte
        <span class="me-spacer"></span>
        <input class="me-subj" type="text" placeholder="subject" value={cell.subject}
          on:input={setSubject} title="The N4L node this picture is about" />
        <button class="me-btn" on:click={promote} disabled={!cell.subject.trim()}>Promote to N4L</button>
```

Add the styles:

```css
  .me-subj { width: 140px; background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 2px 6px; }
  .me-btn:disabled { opacity: 0.5; cursor: default; }
```

- [ ] **Step 6: Verify in the running app**

Run: `make dev`

1. Search `manzana`, pick a picture → the subject field is pre-filled with `manzana`.
2. *Promote to N4L* → an N4L draft cell appears directly below containing:
   ```
   manzana
   "  (pic) media/<hash>.jpg
   ```
3. Edit the subject to `manzana roja`, promote again → the new draft uses the edited subject.
4. Clear the subject → *Promote to N4L* is disabled.
5. Push the draft to a file with the existing push control → the file gains the fact and the notebook link mark appears.
6. Open the resulting node in the graph explorer → the `has picture` arrow is present.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/media-fact.js frontend/src/lib/media-fact.test.js frontend/src/components/notebook/MediaCell.svelte Makefile
git commit -m "feat(notebook): promote a media cell to a 'has picture' N4L fact"
```

---

### Task 10: Arrow declaration, full verification, and the manual test plan

**Files:**
- Modify: `SSTconfig/arrows-EP-3.sst`
- Create: `docs/superpowers/specs/2026-07-25-media-cell-slice1-manual-test-plan.md`

**Interfaces:**
- Consumes: the arrow **code** `pic` emitted by `lib/media-fact.js` (Task 9). N4L references arrows by code, not long name, so the code in the emitter and the code declared here must match exactly.

- [ ] **Step 1: Declare the arrow**

In `SSTconfig/arrows-EP-3.sst`, in the `:: attributes, supporting information ::` section, add alongside the other `has …` lines, matching the existing column alignment:

```
 + has picture       (pic)          - is a picture of      (pic-of)
```

- [ ] **Step 2: Verify the arrow resolves**

Run: `make dev`

Promote a media cell and push it to a file, then confirm the arrow is recognized rather than synthesized as unknown: the arrow renders with its long name in the graph explorer, and no "unknown arrow" warning appears for `has picture`.

- [ ] **Step 3: Run the full verification suite**

```bash
make test
make test-js
make lint
make build
```

Expected: Go tests PASS across all packages; JS tests PASS; no lint findings; the binary builds.

- [ ] **Step 4: Write the manual test plan**

Create `docs/superpowers/specs/2026-07-25-media-cell-slice1-manual-test-plan.md`, following the structure of `2026-07-25-graph-cell-iso-view-manual-test-plan.md`. Cover, each as a numbered case with explicit steps and expected results:

1. **Buttons** — `+ Scan` retains the old image-cell behaviour including Transcribe; `+ Media` adds an empty media cell; both persist across reload.
2. **Search, both providers** — Commons and Openverse each return a grid for `apple`; switching providers re-searches.
3. **Pick and persist** — picking renders the image; reload keeps it; the same picture picked twice leaves one file in `<collection>/media/` (content-addressed dedup).
4. **Empty results** — `qwertyuiopasdfgh` shows the "No results" note, not an error.
5. **Provider failure** — disconnect the network, search → an error message with the query preserved; reconnect and retry succeeds.
6. **Stale search** — search, then immediately search again; the second query's results win.
7. **Import file** — a local JPEG imports and renders with no source link.
8. **Paste URL** — a direct image URL fetches; an HTML page errors on type; a 404 errors on status; a hotlink-protected URL produces the 403 message pointing at Paste URL.
9. **Oversize** — a >8 MB image errors naming the cap.
10. **Replace** — returns to the panel with the query intact; Cancel restores the existing picture.
11. **Crop** — drag crops, persists across reload, Clear crop restores, sub-2% drags are ignored, and the Media cell has no Rotate.
12. **Promote** — subject pre-fills from the query, is editable, empty subject disables the button, and the emitted draft matches the expected two lines exactly.
13. **Push and graph** — pushing the draft writes the fact and the `has picture` arrow appears in the explorer.
14. **No collection open** — with no collection, picking or fetching reports "no collection open".
15. **Read-only notebook** — no acquisition panel, no toolbar, no delete.
16. **Regression** — existing notebooks with image, audio, sketch, graph, compute, and LLM cells all still render.

- [ ] **Step 5: Commit**

```bash
git add SSTconfig/arrows-EP-3.sst docs/superpowers/specs/2026-07-25-media-cell-slice1-manual-test-plan.md
git commit -m "feat(config): declare 'has picture' arrow; docs: media cell slice 1 test plan"
```

---

## Self-Review

**Spec coverage.** Every section of the design maps to a task: `internal/mediasearch` and both providers → Tasks 2–3; `media.FetchRemote` → Task 1; the three bindings incl. `ProviderInfo{ID,Name,Kinds}` → Task 4; `MediaCell` / `MediaSearchPanel` / `MediaImageView` / kind→component map → Tasks 5–6; the full cell shape incl. reserved `embedURL` → Task 5; `+ Media` and the `+ Scan` relabel with `type` strings unchanged → Task 5; paste-URL as a peer path → Task 7; crop-in / rotate-out → Task 8; `buildMediaFact` and promotion via `onInsert` → Task 9; the `has picture` arrow → Task 10. Error handling is distributed to where it occurs: empty-vs-error states and stale-search cancellation in Task 6, the 403 hotlink message and fetch failures in Tasks 6–7, missing-asset render in Task 6, no-collection in Task 4. Testing matches the spec: Go `httptest` suites in Tasks 1–4, the `buildMediaFact` JS suite in Task 9, the manual plan in Task 10. Nothing in the spec's out-of-scope list appears in any task.

**Type consistency.** `Result` field names are identical across the two providers, both test fixtures, the binding signature, and the Svelte consumers (`r.thumbUrl`, `r.fullUrl` — camelCase per the JSON tags, since Wails marshals through them). `Asset{Name, Mime}` is the existing type, unchanged. `FetchMediaURL` returns `{asset, mime}`, matching `ImportImage` so `onPick` handles both paths identically. `buildMediaFact` emits the arrow **code** `pic`, matching the code declared in Task 10 — N4L's registry is keyed by code, so emitting the long name `has picture` would parse as plain text and create no relation at all.

**One deliberate forward reference:** Task 6 writes `cell.provider = result.provider ?? cell.provider` against a field the panel does not yet send, and Task 7 supplies it. This is called out inline in Task 6 Step 3, and the `??` makes the intermediate state correct rather than merely harmless.

**One fix applied during review:** an earlier draft defined a `clearAsset` helper in Task 7 that no task ever wired to a control. Since `Replace` already covers returning to the acquisition panel, and `onPick` overwrites the asset fields wholesale, the helper was removed rather than given a button to justify it.
