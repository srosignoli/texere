package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"texere/internal/collection"
	"texere/internal/explorer"
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
