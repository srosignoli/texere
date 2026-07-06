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

// Renaming/moving a watched subdirectory out of the collection reports its
// contained .n4l files as EventRemoved — inotify emits only one directory-level
// event with no per-file events, so the watcher must derive the removals.
func TestWatchCollectionSubdirRenamedAwayRemovesFiles(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "reading")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	foo := filepath.Join(sub, "foo.n4l")
	if err := os.WriteFile(foo, []byte("- ch\nnode\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cw, err := WatchCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cw.Close()

	if err := os.Rename(sub, filepath.Join(t.TempDir(), "moved")); err != nil {
		t.Fatal(err)
	}

	// Drain events until the expected removal appears (order/multiplicity of
	// directory-move events is OS-dependent).
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-cw.Events:
			if !ok {
				t.Fatal("events channel closed")
			}
			if ev.Kind == EventRemoved && ev.Path == foo {
				return
			}
		case <-deadline:
			t.Fatalf("did not receive EventRemoved for %s", foo)
		}
	}
}
