package main

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/learning"
)

func TestGradeItemAdvancesQueue(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	if err := os.WriteFile(p, []byte("- Ch\n# @cloze\nSystem 1 is {{fast}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := newTestApp(t, dir, nil)

	if _, err := a.SyncStudyItems(); err != nil {
		t.Fatalf("SyncStudyItems: %v", err)
	}
	q, err := a.GetDueQueue(10)
	if err != nil || len(q) != 1 {
		t.Fatalf("GetDueQueue len=%d err=%v want 1", len(q), err)
	}
	// Grading the only due card returns nil (queue now empty).
	next, err := a.GradeItem(q[0].ID, int(learning.GradeGood), 1000)
	if err != nil {
		t.Fatalf("GradeItem: %v", err)
	}
	if next != nil {
		t.Fatalf("next=%+v want nil (no more due)", next)
	}
}
