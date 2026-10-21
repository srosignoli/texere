package learning

import (
	"database/sql"
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
	if v != schemaVersion {
		t.Fatalf("schema version = %d, want %d", v, schemaVersion)
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
	s2.Close()
}

// v5 creates notebooks and notebook_links; v9 drops notebook_links again.
func TestNotebookTablesAfterMigrate(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if !tableExists(t, s, "notebooks") {
		t.Fatal("table notebooks missing after migrate")
	}
	if tableExists(t, s, "notebook_links") {
		t.Fatal("table notebook_links still exists after migrate")
	}
	var v int
	if err := s.db.QueryRow(`SELECT version FROM schema_meta`).Scan(&v); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if v != schemaVersion {
		t.Fatalf("schema version = %d, want %d", v, schemaVersion)
	}
}

func tableExists(t *testing.T, s *Store, name string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// The text-keyed push provenance had one writer and one reader, both gone;
// v9 drops its table and whatever rows an existing Collection still holds.
func TestMigrateV9DropsNotebookLinks(t *testing.T) {
	dir := t.TempDir()
	raw, err := sql.Open("sqlite", filepath.Join(dir, "learning.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{ddlV1, ddlV2, ddlV3, ddlV4, ddlV5, ddlV6,
		`CREATE TABLE schema_meta (version INTEGER NOT NULL)`,
		`INSERT INTO schema_meta(version) VALUES (8)`,
		`INSERT INTO notebook_links (file_path, chapter, node_text, notebook_id, created_at) VALUES ('a.n4l','ch','n','nb1',0)`,
	} {
		if _, err := raw.Exec(ddl); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if tableExists(t, s, "notebook_links") {
		t.Error("notebook_links survived the v9 migration")
	}
	var v int
	if err := s.db.QueryRow(`SELECT version FROM schema_meta`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 9 {
		t.Errorf("schema version = %d, want 9", v)
	}
}
