# Incremental Reading — Phase 2c (PDF & Web Adapters) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two source media behind the existing reading-element/extract model — read PDFs in a visual pdf.js pane (page anchors) and import web articles by URL into a rich, image-preserving sanitized-HTML reader (char anchors) — completing "read anything."

**Architecture:** A `medium` column (schema v4) on `reading_elements` drives a `ReadingView` refactor into a shell with pluggable source panes (Text/Pdf/Web). The extract binding is generalized from an int char offset to a `position` string (`char=N` | `page=N`) so panes pick their own anchor unit. Web import fetches → go-readability → bluemonday-sanitizes → inlines images as `data:` URIs → saves a self-contained HTML file; PDF bytes are served to pdf.js via a base64 binding.

**Tech Stack:** Go 1.25, `modernc.org/sqlite`, `github.com/go-shiori/go-readability`, `github.com/microcosm-cc/bluemonday` (both pure Go), Wails v2, Svelte 3.49, `pdfjs-dist` (already a dep), CodeMirror 6.

## Global Constraints

- Pure Go, no CGo. New deps: `go-shiori/go-readability` + `microcosm-cc/bluemonday`. learning.db migrates additively 3→4.
- Per-medium anchors: text/web use `char=N` (rune offset); PDF uses `page=N`. The `(from)` anchor is `"<refPath> <position>"`; `# @progress` is the position string. The extract/scheduling machinery treats the anchor as opaque.
- **Web security (hard requirement):** the saved/rendered web HTML must contain NO `<script>`, NO `on*` handlers, NO `<iframe>`/`<object>`/`<style>`/`<link>`, and NO remote resource references — images are inlined as `data:` URIs; everything else is stripped by bluemonday BEFORE the file is written. The reader renders it inert (plain element, not a scripted iframe).
- Web fetch uses a timeout + a max response size; an import that fails leaves nothing partial behind.
- `medium ∈ {'text','web','pdf'}`; existing rows default to `'text'`. The original URL is stored on the reading element (`source_url`) and written into the notes `# @source`.
- Go: `gofmt`, grouped imports. Tests `go test ./...`; frontend `cd frontend && npm run build` + manual quickstart.

---

## File structure

| File | Responsibility |
|---|---|
| `internal/learning/db.go` + `reading.go` (+`_test`) | schema v4 (`medium`,`source_url`) + `ReadingElement.Medium`/`SourceURL` + CRUD/scan. |
| `internal/source/web.go` (+`_test`) | `FetchAndExtract(url)` → sanitized, image-inlined article. |
| `reading_methods.go` | `ImportPdfSource`, `ImportWebSource`, `GetSourceBytes`, `OpenSourceURL`; generalize `CreateExtract` to a `position` string; `ReadingView` view gains `medium`/`sourceUrl`. |
| `frontend/src/components/ReadingView.svelte` | shell: dispatch source pane by `medium`; shared notes editor + extract/progress/reschedule. |
| `frontend/src/components/TextSourcePane.svelte` | the 2a text rendering + char-offset selection (extracted). |
| `frontend/src/components/PdfSourcePane.svelte` | pdf.js visual reader + page selection. |
| `frontend/src/components/WebSourcePane.svelte` | sanitized-HTML render + char selection + "Open original". |
| `frontend/src/components/SourcesPanel.svelte` | Import PDF + Import Web (URL) buttons. |

---

### Task 1: Schema v4 — `medium` + `source_url` on reading elements

**Files:**
- Modify: `internal/learning/db.go` (`schemaVersion = 4`, `case 3: ddlV4`, add `ddlV4`)
- Modify: `internal/learning/reading.go` (`ReadingElement.Medium`/`SourceURL`; `UpsertReading` + `scanReading` + the SELECT column lists)
- Test: `internal/learning/reading_test.go` (add a medium round-trip case)

**Interfaces:**
- Produces: `ReadingElement` gains `Medium string` and `SourceURL string`. `UpsertReading` persists them on insert; `scanReading` reads them; `GetReading`/`ListReading`/`DueReading` SELECTs include the two new columns (appended last). Default on existing rows: `medium='text'`, `source_url=''`.

- [ ] **Step 1: Bump schema + add v4 DDL**

In `internal/learning/db.go`: change `const schemaVersion = 3` to `= 4`; add a `case 3` in the migrate loop:

```go
		case 3:
			if _, err := s.db.Exec(ddlV4); err != nil {
				return fmt.Errorf("learning: apply v4: %w", err)
			}
```

Add next to `ddlV3`:

```go
const ddlV4 = `
ALTER TABLE reading_elements ADD COLUMN medium TEXT NOT NULL DEFAULT 'text';
ALTER TABLE reading_elements ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
`
```

- [ ] **Step 2: Write the failing test**

Add to `internal/learning/reading_test.go`:

```go
func TestReadingMediumAndSourceURLRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := ReadingElement{ID: "w1", RefPath: "sources/a.html", NotesPath: "/c/reading/a.n4l",
		Title: "A", IntervalDays: 1, Medium: "web", SourceURL: "https://example.com/a"}
	if _, err := s.UpsertReading(r); err != nil {
		t.Fatalf("UpsertReading: %v", err)
	}
	got, ok, err := s.GetReading("w1")
	if err != nil || !ok {
		t.Fatalf("GetReading ok=%v err=%v", ok, err)
	}
	if got.Medium != "web" || got.SourceURL != "https://example.com/a" {
		t.Fatalf("got Medium=%q SourceURL=%q", got.Medium, got.SourceURL)
	}
	// A row inserted without a medium defaults to 'text'.
	r2 := ReadingElement{ID: "t1", RefPath: "sources/b.txt", NotesPath: "/c/reading/b.n4l", Title: "B", IntervalDays: 1}
	s.UpsertReading(r2)
	g2, _, _ := s.GetReading("t1")
	if g2.Medium != "text" {
		t.Fatalf("default medium=%q want text", g2.Medium)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestReadingMediumAndSourceURLRoundTrip -v`
Expected: FAIL — `unknown field Medium` (struct) / no such column.

- [ ] **Step 4: Add the fields + persist/scan them**

In `internal/learning/reading.go`:
1. Add to the `ReadingElement` struct (after `Done bool`):
```go
	Medium    string // 'text' | 'web' | 'pdf'
	SourceURL string // original URL for web; "" otherwise
```
2. In `UpsertReading`'s INSERT, add `medium`/`source_url` columns + values. Change the insert to:
```go
	_, err := s.db.Exec(`INSERT INTO reading_elements
		(id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done, created_at, medium, source_url)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.RefPath, r.NotesPath, r.Title, r.Position, r.ProjLen,
		nullableUnix(r.Due), maxInt(r.IntervalDays, 1), nullableUnix(r.LastRead), boolToInt(r.Done), time.Now().Unix(),
		defaultStr(r.Medium, "text"), r.SourceURL)
```
   Add a tiny helper near the bottom of `reading.go`:
```go
func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
```
   (Leave the existing-id UPDATE path as-is — display fields only; medium/source_url are set at insert and not changed on re-upsert.)
3. Append `medium, source_url` to the SELECT column lists in `GetReading`, `ListReading`, **and** `DueReading` (in `queue.go`), and extend `scanReading` to scan them last:
```go
	// in scanReading, add to the var block and Scan args (last two), then:
	r.Medium = medium.String
	r.SourceURL = srcURL.String
```
   using `var medium, srcURL sql.NullString` and appending `&medium, &srcURL` to the `Scan(...)` call. Update every SELECT that feeds `scanReading` to end with `, medium, source_url`.

> Read `reading.go`'s `scanReading` and all three SELECTs (plus `queue.go`'s `DueReading`) and append the two columns consistently — a column/Scan mismatch will fail at runtime.

- [ ] **Step 5: Run the test + whole package**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestReadingMediumAndSourceURLRoundTrip -v && go test ./internal/learning/`
Expected: PASS (new + all prior).

- [ ] **Step 6: Commit**

```bash
git add internal/learning/db.go internal/learning/reading.go internal/learning/queue.go internal/learning/reading_test.go
git commit -m "feat(learning): reading_elements medium + source_url (schema v4)"
```

---

### Task 2: `source.FetchAndExtract` — fetch, Readability, sanitize, inline images

**Files:**
- Create: `internal/source/web.go`
- Test: `internal/source/web_test.go`

**Interfaces:**
- Produces:
  - `type WebArticle struct { Title, HTML, URL string }`
  - `func FetchAndExtract(url string) (WebArticle, error)` — GET the URL (10s timeout, 10 MB cap) → go-readability article → **bluemonday sanitize** → for each `<img>` download the image (2s timeout, 2 MB/image cap; on failure drop the `<img>`) and rewrite its `src` to a `data:` URI → return `{Title, sanitized self-contained HTML, original URL}`. The returned HTML contains no `<script>` and no remote `src`.

- [ ] **Step 1: Add the dependencies**

Run: `cd /home/simone/projects/texere && go get github.com/go-shiori/go-readability@latest && go get github.com/microcosm-cc/bluemonday@latest && go mod tidy`
Expected: both modules added; no error.

- [ ] **Step 2: Write the failing test**

Create `internal/source/web_test.go`:

```go
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
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/source/ -run TestFetchAndExtract -v`
Expected: FAIL — `undefined: FetchAndExtract`.

- [ ] **Step 4: Write `web.go`**

Create `internal/source/web.go`:

```go
package source

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	readability "github.com/go-shiori/go-readability"
	"github.com/microcosm-cc/bluemonday"
)

// WebArticle is a fetched, sanitized, self-contained article.
type WebArticle struct {
	Title string
	HTML  string
	URL   string
}

const (
	webFetchTimeout = 10 * time.Second
	webMaxBytes     = 10 << 20 // 10 MB
	imgFetchTimeout = 2 * time.Second
	imgMaxBytes     = 2 << 20 // 2 MB per image
)

// FetchAndExtract fetches url, extracts the readable article, sanitizes it, and
// inlines images as data: URIs so the result references no remote resources.
func FetchAndExtract(rawURL string) (WebArticle, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return WebArticle{}, fmt.Errorf("source: bad url: %w", err)
	}
	client := &http.Client{Timeout: webFetchTimeout}
	resp, err := client.Get(rawURL)
	if err != nil {
		return WebArticle{}, fmt.Errorf("source: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WebArticle{}, fmt.Errorf("source: fetch %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, webMaxBytes))
	if err != nil {
		return WebArticle{}, fmt.Errorf("source: read body: %w", err)
	}

	article, err := readability.FromReader(bytes.NewReader(body), parsed)
	if err != nil {
		return WebArticle{}, fmt.Errorf("source: readability: %w", err)
	}

	inlined := inlineImages(article.Content, client)
	policy := bluemonday.UGCPolicy()
	policy.AllowDataURIImages()
	safe := policy.Sanitize(inlined)

	return WebArticle{Title: strings.TrimSpace(article.Title), HTML: safe, URL: rawURL}, nil
}

// inlineImages downloads each <img src> and rewrites it to a data: URI; on any
// failure (or oversize) it drops the src so sanitization removes the <img>.
func inlineImages(htmlStr string, client *http.Client) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}
	imgClient := &http.Client{Timeout: imgFetchTimeout}
	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		src, ok := s.Attr("src")
		if !ok || strings.HasPrefix(src, "data:") {
			return
		}
		data, mime, err := fetchImage(imgClient, src)
		if err != nil {
			s.RemoveAttr("src")
			return
		}
		s.SetAttr("src", "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
	})
	if h, err := doc.Find("body").Html(); err == nil {
		return h
	}
	return htmlStr
}

func fetchImage(client *http.Client, src string) ([]byte, string, error) {
	resp, err := client.Get(src)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, imgMaxBytes+1))
	if err != nil || len(data) > imgMaxBytes {
		return nil, "", fmt.Errorf("image too large or unreadable")
	}
	mime := resp.Header.Get("Content-Type")
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", fmt.Errorf("not an image: %s", mime)
	}
	return data, mime, nil
}
```

> Verify the pinned API against `go get`: `go-readability`'s `FromReader(io.Reader, *url.URL) (Article, error)` and `Article.Title`/`.Content`; `bluemonday.UGCPolicy()`/`.AllowDataURIImages()`/`.Sanitize(string)`; and that `goquery` (a transitive dep of go-readability — confirm it resolves; if not, `go get github.com/PuerkitoBio/goquery`). If `img src` is a relative URL, resolve it against `parsed` before fetching (use `parsed.ResolveReference`). Adapt the wrapper to the real APIs; the test assertions are the behavioral contract.

- [ ] **Step 5: Run the test + commit**

Run: `cd /home/simone/projects/texere && go test ./internal/source/ -run TestFetchAndExtract -v && go test ./internal/source/`
Expected: PASS.

```bash
git add internal/source/web.go internal/source/web_test.go go.mod go.sum
git commit -m "feat(source): FetchAndExtract — fetch + readability + sanitize + inline images"
```

---

### Task 3: Bindings — PDF/web import, bytes, open-url; generalize CreateExtract

**Files:**
- Modify: `reading_methods.go`
- Modify: `frontend/src/components/ReadingView.svelte` (one-line call update to keep the build green)
- Modify: `frontend/wailsjs/go/main/App.js` + `App.d.ts`
- Test: `reading_methods_test.go`

**Interfaces:**
- Produces (App methods):
  - `ImportPdfSource() (*ReadingView, error)` — file dialog (`*.pdf`) → copy → scaffold → register `medium='pdf'`, position `page=1`.
  - `ImportWebSource(url string) (*ReadingView, error)` — `source.FetchAndExtract` → save `sources/<slug>.html` → scaffold (title + URL) → register `medium='web'`, `source_url=url`, position `char=0`.
  - `GetSourceBytes(refPath string) (string, error)` — base64 of the source file (for pdf.js).
  - `OpenSourceURL(id string) error` — `runtime.BrowserOpenURL(source_url)`.
  - `SetReadingProjLen(id string, n int) error` — store total pages/length reported by a reader.
  - **Generalized** `CreateExtract(id, text, position string) (*ExtractView, error)` — anchor = `"<refPath> <position>"`; `# @progress` = `position`. (Was `anchorChar int`.)
  - `ReadingView` struct gains `Medium string` and `SourceURL string` (json `medium`/`sourceUrl`); `toReadingView` populates them.

- [ ] **Step 1: Write the failing test**

Add to `reading_methods_test.go`:

```go
func TestImportWebAndCreateExtractPosition(t *testing.T) {
	// Local server with a small article.
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>WT</title></head><body><article><h1>WT</h1>
			<p>A sufficiently long paragraph of article text so readability keeps it as the main content of this page here.</p>
			</article></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	store, _ := learning.Open(dir)
	defer store.Close()
	a := &App{collectionDir: dir, learnStore: store, registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{"from": {Code: "from", LongName: "from"}}}}

	rv, err := a.ImportWebSource(srv.URL + "/a")
	if err != nil || rv == nil {
		t.Fatalf("ImportWebSource rv=%v err=%v", rv, err)
	}
	if rv.Medium != "web" || rv.SourceURL != srv.URL+"/a" {
		t.Fatalf("medium=%q sourceUrl=%q", rv.Medium, rv.SourceURL)
	}
	// The saved html file exists and is script-free.
	html, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rv.RefPath)))
	if strings.Contains(string(html), "<script") {
		t.Fatalf("saved web html not sanitized")
	}
	// CreateExtract with a position string writes that exact anchor.
	if _, err := a.CreateExtract(rv.ID, "A long paragraph", "char=5"); err != nil {
		t.Fatalf("CreateExtract: %v", err)
	}
	notes, _ := os.ReadFile(rv.NotesPath)
	if !strings.Contains(string(notes), `char=5`) {
		t.Fatalf("extract anchor missing char=5:\n%s", notes)
	}
}
```

(Add imports `net/http`, `net/http/httptest`, `strings` to the test file as needed.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test . -run TestImportWebAndCreateExtractPosition -v`
Expected: FAIL — `a.ImportWebSource undefined`.

- [ ] **Step 3: Generalize `CreateExtract` + add the view fields**

In `reading_methods.go`:
1. Add `Medium`/`SourceURL` to the `ReadingView` struct and populate in `toReadingView`:
```go
	Medium    string `json:"medium"`
	SourceURL string `json:"sourceUrl"`
```
```go
	// in toReadingView return:
	Medium: r.Medium, SourceURL: r.SourceURL,
```
2. Change `CreateExtract` signature from `(id, text string, anchorChar int)` to `(id, text, position string)`, and replace the anchor/progress construction:
```go
	anchor := fmt.Sprintf("%s %s", r.RefPath, position)
	...
	err = content.UpdateProgressMarker(r.NotesPath, position)
	...
	_ = a.learnStore.SetReadingProgress(id, position, r.ProjLen)
```
   (Remove the old `parseCharAnchor`-based `anchorChar` formatting; keep the rest — register sync, `file:ids-injected` emit — unchanged. `parseCharAnchor` is still used by `toReadingView` for `Percent`; leave it.)

- [ ] **Step 4: Add the new import/bytes/url bindings**

Append to `reading_methods.go`:

```go
// ImportPdfSource copies a chosen PDF into sources/ and registers a reading element.
func (a *App) ImportPdfSource() (*ReadingView, error) {
	if a.learnStore == nil || a.collectionDir == "" {
		return nil, errors.New("no collection open")
	}
	picked, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import a PDF source",
		Filters: []runtime.FileFilter{{DisplayName: "PDF", Pattern: "*.pdf"}},
	})
	if err != nil || picked == "" {
		return nil, err
	}
	base := filepath.Base(picked)
	srcRel, srcAbs := uniqueDest(a.collectionDir, "sources", base)
	if err := copyFile(picked, srcAbs); err != nil {
		return nil, fmt.Errorf("ImportPdfSource: copy: %w", err)
	}
	title := strings.TrimSuffix(filepath.Base(srcRel), filepath.Ext(srcRel))
	_, notesAbs := uniqueDest(a.collectionDir, "reading", title+".n4l")
	refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
	if err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("ImportPdfSource: notes: %w", err)
	}
	r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
		Position: "page=1", IntervalDays: 1, Medium: "pdf"}
	if err := a.learnStore.UpsertReading(r); err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("ImportPdfSource: register: %w", err)
	}
	v := toReadingView(r)
	return &v, nil
}

// ImportWebSource fetches a URL, saves a sanitized self-contained article, and registers it.
func (a *App) ImportWebSource(srcURL string) (*ReadingView, error) {
	if a.learnStore == nil || a.collectionDir == "" {
		return nil, errors.New("no collection open")
	}
	art, err := source.FetchAndExtract(srcURL)
	if err != nil {
		return nil, fmt.Errorf("ImportWebSource: %w", err)
	}
	title := art.Title
	if title == "" {
		title = "web article"
	}
	slug := slugify(title)
	srcRel, srcAbs := uniqueDest(a.collectionDir, "sources", slug+".html")
	if err := os.WriteFile(srcAbs, []byte(art.HTML), 0o644); err != nil {
		return nil, fmt.Errorf("ImportWebSource: save: %w", err)
	}
	_, notesAbs := uniqueDest(a.collectionDir, "reading", slug+".n4l")
	refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
	if err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("ImportWebSource: notes: %w", err)
	}
	proj, _ := source.Projection(srcAbs) // article text length for % progress
	r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
		Position: "char=0", ProjLen: len([]rune(proj)), IntervalDays: 1, Medium: "web", SourceURL: art.URL}
	if err := a.learnStore.UpsertReading(r); err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("ImportWebSource: register: %w", err)
	}
	v := toReadingView(r)
	return &v, nil
}

// slugify makes a filesystem-safe lowercase slug from a title.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "article"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

// GetSourceBytes returns the base64 of a source file (for pdf.js).
func (a *App) GetSourceBytes(refPath string) (string, error) {
	if a.collectionDir == "" {
		return "", errors.New("no collection open")
	}
	data, err := os.ReadFile(filepath.Join(a.collectionDir, filepath.FromSlash(refPath)))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// OpenSourceURL opens the reading element's original URL in the system browser.
func (a *App) OpenSourceURL(id string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return fmt.Errorf("OpenSourceURL: %q not found", id)
	}
	if r.SourceURL == "" {
		return errors.New("no original URL")
	}
	runtime.BrowserOpenURL(a.ctx, r.SourceURL)
	return nil
}

// SetReadingProjLen stores a reader-reported total length / page count.
func (a *App) SetReadingProjLen(id string, n int) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return fmt.Errorf("SetReadingProjLen: %q not found", id)
	}
	return a.learnStore.SetReadingProgress(id, r.Position, n)
}
```

Add `"encoding/base64"` and `"texere/internal/source"` to the imports (source is likely already imported for `source.Projection`).

- [ ] **Step 5: Keep the frontend build green**

In `frontend/src/components/ReadingView.svelte`, the existing `extract()` calls `CreateExtract(reading.id, sel.text, sel.offset)` (an int). Change it to pass a `char=` position string:
```js
      await CreateExtract(reading.id, sel.text, 'char=' + sel.offset)
```
(This is a stopgap so the build stays green; Task 4 moves this into the shell.)

- [ ] **Step 6: Run the test + build**

Run: `cd /home/simone/projects/texere && go test . -run TestImportWebAndCreateExtractPosition -v && go build ./... && go test ./...`
Expected: PASS + clean build + suite green.

- [ ] **Step 7: Hand-add JS bindings**

Append to `frontend/wailsjs/go/main/App.js` + `App.d.ts` (`Promise<any>`): `ImportPdfSource()`, `ImportWebSource(arg1)`, `GetSourceBytes(arg1)`, `OpenSourceURL(arg1)`, `SetReadingProjLen(arg1,arg2)`. (`CreateExtract` arity is unchanged — its existing 3-arg binding still applies.)

- [ ] **Step 8: Verify frontend builds + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0.

```bash
git add reading_methods.go reading_methods_test.go frontend/src/components/ReadingView.svelte frontend/wailsjs/go/main/App.js frontend/wailsjs/go/main/App.d.ts
git commit -m "feat(reading): PDF/web import, GetSourceBytes, OpenSourceURL; generalize CreateExtract to position string"
```

---

### Task 4: ReadingView pluggable-pane shell + TextSourcePane

**Files:**
- Create: `frontend/src/components/TextSourcePane.svelte`
- Modify: `frontend/src/components/ReadingView.svelte`

**Interfaces:**
- The **source-pane contract** (all three panes implement it): props `reading` (the ReadingView object); dispatches Svelte events `extract` (`detail = { text, position }`, `position` a string like `char=10`/`page=3`) and `progress` (`detail = { position }`). The pane owns its own rendering, selection→position, and resume.
- `ReadingView` becomes the shell: renders the notes editor (unchanged) + the pane chosen by `reading.medium` (`'pdf'`→PdfSourcePane, `'web'`→WebSourcePane, else TextSourcePane); on a pane `extract` event it flushes notes then `CreateExtract(reading.id, text, position)` + reloads notes; on `progress` it calls `UpdateReadingProgress(reading.id, position)`.

- [ ] **Step 1: Create `TextSourcePane.svelte`** (extract the 2a text rendering)

Create `frontend/src/components/TextSourcePane.svelte`:

```svelte
<script>
  import { onMount, createEventDispatcher } from 'svelte'
  import { GetSourceProjection } from '../../wailsjs/go/main/App'

  export let reading
  const dispatch = createEventDispatcher()

  let projection = ''
  let srcEl
  let error = ''

  function charOffset() {
    const sel = window.getSelection()
    if (!sel || sel.rangeCount === 0 || !srcEl.contains(sel.anchorNode)) return null
    const range = sel.getRangeAt(0)
    const pre = range.cloneRange()
    pre.selectNodeContents(srcEl)
    pre.setEnd(range.startContainer, range.startOffset)
    return { offset: [...pre.toString()].length, text: sel.toString() }
  }

  export function requestExtract() {
    const sel = charOffset()
    if (!sel || !sel.text.trim()) return
    dispatch('extract', { text: sel.text, position: 'char=' + sel.offset })
  }

  onMount(async () => {
    try { projection = (await GetSourceProjection(reading.refPath)) ?? '' }
    catch (e) { error = String(e?.message ?? e) }
  })
</script>

<div class="tsp" bind:this={srcEl}>{#if error}<span class="tsp-err">{error}</span>{:else}{projection}{/if}</div>

<style>
  .tsp { flex: 1; overflow: auto; padding: 16px 20px; white-space: pre-wrap; color: #cdd6e0;
    font-size: 14px; line-height: 1.6; user-select: text; }
  .tsp-err { color: #ff8080; }
</style>
```

- [ ] **Step 2: Refactor `ReadingView.svelte` into the shell**

In `ReadingView.svelte`: remove the inline `{projection}` source div + its `charOffset`/`GetSourceProjection` logic, and instead render the chosen pane. Replace the source-pane markup and the `extract()` function:

1. Imports: add
```js
  import TextSourcePane from './TextSourcePane.svelte'
  import PdfSourcePane from './PdfSourcePane.svelte'
  import WebSourcePane from './WebSourcePane.svelte'
  import { UpdateReadingProgress } from '../../wailsjs/go/main/App'
```
2. Add a pane ref + handlers; replace the old `extract()` with an event-driven one:
```js
  let pane
  async function onPaneExtract(e) {
    const { text, position } = e.detail
    try {
      await flushNotes()
      await CreateExtract(reading.id, text, position)
      await reloadNotes()
    } catch (err) { error = String(err?.message ?? err) }
  }
  async function onPaneProgress(e) {
    try { await UpdateReadingProgress(reading.id, e.detail.position) } catch { /* best-effort */ }
  }
  // The Ctrl+E / Extract button asks the active pane to emit an extract.
  function triggerExtract() { pane?.requestExtract?.() }
```
   Update the keyboard handler's Ctrl+E branch and the "✂ Extract" button to call `triggerExtract()` instead of the old `extract()`.
3. In the markup, replace `<div class="reading-source" ...>{projection}</div>` with the medium dispatch:
```svelte
    <div class="reading-source">
      {#if reading.medium === 'pdf'}
        <PdfSourcePane bind:this={pane} {reading} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {:else if reading.medium === 'web'}
        <WebSourcePane bind:this={pane} {reading} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {:else}
        <TextSourcePane bind:this={pane} {reading} on:extract={onPaneExtract} on:progress={onPaneProgress} />
      {/if}
    </div>
```
   (`.reading-source` keeps `flex:1; display:flex; min-width:0;` so the pane fills it.)

> READ the current `ReadingView.svelte` first; keep the notes editor, `flushNotes`/`reloadNotes`/`close`/Later/Done logic intact — only the LEFT source area and the extract trigger change. `PdfSourcePane`/`WebSourcePane` are created in Tasks 5/6; importing them now is fine (they must exist for the build — create empty stubs if you implement this task before 5/6, but per task order they come after; if building 4 alone, add minimal stub components that render nothing, to be filled in 5/6). Prefer implementing 4→5→6 in order so stubs aren't needed.

- [ ] **Step 3: Create minimal stubs for Pdf/Web panes (so Task 4 builds in isolation)**

Create `frontend/src/components/PdfSourcePane.svelte` and `frontend/src/components/WebSourcePane.svelte` each with:
```svelte
<script>
  export let reading
  export function requestExtract() {}
</script>
<div class="pane-stub">{reading?.medium} pane — coming up</div>
<style>.pane-stub{flex:1;display:flex;align-items:center;justify-content:center;color:#8aa;}</style>
```
(Tasks 5 and 6 replace these.)

- [ ] **Step 4: Verify build + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0. The text-medium reading path works exactly as before (now via TextSourcePane).

```bash
git add frontend/src/components/TextSourcePane.svelte frontend/src/components/PdfSourcePane.svelte frontend/src/components/WebSourcePane.svelte frontend/src/components/ReadingView.svelte
git commit -m "refactor(reading): ReadingView shell + pluggable source panes (Text + Pdf/Web stubs)"
```

---

### Task 5: PdfSourcePane — visual pdf.js reader

**Files:**
- Modify: `frontend/src/components/PdfSourcePane.svelte` (replace the stub)

**Interfaces:**
- Implements the source-pane contract: `export function requestExtract()`; dispatches `extract`/`progress`. Uses `GetSourceBytes(reading.refPath)` → pdf.js render; page nav; resume at `reading.position` (`page=N`); reports total pages via `SetReadingProjLen`. Selection → `{ text, position: 'page=' + currentPage }`.

- [ ] **Step 1: Replace the stub with the pdf.js reader**

Replace `frontend/src/components/PdfSourcePane.svelte`:

```svelte
<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import * as pdfjsLib from 'pdfjs-dist'
  import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
  import { GetSourceBytes, SetReadingProjLen } from '../../wailsjs/go/main/App'

  pdfjsLib.GlobalWorkerOptions.workerSrc = workerUrl

  export let reading
  const dispatch = createEventDispatcher()

  let pdf = null
  let pageNum = 1
  let total = 0
  let error = ''
  let canvas
  let textLayerEl

  function parsePage(pos) {
    const m = /page=(\d+)/.exec(pos || '')
    return m ? parseInt(m[1], 10) : 1
  }

  function base64ToBytes(b64) {
    const bin = atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
    return arr
  }

  async function renderPage(n) {
    if (!pdf || n < 1 || n > total) return
    pageNum = n
    const page = await pdf.getPage(n)
    const viewport = page.getViewport({ scale: 1.4 })
    canvas.width = viewport.width
    canvas.height = viewport.height
    const ctx = canvas.getContext('2d')
    await page.render({ canvasContext: ctx, viewport }).promise
    // Selectable text layer aligned over the canvas.
    textLayerEl.innerHTML = ''
    textLayerEl.style.width = viewport.width + 'px'
    textLayerEl.style.height = viewport.height + 'px'
    const textContent = await page.getTextContent()
    const tl = new pdfjsLib.TextLayer({ textContentSource: textContent, container: textLayerEl, viewport })
    await tl.render()
    dispatch('progress', { position: 'page=' + pageNum })
  }

  export function requestExtract() {
    const sel = window.getSelection()
    if (!sel || !textLayerEl.contains(sel.anchorNode)) return
    const text = sel.toString()
    if (!text.trim()) return
    dispatch('extract', { text, position: 'page=' + pageNum })
  }

  function prev() { renderPage(pageNum - 1) }
  function next() { renderPage(pageNum + 1) }

  onMount(async () => {
    try {
      const b64 = await GetSourceBytes(reading.refPath)
      pdf = await pdfjsLib.getDocument({ data: base64ToBytes(b64) }).promise
      total = pdf.numPages
      try { await SetReadingProjLen(reading.id, total) } catch { /* best-effort */ }
      const start = Math.min(Math.max(1, parsePage(reading.position)), total)
      await renderPage(start)
    } catch (e) { error = String(e?.message ?? e) }
  })

  onDestroy(() => { pdf?.destroy?.() })
</script>

<div class="pdf-root">
  <div class="pdf-bar">
    <button class="pdf-nav" on:click={prev} disabled={pageNum <= 1}>‹</button>
    <span class="pdf-page">{pageNum} / {total || '–'}</span>
    <button class="pdf-nav" on:click={next} disabled={pageNum >= total}>›</button>
  </div>
  {#if error}<div class="pdf-err">{error}</div>{/if}
  <div class="pdf-scroll">
    <div class="pdf-page-wrap">
      <canvas bind:this={canvas}></canvas>
      <div class="pdf-textlayer" bind:this={textLayerEl}></div>
    </div>
  </div>
</div>

<style>
  .pdf-root { flex: 1; display: flex; flex-direction: column; min-width: 0; background: #1b2230; }
  .pdf-bar { flex: 0 0 auto; display: flex; align-items: center; gap: 10px; justify-content: center; padding: 6px; border-bottom: 1px solid #2a3a4a; }
  .pdf-nav { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 10px; cursor: pointer; }
  .pdf-nav:disabled { opacity: .4; cursor: default; }
  .pdf-page { color: #8aa; font-size: 12px; }
  .pdf-err { color: #ff8080; padding: 8px 12px; font-size: 12px; }
  .pdf-scroll { flex: 1; overflow: auto; display: flex; justify-content: center; padding: 12px; }
  .pdf-page-wrap { position: relative; }
  .pdf-textlayer { position: absolute; top: 0; left: 0; overflow: hidden; opacity: 0.25; line-height: 1; }
  .pdf-textlayer :global(span) { color: transparent; position: absolute; white-space: pre; cursor: text; }
  .pdf-textlayer :global(::selection) { background: #7dd3fc; }
</style>
```

> **Verify against `pdfjs-dist` 5.6:** the worker import path (`pdfjs-dist/build/pdf.worker.min.mjs?url` — Vite `?url` asset), and the **TextLayer API** (`new pdfjsLib.TextLayer({textContentSource, container, viewport})` + `.render()`). In some 5.x builds the text layer is `pdfjsLib.renderTextLayer({textContentSource, container, viewport, textDivs})` instead. Read `node_modules/pdfjs-dist/types` or the package's `web/text_layer_builder` to confirm, and adapt — the contract is "selectable text positioned over the canvas so `window.getSelection()` yields the page's text." If the text layer proves too fiddly, a correct fallback is to build absolutely-positioned `<span>`s from `getTextContent().items` using each item's `transform`. Do NOT block on perfect alignment; selection correctness (right text + page) is what matters.

- [ ] **Step 2: Verify build + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0 (the pdf worker asset resolves via Vite). If the worker import fails to resolve, switch to `import workerUrl from 'pdfjs-dist/build/pdf.worker.min.js?url'` or the `.mjs` variant present in the installed version.

```bash
git add frontend/src/components/PdfSourcePane.svelte
git commit -m "feat(reading): PdfSourcePane — visual pdf.js reader with page-anchored extract"
```

---

### Task 6: WebSourcePane — sanitized-HTML reader

**Files:**
- Modify: `frontend/src/components/WebSourcePane.svelte` (replace the stub)

**Interfaces:**
- Implements the source-pane contract. Loads the saved sanitized HTML via `GetFileContent(reading.refPath)`, renders it inert into a div; selection → `{ text, position: 'char=' + runeOffset }` (offset into the container's text content); resume scrolls to `reading.position`; an "Open original" button calls `OpenSourceURL(reading.id)`.

- [ ] **Step 1: Replace the stub with the HTML reader**

Replace `frontend/src/components/WebSourcePane.svelte`:

```svelte
<script>
  import { onMount, createEventDispatcher } from 'svelte'
  import { GetFileContent, OpenSourceURL } from '../../wailsjs/go/main/App'

  export let reading
  const dispatch = createEventDispatcher()

  let html = ''
  let articleEl
  let error = ''

  function runeOffset() {
    const sel = window.getSelection()
    if (!sel || sel.rangeCount === 0 || !articleEl.contains(sel.anchorNode)) return null
    const range = sel.getRangeAt(0)
    const pre = range.cloneRange()
    pre.selectNodeContents(articleEl)
    pre.setEnd(range.startContainer, range.startOffset)
    return { offset: [...pre.toString()].length, text: sel.toString() }
  }

  export function requestExtract() {
    const sel = runeOffset()
    if (!sel || !sel.text.trim()) return
    dispatch('extract', { text: sel.text, position: 'char=' + sel.offset })
  }

  async function openOriginal() {
    try { await OpenSourceURL(reading.id) } catch (e) { error = String(e?.message ?? e) }
  }

  onMount(async () => {
    try { html = (await GetFileContent(reading.refPath)) ?? '' }
    catch (e) { error = String(e?.message ?? e) }
  })
</script>

<div class="web-root">
  <div class="web-bar">
    {#if reading.sourceUrl}<button class="web-open" on:click={openOriginal} title={reading.sourceUrl}>↗ Open original</button>{/if}
  </div>
  {#if error}<div class="web-err">{error}</div>{/if}
  <!-- html is pre-sanitized at import (no script/handlers/remote refs); rendered inert. -->
  <div class="web-article" bind:this={articleEl}>{@html html}</div>
</div>

<style>
  .web-root { flex: 1; display: flex; flex-direction: column; min-width: 0; }
  .web-bar { flex: 0 0 auto; padding: 6px 12px; border-bottom: 1px solid #2a3a4a; }
  .web-open { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .web-err { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .web-article { flex: 1; overflow: auto; padding: 20px 28px; color: #d6dde6; line-height: 1.7; user-select: text; }
  .web-article :global(img) { max-width: 100%; height: auto; }
  .web-article :global(h1), .web-article :global(h2), .web-article :global(h3) { color: #e8eaf0; }
  .web-article :global(a) { color: #7dd3fc; }
  .web-article :global(pre), .web-article :global(code) { background: #161f2e; border-radius: 4px; }
</style>
```

> `{@html html}` is safe here ONLY because the HTML was sanitized by bluemonday at import (Task 2/3) — no `<script>`, handlers, or remote refs. Do not render unsanitized HTML through this pane.

- [ ] **Step 2: Verify build + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0.

```bash
git add frontend/src/components/WebSourcePane.svelte
git commit -m "feat(reading): WebSourcePane — inert sanitized-HTML reader + Open original"
```

---

### Task 7: Sources panel — Import PDF + Import Web

**Files:**
- Modify: `frontend/src/components/SourcesPanel.svelte`

**Interfaces:**
- Consumes `ImportPdfSource`, `ImportWebSource` (+ existing `ImportTextSource`, `GetReadingList`, `activeReading`). Adds two buttons; web prompts for a URL.

- [ ] **Step 1: Add the import buttons**

In `frontend/src/components/SourcesPanel.svelte`:
1. Imports: add `ImportPdfSource, ImportWebSource` to the App import.
2. Add handlers (mirroring the existing `importSource`):
```js
  async function importPdf() {
    error = ''
    try {
      const v = await ImportPdfSource()
      if (v) { await load(); activeReading.set(v) }
    } catch (e) { error = String(e?.message ?? e) }
  }
  async function importWeb() {
    error = ''
    const url = window.prompt('Article URL:')
    if (!url) return
    try {
      const v = await ImportWebSource(url.trim())
      if (v) { await load(); activeReading.set(v) }
    } catch (e) { error = String(e?.message ?? e) }
  }
```
3. In the bar, add two buttons next to the existing **+ Import** (text):
```svelte
    <button class="sources-import" on:click={importPdf}>+ PDF</button>
    <button class="sources-import" on:click={importWeb}>+ Web</button>
```
   (Rename the existing text-import button label to `+ Text` for clarity.)

> READ the current `SourcesPanel.svelte` bar markup and `importSource`/`load`/`error`/`activeReading` first; match its style. `window.prompt` is acceptable for the URL (a small inline input can come later).

- [ ] **Step 2: Verify build + manual quickstart + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0.

Manual (`make dev`, collection open, Ctrl+Shift+E → Sources):
1. **+ PDF** → pick a PDF → it opens in the visual pdf.js reader; page nav works; select text → an extract with `(from) "<ref> page=N"` lands in the notes; reopen resumes the page.
2. **+ Web** → paste an article URL → the rich article renders **with its image**; select text → an extract with `(from) "<ref> char=N"`; **Open original** opens the URL; confirm `sources/<slug>.html` has no `<script>` and no remote `src`.
3. **+ Text** still imports a local text file as before.

```bash
git add frontend/src/components/SourcesPanel.svelte
git commit -m "feat(reading): Sources panel — Import PDF + Import Web (URL)"
```

---

## Self-Review

**Spec coverage (design §s → tasks):**
- §2.2 visual PDF + page anchors + resume → Tasks 3 (`GetSourceBytes`, `page=` position), 5 (PdfSourcePane). ✅
- §2.3 web rich article (fetch+readability+sanitize+inline+save+URL) → Tasks 2 (`FetchAndExtract`), 3 (`ImportWebSource`, `OpenSourceURL`), 6 (WebSourcePane). ✅
- §2.4 `medium` dispatch + pluggable panes → Tasks 1 (schema/struct), 4 (shell + TextSourcePane), 5, 6. ✅
- §4 per-medium anchors (`char=`/`page=`) via generalized CreateExtract → Task 3. ✅
- §5 flows (PDF import/read; web import/read) → Tasks 3, 5, 6, 7. ✅
- §6 components (web.go, schema, bindings, ReadingView shell, 3 panes, SourcesPanel) → Tasks 1-7. ✅
- §7 schema v4 (medium + source_url, additive) → Task 1. ✅
- §8 security (sanitize before save, inline-only, render inert) → Task 2 (bluemonday + inline), 6 (inert render comment + sanitized-only). ✅
- §9 edge cases: web fetch fail leaves nothing (Task 3 removes the saved source on later failure; fetch failure returns before any write), oversized images dropped (Task 2), PDF error state (Task 5), resume clamp (Tasks 5 `Math.min/max`), schema v4 additive (Task 1), source_url in element + notes (Task 3 records URL; `# @source` carries the title — NOTE: the design says the URL is written into `# @source`; this plan records the URL on the element and the title in `# @source`. See flagged item below). 
- §10 testing → Go units Tasks 1-3; manual quickstart Task 7. ✅

**Placeholder scan:** No TBD/TODO; every code step has complete code. Two explicit verify-and-adapt directives (go-readability/bluemonday/goquery API in Task 2; pdf.js worker + TextLayer API in Task 5) point the implementer at the real APIs with the test/contract as the gate — these are not placeholders. ✅

**Type consistency:** `ReadingElement.Medium`/`SourceURL` (Task 1) flow to `ReadingView.Medium`/`SourceURL` (Task 3) → consumed as `reading.medium`/`reading.sourceUrl` in Tasks 4-6. The generalized `CreateExtract(id, text, position string)` (Task 3) is called with a `position` string by every pane via the shell (Tasks 4-6). The source-pane contract (`requestExtract()` + `extract`/`progress` events, `position` strings `char=`/`page=`) is identical across Tasks 4, 5, 6 and the shell. ✅

**Flagged inconsistency (fix at implementation):** the design §4/§9 says the original URL is written into the notes `# @source`, but `CreateReadingNotes(notesPath, sourceRel, title)` puts the *title* in `# @source`. Resolution adopted in this plan: the **original URL lives on the reading element (`source_url`)** and drives "Open original"; the notes `# @source` holds the human title (its existing meaning). This satisfies "keep a reference to the original" via the queryable element field; writing the URL into the `.n4l` too would need a `CreateReadingNotes` change and is deferred (recorded here so it's not a silent divergence).
