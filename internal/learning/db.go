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
