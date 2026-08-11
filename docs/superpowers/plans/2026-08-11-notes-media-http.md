# Notes Media Over HTTP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve every asset referenced from a note as an ordinary `src` — a `http(s)://` URL directly, any other path over a new `/collection/` route — replacing the base64 binding, the blob cache and the object URLs.

**Architecture:** A new `CollectionServer` handler serves collection-relative paths with a containment check, composed with the existing `/media/` handler behind a `http.ServeMux`. One pure JS function decides a `src`. The media widget becomes synchronous.

**Tech Stack:** Go 1.25, Wails v2 (`assetserver.Options.Handler`), CodeMirror 6, Svelte 3.59, `node --test`.

## Global Constraints

- **Detection is untouched.** `mediaKind` and `findMedia` in `frontend/src/lib/n4l-media.js` keep working exactly as they do. This change replaces *loading*, not *detection*. Do not add or remove an extension, and do not change the arrow-agnostic rule.
- **`mediaMime` is deleted.** It existed only to type a `Blob`; the server sets `Content-Type` now. Delete its tests with it.
- **The new route enforces containment, and that tightens today's behaviour deliberately.** `GetSourceBytes` cleans `..` and reads outside the collection; `/collection/` must 404 that instead. This is an intended behaviour change, not a regression.
- **`/media/` must not be widened.** Its base-name collapse is its traversal defence and the notebook depends on it. Add a route; do not loosen that one.
- **`GetSourceBytes` itself does not change.** `PdfSourcePane` still uses it.
- **Any accessor reading `collectionDir` for the HTTP handler must take `a.dirMu`.** The handler runs on its own goroutine — that is why `mediaDir()` locks (`app.go:243`).
- **A 404 from the handler falls through to the embedded frontend assets.** `FileServer` already 404s everything outside `/media/` and the app still loads, which is what makes a `ServeMux` safe here.
- Go: `gofmt`, imports grouped stdlib / external / internal. JS/Svelte: 2-space indent, **no semicolons**, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.
- `TestWatchCollectionAdded` in `internal/content` is a **known pre-existing fsnotify flake**. Ignore it.

## MANDATORY: negative-control every test you write

Across the branches this follows, **eight** plausible-looking negative controls turned out to prove nothing — fixtures sampling only values on one side of a boundary, intervals that divided evenly and hid integer division, a single seeded row that made two queries indistinguishable, a regex sabotage that was not a sabotage. Every one was caught only because someone ran the control and read the output.

Before committing, for each behaviour asserted: break the production code in the specific way the test exists to catch, run the test, confirm it **fails with a message naming the real problem**, then restore and re-run green. Report the actual failure output.

- A sabotage that does not compile is **not** a control. Change it until it runs and the test genuinely fails.
- If a control does not make its test fail, **say so plainly** and name the assertion that was supposed to catch it. Do not weaken a test until something goes red.

## File Structure

- `internal/media/server.go` — add `CollectionServer`.
- `internal/media/media_test.go` — its tests, beside the existing `FileServer` ones.
- `app.go` — add `collectionRoot()`.
- `main.go` — compose both routes behind a `ServeMux`.
- `frontend/src/lib/n4l-media.js` — drop `mediaMime`, add `srcFor`.
- `frontend/src/lib/n4l-media.test.js` — drop the `mediaMime` tests, add `srcFor` tests.
- `frontend/src/lib/n4l-media-view.js` — synchronous `toDOM`; delete the cache, `bytesFrom`, the object URLs and the `GetSourceBytes` import.
- `docs/superpowers/specs/2026-08-11-notes-media-manual-test-plan.md` — update in place; this supersedes parts of it.

---

### Task 1: The `/collection/` route

**Files:**
- Modify: `internal/media/server.go`
- Modify: `internal/media/media_test.go`
- Modify: `app.go`, `main.go`

**Interfaces:**
- Consumes: the existing `audioMimeByExt` map in `internal/media`.
- Produces:
  - `func CollectionServer(dirFn func() string) http.Handler` — serves `/collection/<relative-path>` from `dirFn()`.
  - `func (a *App) collectionRoot() string` — the collection directory, or `""`, read under `a.dirMu`.

Model the handler and its test on the existing `FileServer` (`internal/media/server.go:13`) and `TestFileServerServesAndBlocksTraversal` (`internal/media/media_test.go:61`).

- [ ] **Step 1: Write the failing test**

Append to `internal/media/media_test.go`:

```go
func TestCollectionServerServesNestedPaths(t *testing.T) {
	dir := t.TempDir()
	// Both folders matter: conflating "media/" with some blessed assets folder
	// is the bug that prompted this route.
	for _, rel := range []string{"media/a.png", "assets/b.png", "deep/nest/c.png"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), pngBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := CollectionServer(func() string { return dir })

	for _, rel := range []string{"media/a.png", "assets/b.png", "deep/nest/c.png"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/collection/"+rel, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("%s: code=%d, want 200", rel, rr.Code)
		}
		if rr.Body.Len() != len(pngBytes) {
			t.Errorf("%s: body=%d bytes, want %d", rel, rr.Body.Len(), len(pngBytes))
		}
		if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("%s: Content-Type=%q, want image/png — a webview needs a usable type", rel, ct)
		}
	}
}

// This tightens behaviour deliberately: GetSourceBytes cleans ".." and reads
// outside the collection. This route must not.
func TestCollectionServerBlocksEscapes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media", "in.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	// A real file one level ABOVE the collection: the thing an escape would reach.
	outside := filepath.Join(filepath.Dir(dir), "escaped-"+filepath.Base(dir)+".png")
	if err := os.WriteFile(outside, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(outside) })

	h := CollectionServer(func() string { return dir })
	for _, p := range []string{
		"/collection/../" + filepath.Base(outside),
		"/collection/media/../../" + filepath.Base(outside),
		"/collection/..%2f" + filepath.Base(outside),
		"/other/media/in.png",
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code == http.StatusOK {
			t.Errorf("path %q was served (code %d) — it escapes the collection", p, rr.Code)
		}
	}
	// The control: the same handler DOES serve something legitimate, so a
	// blanket 404 cannot be mistaken for containment working.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/collection/media/in.png", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("in-collection file not served (code %d) — the fixture proves nothing", rr.Code)
	}
}

func TestCollectionServerWithNoCollectionOpen(t *testing.T) {
	h := CollectionServer(func() string { return "" })
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/collection/media/a.png", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("code=%d, want 404", rr.Code)
	}
}
```

`media_test.go` already imports `net/http`, `net/http/httptest`, `os`, `path/filepath` and `testing`, and already defines `pngBytes`. Add nothing.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/media/ -run CollectionServer -v`
Expected: FAIL — `undefined: CollectionServer`.

- [ ] **Step 3: Implement the handler**

Append to `internal/media/server.go`:

```go
// CollectionServer serves files anywhere under the collection directory at
// /collection/<relative-path>. dirFn returns the current collection directory
// (empty when none is open).
//
// Separate from FileServer rather than a widening of it: FileServer's collapse
// to a base name IS its traversal defence, and the notebook depends on that
// route. This one needs directories, so it defends differently — resolve, then
// require the result to stay under the root.
//
// That containment is stricter than GetSourceBytes, which cleans ".." and will
// happily read outside the collection. Tightening it here is deliberate.
func CollectionServer(dirFn func() string) http.Handler {
	const prefix = "/collection/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		dir := dirFn()
		if dir == "" {
			http.NotFound(w, r)
			return
		}
		root, err := filepath.Abs(dir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		// filepath.Join cleans "..", so the result may sit outside root; the
		// prefix check below is what actually enforces containment.
		full := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, prefix)))
		if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
			http.NotFound(w, r)
			return
		}
		if ct, ok := audioMimeByExt[strings.ToLower(filepath.Ext(full))]; ok {
			w.Header().Set("Content-Type", ct) // Go calls .webm video/webm; be explicit for <audio>
		}
		http.ServeFile(w, r, full)
	})
}
```

- [ ] **Step 4: Add the accessor and wire the routes**

In `app.go`, beside `mediaDir()` (`:243`):

```go
// collectionRoot is the collection directory, or "" when none is open. Read
// under dirMu because the HTTP asset handler calls it from its own goroutine,
// exactly as mediaDir does.
func (a *App) collectionRoot() string {
	a.dirMu.RLock()
	defer a.dirMu.RUnlock()
	return a.collectionDir
}
```

In `main.go`, replace `Handler: media.FileServer(app.mediaDir),` with a mux built just above the `options.App` literal:

```go
	assetHandler := http.NewServeMux()
	assetHandler.Handle("/media/", media.FileServer(app.mediaDir))
	assetHandler.Handle("/collection/", media.CollectionServer(app.collectionRoot))
```

and use `Handler: assetHandler,`. Add `"net/http"` to `main.go`'s imports.

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/media/ -run CollectionServer -v` then `make test`
Expected: PASS (3 tests), and the whole suite still green.

- [ ] **Step 6: Negative controls (report both)**

1. Delete the containment check — the `if full != root && !strings.HasPrefix(...)` block. `TestCollectionServerBlocksEscapes` must FAIL naming a served escape path. Restore.
2. Change `filepath.FromSlash(strings.TrimPrefix(r.URL.Path, prefix))` to `path.Base(r.URL.Path)`, i.e. copy `FileServer`'s flattening. `TestCollectionServerServesNestedPaths` must FAIL on `deep/nest/c.png`. Restore.

- [ ] **Step 7: Commit**

```bash
git add internal/media/server.go internal/media/media_test.go app.go main.go
git commit -m "feat(media): serve collection-relative paths over HTTP"
```

---

### Task 2: `srcFor`, replacing `mediaMime`

**Files:**
- Modify: `frontend/src/lib/n4l-media.js`
- Modify: `frontend/src/lib/n4l-media.test.js`

**Interfaces:**
- Consumes: nothing — the module stays dependency-free.
- Produces: `srcFor(path) -> string`.
- Removes: `mediaMime`, which existed only to type a `Blob`.

A remote path (`http://` or `https://`, case-insensitive) is returned unchanged.
Anything else becomes `/collection/` plus the path with **each segment**
`encodeURIComponent`-ed — per-segment so `/` separators survive while a `#`, `?`
or space inside a filename does not truncate the URL.

- [ ] **Step 1: Write the failing test**

In `frontend/src/lib/n4l-media.test.js`, change the import to drop `mediaMime` and add `srcFor`:

```js
import { mediaKind, srcFor, findMedia } from './n4l-media.js'
```

Delete the whole `test('mediaMime maps each kind to a playable type', …)` block, and append:

```js
test('srcFor passes a remote URL through untouched', () => {
  assert.equal(srcFor('https://example.com/a.png'), 'https://example.com/a.png')
  assert.equal(srcFor('http://example.com/a.png'), 'http://example.com/a.png')
})

test('srcFor recognises a remote URL whatever its case', () => {
  assert.equal(srcFor('HTTPS://Example.com/A.png'), 'HTTPS://Example.com/A.png')
})

test('srcFor routes a local path through the collection server', () => {
  assert.equal(srcFor('media/a.png'), '/collection/media/a.png')
  assert.equal(srcFor('deep/nest/c.png'), '/collection/deep/nest/c.png')
})

// Encoding is per SEGMENT: whole-string encoding would escape the slashes and
// the server would never find the file.
test('srcFor keeps path separators while encoding the segments', () => {
  assert.equal(srcFor('my folder/a b.png'), '/collection/my%20folder/a%20b.png')
})

// A '#' or '?' inside a filename would otherwise start a fragment or query and
// silently truncate the request.
test('srcFor encodes characters that would truncate the URL', () => {
  assert.equal(srcFor('notes/a#1.png'), '/collection/notes/a%231.png')
  assert.equal(srcFor('notes/a?b.png'), '/collection/notes/a%3Fb.png')
})

test('srcFor handles an empty path without throwing', () => {
  assert.equal(srcFor(''), '/collection/')
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/n4l-media.test.js`
Expected: FAIL — `srcFor` is not a function.

- [ ] **Step 3: Implement**

In `frontend/src/lib/n4l-media.js`, delete the `MIME` constant and the `mediaMime` export, then add:

```js
/**
 * The `src` for a referenced asset.
 *
 * A remote URL is used as-is. Everything else is served by the Go asset server
 * from the collection directory — which is why nothing here decodes bytes any
 * more: http.ServeFile gives native range requests, so a video seeks with no
 * work at all, and the browser does the caching.
 */
export function srcFor(path) {
  const s = String(path ?? '')
  if (/^https?:\/\//i.test(s)) return s
  // Per segment, not whole-string: encoding the slashes too would mean the
  // server never finds the file, while leaving a '#' or '?' raw would truncate
  // the request at a fragment or query that is really part of the filename.
  return '/collection/' + s.split('/').map(encodeURIComponent).join('/')
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `node --test frontend/src/lib/n4l-media.test.js` then `make test-js`
Expected: all PASS.

- [ ] **Step 5: Negative controls (report both)**

1. Change the encoding to whole-string: `'/collection/' + encodeURIComponent(s)`. The separators test must FAIL, showing `%2F` where `/` belongs. Restore.
2. Drop the `i` flag from the remote-URL test: `/^https?:\/\//`. The case test must FAIL, showing `HTTPS://…` rewritten as a collection path. Restore.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/n4l-media.js frontend/src/lib/n4l-media.test.js
git commit -m "feat(notes): resolve a media src locally or remotely"
```

---

### Task 3: A synchronous widget

**Files:**
- Modify: `frontend/src/lib/n4l-media-view.js`

**Interfaces:**
- Consumes: `findMedia` and `srcFor` from `n4l-media.js` (Task 2); the `/collection/` route (Task 1).
- Produces: no change to `n4lMedia()`'s signature.

Deleted in this task: the per-editor cache, the `load` function threaded through every widget, `bytesFrom`, `URL.createObjectURL`, and the `GetSourceBytes` import. `toDOM` becomes synchronous.

**Three things must survive.** Each was a real defect found by review on this feature:
1. The `StateField` — decorations must **not** come from a `ViewPlugin`. CodeMirror registers plugin decorations as a function and throws `RangeError: Block decorations may not be specified via plugins` for a block decoration from that path, out of the `EditorView` constructor.
2. `eq()` — without it CodeMirror rebuilds the widget on every update, restarting a playing video on each keystroke.
3. `view.requestMeasure()` when the media loads. Decoding grows the element and mutates no DOM, so nothing else re-measures and the height map under-reports every media block, drifting the line numbers beside it.

- [ ] **Step 1: Replace the loading machinery**

Change the imports to drop `GetSourceBytes` and take `srcFor`:

```js
import { Decoration, EditorView, WidgetType } from '@codemirror/view'
import { RangeSetBuilder, StateField } from '@codemirror/state'
import { findMedia, srcFor } from './n4l-media.js'
```

Delete `bytesFrom` entirely.

Replace the `MediaWidget` class body with:

```js
class MediaWidget extends WidgetType {
  constructor(path, kind) {
    super()
    this.path = path
    this.kind = kind
  }

  // Without this CodeMirror rebuilds the widget on every update, which would
  // restart a playing video on each keystroke.
  eq(other) { return other.path === this.path && other.kind === this.kind }

  // Reserves roughly the final height so the height map is not wildly wrong
  // before the media loads; requestMeasure below corrects it exactly.
  get estimatedHeight() { return this.kind === 'audio' ? 54 : 240 }

  toDOM(view) {
    const box = document.createElement('div')
    box.className = 'n4l-media'
    const el = document.createElement(this.kind === 'image' ? 'img' : this.kind)
    if (this.kind !== 'image') el.controls = true

    // Loading grows the element to its cap and mutates no DOM, so nothing else
    // would re-measure: the height map would under-report every media block and
    // drift the line numbers beside it.
    el.addEventListener(this.kind === 'image' ? 'load' : 'loadedmetadata',
      () => view.requestMeasure(), { once: true })

    // Without this a failed load shows the browser's broken-image icon. The
    // chip names the path instead, so a typo is diagnosable in place.
    el.addEventListener('error', () => {
      el.remove()
      const miss = document.createElement('span')
      miss.className = 'n4l-media-missing'
      miss.textContent = `media not found: ${this.path}`
      box.appendChild(miss)
      view.requestMeasure()
    }, { once: true })

    el.src = srcFor(this.path)
    box.appendChild(el)
    return box
  }

  // Governs DOM events, not diffing: without it the video scrubber's pointer
  // events would be treated as editor input.
  ignoreEvent() { return true }
}
```

- [ ] **Step 2: Simplify `n4lMedia`**

`mediaDecorations` no longer needs a `load` argument:

```js
function mediaDecorations(state) {
  const b = new RangeSetBuilder()
  for (let i = 1; i <= state.doc.lines; i++) {
    const line = state.doc.line(i)
    for (const m of findMedia(line.text)) {
      b.add(line.to, line.to, Decoration.widget({
        widget: new MediaWidget(m.path, m.kind), side: 1, block: true,
      }))
    }
  }
  return b.finish()
}
```

and the export loses its closure:

```js
export function n4lMedia() {
  // A StateField, NOT a ViewPlugin: CodeMirror registers plugin decorations as a
  // function, and a block decoration from that path throws
  // "Block decorations may not be specified via plugins" out of the EditorView
  // constructor. A StateField provides its value directly and is allowed block
  // widgets. It has no viewport, so this scans the whole document; reading notes
  // files are small.
  const field = StateField.define({
    create: (state) => mediaDecorations(state),
    update: (deco, tr) => (tr.docChanged ? mediaDecorations(tr.state) : deco),
    provide: (f) => EditorView.decorations.from(f),
  })
  return [field, theme]
}
```

Update the module header comment: the cache and object-URL paragraphs describe
code that no longer exists. Keep the `StateField` paragraph, which still
explains a live constraint.

- [ ] **Step 3: Verify**

Run: `make test`, `make test-js`, and `cd frontend && npx vite build`.

Then confirm the machinery is gone and the survivors are intact:

- `grep -c "createObjectURL\|revokeObjectURL\|GetSourceBytes\|bytesFrom\|atob" frontend/src/lib/n4l-media-view.js` — must be **0**.
- `grep -c "StateField.define" frontend/src/lib/n4l-media-view.js` — must be **1**.
- `grep -c "ViewPlugin" frontend/src/lib/n4l-media-view.js` — must be **0**. A `ViewPlugin` here throws at construction.
- `grep -c "requestMeasure" frontend/src/lib/n4l-media-view.js` — must be **2**, one per listener.
- `grep -n "eq(other)" frontend/src/lib/n4l-media-view.js` — must be present.
- `git diff --stat frontend/src/lib/n4l-editor.js` — must be empty.

- [ ] **Step 4: Report the symptoms**

State what a user would observe if each were wrong: decorations from a `ViewPlugin`, a missing `error` listener, and a missing `requestMeasure`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/n4l-media-view.js
git commit -m "feat(notes): set media src directly, dropping the blob machinery"
```

---

### Task 4: Update the manual test plan

**Files:**
- Modify: `docs/superpowers/specs/2026-08-11-notes-media-manual-test-plan.md`

This change supersedes several of that document's claims. Read it in full and the
shipped code, then bring it into line.

- [ ] **Step 1: Correct what is now wrong**

The following statements in it are no longer true and must be rewritten, not
merely supplemented:

- Anything implying assets live in a particular folder. They do not, and never
  did — **add a case that a file the notebook wrote under `media/` renders in a
  reading note**, since that is the case that prompted this change.
- The bullet saying a `http(s)://` URL shows a `media not found` chip. Remote
  URLs now **render**.
- The bullet saying a relative path escaping the collection loads. It is now
  **404ed and shows the chip** — call this out as an intended change.
- The bullet about a path being re-read each time the document is opened, and
  about a cached failure. There is no cache any more; the browser's HTTP cache
  applies instead, and a file created after you saw the chip appears on the next
  render.

- [ ] **Step 2: Add cases for the new behaviour**

1. An asset the notebook stored under `media/` renders in a reading note.
2. A remote `https://…/x.png` renders.
3. With the machine offline, that same remote image shows the `media not found`
   chip while a local image beside it still renders — the one case that
   distinguishes the two routes.
4. A local video **seeks** when the scrubber is dragged. This now comes from
   `http.ServeFile`'s range support rather than a blob, so it is a regression
   check on the whole mechanism.
5. `"../../outside.png"` shows the chip. It used to load.
6. A filename containing a space renders, which is what per-segment encoding is
   for.

State explicitly, so a tester does not file them: a remote asset makes a request
to that host when the note opens, by design; and remote media cannot work
offline while local media can.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-08-11-notes-media-manual-test-plan.md
git commit -m "docs(test-plan): notes media over HTTP"
```

---

## Self-Review

**Spec coverage.** `CollectionServer` with containment → Task 1. `collectionRoot()` under `dirMu` and the `ServeMux` → Task 1 Step 4. `srcFor` with per-segment encoding and remote pass-through → Task 2. `mediaMime` deleted → Task 2 Step 3. Synchronous `toDOM`, cache and blob machinery removed, `error` chip and `requestMeasure` kept → Task 3. Detection untouched → stated in Global Constraints and no task modifies `mediaKind`/`findMedia`. The traversal tightening → Task 1's second test plus Task 4's case 5. Manual coverage of the notebook-`media/` case → Task 4.

**Placeholders.** None: every code step carries literal code and every test its assertions. Task 4 is prose because it is editing prose, and it enumerates each false statement to fix and each case to add.

**Type consistency.** `CollectionServer(dirFn func() string) http.Handler` is defined in Task 1 and called in Task 1 Step 4 with `app.collectionRoot`, defined in the same step. `srcFor(path) -> string` is defined in Task 2 and called in Task 3 as `srcFor(this.path)`. `findMedia`'s `{path, kind}` shape is unchanged and consumed identically. `n4lMedia()` keeps returning `[field, theme]`.

**One thing worth flagging.** Task 3 deletes code that a previous review specifically vetted — the per-editor cache was itself a fix for a collection-switch bug, and the object-URL-per-path was a fix for a leak. Both fixes become moot rather than wrong: with no bytes in JS there is nothing to cache and no URL to leak. The grep in Step 3 exists so a reviewer can confirm the machinery is gone rather than half-removed.
