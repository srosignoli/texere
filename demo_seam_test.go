//go:build demo

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Under -tags demo the seam exists and ingests a path with no dialog, so a
// scripted demo can drive an import unattended.
func TestDemoImportTextSourceIngestsAPath(t *testing.T) {
	a, dir := seamFixture(t)

	outside := filepath.Join(t.TempDir(), "essay.md")
	if err := os.WriteFile(outside, []byte("# Essay\n\nBody."), 0o644); err != nil {
		t.Fatal(err)
	}

	rv, err := a.DemoImportTextSource(outside)
	if err != nil || rv == nil {
		t.Fatalf("DemoImportTextSource rv=%v err=%v", rv, err)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rv.RefPath))); err != nil {
		t.Fatalf("source not copied into collection: %v", err)
	}
}

func TestDemoImportPdfSourceIngestsAPath(t *testing.T) {
	a, dir := seamFixture(t)

	outside := filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(outside, []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rv, err := a.DemoImportPdfSource(outside)
	if err != nil || rv == nil {
		t.Fatalf("DemoImportPdfSource rv=%v err=%v", rv, err)
	}
	if rv.Medium != "pdf" {
		t.Fatalf("medium = %q, want pdf", rv.Medium)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rv.RefPath))); err != nil {
		t.Fatalf("pdf not copied into collection: %v", err)
	}
}

// The seam must not become a way to bypass the collection check.
func TestDemoImportRefusesWithNoCollectionOpen(t *testing.T) {
	a := &App{}
	if _, err := a.DemoImportTextSource("/etc/passwd"); err == nil {
		t.Fatal("DemoImportTextSource with no collection = nil error, want an error")
	}
}
