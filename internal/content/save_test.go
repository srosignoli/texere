package content

import (
	"path/filepath"
	"testing"
)

// The id already used elsewhere in the Collection must be skipped, and the
// returned content must be exactly what landed on disk.
func TestSaveInjectsIDsUniqueAcrossTheCollection(t *testing.T) {
	dir := t.TempDir()
	writeCollectionFile(t, dir, "a.n4l", "- a\n\n# @id aaaaaaaa\nSomeone else's node\n")
	b := filepath.Join(dir, "b.n4l")

	scriptShortIDs(t, "aaaaaaaa", "bbbbbbbb")

	res, err := Save(b, "- b\n\n# @card\nA node needing an id\n", nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Injections) != 1 {
		t.Fatalf("want 1 injection, got %d: %+v", len(res.Injections), res.Injections)
	}
	if res.Injections[0].ShortID == "aaaaaaaa" {
		t.Error("issued the id a.n4l already uses")
	}
	if got := string(readFileBytes(t, b)); got != res.FinalContent {
		t.Errorf("FinalContent differs from disk:\n--- returned ---\n%s\n--- disk ---\n%s", res.FinalContent, got)
	}
}

// With no Collection there is nothing to be unique against: the save still
// succeeds and ids are unique within the file.
func TestSaveWithoutACollectionFallsBackToFileScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loose.n4l")

	res, err := Save(path, "- loose\n\n# @card\nA node needing an id\n", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Injections) != 1 {
		t.Fatalf("want 1 injection, got %d", len(res.Injections))
	}
}
