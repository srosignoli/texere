# Recursive Collection Scanning & Watching — Design

**Date:** 2026-06-22
**Branch context:** follows the incremental-reading work (011-*), which writes notes into `<collectionDir>/reading/`.

## Problem

The collection folder is read non-recursively, so `.n4l` files in subdirectories never appear in the application browser. Concretely:

- `ScanCollection` (`internal/content/scanner.go`) skips subdirectories — it reads only the top level.
- `WatchCollection` (`internal/content/watcher.go`) calls `fw.Add(dir)` once; `fsnotify` is non-recursive on Linux, so subdirectories are not watched.

The incremental-reading feature writes per-source notes to `<collectionDir>/reading/<name>.n4l` (`reading_methods.go`). A code comment there already acknowledges "reading/ is an unwatched subtree (the collection watcher is non-recursive)." As a result, imported reading notes are invisible in the file browser.

A collection directory also contains non-content subdirectories: `sources/` (imported PDF/HTML), `sst_data/` (BadgerDB graph store), and `bleve/` (full-text index). Only `reading/` (and any future user-made folder) holds `.n4l` files.

## Goal

Make the browser show every `.n4l` file anywhere under the collection root, kept live as files are added, changed, or removed — including in subdirectories that do not exist yet when a collection is opened.

## Decisions

- **Traversal:** full recursion to any depth, with a denylist of non-content directories.
- **Display:** flat, sorted list (as today); subfolder files labeled with their collection-relative path, top-level files with the bare base name.
- **Watching:** fully recursive and self-healing — watches are added for subdirectories that appear after startup.

## Denylist

A directory is excluded (not descended into, not watched) when its base name is:

- `sst_data` — BadgerDB graph store
- `bleve` — full-text index
- `sources` — imported binary assets (PDF/HTML)

…or begins with `.` (e.g. `.git`). The collection root itself is never excluded.

Excluding `sources/` and `sst_data/` also keeps the `< 500 ms / ≤ 500 files` scan performance contract intact by avoiding the large/binary trees.

## Data model

Add one field to `ChapterSummary` (`internal/content/types.go`):

```go
RelPath string // collection-relative, slash-normalized path, e.g. "reading/Foo.n4l"
```

`FilePath` (absolute) remains the stable key used for deduplication and selection throughout the backend and frontend. `FileName` (base name) is unchanged. `RelPath` is new and drives only the browser label. Both the scan and the watch populate it.

This requires regenerating the Wails `frontend/wailsjs/go/models` bindings.

## Backend — recursive scan (`scanner.go`)

Replace the flat `os.ReadDir` loop with `filepath.WalkDir(dir, …)`:

- Skip excluded directories via `fs.SkipDir` (see Denylist).
- For every `.n4l` file at any depth, build its `ChapterSummary`, setting `RelPath = filepath.ToSlash(rel)` where `rel = filepath.Rel(dir, path)`.
- Sort by lowercased `RelPath` (so folders group stably), replacing the current sort by `FileName`.
- A per-file read error still yields a summary with a non-nil `ScanError`, as today. A directory-level error (root missing/unreadable) is still returned as the function error.

## Backend — recursive, self-healing watch (`watcher.go`)

**At start:** `WalkDir` the tree (same denylist) and `fw.Add` every surviving directory, including the root.

**In the event loop:**

- **Directory `Create`:** if the created path is a directory and not denylisted, `fw.Add` it (and recurse to add watches for any nested subdirectories created in the same burst), then scan it and emit one `EventAdded` per `.n4l` file already inside. This is what lets a freshly-created `reading/` self-heal without a re-open.
- **`.n4l` file events** (Create/Write/Remove/Rename): unchanged in behavior, but now fire at any depth and carry `RelPath`.

To support "new directory containing N files → N events," `translateEvent` changes from returning a single `*CollectionEvent` to returning `[]CollectionEvent`; the event loop ranges over the slice and sends each. The watcher needs the collection root threaded through (it is currently the ignored `_ string` parameter of `translateEvent`) in order to compute `RelPath`.

The emitted `CollectionEvent.Chapters` entries carry the populated `RelPath`.

## Known limitation (out of scope)

Deleting a whole subdirectory does not reliably emit a `Removed` event per contained file — `fsnotify` drops the directory's watch and does not enumerate the children. Those browser entries go stale until the collection is re-opened. Directory-removal reconciliation is intentionally not built here.

## Frontend — display (`ColumnChapters.svelte`)

The list stays flat, sorted, and keyed by `FilePath`. Only the label expression changes:

```svelte
<span class="file-name">{summary.RelPath?.includes('/') ? summary.RelPath : summary.FileName}</span>
```

- Top-level files render the bare base name, exactly as today.
- Subfolder files render their collection-relative path (e.g. `reading/Foo.n4l`), disambiguating same-named files in different folders.

The existing `title={summary.FilePath}` tooltip already shows the absolute path on hover. Collapse and selection logic are keyed on `FilePath` and are untouched.

## Frontend — event handling (`App.svelte`)

No logic changes. The `collection:change` handler already upserts/removes by `FilePath`, and the optimistic new-file append path already deduplicates against the eventual watcher event by `FilePath`. Every emitted `ChapterSummary` now additionally carries `RelPath`, which rides along into the store and reaches the label automatically. An `EventAdded` for `reading/*.n4l` flows through the existing upsert branch unchanged.

## Sorting

The backend sorts summaries by `RelPath`. The store preserves backend order. The optimistic-append path (`App.svelte`, new-file creation) pushes a freshly created file to the end of the list; this is a pre-existing, momentary cosmetic quirk corrected by the next full scan / re-open. It is left as-is — no client-side resort is added.

## Testing

**`ScanCollection` (`scanner_test.go`):**
- A `.n4l` file in a subdirectory is included, with the correct `RelPath`.
- Files under denylisted directories (`sst_data`, `bleve`, `sources`) and hidden directories are excluded.
- Top-level files keep `RelPath` equal to their base name (no slash).
- Results are sorted by lowercased `RelPath`.

**`WatchCollection` (watcher test):**
- A `.n4l` dropped into a pre-existing watched subdirectory emits `EventAdded` with the right `RelPath`.
- A subdirectory created after the watcher starts is self-healed: dropping a `.n4l` into it afterward emits `EventAdded`.
- A new subdirectory that already contains a `.n4l` at creation time emits an `EventAdded` for that file.

**Frontend:** the label change is small and verified manually (a `reading/*.n4l` shows its relative path; a top-level file shows its bare name).

## Files touched

- `internal/content/types.go` — add `RelPath`.
- `internal/content/scanner.go` — recursive `WalkDir`, denylist, `RelPath`, sort.
- `internal/content/watcher.go` — recursive add at start, self-healing on dir-create, `translateEvent` returns a slice, `RelPath`.
- `frontend/wailsjs/go/models.*` — regenerated bindings.
- `frontend/src/components/ColumnChapters.svelte` — label expression.
- Tests: `scanner_test.go`, watcher test.
