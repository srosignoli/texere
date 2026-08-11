package learning

import (
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
	if v != schemaVersion {
		t.Fatalf("schema version = %d, want %d", v, schemaVersion)
	}
}
