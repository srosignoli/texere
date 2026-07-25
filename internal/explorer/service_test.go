package explorer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"texere/internal/content"
	"texere/internal/graph"
	"texere/internal/search"
)

// =============================================================================
// T044: ExplorerService.Reanchor — error paths + determinism + cache lifecycle
// =============================================================================

// newServiceWithActive constructs an ExplorerService whose `active`
// cache is pre-populated with the supplied ResultSet so tests can drive
// Reanchor / Close without going through Open (which needs a real
// search.SearchService and BadgerDB).
func newServiceWithActive(rs *ResultSet) *ExplorerService {
	return &ExplorerService{active: rs}
}

// newTestServiceWithGraph builds a real ExplorerService backed by a ready
// search.SearchService: a small fixture collection is written to a temp
// dir, graph.BuildFromCollection populates its BadgerDB store, and a
// search.SearchService is opened against it. Mirrors the working pattern in
// build_explorer_test.go (package main) but stays self-contained here so
// internal/explorer's own tests don't need App wiring.
func newTestServiceWithGraph(t *testing.T) *ExplorerService {
	t.Helper()
	dir := t.TempDir()
	n4l := "- Chapter One\n" +
		":: ctx ::\n" +
		"alpha (contains) beta\n"
	if err := os.WriteFile(filepath.Join(dir, "c.n4l"), []byte(n4l), 0o644); err != nil {
		t.Fatalf("write n4l fixture: %v", err)
	}

	reg, _ := content.LoadRegistry(filepath.Join(dir, "SSTconfig")) // degraded (no SSTconfig dir) is fine
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}

	searchSvc := search.NewSearchService(dir, reg)
	t.Cleanup(searchSvc.Close)

	return NewExplorerService(searchSvc, reg)
}

func TestReanchor_StaleResultIDReturnsError(t *testing.T) {
	rs := buildSampleResultSet()
	rs.ID = "real-result"
	svc := newServiceWithActive(rs)

	_, err := svc.Reanchor("not-the-real-id", rs.Nodes[0].Key.String(), "")
	if !errors.Is(err, ErrStaleResultID) {
		t.Fatalf("expected ErrStaleResultID, got %v", err)
	}
}

func TestReanchor_NoActiveResultReturnsError(t *testing.T) {
	svc := &ExplorerService{}
	_, err := svc.Reanchor("any-id", "any-anchor", "")
	if !errors.Is(err, ErrStaleResultID) {
		t.Fatalf("expected ErrStaleResultID for no-active-cache, got %v", err)
	}
}

func TestReanchor_AnchorNotInResultReturnsError(t *testing.T) {
	rs := buildSampleResultSet()
	rs.ID = "real-result"
	svc := newServiceWithActive(rs)

	_, err := svc.Reanchor(rs.ID, "t::not-a-real-node", "")
	if !errors.Is(err, ErrAnchorNotInResult) {
		t.Fatalf("expected ErrAnchorNotInResult, got %v", err)
	}
}

func TestReanchor_SameAnchorIsDeterministic(t *testing.T) {
	rs := buildSampleResultSet()
	rs.ID = "real-result"
	svc := newServiceWithActive(rs)
	anchor := rs.Nodes[0].Key.String()

	first, err := svc.Reanchor(rs.ID, anchor, "")
	if err != nil {
		t.Fatalf("first Reanchor: %v", err)
	}
	for i := 0; i < 50; i++ {
		got, err := svc.Reanchor(rs.ID, anchor, "")
		if err != nil {
			t.Fatalf("iteration %d: Reanchor: %v", i, err)
		}
		if got.AnchorID != first.AnchorID {
			t.Fatalf("iteration %d: anchor flipped from %q to %q", i, first.AnchorID, got.AnchorID)
		}
		if len(got.Nodes) != len(first.Nodes) {
			t.Fatalf("iteration %d: node count drifted %d→%d", i, len(first.Nodes), len(got.Nodes))
		}
		for j := range got.Nodes {
			if got.Nodes[j] != first.Nodes[j] {
				t.Fatalf("iteration %d: node[%d] drifted\n  got=%+v\n  first=%+v", i, j, got.Nodes[j], first.Nodes[j])
			}
		}
	}
}

func TestReanchor_DifferentAnchorChangesLayout(t *testing.T) {
	rs := buildSampleResultSet()
	rs.ID = "real-result"
	svc := newServiceWithActive(rs)

	a := rs.Nodes[0].Key.String()
	b := rs.Nodes[len(rs.Nodes)-1].Key.String()
	if a == b {
		t.Skip("fixture too small to pick two distinct anchors")
	}

	resA, err := svc.Reanchor(rs.ID, a, "")
	if err != nil {
		t.Fatalf("Reanchor(a): %v", err)
	}
	resB, err := svc.Reanchor(rs.ID, b, "")
	if err != nil {
		t.Fatalf("Reanchor(b): %v", err)
	}
	if resA.AnchorID == resB.AnchorID {
		t.Fatalf("two distinct anchors produced the same anchor in result: %s", resA.AnchorID)
	}
	// At least one node MUST be at a different position under the
	// two anchors — otherwise the layout pass isn't honoring the
	// anchor argument.
	differ := 0
	for i := range resA.Nodes {
		if resA.Nodes[i].ID != resB.Nodes[i].ID {
			t.Fatalf("node order changed between anchors at index %d", i)
		}
		if resA.Nodes[i].X != resB.Nodes[i].X || resA.Nodes[i].Y != resB.Nodes[i].Y || resA.Nodes[i].Z != resB.Nodes[i].Z {
			differ++
		}
	}
	if differ == 0 {
		t.Fatalf("Reanchor produced identical positions for two distinct anchors — layout is anchor-blind")
	}
}

func TestReanchor_RepeatedSwapsConverge(t *testing.T) {
	// Deterministic round-tripping: A → B → A produces the same
	// LayoutResult as the initial A. This locks the contract that
	// Reanchor is pure (no hidden state mutation between calls).
	rs := buildSampleResultSet()
	rs.ID = "real-result"
	svc := newServiceWithActive(rs)

	a := rs.Nodes[0].Key.String()
	b := rs.Nodes[1].Key.String()

	resA1, err := svc.Reanchor(rs.ID, a, "")
	if err != nil {
		t.Fatalf("Reanchor(a) #1: %v", err)
	}
	if _, err := svc.Reanchor(rs.ID, b, ""); err != nil {
		t.Fatalf("Reanchor(b): %v", err)
	}
	resA2, err := svc.Reanchor(rs.ID, a, "")
	if err != nil {
		t.Fatalf("Reanchor(a) #2: %v", err)
	}
	if len(resA1.Nodes) != len(resA2.Nodes) {
		t.Fatalf("round-trip Reanchor changed node count")
	}
	for i := range resA1.Nodes {
		if resA1.Nodes[i] != resA2.Nodes[i] {
			t.Fatalf("round-trip Reanchor: node[%d] drifted\n  before=%+v\n  after=%+v",
				i, resA1.Nodes[i], resA2.Nodes[i])
		}
	}
}

func TestClose_EvictsActiveCache(t *testing.T) {
	rs := buildSampleResultSet()
	rs.ID = "real-result"
	svc := newServiceWithActive(rs)

	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if svc.active != nil {
		t.Fatalf("Close did not evict s.active")
	}
	// Subsequent Reanchor should fail with ErrStaleResultID.
	_, err := svc.Reanchor(rs.ID, rs.Nodes[0].Key.String(), "")
	if !errors.Is(err, ErrStaleResultID) {
		t.Fatalf("Reanchor after Close should error with ErrStaleResultID, got %v", err)
	}
}

func TestClose_OnEmptyServiceIsNoOp(t *testing.T) {
	svc := &ExplorerService{}
	if err := svc.Close(); err != nil {
		t.Fatalf("Close on empty service should succeed, got %v", err)
	}
}

// =============================================================================
// GraphLayout binding: stateless Layout must not evict the active session.
// =============================================================================

func TestLayoutMatchesOpenAndDoesNotEvictSession(t *testing.T) {
	svc := newTestServiceWithGraph(t)
	const q = "alpha"

	// Establish a session via Open, capture its result.
	openOut, err := svc.Open(q, "stacked")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if openOut.Diagnostics.GraphError != "" {
		t.Fatalf("Open graphError: %q", openOut.Diagnostics.GraphError)
	}
	if len(openOut.Nodes) == 0 {
		t.Fatalf("Open returned 0 nodes for fixture query %q", q)
	}

	// Layout returns an equivalent result (same node/edge counts) …
	layoutOut, err := svc.Layout(q, "stacked")
	if err != nil {
		t.Fatalf("Layout: %v", err)
	}
	if len(layoutOut.Nodes) != len(openOut.Nodes) || len(layoutOut.Edges) != len(openOut.Edges) {
		t.Errorf("Layout nodes/edges = %d/%d, want %d/%d",
			len(layoutOut.Nodes), len(layoutOut.Edges), len(openOut.Nodes), len(openOut.Edges))
	}

	// … and Layout did NOT evict the active session: a Reanchor against the
	// Open result still works.
	if openOut.AnchorID != "" {
		if _, err := svc.Reanchor(openOut.ResultID, openOut.AnchorID, "stacked"); err != nil {
			t.Errorf("Reanchor after interleaved Layout failed (session evicted): %v", err)
		}
	}
}

func TestSetSearchService_EvictsActiveCache(t *testing.T) {
	// Switching collections (which calls SetSearchService) must drop
	// any cached ResultSet — its ResultSet is keyed against the old
	// collection's NodePtrs and would silently misalign with new
	// queries on the new collection.
	rs := buildSampleResultSet()
	rs.ID = "real-result"
	svc := newServiceWithActive(rs)

	svc.SetSearchService(nil)
	if svc.active != nil {
		t.Fatalf("SetSearchService did not evict s.active")
	}
}
