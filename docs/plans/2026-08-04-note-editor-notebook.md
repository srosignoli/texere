# Note Editor Notebook Implementation Plan

> **⚠️ SUPERSEDED IN PART — read this before trusting the code below.**
> Task 1 shipped as `NotebooksForNodes(nodeIDs []string)`, **not** the
> `NotebooksForFile(filePath)` prefix-range design written here. Review found the
> range `[path+":", path+";")` can return a *different* file's notebooks: a colon
> is legal in a filename, so `/c/notes.n4l:bak.n4l:Ch:id:x` falls inside the range
> built for `/c/notes.n4l` (reproduced against the real SQLite engine). Task 2's
> binding parses the file first and queries by exact ids. Task 4's `loadSpans` and
> the reload paths also gained fixes not described here. **The shipped code and
> `.superpowers/sdd/2026-08-04-note-editor-notebook/progress.md` are authoritative;
> this document records the original plan.**

**Goal:** In the note editor, a gutter dot means "this node has a notebook", and the read-only overlay — once opened by a dot click — follows the caret and hides when the focused node has none.

**Architecture:** A new `NotebookMarks` binding resolves the file's notebooks to line numbers by node id, replacing the text-keyed `GetNotebookLinks` in the gutter. `SourceTab` then reuses the machinery 032 shipped — `FileNodeSpans`, `nodeIdAtLine`, `PeekNotebook` — to resolve the caret locally and route the overlay.

**Tech Stack:** Go 1.25, SQLite (`internal/learning`), Wails v2 bindings, Svelte 3.49, CodeMirror 6.

## Global Constraints

- **Read-only.** This feature creates no notebook and writes to no file. Use `PeekNotebook`, never `OpenNotebook`.
- **No schema migration.** `notebooks.extract_node_id` is already node-id keyed and indexed (`internal/learning/db.go:170`). Do not alter any table.
- **Never write `# @id` into a note.** This is an explicit constraint carried from the 032 design.
- **Leave `notebook_links`, `RecordPush` and `GetNotebookLinks` alone.** Push provenance keeps being recorded; it simply stops driving the gutter. Do not delete them.
- Node ids are fully qualified: `<path>:<chapter>:id:<short>`. Never assume the short form.
- Go: `gofmt`, imports grouped stdlib / external / internal. JS/Svelte: 2-space indent, **no semicolons**, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.
- `wails` is not on PATH; it is at `/home/simone/go/bin/wails`.
- `git add frontend/wailsjs` (bare dir) exits non-zero because `frontend/wailsjs/runtime/` is gitignored. Add explicit file paths and confirm with `git log`.

## MANDATORY: negative-control every test you write

This project has shipped tests that passed against the exact bug they were written to catch. Before committing, for each behaviour asserted: break the production code in the specific way the test exists to catch, run the test, confirm it **fails with a message naming the real problem**, then restore and re-run green. Report the actual failure output for the most load-bearing assertion. "Verified" without the output will be sent back.

## File Structure

- `internal/learning/notebooks.go` — add `NotebooksForFile`, a one-query lookup of a file's notebooks by node id.
- `internal/learning/notebooks_test.go` — its tests.
- `notebook_methods.go` — add `NotebookMark` and the `NotebookMarks` binding.
- `notebook_methods_test.go` — its tests.
- `frontend/src/components/SourceTab.svelte` — gutter source, caret tracking, overlay routing.
- `docs/specs/2026-08-04-note-editor-notebook-manual-test-plan.md` — the manual gate.

---

### Task 1: `NotebooksForFile` store lookup

**Files:**
- Modify: `internal/learning/notebooks.go`
- Test: `internal/learning/notebooks_test.go`

**Interfaces:**
- Consumes: the existing `notebooks` table and `Store`.
- Produces: `func (s *Store) NotebooksForFile(filePath string) (map[string]string, error)` — node id → notebook id.

- [ ] **Step 1: Write the failing test**

Append to `internal/learning/notebooks_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/learning/ -run NotebooksForFile -v`
Expected: FAIL — `st.NotebooksForFile undefined`.

- [ ] **Step 3: Implement**

Add to `internal/learning/notebooks.go`:

```go
// NotebooksForFile returns node id -> notebook id for every notebook whose node
// lives in filePath.
//
// Node ids are "<path>:<chapter>:id:<short>", so a file's notebooks form a
// contiguous range on the indexed extract_node_id column. A range scan is used
// rather than LIKE because a file path may legitimately contain % or _, which
// LIKE would treat as wildcards and silently match other files. ';' is ':'+1,
// so [path+":", path+";") bounds the prefix exactly.
//
// Where a node somehow has two notebooks, the earliest wins — matching
// GetNotebookByExtract's "ORDER BY created_at ASC LIMIT 1".
func (s *Store) NotebooksForFile(filePath string) (map[string]string, error) {
	rows, err := s.db.Query(
		`SELECT extract_node_id, notebook_id FROM notebooks
		 WHERE extract_node_id >= ? AND extract_node_id < ?
		 ORDER BY created_at ASC`, filePath+":", filePath+";")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var nodeID, nbID string
		if err := rows.Scan(&nodeID, &nbID); err != nil {
			return nil, err
		}
		if _, dup := out[nodeID]; !dup {
			out[nodeID] = nbID
		}
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/learning/ -run NotebooksForFile -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Negative control**

Change the query to `LIKE ? || ':%'` with `filePath` and re-run. `TestNotebooksForFileTreatsThePathLiterally` must FAIL. Restore, confirm `git status --porcelain` shows only the intended files, re-run green. Report the failure output.

- [ ] **Step 6: Commit**

```bash
git add internal/learning/notebooks.go internal/learning/notebooks_test.go
git commit -m "feat(learning): look up a file's notebooks by node id"
```

---

### Task 2: `NotebookMarks` binding

**Files:**
- Modify: `notebook_methods.go`
- Test: `notebook_methods_test.go`
- Regenerate: `frontend/wailsjs/go/main/App.d.ts`, `App.js`, `models.ts`

**Interfaces:**
- Consumes: `NotebooksForFile` (Task 1), `content.ParseFile`.
- Produces, for the frontend: `NotebookMarks(filePath string) ([]NotebookMark, error)` where `NotebookMark{Line int json:"line"; NotebookID string json:"notebookId"}`.

- [ ] **Step 1: Write the failing test**

Append to `notebook_methods_test.go`. Use the established App-with-store pattern from `TestRecordPushAndGetLinks` in this file.

```go
func TestNotebookMarksMarksOnlyNodesThatHaveANotebook(t *testing.T) {
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := content.LoadRegistry(filepath.Join(dir, "nonexistent"))
	a := &App{learnStore: st, registry: reg, collectionDir: dir}

	p := filepath.Join(dir, "f.n4l")
	body := "- Ch\n\n# @id has\nnode with a notebook\n\n# @id none\nnode without one\n\na node with no id at all\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Give only the "has" node a notebook, via the real creating path.
	pf, _ := content.ParseFile(p, reg)
	var hasID string
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if strings.HasSuffix(n.ID, ":id:has") {
				hasID = n.ID
			}
		}
	}
	if hasID == "" {
		t.Fatal("fixture: could not find the has node")
	}
	if _, err := a.OpenNotebook(hasID); err != nil {
		t.Fatal(err)
	}

	marks, err := a.NotebookMarks(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 1 {
		t.Fatalf("got %+v, want exactly one mark (only the node with a notebook)", marks)
	}
	if marks[0].Line != 4 {
		t.Errorf("mark on line %d, want 4 (the node line, not the # @id marker line)", marks[0].Line)
	}
	if marks[0].NotebookID == "" {
		t.Error("mark carries no notebook id, so a dot click would have nothing to open")
	}
}

// The dot must survive an edit to the node's text. That is the whole reason for
// keying on node id: the previous text-keyed links dropped the dot silently.
func TestNotebookMarksSurviveANodeTextEdit(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	reg, _ := content.LoadRegistry(filepath.Join(dir, "nonexistent"))
	a := &App{learnStore: st, registry: reg, collectionDir: dir}

	p := filepath.Join(dir, "f.n4l")
	os.WriteFile(p, []byte("- Ch\n\n# @id keep\noriginal text\n"), 0o644)
	pf, _ := content.ParseFile(p, reg)
	id := pf.Chapters[0].Nodes[0].ID
	if _, err := a.OpenNotebook(id); err != nil {
		t.Fatal(err)
	}

	// Rewrite the node's text, exactly as making a card or any edit would.
	os.WriteFile(p, []byte("- Ch\n\n# @id keep\ncompletely different text now\n"), 0o644)

	marks, err := a.NotebookMarks(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 1 {
		t.Fatalf("got %+v, want the mark to survive the edit", marks)
	}
}

func TestNotebookMarksCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	reg, _ := content.LoadRegistry(filepath.Join(dir, "nonexistent"))
	a := &App{learnStore: st, registry: reg, collectionDir: dir}

	p := filepath.Join(dir, "f.n4l")
	os.WriteFile(p, []byte("- Ch\n\n# @id fresh\nnever opened\n"), 0o644)

	marks, err := a.NotebookMarks(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 0 {
		t.Fatalf("got %+v, want none", marks)
	}
	// Asserting the empty result is not enough — query the store to prove no row
	// was written. A creating implementation would return marks on the SECOND
	// call and look correct on the first.
	nbs, err := st.NotebooksForFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(nbs) != 0 {
		t.Fatalf("NotebookMarks created %d notebook row(s) — it must only read", len(nbs))
	}
}

func TestNotebookMarksEmptyNotNilForAFileWithNoNodes(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	reg, _ := content.LoadRegistry(filepath.Join(dir, "nonexistent"))
	a := &App{learnStore: st, registry: reg, collectionDir: dir}
	p := filepath.Join(dir, "empty.n4l")
	os.WriteFile(p, []byte(""), 0o644)

	marks, err := a.NotebookMarks(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if marks == nil {
		t.Fatal("returned a nil slice; it marshals to null instead of [] and the caller indexes it")
	}
}
```

No import changes are needed: `notebook_methods_test.go` already imports `os`, `path/filepath`, `strings`, `testing`, `texere/internal/content` and `texere/internal/learning`.

The fixture's line numbers were verified against the real parser: `has` is on line 4, `none` on line 7, and the id-less node on line 9. The mark must land on the **node** line (4), not the `# @id` marker line (3).

- [ ] **Step 2: Run to verify it fails**

Run: `go test . -run NotebookMarks -v`
Expected: FAIL — `a.NotebookMarks undefined`.

- [ ] **Step 3: Implement**

Add to `notebook_methods.go`, directly below `NotebookLinkMark`:

```go
// NotebookMark is one gutter dot: a line whose node has a notebook.
//
// This replaces NotebookLinkMark as the gutter's source. The old mark meant
// "text on this line was pushed out of a notebook" and was keyed by
// (chapter, node_text), so any edit to the line silently dropped the dot. This
// one is keyed by node id, which is stable across edits.
type NotebookMark struct {
	Line       int    `json:"line"`
	NotebookID string `json:"notebookId"`
}

// NotebookMarks returns the lines of nodes in filePath that have a notebook.
//
// Read-only: it never creates a notebook. One store query for the file, then a
// single parse to turn node ids into line numbers.
func (a *App) NotebookMarks(filePath string) ([]NotebookMark, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	byNode, err := a.learnStore.NotebooksForFile(filePath)
	if err != nil {
		return nil, err
	}
	out := []NotebookMark{}
	if len(byNode) == 0 {
		return out, nil
	}
	pf, _ := content.ParseFile(filePath, a.registry)
	if pf == nil {
		return out, nil
	}
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.ID == "" {
				continue
			}
			if nbID, ok := byNode[n.ID]; ok {
				out = append(out, NotebookMark{Line: n.Line, NotebookID: nbID})
			}
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test . -run NotebookMarks -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Negative control**

Make `NotebookMarks` call `a.OpenNotebook(n.ID)` instead of reading `byNode`. `TestNotebookMarksCreatesNothing` must FAIL on the store re-query. Restore and re-run green. Report the failure output.

- [ ] **Step 6: Regenerate the bindings**

```bash
/home/simone/go/bin/wails generate module
```

Confirm `NotebookMarks` appears in `frontend/wailsjs/go/main/App.d.ts` and `NotebookMark` in `models.ts`.

- [ ] **Step 7: Commit**

```bash
git add notebook_methods.go notebook_methods_test.go \
  frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js frontend/wailsjs/go/models.ts
git commit -m "feat(notebook): mark the lines of nodes that have a notebook"
git log --oneline -1
```

---

### Task 3: the gutter reads `NotebookMarks`

**Files:**
- Modify: `frontend/src/components/SourceTab.svelte`

**Interfaces:**
- Consumes: `NotebookMarks` (Task 2), returning `[{ line, notebookId }]`.
- Produces: `dotMarks` in the new shape, for Task 5's dot click.

There is no Svelte component harness in this repo, so this task has no unit test. Verify statically and via the build, and state in your report what a user would see if each change were wrong.

- [ ] **Step 1: Swap the import**

In `SourceTab.svelte:12`, replace `GetNotebookLinks` with `NotebookMarks` in the import from `'../../wailsjs/go/main/App'`. Leave every other import untouched.

- [ ] **Step 2: Rewrite the dot source**

Replace the `refreshDots` function (`:38-44`) and the `dotMarks` comment (`:26-27`) with:

```js
  // Gutter dots: lines whose node has a notebook. Keyed by node id server-side,
  // so a dot survives an edit to the line's text — the text-keyed links this
  // replaced dropped the dot on any edit, including the rewrite that making a
  // card performs.
  let dotMarks = []      // [{line, notebookId}]

  async function refreshDots(path) {
    try {
      dotMarks = (await NotebookMarks(path)) ?? []
      if (view) applyDots(view, dotMarks.map((m) => m.line))
    } catch { /* best-effort */ }
  }
```

- [ ] **Step 3: Update the dot click to the new shape**

Replace `onDotClick` (`:29-35`) with:

```js
  function onDotClick(line) {
    const m = dotMarks.find((x) => x.line === line)
    if (m) openNotebookReadOnly(line)
  }
```

`openNotebookReadOnly` is rewritten in Task 5 to take a line. Until then it takes an id, so this step leaves the click temporarily mismatched — that is expected and Task 5 closes it. Note this in your report.

- [ ] **Step 4: Verify**

Run: `make test-js && cd frontend && npx vite build`
Expected: build succeeds. Confirm with `grep -n 'GetNotebookLinks' frontend/src/components/SourceTab.svelte` that no reference remains (it must return nothing).

`notebook:links-changed` (`:324-325`) still triggers `refreshDots` — leave that wiring alone; a push can now change marks only indirectly, but re-fetching is harmless and keeps the dot fresh after any notebook change.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/SourceTab.svelte
git commit -m "feat(notebook): the gutter marks nodes that have a notebook"
```

---

### Task 4: `SourceTab` tracks the caret and loads spans

**Files:**
- Modify: `frontend/src/components/SourceTab.svelte`

**Interfaces:**
- Consumes: `FileNodeSpans`, `PeekNotebook` from `'../../wailsjs/go/main/App'`; `nodeIdAtLine` from `'../lib/node-span.js'`.
- Produces: `focusedId`, `focusedNb`, `focusedNbFor` for Task 5.

No unit test — same reason as Task 3. Four properties to verify statically and evidence in your report, each with the concrete symptom if wrong: the reported line is 1-indexed; the listener fires on **both** `selectionSet` and `docChanged`; a same-line guard prevents re-dispatch; the debounce timer is cleared in `onDestroy`.

- [ ] **Step 1: Add imports and state**

Extend the App import (`:12`) with `FileNodeSpans, PeekNotebook`, and add below the other imports:

```js
  import { nodeIdAtLine } from '../lib/node-span.js'
```

Add beside the other state:

```js
  // Caret -> node -> notebook, for the read-only overlay. Same approach as
  // ProcessView: fetch the line map once per file and resolve locally, because
  // caret movement is constant and a round trip per keystroke would be a stream
  // of work for a read that changes nothing.
  let spans = []
  let caretLine = 0
  let focusedId = null
  let focusedNb = null      // NotebookView for focusedId, or null when it has none
  let focusedNbFor = null   // which id focusedNb describes, so a stale reply loses
  let caretTimer = null
  let lastCaretLine = 0

  async function loadSpans(path) {
    if (!path) return
    try { spans = await FileNodeSpans(path) } catch { spans = [] }
    if (caretLine) await resolveCaret(caretLine)
  }

  async function resolveCaret(line) {
    caretLine = line
    const id = spans.length ? nodeIdAtLine(spans, line) : null
    if (id === focusedId) return
    focusedId = id
    // Moving to a different node drops back to read-only: "Resume (edit)" was
    // granted for the notebook you were looking at, not the next one.
    reopenReadOnly = true
    if (!id) { focusedNb = null; focusedNbFor = null; return }
    try {
      const v = await PeekNotebook(id)
      if (focusedId !== id) return   // a slower reply for a node we have left
      focusedNb = v
      focusedNbFor = id
    } catch { focusedNb = null; focusedNbFor = null }
  }

  function reportCaret(v) {
    const line = v.state.doc.lineAt(v.state.selection.main.head).number
    if (line === lastCaretLine) return
    lastCaretLine = line
    clearTimeout(caretTimer)
    caretTimer = setTimeout(() => resolveCaret(line), 150)
  }
```

- [ ] **Step 2: Install the update listener**

In the `extra:` array (`:283-284`), alongside `notebookDotsGutter(onDotClick)`:

```js
      extra: [
        notebookDotsGutter(onDotClick),
        EditorView.updateListener.of((u) => {
          if (u.selectionSet || u.docChanged) reportCaret(u.view)
        }),
      ],
```

`EditorView` is already imported at `:4`.

- [ ] **Step 3: Report the caret after the view is (re)built**

`rebuildEditor()` (`:300-305`) destroys and constructs a new `EditorView`. **Construction does not fire `updateListener` — only `view.dispatch` does.** Without an explicit report the overlay would keep showing the pre-rebuild node's notebook while the caret sits elsewhere. This exact bug shipped in 032 and had to be fixed after review.

Immediately after the `view = new EditorView(...)` line in `rebuildEditor` (`:305`), and after the one in the initial mount (`:321`), add:

```js
    reportCaret(view)
```

Unconditional in both places; the same-line guard collapses a duplicate.

- [ ] **Step 4: Load spans with the file, and refresh after a save**

Where `refreshDots(loadedFilePath)` is called after a file loads (`:81` and `:307`), add `loadSpans(loadedFilePath)` immediately after each.

After the successful `SaveFile` at `:239`, add:

```js
      await loadSpans(loadedFilePath)   // a save can shift every line below an edit
```

- [ ] **Step 5: Clear the timer on teardown**

In the existing `onDestroy`, add `clearTimeout(caretTimer)`. Without it a pending callback fires against a destroyed component.

- [ ] **Step 6: Verify**

Run: `make test-js && cd frontend && npx vite build`
Expected: build succeeds, no unused-variable or unused-CSS warning for this file.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/SourceTab.svelte
git commit -m "feat(notebook): the note editor resolves the caret to a node"
```

---

### Task 5: the overlay follows the caret

**Files:**
- Modify: `frontend/src/components/SourceTab.svelte`

**Interfaces:**
- Consumes: `focusedId`, `focusedNb`, `focusedNbFor` (Task 4); `dotMarks` (Task 3).
- Produces: nothing later tasks rely on.

- [ ] **Step 1: Replace the overlay state**

Replace `reopenId` / `openNotebookReadOnly` / `closeReopen` (`:46-49`) with:

```js
  // The overlay is "armed" by a dot click and stays armed until Close. While
  // armed it shows whatever notebook the focused node has, and hides when that
  // node has none — so it can never display a notebook for a note you are not
  // reading. Close is sticky: without that, moving the caret would reopen what
  // you just dismissed.
  let armed = false
  let reopenReadOnly = true
  function openNotebookReadOnly(line) {
    armed = true
    reopenReadOnly = true
    resolveCaret(line)
  }
  function closeReopen() { armed = false; reopenReadOnly = true }
  function resumeReopen() { reopenReadOnly = false }
```

- [ ] **Step 2: Route the overlay**

Replace the overlay block (`:355-366`) with:

```svelte
  {#if armed && focusedNb && focusedNbFor === focusedId}
    <div class="nb-overlay">
      <div class="nb-overlay-bar">
        <span>Notebook</span>
        {#if reopenReadOnly}<button on:click={resumeReopen}>Resume (edit)</button>{/if}
        <button on:click={closeReopen}>✕ Close</button>
      </div>
      <div class="nb-overlay-body">
        {#key focusedNb.notebookId + reopenReadOnly}
          <NotebookPane notebookId={focusedNb.notebookId} readOnly={reopenReadOnly} onPush={() => {}} />
        {/key}
      </div>
    </div>
  {/if}
```

The `focusedNbFor === focusedId` check is what stops a slower `PeekNotebook` reply for a node you have already left from painting the wrong notebook — the exact defect this feature exists to remove.

- [ ] **Step 3: Verify no stale references remain**

Run: `grep -n 'reopenId' frontend/src/components/SourceTab.svelte`
Expected: no output. If any remains, the build will still succeed while the overlay silently never renders.

- [ ] **Step 4: Verify**

Run: `make test`, `make test-js`, and `cd frontend && npx vite build`
Expected: all pass; no unused-CSS warning for `.nb-overlay*` (their markup is retained, so a warning means the block was dropped).

- [ ] **Step 5: Report the behaviour**

For each of these, state in your report the concrete user-visible symptom if it were wrong: the `armed` gate; the `focusedNbFor === focusedId` guard; the read-only reset in `resolveCaret`; the sticky Close.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/SourceTab.svelte
git commit -m "feat(notebook): the read-only overlay follows the caret"
```

---

### Task 6: Manual test plan

**Files:**
- Create: `docs/specs/2026-08-04-note-editor-notebook-manual-test-plan.md`

Every prior slice shipped one, because this repo has no Svelte component harness and the behaviour is only observable running. Match the house format of `docs/specs/2026-08-03-process-focus-notebook-manual-test-plan.md`: a preamble stating what is automated and what is not, prereqs, then numbered cases with explicit setup, steps and observable expected results. Mark database-only checks `**[DB]**`.

- [ ] **Step 1: Write the document**

Cover at least:

1. Creating a notebook (process an extract) makes a dot appear on that node's line in the note editor.
2. **The dot survives editing the node's text** — the failure the old keying had. Edit the line, save, confirm the dot is still there.
3. A node with no notebook has no dot; a node with no `# @id` has no dot.
4. Clicking a dot opens the overlay showing that node's notebook.
5. With the overlay open, moving the caret to another node **that has a notebook** switches the overlay to it.
6. Moving the caret to a node **without** a notebook hides the overlay entirely.
7. Moving back to a node with a notebook shows it again.
8. **Close is sticky**: click Close, move the caret across several nodes that have notebooks, confirm the overlay stays shut until another dot is clicked.
9. **Resume (edit)** makes the shown notebook editable, and the edit persists.
10. Moving to a different node after Resume returns to read-only — you must not silently keep editing rights on a different notebook.
11. **[DB]** Browsing creates nothing: note `SELECT count(*) FROM notebooks;`, move the caret across many nodes including ones without notebooks, confirm the count is unchanged. This is invisible in the UI and is the most important check.
12. Dots and the overlay still behave after toggling Vim (the editor is rebuilt).
13. Process mode is unaffected — its own notebook pane and dots still work.

Also state explicitly, so a tester does not report it as a bug: **dots no longer mark text pushed from a notebook.** That provenance is still recorded in `notebook_links` but is no longer displayed anywhere.

- [ ] **Step 2: Commit**

```bash
git add docs/specs/2026-08-04-note-editor-notebook-manual-test-plan.md
git commit -m "docs(test-plan): note editor notebook"
```

---

## Self-Review

**Spec coverage.** Dot re-keyed to node id → Tasks 1-3. Overlay follows the caret → Tasks 4-5. Hides when the node has none → Task 5 Step 2. Sticky Close → Task 5 Step 1. No migration, no id injection, `notebook_links` untouched → Global Constraints, and no task modifies them. Read-only → `PeekNotebook` in Task 4, asserted by `TestNotebookMarksCreatesNothing` in Task 2.

**Placeholders.** None: every code step carries the literal code, and every test carries its assertions.

**Type consistency.** `NotebookMark{Line, NotebookID}` (Go) ↔ `{ line, notebookId }` (JS) — matching the `json:` tags, and used in that form in Tasks 3 and 5. `NotebooksForFile` returns `map[string]string` (node id → notebook id) and is consumed as such in Task 2. `PeekNotebook` returns a `NotebookView` whose field is `notebookId`, used as `focusedNb.notebookId` in Task 5.

**Two ordering hazards, called out where they bite.** Task 3 Step 3 leaves `openNotebookReadOnly` taking a line while it still accepts an id — closed in Task 5 Step 1, and flagged in the task itself so a reviewer does not read it as a defect. Task 4's `resolveCaret` assigns `reopenReadOnly`, which Task 5 declares; the tasks must land in order.
