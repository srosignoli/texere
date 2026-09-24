package main

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/content"
)

// TestBuildGraphRewiresExplorer is a regression test for the bug where
// pressing Ctrl+G (BuildGraph) created a fresh SearchService but left the
// ExplorerService pointing at the OLD one. Because BadgerDB allows only one
// opener, the OmniBar (using the new service) worked while the explorer
// (stale service) failed to re-open the now-locked store and returned zero
// nodes — "no graph data" — until the app was restarted.
//
// The assertion that catches it: after BuildGraph, the OmniBar SearchGraph
// path AND the ExplorerOpen path must BOTH see the graph. Exercising the
// explorer alone isn't enough — the bug only manifests when both services
// contend for the same store, so we query both here.
func TestBuildGraphRewiresExplorer(t *testing.T) {
	dir := t.TempDir()
	n4l := "- Chapter One\n" +
		":: ctx ::\n" +
		"alpha (contains) beta\n"
	if err := os.WriteFile(filepath.Join(dir, "c.n4l"), []byte(n4l), 0o644); err != nil {
		t.Fatalf("write n4l: %v", err)
	}
	reg, _ := content.LoadRegistry(filepath.Join(dir, "SSTconfig")) // degraded is fine
	app, _ := newTestApp(t, dir, reg)

	if r := app.BuildGraph(); r.Error != "" {
		t.Fatalf("BuildGraph: %s", r.Error)
	}

	// OmniBar path (uses the post-build search service) opens the store first.
	sg, err := app.SearchGraph("alpha")
	if err != nil {
		t.Fatalf("SearchGraph: %v", err)
	}
	if len(sg.Nodes) == 0 {
		t.Fatalf("SearchGraph returned 0 nodes after BuildGraph")
	}

	// Explorer path must ALSO see the graph. With the old bug the explorer
	// held a stale service that couldn't re-open the locked store → 0 nodes.
	lr, err := app.ExplorerOpen("alpha", "extended")
	if err != nil {
		t.Fatalf("ExplorerOpen: %v", err)
	}
	if lr.Diagnostics.GraphError != "" {
		t.Fatalf("ExplorerOpen graphError: %q", lr.Diagnostics.GraphError)
	}
	if len(lr.Nodes) == 0 {
		t.Fatalf("ExplorerOpen returned 0 nodes after BuildGraph — explorer is pointing at a stale/closed search service (restart-required bug)")
	}

	// A second build must keep both paths working (re-point, not leak/relock).
	if r := app.BuildGraph(); r.Error != "" {
		t.Fatalf("2nd BuildGraph: %s", r.Error)
	}
	if sg2, _ := app.SearchGraph("alpha"); len(sg2.Nodes) == 0 {
		t.Fatalf("SearchGraph returned 0 nodes after 2nd BuildGraph")
	}
	if lr2, _ := app.ExplorerOpen("alpha", "extended"); len(lr2.Nodes) == 0 {
		t.Fatalf("ExplorerOpen returned 0 nodes after 2nd BuildGraph")
	}
}
