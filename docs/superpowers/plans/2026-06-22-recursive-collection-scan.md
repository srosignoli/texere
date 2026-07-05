# Recursive Collection Scanning & Watching Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show every `.n4l` file anywhere under the collection root in the browser, kept live as files appear, change, or are removed — including in subdirectories created after the collection is opened.

**Architecture:** Replace the flat top-level directory read with a recursive `filepath.WalkDir`, guarded by a denylist of non-content directories. Make the `fsnotify` watcher recursive (add a watch per directory) and self-healing (watch new directories as they are created, surfacing any `.n4l` files already inside). Carry a new collection-relative `RelPath` field on each summary so the browser can label subfolder files unambiguously.

**Tech Stack:** Go 1.22 (`io/fs`, `path/filepath`, `github.com/fsnotify/fsnotify`), Svelte 3.49, Wails v2.12.

## Global Constraints

- Go: standard `gofmt` formatting; imports grouped stdlib / external / internal.
- No new dependencies — `io/fs` is stdlib; `fsnotify` is already in `go.mod`.
- No CGo.
- `ScanCollection` performance contract: completes in < 500 ms for ≤ 500 files (the denylist keeps the walk out of `sst_data/`, `bleve/`, `sources/`).
- `FilePath` (absolute path) stays the stable dedup/selection key everywhere — backend and frontend. `RelPath` is additive and drives only the browser label.

---

### Task 1: Recursive scan + RelPath field + denylist

**Files:**
- Modify: `internal/content/types.go` (add `RelPath` to `ChapterSummary`)
- Modify: `internal/content/scanner.go` (recursive `WalkDir`, denylist helper, `RelPath`, sort)
- Test: `internal/content/parser_test.go` (where the existing `ScanCollection` tests live)

**Interfaces:**
- Consumes: existing `scanChapterTitles(path string) ([]string, error)` (unchanged).
- Produces:
  - `ChapterSummary.RelPath string` — collection-relative, slash-normalized (e.g. `"reading/Foo.n4l"`; for a top-level file it equals the base name, no slash).
  - `func isExcludedDir(name string) bool` — true for `sst_data`, `bleve`, `sources`, or any name beginning with `.`. Used here and by Task 2.
  - `ScanCollection(dir string) ([]ChapterSummary, error)` — now recursive; results sorted by lowercased `RelPath`.

- [ ] **Step 1: Add the `RelPath` field**

In `internal/content/types.go`, in the `ChapterSummary` struct, add the field after `FileName`:

```go
// ChapterSummary is the lightweight result of the Phase 1 scan (title extraction only).
type ChapterSummary struct {
	FilePath  string   // Absolute path
	FileName  string   // Base name (for display)
	RelPath   string   // Collection-relative, slash-normalized path (e.g. "reading/Foo.n4l")
	Chapters  []string // Chapter titles in file order; empty if no "- title" found
	ScanError error    // Non-nil if file could not be read
}
```

- [ ] **Step 2: Write the failing recursive-scan test**

Add to `internal/content/parser_test.go`:

```go
func TestScanCollectionRecursive(t *testing.T) {
	dir := t.TempDir()

	// Top-level file.
	if err := os.WriteFile(filepath.Join(dir, "top.n4l"), []byte("- a\nnode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// reading/ subdir file — must be discovered.
	if err := os.MkdirAll(filepath.Join(dir, "reading"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reading", "deep.n4l"), []byte("- b\nnode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Denylisted + hidden dirs containing .n4l — must be ignored.
	for _, d := range []string{"sst_data", "bleve", "sources", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, d, "x.n4l"), []byte("- c\nnode\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	summaries, err := ScanCollection(dir)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, s := range summaries {
		got[s.RelPath] = true
	}
	if len(summaries) != 2 {
		t.Fatalf("want 2 summaries, got %d: %v", len(summaries), got)
	}
	if !got["top.n4l"] {
		t.Errorf("missing top-level file; got %v", got)
	}
	if !got["reading/deep.n4l"] {
		t.Errorf("missing subdir file; got %v", got)
	}
	// Top-level RelPath is the bare base name (no slash); subdir RelPath is slashed.
	for _, s := range summaries {
		switch s.FileName {
		case "top.n4l":
			if s.RelPath != "top.n4l" {
				t.Errorf("top-level RelPath = %q, want %q", s.RelPath, "top.n4l")
			}
		case "deep.n4l":
			if s.RelPath != "reading/deep.n4l" {
				t.Errorf("subdir RelPath = %q, want %q", s.RelPath, "reading/deep.n4l")
			}
		}
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/content/ -run TestScanCollectionRecursive -v`
Expected: FAIL — the denylisted/hidden/subdir files are currently either skipped entirely (subdirs ignored → only `top.n4l` found, count 1) or `RelPath` is empty. Either way the assertions fail. (It may also fail to compile until Step 1's field exists — if so, Step 1 must be in place first.)

- [ ] **Step 4: Rewrite `ScanCollection` recursively**

Replace the body of `internal/content/scanner.go` `ScanCollection` and add the helper. The file's imports must include `io/fs` (grouped with stdlib). New `scanner.go` (top portion):

```go
package content

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanCollection recursively scans dir for .n4l files and returns a sorted
// (case-insensitive by collection-relative path) slice of ChapterSummary, each
// containing the chapter titles extracted from that file.
//
// Non-.n4l files are ignored. Directories named sst_data, bleve, sources, or
// beginning with "." are not descended into (see isExcludedDir). The collection
// root itself is always scanned.
// If a file cannot be read, its ChapterSummary.ScanError is non-nil; it is still
// included. Returns a directory-level error only if dir does not exist or cannot
// be read.
//
// Performance contract: completes in < 500 ms for ≤ 500 files.
func ScanCollection(dir string) ([]ChapterSummary, error) {
	var results []ChapterSummary
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err // root missing/unreadable → propagate
			}
			return fs.SkipDir // unreadable subtree → skip it, keep going
		}
		if d.IsDir() {
			if path != dir && isExcludedDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".n4l") {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = d.Name()
		}
		summary := ChapterSummary{
			FilePath: path,
			FileName: d.Name(),
			RelPath:  filepath.ToSlash(rel),
		}
		summary.Chapters, summary.ScanError = scanChapterTitles(path)
		results = append(results, summary)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("content: ScanCollection: %w", walkErr)
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].RelPath) < strings.ToLower(results[j].RelPath)
	})

	return results, nil
}

// isExcludedDir reports whether a directory (by base name) is a non-content
// directory that the scan and watch must not descend into: the BadgerDB graph
// store, the full-text index, the imported-binary assets dir, or any hidden
// directory. Shared by ScanCollection and the collection watcher.
func isExcludedDir(name string) bool {
	switch name {
	case "sst_data", "bleve", "sources":
		return true
	}
	return strings.HasPrefix(name, ".")
}
```

Leave `scanChapterTitles` below unchanged.

- [ ] **Step 5: Run the new test and the existing scan tests**

Run: `go test ./internal/content/ -run TestScanCollection -v`
Expected: PASS for `TestScanCollectionRecursive`, `TestScanCollectionEmpty`, `TestScanCollectionSorted`, `TestScanCollectionMultiChapter` (all top-level files keep `RelPath` == base name, so the existing FileName-based sort assertion still holds).

- [ ] **Step 6: gofmt + full package test**

Run: `gofmt -w internal/content/scanner.go internal/content/types.go && go test ./internal/content/`
Expected: PASS, no diff from gofmt.

- [ ] **Step 7: Commit**

```bash
git add internal/content/types.go internal/content/scanner.go internal/content/parser_test.go
git commit -m "feat(content): recursive ScanCollection with RelPath + denylist"
```

---

### Task 2: Recursive, self-healing watcher

**Files:**
- Modify: `internal/content/watcher.go` (recursive add at start; `translateEvent` returns a slice; self-heal on directory create; populate `RelPath`)
- Create: `internal/content/watcher_test.go`

**Interfaces:**
- Consumes from Task 1: `isExcludedDir(name string) bool`; `ChapterSummary.RelPath`; `scanChapterTitles`.
- Produces (internal helpers, same package):
  - `func addWatchRecursive(fw *fsnotify.Watcher, root, dir string) error`
  - `func summaryFor(root, path string) []ChapterSummary` — one-element slice, or `nil` if the file can't be scanned (mirrors today's "Chapters set only when err == nil").
  - `func scanSubtreeAdded(root, dir string) []CollectionEvent` — one `EventAdded` per `.n4l` already inside a newly created directory.
  - `translateEvent(fw *fsnotify.Watcher, ev fsnotify.Event, root string) []CollectionEvent` (signature change: was `(fsnotify.Event, string) *CollectionEvent`).
- Unchanged public surface: `WatchCollection(dir string) (*CollectionWatcher, error)`, `CollectionWatcher`, `CollectionEvent`.

- [ ] **Step 1: Write the failing watcher tests**

Create `internal/content/watcher_test.go`:

```go
package content

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitForEvent reads one CollectionEvent or fails the test on timeout.
func waitForEvent(t *testing.T, cw *CollectionWatcher, timeout time.Duration) CollectionEvent {
	t.Helper()
	select {
	case ev, ok := <-cw.Events:
		if !ok {
			t.Fatal("events channel closed")
		}
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for event")
		return CollectionEvent{}
	}
}

// A .n4l dropped into a pre-existing subdirectory is watched and reported
// with its collection-relative path.
func TestWatchCollectionSubdirFileAdded(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "reading")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cw, err := WatchCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cw.Close()

	if err := os.WriteFile(filepath.Join(sub, "foo.n4l"), []byte("- ch\nnode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev := waitForEvent(t, cw, 2*time.Second)
	if ev.Kind != EventAdded {
		t.Fatalf("want EventAdded, got %v", ev.Kind)
	}
	if len(ev.Chapters) != 1 || ev.Chapters[0].RelPath != "reading/foo.n4l" {
		t.Fatalf("want RelPath reading/foo.n4l, got %+v", ev.Chapters)
	}
}

// A subdirectory created after the watcher starts is self-healed: a .n4l
// dropped into it afterward is reported.
func TestWatchCollectionSelfHealNewSubdir(t *testing.T) {
	dir := t.TempDir()
	cw, err := WatchCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cw.Close()

	sub := filepath.Join(dir, "reading")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Let the watcher process the dir-create and add the new watch before
	// the file lands (the dir-create itself emits no event, so we cannot
	// synchronize on the channel here).
	time.Sleep(150 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(sub, "bar.n4l"), []byte("- ch\nnode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev := waitForEvent(t, cw, 3*time.Second)
	if ev.Kind != EventAdded || len(ev.Chapters) != 1 || ev.Chapters[0].RelPath != "reading/bar.n4l" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

// A directory that already contains a .n4l when it appears (moved in
// atomically) reports an EventAdded for the contained file.
func TestWatchCollectionMovedInSubdirWithFile(t *testing.T) {
	dir := t.TempDir()
	cw, err := WatchCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cw.Close()

	staging := t.TempDir()
	src := filepath.Join(staging, "reading")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "baz.n4l"), []byte("- ch\nnode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, filepath.Join(dir, "reading")); err != nil {
		t.Fatal(err)
	}
	ev := waitForEvent(t, cw, 3*time.Second)
	if ev.Kind != EventAdded || len(ev.Chapters) != 1 || ev.Chapters[0].RelPath != "reading/baz.n4l" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/content/ -run TestWatchCollection -v`
Expected: FAIL — today the watcher only watches the root, so dropping a file into `reading/` produces no event and the tests time out. (May also fail to compile once Step 3's signature change is partially applied — apply Step 3 fully before re-running.)

- [ ] **Step 3: Rewrite `watcher.go`**

Replace the entire contents of `internal/content/watcher.go` with:

```go
package content

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// WatchCollection starts a recursive filesystem watcher on dir. Returns a
// *CollectionWatcher whose Events channel emits CollectionEvent values as .n4l
// files are added, removed, renamed, or modified anywhere under dir (excluding
// the directories rejected by isExcludedDir).
//
// At start, a watch is added for every non-excluded directory in the tree. When
// a new directory is created later, it is watched too, and any .n4l files
// already inside it are surfaced as EventAdded — so subtrees created after open
// (e.g. reading/ on first import) self-heal without a re-open.
//
// Each emitted ChapterSummary carries a collection-relative RelPath.
// Call CollectionWatcher.Close() to stop the watcher and close both channels.
// Returns an error if dir does not exist or the OS watcher cannot be initialised.
func WatchCollection(dir string) (*CollectionWatcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("content: WatchCollection: %w", err)
	}
	if err := addWatchRecursive(fw, dir, dir); err != nil {
		fw.Close()
		return nil, fmt.Errorf("content: WatchCollection: %w", err)
	}

	events := make(chan CollectionEvent, 16)
	errors := make(chan error, 4)

	go func() {
		defer close(events)
		defer close(errors)
		for {
			select {
			case ev, ok := <-fw.Events:
				if !ok {
					return
				}
				for _, ce := range translateEvent(fw, ev, dir) {
					events <- ce
				}
			case err, ok := <-fw.Errors:
				if !ok {
					return
				}
				errors <- err
			}
		}
	}()

	cw := &CollectionWatcher{
		Events: events,
		Errors: errors,
		close:  fw.Close,
	}
	return cw, nil
}

// addWatchRecursive adds a watch for dir and every non-excluded directory below
// it. root is the collection root, which is never excluded regardless of name.
func addWatchRecursive(fw *fsnotify.Watcher, root, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			return fs.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && isExcludedDir(d.Name()) {
			return fs.SkipDir
		}
		return fw.Add(path)
	})
}

// translateEvent maps one fsnotify.Event to zero or more CollectionEvents.
//
// A directory-create self-heals: the new subtree is watched and its existing
// .n4l files are surfaced as EventAdded (which may be several events). All other
// events concern a single .n4l file.
func translateEvent(fw *fsnotify.Watcher, ev fsnotify.Event, root string) []CollectionEvent {
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if isExcludedDir(filepath.Base(ev.Name)) {
				return nil
			}
			_ = addWatchRecursive(fw, root, ev.Name)
			return scanSubtreeAdded(root, ev.Name)
		}
	}

	if !strings.EqualFold(filepath.Ext(ev.Name), ".n4l") {
		return nil
	}

	switch {
	case ev.Has(fsnotify.Create):
		return []CollectionEvent{{Kind: EventAdded, Path: ev.Name, Chapters: summaryFor(root, ev.Name)}}
	case ev.Has(fsnotify.Remove):
		return []CollectionEvent{{Kind: EventRemoved, Path: ev.Name}}
	case ev.Has(fsnotify.Rename):
		return []CollectionEvent{{Kind: EventRenamed, Path: ev.Name, Chapters: summaryFor(root, ev.Name)}}
	case ev.Has(fsnotify.Write):
		// Re-scan chapter titles so the file tree updates when chapters are
		// added/removed/renamed by an editor save or external tool. Cheap:
		// reads only header-looking lines, not the whole node body.
		return []CollectionEvent{{Kind: EventModified, Path: ev.Name, Chapters: summaryFor(root, ev.Name)}}
	}
	return nil
}

// summaryFor builds the single-element Chapters slice attached to an Added /
// Renamed / Modified event. Returns nil when the file cannot be scanned, so the
// frontend's "Chapters present" guards behave exactly as before.
func summaryFor(root, path string) []ChapterSummary {
	chapters, err := scanChapterTitles(path)
	if err != nil {
		return nil
	}
	rel, relErr := filepath.Rel(root, path)
	if relErr != nil {
		rel = filepath.Base(path)
	}
	return []ChapterSummary{{
		FilePath: path,
		FileName: filepath.Base(path),
		RelPath:  filepath.ToSlash(rel),
		Chapters: chapters,
	}}
}

// scanSubtreeAdded walks a newly appeared directory and returns one EventAdded
// per .n4l file inside it (respecting the denylist for nested directories).
func scanSubtreeAdded(root, dir string) []CollectionEvent {
	var out []CollectionEvent
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			return fs.SkipDir
		}
		if d.IsDir() {
			if path != dir && isExcludedDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".n4l") {
			return nil
		}
		out = append(out, CollectionEvent{Kind: EventAdded, Path: path, Chapters: summaryFor(root, path)})
		return nil
	})
	return out
}
```

- [ ] **Step 4: Run the watcher tests to verify they pass**

Run: `go test ./internal/content/ -run TestWatchCollection -v`
Expected: PASS for all three watcher tests.

- [ ] **Step 5: gofmt + full package test**

Run: `gofmt -w internal/content/watcher.go && go test ./internal/content/`
Expected: PASS, no gofmt diff.

- [ ] **Step 6: Build the whole backend (catch callers of the old `translateEvent` signature)**

Run: `go build ./...`
Expected: success. (`translateEvent` is unexported and only called inside `watcher.go`; no external caller exists. `consumeWatchEvents` in `methods.go` consumes `CollectionEvent` values off the channel and is unaffected.)

- [ ] **Step 7: Commit**

```bash
git add internal/content/watcher.go internal/content/watcher_test.go
git commit -m "feat(content): recursive self-healing collection watcher"
```

---

### Task 3: Frontend label for subfolder files

**Files:**
- Modify: `frontend/wailsjs/go/models.ts` (add `RelPath` to the generated `ChapterSummary` type)
- Modify: `frontend/src/components/ColumnChapters.svelte:62` (label expression)

**Interfaces:**
- Consumes from Tasks 1 & 2: each `ChapterSummary` delivered over the Wails bridge now includes a `RelPath` string at runtime.
- Produces: no new interfaces — purely presentational.

**Note on bindings:** `RelPath` reaches the frontend at runtime automatically because Wails serializes the Go struct to JSON; the `models.ts` edit only keeps the TypeScript type accurate. The canonical regeneration command is `wails generate module` (run from the repo root), which rewrites `frontend/wailsjs/`. If that CLI is available, running it instead of hand-editing is fine and will produce the same field. The manual edit below is the deterministic fallback.

- [ ] **Step 1: Add `RelPath` to the generated type**

In `frontend/wailsjs/go/models.ts`, update the `ChapterSummary` class. Add the field declaration after `FileName` and the constructor assignment after the `FileName` assignment:

```ts
	export class ChapterSummary {
	    FilePath: string;
	    FileName: string;
	    RelPath: string;
	    Chapters: string[];
	    ScanError: any;

	    static createFrom(source: any = {}) {
	        return new ChapterSummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.FilePath = source["FilePath"];
	        this.FileName = source["FileName"];
	        this.RelPath = source["RelPath"];
	        this.Chapters = source["Chapters"];
	        this.ScanError = source["ScanError"];
	    }
	}
```

- [ ] **Step 2: Change the file label to prefer `RelPath` for subfolder files**

In `frontend/src/components/ColumnChapters.svelte`, find line 62:

```svelte
            <span class="file-name">{summary.FileName}</span>
```

Replace it with:

```svelte
            <span class="file-name">{summary.RelPath?.includes('/') ? summary.RelPath : summary.FileName}</span>
```

Top-level files (no slash in `RelPath`) render the bare base name exactly as before; subfolder files render their collection-relative path (e.g. `reading/Foo.n4l`). The existing `title={summary.FilePath}` tooltip on the row already shows the absolute path on hover.

- [ ] **Step 3: Build the frontend to verify it compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds with no type errors.

- [ ] **Step 4: Manual verification**

Build and run the app (`make dev` from repo root), open a collection that has a `reading/*.n4l` file (import a text source if needed, or drop a `.n4l` into `<collection>/reading/`). Confirm:
- The reading file appears in the left file column without re-opening the collection.
- Its label reads `reading/<name>.n4l`, while top-level files still show their bare name.
- Hovering the row shows the absolute path.

- [ ] **Step 5: Commit**

```bash
git add frontend/wailsjs/go/models.ts frontend/src/components/ColumnChapters.svelte
git commit -m "feat(browser): label subfolder .n4l files with their relative path"
```

---

## Out of scope (documented limitation)

Deleting a whole subdirectory does not reliably emit a `Removed` event per contained file — `fsnotify` drops the directory's watch without enumerating its children, so those browser entries go stale until the collection is re-opened. Directory-removal reconciliation is intentionally not built here.

## Self-Review

- **Spec coverage:**
  - Recursion + denylist → Task 1 (`WalkDir`, `isExcludedDir`). ✓
  - `RelPath` data model → Task 1 Step 1 (Go) + Task 3 Step 1 (TS). ✓
  - Recursive scan sort by `RelPath` → Task 1 Step 4. ✓
  - Recursive watch at start + self-healing on dir-create + `translateEvent` returns slice → Task 2. ✓
  - Frontend flat-list relative-path label → Task 3 Step 2. ✓
  - Frontend event handling unchanged (rides along) → no task needed; verified by Task 3 Step 4 manual check. ✓
  - Sorting quirk left as-is → no task (explicitly nothing to do). ✓
  - Known limitation documented → "Out of scope" section. ✓
- **Placeholder scan:** No TBD/TODO; every code step shows full code; every run step has an expected result. ✓
- **Type consistency:** `RelPath` (Go field, TS field, Svelte access) consistent; `isExcludedDir`, `addWatchRecursive`, `summaryFor`, `scanSubtreeAdded`, and the new `translateEvent(fw, ev, root)` signature are referenced consistently between Tasks 1 and 2. ✓
