# Processing Notebook — Slice 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a split-screen Process view — the existing N4L editor on the left, a persistent notebook reasoning workspace on the right — whose only job is to produce N4L that gets pushed into the editor, with provenance dots linking pushed nodes back to their notebook.

**Architecture:** `ProcessView.svelte` becomes a thin container (Splitter + shared left-editor handle) hosting `ProcessEditor` (today's editor, lifted out) and `NotebookPane`. The pane renders a generated read-only **context cell** plus editable **N4L draft cells**; pushing a draft inserts at the editor cursor and records provenance. A new SQLite v5 schema (`notebooks`, `notebook_links`) persists user cells and links. Everything else (LLM/graph/compute/sketch/image cells, queue-bar redesign) is a later slice.

**Tech Stack:** Go 1.22 (Wails bindings + `internal/learning` SQLite via `modernc.org/sqlite`, `internal/content` parser, `internal/source` projection, `internal/search`), Svelte 3.49 + CodeMirror 6, Wails v2.12.

## Global Constraints

- Go: standard `gofmt`; imports grouped stdlib / external / internal.
- No CGo except via Wails; **all new dependencies MUST be pure Go** — this slice adds **no** new third-party dependencies (Go or npm).
- SQLite is single-writer (`db.SetMaxOpenConns(1)` already set); follow the existing versioned-DDL migration pattern in `internal/learning/db.go`.
- Notebooks are lab notebooks: not in the knowledge graph, not scheduled, not studied, not committed to git (`learning.db` is already git-ignored).
- Frontend has no JS test runner; frontend tasks verify with `cd frontend && npm run build` plus explicit manual steps, matching the repo's existing manual-test-plan practice.

## Design correction vs. the spec (read before starting)

The approved design said provenance links are keyed by `node_id`. Grounding the code showed node IDs are **random 8-char short IDs written as `# @id` markers, injected by `SaveFile` only for study-marked (`# @card`) nodes** (`internal/content/nodeid.go`, `SaveFile` in `methods.go`). Most pushed relationship notes therefore have **no** `node_id`, and `SaveFile`'s injection **shifts line numbers**. So this plan keys `notebook_links` on the stable, line-independent tuple **(file_path, chapter, node_text)** and resolves dots by re-parsing the file. This is the only deviation from the spec; everything else matches.

---

## File Structure

**Backend (Go)**
- `internal/learning/db.go` — modify: bump `schemaVersion` to 5, add `ddlV5`, wire migration case.
- `internal/learning/notebooks.go` — create: `Notebook` / `NotebookLink` types + store CRUD.
- `internal/learning/notebooks_test.go` — create: store tests.
- `internal/source/projection.go` — modify: add pure `Excerpt(full, needle)` helper.
- `internal/source/projection_test.go` — modify: add `Excerpt` tests.
- `notebook_methods.go` — create: Wails methods (`OpenNotebook`, `SaveNotebook`, `GetNotebook`, `RecordPush`, `GetNotebookLinks`) + view types + `pushedNodeKeys` resolver.
- `notebook_methods_test.go` — create: resolver + link-mark tests.

**Frontend (Svelte/JS)**
- `frontend/src/lib/notebook.js` — create: cell-model helpers + `cellRegistry`.
- `frontend/src/components/notebook/ContextCell.svelte` — create.
- `frontend/src/components/notebook/N4LDraftCell.svelte` — create.
- `frontend/src/components/notebook/NotebookPane.svelte` — create.
- `frontend/src/components/ProcessEditor.svelte` — create (lifted from today's `ProcessView`).
- `frontend/src/components/ProcessView.svelte` — modify: becomes the container.
- `frontend/src/lib/n4l-editor.js` — modify: add `notebookDotsGutter(...)` extension.
- `frontend/src/components/SourceTab.svelte` — modify: mount the gutter + links refresh.
- `docs/superpowers/specs/2026-07-22-processing-notebook-slice1-manual-test-plan.md` — create.

---

## Task 1: Schema v5 — notebooks + notebook_links tables

**Files:**
- Modify: `internal/learning/db.go`
- Test: `internal/learning/db_test.go` (add a test)

**Interfaces:**
- Consumes: existing `migrate()` pattern, `schemaVersion` const.
- Produces: two new tables at schema version 5.

- [ ] **Step 1: Write the failing test**

Add to `internal/learning/db_test.go`:

```go
func TestMigrate_V5_NotebookTables(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	for _, tbl := range []string{"notebooks", "notebook_links"} {
		var name string
		err := s.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %q missing after migrate: %v", tbl, err)
		}
	}
	var v int
	if err := s.db.QueryRow(`SELECT version FROM schema_meta`).Scan(&v); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if v != 5 {
		t.Fatalf("schema version = %d, want 5", v)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/learning/ -run TestMigrate_V5_NotebookTables -v`
Expected: FAIL — `notebooks` missing / version = 4.

- [ ] **Step 3: Implement the migration**

In `internal/learning/db.go`, change the version const:

```go
const schemaVersion = 5
```

Add a case to the `for v < schemaVersion` switch (after the `case 3:` block):

```go
		case 4:
			if _, err := s.db.Exec(ddlV5); err != nil {
				return fmt.Errorf("learning: apply v5: %w", err)
			}
```

Append the DDL after `ddlV3`:

```go
const ddlV5 = `
CREATE TABLE IF NOT EXISTS notebooks (
  notebook_id     TEXT PRIMARY KEY,
  extract_node_id TEXT NOT NULL,
  chapter         TEXT NOT NULL DEFAULT '',
  context_tags    TEXT NOT NULL DEFAULT '',
  cells_json      TEXT NOT NULL DEFAULT '[]',
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notebooks_node ON notebooks(extract_node_id);

CREATE TABLE IF NOT EXISTS notebook_links (
  file_path   TEXT NOT NULL,
  chapter     TEXT NOT NULL,
  node_text   TEXT NOT NULL,
  notebook_id TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  PRIMARY KEY (file_path, chapter, node_text, notebook_id)
);
CREATE INDEX IF NOT EXISTS idx_nblinks_file ON notebook_links(file_path);
`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/learning/ -run TestMigrate_V5_NotebookTables -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/learning/db.go internal/learning/db_test.go
git commit -m "feat(learning): schema v5 — notebooks + notebook_links tables"
```

---

## Task 2: Notebook store CRUD

**Files:**
- Create: `internal/learning/notebooks.go`
- Test: `internal/learning/notebooks_test.go`

**Interfaces:**
- Consumes: `Store.db`, schema v5.
- Produces:
  - `type Notebook struct { NotebookID, ExtractNodeID, Chapter, ContextTags, CellsJSON string; CreatedAt, UpdatedAt time.Time }`
  - `func (s *Store) UpsertNotebook(nb Notebook) error`
  - `func (s *Store) GetNotebook(id string) (Notebook, bool, error)`
  - `func (s *Store) GetNotebookByExtract(extractNodeID string) (Notebook, bool, error)`
  - `func (s *Store) UpdateNotebookCells(id, cellsJSON string, updated time.Time) error`

- [ ] **Step 1: Write the failing test**

Create `internal/learning/notebooks_test.go`:

```go
package learning

import (
	"testing"
	"time"
)

func TestNotebookUpsertAndGet(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	now := time.Unix(1000, 0)
	nb := Notebook{
		NotebookID: "nb-1", ExtractNodeID: "file.n4l:Chap:id:abc",
		Chapter: "Chap", ContextTags: `["a","b"]`, CellsJSON: `[{"id":"c1","type":"n4l","text":"x"}]`,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertNotebook(nb); err != nil {
		t.Fatalf("UpsertNotebook: %v", err)
	}

	got, ok, err := s.GetNotebook("nb-1")
	if err != nil || !ok {
		t.Fatalf("GetNotebook: ok=%v err=%v", ok, err)
	}
	if got.CellsJSON != nb.CellsJSON || got.ExtractNodeID != nb.ExtractNodeID {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	byExtract, ok, err := s.GetNotebookByExtract("file.n4l:Chap:id:abc")
	if err != nil || !ok || byExtract.NotebookID != "nb-1" {
		t.Fatalf("GetNotebookByExtract: ok=%v id=%q err=%v", ok, byExtract.NotebookID, err)
	}

	if err := s.UpdateNotebookCells("nb-1", `[]`, time.Unix(2000, 0)); err != nil {
		t.Fatalf("UpdateNotebookCells: %v", err)
	}
	got, _, _ = s.GetNotebook("nb-1")
	if got.CellsJSON != `[]` || !got.UpdatedAt.Equal(time.Unix(2000, 0)) {
		t.Fatalf("after update: %+v", got)
	}
}

func TestGetNotebookMissing(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	_, ok, err := s.GetNotebook("nope")
	if err != nil || ok {
		t.Fatalf("expected (false,nil), got ok=%v err=%v", ok, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/learning/ -run 'TestNotebook|TestGetNotebookMissing' -v`
Expected: FAIL — `undefined: Notebook` / `UpsertNotebook`.

- [ ] **Step 3: Implement the store**

Create `internal/learning/notebooks.go`:

```go
package learning

import (
	"database/sql"
	"time"
)

// Notebook is one processing-notebook lab record. CellsJSON holds only the
// user-authored cells (the context cell is derived and never persisted).
type Notebook struct {
	NotebookID    string
	ExtractNodeID string
	Chapter       string
	ContextTags   string // JSON array of tag strings
	CellsJSON     string // JSON array of cell objects
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UpsertNotebook inserts a new notebook or replaces an existing one by ID.
func (s *Store) UpsertNotebook(nb Notebook) error {
	_, err := s.db.Exec(
		`INSERT INTO notebooks
		   (notebook_id, extract_node_id, chapter, context_tags, cells_json, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?)
		 ON CONFLICT(notebook_id) DO UPDATE SET
		   extract_node_id=excluded.extract_node_id,
		   chapter=excluded.chapter,
		   context_tags=excluded.context_tags,
		   cells_json=excluded.cells_json,
		   updated_at=excluded.updated_at`,
		nb.NotebookID, nb.ExtractNodeID, nb.Chapter, nb.ContextTags, nb.CellsJSON,
		nb.CreatedAt.Unix(), nb.UpdatedAt.Unix())
	return err
}

// UpdateNotebookCells writes just the cells payload + updated timestamp.
func (s *Store) UpdateNotebookCells(id, cellsJSON string, updated time.Time) error {
	_, err := s.db.Exec(
		`UPDATE notebooks SET cells_json=?, updated_at=? WHERE notebook_id=?`,
		cellsJSON, updated.Unix(), id)
	return err
}

func (s *Store) GetNotebook(id string) (Notebook, bool, error) {
	return scanNotebook(s.db.QueryRow(
		`SELECT notebook_id, extract_node_id, chapter, context_tags, cells_json, created_at, updated_at
		 FROM notebooks WHERE notebook_id=?`, id))
}

func (s *Store) GetNotebookByExtract(extractNodeID string) (Notebook, bool, error) {
	return scanNotebook(s.db.QueryRow(
		`SELECT notebook_id, extract_node_id, chapter, context_tags, cells_json, created_at, updated_at
		 FROM notebooks WHERE extract_node_id=? ORDER BY created_at ASC LIMIT 1`, extractNodeID))
}

func scanNotebook(row *sql.Row) (Notebook, bool, error) {
	var (
		nb               Notebook
		created, updated int64
	)
	err := row.Scan(&nb.NotebookID, &nb.ExtractNodeID, &nb.Chapter,
		&nb.ContextTags, &nb.CellsJSON, &created, &updated)
	if err == sql.ErrNoRows {
		return Notebook{}, false, nil
	}
	if err != nil {
		return Notebook{}, false, err
	}
	nb.CreatedAt = time.Unix(created, 0)
	nb.UpdatedAt = time.Unix(updated, 0)
	return nb, true, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/learning/ -run 'TestNotebook|TestGetNotebookMissing' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/learning/notebooks.go internal/learning/notebooks_test.go
git commit -m "feat(learning): notebook store CRUD"
```

---

## Task 3: Notebook link store methods

**Files:**
- Modify: `internal/learning/notebooks.go`
- Test: `internal/learning/notebooks_test.go`

**Interfaces:**
- Consumes: schema v5 `notebook_links`.
- Produces:
  - `type NotebookLink struct { FilePath, Chapter, NodeText, NotebookID string }`
  - `func (s *Store) AddNotebookLinks(links []NotebookLink, created time.Time) error`
  - `func (s *Store) LinksForFile(filePath string) ([]NotebookLink, error)`

- [ ] **Step 1: Write the failing test**

Add to `internal/learning/notebooks_test.go`:

```go
func TestNotebookLinks(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	links := []NotebookLink{
		{FilePath: "a.n4l", Chapter: "C", NodeText: "Newton", NotebookID: "nb-1"},
		{FilePath: "a.n4l", Chapter: "C", NodeText: "Newton", NotebookID: "nb-1"}, // dup, idempotent
		{FilePath: "a.n4l", Chapter: "C", NodeText: "calculus", NotebookID: "nb-1"},
		{FilePath: "b.n4l", Chapter: "C", NodeText: "gravity", NotebookID: "nb-2"},
	}
	if err := s.AddNotebookLinks(links, time.Unix(1, 0)); err != nil {
		t.Fatalf("AddNotebookLinks: %v", err)
	}

	got, err := s.LinksForFile("a.n4l")
	if err != nil {
		t.Fatalf("LinksForFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 links for a.n4l, got %d: %+v", len(got), got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/learning/ -run TestNotebookLinks -v`
Expected: FAIL — `undefined: NotebookLink`.

- [ ] **Step 3: Implement**

Append to `internal/learning/notebooks.go`:

```go
// NotebookLink ties a produced node (identified by the stable
// file+chapter+text tuple, since most nodes have no # @id) to its notebook.
type NotebookLink struct {
	FilePath   string
	Chapter    string
	NodeText   string
	NotebookID string
}

// AddNotebookLinks inserts links, ignoring exact duplicates.
func (s *Store) AddNotebookLinks(links []NotebookLink, created time.Time) error {
	for _, l := range links {
		if _, err := s.db.Exec(
			`INSERT OR IGNORE INTO notebook_links
			   (file_path, chapter, node_text, notebook_id, created_at)
			 VALUES (?,?,?,?,?)`,
			l.FilePath, l.Chapter, l.NodeText, l.NotebookID, created.Unix()); err != nil {
			return err
		}
	}
	return nil
}

// LinksForFile returns every link whose file_path matches.
func (s *Store) LinksForFile(filePath string) ([]NotebookLink, error) {
	rows, err := s.db.Query(
		`SELECT file_path, chapter, node_text, notebook_id
		 FROM notebook_links WHERE file_path=?`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NotebookLink
	for rows.Next() {
		var l NotebookLink
		if err := rows.Scan(&l.FilePath, &l.Chapter, &l.NodeText, &l.NotebookID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/learning/ -run TestNotebookLinks -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/learning/notebooks.go internal/learning/notebooks_test.go
git commit -m "feat(learning): notebook_links store methods"
```

---

## Task 4: Source excerpt helper

**Files:**
- Modify: `internal/source/projection.go`
- Test: `internal/source/projection_test.go`

**Interfaces:**
- Produces: `func Excerpt(full, needle string) string` — returns a whitespace-collapsed window (~600 chars) of `full` centered on the first occurrence of `needle` (collapsed). Empty string when `needle` is empty or not found.

- [ ] **Step 1: Write the failing test**

Add to `internal/source/projection_test.go`:

```go
func TestExcerpt(t *testing.T) {
	full := "Intro paragraph. " + strings.Repeat("x ", 400) +
		"The quick brown fox jumps. " + strings.Repeat("y ", 400) + "End."
	got := Excerpt(full, "quick   brown\nfox")
	if !strings.Contains(got, "quick brown fox") {
		t.Fatalf("excerpt should contain the needle window, got: %q", got)
	}
	if len(got) > 800 {
		t.Fatalf("excerpt too long: %d", len(got))
	}
	if Excerpt(full, "") != "" {
		t.Fatalf("empty needle must yield empty excerpt")
	}
	if Excerpt(full, "not-present-anywhere") != "" {
		t.Fatalf("missing needle must yield empty excerpt")
	}
}
```

Ensure `projection_test.go` imports `strings` (add to the import block if absent).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/source/ -run TestExcerpt -v`
Expected: FAIL — `undefined: Excerpt`.

- [ ] **Step 3: Implement**

Append to `internal/source/projection.go`:

```go
// Excerpt returns a whitespace-collapsed window of full (about 600 chars)
// centered on the first occurrence of needle (also collapsed). It returns ""
// when needle is empty or not found, so callers can fall back to the anchor.
func Excerpt(full, needle string) string {
	const window = 600
	collapse := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	cfull := collapse(full)
	cneedle := collapse(needle)
	if cneedle == "" {
		return ""
	}
	idx := strings.Index(cfull, cneedle)
	if idx < 0 {
		return ""
	}
	start := idx - window/2
	if start < 0 {
		start = 0
	}
	end := idx + len(cneedle) + window/2
	if end > len(cfull) {
		end = len(cfull)
	}
	return cfull[start:end]
}
```

Add `"strings"` to `projection.go`'s import block if it is not already imported.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/source/ -run TestExcerpt -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/source/projection.go internal/source/projection_test.go
git commit -m "feat(source): Excerpt helper for context-cell source passage"
```

---

## Task 5: Wails methods — OpenNotebook, SaveNotebook, GetNotebook

**Files:**
- Create: `notebook_methods.go`
- Test: `notebook_methods_test.go`

**Interfaces:**
- Consumes: `a.learnStore` (Tasks 2–3), `a.searchSvc.Search`, `a.registry`, `a.collectionDir`, `a.GetExtractTarget` (`study_methods.go`), `learning.Store.ListReading`, `source.Projection`, `source.Excerpt`.
- Produces:
  - `type NotebookView`, `ContextPayload`, `OrbitItem`, `SourceInfo` (JSON-tagged, below).
  - `func (a *App) OpenNotebook(nodeID string) (*NotebookView, error)`
  - `func (a *App) SaveNotebook(notebookID, cellsJSON string) error`
  - `func (a *App) GetNotebook(notebookID string) (*NotebookView, error)`
  - `func newNotebookID() (string, error)`
  - `func (a *App) assembleContext(nodeID string) ContextPayload`

- [ ] **Step 1: Write the failing test**

Create `notebook_methods_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestNewNotebookID(t *testing.T) {
	a, err := newNotebookID()
	if err != nil {
		t.Fatalf("newNotebookID: %v", err)
	}
	b, _ := newNotebookID()
	if a == b {
		t.Fatalf("ids should differ: %q == %q", a, b)
	}
	if !strings.HasPrefix(a, "nb-") || len(a) != len("nb-")+16 {
		t.Fatalf("unexpected id shape: %q", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestNewNotebookID -v`
Expected: FAIL — `undefined: newNotebookID`.

- [ ] **Step 3: Implement**

Create `notebook_methods.go`:

```go
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/content"
	"texere/internal/learning"
	"texere/internal/source"
)

// NotebookView is the frontend payload for a processing notebook: persisted
// user cells plus a freshly-computed (never persisted) context payload.
type NotebookView struct {
	NotebookID string          `json:"notebookId"`
	NodeID     string          `json:"nodeId"`
	Chapter    string          `json:"chapter"`
	Cells      json.RawMessage `json:"cells"`
	Context    ContextPayload  `json:"context"`
}

// ContextPayload is the auto-generated pinned context cell (read-only).
type ContextPayload struct {
	ExtractText string      `json:"extractText"`
	Tags        []string    `json:"tags"`
	Orbit       []OrbitItem `json:"orbit"`
	Source      SourceInfo  `json:"source"`
}

type OrbitItem struct {
	ArrowLong    string `json:"arrowLong"`
	NeighborText string `json:"neighborText"`
}

type SourceInfo struct {
	Kind    string `json:"kind"`    // "text" | "web" | "pdf" | "none"
	Anchor  string `json:"anchor"`  // the (from) anchor
	Passage string `json:"passage"` // empty for pdf/none
	RefPath string `json:"refPath"` // collection-relative source path
}

func newNotebookID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "nb-" + hex.EncodeToString(b), nil
}

// OpenNotebook loads (or creates) the notebook for an extract node and returns
// it with a freshly-assembled context payload.
func (a *App) OpenNotebook(nodeID string) (*NotebookView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	nb, ok, err := a.learnStore.GetNotebookByExtract(nodeID)
	if err != nil {
		return nil, err
	}
	if !ok {
		id, err := newNotebookID()
		if err != nil {
			return nil, err
		}
		now := time.Now()
		nb = learning.Notebook{
			NotebookID: id, ExtractNodeID: nodeID,
			Chapter: a.chapterOfNode(nodeID), ContextTags: "[]", CellsJSON: "[]",
			CreatedAt: now, UpdatedAt: now,
		}
		if err := a.learnStore.UpsertNotebook(nb); err != nil {
			return nil, err
		}
	}
	return &NotebookView{
		NotebookID: nb.NotebookID, NodeID: nodeID, Chapter: nb.Chapter,
		Cells: json.RawMessage(nb.CellsJSON), Context: a.assembleContext(nodeID),
	}, nil
}

// SaveNotebook persists just the user cells (debounced autosave from the pane).
func (a *App) SaveNotebook(notebookID, cellsJSON string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	return a.learnStore.UpdateNotebookCells(notebookID, cellsJSON, time.Now())
}

// GetNotebook returns a notebook by ID for read-only reopen; its context is
// recomputed from the notebook's extract node.
func (a *App) GetNotebook(notebookID string) (*NotebookView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	nb, ok, err := a.learnStore.GetNotebook(notebookID)
	if err != nil || !ok {
		return nil, errors.New("notebook not found")
	}
	return &NotebookView{
		NotebookID: nb.NotebookID, NodeID: nb.ExtractNodeID, Chapter: nb.Chapter,
		Cells: json.RawMessage(nb.CellsJSON), Context: a.assembleContext(nb.ExtractNodeID),
	}, nil
}

// chapterOfNode returns the chapter title containing the node with id, "" if
// not resolvable.
func (a *App) chapterOfNode(id string) string {
	tgt, err := a.GetExtractTarget(id)
	if err != nil {
		return ""
	}
	pf, _ := content.ParseFile(tgt.FilePath, a.registry)
	if pf == nil {
		return ""
	}
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.ID == id {
				return ch.Title
			}
		}
	}
	return ""
}

// assembleContext builds the read-only context payload: extract text, tags,
// radius-1 orbit, and a text/web source passage. Every input degrades to a
// safe empty value; PDF sources omit the passage.
func (a *App) assembleContext(nodeID string) ContextPayload {
	out := ContextPayload{Tags: []string{}, Orbit: []OrbitItem{}, Source: SourceInfo{Kind: "none"}}
	tgt, err := a.GetExtractTarget(nodeID)
	if err != nil {
		return out
	}
	out.ExtractText = tgt.Text
	out.Source.Anchor = tgt.FromAnchor

	// Orbit + tags from the graph (empty & silent if the graph is unbuilt).
	if res, err := a.SearchGraph(tgt.Text); err == nil && res != nil {
		for _, gn := range res.Nodes {
			if gn.NodeText != tgt.Text {
				continue
			}
			out.Tags = append(out.Tags, gn.ContextTags...)
			for _, oe := range gn.Orbit {
				if oe.Radius == 1 {
					out.Orbit = append(out.Orbit, OrbitItem{ArrowLong: oe.ArrowLong, NeighborText: oe.NeighborText})
				}
			}
			break
		}
	}

	// Source passage: find the reading element whose notes file is this file.
	if tgt.FromAnchor != "" {
		if elems, err := a.learnStore.ListReading(); err == nil {
			for _, e := range elems {
				if e.NotesPath != tgt.FilePath {
					continue
				}
				out.Source.RefPath = e.RefPath
				if e.Medium == "pdf" {
					out.Source.Kind = "pdf"
					break
				}
				out.Source.Kind = e.Medium
				if proj, perr := source.Projection(filepath.Join(a.collectionDir, filepath.FromSlash(e.RefPath))); perr == nil {
					out.Source.Passage = source.Excerpt(proj, tgt.Text)
				}
				break
			}
		}
	}
	return out
}

// notebookLinksChanged notifies the frontend that a file's provenance dots
// need refreshing.
func (a *App) notebookLinksChanged(filePath string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "notebook:links-changed", map[string]string{"path": filePath})
	}
}
```

> Note: `Medium` values are `text` / `web` / `pdf` (`internal/learning/reading.go`); `e.Medium` may be `""` for legacy rows — `Source.Kind` then becomes `""`, which the frontend treats as text.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestNewNotebookID -v`
Expected: PASS. Also run `go build ./...` — Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add notebook_methods.go notebook_methods_test.go
git commit -m "feat(app): OpenNotebook/SaveNotebook/GetNotebook + context assembly"
```

---

## Task 6: Wails methods — RecordPush + GetNotebookLinks

**Files:**
- Modify: `notebook_methods.go`
- Test: `notebook_methods_test.go`

**Interfaces:**
- Consumes: `content.ParseFile`, `a.learnStore.AddNotebookLinks`, `a.learnStore.LinksForFile`, `a.notebookLinksChanged`.
- Produces:
  - `func pushedNodeKeys(pf *content.ParsedFile, insertedText string) [][2]string` — pure; returns `{chapterTitle, nodeText}` pairs for nodes the pushed text authored (subject match, line-independent).
  - `func (a *App) RecordPush(notebookID, filePath, insertedText string) ([]string, error)` — parses the file, records links, emits the refresh event, returns matched node texts.
  - `type NotebookLinkMark struct { Line int; NotebookIDs []string }`
  - `func (a *App) GetNotebookLinks(filePath string) ([]NotebookLinkMark, error)`

- [ ] **Step 1: Write the failing test**

Add to `notebook_methods_test.go`:

```go
import (
	// keep existing imports; add:
	"texere/internal/content"
)

func fakeParsed(chapter string, nodes []struct{ Text string; Line int }) *content.ParsedFile {
	ch := content.Chapter{Title: chapter}
	for _, n := range nodes {
		ch.Nodes = append(ch.Nodes, content.Node{Text: n.Text, Line: n.Line})
	}
	return &content.ParsedFile{Chapters: []content.Chapter{ch}}
}

func TestPushedNodeKeys(t *testing.T) {
	pf := fakeParsed("C", []struct{ Text string; Line int }{
		{"Newton", 10}, {"calculus", 11}, {"gravity", 12},
	})

	// Bare node push.
	keys := pushedNodeKeys(pf, "Newton\n")
	if len(keys) != 1 || keys[0] != [2]string{"C", "Newton"} {
		t.Fatalf("bare node: got %v", keys)
	}

	// Subject + arrow + target: links the subject, NOT the target.
	keys = pushedNodeKeys(pf, "Newton (invented) calculus\n")
	if len(keys) != 1 || keys[0] != [2]string{"C", "Newton"} {
		t.Fatalf("subject/target: got %v", keys)
	}

	// Matching is by text, independent of the node's .Line (id-injection safe).
	shifted := fakeParsed("C", []struct{ Text string; Line int }{{"Newton", 999}})
	keys = pushedNodeKeys(shifted, "Newton (is a) physicist\n")
	if len(keys) != 1 {
		t.Fatalf("line-independent match failed: got %v", keys)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestPushedNodeKeys -v`
Expected: FAIL — `undefined: pushedNodeKeys`.

- [ ] **Step 3: Implement**

Append to `notebook_methods.go` (add `"sort"` and `"strings"` to the import block):

```go
// pushedNodeKeys returns the (chapterTitle, nodeText) of every parsed node that
// the pushed text authored as a subject. A node matches when a collapsed
// inserted line equals its text or begins with "<text> " (text then arrow).
// Arrow targets are not matched. Matching is by text, so it survives the line
// shifts SaveFile causes when it injects "# @id" markers.
func pushedNodeKeys(pf *content.ParsedFile, insertedText string) [][2]string {
	if pf == nil {
		return nil
	}
	var lines []string
	for _, raw := range strings.Split(insertedText, "\n") {
		if c := strings.Join(strings.Fields(raw), " "); c != "" {
			lines = append(lines, c)
		}
	}
	var out [][2]string
	seen := map[[2]string]bool{}
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.Text == "" {
				continue
			}
			for _, c := range lines {
				if c == n.Text || strings.HasPrefix(c, n.Text+" ") {
					key := [2]string{ch.Title, n.Text}
					if !seen[key] {
						seen[key] = true
						out = append(out, key)
					}
					break
				}
			}
		}
	}
	return out
}

// RecordPush records provenance links for the nodes a pushed draft authored,
// then signals the gutter to refresh. filePath must be the file the text was
// pushed into (already saved to disk by the caller).
func (a *App) RecordPush(notebookID, filePath, insertedText string) ([]string, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	pf, _ := content.ParseFile(filePath, a.registry)
	keys := pushedNodeKeys(pf, insertedText)
	links := make([]learning.NotebookLink, 0, len(keys))
	texts := make([]string, 0, len(keys))
	for _, k := range keys {
		links = append(links, learning.NotebookLink{
			FilePath: filePath, Chapter: k[0], NodeText: k[1], NotebookID: notebookID})
		texts = append(texts, k[1])
	}
	if err := a.learnStore.AddNotebookLinks(links, time.Now()); err != nil {
		return nil, err
	}
	a.notebookLinksChanged(filePath)
	return texts, nil
}

// NotebookLinkMark is one gutter dot: a line and the notebooks linked to the
// node on that line.
type NotebookLinkMark struct {
	Line        int      `json:"line"`
	NotebookIDs []string `json:"notebookIds"`
}

// GetNotebookLinks resolves this file's links to line marks by re-parsing and
// matching each linked (chapter, text) tuple to a current node line.
func (a *App) GetNotebookLinks(filePath string) ([]NotebookLinkMark, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	links, err := a.learnStore.LinksForFile(filePath)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return []NotebookLinkMark{}, nil
	}
	// (chapter, text) -> set of notebook IDs
	byKey := map[[2]string]map[string]bool{}
	for _, l := range links {
		k := [2]string{l.Chapter, l.NodeText}
		if byKey[k] == nil {
			byKey[k] = map[string]bool{}
		}
		byKey[k][l.NotebookID] = true
	}
	pf, _ := content.ParseFile(filePath, a.registry)
	byLine := map[int]map[string]bool{}
	if pf != nil {
		for _, ch := range pf.Chapters {
			for _, n := range ch.Nodes {
				ids, ok := byKey[[2]string{ch.Title, n.Text}]
				if !ok {
					continue
				}
				if byLine[n.Line] == nil {
					byLine[n.Line] = map[string]bool{}
				}
				for id := range ids {
					byLine[n.Line][id] = true
				}
			}
		}
	}
	out := make([]NotebookLinkMark, 0, len(byLine))
	for line, ids := range byLine {
		list := make([]string, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		sort.Strings(list)
		out = append(out, NotebookLinkMark{Line: line, NotebookIDs: list})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run 'TestPushedNodeKeys|TestNewNotebookID' -v`
Expected: PASS. Then `go test ./...` — Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add notebook_methods.go notebook_methods_test.go
git commit -m "feat(app): RecordPush + GetNotebookLinks provenance resolution"
```

---

## Task 7: Regenerate Wails bindings

**Files:**
- Modify (generated): `frontend/wailsjs/go/main/App.js`, `frontend/wailsjs/go/main/App.d.ts`, `frontend/wailsjs/go/models.ts`

**Interfaces:**
- Produces: JS bindings for `OpenNotebook`, `SaveNotebook`, `GetNotebook`, `RecordPush`, `GetNotebookLinks` importable from `../../wailsjs/go/main/App`.

- [ ] **Step 1: Regenerate bindings**

Run: `wails generate module`
(If the `wails` CLI is not on PATH, run `make build` — the Wails build regenerates bindings as a side effect.)

- [ ] **Step 2: Verify the new methods are present**

Run: `grep -E "OpenNotebook|RecordPush|GetNotebookLinks" frontend/wailsjs/go/main/App.js`
Expected: one match per method name.

- [ ] **Step 3: Commit**

```bash
git add frontend/wailsjs/go/main/App.js frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/models.ts
git commit -m "chore(bindings): regenerate wailsjs for notebook methods"
```

---

## Task 8: Cell model + ContextCell + N4LDraftCell components

**Files:**
- Create: `frontend/src/lib/notebook.js`
- Create: `frontend/src/components/notebook/ContextCell.svelte`
- Create: `frontend/src/components/notebook/N4LDraftCell.svelte`

**Interfaces:**
- Consumes: `n4lBaseExtensions` from `../lib/n4l-editor.js`, `workbenchStore` from `../stores/workbench.js`.
- Produces:
  - `newDraftCell(text = '')` → `{ id, type: 'n4l', text }`
  - `cellRegistry` → `{ context: ContextCell, n4l: N4LDraftCell }`
  - `ContextCell` props: `context` (the `ContextPayload`).
  - `N4LDraftCell` props: `cell`, `onChange(text)`, `onPush(text)`, `onDelete()`, `readOnly`.

- [ ] **Step 1: Create the cell-model helpers**

Create `frontend/src/lib/notebook.js`:

```js
import ContextCell from '../components/notebook/ContextCell.svelte'
import N4LDraftCell from '../components/notebook/N4LDraftCell.svelte'

let seq = 0
export function newDraftCell(text = '') {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'n4l', text }
}

// type -> Svelte component. Later slices add: llm, graph, compute, sketch, image.
export const cellRegistry = {
  context: ContextCell,
  n4l: N4LDraftCell,
}
```

- [ ] **Step 2: Create ContextCell**

Create `frontend/src/components/notebook/ContextCell.svelte`:

```svelte
<script>
  export let context = { extractText: '', tags: [], orbit: [], source: { kind: 'none' } }
  $: src = context.source ?? { kind: 'none' }
</script>

<div class="ctx-cell">
  <div class="ctx-badge">CONTEXT</div>

  <section>
    <h4>Extract</h4>
    <p class="extract">{context.extractText || '—'}</p>
  </section>

  {#if context.tags?.length}
    <section>
      <h4>Tags</h4>
      <div class="tags">{#each context.tags as t}<span class="tag">{t}</span>{/each}</div>
    </section>
  {/if}

  <section>
    <h4>Neighbors</h4>
    {#if context.orbit?.length}
      <ul class="orbit">
        {#each context.orbit as o}<li><span class="arrow">{o.arrowLong}</span> {o.neighborText}</li>{/each}
      </ul>
    {:else}
      <p class="hint">No graph neighbors — build the graph (Ctrl+G) to see them.</p>
    {/if}
  </section>

  <section>
    <h4>Source</h4>
    {#if src.kind === 'pdf'}
      <p class="hint">PDF source — <a href={'#'} on:click|preventDefault>{src.anchor || 'open source'}</a> (page text extraction is a later slice).</p>
    {:else if src.passage}
      <p class="passage">…{src.passage}…</p>
    {:else if src.anchor}
      <p class="hint">From: {src.anchor}</p>
    {:else}
      <p class="hint">No linked source.</p>
    {/if}
  </section>
</div>

<style>
  .ctx-cell { position: relative; background: #101826; border: 1px solid #26374a; border-radius: 8px; padding: 12px 14px 12px; margin-bottom: 10px; }
  .ctx-badge { position: absolute; top: 8px; right: 10px; font-size: 9px; letter-spacing: .1em; color: #5a7a9a; }
  section { margin-bottom: 10px; }
  h4 { margin: 0 0 4px; font-size: 10px; letter-spacing: .08em; text-transform: uppercase; color: #7dd3fc; }
  .extract { margin: 0; color: #e8eaf0; font-size: 13px; }
  .passage { margin: 0; color: #b9c6d6; font-size: 12px; font-style: italic; }
  .hint { margin: 0; color: #5a7a9a; font-size: 12px; }
  .tags { display: flex; flex-wrap: wrap; gap: 4px; }
  .tag { background: #1e3a5a; color: #93c5fd; border-radius: 4px; padding: 1px 6px; font-size: 11px; }
  .orbit { margin: 0; padding-left: 14px; color: #cdd6e2; font-size: 12px; }
  .arrow { color: #7dd3fc; }
</style>
```

- [ ] **Step 3: Create N4LDraftCell**

Create `frontend/src/components/notebook/N4LDraftCell.svelte`:

```svelte
<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { workbenchStore } from '../../stores/workbench.js'
  import { n4lBaseExtensions } from '../../lib/n4l-editor.js'

  export let cell
  export let onChange = () => {}
  export let onPush = () => {}
  export let onDelete = () => {}
  export let readOnly = false

  let container, view
  let arrowDefsRef = []
  $: arrowDefsRef = $workbenchStore.arrowDefs ?? []

  onMount(() => {
    view = new EditorView({
      parent: container,
      state: EditorState.create({
        doc: cell.text ?? '',
        extensions: [
          ...n4lBaseExtensions({ getArrowDefs: () => arrowDefsRef, getRecent: () => [], onArrowSelected: () => {}, fontSize: '0.8rem' }),
          EditorView.editable.of(!readOnly),
          EditorView.updateListener.of((u) => { if (u.docChanged) onChange(u.state.doc.toString()) }),
        ],
      }),
    })
  })
  onDestroy(() => view?.destroy())

  function push() { if (view) onPush(view.state.doc.toString()) }
</script>

<div class="draft-cell">
  <div class="draft-editor" bind:this={container}></div>
  {#if !readOnly}
    <div class="draft-bar">
      <button class="push-btn" on:click={push} title="Insert this N4L at the editor cursor">→ Push to source</button>
      <button class="del-btn" on:click={onDelete} title="Delete cell">✕</button>
    </div>
  {/if}
</div>

<style>
  .draft-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .draft-editor { min-height: 48px; }
  .draft-bar { display: flex; gap: 6px; padding: 6px 8px; border-top: 1px solid #1c2a3a; }
  .push-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .push-btn:hover { background: #244a72; }
  .del-btn { margin-left: auto; background: none; color: #6b7a8a; border: none; cursor: pointer; font-size: 12px; }
  .del-btn:hover { color: #ff8080; }
</style>
```

- [ ] **Step 4: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds (no unresolved imports / syntax errors).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/notebook.js frontend/src/components/notebook/
git commit -m "feat(notebook): cell model + ContextCell + N4LDraftCell"
```

---

## Task 8A: Draft-cell arrow validation (amber underline + replace quick-fix)

**Files:**
- Modify: `frontend/src/lib/n4l-editor.js`
- Modify: `frontend/src/components/notebook/N4LDraftCell.svelte`

**Interfaces:**
- Consumes: `arrowDefsRef` (from `workbenchStore.arrowDefs`), each def `{ Code, LongName }`.
- Produces: `arrowLint(getArrowDefs)` — a CodeMirror extension that amber-wavy-underlines any `(code)` whose trimmed code is not a registered arrow and that has a non-empty target, and shows a hover tooltip offering the nearest registered arrows as one-click replacements. Scope note: "register a new arrow" is **out of scope** (no SSTconfig writer exists yet) — later slice.

- [ ] **Step 1: Add the `arrowLint` extension**

In `frontend/src/lib/n4l-editor.js`, extend the `@codemirror/view` import to also bring in `Decoration`, `ViewPlugin`, `hoverTooltip` (in addition to whatever it already imports), then append:

```js
// Mark every (code) arrow whose code is unregistered and that has a target.
function unregisteredArrowDeco(view, getArrowDefs) {
  const codes = new Set((getArrowDefs() ?? []).map((d) => d.Code))
  const marks = []
  const re = /\(([^()\n]+)\)/g
  for (const { from, to } of view.visibleRanges) {
    const text = view.state.doc.sliceString(from, to)
    let m
    while ((m = re.exec(text))) {
      const code = m[1].trim()
      if (!code || codes.has(code)) continue
      const after = text.slice(m.index + m[0].length)
      const target = after.split(/\(|\n/)[0].trim() // arrow needs a non-empty target
      if (!target) continue
      const start = from + m.index
      marks.push(Decoration.mark({ class: 'cm-arrow-unreg' }).range(start, start + m[0].length))
    }
  }
  return Decoration.set(marks, true)
}

// Rank registered arrows by shared-prefix length, then closeness in length.
function nearestArrows(code, defs) {
  const score = (c) => {
    let p = 0
    const n = Math.min(c.length, code.length)
    while (p < n && c[p] === code[p]) p++
    return p * 100 - Math.abs(c.length - code.length)
  }
  return [...(defs ?? [])].sort((a, b) => score(b.Code) - score(a.Code))
}

// arrowLint underlines unregistered arrows and offers replacement suggestions.
export function arrowLint(getArrowDefs) {
  const decoPlugin = ViewPlugin.fromClass(
    class {
      constructor(view) { this.decorations = unregisteredArrowDeco(view, getArrowDefs) }
      update(u) { if (u.docChanged || u.viewportChanged) this.decorations = unregisteredArrowDeco(u.view, getArrowDefs) }
    },
    { decorations: (v) => v.decorations },
  )

  const tip = hoverTooltip((view, pos) => {
    const line = view.state.doc.lineAt(pos)
    const rel = pos - line.from
    const re = /\(([^()\n]+)\)/g
    let m
    while ((m = re.exec(line.text))) {
      const s = m.index
      const e = m.index + m[0].length
      if (rel < s || rel > e) continue
      const code = m[1].trim()
      const codes = (getArrowDefs() ?? []).map((d) => d.Code)
      if (!code || codes.includes(code)) return null
      const from = line.from + s
      const to = line.from + e
      const suggestions = nearestArrows(code, getArrowDefs()).slice(0, 5)
      return {
        pos: from,
        end: to,
        above: true,
        create() {
          const dom = document.createElement('div')
          dom.className = 'cm-arrow-fix'
          const h = document.createElement('div')
          h.className = 'cm-arrow-fix-h'
          h.textContent = `Unregistered arrow "${code}"`
          dom.appendChild(h)
          for (const sug of suggestions) {
            const b = document.createElement('button')
            b.textContent = `(${sug.Code}) ${sug.LongName || ''}`.trim()
            b.onclick = () => view.dispatch({ changes: { from, to, insert: `(${sug.Code})` } })
            dom.appendChild(b)
          }
          return { dom }
        },
      }
    }
    return null
  })

  const theme = EditorView.theme({
    '.cm-arrow-unreg': { textDecoration: 'underline wavy #f59e0b' },
    '.cm-arrow-fix': { background: '#1a2537', border: '1px solid #2a3a4a', borderRadius: '6px', padding: '6px', maxWidth: '260px' },
    '.cm-arrow-fix-h': { color: '#f59e0b', fontSize: '11px', marginBottom: '4px' },
    '.cm-arrow-fix button': { display: 'block', width: '100%', textAlign: 'left', background: 'none', border: 'none', color: '#cdd6e2', cursor: 'pointer', fontSize: '12px', padding: '2px 4px' },
    '.cm-arrow-fix button:hover': { background: '#24374d' },
  })

  return [decoPlugin, tip, theme]
}
```

- [ ] **Step 2: Wire it into N4LDraftCell**

In `frontend/src/components/notebook/N4LDraftCell.svelte`, extend the editor-lib import to include `arrowLint`, and add it to the extensions array (after the `n4lBaseExtensions(...)` spread, before `EditorView.editable.of(...)`):

```js
  import { n4lBaseExtensions, arrowLint } from '../../lib/n4l-editor.js'
```

```js
          ...n4lBaseExtensions({ getArrowDefs: () => arrowDefsRef, getRecent: () => [], onArrowSelected: () => {}, fontSize: '0.8rem' }),
          arrowLint(() => arrowDefsRef),
          EditorView.editable.of(!readOnly),
```

- [ ] **Step 3: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 4: Manual test**

Run: `make build && ./build/bin/texere`
Verify: in a draft cell, typing `X (notacode) Y` amber-underlines `(notacode)`; hovering it shows a suggestion list; clicking a suggestion replaces the code; a registered arrow (e.g. `(then)`) is not underlined.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/n4l-editor.js frontend/src/components/notebook/N4LDraftCell.svelte
git commit -m "feat(notebook): draft-cell arrow underline + replace quick-fix"
```

---

## Task 9: NotebookPane component

**Files:**
- Create: `frontend/src/components/notebook/NotebookPane.svelte`

**Interfaces:**
- Consumes: `OpenNotebook`, `SaveNotebook` bindings; `newDraftCell`, `ContextCell` from Task 8.
- Produces: `NotebookPane` props `nodeID`, `readOnly` (default false), `onPush(text)` callback, and `notebookId` exposed via `on:ready` event `{ detail: { notebookId } }`. Autosaves user cells (debounced ~600ms).

- [ ] **Step 1: Create the pane**

Create `frontend/src/components/notebook/NotebookPane.svelte`:

```svelte
<script>
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { OpenNotebook, GetNotebook, SaveNotebook } from '../../wailsjs/go/main/App'
  import { newDraftCell } from '../../lib/notebook.js'
  import ContextCell from './ContextCell.svelte'
  import N4LDraftCell from './N4LDraftCell.svelte'

  export let nodeID
  export let notebookId = null      // when set, reopen this notebook read-only
  export let readOnly = false
  export let onPush = () => {}

  const dispatch = createEventDispatcher()
  let context = { extractText: '', tags: [], orbit: [], source: { kind: 'none' } }
  let cells = []
  let nbId = null
  let error = ''
  let saveTimer = null

  async function load() {
    error = ''
    try {
      const view = notebookId ? await GetNotebook(notebookId) : await OpenNotebook(nodeID)
      nbId = view.notebookId
      context = view.context ?? context
      cells = Array.isArray(view.cells) ? view.cells : (view.cells ? JSON.parse(view.cells) : [])
      dispatch('ready', { notebookId: nbId })
    } catch (e) { error = String(e?.message ?? e) }
  }
  $: if (nodeID || notebookId) load()

  function scheduleSave() {
    if (readOnly) return
    clearTimeout(saveTimer)
    saveTimer = setTimeout(() => { SaveNotebook(nbId, JSON.stringify(cells)).catch(() => {}) }, 600)
  }
  onDestroy(() => clearTimeout(saveTimer))

  function addDraft() { cells = [...cells, newDraftCell('')]; scheduleSave() }
  function updateCell(i, text) { cells[i].text = text; scheduleSave() }
  function deleteCell(i) { cells = cells.filter((_, j) => j !== i); scheduleSave() }
  function move(i, dir) {
    const j = i + dir
    if (j < 0 || j >= cells.length) return
    const next = cells.slice(); [next[i], next[j]] = [next[j], next[i]]; cells = next; scheduleSave()
  }
</script>

<div class="nb-pane">
  <div class="nb-head">
    <span class="nb-title">Notebook</span>
    {#if readOnly}<span class="nb-ro">read-only</span>{/if}
  </div>
  {#if error}<div class="nb-error">{error}</div>{/if}
  <div class="nb-scroll">
    <ContextCell {context} />
    {#each cells as cell, i (cell.id)}
      <div class="nb-cellwrap">
        {#if !readOnly}
          <div class="nb-reorder">
            <button on:click={() => move(i, -1)} title="Move up">▲</button>
            <button on:click={() => move(i, 1)} title="Move down">▼</button>
          </div>
        {/if}
        <div class="nb-cell">
          <N4LDraftCell {cell} {readOnly}
            onChange={(t) => updateCell(i, t)}
            onPush={(t) => onPush(t)}
            onDelete={() => deleteCell(i)} />
        </div>
      </div>
    {/each}
    {#if !readOnly}
      <button class="nb-add" on:click={addDraft}>+ N4L draft cell</button>
    {/if}
  </div>
</div>

<style>
  .nb-pane { position: absolute; inset: 0; display: flex; flex-direction: column; background: #0b1220; }
  .nb-head { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #101826; }
  .nb-title { color: #7dd3fc; font-weight: 600; }
  .nb-ro { color: #b58a3a; font-size: 11px; }
  .nb-error { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .nb-scroll { flex: 1; min-height: 0; overflow-y: auto; padding: 12px; }
  .nb-cellwrap { display: flex; gap: 6px; }
  .nb-reorder { display: flex; flex-direction: column; gap: 2px; padding-top: 4px; }
  .nb-reorder button { background: none; border: none; color: #46586a; cursor: pointer; font-size: 9px; }
  .nb-reorder button:hover { color: #7dd3fc; }
  .nb-cell { flex: 1; min-width: 0; }
  .nb-add { background: #14233a; color: #93c5fd; border: 1px dashed #2a4a6a; border-radius: 6px; padding: 6px 12px; cursor: pointer; font-size: 12px; width: 100%; }
  .nb-add:hover { background: #1a2f4a; }
</style>
```

- [ ] **Step 2: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): NotebookPane with debounced autosave + cell ops"
```

---

## Task 10: Split ProcessView into container + ProcessEditor, wire push

**Files:**
- Create: `frontend/src/components/ProcessEditor.svelte` (lifted from today's `ProcessView.svelte`)
- Modify: `frontend/src/components/ProcessView.svelte` (becomes the container)

**Interfaces:**
- Consumes: `Splitter.svelte`, `NotebookPane`, `SaveFile`, `RecordPush`.
- Produces: `ProcessEditor` exposes `export async function insertAndSave(text)` → returns the inserted text after saving; `ProcessView` coordinates push (`insertAndSave` → `RecordPush`).

- [ ] **Step 1: Create ProcessEditor from the current ProcessView**

Create `frontend/src/components/ProcessEditor.svelte` with the **entire current contents of** `frontend/src/components/ProcessView.svelte`, then add an exported `insertAndSave` function inside its `<script>` (after `flush()`):

```js
  // Insert text as its own block at the cursor, persist, and return the inserted
  // text so the caller can record provenance. Handles SaveFile's # @id injection
  // by reloading the editor to the final on-disk content.
  export async function insertAndSave(text) {
    if (!view || !target) return ''
    let block = text.endsWith('\n') ? text : text + '\n'
    const sel = view.state.selection.main
    const line = view.state.doc.lineAt(sel.head)
    const at = line.to               // end of the cursor's line
    if (view.state.doc.sliceString(at - 1, at) !== '\n' && at !== 0) block = '\n' + block
    view.dispatch({ changes: { from: at, insert: block }, selection: { anchor: at + block.length } })
    const res = await SaveFile(target.filePath, view.state.doc.toString())
    const final = res?.finalContent ?? view.state.doc.toString()
    if (final !== view.state.doc.toString()) {
      savedContent = final
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: final } })
    } else {
      savedContent = final
    }
    return block
  }
```

Add `RecordPush` is **not** imported here (the container calls it). No other changes to the lifted file.

- [ ] **Step 2: Rewrite ProcessView as the container**

Replace the entire contents of `frontend/src/components/ProcessView.svelte`:

**Splitter contract (verified):** `Splitter.svelte` is NOT a two-slot wrapper — it is a bare drag *handle*. It takes `orientation` / `size` / `min` / `max` props and dispatches a `resize` CustomEvent whose `event.detail` is the new pixel size; the **parent owns the layout and pane widths** (see `TopBand.svelte` for the canonical usage: sized flex slots on each side of a `<Splitter>` handle, width tracked in parent state and fed back via `size`). The container below follows that pattern, tracking `leftWidth` locally and feeding it back to `size`.

```svelte
<script>
  import Splitter from './Splitter.svelte'
  import ProcessEditor from './ProcessEditor.svelte'
  import NotebookPane from './notebook/NotebookPane.svelte'
  import { RecordPush } from '../../wailsjs/go/main/App'

  export let extractId
  export let onAdvance = null

  let editorRef            // bound ProcessEditor instance
  let notebookId = null
  let filePath = null
  let leftWidth = 520      // px width of the left (editor) pane; user-resizable

  function onReady(e) { notebookId = e.detail.notebookId }
  function onEditorReady(e) { filePath = e.detail?.filePath ?? filePath }

  async function handlePush(text) {
    if (!editorRef) return
    const inserted = await editorRef.insertAndSave(text)
    if (notebookId && filePath && inserted) {
      try { await RecordPush(notebookId, filePath, inserted) } catch { /* provenance is best-effort */ }
    }
  }
</script>

<div class="pv-split">
  <div class="pv-pane pv-left" style="width: {leftWidth}px;">
    <ProcessEditor bind:this={editorRef} {extractId} {onAdvance} on:ready={onEditorReady} />
  </div>
  <Splitter orientation="vertical" size={leftWidth} min={340} max={1000}
            on:resize={(e) => (leftWidth = e.detail)} />
  <div class="pv-pane pv-right">
    <NotebookPane nodeID={extractId} onPush={handlePush} on:ready={onReady} />
  </div>
</div>

<style>
  .pv-split { position: absolute; inset: 0; display: flex; flex-direction: row; }
  .pv-pane { position: relative; height: 100%; }
  .pv-left { flex: 0 0 auto; }
  .pv-right { flex: 1 1 0; min-width: 0; }
</style>
```

Note: both child panes root themselves with `position: absolute; inset: 0` (`.pv-root`, `.nb-pane`), so each `.pv-pane` slot is `position: relative` with a real size for the children to fill. The `size={leftWidth}` feedback is required — `Splitter` reads `size` as the drag's starting value.

- [ ] **Step 3: Emit filePath from ProcessEditor**

In `ProcessEditor.svelte`, add `import { createEventDispatcher } from 'svelte'` and, inside `onMount` after `target = await GetExtractTarget(extractId)`, dispatch it:

```js
  const dispatch = createEventDispatcher()
  // ... inside onMount, right after target is set:
  dispatch('ready', { filePath: target.filePath })
```

- [ ] **Step 4: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 5: Manual smoke test**

Run: `make build && ./build/bin/texere`
Verify: opening an extract for processing shows the editor on the left and the notebook on the right; the context cell is populated; "+ N4L draft cell" adds a cell; typing then "Push to source" inserts the text at the editor cursor and it persists.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/ProcessEditor.svelte frontend/src/components/ProcessView.svelte
git commit -m "feat(process): split view container + ProcessEditor + push-to-source"
```

---

## Task 11: Provenance gutter dots

**Files:**
- Modify: `frontend/src/lib/n4l-editor.js`
- Modify: `frontend/src/components/ProcessEditor.svelte`
- Modify: `frontend/src/components/SourceTab.svelte`

**Interfaces:**
- Produces: `notebookDotsGutter({ getMarks })` — a CodeMirror gutter extension drawing a purple dot on lines returned by `getMarks()` (array of `{ line, notebookIds }`), with a click handler dispatching a DOM `CustomEvent('notebook:open', { detail: { notebookId } })` on the editor DOM.
- Consumes: `GetNotebookLinks` binding; `notebook:links-changed` Wails event.

- [ ] **Step 1: Add the gutter extension**

In `frontend/src/lib/n4l-editor.js`, add imports at the top (extend the existing `@codemirror/view` import) for `gutter`, `GutterMarker`, and add `StateField`, `StateEffect` from `@codemirror/state`. Then append:

```js
import { gutter, GutterMarker } from '@codemirror/view'
import { StateField, StateEffect } from '@codemirror/state'

const setDots = StateEffect.define() // payload: number[] of 1-indexed lines with dots
const dotsField = StateField.define({
  create() { return [] },
  update(val, tr) {
    for (const e of tr.effects) if (e.is(setDots)) return e.value
    return val
  },
})

class DotMarker extends GutterMarker {
  toDOM() {
    const el = document.createElement('span')
    el.className = 'cm-nb-dot'
    el.textContent = '●'
    return el
  }
}
const dotMarker = new DotMarker()

// notebookDotsGutter renders a purple dot in the gutter for each line in the
// dotsField. onClick receives the 1-indexed line number.
export function notebookDotsGutter(onClick) {
  return [
    dotsField,
    gutter({
      class: 'cm-nb-gutter',
      lineMarker(view, line) {
        const ln = view.state.doc.lineAt(line.from).number
        return view.state.field(dotsField).includes(ln) ? dotMarker : null
      },
      initialSpacer: () => dotMarker,
      domEventHandlers: {
        click(view, line) {
          const ln = view.state.doc.lineAt(line.from).number
          if (view.state.field(dotsField).includes(ln)) { onClick(ln); return true }
          return false
        },
      },
    }),
    EditorView.theme({
      '.cm-nb-dot': { color: '#a855f7', cursor: 'pointer', fontSize: '10px' },
    }),
  ]
}

// applyDots dispatches new dot lines onto an existing view.
export function applyDots(view, lines) {
  view.dispatch({ effects: setDots.of(lines) })
}
```

- [ ] **Step 2: Wire the gutter into ProcessEditor**

In `ProcessEditor.svelte`: import `notebookDotsGutter, applyDots` from `../lib/n4l-editor.js` and `GetNotebookLinks` from the bindings, plus Wails `EventsOn`. Add `notebookDotsGutter(() => {})` to the `extra:` extensions in `buildExtensions()` (dots are informational in the process editor; clicks are wired in the SourceTab). After the editor mounts and after each save, refresh dots:

```js
  async function refreshDots() {
    if (!view || !target) return
    try {
      const marks = await GetNotebookLinks(target.filePath)
      applyDots(view, (marks ?? []).map((m) => m.line))
    } catch { /* best-effort */ }
  }
```

Call `refreshDots()` at the end of `onMount` and at the end of `insertAndSave`. Add to `buildExtensions()` return:

```js
      extra: [notebookDotsGutter(() => {})],
```

(Insert `extra` into the `n4lBaseExtensions({...})` options object; it is already a supported field.)

- [ ] **Step 3: Wire the gutter + click into SourceTab**

Read `frontend/src/components/SourceTab.svelte` to find where it builds its CodeMirror view via `n4lBaseExtensions`. Add the same `extra: [notebookDotsGutter(onDotClick)]`, where:

```js
  import { notebookDotsGutter, applyDots } from '../lib/n4l-editor.js'
  import { GetNotebookLinks } from '../../wailsjs/go/main/App'
  import { EventsOn } from '../../wailsjs/runtime/runtime'

  let dotMarks = []      // [{line, notebookIds}]
  function onDotClick(line) {
    const m = dotMarks.find((x) => x.line === line)
    if (m && m.notebookIds?.length) {
      // Slice 1: open the first linked notebook read-only (see Task 12).
      openNotebookReadOnly(m.notebookIds[0])
    }
  }
  async function refreshDots(path) {
    try { dotMarks = (await GetNotebookLinks(path)) ?? []; if (view) applyDots(view, dotMarks.map((m) => m.line)) } catch { /* best-effort */ }
  }
```

Call `refreshDots(currentPath)` when a file loads into the SourceTab editor, and subscribe once: `EventsOn('notebook:links-changed', (p) => { if (p?.path === currentPath) refreshDots(currentPath) })`. `openNotebookReadOnly` is implemented in Task 12 — for this task, stub it as `function openNotebookReadOnly() {}` so the file compiles.

> Adapt variable names (`view`, `currentPath`) to the actual identifiers in `SourceTab.svelte`.

- [ ] **Step 4: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 5: Manual test**

Run: `make build && ./build/bin/texere`
Verify: after pushing a draft to source in the Process view, a purple dot appears in the gutter next to the pushed node's line (in the Process editor immediately; in the SourceTab after opening/refreshing that file).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/n4l-editor.js frontend/src/components/ProcessEditor.svelte frontend/src/components/SourceTab.svelte
git commit -m "feat(notebook): purple provenance gutter dots"
```

---

## Task 12: Reopen notebook read-only + Resume

**Files:**
- Modify: `frontend/src/components/SourceTab.svelte`
- (Uses existing `NotebookPane` `readOnly` prop from Task 9.)

**Interfaces:**
- Consumes: `NotebookPane` (`notebookId`, `readOnly` props), `GetNotebook`.
- Produces: `openNotebookReadOnly(notebookId)` opens an overlay panel in the SourceTab showing the notebook read-only with a **Resume** button that flips `readOnly` to false.

- [ ] **Step 1: Implement the reopen overlay in SourceTab**

Replace the Task 11 stub `openNotebookReadOnly` with real state, and render an overlay panel. Add to the `<script>`:

```js
  import NotebookPane from './notebook/NotebookPane.svelte'
  let reopenId = null
  let reopenReadOnly = true
  function openNotebookReadOnly(id) { reopenId = id; reopenReadOnly = true }
  function closeReopen() { reopenId = null }
  function resumeReopen() { reopenReadOnly = false }
```

Add to the component markup (as a right-docked overlay; adjust container class to match SourceTab's layout):

```svelte
{#if reopenId}
  <div class="nb-overlay">
    <div class="nb-overlay-bar">
      <span>Notebook</span>
      {#if reopenReadOnly}<button on:click={resumeReopen}>Resume (edit)</button>{/if}
      <button on:click={closeReopen}>✕ Close</button>
    </div>
    <div class="nb-overlay-body">
      {#key reopenId + reopenReadOnly}
        <NotebookPane notebookId={reopenId} readOnly={reopenReadOnly} onPush={() => {}} />
      {/key}
    </div>
  </div>
{/if}

<style>
  .nb-overlay { position: absolute; top: 0; right: 0; bottom: 0; width: 42%; min-width: 360px; display: flex; flex-direction: column; background: #0b1220; border-left: 1px solid #2a3a4a; z-index: 20; }
  .nb-overlay-bar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #101826; color: #7dd3fc; }
  .nb-overlay-bar button { margin-left: auto; background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .nb-overlay-bar button + button { margin-left: 6px; }
  .nb-overlay-body { position: relative; flex: 1; min-height: 0; }
</style>
```

> `NotebookPane` with a `notebookId` prop loads via `GetNotebook` (Task 9). When `readOnly` is false after Resume, its draft cells become editable and autosave resumes (its `onPush` is a no-op here, since this overlay has no left editor to push into — Resume is for editing the notebook itself, not pushing).

- [ ] **Step 2: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Manual test**

Run: `make build && ./build/bin/texere`
Verify: clicking a purple dot in the SourceTab opens the notebook read-only in the overlay; "Resume (edit)" makes its cells editable; "Close" dismisses it.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SourceTab.svelte
git commit -m "feat(notebook): reopen linked notebook read-only with Resume"
```

---

## Task 13: Manual test plan + full verification

**Files:**
- Create: `docs/superpowers/specs/2026-07-22-processing-notebook-slice1-manual-test-plan.md`

**Interfaces:** none.

- [ ] **Step 1: Write the manual test plan**

Create `docs/superpowers/specs/2026-07-22-processing-notebook-slice1-manual-test-plan.md`:

```markdown
# Processing Notebook — Slice 1 Manual Test Plan

Prereqs: a collection with at least one reading source that has produced an
extract in the Process queue; run Build Graph (Ctrl+G) once for orbit coverage.

## TC-1 Split view + context cell
1. Open an extract for processing.
   - EXPECT: left = N4L editor scrolled to the extract; right = notebook.
   - EXPECT: context cell shows Extract text; Tags/Neighbors if the graph is
     built; Source passage for a text/web source (or "From: <anchor>" / PDF note).
   - EXPECT: the split appears in well under a second (target < 300ms).

## TC-2 Draft cells CRUD + persistence
1. Click "+ N4L draft cell"; type `Newton (invented) calculus`.
2. Add a second cell; reorder with ▲/▼; delete one.
3. Restart the app; reopen the same extract.
   - EXPECT: the remaining draft cell and its text are still there.

## TC-3 Push to source + provenance dot
1. In a draft cell type `Newton (is a) physicist`; click "Push to source".
   - EXPECT: the line is inserted at the editor cursor and persists on disk.
   - EXPECT: a purple dot appears in the gutter on the pushed node's line.
2. Open the same file in the Source tab.
   - EXPECT: the purple dot appears there too.

## TC-4 Reopen read-only + Resume
1. In the Source tab, click the purple dot.
   - EXPECT: the notebook opens read-only in the overlay (cells not editable).
2. Click "Resume (edit)".
   - EXPECT: cells become editable; edits autosave.
3. Close and reopen the dot.
   - EXPECT: the edit persisted.

## TC-5 Graph-unbuilt degradation
1. On a fresh collection with no graph built, open an extract.
   - EXPECT: Neighbors shows "build the graph" hint, no error.
```

- [ ] **Step 2: Run the full backend test suite**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 3: Lint + full build**

Run: `make lint && make build`
Expected: lint clean; binary builds to `build/bin/texere`.

- [ ] **Step 4: Execute the manual test plan**

Run: `./build/bin/texere` and walk TC-1 … TC-5. All must pass.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-07-22-processing-notebook-slice1-manual-test-plan.md
git commit -m "docs(test-plan): Processing Notebook Slice 1 manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** split view (T10), persistence tables (T1), autosave (T9),
  context cell full-except-PDF (T5 `assembleContext` + T8 `ContextCell`), N4L
  draft cell with arrow highlighting/autocomplete (T8) + amber-underline
  validation & replace quick-fix (T8A), push-to-source +
  provenance (T6, T10), gutter dots (T11), reopen read-only + Resume (T12),
  success-criteria coverage in the manual plan (T13). Deferred items
  (LLM/graph/compute/sketch/image cells, queue "done", PDF page-text,
  manual-process, `\notebooks`, feed-forward, push-as-image, distillation nudge)
  are out of scope by design.
- **Arrow validation (T8A):** amber wavy underline on unregistered `(code)`
  arrows via `Decoration.mark` (no new dep — `@codemirror/lint` is intentionally
  avoided), plus a hover-tooltip "replace with nearest registered arrow"
  quick-fix. Per the scoping decision, the "register a new arrow" quick-fix is
  deferred to a later slice because no SSTconfig writer exists yet.
- **Provenance key deviation:** `(file_path, chapter, node_text)` instead of
  `node_id` — rationale in "Design correction" above.
- **Type consistency:** `NotebookView`/`ContextPayload`/`SourceInfo`/`OrbitItem`
  JSON tags match the frontend field reads (`camelCase`); `pushedNodeKeys`
  returns `[2]string{chapter, text}` consumed identically by `RecordPush` and
  mirrored by `GetNotebookLinks`.
```
