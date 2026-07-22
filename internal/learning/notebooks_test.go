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

func TestGetNotebookMissing(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	_, ok, err := s.GetNotebook("nope")
	if err != nil || ok {
		t.Fatalf("expected (false,nil), got ok=%v err=%v", ok, err)
	}
}
