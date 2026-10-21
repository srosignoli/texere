package learning

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- rehome: the migration's brain, tested directly -----------------------

func TestRehomeStripsTheCurrentPrefix(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "reading", "x.n4l"), "x")
	rel, ok := rehome(filepath.Join(dir, "reading", "x.n4l"), dir)
	if !ok || rel != "reading/x.n4l" {
		t.Fatalf("rehome = %q, %v", rel, ok)
	}
}

// The case this feature exists for: the database came from another location.
func TestRehomeRecoversAMovedCollection(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "reading", "x.n4l"), "x")
	rel, ok := rehome("/somewhere/else/entirely/reading/x.n4l", dir)
	if !ok || rel != "reading/x.n4l" {
		t.Fatalf("rehome = %q, %v", rel, ok)
	}
}

func TestRehomeLeavesAnAlreadyRelativeValue(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "reading", "x.n4l"), "x")
	rel, ok := rehome("reading/x.n4l", dir)
	if !ok || rel != "reading/x.n4l" {
		t.Fatalf("rehome = %q, %v", rel, ok)
	}
}

func TestRehomeGivesUpWhenNothingMatches(t *testing.T) {
	dir := t.TempDir()
	if _, ok := rehome("/old/place/gone.n4l", dir); ok {
		t.Fatal("rehome claimed to resolve a file that does not exist")
	}
}

// Longest suffix, not first basename match: a bare basename search would pick
// the wrong file when the name repeats at two depths.
func TestRehomePrefersTheLongestMatchingSuffix(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "notes.n4l"), "shallow")
	mustWrite(t, filepath.Join(dir, "reading", "notes.n4l"), "deep")
	rel, ok := rehome("/old/place/reading/notes.n4l", dir)
	if !ok || rel != "reading/notes.n4l" {
		t.Fatalf("rehome = %q, %v — picked the wrong depth", rel, ok)
	}
}

// A single-segment value has only one tail to try — itself — and no
// shortening beyond it.
func TestRehomeResolvesASingleSegmentValue(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "x.n4l"), "x")
	rel, ok := rehome("x.n4l", dir)
	if !ok || rel != "x.n4l" {
		t.Fatalf("rehome = %q, %v", rel, ok)
	}
}

// The explicit empty-string guard in rehome, exercised directly rather than
// left to "correct by inspection".
func TestRehomeGivesUpOnAnEmptyValue(t *testing.T) {
	dir := t.TempDir()
	if rel, ok := rehome("", dir); ok {
		t.Fatalf("rehome resolved an empty value: %q", rel)
	}
}

// --- Store.Rel / Store.Resolve --------------------------------------------

func TestStoreRelAndResolveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "reading", "x.n4l"), "x")
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	abs := filepath.Join(dir, "reading", "x.n4l")
	rel, err := s.rel(abs)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	if rel != "reading/x.n4l" {
		t.Fatalf("Rel(%q) = %q, want %q", abs, rel, "reading/x.n4l")
	}
	got, err := s.resolve(rel)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != abs {
		t.Fatalf("Resolve(%q) = %q, want %q", rel, got, abs)
	}
}

func TestStoreResolveRefusesEscape(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if _, err := s.resolve("../escape.n4l"); err == nil {
		t.Fatal("Resolve accepted a path escaping the collection")
	}
}

// --- The migration itself, over a real database ----------------------------

// openAtSchemaSix creates <dir>/learning.db, applies migrations through v6
// (one step short of the relative-path change under test), and stamps
// schema_meta at version 6. It returns the raw *sql.DB so the caller can seed
// rows before triggering the v7 migration via Open. The caller must Close it
// before calling Open(dir) again.
func openAtSchemaSix(t *testing.T, dir string) *sql.DB {
	t.Helper()
	path := filepath.Join(dir, "learning.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	for _, ddl := range []string{ddlV1, ddlV2, ddlV3, ddlV4, ddlV5, ddlV6} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("apply ddl: %v", err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_meta (version INTEGER NOT NULL)`); err != nil {
		t.Fatalf("schema_meta: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_meta(version) VALUES (6)`); err != nil {
		t.Fatalf("seed schema_meta: %v", err)
	}
	return db
}

// notebook_links is still seeded: v7 must rewrite it while it exists. v9 then
// drops it, so only the surviving columns are checked after Open.
func TestMigrationRewritesThePathColumns(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "reading", "x.n4l"), "x")
	abs := filepath.Join(dir, "reading", "x.n4l")

	raw := openAtSchemaSix(t, dir)
	if _, err := raw.Exec(`INSERT INTO items
		(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES ('i1','n1',?,'cloze','f','b',1,0,0,0,0)`, abs); err != nil {
		t.Fatalf("seed items: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO extracts (id, file_path, text, created_at) VALUES ('e1',?,'t',0)`, abs); err != nil {
		t.Fatalf("seed extracts: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO notebook_links (file_path, chapter, node_text, notebook_id, created_at)
		VALUES (?,'ch','node','nb1',0)`, abs); err != nil {
		t.Fatalf("seed notebook_links: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO reading_elements (id, ref_path, notes_path, title, created_at)
		VALUES ('r1','sources/book.pdf',?,'t',0)`, abs); err != nil {
		t.Fatalf("seed reading_elements: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	want := "reading/x.n4l"
	checks := []struct{ query, wantVal string }{
		{`SELECT file_path FROM items WHERE id='i1'`, want},
		{`SELECT file_path FROM extracts WHERE id='e1'`, want},
		{`SELECT notes_path FROM reading_elements WHERE id='r1'`, want},
	}
	for _, c := range checks {
		var got string
		if err := s.db.QueryRow(c.query).Scan(&got); err != nil {
			t.Fatalf("query %q: %v", c.query, err)
		}
		if got != c.wantVal {
			t.Errorf("query %q = %q, want %q", c.query, got, c.wantVal)
		}
	}
}

// TestMigrationIsIdempotent covers the migrated columns, not just
// items.file_path. notebook_links is seeded but not dumped: v9 drops it, so the
// second run below also proves the v7 step tolerates the table being gone. Each table gets one resolvable row (proving a
// second pass over an already-relative value is a true no-op) and one
// unresolvable row (proving a second pass doesn't newly damage a row it
// already gave up on).
func TestMigrationIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "reading", "x.n4l"), "x")
	mustWrite(t, filepath.Join(dir, "notes", "y.n4l"), "y")
	resolvableA := filepath.Join(dir, "reading", "x.n4l")
	resolvableB := filepath.Join(dir, "notes", "y.n4l")
	stale := "/old/place/gone.n4l"

	raw := openAtSchemaSix(t, dir)
	seed := func(query string, args ...any) {
		t.Helper()
		if _, err := raw.Exec(query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	seed(`INSERT INTO items
		(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES ('i1','n1',?,'cloze','f','b',1,0,0,0,0)`, resolvableA)
	seed(`INSERT INTO items
		(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES ('i2','n2',?,'cloze','f','b',1,0,0,0,0)`, stale)

	seed(`INSERT INTO extracts (id, file_path, text, created_at) VALUES ('e1',?,'t',0)`, resolvableA)
	seed(`INSERT INTO extracts (id, file_path, text, created_at) VALUES ('e2',?,'t',0)`, stale)

	seed(`INSERT INTO notebook_links (file_path, chapter, node_text, notebook_id, created_at)
		VALUES (?,'ch1','node1','nb1',0)`, resolvableA)
	seed(`INSERT INTO notebook_links (file_path, chapter, node_text, notebook_id, created_at)
		VALUES (?,'ch2','node2','nb2',0)`, stale)

	seed(`INSERT INTO reading_elements (id, ref_path, notes_path, title, created_at)
		VALUES ('r1','sources/book.pdf',?,'t',0)`, resolvableB)
	seed(`INSERT INTO reading_elements (id, ref_path, notes_path, title, created_at)
		VALUES ('r2','sources/other.pdf',?,'t',0)`, stale)

	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	dump := func() map[string]string {
		out := map[string]string{}
		add := func(key, query string) {
			t.Helper()
			rows, err := s.db.Query(query)
			if err != nil {
				t.Fatalf("dump %s: %v", key, err)
			}
			defer rows.Close()
			for rows.Next() {
				var id, val string
				if err := rows.Scan(&id, &val); err != nil {
					t.Fatalf("dump scan %s: %v", key, err)
				}
				out[key+":"+id] = val
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("dump rows %s: %v", key, err)
			}
		}
		add("items", `SELECT id, file_path FROM items ORDER BY id`)
		add("extracts", `SELECT id, file_path FROM extracts ORDER BY id`)
		add("reading_elements", `SELECT id, notes_path FROM reading_elements ORDER BY id`)
		return out
	}

	first := dump()

	// Force the v7 step to run again over already-migrated (and
	// already-unresolvable) data, as if the version gate did not protect it.
	if _, err := s.db.Exec(`UPDATE schema_meta SET version = 6`); err != nil {
		t.Fatalf("reset schema_meta: %v", err)
	}
	if err := s.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	second := dump()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("migration not idempotent:\nfirst:  %v\nsecond: %v", first, second)
	}
}

func TestMigrationKeepsAnUnresolvableRow(t *testing.T) {
	dir := t.TempDir()
	stale := "/old/place/gone.n4l"

	raw := openAtSchemaSix(t, dir)
	if _, err := raw.Exec(`INSERT INTO items
		(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES ('i1','n1',?,'cloze','f','b',1,0,0,0,0)`, stale); err != nil {
		t.Fatalf("seed items: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var got string
	if err := s.db.QueryRow(`SELECT file_path FROM items WHERE id='i1'`).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != stale {
		t.Fatalf("unresolvable row was changed: got %q, want unchanged %q", got, stale)
	}
}

func TestMigrationLeavesRefPathAlone(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "sources", "book.pdf"), "b")
	// Deliberately absolute, even though real rows already store ref_path
	// relative: if the migration touched this column, this value — which
	// resolves fine under dir — would be rewritten to "sources/book.pdf",
	// exposing the bug this test exists to catch.
	absRef := filepath.Join(dir, "sources", "book.pdf")

	raw := openAtSchemaSix(t, dir)
	if _, err := raw.Exec(`INSERT INTO reading_elements (id, ref_path, notes_path, title, created_at)
		VALUES ('r1',?,'reading/x.n4l','t',0)`, absRef); err != nil {
		t.Fatalf("seed reading_elements: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var got string
	if err := s.db.QueryRow(`SELECT ref_path FROM reading_elements WHERE id='r1'`).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != absRef {
		t.Fatalf("ref_path was changed: got %q, want unchanged %q", got, absRef)
	}
}
