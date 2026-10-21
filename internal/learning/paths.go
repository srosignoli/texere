package learning

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"texere/internal/collectionpath"
)

// rel converts a path under the collection (absolute, or already relative) to
// its stored, collection-relative form. Every Store write stores paths through
// it. It refuses (via collectionpath.Resolve) any path that does not lie under
// the collection root.
func (s *Store) rel(abs string) (string, error) {
	full, err := collectionpath.Resolve(s.dir, abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.dir, full)
	if err != nil {
		return "", fmt.Errorf("learning: rel %q under %q: %w", full, s.dir, err)
	}
	return filepath.ToSlash(rel), nil
}

// errUnresolvable marks a row whose stored path does not resolve under the
// Collection. A single-row read returns it; a many-row read skips the row.
var errUnresolvable = errors.New("stored path outside the collection")

// resolve converts a stored (collection-relative, or legacy absolute) value
// back to an absolute path, guarded to remain under the collection root. Every
// Store read returns paths through it.
func (s *Store) resolve(stored string) (string, error) {
	return collectionpath.Resolve(s.dir, stored)
}

// rehome is the migration's brain. It takes a stored value that may already
// be relative, may be an absolute path still under collectionDir, or may be a
// stale absolute path left over from a since-moved-or-copied collection, and
// tries to recover the file's location under collectionDir.
//
// It works by splitting stored into path segments and trying successively
// shorter tails against collectionDir on disk, longest first: for
// "/old/place/reading/notes.n4l" it tries "old/place/reading/notes.n4l",
// then "place/reading/notes.n4l", then "reading/notes.n4l" (found), never
// falling as far as the bare basename "notes.n4l" — which matters when a
// name repeats at two depths, since a basename-only search would silently
// pick the wrong file.
//
// ok is false when no suffix exists under collectionDir; the caller must
// leave the original value untouched rather than guess.
//
// Known limitation: the split is on "/" (after filepath.ToSlash, which only
// rewrites the current OS's own separator). A value written with Windows-style
// backslashes and then read back on a different platform — or vice versa —
// will not be segmented on those backslashes and so will not be found, even
// if the file exists. This fails safe: the caller sees ok=false and leaves
// the row untouched, the same outcome as any other unresolvable row. Left
// undone deliberately: also splitting on "\" would misparse any segment that
// legitimately contains a literal backslash (a valid filename byte on Unix),
// so handling both separators needs more than a one-line fix.
func rehome(stored, collectionDir string) (rel string, ok bool) {
	cleaned := filepath.ToSlash(filepath.Clean(stored))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return "", false
	}
	segments := strings.Split(cleaned, "/")
	for i := 0; i < len(segments); i++ {
		candidate := filepath.Join(segments[i:]...)
		full := filepath.Join(collectionDir, candidate)
		if _, err := os.Stat(full); err == nil {
			return filepath.ToSlash(candidate), true
		}
	}
	return "", false
}

// migrateRelativePaths rewrites the stored-path columns of the four affected
// tables in place, using rehome to recover each row's file under s.dir. Rows
// rehome cannot resolve are left with their original value — never dropped,
// never blanked — so a scheduling row that can no longer be placed still
// exists and fails visibly, by name, at the point of use rather than
// vanishing silently here.
//
// reading_elements.ref_path is not one of the four columns walked below: it
// already stores collection-relative values and is out of scope.
func (s *Store) migrateRelativePaths() error {
	columns := []struct{ table, column string }{
		{"items", "file_path"},
		{"extracts", "file_path"},
		{"notebook_links", "file_path"},
		{"reading_elements", "notes_path"},
	}
	for _, c := range columns {
		if err := s.migrateColumnToRelative(c.table, c.column); err != nil {
			return fmt.Errorf("%s.%s: %w", c.table, c.column, err)
		}
	}
	return nil
}

// migrateColumnToRelative rewrites one column of one table via rehome,
// addressing rows by SQLite's implicit rowid so it works regardless of the
// table's declared primary key (notebook_links has no single-column key, and
// the value being rewritten is itself part of its composite key).
func (s *Store) migrateColumnToRelative(table, column string) error {
	// A table a later version dropped (notebook_links, at v9) is absent when
	// this step runs again over an already-migrated database.
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
		return fmt.Errorf("lookup: %w", err)
	}
	if n == 0 {
		return nil
	}
	rows, err := s.db.Query(fmt.Sprintf(`SELECT rowid, %s FROM %s`, column, table))
	if err != nil {
		return fmt.Errorf("select: %w", err)
	}
	type pending struct {
		rowid int64
		value string
	}
	var updates []pending
	for rows.Next() {
		var rowid int64
		var value string
		if err := rows.Scan(&rowid, &value); err != nil {
			rows.Close()
			return fmt.Errorf("scan: %w", err)
		}
		if rel, ok := rehome(value, s.dir); ok {
			updates = append(updates, pending{rowid, rel})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("rows: %w", err)
	}
	rows.Close()

	stmt := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE rowid = ?`, table, column)
	for _, u := range updates {
		if _, err := s.db.Exec(stmt, u.value, u.rowid); err != nil {
			return fmt.Errorf("update rowid %d: %w", u.rowid, err)
		}
	}
	return nil
}
