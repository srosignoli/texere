# LLM Learning Skills Implementation Plan

**Goal:** Replace the LLM cell's hardcoded `tutor`/`ask` modes with Markdown "skills" a user authors, shipping the two current prompts as embedded defaults that a user file can shadow by id.

**Architecture:** A new pure-Go `internal/skills` package parses frontmatter and resolves embedded built-ins against user files in `os.UserConfigDir()/texere/skills/`, exposed through `skills_methods.go`. On the frontend, `buildSystem` moves out of the Wails-importing `llm.js` into a dependency-free `llm-prompt.js` so it is testable, and its first argument becomes the skill body rather than a mode.

**Tech Stack:** Go 1.25 (`go:embed`, `os`, `strings`), Wails v2, Svelte 3.49, `node:test`.

## Global Constraints

- **A skill is a system prompt, not an agent** — no tools, no autonomy, no loop. Nothing in this plan adds any.
- **The filename is the id.** `socratic-tutor.md` → id `socratic-tutor`. There is no `id:` frontmatter key; two sources of truth drift.
- **`Parse` NEVER returns an error.** A file with no frontmatter becomes a skill whose whole content is the body and whose name is its id. A skill with an empty body is still listed. Silently dropping a malformed skill is forbidden — a half-edited file must appear looking wrong, not vanish leaving the user unsure the app saw it.
- **A missing skill is never silently substituted.** If `cell.skill` is absent from the list, name it, leave the saved id untouched, do not persist a replacement, and disable Send until the user chooses. This is the rule proven for the shell cell's missing-shell case.
- **`DuplicateSkill` refuses rather than overwrites** when the target file exists. The user file is the one thing Texere did not author; replacing an edited skill with shipped text destroys work with no undo.
- **No new dependencies.** Frontmatter is read by a small hand-rolled parser — three flat `key: value` strings do not justify a YAML module, and this repo hand-rolls its N4L parsing.
- **`TRANSCRIBE_SYSTEM` is out of scope** and must keep working. `TranscribeBar.svelte:4` imports it from `llm.js`; that import must not break.
- **Migration is lazy and non-destructive.** Never rewrite a saved notebook to convert `mode` → `skill`.
- Follow the existing style of the files edited: Go `gofmt`, imports grouped stdlib/external/internal; JS 2-space indent, NO semicolons, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.

---

### Task 1: Skill type and frontmatter parsing

**Files:**
- Create: `internal/skills/skills.go`
- Create: `internal/skills/parse.go`
- Create: `internal/skills/parse_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Skill struct { ID, Name, Description, Placeholder, Body string; Builtin, Overridden bool; Path string }` with JSON tags `id`, `name`, `description`, `placeholder`, `body`, `builtin`, `overridden`, `path`.
  - `func Parse(id string, data []byte) Skill`

**Why `Parse` cannot fail.** Every other design in this repo that silently
dropped bad input caused a defect that took a review round to find. A skill file
the user is midway through editing must still appear in the picker, looking
wrong, so they can see the app read it.

- [ ] **Step 1: Write the failing test**

Create `internal/skills/parse_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/skills/`
Expected: FAIL — the package does not exist (`no Go files` / undefined `Parse`).

- [ ] **Step 3: Write `internal/skills/skills.go`**

```go
// Package skills loads the learning skills that frame an LLM cell's
// conversation: Markdown files whose body is used verbatim as the system
// prompt.
//
// A skill is a PROMPT, not an agent. There are no tools, no loop and no
// autonomy — the body is one string prepended to the context block that the
// notebook assembles. Keeping that boundary is what keeps this package small.
package skills

// Skill is one entry in the LLM cell's picker.
//
// ID is the filename without its extension, and is what a notebook cell
// persists — it must stay stable, or a saved notebook loses its skill. Body is
// the system prompt, used verbatim.
type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Placeholder string `json:"placeholder"`
	Body        string `json:"body"`

	// Builtin marks a skill shipped inside the binary. Overridden marks a
	// builtin that a user file of the same id has replaced; the surviving entry
	// is the user's, and deleting that file restores the builtin.
	Builtin    bool   `json:"builtin"`
	Overridden bool   `json:"overridden"`
	Path       string `json:"path"` // user file; "" for a builtin
}
```

- [ ] **Step 4: Write `internal/skills/parse.go`**

```go
package skills

import "strings"

// Parse reads a skill file. It NEVER fails.
//
// Every design in this repo that silently dropped malformed input has cost a
// review round to find. A file the user is midway through editing must appear
// in the picker looking wrong, rather than vanishing and leaving them unsure
// whether the app read it at all. So anything unparseable degrades to "the
// whole file is the prompt", which is both harmless and visible.
//
// The frontmatter reader is deliberately tiny: three flat `key: value` strings
// do not justify a YAML dependency, and this repo hand-rolls its N4L parsing.
// The limitation is real and documented — flat keys only, no nesting, no
// multi-line values.
func Parse(id string, data []byte) Skill {
	s := Skill{ID: id, Name: id}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	front, body, ok := splitFrontmatter(text)
	if !ok {
		// No frontmatter, or an unterminated block: treat everything as prompt.
		s.Body = strings.TrimSpace(text)
		return s
	}

	for _, line := range strings.Split(front, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			if v := cleanValue(value); v != "" {
				s.Name = v
			}
		case "description":
			s.Description = cleanValue(value)
		case "placeholder":
			s.Placeholder = cleanValue(value)
		}
	}
	s.Body = strings.TrimSpace(body)
	return s
}

// splitFrontmatter returns the block between a leading "---" line and the next
// "---" line, plus everything after it. ok is false when the file does not open
// with a delimiter or the block is never closed — a "---" horizontal rule
// inside the prompt is therefore safe, because only the FIRST closing
// delimiter after a LEADING one counts.
func splitFrontmatter(text string) (front, body string, ok bool) {
	rest, found := strings.CutPrefix(text, "---\n")
	if !found {
		return "", "", false
	}
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		// Also accept a file that ends immediately after the closing delimiter.
		if trimmed, endFound := strings.CutSuffix(rest, "\n---"); endFound {
			return trimmed, "", true
		}
		return "", "", false
	}
	return rest[:end], rest[end+len("\n---\n"):], true
}

// cleanValue trims surrounding whitespace and one layer of matching quotes, so
// `name: "Socratic tutor"` and `name: Socratic tutor` mean the same thing.
func cleanValue(v string) string {
	v = strings.TrimSpace(v)
	for _, q := range []string{`"`, `'`} {
		if len(v) >= 2 && strings.HasPrefix(v, q) && strings.HasSuffix(v, q) {
			return v[1 : len(v)-1]
		}
	}
	return v
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/skills/ -v`
Expected: PASS — all ten tests.

- [ ] **Step 6: Commit**

```bash
git add internal/skills/skills.go internal/skills/parse.go internal/skills/parse_test.go
git commit -m "feat(skills): parse a learning skill's frontmatter and body"
```

---

### Task 2: Built-ins, listing and shadowing

**Files:**
- Create: `internal/skills/builtin/socratic-tutor.md`
- Create: `internal/skills/builtin/thinking-partner.md`
- Create: `internal/skills/load.go`
- Create: `internal/skills/load_test.go`

**Interfaces:**
- Consumes: `Skill` and `Parse` from Task 1.
- Produces:
  - `func List(dir string) ([]Skill, error)` — built-ins plus user files, shadowed, sorted by name
  - `func Dir() (string, error)` — `os.UserConfigDir()/texere/skills`
  - `func Duplicate(dir, id string) (string, error)` — writes a builtin's body to `dir/<id>.md`; refuses if it exists

**The two builtin prompts are moved verbatim** from `frontend/src/lib/llm.js`.
`TUTOR_SYSTEM` (lines 10-21) becomes `socratic-tutor.md`; `ASK_SYSTEM` (lines
4-8) becomes `thinking-partner.md`. Do not reword them — this is a relocation.
The JS constants are removed in Task 5, not here.

- [ ] **Step 1: Write the builtin skill files**

Create `internal/skills/builtin/thinking-partner.md`:

```markdown
---
name: Thinking partner
description: Concise, substantive help reasoning about and connecting knowledge
placeholder: Ask…
---
You are a thinking partner helping the user reason about and connect knowledge while they process reading extracts into notes. Be concise and substantive. Use the provided context (the extract, its neighbours, and source) when relevant. Do not fabricate; say when you are unsure.
```

Create `internal/skills/builtin/socratic-tutor.md`:

```markdown
---
name: Socratic tutor
description: Feynman technique and Wozniak's rules; you write the cards, not the model
placeholder: Explain your understanding, or draft a card…
---
You are a Socratic learning tutor. Your goal is to strengthen the user’s understanding and help them formulate their OWN spaced-repetition cards — never to write finished cards for them.

Method — Feynman Technique: make the user explain concepts in plain language; probe where the explanation is vague, circular, or incomplete; ask "why" and "how do you know".

Method — Wozniak’s Rules of Knowledge Formulation, especially: prefer the minimum information per item; build on basics before the complex; avoid sets and enumerations; make each question specific and unambiguous; encourage the user’s own wording; keep context and sources.

Behaviour: ask one focused question at a time. When the user drafts a card, critique it against the rules (Is it minimal? Unambiguous? Does it test one thing?) and suggest how THEY could improve it — do not hand them a finished front/back. If asked to "just write the card", decline and guide them to write it. Be encouraging but rigorous. Use the provided context when relevant.
```

- [ ] **Step 2: Write the failing test**

Create `internal/skills/load_test.go`:

```go
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
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/skills/`
Expected: FAIL — `List`, `Duplicate` undefined.

- [ ] **Step 4: Write `internal/skills/load.go`**

```go
package skills

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The two shipped skills. They are embedded rather than written to disk on
// first run so that improvements reach existing installs, and so that deleting
// a user's copy restores the original — "reset to default" for free.
//
//go:embed builtin/*.md
var builtinFS embed.FS

const skillExt = ".md"

// Dir is where user-authored skills live: alongside config.json, because a
// learning method is personal rather than per-collection — how you want to be
// taught applies whether the collection is Latin or Kubernetes.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("skills: UserConfigDir: %w", err)
	}
	return filepath.Join(base, "texere", "skills"), nil
}

// List returns the builtins overlaid with any user files, sorted by name.
//
// A user file whose id matches a builtin REPLACES it and is marked Overridden;
// there is never one id twice. Sorting is by name regardless of origin so the
// order is predictable — origin is shown as a label, not as position.
//
// A missing directory is normal on first run and is not an error.
func List(dir string) ([]Skill, error) {
	byID := map[string]Skill{}

	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, fmt.Errorf("skills: read embedded: %w", err)
	}
	for _, e := range entries {
		data, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("skills: read embedded %s: %w", e.Name(), err)
		}
		id := strings.TrimSuffix(e.Name(), skillExt)
		s := Parse(id, data)
		s.Builtin = true
		byID[id] = s
	}

	userEntries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("skills: read %s: %w", dir, err)
	}
	for _, e := range userEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), skillExt) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("skills: read %s: %w", path, err)
		}
		id := strings.TrimSuffix(e.Name(), skillExt)
		s := Parse(id, data)
		s.Path = path
		_, shadows := byID[id]
		s.Overridden = shadows
		byID[id] = s
	}

	out := make([]Skill, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a == b {
			return out[i].ID < out[j].ID
		}
		return a < b
	})
	return out, nil
}

// Duplicate copies a builtin's file into dir so the user can edit it. The copy
// keeps its frontmatter, so it remains a well-formed skill.
//
// It REFUSES when the target exists. The user file is the one thing here Texere
// did not author, and replacing an edited skill with the shipped text would
// destroy work with no undo.
func Duplicate(dir, id string) (string, error) {
	data, err := builtinFS.ReadFile("builtin/" + id + skillExt)
	if err != nil {
		return "", fmt.Errorf("skills: no builtin %q", id)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("skills: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, id+skillExt)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("skills: %s already exists", path)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("skills: write %s: %w", path, err)
	}
	return path, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/skills/ -v`
Expected: PASS — Task 1's ten tests plus Task 2's twelve.

- [ ] **Step 6: Commit**

```bash
git add internal/skills/builtin internal/skills/load.go internal/skills/load_test.go
git commit -m "feat(skills): embed the two default skills, list and shadow them"
```

---

### Task 3: Wails bindings

**Files:**
- Create: `skills_methods.go`

**Interfaces:**
- Consumes: `skills.List`, `skills.Dir`, `skills.Duplicate`, `skills.Skill` (Tasks 1-2).
- Produces, for the frontend:
  - `ListSkills() ([]skills.Skill, error)`
  - `DuplicateSkill(id string) error`
  - `RevealSkillsDir() error`

No unit test: these need a live Wails context. `go build` plus the existing
suites are the gate, and the manual test plan covers behaviour. There is no
`App` struct state to add — skills are read from disk on demand, which is why
this feature needs no watcher.

- [ ] **Step 1: Write `skills_methods.go`**

```go
package main

import (
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/skills"
)

// ListSkills returns the learning skills available to an LLM cell: the two
// embedded defaults overlaid with any Markdown files in the user's skills
// directory. Read on demand rather than cached, so editing a file in your own
// editor shows up the next time the picker opens — no watcher required.
func (a *App) ListSkills() ([]skills.Skill, error) {
	dir, err := skills.Dir()
	if err != nil {
		return nil, err
	}
	list, err := skills.List(dir)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return []skills.Skill{}, nil
	}
	return list, nil
}

// DuplicateSkill writes an editable copy of a builtin into the skills
// directory, where it then shadows the original. It fails rather than
// overwriting an existing file.
func (a *App) DuplicateSkill(id string) error {
	dir, err := skills.Dir()
	if err != nil {
		return err
	}
	_, err = skills.Duplicate(dir, id)
	return err
}

// RevealSkillsDir opens the skills directory in the OS file manager, creating
// it first so the user is never sent to a path that does not exist. Authoring
// happens in the user's own editor; this is the whole of the in-app support.
func (a *App) RevealSkillsDir() error {
	dir, err := skills.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(dir))
	return nil
}
```

- [ ] **Step 2: Verify it builds on both platforms**

Run: `go build ./... && GOOS=windows GOARCH=amd64 go build ./... && make test`
Expected: no build output; all Go tests pass.

- [ ] **Step 3: Regenerate the Wails bindings**

`frontend/wailsjs` is tracked in git and generated by Wails. The `wails` binary
is NOT on PATH; invoke it by absolute path:

Run: `/home/simone/go/bin/wails generate module`
Expected: `frontend/wailsjs/go/main/App.js`, `App.d.ts` and `models.ts` gain
`ListSkills`, `DuplicateSkill`, `RevealSkillsDir` and a `skills.Skill` model.
Report what you saw. Do not hand-edit the generated files.

- [ ] **Step 4: Verify the frontend still builds**

Run: `cd frontend && npx vite build`
Expected: succeeds.

- [ ] **Step 5: Commit**

```bash
git add skills_methods.go frontend/wailsjs
git commit -m "feat(skills): ListSkills, DuplicateSkill and RevealSkillsDir bindings"
```

---

### Task 4: Extract `buildSystem` into a testable module

**Files:**
- Create: `frontend/src/lib/llm-prompt.js`
- Create: `frontend/src/lib/llm-prompt.test.js`
- Modify: `frontend/src/lib/llm.js` (remove `buildSystem`; keep `streamTurn`, `ASK_SYSTEM`, `TUTOR_SYSTEM`, `TRANSCRIBE_SYSTEM`)
- Modify: `frontend/src/components/notebook/LLMCell.svelte` (import site and the `buildSystem` call at line 68)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `buildSystem(systemText, context, attachedCells) -> string`
  - `skillIdForCell(cell) -> string`

**This task is a pure refactor and must not change behaviour.** It exists
because `buildSystem` composes the entire system prompt and has NEVER had a
test: `llm.js` imports `../../wailsjs/runtime/runtime`, which Node's test runner
cannot resolve — `frontend/wailsjs/runtime/` is gitignored and absent on a fresh
clone. This is the same structural trap fixed for `shell-lines.js`.

`buildSystem`'s first parameter becomes the system text itself rather than a
mode. **In this task the caller keeps choosing between the two constants**, so
nothing changes at runtime; Task 5 replaces that call site with a skill body.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/llm-prompt.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildSystem, skillIdForCell } from './llm-prompt.js'

test('the system text leads, and stands alone when there is no context', () => {
  assert.equal(buildSystem('You are a tutor.', null, []), 'You are a tutor.')
})

test('an extract, tags, neighbours and source each become their own block', () => {
  const got = buildSystem('S', {
    extractText: 'the extract',
    tags: ['latin', 'grammar'],
    orbit: [{ arrowLong: 'means', neighborText: 'a thing' }],
    source: { passage: 'the passage' },
  }, [])
  assert.ok(got.startsWith('S\n\n--- Context ---\n'), got)
  assert.ok(got.includes('# Extract\nthe extract'), got)
  assert.ok(got.includes('# Tags\nlatin, grammar'), got)
  assert.ok(got.includes('# Neighbours\n- means a thing'), got)
  assert.ok(got.includes('# Source passage\nthe passage'), got)
})

test('a context block is absent when its data is missing', () => {
  const got = buildSystem('S', { extractText: 'x' }, [])
  assert.ok(got.includes('# Extract'), got)
  assert.ok(!got.includes('# Tags'), got)
  assert.ok(!got.includes('# Neighbours'), got)
  assert.ok(!got.includes('# Source passage'), got)
})

test('an attached N4L draft is included', () => {
  const got = buildSystem('S', null, [{ type: 'n4l', text: 'a (means) b' }])
  assert.ok(got.includes('# Attached N4L draft\na (means) b'), got)
})

test('an attached conversation is flattened to role: content lines', () => {
  const got = buildSystem('S', null, [{
    type: 'llm', messages: [{ role: 'user', content: 'q' }, { role: 'assistant', content: 'a' }],
  }])
  assert.ok(got.includes('# Attached conversation\nuser: q\nassistant: a'), got)
})

test('an attached graph result is included', () => {
  const got = buildSystem('S', null, [{ type: 'graph', resultText: 'node -> node' }])
  assert.ok(got.includes('# Attached graph result\nnode -> node'), got)
})

test('an attached compute cell shows its code fenced and its output', () => {
  const got = buildSystem('S', null, [{ type: 'compute', code: 'print(1)', resultText: '1' }])
  assert.ok(got.includes('```js\nprint(1)\n```'), got)
  assert.ok(got.includes('Output:\n1'), got)
})

test('an empty attachment contributes nothing', () => {
  assert.equal(buildSystem('S', null, [{ type: 'n4l', text: '' }]), 'S')
  assert.equal(buildSystem('S', null, [{ type: 'llm', messages: [] }]), 'S')
})

test('a missing attachment list is tolerated', () => {
  assert.equal(buildSystem('S', null, undefined), 'S')
})

// --- migration ---
// cell.mode is what notebooks saved before this change carry. Mapping it at
// READ time means no rewrite pass and no risk to existing files.

test('a cell with an explicit skill uses it', () => {
  assert.equal(skillIdForCell({ skill: 'exam-drill', mode: 'tutor' }), 'exam-drill')
})

test('a legacy mode maps to the skill that replaced it', () => {
  assert.equal(skillIdForCell({ mode: 'tutor' }), 'socratic-tutor')
  assert.equal(skillIdForCell({ mode: 'ask' }), 'thinking-partner')
})

test('an unknown or absent mode falls back to the default skill', () => {
  assert.equal(skillIdForCell({ mode: 'wat' }), 'thinking-partner')
  assert.equal(skillIdForCell({}), 'thinking-partner')
  assert.equal(skillIdForCell(null), 'thinking-partner')
  assert.equal(skillIdForCell(undefined), 'thinking-partner')
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `make test-js`
Expected: FAIL — `Cannot find module '.../frontend/src/lib/llm-prompt.js'`.

- [ ] **Step 3: Write `frontend/src/lib/llm-prompt.js`**

```js
// Composes the system prompt sent with every LLM turn: the selected skill's
// body, followed by a context block assembled from the notebook.
//
// Deliberately dependency-free. Its sibling llm.js imports the Wails runtime
// from ../../wailsjs/runtime/runtime, which is gitignored and therefore absent
// on a fresh clone — so anything living there cannot be loaded by Node's test
// runner. buildSystem composes the entire prompt and had no test at all for
// exactly that reason. Keep this file importing nothing.

// The skill a cell falls back to when it names none.
export const DEFAULT_SKILL_ID = 'thinking-partner'

// Notebooks saved before skills existed carry `mode: 'ask' | 'tutor'`.
const LEGACY_MODE_SKILLS = {
  ask: 'thinking-partner',
  tutor: 'socratic-tutor',
}

// skillIdForCell resolves which skill a cell wants.
//
// Migration is deliberately lazy: the legacy `mode` is mapped here, at read
// time, rather than by rewriting saved notebooks. Cells persist as one opaque
// JSON blob, so the stale field costs nothing and no file is ever touched to
// convert it — a rewrite pass over a user's notes is a risk with no upside.
export function skillIdForCell(cell) {
  if (cell?.skill) return cell.skill
  return LEGACY_MODE_SKILLS[cell?.mode] ?? DEFAULT_SKILL_ID
}

// buildSystem prepends the skill body to a context block built from the pinned
// context cell and any attached cells' text.
//
// The first argument is the system text itself, not a mode: choosing it belongs
// to the caller, which knows the selected skill. This function only composes.
export function buildSystem(systemText, context, attachedCells) {
  const parts = [systemText]
  const ctx = []
  if (context?.extractText) ctx.push(`# Extract\n${context.extractText}`)
  if (context?.tags?.length) ctx.push(`# Tags\n${context.tags.join(', ')}`)
  if (context?.orbit?.length) {
    ctx.push('# Neighbours\n' + context.orbit.map((o) => `- ${o.arrowLong} ${o.neighborText}`).join('\n'))
  }
  if (context?.source?.passage) ctx.push(`# Source passage\n${context.source.passage}`)
  for (const c of attachedCells ?? []) {
    if (c.type === 'n4l' && c.text) ctx.push(`# Attached N4L draft\n${c.text}`)
    else if (c.type === 'llm' && c.messages?.length) {
      ctx.push('# Attached conversation\n' + c.messages.map((m) => `${m.role}: ${m.content}`).join('\n'))
    }
    else if (c.type === 'graph' && c.resultText) ctx.push(`# Attached graph result\n${c.resultText}`)
    else if (c.type === 'compute' && (c.code || c.resultText)) {
      let s = '# Attached compute cell'
      if (c.code) s += `\n\`\`\`js\n${c.code}\n\`\`\``
      if (c.resultText) s += `\n\nOutput:\n${c.resultText}`
      ctx.push(s)
    }
  }
  if (ctx.length) parts.push('--- Context ---\n' + ctx.join('\n\n'))
  return parts.join('\n\n')
}
```

- [ ] **Step 4: Remove `buildSystem` from `llm.js`**

Delete the whole `buildSystem` function and its preceding comment (currently
`llm.js:30-56`). **Leave `ASK_SYSTEM`, `TUTOR_SYSTEM` and `TRANSCRIBE_SYSTEM`
in place** — `TranscribeBar.svelte:4` imports `TRANSCRIBE_SYSTEM` from here and
must not break, and Task 5 removes the other two.

- [ ] **Step 5: Update the call site**

In `frontend/src/components/notebook/LLMCell.svelte`, change the import at
line 4 and the call at line 68:

```js
  import { streamTurn, ASK_SYSTEM, TUTOR_SYSTEM } from '../../lib/llm.js'
  import { buildSystem } from '../../lib/llm-prompt.js'
```

```js
    const system = buildSystem(cell.mode === 'tutor' ? TUTOR_SYSTEM : ASK_SYSTEM, context, attached)
```

This preserves today's behaviour exactly. Task 5 replaces it.

- [ ] **Step 6: Verify**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS including the new file; build succeeds.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/llm-prompt.js frontend/src/lib/llm-prompt.test.js \
  frontend/src/lib/llm.js frontend/src/components/notebook/LLMCell.svelte
git commit -m "refactor(llm): extract buildSystem into a testable module"
```

---

### Task 5: The skill picker in the LLM cell

**Files:**
- Modify: `frontend/src/components/notebook/LLMCell.svelte`
- Modify: `frontend/src/lib/llm.js` (delete `ASK_SYSTEM` and `TUTOR_SYSTEM`)
- Modify: `frontend/src/lib/notebook.js` (`newLLMCell`)
- Modify: `frontend/src/lib/cell-label.js` + `cell-label.test.js`

**Interfaces:**
- Consumes: `ListSkills()` from `../../../wailsjs/go/main/App` (Task 3); `buildSystem` and `skillIdForCell` from `llm-prompt.js` (Task 4).
- Produces: cells persisting `skill` and `skillName` alongside the existing `mode` (left untouched on old cells).

`skillName` is persisted so `cell-label.js` — a pure function with no access to
the skill list — can render `LLM · Socratic tutor` instead of a bare id. Same
reasoning as `shellLabel` on the shell cell.

- [ ] **Step 1: Write the failing tests for the pure module**

Add to `frontend/src/lib/cell-label.test.js`:

```js
test('an LLM cell is labelled with its skill name', () => {
  assert.equal(cellLabel({ type: 'llm', skillName: 'Socratic tutor' }), 'LLM · Socratic tutor')
})

// A notebook saved before skills existed still labels sensibly.
test('a legacy LLM cell falls back to its mode', () => {
  assert.equal(cellLabel({ type: 'llm', mode: 'tutor' }), 'LLM · tutor')
})

test('an LLM cell with neither shows no dangling separator', () => {
  assert.equal(cellLabel({ type: 'llm' }), 'LLM')
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `make test-js`
Expected: FAIL — the first case returns `LLM · undefined` or `LLM`, since
`cell-label.js:42` reads `cell.mode`.

- [ ] **Step 3: Update `cell-label.js`**

Replace line 42's arm with:

```js
  // skillName is persisted on the cell so this stays a pure function; mode is
  // the legacy field, kept as a fallback so old notebooks still label sensibly.
  if (type === 'llm') detail = cell.skillName || cell.mode || ''
```

- [ ] **Step 4: Run to verify it passes**

Run: `make test-js`
Expected: PASS.

- [ ] **Step 5: Update the cell factory**

In `frontend/src/lib/notebook.js`, replace `newLLMCell`:

```js
export function newLLMCell() {
  seq += 1
  return {
    id: `cell-${Date.now()}-${seq}`, type: 'llm',
    skill: 'thinking-partner', skillName: 'Thinking partner',
    profileId: null, messages: [], attachments: [],
  }
}
```

Then in `NotebookPane.svelte`, change `newLLMCell('ask')` to `newLLMCell()`.

- [ ] **Step 6: Wire the picker into `LLMCell.svelte`**

Replace the imports at lines 4-5 and add skill state to the script:

```js
  import { streamTurn } from '../../lib/llm.js'
  import { buildSystem, skillIdForCell } from '../../lib/llm-prompt.js'
  import { ListSkills } from '../../../wailsjs/go/main/App'
```

```js
  let skills = []
  let skillMissing = false // cell.skill names a file that is no longer present

  onMount(async () => {
    try {
      skills = await ListSkills()
      const want = skillIdForCell(cell)
      const found = skills.find((s) => s.id === want)
      // A saved skill that has gone away must NOT be silently swapped for
      // another: the conversation would continue under a different teaching
      // method with nothing to say so, and persisting the swap would destroy
      // the original choice. Surface it and let the user decide.
      if (!found) { skillMissing = true; return }
      if (cell.skill !== found.id || cell.skillName !== found.name) selectSkill(found)
    } catch (e) { error = String(e?.message ?? e) }
  })

  function selectSkill(s) {
    cell.skill = s.id
    cell.skillName = s.name
    cell = cell
    skillMissing = false
    persist()
  }
  function onPickSkill(e) {
    const s = skills.find((x) => x.id === e.target.value)
    if (s) selectSkill(s)
  }
  $: activeSkill = skills.find((s) => s.id === cell.skill)
```

Delete `setMode`. Replace the `.modes` block in the markup with a picker, and
add the missing-skill notice above the bar:

```svelte
  {#if skillMissing}
    <div class="llm-warn">
      {#if readOnly}
        Skill "{cell.skillName || cell.skill}" is not installed on this machine.
      {:else}
        Skill "{cell.skillName || cell.skill}" was not found. Choose one below to continue.
      {/if}
    </div>
  {/if}
```

```svelte
    <select class="skill" value={cell.skill || ''} on:change={onPickSkill}
      disabled={readOnly || streaming} title="Learning skill">
      {#each skills as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
    </select>
```

Update the send path and the placeholder:

```js
    const system = buildSystem(activeSkill?.body ?? '', context, attached)
```

```svelte
        <textarea bind:value={input} on:keydown={onKey}
          placeholder={activeSkill?.placeholder || 'Ask…'} rows="2"
          disabled={streaming || skillMissing}></textarea>
```

Guard `send()` so the keyboard shortcut cannot bypass the disabled state — add
`skillMissing` to its existing early-return condition, and disable the Send
button on it too.

Add the notice style beside the existing rules, reusing the amber already used
for warnings elsewhere in this app:

```css
  .llm-warn { padding: 6px 8px; color: #b58a3a; font-size: 12px; border-bottom: 1px solid #2a3a4a; }
  .skill { background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 6px; font-size: 12px; }
```

- [ ] **Step 7: Delete the now-unused constants**

In `frontend/src/lib/llm.js`, delete `ASK_SYSTEM` (lines 4-8) and `TUTOR_SYSTEM`
(lines 10-21). Their text now lives in `internal/skills/builtin/*.md`.
**Keep `TRANSCRIBE_SYSTEM`** — `TranscribeBar.svelte` imports it.

Run: `grep -rn "ASK_SYSTEM\|TUTOR_SYSTEM" frontend/src/`
Expected: no output. Any remaining reference must be fixed before committing.

- [ ] **Step 8: Verify**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS; build succeeds with no unused-CSS-selector warning for
this file.

- [ ] **Step 9: Verify by hand**

Run `make dev`, open a notebook, add an LLM cell:

1. The picker lists **Socratic tutor** and **Thinking partner**; the header reads
   `LLM · Thinking partner`.
2. Send a message; the reply arrives as before.
3. Switch to Socratic tutor — the placeholder changes to
   `Explain your understanding, or draft a card…` and the header updates.
4. Reopen the notebook: the skill and its name are still selected.
5. Open a notebook saved before this change (one whose cell JSON has
   `mode: "tutor"`): it opens on Socratic tutor and can send.
6. Read-only: the picker is disabled and the cell still shows its skill name.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/components/notebook/LLMCell.svelte frontend/src/lib/llm.js \
  frontend/src/lib/notebook.js frontend/src/lib/cell-label.js \
  frontend/src/lib/cell-label.test.js frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(llm): choose a learning skill instead of a hardcoded mode"
```

---

### Task 6: The Skills tab in settings

**Files:**
- Modify: `frontend/src/components/LLMSettings.svelte`

**Interfaces:**
- Consumes: `ListSkills`, `DuplicateSkill`, `RevealSkillsDir` (Task 3).
- Produces: nothing later tasks rely on.

**Do NOT force skills into `apiFor(t)`.** That helper returns a
profile-shaped API — `get`/`save`/`del`/`setActive`/`test`/`presets`/`hasKind` —
and skills have none of those: no active selection, no presets, no test button,
no in-app editing. Bending them into that shape would mean six null fields and a
misleading abstraction. Render a separate block when `tab === 'skills'` and
leave `apiFor` alone.

- [ ] **Step 1: Add the tab state and loader**

In the script, beside the existing state:

```js
  import { ListSkills, DuplicateSkill, RevealSkillsDir } from '../../wailsjs/go/main/App'

  let skills = []

  // Read on demand rather than watched: this is a folder edited a handful of
  // times ever, in the user's own editor, so re-listing when the tab opens is
  // enough and costs nothing.
  async function loadSkills() {
    error = ''
    try { skills = await ListSkills() } catch (e) { error = String(e?.message ?? e) }
  }
  async function duplicate(id) {
    error = ''
    try { await DuplicateSkill(id); await loadSkills() } catch (e) { error = String(e?.message ?? e) }
  }
  async function revealSkills() {
    error = ''
    try { await RevealSkillsDir() } catch (e) { error = String(e?.message ?? e) }
  }
```

Extend `setTab` so selecting the skills tab loads skills rather than profiles:

```js
  function setTab(t) {
    tab = t; editing = null; testMsg = {}
    if (t === 'skills') loadSkills()
    else load()
  }
```

- [ ] **Step 2: Add the tab button and panel**

Add a `Skills` button beside the existing tab buttons, then the panel:

```svelte
{#if tab === 'skills'}
  <div class="skills">
    <p class="hint">
      Skills are Markdown files holding the system prompt that frames an LLM
      cell. Edit them in your own editor — the list refreshes when you reopen
      this tab.
    </p>
    {#each skills as s (s.id)}
      <div class="skill-row">
        <div class="skill-main">
          <span class="skill-name">{s.name}</span>
          <span class="skill-origin">{s.builtin ? 'built-in' : (s.overridden ? 'yours, overrides built-in' : 'yours')}</span>
          {#if !s.body}<span class="skill-empty">(no content)</span>{/if}
          {#if s.description}<div class="skill-desc">{s.description}</div>{/if}
        </div>
        {#if s.builtin}
          <button on:click={() => duplicate(s.id)} title="Write an editable copy you can change">Duplicate</button>
        {/if}
      </div>
    {/each}
    <button class="reveal" on:click={revealSkills}>Open skills folder</button>
  </div>
{/if}
```

Wrap the existing profile markup in `{#if tab !== 'skills'}` so it does not
render on this tab.

- [ ] **Step 3: Add the styles**

```css
  .skills { display: flex; flex-direction: column; gap: 8px; }
  .hint { color: #5d7186; font-size: 12px; margin: 0 0 4px; }
  .skill-row { display: flex; align-items: flex-start; gap: 8px; padding: 6px 8px; border: 1px solid #2a3a4a; border-radius: 6px; }
  .skill-main { flex: 1; min-width: 0; }
  .skill-name { color: #e8eaf0; font-size: 13px; }
  .skill-origin { color: #5d7186; font-size: 11px; margin-left: 6px; }
  .skill-empty { color: #b58a3a; font-size: 11px; margin-left: 6px; }
  .skill-desc { color: #5d7186; font-size: 11px; margin-top: 2px; }
  .reveal { align-self: flex-start; background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 4px 10px; cursor: pointer; font-size: 12px; }
```

- [ ] **Step 4: Verify**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS; build succeeds with no unused-CSS-selector warning.

- [ ] **Step 5: Verify by hand**

Run `make dev`, open ⚙ Models → Skills:

1. Both built-ins are listed with descriptions and a `built-in` label.
2. `Open skills folder` opens the folder in your file manager, creating it.
3. Click `Duplicate` on Socratic tutor; the row becomes
   `yours, overrides built-in` and Duplicate disappears.
4. Clicking Duplicate again is impossible — but confirm the backend refuses by
   observing the row no longer offers it.
5. Create `exam-drill.md` by hand in that folder, reopen the tab: it appears.
6. Create a file with frontmatter but no body: it appears with `(no content)`.
7. Delete your `socratic-tutor.md`, reopen the tab: it reads `built-in` again.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/LLMSettings.svelte
git commit -m "feat(settings): a Skills tab listing, duplicating and revealing skills"
```

---

### Task 7: Manual test plan document

**Files:**
- Create: `docs/specs/2026-08-01-llm-learning-skills-manual-test-plan.md`

**Interfaces:**
- Consumes: the behaviour built in Tasks 1-6.
- Produces: nothing consumed by code.

Every prior slice shipped one, because this repo has no Svelte component
harness. Fold in every deferred GUI check from the Task 5 and Task 6 reports —
read to the END of both, since later fix rounds append further sections.

- [ ] **Step 1: Write the document**

Create the file with this content:

```markdown
# LLM learning skills — manual test plan

**Design:** `2026-08-01-llm-learning-skills-design.md`
**Build:** `make dev`

Automated coverage: `internal/skills` (frontmatter parsing, listing, shadowing,
Duplicate) via `make test`; `llm-prompt.js` (prompt composition and the legacy
`mode` migration) and `cell-label.js` via `make test-js`. Everything below is
what those cannot reach.

Skills live in `<UserConfigDir>/texere/skills/` — on Linux
`~/.config/texere/skills/`.

## TC-1 The picker lists both built-ins

Add an LLM cell.

**Expect:** the dropdown offers **Socratic tutor** and **Thinking partner**. The
cell header reads `LLM · Thinking partner`.

## TC-2 A skill frames the conversation

With Thinking partner selected, ask a factual question. Switch to Socratic tutor
and ask it to "just write me a card about X".

**Expect:** the tutor declines to write a finished card and guides you to write
it — the behaviour its prompt specifies. The two skills visibly differ.

## TC-3 The placeholder follows the skill

Switch between the two skills with the input empty.

**Expect:** Thinking partner shows `Ask…`; Socratic tutor shows
`Explain your understanding, or draft a card…`.

## TC-4 The choice persists

Select Socratic tutor, close the notebook, reopen it.

**Expect:** still on Socratic tutor, and the header still reads
`LLM · Socratic tutor`.

## TC-5 A notebook saved before skills existed still works

Open a notebook whose LLM cell predates this change (its saved JSON has
`mode: "tutor"` and no `skill`).

**Expect:** it opens on Socratic tutor, sends normally, and the header labels it.
No error, and the notebook file is NOT rewritten just by opening it.

## TC-6 Your own skill appears

Create `~/.config/texere/skills/exam-drill.md`:

```
---
name: Exam drill
description: rapid recall, no hints
placeholder: Ready?
---
You are an exam drill instructor. Ask short recall questions one at a time. Never give the answer until the user has attempted it.
```

Reopen the LLM cell's picker.

**Expect:** Exam drill is offered, its placeholder is `Ready?`, and selecting it
changes how the model responds.

## TC-7 Duplicating a built-in

⚙ Models → Skills → Duplicate on Socratic tutor.

**Expect:** a file appears at `~/.config/texere/skills/socratic-tutor.md`
containing the full prompt WITH its frontmatter. The row now reads
`yours, overrides built-in`.

Edit that file — change one sentence — and reopen the tab.

**Expect:** your edit is what the cell now uses.

## TC-8 Deleting your copy restores the built-in

Delete `~/.config/texere/skills/socratic-tutor.md`, reopen the Skills tab.

**Expect:** the row reads `built-in` again and the original prompt is back.

## TC-9 A missing skill is not silently substituted

Select Exam drill in a cell, close the notebook, delete `exam-drill.md`, reopen.

**Expect:** a notice naming the missing skill, the picker NOT switched to
something else, and Send disabled. Choosing a skill clears the notice and
persists normally.

Confirm the saved id was not overwritten: before choosing, close and reopen —
the same notice appears, not a silent fallback.

## TC-10 Read-only wording

Open a notebook read-only whose cell references a deleted skill.

**Expect:** the notice states the fact without telling you to choose one, since
there is nothing to choose with. The picker is disabled.

## TC-11 A malformed skill is visible, not swallowed

Create `broken.md` containing only `---` and a name line, with no closing
delimiter.

**Expect:** it still appears in the list. Nothing vanishes silently.

Create `blank.md` with valid frontmatter and no body.

**Expect:** it appears marked `(no content)`.

## TC-12 Open skills folder

⚙ Models → Skills → **Open skills folder**, with the directory not yet existing.

**Expect:** the folder is created and opens in your file manager.

## TC-13 Switching skills mid-conversation

Send two messages under Thinking partner, then switch to Socratic tutor and send
another.

**Expect:** the transcript is preserved, and the new reply follows the tutor's
method. Switching does not clear the conversation.

## TC-14 A long skill still works

Create a skill whose body is several hundred words.

**Expect:** it is used in full; no truncation and no error.
```

- [ ] **Step 2: Commit**

```bash
git add docs/specs/2026-08-01-llm-learning-skills-manual-test-plan.md
git commit -m "docs(test-plan): LLM learning skills"
```

---

## Spec coverage

| Spec requirement | Task |
|---|---|
| Skill format: filename is the id; name/description/placeholder; body is the prompt | 1 |
| `Parse` never errors; no frontmatter → whole file is body; empty body still listed | 1 |
| Hand-rolled frontmatter reader, no YAML dependency | 1 |
| Two built-ins embedded via `go:embed`, prompts moved verbatim | 2 |
| `List`: embed + user dir, shadowing sets `Overridden`, sorted by name | 2 |
| Deleting a user file restores the built-in | 2 |
| `Dir()` = `UserConfigDir()/texere/skills` | 2 |
| `Duplicate` writes the body, creates the dir, refuses to clobber | 2 |
| `ListSkills` / `DuplicateSkill` / `RevealSkillsDir` bindings | 3 |
| Reveal uses `BrowserOpenURL` with a `file://` URL | 3 |
| `buildSystem` extracted to a dependency-free module and finally tested | 4 |
| `buildSystem`'s first arg becomes the system text | 4 |
| Lazy, non-destructive `mode` → `skill` migration | 4 |
| Picker replaces the two mode buttons; `skill`/`skillName` persisted | 5 |
| Placeholder comes from the skill | 5 |
| Missing skill: named, not reassigned, Send disabled, read-only wording | 5 |
| `cell-label.js` reads `skillName` with a legacy `mode` fallback | 5 |
| `newLLMCell` defaults to `thinking-partner` | 5 |
| Skills tab: list with origins, Duplicate, Reveal, `(no content)` marker | 6 |
| Re-list on demand, no watcher | 6 |
| `TRANSCRIBE_SYSTEM` untouched and still imported by `TranscribeBar` | 4, 5 |
| Manual test plan | 7 |

Nothing in the spec's out-of-scope list is built: skills do not select a model or
profile, do not declare which context blocks to include, are not per-collection,
have no in-app editor, no sharing or import, and no file watcher; and streaming,
cancellation, attachments and the context block's content are all unchanged.
