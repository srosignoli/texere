# Incremental Reading — Phase 1 (Recall Spine) Implementation Plan

**Goal:** Ship working spaced-repetition recall over cards authored in N4L today — the scheduling engine (`internal/learning/`) plus a Study/Recall UI — as the foundation the later reading/extract loop plugs into.

**Architecture:** A new pure-Go `internal/learning/` package owns a local SQLite database (`learning.db`) of study items and review logs, an FSRS scheduler, card generation from parsed `.n4l` files, and a sync pass that discovers items (assigning `# @id` via the existing `content.InjectMissingIDs`). Wails-bound `App` methods expose sync/queue/grade/stats to a new Svelte Study mode that shows a card front, reveals the back, and grades it. No source/extract layer in this phase.

**Tech Stack:** Go 1.24, `modernc.org/sqlite` (pure-Go SQLite, no CGo), `github.com/open-spaced-repetition/go-fsrs/v3` (pure-Go FSRS), Wails v2, Svelte 3.49. Reuses `internal/content` parser + writeback.

## Global Constraints

- All new dependencies MUST be pure Go (no CGo). SQLite driver = `modernc.org/sqlite`; FSRS = `github.com/open-spaced-repetition/go-fsrs/v3`. (CLAUDE.md code style + constitution.)
- `learning.db` lives at `<collectionDir>/learning.db`, is machine-local, and is gitignored (it already is — `.gitignore` excludes `learning.db`). It is NOT rebuildable: review history is earned.
- Study items are keyed by `# @id`; the engine assigns missing ids via `content.InjectMissingIDs` (which writes back to the `.n4l` file). Content stays the source of truth.
- Phase-1 card types only: **cloze** (from a node whose `StudyMeta.CardType == content.CardTypeCloze`) and **basic** (one forward card per studyable arrow on a node whose `StudyMeta != nil` and `CardType == content.CardTypeBasic`). Reverse and sequence cards are out of scope but the schema must not preclude them (a `card_type` column).
- Go: `gofmt`, imports grouped stdlib / external / internal. Go tests via `go test ./...`; frontend verified via `cd frontend && npm run build` + a manual quickstart matrix.

---

## File structure

| File | Responsibility |
|---|---|
| `internal/learning/types.go` | Shared types: `Grade`, `CardType`, `SchedState`, `GeneratedCard`, `Item`, `CardView`, `SyncResult`, `Stats`. |
| `internal/learning/db.go` | `Open`/`Close`, schema creation + versioned migration. |
| `internal/learning/store.go` | Item CRUD, due queue, review-log insert, stats. |
| `internal/learning/fsrs.go` | `Schedule(prev, grade, now)` wrapping go-fsrs. |
| `internal/learning/cards.go` | `GenerateCards(pf, reg)` — cloze + basic forward arrow. |
| `internal/learning/sync.go` | `Sync(store, files, reg)` — parse, inject ids, generate, upsert, retire. |
| `learning_methods.go` (repo root) | `App` Wails methods: `SyncStudyItems`, `GetDueQueue`, `GradeItem`, `GetStudyStats`. |
| `app.go` (modify) | Open/close the `*learning.Store` with the collection lifecycle. |
| `graph_methods.go` (modify) | Call `SyncStudyItems` after a successful `BuildGraph`. |
| `frontend/src/stores/study.js` | `studyOpen` store. |
| `frontend/src/components/StudyMode.svelte` | Recall UI overlay. |
| `frontend/src/App.svelte` (modify) | Ctrl+Shift+R toggles Study; render overlay. |
| `docs/Texere_Specification_v1.3.md` (modify) | Correct the "rebuildable" claim for `learning.db`. |

---

### Task 1: Learning store — types, DB open, schema & migration

**Files:**
- Create: `internal/learning/types.go`
- Create: `internal/learning/db.go`
- Test: `internal/learning/db_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Grade int` with `GradeAgain=1, GradeHard=2, GradeGood=3, GradeEasy=4`.
  - `type SchedState struct { Stability, Difficulty float64; Due, LastReview time.Time; Reps, Lapses, State int }` (zero value = a never-reviewed "new" card; `Due.IsZero()` means due now/new).
  - `type GeneratedCard struct { NodeID, FilePath, CardType, Front, Back, Discriminator string }` with method `func (g GeneratedCard) ItemID() string` = `NodeID + "::" + CardType + "::" + Discriminator`.
  - `type CardView struct { ID, Front, Back, CardType, FilePath, NodeID string }`.
  - `type SyncResult struct { Added, Retired, Total int }`.
  - `type Stats struct { Due, New, Total int }`.
  - `const CardTypeCloze = "cloze"`, `const CardTypeBasic = "basic"`.
  - `func Open(collectionDir string) (*Store, error)`, `func (s *Store) Close() error`. `Store` wraps `*sql.DB`.

- [ ] **Step 1: Add the pure-Go SQLite dependency**

Run: `cd /home/simone/projects/texere && go get modernc.org/sqlite@latest`
Expected: `go.mod` gains `modernc.org/sqlite`; no error.

- [ ] **Step 2: Write the failing test**

Create `internal/learning/db_test.go`:

```go
package learning

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesSchemaAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// learning.db file exists at <dir>/learning.db
	if _, err := s.db.Exec(`INSERT INTO items
		(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES ('n1::cloze::0','n1','/x.n4l','cloze','f','b',1,0,0,0,0)`); err != nil {
		t.Fatalf("items table not usable: %v", err)
	}
	var v int
	if err := s.db.QueryRow(`SELECT version FROM schema_meta`).Scan(&v); err != nil {
		t.Fatalf("schema_meta: %v", err)
	}
	if v != 1 {
		t.Fatalf("schema version = %d, want 1", v)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Re-open the same dir: must not error or duplicate-create.
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	var n int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&n); err != nil {
		t.Fatalf("count after reopen: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows after reopen = %d, want 1 (data preserved)", n)
	}
	_ = filepath.Join // keep import if unused elsewhere
	s2.Close()
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestOpenCreatesSchema -v`
Expected: FAIL — package/types don't exist (`undefined: Open`).

- [ ] **Step 4: Write `types.go`**

Create `internal/learning/types.go`:

```go
// Package learning owns the local SQLite scheduling state (learning.db) for
// Texere's spaced-repetition recall: study items derived from N4L cards, their
// FSRS scheduling state, and review history. learning.db is machine-local and
// NOT rebuildable (review history is earned), keyed by each node's # @id.
package learning

import "time"

// Grade is a four-point FSRS review rating.
type Grade int

const (
	GradeAgain Grade = 1
	GradeHard  Grade = 2
	GradeGood  Grade = 3
	GradeEasy  Grade = 4
)

// Card type discriminators stored in items.card_type. Phase 1 generates only
// these two; the column leaves room for "reverse"/"sequence" later.
const (
	CardTypeCloze = "cloze"
	CardTypeBasic = "basic"
)

// SchedState is the FSRS scheduling state of one item. The zero value is a
// never-reviewed "new" card (Due.IsZero()).
type SchedState struct {
	Stability  float64
	Difficulty float64
	Due        time.Time
	LastReview time.Time
	Reps       int
	Lapses     int
	State      int // FSRS state enum (0=New,1=Learning,2=Review,3=Relearning)
}

// GeneratedCard is one card produced from a parsed node, before persistence.
type GeneratedCard struct {
	NodeID        string
	FilePath      string
	CardType      string
	Front         string
	Back          string
	Discriminator string // unique within (NodeID, CardType): brace index or arrow index
}

// ItemID is the stable primary key of the persisted item.
func (g GeneratedCard) ItemID() string {
	return g.NodeID + "::" + g.CardType + "::" + g.Discriminator
}

// CardView is the front-end-facing shape of a due card.
type CardView struct {
	ID       string `json:"id"`
	Front    string `json:"front"`
	Back     string `json:"back"`
	CardType string `json:"cardType"`
	FilePath string `json:"filePath"`
	NodeID   string `json:"nodeId"`
}

// SyncResult summarizes one discovery pass.
type SyncResult struct {
	Added   int `json:"added"`
	Retired int `json:"retired"`
	Total   int `json:"total"`
}

// Stats are minimal counts for the Study entry badge.
type Stats struct {
	Due   int `json:"due"`
	New   int `json:"new"`
	Total int `json:"total"`
}
```

- [ ] **Step 5: Write `db.go`**

Create `internal/learning/db.go`:

```go
package learning

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers "sqlite")
)

// Store is the handle to one collection's learning.db.
type Store struct {
	db *sql.DB
}

const schemaVersion = 1

// Open opens (creating if needed) <collectionDir>/learning.db and migrates it
// to the current schema version.
func Open(collectionDir string) (*Store, error) {
	path := filepath.Join(collectionDir, "learning.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("learning: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // SQLite single-writer; avoids "database is locked"
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_meta (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("learning: schema_meta: %w", err)
	}
	var v int
	switch err := s.db.QueryRow(`SELECT version FROM schema_meta`).Scan(&v); err {
	case sql.ErrNoRows:
		v = 0
	case nil:
	default:
		return fmt.Errorf("learning: read version: %w", err)
	}
	if v >= schemaVersion {
		return nil
	}
	if _, err := s.db.Exec(ddlV1); err != nil {
		return fmt.Errorf("learning: apply v1: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM schema_meta`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO schema_meta(version) VALUES (?)`, schemaVersion); err != nil {
		return err
	}
	return nil
}

const ddlV1 = `
CREATE TABLE IF NOT EXISTS items (
  id           TEXT PRIMARY KEY,
  node_id      TEXT NOT NULL,
  file_path    TEXT NOT NULL,
  card_type    TEXT NOT NULL,
  front        TEXT NOT NULL,
  back         TEXT NOT NULL,
  active       INTEGER NOT NULL DEFAULT 1,
  stability    REAL,
  difficulty   REAL,
  due          INTEGER,
  last_review  INTEGER,
  reps         INTEGER NOT NULL DEFAULT 0,
  lapses       INTEGER NOT NULL DEFAULT 0,
  state        INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_items_due ON items(due) WHERE active = 1;
CREATE TABLE IF NOT EXISTS review_logs (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id       TEXT NOT NULL,
  reviewed_at   INTEGER NOT NULL,
  grade         INTEGER NOT NULL,
  elapsed_ms    INTEGER,
  old_state     INTEGER,
  new_stability REAL,
  card_type     TEXT
);
CREATE INDEX IF NOT EXISTS idx_logs_item ON review_logs(item_id);
`
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestOpenCreatesSchema -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/learning/types.go internal/learning/db.go internal/learning/db_test.go go.mod go.sum
git commit -m "feat(learning): SQLite store open + schema/migration (modernc sqlite)"
```

---

### Task 2: Store CRUD — upsert, retire, due queue, review log, stats

**Files:**
- Create: `internal/learning/store.go`
- Test: `internal/learning/store_test.go`

**Interfaces:**
- Consumes: `Store` (Task 1), `GeneratedCard`, `SchedState`, `CardView`, `Stats`, `Grade`.
- Produces (methods on `*Store`):
  - `UpsertItem(g GeneratedCard) (added bool, err error)` — insert new (active=1, zero sched, created_at=now); if it exists, update front/back and set active=1, **preserving** scheduling columns.
  - `RetireMissing(filePath string, keep map[string]bool) (retired int, err error)` — set active=0 for active items of `filePath` whose id ∉ keep.
  - `DueQueue(now time.Time, limit int) ([]CardView, error)` — active items with `due IS NULL OR due <= now`, due cards first (by due ASC) then new.
  - `LoadSched(itemID string) (SchedState, bool, error)`.
  - `ApplyReview(itemID string, st SchedState, grade Grade, elapsedMs int, oldState int, now time.Time) error` — write sched columns + insert a review_logs row.
  - `GetCardView(itemID string) (CardView, bool, error)`.
  - `CountStats(now time.Time) (Stats, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/learning/store_test.go`:

```go
package learning

import (
	"testing"
	"time"
)

func newCard(id string) GeneratedCard {
	return GeneratedCard{NodeID: id, FilePath: "/x.n4l", CardType: CardTypeCloze,
		Front: "f-" + id, Back: "b-" + id, Discriminator: "0"}
}

func TestUpsertRetireQueueReview(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_000_000, 0)

	// Upsert two cards.
	for _, id := range []string{"a", "b"} {
		added, err := s.UpsertItem(newCard(id))
		if err != nil || !added {
			t.Fatalf("UpsertItem(%s) added=%v err=%v", id, added, err)
		}
	}
	// Re-upsert "a" with new front: not added, front updated, still active.
	g := newCard("a")
	g.Front = "updated"
	if added, err := s.UpsertItem(g); err != nil || added {
		t.Fatalf("re-upsert added=%v err=%v want added=false", added, err)
	}

	// Both new cards are due (due IS NULL).
	q, err := s.DueQueue(now, 10)
	if err != nil || len(q) != 2 {
		t.Fatalf("DueQueue len=%d err=%v want 2", len(q), err)
	}

	// Retire "b" (no longer generated for /x.n4l).
	keep := map[string]bool{newCard("a").ItemID(): true}
	retired, err := s.RetireMissing("/x.n4l", keep)
	if err != nil || retired != 1 {
		t.Fatalf("RetireMissing retired=%d err=%v want 1", retired, err)
	}
	if q, _ := s.DueQueue(now, 10); len(q) != 1 {
		t.Fatalf("DueQueue after retire len=%d want 1", len(q))
	}

	// Review "a": schedule it into the future, it leaves the due queue.
	itemID := newCard("a").ItemID()
	st := SchedState{Stability: 5, Difficulty: 5, Due: now.Add(48 * time.Hour),
		LastReview: now, Reps: 1, State: 2}
	if err := s.ApplyReview(itemID, st, GradeGood, 1200, 0, now); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}
	if q, _ := s.DueQueue(now, 10); len(q) != 0 {
		t.Fatalf("DueQueue after review len=%d want 0", len(q))
	}
	// Loaded sched round-trips.
	got, ok, err := s.LoadSched(itemID)
	if err != nil || !ok || got.Reps != 1 || got.Due.Unix() != st.Due.Unix() {
		t.Fatalf("LoadSched got=%+v ok=%v err=%v", got, ok, err)
	}
	// Stats: 1 due (the future-due 'a' is not due; 'b' retired) -> due=0, total active=1.
	stats, err := s.CountStats(now)
	if err != nil || stats.Total != 1 || stats.Due != 0 {
		t.Fatalf("CountStats=%+v err=%v want Total=1 Due=0", stats, err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestUpsertRetireQueueReview -v`
Expected: FAIL — `s.UpsertItem undefined`.

- [ ] **Step 3: Write `store.go`**

Create `internal/learning/store.go`:

```go
package learning

import (
	"database/sql"
	"fmt"
	"time"
)

func nullableUnix(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}

func unixToTime(v sql.NullInt64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return time.Unix(v.Int64, 0)
}

// UpsertItem inserts a new item or updates front/back + reactivates an existing
// one, preserving its scheduling columns.
func (s *Store) UpsertItem(g GeneratedCard) (bool, error) {
	id := g.ItemID()
	var exists int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE id = ?`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("learning: upsert lookup: %w", err)
	}
	if exists > 0 {
		_, err := s.db.Exec(`UPDATE items SET front=?, back=?, file_path=?, active=1 WHERE id=?`,
			g.Front, g.Back, g.FilePath, id)
		return false, err
	}
	_, err = s.db.Exec(`INSERT INTO items
		(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES (?,?,?,?,?,?,1,0,0,0,?)`,
		id, g.NodeID, g.FilePath, g.CardType, g.Front, g.Back, time.Now().Unix())
	if err != nil {
		return false, fmt.Errorf("learning: insert item: %w", err)
	}
	return true, nil
}

// RetireMissing deactivates active items of filePath whose id is not in keep.
func (s *Store) RetireMissing(filePath string, keep map[string]bool) (int, error) {
	rows, err := s.db.Query(`SELECT id FROM items WHERE file_path=? AND active=1`, filePath)
	if err != nil {
		return 0, err
	}
	var toRetire []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		if !keep[id] {
			toRetire = append(toRetire, id)
		}
	}
	rows.Close()
	for _, id := range toRetire {
		if _, err := s.db.Exec(`UPDATE items SET active=0 WHERE id=?`, id); err != nil {
			return 0, err
		}
	}
	return len(toRetire), nil
}

// DueQueue returns active items that are due now (due<=now) or new (due NULL),
// due cards first by ascending due date, then new cards.
func (s *Store) DueQueue(now time.Time, limit int) ([]CardView, error) {
	rows, err := s.db.Query(`SELECT id, front, back, card_type, file_path, node_id
		FROM items
		WHERE active=1 AND (due IS NULL OR due <= ?)
		ORDER BY (due IS NULL), due ASC
		LIMIT ?`, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CardView
	for rows.Next() {
		var c CardView
		if err := rows.Scan(&c.ID, &c.Front, &c.Back, &c.CardType, &c.FilePath, &c.NodeID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// LoadSched returns the scheduling state for an item.
func (s *Store) LoadSched(itemID string) (SchedState, bool, error) {
	var (
		st                    SchedState
		stab, diff            sql.NullFloat64
		due, last             sql.NullInt64
	)
	err := s.db.QueryRow(`SELECT stability, difficulty, due, last_review, reps, lapses, state
		FROM items WHERE id=?`, itemID).
		Scan(&stab, &diff, &due, &last, &st.Reps, &st.Lapses, &st.State)
	if err == sql.ErrNoRows {
		return SchedState{}, false, nil
	}
	if err != nil {
		return SchedState{}, false, err
	}
	st.Stability = stab.Float64
	st.Difficulty = diff.Float64
	st.Due = unixToTime(due)
	st.LastReview = unixToTime(last)
	return st, true, nil
}

// ApplyReview writes the new scheduling state and appends a review log.
func (s *Store) ApplyReview(itemID string, st SchedState, grade Grade, elapsedMs, oldState int, now time.Time) error {
	if _, err := s.db.Exec(`UPDATE items
		SET stability=?, difficulty=?, due=?, last_review=?, reps=?, lapses=?, state=?
		WHERE id=?`,
		st.Stability, st.Difficulty, nullableUnix(st.Due), nullableUnix(st.LastReview),
		st.Reps, st.Lapses, st.State, itemID); err != nil {
		return fmt.Errorf("learning: apply review update: %w", err)
	}
	var cardType string
	_ = s.db.QueryRow(`SELECT card_type FROM items WHERE id=?`, itemID).Scan(&cardType)
	_, err := s.db.Exec(`INSERT INTO review_logs
		(item_id, reviewed_at, grade, elapsed_ms, old_state, new_stability, card_type)
		VALUES (?,?,?,?,?,?,?)`,
		itemID, now.Unix(), int(grade), elapsedMs, oldState, st.Stability, cardType)
	return err
}

// GetCardView fetches one item's view by id.
func (s *Store) GetCardView(itemID string) (CardView, bool, error) {
	var c CardView
	err := s.db.QueryRow(`SELECT id, front, back, card_type, file_path, node_id
		FROM items WHERE id=?`, itemID).
		Scan(&c.ID, &c.Front, &c.Back, &c.CardType, &c.FilePath, &c.NodeID)
	if err == sql.ErrNoRows {
		return CardView{}, false, nil
	}
	return c, err == nil, err
}

// CountStats returns due/new/total counts over active items.
func (s *Store) CountStats(now time.Time) (Stats, error) {
	var st Stats
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE active=1`).Scan(&st.Total); err != nil {
		return st, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE active=1 AND due IS NOT NULL AND due <= ?`,
		now.Unix()).Scan(&st.Due); err != nil {
		return st, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE active=1 AND due IS NULL`).Scan(&st.New); err != nil {
		return st, err
	}
	return st, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestUpsertRetireQueueReview -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/learning/store.go internal/learning/store_test.go
git commit -m "feat(learning): item CRUD, due queue, review logs, stats"
```

---

### Task 3: FSRS scheduler wrapper

**Files:**
- Create: `internal/learning/fsrs.go`
- Test: `internal/learning/fsrs_test.go`

**Interfaces:**
- Consumes: `SchedState`, `Grade`.
- Produces: `func Schedule(prev SchedState, grade Grade, now time.Time) (SchedState, error)` — applies FSRS to `prev` (zero = new card), returning the updated state with a future `Due`, `LastReview=now`, and incremented `Reps` (and `Lapses` on Again from a Review state).

**Note on the dependency:** verify the symbol names against the version `go get` pins. The go-fsrs v3 surface used below is: `fsrs.DefaultParam() Parameters`, `fsrs.NewCard() Card`, `Parameters.Repeat(card Card, now time.Time) map[Rating]SchedulingInfo`, `SchedulingInfo{Card, ReviewLog}`, ratings `fsrs.Again/Hard/Good/Easy`, and `Card{Due, Stability, Difficulty, Reps, Lapses, State, LastReview}`. If the pinned version differs, adapt this thin wrapper — **the test below is the behavioral contract that must keep passing.**

- [ ] **Step 1: Add the dependency**

Run: `cd /home/simone/projects/texere && go get github.com/open-spaced-repetition/go-fsrs/v3@latest`
Expected: `go.mod` gains the module; no error.

- [ ] **Step 2: Write the failing test**

Create `internal/learning/fsrs_test.go`:

```go
package learning

import (
	"testing"
	"time"
)

func TestScheduleNewCardGoodIsDueInFuture(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	out, err := Schedule(SchedState{}, GradeGood, now)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if !out.Due.After(now) {
		t.Fatalf("Due=%v not after now=%v", out.Due, now)
	}
	if out.Reps != 1 {
		t.Fatalf("Reps=%d want 1", out.Reps)
	}
	if out.LastReview.Unix() != now.Unix() {
		t.Fatalf("LastReview=%v want %v", out.LastReview, now)
	}
	if out.Stability <= 0 {
		t.Fatalf("Stability=%v want > 0", out.Stability)
	}
}

func TestScheduleAgainShorterThanEasy(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	again, _ := Schedule(SchedState{}, GradeAgain, now)
	easy, _ := Schedule(SchedState{}, GradeEasy, now)
	if !again.Due.Before(easy.Due) {
		t.Fatalf("Again due %v should be before Easy due %v", again.Due, easy.Due)
	}
}
```

- [ ] **Step 3: Write `fsrs.go`**

Create `internal/learning/fsrs.go`:

```go
package learning

import (
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

// gradeToRating maps our Grade to the library's Rating.
func gradeToRating(g Grade) fsrs.Rating {
	switch g {
	case GradeAgain:
		return fsrs.Again
	case GradeHard:
		return fsrs.Hard
	case GradeEasy:
		return fsrs.Easy
	default:
		return fsrs.Good
	}
}

// Schedule applies FSRS to prev (the zero value is a new card) and returns the
// updated scheduling state.
func Schedule(prev SchedState, grade Grade, now time.Time) (SchedState, error) {
	card := fsrs.NewCard()
	if prev.Reps > 0 || !prev.Due.IsZero() {
		card.Due = prev.Due
		card.Stability = prev.Stability
		card.Difficulty = prev.Difficulty
		card.Reps = uint64(prev.Reps)
		card.Lapses = uint64(prev.Lapses)
		card.State = fsrs.State(prev.State)
		card.LastReview = prev.LastReview
	}
	params := fsrs.DefaultParam()
	info := params.Repeat(card, now)[gradeToRating(grade)]
	c := info.Card
	return SchedState{
		Stability:  c.Stability,
		Difficulty: c.Difficulty,
		Due:        c.Due,
		LastReview: now,
		Reps:       int(c.Reps),
		Lapses:     int(c.Lapses),
		State:      int(c.State),
	}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestSchedule -v`
Expected: PASS (both). If a symbol is undefined, adjust the wrapper to the pinned version's API; the assertions must still hold.

- [ ] **Step 5: Commit**

```bash
git add internal/learning/fsrs.go internal/learning/fsrs_test.go go.mod go.sum
git commit -m "feat(learning): FSRS scheduler wrapper (go-fsrs v3)"
```

---

### Task 4: Card generation from a parsed file

**Files:**
- Create: `internal/learning/cards.go`
- Test: `internal/learning/cards_test.go`

**Interfaces:**
- Consumes: `content.ParsedFile`, `content.SSTRegistry`, `GeneratedCard`.
- Produces: `func GenerateCards(pf *content.ParsedFile, reg *content.SSTRegistry) []GeneratedCard`.
  - **Cloze:** for each node with `StudyMeta != nil && StudyMeta.CardType == content.CardTypeCloze`, scan `node.Text` for `{...}` groups in order; emit one card per group: `Front` = text with that group rendered `[…]` and every other group's braces stripped; `Back` = text with all braces stripped; `Discriminator` = the group index ("0","1",…). Skips nodes with no `# @id` (cannot be keyed).
  - **Basic:** for each node with `StudyMeta != nil && StudyMeta.CardType == content.CardTypeBasic`, emit one card per arrow in `node.Arrows` whose registered `ArrowDef.Studyable` is true: `Front` = `node.Text + " — " + def.LongName + "?"`, `Back` = `arrow.Target`, `Discriminator` = the arrow index. Arrows with no registry entry or `Studyable==false` are skipped.

- [ ] **Step 1: Write the failing test**

Create `internal/learning/cards_test.go`:

```go
package learning

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/content"
)

func writeFixture(t *testing.T, body string) (string, *content.SSTRegistry) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{
		"means": {Code: "means", LongName: "means", Studyable: true, MetaType: "properties"},
		"note":  {Code: "note", LongName: "note", Studyable: false},
	}}
	return p, reg
}

func TestGenerateClozeAndBasicCards(t *testing.T) {
	// Cloze node + a basic node with one studyable and one non-studyable arrow.
	body := "- Ch\n" +
		"# @id n1\n" +
		"# @cloze\n" +
		"System 1 is {fast} and {automatic}\n" +
		"# @id n2\n" +
		"# @card\n" +
		"System 2 (means) effortful (note) hint\n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	got := GenerateCards(pf, reg)

	// Expect 2 cloze cards (fast, automatic) + 1 basic card (means -> effortful).
	if len(got) != 3 {
		t.Fatalf("got %d cards, want 3: %+v", len(got), got)
	}
	var cloze0 *GeneratedCard
	var basic *GeneratedCard
	for i := range got {
		if got[i].NodeID == "n1" && got[i].Discriminator == "0" {
			cloze0 = &got[i]
		}
		if got[i].NodeID == "n2" && got[i].CardType == CardTypeBasic {
			basic = &got[i]
		}
	}
	if cloze0 == nil {
		t.Fatalf("missing cloze card 0")
	}
	if cloze0.Front != "System 1 is […] and automatic" || cloze0.Back != "System 1 is fast and automatic" {
		t.Fatalf("cloze0 front=%q back=%q", cloze0.Front, cloze0.Back)
	}
	if basic == nil {
		t.Fatalf("missing basic card")
	}
	if basic.Front != "System 2 — means?" || basic.Back != "effortful" {
		t.Fatalf("basic front=%q back=%q", basic.Front, basic.Back)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestGenerateClozeAndBasic -v`
Expected: FAIL — `undefined: GenerateCards`.

- [ ] **Step 3: Write `cards.go`**

Create `internal/learning/cards.go`:

```go
package learning

import (
	"strconv"
	"strings"

	"texere/internal/content"
)

// GenerateCards derives Phase-1 study cards (cloze + basic forward arrow) from a
// parsed file. Nodes without a # @id are skipped (they cannot be keyed).
func GenerateCards(pf *content.ParsedFile, reg *content.SSTRegistry) []GeneratedCard {
	var out []GeneratedCard
	if pf == nil {
		return out
	}
	for _, ch := range pf.Chapters {
		for _, node := range ch.Nodes {
			if node.StudyMeta == nil || node.ID == "" {
				continue
			}
			switch node.StudyMeta.CardType {
			case content.CardTypeCloze:
				out = append(out, clozeCards(node.ID, pf.Path, node.Text)...)
			case content.CardTypeBasic:
				out = append(out, basicCards(node.ID, pf.Path, node.Text, node.Arrows, reg)...)
			}
		}
	}
	return out
}

// braceGroups returns the [start,end) byte ranges of each {...} group, in order.
func braceGroups(text string) [][2]int {
	var groups [][2]int
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		if j := strings.IndexByte(text[i:], '}'); j >= 0 {
			groups = append(groups, [2]int{i, i + j + 1})
			i += j
		}
	}
	return groups
}

// stripBraces removes every { and } from text (the "back" rendering).
func stripBraces(text string) string {
	r := strings.NewReplacer("{", "", "}", "")
	return r.Replace(text)
}

func clozeCards(nodeID, filePath, text string) []GeneratedCard {
	groups := braceGroups(text)
	var out []GeneratedCard
	for idx, g := range groups {
		// Build the front: the chosen group becomes […], others lose their braces.
		var b strings.Builder
		last := 0
		for k, gg := range groups {
			b.WriteString(stripBraces(text[last:gg[0]]))
			if k == idx {
				b.WriteString("[…]")
			} else {
				b.WriteString(stripBraces(text[gg[0]:gg[1]]))
			}
			last = gg[1]
		}
		b.WriteString(stripBraces(text[last:]))
		out = append(out, GeneratedCard{
			NodeID: nodeID, FilePath: filePath, CardType: CardTypeCloze,
			Front: b.String(), Back: stripBraces(text), Discriminator: strconv.Itoa(idx),
		})
		_ = g
	}
	return out
}

func basicCards(nodeID, filePath, text string, arrows []content.Arrow, reg *content.SSTRegistry) []GeneratedCard {
	var out []GeneratedCard
	for i, a := range arrows {
		def, ok := reg.Arrows[a.Code]
		if !ok || !def.Studyable {
			continue
		}
		out = append(out, GeneratedCard{
			NodeID: nodeID, FilePath: filePath, CardType: CardTypeBasic,
			Front: text + " — " + def.LongName + "?", Back: a.Target,
			Discriminator: strconv.Itoa(i),
		})
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestGenerateClozeAndBasic -v`
Expected: PASS.

> If the parser does not populate `StudyMeta.CardType` exactly as assumed for `# @cloze`/`# @card` on these fixtures, read `internal/content/parser.go` around the `@cloze`/`@card` handling to confirm the enum values, and adjust only the test's expectations or the `switch` — do not change the parser. Report as a concern if the parser's behavior contradicts the spec.

- [ ] **Step 5: Commit**

```bash
git add internal/learning/cards.go internal/learning/cards_test.go
git commit -m "feat(learning): cloze + basic-arrow card generation from parsed files"
```

---

### Task 5: Sync — discover, assign ids, upsert, retire

**Files:**
- Create: `internal/learning/sync.go`
- Test: `internal/learning/sync_test.go`

**Interfaces:**
- Consumes: `Store` (Tasks 1-2), `GenerateCards` (Task 4), `content.ParseFile`, `content.InjectMissingIDs`, `content.SSTRegistry`, `SyncResult`.
- Produces: `func Sync(store *Store, files []string, reg *content.SSTRegistry) (SyncResult, error)` — for each file: parse, `InjectMissingIDs` (writes `# @id` back to disk and mutates the parsed file), generate cards, `UpsertItem` each, `RetireMissing` for ids no longer present. Accumulates `Added`/`Retired`; `Total` = active item count after the pass.

- [ ] **Step 1: Write the failing test**

Create `internal/learning/sync_test.go`:

```go
package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"texere/internal/content"
)

func TestSyncAssignsIDsUpsertsAndRetires(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	// A cloze node with NO # @id — Sync must inject one and create a card.
	body := "- Ch\n# @cloze\nSystem 1 is {fast}\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	res, err := Sync(store, []string{p}, reg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Added != 1 || res.Total != 1 {
		t.Fatalf("res=%+v want Added=1 Total=1", res)
	}
	// The file now contains an injected "# @id".
	after, _ := os.ReadFile(p)
	if !strings.Contains(string(after), "# @id ") {
		t.Fatalf("expected an injected # @id in file:\n%s", after)
	}
	// Re-syncing the same file adds nothing.
	res2, _ := Sync(store, []string{p}, reg)
	if res2.Added != 0 || res2.Total != 1 {
		t.Fatalf("res2=%+v want Added=0 Total=1", res2)
	}
	// Removing the cloze marker retires the card on next sync.
	if err := os.WriteFile(p, []byte("- Ch\nSystem 1 is fast\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res3, _ := Sync(store, []string{p}, reg)
	if res3.Retired != 1 {
		t.Fatalf("res3=%+v want Retired=1", res3)
	}
	if st, _ := store.CountStats(time.Now()); st.Total != 0 {
		t.Fatalf("active total=%d want 0 after retire", st.Total)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestSyncAssigns -v`
Expected: FAIL — `undefined: Sync`.

- [ ] **Step 3: Write `sync.go`**

Create `internal/learning/sync.go`:

```go
package learning

import (
	"time"

	"texere/internal/content"
)

// Sync discovers study items across the given files, assigning # @id markers to
// study nodes that lack them (written back to disk), and reconciles the store:
// new cards are inserted, cards no longer generated for a file are retired.
func Sync(store *Store, files []string, reg *content.SSTRegistry) (SyncResult, error) {
	var res SyncResult
	for _, path := range files {
		pf, _ := content.ParseFile(path, reg)
		if pf == nil {
			continue
		}
		// Assign # @id to study-marked nodes missing one (mutates pf in place).
		if _, err := content.InjectMissingIDs(pf); err != nil {
			return res, err
		}
		cards := GenerateCards(pf, reg)
		keep := make(map[string]bool, len(cards))
		for _, c := range cards {
			added, err := store.UpsertItem(c)
			if err != nil {
				return res, err
			}
			if added {
				res.Added++
			}
			keep[c.ItemID()] = true
		}
		retired, err := store.RetireMissing(path, keep)
		if err != nil {
			return res, err
		}
		res.Retired += retired
	}
	st, err := store.CountStats(time.Now())
	if err != nil {
		return res, err
	}
	res.Total = st.Total
	return res, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -run TestSyncAssigns -v`
Expected: PASS.

- [ ] **Step 5: Run the whole package and commit**

Run: `cd /home/simone/projects/texere && go test ./internal/learning/ -v`
Expected: all PASS.

```bash
git add internal/learning/sync.go internal/learning/sync_test.go
git commit -m "feat(learning): sync — discover, assign # @id, upsert, retire"
```

---

### Task 6: Wails bindings + App lifecycle + Build Graph hook

**Files:**
- Create: `learning_methods.go` (repo root)
- Modify: `app.go` (add `learnStore` field; open on startup/OpenCollection; close on shutdown)
- Modify: `graph_methods.go` (call sync after a successful build)
- Test: `learning_methods_test.go` (repo root)

**Interfaces:**
- Consumes: `learning.Open/Sync/DueQueue/LoadSched/Schedule/ApplyReview/GetCardView/CountStats`, `content.ScanCollection`.
- Produces (App methods, Wails-bound):
  - `func (a *App) SyncStudyItems() (*learning.SyncResult, error)`
  - `func (a *App) GetDueQueue(limit int) ([]learning.CardView, error)`
  - `func (a *App) GradeItem(itemID string, grade int, elapsedMs int) (*learning.CardView, error)` — returns the next due card, or `nil` when the queue is empty.
  - `func (a *App) GetStudyStats() (*learning.Stats, error)`

- [ ] **Step 1: Add the store to `App` and its lifecycle**

In `app.go`: add a field to the `App` struct (next to `searchSvc`):

```go
	learnStore    *learning.Store
```

Add the import `"texere/internal/learning"` to `app.go`'s internal import group.

In `startup` (after `a.collectionDir`/`a.searchSvc` are set for the auto-loaded collection) and in `OpenCollection` (after `a.collectionDir = dir`), open the store, replacing any previous one:

```go
	if a.learnStore != nil {
		a.learnStore.Close()
		a.learnStore = nil
	}
	if a.collectionDir != "" {
		if ls, err := learning.Open(a.collectionDir); err != nil {
			log.Printf("learning: open store: %v", err)
		} else {
			a.learnStore = ls
		}
	}
```

In `shutdown`, close it:

```go
	if a.learnStore != nil {
		a.learnStore.Close()
	}
```

> Read the exact surrounding lines in `app.go` for `startup`, `OpenCollection`, and `shutdown` and place these blocks where `collectionDir` is already known. Use the existing `log` import.

- [ ] **Step 2: Write the failing binding test**

Create `learning_methods_test.go` at the repo root:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/content"
	"texere/internal/learning"
)

func TestGradeItemAdvancesQueue(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	if err := os.WriteFile(p, []byte("- Ch\n# @cloze\nSystem 1 is {fast}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{collectionDir: dir, learnStore: store, registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}}
	defer store.Close()

	if _, err := a.SyncStudyItems(); err != nil {
		t.Fatalf("SyncStudyItems: %v", err)
	}
	q, err := a.GetDueQueue(10)
	if err != nil || len(q) != 1 {
		t.Fatalf("GetDueQueue len=%d err=%v want 1", len(q), err)
	}
	// Grading the only due card returns nil (queue now empty).
	next, err := a.GradeItem(q[0].ID, int(learning.GradeGood), 1000)
	if err != nil {
		t.Fatalf("GradeItem: %v", err)
	}
	if next != nil {
		t.Fatalf("next=%+v want nil (no more due)", next)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /home/simone/projects/texere && go test . -run TestGradeItemAdvancesQueue -v`
Expected: FAIL — `a.SyncStudyItems undefined`.

- [ ] **Step 4: Write `learning_methods.go`**

Create `learning_methods.go` at the repo root:

```go
package main

import (
	"errors"
	"time"

	"texere/internal/content"
	"texere/internal/learning"
)

var errNoStore = errors.New("no learning store (open a collection first)")

// collectionN4LFiles lists the .n4l files of the open collection.
func (a *App) collectionN4LFiles() []string {
	sums, err := content.ScanCollection(a.collectionDir)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(sums))
	for _, s := range sums {
		if s.FilePath != "" && s.ScanError == nil {
			files = append(files, s.FilePath)
		}
	}
	return files
}

// SyncStudyItems discovers study items across the collection into learning.db.
func (a *App) SyncStudyItems() (*learning.SyncResult, error) {
	if a.learnStore == nil {
		return nil, errNoStore
	}
	res, err := learning.Sync(a.learnStore, a.collectionN4LFiles(), a.registry)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// GetDueQueue returns up to limit due/new cards.
func (a *App) GetDueQueue(limit int) ([]learning.CardView, error) {
	if a.learnStore == nil {
		return nil, errNoStore
	}
	if limit <= 0 {
		limit = 50
	}
	return a.learnStore.DueQueue(time.Now(), limit)
}

// GradeItem applies an FSRS grade to one item and returns the next due card
// (nil when the queue is empty).
func (a *App) GradeItem(itemID string, grade int, elapsedMs int) (*learning.CardView, error) {
	if a.learnStore == nil {
		return nil, errNoStore
	}
	now := time.Now()
	prev, ok, err := a.learnStore.LoadSched(itemID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("item not found: " + itemID)
	}
	next, err := learning.Schedule(prev, learning.Grade(grade), now)
	if err != nil {
		return nil, err
	}
	if err := a.learnStore.ApplyReview(itemID, next, learning.Grade(grade), elapsedMs, prev.State, now); err != nil {
		return nil, err
	}
	q, err := a.learnStore.DueQueue(now, 1)
	if err != nil {
		return nil, err
	}
	if len(q) == 0 {
		return nil, nil
	}
	return &q[0], nil
}

// GetStudyStats returns due/new/total counts for the Study entry badge.
func (a *App) GetStudyStats() (*learning.Stats, error) {
	if a.learnStore == nil {
		return nil, errNoStore
	}
	st, err := a.learnStore.CountStats(time.Now())
	if err != nil {
		return nil, err
	}
	return &st, nil
}
```

- [ ] **Step 5: Hook sync into Build Graph**

In `graph_methods.go`, at the end of `BuildGraph` (after `a.searchSvc`/`explorerSvc` are refreshed and just before `return result`), add:

```go
	if a.learnStore != nil {
		if _, err := learning.Sync(a.learnStore, a.collectionN4LFiles(), a.registry); err != nil {
			log.Printf("learning: sync after build: %v", err)
		}
	}
```

Add `"texere/internal/learning"` and ensure `log` are imported in `graph_methods.go`.

- [ ] **Step 6: Run the test + full build**

Run: `cd /home/simone/projects/texere && go test . -run TestGradeItemAdvancesQueue -v && go build ./...`
Expected: PASS and a clean build.

- [ ] **Step 7: Regenerate Wails bindings**

Run: `cd /home/simone/projects/texere && wails generate module`
Expected: `frontend/wailsjs/go/main/App.*` now include `SyncStudyItems`, `GetDueQueue`, `GradeItem`, `GetStudyStats`. (If `wails` is unavailable in this environment, note it — the frontend task references these bindings; they are generated at build time by `make build`/`wails dev`.)

- [ ] **Step 8: Commit**

```bash
git add learning_methods.go learning_methods_test.go app.go graph_methods.go frontend/wailsjs/
git commit -m "feat(learning): Wails bindings, store lifecycle, Build Graph sync hook"
```

---

### Task 7: Study mode UI (Recall face)

**Files:**
- Create: `frontend/src/stores/study.js`
- Create: `frontend/src/components/StudyMode.svelte`
- Modify: `frontend/src/App.svelte` (Ctrl+Shift+R toggles study; render overlay)

**Interfaces:**
- Consumes: bindings `GetDueQueue`, `GradeItem`, `GetStudyStats`, `SyncStudyItems` from `../../wailsjs/go/main/App`.
- Produces: a `studyOpen` writable store and a `StudyMode` overlay component.

- [ ] **Step 1: Create the store**

Create `frontend/src/stores/study.js`:

```js
import { writable } from 'svelte/store'

// studyOpen — true while the Recall (Study) overlay is mounted.
export const studyOpen = writable(false)
```

- [ ] **Step 2: Create the component**

Create `frontend/src/components/StudyMode.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import { SyncStudyItems, GetDueQueue, GradeItem } from '../../wailsjs/go/main/App'

  export let onClose = null

  let card = null        // current CardView or null
  let revealed = false
  let loading = true
  let error = ''
  let remaining = 0
  let shownAt = 0

  const GRADES = [
    { n: 1, label: 'Again' },
    { n: 2, label: 'Hard' },
    { n: 3, label: 'Good' },
    { n: 4, label: 'Easy' },
  ]

  async function load() {
    loading = true
    error = ''
    try {
      await SyncStudyItems()
      const q = await GetDueQueue(50)
      remaining = q?.length ?? 0
      card = q && q.length ? q[0] : null
    } catch (e) {
      error = String(e?.message ?? e)
    } finally {
      loading = false
      revealed = false
      shownAt = Date.now()
    }
  }

  async function grade(n) {
    if (!card) return
    const elapsed = Date.now() - shownAt
    try {
      const next = await GradeItem(card.id, n, elapsed)
      card = next ?? null
      remaining = card ? Math.max(0, remaining - 1) : 0
    } catch (e) {
      error = String(e?.message ?? e)
    }
    revealed = false
    shownAt = Date.now()
  }

  function onKey(e) {
    if (e.key === 'Escape') { onClose?.(); return }
    if (!card) return
    if (!revealed && (e.key === ' ' || e.key === 'Enter')) {
      e.preventDefault(); revealed = true; return
    }
    if (revealed && e.key >= '1' && e.key <= '4') {
      e.preventDefault(); grade(parseInt(e.key, 10))
    }
  }

  onMount(load)
</script>

<svelte:window on:keydown={onKey} />

<div class="study-root" role="dialog" aria-label="Study">
  <div class="study-bar">
    <span class="study-title">Recall</span>
    <span class="study-count">{remaining} due</span>
    <button class="study-close" on:click={() => onClose?.()} title="Close (Esc)">×</button>
  </div>

  {#if loading}
    <div class="study-center"><p>Loading…</p></div>
  {:else if error}
    <div class="study-center"><p class="study-error">{error}</p></div>
  {:else if !card}
    <div class="study-center"><p>Nothing due. 🎉</p></div>
  {:else}
    <div class="study-center">
      <div class="study-card">
        <div class="study-front">{card.front}</div>
        {#if revealed}
          <hr />
          <div class="study-back">{card.back}</div>
        {/if}
      </div>
      {#if !revealed}
        <button class="study-reveal" on:click={() => (revealed = true)}>Show answer (Space)</button>
      {:else}
        <div class="study-grades">
          {#each GRADES as g}
            <button class="study-grade" on:click={() => grade(g.n)}>{g.label} ({g.n})</button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .study-root {
    position: absolute; inset: 0; z-index: 60;
    display: flex; flex-direction: column;
    background: #131c29;
  }
  .study-bar {
    flex: 0 0 auto; display: flex; align-items: center; gap: 12px;
    padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #161f2e;
  }
  .study-title { color: #7dd3fc; font-weight: 600; }
  .study-count { color: #8aa; font-size: 12px; }
  .study-close {
    margin-left: auto; background: none; border: none; color: #8899aa;
    font-size: 20px; line-height: 1; cursor: pointer;
  }
  .study-close:hover { color: #e8eaf0; }
  .study-center { flex: 1 1 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 18px; padding: 24px; }
  .study-card { max-width: 640px; width: 100%; border: 1px solid #2a3a4a; border-radius: 8px; padding: 28px; background: #1b2636; text-align: center; }
  .study-front { color: #e8eaf0; font-size: 16px; }
  .study-back { color: #7ecfa0; font-size: 15px; }
  .study-card hr { border: none; border-top: 1px solid #2a3a4a; margin: 16px 0; }
  .study-reveal, .study-grade {
    background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a;
    border-radius: 6px; padding: 8px 14px; cursor: pointer; font-size: 13px;
  }
  .study-reveal:hover, .study-grade:hover { background: #244a72; }
  .study-grades { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; }
  .study-error { color: #ff8080; }
</style>
```

- [ ] **Step 3: Wire it into `App.svelte`**

In `frontend/src/App.svelte`:

1. Add to the imports:

```js
  import StudyMode from './components/StudyMode.svelte'
  import { studyOpen } from './stores/study.js'
```

2. In the global `handleKeydown` (the capture-phase handler registered in `onMount`), add a branch alongside the other global shortcuts (e.g. near the Ctrl+G handling) — Ctrl+Shift+R toggles study:

```js
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'r' || e.key === 'R')) {
      e.preventDefault()
      e.stopPropagation()
      studyOpen.update((v) => !v)
      return
    }
```

3. In the template, alongside the other overlays (near `{#if $explorerOpen}`), add:

```svelte
    {#if $studyOpen}
      <StudyMode onClose={() => studyOpen.set(false)} />
    {/if}
```

> Read `App.svelte`'s `handleKeydown` and overlay region first; match the existing placement/parenthesis style for the `collectionOpen` guard so Study only opens when a collection is loaded.

- [ ] **Step 4: Verify the build**

Run: `cd /home/simone/projects/texere/frontend && npm run build`
Expected: exit 0, no new warnings.

- [ ] **Step 5: Manual verification (quickstart)**

Run `make dev`. With a collection open:
1. In a `.n4l` file add a cloze: a line `# @cloze` then `System 1 is {fast}`. Save.
2. Press **Ctrl+G** (Build Graph) — this also syncs study items; the file gains an injected `# @id`.
3. Press **Ctrl+Shift+R** — the Recall overlay opens showing `System 1 is […]`.
4. Press **Space** → the back (`System 1 is fast`) reveals. Press **3** (Good) → card leaves the queue; "0 due" / "Nothing due" appears.
5. Press **Esc** → overlay closes.

Expected: all steps behave as described.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/stores/study.js frontend/src/components/StudyMode.svelte frontend/src/App.svelte
git commit -m "feat(study): Recall overlay (Ctrl+Shift+R) — front/reveal/grade"
```

---

### Task 8: Correct the spec's "rebuildable" claim

**Files:**
- Modify: `docs/Texere_Specification_v1.3.md`

**Interfaces:** none (docs only).

- [ ] **Step 1: Fix the claim**

In `docs/Texere_Specification_v1.3.md`, find §5.7 (the version-control layer) where it says the `badger/` directory and `learning.db` are gitignored "because they are binary, machine-specific, and rebuildable." Change the wording so it no longer claims `learning.db` is rebuildable, e.g.:

> The `badger/` directory is .gitignored because it is binary, machine-specific, and **rebuildable** from the `.n4l` files. `learning.db` is also .gitignored because it is binary and machine-specific, **but it is NOT rebuildable** — it holds earned review history; back it up with the collection if you want to preserve scheduling progress.

Keep the surrounding sentences intact; change only the rebuildable claim as it applies to `learning.db`.

- [ ] **Step 2: Commit**

```bash
git add docs/Texere_Specification_v1.3.md
git commit -m "docs: learning.db is machine-local and not rebuildable (review history is earned)"
```

---

## Self-Review

**Spec coverage (design §s → tasks):**
- §3 deps pinned (pure-Go FSRS + modernc SQLite) → Tasks 1, 3. ✅
- §4.1 three element kinds — Phase 1 implements **items** only → Tasks 4-7; reading/extract explicitly deferred. ✅
- §4.3 state boundary (learning.db local, keyed by `# @id`) → Tasks 1, 5 (InjectMissingIDs). ✅
- §5.2 scheduling: items FSRS → Task 3; extract/reading scheduling deferred (Phase 2). ✅
- §7.1 card subset (cloze + basic forward arrow), reverse/sequence deferred but `card_type` column present → Task 4 + schema Task 1. ✅
- §7.1 item lifecycle (discover, ensure `# @id`, upsert, retire keep logs) → Task 5; retirement keeps logs (RetireMissing sets active=0, never deletes review_logs). ✅
- §7.2 schema (items, review_logs, schema_meta) → Task 1. ✅
- §7.3 bindings (SyncStudyItems/GetDueQueue/GradeItem/GetStudyStats) → Task 6. ✅
- §7.4 FSRS behind a small interface → Task 3 (`Schedule`). ✅
- §7.1 Recall UI (front→reveal→4 grades), discovery on Build Graph → Tasks 7, 6 (Build Graph hook). ✅
- §9 error/edge cases: DB open failure logged not fatal (Task 1/6); node edited/deleted → retire keeps logs, reappearing id reactivates with prior sched (Task 2 UpsertItem preserves sched on existing rows) ✅; cloze with no braces → no card (Task 4 `braceGroups` empty) ✅; new item due=NULL surfaced after due (Task 2 DueQueue ordering) ✅.
- §3 spec correction (rebuildable) → Task 8. ✅
- `# @id` writeback vs dirty editor buffer (design §9): **partially** addressed — `InjectMissingIDs` uses the existing atomic writeback, but this plan does not add the "defer if the open buffer is dirty" guard. Noted as a Phase-1 limitation: sync runs on Build Graph (an intentional act), and the existing watcher reconciles the editor; the dirty-buffer guard is a follow-up if it proves to bite. (Not a silent gap — recorded here.)

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. The one external-API caveat (go-fsrs symbols) is explicit with a behavioral test as the contract. ✅

**Type consistency:** `GeneratedCard.ItemID()` format `NodeID::CardType::Discriminator` is defined in Task 1 and used in Tasks 2 (keys), 4 (generation), 5 (keep-set). `SchedState` fields are identical across Tasks 1, 2, 3, 6. `CardView` JSON shape is identical in Tasks 1, 2, 6, 7. Method signatures in the Task 2 "Produces" block match their call sites in Tasks 5-6. `Schedule(prev, grade, now)` signature matches Task 6's call. ✅
