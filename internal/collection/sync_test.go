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

// TestSyncStillNotifiesWhenTheSyncFailsAfterInjection covers the case where
// learning.Sync writes # @id markers (and records them in InjectedPaths)
// before failing on a later step, such as UpsertItem or RetireMissing. The
// editor must still be told to reload the open file, or its buffer goes
// stale and the next save strips the IDs the failed sync just injected.
//
// Closing the learning store after the file is written, but before Sync
// runs, reproduces this deterministically: InjectMissingIDs succeeds and
// records the path, and the very next store write (UpsertItem) then fails
// because the underlying *sql.DB is closed.
func TestSyncStillNotifiesWhenTheSyncFailsAfterInjection(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.n4l")
	writeFile(t, f, clozeNote)
	s, rec := openT(t, dir)
	s.SetOpenFile(f)

	if err := s.learn.Close(); err != nil {
		t.Fatal(err)
	}

	res, err := s.Sync()
	if err == nil {
		t.Fatal("Sync with a closed learning store = nil error, want an error")
	}
	if len(res.InjectedPaths) != 1 || res.InjectedPaths[0] != f {
		t.Fatalf("InjectedPaths = %v, want [%s] even though the later sync step failed", res.InjectedPaths, f)
	}
	if rec.count(EventIDsInjected, f) != 1 {
		t.Fatalf("file:ids-injected after a sync that failed post-injection: %d events, want 1", rec.count(EventIDsInjected, f))
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
