# Study Sources Sidebar Implementation Plan

**Goal:** In Study mode's Read focus, the document list becomes a collapsible left sidebar with per-document extract trees, a search box that matches titles and extract text, and extract clicks that jump to the passage without moving reading progress.

**Architecture:** One new Wails binding returns every reading's extracts grouped by reading id. A dependency-free JS module does the filtering. A new `SourcesSidebar` component replaces `SourcesPanel`, and `ReadingView` gains a peek mode that suppresses all three progress-persist paths.

**Tech Stack:** Go 1.25, SQLite (`internal/learning`), Wails v2, Svelte 3.49, `node --test`.

## Global Constraints

- **A peek must not move reading progress.** Progress is persisted in **three** places — `ReadingView.svelte` `close()`, `onDestroy`, and `onPaneProgress`. PDF and web panes *push* progress as the user navigates, so suppressing only `close()` works for text and fails silently for PDFs.
- **Anchors are unit-prefixed and passed through verbatim.** `char=N` for text/markdown/web, `page=N` for PDF. Never parse an anchor into a character offset in shared code — only the pane knows its unit.
- **Extracts for the whole list load in one call.** Search must match text inside collapsed documents, so the data must exist before any tree is opened.
- **Three distinct click targets per row:** triangle toggles the tree, title opens the document normally, an extract peeks. They must not be nested in a way that makes one fire another.
- The `%`, the `due`/`done` tags and the `+ Text / + PDF / + Web` imports carry over unchanged.
- Go: `gofmt`, imports grouped stdlib / external / internal. JS/Svelte: 2-space indent, **no semicolons**, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.
- `wails` is not on PATH; it is at `/home/simone/go/bin/wails`.
- `git add frontend/wailsjs` (bare dir) exits non-zero because `frontend/wailsjs/runtime/` is gitignored. Add explicit file paths and confirm with `git log`.
- `TestWatchCollectionAdded` in `internal/content` is a **known pre-existing fsnotify flake** that fails intermittently under full-suite load on `main` too. Ignore it; never "fix" it.

## MANDATORY: negative-control every test you write

This project has shipped tests that passed against the exact bug they were written to catch. Before committing, for each behaviour asserted: break the production code in the specific way the test exists to catch, run the test, confirm it **fails with a message naming the real problem**, then restore and re-run green. Report the actual failure output for the most load-bearing assertion. "Verified" without the output will be sent back.

## File Structure

- `internal/learning/extracts.go` — add `ListActiveExtracts`.
- `internal/learning/extracts_test.go` — its test.
- `reading_methods.go` — add `ReadingExtractView` and `GetReadingExtracts`.
- `reading_methods_test.go` — its tests.
- `frontend/src/lib/sources-filter.js` — the pure filter, testable under `node --test`.
- `frontend/src/lib/sources-filter.test.js` — its tests.
- `frontend/src/components/SourcesSidebar.svelte` — new; replaces `SourcesPanel.svelte`.
- `frontend/src/components/ReadingView.svelte` — peek support.
- `frontend/src/components/TextSourcePane.svelte` — honour a start anchor.
- `frontend/src/components/StudySurface.svelte` — the two-column read focus.
- `docs/specs/2026-08-06-study-sources-sidebar-manual-test-plan.md` — the manual gate.

---

### Task 1: `ListActiveExtracts` store method

**Files:**
- Modify: `internal/learning/extracts.go`
- Test: `internal/learning/extracts_test.go`

**Interfaces:**
- Consumes: the existing `extracts` table and `Store`.
- Produces: `func (s *Store) ListActiveExtracts() ([]Extract, error)`.

- [ ] **Step 1: Write the failing test**

Append to `internal/learning/extracts_test.go`:

```go
func TestListActiveExtractsReturnsOnlyActiveOnes(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, e := range []Extract{
		{ID: "/c/n.n4l:C:id:a", FilePath: "/c/n.n4l", Text: "first", FromAnchor: "sources/x.txt char=10", Due: now, Active: true},
		{ID: "/c/n.n4l:C:id:b", FilePath: "/c/n.n4l", Text: "second", FromAnchor: "sources/x.txt char=200", Due: now, Active: true},
		{ID: "/c/o.n4l:C:id:c", FilePath: "/c/o.n4l", Text: "other file", FromAnchor: "sources/y.txt char=5", Due: now, Active: true},
	} {
		if _, err := st.UpsertExtract(e); err != nil {
			t.Fatal(err)
		}
	}
	// Retiring one must remove it from the listing: a retired extract no longer
	// exists as far as the reader is concerned, and showing it in the tree would
	// offer a jump to a passage that is no longer tracked.
	if _, err := st.RetireMissingExtracts("/c/o.n4l", map[string]bool{}); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListActiveExtracts()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d extracts, want 2 (the retired one must be excluded): %+v", len(got), got)
	}
	for _, e := range got {
		if e.FilePath != "/c/n.n4l" {
			t.Errorf("unexpected file_path %q", e.FilePath)
		}
		if e.FromAnchor == "" {
			t.Errorf("extract %q lost its anchor; the tree cannot jump without one", e.ID)
		}
	}
}

func TestListActiveExtractsEmptyIsNotAnError(t *testing.T) {
	st, _ := Open(t.TempDir())
	got, err := st.ListActiveExtracts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("returned nil; callers range over this directly")
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
}
```

`extracts_test.go` already imports `testing` and `time`. Add nothing else.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/learning/ -run ListActiveExtracts -v`
Expected: FAIL — `st.ListActiveExtracts undefined`.

- [ ] **Step 3: Implement**

Add to `internal/learning/extracts.go`:

```go
// ListActiveExtracts returns every non-retired extract, newest table order.
//
// Deliberately unfiltered: the sources sidebar searches extract text across
// documents whose trees are still collapsed, so the caller needs them all
// before any one document is opened. Grouping and ordering happen in the
// caller, which knows how anchors map to readings.
func (s *Store) ListActiveExtracts() ([]Extract, error) {
	rows, err := s.db.Query(
		`SELECT id, file_path, text, from_anchor FROM extracts WHERE active=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Extract{}
	for rows.Next() {
		var e Extract
		var anchor sql.NullString
		if err := rows.Scan(&e.ID, &e.FilePath, &e.Text, &anchor); err != nil {
			return nil, err
		}
		e.FromAnchor = anchor.String
		e.Active = true
		out = append(out, e)
	}
	return out, rows.Err()
}
```

`extracts.go` already imports `database/sql`. If it does not, add it.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/learning/ -run ListActiveExtracts -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Negative control**

Change `WHERE active=1` to `WHERE 1=1`. `TestListActiveExtractsReturnsOnlyActiveOnes` must FAIL saying it got 3. Restore, confirm `git status --porcelain`, re-run green. Report the failure output.

- [ ] **Step 6: Commit**

```bash
git add internal/learning/extracts.go internal/learning/extracts_test.go
git commit -m "feat(learning): list every active extract"
```

---

### Task 2: `GetReadingExtracts` binding

**Files:**
- Modify: `reading_methods.go`
- Test: `reading_methods_test.go`
- Regenerate: `frontend/wailsjs/go/main/App.d.ts`, `App.js`, `models.ts`

**Interfaces:**
- Consumes: `ListActiveExtracts` (Task 1); the existing `ListReading`, `parsePositionValue` (`reading_methods.go:66`).
- Produces, for the frontend: `GetReadingExtracts() (map[string][]ReadingExtractView, error)` where `ReadingExtractView{ID, Text, Anchor string}` with JSON tags `id`, `text`, `anchor`.

An extract belongs to a reading when its `FilePath` equals that reading's `NotesPath` — **not** its `RefPath`. `FilePath` is the notes file the extract node lives in; the source is only referenced inside `FromAnchor`.

- [ ] **Step 1: Write the failing test**

Append to `reading_methods_test.go`:

```go
func TestGetReadingExtractsGroupsByNotesPath(t *testing.T) {
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{learnStore: st, collectionDir: dir}

	now := time.Now()
	if err := st.UpsertReading(learning.ReadingElement{
		ID: "r1", RefPath: "sources/x.txt", NotesPath: "notes/x.n4l",
		Title: "X", Position: "char=0", ProjLen: 100, Due: now, IntervalDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []learning.Extract{
		{ID: "e-late", FilePath: "notes/x.n4l", Text: "later passage", FromAnchor: "sources/x.txt char=100", Due: now, Active: true},
		{ID: "e-early", FilePath: "notes/x.n4l", Text: "early passage", FromAnchor: "sources/x.txt char=9", Due: now, Active: true},
		{ID: "e-elsewhere", FilePath: "notes/other.n4l", Text: "different doc", FromAnchor: "sources/y.txt char=1", Due: now, Active: true},
	} {
		if _, err := st.UpsertExtract(e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatal(err)
	}
	list := got["r1"]
	if len(list) != 2 {
		t.Fatalf("got %d extracts for r1, want 2 (grouping is by notes path): %+v", len(list), list)
	}
	// Document order, numerically: char=9 comes before char=100. Sorting the
	// anchors as strings would put "char=100" first and the tree would read out
	// of order for any document past its hundredth character.
	if list[0].ID != "e-early" || list[1].ID != "e-late" {
		t.Fatalf("wrong order: %s then %s — anchors must sort numerically, not lexically", list[0].ID, list[1].ID)
	}
	if list[0].Anchor != "sources/x.txt char=9" {
		t.Errorf("anchor = %q, want it passed through verbatim", list[0].Anchor)
	}
}

// PDF anchors use page=N. Nothing may assume char=.
func TestGetReadingExtractsPassesPageAnchorsThrough(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	a := &App{learnStore: st, collectionDir: dir}
	now := time.Now()
	st.UpsertReading(learning.ReadingElement{
		ID: "r-pdf", RefPath: "sources/p.pdf", NotesPath: "notes/p.n4l",
		Title: "P", Position: "page=1", ProjLen: 10, Due: now, IntervalDays: 1, Medium: "pdf",
	})
	st.UpsertExtract(learning.Extract{
		ID: "e-p", FilePath: "notes/p.n4l", Text: "from page 12",
		FromAnchor: "sources/p.pdf page=12", Due: now, Active: true,
	})

	got, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatal(err)
	}
	if len(got["r-pdf"]) != 1 || got["r-pdf"][0].Anchor != "sources/p.pdf page=12" {
		t.Fatalf("page anchor not passed through: %+v", got["r-pdf"])
	}
}

func TestGetReadingExtractsEmptyIsNotNil(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	a := &App{learnStore: st, collectionDir: dir}
	got, err := a.GetReadingExtracts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("returned a nil map; it crosses into JSON, where nil marshals to null")
	}
}
```

Add `"texere/internal/learning"` and `"time"` to the test file's imports if absent.

- [ ] **Step 2: Run to verify it fails**

Run: `go test . -run GetReadingExtracts -v`
Expected: FAIL — `a.GetReadingExtracts undefined`.

- [ ] **Step 3: Implement**

Add to `reading_methods.go`:

```go
// ReadingExtractView is one row in a document's extract tree.
//
// Anchor is the extract's from_anchor VERBATIM — "sources/x.txt char=1173" or
// "sources/p.pdf page=12". It is not parsed into an offset here: the unit
// depends on the medium and only the source pane knows what to do with it.
type ReadingExtractView struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Anchor string `json:"anchor"`
}

// GetReadingExtracts returns every reading's extracts, keyed by reading id and
// ordered by position within the document.
//
// One call for the whole library rather than a lookup per document: the sidebar
// searches extract text across documents whose trees are still collapsed, so it
// needs them all before any one is expanded.
//
// An extract belongs to a reading when its FilePath matches that reading's
// NotesPath. FilePath is the notes file the extract node lives in; the source
// appears only inside FromAnchor.
func (a *App) GetReadingExtracts() (map[string][]ReadingExtractView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	rs, err := a.learnStore.ListReading()
	if err != nil {
		return nil, err
	}
	readingIDByNotes := make(map[string]string, len(rs))
	for _, r := range rs {
		readingIDByNotes[r.NotesPath] = r.ID
	}

	exs, err := a.learnStore.ListActiveExtracts()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]ReadingExtractView, len(rs))
	for _, e := range exs {
		id, ok := readingIDByNotes[e.FilePath]
		if !ok {
			continue // an extract in a notes file that is not a reading
		}
		out[id] = append(out[id], ReadingExtractView{ID: e.ID, Text: e.Text, Anchor: e.FromAnchor})
	}
	// Document order. parsePositionValue pulls N out of "<unit>=N", so this
	// sorts numerically — sorting the anchor strings would put char=100 before
	// char=9 and the tree would read out of order.
	for id := range out {
		list := out[id]
		sort.SliceStable(list, func(i, j int) bool {
			return parsePositionValue(list[i].Anchor) < parsePositionValue(list[j].Anchor)
		})
		out[id] = list
	}
	return out, nil
}
```

Add `"sort"` to the imports.

- [ ] **Step 4: Run to verify it passes**

Run: `go test . -run GetReadingExtracts -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Negative control**

Replace the `sort.SliceStable` comparison with a string compare (`list[i].Anchor < list[j].Anchor`). `TestGetReadingExtractsGroupsByNotesPath` must FAIL on the order assertion. Restore. Report the output.

- [ ] **Step 6: Regenerate bindings and commit**

```bash
/home/simone/go/bin/wails generate module
git add reading_methods.go reading_methods_test.go \
  frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js frontend/wailsjs/go/models.ts
git commit -m "feat(reading): expose each reading's extracts for the sidebar tree"
git log --oneline -1
```

---

### Task 3: The filter function

**Files:**
- Create: `frontend/src/lib/sources-filter.js`
- Create: `frontend/src/lib/sources-filter.test.js`

**Interfaces:**
- Consumes: nothing — dependency-free by design.
- Produces: `filterSources(readings, extractsByReading, query) -> [{ reading, extracts, autoExpand }]`.

This lives in `lib/` rather than the component because this repo has **no Svelte component test harness**: logic left in a `.svelte` file has no tests at all.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/sources-filter.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { filterSources } from './sources-filter.js'

const readings = [
  { id: 'r1', title: 'Incremental learning' },
  { id: 'r2', title: 'Darwin notes' },
]
const byReading = {
  r1: [
    { id: 'e1', text: 'ensures 95% recall of top-priority material', anchor: 'char=10' },
    { id: 'e2', text: 'the fastest avenue towards knowledge', anchor: 'char=90' },
  ],
  r2: [{ id: 'e3', text: 'natural selection acts only by recall of variation', anchor: 'char=5' }],
}

test('an empty query returns every reading, nothing expanded', () => {
  const got = filterSources(readings, byReading, '')
  assert.equal(got.length, 2)
  assert.ok(got.every((r) => r.autoExpand === false))
  assert.ok(got.every((r) => r.extracts.length === 0))
})

test('a title match keeps the reading without expanding it', () => {
  const got = filterSources(readings, byReading, 'darwin')
  assert.equal(got.length, 1)
  assert.equal(got[0].reading.id, 'r2')
  assert.equal(got[0].autoExpand, false, 'a title match should not force the tree open')
})

// The case the search box exists for: finding a passage when you have
// forgotten which document it came from.
test('an extract match keeps the reading and expands to the matches only', () => {
  const got = filterSources(readings, byReading, 'fastest avenue')
  assert.equal(got.length, 1)
  assert.equal(got[0].reading.id, 'r1')
  assert.equal(got[0].autoExpand, true)
  assert.deepEqual(got[0].extracts.map((e) => e.id), ['e2'], 'only matching extracts are shown')
})

test('a query matching extracts in several documents keeps them all', () => {
  const got = filterSources(readings, byReading, 'recall')
  assert.deepEqual(got.map((r) => r.reading.id), ['r1', 'r2'])
  assert.deepEqual(got[0].extracts.map((e) => e.id), ['e1'])
  assert.deepEqual(got[1].extracts.map((e) => e.id), ['e3'])
})

test('matching is case-insensitive and trims the query', () => {
  assert.equal(filterSources(readings, byReading, '  DARWIN  ').length, 1)
  assert.equal(filterSources(readings, byReading, 'RECALL').length, 2)
})

test('no match returns an empty list', () => {
  assert.deepEqual(filterSources(readings, byReading, 'zzzznotathing'), [])
})

test('a reading with no extracts is handled', () => {
  const got = filterSources([{ id: 'r9', title: 'Lonely' }], {}, 'lonely')
  assert.equal(got.length, 1)
  assert.deepEqual(got[0].extracts, [])
})

test('order follows the input reading list', () => {
  const got = filterSources(readings, byReading, '')
  assert.deepEqual(got.map((r) => r.reading.id), ['r1', 'r2'])
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/sources-filter.test.js`
Expected: FAIL — cannot resolve `./sources-filter.js`.

- [ ] **Step 3: Implement**

Create `frontend/src/lib/sources-filter.js`:

```js
// Filters the sources sidebar by document title and extract text together.
//
// Kept out of the component on purpose: this repo has no Svelte component test
// harness, so anything left in a .svelte file has no tests. The component keeps
// only rendering and click handling.

/**
 * @param {Array<{id: string, title: string}>} readings
 * @param {Record<string, Array<{id: string, text: string, anchor: string}>>} extractsByReading
 * @param {string} query
 * @returns {Array<{reading: object, extracts: Array<object>, autoExpand: boolean}>}
 */
export function filterSources(readings, extractsByReading, query) {
  const list = Array.isArray(readings) ? readings : []
  const byReading = extractsByReading || {}
  const q = String(query ?? '').trim().toLowerCase()

  // No query: every document, trees closed. The caller's own expansion state
  // decides what is open, so nothing is forced here.
  if (!q) return list.map((reading) => ({ reading, extracts: [], autoExpand: false }))

  const out = []
  for (const reading of list) {
    const all = byReading[reading.id] ?? []
    const hits = all.filter((e) => String(e.text ?? '').toLowerCase().includes(q))
    const titleHit = String(reading.title ?? '').toLowerCase().includes(q)
    if (!titleHit && hits.length === 0) continue
    // Expand only when the reason it survived is inside the tree. A title match
    // has nothing to reveal, and forcing it open would bury the other results.
    out.push({ reading, extracts: hits, autoExpand: hits.length > 0 })
  }
  return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `node --test frontend/src/lib/sources-filter.test.js` then `make test-js`
Expected: all PASS.

- [ ] **Step 5: Negative controls (report both)**

1. Change `autoExpand: hits.length > 0` to `autoExpand: true`. The title-match test must FAIL. Restore.
2. Drop `.toLowerCase()` from the extract comparison. The case-insensitive test must FAIL. Restore.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/sources-filter.js frontend/src/lib/sources-filter.test.js
git commit -m "feat(study): filter sources by title and extract text"
```

---

### Task 4: Peek support in `ReadingView` and `TextSourcePane`

**Files:**
- Modify: `frontend/src/components/TextSourcePane.svelte`
- Modify: `frontend/src/components/ReadingView.svelte`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `ReadingView` accepts `export let peekAt = null` — an anchor string such as `char=1173` or `page=12`, or null for a normal read.

**This is the task most likely to be got wrong, and its failure is silent.** Reading progress is written in three places, and PDF/web push progress as the user navigates rather than only on leave. Missing one path means peeking works in text and quietly destroys progress in PDFs.

No unit test is possible — no Svelte component harness. Verify statically and state, in your report, the user-visible symptom if each suppression were missing.

- [ ] **Step 1: Let the pane start at a given anchor**

In `TextSourcePane.svelte`, add a prop beside the existing `export let reading`:

```js
  // When set, open here instead of the reading's saved position. Used by the
  // sidebar's extract peek.
  export let startAt = null
```

Then change the `onMount` line that currently reads `scrollToOffset(parseChar(reading.position))` (`:97`) to:

```js
    scrollToOffset(parseChar(startAt ?? reading.position))
```

- [ ] **Step 2: Add the peek prop and suppress all three persist paths**

In `ReadingView.svelte`, beside `export let reading`:

```js
  // An anchor to open at instead of saved progress, or null for a normal read.
  // While peeking, progress is NOT persisted: you came to look at one passage,
  // and closing must leave you where you had actually read up to.
  export let peekAt = null
```

Pass it to the pane — find the `<TextSourcePane` element and add `startAt={peekAt}`.

Then suppress each of the three writes. In `onPaneProgress` (`:42`):

```js
  async function onPaneProgress(e) {
    if (peekAt) return   // PDF/web push as the user navigates; a peek must not record
    try { await UpdateReadingProgress(reading.id, e.detail.position) } catch { /* best-effort */ }
  }
```

In `close()` (`:84`), guard the pull:

```js
      const pos = peekAt ? null : pane?.getProgress?.()
      if (pos) await UpdateReadingProgress(reading.id, pos)
```

And in `onDestroy` (`:130`), the same:

```js
      const pos = peekAt ? null : pane?.getProgress?.()
      if (pos) UpdateReadingProgress(reading.id, pos).catch(() => {})
```

Leave `flushNotes()` and `RescheduleReading` alone in both — a peek still saves notes you typed, and still counts as having visited the document.

- [ ] **Step 3: Verify statically**

Run: `grep -n "peekAt" frontend/src/components/ReadingView.svelte`
Expected: five hits — the prop, the pane, and one in each of the three persist paths. **Fewer than five means a path is unguarded.**

Then `make test-js` and `cd frontend && npx vite build`.

- [ ] **Step 4: Report the symptom of each missing guard**

In your report, state what a user would see if each were absent:
`onPaneProgress` — PDFs lose progress the moment you scroll during a peek.
`close()` — progress is lost when you close a peeked text document.
`onDestroy` — progress is lost when the surface unmounts without an explicit close.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ReadingView.svelte frontend/src/components/TextSourcePane.svelte
git commit -m "feat(reading): peek a passage without moving reading progress"
```

---

### Task 5: `SourcesSidebar` and the two-column read focus

**Files:**
- Create: `frontend/src/components/SourcesSidebar.svelte`
- Delete: `frontend/src/components/SourcesPanel.svelte`
- Modify: `frontend/src/components/StudySurface.svelte`

**Interfaces:**
- Consumes: `GetReadingExtracts` (Task 2), `filterSources` (Task 3), `ReadingView`'s `peekAt` (Task 4); the existing `GetReadingList`, `ImportTextSource`, `ImportPdfSource`, `ImportWebSource` and the `activeReading` store.
- Produces: events `open` (a reading) and `peek` (`{ reading, anchor }`).

`SourcesPanel.svelte` is imported only by `StudySurface.svelte:7` — nothing else references it.

The sidebar drops `SourcesPanel`'s `onClose` prop and its `×` on purpose. That
button closed the whole study surface, which the study chrome already offers at
`StudySurface.svelte:94` (`.ss-close`) and via Esc. In a sidebar that is always
visible, the same slot is worth more as the collapse chevron.

**Check `.ss-body`'s layout before wiring.** It is `position: relative`, and a sibling added in normal flow inside a container whose children are absolutely positioned renders underneath them. The explorer's ask box was invisible for a commit for exactly this reason.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/SourcesSidebar.svelte`:

```svelte
<script>
  // The reading library: a collapsible sidebar that stays visible while you
  // read, so switching documents or checking what you have already extracted
  // does not mean closing what you are in.
  import { onMount, createEventDispatcher } from 'svelte'
  import {
    ImportTextSource, ImportPdfSource, ImportWebSource, GetReadingList, GetReadingExtracts,
  } from '../../wailsjs/go/main/App'
  import { filterSources } from '../lib/sources-filter.js'

  const dispatch = createEventDispatcher()

  let items = []
  let extractsByReading = {}
  let query = ''
  let expanded = {}     // reading id -> manually opened
  let collapsed = false
  let error = ''

  $: rows = filterSources(items, extractsByReading, query)

  async function load() {
    try {
      items = (await GetReadingList()) ?? []
      extractsByReading = (await GetReadingExtracts()) ?? {}
    } catch (e) {
      error = String(e?.message ?? e)
    }
  }

  // A never-scheduled source has due===0 (NULL); StudyCounts and StudySurface
  // both count that as due, so the badge must too.
  const dueNow = (v) => !v.done && (v.due === 0 || v.due * 1000 <= Date.now())

  function toggle(id) { expanded[id] = !expanded[id]; expanded = expanded }
  function shown(row) { return row.autoExpand || expanded[row.reading.id] }
  function extractsFor(row) {
    return row.autoExpand ? row.extracts : (extractsByReading[row.reading.id] ?? [])
  }

  async function importVia(fn, arg) {
    error = ''
    try {
      const v = arg === undefined ? await fn() : await fn(arg)
      if (v) { await load(); dispatch('open', v) }
    } catch (e) { error = String(e?.message ?? e) }
  }
  const importText = () => importVia(ImportTextSource)
  const importPdf = () => importVia(ImportPdfSource)
  function importWeb() {
    const url = window.prompt('Article URL:')
    if (url) importVia(ImportWebSource, url.trim())
  }

  export function refresh() { return load() }

  onMount(load)
</script>

{#if collapsed}
  <div class="sb-rail">
    <button class="sb-toggle" on:click={() => (collapsed = false)} title="Show sources">›</button>
  </div>
{:else}
  <div class="sb-root">
    <div class="sb-bar">
      <span class="sb-title">Sources</span>
      <button class="sb-btn" on:click={importText} title="Import a text or markdown file">+ Text</button>
      <button class="sb-btn" on:click={importPdf} title="Import a PDF">+ PDF</button>
      <button class="sb-btn" on:click={importWeb} title="Import a web article">+ Web</button>
      <button class="sb-toggle" on:click={() => (collapsed = true)} title="Hide sources">‹</button>
    </div>

    <input class="sb-search" type="text" bind:value={query} placeholder="Search titles and extracts…"
           autocomplete="off" spellcheck="false" aria-label="Search sources" />

    {#if error}<div class="sb-error">{error}</div>{/if}

    {#if rows.length === 0}
      <div class="sb-empty">
        {query ? 'Nothing matches.' : 'No sources yet. Import a .txt, .md, or .html file to start reading.'}
      </div>
    {:else}
      <ul class="sb-list">
        {#each rows as row (row.reading.id)}
          <li class="sb-doc">
            <div class="sb-docrow">
              <!-- Its own control: the triangle reveals the tree, the title opens
                   the document. Making the whole row toggle would mean you could
                   not open a document without also expanding it. -->
              <button class="sb-tri" on:click|stopPropagation={() => toggle(row.reading.id)}
                      aria-label={shown(row) ? 'Hide extracts' : 'Show extracts'}>
                {shown(row) ? '▾' : '▸'}
              </button>
              <button class="sb-name" on:click={() => dispatch('open', row.reading)} title={row.reading.title}>
                {row.reading.title}
              </button>
              <span class="sb-pct">{row.reading.percent}%</span>
              {#if row.reading.done}<span class="sb-tag done">done</span>
              {:else if dueNow(row.reading)}<span class="sb-tag due">due</span>{/if}
            </div>

            {#if shown(row)}
              <ul class="sb-extracts">
                {#each extractsFor(row) as ex (ex.id)}
                  <li>
                    <button class="sb-extract"
                            on:click={() => dispatch('peek', { reading: row.reading, anchor: ex.anchor })}
                            title={ex.text}>⤷ {ex.text}</button>
                  </li>
                {:else}
                  <li class="sb-noextracts">No extracts yet.</li>
                {/each}
              </ul>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

<style>
  .sb-root { width: 300px; flex: 0 0 auto; display: flex; flex-direction: column;
    background: #131c29; border-right: 1px solid #2a3a4a; min-height: 0; }
  .sb-rail { width: 22px; flex: 0 0 auto; display: flex; justify-content: center; padding-top: 8px;
    background: #131c29; border-right: 1px solid #2a3a4a; }
  .sb-bar { display: flex; align-items: center; gap: 6px; padding: 8px 10px;
    border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .sb-title { color: #7dd3fc; font-weight: 600; flex: 1; font-size: 13px; }
  .sb-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px;
    padding: 3px 7px; cursor: pointer; font-size: 11px; }
  .sb-toggle { background: none; border: none; color: #8899aa; font-size: 15px; cursor: pointer; padding: 0 4px; }
  .sb-search { margin: 8px 10px; background: #0b1220; border: 1px solid #2a3a4a; border-radius: 6px;
    color: #e8eaf0; font-size: 12px; padding: 5px 8px; outline: none; }
  .sb-search::placeholder { color: #5d7186; }
  .sb-error { color: #ff8080; padding: 6px 10px; font-size: 12px; }
  .sb-empty { color: #8aa; padding: 14px 10px; font-size: 13px; }
  .sb-list { list-style: none; margin: 0; padding: 0; overflow: auto; flex: 1; min-height: 0; }
  .sb-doc { border-bottom: 1px solid #1d2a3a; }
  .sb-docrow { display: flex; align-items: center; gap: 6px; padding: 6px 10px; }
  .sb-tri { background: none; border: none; color: #8899aa; cursor: pointer; font-size: 11px; padding: 0 2px; }
  .sb-name { flex: 1; min-width: 0; background: none; border: none; color: #cdd6e0; font-size: 13px;
    text-align: left; cursor: pointer; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sb-name:hover { color: #7dd3fc; }
  .sb-pct { color: #7ecfa0; font-size: 12px; }
  .sb-tag { font-size: 10px; border-radius: 3px; padding: 1px 5px; }
  .sb-tag.due { background: #4a3a1a; color: #e0b000; }
  .sb-tag.done { background: #1a3a2a; color: #7ecfa0; }
  .sb-extracts { list-style: none; margin: 0 0 4px; padding: 0 0 0 22px; }
  .sb-extract { background: none; border: none; color: #9fb3c8; font-size: 12px; text-align: left;
    cursor: pointer; padding: 3px 8px 3px 0; display: block; width: 100%;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sb-extract:hover { color: #7dd3fc; }
  .sb-noextracts { color: #5d7186; font-size: 11px; padding: 2px 0 4px; }
</style>
```

- [ ] **Step 2: Wire it into the read focus**

In `StudySurface.svelte`, replace the `SourcesPanel` import with:

```js
  import SourcesSidebar from './SourcesSidebar.svelte'
```

Add state beside the others:

```js
  let peekAt = null   // anchor for an extract peek, cleared on a normal open
```

Then replace the read-focus branch — currently `StudySurface.svelte:99-104`, the
`{#if $studyFocus === 'read' …}` line and the inner `{#if $activeReading} …
{:else} <SourcesPanel …/> {/if}`, stopping **before** the
`{:else if $studyFocus === 'process' …}` that follows — with:

```svelte
    {#if $studyFocus === 'read' || ($studyFocus === 'next' && nextKind() === 'read')}
      <div class="ss-read">
        <SourcesSidebar
          on:open={(e) => { peekAt = null; activeReading.set(e.detail) }}
          on:peek={(e) => { peekAt = e.detail.anchor; activeReading.set(e.detail.reading) }} />
        <div class="ss-reading">
          {#if $activeReading}
            {#key ($activeReading.id ?? '') + '|' + (peekAt ?? '')}
              <ReadingView reading={$activeReading} {peekAt}
                onClose={() => { peekAt = null; activeReading.set(null); enter('next') }} />
            {/key}
          {:else}
            <div class="ss-empty">Pick a document to read.</div>
          {/if}
        </div>
      </div>
```

The `{#key}` remounts when either the document **or** the peek anchor changes, so peeking a second extract in the document you are already in re-opens at the new passage rather than doing nothing.

- [ ] **Step 3: Add the layout styles**

`.ss-body` is `position: relative` and `ReadingView` is `position: absolute; inset: 0`, so the sidebar needs a flex row that gives the reading area its own positioning context:

```css
  .ss-read { position: absolute; inset: 0; display: flex; min-height: 0; }
  .ss-reading { position: relative; flex: 1; min-width: 0; }
```

- [ ] **Step 4: Delete the old panel**

```bash
git rm frontend/src/components/SourcesPanel.svelte
grep -rn "SourcesPanel" frontend/src/ || echo "(no references remain)"
```

- [ ] **Step 5: Verify**

Run: `make test`, `make test-js`, and `cd frontend && npx vite build`
Expected: all pass, and **no unused-CSS-selector warning naming `.sb-root`, `.ss-read` or `.ss-reading`** — such a warning means the markup did not land.

State in your report the user-visible symptom if each of these were wrong: the collapse toggle, the triangle-versus-title split, and the `{#key}` including `peekAt`.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/SourcesSidebar.svelte frontend/src/components/StudySurface.svelte
git commit -m "feat(study): sources sidebar with extract trees"
```

---

### Task 6: Manual test plan

**Files:**
- Create: `docs/specs/2026-08-06-study-sources-sidebar-manual-test-plan.md`

Match the house format of `docs/specs/2026-08-05-nl-query-translation-manual-test-plan.md`: a preamble stating what is automated and what is not, prereqs, numbered cases with explicit setup and observable results, `**[DB]**` on anything invisible.

- [ ] **Step 1: Write the document**

Cover at least:

1. The sidebar appears on the **left** with the document list, `%`, and due/done tags.
2. Collapse and reopen it in one click; the reading area takes the full width when collapsed.
3. The list stays visible while a document is open, and another document can be opened without closing the first.
4. The triangle expands a document to its extracts, in document order.
5. Clicking the **title** opens the document at saved progress.
6. Clicking an **extract** opens the document at that passage.
7. **The peek, for a text document:** read part-way, note the `%`, peek an early extract, close, confirm the `%` is unchanged.
8. **The peek, for a PDF** — the case a partial implementation fails. PDFs push progress as you navigate, so scroll around during the peek before closing, then confirm the `%` is unchanged.
9. Opening by title after a peek resumes normally and **does** save progress.
10. Search matches a title; search matches extract text and auto-expands showing only the matches; a search matching nothing shows "Nothing matches."
11. A document with no extracts expands to "No extracts yet."
12. Importing a source still works via all three buttons and the new document appears.
13. Peeking a second extract in the document already open jumps to the new passage.

State explicitly, so a tester does not report it as a bug: a peeked document deliberately does **not** update progress, and extracts appear only after sync has discovered them — the same delay the processing queue already has.

- [ ] **Step 2: Commit**

```bash
git add docs/specs/2026-08-06-study-sources-sidebar-manual-test-plan.md
git commit -m "docs(test-plan): study sources sidebar"
```

---

## Self-Review

**Spec coverage.** Collapsible left sidebar → Task 5. `%`/tags/imports retained → Task 5 Step 1. Two-level tree → Tasks 2 and 5. Search over titles and extract text → Task 3, wired in Task 5. Peek that preserves progress → Task 4, all three paths. One call for all extracts → Task 2. Three click targets → Task 5 Step 1. Anchors passed verbatim → Task 2's `ReadingExtractView.Anchor` and Task 4's `startAt`.

**Placeholders.** None: every code step carries literal code, every test its assertions.

**Type consistency.** `ReadingExtractView{ID, Text, Anchor}` (Go, tags `id`/`text`/`anchor`) is consumed as `ex.id`/`ex.text`/`ex.anchor` in Task 5 and as `{id, text, anchor}` in Task 3's fixtures. `filterSources` returns `{reading, extracts, autoExpand}`, destructured as `row.reading`/`row.extracts`/`row.autoExpand` in Task 5. `peekAt` is an anchor string in Tasks 4 and 5; `TextSourcePane`'s prop is `startAt`, set from it.

**Two hazards called out where they bite.** Task 4 requires five `peekAt` references and says fewer means an unguarded path, because the missing-path failure is silent and PDF-only. Task 5 warns that `.ss-body` is `position: relative` with absolutely-positioned children, which already made one component invisible in this codebase.

**One deliberate limitation.** `filterSources` returns *all* of a document's extracts when it is manually expanded with no query, and only the matching ones when a query expanded it. That asymmetry is intended — a manual expand asks "what did I take from this?", a search asks "where is this passage?" — and Task 5's `extractsFor` implements exactly that.
