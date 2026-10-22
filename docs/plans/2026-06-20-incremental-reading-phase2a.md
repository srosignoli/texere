# Incremental Reading — Phase 2a (Read & Extract) Implementation Plan

**Goal:** Import a local text/HTML source, read it (resuming where you left off) beside its auto-created notes file, select passages to create verbatim N4L extracts with `(from)` anchors, and have the source resurface on a schedule.

**Architecture:** A new pure-Go `internal/source/` produces the canonical text projection a source renders + anchors into. New `internal/content` writeback ops create the per-source notes file and append extract nodes (generating `# @id`s explicitly, since these nodes aren't study-marked). `internal/learning` gains a `reading_elements` table (schema v2) + a simple resurface schedule. New `App` bindings drive a standalone Svelte Sources panel + ReadingView overlay (source pane beside a reused N4L editor).

**Tech Stack:** Go 1.25, `modernc.org/sqlite`, `golang.org/x/net/html` (already an indirect dep; pure Go), Wails v2, Svelte 3.49, CodeMirror 6. Reuses `internal/content` + the Phase-1 `internal/learning` store.

## Global Constraints

- All new dependencies pure Go (no CGo). HTML→text uses `golang.org/x/net/html` (already in `go.sum`; becomes a direct dep). (CLAUDE.md.)
- Anchors are `char=<offset>` into the source's **text projection** (`.txt`/`.md` = file content; `.html` = stripped article text). No snippet-relocation in 2a.
- Per-source notes live at `<collectionDir>/reading/<name>.n4l`; the original is copied to `<collectionDir>/sources/<name>`. Name collisions are suffixed (`name-2`), never overwritten.
- Reading-element and extract nodes are NOT study-marked, so their `# @id`s are generated explicitly via `content.GenerateShortID()` in the new writeback ops (NOT `InjectMissingIDs`).
- `learning.db` is machine-local, gitignored; schema migrates additively 1→2 (existing `items`/`review_logs` untouched).
- `# @progress`/extract writeback must never clobber a dirty notes buffer — reuse the Phase-1 invariant (silent-reload only when clean, via `file:ids-injected`).
- Go: `gofmt`, grouped imports (stdlib / external / internal). Tests: `go test ./...`; frontend `cd frontend && npm run build` + manual quickstart.

---

## File structure

| File | Responsibility |
|---|---|
| `internal/source/projection.go` (+ `_test.go`) | `Projection(path) (string, error)` — txt/md passthrough; html→text. |
| `internal/content/reading.go` (+ `_test.go`) | `CreateReadingNotes`, `UpdateProgressMarker`, `AppendExtract`. |
| `internal/learning/reading.go` (+ `_test.go`) | `reading_elements` CRUD + `ScheduleReading`; schema bumped to v2 in `db.go`. |
| `reading_methods.go` (repo root) (+ `_test.go`) | Bindings: `ImportTextSource`, `GetReadingList`, `GetSourceProjection`, `CreateExtract`, `UpdateReadingProgress`, `RescheduleReading`. |
| `frontend/src/stores/reading.js` | `readingOpen`, `activeReading` stores. |
| `frontend/src/components/SourcesPanel.svelte` | Sources list + Import button; opens ReadingView. |
| `frontend/src/components/ReadingView.svelte` | Source pane (render projection, select→Extract) + notes editor. |
| `frontend/src/App.svelte` (modify) | Ctrl+Shift+E toggles the Sources panel; render overlays. |

---

### Task 1: Source text projection

**Files:**
- Create: `internal/source/projection.go`
- Test: `internal/source/projection_test.go`

**Interfaces:**
- Consumes: nothing (reads a file by path).
- Produces: `func Projection(path string) (string, error)` — returns the canonical text the reader renders + anchors into. `.txt`/`.md`/unknown → file content verbatim. `.html`/`.htm` → article text with tags stripped (script/style dropped, whitespace collapsed to single spaces).

- [ ] **Step 1: Add the dependency as direct**

Run: `cd /home/simone/projects/texere && go get golang.org/x/net/html@latest && go mod tidy`
Expected: `golang.org/x/net` moves out of the `// indirect` group; no error.

- [ ] **Step 2: Write the failing test**

Create `internal/source/projection_test.go`:

```go
package source

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProjectionPlainTextPassthrough(t *testing.T) {
	p := write(t, "a.txt", "System 1 is fast.\nSystem 2 is slow.")
	got, err := Projection(p)
	if err != nil || got != "System 1 is fast.\nSystem 2 is slow." {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestProjectionMarkdownPassthrough(t *testing.T) {
	p := write(t, "a.md", "# Title\n\nbody **bold**")
	got, err := Projection(p)
	if err != nil || got != "# Title\n\nbody **bold**" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestProjectionHTMLStripsToText(t *testing.T) {
	p := write(t, "a.html", "<html><head><style>x{}</style></head><body><p>Hello <b>world</b></p><script>ignore()</script></body></html>")
	got, err := Projection(p)
	if err != nil || got != "Hello world" {
		t.Fatalf("got %q err %v", got, err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/source/ -v`
Expected: FAIL — `undefined: Projection`.

- [ ] **Step 4: Write `projection.go`**

Create `internal/source/projection.go`:

```go
// Package source produces the canonical text "projection" a study source is
// read and extracted from. Anchors in (from) strings are char offsets into this
// projection. Phase 2a handles local text/HTML; PDF and web URLs are later
// adapters behind the same Projection entry point.
package source

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

// Projection returns the text the reader renders and anchors into.
func Projection(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm":
		return htmlToText(data)
	default:
		return string(data), nil
	}
}

// htmlToText extracts visible text, dropping <script>/<style> content and
// collapsing all whitespace to single spaces (deterministic so char anchors are
// stable).
func htmlToText(data []byte) (string, error) {
	z := html.NewTokenizer(bytes.NewReader(data))
	var b strings.Builder
	skip := 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break // io.EOF terminates the token stream
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			if n := string(name); n == "script" || n == "style" {
				skip++
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if n := string(name); (n == "script" || n == "style") && skip > 0 {
				skip--
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " "), nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd /home/simone/projects/texere && go test ./internal/source/ -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/source/ go.mod go.sum
git commit -m "feat(source): text projection — txt/md passthrough, html->text"
```

---

### Task 2: Content writeback — create reading notes + update progress

**Files:**
- Create: `internal/content/reading.go`
- Test: `internal/content/reading_test.go`

**Interfaces:**
- Consumes: `GenerateShortID` (existing, `nodeid.go`), `readLines`/`writeLines` (existing, `writeback.go`, same package).
- Produces:
  - `func CreateReadingNotes(notesPath, sourceRel, title string) (refNodeID string, err error)` — writes the per-source notes scaffold and returns the generated `# @id` of the reading-element node. Fails if `notesPath` already exists.
  - `func UpdateProgressMarker(notesPath, newValue string) error` — rewrites the single `# @progress` line in the file to `# @progress <newValue>`.

- [ ] **Step 1: Write the failing test**

Create `internal/content/reading_test.go`:

```go
package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateReadingNotesScaffold(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "kahneman.n4l")
	id, err := CreateReadingNotes(notes, "sources/kahneman.txt", "Thinking Fast and Slow")
	if err != nil {
		t.Fatalf("CreateReadingNotes: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty ref node id")
	}
	body, _ := os.ReadFile(notes)
	s := string(body)
	for _, want := range []string{
		"- Thinking Fast and Slow",
		"# @id " + id,
		`# @ref "sources/kahneman.txt"`,
		`# @source "Thinking Fast and Slow"`,
		"# @progress char=0",
		"_sequence_",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("scaffold missing %q in:\n%s", want, s)
		}
	}
	// Creating again on the same path must fail (never overwrite).
	if _, err := CreateReadingNotes(notes, "sources/x.txt", "X"); err == nil {
		t.Fatal("expected error creating over an existing file")
	}
}

func TestUpdateProgressMarker(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "k.n4l")
	if _, err := CreateReadingNotes(notes, "sources/k.txt", "K"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateProgressMarker(notes, "char=4180"); err != nil {
		t.Fatalf("UpdateProgressMarker: %v", err)
	}
	body, _ := os.ReadFile(notes)
	if !strings.Contains(string(body), "# @progress char=4180") {
		t.Fatalf("progress not updated:\n%s", body)
	}
	if strings.Contains(string(body), "# @progress char=0") {
		t.Fatal("old progress value still present")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/content/ -run 'TestCreateReadingNotes|TestUpdateProgressMarker' -v`
Expected: FAIL — `undefined: CreateReadingNotes`.

- [ ] **Step 3: Write `reading.go`**

Create `internal/content/reading.go`:

```go
package content

import (
	"fmt"
	"os"
	"strings"
)

// CreateReadingNotes writes the per-source notes scaffold for a reading element
// and returns the generated # @id of the reading-element node. The reading
// element is a node (text = title) carrying # @ref/# @source/# @progress and an
// explicit # @id (these nodes are not study-marked, so InjectMissingIDs would
// not key them). Extracts are appended later under the _sequence_ context.
func CreateReadingNotes(notesPath, sourceRel, title string) (string, error) {
	if _, err := os.Stat(notesPath); err == nil {
		return "", fmt.Errorf("content: CreateReadingNotes: file exists: %s", notesPath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("content: CreateReadingNotes: stat: %w", err)
	}
	id, err := GenerateShortID()
	if err != nil {
		return "", err
	}
	lines := []string{
		"- " + title,
		"# @id " + id,
		`# @ref "` + sourceRel + `"`,
		`# @source "` + title + `"`,
		"# @progress char=0",
		title,
		":: " + title + " _sequence_",
		"",
	}
	if err := writeLines(notesPath, lines); err != nil {
		return "", fmt.Errorf("content: CreateReadingNotes: write: %w", err)
	}
	return id, nil
}

// UpdateProgressMarker rewrites the single "# @progress" line in notesPath.
func UpdateProgressMarker(notesPath, newValue string) error {
	lines, err := readLines(notesPath)
	if err != nil {
		return fmt.Errorf("content: UpdateProgressMarker: %w", err)
	}
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "# @progress") {
			lines[i] = "# @progress " + newValue
			if err := writeLines(notesPath, lines); err != nil {
				return fmt.Errorf("content: UpdateProgressMarker: write: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("content: UpdateProgressMarker: no # @progress line in %s", notesPath)
}
```

> Note: `writeLines`/`readLines` are existing unexported helpers in `internal/content/writeback.go`. Confirm `writeLines` joins with `\n` and writes atomically; if it does not append a trailing newline and the test needs one, the trailing `""` element in `lines` provides it.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/simone/projects/texere && go test ./internal/content/ -run 'TestCreateReadingNotes|TestUpdateProgressMarker' -v`
Expected: PASS.

- [ ] **Step 5: Run the whole content package + commit**

Run: `cd /home/simone/projects/texere && go test ./internal/content/`
Expected: all PASS (existing tests unaffected).

```bash
git add internal/content/reading.go internal/content/reading_test.go
git commit -m "feat(content): reading-notes scaffold + progress-marker writeback"
```

---

### Task 3: Content writeback — append extract

**Files:**
- Modify: `internal/content/reading.go`
- Test: `internal/content/reading_test.go` (add a test)

**Interfaces:**
- Consumes: `GenerateShortID`, `readLines`/`writeLines`.
- Produces: `func AppendExtract(notesPath, text, fromAnchor string) (extractID string, err error)` — appends a `_sequence_` extract node to the end of the notes file:
  ```
  # @id <generated>
  <single-line text>
      (from) "<fromAnchor>"
  ```
  The node text is the selection with internal newlines collapsed to single spaces (N4L nodes are single-line). Returns the generated `# @id`.

- [ ] **Step 1: Write the failing test**

Add to `internal/content/reading_test.go`:

```go
func TestAppendExtract(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "k.n4l")
	if _, err := CreateReadingNotes(notes, "sources/k.txt", "K"); err != nil {
		t.Fatal(err)
	}
	id, err := AppendExtract(notes, "System 1 is\nfast and automatic", "sources/k.txt char=1200")
	if err != nil {
		t.Fatalf("AppendExtract: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty extract id")
	}
	body, _ := os.ReadFile(notes)
	s := string(body)
	// Newlines in the selection collapse to a single space.
	for _, want := range []string{
		"# @id " + id,
		"System 1 is fast and automatic",
		`(from) "sources/k.txt char=1200"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("append missing %q in:\n%s", want, s)
		}
	}
	// The extract must come after the _sequence_ context line.
	if strings.Index(s, "_sequence_") > strings.Index(s, "# @id "+id) {
		t.Fatal("extract was not appended under the _sequence_ context")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/content/ -run TestAppendExtract -v`
Expected: FAIL — `undefined: AppendExtract`.

- [ ] **Step 3: Add `AppendExtract` to `reading.go`**

Append to `internal/content/reading.go`:

```go
// AppendExtract appends a verbatim extract node (with a (from) anchor and a
// generated # @id) to the end of the notes file, under the _sequence_ context
// established by CreateReadingNotes. Internal newlines in text collapse to
// single spaces so the node stays single-line.
func AppendExtract(notesPath, text, fromAnchor string) (string, error) {
	id, err := GenerateShortID()
	if err != nil {
		return "", err
	}
	lines, err := readLines(notesPath)
	if err != nil {
		return "", fmt.Errorf("content: AppendExtract: %w", err)
	}
	oneLine := strings.Join(strings.Fields(text), " ")
	lines = append(lines,
		"# @id "+id,
		oneLine,
		`    (from) "`+fromAnchor+`"`,
		"",
	)
	if err := writeLines(notesPath, lines); err != nil {
		return "", fmt.Errorf("content: AppendExtract: write: %w", err)
	}
	return id, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/simone/projects/texere && go test ./internal/content/ -run TestAppendExtract -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/content/reading.go internal/content/reading_test.go
git commit -m "feat(content): append verbatim extract node with (from) anchor"
```

---

### Task 4: Learning — reading_elements (schema v2) + scheduling

**Files:**
- Modify: `internal/learning/db.go` (bump `schemaVersion` to 2; stepwise migration; add `ddlV2`)
- Create: `internal/learning/reading.go`
- Test: `internal/learning/reading_test.go`

**Interfaces:**
- Consumes: `Store` (Phase 1).
- Produces:
  - `type ReadingElement struct { ID, RefPath, NotesPath, Title, Position string; ProjLen int; Due, LastRead time.Time; IntervalDays int; Done bool }`.
  - `func (s *Store) UpsertReading(r ReadingElement) error` (insert if new; on existing id, update display fields, preserve schedule).
  - `func (s *Store) ListReading() ([]ReadingElement, error)` (all, newest first).
  - `func (s *Store) GetReading(id string) (ReadingElement, bool, error)`.
  - `func (s *Store) SetReadingProgress(id, position string, projLen int) error`.
  - `func (s *Store) ApplyReadingSchedule(id string, r ReadingElement) error` (writes due/interval/last_read/done).
  - `func ScheduleReading(prev ReadingElement, action string, now time.Time) ReadingElement` — pure; `action ∈ {"continue","later","done"}`. continue/later: `interval = min(round(max(1,interval)×1.5), 30)`, `due = now + interval days`, `last_read = now`; done: `Done = true`. "continue" also is the default on leaving.

- [ ] **Step 1: Bump schema + stepwise migration in `db.go`**

In `internal/learning/db.go`: change `const schemaVersion = 1` to `const schemaVersion = 2`, and replace the body of `migrate()` after the version read with a stepwise apply:

```go
	for v < schemaVersion {
		switch v {
		case 0:
			if _, err := s.db.Exec(ddlV1); err != nil {
				return fmt.Errorf("learning: apply v1: %w", err)
			}
		case 1:
			if _, err := s.db.Exec(ddlV2); err != nil {
				return fmt.Errorf("learning: apply v2: %w", err)
			}
		}
		v++
	}
	if _, err := s.db.Exec(`DELETE FROM schema_meta`); err != nil {
		return fmt.Errorf("learning: reset schema_meta: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_meta(version) VALUES (?)`, schemaVersion); err != nil {
		return fmt.Errorf("learning: write schema_meta: %w", err)
	}
	return nil
```

(Keep the `if v >= schemaVersion { return nil }` short-circuit above this loop.) Add the new DDL constant next to `ddlV1`:

```go
const ddlV2 = `
CREATE TABLE IF NOT EXISTS reading_elements (
  id            TEXT PRIMARY KEY,
  ref_path      TEXT NOT NULL,
  notes_path    TEXT NOT NULL,
  title         TEXT NOT NULL,
  position      TEXT,
  proj_len      INTEGER,
  due           INTEGER,
  interval_days INTEGER NOT NULL DEFAULT 1,
  last_read     INTEGER,
  done          INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_reading_due ON reading_elements(due) WHERE done = 0;
`
```

- [ ] **Step 2: Write the failing test**

Create `internal/learning/reading_test.go`:

```go
package learning

import (
	"testing"
	"time"
)

func TestReadingCRUDAndSchedule(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_000_000, 0)

	r := ReadingElement{ID: "r1", RefPath: "sources/k.txt", NotesPath: "/c/reading/k.n4l",
		Title: "K", Position: "char=0", ProjLen: 10000, IntervalDays: 1, Done: false}
	if err := s.UpsertReading(r); err != nil {
		t.Fatalf("UpsertReading: %v", err)
	}
	if err := s.SetReadingProgress("r1", "char=4180", 10000); err != nil {
		t.Fatalf("SetReadingProgress: %v", err)
	}
	got, ok, err := s.GetReading("r1")
	if err != nil || !ok || got.Position != "char=4180" {
		t.Fatalf("GetReading got=%+v ok=%v err=%v", got, ok, err)
	}

	// Schedule "continue": interval grows 1 -> 2 (round(1*1.5)=2), due in the future.
	sched := ScheduleReading(got, "continue", now)
	if sched.IntervalDays != 2 {
		t.Fatalf("interval=%d want 2", sched.IntervalDays)
	}
	if !sched.Due.After(now) {
		t.Fatalf("due %v not after now", sched.Due)
	}
	if err := s.ApplyReadingSchedule("r1", sched); err != nil {
		t.Fatalf("ApplyReadingSchedule: %v", err)
	}

	// "done" marks done.
	d := ScheduleReading(sched, "done", now)
	if !d.Done {
		t.Fatal("expected Done")
	}

	// Interval cap at 30.
	big := ReadingElement{IntervalDays: 25}
	if g := ScheduleReading(big, "continue", now); g.IntervalDays != 30 {
		t.Fatalf("capped interval=%d want 30", g.IntervalDays)
	}

	list, err := s.ListReading()
	if err != nil || len(list) != 1 {
		t.Fatalf("ListReading len=%d err=%v want 1", len(list), err)
	}
}

func TestMigrationV1toV2Additive(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir) // fresh open creates v2 directly
	if err != nil {
		t.Fatal(err)
	}
	// items table (v1) and reading_elements (v2) both exist.
	if _, err := s.db.Exec(`INSERT INTO items
		(id,node_id,file_path,card_type,front,back,active,reps,lapses,state,created_at)
		VALUES ('i','n','/f','cloze','f','b',1,0,0,0,0)`); err != nil {
		t.Fatalf("items missing after v2: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO reading_elements
		(id,ref_path,notes_path,title,interval_days,done,created_at)
		VALUES ('r','s','n','t',1,0,0)`); err != nil {
		t.Fatalf("reading_elements missing: %v", err)
	}
	s.Close()
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run 'TestReadingCRUD|TestMigrationV1' -v`
Expected: FAIL — `undefined: ReadingElement` / `ScheduleReading`.

- [ ] **Step 4: Write `reading.go`**

Create `internal/learning/reading.go`:

```go
package learning

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

// ReadingElement is one source being read incrementally.
type ReadingElement struct {
	ID           string
	RefPath      string
	NotesPath    string
	Title        string
	Position     string
	ProjLen      int
	Due          time.Time
	LastRead     time.Time
	IntervalDays int
	Done         bool
}

const readingIntervalCap = 30

// ScheduleReading is the pure resurface rule. action ∈ {"continue","later","done"}.
func ScheduleReading(prev ReadingElement, action string, now time.Time) ReadingElement {
	out := prev
	if action == "done" {
		out.Done = true
		return out
	}
	iv := prev.IntervalDays
	if iv < 1 {
		iv = 1
	}
	iv = int(math.Round(float64(iv) * 1.5))
	if iv > readingIntervalCap {
		iv = readingIntervalCap
	}
	out.IntervalDays = iv
	out.Due = now.Add(time.Duration(iv) * 24 * time.Hour)
	out.LastRead = now
	return out
}

func (s *Store) UpsertReading(r ReadingElement) error {
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM reading_elements WHERE id=?`, r.ID).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		_, err := s.db.Exec(`UPDATE reading_elements SET ref_path=?, notes_path=?, title=? WHERE id=?`,
			r.RefPath, r.NotesPath, r.Title, r.ID)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO reading_elements
		(id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.RefPath, r.NotesPath, r.Title, r.Position, r.ProjLen,
		nullableUnix(r.Due), maxInt(r.IntervalDays, 1), nullableUnix(r.LastRead), boolToInt(r.Done), time.Now().Unix())
	return err
}

func (s *Store) SetReadingProgress(id, position string, projLen int) error {
	_, err := s.db.Exec(`UPDATE reading_elements SET position=?, proj_len=? WHERE id=?`, position, projLen, id)
	return err
}

func (s *Store) ApplyReadingSchedule(id string, r ReadingElement) error {
	_, err := s.db.Exec(`UPDATE reading_elements SET due=?, interval_days=?, last_read=?, done=? WHERE id=?`,
		nullableUnix(r.Due), r.IntervalDays, nullableUnix(r.LastRead), boolToInt(r.Done), id)
	return err
}

func (s *Store) GetReading(id string) (ReadingElement, bool, error) {
	row := s.db.QueryRow(`SELECT id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done
		FROM reading_elements WHERE id=?`, id)
	r, err := scanReading(row)
	if err == sql.ErrNoRows {
		return ReadingElement{}, false, nil
	}
	return r, err == nil, err
}

func (s *Store) ListReading() ([]ReadingElement, error) {
	rows, err := s.db.Query(`SELECT id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done
		FROM reading_elements ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReadingElement
	for rows.Next() {
		r, err := scanReading(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanReading(sc scanner) (ReadingElement, error) {
	var (
		r              ReadingElement
		pos            sql.NullString
		projLen        sql.NullInt64
		due, last      sql.NullInt64
		done           int
	)
	if err := sc.Scan(&r.ID, &r.RefPath, &r.NotesPath, &r.Title, &pos, &projLen, &due, &r.IntervalDays, &last, &done); err != nil {
		return ReadingElement{}, err
	}
	r.Position = pos.String
	r.ProjLen = int(projLen.Int64)
	r.Due = unixToTime(due)
	r.LastRead = unixToTime(last)
	r.Done = done != 0
	return r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

(`nullableUnix` and `unixToTime` already exist in `store.go`.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -v`
Expected: all PASS (Phase-1 tests + the two new ones).

- [ ] **Step 6: Commit**

```bash
git add internal/learning/db.go internal/learning/reading.go internal/learning/reading_test.go
git commit -m "feat(learning): reading_elements table (schema v2) + resurface schedule"
```

---

### Task 5: Bindings — import, list, projection, extract, progress, reschedule

**Files:**
- Create: `reading_methods.go` (repo root)
- Modify: `frontend/wailsjs/go/main/App.js` + `App.d.ts` (hand-add bindings; `wails` unavailable)
- Test: `reading_methods_test.go` (repo root)

**Interfaces:**
- Consumes: `source.Projection`, `content.CreateReadingNotes/AppendExtract/UpdateProgressMarker`, `learning.*Reading*`/`ScheduleReading`, `atomicWriteText` (methods.go), `App.learnStore`, `App.collectionDir`, `runtime.OpenFileDialog`/`EventsEmit`.
- Produces (App methods):
  - `ImportTextSource() (*ReadingView, error)` — opens a file dialog (txt/md/html), copies into `sources/` (suffix on collision), creates `reading/<name>.n4l` scaffold, registers a reading element; returns the view, or `nil` if the dialog was cancelled.
  - `GetReadingList() ([]ReadingView, error)`.
  - `GetSourceProjection(refPath string) (string, error)`.
  - `CreateExtract(id, text string, anchorChar int) (*ExtractView, error)` — appends the extract, updates `# @progress` + DB position, returns `{id, line}`; emits `file:ids-injected` for the notes file (clean-buffer reload).
  - `UpdateReadingProgress(id, position string) error`.
  - `RescheduleReading(id, action string) (*ReadingView, error)`.
  - `type ReadingView struct { ID, RefPath, NotesPath, Title, Position string; Percent, Due int; Done bool }` (`Due` = unix secs, 0 if none); `type ExtractView struct { ID string; ... }`.

- [ ] **Step 1: Write the failing test**

Create `reading_methods_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"texere/internal/content"
	"texere/internal/learning"
)

func TestCreateExtractAppendsAndAdvances(t *testing.T) {
	dir := t.TempDir()
	// Set up a source + reading element by hand (no dialog).
	os.MkdirAll(filepath.Join(dir, "sources"), 0o755)
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	os.WriteFile(filepath.Join(dir, "sources", "k.txt"), []byte("System 1 is fast and automatic. System 2 is slow."), 0o644)
	notes := filepath.Join(dir, "reading", "k.n4l")
	refID, err := content.CreateReadingNotes(notes, "sources/k.txt", "k")
	if err != nil {
		t.Fatal(err)
	}
	store, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	store.UpsertReading(learning.ReadingElement{ID: refID, RefPath: "sources/k.txt", NotesPath: notes, Title: "k", IntervalDays: 1})

	a := &App{collectionDir: dir, learnStore: store, registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}}
	defer store.Close()

	ev, err := a.CreateExtract(refID, "System 1 is fast", 0)
	if err != nil || ev == nil || ev.ID == "" {
		t.Fatalf("CreateExtract ev=%+v err=%v", ev, err)
	}
	body, _ := os.ReadFile(notes)
	if !strings.Contains(string(body), `(from) "sources/k.txt char=0"`) {
		t.Fatalf("extract anchor missing:\n%s", body)
	}
	// Reschedule "done" then list reflects it.
	if _, err := a.RescheduleReading(refID, "done"); err != nil {
		t.Fatalf("RescheduleReading: %v", err)
	}
	list, _ := a.GetReadingList()
	if len(list) != 1 || !list[0].Done {
		t.Fatalf("list=%+v want 1 done", list)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test . -run TestCreateExtractAppendsAndAdvances -v`
Expected: FAIL — `a.CreateExtract undefined`.

- [ ] **Step 3: Write `reading_methods.go`**

Create `reading_methods.go`:

```go
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/content"
	"texere/internal/learning"
	"texere/internal/source"
)

// ReadingView is the frontend-facing shape of a reading element.
type ReadingView struct {
	ID        string `json:"id"`
	RefPath   string `json:"refPath"`
	NotesPath string `json:"notesPath"`
	Title     string `json:"title"`
	Position  string `json:"position"`
	Percent   int    `json:"percent"`
	Due       int64  `json:"due"`
	Done      bool   `json:"done"`
}

// ExtractView is returned after appending an extract.
type ExtractView struct {
	ID string `json:"id"`
}

func toReadingView(r learning.ReadingElement) ReadingView {
	pct := 0
	if r.ProjLen > 0 {
		if off := parseCharAnchor(r.Position); off > 0 {
			pct = int(float64(off) / float64(r.ProjLen) * 100)
			if pct > 100 {
				pct = 100
			}
		}
	}
	var due int64
	if !r.Due.IsZero() {
		due = r.Due.Unix()
	}
	return ReadingView{ID: r.ID, RefPath: r.RefPath, NotesPath: r.NotesPath, Title: r.Title,
		Position: r.Position, Percent: pct, Due: due, Done: r.Done}
}

// parseCharAnchor pulls N from "char=N" (0 if absent/malformed).
func parseCharAnchor(s string) int {
	i := strings.Index(s, "char=")
	if i < 0 {
		return 0
	}
	n := 0
	for _, c := range s[i+5:] {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ImportTextSource opens a file dialog, copies the chosen text/HTML file into
// sources/, scaffolds reading/<name>.n4l, and registers a reading element.
func (a *App) ImportTextSource() (*ReadingView, error) {
	if a.learnStore == nil || a.collectionDir == "" {
		return nil, errors.New("no collection open")
	}
	picked, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import a text or HTML source",
		Filters: []runtime.FileFilter{{
			DisplayName: "Text / Markdown / HTML",
			Pattern:     "*.txt;*.md;*.html;*.htm",
		}},
	})
	if err != nil {
		return nil, err
	}
	if picked == "" {
		return nil, nil // cancelled
	}
	base := filepath.Base(picked)
	srcRel, srcAbs := uniqueDest(a.collectionDir, "sources", base)
	if err := copyFile(picked, srcAbs); err != nil {
		return nil, fmt.Errorf("ImportTextSource: copy: %w", err)
	}
	title := strings.TrimSuffix(filepath.Base(srcRel), filepath.Ext(srcRel))
	notesRel, notesAbs := uniqueDest(a.collectionDir, "reading", title+".n4l")
	_ = notesRel
	refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
	if err != nil {
		return nil, fmt.Errorf("ImportTextSource: notes: %w", err)
	}
	proj, _ := source.Projection(srcAbs)
	r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
		Position: "char=0", ProjLen: len(proj), IntervalDays: 1}
	if err := a.learnStore.UpsertReading(r); err != nil {
		return nil, fmt.Errorf("ImportTextSource: register: %w", err)
	}
	v := toReadingView(r)
	return &v, nil
}

// uniqueDest returns (relPath, absPath) under <collection>/<subdir>/<name>,
// suffixing the stem with -2, -3, … until the path does not exist.
func uniqueDest(collectionDir, subdir, name string) (string, string) {
	os.MkdirAll(filepath.Join(collectionDir, subdir), 0o755)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 2; ; i++ {
		abs := filepath.Join(collectionDir, subdir, candidate)
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return filepath.ToSlash(filepath.Join(subdir, candidate)), abs
		}
		candidate = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (a *App) GetReadingList() ([]ReadingView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	rs, err := a.learnStore.ListReading()
	if err != nil {
		return nil, err
	}
	out := make([]ReadingView, 0, len(rs))
	for _, r := range rs {
		out = append(out, toReadingView(r))
	}
	return out, nil
}

func (a *App) GetSourceProjection(refPath string) (string, error) {
	if a.collectionDir == "" {
		return "", errors.New("no collection open")
	}
	return source.Projection(filepath.Join(a.collectionDir, filepath.FromSlash(refPath)))
}

// CreateExtract appends a verbatim extract, advances # @progress + DB position,
// and signals the notes editor to reload (clean buffer only).
func (a *App) CreateExtract(id, text string, anchorChar int) (*ExtractView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return nil, fmt.Errorf("CreateExtract: reading element %q not found", id)
	}
	anchor := fmt.Sprintf("%s char=%d", r.RefPath, anchorChar)
	a.savingPath = r.NotesPath
	exID, err := content.AppendExtract(r.NotesPath, text, anchor)
	if err == nil {
		err = content.UpdateProgressMarker(r.NotesPath, fmt.Sprintf("char=%d", anchorChar))
	}
	a.savingPath = ""
	if err != nil {
		return nil, fmt.Errorf("CreateExtract: %w", err)
	}
	_ = a.learnStore.SetReadingProgress(id, fmt.Sprintf("char=%d", anchorChar), r.ProjLen)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "file:ids-injected", map[string]string{"path": r.NotesPath})
	}
	return &ExtractView{ID: exID}, nil
}

func (a *App) UpdateReadingProgress(id, position string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return fmt.Errorf("UpdateReadingProgress: %q not found", id)
	}
	a.savingPath = r.NotesPath
	wErr := content.UpdateProgressMarker(r.NotesPath, position)
	a.savingPath = ""
	if wErr != nil {
		return wErr
	}
	return a.learnStore.SetReadingProgress(id, position, r.ProjLen)
}

func (a *App) RescheduleReading(id, action string) (*ReadingView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return nil, fmt.Errorf("RescheduleReading: %q not found", id)
	}
	sched := learning.ScheduleReading(r, action, time.Now())
	if err := a.learnStore.ApplyReadingSchedule(id, sched); err != nil {
		return nil, err
	}
	v := toReadingView(sched)
	return &v, nil
}
```

- [ ] **Step 4: Run the test + build**

Run: `cd /home/simone/projects/texere && go test . -run TestCreateExtractAppendsAndAdvances -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Hand-add the JS bindings**

`wails` is not installed; append to `frontend/wailsjs/go/main/App.js` (matching the existing `export function X(args){ return window['go']['main']['App']['X'](args); }` style) the six functions: `ImportTextSource()`, `GetReadingList()`, `GetSourceProjection(arg1)`, `CreateExtract(arg1,arg2,arg3)`, `UpdateReadingProgress(arg1,arg2)`, `RescheduleReading(arg1,arg2)`. Add matching loose declarations to `App.d.ts` (`export function CreateExtract(arg1:string,arg2:string,arg3:number):Promise<any>;` etc.; use `Promise<any>` to avoid referencing ungenerated model types).

- [ ] **Step 6: Verify frontend still builds + commit**

Run: `cd /home/simone/projects/texere && (cd frontend && npm run build)`
Expected: exit 0.

```bash
git add reading_methods.go reading_methods_test.go frontend/wailsjs/go/main/App.js frontend/wailsjs/go/main/App.d.ts
git commit -m "feat(reading): App bindings — import, list, projection, extract, progress, reschedule"
```

---

### Task 6: Frontend — Sources panel, store, app entry

**Files:**
- Create: `frontend/src/stores/reading.js`
- Create: `frontend/src/components/SourcesPanel.svelte`
- Modify: `frontend/src/App.svelte`

**Interfaces:**
- Consumes: bindings `ImportTextSource`, `GetReadingList` from `../../wailsjs/go/main/App`.
- Produces: `readingOpen` (panel visible) + `activeReading` (`ReadingView | null`, opens the ReadingView in Task 7) stores.

- [ ] **Step 1: Create the store**

Create `frontend/src/stores/reading.js`:

```js
import { writable } from 'svelte/store'

// readingOpen — Sources panel overlay visible.
export const readingOpen = writable(false)
// activeReading — the reading element currently open in the ReadingView, or null.
export const activeReading = writable(/** @type {any} */ (null))
```

- [ ] **Step 2: Create `SourcesPanel.svelte`**

Create `frontend/src/components/SourcesPanel.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import { ImportTextSource, GetReadingList } from '../../wailsjs/go/main/App'
  import { activeReading } from '../stores/reading.js'

  export let onClose = null

  let items = []
  let error = ''

  async function load() {
    try {
      items = (await GetReadingList()) ?? []
    } catch (e) {
      error = String(e?.message ?? e)
    }
  }

  async function importSource() {
    error = ''
    try {
      const v = await ImportTextSource()
      if (v) {
        await load()
        activeReading.set(v) // open it immediately
      }
    } catch (e) {
      error = String(e?.message ?? e)
    }
  }

  function open(v) { activeReading.set(v) }

  const dueNow = (v) => !v.done && v.due !== 0 && v.due * 1000 <= Date.now()

  onMount(load)
</script>

<div class="sources-root" role="dialog" aria-label="Sources">
  <div class="sources-bar">
    <span class="sources-title">Sources</span>
    <button class="sources-import" on:click={importSource}>+ Import</button>
    <button class="sources-close" on:click={() => onClose?.()} title="Close (Esc)">×</button>
  </div>
  {#if error}<div class="sources-error">{error}</div>{/if}
  {#if items.length === 0}
    <div class="sources-empty">No sources yet. Import a .txt, .md, or .html file to start reading.</div>
  {:else}
    <ul class="sources-list">
      {#each items as v}
        <li class="sources-item" on:click={() => open(v)}>
          <span class="si-title">{v.title}</span>
          <span class="si-pct">{v.percent}%</span>
          {#if v.done}<span class="si-tag done">done</span>
          {:else if dueNow(v)}<span class="si-tag due">due</span>{/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .sources-root { position: absolute; top: 0; right: 0; bottom: 0; width: 340px; z-index: 55;
    background: #131c29; border-left: 1px solid #2a3a4a; display: flex; flex-direction: column; }
  .sources-bar { display: flex; align-items: center; gap: 10px; padding: 8px 12px;
    border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .sources-title { color: #7dd3fc; font-weight: 600; flex: 1; }
  .sources-import, .sources-close { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a;
    border-radius: 6px; padding: 4px 10px; cursor: pointer; font-size: 12px; }
  .sources-close { background: none; border: none; color: #8899aa; font-size: 18px; }
  .sources-error { color: #ff8080; padding: 8px 12px; font-size: 12px; }
  .sources-empty { color: #8aa; padding: 16px 12px; font-size: 13px; }
  .sources-list { list-style: none; margin: 0; padding: 0; overflow: auto; }
  .sources-item { display: flex; align-items: center; gap: 8px; padding: 8px 12px;
    border-bottom: 1px solid #1d2a3a; cursor: pointer; }
  .sources-item:hover { background: #18222f; }
  .si-title { flex: 1; color: #cdd6e0; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .si-pct { color: #7ecfa0; font-size: 12px; }
  .si-tag { font-size: 10px; border-radius: 3px; padding: 1px 5px; }
  .si-tag.due { background: #4a3a1a; color: #e0b000; }
  .si-tag.done { background: #1a3a2a; color: #7ecfa0; }
</style>
```

- [ ] **Step 3: Wire into `App.svelte`**

In `frontend/src/App.svelte`:
1. Imports: `import SourcesPanel from './components/SourcesPanel.svelte'` and `import { readingOpen } from './stores/reading.js'`.
2. In the capture-phase `handleKeydown` (next to the Ctrl+Shift+R study branch), add:
```js
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'e' || e.key === 'E')) {
      e.preventDefault()
      e.stopPropagation()
      readingOpen.update((v) => !v)
      return
    }
```
3. In the overlay region (inside the `collectionOpen` guard, near `{#if $studyOpen}`), add:
```svelte
    {#if $readingOpen}
      <SourcesPanel onClose={() => readingOpen.set(false)} />
    {/if}
```

> Read `App.svelte`'s `handleKeydown` and overlay region first; match the existing placement and the `collectionOpen` guard so the panel only opens with a collection loaded.

- [ ] **Step 4: Verify build + manual + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0.

Manual (`make dev`): Ctrl+Shift+E opens the Sources panel; **+ Import** opens a file dialog; pick a `.txt` → it appears in the list with `0%` and the file tree shows a new `reading/<name>.n4l` + `sources/<name>`.

```bash
git add frontend/src/stores/reading.js frontend/src/components/SourcesPanel.svelte frontend/src/App.svelte
git commit -m "feat(reading): Sources panel overlay (Ctrl+Shift+E) + import"
```

---

### Task 7: Frontend — ReadingView (source pane + extract + notes editor)

**Files:**
- Create: `frontend/src/components/ReadingView.svelte`
- Modify: `frontend/src/App.svelte` (render ReadingView when `activeReading` is set)

**Interfaces:**
- Consumes: bindings `GetSourceProjection`, `CreateExtract`, `UpdateReadingProgress`, `RescheduleReading`, `GetFileContent`, `SaveFile`; the `activeReading` store; the n4l CodeMirror extensions (`lib/n4l-language.js`, `n4l-highlight.js`, `arrow-autocomplete.js`).
- Produces: a self-contained reading surface (source pane + notes editor) for 2b reuse.

- [ ] **Step 1: Create `ReadingView.svelte`**

Create `frontend/src/components/ReadingView.svelte`:

```svelte
<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView, keymap, lineNumbers } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { defaultKeymap, historyKeymap, history } from '@codemirror/commands'
  import { autocompletion } from '@codemirror/autocomplete'
  import { n4lLanguage } from '../lib/n4l-language.js'
  import { n4lHighlighting } from '../lib/n4l-highlight.js'
  import { GetSourceProjection, CreateExtract, UpdateReadingProgress, RescheduleReading, GetFileContent, SaveFile } from '../../wailsjs/go/main/App'
  import { activeReading } from '../stores/reading.js'

  export let reading            // ReadingView object
  export let onClose = null

  let projection = ''
  let srcEl                     // the source-pane element (for selection offset)
  let notesContainer
  let notesView
  let savedNotes = ''
  let error = ''

  function charOffset(node) {
    // Offset of the selection start within the rendered projection text.
    const sel = window.getSelection()
    if (!sel || sel.rangeCount === 0 || !srcEl.contains(sel.anchorNode)) return null
    const range = sel.getRangeAt(0)
    const pre = range.cloneRange()
    pre.selectNodeContents(srcEl)
    pre.setEnd(range.startContainer, range.startOffset)
    return { offset: pre.toString().length, text: sel.toString() }
  }

  async function extract() {
    const sel = charOffset()
    if (!sel || !sel.text.trim()) return
    try {
      await flushNotes() // never lose an unsaved notes edit
      await CreateExtract(reading.id, sel.text, sel.offset)
      await reloadNotes() // pick up the appended extract
    } catch (e) {
      error = String(e?.message ?? e)
    }
  }

  async function flushNotes() {
    if (!notesView) return
    const cur = notesView.state.doc.toString()
    if (cur !== savedNotes) {
      const res = await SaveFile(reading.notesPath, cur)
      savedNotes = res?.finalContent ?? cur
      if (res?.finalContent) setNotes(res.finalContent)
    }
  }

  async function reloadNotes() {
    const text = (await GetFileContent(reading.notesPath)) ?? ''
    setNotes(text)
  }

  function setNotes(text) {
    savedNotes = text
    notesView?.dispatch({ changes: { from: 0, to: notesView.state.doc.length, insert: text } })
  }

  async function done() {
    await flushNotes()
    await RescheduleReading(reading.id, 'done')
    close('done')
  }

  async function close(action) {
    try {
      await flushNotes()
      if (action !== 'done') await RescheduleReading(reading.id, action || 'continue')
    } catch (e) { /* best-effort on close */ }
    activeReading.set(null)
    onClose?.()
  }

  function onKey(e) {
    if (e.key === 'Escape') { close('continue'); return }
    if ((e.ctrlKey || e.metaKey) && (e.key === 'e' || e.key === 'E')) { e.preventDefault(); extract() }
    if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) { e.preventDefault(); flushNotes() }
  }

  onMount(async () => {
    try {
      projection = (await GetSourceProjection(reading.refPath)) ?? ''
    } catch (e) {
      error = String(e?.message ?? e)
    }
    const text = (await GetFileContent(reading.notesPath)) ?? ''
    savedNotes = text
    notesView = new EditorView({
      parent: notesContainer,
      state: EditorState.create({
        doc: text,
        extensions: [
          lineNumbers(), history(), n4lLanguage, n4lHighlighting,
          autocompletion(),
          keymap.of([...defaultKeymap, ...historyKeymap]),
          EditorView.lineWrapping,
          EditorView.theme({ '&': { height: '100%', background: '#1b2636' },
            '.cm-content': { fontFamily: 'monospace', fontSize: '0.85rem', color: '#e8eaf0' } }),
        ],
      }),
    })
  })

  onDestroy(() => notesView?.destroy())
</script>

<svelte:window on:keydown={onKey} />

<div class="reading-root" role="dialog" aria-label="Reading">
  <div class="reading-bar">
    <span class="reading-title">{reading.title}</span>
    <button class="rb-btn" on:click={extract} title="Extract selection (Ctrl+E)">✂ Extract</button>
    <button class="rb-btn" on:click={() => close('later')}>⏳ Later</button>
    <button class="rb-btn" on:click={done}>✓ Done</button>
    <span class="rb-spacer"></span>
    <button class="rb-close" on:click={() => close('continue')} title="Close (Esc)">×</button>
  </div>
  {#if error}<div class="reading-error">{error}</div>{/if}
  <div class="reading-body">
    <div class="reading-source" bind:this={srcEl}>{projection}</div>
    <div class="reading-notes" bind:this={notesContainer}></div>
  </div>
</div>

<style>
  .reading-root { position: absolute; inset: 0; z-index: 58; background: #131c29; display: flex; flex-direction: column; }
  .reading-bar { display: flex; align-items: center; gap: 8px; padding: 8px 12px;
    border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .reading-title { color: #7dd3fc; font-weight: 600; margin-right: 8px; }
  .rb-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px;
    padding: 4px 10px; cursor: pointer; font-size: 12px; }
  .rb-btn:hover { background: #244a72; }
  .rb-spacer { flex: 1; }
  .rb-close { background: none; border: none; color: #8899aa; font-size: 18px; cursor: pointer; }
  .reading-error { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .reading-body { flex: 1; min-height: 0; display: flex; }
  .reading-source { flex: 1; overflow: auto; padding: 16px 20px; white-space: pre-wrap;
    color: #cdd6e0; font-size: 14px; line-height: 1.6; border-right: 1px solid #2a3a4a;
    user-select: text; }
  .reading-notes { flex: 1; min-width: 0; overflow: hidden; }
</style>
```

> The source pane uses `white-space: pre-wrap` so the rendered text matches the projection character-for-character — this keeps the DOM-range char offset (`charOffset`) aligned with the backend `char=` anchor. Do not collapse or transform the projection text in the DOM.

- [ ] **Step 2: Render it in `App.svelte`**

In `frontend/src/App.svelte`, add the import `import ReadingView from './components/ReadingView.svelte'` and `activeReading` to the `reading.js` import, then in the overlay region add (after the SourcesPanel block):

```svelte
    {#if $activeReading}
      <ReadingView reading={$activeReading} />
    {/if}
```

- [ ] **Step 3: Verify build**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0, no new errors.

- [ ] **Step 4: Manual verification (quickstart)**

`make dev`, with a collection open:
1. Ctrl+Shift+E → Import a `.txt` → it opens in the ReadingView; the left pane shows the text, the right pane is the `reading/<name>.n4l` editor.
2. Select a passage in the left pane → **Extract** (or Ctrl+E) → an extract node with `(from) "sources/<name> char=N"` appears in the notes editor under `_sequence_`, and `# @progress` updates.
3. **Later** → close; reopen via the Sources panel → the source's due date moved out. **Done** → it's tagged "done" in the list.
4. Import a `.html` → the left pane shows readable stripped text; extracting from it produces a correct `char=` anchor.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ReadingView.svelte frontend/src/App.svelte
git commit -m "feat(reading): ReadingView — source pane + select-to-extract + notes editor"
```

---

## Self-Review

**Spec coverage (design §s → tasks):**
- §2.1 text/HTML first → Task 1 (`Projection`). ✅
- §2.2 per-source notes file + scaffold → Task 2 (`CreateReadingNotes`). ✅
- §2.3 verbatim extract → Task 3 (`AppendExtract`) + Task 7 (select→extract). ✅
- §2.4 standalone overlay → Tasks 6 (Sources panel) + 7 (ReadingView). ✅
- §2.5 own `reading_elements` table → Task 4. ✅
- §4 N4L model (`# @ref`/`# @source`/`# @progress`/`(from)`/`_sequence_`, explicit `# @id` gen) → Tasks 2-3. ✅
- §5 flow (import → read → extract → progress → reschedule) → Task 5 (bindings) + Tasks 6-7 (UI). ✅
- §6 reading schedule (×1.5, cap 30, Later, Done) → Task 4 (`ScheduleReading`). ✅
- §7 components (`internal/source`, learning v2, content writeback, bindings, ReadingView, SourcesPanel) → Tasks 1-7. ✅
- §8 schema v2 (additive migration) → Task 4 (stepwise migrate + `ddlV2` + migration test). ✅
- §9 edge cases: dup-name suffix → Task 5 (`uniqueDest`, tested via name reuse in real use; the unit covers the suffixing path indirectly); dirty-buffer invariant → Tasks 5 (`savingPath` + `file:ids-injected`) + 7 (`flushNotes` before extract/close); html-no-text → Task 1 (empty projection). ✅
- §10 testing → Go units in Tasks 1-5; manual quickstart in Tasks 6-7. ✅

**Placeholder scan:** No TBD/TODO; every code step has complete code; commands have expected output. The JS-bindings hand-add (Task 5 Step 5) mirrors the Phase-1 precedent (wails unavailable). ✅

**Type consistency:** `ReadingElement` fields are identical in Task 4 (definition) and Task 5 (construction/scan). `ReadingView`/`ExtractView` JSON shapes defined in Task 5 are consumed unchanged in Tasks 6-7. `ScheduleReading(prev, action, now)` signature matches its Task 5 call. `CreateReadingNotes`/`AppendExtract`/`UpdateProgressMarker` signatures match between Tasks 2-3 (definition) and Task 5 (calls). The `char=N` anchor format is produced in Task 5 (`fmt.Sprintf("%s char=%d", …)`) and parsed in Task 5 (`parseCharAnchor`) consistently. ✅

**One flagged gap:** `uniqueDest`'s suffixing is exercised only in real use, not a dedicated unit test (the `reading_methods_test.go` sets up files by hand). Acceptable for 2a — `uniqueDest` is simple and the manual quickstart covers re-import; a unit test can be added if the reviewer wants it.
