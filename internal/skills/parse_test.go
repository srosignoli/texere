package skills

import "testing"

func TestParseFullFrontmatter(t *testing.T) {
	src := "---\nname: Socratic tutor\ndescription: you write the cards\nplaceholder: Explain your understanding…\n---\nYou are a tutor.\nBe rigorous.\n"
	got := Parse("socratic-tutor", []byte(src))
	if got.ID != "socratic-tutor" {
		t.Fatalf("ID = %q", got.ID)
	}
	if got.Name != "Socratic tutor" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Description != "you write the cards" {
		t.Fatalf("Description = %q", got.Description)
	}
	if got.Placeholder != "Explain your understanding…" {
		t.Fatalf("Placeholder = %q", got.Placeholder)
	}
	if got.Body != "You are a tutor.\nBe rigorous." {
		t.Fatalf("Body = %q", got.Body)
	}
}

// A file the user has not finished writing must still show up.
func TestParseNoFrontmatterTreatsWholeFileAsBody(t *testing.T) {
	got := Parse("draft", []byte("You are a drill sergeant.\n"))
	if got.Body != "You are a drill sergeant." {
		t.Fatalf("Body = %q", got.Body)
	}
	if got.Name != "draft" {
		t.Fatalf("Name = %q, want the id as fallback", got.Name)
	}
}

func TestParseMissingNameFallsBackToID(t *testing.T) {
	got := Parse("exam-drill", []byte("---\ndescription: only a description\n---\nBody here.\n"))
	if got.Name != "exam-drill" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Description != "only a description" {
		t.Fatalf("Description = %q", got.Description)
	}
}

func TestParseUnknownKeysAreIgnored(t *testing.T) {
	got := Parse("x", []byte("---\nname: X\nmodel: gpt-9\ntemperature: 0.7\n---\nBody.\n"))
	if got.Name != "X" || got.Body != "Body." {
		t.Fatalf("got %+v", got)
	}
}

func TestParseHandlesCRLF(t *testing.T) {
	got := Parse("x", []byte("---\r\nname: X\r\n---\r\nBody line one.\r\nLine two.\r\n"))
	if got.Name != "X" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Body != "Body line one.\nLine two." {
		t.Fatalf("Body = %q", got.Body)
	}
}

// A horizontal rule inside the prompt must not be mistaken for a delimiter.
func TestParseBodyMayContainTripleDash(t *testing.T) {
	src := "---\nname: X\n---\nFirst part.\n\n---\n\nSecond part.\n"
	got := Parse("x", []byte(src))
	if got.Name != "X" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Body != "First part.\n\n---\n\nSecond part." {
		t.Fatalf("Body = %q", got.Body)
	}
}

func TestParseEmptyBodyIsStillASkill(t *testing.T) {
	got := Parse("empty", []byte("---\nname: Empty\n---\n"))
	if got.Name != "Empty" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Body != "" {
		t.Fatalf("Body = %q, want empty", got.Body)
	}
}

func TestParseUnterminatedFrontmatterIsTreatedAsBody(t *testing.T) {
	got := Parse("broken", []byte("---\nname: Broken\nstill going\n"))
	if got.Name != "broken" {
		t.Fatalf("Name = %q, want the id fallback", got.Name)
	}
	if got.Body == "" {
		t.Fatal("body should not be empty — nothing may be silently dropped")
	}
}

func TestParseValuesAreTrimmedAndQuotesStripped(t *testing.T) {
	got := Parse("x", []byte("---\nname:   \"Quoted Name\"  \nplaceholder: 'single'\n---\nB.\n"))
	if got.Name != "Quoted Name" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Placeholder != "single" {
		t.Fatalf("Placeholder = %q", got.Placeholder)
	}
}

func TestParseEmptyFile(t *testing.T) {
	got := Parse("nothing", []byte(""))
	if got.ID != "nothing" || got.Name != "nothing" || got.Body != "" {
		t.Fatalf("got %+v", got)
	}
}
