package learning

import (
	"testing"
	"time"
)

func TestNotebookUpsertAndGet(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	now := time.Unix(1000, 0)
	nb := Notebook{
		NotebookID: "nb-1", ExtractNodeID: "file.n4l:Chap:id:abc",
		Chapter: "Chap", ContextTags: `["a","b"]`, CellsJSON: `[{"id":"c1","type":"n4l","text":"x"}]`,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertNotebook(nb); err != nil {
		t.Fatalf("UpsertNotebook: %v", err)
	}

	got, ok, err := s.GetNotebook("nb-1")
	if err != nil || !ok {
		t.Fatalf("GetNotebook: ok=%v err=%v", ok, err)
	}
	if got.CellsJSON != nb.CellsJSON || got.ExtractNodeID != nb.ExtractNodeID {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	byExtract, ok, err := s.GetNotebookByExtract("file.n4l:Chap:id:abc")
	if err != nil || !ok || byExtract.NotebookID != "nb-1" {
		t.Fatalf("GetNotebookByExtract: ok=%v id=%q err=%v", ok, byExtract.NotebookID, err)
	}

	if err := s.UpdateNotebookCells("nb-1", `[]`, time.Unix(2000, 0)); err != nil {
		t.Fatalf("UpdateNotebookCells: %v", err)
	}
	got, _, _ = s.GetNotebook("nb-1")
	if got.CellsJSON != `[]` || !got.UpdatedAt.Equal(time.Unix(2000, 0)) {
		t.Fatalf("after update: %+v", got)
	}
}

func TestNotebookUpsertConflictUpdate(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	created := time.Unix(1000, 0)
	first := Notebook{
		NotebookID: "nb-1", ExtractNodeID: "file.n4l:Chap:id:abc",
		Chapter: "Chap", ContextTags: `["a"]`, CellsJSON: `[{"id":"c1"}]`,
		CreatedAt: created, UpdatedAt: created,
	}
	if err := s.UpsertNotebook(first); err != nil {
		t.Fatalf("UpsertNotebook (insert): %v", err)
	}

	updatedAt := time.Unix(2000, 0)
	second := first
	second.Chapter = "Chap2"
	second.CellsJSON = `[{"id":"c2"}]`
	second.CreatedAt = time.Unix(9999, 0) // must be ignored on conflict-update
	second.UpdatedAt = updatedAt
	if err := s.UpsertNotebook(second); err != nil {
		t.Fatalf("UpsertNotebook (conflict-update): %v", err)
	}

	got, ok, err := s.GetNotebook("nb-1")
	if err != nil || !ok {
		t.Fatalf("GetNotebook: ok=%v err=%v", ok, err)
	}
	if got.Chapter != "Chap2" || got.CellsJSON != `[{"id":"c2"}]` {
		t.Fatalf("changed fields not persisted: %+v", got)
	}
	if !got.CreatedAt.Equal(created) {
		t.Fatalf("CreatedAt should be preserved: got %v want %v", got.CreatedAt, created)
	}
	if !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("UpdatedAt should reflect new value: got %v want %v", got.UpdatedAt, updatedAt)
	}
}

func TestGetNotebookMissing(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	_, ok, err := s.GetNotebook("nope")
	if err != nil || ok {
		t.Fatalf("expected (false,nil), got ok=%v err=%v", ok, err)
	}
}

func TestNotebookLinks(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	links := []NotebookLink{
		{FilePath: "a.n4l", Chapter: "C", NodeText: "Newton", NotebookID: "nb-1"},
		{FilePath: "a.n4l", Chapter: "C", NodeText: "Newton", NotebookID: "nb-1"}, // dup, idempotent
		{FilePath: "a.n4l", Chapter: "C", NodeText: "calculus", NotebookID: "nb-1"},
		{FilePath: "b.n4l", Chapter: "C", NodeText: "gravity", NotebookID: "nb-2"},
	}
	if err := s.AddNotebookLinks(links, time.Unix(1, 0)); err != nil {
		t.Fatalf("AddNotebookLinks: %v", err)
	}

	got, err := s.LinksForFile("a.n4l")
	if err != nil {
		t.Fatalf("LinksForFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 links for a.n4l, got %d: %+v", len(got), got)
	}
}

func TestNotebooksForFileReturnsOnlyThatFilesNotebooks(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, nodeID := range []string{
		"/c/a.n4l:Ch:id:one",
		"/c/a.n4l:Ch:id:two",
		"/c/b.n4l:Ch:id:three",
	} {
		if err := st.UpsertNotebook(Notebook{
			NotebookID: "nb-" + nodeID, ExtractNodeID: nodeID,
			Chapter: "Ch", ContextTags: "[]", CellsJSON: "[]",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.NotebooksForFile("/c/a.n4l")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d notebooks, want 2 (b.n4l must not leak in): %+v", len(got), got)
	}
	if got["/c/a.n4l:Ch:id:one"] != "nb-/c/a.n4l:Ch:id:one" {
		t.Errorf("wrong notebook id for :one: %q", got["/c/a.n4l:Ch:id:one"])
	}
}

// A path is not a LIKE pattern. A file literally named with % or _ must not
// match its neighbours, which is why the query uses a range rather than LIKE.
func TestNotebooksForFileTreatsThePathLiterally(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Now()
	for _, nodeID := range []string{"/c/a%b.n4l:Ch:id:one", "/c/axxb.n4l:Ch:id:two"} {
		if err := st.UpsertNotebook(Notebook{
			NotebookID: "nb-" + nodeID, ExtractNodeID: nodeID,
			Chapter: "Ch", ContextTags: "[]", CellsJSON: "[]",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.NotebooksForFile("/c/a%b.n4l")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 — the %% was treated as a wildcard: %+v", len(got), got)
	}
}

func TestNotebooksForFileEmptyIsNotAnError(t *testing.T) {
	st, _ := Open(t.TempDir())
	got, err := st.NotebooksForFile("/c/none.n4l")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("returned a nil map; callers index it directly, so it must be usable")
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
}
