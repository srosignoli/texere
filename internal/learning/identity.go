package learning

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// idMarker separates the location prefix from the short ID in a pre-v8 identity
// key: "<path>:<chapter>:id:<short>" for a node, and the same with
// "::<cardType>::<discriminator>" appended for a card.
const idMarker = ":id:"

// shortKey strips the location prefix from a pre-v8 identity key, returning
// everything after the LAST ":id:" marker — the short ID for a node key, and
// "<short>::<cardType>::<discriminator>" for a card key, since the card suffix
// sits after the marker and rides along untouched.
//
// The split is on the last marker, not the first, because ":id:" can appear
// earlier by accident: a chapter title may contain it, and a colon is a legal
// filename byte. The last one is always the real one, because the short-ID
// alphabet is [0-9a-z] and so contains no colon — nothing after the true marker
// can look like another one.
//
// A value with no marker is returned unchanged. That covers two cases that must
// both be left alone: a key already migrated by an earlier run, and a value that
// was never an identity key at all (activity_log.ref_id is polymorphic — its
// "read" rows carry a reading_elements id).
func shortKey(old string) string {
	i := strings.LastIndex(old, idMarker)
	if i < 0 {
		return old
	}
	return old[i+len(idMarker):]
}

// identityColumn is one column holding identity keys, and whether a collapse in
// it is fatal.
type identityColumn struct {
	table  string
	column string
	// unique marks a column whose values identify one row each (a primary key).
	// Two distinct old values landing on one new value there would merge two
	// nodes' study history, so the migration aborts. On a reference column
	// (review_logs.item_id, notebooks.extract_node_id, activity_log.ref_id)
	// many rows pointing at one key is the normal case, not an error.
	unique bool
}

// identityColumns are the six columns holding keys that embed the file path.
//
// The key columns come before the columns referencing them. Ordering is not
// load-bearing here — this schema declares no FOREIGN KEY constraints, so
// SQLite enforces none regardless of PRAGMA foreign_keys, and the whole rewrite
// runs in one transaction in any case. It is written parent-first so that a
// future schema which does declare constraints does not silently start
// depending on the opposite order.
//
// notebooks.notebook_id and notebook_links.notebook_id are deliberately absent:
// those are "nb-" + random hex, minted independently of any file path, so they
// were never location-dependent and need no rewrite.
var identityColumns = []identityColumn{
	{"items", "id", true},
	{"items", "node_id", false}, // many cards share one node
	{"extracts", "id", true},
	{"review_logs", "item_id", false},
	{"notebooks", "extract_node_id", false},
	{"activity_log", "ref_id", false},
}

// migrateIdentityKeys rewrites every identity key in the store from
// "<path>:<chapter>:id:<short>" to the bare "<short>", so the keys stop
// depending on where the collection sits on disk.
//
// All six columns move in one transaction: review_logs.item_id references
// items.id and notebooks.extract_node_id references extracts.id, so rewriting
// keys without their references — or the reverse — would orphan rows. Either
// every column moves or none does.
//
// It runs in two phases. The first only reads: it computes every row's new
// value and refuses the whole migration if any two keys collapse, so a database
// that cannot be migrated safely is never written to at all. The second copies
// the database to learning.db.bak-v7 and applies the updates. Nothing else can
// be writing in between — migrate runs inside Open, before the Store is handed
// to any caller.
//
// The backup exists because a transaction can roll back a migration that fails
// but cannot undo one that succeeds while being wrong, and this rewrite is not
// reversible from its own output: the discarded path prefix cannot be
// reconstructed from a short key.
//
// A database with no rows to rewrite (a freshly created collection, or one
// already migrated) skips both the backup and the transaction. Otherwise every
// new collection would be born with a backup copy of its own empty database.
func (s *Store) migrateIdentityKeys() error {
	var updates []identityUpdate
	for _, c := range identityColumns {
		planned, err := planIdentityColumn(s.db, c)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", c.table, c.column, err)
		}
		updates = append(updates, planned...)
	}
	if len(updates) == 0 {
		return nil
	}

	if err := s.backupBeforeV8(); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	// A no-op once committed: Rollback then returns ErrTxDone, which is the
	// expected outcome on the success path and carries nothing to act on.
	defer func() { _ = tx.Rollback() }()

	for _, u := range updates {
		stmt := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE rowid = ?`, u.table, u.column)
		if _, err := tx.Exec(stmt, u.value, u.rowid); err != nil {
			return fmt.Errorf("%s.%s rowid %d: %w", u.table, u.column, u.rowid, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// identityUpdate is one planned rewrite: set table.column to value on one row.
type identityUpdate struct {
	table  string
	column string
	rowid  int64
	value  string
}

// backupBeforeV8 copies learning.db to learning.db.bak-v7 beside it.
//
// An existing backup is left alone. The copy worth keeping is the one taken
// before the first migration, holding the original keys; overwriting it on a
// later pass — after the rows have already been rewritten — would replace the
// only record of what the keys were with a copy of the migrated state.
func (s *Store) backupBeforeV8() error {
	src := filepath.Join(s.dir, "learning.db")
	dst := filepath.Join(s.dir, "learning.db.bak-v7")

	if _, err := os.Stat(dst); err == nil {
		return nil // already backed up by an earlier pass
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat backup: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open db for backup: %w", err)
	}
	defer in.Close()

	// O_EXCL: if another process created the backup between the Stat above and
	// here, that copy is the older one and must win.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("create backup: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst) // a truncated backup is worse than none: it invites false confidence
		return fmt.Errorf("copy backup: %w", err)
	}
	// Close, not defer: a backup that failed to flush must fail the migration.
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("flush backup: %w", err)
	}
	return nil
}

// planIdentityColumn reads one column and returns the rewrites it needs, without
// writing anything. Rows are addressed by SQLite's implicit rowid so this works
// whatever the table's declared primary key is — including when the value being
// rewritten is itself that primary key.
//
// On a unique column it checks that no two distinct old values map onto one new
// value and refuses the whole migration if any pair does. SQLite's PRIMARY KEY
// would reject the colliding UPDATE anyway, so this is not what makes the
// operation safe; what it adds is an error naming the offending keys instead of
// a bare "UNIQUE constraint failed", raised before the first write rather than
// partway through the rewrite.
func planIdentityColumn(db *sql.DB, c identityColumn) ([]identityUpdate, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT rowid, %s FROM %s`, c.column, c.table))
	if err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}
	var updates []identityUpdate
	// origins maps a post-migration value to the distinct pre-migration values
	// that produced it. Built over every row, not only rewritten ones: a row
	// already holding "73dhx9t2" collides with a row migrating to it just as
	// surely as two migrating rows collide with each other.
	origins := make(map[string]map[string]bool)
	for rows.Next() {
		var rowid int64
		var value string
		if err := rows.Scan(&rowid, &value); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan: %w", err)
		}
		next := shortKey(value)
		if c.unique {
			if origins[next] == nil {
				origins[next] = make(map[string]bool)
			}
			origins[next][value] = true
		}
		if next != value {
			updates = append(updates, identityUpdate{c.table, c.column, rowid, next})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("rows: %w", err)
	}
	rows.Close()

	for next, from := range origins {
		if len(from) > 1 {
			keys := make([]string, 0, len(from))
			for k := range from {
				keys = append(keys, k)
			}
			sort.Strings(keys) // deterministic message
			return nil, fmt.Errorf(
				"refusing to migrate: %d distinct keys collapse to %q (%v). "+
					"Migrating would merge their study history into one row. "+
					"The database has not been modified",
				len(from), next, keys)
		}
	}
	return updates, nil
}
