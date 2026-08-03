package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ids(list []Skill) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.ID)
	}
	return out
}

func find(t *testing.T, list []Skill, id string) Skill {
	t.Helper()
	for _, s := range list {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("skill %q not in %v", id, ids(list))
	return Skill{}
}

// An empty directory still yields both builtins — they ship in the binary.
func TestListReturnsBuiltinsFromAnEmptyDir(t *testing.T) {
	list, err := List(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %v, want the two builtins", ids(list))
	}
	for _, s := range list {
		if !s.Builtin || s.Overridden || s.Path != "" {
			t.Fatalf("%s: got %+v", s.ID, s)
		}
	}
	tutor := find(t, list, "socratic-tutor")
	if !strings.Contains(tutor.Body, "Socratic") {
		t.Fatalf("builtin body looks wrong: %q", tutor.Body)
	}
	if tutor.Placeholder == "" {
		t.Fatal("builtin should carry a placeholder")
	}
}

// A missing directory is normal on first run and is not an error.
func TestListToleratesAMissingDir(t *testing.T) {
	list, err := List(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("a missing skills dir must not be an error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %v, want the two builtins", ids(list))
	}
}

func TestListIncludesUserSkills(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "exam-drill.md", "---\nname: Exam drill\n---\nDrill me.\n")
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %v, want three", ids(list))
	}
	s := find(t, list, "exam-drill")
	if s.Builtin || s.Path == "" || s.Name != "Exam drill" {
		t.Fatalf("got %+v", s)
	}
}

// The whole point of the shadowing model.
func TestUserFileShadowsABuiltin(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "socratic-tutor.md", "---\nname: My tutor\n---\nMy own prompt.\n")
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %v — shadowing must replace, not duplicate", ids(list))
	}
	s := find(t, list, "socratic-tutor")
	if s.Builtin {
		t.Fatal("the surviving entry must be the user's")
	}
	if !s.Overridden {
		t.Fatal("Overridden must be set so the UI can say so")
	}
	if s.Body != "My own prompt." || s.Name != "My tutor" {
		t.Fatalf("got %+v", s)
	}
}

// Deleting your copy restores the shipped skill — "reset to default" for free.
func TestDeletingAUserFileRestoresTheBuiltin(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "socratic-tutor.md", "---\nname: Mine\n---\nMine.\n")
	if s := find(t, mustList(t, dir), "socratic-tutor"); s.Builtin {
		t.Fatal("precondition: the user file should be winning")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	s := find(t, mustList(t, dir), "socratic-tutor")
	if !s.Builtin || s.Overridden {
		t.Fatalf("got %+v, want the builtin back", s)
	}
}

func TestListIgnoresNonMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "notes.txt", "not a skill")
	write(t, dir, "README", "nor this")
	if list := mustList(t, dir); len(list) != 2 {
		t.Fatalf("got %v, want only the builtins", ids(list))
	}
}

// A skill really must not vanish because an editor saved an uppercase
// extension — Windows and macOS editors do this readily, and load.go's own
// design principle (see parse.go) is that a skill is never silently dropped.
func TestListFindsAnUppercaseExtension(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Drill.MD", "---\nname: Drill\n---\nDrill me.\n")
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := find(t, list, "Drill")
	if s.Name != "Drill" || s.Body != "Drill me." {
		t.Fatalf("got %+v", s)
	}
}

// A mixed-case extension must also be recognised, and the id must have the
// actual extension stripped — not a lowercased guess — so it matches whatever
// a notebook already persisted for this skill.
func TestListFindsAMixedCaseExtensionAndStripsIt(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "notes.Md", "---\nname: Notes\n---\nBody.\n")
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := find(t, list, "notes")
	if s.Name != "Notes" {
		t.Fatalf("got %+v", s)
	}
	for _, other := range list {
		if other.ID == "notes.Md" {
			t.Fatalf("id must have the extension stripped, got %q", other.ID)
		}
	}
}

func TestListIsSortedByName(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "aaa.md", "---\nname: Zebra\n---\nB.\n")
	write(t, dir, "zzz.md", "---\nname: Aardvark\n---\nB.\n")
	list := mustList(t, dir)
	names := make([]string, len(list))
	for i, s := range list {
		names[i] = s.Name
	}
	for i := 1; i < len(names); i++ {
		if strings.ToLower(names[i-1]) > strings.ToLower(names[i]) {
			t.Fatalf("not sorted by name: %v", names)
		}
	}
}

// An empty body is listed, not dropped — the UI marks it instead.
func TestListKeepsAnEmptyBodySkill(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "blank.md", "---\nname: Blank\n---\n")
	s := find(t, mustList(t, dir), "blank")
	if s.Body != "" {
		t.Fatalf("Body = %q", s.Body)
	}
}

func TestDuplicateWritesTheBuiltinBody(t *testing.T) {
	dir := t.TempDir()
	path, err := Duplicate(dir, "socratic-tutor")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Socratic") {
		t.Fatalf("copy does not contain the builtin prompt: %q", data)
	}
	if !strings.HasPrefix(string(data), "---\n") {
		t.Fatal("the copy must keep its frontmatter so it stays editable")
	}
	s := find(t, mustList(t, dir), "socratic-tutor")
	if s.Builtin || !s.Overridden {
		t.Fatalf("got %+v, want the copy to shadow the builtin", s)
	}
}

// Refusing protects work the user authored; overwriting has no undo.
func TestDuplicateRefusesToClobberAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "socratic-tutor.md", "---\nname: Mine\n---\nHand written.\n")
	if _, err := Duplicate(dir, "socratic-tutor"); err == nil {
		t.Fatal("want an error rather than an overwrite")
	}
	s := find(t, mustList(t, dir), "socratic-tutor")
	if s.Body != "Hand written." {
		t.Fatalf("the user's file was modified: %q", s.Body)
	}
}

func TestDuplicateRejectsAnUnknownID(t *testing.T) {
	if _, err := Duplicate(t.TempDir(), "no-such-skill"); err == nil {
		t.Fatal("want an error for an unknown builtin")
	}
}

func TestDuplicateCreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	if _, err := Duplicate(dir, "thinking-partner"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory was not created: %v", err)
	}
}

// An unreadable file (dangling symlink, bad permissions) must not prevent
// the built-ins and other readable skills from appearing. One bad file should
// not take down the entire list.
func TestListSkipsUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	// Create a good user skill
	write(t, dir, "exam-drill.md", "---\nname: Exam drill\n---\nDrill me.\n")
	// Create an unreadable file (dangling symlink is portable)
	if err := os.Symlink("/nonexistent", filepath.Join(dir, "broken.md")); err != nil {
		t.Fatal(err)
	}
	list, err := List(dir)
	if err != nil {
		t.Fatalf("List should not fail due to an unreadable file: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("got %v, want 3 (two builtins + one good user skill)", ids(list))
	}
	// Verify the readable user skill is there
	s := find(t, list, "exam-drill")
	if s.Name != "Exam drill" {
		t.Fatalf("good user skill should be readable: got %+v", s)
	}
	// Verify both built-ins are there
	find(t, list, "socratic-tutor")
	find(t, list, "thinking-partner")
}

func mustList(t *testing.T, dir string) []Skill {
	t.Helper()
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
