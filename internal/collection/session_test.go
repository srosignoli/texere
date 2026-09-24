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

// closeOnce returns a function that closes ch exactly once, safe to call from
// both the test's normal path and a t.Cleanup registered right after ch is
// created — so a t.Fatal between the two still closes it and a helper
// goroutine blocked on <-ch is never left holding the Session's lock forever.
func closeOnce(ch chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
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
	err := s.Read(func(h Handles) error {
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
	if err != nil {
		t.Fatal(err)
	}
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
