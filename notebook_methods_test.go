package main

import (
	"strings"
	"testing"
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
