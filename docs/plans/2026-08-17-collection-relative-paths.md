# Collection-Relative Paths Implementation Plan

**Goal:** The learning database stores paths relative to the collection, so a moved or copied collection stops silently reading and writing the original location.

**Architecture:** Four columns change from absolute to collection-relative values, matching what `reading_elements.ref_path` already does. A schema-version-7 migration rewrites existing rows, re-homing a moved collection by longest matching suffix. Every resolution passes a symlink-aware containment guard, so a path that escapes the collection fails visibly instead of being used.

**Tech Stack:** Go 1.25, SQLite via `modernc.org/sqlite`. No new dependencies, no frontend changes.

**Spec:** `docs/specs/2026-08-17-collection-relative-paths-design.md`

## Global Constraints

- **Nothing outside the collection directory may be read or written through a stored path.** This is the whole point of the feature; every other requirement serves it.
- **A row that cannot be re-homed keeps its value.** Migration never drops or blanks a row. An unresolvable path fails visibly at the point of use, naming the file.
- **The migration is idempotent.** Running it twice must leave the tables identical. An already-relative value is left alone.
- **`reading_elements.ref_path` is not touched** — it is already relative and correct. The four columns in scope are `items.file_path`, `extracts.file_path`, `notebook_links.file_path` and `reading_elements.notes_path`.
- **The containment guard must not be a second copy.** `internal/media/server.go:94` already has `resolveUnder`; the symlink-aware guard replaces or absorbs it. Two similar guards that drift apart is the failure this avoids.
- **No new dependencies. No frontend changes. No schema changes** — the same columns hold different values.
- **Every test must be proven able to fail.** Each test step names the production change that breaks it; make that change, watch it fail for that reason, revert, and record the captured output. This project has twice caught controls that could not have produced the failure they claimed — capture from real runs.

---

### Task 1: The containment guard

Build the guard first: everything else depends on it, and it is the piece that makes the bug class impossible.

**Files:**
- Create: `internal/collectionpath/collectionpath.go`
- Create: `internal/collectionpath/collectionpath_test.go`
- Modify: `internal/media/server.go` (drop its local `resolveUnder`, call the shared one)

**Interfaces:**
- Produces, for Tasks 2-4:

```go
// Resolve joins rel to root and returns the absolute path, refusing anything
// that escapes root. rel may be relative or absolute; an absolute path is
// accepted only if it already lies under root.
func Resolve(root, rel string) (string, error)

// EscapeError is returned when a path resolves outside the collection.
type EscapeError struct{ Path string }
```

`EscapeError` names the offending path so the error a user sees identifies the file.

- [ ] **Step 1: Write the failing tests**

Create `internal/collectionpath/collectionpath_test.go`. Use `t.TempDir()` for the root; every case builds real files and directories, because a lexical-only test cannot prove the symlink behaviour.

```go
func TestResolveAcceptsPathsUnderRoot(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "reading", "x.n4l"), "x")
	got, err := Resolve(root, "reading/x.n4l")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := filepath.Join(root, "reading", "x.n4l"); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestResolveAcceptsAnAbsolutePathAlreadyUnderRoot(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, "a.n4l")
	mustWrite(t, abs, "a")
	got, err := Resolve(root, abs)
	if err != nil || got != abs {
		t.Fatalf("Resolve(%q) = %q, %v", abs, got, err)
	}
}

// The stale-database case: an absolute path from the collection's old location.
func TestResolveRefusesAnAbsolutePathElsewhere(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(t.TempDir(), "elsewhere.n4l")
	mustWrite(t, other, "e")
	if _, err := Resolve(root, other); err == nil {
		t.Fatal("Resolve accepted a path outside the collection")
	}
}

func TestResolveRefusesTraversal(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../escape.n4l", "reading/../../escape.n4l", "reading/../.."} {
		if _, err := Resolve(root, rel); err == nil {
			t.Errorf("Resolve(%q) accepted a traversal", rel)
		}
	}
}

// A filename may contain dots without being a traversal.
func TestResolveAcceptsDotsInsideAName(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a..b.n4l"), "x")
	if _, err := Resolve(root, "a..b.n4l"); err != nil {
		t.Errorf("Resolve rejected a legitimate name: %v", err)
	}
}

// The strengthening over the lexical helper this replaces: a symlink inside the
// collection pointing outside it must be refused, not followed.
func TestResolveRefusesASymlinkEscapingTheCollection(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.n4l")
	mustWrite(t, outside, "s")
	link := filepath.Join(root, "link.n4l")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Resolve(root, "link.n4l"); err == nil {
		t.Fatal("Resolve followed a symlink out of the collection")
	}
}

// A symlink that stays inside is fine — the guard must not reject all links.
func TestResolveAcceptsASymlinkInsideTheCollection(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real.n4l")
	mustWrite(t, target, "r")
	link := filepath.Join(root, "alias.n4l")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Resolve(root, "alias.n4l"); err != nil {
		t.Errorf("Resolve rejected an internal symlink: %v", err)
	}
}

// A file being created does not exist yet, so EvalSymlinks cannot be applied to
// it — the parent must be evaluated instead. Without this, creating any new note
// would fail.
func TestResolveAcceptsAPathThatDoesNotExistYet(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "reading"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "reading/brand-new.n4l"); err != nil {
		t.Errorf("Resolve rejected a not-yet-created file: %v", err)
	}
}

// ...but not if its parent escapes.
func TestResolveRefusesANewFileUnderAnEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(root, "away")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Resolve(root, "away/new.n4l"); err == nil {
		t.Fatal("Resolve accepted a new file under an escaping symlink")
	}
}

func TestEscapeErrorNamesThePath(t *testing.T) {
	root := t.TempDir()
	_, err := Resolve(root, "../escape.n4l")
	var esc *EscapeError
	if !errors.As(err, &esc) {
		t.Fatalf("want *EscapeError, got %T", err)
	}
	if !strings.Contains(esc.Error(), "escape.n4l") {
		t.Errorf("error does not name the file: %q", esc.Error())
	}
}
```

Write `mustWrite` as a small helper that creates parent directories and the file.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/collectionpath/...`
Expected: FAIL to build — the package does not exist.

- [ ] **Step 3: Write the implementation**

`Resolve` must, in order: reject an absolute `rel` that is not under `root`; join; clean; evaluate symlinks on the target, or on the nearest existing ancestor when the target does not exist; then require the evaluated path to equal `root` or sit under `root + separator`. Evaluate `root` itself the same way before comparing — a collection reached through a symlinked parent would otherwise never match.

That last point is the subtle one: if the user's collection is itself behind a symlink, comparing an evaluated target against an unevaluated root fails every time.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/collectionpath/...`
Expected: PASS, output pristine.

- [ ] **Step 5: Prove the tests can fail**

Three controls. Make each, run the package's tests, confirm the named test fails, revert.

1. Drop the `EvalSymlinks` step and compare the lexical path.
   Expected: `TestResolveRefusesASymlinkEscapingTheCollection` fails — this is exactly the lexical helper being replaced, so the control also demonstrates what the strengthening buys.
2. Compare with `strings.HasPrefix(full, root)` without the separator.
   Expected: add a case where a sibling directory shares the root's prefix (`/tmp/x` root, `/tmp/xy/f.n4l`) — it must be refused. If no existing test covers that, **that is a gap: add the test.**
3. Remove the not-yet-exists fallback.
   Expected: `TestResolveAcceptsAPathThatDoesNotExistYet` fails.

- [ ] **Step 6: Replace `internal/media`'s local helper**

Delete `resolveUnder` from `internal/media/server.go:94` and call `collectionpath.Resolve`. Its existing tests must still pass unchanged — if one now fails, decide deliberately whether the new behaviour is correct and say so in your report rather than adjusting the test to match.

Run: `go test ./internal/media/...`

- [ ] **Step 7: Commit**

```bash
git add internal/collectionpath internal/media/server.go
git commit -m "feat(collectionpath): a symlink-aware containment guard"
```

---

### Task 2: Relative storage and the migration

**Files:**
- Modify: `internal/learning/db.go` (the `Store` struct, `Open`, `schemaVersion`, `migrate`)
- Create: `internal/learning/paths.go`
- Create: `internal/learning/paths_test.go`

**Interfaces:**
- Consumes: `collectionpath.Resolve` from Task 1.
- Produces, for Tasks 3-4:

```go
func (s *Store) Rel(abs string) (string, error)      // absolute under the collection -> stored form
func (s *Store) Resolve(stored string) (string, error) // stored form -> absolute, guarded
func rehome(stored, collectionDir string) (rel string, ok bool)
```

- [ ] **Step 1: Write the failing tests**

Create `internal/learning/paths_test.go`.

`rehome` is the migration's brain and is tested directly:

```go
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
```

Then the migration itself, over a real database:

```go
func TestMigrationRewritesAllFourColumns(t *testing.T)
func TestMigrationIsIdempotent(t *testing.T)   // run twice, compare table contents
func TestMigrationKeepsAnUnresolvableRow(t *testing.T)
func TestMigrationLeavesRefPathAlone(t *testing.T)
```

Build each by opening a store at schema 6 with absolute rows seeded, then opening again and asserting on the contents. Write the seeding helper so each test states its own rows — a shared fixture across four tests makes it unclear which row each assertion depends on.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/learning/ -run 'Rehome|Migration'`
Expected: FAIL — `rehome` undefined.

- [ ] **Step 3: Implement**

`Store` gains `dir string`, set in `Open`. `Rel` and `Resolve` wrap `collectionpath`. `rehome` implements strip-then-suffix-search. `schemaVersion` goes to 7 with a `case 6:` that walks the four columns.

Suffix search: split the stored path into segments and try successively shorter tails — `reading/notes.n4l`, then `notes.n4l` — returning the first that exists. Longest first is what makes `TestRehomePrefersTheLongestMatchingSuffix` pass.

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/learning/...`
Expected: PASS, whole package, output pristine.

- [ ] **Step 5: Prove the tests can fail**

1. In `rehome`, try shortest suffix first.
   Expected: `TestRehomePrefersTheLongestMatchingSuffix` fails.
2. In the migration, `UPDATE` unconditionally instead of skipping unresolvable rows.
   Expected: `TestMigrationKeepsAnUnresolvableRow` fails.
3. Include `ref_path` in the migration.
   Expected: `TestMigrationLeavesRefPathAlone` fails.
4. Bump `schemaVersion` without adding `case 6:`.
   Expected: `TestMigrationRewritesAllFourColumns` fails, rows still absolute.

- [ ] **Step 6: Commit**

```bash
git add internal/learning
git commit -m "feat(learning): store collection-relative paths, migrate existing rows"
```

---

### Task 3: Convert the call sites

**Files:**
- Modify: `study_methods.go`, `reading_methods.go`, `learning_methods.go`, `notebook_methods.go`, and any other reader of the four columns

**Interfaces:**
- Consumes: `Store.Resolve` and `Store.Rel` from Task 2.

Every site that turns a stored path into file I/O must go through `Store.Resolve`; every site that stores a path must go through `Store.Rel`. `GetExtractTarget` (`study_methods.go:85-89`) is the one the incident came through.

**This task's risk is omission, and the compiler cannot help** — both forms are `string`, so a missed call site compiles and behaves correctly right up until someone moves a collection. Find them by following the four columns' readers and writers rather than by memory, and list every one you found in your report, including the ones you concluded needed no change and why.

- [ ] **Step 1: Write the failing test**

The regression test for the actual incident, in `study_methods` or an integration test file:

```go
// Point a store at a COPY of a collection whose database still holds the
// original's absolute paths, exercise the Study read/write path, and assert
// nothing outside the copy is touched. This fails against the pre-fix code.
func TestStudyDoesNotTouchTheOriginalCollection(t *testing.T)
```

Build it by creating an original collection with a note, opening a store to seed a row, copying the whole directory (database included) to a second temp dir, then driving the read/write path against the copy. Assert on the original note's **mtime and contents**, not just on return values — the incident was a silent write, and a test that only checks what a function returns would have passed.

- [ ] **Step 2: Run to verify it fails**

Expected: FAIL — the original's mtime changes, or its contents do.

- [ ] **Step 3: Convert the call sites**

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Prove the test can fail**

Revert one converted call site to use the stored path directly.
Expected: `TestStudyDoesNotTouchTheOriginalCollection` fails, naming the original file.

If it does **not** fail, that call site is not on the path the test exercises — say so, and either extend the test to reach it or record that it is untested and why.

- [ ] **Step 6: Commit**

```bash
git commit -am "fix(study): resolve stored paths against the open collection"
```

---

### Task 4: Manual verification

**Files:**
- Create: `docs/specs/2026-08-17-collection-relative-paths-manual-test-plan.md`

- [ ] **Step 1: The real-collection migration, on a copy**

Copy the real collection — `cp -r /home/simone/projects/SStorytimeFork/SStorytime-main/examples/test "$CLAUDE_JOB_DIR/tmp/paths-a"` — and open the copy in a build of this branch. Confirm the migration rewrote the four columns: `sqlite3 learning.db "select file_path from extracts limit 5"` should show relative values, and `ref_path` should be unchanged.

**Do not open the user's real collection at any point in this task.**

- [ ] **Step 2: The copy test — the actual defect**

Copy the *migrated* copy again to `paths-b`. Open `paths-b`. Record the mtimes of every file in `paths-a` first. Use Study → Read, let it write reading progress, quit, and confirm **no file in `paths-a` changed**. Against the old code this is exactly what failed.

- [ ] **Step 3: An unresolvable row fails visibly**

In `paths-b`, rename a note that has an extract. Open Study and reach that extract. Confirm you get a readable error naming the file, and that the app does not crash or silently skip.

- [ ] **Step 4: An ordinary collection is unaffected**

Open `paths-a` again — a collection that has never moved — and confirm Study, the notebook and the reading list all behave normally.

- [ ] **Step 5: Write up the result**

Every case with a pass/fail and a one-line observation, in the shape of `docs/specs/2026-08-12-node-filters-manual-test-plan.md`. A failure is a finding.

- [ ] **Step 6: Commit**

```bash
git add docs/specs/2026-08-17-collection-relative-paths-manual-test-plan.md
git commit -m "docs(test-plan): collection-relative paths"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Relative storage in the four columns | 2 |
| `ref_path` untouched | 2 (test + control 3) |
| Migration as schema 7, runs once | 2 |
| Re-home by longest matching suffix | 2 (test + control 1) |
| Unresolvable rows keep their value | 2 (test + control 2) |
| Containment guard on every resolve | 1, applied in 3 |
| Symlink resolution, with the not-yet-exists fallback | 1 (tests + controls 1 and 3) |
| One guard, not two | 1 step 6 |
| `Store` retains the collection directory | 2 step 3 |
| The regression test for the incident | 3 step 1, 4 step 2 |
| No new dependencies, no frontend, no schema change | Global Constraints |

**Type consistency:** `Resolve`, `EscapeError`, `Rel` and `rehome` keep their names and signatures across Task 1's implementation, Task 2's wrappers and Task 3's call sites. `collectionpath.Resolve(root, rel)` takes the root first, matching `resolveUnder`'s existing argument order so the `internal/media` swap is a rename rather than a reordering.

**Known risk this plan accepts:** Task 3 cannot be proven complete by the compiler or by any single test, because a missed call site is type-correct and behaves normally until a collection moves. The mitigation is enumeration in the report plus the manual pass, and the plan says so rather than implying the test suite closes it.
