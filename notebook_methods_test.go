package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatalf("learning.Open: %v", err)
	}
	reg, _ := content.LoadRegistry(filepath.Join(t.TempDir(), "nonexistent"))
	a := &App{learnStore: st, registry: reg, collectionDir: dir}

	path := filepath.Join(dir, "notes.n4l")
	if err := os.WriteFile(path, []byte("- C\n\nNewton\n"), 0o644); err != nil {
		t.Fatalf("write n4l: %v", err)
	}

	// Self-check: confirm the fixture parses to a `Newton` node with a line.
	pf, _ := content.ParseFile(path, reg)
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

func peekApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatalf("learning.Open: %v", err)
	}
	reg, _ := content.LoadRegistry(filepath.Join(t.TempDir(), "nonexistent"))
	return &App{learnStore: st, registry: reg, collectionDir: dir}, dir
}

// The whole feature rests on this: peeking must be a read. If it creates, then
// browsing a node gives it a notebook and the pane starts showing one for every
// node visited — the opposite of what this is for.
func TestPeekNotebookCreatesNothing(t *testing.T) {
	a, _ := peekApp(t)
	const id = "notes.n4l:C:id:never-seen"

	got, err := a.PeekNotebook(id)
	if err != nil {
		t.Fatalf("PeekNotebook: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil for a node with no notebook", got)
	}

	// The real assertion: nothing was written.
	if _, ok, err := a.learnStore.GetNotebookByExtract(id); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("PeekNotebook created a notebook row — it must only read")
	}
}

// Absence is the common case (any plain line), so it must not surface as an
// error, or the pane's error banner would flash constantly while navigating.
func TestPeekNotebookAbsenceIsNotAnError(t *testing.T) {
	a, _ := peekApp(t)
	got, err := a.PeekNotebook("nothing-here")
	if err != nil {
		t.Fatalf("absence must not be an error, got %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func TestPeekNotebookReturnsAnExistingNotebook(t *testing.T) {
	a, _ := peekApp(t)
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
	a, dir := peekApp(t)
	path := filepath.Join(dir, "notes.n4l")
	if err := os.WriteFile(path, []byte("- C\n\n# @id abc\nNewton\n\nplain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spans, err := a.FileNodeSpans(path)
	if err != nil {
		t.Fatalf("FileNodeSpans: %v", err)
	}
	// NodeSpan.ID carries the fully-qualified content.Node.ID
	// ("<path>:<chapter>:id:<short>"), not the bare short id, so match on the
	// suffix rather than an exact "abc".
	if len(spans) != 1 || !strings.HasSuffix(spans[0].ID, ":C:id:abc") {
		t.Fatalf("got %+v, want one span whose id ends in :C:id:abc (the plain node has no id)", spans)
	}
}

// A nil slice marshals to JSON null, and the frontend iterates the result.
func TestFileNodeSpansReturnsEmptyNotNilForAnUnparseableFile(t *testing.T) {
	a, dir := peekApp(t)
	spans, err := a.FileNodeSpans(filepath.Join(dir, "does-not-exist.n4l"))
	if err != nil {
		t.Fatalf("a missing file must not be an error here: %v", err)
	}
	if spans == nil {
		t.Fatal("got nil, want an empty slice — nil marshals to JSON null")
	}
}
