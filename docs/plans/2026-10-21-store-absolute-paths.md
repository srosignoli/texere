# Store Absolute Paths Implementation Plan

**Goal:** The learning Store returns and accepts absolute paths, applying one failure policy for rows that do not resolve. The ~20 per-binding conversions go.

**Architecture:** Conversion moves into the Store's row scanners (reads) and its path-taking writes. `Resolve` and `Rel` become private. Bindings and the Sitting Tracker stop converting.

**Spec:** `docs/specs/2026-10-21-store-absolute-paths-design.md`. Decisions are cited as **D1–D7**.

## Constraints

- Branch `107-store-absolute-paths`, in a worktree (copy `frontend/dist`, symlink and locally exclude `frontend/node_modules`). Stage `frontend/wailsjs` with `git add -u`.
- No change to what is stored: the columns stay relative to the Collection (D6). The migrations keep using raw column values.
- `RefPath` is untouched (D6).
- Test-first, one slice at a time, at the Store's methods against a real database (D7). The existing binding, Sitting and Queue tests stay unchanged and must keep passing.
- `make test`, `make lint`, `make test-js` and the frontend build pass at every commit.

## Seams

1. **Store reads:** `GetReading`, `ListReading`, `DueReading`, `GetExtract`, `DueExtracts`, `GetCardView`, `DueQueue`.
2. **Store writes:** `UpsertReading`, `UpsertItem`, `UpsertExtract`, `RetireMissing`, `RetireMissingExtracts`, and `Sync` through them.
3. **Existing callers** (bindings, `sitting.Tracker`, `learning.Queue`): no new tests. Their current suites are the regression check.

---

### Task 1: Reads return absolute paths, with one failure policy

**Files:** `internal/learning/reading.go`, `extracts.go`, `store.go`, `queue.go` (`DueReading`), and a new `internal/learning/abspath_test.go`.

**Slices** (one test, then the code for it), with a fixture Store on `t.TempDir()` whose rows are seeded through the existing upserts with relative paths:
1. `TestGetReadingReturnsAnAbsoluteNotesPath`: `filepath.IsAbs`, equal to `<dir>/reading/x.n4l`.
2. `TestListAndDueReadingReturnAbsoluteNotesPaths`.
3. `TestExtractReadsReturnAbsoluteFilePaths` (`GetExtract`, `DueExtracts`).
4. `TestCardReadsReturnAbsoluteFilePaths` (`GetCardView`, `DueQueue`). This introduces `scanCard`.
5. `TestASingleRowThatEscapesTheCollectionIsAnError`: a row with a stored `/elsewhere/x.n4l` (written with raw SQL, the legacy shape) → `GetReading` / `GetExtract` / `GetCardView` return an error naming the id.
6. `TestManyRowReadsSkipARowThatEscapes`: one good and one escaping row → the list holds only the good one.
7. `TestADueQueryFillsItsLimitPastAnEscapingRow`: the first-due row escapes and the second is good → `DueQueue(now, 1)`, `DueExtracts(now, 1)` and `DueReading(now, 1)` each return the good one.

**Implementation notes:**
- `scanReading`, `scanExtract` and `scanCard` take the Store and resolve the path. They return a sentinel-wrapped error (`errUnresolvable`) when it escapes.
- Many-row readers `errors.Is` that sentinel, then log and skip.
- Limited due queries read without `LIMIT` and stop once `limit` rows have resolved. `LIMIT` is a performance hint only, and these tables are small.

**Check:** `go test -race ./...`. Every existing test still passes, because the bindings' own `Resolve` calls are idempotent on an absolute path (`collectionpath.Resolve` accepts an absolute path under the root).

**Commit:** `refactor(learning): Store reads return absolute paths; one policy for rows that escape the Collection`.

---

### Task 2: Writes accept absolute paths

**Files:** `internal/learning/reading.go`, `extracts.go`, `store.go`, `sync.go`, and `abspath_test.go`.

**Slices:**
1. `TestUpsertReadingStoresAnAbsoluteNotesPathRelative`: upsert with `<dir>/reading/x.n4l`; the raw column (read via `s.db`, as `paths_test.go` already does) is `reading/x.n4l`; `GetReading` returns the absolute path.
2. `TestUpsertItemAndExtractStoreRelative`: same, for `items.file_path` and `extracts.file_path`.
3. `TestRetireMissingTakesAnAbsolutePath`: two items in one file; `RetireMissing(<abs>, keep one)` retires the other. Same for `RetireMissingExtracts`.
4. `TestAWriteOutsideTheCollectionIsRefused`: `UpsertReading` with `/elsewhere/x.n4l` → an error, and no row.

**Implementation notes:**
- Each write calls `s.rel(path)` first.
- `Sync` keeps its early check (it must not inject ids into a file outside the Collection) through the now-private `store.rel`, and passes the absolute path to the Store.

**Check:** `go test -race ./...`. Expected: PASS. The bindings' `upsertReadingRelative` still works, because a relative path is accepted (D2).

**Commit:** `refactor(learning): Store writes take absolute paths and store them relative`.

---

### Task 3: Callers stop converting; `Resolve` and `Rel` go private

**Files:**
- `reading_methods.go`: delete `upsertReadingRelative` and its 4 callers' use of it, and the `Resolve` calls at :204, :311, :557.
- `study_methods.go`: :57, :98, :120.
- `learning_methods.go`: delete `resolveCardViewPaths` and its calls.
- `queue_methods.go`: :30, :38, :43.
- `notebook_methods.go`: :224.
- `internal/sitting/tracker.go`: :68.
- `internal/learning/paths.go`: `Resolve` → `resolve`, `Rel` → `rel`.

**Steps:**
1. Delete each conversion. Where a binding skipped or errored on a resolve failure, the Store now does the same (D3): a single-row error surfaces through the existing `err` path, and lists arrive already filtered. Remove comments that explain the old conversion.
2. Rename `Resolve` and `Rel` to private. `go build ./...` shows any caller outside `internal/learning`; there must be none left.
3. `go test -race ./... && make lint && make test-js && (cd frontend && npx vite build)`. Expected: PASS with **no test edits outside `internal/learning`**. If a binding test fails, the Store's policy differs from that binding's old one: fix the Store, or stop and raise it. Do not edit the binding test.
4. `grep -rn "\.Resolve(\|\.Rel(\|upsertReadingRelative\|resolveCardViewPaths" --include=*.go . | grep -v "_test.go"` finds only `collectionpath`, `Handles.Path`, `content` and `media` uses, which handle `RefPath` and other unrelated paths.

**Commit:** `refactor: callers stop converting stored paths; Store.Resolve and Rel are private`.

## Manual check

After merging, open a Collection in `make dev`, read a Source, take an Extract, process it, and review a Card. Nothing should look different.
