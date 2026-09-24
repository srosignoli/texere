package main

import (
	"testing"

	"texere/internal/collection"
	"texere/internal/content"
	"texere/internal/learning"
)

// newTestApp returns an App with dir open as its Session, plus that Session's
// learning store for seeding test data. The Session closes when the test ends.
// A nil reg means an empty registry.
//
// Returning the store outside a Read breaks the Handles rule on purpose: the
// test owns the Session's whole lifetime, so nothing can close it early.
func newTestApp(t *testing.T, dir string, reg *content.SSTRegistry) (*App, *learning.Store) {
	t.Helper()
	if reg == nil {
		reg = &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}
	}
	a := &App{registry: reg}
	if _, err := a.switchTo(dir); err != nil {
		t.Fatalf("open collection %s: %v", dir, err)
	}
	t.Cleanup(a.closeSession)
	var st *learning.Store
	if err := a.withCollection(func(h collection.Handles) error { st = h.Learning(); return nil }); err != nil {
		t.Fatal(err)
	}
	return a, st
}
