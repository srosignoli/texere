# Media Cell Slice 3 — Video Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add video as the third media kind — searched across three keyless learning-oriented providers, played in-cell from a remote URL, marked with a start time captured from the player, and promoted to an N4L fact that links to that exact moment.

**Architecture:** Three new `Provider` implementations (Wikimedia Commons, PeerTube via SepiaSearch, Internet Archive) plus a small `VideoResolver` interface that turns a search result into a playable file URL lazily, on pick. Nothing is stored locally: the cell keeps a URL in the `embedURL` field slice 1 reserved, and a new `MediaVideoView` renders either our own `<video>` (all three search providers) or a YouTube iframe (pasted links only).

**Tech Stack:** Go 1.22+ (stdlib only), Svelte 3.49, Wails v2.12, `node:test`.

## Global Constraints

- **Pure Go, standard library only.** No CGo, no new dependencies in `go.mod`.
- **gofmt** formatting; imports grouped stdlib / external / internal. `make lint` does **not** catch gofmt violations in this repo, so `gofmt -l` is the authority — an earlier slice was committed unformatted and needed a follow-up fix.
- **Nothing is stored locally for video.** `cell.asset` and `cell.mime` stay empty; `media.FetchRemote` is not called and gains no video kind.
- **Provider IDs are contract, not display labels.** They are persisted on the cell and `buildVideoURL` branches on them, so a mismatch yields a silently wrong URL rather than an error. New: `commons-video`, `peertube`, `archive`. Set by the paste path only: `youtube`, and the pre-existing `url`. Pre-existing elsewhere: `commons`, `openverse`, `pronunciation`, `commons-audio`, `file`.
- **A descriptive User-Agent on every outbound request** — `internal/mediasearch` already has `searchUserAgent`. Go's default is 403'd by Wikimedia.
- **Search results are never persisted onto the cell.**
- **Do not modify** `ImageCell.svelte`, `bake.js`, `TranscribeBar.svelte`, or `AudioCell.svelte`.
- **Image and audio behaviour must not regress.** Both shipped and are in use.
- Run `make test` and `make test-js` before each commit that touches the corresponding language.

---

### Task 1: `VideoResolver` and the Commons video provider

**Files:**
- Modify: `internal/mediasearch/mediasearch.go` (add the `VideoResolver` interface)
- Create: `internal/mediasearch/commonsvideo.go`
- Create: `internal/mediasearch/commonsvideo_test.go`

**Interfaces:**
- Consumes: the existing `Result`, `Provider`, unexported `getJSON(ctx, rawURL, out) error`, `commonsAPI`, and the `llResponse` struct declared in `pronunciation.go`.
- Produces:
  - `VideoResolver` interface with `ResolvePlayURL(ctx context.Context, r Result) (string, error)`.
  - `mediasearch.CommonsVideo{BaseURL string}` implementing both `Provider` and `VideoResolver`, `ID() == "commons-video"`, `Kinds() == []string{"video"}`.

Commons is the only provider that already knows the play URL at search time, so its `ResolvePlayURL` returns `r.FullURL` unchanged. Implementing the interface anyway keeps the call site uniform.

`llResponse` (from the audio slice) already models `imageinfo[0]` with `url`, `descriptionurl`, `mime` and `duration`. Video needs `thumburl` as well, so this task adds that one field to it — harmless for the audio providers, which simply leave it empty.

- [ ] **Step 1: Write the failing test**

Create `internal/mediasearch/commonsvideo_test.go`. The fixture mirrors a real `formatversion=2` response for `photosynthesis filetype:video`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mediasearch/ -run TestCommonsVideo -v`
Expected: FAIL — `undefined: CommonsVideo`.

- [ ] **Step 3: Add `thumburl` to `llResponse` and declare `VideoResolver`**

In `internal/mediasearch/pronunciation.go`, add one field to the `imageinfo` struct inside `llResponse`, leaving every other field untouched:

```go
				ThumbURL       string  `json:"thumburl"`
```

In `internal/mediasearch/mediasearch.go`, add below the `Provider` interface:

```go
// VideoResolver is implemented by video providers whose search results do not
// already carry a playable URL. Resolution is deliberately lazy — SepiaSearch
// returns only a watch page and the Internet Archive only an identifier, so
// resolving every result eagerly would mean dozens of extra requests per
// search instead of one per pick.
type VideoResolver interface {
	ResolvePlayURL(ctx context.Context, r Result) (string, error)
}
```

- [ ] **Step 4: Write the provider**

Create `internal/mediasearch/commonsvideo.go`:

```go
package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// CommonsVideo searches Wikimedia Commons for video — NASA visualisations,
// science animations, classroom explainers. It is the only video provider whose
// search results already contain a playable file URL. BaseURL is injectable for
// tests.
type CommonsVideo struct{ BaseURL string }

func (c CommonsVideo) ID() string      { return "commons-video" }
func (c CommonsVideo) Name() string    { return "Wikimedia Commons" }
func (c CommonsVideo) Kinds() []string { return []string{"video"} }

func (c CommonsVideo) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "video" {
		return nil, fmt.Errorf("mediasearch: commons-video: unsupported kind %q", kind)
	}
	q := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"generator":     {"search"},
		"gsrsearch":     {query + " filetype:video"},
		"gsrnamespace":  {"6"},
		"gsrlimit":      {strconv.Itoa(limit)},
		"prop":          {"imageinfo"},
		"iiprop":        {"url|mime|size"}, // size also yields duration
		"iiurlwidth":    {"320"},           // required, or imageinfo has no thumburl
	}
	var body llResponse
	if err := getJSON(ctx, c.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Query.Pages))
	for _, pg := range body.Query.Pages {
		if len(pg.ImageInfo) == 0 {
			continue // search can return pages whose imageinfo is unavailable
		}
		ii := pg.ImageInfo[0]
		out = append(out, Result{
			Kind:       "video",
			ThumbURL:   ii.ThumbURL,
			FullURL:    ii.URL,
			Title:      strings.TrimPrefix(pg.Title, "File:"),
			SourcePage: ii.DescriptionURL,
			Mime:       ii.Mime,
			DurationMs: int(ii.Duration * 1000),
		})
	}
	return out, nil
}

// ResolvePlayURL returns the URL unchanged: Commons search already supplies a
// direct file URL a <video> element can play.
func (c CommonsVideo) ResolvePlayURL(ctx context.Context, r Result) (string, error) {
	if r.FullURL == "" {
		return "", fmt.Errorf("mediasearch: commons-video: result has no file URL")
	}
	return r.FullURL, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/mediasearch/ -v`
Expected: PASS — the three new tests plus every pre-existing one (the `llResponse` field is additive).

- [ ] **Step 6: Check formatting and commit**

Run: `gofmt -l internal/mediasearch/` — must print nothing.

```bash
git add internal/mediasearch/
git commit -m "feat(mediasearch): VideoResolver + Wikimedia Commons video provider"
```

---

### Task 2: The PeerTube provider (search)

**Files:**
- Create: `internal/mediasearch/peertube.go`
- Create: `internal/mediasearch/peertube_test.go`

**Interfaces:**
- Consumes: `Result`, `Provider`, `getJSON`.
- Produces: `mediasearch.PeerTube{BaseURL string}` implementing `Provider`, `ID() == "peertube"`, `Kinds() == []string{"video"}`. `ResolvePlayURL` arrives in Task 3.

Background the implementer needs. SepiaSearch (`https://sepiasearch.org/api/v1/search/videos`) federates search across PeerTube instances, including education-ministry ones such as `tube-sciences-technologies.apps.education.fr`. Its response is `{"total": N, "data": [...]}` and each entry carries `uuid`, `name`, `duration` (**seconds, integer**), `url` (the full watch page), and — importantly — a **relative** `thumbnailPath` such as `/lazy-static/thumbnails/….jpg`, which must be joined with the instance host from `account.host` to form a loadable URL. Leaving it relative would silently produce broken thumbnails against the app's own origin.

- [ ] **Step 1: Write the failing test**

Create `internal/mediasearch/peertube_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mediasearch/ -run TestPeerTube -v`
Expected: FAIL — `undefined: PeerTube`.

- [ ] **Step 3: Write the provider**

Create `internal/mediasearch/peertube.go`:

```go
package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// sepiaSearchAPI federates search across PeerTube instances, including
// education-ministry ones. Keyless.
const sepiaSearchAPI = "https://sepiasearch.org/api/v1/search/videos"

// PeerTube searches the PeerTube federation through SepiaSearch. Search returns
// only a watch page; the playable file URL is fetched on demand by
// ResolvePlayURL. BaseURL is injectable for tests.
type PeerTube struct{ BaseURL string }

func (p PeerTube) ID() string      { return "peertube" }
func (p PeerTube) Name() string    { return "PeerTube" }
func (p PeerTube) Kinds() []string { return []string{"video"} }

type sepiaResponse struct {
	Data []struct {
		UUID          string `json:"uuid"`
		Name          string `json:"name"`
		Duration      int    `json:"duration"` // seconds
		ThumbnailPath string `json:"thumbnailPath"`
		URL           string `json:"url"`
		Account       struct {
			Host string `json:"host"`
		} `json:"account"`
	} `json:"data"`
}

func (p PeerTube) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "video" {
		return nil, fmt.Errorf("mediasearch: peertube: unsupported kind %q", kind)
	}
	q := url.Values{
		"search": {query},
		"count":  {strconv.Itoa(limit)},
	}
	var body sepiaResponse
	if err := getJSON(ctx, p.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Data))
	for _, v := range body.Data {
		// thumbnailPath is relative to the hosting instance. Without the host it
		// would resolve against the app's own origin and render nothing, so an
		// unknown host yields no thumbnail rather than a broken one.
		thumb := ""
		if v.Account.Host != "" && v.ThumbnailPath != "" {
			thumb = "https://" + v.Account.Host + v.ThumbnailPath
		}
		out = append(out, Result{
			Kind:       "video",
			ThumbURL:   thumb,
			Title:      v.Name,
			SourcePage: v.URL,
			DurationMs: v.Duration * 1000,
			// FullURL stays empty: resolved lazily on pick.
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/mediasearch/ -run TestPeerTube -v`
Expected: PASS — three tests.

- [ ] **Step 5: Check formatting and commit**

Run: `gofmt -l internal/mediasearch/` — must print nothing.

```bash
git add internal/mediasearch/peertube.go internal/mediasearch/peertube_test.go
git commit -m "feat(mediasearch): PeerTube video search via SepiaSearch"
```

---

### Task 3: `PeerTube.ResolvePlayURL`, including HLS-only instances

The trickiest piece in the slice, and the one most likely to fail in the wild.

**Files:**
- Modify: `internal/mediasearch/peertube.go`
- Modify: `internal/mediasearch/peertube_test.go`

**Interfaces:**
- Consumes: `PeerTube` (Task 2), `VideoResolver` (Task 1), `getJSON`.
- Produces: `PeerTube` now also implements `VideoResolver`.

Background. A PeerTube video's detail endpoint is `https://{host}/api/v1/videos/{uuid}`. Both host and uuid are recoverable from the watch URL already in `SourcePage` — `https://diler.tube/videos/watch/<uuid>` — so no new `Result` fields are needed.

The response may carry:

- `files[]`, each with `fileUrl` and `resolution.label` — **progressive files a `<video>` can play directly**; or
- `streamingPlaylists[]` with `playlistUrl` pointing at `.m3u8` — **HLS, which a bare `<video>` cannot play**.

Verified live: `diler.tube` returns two progressive files and no HLS, while the education instance returns three progressive files *and* HLS. But instances configured HLS-only exist, and this slice does not ship `hls.js`. Those must fail with a clear, reportable message rather than an empty string or a panic.

- [ ] **Step 1: Write the failing tests**

Append to `internal/mediasearch/peertube_test.go`:

```go
func TestPeerTubeResolvePicksAProgressiveFile(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{
		  "files": [
		    {"fileUrl": "https://diler.tube/static/web-videos/low.mp4", "resolution": {"id": 480}},
		    {"fileUrl": "https://diler.tube/static/web-videos/high.mp4", "resolution": {"id": 1080}}
		  ],
		  "streamingPlaylists": []
		}`))
	}))
	defer srv.Close()

	p := PeerTube{BaseURL: "http://unused", DetailBase: srv.URL}
	got, err := p.ResolvePlayURL(context.Background(), Result{
		SourcePage: "https://diler.tube/videos/watch/81840642-e10d-4a74-9c02-621061fccbc1",
	})
	if err != nil {
		t.Fatalf("ResolvePlayURL: %v", err)
	}
	// Highest resolution wins: the user picked this video deliberately.
	if got != "https://diler.tube/static/web-videos/high.mp4" {
		t.Errorf("got %q, want the 1080p file", got)
	}
	if !strings.Contains(gotPath, "81840642-e10d-4a74-9c02-621061fccbc1") {
		t.Errorf("path %q should address the video by uuid", gotPath)
	}
}

// Some instances publish only HLS. A bare <video> cannot play .m3u8 and this
// slice ships no hls.js, so this must be a clear, reportable outcome.
func TestPeerTubeResolveReportsHLSOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
		  "files": [],
		  "streamingPlaylists": [{"playlistUrl": "https://x.tube/static/streaming/master.m3u8"}]
		}`))
	}))
	defer srv.Close()

	p := PeerTube{BaseURL: "http://unused", DetailBase: srv.URL}
	got, err := p.ResolvePlayURL(context.Background(), Result{
		SourcePage: "https://x.tube/videos/watch/abc",
	})
	if err == nil {
		t.Fatalf("expected an error for an HLS-only video, got %q", got)
	}
	if !strings.Contains(err.Error(), "no playable file") {
		t.Errorf("error = %q, want it to say there is no playable file", err)
	}
}

func TestPeerTubeResolveRejectsUnusableSourcePage(t *testing.T) {
	p := PeerTube{BaseURL: "http://unused"}
	for _, sp := range []string{"", "not a url", "https://x.tube/"} {
		if _, err := p.ResolvePlayURL(context.Background(), Result{SourcePage: sp}); err == nil {
			t.Errorf("expected an error for sourcePage %q", sp)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/mediasearch/ -run TestPeerTubeResolve -v`
Expected: FAIL — `PeerTube` has no field `DetailBase` and no method `ResolvePlayURL`.

- [ ] **Step 3: Write the implementation**

In `internal/mediasearch/peertube.go`, add `DetailBase` to the struct and the resolver below it. The struct comment is updated to explain the second field:

```go
// PeerTube searches the PeerTube federation through SepiaSearch. Search returns
// only a watch page; the playable file URL is fetched on demand by
// ResolvePlayURL. BaseURL is the search endpoint and DetailBase, when set,
// overrides the per-instance detail endpoint — both are injectable for tests.
type PeerTube struct {
	BaseURL    string
	DetailBase string
}
```

```go
type peertubeDetail struct {
	Files []struct {
		FileURL    string `json:"fileUrl"`
		Resolution struct {
			ID int `json:"id"`
		} `json:"resolution"`
	} `json:"files"`
	StreamingPlaylists []struct {
		PlaylistURL string `json:"playlistUrl"`
	} `json:"streamingPlaylists"`
}

// ResolvePlayURL fetches the video's file list from its own instance and returns
// the highest-resolution progressive file. Host and uuid are recovered from the
// watch URL, so no extra Result fields are needed.
//
// Instances that publish only HLS (.m3u8) cannot be played by a bare <video>
// element and this slice ships no hls.js, so they are reported plainly rather
// than yielding an empty URL that would fail later and further away.
func (p PeerTube) ResolvePlayURL(ctx context.Context, r Result) (string, error) {
	host, uuid, err := peertubeWatchParts(r.SourcePage)
	if err != nil {
		return "", err
	}
	base := p.DetailBase
	if base == "" {
		base = "https://" + host + "/api/v1/videos"
	}
	var d peertubeDetail
	if err := getJSON(ctx, base+"/"+uuid, &d); err != nil {
		return "", fmt.Errorf("mediasearch: peertube: %w", err)
	}
	best, bestRes := "", -1
	for _, f := range d.Files {
		if f.FileURL != "" && f.Resolution.ID > bestRes {
			best, bestRes = f.FileURL, f.Resolution.ID
		}
	}
	if best == "" {
		return "", fmt.Errorf("mediasearch: peertube: no playable file (this instance publishes HLS only)")
	}
	return best, nil
}

// peertubeWatchParts splits a PeerTube watch URL into its instance host and
// video uuid: https://diler.tube/videos/watch/<uuid>
func peertubeWatchParts(watchURL string) (host, uuid string, err error) {
	u, err := url.Parse(watchURL)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("mediasearch: peertube: unusable watch url %q", watchURL)
	}
	id := path.Base(u.Path)
	if id == "" || id == "." || id == "/" || id == "watch" {
		return "", "", fmt.Errorf("mediasearch: peertube: no video id in %q", watchURL)
	}
	return u.Host, id, nil
}
```

Extend the import block with `"path"`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/mediasearch/ -run TestPeerTube -v`
Expected: PASS — six tests, including the three from Task 2.

- [ ] **Step 5: Check formatting and commit**

Run: `gofmt -l internal/mediasearch/` — must print nothing.

```bash
git add internal/mediasearch/peertube.go internal/mediasearch/peertube_test.go
git commit -m "feat(mediasearch): resolve PeerTube play URLs; report HLS-only instances"
```

---

### Task 4: The Internet Archive provider

**Files:**
- Create: `internal/mediasearch/archive.go`
- Create: `internal/mediasearch/archive_test.go`

**Interfaces:**
- Consumes: `Result`, `Provider`, `VideoResolver`, `getJSON`.
- Produces: `mediasearch.InternetArchive{BaseURL, MetadataBase string}` implementing both, `ID() == "archive"`, `Kinds() == []string{"video"}`.

Background. Search is `https://archive.org/advancedsearch.php` with `output=json`, returning `{"response": {"numFound": N, "docs": [{identifier, title}]}}`.

The query **must** include `AND NOT collection:tvarchive`. Verified live: without it the TV News Archive dominates completely — 418 hits, all news captures — and with it 256 remain, including UC Berkeley lecture webcasts. This is load-bearing, not tidying.

Thumbnails come from `https://archive.org/services/img/<identifier>` (verified 200). The watch page is `https://archive.org/details/<identifier>`.

Resolution fetches `https://archive.org/metadata/<identifier>`, whose `files[]` entries carry `name` and `format` (`"h.264"`, `"WebM"`, `"MPEG4"`, `"512Kb MPEG4"`, `"Ogg Video"`). The play URL is `https://archive.org/download/<identifier>/<name>`, which answers 302 to a CDN node — fine for a `<video>` element, which follows redirects.

- [ ] **Step 1: Write the failing test**

Create `internal/mediasearch/archive_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mediasearch/ -run TestInternetArchive -v`
Expected: FAIL — `undefined: InternetArchive`.

- [ ] **Step 3: Write the provider**

Create `internal/mediasearch/archive.go`:

```go
package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strconv"
)

const (
	archiveSearchAPI   = "https://archive.org/advancedsearch.php"
	archiveMetadataAPI = "https://archive.org/metadata"
)

// InternetArchive searches the Internet Archive's moving-image collections —
// university lecture webcasts, public-domain film, science material. BaseURL and
// MetadataBase are injectable for tests.
//
// Its corpus is uneven even after filtering (the top hit for "photosynthesis" is
// a YouTube mirror); that is a characteristic of the source, not a defect.
type InternetArchive struct {
	BaseURL      string
	MetadataBase string
}

func (i InternetArchive) ID() string      { return "archive" }
func (i InternetArchive) Name() string    { return "Internet Archive" }
func (i InternetArchive) Kinds() []string { return []string{"video"} }

type archiveSearchResponse struct {
	Response struct {
		Docs []struct {
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
		} `json:"docs"`
	} `json:"response"`
}

func (i InternetArchive) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "video" {
		return nil, fmt.Errorf("mediasearch: archive: unsupported kind %q", kind)
	}
	q := url.Values{
		// The tvarchive exclusion is load-bearing: without it the TV News
		// Archive dominates the results entirely.
		"q":      {query + " AND mediatype:movies AND NOT collection:tvarchive"},
		"rows":   {strconv.Itoa(limit)},
		"output": {"json"},
	}
	q["fl[]"] = []string{"identifier", "title"}

	var body archiveSearchResponse
	if err := getJSON(ctx, i.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Response.Docs))
	for _, d := range body.Response.Docs {
		if d.Identifier == "" {
			continue
		}
		out = append(out, Result{
			Kind:       "video",
			ThumbURL:   "https://archive.org/services/img/" + d.Identifier,
			Title:      d.Title,
			SourcePage: "https://archive.org/details/" + d.Identifier,
			// FullURL stays empty: resolved lazily on pick.
		})
	}
	return out, nil
}

type archiveMetadata struct {
	Files []struct {
		Name   string `json:"name"`
		Format string `json:"format"`
	} `json:"files"`
}

// archivePlayableFormats are the metadata `format` values a <video> element can
// play, in preference order.
var archivePlayableFormats = []string{"h.264", "MPEG4", "512Kb MPEG4", "WebM", "Ogg Video"}

// ResolvePlayURL fetches the item's file list and returns a download URL for the
// most preferred playable format. archive.org/download answers 302 to a CDN
// node, which a <video> element follows.
func (i InternetArchive) ResolvePlayURL(ctx context.Context, r Result) (string, error) {
	id, err := archiveIdentifier(r.SourcePage)
	if err != nil {
		return "", err
	}
	base := i.MetadataBase
	if base == "" {
		base = archiveMetadataAPI
	}
	var m archiveMetadata
	if err := getJSON(ctx, base+"/"+id, &m); err != nil {
		return "", fmt.Errorf("mediasearch: archive: %w", err)
	}
	for _, want := range archivePlayableFormats {
		for _, f := range m.Files {
			if f.Format == want && f.Name != "" {
				return "https://archive.org/download/" + id + "/" + f.Name, nil
			}
		}
	}
	return "", fmt.Errorf("mediasearch: archive: no playable file for %q", id)
}

// archiveIdentifier extracts the item id from a details page URL:
// https://archive.org/details/<identifier>
func archiveIdentifier(detailsURL string) (string, error) {
	u, err := url.Parse(detailsURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("mediasearch: archive: unusable details url %q", detailsURL)
	}
	id := path.Base(u.Path)
	if id == "" || id == "." || id == "/" || id == "details" {
		return "", fmt.Errorf("mediasearch: archive: no identifier in %q", detailsURL)
	}
	return id, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/mediasearch/ -run TestInternetArchive -v`
Expected: PASS — four tests.

- [ ] **Step 5: Check formatting and commit**

Run: `gofmt -l internal/mediasearch/` — must print nothing.

```bash
git add internal/mediasearch/archive.go internal/mediasearch/archive_test.go
git commit -m "feat(mediasearch): Internet Archive video provider"
```

---

### Task 5: Registry wiring and the resolve binding

**Files:**
- Modify: `internal/mediasearch/registry.go`
- Modify: `internal/mediasearch/registry_test.go`
- Modify: `media_methods.go`
- Modify: `media_methods_test.go`

**Interfaces:**
- Consumes: `CommonsVideo` (Task 1), `PeerTube` (Tasks 2–3), `InternetArchive` (Task 4), `VideoResolver` (Task 1).
- Produces:
  - `Providers()` returning **seven** providers.
  - `(a *App) ResolveVideoPlayURL(providerID string, r mediasearch.Result) (string, error)`.

Note the pre-existing `TestProvidersCoverBothKinds` asserts `len(ps) != 4` and will fail once three more are registered; update its count and extend its ID list. Similarly `TestListMediaProvidersReturnsBoth` in `media_methods_test.go` hardcodes a provider count.

- [ ] **Step 1: Write the failing tests**

In `internal/mediasearch/registry_test.go`, replace `TestProvidersCoverBothKinds` with:

```go
func TestProvidersCoverAllThreeKinds(t *testing.T) {
	ps := Providers()
	if len(ps) != 7 {
		t.Fatalf("got %d providers, want 7", len(ps))
	}
	byID := map[string]Provider{}
	for _, p := range ps {
		if p.Name() == "" || len(p.Kinds()) == 0 {
			t.Errorf("provider %q is incompletely described", p.ID())
		}
		byID[p.ID()] = p
	}
	for _, id := range []string{
		"commons", "openverse", "pronunciation", "commons-audio",
		"commons-video", "peertube", "archive",
	} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing provider %q", id)
		}
	}
	// Openverse's audio endpoint indexes music, so it stays image-only.
	if k := byID["openverse"].Kinds(); len(k) != 1 || k[0] != "image" {
		t.Errorf("openverse Kinds = %v, want [image] only", k)
	}
	// Every video provider must be resolvable, or picking one cannot produce a
	// playable URL.
	for _, id := range []string{"commons-video", "peertube", "archive"} {
		if _, ok := byID[id].(VideoResolver); !ok {
			t.Errorf("provider %q does not implement VideoResolver", id)
		}
	}
}
```

Add to `media_methods_test.go`:

```go
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
```

Ensure `media_methods_test.go` imports `"texere/internal/mediasearch"`. Update `TestListMediaProvidersReturnsBoth`'s expected count from 4 to 7.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/mediasearch/ ./ -run 'TestProviders|TestResolveVideo|TestListMediaProviders' -v`
Expected: FAIL — `Providers()` returns 4, and `ResolveVideoPlayURL` is undefined.

- [ ] **Step 3: Wire the registry**

Replace `Providers()` in `internal/mediasearch/registry.go`:

```go
// Providers returns the live providers in display order. All seven are keyless.
// Keyed providers (YouTube search, Forvo, Pexels) arrive in a later slice and
// will be configured through the LLMProfile pattern rather than hardcoded here.
func Providers() []Provider {
	return []Provider{
		Commons{BaseURL: commonsAPI},
		Openverse{BaseURL: openverseAPI},
		Pronunciation{BaseURL: commonsAPI},
		CommonsAudio{BaseURL: commonsAPI},
		CommonsVideo{BaseURL: commonsAPI},
		PeerTube{BaseURL: sepiaSearchAPI},
		InternetArchive{BaseURL: archiveSearchAPI},
	}
}
```

- [ ] **Step 4: Add the binding**

In `media_methods.go`:

```go
// ResolveVideoPlayURL turns a chosen video search result into a URL a <video>
// element can play. It is called once per pick rather than for every result,
// because PeerTube and the Internet Archive each need an extra request.
func (a *App) ResolveVideoPlayURL(providerID string, r mediasearch.Result) (string, error) {
	p, ok := mediasearch.Find(providerID)
	if !ok {
		return "", fmt.Errorf("unknown media provider %q", providerID)
	}
	vr, ok := p.(mediasearch.VideoResolver)
	if !ok {
		return "", fmt.Errorf("provider %q cannot resolve video URLs", providerID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchCallTimeout)
	defer cancel()
	return vr.ResolvePlayURL(ctx, r)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `make test`
Expected: all packages PASS.

- [ ] **Step 6: Regenerate the Wails bindings**

Run: `wails generate module` (the binary is at `$(go env GOPATH)/bin/wails` if not on `PATH`).

Expected: `App.d.ts` and `App.js` gain `ResolveVideoPlayURL`. Note `frontend/wailsjs/` matches a gitignore directory pattern although its files are tracked, so stage with `git add -u frontend/wailsjs/...` if `git add <dir>` is refused.

- [ ] **Step 7: Check formatting and commit**

Run: `gofmt -l internal/mediasearch/ .` — must print nothing for the files you touched.

```bash
git add internal/mediasearch/ media_methods.go media_methods_test.go
git add -u frontend/wailsjs/
git commit -m "feat(media): register video providers + ResolveVideoPlayURL binding"
```

---

### Task 6: Video facts in `media-fact.js`

**Files:**
- Modify: `frontend/src/lib/media-fact.js`
- Modify: `frontend/src/lib/media-fact.test.js`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces, all exported from `frontend/src/lib/media-fact.js`:
  - `mediaArrow({kind, provider})` — now returns `vid` for video.
  - `buildVideoURL({sourcePage, provider, startSec})` — a timestamped watch URL.
  - `buildMediaFact(cell)` — a two-line fact for video.
  - `hasContent(cell)` — `cell.asset || cell.embedURL`.

`hasContent` exists because `MediaCell` currently decides it is "filled" with `cell.asset`, which is never set for video; without it the video cell would be stuck on the acquisition panel forever. Task 7 consumes it.

Each provider spells time differently — verified shapes: Commons `#t=223`, PeerTube `?start=223`, Archive `?start=223`, YouTube `&t=223s`. All four survive the N4L parser with the target preserved exactly.

- [ ] **Step 1: Write the failing tests**

Append to `frontend/src/lib/media-fact.test.js`, and extend its import to
`import { buildMediaFact, mediaArrow, buildVideoURL, hasContent } from './media-fact.js'`:

```js
test('video uses the vid arrow', () => {
  assert.equal(mediaArrow({ kind: 'video', provider: 'peertube' }), 'vid')
  assert.equal(mediaArrow({ kind: 'video', provider: 'youtube' }), 'vid')
})

test('buildVideoURL appends each provider its own time parameter', () => {
  assert.equal(
    buildVideoURL({ sourcePage: 'https://diler.tube/videos/watch/abc', provider: 'peertube', startSec: 223 }),
    'https://diler.tube/videos/watch/abc?start=223')
  assert.equal(
    buildVideoURL({ sourcePage: 'https://archive.org/details/x', provider: 'archive', startSec: 223 }),
    'https://archive.org/details/x?start=223')
  assert.equal(
    buildVideoURL({ sourcePage: 'https://commons.wikimedia.org/wiki/File:x.webm', provider: 'commons-video', startSec: 223 }),
    'https://commons.wikimedia.org/wiki/File:x.webm#t=223')
  assert.equal(
    buildVideoURL({ sourcePage: 'https://www.youtube.com/watch?v=abc', provider: 'youtube', startSec: 223 }),
    'https://www.youtube.com/watch?v=abc&t=223s')
})

test('a URL that already has a query gets & rather than a second ?', () => {
  assert.equal(
    buildVideoURL({ sourcePage: 'https://x.tube/w?a=1', provider: 'peertube', startSec: 5 }),
    'https://x.tube/w?a=1&start=5')
})

test('no time parameter is appended when the start is zero', () => {
  assert.equal(
    buildVideoURL({ sourcePage: 'https://x.tube/w', provider: 'peertube', startSec: 0 }),
    'https://x.tube/w')
  assert.equal(
    buildVideoURL({ sourcePage: 'https://x.tube/w', provider: 'peertube' }),
    'https://x.tube/w')
})

test('buildVideoURL returns empty for a missing source page', () => {
  assert.equal(buildVideoURL({ sourcePage: '', provider: 'peertube', startSec: 5 }), '')
  assert.equal(buildVideoURL({}), '')
})

test('a video fact is the link plus a readable note line', () => {
  assert.equal(
    buildMediaFact({
      subject: 'photosynthesis', kind: 'video', provider: 'peertube',
      sourcePage: 'https://diler.tube/videos/watch/abc',
      title: 'Dr. Binocs Show — Photosynthesis', startSec: 223,
    }),
    'photosynthesis\n"  (vid) https://diler.tube/videos/watch/abc?start=223\n' +
    '"  (note) Dr. Binocs Show — Photosynthesis (3:43)\n')
})

test('the note line carries no time when the start is zero', () => {
  assert.equal(
    buildMediaFact({
      subject: 'photosynthesis', kind: 'video', provider: 'peertube',
      sourcePage: 'https://diler.tube/videos/watch/abc', title: 'Explainer', startSec: 0,
    }),
    'photosynthesis\n"  (vid) https://diler.tube/videos/watch/abc\n"  (note) Explainer\n')
})

test('an untitled video emits no note line rather than an empty one', () => {
  assert.equal(
    buildMediaFact({
      subject: 'photosynthesis', kind: 'video', provider: 'url',
      sourcePage: 'https://example.org/v.mp4', title: '', startSec: 0,
    }),
    'photosynthesis\n"  (vid) https://example.org/v.mp4\n')
})

test('an hour-long start time is clocked as h:mm:ss', () => {
  assert.equal(
    buildMediaFact({
      subject: 'x', kind: 'video', provider: 'peertube',
      sourcePage: 'https://x.tube/w', title: 'Long lecture', startSec: 3723,
    }),
    'x\n"  (vid) https://x.tube/w?start=3723\n"  (note) Long lecture (1:02:03)\n')
})

test('a video with no source page yields no fact', () => {
  assert.equal(
    buildMediaFact({ subject: 'x', kind: 'video', provider: 'peertube', sourcePage: '' }), '')
})

test('hasContent covers both stored assets and referenced URLs', () => {
  assert.equal(hasContent({ asset: 'a3.jpg' }), true)
  assert.equal(hasContent({ embedURL: 'https://x.tube/w' }), true)
  assert.equal(hasContent({ asset: '', embedURL: '' }), false)
  assert.equal(hasContent({}), false)
  assert.equal(hasContent(null), false)
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test-js`
Expected: FAIL — `buildVideoURL` and `hasContent` are not exported, and the video cases produce a `pic` fact.

- [ ] **Step 3: Write the implementation**

Replace `frontend/src/lib/media-fact.js`:

```js
// Builds the N4L text promoted from a media cell. Kept as pure functions so they
// are unit-testable without mounting Svelte.
//
// Stored kinds (image, audio) reference a collection-relative asset path
// (`media/<hash>.<ext>`), so a collection stays portable. Video is referenced
// rather than stored, so it points at the provider's watch page — a durable,
// shareable URL, unlike the CDN file URLs used for playback, which rotate.

// N4L references arrows by their short CODE, not their long name. These are the
// tokens declared in SSTconfig/arrows-EP-3.sst:
//   pic = "has picture"   pron = "has pronunciation"
//   aud = "has audio"     vid  = "has video"
const PICTURE_ARROW = 'pic'
const PRONUNCIATION_ARROW = 'pron'
const AUDIO_ARROW = 'aud'
const VIDEO_ARROW = 'vid'

// mediaArrow picks the arrow for a cell. Provenance chooses only a DEFAULT in
// editable text — the promoted draft is an N4L cell the user is about to read —
// so a wrong guess costs one keystroke rather than hiding anything.
// `kind` is absent on cells saved before audio existed, and means image.
export function mediaArrow({ kind, provider } = {}) {
  if (kind === 'video') return VIDEO_ARROW
  if (kind !== 'audio') return PICTURE_ARROW
  return provider === 'pronunciation' ? PRONUNCIATION_ARROW : AUDIO_ARROW
}

// hasContent reports whether a media cell holds anything yet. Stored kinds set
// `asset`; video sets only `embedURL`, so testing `asset` alone would leave a
// video cell permanently showing its acquisition panel.
export function hasContent(cell) {
  return Boolean(cell?.asset || cell?.embedURL)
}

// buildVideoURL appends the provider's own time parameter to a watch page.
// Each spells it differently, and all four shapes are known to survive the N4L
// parser with the target preserved exactly.
export function buildVideoURL({ sourcePage, provider, startSec } = {}) {
  const url = (sourcePage ?? '').trim()
  if (!url) return ''
  const t = Math.max(0, Math.round(startSec ?? 0))
  if (!t) return url
  if (provider === 'commons-video') return `${url}#t=${t}`
  const sep = url.includes('?') ? '&' : '?'
  if (provider === 'youtube') return `${url}${sep}t=${t}s`
  return `${url}${sep}start=${t}`
}

// clock renders seconds as m:ss, or h:mm:ss past an hour.
function clock(sec) {
  const s = Math.max(0, Math.round(sec))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

export function buildMediaFact(cell) {
  const subject = (cell?.subject ?? '').trim()
  if (!subject) return ''

  if (cell.kind === 'video') {
    const url = buildVideoURL(cell)
    if (!url) return ''
    let out = `${subject}\n"  (${VIDEO_ARROW}) ${url}\n`
    // A bare URL is unreadable months later, so a title — when there is one —
    // goes on a note line. An empty note line would be worse than none.
    const title = (cell.title ?? '').trim()
    if (title) {
      const t = Math.max(0, Math.round(cell.startSec ?? 0))
      out += `"  (note) ${t ? `${title} (${clock(t)})` : title}\n`
    }
    return out
  }

  if (!cell.asset) return ''
  return `${subject}\n"  (${mediaArrow(cell)}) media/${cell.asset}\n`
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS — the new video tests plus every pre-existing image and audio assertion, unchanged.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/media-fact.js frontend/src/lib/media-fact.test.js
git commit -m "feat(notebook): video facts — vid arrow, timestamped URL, note line"
```

---

### Task 7: The video cell shape and `MediaCell` wiring

**Files:**
- Modify: `frontend/src/lib/notebook.js`
- Modify: `frontend/src/components/notebook/MediaCell.svelte`

**Interfaces:**
- Consumes: `hasContent`, `mediaArrow`, `buildMediaFact` (Task 6).
- Produces: `newMediaCell()` gains the video fields; `MediaCell` exposes `onStart(sec)` and passes `startSec` to the view registered in Task 8.

- [ ] **Step 1: Extend the cell shape**

In `frontend/src/lib/notebook.js`, add the video fields to the object returned by `newMediaCell()`, beside `embedURL`:

```js
    asset: '', mime: '', embedURL: '',
    playerKind: '',    // 'file' | 'iframe'; video only
    title: '',         // video only; drives the promoted (note) line
    startSec: 0,       // video only; the marked moment
```

`embedURL` already exists and was reserved for exactly this; `durationMs` is already present from the audio slice.

- [ ] **Step 2: Use `hasContent` for the filled test**

In `frontend/src/components/notebook/MediaCell.svelte`, extend the import:

```js
  import { buildMediaFact, mediaArrow, hasContent } from '../../lib/media-fact.js'
```

Change the filled/empty branch so a video cell — which never sets `asset` — is not stuck on the acquisition panel:

```svelte
  {#if !hasContent(cell) || replacing}
```

and the Promote guard, so an untitled video with a subject can still promote:

```svelte
        <button class="me-btn" on:click={promote} disabled={!cell.subject.trim() || !hasContent(cell)}
```

- [ ] **Step 3: Clear the video fields when the asset changes**

In `onPick`, add the video fields to the existing reset block so a replaced asset cannot inherit a stale timestamp or title. Place these beside the existing `cell.crop = null` reset:

```js
    cell.embedURL = result.embedURL ?? ''
    cell.playerKind = result.playerKind ?? ''
    cell.title = result.title ?? ''
    cell.startSec = 0
    cell.durationMs = result.durationMs ?? 0
```

- [ ] **Step 4: Add the start-time handler and pass it down**

Add to the script:

```js
  function onStart(sec) {
    cell.startSec = Math.max(0, Math.round(sec || 0))
    cell = cell
    persist()
  }
```

and extend the rendered view so the video component receives what it needs:

```svelte
    <svelte:component this={views[cell.kind]} {cell} {cropping} {onMeta} {onStart} onCrop={applyCrop} />
```

`MediaImageView` and `MediaAudioView` will now receive an `onStart` prop they do not declare. Add `export let onStart = () => {}` to **both**, beside their existing unused shared props, so Svelte does not warn — matching how those files already declare `cropping`/`onCrop`/`onMeta` they do not use.

- [ ] **Step 5: Verify the build**

Run: `cd frontend && npx vite build` — must succeed. Then `make test-js` from the repo root — must pass.

The GUI walkthrough belongs to the manual pass in Task 10.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/notebook.js frontend/src/components/notebook/MediaCell.svelte frontend/src/components/notebook/media/
git commit -m "feat(notebook): video cell fields; hasContent gates the filled state"
```

---

### Task 8: `MediaVideoView`

**Files:**
- Create: `frontend/src/components/notebook/media/MediaVideoView.svelte`
- Modify: `frontend/src/components/notebook/MediaCell.svelte` (register `views.video`)

**Interfaces:**
- Consumes: `mediaErrorMessage(err, what)` from `frontend/src/lib/media-error.js` (audio slice); `onStart(sec)` (Task 7).
- Produces: `views.video = MediaVideoView`.

Do **not** re-implement the `MediaError` wording — import it. It exists so the wording cannot drift between players.

Two render paths. `playerKind === 'file'` uses our own `<video>` with a **remote** `src`, which is what makes the timestamp button possible (`currentTime` is readable) and what sidesteps the Linux local-playback bug. `playerKind === 'iframe'` is a pasted YouTube link, where the position cannot be read without YouTube's player API — so the **Set start here** button is absent there rather than present and broken.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/notebook/media/MediaVideoView.svelte`:

```svelte
<script>
  import { mediaErrorMessage } from '../../../lib/media-error.js'

  export let cell
  export let onStart = () => {}
  export let cropping = false  // unused; images only
  export let onCrop = () => {} // unused; images only
  export let onMeta = () => {} // unused; video reports no intrinsic size

  let videoEl
  let playErr = ''

  $: isFile = cell.playerKind !== 'iframe'
  // youtube-nocookie avoids setting tracking cookies until playback starts.
  $: embedSrc = cell.playerKind === 'iframe'
    ? `https://www.youtube-nocookie.com/embed/${youtubeId(cell.embedURL)}${cell.startSec ? `?start=${cell.startSec}` : ''}`
    : ''

  // Accepts both watch?v=<id> and youtu.be/<id> forms.
  function youtubeId(url) {
    const m = String(url ?? '').match(/(?:youtube\.com\/.*[?&]v=|youtu\.be\/)([A-Za-z0-9_-]{6,})/)
    return m ? m[1] : ''
  }

  // Resume at the marked moment, so reopening a cell lands where it left off.
  function onLoaded() {
    playErr = ''
    if (cell.startSec && videoEl && Math.abs(videoEl.currentTime - cell.startSec) > 1) {
      videoEl.currentTime = cell.startSec
    }
  }

  function onVideoError(e) {
    // Referencing rather than storing means a removed upstream video is a real
    // outcome; say so rather than showing an empty frame.
    playErr = mediaErrorMessage(e.target?.error, 'Video')
  }

  function setStartHere() {
    if (videoEl) onStart(videoEl.currentTime)
  }
</script>

<div class="vv-wrap">
  {#if playErr}
    <div class="vv-err">{playErr}</div>
  {/if}

  {#if isFile}
    <video bind:this={videoEl} class="vv-player" src={cell.embedURL} controls preload="metadata"
      on:loadedmetadata={onLoaded} on:error={onVideoError}></video>
  {:else if embedSrc}
    <iframe class="vv-player" src={embedSrc} title={cell.title || 'video'}
      allow="fullscreen; encrypted-media" frameborder="0"></iframe>
  {:else}
    <div class="vv-err">Not a recognised video link: {cell.embedURL}</div>
  {/if}

  <div class="vv-tools">
    {#if isFile}
      <button class="vv-btn" on:click={setStartHere} title="Mark the moment currently playing">
        Set start here
      </button>
    {/if}
    <span class="vv-start">start {cell.startSec || 0}s</span>
  </div>
</div>

<style>
  .vv-wrap { padding: 8px; }
  .vv-player { width: 100%; aspect-ratio: 16 / 9; background: #000; border: 0; border-radius: 4px; display: block; }
  .vv-tools { display: flex; gap: 8px; align-items: center; margin-top: 6px; }
  .vv-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 2px 8px; cursor: pointer; }
  .vv-btn:hover { background: #24334a; }
  .vv-start { font-size: 11px; color: #8aa; }
  .vv-err { padding: 8px 0; color: #d88; font-size: 13px; }
</style>
```

- [ ] **Step 2: Register it**

In `frontend/src/components/notebook/MediaCell.svelte`:

```js
  import MediaVideoView from './media/MediaVideoView.svelte'
```

```js
  const views = { image: MediaImageView, audio: MediaAudioView, video: MediaVideoView }
```

- [ ] **Step 3: Verify the build**

Run: `cd frontend && npx vite build` — must succeed. Then `make test-js` — must pass.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/notebook/media/MediaVideoView.svelte frontend/src/components/notebook/MediaCell.svelte
git commit -m "feat(notebook): MediaVideoView with set-start-here and YouTube embed"
```

---

### Task 9: Video in the search panel

**Files:**
- Modify: `frontend/src/components/notebook/MediaSearchPanel.svelte`

**Interfaces:**
- Consumes: `ResolveVideoPlayURL` from `wailsjs/go/main/App` (Task 5); the existing `stopPreview()`, `acqGen` guard, and `onPick(result, ref)` contract.
- Produces: nothing later tasks depend on.

Four changes: a Video button on the kind toggle, tiles carrying a duration badge, resolution on pick, and YouTube recognition on paste. **"Import file…" is hidden for video** — this slice stores nothing locally.

The existing `onPick(result, ref)` signature is kept: for video, `ref` carries `{embedURL, playerKind}` in place of `{asset, mime}`, which is what Task 7's `onPick` already reads.

- [ ] **Step 1: Add the Video kind button and hide file import**

Extend the import:

```js
  import { ListMediaProviders, ListLanguages, SearchMedia, FetchMediaURL, ImportImage, ImportAudio, ResolveVideoPlayURL } from '../../../wailsjs/go/main/App'
```

Add the third kind button beside Image and Audio:

```svelte
    <button class="ms-btn" class:ms-on={kind === 'video'}
      on:click={() => setKind('video')} disabled={busy}>Video</button>
```

Gate the file-import control, leaving the paste field visible for every kind:

```svelte
    {#if kind !== 'video'}
      <button class="ms-btn" on:click={importFile} disabled={busy}>Import file…</button>
    {/if}
```

- [ ] **Step 2: Resolve the play URL on pick**

Replace `pick()` so video resolves before handing the cell its content. The `fetchingURL` single-flight lock, the `acqGen` guard, and the `stopPreview()` call are all preserved exactly:

```js
  async function pick(r) {
    if (fetchingURL) return
    stopPreview()
    fetchingURL = r.fullUrl || r.sourcePage
    err = ''
    const mine = ++acqGen
    try {
      if (kind === 'video') {
        // Resolved here rather than during search: PeerTube and the Internet
        // Archive each need an extra request, which would be dozens per search.
        const playURL = await ResolveVideoPlayURL(providerId, r)
        if (mine !== acqGen) return
        onPick({ ...r, provider: providerId, playerKind: 'file' }, { embedURL: playURL })
      } else {
        const ref = await FetchMediaURL(r.fullUrl, kind)
        if (mine !== acqGen) return
        onPick({ ...r, provider: providerId }, ref)
      }
    } catch (e) {
      if (mine !== acqGen) return
      const msg = String(e?.message ?? e)
      err = msg.includes('403')
        ? `${msg} — this host blocks direct downloads. Open the source page and use Paste URL with the real image address.`
        : msg
    } finally {
      fetchingURL = ''
    }
  }
```

- [ ] **Step 3: Recognise YouTube and direct video links on paste**

Replace `fetchPasted()` so a pasted link is referenced rather than downloaded when the kind is video:

```js
  const YOUTUBE_RE = /(?:youtube\.com\/.*[?&]v=|youtu\.be\/)([A-Za-z0-9_-]{6,})/

  async function fetchPasted() {
    const u = pasteURL.trim()
    if (!u) return
    stopPreview()
    err = ''
    busy = true
    acqGen++ // supersede any in-flight acquisition
    try {
      if (kind === 'video') {
        // Video is referenced, never stored, so nothing is fetched here — which
        // is exactly why the scheme is checked: this string becomes a <video>
        // src or an iframe target unexamined otherwise. Same discipline as
        // openSource() in MediaCell, and it also catches the common mistake of
        // pasting a title or a file path.
        if (!/^https?:\/\//i.test(u)) {
          err = 'Paste an http(s) video or YouTube URL.'
          return
        }
        const isYT = YOUTUBE_RE.test(u)
        onPick(
          { kind, title: '', sourcePage: u, license: '', provider: isYT ? 'youtube' : 'url',
            playerKind: isYT ? 'iframe' : 'file' },
          { embedURL: u })
        return
      }
      // Bound only on the path that actually awaits and therefore needs it.
      const mine = acqGen
      const ref = await FetchMediaURL(u, kind)
      if (mine !== acqGen) return
      onPick({ kind, title: '', sourcePage: u, license: '', provider: 'url' }, ref)
    } catch (e) {
      err = String(e?.message ?? e)
    } finally { busy = false }
  }
```

- [ ] **Step 4: Render video results as tiles with a duration badge**

Extend the results markup so video gets its own branch beside the existing audio rows and image tiles:

```svelte
    {:else if kind === 'video'}
      <div class="ms-grid ms-vgrid">
        {#each results as r (r.sourcePage)}
          <button class="ms-hit ms-vhit" class:ms-loading={fetchingURL === (r.fullUrl || r.sourcePage)}
            on:click={() => pick(r)} title={r.title}>
            {#if r.thumbUrl}<img src={r.thumbUrl} alt={r.title} loading="lazy" />{/if}
            {#if r.durationMs}<span class="ms-dur">{fmtDuration(r.durationMs)}</span>{/if}
            <span class="ms-vtitle">{r.title}</span>
          </button>
        {/each}
      </div>
```

`fmtDuration` already exists from the audio slice and renders `671000` as `671.0s`. Video wants a clock, so add a second helper beside it and use it in the badge:

```js
  // Video durations are minutes long, so a clock reads better than seconds.
  function fmtClock(ms) {
    const s = Math.round((ms || 0) / 1000)
    if (!s) return ''
    const m = Math.floor(s / 60)
    return `${m}:${String(s % 60).padStart(2, '0')}`
  }
```

and use `fmtClock(r.durationMs)` in the badge rather than `fmtDuration`.

Add the styles. **Placement matters and is not cosmetic:** `.ms-hit` sets
`aspect-ratio: 1` and `.ms-hit img` sets `height: 100%`, and the new rules have
*equal specificity*, so the later declaration wins. Put this block **after**
every existing `.ms-hit` rule (currently the last is `.ms-hit img`, around line
285) or the video tiles will silently render as squares with stretched
thumbnails. This exact mistake — a rule declared before the one it meant to
override, and therefore dead — already shipped once in this component with
`.ms-on`.

```css
  .ms-vgrid { grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); }
  .ms-vhit { position: relative; aspect-ratio: auto; display: flex; flex-direction: column; align-items: stretch; padding: 0 0 4px; }
  .ms-vhit img { width: 100%; aspect-ratio: 16 / 9; object-fit: cover; display: block; }
  .ms-dur { position: absolute; right: 4px; top: 4px; background: rgba(0,0,0,0.75); color: #fff; font-size: 10px; padding: 1px 4px; border-radius: 3px; }
  .ms-vtitle { padding: 4px 6px 0; font-size: 11px; color: #cde; text-align: left; line-height: 1.25; max-height: 2.6em; overflow: hidden; }
```

- [ ] **Step 5: Verify the build**

Run: `cd frontend && npx vite build` — must succeed. Then `make test-js` — must pass.

Re-read your diff and confirm: the image and audio branches are unchanged; `stopPreview()` still runs first in both `pick` and `fetchPasted`; and the `acqGen` guard is intact on every await.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/notebook/MediaSearchPanel.svelte
git commit -m "feat(notebook): video kind, result tiles, resolve-on-pick, YouTube paste"
```

---

### Task 10: Arrow declaration, live verification, and the manual test plan

**Files:**
- Modify: `SSTconfig/arrows-EP-3.sst`
- Create: `docs/superpowers/specs/2026-07-28-media-cell-slice3-manual-test-plan.md`

**Interfaces:**
- Consumes: the arrow code `vid` emitted by `mediaArrow` (Task 6). They must match exactly.

- [ ] **Step 1: Declare the arrow**

In `SSTconfig/arrows-EP-3.sst`, in the `:: attributes, supporting information ::` section beside `has picture`, `has pronunciation` and `has audio`, add:

```
 + has video         (vid)          - is video for                   (vid-for)
```

Match the column alignment of the neighbouring lines rather than trusting the spacing above — the file is read by hand.

`vid` and `vid-for` were both verified free against the loaded registry. Do **not** use `url`/`isurl`: line 185 reserves them in a comment.

- [ ] **Step 2: Verify the promoted fact parses into a real arrow**

The unit tests only compare strings, which is exactly what let an equivalent defect ship in slice 1. Confirm with a temporary Go test in `internal/content`:

```go
package content

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTempVideoFactParses(t *testing.T) {
	reg, err := LoadRegistry("../../SSTconfig")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "v.n4l")
	body := "- v\n\nphotosynthesis\n\"  (vid) https://diler.tube/videos/watch/abc?start=223\n" +
		"\"  (note) Dr. Binocs Show (3:43)\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, perrs := ParseFile(p, reg)
	t.Logf("parseErrors=%d", len(perrs))
	for _, ch := range doc.Chapters {
		for _, n := range ch.Nodes {
			for _, d := range n.Dittos {
				for _, a := range d.Arrows {
					t.Logf("NODE %q ARROW code=%q target=%q registered=%v",
						n.Text, a.Code, a.Target, reg.Arrows[a.Code].Code != "")
				}
			}
		}
	}
}
```

Run it. Confirm **both** ditto lines appear — `code="vid"` with the full timestamped URL as its target and `registered=true`, and `code="note"` likewise. Paste the output into your report, then **delete the temporary file** and confirm `git status` is clean.

- [ ] **Step 3: Live-API check with production parameters**

Slice 1 shipped a broken provider because its verification used different parameters from the app. Use `limit` 24, as the panel does:

```bash
UA="texere/1.0 (+https://github.com/srosignoli/texere) media-search"
curl -s -A "$UA" --get --data-urlencode 'gsrsearch=photosynthesis filetype:video' \
  'https://commons.wikimedia.org/w/api.php?action=query&format=json&formatversion=2&generator=search&gsrnamespace=6&gsrlimit=24&prop=imageinfo&iiprop=url|mime|size&iiurlwidth=320' | head -c 400
curl -s -A "$UA" 'https://sepiasearch.org/api/v1/search/videos?search=photosynthesis&count=24' | head -c 400
curl -s -A "$UA" --get --data-urlencode 'q=photosynthesis AND mediatype:movies AND NOT collection:tvarchive' \
  --data-urlencode 'fl[]=identifier' --data-urlencode 'fl[]=title' --data-urlencode 'rows=24' --data-urlencode 'output=json' \
  'https://archive.org/advancedsearch.php' | head -c 400
```

Confirm each returns results, that Commons entries carry `thumburl` and `duration`, that SepiaSearch entries carry `thumbnailPath` and `account.host`, and that the Archive response contains no `tvarchive` identifiers.

- [ ] **Step 4: Run the full verification suite**

```bash
make test
make test-js
make build
gofmt -l internal/ *.go
```

Expected: Go PASS, JS PASS, build exit 0. `gofmt -l` must name no file this slice created or modified — pre-existing findings in `internal/content`, `internal/explorer`, `internal/search` and `internal/source` are not yours. Also run `make lint` for information and report only whether any finding names a file from this slice.

- [ ] **Step 5: Write the manual test plan**

Create `docs/superpowers/specs/2026-07-28-media-cell-slice3-manual-test-plan.md`, following the structure of `docs/superpowers/specs/2026-07-27-media-cell-slice2-manual-test-plan.md` (a Prereqs line, then `## TC-N Title` with numbered steps and `EXPECT:` lines).

Every expectation must be specific enough to **fail** against, and must describe the feature **as built** — in slice 2 two cases asserted behaviour the code did not have and had to be corrected. Where unsure what the UI does, read the component rather than guessing.

**TC-1 must be the platform risk**, because everything else depends on it:

1. **Remote video plays at all.** Add a Media cell, switch to Video, search Commons for `photosynthesis`, pick a result. EXPECT: the video plays in the cell. If it fails with a `MediaError`, **stop** — the slice's core assumption is wrong, since it deliberately avoids local storage to route around the known Linux local-playback limitation, and there is no fallback. Record the exact error and halt the pass.

Then:

2. **Kind toggle** — Video joins Image and Audio; the selected one is visibly distinct; switching clears stale results.
3. **Provider list for video** — exactly Wikimedia Commons, PeerTube and Internet Archive; the image and audio providers are absent.
4. **Result tiles** — thumbnails render for all three providers (PeerTube's especially, since its path is relative and must be joined with the instance host), each with a duration badge and title.
5. **PeerTube pick** — resolves and plays; an HLS-only instance reports "no playable file" rather than a blank player.
6. **Archive pick** — resolves and plays; note its corpus is uneven and the top hit may be a YouTube mirror.
7. **Set start here** — scrub to a moment, click, the displayed start updates; reload the notebook and the cell resumes at that moment.
8. **Promote** — the draft is exactly two lines: `(vid)` with the timestamped watch URL, and `(note)` with the title and clock time. An untitled video emits only the `(vid)` line.
9. **Push and graph** — pushing writes the fact, and the explorer shows a `has video` edge resolved to its long name, not a synthesized unknown arrow.
10. **YouTube paste** — a `youtube.com/watch?v=…` link embeds and plays; **Set start here is absent**; a typed start time is honoured in the promoted URL as `&t=<n>s`.
11. **Non-YouTube paste** — a direct `.mp4`/`.webm` URL plays in the native player with Set start here present.
12. **Import file is hidden for video** and still present for Image and Audio.
13. **Dead link** — an unreachable video URL reports a load failure rather than a blank frame.
14. **Regression** — image search, pick, crop and `Promote to N4L (pic)` behave as before.
15. **Regression** — audio search, preview, pick and `(pron)`/`(aud)` promotion behave as before; the Scan cell and the separate Audio cell are unaffected.
16. **Regression** — existing notebooks containing image and audio media cells still render.

- [ ] **Step 6: Commit**

```bash
git add SSTconfig/arrows-EP-3.sst docs/superpowers/specs/2026-07-28-media-cell-slice3-manual-test-plan.md
git commit -m "feat(config): declare has video arrow; docs: slice 3 manual test plan"
```

---

## Self-Review

**Spec coverage.** Every spec section maps to a task: the three providers → Tasks 1, 2, 4; lazy `ResolvePlayURL` and the `VideoResolver` interface → Tasks 1–4; HLS-only reporting → Task 3; provider IDs as contract → Tasks 1, 2, 4 and Task 6's `buildVideoURL`; registry and binding → Task 5; the cell shape including `embedURL`/`playerKind`/`startSec`/`title` → Task 7; `hasContent` closing the `asset`-gated filled test → Tasks 6–7; `MediaVideoView` with both render paths and the seek-to-start → Task 8; **Set start here** present for files and absent for iframes → Task 8; result tiles with duration badges → Task 9; paste-URL YouTube recognition → Task 9; hidden file import → Task 9; the `vid` arrow and two-line fact → Tasks 6 and 10; the platform risk as TC-1 → Task 10. Error handling is distributed: HLS-only in Task 3, no-playable-file in Task 4, unknown/non-video provider in Task 5, `mediaErrorMessage` reuse and dead links in Task 8. Nothing from the spec's out-of-scope list appears — no YouTube search, no key configuration, no `hls.js`, no local storage, no looping.

**Type consistency.** `Result` field names are identical across all three providers, their fixtures, and the Svelte consumers (`r.thumbUrl`, `r.sourcePage`, `r.durationMs`, `r.fullUrl` — camelCase per the JSON tags, since Wails marshals through them). Provider IDs `commons-video`, `peertube`, `archive` are used identically in the providers, Task 5's registry test, and Task 6's `buildVideoURL`; `youtube` and `url` are set only by Task 9's paste path and consumed only by `buildVideoURL`. `ResolvePlayURL(ctx, Result) (string, error)` matches between the interface, all three implementations, and the binding. `hasContent` and `buildVideoURL` are defined in Task 6 and consumed in Tasks 7 and 9. `mediaArrow`'s `vid` matches the `SSTconfig` declaration in Task 10.

**One trap verified during review.** Task 9's video tiles override `.ms-hit`'s
`aspect-ratio: 1` and `.ms-hit img`'s `height: 100%` at equal specificity, so
the new rules are dead unless declared after them. That is the same failure that
made `.ms-on` invisible earlier in this very component, so the step now states
the required placement rather than leaving it to chance. Also confirmed: the
existing `fmtDuration` renders `671000` as `671.0s`, which is why video needs
its own `fmtClock`; and both `MediaImageView` and `MediaAudioView` already
declare their unused shared props, so `onStart` must join them.

**Two deliberate cross-task couplings, called out where they bite.** Task 7 adds an `onStart` prop that `MediaCell` passes to *every* view, so Task 7 also declares it as an unused prop on `MediaImageView` and `MediaAudioView` — stated in its own step rather than left to a console warning. And Task 5 must fix two pre-existing tests that hardcode a provider count (`TestProvidersCoverBothKinds`, `TestListMediaProvidersReturnsBoth`), which is stated explicitly because the same omission cost a review cycle in slice 2.
