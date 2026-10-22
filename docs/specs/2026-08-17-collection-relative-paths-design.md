# Collection-relative paths in the learning store

**Date:** 2026-08-17
**Status:** Proposed — awaiting approval
**Feature area:** `internal/learning`, Study read/write paths

## Summary

The learning database stores absolute file paths. It also lives *inside* the
collection, so a copied or moved collection carries a database still pointing at
the original — and Study silently reads and writes there. Paths become relative
to the collection directory, resolved on read behind a containment guard.

## Motivation

Found by the editor-theming verification pass, which ran against a copy of the
collection. Opening **Study → Read → Spaced repetition** displayed a file that
did not match the copy, and the *real* file took a new mtime at that moment:

```
- # @progress char=11555
+ # @progress char=0
```

A reading position, silently reset in a directory nobody was looking at. The
general form: **a user who moves or copies their collection will have Study read
and write the old location.** Nothing warns them, and the damage is to notes,
not to the database.

## Grounding in the current implementation

Measured against the collection at
`/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`:

- **Five path columns across four tables.** Four hold absolute paths:
  `items.file_path` (15 rows), `extracts.file_path` (20),
  `notebook_links.file_path` (25), `reading_elements.notes_path` (5).
- **The fifth already does it right.** `reading_elements.ref_path` holds
  `sources/SW_SmartMS42….pdf` — relative, in every row. The convention this
  design adopts already exists in the schema; four columns simply do not follow
  it.
- **`learning.db` lives inside the collection** (`internal/learning/db.go:21`,
  `filepath.Join(collectionDir, "learning.db")`), which is what makes the defect
  acute: the database travels with a copy while its contents do not.
- **`GetExtractTarget` hands the stored path straight back**
  (`study_methods.go:85-89`), and `ReadingView`/`ProcessEditor` call
  `GetFileContent`/`SaveFile` on it. No layer re-resolves it against
  `collectionDir`.
- **A versioned migration mechanism already exists** — `schema_meta` and
  `schemaVersion` (currently 6) with a `switch` per version
  (`internal/learning/db.go:43-91`).
- **A containment helper already exists** — `resolveUnder(root, rel)`
  (`internal/media/server.go:94`), written and reviewed for the collection asset
  route. It is lexical: an earlier review logged, and deferred, that a symlink
  inside the collection can point outside it.
- **`Store` does not retain the collection directory** — `Open` uses it only to
  build the database path (`db.go:20-33`), and the struct holds `db` alone.

## Decisions

- **Store paths relative to the collection directory**, matching `ref_path`. No
  schema change: the same columns hold different values.
- **Migrate as schema version 7**, so it runs once and is recorded, using the
  mechanism already in place.
- **Re-home by longest matching suffix when the prefix does not match.** For a
  collection still in place, stripping the current prefix is enough. For one
  that has been moved or copied — the case that motivates this work — the stored
  prefix is the *old* directory. Falling back to the longest trailing portion of
  the stored path that exists under the current collection resolves
  `/old/place/reading/x.n4l` to `reading/x.n4l`. It is a heuristic; it is
  testable, and it is right in the case that matters.
- **A row that cannot be resolved keeps its value.** Migration leaves it alone
  and the containment guard turns any use into an error naming the file. Nothing
  is lost, and the user learns which note is missing rather than silently losing
  its scheduling history. Rejected: dropping such rows, which is irreversible if
  a file was only temporarily moved.
- **Every resolution passes a containment guard.** This is what makes the class
  of bug impossible rather than merely fixed — with it, the incident above would
  have been a visible error instead of a write to the wrong file.
- **The guard resolves symlinks.** `resolveUnder` is lexical, which was
  tolerable on a read-only asset route and is not here, because this guard
  stands in front of writes to the user's notes. The resolved path is evaluated
  with `filepath.EvalSymlinks` before the containment check, falling back to the
  parent directory when the target does not exist yet — a file about to be
  created has nothing to evaluate. This closes the deferred media finding on the
  path where it matters.
- **`Store` retains the collection directory.** Both the migration and every
  resolution need it, and threading it through each call site instead would give
  four opportunities to pass the wrong one.

## Architecture

### `internal/learning`

`Store` gains a `dir string` field, set by `Open`.

```go
// Rel converts an absolute path under the collection to its stored form.
func (s *Store) Rel(abs string) (string, error)

// Resolve turns a stored path into an absolute one, refusing anything that
// escapes the collection directory. Returns a *PathEscapeError naming the file
// when it does.
func (s *Store) Resolve(stored string) (string, error)
```

`Resolve` accepts a value that is already absolute — the pre-migration form, and
the form an unresolvable row keeps — and subjects it to the same containment
check, so a stale absolute path fails visibly rather than being used.

Migration `case 6:` rewrites the four columns. Its re-homing logic is a separate,
directly testable function rather than inline SQL glue:

```go
// rehome returns the collection-relative form of a stored path, and whether it
// could be resolved at all.
func rehome(stored, collectionDir string) (rel string, ok bool)
```

### Containment

The symlink-aware guard is the shared piece. `internal/media`'s `resolveUnder`
and this guard should be one function in one place rather than two similar ones
drifting apart — the implementation decides where it lives, but it must not be
copied.

### `study_methods.go` and callers

Every site that turns a stored path into file I/O goes through `Store.Resolve`.
`GetExtractTarget` is the one the incident came through; the others are found by
following the four columns' readers.

## Error handling

| Case | Behaviour |
|---|---|
| Stored path resolves inside the collection | Used normally |
| Stored path escapes, or is a stale absolute path from another machine | Error naming the file; no read, no write |
| Symlink inside the collection pointing outside | Same — refused after `EvalSymlinks` |
| File does not exist yet (a new note) | Parent directory is evaluated instead; creation proceeds |
| Migration cannot re-home a row | Value left untouched; the row fails visibly when used |
| Migration runs twice | Idempotent — an already-relative value is left alone |

## Testing

**The test that would have caught this:** point the store at a *copy* of a
collection whose database still holds the original's absolute paths, exercise the
Study read/write path, and assert that **no file outside the copy is touched** —
comparing mtimes before and after, not just return values. This is the regression
test for the incident, and it fails against today's code.

**Unit:** `rehome` for an in-place collection, a moved one, an already-relative
value, an unresolvable value, and a path whose basename appears twice at
different depths. `Resolve` for a normal path, `..` traversal, an absolute path
from elsewhere, and a symlink escape — the last requires creating a real symlink
in a temp directory, which is the only way to prove the strengthening works.
Migration idempotency by running it twice and comparing the table contents.

**Not covered by tests, and stated rather than hidden:** whether every call site
was converted. The compiler cannot enforce it, since both forms are `string`.
The manual pass covers the reachable ones.

**Manual:** open a copied collection and confirm Study operates on the copy;
confirm an unresolvable row produces a readable error; confirm an ordinary,
never-moved collection is unaffected.

## Out of scope

- `reading_elements.ref_path`, which is already relative and correct.
- Any UI for listing or repairing unresolvable rows.
- The graph store (`sst_data/`) and the Bleve index, which are rebuilt rather
  than migrated.
- Restoring the note modified by the incident, which the user has chosen to
  handle themselves.
