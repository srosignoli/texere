package learning

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- shortKey ---------------------------------------------------------------

func TestShortKeyOfANodeKey(t *testing.T) {
	got := shortKey("/home/u/notes/x.n4l:x:id:73dhx9t2")
	if got != "73dhx9t2" {
		t.Errorf("got %q", got)
	}
}

func TestShortKeyOfACardKey(t *testing.T) {
	got := shortKey("/home/u/notes/x.n4l:x:id:73dhx9t2::basic::0")
	if got != "73dhx9t2::basic::0" {
		t.Errorf("got %q", got)
	}
}

// A colon is legal in a filename; the last marker is still the real one,
// because the short-id alphabet has no colon.
func TestShortKeyWithAColonInThePath(t *testing.T) {
	got := shortKey("/home/u/a:b/x.n4l:x:id:73dhx9t2")
	if got != "73dhx9t2" {
		t.Errorf("got %q", got)
	}
}

// The adversarial case a first-match split gets wrong.
func TestShortKeyWithTheMarkerInsideTheChapterTitle(t *testing.T) {
	got := shortKey("/home/u/x.n4l:chapter:id:fake:id:73dhx9t2")
	if got != "73dhx9t2" {
		t.Errorf("got %q", got)
	}
}

func TestShortKeyLeavesAnAlreadyMigratedValue(t *testing.T) {
	if got := shortKey("73dhx9t2"); got != "73dhx9t2" {
		t.Errorf("got %q", got)
	}
}

func TestShortKeyLeavesAValueWithNoMarker(t *testing.T) {
	if got := shortKey("some-other-reference"); got != "some-other-reference" {
		t.Errorf("got %q", got)
	}
}

// --- the migration, over a real database ------------------------------------

// openAtSchemaSeven creates <dir>/learning.db, applies every DDL through v6 and
// stamps schema_meta at version 7 — one step short of the identity-key change
// under test. The v7 relative-path migration is skipped rather than run: it
// rewrites file_path columns, which this feature does not touch, and the rows
// seeded here already carry relative paths. Returns the raw *sql.DB so the
// caller can seed before triggering v8 via Open; the caller must Close it first.
func openAtSchemaSeven(t *testing.T, dir string) *sql.DB {
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
	if _, err := db.Exec(`INSERT INTO schema_meta(version) VALUES (7)`); err != nil {
		t.Fatalf("seed schema_meta: %v", err)
	}
	return db
}

const (
	keyOldNode    = "notes/x.n4l:chapter:id:73dhx9t2"
	keyOldCard    = "notes/x.n4l:chapter:id:73dhx9t2::basic::0"
	keyOldExtract = "notes/y.n4l:chapter:id:9wq4mm10"
	keyNewNode    = "73dhx9t2"
	keyNewCard    = "73dhx9t2::basic::0"
	keyNewExtract = "9wq4mm10"
	// A reading_elements id. activity_log.ref_id is polymorphic, and this is the
	// shape its "read" rows carry: CreateReadingNotes returns a bare short id, so
	// the value has no :id: marker and must pass through untouched. The realism
	// matters — an 8-char marker-less string is exactly what unguarded index
	// arithmetic on a missing marker silently truncates.
	keyReadRef  = "k2m8p4qa"
	keyOtherRef = "some-other-reference"
)

// seedSchemaSeven fills a v7 database with one item and its review log, one
// extract and its notebook, and two activity rows — one identity-shaped, one
// not. Returns after closing the raw handle so the caller can Open(dir).
func seedSchemaSeven(t *testing.T, dir string) {
	t.Helper()
	raw := openAtSchemaSeven(t, dir)

	if _, err := raw.Exec(`INSERT INTO items
		(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES (?,?,'notes/x.n4l','basic','f','b',1,0,0,0,0)`, keyOldCard, keyOldNode); err != nil {
		t.Fatalf("seed items: %v", err)
	}
	// Two logs on one item: the reference side is legitimately many-to-one, so a
	// collapse there must NOT be treated as an error.
	for i := 0; i < 2; i++ {
		if _, err := raw.Exec(`INSERT INTO review_logs (item_id, reviewed_at, grade)
			VALUES (?,?,3)`, keyOldCard, 1000+i); err != nil {
			t.Fatalf("seed review_logs: %v", err)
		}
	}
	if _, err := raw.Exec(`INSERT INTO extracts (id, file_path, text, created_at)
		VALUES (?,'notes/y.n4l','t',0)`, keyOldExtract); err != nil {
		t.Fatalf("seed extracts: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO notebooks
		(notebook_id, extract_node_id, chapter, context_tags, cells_json, created_at, updated_at)
		VALUES ('nb-abc',?,'chapter','[]','[]',0,0)`, keyOldExtract); err != nil {
		t.Fatalf("seed notebooks: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO activity_log (kind, ref_id, at, detail, amount)
		VALUES ('process',?,0,'promote',0)`, keyOldExtract); err != nil {
		t.Fatalf("seed activity_log process: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO activity_log (kind, ref_id, at, detail, amount)
		VALUES ('read',?,0,'',60)`, keyReadRef); err != nil {
		t.Fatalf("seed activity_log read: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO activity_log (kind, ref_id, at, detail, amount)
		VALUES ('process',?,0,'defer',0)`, keyOtherRef); err != nil {
		t.Fatalf("seed activity_log other: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
}

func scalar(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var got string
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return got
}

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

const (
	orphanLogs      = `SELECT count(*) FROM review_logs WHERE item_id NOT IN (SELECT id FROM items)`
	orphanNotebooks = `SELECT count(*) FROM notebooks WHERE extract_node_id NOT IN (SELECT id FROM extracts)`
)

// The acceptance property. Counting, not sampling: every reference row is
// checked against every key row on both sides of the migration.
func TestIdentityMigrationLeavesNoOrphanedReferences(t *testing.T) {
	dir := t.TempDir()
	seedSchemaSeven(t, dir)

	// Before: the seed is internally consistent, so the check is meaningful.
	raw, err := sql.Open("sqlite", filepath.Join(dir, "learning.db"))
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, raw, orphanLogs); n != 0 {
		t.Fatalf("seed already had %d orphaned review_logs; the test proves nothing", n)
	}
	if n := count(t, raw, orphanNotebooks); n != 0 {
		t.Fatalf("seed already had %d orphaned notebooks; the test proves nothing", n)
	}
	raw.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if n := count(t, s.db, orphanLogs); n != 0 {
		t.Errorf("%d review_logs rows reference an item that no longer exists", n)
	}
	if n := count(t, s.db, orphanNotebooks); n != 0 {
		t.Errorf("%d notebooks rows reference an extract that no longer exists", n)
	}
	// Zero orphans is also satisfied by zero rows. Assert the rows survived.
	if n := count(t, s.db, `SELECT count(*) FROM review_logs`); n != 2 {
		t.Errorf("review_logs has %d rows, want 2 — the migration dropped history", n)
	}
	if n := count(t, s.db, `SELECT count(*) FROM notebooks`); n != 1 {
		t.Errorf("notebooks has %d rows, want 1", n)
	}
}

func TestIdentityMigrationRewritesAllSixColumns(t *testing.T) {
	dir := t.TempDir()
	seedSchemaSeven(t, dir)

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	checks := []struct{ what, query, want string }{
		{"items.id", `SELECT id FROM items`, keyNewCard},
		{"items.node_id", `SELECT node_id FROM items`, keyNewNode},
		{"extracts.id", `SELECT id FROM extracts`, keyNewExtract},
		{"review_logs.item_id", `SELECT item_id FROM review_logs LIMIT 1`, keyNewCard},
		{"notebooks.extract_node_id", `SELECT extract_node_id FROM notebooks`, keyNewExtract},
		{"activity_log.ref_id", `SELECT ref_id FROM activity_log WHERE detail='promote'`, keyNewExtract},
	}
	for _, c := range checks {
		if got := scalar(t, s.db, c.query); got != c.want {
			t.Errorf("%s = %q, want %q", c.what, got, c.want)
		}
	}

	var v int
	if err := s.db.QueryRow(`SELECT version FROM schema_meta`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Errorf("schema version = %d, want %d", v, schemaVersion)
	}
}

// activity_log.ref_id is polymorphic: "read" rows carry a reading_elements id,
// which is not an identity key and has no :id: marker. Rewriting on shape, not
// on table, is what keeps those rows intact.
func TestIdentityMigrationLeavesNonIdentityRefIDsAlone(t *testing.T) {
	dir := t.TempDir()
	seedSchemaSeven(t, dir)

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if got := scalar(t, s.db, `SELECT ref_id FROM activity_log WHERE kind='read'`); got != keyReadRef {
		t.Errorf("read ref_id = %q, want %q untouched", got, keyReadRef)
	}
	if got := scalar(t, s.db, `SELECT ref_id FROM activity_log WHERE detail='defer'`); got != keyOtherRef {
		t.Errorf("non-identity ref_id = %q, want %q untouched", got, keyOtherRef)
	}
}

// What this proves: applying the migration to already-migrated data is a no-op,
// because shortKey of a short key is that key. That is the real property — a
// database migrated by an earlier build, or a value written in the new format
// before the migration ran, must survive a second pass unchanged.
//
// What it does NOT prove: that the migration is correct. A rewrite that
// converges on a stable wrong value would satisfy this test exactly as well as
// the right one does. Correctness is the job of TestIdentityMigrationRewritesAllSixColumns
// and TestIdentityMigrationLeavesNoOrphanedReferences; this test only rules out drift.
func TestIdentityMigrationIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	seedSchemaSeven(t, dir)

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	first := map[string]string{
		"items.id":      scalar(t, s.db, `SELECT id FROM items`),
		"items.node_id": scalar(t, s.db, `SELECT node_id FROM items`),
		"extracts.id":   scalar(t, s.db, `SELECT id FROM extracts`),
		"review_logs":   scalar(t, s.db, `SELECT item_id FROM review_logs LIMIT 1`),
		"notebooks":     scalar(t, s.db, `SELECT extract_node_id FROM notebooks`),
		"activity_log":  scalar(t, s.db, `SELECT ref_id FROM activity_log WHERE detail='promote'`),
	}
	// Wind the version back so the migration genuinely runs a second time over
	// the already-migrated rows. Without this, Open would short-circuit on
	// version >= schemaVersion and the test would assert nothing at all.
	if _, err := s.db.Exec(`UPDATE schema_meta SET version = 7`); err != nil {
		t.Fatalf("reset schema_meta: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	second := map[string]string{
		"items.id":      scalar(t, s2.db, `SELECT id FROM items`),
		"items.node_id": scalar(t, s2.db, `SELECT node_id FROM items`),
		"extracts.id":   scalar(t, s2.db, `SELECT id FROM extracts`),
		"review_logs":   scalar(t, s2.db, `SELECT item_id FROM review_logs LIMIT 1`),
		"notebooks":     scalar(t, s2.db, `SELECT extract_node_id FROM notebooks`),
		"activity_log":  scalar(t, s2.db, `SELECT ref_id FROM activity_log WHERE detail='promote'`),
	}
	for k, want := range first {
		if second[k] != want {
			t.Errorf("%s changed on the second migration: %q -> %q", k, want, second[k])
		}
	}
}

func TestIdentityMigrationBacksUpTheDatabase(t *testing.T) {
	dir := t.TempDir()
	seedSchemaSeven(t, dir)

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	backup := filepath.Join(dir, "learning.db.bak-v7")
	// Stat first. sql.Open creates a missing file rather than failing, so
	// without this check a wholly absent backup would surface as a confusing
	// "no such table: items" instead of naming the actual problem.
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("no backup at %s: %v", backup, err)
	}
	bak, err := sql.Open("sqlite", backup)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer bak.Close()

	// Existence is not enough: a backup taken after the rewrite would be
	// worthless. It must hold the PRE-migration key.
	if got := scalar(t, bak, `SELECT id FROM items`); got != keyOldCard {
		t.Errorf("backup items.id = %q, want the pre-migration key %q", got, keyOldCard)
	}
	var v int
	if err := bak.QueryRow(`SELECT version FROM schema_meta`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 7 {
		t.Errorf("backup schema version = %d, want 7", v)
	}
}

// A second backup must not overwrite the first: the useful copy is the one
// holding the original keys, and clobbering it with already-migrated rows would
// destroy the only record of what the keys were.
//
// This drives backupBeforeV8 directly rather than through two Opens. Going
// through Open would not test the guard at all: by the second Open every row is
// already migrated, so migrateIdentityKeys finds no work and returns before it
// ever reaches the backup. The guard covers the case that early return does not
// — a database where some rows still need rewriting and a backup already
// exists, which is what a partially-applied migration leaves behind.
func TestBackupDoesNotOverwriteAnExistingBackup(t *testing.T) {
	dir := t.TempDir()
	seedSchemaSeven(t, dir)

	backup := filepath.Join(dir, "learning.db.bak-v7")
	const sentinel = "the original backup"
	if err := os.WriteFile(backup, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Store{dir: dir}
	if err := s.backupBeforeV8(); err != nil {
		t.Fatalf("backupBeforeV8: %v", err)
	}

	got, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Errorf("the existing backup was overwritten (now %d bytes)", len(got))
	}
}

// The companion case: with no backup present, one is written and it holds the
// pre-migration state.
func TestBackupWritesWhenNoneExists(t *testing.T) {
	dir := t.TempDir()
	seedSchemaSeven(t, dir)

	s := &Store{dir: dir}
	if err := s.backupBeforeV8(); err != nil {
		t.Fatalf("backupBeforeV8: %v", err)
	}

	bak, err := sql.Open("sqlite", filepath.Join(dir, "learning.db.bak-v7"))
	if err != nil {
		t.Fatal(err)
	}
	defer bak.Close()
	if got := scalar(t, bak, `SELECT id FROM items`); got != keyOldCard {
		t.Errorf("backup items.id = %q, want %q", got, keyOldCard)
	}
}

// Two nodes whose keys differ only in their path collapse to one short key.
// Silently merging them would fuse two nodes' study history; the migration must
// refuse and leave the database as it found it.
func TestIdentityMigrationFailsWhenTwoKeysCollapseToOne(t *testing.T) {
	dir := t.TempDir()
	raw := openAtSchemaSeven(t, dir)
	for _, id := range []string{
		"notes/a.n4l:chapter:id:73dhx9t2::basic::0",
		"notes/b.n4l:chapter:id:73dhx9t2::basic::0",
	} {
		if _, err := raw.Exec(`INSERT INTO items
			(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
			VALUES (?,'n','notes/a.n4l','basic','f','b',1,0,0,0,0)`, id); err != nil {
			t.Fatalf("seed items: %v", err)
		}
	}
	raw.Close()

	s, err := Open(dir)
	if err == nil {
		s.Close()
		t.Fatal("Open succeeded; a key collapse must abort the migration, not merge two nodes' history")
	}
	// Assert on the message, not merely on failure. items.id is a PRIMARY KEY, so
	// SQLite would reject the second UPDATE with "UNIQUE constraint failed"
	// whether or not the migration checks for a collapse itself — an
	// abort-or-not assertion therefore cannot tell the two apart and would pass
	// with the check deleted. What the explicit check adds is what is asserted
	// here: a message naming the colliding keys, produced before any row is
	// written rather than partway through the rewrite.
	if !strings.Contains(err.Error(), "collapse") {
		t.Errorf("error did not name the collapse: %v", err)
	}
	for _, key := range []string{
		"notes/a.n4l:chapter:id:73dhx9t2::basic::0",
		"notes/b.n4l:chapter:id:73dhx9t2::basic::0",
	} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error did not name the colliding key %q: %v", key, err)
		}
	}

	// The database must be left exactly as it was: still at 7, keys untouched.
	after, openErr := sql.Open("sqlite", filepath.Join(dir, "learning.db"))
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer after.Close()
	var v int
	if err := after.QueryRow(`SELECT version FROM schema_meta`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 7 {
		t.Errorf("schema version = %d after a failed migration, want 7", v)
	}
	if n := count(t, after, `SELECT count(*) FROM items WHERE id LIKE '%:id:%'`); n != 2 {
		t.Errorf("%d rows still hold their original key, want 2 — the failed migration was not rolled back", n)
	}
}

// A brand-new collection migrates 0 -> 8 with every table empty. There is
// nothing to rewrite and so nothing to protect: backing up would leave a copy
// of an empty database beside every collection ever created.
func TestIdentityMigrationOnAFreshCollectionWritesNoBackup(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(filepath.Join(dir, "learning.db.bak-v7")); !os.IsNotExist(err) {
		t.Errorf("a fresh collection was given a backup of its own empty database (stat err: %v)", err)
	}
}

// An already-migrated database re-entering the migration has no rows to rewrite
// either, so it must not produce a backup that would be indistinguishable from
// the pre-migration one.
func TestIdentityMigrationWritesNoBackupWhenNothingNeedsRewriting(t *testing.T) {
	dir := t.TempDir()
	raw := openAtSchemaSeven(t, dir)
	// Seeded in the NEW format: nothing here carries a :id: marker.
	if _, err := raw.Exec(`INSERT INTO items
		(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES (?,?,'notes/x.n4l','basic','f','b',1,0,0,0,0)`, keyNewCard, keyNewNode); err != nil {
		t.Fatalf("seed items: %v", err)
	}
	raw.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(filepath.Join(dir, "learning.db.bak-v7")); !os.IsNotExist(err) {
		t.Errorf("backup written for a database that needed no rewriting (stat err: %v)", err)
	}
	if got := scalar(t, s.db, `SELECT id FROM items`); got != keyNewCard {
		t.Errorf("items.id = %q, want %q unchanged", got, keyNewCard)
	}
}
