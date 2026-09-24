package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"texere/internal/content"
	"texere/internal/learning"
)

func TestNewNotebookID(t *testing.T) {
	a, err := newNotebookID()
	if err != nil {
		t.Fatalf("newNotebookID: %v", err)
	}
	b, _ := newNotebookID()
	if a == b {
		t.Fatalf("ids should differ: %q == %q", a, b)
	}
	if !strings.HasPrefix(a, "nb-") || len(a) != len("nb-")+16 {
		t.Fatalf("unexpected id shape: %q", a)
	}
}

func fakeParsed(chapter string, nodes []struct {
	Text string
	Line int
}) *content.ParsedFile {
	ch := content.Chapter{Title: chapter}
	for _, n := range nodes {
		ch.Nodes = append(ch.Nodes, content.Node{Text: n.Text, Line: n.Line})
	}
	return &content.ParsedFile{Chapters: []content.Chapter{ch}}
}

func TestPushedNodeKeys(t *testing.T) {
	pf := fakeParsed("C", []struct {
		Text string
		Line int
	}{
		{"Newton", 10}, {"calculus", 11}, {"gravity", 12},
	})

	// Bare node push.
	keys := pushedNodeKeys(pf, "Newton\n")
	if len(keys) != 1 || keys[0] != [2]string{"C", "Newton"} {
		t.Fatalf("bare node: got %v", keys)
	}

	// Subject + arrow + target: links the subject, NOT the target.
	keys = pushedNodeKeys(pf, "Newton (invented) calculus\n")
	if len(keys) != 1 || keys[0] != [2]string{"C", "Newton"} {
		t.Fatalf("subject/target: got %v", keys)
	}

	// Matching is by text, independent of the node's .Line (id-injection safe).
	shifted := fakeParsed("C", []struct {
		Text string
		Line int
	}{{"Newton", 999}})
	keys = pushedNodeKeys(shifted, "Newton (is a) physicist\n")
	if len(keys) != 1 {
		t.Fatalf("line-independent match failed: got %v", keys)
	}

	// Node text with an internal double space is whitespace-collapsed on both
	// sides, so a pushed line that collapses to the same string still matches.
	dbl := fakeParsed("C", []struct {
		Text string
		Line int
	}{{"New  ton", 1}})
	keys = pushedNodeKeys(dbl, "New ton\n")
	if len(keys) != 1 || keys[0] != [2]string{"C", "New  ton"} {
		t.Fatalf("double-space collapse match failed: got %v", keys)
	}
}

func TestRecordPushAndGetLinks(t *testing.T) {
	dir := t.TempDir()
	a, _ := newTestApp(t, dir, nil)

	path := filepath.Join(dir, "notes.n4l")
	if err := os.WriteFile(path, []byte("- C\n\nNewton\n"), 0o644); err != nil {
		t.Fatalf("write n4l: %v", err)
	}

	// Self-check: confirm the fixture parses to a `Newton` node with a line.
	pf, _ := content.ParseFile(path, a.registry)
	if pf == nil {
		t.Fatalf("ParseFile returned nil")
	}
	var wantLine int
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.Text == "Newton" {
				wantLine = n.Line
			}
		}
	}
	if wantLine == 0 {
		t.Fatalf("fixture did not parse a Newton node: %+v", pf.Chapters)
	}

	texts, err := a.RecordPush("nb-x", path, "Newton\n")
	if err != nil {
		t.Fatalf("RecordPush: %v", err)
	}
	found := false
	for _, x := range texts {
		if x == "Newton" {
			found = true
		}
	}
	if !found {
		t.Fatalf("RecordPush returned %v, want it to contain Newton", texts)
	}

	marks, err := a.GetNotebookLinks(path)
	if err != nil {
		t.Fatalf("GetNotebookLinks: %v", err)
	}
	if len(marks) != 1 {
		t.Fatalf("want 1 mark, got %d: %+v", len(marks), marks)
	}
	if marks[0].Line != wantLine {
		t.Fatalf("mark line = %d, want %d", marks[0].Line, wantLine)
	}
	if len(marks[0].NotebookIDs) != 1 || marks[0].NotebookIDs[0] != "nb-x" {
		t.Fatalf("mark NotebookIDs = %v, want [nb-x]", marks[0].NotebookIDs)
	}
}

func peekApp(t *testing.T) (*App, string, *learning.Store) {
	t.Helper()
	dir := t.TempDir()
	a, st := newTestApp(t, dir, nil)
	return a, dir, st
}

// The whole feature rests on this: peeking must be a read. If it creates, then
// browsing a node gives it a notebook and the pane starts showing one for every
// node visited — the opposite of what this is for.
func TestPeekNotebookCreatesNothing(t *testing.T) {
	a, _, st := peekApp(t)
	const id = "notes.n4l:C:id:never-seen"

	got, err := a.PeekNotebook(id)
	if err != nil {
		t.Fatalf("PeekNotebook: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil for a node with no notebook", got)
	}

	// The real assertion: nothing was written.
	if _, ok, err := st.GetNotebookByExtract(id); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("PeekNotebook created a notebook row — it must only read")
	}
}

// Absence is the common case (any plain line), so it must not surface as an
// error, or the pane's error banner would flash constantly while navigating.
func TestPeekNotebookAbsenceIsNotAnError(t *testing.T) {
	a, _, _ := peekApp(t)
	got, err := a.PeekNotebook("nothing-here")
	if err != nil {
		t.Fatalf("absence must not be an error, got %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func TestPeekNotebookReturnsAnExistingNotebook(t *testing.T) {
	a, _, _ := peekApp(t)
	const id = "notes.n4l:C:id:abc"

	// OpenNotebook is the creating path; use it to establish the fixture.
	made, err := a.OpenNotebook(id)
	if err != nil {
		t.Fatalf("OpenNotebook: %v", err)
	}

	got, err := a.PeekNotebook(id)
	if err != nil {
		t.Fatalf("PeekNotebook: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want the notebook OpenNotebook just created")
	}
	if got.NotebookID != made.NotebookID {
		t.Fatalf("NotebookID = %q, want %q", got.NotebookID, made.NotebookID)
	}
	if got.NodeID != id {
		t.Fatalf("NodeID = %q, want %q", got.NodeID, id)
	}
}

func TestFileNodeSpansReturnsSpansForIDBearingNodes(t *testing.T) {
	a, dir, _ := peekApp(t)
	path := filepath.Join(dir, "notes.n4l")
	if err := os.WriteFile(path, []byte("- C\n\n# @id abc\nNewton\n\nplain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spans, err := a.FileNodeSpans(path)
	if err != nil {
		t.Fatalf("FileNodeSpans: %v", err)
	}
	// NodeSpan.ID carries content.Node.ID verbatim, which is now the bare
	// short id (see internal/content/nodeid.go), so match it exactly.
	if len(spans) != 1 || spans[0].ID != "abc" {
		t.Fatalf("got %+v, want one span with id \"abc\" (the plain node has no id)", spans)
	}
}

// A nil slice marshals to JSON null, and the frontend iterates the result.
func TestFileNodeSpansReturnsEmptyNotNilForAnUnparseableFile(t *testing.T) {
	a, dir, _ := peekApp(t)
	spans, err := a.FileNodeSpans(filepath.Join(dir, "does-not-exist.n4l"))
	if err != nil {
		t.Fatalf("a missing file must not be an error here: %v", err)
	}
	if spans == nil {
		t.Fatal("got nil, want an empty slice — nil marshals to JSON null")
	}
}

func TestNotebookMarksMarksOnlyNodesThatHaveANotebook(t *testing.T) {
	dir := t.TempDir()
	a, _ := newTestApp(t, dir, nil)

	p := filepath.Join(dir, "f.n4l")
	body := "- Ch\n\n# @id has\nnode with a notebook\n\n# @id none\nnode without one\n\na node with no id at all\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Give only the "has" node a notebook, via the real creating path.
	pf, _ := content.ParseFile(p, a.registry)
	var hasID string
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.ID == "has" {
				hasID = n.ID
			}
		}
	}
	if hasID == "" {
		t.Fatal("fixture: could not find the has node")
	}
	if _, err := a.OpenNotebook(hasID); err != nil {
		t.Fatal(err)
	}

	marks, err := a.NotebookMarks(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 1 {
		t.Fatalf("got %+v, want exactly one mark (only the node with a notebook)", marks)
	}
	if marks[0].Line != 4 {
		t.Errorf("mark on line %d, want 4 (the node line, not the # @id marker line)", marks[0].Line)
	}
	if marks[0].NotebookID == "" {
		t.Error("mark carries no notebook id, so a dot click would have nothing to open")
	}
}

// The dot must survive an edit to the node's text. That is the whole reason for
// keying on node id: the previous text-keyed links dropped the dot silently.
func TestNotebookMarksSurviveANodeTextEdit(t *testing.T) {
	dir := t.TempDir()
	a, _ := newTestApp(t, dir, nil)

	p := filepath.Join(dir, "f.n4l")
	os.WriteFile(p, []byte("- Ch\n\n# @id keep\noriginal text\n"), 0o644)
	pf, _ := content.ParseFile(p, a.registry)
	id := pf.Chapters[0].Nodes[0].ID
	if _, err := a.OpenNotebook(id); err != nil {
		t.Fatal(err)
	}

	// Rewrite the node's text, exactly as making a card or any edit would.
	os.WriteFile(p, []byte("- Ch\n\n# @id keep\ncompletely different text now\n"), 0o644)

	marks, err := a.NotebookMarks(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 1 {
		t.Fatalf("got %+v, want the mark to survive the edit", marks)
	}
}

func TestNotebookMarksCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	a, st := newTestApp(t, dir, nil)

	p := filepath.Join(dir, "f.n4l")
	os.WriteFile(p, []byte("- Ch\n\n# @id fresh\nnever opened\n"), 0o644)

	marks, err := a.NotebookMarks(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 0 {
		t.Fatalf("got %+v, want none", marks)
	}
	// Asserting the empty result is not enough — query the store to prove no row
	// was written. A creating implementation would return marks on the SECOND
	// call and look correct on the first.
	pf, _ := content.ParseFile(p, a.registry)
	var ids []string
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.ID != "" {
				ids = append(ids, n.ID)
			}
		}
	}
	nbs, err := st.NotebooksForNodes(ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(nbs) != 0 {
		t.Fatalf("NotebookMarks created %d notebook row(s) — it must only read", len(nbs))
	}
}

func TestNotebookMarksEmptyNotNilForAFileWithNoNodes(t *testing.T) {
	dir := t.TempDir()
	a, _ := newTestApp(t, dir, nil)
	p := filepath.Join(dir, "empty.n4l")
	os.WriteFile(p, []byte(""), 0o644)

	marks, err := a.NotebookMarks(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if marks == nil {
		t.Fatal("returned a nil slice; it marshals to null instead of [] and the caller indexes it")
	}
}

// notebookExtractFixture returns an App with an open collection containing one
// due extract, and that extract's node id — built the same way
// TestExtractTargetMatchesGetExtractTarget does.
func notebookExtractFixture(t *testing.T) (*App, string) {
	t.Helper()
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
	return a, q[0].ID
}

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
