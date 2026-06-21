package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateReadingNotesScaffold(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "kahneman.n4l")
	id, err := CreateReadingNotes(notes, "sources/kahneman.txt", "Thinking Fast and Slow")
	if err != nil {
		t.Fatalf("CreateReadingNotes: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty ref node id")
	}
	body, _ := os.ReadFile(notes)
	s := string(body)
	for _, want := range []string{
		"- Thinking Fast and Slow",
		"# @id " + id,
		`# @ref "sources/kahneman.txt"`,
		`# @source "Thinking Fast and Slow"`,
		"# @progress char=0",
		"_sequence_",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("scaffold missing %q in:\n%s", want, s)
		}
	}
	// Creating again on the same path must fail (never overwrite).
	if _, err := CreateReadingNotes(notes, "sources/x.txt", "X"); err == nil {
		t.Fatal("expected error creating over an existing file")
	}
}

func TestCreateReadingNotesRejectsQuotes(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "q.n4l")
	if _, err := CreateReadingNotes(notes, "sources/q.txt", `Bad "title"`); err == nil {
		t.Fatal("expected error for title containing a double-quote")
	}
	if _, err := CreateReadingNotes(notes, `sources/q"x.txt`, "ok"); err == nil {
		t.Fatal("expected error for sourceRel containing a double-quote")
	}
}

func TestUpdateProgressMarker(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "k.n4l")
	if _, err := CreateReadingNotes(notes, "sources/k.txt", "K"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateProgressMarker(notes, "char=4180"); err != nil {
		t.Fatalf("UpdateProgressMarker: %v", err)
	}
	body, _ := os.ReadFile(notes)
	if !strings.Contains(string(body), "# @progress char=4180") {
		t.Fatalf("progress not updated:\n%s", body)
	}
	if strings.Contains(string(body), "# @progress char=0") {
		t.Fatal("old progress value still present")
	}
}

func TestAppendExtract(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "k.n4l")
	if _, err := CreateReadingNotes(notes, "sources/k.txt", "K"); err != nil {
		t.Fatal(err)
	}
	id, err := AppendExtract(notes, "System 1 is\nfast and automatic", "sources/k.txt char=1200")
	if err != nil {
		t.Fatalf("AppendExtract: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty extract id")
	}
	body, _ := os.ReadFile(notes)
	s := string(body)
	// Newlines in the selection collapse to a single space.
	for _, want := range []string{
		"# @id " + id,
		"System 1 is fast and automatic",
		`(from) "sources/k.txt char=1200"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("append missing %q in:\n%s", want, s)
		}
	}
	// The extract must come after the _sequence_ context line.
	if strings.Index(s, "_sequence_") > strings.Index(s, "# @id "+id) {
		t.Fatal("extract was not appended under the _sequence_ context")
	}
}
