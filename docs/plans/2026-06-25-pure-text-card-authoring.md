# Pure Text Card Authoring Implementation Plan

**Goal:** Author study cards as explicit text — `front >> back` for basic, `{{ }}` for cloze — decoupled from arrows, deleting the never-populated `ArrowDef.Studyable` dependency that silently dropped every basic card (the TC-1.1 root cause).

**Architecture:** Card/cloze nodes carry literal text instead of arrows. The parser stops extracting arrows from study-marked nodes (except extract nodes anchored with `(from)`), the learning-layer generator splits on `>>` / scans `{{ }}` instead of iterating studyable arrows, and the Process "Make card" promotion seeds a `<passage> >> ` card for the author to complete.

**Tech Stack:** Go 1.22 (parser `internal/content`, learning `internal/learning`, Wails methods); Svelte 3.49 + CodeMirror 6 (`frontend/src/components/ProcessView.svelte`).

## Global Constraints

- Go: standard `gofmt`; imports grouped stdlib / external / internal.
- All new dependencies MUST be pure Go (none are added here).
- Tests run with `make test` (`go test ./...`); lint with `make lint`.
- Item identity key is `NodeID + "::" + CardType + "::" + Discriminator` (`GeneratedCard.ItemID`) — must stay stable for Build-Graph idempotency.
- Commit messages end with: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
- Spec: `docs/specs/2026-06-25-pure-text-card-authoring-design.md`.

---

## Part A — Core authoring & generation (independently shippable; fixes TC-1.1 / TC-1.2)

### Task 1: Parser skips arrow extraction on study-marked non-`(from)` nodes

**Files:**
- Modify: `internal/content/parser.go` (`parseNode`, lines ~465–481; add `hasFromArrowToken` helper near the other node helpers)
- Test: `internal/content/parser_test.go`

**Interfaces:**
- Consumes: existing `extractArrows`, `studyMetaFromBlock`, `state.pendingStudy`, `state.studyBlock`.
- Produces: a `Node` whose `Text` is the full literal line and whose `Arrows` is empty when the node is study-marked and has no `(from)` arrow; unchanged extraction otherwise.

- [ ] **Step 1: Write the failing tests**

Add to `internal/content/parser_test.go`:

```go
func TestCardNodeKeepsLiteralTextNoArrows(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "card.n4l")
	body := "- Ch\n# @card\nWhat is the capital (city) of France? >> Paris\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, _ := ParseFile(p, emptyRegistry())
	n := pf.Chapters[0].Nodes[0]
	if n.Text != "What is the capital (city) of France? >> Paris" {
		t.Fatalf("card text=%q want literal line", n.Text)
	}
	if len(n.Arrows) != 0 {
		t.Fatalf("card node should have no arrows, got %v", n.Arrows)
	}
	if len(pf.Errors) != 0 {
		t.Fatalf("card node should not log unknown-arrow errors, got %v", pf.Errors)
	}
}

func TestClozeExtractNodeKeepsFromAnchorSeparate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ex.n4l")
	// A cloze-promoted extract: study-marked AND carries a (from) anchor.
	body := "- Ch\n# @cloze\nextracted sentence (from) \"sources/x.txt char=0\"\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, _ := ParseFile(p, &SSTRegistry{Arrows: map[string]ArrowDef{"from": {Code: "from", LongName: "from"}}})
	n := pf.Chapters[0].Nodes[0]
	if n.Text != "extracted sentence" {
		t.Fatalf("cloze-extract text=%q want passage with anchor stripped", n.Text)
	}
	if !nodeHasFromArrow(&n) {
		t.Fatalf("cloze-extract node should retain its (from) arrow")
	}
}
```

(`emptyRegistry()` and `nodeHasFromArrow` already exist in the `content` package.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/content/ -run 'TestCardNodeKeepsLiteralTextNoArrows|TestClozeExtractNodeKeepsFromAnchorSeparate' -v`
Expected: FAIL — `TestCardNodeKeepsLiteralTextNoArrows` fails because `(city)` is extracted as an arrow (text truncated, `Arrows` non-empty, spurious error logged).

- [ ] **Step 3: Add the `hasFromArrowToken` helper**

In `internal/content/parser.go`, add near `parseNode`:

```go
// hasFromArrowToken reports whether text contains a "(from)" extract-anchor arrow
// with a non-empty target. Cheap read-only check (no error logging) used to decide
// whether a study-marked node should retain arrow extraction so its (from) anchor
// stays separate from the card text.
func hasFromArrowToken(text string) bool {
	i := strings.Index(text, "(from)")
	if i < 0 {
		return false
	}
	return strings.TrimSpace(text[i+len("(from)"):]) != ""
}
```

- [ ] **Step 4: Reorder `parseNode` to decide extraction from the study marker**

In `internal/content/parser.go`, replace this block:

```go
	// Extract arrows from node text
	cleanText, arrows := extractArrows(nodeText, lineNum, false, reg, path, pf)
	resolveObjectDittos(state, arrows)

	// Build study meta from pending or block
	var sm *StudyMeta
	if state.pendingStudy != nil {
		sm = state.pendingStudy
		state.pendingStudy = nil
	} else if state.studyBlock.kind != BlockTypeNone {
		sm = studyMetaFromBlock(state.studyBlock)
	}

	// Extract cloze targets from node text
	if sm != nil && sm.CardType == CardTypeCloze {
		sm.ClozeTargets = extractClozeTargets(cleanText)
	}
```

with:

```go
	// Build study meta first — a study marker (without a (from) anchor) means this
	// line is a card, not a relationship, so its text is taken literally and arrows
	// are NOT parsed from it.
	var sm *StudyMeta
	if state.pendingStudy != nil {
		sm = state.pendingStudy
		state.pendingStudy = nil
	} else if state.studyBlock.kind != BlockTypeNone {
		sm = studyMetaFromBlock(state.studyBlock)
	}

	var cleanText string
	var arrows []Arrow
	if sm != nil && !hasFromArrowToken(nodeText) {
		// Card/cloze/study node without a (from) anchor: the whole line is literal
		// card text (with ">>" kept literal for the generator to split).
		cleanText = nodeText
	} else {
		cleanText, arrows = extractArrows(nodeText, lineNum, false, reg, path, pf)
		resolveObjectDittos(state, arrows)
	}

	// Extract cloze targets from node text
	if sm != nil && sm.CardType == CardTypeCloze {
		sm.ClozeTargets = extractClozeTargets(cleanText)
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/content/ -run 'TestCardNodeKeepsLiteralTextNoArrows|TestClozeExtractNodeKeepsFromAnchorSeparate' -v`
Expected: PASS.

- [ ] **Step 6: Run the full content package + lint**

Run: `go test ./internal/content/ && gofmt -l internal/content/parser.go`
Expected: PASS, and `gofmt -l` prints nothing.

- [ ] **Step 7: Commit**

```bash
git add internal/content/parser.go internal/content/parser_test.go
git commit -m "$(cat <<'EOF'
feat(content): card/cloze nodes take literal text; skip arrow extraction

Study-marked nodes without a (from) anchor keep their whole line as node.Text
(no arrow parsing), so >> card text and parenthetical prose survive intact.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Cloze targets recognize `{{ }}` only

**Files:**
- Modify: `internal/content/parser.go` (`extractClozeTargets`, lines ~1042–1055)
- Test: `internal/content/parser_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `extractClozeTargets(text string) []string` returns the contents of each `{{ … }}` group; single `{ … }` is ignored.

- [ ] **Step 1: Write the failing test**

Add to `internal/content/parser_test.go`:

```go
func TestExtractClozeTargetsDoubleBraceOnly(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"The {{capital}} of {{France}}", []string{"capital", "France"}},
		{"single {brace} is literal", nil},
		{"mixed {x} and {{y}}", []string{"y"}},
		{"no braces here", nil},
	}
	for _, c := range cases {
		got := extractClozeTargets(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("%q: got %v want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%q: got[%d]=%q want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/content/ -run TestExtractClozeTargetsDoubleBraceOnly -v`
Expected: FAIL — the current single-brace scanner returns `["x"... ]` for `{x}` and mishandles `{{ }}`.

- [ ] **Step 3: Rewrite `extractClozeTargets`**

Replace the function body in `internal/content/parser.go`:

```go
// extractClozeTargets finds the contents of each {{double-brace}} group in text,
// in order. A single {brace} is literal text and is ignored.
func extractClozeTargets(text string) []string {
	var targets []string
	i := 0
	for i+1 < len(text) {
		if text[i] == '{' && text[i+1] == '{' {
			if j := strings.Index(text[i+2:], "}}"); j >= 0 {
				targets = append(targets, text[i+2:i+2+j])
				i = i + 2 + j + 2
				continue
			}
			break // unmatched "{{" — no further groups
		}
		i++
	}
	return targets
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/content/ -run TestExtractClozeTargetsDoubleBraceOnly -v`
Expected: PASS.

- [ ] **Step 5: Run full content package**

Run: `go test ./internal/content/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/content/parser.go internal/content/parser_test.go
git commit -m "$(cat <<'EOF'
feat(content): cloze targets recognize {{double braces}} only

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Rewrite card generation (`>>` basic + reverse, `{{ }}` cloze; drop `reg`)

**Files:**
- Modify: `internal/learning/cards.go` (whole file)
- Modify: `internal/learning/sync.go:35` (caller)
- Test: `internal/learning/cards_test.go` (rewrite)

**Interfaces:**
- Consumes: `content.ParsedFile`, `content.CardTypeBasic`, `content.CardTypeReverse`, `content.CardTypeCloze`, `GeneratedCard`.
- Produces: `func GenerateCards(pf *content.ParsedFile) []GeneratedCard` (no `reg` parameter). Basic → one card from the first `>>`; reverse → two cards (`Discriminator` `"0"` and `"1"`); cloze → one card per `{{ }}` group. No `>>` or empty side → no card.

- [ ] **Step 1: Rewrite the test file**

Replace the entire contents of `internal/learning/cards_test.go`:

```go
package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"texere/internal/content"
)

func writeFixture(t *testing.T, body string) (string, *content.SSTRegistry) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p, &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}
}

func findCard(cards []GeneratedCard, idSuffix, cardType, disc string) *GeneratedCard {
	for i := range cards {
		if strings.HasSuffix(cards[i].NodeID, idSuffix) &&
			cards[i].CardType == cardType && cards[i].Discriminator == disc {
			return &cards[i]
		}
	}
	return nil
}

func TestGenerateBasicAndClozeCards(t *testing.T) {
	body := "- Ch\n" +
		"# @id n1\n# @cloze\nSystem 1 is {{fast}} and {{automatic}}\n" +
		"# @id n2\n# @card\nWhat is System 2? >> effortful\n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	got := GenerateCards(pf)

	if len(got) != 3 {
		t.Fatalf("got %d cards want 3: %+v", len(got), got)
	}
	cloze0 := findCard(got, ":id:n1", CardTypeCloze, "0")
	if cloze0 == nil || cloze0.Front != "System 1 is […] and automatic" ||
		cloze0.Back != "System 1 is fast and automatic" {
		t.Fatalf("cloze0=%+v", cloze0)
	}
	basic := findCard(got, ":id:n2", CardTypeBasic, "0")
	if basic == nil || basic.Front != "What is System 2?" || basic.Back != "effortful" {
		t.Fatalf("basic=%+v", basic)
	}
}

func TestGenerateReverseEmitsTwoCards(t *testing.T) {
	body := "- Ch\n# @id n1\n# @card reverse\ncapital of France >> Paris\n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	got := GenerateCards(pf)

	if len(got) != 2 {
		t.Fatalf("got %d cards want 2: %+v", len(got), got)
	}
	fwd := findCard(got, ":id:n1", CardTypeBasic, "0")
	rev := findCard(got, ":id:n1", CardTypeBasic, "1")
	if fwd == nil || fwd.Front != "capital of France" || fwd.Back != "Paris" {
		t.Fatalf("fwd=%+v", fwd)
	}
	if rev == nil || rev.Front != "Paris" || rev.Back != "capital of France" {
		t.Fatalf("rev=%+v", rev)
	}
}

func TestBasicNoSeparatorYieldsNoCard(t *testing.T) {
	body := "- Ch\n# @id n1\n# @card\njust a statement with no separator\n" +
		"# @id n2\n# @card\nempty back >> \n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	if got := GenerateCards(pf); len(got) != 0 {
		t.Fatalf("got %d cards want 0: %+v", len(got), got)
	}
}

func TestClozeSingleBraceIsNotABlank(t *testing.T) {
	body := "- Ch\n# @id n1\n# @cloze\nThe capital is {Paris}\n"
	p, reg := writeFixture(t, body)
	pf, _ := content.ParseFile(p, reg)
	if got := GenerateCards(pf); len(got) != 0 {
		t.Fatalf("single-brace cloze should yield no card, got %+v", got)
	}
}

// Regression for the original TC-1.1 failure: with the REAL SSTconfig registry,
// a # @card node with >> must produce exactly one item (the old arrow/Studyable
// path produced zero because ArrowDef.Studyable was never populated).
func TestGenerateBasicWithRealRegistry(t *testing.T) {
	reg, err := content.LoadRegistry(filepath.Join("..", "..", "SSTconfig"))
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "card.n4l")
	if err := os.WriteFile(p, []byte("- Ch\n# @id k1\n# @card\ncapital of France >> Paris\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, _ := content.ParseFile(p, reg)
	got := GenerateCards(pf)
	if len(got) != 1 || got[0].Front != "capital of France" || got[0].Back != "Paris" {
		t.Fatalf("real-registry basic: got %+v want 1 card Q='capital of France' A='Paris'", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

Run: `go test ./internal/learning/ -run TestGenerate -v`
Expected: FAIL — does not compile: `GenerateCards` still requires a `reg` argument / still keys on `Studyable`.

- [ ] **Step 3: Rewrite `internal/learning/cards.go`**

Replace the whole file:

```go
package learning

import (
	"strconv"
	"strings"

	"texere/internal/content"
)

// GenerateCards derives Phase-1 study cards from a parsed file. Basic and reverse
// cards split the node's literal text on the first ">>"; cloze cards blank each
// {{double-brace}} group. Nodes without a # @id are skipped (they cannot be keyed).
func GenerateCards(pf *content.ParsedFile) []GeneratedCard {
	var out []GeneratedCard
	if pf == nil {
		return out
	}
	for _, ch := range pf.Chapters {
		for _, node := range ch.Nodes {
			if node.StudyMeta == nil || node.ID == "" {
				continue
			}
			switch node.StudyMeta.CardType {
			case content.CardTypeCloze:
				out = append(out, clozeCards(node.ID, pf.Path, node.Text)...)
			case content.CardTypeBasic:
				out = append(out, basicCards(node.ID, pf.Path, node.Text, false)...)
			case content.CardTypeReverse:
				out = append(out, basicCards(node.ID, pf.Path, node.Text, true)...)
			}
		}
	}
	return out
}

// basicCards splits text on the first ">>" into front/back. A line without ">>"
// (or with an empty side after trimming) yields no card. With reverse, the swapped
// card is emitted too (Discriminator "1").
func basicCards(nodeID, filePath, text string, reverse bool) []GeneratedCard {
	front, back, ok := strings.Cut(text, ">>")
	if !ok {
		return nil
	}
	front = strings.TrimSpace(front)
	back = strings.TrimSpace(back)
	if front == "" || back == "" {
		return nil
	}
	out := []GeneratedCard{{
		NodeID: nodeID, FilePath: filePath, CardType: CardTypeBasic,
		Front: front, Back: back, Discriminator: "0",
	}}
	if reverse {
		out = append(out, GeneratedCard{
			NodeID: nodeID, FilePath: filePath, CardType: CardTypeBasic,
			Front: back, Back: front, Discriminator: "1",
		})
	}
	return out
}

// braceGroups returns the [start,end) byte ranges of each {{...}} group, in order.
func braceGroups(text string) [][2]int {
	var groups [][2]int
	i := 0
	for i+1 < len(text) {
		if text[i] == '{' && text[i+1] == '{' {
			if j := strings.Index(text[i+2:], "}}"); j >= 0 {
				end := i + 2 + j + 2
				groups = append(groups, [2]int{i, end})
				i = end
				continue
			}
			break
		}
		i++
	}
	return groups
}

// stripBraces removes every {{ and }} from text (the "back" rendering).
func stripBraces(text string) string {
	r := strings.NewReplacer("{{", "", "}}", "")
	return r.Replace(text)
}

func clozeCards(nodeID, filePath, text string) []GeneratedCard {
	groups := braceGroups(text)
	var out []GeneratedCard
	for idx := range groups {
		var b strings.Builder
		last := 0
		for k, gg := range groups {
			b.WriteString(stripBraces(text[last:gg[0]]))
			if k == idx {
				b.WriteString("[…]")
			} else {
				b.WriteString(stripBraces(text[gg[0]:gg[1]]))
			}
			last = gg[1]
		}
		b.WriteString(stripBraces(text[last:]))
		out = append(out, GeneratedCard{
			NodeID: nodeID, FilePath: filePath, CardType: CardTypeCloze,
			Front: b.String(), Back: stripBraces(text), Discriminator: strconv.Itoa(idx),
		})
	}
	return out
}
```

- [ ] **Step 4: Update the `Sync` caller**

In `internal/learning/sync.go`, change line 35 from:

```go
		cards := GenerateCards(pf, reg)
```

to:

```go
		cards := GenerateCards(pf)
```

(`reg` is still used elsewhere in `Sync` for `content.ParseFile`, so leave the parameter on `Sync`.)

- [ ] **Step 5: Run the new tests**

Run: `go test ./internal/learning/ -run 'TestGenerate|TestBasic|TestCloze' -v`
Expected: PASS (all five generation tests).

- [ ] **Step 6: Commit**

```bash
git add internal/learning/cards.go internal/learning/sync.go internal/learning/cards_test.go
git commit -m "$(cat <<'EOF'
feat(learning): generate cards from >> text and {{ }} cloze, drop arrow path

Basic/reverse cards split node text on the first >>; cloze blanks {{ }} groups.
GenerateCards no longer takes a registry or depends on ArrowDef.Studyable.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Migrate `sync_test.go` fixtures to `>>` / `{{ }}`

**Files:**
- Modify: `internal/learning/sync_test.go`

**Interfaces:**
- Consumes: `Sync`, `Store`, the rewritten `GenerateCards`.
- Produces: no API change — fixture/text updates so Sync tests exercise the new syntax and drop `Studyable`.

- [ ] **Step 1: Update the three affected fixtures**

In `internal/learning/sync_test.go`:

1. `TestSyncAssignsIDsUpsertsAndRetires` — change the cloze body (line ~17) from:

```go
	body := "- Ch\n# @cloze\nSystem 1 is {fast}\n"
```
to:
```go
	body := "- Ch\n# @cloze\nSystem 1 is {{fast}}\n"
```

2. `TestSyncMixedCardAndExtractFileIdempotent` — change the body (line ~105) from:

```go
	body := "- Ch\n# @card\nitem front (means) \"item back\"\nextracted sentence (from) \"sources/x.txt char=0\"\n"
```
to:
```go
	body := "- Ch\n# @card\nitem front >> item back\nextracted sentence (from) \"sources/x.txt char=0\"\n"
```

   and simplify its registry (line ~107–110) from:

```go
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{
		"means": {Code: "means", LongName: "means", Studyable: true},
		"from":  {Code: "from", LongName: "from"},
	}}
```
to:
```go
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{
		"from": {Code: "from", LongName: "from"},
	}}
```

3. `TestSyncDiscoversAndRetiresExtracts` — the promote step (line ~90) keeps the `(from)`
   node but is a cloze-style promotion; leave the `# @card` marker but ensure the assertion
   set is unaffected. Change line ~90 from:

```go
	body2 := "- Ch\n# @id e1\n# @card\nSystem 1 is fast (from) \"sources/f.txt char=0\"\n"
```
to:
```go
	body2 := "- Ch\n# @id e1\n# @cloze\nSystem 1 is fast (from) \"sources/f.txt char=0\"\n"
```

   (This node has a `(from)` anchor, so the parser keeps extraction and the marker retires
   the extract; the test only asserts `ExtractsRetired == 1`, which still holds.)

- [ ] **Step 2: Run the sync tests**

Run: `go test ./internal/learning/ -run TestSync -v`
Expected: PASS.

- [ ] **Step 3: Run the whole learning package**

Run: `go test ./internal/learning/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/learning/sync_test.go
git commit -m "$(cat <<'EOF'
test(learning): migrate sync fixtures to >> / {{ }} card syntax

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Remove the dead `ArrowDef.Studyable` field

**Files:**
- Modify: `internal/content/types.go` (remove the `Studyable` field, line ~122)

**Interfaces:**
- Consumes: nothing.
- Produces: `ArrowDef` without a `Studyable` field. No production or test code references it after Tasks 3–4.

- [ ] **Step 1: Confirm there are no remaining references**

Run: `grep -rn "Studyable" --include="*.go" .`
Expected: only the field declaration in `internal/content/types.go`. If any test still sets it, remove that line first.

- [ ] **Step 2: Remove the field**

In `internal/content/types.go`, delete the line:

```go
	Studyable     bool   // From SSTconfig comment metadata
```

- [ ] **Step 3: Build everything**

Run: `go build ./... && go test ./internal/content/ ./internal/learning/`
Expected: PASS (no compile errors).

- [ ] **Step 4: Commit**

```bash
git add internal/content/types.go
git commit -m "$(cat <<'EOF'
refactor(content): drop dead ArrowDef.Studyable field

Never populated in production; the card generator no longer reads it.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Update the manual test plan (Phase-1 cases)

**Files:**
- Modify: `docs/specs/2026-06-22-incremental-reading-manual-test-plan.md` (TC-1.1, TC-1.2)

**Interfaces:** none (documentation).

- [ ] **Step 1: Update TC-1.1**

Replace the TC-1.1 card fixture block:

```
   # @card
   capital of France (means) Paris
```
with:
```
   # @card
   What is the capital of France? >> Paris
```

and adjust the Expected bullet to read: "`SELECT count(*) FROM items;` increased by 1 (a basic card split on `>>`)."

- [ ] **Step 2: Update TC-1.2**

Replace the cloze fixture:

```
   # @cloze
   The mitochondrion is the {powerhouse} of the {cell}.
```
with:
```
   # @cloze
   The mitochondrion is the {{powerhouse}} of the {{cell}}.
```

and update the negative-check bullet to: "A cloze with **no** `{{braces}}` (or only single `{ }`) creates **no** item."

- [ ] **Step 3: Commit**

```bash
git add docs/specs/2026-06-22-incremental-reading-manual-test-plan.md
git commit -m "$(cat <<'EOF'
docs(test-plan): TC-1.1/1.2 use >> basic and {{ }} cloze syntax

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

## Part B — Promote "Make card" seeds a `>>` card (Phase-2b; depends on Part A)

### Task 7: `SeedBasicCardNode` content helper

**Files:**
- Modify: `internal/content/promote.go` (add `SeedBasicCardNode`)
- Test: `internal/content/promote_test.go`

**Interfaces:**
- Consumes: existing `readLines`, `writeLines`.
- Produces: `func SeedBasicCardNode(path string, nodeLine int, front string) error` — atomic rewrite that replaces line `nodeLine` with `front + " >> "` (preserving the original line's leading indentation) and inserts a `# @card` line directly above it.

- [ ] **Step 1: Write the failing test**

Add to `internal/content/promote_test.go`:

```go
func TestSeedBasicCardNode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ex.n4l")
	body := "- Ch\n# @id e1\n  extracted sentence (from) \"sources/x.txt char=0\"\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// The extract node is on line 3.
	if err := SeedBasicCardNode(p, 3, "extracted sentence"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want := "- Ch\n# @id e1\n# @card\n  extracted sentence >> \n"
	if string(got) != want {
		t.Fatalf("seeded file:\n%q\nwant:\n%q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/content/ -run TestSeedBasicCardNode -v`
Expected: FAIL — `SeedBasicCardNode` undefined.

- [ ] **Step 3: Implement `SeedBasicCardNode`**

Add to `internal/content/promote.go`:

```go
// SeedBasicCardNode rewrites the node at the 1-indexed nodeLine into a seeded
// basic card: the line becomes "<front> >> " (back left empty for the author),
// preserving the original line's leading indentation, and a "# @card" marker is
// inserted directly above it — in a single atomic rewrite. Used when promoting an
// extract to a basic card; the caller passes the extract's passage text as front
// and the original (from) anchor is intentionally dropped from the file.
func SeedBasicCardNode(path string, nodeLine int, front string) error {
	lines, err := readLines(path)
	if err != nil {
		return fmt.Errorf("content: SeedBasicCardNode: %w", err)
	}
	if nodeLine < 1 || nodeLine > len(lines) {
		return fmt.Errorf("content: SeedBasicCardNode: line %d out of range (1..%d)", nodeLine, len(lines))
	}
	idx := nodeLine - 1
	indent := lines[idx][:len(lines[idx])-len(strings.TrimLeft(lines[idx], " \t"))]
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, "# @card")
	out = append(out, indent+front+" >> ")
	out = append(out, lines[idx+1:]...)
	if err := writeLines(path, out); err != nil {
		return fmt.Errorf("content: SeedBasicCardNode: write: %w", err)
	}
	return nil
}
```

(`fmt` and `strings` are already imported in `promote.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/content/ -run TestSeedBasicCardNode -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/content/promote.go internal/content/promote_test.go
git commit -m "$(cat <<'EOF'
feat(content): SeedBasicCardNode rewrites an extract into a >> card stub

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: `PromoteExtract("card")` seeds the `>>` card

**Files:**
- Modify: `internal/learning`/Wails method `PromoteExtract` in `study_methods.go` (lines ~106–118)
- Test: `study_methods_test.go` if present, else verify via `internal/content` test from Task 7 + manual step

**Interfaces:**
- Consumes: `content.SeedBasicCardNode` (Task 7), existing `content.InsertMarkerAboveNode`, `learning.Sync`, `a.learnStore.GetExtract`.
- Produces: `PromoteExtract(id, "card")` seeds `<extract text> >> ` + `# @card` (no immediate item); `PromoteExtract(id, "cloze")` still inserts `# @cloze` only.

- [ ] **Step 1: Update `PromoteExtract` to branch on kind**

In `study_methods.go`, replace:

```go
	a.savingPath = e.FilePath
	wErr := content.InsertMarkerAboveNode(e.FilePath, line, "# @"+kind)
	a.savingPath = ""
	if wErr != nil {
		return wErr
	}
```

with:

```go
	a.savingPath = e.FilePath
	var wErr error
	if kind == "card" {
		// Seed a basic card stub "<passage> >> " for the author to complete; the
		// (from) anchor is dropped (provenance stays in the retired extract row).
		wErr = content.SeedBasicCardNode(e.FilePath, line, e.Text)
	} else {
		wErr = content.InsertMarkerAboveNode(e.FilePath, line, "# @cloze")
	}
	a.savingPath = ""
	if wErr != nil {
		return wErr
	}
```

- [ ] **Step 2: Build and run learning + app tests**

Run: `go build ./... && go test ./...`
Expected: PASS. (The existing `TestSyncDiscoversAndRetiresExtracts`, updated in Task 4 to a cloze promotion, still passes; the card branch is covered by Task 7 + Task 9 manual verification.)

- [ ] **Step 3: Commit**

```bash
git add study_methods.go
git commit -m "$(cat <<'EOF'
feat(study): PromoteExtract card seeds a >> stub instead of a bare marker

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: ProcessView — edit the seeded card, then create on confirm

**Files:**
- Modify: `frontend/src/components/ProcessView.svelte`

**Interfaces:**
- Consumes: existing `PromoteExtract`, `GetFileContent`, `SaveFile`, `GetExtractTarget`; add `SyncStudyItems` to the Wails import.
- Produces: after "→ Make card", the view stays open on the seeded `<passage> >> ` line (cursor at end) instead of advancing; a new "✓ Create card" action saves + `SyncStudyItems()` + advances.

- [ ] **Step 1: Add `SyncStudyItems` to the imports and a `seeding` flag**

Change the import line (line ~11) to add `SyncStudyItems`:

```js
  import { GetExtractTarget, PromoteExtract, DeferExtract, DismissExtract, GetFileContent, SaveFile, SyncStudyItems, GetRecentArrows, UpdateRecentArrows } from '../../wailsjs/go/main/App'
```

Add state near the other `let` declarations (after line ~21):

```js
  let seeding = false   // true after "Make card": author is filling in the back
```

- [ ] **Step 2: Replace `makeCard` with a seed-then-edit flow**

Replace:

```js
  const makeCard = () => act(() => PromoteExtract(extractId, 'card'))
```

with:

```js
  async function makeCard() {
    error = ''
    try {
      await flush()
      await PromoteExtract(extractId, 'card')   // seeds "<passage> >> " + # @card
      const text = (await GetFileContent(target.filePath)) ?? ''
      savedContent = text
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } })
      seeding = true
      focusSeedLine()
    } catch (e) { error = String(e?.message ?? e) }
  }

  // Place the cursor at the end of the seeded "... >> " line so the author types the back.
  function focusSeedLine() {
    if (!view) return
    const doc = view.state.doc
    for (let n = 1; n <= doc.lines; n++) {
      const line = doc.line(n)
      if (line.text.trimEnd().endsWith('>>')) {
        view.dispatch({ selection: { anchor: line.to }, effects: EditorView.scrollIntoView(line.to, { y: 'center' }) })
        view.focus()
        return
      }
    }
    view.focus()
  }

  async function createCard() {
    error = ''
    try {
      await flush()             // persist the filled-in back
      await SyncStudyItems()    // SaveFile does not sync; materialize the card now
      seeding = false
      onAdvance?.()
    } catch (e) { error = String(e?.message ?? e) }
  }
```

- [ ] **Step 3: Swap the action bar buttons while seeding**

Replace the action-bar button block (lines ~93–96):

```svelte
    <button class="pv-btn" on:click={makeCard}>→ Make card</button>
    <button class="pv-btn" on:click={makeCloze}>{'{ }'} Cloze</button>
    <button class="pv-btn" on:click={later}>⏳ Later</button>
    <button class="pv-btn" on:click={dismiss}>✕ Dismiss</button>
```

with:

```svelte
    {#if seeding}
      <span class="pv-anchor">Fill in the back after “&gt;&gt;”, then:</span>
      <button class="pv-btn" on:click={createCard}>✓ Create card</button>
    {:else}
      <button class="pv-btn" on:click={makeCard}>→ Make card</button>
      <button class="pv-btn" on:click={makeCloze}>{'{ }'} Cloze</button>
      <button class="pv-btn" on:click={later}>⏳ Later</button>
      <button class="pv-btn" on:click={dismiss}>✕ Dismiss</button>
    {/if}
```

- [ ] **Step 4: Build the frontend**

Run: `cd frontend && npm run build`
Expected: build succeeds with no errors. (If `SyncStudyItems` is missing from `wailsjs`, run `make build` / `wails generate module` first so bindings regenerate, then rebuild.)

- [ ] **Step 5: Manual verification**

Run: `make dev`. Open a collection, import a text source, create an extract, go to **Process**, click **→ Make card**. Verify:
- the editor stays open on the `<passage> >> ` line with the cursor after `>>`;
- type a back, click **✓ Create card**;
- the view advances and the new card is recallable (Ctrl+Shift+R) this session;
- `SELECT * FROM extracts WHERE id=...` shows `active=0`; `SELECT count(*) FROM items` rose by 1, keyed off the same NodeID base.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/ProcessView.svelte frontend/wailsjs
git commit -m "$(cat <<'EOF'
feat(process): Make card seeds an editable >> stub, Create card syncs it

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: Update the manual test plan (promote cases)

**Files:**
- Modify: `docs/specs/2026-06-22-incremental-reading-manual-test-plan.md` (TC-2b.3, TC-2b.4)

**Interfaces:** none (documentation).

- [ ] **Step 1: Update TC-2b.3 (Make card)**

Rewrite the Expected bullets to describe the seed flow:
- Clicking **→ Make card** rewrites the extract node to `<passage> >> ` with `# @card` above it and keeps the Process editor open for the author to type the back.
- Clicking **✓ Create card** saves and runs `SyncStudyItems()`; the `extracts` row is `active=0` and a new `items` row (same NodeID base) exists with `active=1`, recallable this session.

- [ ] **Step 2: Update TC-2b.4 (Cloze no braces)**

Change the marker reference to `{{braces}}`: "The `# @cloze` marker is written and the extract retires (`active=0`), but with no `{{braces}}` `GenerateCards` yields no item."

- [ ] **Step 3: Commit**

```bash
git add docs/specs/2026-06-22-incremental-reading-manual-test-plan.md
git commit -m "$(cat <<'EOF'
docs(test-plan): TC-2b.3 seed-card flow; TC-2b.4 {{ }} cloze

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

## Final verification

- [ ] Run the full suite: `make test` → all green.
- [ ] Run lint: `make lint` → clean.
- [ ] Manually run TC-1.1, TC-1.2, TC-2b.3 from the test plan against `make dev`.

---

## Self-review notes

- **Spec coverage:** §2 syntax → Tasks 1–3, 6; §3.1 narrowed skip rule → Task 1; §3.2 `{{ }}` → Tasks 2–3; §4 generator + `Studyable` removal → Tasks 3, 5; §6 tests → Tasks 1–4 (incl. real-registry regression); §8 promote seed-`>>` → Tasks 7–9; doc migration → Tasks 6, 10.
- **Type consistency:** `GenerateCards(pf)` (no `reg`) used identically in `cards.go`, `sync.go:35`, and all tests; `SeedBasicCardNode(path, nodeLine, front)` signature identical in Tasks 7 and 8; `CardTypeBasic` discriminators `"0"`/`"1"` consistent for reverse.
- **Part A is independently shippable** and closes the reported TC-1.1 bug on its own; Part B depends on Part A.
