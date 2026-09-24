# Collection Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the open Collection one owner, a `collection.Session` in `internal/collection`, so the lifecycle, locking, watcher, self-write and path resolution live in one module instead of being re-derived by ~50 App methods.

**Architecture:** `internal/collection.Session` owns the learning store, search, explorer, watcher, open file and self-write set, behind `Open` / `Close` / `Read(func(Handles) error)` / `Sync` / `RebuildGraph` / `SelfWrite` / `SetOpenFile`. App keeps one `*collection.Session` behind a small mutex and reaches it through `withCollection` / `inCollection`. Methods move over file by file; until the last task App mirrors the Session's handles into its old fields so unmigrated methods keep working.

**Tech Stack:** Go 1.25 (`go.mod`), Wails v2 (root package only), SSTorytime-KV/BadgerDB + Bleve (`internal/search`), SQLite via `internal/learning`, fsnotify via `internal/content`.

**Spec:** `docs/superpowers/specs/2026-09-23-collection-session-design.md` (glossary: `CONTEXT.md`)

## Global Constraints

- Branch: `099-collection-session`, created from `main`. One commit per task; `go test ./...` passes after every commit.
- `internal/collection` must not import `github.com/wailsapp/wails/...`.
- No new dependencies. Pure Go only (CLAUDE.md).
- `gofmt`; imports grouped stdlib / external / internal (CLAUDE.md).
- Commit messages follow the repo style (`feat(collection): …`, `refactor(app): …`) and end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **Lock rule, applies to every task:** code running inside a `Read` callback (directly or through `withCollection` / `inCollection`) must never call `a.session()`, `a.withCollection`, `inCollection`, `a.currentDir()`, `a.selfWrite`, any exported App binding, or any `Session` method. Inside a callback use only `h` (`Handles`) and `a.registry` / `a.emit` / `a.ctx`. A nested read lock deadlocks as soon as a writer (switch or rebuild) is waiting.
- **Blocking rule:** file dialogs, network fetches and LLM/shell streams never run inside a callback. Do them before or after, and pass plain values across.
- Frontend event names do not change: `collection:change`, `file:modified-externally`, `file:ids-injected`, `graph:build-started`, `graph:build-complete`.
- The error text for "nothing open" becomes `collection.ErrNoCollection` ("no collection open"). The frontend matches no error strings (verified with grep), so this is safe.
- The demo-only file `demo_seam.go` has `//go:build demo`. Any task that touches it also runs `go test -tags demo ./...`.

## Review Focus

1. **A switch while a query is in flight.** Expected: `OpenCollection` waits for the running call to finish, then switches. No crash and no query against a closed store. Pinned in Task 3 (`TestOpenCollectionWaitsForInFlightCall`).
2. **Ctrl+G while the explorer or search is being used.** Expected: the rebuild waits for in-flight reads, and reads that start during the rebuild wait for it. Pinned in Task 2 (`TestRebuildGraphWaitsForInFlightRead`).
3. **Opening a folder whose `learning.db` is corrupt.** Expected: an error, and the previously open Collection stays open and usable. Pinned in Task 1 (`TestOpenFailsWhenTheLearningStoreCannotOpen`) and Task 3 (`TestOpenCollectionKeepsCurrentWhenNewFails`).
4. **A stored or crafted `refPath` containing `../`.** Expected: refused, and nothing outside the Collection is read. Pinned in Task 5 (`TestSourceReadsRefuseTraversal`).
5. **Re-selecting the Collection that is already open, and files changing after Close.** Expected: re-selecting works (two brief handles on the same SQLite file are fine). After Close, no events are delivered and no goroutine is left behind. Pinned in Task 3 (`TestOpenCollectionTwiceOnSameDir`) and Task 1 (`TestNoEventsAfterClose`).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/collection/session.go` (new) | `Session`, `Open`, `Close`, `Read`, `Handles`, `SelfWrite`, `SetOpenFile`, watcher goroutine, event constants, `ErrNoCollection` |
| `internal/collection/sync.go` (new) | `Sync`, `RebuildGraph`, the shared sync step, `n4lFiles` |
| `internal/collection/session_test.go` (new) | Lifecycle, watcher, self-write, path tests |
| `internal/collection/sync_test.go` (new) | Sync and rebuild tests |
| `session_app.go` (new, root) | App-side glue: `session`, `withCollection`, `inCollection`, `currentDir`, `selfWrite`, `emit`, `switchTo`, `closeSession`, (temporary) `mirrorLegacy` |
| `testapp_test.go` (new, root) | `newTestApp` helper |
| `session_app_test.go` (new, root) | Switch and lifecycle tests at the App level |
| `app.go`, `methods.go`, `graph_methods.go`, `learning_methods.go`, `study_methods.go`, `stats_methods.go`, `reading_methods.go`, `notebook_methods.go`, `search_methods.go`, `query_hints_methods.go`, `media_methods.go`, `demo_seam.go` | Migrated to the Session |

---

### Task 1: `collection.Session` lifecycle, watcher, self-write, paths

**Files:**
- Create: `internal/collection/session.go`
- Test: `internal/collection/session_test.go`

**Interfaces:**
- Consumes: `content.ScanCollection(dir) ([]content.ChapterSummary, error)`, `content.WatchCollection(dir) (*content.CollectionWatcher, error)`, `learning.Open(dir) (*learning.Store, error)`, `search.NewSearchService(dir, reg) *search.SearchService`, `explorer.NewExplorerService(svc, reg) *explorer.ExplorerService`, `collectionpath.Resolve(root, rel) (string, error)`.
- Produces:
  - `var ErrNoCollection = errors.New("no collection open")`
  - `const EventCollectionChange = "collection:change"`, `EventFileModifiedExternally = "file:modified-externally"`, `EventIDsInjected = "file:ids-injected"`
  - `type Notify func(name string, data any)`
  - `func Open(dir string, reg *content.SSTRegistry, notify Notify) (*Session, []content.ChapterSummary, error)`
  - `func (s *Session) Dir() string` (immutable, needs no lock)
  - `func (s *Session) Close()` (idempotent)
  - `func (s *Session) Read(fn func(h Handles) error) error` (returns `ErrNoCollection` after Close)
  - `func (s *Session) SelfWrite(path string, fn func() error) error`
  - `func (s *Session) SetOpenFile(path string)`
  - `type Handles` with `Dir() string`, `Learning() *learning.Store`, `Search() *search.SearchService`, `Explorer() *explorer.ExplorerService`, `Path(rel string) (string, error)`, `SelfWrite(path string, fn func() error) error`

- [ ] **Step 0: Create the branch**

```bash
git checkout -b 099-collection-session
```

- [ ] **Step 1: Write the failing tests**

`internal/collection/session_test.go`:

```go
package collection

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"texere/internal/content"
)

// recorder is the test adapter for Notify: it keeps every event in order.
type recorder struct {
	mu     sync.Mutex
	events []recorded
}

type recorded struct {
	name string
	data any
}

func (r *recorder) notify(name string, data any) {
	r.mu.Lock()
	r.events = append(r.events, recorded{name, data})
	r.mu.Unlock()
}

// count reports how many events named name mention path (any path when path is "").
func (r *recorder) count(name, path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.name != name {
			continue
		}
		if path == "" || eventPath(e.data) == path {
			n++
		}
	}
	return n
}

// waitFor polls until an event named name for path has arrived, or fails the test.
func (r *recorder) waitFor(t *testing.T, name, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r.count(name, path) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %s event for %s within 3s", name, path)
}

func eventPath(data any) string {
	switch d := data.(type) {
	case map[string]string:
		return d["path"]
	case content.CollectionEvent:
		return d.Path
	}
	return ""
}

func emptyRegistry() *content.SSTRegistry {
	return &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openT(t *testing.T, dir string) (*Session, *recorder) {
	t.Helper()
	rec := &recorder{}
	s, _, err := Open(dir, emptyRegistry(), rec.notify)
	if err != nil {
		t.Fatalf("Open(%s): %v", dir, err)
	}
	t.Cleanup(s.Close)
	return s, rec
}

func TestOpenRefusesAMissingDirectory(t *testing.T) {
	if _, _, err := Open(filepath.Join(t.TempDir(), "nope"), emptyRegistry(), nil); err == nil {
		t.Fatal("Open on a missing directory = nil error")
	}
}

func TestOpenFailsWhenTheLearningStoreCannotOpen(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "learning.db"), "this is not a sqlite database, just text padding it out past a header\n")
	if _, _, err := Open(dir, emptyRegistry(), nil); err == nil {
		t.Fatal("Open with a corrupt learning.db = nil error, want an error")
	}
}

func TestOpenReturnsChapterSummaries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.n4l"), "- Chapter A\nalpha\n")
	rec := &recorder{}
	s, sums, err := Open(dir, emptyRegistry(), rec.notify)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(sums) != 1 {
		t.Fatalf("summaries = %d, want 1", len(sums))
	}
	if s.Dir() != dir {
		t.Fatalf("Dir() = %q, want %q", s.Dir(), dir)
	}
}

func TestReadGivesNonNilHandles(t *testing.T) {
	s, _ := openT(t, t.TempDir())
	err := s.Read(func(h Handles) error {
		if h.Learning() == nil || h.Search() == nil || h.Explorer() == nil {
			return errors.New("a handle is nil")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadAfterCloseReturnsErrNoCollection(t *testing.T) {
	s, _, err := Open(t.TempDir(), emptyRegistry(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.Close() // idempotent
	called := false
	err = s.Read(func(Handles) error { called = true; return nil })
	if !errors.Is(err, ErrNoCollection) || called {
		t.Fatalf("Read after Close: err=%v called=%v, want ErrNoCollection and no call", err, called)
	}
}

func TestHandlesPathRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	s, _ := openT(t, dir)
	_ = s.Read(func(h Handles) error {
		if _, err := h.Path("../outside.txt"); err == nil {
			t.Error("Path(\"../outside.txt\") = nil error")
		}
		got, err := h.Path("sources/x.txt")
		if err != nil {
			t.Fatalf("Path(sources/x.txt): %v", err)
		}
		if want := filepath.Join(dir, "sources", "x.txt"); got != want {
			t.Errorf("Path = %q, want %q", got, want)
		}
		return nil
	})
}

func TestWatcherReportsAnExternalChangeToTheOpenFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.n4l")
	writeFile(t, f, "- Ch\nalpha\n")
	s, rec := openT(t, dir)
	s.SetOpenFile(f)

	writeFile(t, f, "- Ch\nalpha\nbeta\n")

	rec.waitFor(t, EventFileModifiedExternally, f)
	if rec.count(EventCollectionChange, f) == 0 {
		t.Error("no collection:change for the modified file")
	}
}

func TestSelfWriteSuppressesModifiedExternally(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.n4l")
	writeFile(t, f, "- Ch\nalpha\n")
	s, rec := openT(t, dir)
	s.SetOpenFile(f)

	err := s.SelfWrite(f, func() error {
		writeFile(t, f, "- Ch\nalpha\nbeta\n")
		// Hold the self-write until the watcher has seen the change, so the
		// assertion below tests suppression and not fsnotify timing.
		rec.waitFor(t, EventCollectionChange, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := rec.count(EventFileModifiedExternally, f); n != 0 {
		t.Fatalf("file:modified-externally fired %d times during a self-write", n)
	}
}

func TestSelfWriteReturnsTheCallbacksError(t *testing.T) {
	s, _ := openT(t, t.TempDir())
	want := errors.New("boom")
	if got := s.SelfWrite("/x", func() error { return want }); !errors.Is(got, want) {
		t.Fatalf("SelfWrite err = %v, want %v", got, want)
	}
}

func TestNoEventsAfterClose(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	s, _, err := Open(dir, emptyRegistry(), rec.notify)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	rec.mu.Lock()
	before := len(rec.events)
	rec.mu.Unlock()

	writeFile(t, filepath.Join(dir, "late.n4l"), "- Late\nx\n")
	time.Sleep(300 * time.Millisecond)

	rec.mu.Lock()
	after := len(rec.events)
	rec.mu.Unlock()
	if after != before {
		t.Fatalf("%d events delivered after Close", after-before)
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/collection/`
Expected: FAIL, because the build fails with `undefined: Open` (and `Handles`, `ErrNoCollection`, and so on).

- [ ] **Step 3: Implement `internal/collection/session.go`**

```go
// Package collection owns a Session: one Collection while it is open in Texere.
//
// A Session holds everything that exists only while a Collection is open: the
// learning store, graph search, the explorer, the filesystem watcher, the file
// open in the editor and the set of paths the app is writing itself. Callers
// reach the stores through Read, which guarantees none of them closes under
// the caller. See CONTEXT.md for the vocabulary.
package collection

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"texere/internal/collectionpath"
	"texere/internal/content"
	"texere/internal/explorer"
	"texere/internal/learning"
	"texere/internal/search"
)

// ErrNoCollection is returned when no Session is open, or the one asked for has closed.
var ErrNoCollection = errors.New("no collection open")

// Event names sent through Notify. They are part of the frontend contract.
const (
	EventCollectionChange       = "collection:change"
	EventFileModifiedExternally = "file:modified-externally"
	EventIDsInjected            = "file:ids-injected"
)

// Notify receives the Session's events. It is called from the watcher
// goroutine as well as from callers' goroutines, so it must be safe for
// concurrent use and must not call back into the Session.
type Notify func(name string, data any)

// Session is one Collection while it is open.
type Session struct {
	dir      string
	registry *content.SSTRegistry
	notify   Notify

	// mu guards the lifecycle of the stores, not access to their data (each
	// store does its own locking). Read holds it shared; RebuildGraph and
	// Close hold it exclusively, so no store is closed or swapped under a
	// running Read.
	mu       sync.RWMutex
	closed   bool
	learn    *learning.Store
	search   *search.SearchService
	explorer *explorer.ExplorerService

	watcher   *content.CollectionWatcher
	watchDone chan struct{} // closed when consumeWatchEvents returns

	// fileMu guards openFile and selfWrites. It is separate from mu so the
	// watcher goroutine never waits on a Read or a rebuild, which lets Close
	// wait for that goroutine while it holds mu.
	fileMu     sync.Mutex
	openFile   string
	selfWrites map[string]int // ref-counted paths the app is writing itself
}

// Open opens dir as a Session and returns it with the Collection's chapter
// summaries. It fails if dir is not a directory, cannot be scanned, or its
// learning store cannot be opened. A watcher that cannot start is logged and
// the Session runs without live updates.
func Open(dir string, reg *content.SSTRegistry, notify Notify) (*Session, []content.ChapterSummary, error) {
	if notify == nil {
		notify = func(string, any) {}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("collection: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("collection: %s is not a directory", dir)
	}
	sums, err := content.ScanCollection(dir)
	if err != nil {
		return nil, nil, err
	}
	ls, err := learning.Open(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("collection: open learning store: %w", err)
	}
	s := &Session{
		dir:        dir,
		registry:   reg,
		notify:     notify,
		learn:      ls,
		selfWrites: map[string]int{},
	}
	s.search = search.NewSearchService(dir, reg)
	s.explorer = explorer.NewExplorerService(s.search, reg)

	w, err := content.WatchCollection(dir)
	if err != nil {
		log.Printf("collection: no live updates, watcher unavailable: %v", err)
	} else {
		s.watcher = w
		s.watchDone = make(chan struct{})
		go s.consumeWatchEvents(w)
	}
	return s, sums, nil
}

// Dir is the Collection's absolute directory. It never changes for a Session.
func (s *Session) Dir() string { return s.dir }

// Close stops the watcher, waits for its goroutine, and closes every store.
// It waits for running Reads to finish. Calling it again does nothing.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.watcher != nil {
		if err := s.watcher.Close(); err != nil {
			log.Printf("collection: watcher close: %v", err)
		}
		<-s.watchDone
	}
	s.search.Close()
	_ = s.explorer.Close()
	s.learn.Close()
}

// Read runs fn with the Session's handles, holding them open for the call.
// The handles must not be kept or used after fn returns. fn must not call any
// Session method (see Handles). Returns ErrNoCollection once the Session has closed.
func (s *Session) Read(fn func(h Handles) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrNoCollection
	}
	return fn(Handles{s: s})
}

// SelfWrite runs fn with path marked as being written by the app, so the
// watcher does not report the change as external. Safe to call inside Read
// through Handles.SelfWrite. An empty path runs fn unmarked.
func (s *Session) SelfWrite(path string, fn func() error) error {
	if path == "" {
		return fn()
	}
	s.fileMu.Lock()
	s.selfWrites[path]++
	s.fileMu.Unlock()
	defer func() {
		s.fileMu.Lock()
		if n := s.selfWrites[path]; n > 1 {
			s.selfWrites[path] = n - 1
		} else {
			delete(s.selfWrites, path)
		}
		s.fileMu.Unlock()
	}()
	return fn()
}

// SetOpenFile records the file open in the editor, whose external changes
// and ID injections are reported to the frontend.
func (s *Session) SetOpenFile(path string) {
	s.fileMu.Lock()
	s.openFile = path
	s.fileMu.Unlock()
}

func (s *Session) currentOpenFile() string {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	return s.openFile
}

func (s *Session) isExternalChangeToOpenFile(path string) bool {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	_, self := s.selfWrites[path]
	return path == s.openFile && !self
}

// consumeWatchEvents forwards watcher events until the watcher closes.
func (s *Session) consumeWatchEvents(w *content.CollectionWatcher) {
	defer close(s.watchDone)
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			switch ev.Kind {
			case content.EventAdded, content.EventRemoved, content.EventRenamed:
				s.notify(EventCollectionChange, ev)
			case content.EventModified:
				// Always tell the file tree, whoever wrote the file; only an
				// external write to the open file gets the reload prompt.
				s.notify(EventCollectionChange, ev)
				if s.isExternalChangeToOpenFile(ev.Path) {
					s.notify(EventFileModifiedExternally, map[string]string{"path": ev.Path})
				}
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Printf("collection: watcher: %v", err)
		}
	}
}

// Handles are the Session's parts, valid only inside the Read call that
// produced them. Code holding Handles must not call Session methods or App
// bindings: a nested Read deadlocks once a rebuild or a switch is waiting.
type Handles struct{ s *Session }

// Dir is the Collection's absolute directory.
func (h Handles) Dir() string { return h.s.dir }

// Learning is the study store. Never nil.
func (h Handles) Learning() *learning.Store { return h.s.learn }

// Search is graph search. Never nil; it opens the graph lazily.
func (h Handles) Search() *search.SearchService { return h.s.search }

// Explorer is the 2.5D explorer. Never nil.
func (h Handles) Explorer() *explorer.ExplorerService { return h.s.explorer }

// Path turns a collection-relative path (slash- or OS-separated) into an
// absolute one, refusing anything that escapes the Collection.
func (h Handles) Path(rel string) (string, error) {
	return collectionpath.Resolve(h.s.dir, filepath.FromSlash(rel))
}

// SelfWrite is Session.SelfWrite, usable inside a Read.
func (h Handles) SelfWrite(path string, fn func() error) error { return h.s.SelfWrite(path, fn) }
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/collection/ -count=1`
Expected: PASS.

If `TestOpenFailsWhenTheLearningStoreCannotOpen` passes `Open` without an error, `learning.Open` is not touching the file yet. Check `internal/learning/db.go`: `Open` runs its schema migration, and that should fail with "file is not a database". Do not weaken the test. If `learning.Open` really does defer every query, add a `db.Ping()` plus the migration call to `learning.Open` so a corrupt file fails at open time.

Also run `go vet ./internal/collection/`, and `go test -race ./internal/collection/ -count=1` if cgo is available. Expected: no reports.

- [ ] **Step 5: Commit**

```bash
git add internal/collection/session.go internal/collection/session_test.go
git commit -m "feat(collection): a Session that owns the open collection

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: `Session.Sync` and `Session.RebuildGraph`

**Files:**
- Create: `internal/collection/sync.go`
- Test: `internal/collection/sync_test.go`

**Interfaces:**
- Consumes: Task 1's `Session` internals (`mu`, `closed`, `learn`, `search`, `explorer`, `registry`, `SelfWrite`, `currentOpenFile`, `notify`); `learning.Sync(store, files, reg) (learning.SyncResult, error)`; `graph.BuildFromCollection(dir, reg) (*graph.BuildResult, error)`; `(*explorer.ExplorerService).SetSearchService(svc)`.
- Produces:
  - `func (s *Session) Sync() (learning.SyncResult, error)`: syncs every `.n4l` file, with the open file guarded as a self-write, and sends `EventIDsInjected` when IDs were injected into the open file.
  - `func (s *Session) RebuildGraph() (*graph.BuildResult, error)`: runs under the write lock. It closes search, builds, installs a fresh search service in the explorer, then runs the same sync step. It returns the build's result and error (the result may be nil on error). A sync failure is logged, not returned, matching today's `BuildGraph`.

- [ ] **Step 1: Write the failing tests**

`internal/collection/sync_test.go`:

```go
package collection

import (
	"path/filepath"
	"testing"
	"time"

	"texere/internal/content"
)

const clozeNote = "- Ch\n# @cloze\nSystem 1 is {{fast}}\n"

func TestSyncInjectsIDsIntoTheOpenFileAndNotifies(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.n4l")
	writeFile(t, f, clozeNote)
	s, rec := openT(t, dir)
	s.SetOpenFile(f)

	res, err := s.Sync()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.InjectedPaths) != 1 || res.InjectedPaths[0] != f {
		t.Fatalf("InjectedPaths = %v, want [%s]", res.InjectedPaths, f)
	}
	if rec.count(EventIDsInjected, f) != 1 {
		t.Fatalf("file:ids-injected for the open file: %d events, want 1", rec.count(EventIDsInjected, f))
	}
	if rec.count(EventFileModifiedExternally, f) != 0 {
		t.Fatal("the sync's own write was reported as external")
	}
}

func TestSyncDoesNotNotifyForAFileThatIsNotOpen(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "f.n4l"), clozeNote)
	s, rec := openT(t, dir)
	s.SetOpenFile(filepath.Join(dir, "other.n4l"))

	if _, err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	if n := rec.count(EventIDsInjected, ""); n != 0 {
		t.Fatalf("file:ids-injected sent %d times for a file that is not open", n)
	}
}

func TestSyncAfterCloseReturnsErrNoCollection(t *testing.T) {
	s, _, err := Open(t.TempDir(), emptyRegistry(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := s.Sync(); err != ErrNoCollection {
		t.Fatalf("Sync after Close err = %v, want ErrNoCollection", err)
	}
}

// graphFixture writes a one-link collection and opens it with the degraded
// registry the app would load when SSTconfig is missing.
func graphFixture(t *testing.T) *Session {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "c.n4l"), "- Chapter One\n:: ctx ::\nalpha (contains) beta\n")
	reg, _ := content.LoadRegistry(filepath.Join(dir, "SSTconfig")) // degraded is fine
	s, _, err := Open(dir, reg, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestRebuildGraphThenSearchAndExplorerBothSeeTheGraph(t *testing.T) {
	s := graphFixture(t)
	for i := 1; i <= 2; i++ { // a second build must keep both paths working
		if _, err := s.RebuildGraph(); err != nil {
			t.Fatalf("build %d: %v", i, err)
		}
		err := s.Read(func(h Handles) error {
			sg, err := h.Search().Search("alpha")
			if err != nil || len(sg.Nodes) == 0 {
				t.Fatalf("build %d: search nodes=%d err=%v", i, len(sg.Nodes), err)
			}
			lr, err := h.Explorer().Open("alpha", "extended")
			if err != nil || len(lr.Nodes) == 0 || lr.Diagnostics.GraphError != "" {
				t.Fatalf("build %d: explorer nodes=%d err=%v graphError=%q", i, len(lr.Nodes), err, lr.Diagnostics.GraphError)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRebuildGraphSyncsStudyItems(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "f.n4l"), clozeNote)
	s, _ := openT(t, dir)

	if _, err := s.RebuildGraph(); err != nil {
		t.Fatal(err)
	}
	_ = s.Read(func(h Handles) error {
		q, err := h.Learning().DueQueue(time.Now(), 10)
		if err != nil || len(q) != 1 {
			t.Fatalf("due queue after rebuild: len=%d err=%v, want 1", len(q), err)
		}
		return nil
	})
}

func TestRebuildGraphWaitsForInFlightRead(t *testing.T) {
	s := graphFixture(t)
	release := make(chan struct{})
	inRead := make(chan struct{})
	go func() {
		_ = s.Read(func(Handles) error {
			close(inRead)
			<-release
			return nil
		})
	}()
	<-inRead

	done := make(chan struct{})
	go func() {
		_, _ = s.RebuildGraph()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("RebuildGraph finished while a Read was still running")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("RebuildGraph did not finish after the Read ended")
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/collection/ -run 'Sync|Rebuild' -count=1`
Expected: FAIL, with `s.Sync undefined` and `s.RebuildGraph undefined`.

- [ ] **Step 3: Implement `internal/collection/sync.go`**

```go
package collection

import (
	"log"

	"texere/internal/content"
	"texere/internal/graph"
	"texere/internal/learning"
	"texere/internal/search"
)

// Sync discovers study items across the Collection into the learning store.
// Writes into the open file are marked as self-writes, and if IDs were
// injected into it EventIDsInjected tells the editor to reload.
func (s *Session) Sync() (learning.SyncResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return learning.SyncResult{}, ErrNoCollection
	}
	return s.syncLocked()
}

// RebuildGraph rebuilds the graph from the Collection's files and then syncs
// study items. It waits for running Reads, and Reads wait for it.
func (s *Session) RebuildGraph() (*graph.BuildResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrNoCollection
	}
	// Release the BadgerDB lock before the build opens the store for writing.
	s.search.Close()
	br, buildErr := graph.BuildFromCollection(s.dir, s.registry)
	// A fresh service rather than a lazy reopen of the old one: the old one's
	// caches describe the previous graph. The explorer must follow it, or it
	// keeps querying the closed service and shows an empty graph until restart.
	s.search = search.NewSearchService(s.dir, s.registry)
	s.explorer.SetSearchService(s.search)

	if _, err := s.syncLocked(); err != nil {
		log.Printf("collection: sync after build: %v", err)
	}
	return br, buildErr
}

// syncLocked is the sync step shared by Sync and RebuildGraph. The caller holds mu.
func (s *Session) syncLocked() (learning.SyncResult, error) {
	open := s.currentOpenFile()
	var res learning.SyncResult
	err := s.SelfWrite(open, func() error {
		var err error
		res, err = learning.Sync(s.learn, s.n4lFiles(), s.registry)
		return err
	})
	if err != nil {
		return res, err
	}
	if open != "" {
		for _, p := range res.InjectedPaths {
			if p == open {
				s.notify(EventIDsInjected, map[string]string{"path": open})
				break
			}
		}
	}
	return res, nil
}

// n4lFiles lists the Collection's readable .n4l files.
func (s *Session) n4lFiles() []string {
	sums, err := content.ScanCollection(s.dir)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(sums))
	for _, sum := range sums {
		if sum.FilePath != "" && sum.ScanError == nil {
			files = append(files, sum.FilePath)
		}
	}
	return files
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/collection/ -count=1`
Expected: PASS for every test from Task 1 and Task 2.

- [ ] **Step 5: Commit**

```bash
git add internal/collection/sync.go internal/collection/sync_test.go
git commit -m "feat(collection): rebuild and sync as Session operations

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: App lifecycle on the Session

This task puts App's lifecycle on the Session: startup, open/switch, shutdown, build, sync, watcher, open file and self-write. Methods not migrated yet keep reading `a.collectionDir`, `a.learnStore`, `a.searchSvc` and `a.explorerSvc`, which `mirrorLegacy` fills from the current Session. Task 9 deletes the mirror.

**Files:**
- Create: `session_app.go`, `testapp_test.go`, `session_app_test.go`
- Modify: `app.go` (fields, `startup`, `shutdown`, `OpenCollection`, `mediaDir`, `collectionRoot`, `ParseFileForUI`), `methods.go` (remove `beginSelfWrite`/`endSelfWrite`/`isSelfWriting`/`consumeWatchEvents`; `SaveFile`, `CreateNewFile`, `GetInitialCollectionDir`), `graph_methods.go` (`BuildGraph`), `learning_methods.go` (remove `collectionN4LFiles`; `SyncStudyItems`), `reading_methods.go` (`CreateExtract`, `UpdateReadingProgress`: self-write only), `study_methods.go` (`PromoteExtract`: self-write only)
- Test: `build_explorer_test.go`, `learning_methods_test.go` (setup only)

**Interfaces:**
- Consumes: everything Task 1 and Task 2 produce.
- Produces (root package; later tasks use these exact names):
  - `func (a *App) session() (*collection.Session, error)`
  - `func (a *App) withCollection(fn func(h collection.Handles) error) error`
  - `func inCollection[T any](a *App, fn func(h collection.Handles) (T, error)) (T, error)`
  - `func (a *App) currentDir() (string, error)`
  - `func (a *App) selfWrite(path string, fn func() error) error` (outside callbacks only)
  - `func (a *App) emit(name string, data any)` (no-op when `a.ctx` is nil)
  - `func (a *App) switchTo(dir string) ([]content.ChapterSummary, error)`
  - `func (a *App) closeSession()`
  - `func newTestApp(t *testing.T, dir string, reg *content.SSTRegistry) (*App, *learning.Store)` (in `testapp_test.go`; a nil `reg` means an empty registry)

- [ ] **Step 1: Write the failing tests**

`testapp_test.go`:

```go
package main

import (
	"testing"

	"texere/internal/collection"
	"texere/internal/content"
	"texere/internal/learning"
)

// newTestApp returns an App with dir open as its Session, plus that Session's
// learning store for seeding test data. The Session closes when the test ends.
// A nil reg means an empty registry.
//
// Returning the store outside a Read breaks the Handles rule on purpose: the
// test owns the Session's whole lifetime, so nothing can close it early.
func newTestApp(t *testing.T, dir string, reg *content.SSTRegistry) (*App, *learning.Store) {
	t.Helper()
	if reg == nil {
		reg = &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}
	}
	a := &App{registry: reg}
	if _, err := a.switchTo(dir); err != nil {
		t.Fatalf("open collection %s: %v", dir, err)
	}
	t.Cleanup(a.closeSession)
	var st *learning.Store
	if err := a.withCollection(func(h collection.Handles) error { st = h.Learning(); return nil }); err != nil {
		t.Fatal(err)
	}
	return a, st
}
```

`session_app_test.go`:

```go
package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"texere/internal/collection"
)

func corruptCollection(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "learning.db"), []byte("this is not a sqlite database, just text padding it out past a header\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestOpenCollectionSwitches(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, _ := newTestApp(t, t.TempDir(), nil)
	b := t.TempDir()
	if _, err := a.OpenCollection(b); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.currentDir(); got != b {
		t.Fatalf("currentDir = %q, want %q", got, b)
	}
}

func TestOpenCollectionKeepsCurrentWhenNewFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dirA := t.TempDir()
	a, _ := newTestApp(t, dirA, nil)

	if _, err := a.OpenCollection(corruptCollection(t)); err == nil {
		t.Fatal("OpenCollection on a corrupt collection = nil error")
	}
	if got, _ := a.currentDir(); got != dirA {
		t.Fatalf("currentDir = %q, want the previous %q", got, dirA)
	}
	if _, err := a.GetStudyStats(); err != nil {
		t.Fatalf("previous collection unusable after a failed switch: %v", err)
	}
	// newTestApp never writes config, so a failed switch must leave it empty.
	cfg, _ := loadConfig()
	if cfg.CollectionDir != "" {
		t.Fatalf("a failed switch wrote %q to config", cfg.CollectionDir)
	}
}

func TestOpenCollectionTwiceOnSameDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	a, _ := newTestApp(t, dir, nil)
	if _, err := a.OpenCollection(dir); err != nil {
		t.Fatalf("re-opening the open collection: %v", err)
	}
	if _, err := a.GetStudyStats(); err != nil {
		t.Fatalf("collection unusable after re-opening it: %v", err)
	}
}

func TestOpenCollectionWaitsForInFlightCall(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, _ := newTestApp(t, t.TempDir(), nil)
	release := make(chan struct{})
	inCall := make(chan struct{})
	go func() {
		_ = a.withCollection(func(collection.Handles) error {
			close(inCall)
			<-release
			return nil
		})
	}()
	<-inCall

	b := t.TempDir()
	done := make(chan error, 1)
	go func() { _, err := a.OpenCollection(b); done <- err }()
	select {
	case <-done:
		t.Fatal("OpenCollection finished while a call was still using the old collection")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("OpenCollection did not finish after the call ended")
	}
	if got, _ := a.currentDir(); got != b {
		t.Fatalf("currentDir = %q, want %q", got, b)
	}
}

func TestNoCollectionMethodsReturnErrNoCollection(t *testing.T) {
	a := &App{}
	if _, err := a.SyncStudyItems(); !errors.Is(err, collection.ErrNoCollection) {
		t.Errorf("SyncStudyItems err = %v", err)
	}
	if r := a.BuildGraph(); r.Error != "No collection open" {
		t.Errorf("BuildGraph Error = %q", r.Error)
	}
	if a.GetInitialCollectionDir() != "" || a.mediaDir() != "" || a.collectionRoot() != "" {
		t.Error("directory accessors non-empty with no collection open")
	}
}
```

Replace the setup in `build_explorer_test.go` (lines 25-40) with:

```go
	dir := t.TempDir()
	n4l := "- Chapter One\n" +
		":: ctx ::\n" +
		"alpha (contains) beta\n"
	if err := os.WriteFile(filepath.Join(dir, "c.n4l"), []byte(n4l), 0o644); err != nil {
		t.Fatalf("write n4l: %v", err)
	}
	reg, _ := content.LoadRegistry(filepath.Join(dir, "SSTconfig")) // degraded is fine
	app, _ := newTestApp(t, dir, reg)
```

Remove the now-unused `explorer` and `search` imports from that file.

Replace the setup in `learning_methods_test.go` `TestGradeItemAdvancesQueue` (the `learning.Open` block, the `&App{…}` literal and `defer store.Close()`) with:

```go
	a, _ := newTestApp(t, dir, nil)
```

Then drop any unused imports.

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test . -run 'OpenCollection|NoCollection|BuildGraphRewires|GradeItem' -count=1`
Expected: FAIL. The build fails with `undefined: a.switchTo` and similar.

- [ ] **Step 3: Write `session_app.go`**

```go
package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/collection"
	"texere/internal/content"
)

// session returns the open Session, or collection.ErrNoCollection.
//
// Never call it (or withCollection, inCollection, currentDir, selfWrite) from
// inside a Read callback: switchTo holds curMu while it waits for Reads to
// finish, so the nested call would deadlock.
func (a *App) session() (*collection.Session, error) {
	a.curMu.Lock()
	defer a.curMu.Unlock()
	if a.cur == nil {
		return nil, collection.ErrNoCollection
	}
	return a.cur, nil
}

// withCollection runs fn against the open Session's handles.
func (a *App) withCollection(fn func(h collection.Handles) error) error {
	s, err := a.session()
	if err != nil {
		return err
	}
	return s.Read(fn)
}

// inCollection is withCollection for callbacks that return a value.
func inCollection[T any](a *App, fn func(h collection.Handles) (T, error)) (T, error) {
	var out T
	err := a.withCollection(func(h collection.Handles) error {
		var err error
		out, err = fn(h)
		return err
	})
	return out, err
}

// currentDir is the open Collection's directory. It holds no lock afterwards,
// so it suits calls that only need a path and then block (dialogs, downloads).
func (a *App) currentDir() (string, error) {
	s, err := a.session()
	if err != nil {
		return "", err
	}
	return s.Dir(), nil
}

// selfWrite runs fn with path marked as an app write when a Collection is
// open, and runs it plainly otherwise (a bare file has no watcher to fool).
// Outside callbacks only; inside one use h.SelfWrite.
func (a *App) selfWrite(path string, fn func() error) error {
	s, err := a.session()
	if err != nil {
		return fn()
	}
	return s.SelfWrite(path, fn)
}

// emit sends a frontend event. A no-op before startup and in tests.
func (a *App) emit(name string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// switchTo opens dir as a new Session and makes it current.
//
// If dir fails to open, the current Session stays open. Otherwise the old
// Session is closed before the new one is published: SSTorytime-KV keeps
// process-wide graph state, so a query still running on the old Session must
// finish before any query on the new one can start. Callers arriving during
// the swap wait on curMu.
func (a *App) switchTo(dir string) ([]content.ChapterSummary, error) {
	s, sums, err := collection.Open(dir, a.registry, a.emit)
	if err != nil {
		return nil, err
	}
	a.curMu.Lock()
	if a.cur != nil {
		a.cur.Close()
	}
	a.cur = s
	a.curMu.Unlock()
	a.mirrorLegacy()
	return sums, nil
}

// closeSession closes the open Session, if any.
func (a *App) closeSession() {
	a.curMu.Lock()
	if a.cur != nil {
		a.cur.Close()
		a.cur = nil
	}
	a.curMu.Unlock()
	a.mirrorLegacy()
}

// mirrorLegacy copies the Session's handles into the App fields that methods
// not yet migrated still read. TEMPORARY: deleted in Task 9.
func (a *App) mirrorLegacy() {
	a.dirMu.Lock()
	defer a.dirMu.Unlock()
	a.collectionDir, a.learnStore, a.searchSvc, a.explorerSvc = "", nil, nil, nil
	s, err := a.session()
	if err != nil {
		return
	}
	_ = s.Read(func(h collection.Handles) error {
		a.collectionDir = h.Dir()
		a.learnStore, a.searchSvc, a.explorerSvc = h.Learning(), h.Search(), h.Explorer()
		return nil
	})
}
```

- [ ] **Step 4: Rewire App's lifecycle**

In `app.go`:

1. Fields: delete `savingMu`, `savingPaths`, `openFilePath` and `watcher`. Add:

```go
	curMu sync.Mutex          // guards cur
	cur   *collection.Session // the open Collection; nil when none
```

   Keep `collectionDir`, `dirMu`, `searchSvc`, `explorerSvc` and `learnStore` for now, and change their comments to `// TEMPORARY mirror of cur (mirrorLegacy); removed in Task 9`.

2. In `startup`, replace everything from `a.dirMu.Lock()` to the end of the function with:

```go
	sums, err := a.switchTo(cfg.CollectionDir)
	if err != nil {
		a.startupError = "Could not load collection: " + err.Error()
		return
	}
	a.initialSums = sums
```

   Also delete the `content.ScanCollection` call and its error branch just above, because `switchTo` scans. Keep the `os.Stat` "not found" branch.

3. Make `shutdown` the following:

```go
func (a *App) shutdown(_ context.Context) {
	a.closeSession()
}
```

4. Make `OpenCollection` the following:

```go
// OpenCollection opens dir as the current Collection and returns its chapter
// summaries for Column 1. If dir cannot be opened, the current one stays open.
func (a *App) OpenCollection(dir string) ([]content.ChapterSummary, error) {
	sums, err := a.switchTo(dir)
	if err != nil {
		return nil, err
	}
	// Persist the chosen directory so startup() can auto-load it next launch.
	cfg, _ := loadConfig()
	cfg.CollectionDir = dir
	if err := saveConfig(cfg); err != nil {
		log.Printf("content: saveConfig: %v", err)
	}
	return sums, nil
}
```

5. Make `mediaDir` and `collectionRoot` the following:

```go
func (a *App) mediaDir() string {
	dir, err := a.currentDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "media")
}

func (a *App) collectionRoot() string {
	dir, _ := a.currentDir()
	return dir
}
```

   Update their doc comments: they no longer read under `dirMu`, but they are still safe from the HTTP goroutine because `currentDir` takes `curMu` and a Session's directory never changes.

6. Make `ParseFileForUI` the following:

```go
func (a *App) ParseFileForUI(path string) (*content.ParsedFile, []content.ParseError) {
	if s, err := a.session(); err == nil {
		s.SetOpenFile(path)
	}
	return content.ParseFile(path, a.registry)
}
```

   Update its comment so it says the call records the open file on the Session.

In `methods.go`:

7. Delete `beginSelfWrite`, `endSelfWrite`, `isSelfWriting`, `consumeWatchEvents` and their section banner.

8. Change `SaveFile`: rename the existing function to `saveFile` and delete its `a.beginSelfWrite(path)` / `defer a.endSelfWrite(path)` lines. Then add:

```go
// SaveFile atomically writes fileContent to path, then injects missing # @id
// markers for any # @card nodes. Returns the final on-disk content and the
// list of injections performed. The whole write is marked as the app's own,
// so the watcher does not report it as an external change.
func (a *App) SaveFile(path, fileContent string) (*SaveFileResult, error) {
	var res *SaveFileResult
	err := a.selfWrite(path, func() error {
		var err error
		res, err = a.saveFile(path, fileContent)
		return err
	})
	return res, err
}
```

   (`saveFile` calls `a.collectionRoot()`. That's allowed, because `selfWrite` holds no Session lock while `fn` runs.)

9. `GetInitialCollectionDir`: `return a.collectionRoot()`.

10. `CreateNewFile`: replace `if a.collectionDir == "" { … }` with

```go
	dir, err := a.currentDir()
	if err != nil {
		return nil, fmt.Errorf("CreateNewFile: %w", err)
	}
```

    Then use `dir` in place of `a.collectionDir`, and replace the begin/write/end triple with

```go
	if err := a.selfWrite(path, func() error { return atomicWriteText(path, "") }); err != nil {
		return nil, fmt.Errorf("CreateNewFile: %w", err)
	}
```

    (Rename any later `err :=` accordingly, so it compiles.)

In `graph_methods.go`:

11. Replace the `BuildGraph` body with:

```go
func (a *App) BuildGraph() *BuildGraphResult {
	result := &BuildGraphResult{}
	s, err := a.session()
	if err != nil {
		result.Error = "No collection open"
		return result
	}
	a.emit("graph:build-started", nil)
	br, err := s.RebuildGraph()
	a.mirrorLegacy() // TEMPORARY (Task 9): RebuildGraph installed a new search service
	if err != nil {
		result.Error = err.Error()
	} else {
		result.NodesIngested = br.NodesIngested
		result.LinksIngested = br.LinksIngested
		result.FilesProcessed = br.FilesProcessed
		result.DurationMs = br.DurationMs
		result.Errors = br.Errors
		result.UnknownArrows = br.UnknownArrows
		result.ParseErrors = br.ParseErrors
	}
	a.emit("graph:build-complete", result)
	return result
}
```

    Remove the imports that are now unused (`log`, `runtime`, `explorer`, `learning`, `search`, as the compiler reports).

In `learning_methods.go`:

12. Delete `collectionN4LFiles`. Replace `SyncStudyItems` with:

```go
// SyncStudyItems discovers study items across the collection into learning.db.
func (a *App) SyncStudyItems() (*learning.SyncResult, error) {
	s, err := a.session()
	if err != nil {
		return nil, err
	}
	res, err := s.Sync()
	if err != nil {
		return nil, err
	}
	return &res, nil
}
```

The self-write sites in `reading_methods.go` (`CreateExtract`, `UpdateReadingProgress`) and `study_methods.go` (`PromoteExtract`) are not in callbacks yet. For now, wrap each existing begin/write/end triple in `a.selfWrite(path, func() error { … })`. For example, in `UpdateReadingProgress`:

```go
	wErr := a.selfWrite(notesPath, func() error {
		return content.UpdateProgressMarker(notesPath, position)
	})
	if wErr != nil {
		return wErr
	}
```

In `CreateExtract`:

```go
	var exID string
	err = a.selfWrite(notesPath, func() error {
		var err error
		exID, err = content.AppendExtract(notesPath, text, anchor)
		if err == nil && advanceProgress {
			err = content.UpdateProgressMarker(notesPath, position)
		}
		return err
	})
```

In `PromoteExtract`:

```go
	wErr := a.selfWrite(filePath, func() error {
		if kind == "card" {
			// (keep the existing comment about seeding a basic card stub)
			return content.SeedBasicCardNode(filePath, line, e.Text, e.FromAnchor)
		}
		return content.InsertMarkerAboveNode(filePath, line, "# @cloze")
	})
```

Tasks 4 and 5 turn these into `h.SelfWrite` once the methods move into callbacks.

- [ ] **Step 5: Run everything and confirm it passes**

Run: `go build ./... && go vet . ./internal/collection/ && go test ./... -count=1`
Expected: PASS. The whole suite passes: tests that still build `&App{collectionDir:…, learnStore:…}` exercise methods not yet migrated, and those still read the mirrored fields.

Run: `go test -tags demo . -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add session_app.go testapp_test.go session_app_test.go app.go methods.go graph_methods.go learning_methods.go reading_methods.go study_methods.go build_explorer_test.go learning_methods_test.go
git commit -m "refactor(app): open, switch, build and watch through the Session

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Migration patterns for Tasks 4 to 8

Every remaining method follows one of these patterns. The task lists name the pattern for each method and spell out anything that isn't mechanical.

**Pattern A (store call with an error return).** Before:

```go
func (a *App) GetStudyStats() (*learning.Stats, error) {
	if a.learnStore == nil {
		return nil, errNoStore
	}
	st, err := a.learnStore.CountStats(time.Now())
	if err != nil {
		return nil, err
	}
	return &st, nil
}
```

After (the guard disappears; `a.learnStore` becomes `h.Learning()`, `a.collectionDir` becomes `h.Dir()`, `a.searchSvc` becomes `h.Search()`, and `a.explorerSvc` becomes `h.Explorer()`):

```go
func (a *App) GetStudyStats() (*learning.Stats, error) {
	return inCollection(a, func(h collection.Handles) (*learning.Stats, error) {
		st, err := h.Learning().CountStats(time.Now())
		if err != nil {
			return nil, err
		}
		return &st, nil
	})
}
```

A method that returns only `error` uses `return a.withCollection(func(h collection.Handles) error { … })`.

**Pattern B (App helper called by several methods).** A helper that reads session state becomes a method that takes `h collection.Handles` as its first argument, and callers pass their `h`. A helper that reached an exported binding (for example `a.GetExtractTarget` or `a.SearchGraph`) calls the handle-taking helper instead. That is the lock rule in the Global Constraints.

**Pattern C (directory only, then blocking work).** Take `dir, err := a.currentDir()` and do the work with no lock held.

**Inside callbacks:** `a.selfWrite(p, fn)` becomes `h.SelfWrite(p, fn)`. `runtime.EventsEmit(a.ctx, "file:ids-injected", …)` guarded by `if a.ctx != nil` becomes `a.emit(collection.EventIDsInjected, …)`. `learning.Sync(a.learnStore, …)` becomes `learning.Sync(h.Learning(), …)`.

**Tests:** when a migrated method's test built `&App{collectionDir: dir, learnStore: st, …}` after `st, err := learning.Open(dir)` and `t.Cleanup(st.Close)` / `defer st.Close()`, replace all three with `a, st := newTestApp(t, dir, reg)`. Use `nil` for `reg` unless the literal set a specific registry. Keep seeding through the returned `st`. A test that read `a.learnStore.X` directly reads `st.X` instead. A test with a bare `&App{}` that checks "no collection" behaviour stays as it is.

**How to find the tests to convert:** after migrating a file, run `go test . -count=1`. Every failure that says `no collection open` is a test driving a migrated method through an App literal. Convert its setup as described above.

---

### Task 4: Study, learning and statistics methods on the Session

**Files:**
- Modify: `learning_methods.go`, `study_methods.go`, `stats_methods.go`, `demo_seam.go`
- Test: `learning_methods_test.go`, `study_methods_test.go`, `stats_methods_test.go` (setup conversions); `demo_seam_test.go` only if it fails

**Interfaces:**
- Consumes: Task 3's `inCollection`, `withCollection`, `emit`; `collection.Handles`.
- Produces: `func (a *App) extractTarget(h collection.Handles, id string) (*ExtractTarget, error)` (Task 7 uses it).

- [ ] **Step 1: Write the failing test**

Add to `study_methods_test.go`:

```go
// The exported binding and the handle-taking helper must agree; notebook code
// calls the helper from inside a callback, where the binding would deadlock.
func TestExtractTargetMatchesGetExtractTarget(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reading", "k.n4l")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("- Ch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := content.AppendExtract(p, "a passage", "sources/k.txt char=0"); err != nil {
		t.Fatal(err)
	}
	a, _ := newTestApp(t, dir, nil)
	if _, err := a.SyncStudyItems(); err != nil {
		t.Fatal(err)
	}
	q, err := a.GetExtractQueue(10)
	if err != nil || len(q) != 1 {
		t.Fatalf("extract queue len=%d err=%v", len(q), err)
	}
	want, err := a.GetExtractTarget(q[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var got *ExtractTarget
	if err := a.withCollection(func(h collection.Handles) error {
		var err error
		got, err = a.extractTarget(h, q[0].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if *got != *want {
		t.Fatalf("extractTarget = %+v, GetExtractTarget = %+v", got, want)
	}
}
```

(Match the name of the queue's ID field to `ExtractView` as it is defined in `study_methods.go`. Add the imports `collection`, `content`, `os` and `filepath` if they're missing.)

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test . -run TestExtractTargetMatchesGetExtractTarget -count=1`
Expected: FAIL, with `a.extractTarget undefined`.

- [ ] **Step 3: Migrate**

- `learning_methods.go`: `GetDueQueue`, `GradeItem` and `GetStudyStats` use Pattern A. `resolveCardViewPaths(a.learnStore, q)` becomes `resolveCardViewPaths(h.Learning(), q)`. Delete `errNoStore` only if nothing references it any more; otherwise Task 9 deletes it.
- `study_methods.go`:
  - `GetStudyCounts`, `GetExtractQueue`, `rescheduleExtract` (and so `DeferExtract` and `DismissExtract`): Pattern A.
  - `GetExtractTarget` becomes:

```go
func (a *App) GetExtractTarget(id string) (*ExtractTarget, error) {
	return inCollection(a, func(h collection.Handles) (*ExtractTarget, error) {
		return a.extractTarget(h, id)
	})
}

// extractTarget is GetExtractTarget for code already inside a callback.
func (a *App) extractTarget(h collection.Handles, id string) (*ExtractTarget, error) {
	// the former body of GetExtractTarget without its nil guard, with
	// a.learnStore → h.Learning()
}
```

    (Move the existing body in verbatim, with the substitution applied. Keep its comments.)
  - `PromoteExtract`: Pattern A for the whole body. Inside it, `a.selfWrite(filePath, …)` becomes `h.SelfWrite(filePath, …)`, `learning.Sync(a.learnStore, []string{filePath}, a.registry)` becomes `learning.Sync(h.Learning(), []string{filePath}, a.registry)`, `a.learnStore.LogActivity` becomes `h.Learning().LogActivity`, and the `if a.ctx != nil { runtime.EventsEmit(… "file:ids-injected" …) }` block becomes `a.emit(collection.EventIDsInjected, map[string]string{"path": filePath})`. The early `kind` validation may stay outside the callback.
  - `nodeByID` stays as it is, because it only uses `a.registry`.
- `stats_methods.go`: `LogReadingSession` and `GetStatistics` use Pattern A.
- `demo_seam.go` `DemoSeedHistory`: Pattern A for the whole body, with every `a.learnStore` replaced by `h.Learning()`.
- Remove `runtime` and `errors` imports that are now unused.
- Convert the test setups that break, as described in "Tests" under the migration patterns.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go build ./... && go test ./... -count=1 && go test -tags demo . -count=1`
Expected: PASS.

Run: `grep -n "a\.learnStore" learning_methods.go study_methods.go stats_methods.go demo_seam.go`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add -A learning_methods.go study_methods.go stats_methods.go demo_seam.go *_test.go
git commit -m "refactor(app): study, extract and statistics methods through the Session

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Reading methods on the Session, and source reads that stay inside the Collection

**Files:**
- Modify: `reading_methods.go`
- Test: `reading_methods_test.go`, `import_seam_test.go` (setup conversions plus the new test)

**Interfaces:**
- Consumes: Task 3's helpers; `collection.Handles.Path`.
- Produces: `func upsertReadingRelative(st *learning.Store, r learning.ReadingElement) error` (a plain function replacing the method).

- [ ] **Step 1: Write the failing test**

Add to `reading_methods_test.go`:

```go
// A refPath is collection-relative. One that climbs out of the collection must
// be refused, not read — before this, GetSourceBytes("../../x") read any file
// the process could.
func TestSourceReadsRefuseTraversal(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	a, _ := newTestApp(t, dir, nil)
	rel, err := filepath.Rel(dir, outside)
	if err != nil {
		t.Fatal(err)
	}
	rel = filepath.ToSlash(rel) // stored paths are slash-separated

	if got, err := a.GetSourceBytes(rel); err == nil {
		t.Errorf("GetSourceBytes(%q) = %q, nil; want an error", rel, got)
	}
	if got, err := a.GetSourceHTML(rel); err == nil {
		t.Errorf("GetSourceHTML(%q) = %q, nil; want an error", rel, got)
	}
	if got, err := a.GetSourceProjection(rel); err == nil {
		t.Errorf("GetSourceProjection(%q) = %q, nil; want an error", rel, got)
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test . -run TestSourceReadsRefuseTraversal -count=1`
Expected: FAIL. All three calls return the secret with a nil error. (If `GetSourceHTML` or `GetSourceProjection` fails first on a `learnStore` guard, the test still fails, because `GetSourceBytes` reads the file.)

- [ ] **Step 3: Migrate**

- `upsertReadingRelative` becomes a plain function:

```go
func upsertReadingRelative(st *learning.Store, r learning.ReadingElement) error {
	rel, err := st.Rel(r.NotesPath)
	if err != nil {
		return fmt.Errorf("notes path: %w", err)
	}
	stored := r
	stored.NotesPath = rel
	return st.UpsertReading(stored)
}
```

  Callers pass `h.Learning()`.
- **Dialog bindings** (`ImportTextSource`, `ImportPdfSource`) use Pattern C for the guard only. Replace `if a.learnStore == nil || a.collectionDir == "" { … }` with `if _, err := a.currentDir(); err != nil { return nil, err }`. The dialog then runs with no lock held, and the `…From` call follows.
- `importTextFrom`, `importPdfFrom` and `CreateLLMSource` use Pattern A. Inside the callback, `a.collectionDir` becomes `h.Dir()` and `a.upsertReadingRelative(r)` becomes `upsertReadingRelative(h.Learning(), r)`. Keep the empty-path and empty-title checks before the callback.
- `ImportWebSource`: do the guard (Pattern C), `source.FetchAndExtract` and the title and slug logic **before** the callback. Put only the file writes, `CreateReadingNotes` and the upsert inside `inCollection`. The network fetch must not hold the lock.
- `GetReadingList`, `SetReadingProjLen`, `GetReadingExtracts` and `RescheduleReading` use Pattern A.
- `OpenSourceURL`: look up `r.SourceURL` inside `inCollection` (returning the string), then call `runtime.BrowserOpenURL(a.ctx, url)` after the callback.
- `UpdateReadingProgress` uses Pattern A. `a.selfWrite` becomes `h.SelfWrite`.
- `CreateExtract` uses Pattern A. `a.selfWrite` becomes `h.SelfWrite`, `learning.Sync(a.learnStore, …)` becomes `learning.Sync(h.Learning(), …)`, and the ids-injected emit becomes `a.emit(collection.EventIDsInjected, map[string]string{"path": notesPath})`.
- The three source reads become:

```go
func (a *App) GetSourceProjection(refPath string) (string, error) {
	return inCollection(a, func(h collection.Handles) (string, error) {
		p, err := h.Path(refPath)
		if err != nil {
			return "", err
		}
		return source.Projection(p)
	})
}

func (a *App) GetSourceHTML(refPath string) (string, error) {
	return inCollection(a, func(h collection.Handles) (string, error) {
		p, err := h.Path(refPath)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
}

func (a *App) GetSourceBytes(refPath string) (string, error) {
	return inCollection(a, func(h collection.Handles) (string, error) {
		p, err := h.Path(refPath)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(data), nil
	})
}
```

  Keep each function's existing doc comment, and add to each: "refPath is refused if it escapes the collection."
- `uniqueDest`, `copyFile` and `slugify` stay as they are.
- Convert the test setups that break (`seamFixture`, `readingTreeFixture`, `peekExtractFixture` and the literals at `reading_methods_test.go:34,78,277`). In `import_seam_test.go`, `a.learnStore.GetReading` becomes `st.GetReading`, with `st` taken from the fixture. Change `seamFixture` to return `(*App, *learning.Store, string)` and update its callers.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go build ./... && go test ./... -count=1 && go test -tags demo . -count=1`
Expected: PASS, including `TestSourceReadsRefuseTraversal`.

Run: `grep -n "a\.learnStore\|a\.collectionDir\|a\.selfWrite" reading_methods.go`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add -A reading_methods.go reading_methods_test.go import_seam_test.go *_test.go
git commit -m "refactor(app): reading methods through the Session; source reads stay inside the collection

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Search, query hints and explorer bindings on the Session

**Files:**
- Modify: `search_methods.go`, `query_hints_methods.go`, `app.go` (`ExplorerOpen`, `GraphLayout`, `ExplorerReanchor`, `ExplorerClose`)
- Test: `session_app_test.go`

**Interfaces:**
- Consumes: Task 3's helpers.
- Produces: `func noGraphLayout() explorer.LayoutResult` (the "build your graph first" layout).

- [ ] **Step 1: Write the failing test**

Add to `session_app_test.go`:

```go
func TestGraphBindingsDegradeWithNoCollection(t *testing.T) {
	a := &App{}
	sr, err := a.SearchGraph("x")
	if err != nil || sr == nil || !sr.GraphUnavailable {
		t.Errorf("SearchGraph: %+v, %v; want GraphUnavailable and nil error", sr, err)
	}
	if got := a.ListChapters(); got == nil || len(got) != 0 {
		t.Errorf("ListChapters = %#v, want empty non-nil", got)
	}
	if got := a.ListContextTags(); got == nil || len(got) != 0 {
		t.Errorf("ListContextTags = %#v, want empty non-nil", got)
	}
	lr, err := a.ExplorerOpen("x", "extended")
	if err != nil || lr.Diagnostics.GraphError == "" || lr.Nodes == nil {
		t.Errorf("ExplorerOpen: %+v, %v; want the build-first diagnostic", lr, err)
	}
	if _, err := a.ExplorerReanchor("r", "n", "extended"); !errors.Is(err, explorer.ErrStaleResultID) {
		t.Errorf("ExplorerReanchor err = %v, want ErrStaleResultID", err)
	}
	if err := a.ExplorerClose(); err != nil {
		t.Errorf("ExplorerClose err = %v", err)
	}
}
```

(Add the `texere/internal/explorer` import.) This pins today's behaviour with no collection, so the migration can't change it.

- [ ] **Step 2: Run the test**

Run: `go test . -run TestGraphBindingsDegradeWithNoCollection -count=1`
Expected: PASS already, because the mirror fields are nil. This is a characterisation test. It has to stay green through Step 3.

- [ ] **Step 3: Migrate**

`search_methods.go`:

```go
func (a *App) SearchGraph(query string) (*search.SearchResult, error) {
	res, err := inCollection(a, func(h collection.Handles) (*search.SearchResult, error) {
		return h.Search().Search(query)
	})
	if errors.Is(err, collection.ErrNoCollection) {
		r := search.NewSearchResult()
		r.GraphUnavailable = true
		return r, nil
	}
	return res, err
}
```

`query_hints_methods.go`: `ListChapters` and `ListContextTags` make their `Search` call through `inCollection` and return `[]string{}` on any error, including `ErrNoCollection`. Keep the listing and sort code as it is, working on the returned result.

`app.go` explorer bindings:

```go
// noGraphLayout is the empty layout shown when no collection is open,
// carrying the same hint as the search overlay's Tier 2.
func noGraphLayout() explorer.LayoutResult {
	return explorer.LayoutResult{
		Nodes:       []explorer.NodeView{},
		Edges:       []explorer.EdgeView{},
		Territories: []explorer.TerritoryView{},
		Diagnostics: explorer.Diagnostics{
			GraphError: "Build your graph first (Ctrl+G) to enable graph search.",
		},
	}
}

func (a *App) ExplorerOpen(query string, mode string) (explorer.LayoutResult, error) {
	lr, err := inCollection(a, func(h collection.Handles) (explorer.LayoutResult, error) {
		return h.Explorer().Open(query, mode)
	})
	if errors.Is(err, collection.ErrNoCollection) {
		return noGraphLayout(), nil
	}
	return lr, err
}

func (a *App) GraphLayout(query string, mode string) (explorer.LayoutResult, error) {
	lr, err := inCollection(a, func(h collection.Handles) (explorer.LayoutResult, error) {
		return h.Explorer().Layout(query, mode)
	})
	if errors.Is(err, collection.ErrNoCollection) {
		return noGraphLayout(), nil
	}
	return lr, err
}

func (a *App) ExplorerReanchor(resultID string, anchorID string, mode string) (explorer.LayoutResult, error) {
	lr, err := inCollection(a, func(h collection.Handles) (explorer.LayoutResult, error) {
		return h.Explorer().Reanchor(resultID, anchorID, mode)
	})
	if errors.Is(err, collection.ErrNoCollection) {
		return explorer.LayoutResult{}, explorer.ErrStaleResultID
	}
	return lr, err
}

func (a *App) ExplorerClose() error {
	err := a.withCollection(func(h collection.Handles) error { return h.Explorer().Close() })
	if errors.Is(err, collection.ErrNoCollection) {
		return nil
	}
	return err
}
```

Keep each binding's existing doc comment.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, including `TestBuildGraphRewiresExplorer` and `TestGraphBindingsDegradeWithNoCollection`.

Run: `grep -n "a\.searchSvc\|a\.explorerSvc" *.go | grep -v _test`
Expected: output only from `session_app.go` (`mirrorLegacy`) and the field declarations in `app.go`.

- [ ] **Step 5: Commit**

```bash
git add search_methods.go query_hints_methods.go app.go session_app_test.go
git commit -m "refactor(app): search and explorer bindings through the Session

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Notebook methods on the Session

**Files:**
- Modify: `notebook_methods.go`
- Test: `notebook_methods_test.go` (setup conversions)

**Interfaces:**
- Consumes: Task 4's `a.extractTarget(h, id)`; Task 3's helpers; `h.Path`.
- Produces: `func (a *App) chapterOfNode(h collection.Handles, id string) string`, `func (a *App) assembleContext(h collection.Handles, nodeID string) ContextPayload`.

- [ ] **Step 1: Write the failing test**

Add to `notebook_methods_test.go`. It pins the nested-lock hazard: `OpenNotebook` called while a rebuild is waiting must not deadlock.

```go
// OpenNotebook assembles its context from the extract target and the graph.
// Reaching those through the exported bindings from inside a callback takes
// the read lock twice, which deadlocks once a rebuild is queued behind it.
func TestOpenNotebookDoesNotDeadlockAgainstARebuild(t *testing.T) {
	a, id := notebookExtractFixture(t)
	for i := 0; i < 20; i++ {
		built := make(chan struct{})
		opened := make(chan error, 1)
		go func() { a.BuildGraph(); close(built) }()
		go func() { _, err := a.OpenNotebook(id); opened <- err }()
		timeout := time.After(20 * time.Second)
		select {
		case err := <-opened:
			if err != nil {
				t.Fatal(err)
			}
		case <-timeout:
			t.Fatal("OpenNotebook deadlocked against BuildGraph")
		}
		select {
		case <-built:
		case <-timeout:
			t.Fatal("BuildGraph deadlocked against OpenNotebook")
		}
	}
}
```

`notebookExtractFixture` is whichever existing fixture in `notebook_methods_test.go` returns an App and an extract node ID. The literal at line 148 already returns `(&App{…}, dir)`. Convert it to `newTestApp`. If none returns an extract ID yet, build one the way Task 4's `TestExtractTargetMatchesGetExtractTarget` does (`content.AppendExtract`, then `a.SyncStudyItems()`, then `a.GetExtractQueue(10)`), and return `q[0].ID`.

- [ ] **Step 2: Run the test**

Run: `go test . -run TestOpenNotebookDoesNotDeadlockAgainstARebuild -count=1 -timeout 120s`
Expected: PASS. The unmigrated notebook code takes no Session lock, so it can't deadlock. This test guards Step 3: a migration that calls `a.GetExtractTarget` or `a.SearchGraph` from inside a callback hangs here.

- [ ] **Step 3: Migrate**

- `OpenNotebook`, `PeekNotebook`, `SaveNotebook`, `GetNotebook`, `RecordPush`, `NotebookMarks` and `GetNotebookLinks` use Pattern A. Inside them, `a.chapterOfNode(nodeID)` becomes `a.chapterOfNode(h, nodeID)` and `a.assembleContext(x)` becomes `a.assembleContext(h, x)`. `a.notebooksChanged()` and `a.notebookLinksChanged(p)` stay; they only emit.
- `chapterOfNode(h, id)`: `a.GetExtractTarget(id)` becomes `a.extractTarget(h, id)`.
- `assembleContext(h, nodeID)`:
  - `a.GetExtractTarget(nodeID)` becomes `a.extractTarget(h, nodeID)`.
  - `a.SearchGraph(tgt.Text)` becomes `h.Search().Search(tgt.Text)`.
  - `a.learnStore` becomes `h.Learning()`.
  - The source passage read becomes:

```go
				if p, perr := h.Path(e.RefPath); perr == nil {
					if proj, perr := source.Projection(p); perr == nil {
						out.Source.Passage = source.Excerpt(proj, tgt.Text)
					}
				}
```

- Convert the test setups that break (`notebook_methods_test.go:87,148,248,294,320,357`). `a.learnStore.GetNotebookByExtract` at line 167 becomes `st.GetNotebookByExtract`.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go build ./... && go test ./... -count=1 -timeout 300s`
Expected: PASS, including the deadlock test.

Run: `grep -n "a\.learnStore\|a\.collectionDir\|a\.GetExtractTarget\|a\.SearchGraph" notebook_methods.go`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add notebook_methods.go notebook_methods_test.go
git commit -m "refactor(app): notebook methods through the Session, without nested bindings

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Media, audio, import, lint and remaining tests on the Session

**Files:**
- Modify: `app.go` (`LintCollection`, `ImportImage`, `importImageFrom`, `ImportAudio`, `importAudioFrom`, `SaveRecording`), `media_methods.go` (`FetchMediaURL`)
- Test: `media_methods_test.go`, `collection_copy_test.go`, `demo_seam_test.go`, and any other file still building `&App{collectionDir:` or `learnStore:`

**Interfaces:**
- Consumes: Task 3's `currentDir`.
- Produces: nothing new.

- [ ] **Step 1: Convert the tests first**

Run: `grep -n "collectionDir:\|learnStore:" *_test.go`

Convert every remaining hit using the "Tests" rule under the migration patterns. For `media_methods_test.go:78,101`, `(&App{collectionDir: dir})` becomes `a, _ := newTestApp(t, dir, nil)` followed by `a.FetchMediaURL(…)`. For `collection_copy_test.go:109,197`, replace `store2, err := learning.Open(cp)`, its cleanup, and the literal with `a, store2 := newTestApp(t, cp, &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}})`.

- [ ] **Step 2: Run the tests**

Run: `go test ./... -count=1 && go test -tags demo . -count=1`
Expected: PASS. The mirror keeps unmigrated methods working with Session-backed Apps.

- [ ] **Step 3: Migrate (Pattern C everywhere, since each of these blocks on a dialog, a copy or a download)**

- `LintCollection`: `root, err := a.currentDir(); if err != nil { return content.LintResult{}, err }`.
- `ImportImage`, `importImageFrom`, `ImportAudio`, `importAudioFrom`, `SaveRecording`: replace each `a.dirMu.RLock(); dir := a.collectionDir; a.dirMu.RUnlock(); if dir == "" { return nil, fmt.Errorf("no collection open") }` with

```go
	dir, err := a.currentDir()
	if err != nil {
		return nil, err
	}
```

  (Rename any later `err :=` so it compiles.)
- `FetchMediaURL`: the same replacement.
- `TranscribeAudio` (`stt_methods.go`) already goes through `a.mediaDir()`. No change.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go build ./... && go test ./... -count=1 && go test -tags demo . -count=1`
Expected: PASS.

Run: `grep -n "a\.dirMu\|a\.collectionDir\|a\.learnStore\|a\.searchSvc\|a\.explorerSvc" *.go | grep -v _test`
Expected: output only from `session_app.go` (`mirrorLegacy`), `graph_methods.go` (the `a.mirrorLegacy()` line) and the field declarations in `app.go`.

- [ ] **Step 5: Commit**

```bash
git add app.go media_methods.go *_test.go
git commit -m "refactor(app): media, import and lint through the Session

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Delete the legacy fields and verify the whole branch

**Files:**
- Modify: `app.go`, `session_app.go`, `graph_methods.go`, `learning_methods.go` (`errNoStore`), `CLAUDE.md`

- [ ] **Step 1: Delete the mirror**

- In `app.go`, delete the fields `collectionDir`, `dirMu`, `searchSvc`, `explorerSvc` and `learnStore`, plus any imports left unused.
- In `session_app.go`, delete `mirrorLegacy` and its two call sites in `switchTo` and `closeSession`.
- In `graph_methods.go`, delete the `a.mirrorLegacy()` line.
- In `learning_methods.go`, delete `errNoStore` if it is still there.

- [ ] **Step 2: Build and apply the grep gates**

Run: `go build ./... && go build -tags demo ./...`
Expected: success. Any compile error points at a use this plan missed. Migrate it with the matching pattern.

Run each of these:

```bash
grep -rn "learnStore\|searchSvc\|explorerSvc\|dirMu\|openFilePath\|savingPaths\|beginSelfWrite\|errNoStore\|mirrorLegacy" --include=*.go .
grep -rn '"no collection open"' --include=*.go . | grep -v internal/collection/session.go
grep -rn "filepath.Join(a\.\|a\.collectionDir" --include=*.go .
grep -rn "wailsapp" internal/collection/
```

Expected: no output from any of them. (The second allows only the `ErrNoCollection` definition. Any other hit is a hand-spelled guard; replace it with `collection.ErrNoCollection` or delete it if it's dead.)

- [ ] **Step 3: Run the full verification**

```bash
gofmt -l . | grep -v '^frontend/' ; go vet ./... && go test ./... -count=1 && go test -tags demo ./... -count=1 && make lint
```

Expected: `gofmt` lists nothing, and vet, tests and lint all pass.

If cgo is available: `go test -race . ./internal/collection/ -count=1`. Expected: no race reports.

- [ ] **Step 4: Manual smoke test in the real app**

Run `make dev`, then:

1. The last collection auto-loads, and the file tree shows its chapters.
2. Open a `.n4l` file, change it in another editor, and the "modified externally" prompt appears. Save in Texere, and no prompt appears.
3. Ctrl+G builds. The OmniBar search and the 2.5D explorer both show the graph without a restart. Press Ctrl+G again, and both still show it.
4. Open a different collection. The tree and search switch over, and the study queue shows the new collection's cards.
5. Open a folder containing a `learning.db` that is plain text. An error appears, and the previous collection is still usable.
6. Quit. The unsaved-changes prompt still works, and the process exits.

- [ ] **Step 5: Update `CLAUDE.md`**

In the Project Structure block, add under `internal/content/`:

```text
├── internal/collection/   # Session: the open Collection (stores, watcher, self-writes); see CONTEXT.md
```

- [ ] **Step 6: Commit**

```bash
git add -A app.go session_app.go graph_methods.go learning_methods.go CLAUDE.md
git commit -m "refactor(app): drop the per-collection fields App no longer owns

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
