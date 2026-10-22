# Incremental Reading — Phase 2b (Process & Unify) Implementation Plan

**Goal:** Schedule extracts, add the Process face that distills/promotes them (recallable the same session), and fold reading + processing + recall into one Study surface with a focus switch over an interleaved queue.

**Architecture:** A new `extracts` table (learning.db schema v3) + deferral schedule; the existing `Sync` is extended to classify `(from)` nodes as extracts (so Build-Graph and a per-file re-sync both discover/retire them); a new content op inserts `# @card`/`# @cloze` above a node for immediate promotion; Wails bindings expose counts/next/promote/defer/dismiss; the frontend factors `StudyMode`'s card into a reusable `RecallCard`, adds a `ProcessView`, and composes everything in one `StudySurface` overlay.

**Tech Stack:** Go 1.25, `modernc.org/sqlite`, Wails v2, Svelte 3.49, CodeMirror 6. Reuses `internal/content`, the Phase-1/2a `internal/learning` store, and the 2a `ReadingView`/`SourcesPanel`.

## Global Constraints

- Pure Go, no new deps. learning.db migrates additively 2→3 (existing `items`/`review_logs`/`reading_elements` untouched).
- Extract PK = the node's **qualified `NodeID`** (`node.ID`, same base items key off) so reconcile matches "this node became a card."
- A node is classified by markers: `StudyMeta != nil` → item; else a node with `node.ID != ""` carrying a `(from)` arrow (on the node line or a ditto line) → extract.
- Extract schedule (machinery hidden): `next_due = now + 1.5^deferral_count` days, capped at 90; **Later** → deferral++ reschedule; **Dismiss/Promote** → `active=0`.
- Promotion is immediate: write the marker, save, re-`Sync` that one file → item created + extract retired in the same session.
- Writeback to a notes file reuses the Phase-1 dirty-buffer invariant (`a.savingPath` set/cleared around the write + `file:ids-injected` clean-reload).
- `StudyMode` stays a standalone recall-only entry (Ctrl+Shift+R), rebuilt on the shared `RecallCard`. The unified surface is Ctrl+Shift+S (Next); Ctrl+Shift+E opens it on Read.
- Go: `gofmt`, grouped imports. Tests `go test ./...`; frontend `cd frontend && npm run build` + manual quickstart.

---

## File structure

| File | Responsibility |
|---|---|
| `internal/learning/extracts.go` (+`_test.go`) | `Extract`, `ScheduleExtract`, extract CRUD; `db.go` schema bump to v3. |
| `internal/content/promote.go` (+`_test.go`) | `InsertMarkerAboveNode`, `InjectMissingExtractIDs`. |
| `internal/learning/sync.go` (+`extracts` in `sync_test.go`) | classify `(from)` nodes → upsert/retire extracts alongside items. |
| `internal/learning/queue.go` (+`_test.go`) | `StudyCounts`, `DueReading`. |
| `reading_methods.go` / `study_methods.go` (+`_test.go`) | bindings: counts/extract-queue/extract-target/promote/defer/dismiss + register-at-creation. |
| `frontend/src/components/RecallCard.svelte` | presentational card (front→reveal→grade). |
| `frontend/src/components/StudyMode.svelte` | standalone recall, rebuilt on `RecallCard`. |
| `frontend/src/components/ProcessView.svelte` | Process face (editor + action bar). |
| `frontend/src/components/StudySurface.svelte` + `stores/study.js` | unified overlay + focus switch. |
| `frontend/src/App.svelte` | entry shortcuts + overlay mounts. |

---

### Task 1: Extracts table (schema v3) + schedule + CRUD

**Files:**
- Modify: `internal/learning/db.go` (`schemaVersion = 3`, `case 2: ddlV3`, add `ddlV3`)
- Create: `internal/learning/extracts.go`
- Test: `internal/learning/extracts_test.go`

**Interfaces:**
- Produces:
  - `type Extract struct { ID, FilePath, Text, FromAnchor string; Due time.Time; DeferralCount int; Active bool }`
  - `func ScheduleExtract(prev Extract, action string, now time.Time) Extract` — `action ∈ {"later","dismiss","promote"}`. `later` → `DeferralCount++`, `Due = now + min(1.5^DeferralCount, 90) days`. `dismiss`/`promote` → `Active=false`.
  - `(*Store)` methods: `UpsertExtract(e Extract) (added bool, err error)` (insert new with `due=NULL`, deferral 0, active 1; existing id → update text/file/anchor + active=1, preserve schedule), `RetireMissingExtracts(filePath string, keep map[string]bool) (int, error)`, `GetExtract(id string) (Extract, bool, error)`, `DueExtracts(now time.Time, limit int) ([]Extract, error)` (active, due NULL or ≤now, due-first then new), `ApplyExtractSchedule(id string, e Extract) error`, `CountDueExtracts(now time.Time) (int, error)`.

- [ ] **Step 1: Bump schema + add v3 DDL in `db.go`**

In `internal/learning/db.go`: change `const schemaVersion = 2` to `= 3`; inside the `for v < schemaVersion { switch v { ... } }` loop add a `case 2`:

```go
		case 2:
			if _, err := s.db.Exec(ddlV3); err != nil {
				return fmt.Errorf("learning: apply v3: %w", err)
			}
```

Add the DDL constant next to `ddlV2`:

```go
const ddlV3 = `
CREATE TABLE IF NOT EXISTS extracts (
  id              TEXT PRIMARY KEY,
  file_path       TEXT NOT NULL,
  text            TEXT NOT NULL,
  from_anchor     TEXT,
  due             INTEGER,
  deferral_count  INTEGER NOT NULL DEFAULT 0,
  active          INTEGER NOT NULL DEFAULT 1,
  created_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_extracts_due ON extracts(due) WHERE active = 1;
`
```

- [ ] **Step 2: Write the failing test**

Create `internal/learning/extracts_test.go`:

```go
package learning

import (
	"testing"
	"time"
)

func TestExtractScheduleAndCRUD(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_000_000, 0)

	e := Extract{ID: "f.n4l:Ch:id:a", FilePath: "/c/reading/f.n4l", Text: "System 1 is fast",
		FromAnchor: "sources/f.txt char=10", Active: true}
	added, err := s.UpsertExtract(e)
	if err != nil || !added {
		t.Fatalf("UpsertExtract added=%v err=%v", added, err)
	}
	// New extract is due (due NULL).
	due, err := s.DueExtracts(now, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("DueExtracts len=%d err=%v want 1", len(due), err)
	}

	// Later → deferral 1, due 1.5 days out, leaves the due list.
	got, _, _ := s.GetExtract(e.ID)
	sched := ScheduleExtract(got, "later", now)
	if sched.DeferralCount != 1 {
		t.Fatalf("deferral=%d want 1", sched.DeferralCount)
	}
	wantDue := now.Add(time.Duration(1.5*24) * time.Hour)
	if sched.Due.Unix() != wantDue.Unix() {
		t.Fatalf("due=%v want %v", sched.Due, wantDue)
	}
	if err := s.ApplyExtractSchedule(e.ID, sched); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.DueExtracts(now, 10); len(d) != 0 {
		t.Fatalf("after later, due len=%d want 0", len(d))
	}

	// Cap at 90 days.
	big := ScheduleExtract(Extract{DeferralCount: 30, Active: true}, "later", now)
	if big.Due.Unix() != now.Add(90*24*time.Hour).Unix() {
		t.Fatalf("cap due=%v want now+90d", big.Due)
	}

	// Dismiss → inactive.
	dm := ScheduleExtract(got, "dismiss", now)
	if dm.Active {
		t.Fatal("dismiss should set Active=false")
	}

	// Re-upsert preserves schedule (does not reset deferral/due).
	got2, _, _ := s.GetExtract(e.ID)
	got2.Text = "edited"
	if a, _ := s.UpsertExtract(got2); a {
		t.Fatal("re-upsert should not be 'added'")
	}
	after, _, _ := s.GetExtract(e.ID)
	if after.DeferralCount != 1 || after.Text != "edited" {
		t.Fatalf("after re-upsert deferral=%d text=%q (want 1, edited)", after.DeferralCount, after.Text)
	}

	// RetireMissingExtracts retires ids not in keep.
	n, err := s.RetireMissingExtracts(e.FilePath, map[string]bool{})
	if err != nil || n != 1 {
		t.Fatalf("RetireMissingExtracts n=%d err=%v want 1", n, err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestExtractScheduleAndCRUD -v`
Expected: FAIL — `undefined: Extract` / `ScheduleExtract`.

- [ ] **Step 4: Write `extracts.go`**

Create `internal/learning/extracts.go`:

```go
package learning

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

// Extract is one distillation candidate: a (from) node not yet promoted to a card.
type Extract struct {
	ID            string
	FilePath      string
	Text          string
	FromAnchor    string
	Due           time.Time
	DeferralCount int
	Active        bool
}

const extractIntervalCapDays = 90

// ScheduleExtract is the pure deferral rule. action ∈ {"later","dismiss","promote"}.
func ScheduleExtract(prev Extract, action string, now time.Time) Extract {
	out := prev
	switch action {
	case "dismiss", "promote":
		out.Active = false
		return out
	case "later":
		out.DeferralCount = prev.DeferralCount + 1
	default:
		return out
	}
	days := math.Pow(1.5, float64(out.DeferralCount))
	if days > extractIntervalCapDays {
		days = extractIntervalCapDays
	}
	out.Due = now.Add(time.Duration(days * 24 * float64(time.Hour)))
	return out
}

func (s *Store) UpsertExtract(e Extract) (bool, error) {
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM extracts WHERE id=?`, e.ID).Scan(&exists); err != nil {
		return false, err
	}
	if exists > 0 {
		_, err := s.db.Exec(`UPDATE extracts SET file_path=?, text=?, from_anchor=?, active=1 WHERE id=?`,
			e.FilePath, e.Text, e.FromAnchor, e.ID)
		return false, err
	}
	_, err := s.db.Exec(`INSERT INTO extracts
		(id, file_path, text, from_anchor, due, deferral_count, active, created_at)
		VALUES (?,?,?,?,NULL,0,1,?)`,
		e.ID, e.FilePath, e.Text, e.FromAnchor, time.Now().Unix())
	if err != nil {
		return false, fmt.Errorf("learning: insert extract: %w", err)
	}
	return true, nil
}

func (s *Store) ApplyExtractSchedule(id string, e Extract) error {
	_, err := s.db.Exec(`UPDATE extracts SET due=?, deferral_count=?, active=? WHERE id=?`,
		nullableUnix(e.Due), e.DeferralCount, boolToInt(e.Active), id)
	return err
}

func (s *Store) RetireMissingExtracts(filePath string, keep map[string]bool) (int, error) {
	rows, err := s.db.Query(`SELECT id FROM extracts WHERE file_path=? AND active=1`, filePath)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		if !keep[id] {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if _, err := s.db.Exec(`UPDATE extracts SET active=0 WHERE id=?`, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (s *Store) GetExtract(id string) (Extract, bool, error) {
	row := s.db.QueryRow(`SELECT id, file_path, text, from_anchor, due, deferral_count, active
		FROM extracts WHERE id=?`, id)
	e, err := scanExtract(row)
	if err == sql.ErrNoRows {
		return Extract{}, false, nil
	}
	return e, err == nil, err
}

func (s *Store) DueExtracts(now time.Time, limit int) ([]Extract, error) {
	rows, err := s.db.Query(`SELECT id, file_path, text, from_anchor, due, deferral_count, active
		FROM extracts
		WHERE active=1 AND (due IS NULL OR due <= ?)
		ORDER BY (due IS NULL), due ASC
		LIMIT ?`, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Extract
	for rows.Next() {
		e, err := scanExtract(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) CountDueExtracts(now time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM extracts WHERE active=1 AND (due IS NULL OR due <= ?)`,
		now.Unix()).Scan(&n)
	return n, err
}

func scanExtract(sc scanner) (Extract, error) {
	var (
		e   Extract
		anc sql.NullString
		due sql.NullInt64
		act int
	)
	if err := sc.Scan(&e.ID, &e.FilePath, &e.Text, &anc, &due, &e.DeferralCount, &act); err != nil {
		return Extract{}, err
	}
	e.FromAnchor = anc.String
	e.Due = unixToTime(due)
	e.Active = act != 0
	return e, nil
}
```

(`nullableUnix`, `unixToTime`, `boolToInt`, and the `scanner` interface already exist in the package from Phase 1/2a.)

- [ ] **Step 5: Run the tests + commit**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestExtractScheduleAndCRUD -v && go test ./internal/learning/`
Expected: PASS (new test + all prior).

```bash
git add internal/learning/db.go internal/learning/extracts.go internal/learning/extracts_test.go
git commit -m "feat(learning): extracts table (schema v3) + deferral schedule + CRUD"
```

---

### Task 2: Content ops — insert marker + inject extract ids

**Files:**
- Create: `internal/content/promote.go`
- Test: `internal/content/promote_test.go`

**Interfaces:**
- Consumes: `readLines`/`writeLines`, `GenerateShortID`, `ParseFile` (existing).
- Produces:
  - `func InsertMarkerAboveNode(path string, nodeLine int, marker string) error` — inserts `marker` as its own line immediately above the 1-indexed `nodeLine`. Errors if `nodeLine` is out of range or `marker` doesn't start with `# @`.
  - `func InjectMissingExtractIDs(parsed *ParsedFile) ([]IDInjection, error)` — for every node that is NOT study-marked, has no `# @id`, and carries a `(from)` arrow (node or ditto), inject a `# @id` line above it (so hand-authored `(from)` nodes become schedulable). Mirrors `InjectMissingIDs` but for extract nodes; mutates `parsed` in place; batched atomic rewrite.

- [ ] **Step 1: Write the failing test**

Create `internal/content/promote_test.go`:

```go
package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInsertMarkerAboveNode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	body := "- Ch\n# @id a1\nSystem 1 is fast\n    (from) \"sources/f.txt char=0\"\n"
	os.WriteFile(p, []byte(body), 0o644)
	// node "System 1 is fast" is line 3 (1-indexed).
	if err := InsertMarkerAboveNode(p, 3, "# @cloze"); err != nil {
		t.Fatalf("InsertMarkerAboveNode: %v", err)
	}
	out, _ := os.ReadFile(p)
	lines := strings.Split(string(out), "\n")
	if strings.TrimSpace(lines[2]) != "# @cloze" || strings.TrimSpace(lines[3]) != "System 1 is fast" {
		t.Fatalf("marker not directly above node:\n%s", out)
	}
	if err := InsertMarkerAboveNode(p, 999, "# @card"); err == nil {
		t.Fatal("expected out-of-range error")
	}
	if err := InsertMarkerAboveNode(p, 3, "not-a-marker"); err == nil {
		t.Fatal("expected invalid-marker error")
	}
}

func TestInjectMissingExtractIDs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	// A (from) node with NO # @id and no card markers.
	body := "- Ch\nanchoring shifts estimates\n    (from) \"sources/f.txt char=20\"\n"
	os.WriteFile(p, []byte(body), 0o644)
	reg := &SSTRegistry{Arrows: map[string]ArrowDef{"from": {Code: "from", LongName: "from"}}}
	pf, _ := ParseFile(p, reg)
	injs, err := InjectMissingExtractIDs(pf)
	if err != nil || len(injs) != 1 {
		t.Fatalf("InjectMissingExtractIDs n=%d err=%v want 1", len(injs), err)
	}
	out, _ := os.ReadFile(p)
	if !strings.Contains(string(out), "# @id ") {
		t.Fatalf("expected an injected # @id:\n%s", out)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/content/ -run 'TestInsertMarkerAboveNode|TestInjectMissingExtractIDs' -v`
Expected: FAIL — `undefined: InsertMarkerAboveNode`.

- [ ] **Step 3: Write `promote.go`**

Create `internal/content/promote.go`:

```go
package content

import (
	"fmt"
	"strings"
)

// InsertMarkerAboveNode inserts marker as its own line directly above the
// 1-indexed nodeLine. marker must be a "# @..." study/metadata marker.
func InsertMarkerAboveNode(path string, nodeLine int, marker string) error {
	if !strings.HasPrefix(strings.TrimSpace(marker), "# @") {
		return fmt.Errorf("content: InsertMarkerAboveNode: not a marker: %q", marker)
	}
	lines, err := readLines(path)
	if err != nil {
		return fmt.Errorf("content: InsertMarkerAboveNode: %w", err)
	}
	if nodeLine < 1 || nodeLine > len(lines) {
		return fmt.Errorf("content: InsertMarkerAboveNode: line %d out of range (1..%d)", nodeLine, len(lines))
	}
	idx := nodeLine - 1
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, marker)
	out = append(out, lines[idx:]...)
	if err := writeLines(path, out); err != nil {
		return fmt.Errorf("content: InsertMarkerAboveNode: write: %w", err)
	}
	return nil
}

// nodeHasFromArrow reports whether the node (or one of its ditto lines) carries
// a (from) arrow, returning its target anchor.
func nodeHasFromArrow(n *Node) (string, bool) {
	for _, a := range n.Arrows {
		if a.Code == "from" {
			return a.Target, true
		}
	}
	for _, d := range n.Dittos {
		for _, a := range d.Arrows {
			if a.Code == "from" {
				return a.Target, true
			}
		}
	}
	return "", false
}

// InjectMissingExtractIDs injects a "# @id <shortid>" for every non-study-marked
// node that carries a (from) arrow and lacks an id. Mutates parsed in place via a
// single atomic rewrite (mirrors InjectMissingIDs for extract nodes).
func InjectMissingExtractIDs(parsed *ParsedFile) ([]IDInjection, error) {
	lines, err := readLines(parsed.Path)
	if err != nil {
		return nil, err
	}
	existing := collectExistingIDs(parsed)
	type pending struct {
		node *Node
		line int
		id   string
	}
	var todo []pending
	for ci := range parsed.Chapters {
		for ni := range parsed.Chapters[ci].Nodes {
			n := parsed.Chapters[ci].Nodes[ni]
			if n.StudyMeta != nil || n.ID != "" {
				continue
			}
			if _, ok := nodeHasFromArrow(n); !ok {
				continue
			}
			id, err := generateUniqueID(existing)
			if err != nil {
				return nil, err
			}
			existing[id] = true
			todo = append(todo, pending{node: n, line: n.Line, id: id})
		}
	}
	if len(todo) == 0 {
		return nil, nil
	}
	// Insert from the bottom up so earlier line numbers stay valid.
	var injs []IDInjection
	for i := len(todo) - 1; i >= 0; i-- {
		t := todo[i]
		idx := t.line - 1
		lines = append(lines[:idx], append([]string{"# @id " + t.id}, lines[idx:]...)...)
		t.node.ID = NodeID(parsed.Path, chapterTitleOf(parsed, t.node), t.id)
		injs = append(injs, IDInjection{Line: t.line, ShortID: t.id})
	}
	if err := writeLines(parsed.Path, lines); err != nil {
		return nil, err
	}
	return injs, nil
}
```

> The helpers `collectExistingIDs`, `generateUniqueID`, `chapterTitleOf`, and the exact `IDInjection` field names may differ in `writeback.go`/`nodeid.go`. READ `InjectMissingIDs` and `IDInjection` first and reuse the SAME helpers it uses (it already collects existing ids, generates unique ones, and computes `NodeID`). If a helper isn't exported within the package, call the same code path `InjectMissingIDs` uses. Adjust the field names in the `IDInjection{...}` literal to match the real struct (the test only checks the file contains `# @id ` and `len(injs)==1`).

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/simone/projects/texere && go test ./internal/content/ -run 'TestInsertMarkerAboveNode|TestInjectMissingExtractIDs' -v`
Expected: PASS.

- [ ] **Step 5: Run the content package + commit**

Run: `cd /home/simone/projects/texere && go test ./internal/content/`
Expected: all PASS.

```bash
git add internal/content/promote.go internal/content/promote_test.go
git commit -m "feat(content): InsertMarkerAboveNode + InjectMissingExtractIDs"
```

---

### Task 3: Extend Sync to discover extracts

**Files:**
- Modify: `internal/learning/sync.go`
- Create: `internal/learning/extracts_gen.go` (the `GenerateExtracts` classifier)
- Test: `internal/learning/sync_test.go` (add a test)

**Interfaces:**
- Consumes: `content.ParseFile`/`InjectMissingExtractIDs` (Task 2), `Store.UpsertExtract`/`RetireMissingExtracts` (Task 1).
- Produces: `func GenerateExtracts(pf *content.ParsedFile) []Extract` — for each node with `StudyMeta == nil`, `ID != ""`, and a `(from)` arrow, an `Extract{ID: node.ID, FilePath: pf.Path, Text: node.Text, FromAnchor: <anchor>}`. `Sync` now also injects extract ids, upserts extracts, and retires missing ones per file (`SyncResult` gains `ExtractsAdded`/`ExtractsRetired`).

- [ ] **Step 1: Write the failing test**

Add to `internal/learning/sync_test.go`:

```go
func TestSyncDiscoversAndRetiresExtracts(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	// A (from) extract node WITH an id, no card markers.
	body := "- Ch\n# @id e1\nSystem 1 is fast\n    (from) \"sources/f.txt char=0\"\n"
	os.WriteFile(p, []byte(body), 0o644)
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{"from": {Code: "from", LongName: "from"}}}
	store, _ := Open(dir)
	defer store.Close()

	res, err := Sync(store, []string{p}, reg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.ExtractsAdded != 1 {
		t.Fatalf("ExtractsAdded=%d want 1", res.ExtractsAdded)
	}
	if n, _ := store.CountDueExtracts(time.Now()); n != 1 {
		t.Fatalf("due extracts=%d want 1", n)
	}

	// Promote it: add # @card → on next sync the extract retires and an item appears.
	body2 := "- Ch\n# @id e1\n# @card\nSystem 1 is fast\n    (from) \"sources/f.txt char=0\"\n"
	os.WriteFile(p, []byte(body2), 0o644)
	res2, _ := Sync(store, []string{p}, reg)
	if res2.ExtractsRetired != 1 {
		t.Fatalf("ExtractsRetired=%d want 1", res2.ExtractsRetired)
	}
	if n, _ := store.CountDueExtracts(time.Now()); n != 0 {
		t.Fatalf("due extracts after promote=%d want 0", n)
	}
}
```

(Add imports `os`, `path/filepath`, `time` to `sync_test.go` if not already present.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestSyncDiscoversAndRetiresExtracts -v`
Expected: FAIL — `res.ExtractsAdded undefined`.

- [ ] **Step 3: Write `extracts_gen.go` + add fields to `SyncResult`**

Create `internal/learning/extracts_gen.go`:

```go
package learning

import "texere/internal/content"

// GenerateExtracts finds (from) nodes that are not study-marked and have an id.
func GenerateExtracts(pf *content.ParsedFile) []Extract {
	var out []Extract
	if pf == nil {
		return out
	}
	for _, ch := range pf.Chapters {
		for _, node := range ch.Nodes {
			if node.StudyMeta != nil || node.ID == "" {
				continue
			}
			anchor, ok := fromArrowTarget(node)
			if !ok {
				continue
			}
			out = append(out, Extract{ID: node.ID, FilePath: pf.Path, Text: node.Text, FromAnchor: anchor})
		}
	}
	return out
}

func fromArrowTarget(n content.Node) (string, bool) {
	for _, a := range n.Arrows {
		if a.Code == "from" {
			return a.Target, true
		}
	}
	for _, d := range n.Dittos {
		for _, a := range d.Arrows {
			if a.Code == "from" {
				return a.Target, true
			}
		}
	}
	return "", false
}
```

In `internal/learning/types.go` (where `SyncResult` is defined), add two fields:

```go
	ExtractsAdded   int `json:"extractsAdded"`
	ExtractsRetired int `json:"extractsRetired"`
```

> Confirm `content.Node.Arrows`/`Dittos[].Arrows` field names against `internal/content/types.go` (they are `Arrows []Arrow` with `Arrow.Code`/`Arrow.Target`, and `Dittos []DittoLine` with `DittoLine.Arrows`). If `GenerateExtracts` takes `content.Node` by value and the parser stores `[]*Node`, adjust to match the iteration style `GenerateCards` uses (it ranges `ch.Nodes` the same way).

- [ ] **Step 4: Extend `Sync` to handle extracts**

In `internal/learning/sync.go`, inside the per-file loop, after the existing items block (after `res.Retired += retired`), add the extract pass:

```go
		// Extract ids for hand-authored (from) nodes lacking one (writes back).
		exInjected, err := content.InjectMissingExtractIDs(pf)
		if err != nil {
			return res, err
		}
		for _, inj := range exInjected {
			if !inj.AlreadyHad {
				res.InjectedPaths = append(res.InjectedPaths, path)
				break
			}
		}
		exs := GenerateExtracts(pf)
		keepEx := make(map[string]bool, len(exs))
		for _, e := range exs {
			added, err := store.UpsertExtract(e)
			if err != nil {
				return res, err
			}
			if added {
				res.ExtractsAdded++
			}
			keepEx[e.ID] = true
		}
		retiredEx, err := store.RetireMissingExtracts(path, keepEx)
		if err != nil {
			return res, err
		}
		res.ExtractsRetired += retiredEx
```

> If `IDInjection` has no `AlreadyHad` field (it's used that way in the items block above — confirm), mirror exactly how the items block tests `inj.AlreadyHad`. `content` must be imported in `sync.go` (it already is).

- [ ] **Step 5: Run the test + whole package**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestSyncDiscoversAndRetiresExtracts -v && go test ./internal/learning/`
Expected: PASS (new + all prior; the item-promote path still works).

- [ ] **Step 6: Commit**

```bash
git add internal/learning/sync.go internal/learning/extracts_gen.go internal/learning/types.go internal/learning/sync_test.go
git commit -m "feat(learning): Sync discovers/retires extracts (classify (from) nodes)"
```

---

### Task 4: Study counts + due-reading query

**Files:**
- Create: `internal/learning/queue.go`
- Test: `internal/learning/queue_test.go`

**Interfaces:**
- Consumes: `Store` (Phase 1/2a/2b tables).
- Produces:
  - `type StudyStats3 struct { Read, Process, Recall int }`
  - `func (s *Store) StudyCounts(now time.Time) (StudyStats3, error)` — `Read` = active reading elements due/new and not done; `Process` = `CountDueExtracts`; `Recall` = active items due/new (reuse the items due count).
  - `func (s *Store) DueReading(now time.Time, limit int) ([]ReadingElement, error)` — active, not done, due NULL or ≤now, due-first.

- [ ] **Step 1: Write the failing test**

Create `internal/learning/queue_test.go`:

```go
package learning

import (
	"testing"
	"time"
)

func TestStudyCountsAndDueReading(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	now := time.Unix(2_000_000, 0)

	// One due reading element, one due extract, one new item.
	s.UpsertReading(ReadingElement{ID: "r1", RefPath: "sources/x.txt", NotesPath: "/c/r/x.n4l", Title: "X", IntervalDays: 1})
	s.UpsertExtract(Extract{ID: "x1", FilePath: "/c/r/x.n4l", Text: "t", Active: true})
	s.UpsertItem(GeneratedCard{NodeID: "n1", FilePath: "/f", CardType: CardTypeCloze, Front: "f", Back: "b", Discriminator: "0"})

	c, err := s.StudyCounts(now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Read != 1 || c.Process != 1 || c.Recall != 1 {
		t.Fatalf("counts=%+v want 1/1/1", c)
	}
	dr, err := s.DueReading(now, 10)
	if err != nil || len(dr) != 1 {
		t.Fatalf("DueReading len=%d err=%v want 1", len(dr), err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestStudyCountsAndDueReading -v`
Expected: FAIL — `s.StudyCounts undefined`.

- [ ] **Step 3: Write `queue.go`**

Create `internal/learning/queue.go`:

```go
package learning

import "time"

// StudyStats3 holds the three due counts driving the focus-switch badges.
type StudyStats3 struct {
	Read    int `json:"read"`
	Process int `json:"process"`
	Recall  int `json:"recall"`
}

func (s *Store) StudyCounts(now time.Time) (StudyStats3, error) {
	var out StudyStats3
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM reading_elements
		WHERE done=0 AND (due IS NULL OR due <= ?)`, now.Unix()).Scan(&out.Read); err != nil {
		return out, err
	}
	p, err := s.CountDueExtracts(now)
	if err != nil {
		return out, err
	}
	out.Process = p
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items
		WHERE active=1 AND (due IS NULL OR due <= ?)`, now.Unix()).Scan(&out.Recall); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) DueReading(now time.Time, limit int) ([]ReadingElement, error) {
	rows, err := s.db.Query(`SELECT id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done
		FROM reading_elements
		WHERE done=0 AND (due IS NULL OR due <= ?)
		ORDER BY (due IS NULL), due ASC
		LIMIT ?`, now.Unix(), limit)
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
```

> `scanReading` exists in `reading.go` (2a) and matches this column order — confirm the column list matches its `Scan` order; if 2a's `ReadingElement`/`scanReading` differs, use the same columns 2a's `ListReading` selects.

- [ ] **Step 4: Run the test + commit**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestStudyCountsAndDueReading -v`
Expected: PASS.

```bash
git add internal/learning/queue.go internal/learning/queue_test.go
git commit -m "feat(learning): StudyCounts + DueReading for the unified queue"
```

---

### Task 5: Bindings — counts, extract queue/target, promote/defer/dismiss, register-at-creation

**Files:**
- Create: `study_methods.go` (repo root)
- Modify: `reading_methods.go` (`CreateExtract` register-at-creation)
- Modify: `frontend/wailsjs/go/main/App.js` + `App.d.ts` (hand-add bindings)
- Test: `study_methods_test.go` (repo root)

**Interfaces:**
- Produces (App methods):
  - `GetStudyCounts() (*learning.StudyStats3, error)`
  - `GetExtractQueue(limit int) ([]ExtractView, error)` — due extracts (`{id, text, fromAnchor, filePath}`).
  - `GetExtractTarget(id string) (*ExtractTarget, error)` — `{id, filePath, line, text, fromAnchor}`; resolves `line` by parsing the file and finding the node whose `ID == id`.
  - `PromoteExtract(id, kind string) error` — `kind ∈ {"card","cloze"}`: resolve line, `InsertMarkerAboveNode(file, line, "# @"+kind)` under `savingPath` suppression, re-`Sync` that one file (item created, extract retired), emit `file:ids-injected`.
  - `DeferExtract(id string) error` / `DismissExtract(id string) error` — `ScheduleExtract`+`ApplyExtractSchedule`.
  - `type ExtractView struct{ ID, Text, FromAnchor, FilePath string }`; `type ExtractTarget struct{ ID, FilePath string; Line int; Text, FromAnchor string }`.
  - `CreateExtract` (reading_methods.go) gains: after appending, `learning.Sync(a.learnStore, []string{notesPath}, a.registry)` so the new extract registers immediately.

- [ ] **Step 1: Write the failing test**

Create `study_methods_test.go`:

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

func TestPromoteExtractCreatesItemRetiresExtract(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	p := filepath.Join(dir, "reading", "k.n4l")
	os.WriteFile(p, []byte("- K\n# @id e1\nSystem 1 is {fast}\n    (from) \"sources/k.txt char=0\"\n"), 0o644)
	store, _ := learning.Open(dir)
	defer store.Close()
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{"from": {Code: "from", LongName: "from"}}}
	a := &App{collectionDir: dir, learnStore: store, registry: reg}

	// Discover the extract.
	if _, err := learning.Sync(store, []string{p}, reg); err != nil {
		t.Fatal(err)
	}
	q, err := a.GetExtractQueue(10)
	if err != nil || len(q) != 1 {
		t.Fatalf("GetExtractQueue len=%d err=%v want 1", len(q), err)
	}
	// Promote to cloze.
	if err := a.PromoteExtract(q[0].ID, "cloze"); err != nil {
		t.Fatalf("PromoteExtract: %v", err)
	}
	body, _ := os.ReadFile(p)
	if !strings.Contains(string(body), "# @cloze") {
		t.Fatalf("marker not written:\n%s", body)
	}
	// Extract retired, an item now exists.
	if n, _ := store.CountDueExtracts(timeNowForTest()); n != 0 {
		t.Fatalf("due extracts after promote=%d want 0", n)
	}
	if st, _ := a.GetStudyCounts(); st.Recall < 1 {
		t.Fatalf("expected a recall item after promote, got %+v", st)
	}
}
```

Add this helper at the bottom of `study_methods_test.go`:

```go
import "time"

func timeNowForTest() time.Time { return time.Now() }
```

(Place the `time` import in the file's import block, not inline.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test . -run TestPromoteExtractCreatesItemRetiresExtract -v`
Expected: FAIL — `a.GetExtractQueue undefined`.

- [ ] **Step 3: Write `study_methods.go`**

Create `study_methods.go`:

```go
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/content"
	"texere/internal/learning"
)

// ExtractView is a due extract for the Process queue.
type ExtractView struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	FromAnchor string `json:"fromAnchor"`
	FilePath   string `json:"filePath"`
}

// ExtractTarget locates an extract's node line for the Process editor.
type ExtractTarget struct {
	ID         string `json:"id"`
	FilePath   string `json:"filePath"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
	FromAnchor string `json:"fromAnchor"`
}

func (a *App) GetStudyCounts() (*learning.StudyStats3, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	c, err := a.learnStore.StudyCounts(time.Now())
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (a *App) GetExtractQueue(limit int) ([]ExtractView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	if limit <= 0 {
		limit = 50
	}
	exs, err := a.learnStore.DueExtracts(time.Now(), limit)
	if err != nil {
		return nil, err
	}
	out := make([]ExtractView, 0, len(exs))
	for _, e := range exs {
		out = append(out, ExtractView{ID: e.ID, Text: e.Text, FromAnchor: e.FromAnchor, FilePath: e.FilePath})
	}
	return out, nil
}

// nodeLineByID parses file and returns the 1-indexed line of the node whose ID==id.
func (a *App) nodeLineByID(filePath, id string) (int, error) {
	pf, _ := content.ParseFile(filePath, a.registry)
	if pf == nil {
		return 0, fmt.Errorf("parse %s failed", filePath)
	}
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.ID == id {
				return n.Line, nil
			}
		}
	}
	return 0, fmt.Errorf("node %q not found in %s", id, filePath)
}

func (a *App) GetExtractTarget(id string) (*ExtractTarget, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	e, ok, err := a.learnStore.GetExtract(id)
	if err != nil || !ok {
		return nil, fmt.Errorf("GetExtractTarget: %q not found", id)
	}
	line, err := a.nodeLineByID(e.FilePath, id)
	if err != nil {
		return nil, err
	}
	return &ExtractTarget{ID: e.ID, FilePath: e.FilePath, Line: line, Text: e.Text, FromAnchor: e.FromAnchor}, nil
}

func (a *App) PromoteExtract(id, kind string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	if kind != "card" && kind != "cloze" {
		return fmt.Errorf("PromoteExtract: bad kind %q", kind)
	}
	e, ok, err := a.learnStore.GetExtract(id)
	if err != nil || !ok {
		return fmt.Errorf("PromoteExtract: %q not found", id)
	}
	line, err := a.nodeLineByID(e.FilePath, id)
	if err != nil {
		return err
	}
	a.savingPath = e.FilePath
	wErr := content.InsertMarkerAboveNode(e.FilePath, line, "# @"+kind)
	a.savingPath = ""
	if wErr != nil {
		return wErr
	}
	// Re-sync this one file: the node now has StudyMeta → item created, extract retired.
	if _, err := learning.Sync(a.learnStore, []string{e.FilePath}, a.registry); err != nil {
		return err
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "file:ids-injected", map[string]string{"path": e.FilePath})
	}
	return nil
}

func (a *App) DeferExtract(id string) error    { return a.rescheduleExtract(id, "later") }
func (a *App) DismissExtract(id string) error  { return a.rescheduleExtract(id, "dismiss") }

func (a *App) rescheduleExtract(id, action string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	e, ok, err := a.learnStore.GetExtract(id)
	if err != nil || !ok {
		return fmt.Errorf("rescheduleExtract: %q not found", id)
	}
	return a.learnStore.ApplyExtractSchedule(id, learning.ScheduleExtract(e, action, time.Now()))
}
```

- [ ] **Step 4: Register-at-creation in `reading_methods.go`**

In `reading_methods.go`'s `CreateExtract`, after the extract is appended and progress updated (after `_ = a.learnStore.SetReadingProgress(...)`, before the `file:ids-injected` emit / return), add a one-file sync so the new extract registers immediately:

```go
	// Register the new extract in the queue immediately (reuses the classify pass).
	if _, err := learning.Sync(a.learnStore, []string{r.NotesPath}, a.registry); err != nil {
		// Non-fatal: it will be picked up on the next Build Graph.
		runtime.LogWarningf(a.ctx, "CreateExtract: register sync: %v", err)
	}
```

> Ensure `learning` is imported in `reading_methods.go` (it is). If `runtime.LogWarningf` isn't the logging idiom used elsewhere, use `log.Printf` (mirror how `graph_methods.go` logs the sync error).

- [ ] **Step 5: Run the test + build**

Run: `cd /home/simone/projects/texere && go test . -run TestPromoteExtractCreatesItemRetiresExtract -v && go build ./... && go test ./...`
Expected: PASS + clean build + whole suite green.

- [ ] **Step 6: Hand-add JS bindings**

Append to `frontend/wailsjs/go/main/App.js` (matching the existing style) and `App.d.ts` (`Promise<any>`): `GetStudyCounts()`, `GetExtractQueue(arg1)`, `GetExtractTarget(arg1)`, `PromoteExtract(arg1,arg2)`, `DeferExtract(arg1)`, `DismissExtract(arg1)`.

- [ ] **Step 7: Verify frontend builds + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0.

```bash
git add study_methods.go study_methods_test.go reading_methods.go frontend/wailsjs/go/main/App.js frontend/wailsjs/go/main/App.d.ts
git commit -m "feat(study): bindings — counts, extract queue/target, promote/defer/dismiss, register-at-creation"
```

---

### Task 6: RecallCard component + StudyMode rebuilt on it

**Files:**
- Create: `frontend/src/components/RecallCard.svelte`
- Modify: `frontend/src/components/StudyMode.svelte`

**Interfaces:**
- Produces: `RecallCard` — props `card` (CardView | null) + `onGrade(n)`; owns reveal + Space/1-4 keys; renders front→reveal→grades, or "Nothing due 🎉" when `card == null`. `StudyMode` keeps its overlay chrome (bar + Esc/close + queue load via `SyncStudyItems`/`GetDueQueue`/`GradeItem`) and renders `<RecallCard>`.

- [ ] **Step 1: Create `RecallCard.svelte`**

Create `frontend/src/components/RecallCard.svelte`:

```svelte
<script>
  export let card = null      // CardView | null
  export let onGrade = null   // (n:1..4) => void

  let revealed = false
  let lastId = null
  // Reset reveal whenever the card changes.
  $: if (card?.id !== lastId) { lastId = card?.id ?? null; revealed = false }

  const GRADES = [
    { n: 1, label: 'Again' }, { n: 2, label: 'Hard' },
    { n: 3, label: 'Good' }, { n: 4, label: 'Easy' },
  ]

  function onKey(e) {
    if (!card) return
    if (!revealed && (e.key === ' ' || e.key === 'Enter')) { e.preventDefault(); revealed = true; return }
    if (revealed && e.key >= '1' && e.key <= '4') { e.preventDefault(); onGrade?.(parseInt(e.key, 10)) }
  }
</script>

<svelte:window on:keydown={onKey} />

{#if !card}
  <div class="rc-center"><p>Nothing due. 🎉</p></div>
{:else}
  <div class="rc-center">
    <div class="rc-card">
      <div class="rc-front">{card.front}</div>
      {#if revealed}<hr /><div class="rc-back">{card.back}</div>{/if}
    </div>
    {#if !revealed}
      <button class="rc-btn" on:click={() => (revealed = true)}>Show answer (Space)</button>
    {:else}
      <div class="rc-grades">
        {#each GRADES as g}
          <button class="rc-btn" on:click={() => onGrade?.(g.n)}>{g.label} ({g.n})</button>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  .rc-center { flex: 1 1 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 18px; padding: 24px; }
  .rc-card { max-width: 640px; width: 100%; border: 1px solid #2a3a4a; border-radius: 8px; padding: 28px; background: #1b2636; text-align: center; }
  .rc-front { color: #e8eaf0; font-size: 16px; }
  .rc-back { color: #7ecfa0; font-size: 15px; }
  .rc-card hr { border: none; border-top: 1px solid #2a3a4a; margin: 16px 0; }
  .rc-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 8px 14px; cursor: pointer; font-size: 13px; }
  .rc-btn:hover { background: #244a72; }
  .rc-grades { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; }
</style>
```

- [ ] **Step 2: Rebuild `StudyMode.svelte` on `RecallCard`**

Replace `frontend/src/components/StudyMode.svelte` with:

```svelte
<script>
  import { onMount } from 'svelte'
  import { SyncStudyItems, GetDueQueue, GradeItem } from '../../wailsjs/go/main/App'
  import RecallCard from './RecallCard.svelte'

  export let onClose = null

  let card = null
  let loading = true
  let error = ''
  let remaining = 0
  let shownAt = 0

  async function load() {
    loading = true; error = ''
    try {
      await SyncStudyItems()
      const q = await GetDueQueue(50)
      remaining = q?.length ?? 0
      card = q && q.length ? q[0] : null
    } catch (e) { error = String(e?.message ?? e) }
    finally { loading = false }
    if (card) shownAt = Date.now()
  }

  async function grade(n) {
    if (!card) return
    const elapsed = Date.now() - shownAt
    try {
      const next = await GradeItem(card.id, n, elapsed)
      card = next ?? null
      remaining = card ? Math.max(0, remaining - 1) : 0
    } catch (e) { error = String(e?.message ?? e) }
    if (card) shownAt = Date.now()
  }

  function onKey(e) { if (e.key === 'Escape') onClose?.() }
  onMount(load)
</script>

<svelte:window on:keydown={onKey} />

<div class="study-root" role="dialog" aria-label="Study">
  <div class="study-bar">
    <span class="study-title">Recall</span>
    <span class="study-count">{remaining} due</span>
    <button class="study-close" on:click={() => onClose?.()} title="Close">×</button>
  </div>
  {#if loading}
    <div class="study-center"><p>Loading…</p></div>
  {:else if error}
    <div class="study-center"><p class="study-error">{error}</p></div>
  {:else}
    <RecallCard {card} onGrade={grade} />
  {/if}
</div>

<style>
  .study-root { position: absolute; inset: 0; z-index: 60; display: flex; flex-direction: column; background: #131c29; }
  .study-bar { flex: 0 0 auto; display: flex; align-items: center; gap: 12px; padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .study-title { color: #7dd3fc; font-weight: 600; }
  .study-count { color: #8aa; font-size: 12px; }
  .study-close { margin-left: auto; background: none; border: none; color: #8899aa; font-size: 20px; line-height: 1; cursor: pointer; }
  .study-close:hover { color: #e8eaf0; }
  .study-center { flex: 1 1 0; display: flex; align-items: center; justify-content: center; }
  .study-error { color: #ff8080; }
</style>
```

- [ ] **Step 3: Verify build + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0. (StudyMode still works on Ctrl+Shift+R, now via RecallCard.)

```bash
git add frontend/src/components/RecallCard.svelte frontend/src/components/StudyMode.svelte
git commit -m "refactor(study): extract RecallCard; rebuild StudyMode on it"
```

---

### Task 7: ProcessView component

**Files:**
- Create: `frontend/src/components/ProcessView.svelte`

**Interfaces:**
- Consumes: `GetExtractTarget`, `PromoteExtract`, `DeferExtract`, `DismissExtract`, `GetFileContent`, `SaveFile`; n4l editor extensions; the `extractId` prop.
- Produces: `ProcessView` — props `extractId` + `onAdvance()` (called after an action so the parent serves the next element). Loads the extract's file into an N4L editor focused on its line; Refine = edit + Ctrl+S save; the action bar promotes/defers/dismisses then calls `onAdvance()`.

- [ ] **Step 1: Create `ProcessView.svelte`**

Create `frontend/src/components/ProcessView.svelte`:

```svelte
<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView, keymap, lineNumbers } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { defaultKeymap, historyKeymap, history } from '@codemirror/commands'
  import { autocompletion } from '@codemirror/autocomplete'
  import { workbenchStore } from '../stores/workbench.js'
  import { n4lLanguage } from '../lib/n4l-language.js'
  import { n4lHighlighting } from '../lib/n4l-highlight.js'
  import { arrowCompletion } from '../lib/arrow-autocomplete.js'
  import { GetExtractTarget, PromoteExtract, DeferExtract, DismissExtract, GetFileContent, SaveFile, GetRecentArrows, UpdateRecentArrows } from '../../wailsjs/go/main/App'

  export let extractId
  export let onAdvance = null

  let target = null
  let container
  let view
  let savedContent = ''
  let error = ''
  let arrowDefsRef = []
  let recentCodesRef = []
  $: arrowDefsRef = $workbenchStore.arrowDefs ?? []

  function handleArrowSelected(code) {
    recentCodesRef = [code, ...recentCodesRef.filter((c) => c !== code)].slice(0, 10)
    UpdateRecentArrows(recentCodesRef).catch(() => {})
  }

  async function flush() {
    if (!view) return
    const cur = view.state.doc.toString()
    if (cur !== savedContent) {
      const res = await SaveFile(target.filePath, cur)
      savedContent = res?.finalContent ?? cur
    }
  }

  async function act(fn) {
    error = ''
    try {
      await flush()                 // never lose a refine edit
      await fn()
      onAdvance?.()
    } catch (e) { error = String(e?.message ?? e) }
  }

  const makeCard = () => act(() => PromoteExtract(extractId, 'card'))
  const makeCloze = () => act(() => PromoteExtract(extractId, 'cloze'))
  const later = () => act(() => DeferExtract(extractId))
  const dismiss = () => act(() => DismissExtract(extractId))

  function focusLine(n) {
    if (!view || !n) return
    const doc = view.state.doc
    if (n < 1 || n > doc.lines) { view.focus(); return }
    const line = doc.line(n)
    view.dispatch({ selection: { anchor: line.from }, effects: EditorView.scrollIntoView(line.from, { y: 'center' }) })
    view.focus()
  }

  onMount(async () => {
    try {
      target = await GetExtractTarget(extractId)
      try { const s = await GetRecentArrows(); if (s?.length) recentCodesRef = s } catch { /* best-effort */ }
      const text = (await GetFileContent(target.filePath)) ?? ''
      savedContent = text
      view = new EditorView({
        parent: container,
        state: EditorState.create({
          doc: text,
          extensions: [
            lineNumbers(), history(), n4lLanguage, n4lHighlighting,
            autocompletion({ override: [(ctx) => arrowCompletion(arrowDefsRef, recentCodesRef, handleArrowSelected)(ctx)] }),
            keymap.of([{ key: 'Mod-s', run: () => { flush(); return true }, preventDefault: true }, ...defaultKeymap, ...historyKeymap]),
            EditorView.lineWrapping,
            EditorView.theme({ '&': { height: '100%', background: '#1b2636' }, '.cm-content': { fontFamily: 'monospace', fontSize: '0.85rem', color: '#e8eaf0' } }),
          ],
        }),
      })
      focusLine(target.line)
    } catch (e) { error = String(e?.message ?? e) }
  })

  onDestroy(() => view?.destroy())
</script>

<div class="pv-root">
  <div class="pv-bar">
    <span class="pv-title">Process</span>
    {#if target}<span class="pv-anchor" title={target.fromAnchor}>▸ {target.fromAnchor}</span>{/if}
    <span class="pv-spacer"></span>
    <button class="pv-btn" on:click={makeCard}>→ Make card</button>
    <button class="pv-btn" on:click={makeCloze}>{'{ }'} Cloze</button>
    <button class="pv-btn" on:click={later}>⏳ Later</button>
    <button class="pv-btn" on:click={dismiss}>✕ Dismiss</button>
  </div>
  {#if error}<div class="pv-error">{error}</div>{/if}
  <div class="pv-editor" bind:this={container}></div>
</div>

<style>
  .pv-root { position: absolute; inset: 0; display: flex; flex-direction: column; background: #131c29; }
  .pv-bar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .pv-title { color: #7dd3fc; font-weight: 600; }
  .pv-anchor { color: #5a7a9a; font-size: 11px; }
  .pv-spacer { flex: 1; }
  .pv-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 4px 10px; cursor: pointer; font-size: 12px; }
  .pv-btn:hover { background: #244a72; }
  .pv-error { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .pv-editor { flex: 1; min-height: 0; overflow: hidden; }
</style>
```

> "Refine" = editing the buffer + Ctrl+S (the `Mod-s` keymap). "Open in explorer" is deferred to keep this lean (the strategy's escape hatch can be added in 2c). The action bar both acts and advances via `onAdvance`.

- [ ] **Step 2: Verify build + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0 (component not yet mounted; this only checks it compiles).

```bash
git add frontend/src/components/ProcessView.svelte
git commit -m "feat(process): ProcessView — focused editor + Make card/Cloze/Later/Dismiss"
```

---

### Task 8: StudySurface + entry wiring

**Files:**
- Modify: `frontend/src/stores/study.js`
- Create: `frontend/src/components/StudySurface.svelte`
- Modify: `frontend/src/App.svelte`

**Interfaces:**
- Consumes: `GetStudyCounts`, `GetExtractQueue`, `GetDueQueue`, `GradeItem`, `DueReading`-via... (reading is served through the existing `GetReadingList` filtered client-side, or a focus opens the next due reading element); the components `ReadingView`, `SourcesPanel`, `ProcessView`, `RecallCard`.
- Produces: `studySurfaceOpen` (+ `studyFocus` initial) store; `StudySurface` overlay composing the four faces.

- [ ] **Step 1: Extend the store**

In `frontend/src/stores/study.js`, add:

```js
// studySurfaceOpen — the unified Study surface overlay is visible.
export const studySurfaceOpen = writable(false)
// studyFocus — 'next' | 'read' | 'process' | 'recall'
export const studyFocus = writable('next')
```

(Keep the existing `studyOpen` export — it drives the standalone StudyMode.)

- [ ] **Step 2: Create `StudySurface.svelte`**

Create `frontend/src/components/StudySurface.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import { GetStudyCounts, GetExtractQueue, GetReadingList, GetDueQueue, GradeItem } from '../../wailsjs/go/main/App'
  import { studyFocus } from '../stores/study.js'
  import { activeReading } from '../stores/reading.js'
  import ReadingView from './ReadingView.svelte'
  import SourcesPanel from './SourcesPanel.svelte'
  import ProcessView from './ProcessView.svelte'
  import RecallCard from './RecallCard.svelte'

  export let onClose = null

  let counts = { read: 0, process: 0, recall: 0 }
  let error = ''

  // Process state
  let processId = null
  // Recall state
  let recallCard = null
  let recallShownAt = 0
  // Read state: activeReading store + library fallback

  async function refresh() {
    try { counts = (await GetStudyCounts()) ?? counts } catch (e) { error = String(e?.message ?? e) }
  }

  async function loadProcess() {
    const q = await GetExtractQueue(1)
    processId = q && q.length ? q[0].id : null
  }
  async function loadRecall() {
    const q = await GetDueQueue(1)
    recallCard = q && q.length ? q[0] : null
    recallShownAt = Date.now()
  }
  async function loadRead() {
    if ($activeReading) return
    const list = await GetReadingList()
    const due = (list ?? []).find((r) => !r.done && (r.due === 0 || r.due * 1000 <= Date.now()))
    if (due) activeReading.set(due) // open the next due source; else SourcesPanel shows the library
  }

  // Drive the active focus.
  async function enter(focus) {
    error = ''
    studyFocus.set(focus)
    await refresh()
    if (focus === 'process' || focus === 'next') await loadProcess()
    if (focus === 'recall' || focus === 'next') await loadRecall()
    if (focus === 'read' || focus === 'next') await loadRead()
  }

  // "Next": rotate read → process → recall, picking the first non-empty.
  function nextKind() {
    if (counts.read > 0 && !$activeReading) return 'read'
    if ($activeReading) return 'read'
    if (counts.process > 0) return 'process'
    if (counts.recall > 0) return 'recall'
    return 'empty'
  }

  async function onProcessAdvance() { await refresh(); await loadProcess(); if ($studyFocus === 'next' && !processId) await advanceNext() }
  async function onRecallGrade(n) {
    if (!recallCard) return
    try {
      recallCard = (await GradeItem(recallCard.id, n, Date.now() - recallShownAt)) ?? null
      recallShownAt = Date.now()
    } catch (e) { error = String(e?.message ?? e) }
    await refresh()
    if ($studyFocus === 'next' && !recallCard) await advanceNext()
  }
  async function advanceNext() { await refresh(); studyFocus.set('next') }

  function onKey(e) { if (e.key === 'Escape') onClose?.() }
  onMount(() => enter($studyFocus))
</script>

<svelte:window on:keydown={onKey} />

<div class="ss-root" role="dialog" aria-label="Study">
  <div class="ss-bar">
    <span class="ss-title">Study</span>
    {#each [['next','Next'],['read','Read'],['process','Process'],['recall','Recall']] as [k, label]}
      <button class="ss-tab" class:active={$studyFocus === k} on:click={() => enter(k)}>
        {label}{#if k === 'read' && counts.read}<sup>{counts.read}</sup>{:else if k === 'process' && counts.process}<sup>{counts.process}</sup>{:else if k === 'recall' && counts.recall}<sup>{counts.recall}</sup>{/if}
      </button>
    {/each}
    <span class="ss-spacer"></span>
    <button class="ss-close" on:click={() => onClose?.()} title="Close (Esc)">×</button>
  </div>
  {#if error}<div class="ss-error">{error}</div>{/if}

  <div class="ss-body">
    {#if $studyFocus === 'read' || ($studyFocus === 'next' && nextKind() === 'read')}
      {#if $activeReading}
        <ReadingView reading={$activeReading} onClose={() => { activeReading.set(null); enter('next') }} />
      {:else}
        <SourcesPanel onClose={() => onClose?.()} />
      {/if}
    {:else if $studyFocus === 'process' || ($studyFocus === 'next' && nextKind() === 'process')}
      {#if processId}
        {#key processId}<ProcessView extractId={processId} onAdvance={onProcessAdvance} />{/key}
      {:else}
        <div class="ss-empty">No extracts to process.</div>
      {/if}
    {:else if $studyFocus === 'recall' || ($studyFocus === 'next' && nextKind() === 'recall')}
      <RecallCard card={recallCard} onGrade={onRecallGrade} />
    {:else}
      <div class="ss-empty">Nothing due. 🎉</div>
    {/if}
  </div>
</div>

<style>
  .ss-root { position: absolute; inset: 0; z-index: 60; display: flex; flex-direction: column; background: #131c29; }
  .ss-bar { display: flex; align-items: center; gap: 6px; padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .ss-title { color: #7dd3fc; font-weight: 600; margin-right: 8px; }
  .ss-tab { background: none; border: 1px solid transparent; color: #8aa; border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .ss-tab.active { background: #1e3a5a; color: #e8eaf0; border-color: #2a3a4a; }
  .ss-tab sup { color: #7ecfa0; margin-left: 2px; }
  .ss-spacer { flex: 1; }
  .ss-close { background: none; border: none; color: #8899aa; font-size: 18px; cursor: pointer; }
  .ss-error { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .ss-body { flex: 1; min-height: 0; position: relative; }
  .ss-empty { display: flex; align-items: center; justify-content: center; height: 100%; color: #8aa; }
</style>
```

- [ ] **Step 3: Wire `App.svelte`**

In `frontend/src/App.svelte`:
1. Imports: `import StudySurface from './components/StudySurface.svelte'` and add `studySurfaceOpen, studyFocus` to the `study.js` import.
2. In `handleKeydown`, change the reading shortcut and add the Study shortcut. Replace the existing `Ctrl+Shift+E` block (which set `readingOpen`) so E opens the surface on Read, and add Ctrl+Shift+S for the surface:
```js
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 's' || e.key === 'S')) {
      e.preventDefault(); e.stopPropagation()
      studyFocus.set('next'); studySurfaceOpen.update((v) => !v); return
    }
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'e' || e.key === 'E')) {
      e.preventDefault(); e.stopPropagation()
      studyFocus.set('read'); studySurfaceOpen.set(true); return
    }
```
(Keep the existing `Ctrl+Shift+R` → `studyOpen` block unchanged — standalone StudyMode stays.)
3. In the overlay region, REPLACE the standalone `{#if $readingOpen}<SourcesPanel/>` and `{#if $activeReading}<ReadingView/>` mounts with a single surface mount, and keep the StudyMode mount:
```svelte
    {#if $studyOpen}
      <StudyMode onClose={() => studyOpen.set(false)} />
    {/if}
    {#if $studySurfaceOpen}
      <StudySurface onClose={() => { studySurfaceOpen.set(false); activeReading.set(null) }} />
    {/if}
```
Remove the now-folded `{#if $readingOpen}…` and the standalone `{#if $activeReading}…` blocks (their content lives inside `StudySurface` now). Leave the `readingOpen`/`activeReading` imports if still referenced; remove `readingOpen` usage if it's no longer mounted.

> READ App.svelte's current handler + overlay region first and adapt these edits to the real surrounding code; match the `collectionOpen` guard so the surface only opens with a collection loaded.

- [ ] **Step 4: Verify build + manual quickstart + commit**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0.

Manual (`make dev`, collection open):
1. `Ctrl+Shift+E` (or import via the Sources library in Read) → read a source, select → Extract.
2. `Ctrl+Shift+S` → Study (Next). Switch to **Process** → the just-made extract appears in a focused editor. Click **Make card** / **Cloze** → it leaves Process; switch to **Recall** → the card is recallable this session.
3. **Later** defers an extract; **Dismiss** retires it. The tab badges show due counts.
4. `Ctrl+Shift+R` still opens the standalone **Recall**-only StudyMode.

```bash
git add frontend/src/stores/study.js frontend/src/components/StudySurface.svelte frontend/src/App.svelte
git commit -m "feat(study): unified StudySurface (Ctrl+Shift+S) composing Read/Process/Recall"
```

---

## Self-Review

**Spec coverage (design §s → tasks):**
- §4.1 extracts table schema v3 → Task 1. ✅
- §4.2 keying by NodeID + marker classification → Tasks 1 (PK), 3 (`GenerateExtracts`). ✅
- §5 extract schedule (deferral, cap 90, Later/Dismiss/Promote-retire) → Task 1 (`ScheduleExtract`). ✅
- §5 discovery hybrid (register-at-creation + reconcile) → Task 3 (Sync) + Task 5 (`CreateExtract` re-sync). ✅
- §5 immediate promotion (marker + per-file re-sync) → Task 2 (`InsertMarkerAboveNode`) + Task 5 (`PromoteExtract`). ✅
- §5 interleaved queue / counts → Task 4 (`StudyCounts`/`DueReading`) + Task 8 (`StudySurface` Next rotation). ✅
- §6 `InjectMissingExtractIDs` for hand-authored `(from)` nodes → Task 2 + Task 3 (called in Sync). ✅
- §6 components: extracts store, content ops, discovery, queue, bindings, ProcessView, RecallCard, StudySurface → Tasks 1-8. ✅
- §6 StudyMode kept standalone on RecallCard; entries S/R/E → Tasks 6, 8. ✅
- §7 edge cases: dirty-buffer invariant (savingPath + file:ids-injected) → Task 5 (`PromoteExtract`) + Task 7 (`flush` before action); empty queues → Tasks 6/8 empty states; schema v3 additive → Task 1. ✅
- §8 testing → Go units Tasks 1-5; manual quickstart Task 8. ✅

**Placeholder scan:** No TBD/TODO; every code step has complete code; commands have expected output. The two "confirm the real helper names" notes (Task 2 `InjectMissingExtractIDs` helpers; Task 3 `IDInjection.AlreadyHad`) point the implementer at the exact existing code to mirror — they are verification directives, not placeholders. ✅

**Type consistency:** `Extract` fields identical in Tasks 1 (def), 3 (`GenerateExtracts`), 5 (binding). `ScheduleExtract(prev, action, now)` signature matches its Task 5 call. `ExtractView`/`ExtractTarget`/`StudyStats3` JSON shapes defined in Tasks 4-5 are consumed unchanged in Tasks 7-8. `PromoteExtract(id, kind)` / `DeferExtract(id)` / `DismissExtract(id)` signatures match between Task 5 (def) and Task 7 (calls). `RecallCard` props (`card`, `onGrade`) match between Task 6 (def) and Tasks 6/8 (use). `studyFocus` values `'next'|'read'|'process'|'recall'` consistent in Task 8. ✅

**Flagged for the implementer:** Task 2's `InjectMissingExtractIDs` reuses internal `writeback.go` helpers whose exact names/signatures must be confirmed against the real `InjectMissingIDs`; the test only asserts the observable effect (`# @id ` written, one injection), so the implementer adapts the internals to the real helpers.
