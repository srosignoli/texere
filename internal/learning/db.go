package learning

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers "sqlite")
)

// Store is the handle to one collection's learning.db.
type Store struct {
	db  *sql.DB
	dir string // collection root; file_path-like columns are stored relative to this
}

const schemaVersion = 8

// Open opens (creating if needed) <collectionDir>/learning.db and migrates it
// to the current schema version.
func Open(collectionDir string) (*Store, error) {
	path := filepath.Join(collectionDir, "learning.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("learning: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // SQLite single-writer; avoids "database is locked"
	s := &Store{db: db, dir: filepath.Clean(collectionDir)}
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
		case 2:
			if _, err := s.db.Exec(ddlV3); err != nil {
				return fmt.Errorf("learning: apply v3: %w", err)
			}
		case 3:
			if _, err := s.db.Exec(ddlV4); err != nil {
				return fmt.Errorf("learning: apply v4: %w", err)
			}
		case 4:
			if _, err := s.db.Exec(ddlV5); err != nil {
				return fmt.Errorf("learning: apply v5: %w", err)
			}
		case 5:
			if _, err := s.db.Exec(ddlV6); err != nil {
				return fmt.Errorf("learning: apply v6: %w", err)
			}
		case 6:
			// No schema change: items.file_path, extracts.file_path,
			// notebook_links.file_path and reading_elements.notes_path move
			// from absolute to collection-relative values.
			// reading_elements.ref_path is deliberately excluded — it already
			// stores relative values.
			if err := s.migrateRelativePaths(); err != nil {
				return fmt.Errorf("learning: apply v7: %w", err)
			}
		case 7:
			// No schema change: the six columns holding identity keys drop the
			// embedded file path, so keys survive the collection being moved or
			// copied. See migrateIdentityKeys.
			if err := s.migrateIdentityKeys(); err != nil {
				return fmt.Errorf("learning: apply v8: %w", err)
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

const ddlV4 = `
ALTER TABLE reading_elements ADD COLUMN medium TEXT NOT NULL DEFAULT 'text';
ALTER TABLE reading_elements ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
`

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

const ddlV6 = `
CREATE TABLE IF NOT EXISTS activity_log (
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  kind   TEXT NOT NULL,
  ref_id TEXT NOT NULL DEFAULT '',
  at     INTEGER NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  amount INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_activity_at ON activity_log(at);
`
