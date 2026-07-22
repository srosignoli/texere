package main

import (
	"strings"
	"testing"

	"texere/internal/content"
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
}
