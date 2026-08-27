package search_test

import (
	"os"
	"path/filepath"
	"testing"

	SST "github.com/srosignoli/sstorytime-kv/pkg/SSTorytime"

	"texere/internal/explorer"
	"texere/internal/graph"
	"texere/internal/search"
)

// TestForwardCone_DoorsExample exercises the canonical SSTorytime `doors.n4l`
// cone (the "multi slit interference" maze) end-to-end: an unknown `(fwd)`
// arrow, object-ditto `"` targets, and `\from start`. The result must be the
// acyclic cone rooted at "start" that fans out through door/port/hole/gate to
// the three targets — exactly the upstream "Local cone paths" picture.
func TestForwardCone_DoorsExample(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join(testFixturesDir(t), "doors.n4l"))
	if err != nil {
		t.Fatalf("read doors fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "doors.n4l"), data, 0644); err != nil {
		t.Fatalf("write doors fixture: %v", err)
	}

	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}
	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)

	res, err := svc.Search(`\from start`)
	if err != nil {
		t.Fatalf(`\from start: %v`, err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\from start: expected a cone; got none`)
	}

	// Collect the cone's directed edges as a set so we can assert the exact
	// subject-ditto fan-out — NOT just which nodes were reached. A subject-ditto
	// regression that mis-chained `door (fwd) passage / " (fwd) road / " (fwd)
	// river` into door→passage→road→river would still "reach" every node, so
	// reached-set checks alone can't catch it; the edge set can.
	edges := map[string]bool{}
	dests := map[string]bool{}
	reached := map[string]bool{}
	for _, ch := range res.Structural {
		texts := chainTexts(ch)
		for j, txt := range texts {
			reached[txt] = true
			if txt == `"` { // object-ditto must have resolved
				t.Errorf(`unresolved object-ditto: node literally named '"' in chain %v`, texts)
			}
			if j > 0 {
				dests[txt] = true
				edges[texts[j-1]+" -> "+txt] = true
			}
		}
	}
	if !reached["start"] {
		t.Error(`cone missing "start"`)
	}
	if dests["start"] {
		t.Error(`"start" must be the root (in-degree 0), but an edge points back into it`)
	}

	// Exact authored fan-out. `"` is a SUBJECT-ditto: each indented `"` repeats
	// the block's subject (start, then door, …), so start fans out to all four
	// first-level rooms and door fans out to all three of its successors.
	wantEdges := []string{
		"start -> door", "start -> port", "start -> hole", "start -> gate",
		"door -> passage", "door -> road", "door -> river",
		"port -> river", "port -> tram",
		"hole -> tram",
		"gate -> tram", "gate -> bike",
		"passage -> target 1", "road -> target 2",
		"river -> target 3", "tram -> target 3", "bike -> target 3",
	}
	for _, e := range wantEdges {
		if !edges[e] {
			t.Errorf("missing cone edge %q (subject-ditto fan-out); got edges=%v", e, keysOf(edges))
		}
	}
	// Guard against the mis-chaining the user reported: door must NOT be a leaf
	// feeding a single chain (door→road→passage→…); these edges must be absent.
	for _, bad := range []string{"road -> passage", "passage -> river", "road -> river"} {
		if edges[bad] {
			t.Errorf("unexpected chained edge %q — subject-ditto must fan out, not chain", bad)
		}
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestForwardCone_CrossChapterMergeStaysConnected guards the fragmentation bug:
// a node authored in two files/chapters (here "start", in both the maze and a
// vocabulary chapter) merges into ONE graph node. The cone must therefore stay a
// SINGLE connected component rooted at "start"; before the fix the explorer keyed
// cone nodes by (text, chapter), so the shared "start" got a different chapter
// per edge and shattered the cone — "start" vanished into a stray fragment and
// the maze rendered rootless. We assert at the explorer layer (where the keying
// lives) that the cone is one component and "start" is its anchor.
func TestForwardCone_CrossChapterMergeStaysConnected(t *testing.T) {
	dir := t.TempDir()
	doors, err := os.ReadFile(filepath.Join(testFixturesDir(t), "doors.n4l"))
	if err != nil {
		t.Fatalf("read doors fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "doors.n4l"), doors, 0644); err != nil {
		t.Fatalf("write doors: %v", err)
	}
	// A second chapter that ALSO defines a "start" node via an express arrow —
	// the same collision as chinese.n4l's `start (eh) 开始`.
	vocab := "- vocabulary\n\nstart (means) begin\n"
	if err := os.WriteFile(filepath.Join(dir, "vocab.n4l"), []byte(vocab), 0644); err != nil {
		t.Fatalf("write vocab: %v", err)
	}

	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}
	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)

	exp := explorer.NewExplorerService(svc, reg)
	lr, err := exp.Open(`\from start`, "")
	if err != nil {
		t.Fatalf(`Open \from start: %v`, err)
	}

	// "start" must be the anchor and appear exactly once (no per-chapter splits).
	starts := 0
	for _, n := range lr.Nodes {
		if n.Label == "start" {
			starts++
		}
	}
	if starts != 1 {
		t.Errorf("want exactly one 'start' node; got %d (per-chapter fragmentation)", starts)
	}
	if lr.AnchorID != "::start" {
		t.Errorf("anchor = %q, want \"::start\" (cone must root at start)", lr.AnchorID)
	}
	// One connected component → one layer. Fragmentation produced several.
	if len(lr.Layers) != 1 {
		t.Errorf("cone must be ONE connected component; got %d layers (fragmented)", len(lr.Layers))
	}
	// No duplicate-labelled nodes — the maze nodes must not be split in two.
	seen := map[string]bool{}
	for _, n := range lr.Nodes {
		if seen[n.Label] {
			t.Errorf("duplicate node %q — cross-chapter keying split a shared node", n.Label)
		}
		seen[n.Label] = true
	}
	// The maze must be reachable from start within the single component.
	for _, want := range []string{"door", "gate", "target 3"} {
		if !seen[want] {
			t.Errorf("cone missing maze node %q", want)
		}
	}
}
