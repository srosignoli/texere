# Media Cell Slice 2 — Audio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the Media cell to a second kind — audio — so a word's pronunciation can be found on Lingua Libre, previewed, stored locally, and promoted to a `has pronunciation` N4L fact.

**Architecture:** Two new `Provider` implementations in `internal/mediasearch` (a purpose-built Lingua Libre pronunciation provider with language scoping plus an exact-word filter, and a generic Commons audio provider). `media.FetchRemote` becomes kind-aware, validating audio by URL extension rather than by sniffing. On the frontend the existing kind→component map gains an audio renderer, and the search panel gains a kind toggle, a language control, and a row-based results list with in-place preview.

**Tech Stack:** Go 1.22+ (stdlib only), Svelte 3.49, Wails v2.12, `node:test`.

## Global Constraints

- **Pure Go, standard library only.** No CGo, no new dependencies in `go.mod`.
- **gofmt** formatting; imports grouped stdlib / external / internal.
- **Audio is validated by the URL path's file extension — never by `http.DetectContentType` and never by the response's `Content-Type` header.** Commons serves Lingua Libre `.wav` as `Content-Type: audio/x-wav`, which is absent from `audioMimeByExt`, so header validation rejects valid files. Audio containers also sniff unreliably; `ImportAudio` already keys off the extension for this reason.
- **Provider IDs are contract, not labels.** `pronunciation` and `commons-audio` are persisted on the cell and drive arrow selection. Slice 1's existing IDs are `commons`, `openverse`, `file`, `url`.
- **A descriptive User-Agent on every outbound request.** Go's default is 403'd by Wikimedia, whose API policy requires an identifying agent. `internal/mediasearch` already has `searchUserAgent`.
- **Search results are never persisted onto the cell.** Cells are serialized wholesale by `SaveNotebook`.
- **Do not modify `ImageCell.svelte`, `bake.js`, `TranscribeBar.svelte`, or `AudioCell.svelte`.** The Media cell must never mount `TranscribeBar`.
- **`Openverse.Kinds()` stays `["image"]`.** Its audio endpoint indexes Jamendo music and is deliberately excluded.
- Run `make test` and `make test-js` before each commit that touches the corresponding language.

---

### Task 1: `Result` audio fields and the verified language map

**Files:**
- Modify: `internal/mediasearch/mediasearch.go` (the `Result` struct)
- Create: `internal/mediasearch/languages.go`
- Create: `internal/mediasearch/languages_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `Result` gains `Lang`, `Speaker`, `Mime string` and `DurationMs int`, with JSON tags `lang`, `speaker`, `mime`, `durationMs`.
  - `mediasearch.Language{Name, Code string}` with JSON tags `name`, `code`.
  - `mediasearch.Languages() []Language` — display order.
  - `mediasearch.ResolveLanguage(nameOrCode string) (string, bool)`.

Every Q-code below was verified against the live Commons API: each returns files titled `LL-<code> (<iso>)-…`, and the recording counts are measured `totalhits`. Do not add languages without verifying them the same way — a wrong code silently returns nothing.

- [ ] **Step 1: Write the failing test**

Create `internal/mediasearch/languages_test.go`:

```go
package mediasearch

import "testing"

func TestLanguagesAreOrderedAndWellFormed(t *testing.T) {
	ls := Languages()
	if len(ls) < 25 {
		t.Fatalf("got %d languages, want at least 25", len(ls))
	}
	// French has by far the most Lingua Libre recordings and must lead, so the
	// best-covered languages are the ones a user sees first.
	if ls[0].Name != "French" || ls[0].Code != "Q150" {
		t.Errorf("first language = %+v, want French/Q150", ls[0])
	}
	seen := map[string]bool{}
	for _, l := range ls {
		if l.Name == "" || l.Code == "" {
			t.Errorf("incomplete entry: %+v", l)
		}
		if l.Code[0] != 'Q' {
			t.Errorf("code %q should be a Wikidata Q-code", l.Code)
		}
		if seen[l.Code] {
			t.Errorf("duplicate code %q", l.Code)
		}
		seen[l.Code] = true
	}
	// Verified as having 3 and 16 recordings respectively — a dropdown entry
	// that returns nothing is worse than its absence.
	if seen["Q7850"] || seen["Q25167"] {
		t.Error("Q7850 (Chinese) and Q25167 (Norwegian) are too sparse to list")
	}
}

func TestResolveLanguageByNameAndRawCode(t *testing.T) {
	if got, ok := ResolveLanguage("Spanish"); !ok || got != "Q1321" {
		t.Errorf("ResolveLanguage(Spanish) = %q, %v; want Q1321, true", got, ok)
	}
	// A raw Q-code passes through, so an unlisted language stays reachable.
	if got, ok := ResolveLanguage("Q12345"); !ok || got != "Q12345" {
		t.Errorf("ResolveLanguage(Q12345) = %q, %v; want passthrough", got, ok)
	}
	if _, ok := ResolveLanguage("Klingon"); ok {
		t.Error("ResolveLanguage(Klingon) reported ok, want false")
	}
	if _, ok := ResolveLanguage(""); ok {
		t.Error("ResolveLanguage(empty) reported ok, want false")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mediasearch/ -run 'TestLanguages|TestResolveLanguage' -v`
Expected: FAIL — `undefined: Languages`, `undefined: ResolveLanguage`.

- [ ] **Step 3: Add the four `Result` fields**

In `internal/mediasearch/mediasearch.go`, extend the `Result` struct. The existing six fields are unchanged:

```go
type Result struct {
	Kind       string `json:"kind"`       // "image" | "audio"
	ThumbURL   string `json:"thumbUrl"`   // small preview for the results grid (images)
	FullURL    string `json:"fullUrl"`    // what gets downloaded on pick
	Title      string `json:"title"`
	SourcePage string `json:"sourcePage"` // human-facing page, for attribution
	License    string `json:"license"`

	// Audio-only; empty/zero for images.
	Lang       string `json:"lang"`       // ISO code parsed from the filename, e.g. "spa"
	Speaker    string `json:"speaker"`    // contributor who made the recording
	Mime       string `json:"mime"`       // e.g. "audio/wav"; drives the format label
	DurationMs int    `json:"durationMs"` // 0 when unknown
}
```

- [ ] **Step 4: Write the language map**

Create `internal/mediasearch/languages.go`:

```go
package mediasearch

import (
	"regexp"
	"strings"
)

// Language is one selectable Lingua Libre language: a display name and the
// Wikidata item code its recordings are filed under (LL-Q1321 = Spanish).
type Language struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

// languages is ordered by measured Lingua Libre recording volume (Commons
// `intitle:LL-<code>` totalhits), so the best-covered languages appear first.
// Every code here was verified to return files titled `LL-<code> (<iso>)-…`.
//
// Deliberately absent: Chinese (Q7850, 3 recordings) and Norwegian Bokmål
// (Q25167, 16) — too sparse to be anything but a dead end in a dropdown.
// Indonesian is absent because Q9240's recordings are all Minangkabau (min),
// so it is listed under that name instead. Unlisted languages remain reachable
// by typing a raw Q-code.
var languages = []Language{
	{"French", "Q150"},          // 431895
	{"English", "Q1860"},        // 106904
	{"Polish", "Q809"},          // 97537
	{"Esperanto", "Q143"},       // 36348
	{"Russian", "Q7737"},        // 31802
	{"Ukrainian", "Q8798"},      // 27402
	{"German", "Q188"},          // 25915
	{"Romanian", "Q7913"},       // 24088
	{"Catalan", "Q7026"},        // 22920
	{"Basque", "Q8752"},         // 20686
	{"Spanish", "Q1321"},        // 19140
	{"Occitan", "Q14185"},       // 14188
	{"Arabic", "Q13955"},        // 13738
	{"Italian", "Q652"},         // 12235
	{"Swedish", "Q9027"},        // 9726
	{"Portuguese", "Q5146"},     // 9339
	{"Minangkabau", "Q9240"},    // 7017
	{"Turkish", "Q256"},         // 6705
	{"Czech", "Q9056"},          // 6232
	{"Hebrew", "Q9288"},         // 3615
	{"Breton", "Q12107"},        // 3451
	{"Hindi", "Q1568"},          // 3430
	{"Vietnamese", "Q9199"},     // 3100
	{"Dutch", "Q7411"},          // 1971
	{"Japanese", "Q5287"},       // 1045
	{"Korean", "Q9176"},         // 1026
	{"Greek", "Q9129"},          // 360
	{"Thai", "Q9217"},           // 289
	{"Hungarian", "Q9067"},      // 259
	{"Finnish", "Q1412"},        // 220
	{"Danish", "Q9035"},         // 147
}

// Languages returns the selectable languages in display order.
func Languages() []Language { return languages }

var qCodeRe = regexp.MustCompile(`^Q\d+$`)

// ResolveLanguage maps a display name to its Wikidata code. A value that is
// already a raw Q-code passes through unchanged, so a language missing from the
// curated list is still reachable.
func ResolveLanguage(nameOrCode string) (string, bool) {
	s := strings.TrimSpace(nameOrCode)
	if s == "" {
		return "", false
	}
	if qCodeRe.MatchString(s) {
		return s, true
	}
	for _, l := range languages {
		if strings.EqualFold(l.Name, s) {
			return l.Code, true
		}
	}
	return "", false
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/mediasearch/ -v`
Expected: PASS — the two new tests plus all pre-existing ones (the `Result` fields are additive, so nothing breaks).

- [ ] **Step 6: Commit**

```bash
git add internal/mediasearch/mediasearch.go internal/mediasearch/languages.go internal/mediasearch/languages_test.go
git commit -m "feat(mediasearch): audio Result fields + verified Lingua Libre language map"
```

---

### Task 2: Lingua Libre filename parsing and the exact-word filter

The two pure functions that make pronunciation search usable. Kept in their own file and tested independently, because this is where the search's precision actually comes from.

**Files:**
- Create: `internal/mediasearch/lingualibre.go`
- Create: `internal/mediasearch/lingualibre_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `matchesWord(name, word string) bool` (unexported)
  - `parseLinguaLibre(name, word string) (lang, speaker, label string)` (unexported)

Both take `name` **without** the `File:` prefix.

Background the implementer needs: Lingua Libre filenames are `LL-{Qcode} ({iso})-{speaker}-{word}.{ext}`. That format cannot be split unambiguously, because **both** the speaker and the word may contain hyphens (`I learned some phrases-pie-eyed`, `Nattes à chat-femme`). The word is therefore recovered only when it is already known — which is exactly the case for a filtered result.

Why the filter exists at all, verified live: `LL-Q150 chat` returns *femme*, *bigenre*, *quoi* because a contributor is named "Nattes à chat"; `LL-Q1860 pie` ranks *pie-eyed*, *pie house*, *pie safe* above the bare word.

- [ ] **Step 1: Write the failing test**

Create `internal/mediasearch/lingualibre_test.go`:

```go
package mediasearch

import "testing"

func TestMatchesWord(t *testing.T) {
	cases := []struct {
		name string
		word string
		want bool
	}{
		// Real result for the query "manzana".
		{"LL-Q1321 (spa)-Rdrg109-manzana.wav", "manzana", true},
		// Real collision: the speaker is "Nattes à chat", the word is "femme".
		{"LL-Q150 (fra)-Nattes à chat-femme.wav", "chat", false},
		// Real compounds that outrank the bare word for the query "pie".
		{"LL-Q1860 (eng)-I learned some phrases-pie-eyed.wav", "pie", false},
		{"LL-Q1860 (eng)-Neøn-pie chart.wav", "pie", false},
		{"LL-Q1860 (eng)-ManjiroLL-pie.wav", "pie", true},
		// Inflected forms are different lemmas and must not match.
		{"LL-Q188 (deu)-Sebastian Wallroth-Äpfel.wav", "Apfel", false},
		{"LL-Q188 (deu)-Sebastian Wallroth-Apfel.wav", "Apfel", true},
		// Case-insensitive, and surrounding whitespace in the query is ignored.
		{"LL-Q188 (deu)-Sebastian Wallroth-Apfel.wav", "apfel", true},
		{"LL-Q188 (deu)-Sebastian Wallroth-Apfel.wav", "  Apfel  ", true},
		// A multi-word query matches when it is the trailing segment.
		{"LL-Q1860 (eng)-Neøn-pie chart.wav", "pie chart", true},
	}
	for _, c := range cases {
		if got := matchesWord(c.name, c.word); got != c.want {
			t.Errorf("matchesWord(%q, %q) = %v, want %v", c.name, c.word, got, c.want)
		}
	}
}

func TestParseLinguaLibreExactMatch(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q1321 (spa)-Rdrg109-manzana.wav", "manzana")
	if lang != "spa" || speaker != "Rdrg109" || label != "manzana" {
		t.Errorf("got (%q, %q, %q), want (spa, Rdrg109, manzana)", lang, speaker, label)
	}
}

func TestParseLinguaLibreSpeakerContainingHyphen(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q188 (deu)-Sebastian-Wallroth-Apfel.wav", "Apfel")
	if lang != "deu" || speaker != "Sebastian-Wallroth" || label != "Apfel" {
		t.Errorf("got (%q, %q, %q), want (deu, Sebastian-Wallroth, Apfel)", lang, speaker, label)
	}
}

func TestParseLinguaLibreWordContainingSpace(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q1860 (eng)-Neøn-pie chart.wav", "pie chart")
	if lang != "eng" || speaker != "Neøn" || label != "pie chart" {
		t.Errorf("got (%q, %q, %q), want (eng, Neøn, pie chart)", lang, speaker, label)
	}
}

// On a non-exact match (the unfiltered fallback) the speaker/word split is
// genuinely unknowable, so no guess is made: the whole remainder is the label.
func TestParseLinguaLibreNonExactMakesNoGuess(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q150 (fra)-Nattes à chat-femme.wav", "chat")
	if lang != "fra" {
		t.Errorf("lang = %q, want fra", lang)
	}
	if speaker != "" {
		t.Errorf("speaker = %q, want empty (split is unknowable)", speaker)
	}
	if label != "Nattes à chat-femme" {
		t.Errorf("label = %q, want the full remainder", label)
	}
}

func TestParseLinguaLibreNonLinguaLibreName(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("Tango de Manzana (ISRC USUAN1100404).mp3", "manzana")
	if lang != "" || speaker != "" {
		t.Errorf("got (%q, %q), want empties for a non-Lingua-Libre name", lang, speaker)
	}
	if label != "Tango de Manzana (ISRC USUAN1100404)" {
		t.Errorf("label = %q, want the name without its extension", label)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mediasearch/ -run 'TestMatchesWord|TestParseLinguaLibre' -v`
Expected: FAIL — `undefined: matchesWord`, `undefined: parseLinguaLibre`.

- [ ] **Step 3: Write the implementation**

Create `internal/mediasearch/lingualibre.go`:

```go
package mediasearch

import (
	"path"
	"regexp"
	"strings"
)

// llPrefixRe matches the fixed head of a Lingua Libre filename:
// "LL-{Qcode} ({iso})-". Everything after it is "{speaker}-{word}", which
// cannot be split unambiguously because either part may contain a hyphen.
var llPrefixRe = regexp.MustCompile(`^LL-Q\d+ \(([a-z-]+)\)-`)

// matchesWord reports whether name is a recording of exactly word, by testing
// the trailing "-{word}" before the extension. This is what excludes results
// that matched a contributor's name (a speaker called "Nattes à chat" answering
// a search for "chat") and compounds (pie-eyed, pie chart) that the search
// ranks above the bare word. name must not carry the "File:" prefix.
func matchesWord(name, word string) bool {
	w := strings.TrimSpace(word)
	if w == "" {
		return false
	}
	base := strings.TrimSuffix(name, path.Ext(name))
	return strings.HasSuffix(strings.ToLower(base), "-"+strings.ToLower(w))
}

// parseLinguaLibre extracts the display fields from a Lingua Libre filename.
// The speaker is recoverable only when the word is already known — i.e. when
// matchesWord passes — so on a non-exact name no split is guessed: the speaker
// is empty and the label is the whole remainder. For a name that is not from
// Lingua Libre at all, lang and speaker are empty and the label is the name
// without its extension. name must not carry the "File:" prefix.
func parseLinguaLibre(name, word string) (lang, speaker, label string) {
	base := strings.TrimSuffix(name, path.Ext(name))
	m := llPrefixRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", base
	}
	lang = m[1]
	rest := base[len(m[0]):] // "{speaker}-{word}", ambiguous on its own
	if matchesWord(name, word) {
		w := strings.TrimSpace(word)
		speaker = strings.TrimSuffix(rest, "-"+rest[len(rest)-len(w):])
		return lang, speaker, rest[len(rest)-len(w):]
	}
	return lang, "", rest
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/mediasearch/ -run 'TestMatchesWord|TestParseLinguaLibre' -v`
Expected: PASS — six tests.

If `TestParseLinguaLibreExactMatch` fails on the speaker, note that the label is taken as the final `len(word)` bytes of `rest` (preserving the file's own casing) and the speaker is `rest` minus `-`+label. Both slices are byte-based, which is correct here because `matchesWord` has already established that the suffix matches.

- [ ] **Step 5: Run the full package suite**

Run: `go test ./internal/mediasearch/`
Expected: `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/mediasearch/lingualibre.go internal/mediasearch/lingualibre_test.go
git commit -m "feat(mediasearch): Lingua Libre filename parsing + exact-word filter"
```

---

### Task 3: The `Pronunciation` provider

**Files:**
- Create: `internal/mediasearch/pronunciation.go`
- Create: `internal/mediasearch/pronunciation_test.go`

**Interfaces:**
- Consumes: `Result` and its audio fields (Task 1); `ResolveLanguage` (Task 1); `matchesWord`, `parseLinguaLibre` (Task 2); the existing unexported `getJSON(ctx, rawURL, out) error`; the existing `commonsAPI` const.
- Produces: `mediasearch.Pronunciation{BaseURL string}` implementing `Provider`, with `ID() == "pronunciation"`, `Name() == "Pronunciation (Lingua Libre)"`, `Kinds() == []string{"audio"}`.

The language reaches the provider through the `query` argument as `"lang|word"` — for example `"Spanish|manzana"` or `"Q1321|manzana"`. This keeps the `Provider` interface unchanged rather than adding a parameter only one provider uses. A query with no `|` is an error naming the expected form.

The Commons API returns `imageinfo[0].duration` in **seconds as a float** when `iiprop` includes `size`.

- [ ] **Step 1: Write the failing test**

Create `internal/mediasearch/pronunciation_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mediasearch/ -run TestPronunciation -v`
Expected: FAIL — `undefined: Pronunciation`.

- [ ] **Step 3: Write the implementation**

Create `internal/mediasearch/pronunciation.go`:

```go
package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Pronunciation searches Lingua Libre's recordings, which are hosted on
// Wikimedia Commons and filed under a Wikidata language code (LL-Q1321 =
// Spanish). BaseURL is injectable for tests.
//
// The language travels in the query as "lang|word" ("Spanish|manzana" or
// "Q1321|manzana") so the Provider interface stays uniform rather than growing
// a parameter only this provider uses.
type Pronunciation struct{ BaseURL string }

func (p Pronunciation) ID() string      { return "pronunciation" }
func (p Pronunciation) Name() string    { return "Pronunciation (Lingua Libre)" }
func (p Pronunciation) Kinds() []string { return []string{"audio"} }

// llResponse is the formatversion=2 shape with the audio fields. Commons
// reports duration in seconds as a float.
type llResponse struct {
	Query struct {
		Pages []struct {
			Title     string `json:"title"`
			ImageInfo []struct {
				URL            string  `json:"url"`
				DescriptionURL string  `json:"descriptionurl"`
				Mime           string  `json:"mime"`
				Duration       float64 `json:"duration"`
			} `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
}

func (p Pronunciation) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "audio" {
		return nil, fmt.Errorf("mediasearch: pronunciation: unsupported kind %q", kind)
	}
	lang, word, ok := strings.Cut(query, "|")
	if !ok {
		return nil, fmt.Errorf("mediasearch: pronunciation: query must be \"language|word\", got %q", query)
	}
	word = strings.TrimSpace(word)
	if word == "" {
		return nil, fmt.Errorf("mediasearch: pronunciation: empty word")
	}
	code, ok := ResolveLanguage(lang)
	if !ok {
		return nil, fmt.Errorf("mediasearch: pronunciation: unknown language %q", lang)
	}

	q := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"generator":     {"search"},
		"gsrsearch":     {"LL-" + code + " " + word},
		"gsrnamespace":  {"6"}, // File:
		"gsrlimit":      {strconv.Itoa(limit)},
		"prop":          {"imageinfo"},
		"iiprop":        {"url|mime|size"}, // size also yields duration
	}
	var body llResponse
	if err := getJSON(ctx, p.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}

	all := make([]Result, 0, len(body.Query.Pages))
	exact := make([]Result, 0, len(body.Query.Pages))
	for _, pg := range body.Query.Pages {
		if len(pg.ImageInfo) == 0 {
			continue
		}
		ii := pg.ImageInfo[0]
		name := strings.TrimPrefix(pg.Title, "File:")
		lng, speaker, label := parseLinguaLibre(name, word)
		r := Result{
			Kind:       "audio",
			FullURL:    ii.URL,
			Title:      label,
			SourcePage: ii.DescriptionURL,
			Lang:       lng,
			Speaker:    speaker,
			Mime:       ii.Mime,
			DurationMs: int(ii.Duration * 1000),
		}
		all = append(all, r)
		if matchesWord(name, word) {
			exact = append(exact, r)
		}
	}
	// Prefer exact recordings of the word; fall back to everything the search
	// returned so a word with no exact recording still shows near-misses.
	if len(exact) > 0 {
		return exact, nil
	}
	return all, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/mediasearch/ -run TestPronunciation -v`
Expected: PASS — five tests.

- [ ] **Step 5: Commit**

```bash
git add internal/mediasearch/pronunciation.go internal/mediasearch/pronunciation_test.go
git commit -m "feat(mediasearch): Lingua Libre pronunciation provider"
```

---

### Task 4: The `CommonsAudio` provider, registry wiring, and the languages binding

**Files:**
- Create: `internal/mediasearch/commonsaudio.go`
- Create: `internal/mediasearch/commonsaudio_test.go`
- Modify: `internal/mediasearch/registry.go`
- Modify: `internal/mediasearch/registry_test.go`
- Modify: `media_methods.go`
- Modify: `media_methods_test.go`

**Interfaces:**
- Consumes: `llResponse`, `getJSON`, `commonsAPI` (Task 3); `Languages()` (Task 1).
- Produces:
  - `mediasearch.CommonsAudio{BaseURL string}` implementing `Provider`, `ID() == "commons-audio"`, `Name() == "Commons audio"`, `Kinds() == []string{"audio"}`.
  - `Providers()` returns four providers: `Commons`, `Openverse`, `Pronunciation`, `CommonsAudio`.
  - `(a *App) ListLanguages() []mediasearch.Language`.

`CommonsAudio` reuses `llResponse` — the shape is identical — and applies no filtering. Its titles are ordinary Commons filenames, not Lingua Libre ones, so it does **not** call `parseLinguaLibre`: it strips the extension directly and leaves `Lang` and `Speaker` empty.

- [ ] **Step 1: Write the failing tests**

Create `internal/mediasearch/commonsaudio_test.go`:

```go
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
```

Replace `TestProvidersIncludesBothAndAllServeImages` in `internal/mediasearch/registry_test.go` with:

```go
func TestProvidersCoverBothKinds(t *testing.T) {
	ps := Providers()
	if len(ps) != 4 {
		t.Fatalf("got %d providers, want 4", len(ps))
	}
	byID := map[string]Provider{}
	for _, p := range ps {
		if p.Name() == "" || len(p.Kinds()) == 0 {
			t.Errorf("provider %q is incompletely described", p.ID())
		}
		byID[p.ID()] = p
	}
	for _, id := range []string{"commons", "openverse", "pronunciation", "commons-audio"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing provider %q", id)
		}
	}
	// Openverse's audio endpoint indexes music, so it stays image-only.
	if k := byID["openverse"].Kinds(); len(k) != 1 || k[0] != "image" {
		t.Errorf("openverse Kinds = %v, want [image] only", k)
	}
	if k := byID["pronunciation"].Kinds(); k[0] != "audio" {
		t.Errorf("pronunciation Kinds = %v, want audio", k)
	}
}
```

Add to `media_methods_test.go`:

```go
func TestListLanguagesIsPopulated(t *testing.T) {
	got := (&App{}).ListLanguages()
	if len(got) < 25 {
		t.Fatalf("got %d languages, want at least 25", len(got))
	}
	if got[0].Name == "" || got[0].Code == "" {
		t.Errorf("incomplete first entry: %+v", got[0])
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/mediasearch/ ./ -run 'TestCommonsAudio|TestProvidersCover|TestListLanguages' -v`
Expected: FAIL — `undefined: CommonsAudio`, `undefined: (*App).ListLanguages`, and `Providers()` returning 2.

- [ ] **Step 3: Write the provider**

Create `internal/mediasearch/commonsaudio.go`:

```go
package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// CommonsAudio searches Wikimedia Commons for any audio — sound effects, animal
// calls, music. It applies no word filtering; for word pronunciations use
// Pronunciation, whose shaped query is what makes those findable at all.
// BaseURL is injectable for tests.
type CommonsAudio struct{ BaseURL string }

func (c CommonsAudio) ID() string      { return "commons-audio" }
func (c CommonsAudio) Name() string    { return "Commons audio" }
func (c CommonsAudio) Kinds() []string { return []string{"audio"} }

func (c CommonsAudio) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "audio" {
		return nil, fmt.Errorf("mediasearch: commons-audio: unsupported kind %q", kind)
	}
	q := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"generator":     {"search"},
		"gsrsearch":     {query + " filetype:audio"},
		"gsrnamespace":  {"6"},
		"gsrlimit":      {strconv.Itoa(limit)},
		"prop":          {"imageinfo"},
		"iiprop":        {"url|mime|size"},
	}
	var body llResponse
	if err := getJSON(ctx, c.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Query.Pages))
	for _, pg := range body.Query.Pages {
		if len(pg.ImageInfo) == 0 {
			continue
		}
		ii := pg.ImageInfo[0]
		name := strings.TrimPrefix(pg.Title, "File:")
		// These are ordinary Commons names, not Lingua Libre ones, so no
		// language or speaker is inferred and the label is simply the filename
		// without its extension. Lang and Speaker are deliberately left empty.
		label := strings.TrimSuffix(name, path.Ext(name))
		out = append(out, Result{
			Kind:       "audio",
			FullURL:    ii.URL,
			Title:      label,
			SourcePage: ii.DescriptionURL,
			Mime:       ii.Mime,
			DurationMs: int(ii.Duration * 1000),
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Wire the registry**

Replace `Providers()` in `internal/mediasearch/registry.go`:

```go
// Providers returns the live providers in display order. All four are keyless.
// Keyed providers (Forvo, Pexels, YouTube) arrive in a later slice and will be
// configured through the LLMProfile pattern rather than hardcoded here.
func Providers() []Provider {
	return []Provider{
		Commons{BaseURL: commonsAPI},
		Openverse{BaseURL: openverseAPI},
		Pronunciation{BaseURL: commonsAPI},
		CommonsAudio{BaseURL: commonsAPI},
	}
}
```

- [ ] **Step 5: Add the languages binding**

In `media_methods.go`, add:

```go
// ListLanguages returns the selectable pronunciation languages, ordered by
// Lingua Libre recording volume.
func (a *App) ListLanguages() []mediasearch.Language {
	return mediasearch.Languages()
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `make test`
Expected: all packages PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/mediasearch/ media_methods.go media_methods_test.go
git commit -m "feat(mediasearch): Commons audio provider + registry and languages binding"
```

---

### Task 5: Kind-aware `FetchRemote` and binding

**Files:**
- Modify: `internal/media/fetch.go`
- Modify: `internal/media/fetch_test.go`
- Modify: `media_methods.go` (`FetchMediaURL`)
- Modify: `media_methods_test.go`

**Interfaces:**
- Consumes: the existing unexported `store`, `extByMime`, `audioMimeByExt`.
- Produces:
  - `media.FetchRemote(collectionDir, rawURL, kind string) (Asset, error)` — **signature change**; the existing two-argument call in `FetchMediaURL` must be updated.
  - `(a *App) FetchMediaURL(rawURL, kind string) (map[string]any, error)`.

The audio path must key off the **URL path's extension**, never the sniffed type and never the `Content-Type` header. Commons serves Lingua Libre `.wav` as `Content-Type: audio/x-wav`, which is absent from `audioMimeByExt`; header validation would reject a valid file. This is also why `ImportAudio` already keys off the extension.

- [ ] **Step 1: Write the failing tests**

Add to `internal/media/fetch_test.go` (it already imports `net/http`, `net/http/httptest`, `os`, `path/filepath`, `strings`, `testing`, and declares `pngBytes`):

```go
// A minimal RIFF/WAVE header — enough that it is unmistakably not an image.
var wavBytes = append([]byte("RIFF$\x00\x00\x00WAVEfmt "), make([]byte, 32)...)

// Commons serves Lingua Libre .wav as Content-Type: audio/x-wav, which is not
// in audioMimeByExt. Validating on the header would reject every pronunciation
// file, so the URL's extension decides.
func TestFetchRemoteAudioIgnoresContentTypeHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-wav")
		_, _ = w.Write(wavBytes)
	}))
	defer srv.Close()

	col := t.TempDir()
	a, err := FetchRemote(col, srv.URL+"/LL-Q1321%20(spa)-Rdrg109-manzana.wav", "audio")
	if err != nil {
		t.Fatalf("FetchRemote: %v", err)
	}
	if a.Mime != "audio/wav" {
		t.Errorf("mime = %q, want audio/wav (from the extension, not the header)", a.Mime)
	}
	if !strings.HasSuffix(a.Name, ".wav") {
		t.Errorf("name = %q, want a .wav", a.Name)
	}
	if _, err := os.Stat(filepath.Join(col, "media", a.Name)); err != nil {
		t.Errorf("asset not written: %v", err)
	}
}

func TestFetchRemoteAudioRejectsUnknownExtension(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(wavBytes)
	}))
	defer srv.Close()

	if _, err := FetchRemote(t.TempDir(), srv.URL+"/clip.xyz", "audio"); err == nil {
		t.Fatal("expected an error for an unsupported audio extension")
	}
}

func TestFetchRemoteAudioUsesItsOwnSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	old := audioMaxBytes
	audioMaxBytes = 1024
	defer func() { audioMaxBytes = old }()

	_, err := FetchRemote(t.TempDir(), srv.URL+"/clip.wav", "audio")
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v, want a size-cap error", err)
	}
}

func TestFetchRemoteRejectsUnknownKind(t *testing.T) {
	if _, err := FetchRemote(t.TempDir(), "http://example.invalid/x.wav", "video"); err == nil {
		t.Fatal("expected an error for an unsupported kind")
	}
}
```

Update the four existing image tests to pass the new argument: `FetchRemote(col, srv.URL+"/apple.png", "image")` and so on. `TestFetchRemoteRejectsOversize` keeps using `remoteMaxBytes`.

Add to `media_methods_test.go`:

```go
func TestFetchMediaURLPassesKindThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(append([]byte("RIFF$\x00\x00\x00WAVEfmt "), make([]byte, 32)...))
	}))
	defer srv.Close()

	dir := t.TempDir()
	got, err := (&App{collectionDir: dir}).FetchMediaURL(srv.URL+"/manzana.wav", "audio")
	if err != nil {
		t.Fatalf("FetchMediaURL: %v", err)
	}
	if got["mime"] != "audio/wav" {
		t.Errorf("mime = %v, want audio/wav", got["mime"])
	}
}
```

Update `TestFetchMediaURLRequiresCollection` and `TestFetchMediaURLStoresAsset` to pass `"image"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/media/ ./ -run 'FetchRemote|FetchMediaURL' -v`
Expected: FAIL — too many arguments to `FetchRemote`, and `undefined: audioMaxBytes`.

- [ ] **Step 3: Rewrite `FetchRemote`**

In `internal/media/fetch.go`, add the audio cap beside the existing one and replace the function. The existing consts (`remoteFetchTimeout`, `remoteUserAgent`) and `remoteClient` are unchanged:

```go
// audioMaxBytes caps a downloaded audio file. A Lingua Libre clip is ~100 KB,
// but a general Commons recording can run for minutes. A var, not a const, so
// tests can shrink it.
var audioMaxBytes = 25 << 20 // 25 MB

// FetchRemote downloads rawURL, verifies it is a supported file of the given
// kind ("image" or "audio"), and stores it under <collectionDir>/media/
// content-addressed — so the same file fetched twice, from any URL, is stored
// once. The URL itself is not persisted here; the caller keeps provenance on
// the notebook cell.
func FetchRemote(collectionDir, rawURL, kind string) (Asset, error) {
	var maxBytes int
	switch kind {
	case "image":
		maxBytes = remoteMaxBytes
	case "audio":
		maxBytes = audioMaxBytes
	default:
		return Asset{}, fmt.Errorf("media: unsupported kind %q", kind)
	}

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
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if err != nil {
		return Asset{}, fmt.Errorf("media: read: %w", err)
	}
	if len(data) > maxBytes {
		return Asset{}, fmt.Errorf("media: %s larger than %d MB", kind, maxBytes>>20)
	}

	var mime, ext string
	if kind == "image" {
		// Sniff rather than trust Content-Type: hotlink guards and CDNs
		// frequently mislabel, and Import() already makes sniffing the rule.
		mime = http.DetectContentType(data)
		var ok bool
		if ext, ok = extByMime[mime]; !ok {
			return Asset{}, fmt.Errorf("media: unsupported type %q", mime)
		}
	} else {
		// Audio is keyed by the URL's extension, never sniffed and never taken
		// from Content-Type: containers sniff unreliably, and Commons serves
		// .wav as "audio/x-wav", which is absent from audioMimeByExt.
		u, err := url.Parse(rawURL)
		if err != nil {
			return Asset{}, fmt.Errorf("media: bad url: %w", err)
		}
		ext = strings.ToLower(path.Ext(u.Path))
		var ok bool
		if mime, ok = audioMimeByExt[ext]; !ok {
			return Asset{}, fmt.Errorf("media: unsupported audio extension %q", ext)
		}
	}

	name, err := store(collectionDir, data, ext)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: name, Mime: mime}, nil
}
```

Extend the import block to `"fmt"`, `"io"`, `"net/http"`, `"net/url"`, `"path"`, `"strings"`, `"time"`.

- [ ] **Step 4: Update the binding**

In `media_methods.go`, replace `FetchMediaURL`:

```go
// FetchMediaURL downloads rawURL into the collection's media/ directory and
// returns {asset, mime}. kind is "image" or "audio". Serves both "clicked a
// search result" and "pasted a URL" — they are the same operation.
func (a *App) FetchMediaURL(rawURL, kind string) (map[string]any, error) {
	a.dirMu.RLock()
	dir := a.collectionDir
	a.dirMu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("no collection open")
	}
	asset, err := media.FetchRemote(dir, rawURL, kind)
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": asset.Name, "mime": asset.Mime}, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `make test`
Expected: all packages PASS.

- [ ] **Step 6: Regenerate the Wails bindings**

Run: `wails generate module`

(If `wails` is not on `PATH`, it is at `$(go env GOPATH)/bin/wails`.)

Expected: `frontend/wailsjs/go/main/App.js` and `App.d.ts` show `FetchMediaURL` taking two arguments and gain `ListLanguages`; `models.ts` gains `mediasearch.Language` and the four new `Result` fields.

- [ ] **Step 7: Commit**

```bash
git add internal/media/ media_methods.go media_methods_test.go frontend/wailsjs/
git commit -m "feat(media): kind-aware FetchRemote; audio keyed by URL extension"
```

---

### Task 6: Kind toggle and language control in the search panel

Ends with: switching the panel to Audio shows the two audio providers and a language selector, and searching returns rows.

**Files:**
- Modify: `frontend/src/components/notebook/MediaSearchPanel.svelte`
- Modify: `frontend/src/lib/notebook.js` (`newMediaCell`)

**Interfaces:**
- Consumes: `ListMediaProviders`, `ListLanguages`, `SearchMedia`, `FetchMediaURL` (now two-argument) from `wailsjs/go/main/App`.
- Produces: the panel gains props `lang` and `onLang`; `newMediaCell()` gains `lang: ''`.

Two behaviours the current code does not have:

1. **Provider filtering must become reactive.** Today `onMount` filters `providers` by `kind` once. With a toggle, the filter has to re-run when `kind` changes, and `providerId` must be re-pointed at a provider that serves the new kind — otherwise Search silently queries an image provider for audio.
2. **The pronunciation provider needs the language**, which it takes inside the query string as `"lang|word"`.

- [ ] **Step 1: Add `lang` to the cell shape**

In `frontend/src/lib/notebook.js`, add `lang: ''` to the object returned by `newMediaCell()`, beside `query`:

```js
    query: '', subject: '', lang: '',
```

- [ ] **Step 2: Rework the panel's provider handling**

In `frontend/src/components/notebook/MediaSearchPanel.svelte`, extend the import:

```js
  import { ListMediaProviders, ListLanguages, SearchMedia, FetchMediaURL, ImportImage } from '../../../wailsjs/go/main/App'
```

Add the two new props beside the existing ones:

```js
  export let lang = ''
  export let onLang = () => {}   // (lang) => persist the language on the cell
```

Replace the `providers` state and `onMount` block with:

```js
  let allProviders = []
  let languages = []

  onMount(async () => {
    try {
      allProviders = await ListMediaProviders()
      languages = await ListLanguages()
      if (!lang) { lang = languages[0]?.name ?? ''; onLang(lang) }
    } catch (e) { err = String(e?.message ?? e) }
  })

  // Re-filter whenever the kind changes; a provider that cannot serve the
  // current kind must never stay selected, or Search queries the wrong one.
  $: providers = allProviders.filter((p) => p.kinds?.includes(kind))
  $: if (providers.length && !providers.some((p) => p.id === providerId)) {
    providerId = providers[0].id
  }
```

Replace the body of `search()`'s query construction so the pronunciation provider receives the language. The rest of `search()` — the `seq` guard, `busy`, the error handling — is unchanged:

```js
  async function search() {
    const q = query.trim()
    if (!q || !providerId) return
    const mine = ++seq
    busy = true; err = ''; results = []
    // The pronunciation provider takes "language|word"; every other provider
    // takes the bare word.
    const arg = providerId === 'pronunciation' ? `${lang}|${q}` : q
    try {
      const rs = await SearchMedia(providerId, arg, kind, LIMIT)
      if (mine !== seq) return
      results = rs ?? []
      searched = true
    } catch (e) {
      if (mine !== seq) return
      err = String(e?.message ?? e)
    } finally {
      if (mine === seq) busy = false
    }
  }
```

Update **both** `FetchMediaURL` call sites to pass the kind — one in `pick` (currently line 67) and one in `fetchPasted` (currently line 109). `importFile` is not one of them; it calls `ImportImage`, which takes no arguments:

```js
      const ref = await FetchMediaURL(r.fullUrl, kind)
```

```js
      const ref = await FetchMediaURL(u, kind)
```

- [ ] **Step 3: Add the toggle and language control to the markup**

Insert above the existing `.ms-bar` div:

```svelte
  <div class="ms-kinds">
    <button class="ms-btn" class:ms-on={kind === 'image'}
      on:click={() => setKind('image')} disabled={busy}>Image</button>
    <button class="ms-btn" class:ms-on={kind === 'audio'}
      on:click={() => setKind('audio')} disabled={busy}>Audio</button>
    {#if kind === 'audio'}
      <select bind:value={lang} on:change={() => onLang(lang)} disabled={busy} title="Recording language">
        {#each languages as l}<option value={l.name}>{l.name}</option>{/each}
      </select>
    {/if}
  </div>
```

And add the handler beside the others in the script:

```js
  function setKind(k) {
    if (k === kind) return
    kind = k
    results = []
    searched = false
    err = ''
    onKind(k)
  }
```

with the matching prop:

```js
  export let onKind = () => {}   // (kind) => persist the kind on the cell
```

Add the styles beside the existing ones:

```css
  .ms-kinds { display: flex; gap: 6px; align-items: center; margin-bottom: 6px; }
  .ms-on { background: #24334a; border-color: #4a7aa8; color: #cfe6ff; }
```

- [ ] **Step 4: Pass the new props from `MediaCell`**

In `frontend/src/components/notebook/MediaCell.svelte`, add the two handlers to the script:

```js
  function onLang(l) { cell.lang = l; persist() }
  function onKind(k) { cell.kind = k; persist() }
```

and extend the panel usage:

```svelte
      <MediaSearchPanel kind={cell.kind} query={cell.query} lang={cell.lang} {onQuery} {onLang} {onKind} {onPick} />
```

- [ ] **Step 5: Verify the build**

Run: `cd frontend && npx vite build`
Expected: succeeds.

The GUI walkthrough (toggling to Audio, seeing the two audio providers and the language list) needs a running app and is deferred to the manual test pass in Task 10.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/notebook/MediaSearchPanel.svelte frontend/src/components/notebook/MediaCell.svelte frontend/src/lib/notebook.js
git commit -m "feat(notebook): media cell kind toggle + pronunciation language control"
```

---

### Task 7: Audio result rows with in-place preview

**Files:**
- Create: `frontend/src/lib/media-error.js`
- Create: `frontend/src/lib/media-error.test.js`
- Modify: `frontend/src/components/notebook/MediaSearchPanel.svelte`

**Interfaces:**
- Consumes: `Result`'s `lang`, `speaker`, `mime`, `durationMs` (Task 1).
- Produces: `mediaErrorMessage(err, what)` from `frontend/src/lib/media-error.js` — **Task 8 imports this**, so the name and signature are contract.

The `MediaError` wording is needed in two places (this task's preview and Task 8's player), so it lives in one module rather than being written twice. `AudioCell.svelte` has its own third copy and is deliberately left alone — it is off-limits per the global constraints, and touching it is out of scope for this slice.

Audio has no thumbnail, so the grid becomes a list and each row carries a ▶ that streams the candidate directly from `upload.wikimedia.org` — which serves media without hotlink protection, as the image thumbnails already demonstrate. A single shared `<audio>` element is used so only one clip can play at a time.

The format label matters: `commons-audio` returns `.ogg`, which WebKitGTK often cannot decode, and that should be visible before clicking rather than discovered as a dead button.

- [ ] **Step 1: Write the failing test for the shared error helper**

Create `frontend/src/lib/media-error.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mediaErrorMessage } from './media-error.js'

// MediaError 3 (DECODE) and 4 (SRC_NOT_SUPPORTED) mean the format cannot be
// played here — WebKitGTK/WSLg frequently lacks the codecs for .ogg. Reporting
// that as "not found" would send the user hunting for the wrong problem.
test('codec failures name the format, not a missing file', () => {
  for (const code of [3, 4]) {
    const msg = mediaErrorMessage({ code })
    assert.match(msg, /format\/codec not supported/)
    assert.match(msg, new RegExp(`MediaError ${code}`))
  }
})

test('a network failure is reported as a load failure', () => {
  assert.match(mediaErrorMessage({ code: 2 }), /failed to load/)
})

test('an unknown or absent error still produces a message', () => {
  assert.match(mediaErrorMessage({ code: 0 }), /MediaError 0/)
  assert.match(mediaErrorMessage(null), /MediaError 0/)
  assert.match(mediaErrorMessage(undefined), /MediaError 0/)
})

test('the subject noun is configurable', () => {
  assert.match(mediaErrorMessage({ code: 2 }, 'Preview'), /^Preview/)
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `make test-js`
Expected: FAIL — `Cannot find module '.../frontend/src/lib/media-error.js'`.

- [ ] **Step 3: Write the shared helper**

Create `frontend/src/lib/media-error.js`:

```js
// Turns an HTMLMediaElement's MediaError into a message that names the real
// cause. Shared by the media cell's search preview and its audio player, so the
// wording cannot drift between them.
//
// Codes 3 (DECODE) and 4 (SRC_NOT_SUPPORTED) mean this webview cannot play the
// format — common for .ogg under WebKitGTK/WSLg — which is a completely
// different remedy from a file that failed to load.
export function mediaErrorMessage(err, what = 'Audio') {
  const code = err?.code ?? 0
  if (code === 3 || code === 4) {
    return `${what}: format/codec not supported in this webview (MediaError ${code}).`
  }
  if (code === 2) {
    return `${what} failed to load (MediaError ${code}).`
  }
  return `${what} error (MediaError ${code}).`
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `make test-js`
Expected: PASS — four `media-error` tests plus the pre-existing suites.

- [ ] **Step 5: Add preview state and helpers**

In `MediaSearchPanel.svelte`, extend the Svelte import to include `tick`, and import the helper:

```js
  import { onMount, onDestroy, tick } from 'svelte'
  import { mediaErrorMessage } from '../../lib/media-error.js'
```

Then, beside the other state:

```js
  let previewURL = ''     // FullURL of the row currently previewing
  let previewErr = ''
  let audioEl              // the single shared <audio>

  function fmtDuration(ms) {
    if (!ms) return ''
    return (ms / 1000).toFixed(1) + 's'
  }
  // "audio/wav" -> "wav"; "application/ogg" -> "ogg"
  function fmtFormat(mime) {
    if (!mime) return ''
    const s = mime.split('/')[1] ?? mime
    return s.replace(/^x-/, '')
  }

  async function togglePreview(r) {
    previewErr = ''
    if (previewURL === r.fullUrl) { audioEl?.pause(); previewURL = ''; return }
    previewURL = r.fullUrl
    await tick() // let the src binding apply before playing
    audioEl?.play().catch(() => {})
  }

  function onPreviewError(e) {
    previewErr = mediaErrorMessage(e.target?.error, 'Preview')
    previewURL = ''
  }
```

- [ ] **Step 6: Branch the results markup by kind**

Replace the `{:else if results.length}` branch so images keep the tile grid and audio gets rows:

```svelte
  {:else if results.length}
    {#if kind === 'audio'}
      <audio bind:this={audioEl} src={previewURL} on:error={onPreviewError} on:ended={() => (previewURL = '')} hidden></audio>
      {#if previewErr}<div class="ms-err">{previewErr}</div>{/if}
      <div class="ms-rows">
        {#each results as r (r.fullUrl)}
          <div class="ms-row" class:ms-loading={fetchingURL === r.fullUrl}>
            <button class="ms-play" on:click={() => togglePreview(r)}
              title={previewURL === r.fullUrl ? 'Stop' : 'Preview'}>
              {previewURL === r.fullUrl ? '■' : '▶'}
            </button>
            <button class="ms-pick" on:click={() => pick(r)} title={r.sourcePage}>
              <span class="ms-word">{r.title}</span>
              <span class="ms-meta">
                {[r.lang, r.speaker, fmtDuration(r.durationMs), fmtFormat(r.mime)].filter(Boolean).join(' · ')}
              </span>
            </button>
          </div>
        {/each}
      </div>
    {:else}
      <div class="ms-grid">
        {#each results as r (r.fullUrl)}
          <button class="ms-hit" class:ms-loading={fetchingURL === r.fullUrl}
            on:click={() => pick(r)} title={`${r.title}${r.license ? ' — ' + r.license : ''}`}>
            <img src={r.thumbUrl} alt={r.title} loading="lazy" />
          </button>
        {/each}
      </div>
    {/if}
  {/if}
```

- [ ] **Step 7: Add the row styles**

```css
  .ms-rows { margin-top: 8px; display: flex; flex-direction: column; gap: 4px; max-height: 320px; overflow-y: auto; }
  .ms-row { display: flex; align-items: stretch; gap: 6px; }
  .ms-row.ms-loading { opacity: 0.4; }
  .ms-play { width: 32px; flex: 0 0 auto; background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; cursor: pointer; }
  .ms-play:hover { background: #24334a; }
  .ms-pick { flex: 1 1 auto; display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 4px 8px; background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; cursor: pointer; text-align: left; }
  .ms-pick:hover { border-color: #4a7aa8; }
  .ms-word { font-size: 13px; }
  .ms-meta { font-size: 11px; color: #8aa; }
```

- [ ] **Step 8: Stop playback when the panel goes away**

An `onDestroy` already exists at line 37 for the acquisition-generation guard. Add the pause to **that** callback rather than registering a second one, so a preview cannot outlive the panel:

```js
  onDestroy(() => { acqGen++; audioEl?.pause() })
```

- [ ] **Step 9: Verify the build and the tests**

Run: `cd frontend && npx vite build` then `make test-js` from the repo root.
Expected: build succeeds; JS tests pass. Preview behaviour needs a GUI and is covered by the manual pass.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/lib/media-error.js frontend/src/lib/media-error.test.js frontend/src/components/notebook/MediaSearchPanel.svelte
git commit -m "feat(notebook): audio result rows with in-place preview"
```

---

### Task 8: `MediaAudioView` and image-only toolbar gating

**Files:**
- Create: `frontend/src/components/notebook/media/MediaAudioView.svelte`
- Modify: `frontend/src/components/notebook/MediaCell.svelte`

**Interfaces:**
- Consumes: the `views` kind→component map in `MediaCell` (slice 1); `mediaErrorMessage(err, what)` from `frontend/src/lib/media-error.js` (Task 7).
- Produces: `views.audio = MediaAudioView`.

Do **not** re-implement the `MediaError` wording here — import it. Task 7 created the shared module precisely so the preview and the player cannot drift apart.

`MediaAudioView` takes only `cell`. It must tolerate the `cropping`, `onCrop`, and `onMeta` props that `MediaCell` passes to whichever view is mounted — Svelte warns about unknown props but does not fail, and declaring the unused ones keeps the console clean, matching how other cells declare unused shared props.

- [ ] **Step 1: Create the audio view**

Create `frontend/src/components/notebook/media/MediaAudioView.svelte`:

```svelte
<script>
  import { mediaErrorMessage } from '../../../lib/media-error.js'

  export let cell
  export let cropping = false  // unused; images only
  export let onCrop = () => {} // unused; images only
  export let onMeta = () => {} // unused; audio has no intrinsic size

  let missing = false
  let playErr = ''

  $: src = `/media/${cell.asset}`

  // Shared with the search preview so the wording cannot drift. It separates a
  // decode failure (WebKitGTK/WSLg often lacks .ogg codecs) from a load
  // failure, because "not found" would send the user after the wrong problem.
  function onAudioError(e) {
    missing = true
    playErr = mediaErrorMessage(e.target?.error)
  }
</script>

{#if missing}
  <div class="av-err">{playErr || `audio not found: ${cell.asset}`}</div>
{:else}
  <div class="av-wrap">
    <audio class="av-player" {src} controls on:error={onAudioError}></audio>
  </div>
{/if}

<style>
  .av-wrap { padding: 8px; }
  .av-player { width: 100%; }
  .av-err { padding: 12px; color: #d88; font-size: 13px; }
</style>
```

- [ ] **Step 2: Register it and gate the crop toolbar**

In `frontend/src/components/notebook/MediaCell.svelte`, extend the import and the map:

```js
  import MediaAudioView from './media/MediaAudioView.svelte'
```

```js
  const views = { image: MediaImageView, audio: MediaAudioView }
```

Gate the crop controls on the kind — Crop and Clear crop are meaningless for a sound file. Change the toolbar's opening condition from `{#if !readOnly}` to:

```svelte
    {#if !readOnly}
      <div class="me-tools">
        {#if cell.kind === 'image'}
          {#if cropping}
            <button class="me-btn" on:click={() => (cropping = false)}>Cancel crop</button>
          {:else}
            <button class="me-btn" on:click={() => (cropping = true)}>Crop</button>
          {/if}
          {#if cell.crop}<button class="me-btn" on:click={clearCrop}>Clear crop</button>{/if}
        {/if}
        <span class="me-spacer"></span>
        <input class="me-subj" type="text" placeholder="subject" value={cell.subject}
          on:input={setSubject} title="The N4L node this recording is about" />
        <button class="me-btn" on:click={promote} disabled={!cell.subject.trim()}>Promote to N4L</button>
      </div>
    {/if}
```

- [ ] **Step 3: Verify the build**

Run: `cd frontend && npx vite build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/notebook/media/MediaAudioView.svelte frontend/src/components/notebook/MediaCell.svelte
git commit -m "feat(notebook): MediaAudioView + image-only crop toolbar"
```

---

### Task 9: Arrow selection in `buildMediaFact`

**Files:**
- Modify: `frontend/src/lib/media-fact.js`
- Modify: `frontend/src/lib/media-fact.test.js`
- Modify: `frontend/src/components/notebook/MediaCell.svelte` (Promote button label)

**Interfaces:**
- Consumes: `cell.kind` and `cell.provider`.
- Produces: `buildMediaFact({subject, asset, kind, provider})`; `mediaArrow({kind, provider})` returning the arrow code.

Slice 1's `buildMediaFact` hardcodes `pic`. It now derives the code:

| kind | provider | arrow |
|---|---|---|
| `image` (or absent) | any | `pic` |
| `audio` | `pronunciation` | `pron` |
| `audio` | anything else (`commons-audio`, `file`, `url`) | `aud` |

`kind` may be absent on cells saved before this slice, and must default to `image`.

- [ ] **Step 1: Write the failing tests**

Replace `frontend/src/lib/media-fact.test.js` with:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildMediaFact, mediaArrow } from './media-fact.js'

test('builds a has-picture fact from subject and asset', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'a3f9c1.jpg', kind: 'image', provider: 'commons' }),
    'manzana\n"  (pic) media/a3f9c1.jpg\n')
})

test('a cell saved before this slice has no kind and stays a picture', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'a3f9c1.jpg' }),
    'manzana\n"  (pic) media/a3f9c1.jpg\n')
})

test('a pronunciation result uses the pron arrow', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'b7.wav', kind: 'audio', provider: 'pronunciation' }),
    'manzana\n"  (pron) media/b7.wav\n')
})

test('other audio uses the generic aud arrow', () => {
  for (const provider of ['commons-audio', 'file', 'url', '']) {
    assert.equal(
      buildMediaFact({ subject: 'nightingale', asset: 'c9.ogg', kind: 'audio', provider }),
      'nightingale\n"  (aud) media/c9.ogg\n',
      `provider=${provider}`)
  }
})

test('a multi-word subject is preserved verbatim', () => {
  assert.equal(
    buildMediaFact({ subject: 'red delicious apple', asset: 'b7.png', kind: 'image' }),
    'red delicious apple\n"  (pic) media/b7.png\n')
})

test('subject is trimmed', () => {
  assert.equal(
    buildMediaFact({ subject: '  manzana  ', asset: 'a3.jpg', kind: 'image' }),
    'manzana\n"  (pic) media/a3.jpg\n')
})

test('an empty subject yields an empty string, not a malformed fact', () => {
  assert.equal(buildMediaFact({ subject: '', asset: 'a3.jpg', kind: 'image' }), '')
  assert.equal(buildMediaFact({ subject: '   ', asset: 'a3.jpg', kind: 'image' }), '')
})

test('a missing asset yields an empty string', () => {
  assert.equal(buildMediaFact({ subject: 'manzana', asset: '', kind: 'image' }), '')
})

test('mediaArrow is exported so the UI can name the arrow it will write', () => {
  assert.equal(mediaArrow({ kind: 'audio', provider: 'pronunciation' }), 'pron')
  assert.equal(mediaArrow({ kind: 'audio', provider: 'commons-audio' }), 'aud')
  assert.equal(mediaArrow({ kind: 'image', provider: 'commons' }), 'pic')
  assert.equal(mediaArrow({}), 'pic')
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test-js`
Expected: FAIL — `mediaArrow` is not exported, and the audio cases produce `(pic)`.

- [ ] **Step 3: Write the implementation**

Replace `frontend/src/lib/media-fact.js`:

```js
// Builds the N4L text promoted from a media cell. Kept as a pure function so it
// is unit-testable without mounting Svelte.
//
// The asset path is collection-relative (`media/<hash>.<ext>`), not absolute and
// not a file:// URL, so a collection stays portable when moved or shared.

// N4L references arrows by their short CODE, not their long name. These are the
// tokens declared in SSTconfig/arrows-EP-3.sst:
//   pic  = "has picture"        pron = "has pronunciation"      aud = "has audio"
const PICTURE_ARROW = 'pic'
const PRONUNCIATION_ARROW = 'pron'
const AUDIO_ARROW = 'aud'

// mediaArrow picks the arrow for a cell. Provenance chooses only a DEFAULT in
// editable text — the promoted draft is an N4L cell the user is about to read —
// so a wrong guess costs one keystroke rather than hiding anything.
// `kind` is absent on cells saved before audio existed, and means image.
export function mediaArrow({ kind, provider } = {}) {
  if (kind !== 'audio') return PICTURE_ARROW
  return provider === 'pronunciation' ? PRONUNCIATION_ARROW : AUDIO_ARROW
}

export function buildMediaFact({ subject, asset, kind, provider }) {
  const s = (subject ?? '').trim()
  if (!s || !asset) return ''
  return `${s}\n"  (${mediaArrow({ kind, provider })}) media/${asset}\n`
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS — nine `media-fact` tests plus the pre-existing `node-source` tests.

- [ ] **Step 5: Name the arrow on the Promote button**

In `frontend/src/components/notebook/MediaCell.svelte`, extend the import:

```js
  import { buildMediaFact, mediaArrow } from '../../lib/media-fact.js'
```

and change the Promote button so it states what it will write:

```svelte
        <button class="me-btn" on:click={promote} disabled={!cell.subject.trim()}
          title={`Insert an N4L draft using the (${mediaArrow(cell)}) arrow`}>
          Promote to N4L ({mediaArrow(cell)})
        </button>
```

- [ ] **Step 6: Verify the build and commit**

Run: `cd frontend && npx vite build`
Expected: succeeds.

```bash
git add frontend/src/lib/media-fact.js frontend/src/lib/media-fact.test.js frontend/src/components/notebook/MediaCell.svelte
git commit -m "feat(notebook): pron/aud arrow selection for promoted audio"
```

---

### Task 10: Arrow declarations, live verification, and the manual test plan

**Files:**
- Modify: `SSTconfig/arrows-EP-3.sst`
- Create: `docs/superpowers/specs/2026-07-27-media-cell-slice2-manual-test-plan.md`

**Interfaces:**
- Consumes: the arrow codes `pron` and `aud` emitted by `mediaArrow` (Task 9). The codes must match exactly.

- [ ] **Step 1: Declare the arrows**

In `SSTconfig/arrows-EP-3.sst`, in the `:: attributes, supporting information ::` section beside the existing `has picture` line, add two lines matching the column alignment of their neighbours:

```
 + has pronunciation (pron)         - has pronunciation recording of (pron-rec)
 + has audio         (aud)          - is audio for                   (aud-for)
```

Verify the alignment matches the surrounding lines rather than trusting the spacing above — the file is read by hand and its columns are deliberate.

**Do not use `pron-of` as the reverse code.** `SSTconfig/arrows-EP-3.sst:88` already declares `+ is pronounced as (pronas) - is the pronunciation for (pron-of)`, and the registry (`internal/content/registry.go`) is a map keyed by code, so redeclaring `pron-of` would silently overwrite that existing relation. Verified against the loaded registry: `pronas` and `pron-of` are registered; `pron`, `aud`, `aud-for` and `pron-rec` are free.

The pre-existing `pronas` is left untouched. It plausibly denotes a phonetic rendering in text (`manzana (pronas) /manˈθana/`), which is a different relation from "here is a stored recording", so the two coexist deliberately.

- [ ] **Step 2: Verify the arrows resolve and the promoted fact parses**

The unit tests only compare strings. Confirm the emitted text actually becomes a relation, using a temporary Go test in `internal/content`:

```go
package content

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTempPronunciationFactParses(t *testing.T) {
	reg, err := LoadRegistry("../../SSTconfig")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "v.n4l")
	if err := os.WriteFile(p, []byte("- v\n\nmanzana\n\"  (pron) media/b7.wav\n"), 0o644); err != nil {
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

Run it, confirm the output shows `code="pron"` with `registered=true`, paste the output into your report, then **delete the temporary file** and confirm `git status` is clean. Repeat for `aud` by changing the arrow in the fixture.

- [ ] **Step 3: Live-API check with production parameters**

Slice 1 shipped a broken Openverse search because its verification curl used `page_size=2` while the app sent 24. Verify with the values the app actually sends — `limit` 24, a real language code:

```bash
curl -s -A "texere/1.0 (+https://github.com/srosignoli/texere) media-search" --get \
  --data-urlencode 'gsrsearch=LL-Q1321 manzana' \
  'https://commons.wikimedia.org/w/api.php?action=query&format=json&formatversion=2&generator=search&gsrnamespace=6&gsrlimit=24&prop=imageinfo&iiprop=url|mime|size' \
  | head -c 600
```

Confirm the response carries `imageinfo[0].url`, `.mime` (`audio/wav`), and `.duration`, and that titles are `LL-Q1321 (spa)-…-manzana.wav`. Then confirm a direct download returns 200:

```bash
curl -s -o /dev/null -w '%{http_code} %{content_type}\n' \
  -A "texere/1.0 (+https://github.com/srosignoli/texere) media-fetch" \
  'https://upload.wikimedia.org/wikipedia/commons/4/4e/LL-Q1321_%28spa%29-Rdrg109-manzana.wav'
```

Expect `200 audio/x-wav` — the mismatch with `audioMimeByExt`'s `audio/wav` is exactly why audio is keyed by extension.

- [ ] **Step 4: Run the full verification suite**

```bash
make test
make test-js
make build
```

Expected: Go PASS, JS PASS, build exit 0. Also run `make lint` for information: the gate is already red from pre-existing findings in `internal/search`, `internal/content`, and `internal/source`; confirm no finding names a file this slice created or modified.

- [ ] **Step 5: Write the manual test plan**

Create `docs/superpowers/specs/2026-07-27-media-cell-slice2-manual-test-plan.md`, following the structure of `docs/superpowers/specs/2026-07-25-media-cell-slice1-manual-test-plan.md` (a Prereqs line, then `## TC-N Title` with numbered steps and `EXPECT:` lines). Every expectation must be specific enough to fail against, and must describe the feature **as built** — a test plan that contradicts the implementation is worse than none, because the tester will trust it over the code.

Cover:

1. **Kind toggle** — a new Media cell defaults to Image; switching to Audio replaces the provider list with "Pronunciation (Lingua Libre)" and "Commons audio", shows the language selector, and clears any previous results.
2. **Openverse absent from audio** — it appears for Image and not for Audio.
3. **Pronunciation search** — language Spanish, query `manzana`, several `.wav` rows each showing `spa`, a speaker, a duration and `wav`.
4. **The exact-word filter** — language French, query `chat`: results are recordings of *chat*, **not** words by the contributor "Nattes à chat" (*femme*, *quoi*). Language English, query `pie`: the bare word appears, not only *pie-eyed* / *pie chart*.
5. **Fallback** — a language/word pair with no exact recording still shows near-misses rather than an empty grid.
6. **Preview** — ▶ plays without downloading; the button becomes ■; playing a second row stops the first; nothing is written to `<collection>/media/` until a row is picked.
7. **Codec failure is named** — a Commons audio `.ogg` result that the webview cannot decode reports a format/codec message, not a dead button.
8. **Pick and persist** — picking downloads the file, the cell shows an `<audio>` player, and it survives a reload.
9. **No crop controls on an audio cell** — and Crop is still present on an image media cell.
10. **Promote** — the button reads `Promote to N4L (pron)` for a pronunciation and `(aud)` for Commons audio; the emitted draft is `manzana` then `"  (pron) media/<hash>.wav`.
11. **Push and graph** — pushing the draft writes the fact, and the explorer shows a `has pronunciation` edge resolved to its long name, not a synthesized unknown arrow.
12. **Language stickiness** — the chosen language persists across a reload and a new Media cell defaults to it.
13. **Paste URL and Import file for audio** — a pasted direct `.wav` URL fetches and plays; the promoted arrow is `aud`, not `pron`.
14. **Oversize and wrong-extension** — a pasted URL ending in an unsupported extension reports it.
15. **Regression** — image search, pick, crop, and `Promote to N4L (pic)` all still behave as in slice 1, and existing notebooks with image media cells still render.
16. **Regression** — the Scan cell and the separate Audio cell (record + Transcribe) are unaffected.

- [ ] **Step 6: Commit**

```bash
git add SSTconfig/arrows-EP-3.sst docs/superpowers/specs/2026-07-27-media-cell-slice2-manual-test-plan.md
git commit -m "feat(config): declare has pronunciation/has audio arrows; docs: slice 2 test plan"
```

---

## Self-Review

**Spec coverage.** Every spec section maps to a task: the `Pronunciation` provider with query shaping, exact-word filter and fallback → Tasks 2–3; `CommonsAudio` → Task 4; Openverse staying image-only → Task 4's registry test; the verified language map with the raw-Q escape hatch → Task 1; `Result`'s four audio fields → Task 1; kind-aware `FetchRemote` with extension keying and the 25 MB cap → Task 5; `FetchMediaURL` gaining `kind` → Task 5; the kind toggle with reactive provider filtering and the language control → Task 6; row-based results with preview and the format label → Task 7; `MediaAudioView` and crop gating → Task 8; the `pron`/`aud` arrows and the Promote label → Task 9; the `SSTconfig` declarations, live check and manual plan → Task 10. Error handling is distributed to where it occurs: `MediaError`-code discrimination in Tasks 7 and 8, unknown-kind and extension rejection in Task 5, unknown-language and malformed-query rejection in Task 3. Nothing from the spec's out-of-scope list appears in any task.

**Type consistency.** `Result`'s audio field names are identical in Task 1's struct, the provider constructions in Tasks 3–4, and the Svelte consumers in Task 7 (`r.lang`, `r.speaker`, `r.mime`, `r.durationMs` — camelCase per the JSON tags, since Wails marshals through them). Provider IDs `pronunciation` and `commons-audio` are used identically in Task 3, Task 4's registry test, Task 6's query-shaping branch, and Task 9's arrow table. `FetchRemote`'s three-argument signature in Task 5 matches its only caller. `mediaArrow`'s codes match the `SSTconfig` declarations in Task 10.

**Breaking change, called out where it bites.** `FetchRemote` and `FetchMediaURL` both gain a parameter in Task 5, which means Task 5 must update the existing image tests and the three frontend call sites; Task 6 updates two of those call sites as part of its own edit. Both tasks state this explicitly rather than leaving it to be discovered by a failing build.

**One deliberate interface choice.** The pronunciation language travels inside the `query` string as `"lang|word"` rather than as a new `Provider` parameter, so the interface stays uniform for the three providers that do not need it. This is stated in Task 3's header and again in Task 6, where the frontend constructs it — the two places an implementer could otherwise get it wrong.
