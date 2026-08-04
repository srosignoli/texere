# Process Focus Notebook Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In Process mode, make the notebook pane follow the caret — showing the notebook of whatever node you are reading, or an empty state when it has none — while the action bar stays anchored to the extract the study queue supplied.

**Architecture:** A pure `content.NodeSpans` fold gives each id-bearing node a line range; a new `FileNodeSpans` binding hands that map to the frontend once per file, so caret movement resolves locally with no IPC. A new read-only `PeekNotebook` looks up a notebook without creating one. `ProcessView` routes between the two `NotebookPane` load paths that already exist.

**Tech Stack:** Go 1.25, Wails v2, Svelte 3.49, CodeMirror 6, `node:test`.

## Global Constraints

- **This feature NEVER creates a notebook and NEVER writes to a file.** It is read-only by construction. The one exception is the untouched existing branch where the caret is on the queued extract, which still uses `OpenNotebook`.
- **`PeekNotebook` must not reuse `OpenNotebook`.** `OpenNotebook` calls `UpsertNotebook` when none exists. Wired to a caret that would create a row for every node scrolled past, and `GetNotebookLinks` would then mark those lines as having notebooks — the feature would defeat itself, because a visited node would start having one.
- **Absence is not an error.** `PeekNotebook` returns `nil, nil` when there is no notebook. Returning an error would light the pane's error banner every time the caret rests on a plain line.
- **The action bar never retargets.** Promote, Cloze, Later and Dismiss keep acting on the queued `extractId`, so the study queue can never be advanced or resolved by accident. Do not change what any action targets.
- **The discriminator is "has an id", not "is an extract".** `GetNotebookByExtract` is a plain lookup on `extract_node_id` with no extract validation, so any id-bearing node can carry a notebook. Nodes with an empty `ID` are omitted from spans.
- **No "Start a notebook" affordance anywhere.** The queue is already how a notebook gets created.
- Follow the existing style of the files edited: Go `gofmt`, imports grouped stdlib/external/internal; JS 2-space indent, NO semicolons, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.

---

### Task 1: `content.NodeSpans`

**Files:**
- Modify: `internal/content/types.go` (add the `NodeSpan` type)
- Create: `internal/content/spans.go`
- Create: `internal/content/spans_test.go`

**Interfaces:**
- Consumes: `ParsedFile`, `Chapter`, `Node`, `DittoLine` (existing).
- Produces: `type NodeSpan struct { ID string; FromLine, ToLine int }` with JSON tags `id`, `fromLine`, `toLine`; and `func NodeSpans(pf *ParsedFile) []NodeSpan`.

**Why this is a pure function in `internal/content`.** The binding in `package main` needs a live `App`; this fold does not. Putting it here means it can be tested against real fixture files through the real parser, which is the only way to be sure a span actually covers what a user would call "that node".

**A node's span includes its `# @id` marker line.** `Node.IDMarkerLine` is the 1-indexed line of the `# @id` above the node, or 0 when absent. A caret resting on that line is, to the user, inside that node — so the span starts there when it exists.

- [ ] **Step 1: Write the failing test**

Create `internal/content/spans_test.go`:

```go
package content

import (
	"os"
	"path/filepath"
	"testing"
)

func spansOf(t *testing.T, src string) []NodeSpan {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.n4l")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := &SSTRegistry{
		Arrows:      map[string]ArrowDef{"means": {Code: "means", MetaType: "test"}},
		Annotations: map[string]AnnotDef{},
	}
	pf, _ := ParseFile(path, reg)
	if pf == nil {
		t.Fatal("ParseFile returned nil")
	}
	return NodeSpans(pf)
}

func find(t *testing.T, spans []NodeSpan, id string) NodeSpan {
	t.Helper()
	for _, s := range spans {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no span for %q in %+v", id, spans)
	return NodeSpan{}
}

// A node with no "# @id" cannot key a notebook, so it must not produce a span —
// otherwise the caret would resolve to an id of "" and peek nonsense.
func TestNodeSpansOmitsNodesWithoutAnID(t *testing.T) {
	spans := spansOf(t, "- C\n\nplain node\n\"  (means) something\n")
	if len(spans) != 0 {
		t.Fatalf("got %+v, want none", spans)
	}
}

func TestNodeSpansCoversTheNodeLine(t *testing.T) {
	src := "- C\n\n# @id abc\nNewton\n"
	s := find(t, spansOf(t, src), "abc")
	if s.FromLine > 3 || s.ToLine < 4 {
		t.Fatalf("got %+v, want a span covering the marker (3) and node (4) lines", s)
	}
}

// Dittos are the node's continuation lines; a caret on one is inside that node.
func TestNodeSpansExtendsThroughDittos(t *testing.T) {
	src := "- C\n\n# @id abc\nNewton\n\"  (means) a physicist\n\"  (means) also a mathematician\n"
	s := find(t, spansOf(t, src), "abc")
	if s.ToLine < 6 {
		t.Fatalf("ToLine = %d, want at least 6 (the last ditto)", s.ToLine)
	}
}

// The "# @id" line reads as part of the node it labels.
func TestNodeSpansStartsAtTheIDMarkerLine(t *testing.T) {
	src := "- C\n\n# @id abc\nNewton\n"
	s := find(t, spansOf(t, src), "abc")
	if s.FromLine != 3 {
		t.Fatalf("FromLine = %d, want 3 (the # @id line)", s.FromLine)
	}
}

func TestNodeSpansHandlesConsecutiveNodes(t *testing.T) {
	src := "- C\n\n# @id one\nAlpha\n\n# @id two\nBeta\n"
	spans := spansOf(t, src)
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2: %+v", len(spans), spans)
	}
	a, b := find(t, spans, "one"), find(t, spans, "two")
	if a.ToLine >= b.FromLine {
		t.Fatalf("spans overlap: %+v then %+v", a, b)
	}
}

func TestNodeSpansEmptyInputs(t *testing.T) {
	if got := NodeSpans(nil); got != nil && len(got) != 0 {
		t.Fatalf("NodeSpans(nil) = %+v, want empty", got)
	}
	if got := spansOf(t, "- C\n\n"); len(got) != 0 {
		t.Fatalf("got %+v, want none for a file with no nodes", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/content/ -run TestNodeSpans`
Expected: FAIL — `undefined: NodeSpan` / `undefined: NodeSpans`.

- [ ] **Step 3: Add the type**

In `internal/content/types.go`, after the `DittoLine` type:

```go
// NodeSpan is the line range a node occupies, for mapping a caret position back
// to the node it sits in — the reverse of looking a node up by its id.
//
// Only nodes with an ID get a span: the id is the only handle a notebook can
// hang on, so a node without one has nothing to resolve to.
type NodeSpan struct {
	ID       string `json:"id"`
	FromLine int    `json:"fromLine"` // 1-indexed, inclusive
	ToLine   int    `json:"toLine"`   // 1-indexed, inclusive
}
```

- [ ] **Step 4: Write `internal/content/spans.go`**

```go
package content

// NodeSpans returns the line range of every id-bearing node in a parsed file.
//
// The span starts at the node's "# @id" marker line when it has one: that line
// labels the node, and a caret resting on it is — to the reader — inside that
// node. It ends at the node's last ditto, since dittos are continuation lines of
// the same node rather than nodes of their own.
//
// Nodes without an ID are skipped. They cannot key a notebook, and emitting a
// span with an empty id would make a caret resolve to "" and peek nonsense.
func NodeSpans(pf *ParsedFile) []NodeSpan {
	out := []NodeSpan{}
	if pf == nil {
		return out
	}
	for ci := range pf.Chapters {
		for ni := range pf.Chapters[ci].Nodes {
			n := &pf.Chapters[ci].Nodes[ni]
			if n.ID == "" {
				continue
			}
			from, to := n.Line, n.Line
			if n.IDMarkerLine > 0 && n.IDMarkerLine < from {
				from = n.IDMarkerLine
			}
			for _, d := range n.Dittos {
				if d.Line > to {
					to = d.Line
				}
			}
			out = append(out, NodeSpan{ID: n.ID, FromLine: from, ToLine: to})
		}
	}
	return out
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/content/ -run TestNodeSpans -v`
Expected: PASS — all six.

- [ ] **Step 6: Commit**

```bash
git add internal/content/types.go internal/content/spans.go internal/content/spans_test.go
git commit -m "feat(content): map node ids to the line ranges they occupy"
```

---

### Task 2: `PeekNotebook` and `FileNodeSpans` bindings

**Files:**
- Modify: `notebook_methods.go`
- Modify: `notebook_methods_test.go`

**Interfaces:**
- Consumes: `content.NodeSpans`, `content.NodeSpan` (Task 1); the existing `NotebookView`, `assembleContext`, `chapterOfNode`.
- Produces, for the frontend:
  - `PeekNotebook(nodeID string) (*NotebookView, error)` — `null` when there is none
  - `FileNodeSpans(filePath string) ([]content.NodeSpan, error)`

**`PeekNotebook` is the load-bearing piece of this whole feature.** Its one job is to be `OpenNotebook` **without** the `UpsertNotebook`. If it ever creates, the feature inverts: browsing a node gives it a notebook, so the pane starts showing one for everything you have looked at.

Tests use the established pattern from `TestRecordPushAndGetLinks` in this file:
`st, _ := learning.Open(t.TempDir())` then `a := &App{learnStore: st, registry: reg, collectionDir: dir}`.

- [ ] **Step 1: Write the failing test**

Append to `notebook_methods_test.go`:

```go
func peekApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatalf("learning.Open: %v", err)
	}
	reg, _ := content.LoadRegistry(filepath.Join(t.TempDir(), "nonexistent"))
	return &App{learnStore: st, registry: reg, collectionDir: dir}, dir
}

// The whole feature rests on this: peeking must be a read. If it creates, then
// browsing a node gives it a notebook and the pane starts showing one for every
// node visited — the opposite of what this is for.
func TestPeekNotebookCreatesNothing(t *testing.T) {
	a, _ := peekApp(t)
	const id = "notes.n4l:C:id:never-seen"

	got, err := a.PeekNotebook(id)
	if err != nil {
		t.Fatalf("PeekNotebook: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil for a node with no notebook", got)
	}

	// The real assertion: nothing was written.
	if _, ok, err := a.learnStore.GetNotebookByExtract(id); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("PeekNotebook created a notebook row — it must only read")
	}
}

// Absence is the common case (any plain line), so it must not surface as an
// error, or the pane's error banner would flash constantly while navigating.
func TestPeekNotebookAbsenceIsNotAnError(t *testing.T) {
	a, _ := peekApp(t)
	got, err := a.PeekNotebook("nothing-here")
	if err != nil {
		t.Fatalf("absence must not be an error, got %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func TestPeekNotebookReturnsAnExistingNotebook(t *testing.T) {
	a, _ := peekApp(t)
	const id = "notes.n4l:C:id:abc"

	// OpenNotebook is the creating path; use it to establish the fixture.
	made, err := a.OpenNotebook(id)
	if err != nil {
		t.Fatalf("OpenNotebook: %v", err)
	}

	got, err := a.PeekNotebook(id)
	if err != nil {
		t.Fatalf("PeekNotebook: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want the notebook OpenNotebook just created")
	}
	if got.NotebookID != made.NotebookID {
		t.Fatalf("NotebookID = %q, want %q", got.NotebookID, made.NotebookID)
	}
	if got.NodeID != id {
		t.Fatalf("NodeID = %q, want %q", got.NodeID, id)
	}
}

func TestFileNodeSpansReturnsSpansForIDBearingNodes(t *testing.T) {
	a, dir := peekApp(t)
	path := filepath.Join(dir, "notes.n4l")
	if err := os.WriteFile(path, []byte("- C\n\n# @id abc\nNewton\n\nplain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spans, err := a.FileNodeSpans(path)
	if err != nil {
		t.Fatalf("FileNodeSpans: %v", err)
	}
	if len(spans) != 1 || spans[0].ID != "abc" {
		t.Fatalf("got %+v, want one span for abc (the plain node has no id)", spans)
	}
}

// A nil slice marshals to JSON null, and the frontend iterates the result.
func TestFileNodeSpansReturnsEmptyNotNilForAnUnparseableFile(t *testing.T) {
	a, dir := peekApp(t)
	spans, err := a.FileNodeSpans(filepath.Join(dir, "does-not-exist.n4l"))
	if err != nil {
		t.Fatalf("a missing file must not be an error here: %v", err)
	}
	if spans == nil {
		t.Fatal("got nil, want an empty slice — nil marshals to JSON null")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./ -run 'TestPeekNotebook|TestFileNodeSpans'`
Expected: FAIL — `a.PeekNotebook undefined`, `a.FileNodeSpans undefined`.

- [ ] **Step 3: Write the bindings**

Add to `notebook_methods.go`, after `OpenNotebook`:

```go
// PeekNotebook returns the notebook for a node WITHOUT creating one, or nil when
// the node has none.
//
// This is OpenNotebook minus its UpsertNotebook, and that difference is the
// whole point. The Process view calls this as the caret moves, so a creating
// lookup would write a notebook row for every node scrolled past, and
// GetNotebookLinks would then mark those lines as having notebooks — after one
// visit a node WOULD have one, so the pane would start showing it. The feature
// would defeat itself.
//
// Absence returns (nil, nil) rather than an error. Resting the caret on a plain
// line is the common case, not a failure, and an error would flash the pane's
// error banner constantly while navigating.
//
// Note GetNotebookByExtract is misnamed: it is a plain lookup on
// extract_node_id with no validation that the node is a registered extract, so
// it already serves any node id.
func (a *App) PeekNotebook(nodeID string) (*NotebookView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	nb, ok, err := a.learnStore.GetNotebookByExtract(nodeID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return &NotebookView{
		NotebookID: nb.NotebookID, NodeID: nodeID, Chapter: nb.Chapter,
		Cells: json.RawMessage(nb.CellsJSON), Context: a.assembleContext(nodeID),
	}, nil
}

// FileNodeSpans gives the frontend the line range of every id-bearing node in a
// file, so a caret position can be resolved to a node id locally.
//
// Fetched once per file rather than queried per keystroke: caret movement is
// constant, and a round trip plus a file parse on every arrow key would be a
// stream of work for a read that changes nothing.
//
// A missing or unparseable file yields an empty slice, not an error — the
// caller is a navigation aid, not an editor, and has nothing useful to do with
// the failure.
func (a *App) FileNodeSpans(filePath string) ([]content.NodeSpan, error) {
	pf, _ := content.ParseFile(filePath, a.registry)
	spans := content.NodeSpans(pf)
	if spans == nil {
		return []content.NodeSpan{}, nil
	}
	return spans, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./ -run 'TestPeekNotebook|TestFileNodeSpans' -v`
Expected: PASS — all five.

Then the whole suite: `make test`
Expected: green.

- [ ] **Step 5: Regenerate the Wails bindings**

`frontend/wailsjs/` is tracked in git. The `wails` binary is NOT on PATH — invoke it by absolute path:

Run: `/home/simone/go/bin/wails generate module`
Expected: `PeekNotebook` and `FileNodeSpans` appear in
`frontend/wailsjs/go/main/App.d.ts` and `App.js`, and a `content.NodeSpan` model
in `models.ts`. Report what you saw. Do not hand-edit the generated files.

Note: `git add frontend/wailsjs` (the bare directory) exits non-zero because
`frontend/wailsjs/runtime/` is gitignored, which silently aborts an `&&`-chained
commit. Add explicit file paths and check `git log` to confirm the commit landed.

- [ ] **Step 6: Verify the build**

Run: `go build ./... && GOOS=windows GOARCH=amd64 go build ./... && cd frontend && npx vite build`
Expected: all succeed.

- [ ] **Step 7: Commit**

```bash
git add notebook_methods.go notebook_methods_test.go \
  frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js frontend/wailsjs/go/models.ts
git commit -m "feat(notebook): peek a notebook without creating it, and map node spans"
```

---

### Task 3: `nodeIdAtLine`

**Files:**
- Create: `frontend/src/lib/node-span.js`
- Create: `frontend/src/lib/node-span.test.js`

**Interfaces:**
- Consumes: nothing.
- Produces: `nodeIdAtLine(spans, line) -> string | null`, where `spans` is the array from `FileNodeSpans` — objects shaped `{id, fromLine, toLine}`, both bounds 1-indexed and inclusive.

**Why its own dependency-free module.** This repo has no Svelte component test harness, so logic left inside a `.svelte` file has no tests at all. This is the only real logic on the frontend for this feature, and it is where every edge case lives.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/node-span.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { nodeIdAtLine } from './node-span.js'

// Mirrors what FileNodeSpans returns for:
//   3: # @id one     6: # @id two
//   4: Alpha         7: Beta
//   5: "  (means) x  8: "  (means) y
const spans = [
  { id: 'one', fromLine: 3, toLine: 5 },
  { id: 'two', fromLine: 6, toLine: 8 },
]

test('a line on the node itself resolves to that node', () => {
  assert.equal(nodeIdAtLine(spans, 4), 'one')
  assert.equal(nodeIdAtLine(spans, 7), 'two')
})

// The "# @id" line labels the node, so a caret there is inside it.
test('the id marker line resolves to the node it labels', () => {
  assert.equal(nodeIdAtLine(spans, 3), 'one')
  assert.equal(nodeIdAtLine(spans, 6), 'two')
})

// Dittos are continuation lines of the same node.
test('a ditto line resolves to its node', () => {
  assert.equal(nodeIdAtLine(spans, 5), 'one')
  assert.equal(nodeIdAtLine(spans, 8), 'two')
})

test('a line before the first node resolves to nothing', () => {
  assert.equal(nodeIdAtLine(spans, 1), null)
  assert.equal(nodeIdAtLine(spans, 2), null)
})

// Reachable in normal use: a blank line between two nodes.
test('a line between nodes resolves to nothing', () => {
  const gapped = [
    { id: 'one', fromLine: 3, toLine: 4 },
    { id: 'two', fromLine: 7, toLine: 8 },
  ]
  assert.equal(nodeIdAtLine(gapped, 5), null)
  assert.equal(nodeIdAtLine(gapped, 6), null)
})

// The span map goes stale while typing; an edit that shortens the file can leave
// the caret past every span. That must be quiet, not a crash.
test('a line past the end resolves to nothing', () => {
  assert.equal(nodeIdAtLine(spans, 99), null)
})

test('an empty or malformed span list resolves to nothing', () => {
  assert.equal(nodeIdAtLine([], 4), null)
  assert.equal(nodeIdAtLine(null, 4), null)
  assert.equal(nodeIdAtLine(undefined, 4), null)
})

test('a single-line node resolves on its only line', () => {
  assert.equal(nodeIdAtLine([{ id: 'solo', fromLine: 10, toLine: 10 }], 10), 'solo')
  assert.equal(nodeIdAtLine([{ id: 'solo', fromLine: 10, toLine: 10 }], 11), null)
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `make test-js`
Expected: FAIL — `Cannot find module '.../frontend/src/lib/node-span.js'`.

- [ ] **Step 3: Write `frontend/src/lib/node-span.js`**

```js
// Resolves a caret line to the node it sits in, from the span map FileNodeSpans
// returns.
//
// Deliberately dependency-free so Node's test runner can load it: this repo has
// no Svelte component harness, so anything left inside a .svelte file is
// untested, and this is where all the edge cases live.

// nodeIdAtLine returns the id of the node containing `line`, or null when the
// line is not inside any id-bearing node — a blank line, a chapter title, a
// context marker, a plain note with no id, or a position past the end of the
// file after an edit shortened it.
//
// Bounds are 1-indexed and inclusive at both ends, matching content.NodeSpan.
// Spans do not overlap, so the first match is the answer.
export function nodeIdAtLine(spans, line) {
  if (!Array.isArray(spans)) return null
  for (const s of spans) {
    if (line >= s.fromLine && line <= s.toLine) return s.id
  }
  return null
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS — the eight new tests plus every existing `lib/*.test.js`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/node-span.js frontend/src/lib/node-span.test.js
git commit -m "feat(notebook): resolve a caret line to the node it sits in"
```

---

### Task 4: `ProcessEditor` reports the caret line

**Files:**
- Modify: `frontend/src/components/ProcessEditor.svelte`

**Interfaces:**
- Consumes: nothing new.
- Produces: a `caret` Svelte event, `dispatch('caret', { line })`, where `line` is the 1-indexed line of the caret head. Also dispatches the existing `ready` event unchanged.

**Keep this component ignorant of nodes and notebooks.** It reports where the caret is; `ProcessView` decides what that means. That split is what keeps the editor from growing a second responsibility.

- [ ] **Step 1: Add the debounced listener**

`buildExtensions()` already passes an `extra:` array to `n4lBaseExtensions`. Add an
update listener there, and the debounce state beside the component's other
`let` declarations:

```js
  // Caret reporting is debounced because it fires on every keystroke and arrow
  // key, and the consumer does a backend peek. Resting on a line should report
  // once; scrolling through fifty should report once at the end.
  let caretTimer = null
  let lastCaretLine = 0

  function reportCaret(view) {
    const line = view.state.doc.lineAt(view.state.selection.main.head).number
    if (line === lastCaretLine) return
    lastCaretLine = line
    clearTimeout(caretTimer)
    caretTimer = setTimeout(() => dispatch('caret', { line }), 150)
  }
```

In `buildExtensions()`, extend `extra`:

```js
      extra: [
        notebookDotsGutter(() => {}),
        EditorView.updateListener.of((u) => {
          if (u.selectionSet || u.docChanged) reportCaret(u.view)
        }),
      ],
```

`EditorView` is already imported at line 3.

- [ ] **Step 2: Clear the timer on destroy**

A pending `setTimeout` firing after the component is gone would dispatch into
nothing. Add to the existing `onDestroy`:

```js
    clearTimeout(caretTimer)
```

- [ ] **Step 3: Report the caret on load**

After the editor is built and scrolled to the extract, the caret already sits on
a real line — but nothing has reported it, so `ProcessView` would start with no
focus. Immediately after the existing `dispatch('ready', { filePath: target.filePath })`,
add:

```js
      if (view) reportCaret(view)
```

- [ ] **Step 4: Verify**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS (unchanged — this component has no unit tests), build
succeeds with no new warnings.

- [ ] **Step 5: Verify by hand**

There is no unit test for a Svelte component in this repo, so confirm the wiring
by inspection and report what you checked:

1. `reportCaret` computes a **1-indexed** line — CodeMirror's `doc.lineAt(pos).number`
   is 1-indexed, matching `content.NodeSpan`. Confirm no `+1`/`-1` was added.
2. The listener fires on `selectionSet` (caret moves) and `docChanged` (edits
   shift lines).
3. The same-line guard means holding an arrow key within one line reports once.
4. `clearTimeout` is in `onDestroy`.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/ProcessEditor.svelte
git commit -m "feat(process): report the caret's line, debounced"
```

---

### Task 5: `ProcessView` follows the caret

**Files:**
- Modify: `frontend/src/components/ProcessView.svelte`

**Interfaces:**
- Consumes: `nodeIdAtLine` from `../lib/node-span.js` (Task 3); `PeekNotebook`, `FileNodeSpans` from `../../wailsjs/go/main/App` (Task 2); the `caret` event from `ProcessEditor` (Task 4).
- Produces: nothing later tasks rely on.

**No new mode on `NotebookPane`.** Both load paths already exist — `nodeID` creates via `OpenNotebook`, `notebookId` reads via `GetNotebook` — so routing between them keeps creation exactly where it was.

- [ ] **Step 1: Add the state and the resolution**

```js
  import { RecordPush, PeekNotebook, FileNodeSpans } from '../../wailsjs/go/main/App'
  import { nodeIdAtLine } from '../lib/node-span.js'

  let spans = []
  let focusedId = extractId   // the node under the caret; starts on the queued one
  let peeked = null           // NotebookView for focusedId, or null when it has none
  let peekedFor = null        // which id `peeked` describes, so a stale reply is ignored
  let caretLine = 0           // last reported caret line, kept so spans arriving later can re-resolve

  async function loadSpans() {
    if (!filePath) return
    try { spans = await FileNodeSpans(filePath) } catch { spans = [] }
    // The editor reports its caret as soon as it is ready, which can beat this
    // fetch — without re-resolving here, opening Process mode would land on the
    // empty state because the map was still [] when the first report arrived.
    if (caretLine) await resolve(caretLine)
  }

  async function onCaret(e) { await resolve(e.detail.line) }

  async function resolve(line) {
    caretLine = line
    const id = nodeIdAtLine(spans, line)
    if (id === focusedId) return
    focusedId = id
    // The queued extract keeps its creating path; nothing else is peeked until
    // it is actually focused.
    if (!id || id === extractId) { peeked = null; peekedFor = id; return }
    try {
      const view = await PeekNotebook(id)
      // A slower reply for a node we have already left must not win.
      if (focusedId !== id) return
      peeked = view
      peekedFor = id
    } catch { peeked = null; peekedFor = id }
  }
```

Extend the existing `onEditorReady` so the span map is fetched once the file
path is known:

```js
  function onEditorReady(e) {
    filePath = e.detail?.filePath ?? filePath
    loadSpans()
  }
```

**The `peekedFor` guard matters.** `PeekNotebook` is awaited, so two quick caret
moves can have their replies arrive out of order; without the check a stale
reply would overwrite the current one and show the wrong notebook — the exact
bug this feature exists to remove.

- [ ] **Step 2: Refetch spans after a save**

Editing shifts line numbers, so the map goes stale until the file is written.
`handlePush` already saves through `editorRef.insertAndSave`. Refresh there:

```js
  async function handlePush(text) {
    if (!editorRef) return
    const inserted = await editorRef.insertAndSave(text)
    if (notebookId && filePath && inserted) {
      try { await RecordPush(notebookId, filePath, inserted) } catch { /* provenance is best-effort */ }
    }
    await loadSpans()   // the insert moved every line below it
  }
```

- [ ] **Step 3: Wire the caret event to the editor**

The handler is useless until it is connected. In the left pane, add `on:caret` to
the existing `<ProcessEditor>` element, keeping every attribute it already has:

```svelte
    <ProcessEditor bind:this={editorRef} {extractId} {onAdvance}
                   on:ready={onEditorReady} on:caret={onCaret} />
```

- [ ] **Step 4: Route the pane and clear the push id**

Replace the single `<NotebookPane …>` in the right-hand pane with:

```svelte
  <div class="pv-pane pv-right">
    {#if focusedId !== extractId}
      <div class="pv-focus">
        Processing <b>{extractId}</b> · reading <b>{focusedId ?? 'a line with no note'}</b>
      </div>
    {/if}

    {#key focusedId}
      {#if focusedId === extractId}
        <NotebookPane nodeID={extractId} onPush={handlePush} on:ready={onReady} />
      {:else if peeked && peekedFor === focusedId}
        <NotebookPane notebookId={peeked.notebookId} onPush={handlePush} on:ready={onReady} />
      {:else}
        <div class="pv-empty">No notebook for this line.</div>
      {/if}
    {/key}
  </div>
```

And clear the provenance id when no pane is mounted:

```js
  function onReady(e) { notebookId = e.detail.notebookId }
  // Cleared whenever the empty state renders. Nothing can push from it today, so
  // a stale id is unreachable rather than harmless — clear it anyway, because the
  // next person to add an affordance there will not know it was load-bearing.
  $: if (focusedId !== extractId && !(peeked && peekedFor === focusedId)) notebookId = null
```

- [ ] **Step 5: Add the styles**

```css
  .pv-focus { padding: 4px 8px; border-bottom: 1px solid #2a3a4a; color: #5d7186; font-size: 11px; }
  .pv-focus b { color: #93c5fd; font-weight: 600; }
  .pv-empty { padding: 16px; color: #5d7186; font-size: 12px; }
```

The right pane is `position: relative` with `NotebookPane` absolutely positioned
inside it, so the header and empty state sit in normal flow above it without
disturbing the pane's layout.

- [ ] **Step 6: Verify**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS, build succeeds with no unused-CSS-selector warning for this
file.

- [ ] **Step 7: Verify by hand**

Run `make dev`, enter Process mode on an extract in a file containing several:

1. The pane opens on the queued extract's notebook; no focus header is shown.
2. Move the caret into another extract **that has a notebook** — the pane switches
   to it and the header names both.
3. Move into an extract **with no notebook** — `No notebook for this line.`
4. Move onto a blank line or a chapter title — the same empty state.
5. Move back into the queued extract — its notebook returns, header disappears.
6. With the caret in another extract, click **Promote** — it acts on the **queued**
   extract, not the one you are reading.
7. Edit a peeked notebook's cell, move away, come back — the edit persisted.
8. **Nothing was created:** open `learning.db` and confirm `SELECT count(*) FROM
   notebooks` did not grow from browsing. This is invisible in the UI and is the
   single most important check.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/ProcessView.svelte
git commit -m "feat(process): the notebook pane follows the caret"
```

---

### Task 6: Manual test plan document

**Files:**
- Create: `docs/superpowers/specs/2026-08-03-process-focus-notebook-manual-test-plan.md`

**Interfaces:**
- Consumes: the behaviour built in Tasks 1-5.
- Produces: nothing consumed by code.

Every prior slice shipped one, because this repo has no Svelte component harness
and the caret-following behaviour is only observable in the running app. Fold in
every deferred check from the Task 4 and Task 5 reports — read both to the end,
since later fix rounds append further sections.

- [ ] **Step 1: Write the document**

Create the file with this content:

```markdown
# Process focus notebook — manual test plan

**Design:** `2026-08-03-process-focus-notebook-design.md`
**Build:** `make dev`

Automated coverage: `content.NodeSpans` (dittos, id-less nodes, consecutive
nodes, the `# @id` marker line) and `PeekNotebook`/`FileNodeSpans` — including
that peeking creates nothing — via `make test`; `nodeIdAtLine`'s edge cases via
`make test-js`. Everything below is what those cannot reach: the caret-following
behaviour itself, which exists only in the running app.

Prereqs: a collection with a `.n4l` file containing at least three extracts, of
which at least one has a notebook and one does not, plus some plain notes and a
chapter title.

## TC-1 The queued extract opens as before

Enter Process mode on an extract.

**Expect:** the right pane shows that extract's notebook, exactly as today. No
focus header is displayed, because what you are reading and what you are
processing are the same.

## TC-2 The pane follows the caret to another extract with a notebook

Move the caret into a different extract that already has a notebook.

**Expect:** the pane switches to that extract's notebook, and a header appears
naming both — what is being processed and what is being read.

## TC-3 An extract with no notebook shows the empty state

Move the caret into an extract that has never been processed.

**Expect:** `No notebook for this line.` — not the previous extract's notebook,
and not an error banner.

## TC-4 A line that cannot carry a notebook shows the same empty state

Move the caret onto a blank line, a chapter title (`- Something`), a context
marker (`:: something ::`), and a plain note you typed that has no `# @id`.

**Expect:** the empty state in every case. No error, no flicker of a previous
notebook.

## TC-5 Returning to the queued extract restores it

Move the caret back into the extract you are processing.

**Expect:** its notebook returns and the focus header disappears.

## TC-6 The action bar never retargets

With the caret sitting in a **different** extract, click **Promote**.

**Expect:** it acts on the **queued** extract — the one named as "processing" in
the header — not the one you are reading. Repeat for **Later**: the queue
advances past the queued extract, not the one under the caret.

*(This is the guarantee that keeps the study queue safe. If it fails, an extract
you merely looked at could be resolved without you asking.)*

## TC-7 A peeked notebook is editable and persists

With the caret in another extract that has a notebook, add a cell and type in it.
Move the caret away, then back.

**Expect:** the edit is still there. It was saved to **that** extract's notebook,
not to the queued one — confirm by returning to the queued extract and seeing it
unchanged.

## TC-8 Browsing creates nothing — the most important check

Note the current count first:

```
sqlite3 <collectionDir>/learning.db 'SELECT count(*) FROM notebooks;'
```

Now move the caret through a dozen extracts and plain lines that have no
notebooks. Re-run the count.

**Expect:** unchanged. Browsing must never create a notebook row.

*(This is invisible in the UI. If it fails, every node you visit gains a
notebook, the file browser starts marking those lines as having notebooks, and
the feature inverts — a visited extract would begin showing one.)*

## TC-9 The staleness window, and that it self-corrects

Put the caret in an extract and note which notebook shows. Now insert several
blank lines **above** it without saving, and move the caret back into that same
extract.

**Expect:** possibly the wrong notebook or the empty state — the span map is
stale between saves and this is accepted, not a bug. Now save (Ctrl+S) and move
the caret again.

**Expect:** correct again. The window is bounded by the next save.

## TC-10 Push still records against the right notebook

With the caret in another extract that has a notebook, push an N4L draft from
that notebook into the file.

**Expect:** the text is inserted and no error appears. The push is recorded
against the notebook that pushed it — the one you were reading.

## TC-11 Rapid caret movement does not show a stale notebook

Move the caret quickly between two extracts that both have notebooks, several
times in succession.

**Expect:** the pane settles on the notebook of wherever the caret finally
rests — never on the other one. *(Replies from the backend can arrive out of
order; the view discards any that no longer match the focused node.)*

## TC-12 A file with no id-bearing nodes at all

Open Process mode on an extract, then move the caret into a file region — or a
file — containing only plain notes.

**Expect:** the empty state throughout, no errors, and no console noise.
```

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-08-03-process-focus-notebook-manual-test-plan.md
git commit -m "docs(test-plan): process focus notebook"
```

---

## Spec coverage

| Spec requirement | Task |
|---|---|
| `NodeSpan{ID, FromLine, ToLine}`; span covers `# @id` line through last ditto | 1 |
| Nodes without an ID produce no span | 1 |
| `PeekNotebook` returns the notebook, or nil when absent | 2 |
| **`PeekNotebook` creates nothing** — asserted directly against the store | 2 |
| Absence is not an error | 2 |
| `FileNodeSpans` returns a map, empty (not nil) on failure | 2 |
| Fetched once per file, not queried per keystroke | 2 (binding), 5 (caller) |
| `nodeIdAtLine` with all the edge cases | 3 |
| Caret reported debounced, 1-indexed, editor stays ignorant of notebooks | 4 |
| Span map refetched after a save | 5 |
| Four-state routing to the two existing `NotebookPane` paths | 5 |
| Creation stays on the untouched queued-extract branch | 5 |
| Header names both subjects when they differ | 5 |
| `{#key focusedId}` remount | 5 |
| Push provenance id cleared when the empty state renders | 5 |
| Out-of-order peek replies discarded | 5 |
| Manual test plan | 6 |

Nothing in the spec's out-of-scope list is built: no "Start a notebook"
affordance, no `# @id` injection, no rename of `GetNotebookByExtract`, no change
to the study queue or to what the action bar targets, no change to `SourceTab`'s
use of `NotebookPane`, and the empty state does not distinguish an unprocessed
extract from a node that cannot carry a notebook.
