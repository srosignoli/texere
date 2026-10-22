# Processing Notebook — Slice 5: Image cell + on-disk assets — Implementation Plan

**Goal:** Add an `image` notebook cell — import via the OS file dialog into an on-disk `media/` directory, serve it through a Wails asset handler, and edit it non-destructively (90° rotation + crop stored as metadata).

**Architecture:** New pure-Go `internal/media` package (content-hashed import + a path-traversal-safe `/media/` file server). `app.go` gains an `ImportImage` binding and a mutex-guarded `mediaDir()`; `main.go` wires the handler as the AssetServer fallback. Frontend: a pure `image-view.js` (rotation+crop → CSS, 3 nested layers, all percentages) and an `ImageCell.svelte` that displays `<img src="/media/…">` with a rotate/crop/reset toolbar.

**Tech Stack:** Go stdlib only (`crypto/sha256`, `net/http`, `os`, `path/filepath`); Wails v2 `AssetServer.Handler` + `runtime.OpenFileDialog`; Svelte 3.49; no new npm or Go dependencies.

## Global Constraints

- No new npm or Go dependencies; no CGo. `internal/media` uses only the standard library.
- Frontend has no JS test runner; frontend tasks verify with `cd frontend && npm run build` plus a manual test plan. Go code uses `go test ./...`.
- On-disk assets live in `<collectionDir>/media/`, named `<sha256(bytes)>.<ext>` (dedup). `cells_json` stores only a reference + edit metadata, never image bytes.
- Supported types: jpeg, png, gif, webp (via `http.DetectContentType`). Reject others. No HEIC.
- Edits are non-destructive: `rotation` ∈ {0,90,180,270} and `crop` (normalized `{x,y,w,h}` of the **rotated** frame, or `null`) are metadata; the file is never modified.
- The `/media/` handler is path-traversal safe (only a sanitized base name under the media dir is served) and returns 404 when no collection is open.
- `collectionDir` is guarded by a `sync.RWMutex` for the handler's cross-goroutine read; `mediaDir()` is **unexported** (used from `main.go`, same package — not a Wails binding). `ImportImage` is exported (frontend binding).
- Persist on completed action only (import, rotate, crop-apply, reset, one-time natural-size fill) — never during a crop drag.

---

## File Structure

- `internal/media/import.go` — CREATE: `Import(collectionDir, srcPath) (Asset, error)`.
- `internal/media/server.go` — CREATE: `FileServer(dirFn func() string) http.Handler`.
- `internal/media/media_test.go` — CREATE: import + server tests.
- `app.go` — MODIFY: `dirMu sync.RWMutex` field, guarded writes, `mediaDir()`, `ImportImage()`.
- `main.go` — MODIFY: wire `Handler: media.FileServer(app.mediaDir)`.
- `frontend/src/lib/image-view.js` — CREATE: pure `imageStyles(cell) → {viewport, pan, img}`.
- `frontend/src/components/notebook/ImageCell.svelte` — CREATE.
- `frontend/src/lib/notebook.js` — MODIFY: import + `newImageCell()` + registry.
- `frontend/src/components/notebook/NotebookPane.svelte` — MODIFY: `addImage()` + button.
- `docs/specs/2026-07-24-processing-notebook-slice5-manual-test-plan.md` — CREATE (Task 6).

---

## Task 1: `internal/media` — import + file server (TDD)

**Files:**
- Create: `internal/media/import.go`, `internal/media/server.go`, `internal/media/media_test.go`

**Interfaces:**
- Produces: `media.Import(collectionDir, srcPath string) (media.Asset, error)` with `type Asset struct { Name, Mime string }`; `media.FileServer(dirFn func() string) http.Handler`. Consumed by `app.go`/`main.go` in Task 2.

- [ ] **Step 1: Write the failing tests**

Create `internal/media/media_test.go`:

```go
package media

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 1x1 PNG.
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestImportHashesAndDedups(t *testing.T) {
	col := t.TempDir()
	src := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(src, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Import(col, src)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if a.Mime != "image/png" {
		t.Errorf("mime = %q, want image/png", a.Mime)
	}
	if !strings.HasSuffix(a.Name, ".png") || len(a.Name) != 64+4 {
		t.Errorf("name = %q, want <64 hex>.png", a.Name)
	}
	if _, err := os.Stat(filepath.Join(col, "media", a.Name)); err != nil {
		t.Errorf("file not written: %v", err)
	}
	// Re-import identical bytes → same name, still one file.
	b, err := Import(col, src)
	if err != nil || b.Name != a.Name {
		t.Fatalf("dedup failed: %v name=%q vs %q", err, b.Name, a.Name)
	}
	entries, _ := os.ReadDir(filepath.Join(col, "media"))
	if len(entries) != 1 {
		t.Errorf("media dir has %d files, want 1", len(entries))
	}
}

func TestImportRejectsNonImage(t *testing.T) {
	col := t.TempDir()
	src := filepath.Join(t.TempDir(), "note.txt")
	_ = os.WriteFile(src, []byte("this is plain text, not an image"), 0o644)
	if _, err := Import(col, src); err == nil {
		t.Fatal("expected error for non-image, got nil")
	}
}

func TestFileServerServesAndBlocksTraversal(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "ok.png"), pngBytes, 0o644)
	h := FileServer(func() string { return dir })

	// Serves a real file.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/media/ok.png", nil))
	if rr.Code != http.StatusOK || rr.Body.Len() != len(pngBytes) {
		t.Errorf("serve: code=%d len=%d", rr.Code, rr.Body.Len())
	}
	// Traversal collapses to a base name and does not escape.
	for _, p := range []string{"/media/../server.go", "/media/..%2f..%2fx", "/other/x"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code == http.StatusOK {
			t.Errorf("path %q unexpectedly served (code %d)", p, rr.Code)
		}
	}
	// No collection open → 404.
	h2 := FileServer(func() string { return "" })
	rr2 := httptest.NewRecorder()
	h2.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/media/ok.png", nil))
	if rr2.Code != http.StatusNotFound {
		t.Errorf("empty dir: code=%d, want 404", rr2.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/media/`
Expected: FAIL (package/functions do not exist yet).

- [ ] **Step 3: Implement `import.go`**

Create `internal/media/import.go`:

```go
// Package media stores notebook image assets on disk under <collectionDir>/media/
// and serves them to the webview. Assets are content-addressed (sha256), so the
// same image imported twice is stored once. Pure standard library, no CGo.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// Asset is a stored image: its filename under media/ and detected MIME type.
type Asset struct {
	Name string
	Mime string
}

var extByMime = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// Import copies the image at srcPath into <collectionDir>/media/ under a
// content-hashed name (<sha256>.<ext>) and returns its reference. Identical bytes
// yield the same name and are written once. Non-image files are rejected.
func Import(collectionDir, srcPath string) (Asset, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return Asset{}, fmt.Errorf("media: read source: %w", err)
	}
	mime := http.DetectContentType(data)
	ext, ok := extByMime[mime]
	if !ok {
		return Asset{}, fmt.Errorf("media: unsupported type %q", mime)
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + ext
	dir := filepath.Join(collectionDir, "media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Asset{}, fmt.Errorf("media: mkdir: %w", err)
	}
	dst := filepath.Join(dir, name)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return Asset{}, fmt.Errorf("media: write: %w", err)
		}
	}
	return Asset{Name: name, Mime: mime}, nil
}
```

- [ ] **Step 4: Implement `server.go`**

Create `internal/media/server.go`:

```go
package media

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// FileServer serves files under the media directory at /media/<name>. dirFn
// returns the current media directory (empty when no collection is open). It is
// path-traversal safe: only a sanitized base name under dirFn() is ever served.
func FileServer(dirFn func() string) http.Handler {
	const prefix = "/media/"
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
		name := path.Base(r.URL.Path) // collapses any ".." / directory components
		if name == "." || name == ".." || name == "/" || strings.ContainsRune(name, filepath.Separator) {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, name))
	})
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/media/`
Expected: PASS (all three tests).

- [ ] **Step 6: Commit**

```bash
git add internal/media/
git commit -m "feat(media): content-hashed image import + path-safe /media file server"
```

---

## Task 2: Wire `app.go` binding + `main.go` asset handler

**Files:**
- Modify: `app.go`, `main.go`

**Interfaces:**
- Consumes: `media.Import`, `media.FileServer` (Task 1).
- Produces: `ImportImage()` Wails binding returning `{asset, mime}` (or `null` when cancelled), consumed by `NotebookPane` in Task 5. The `wails build` in this task regenerates the JS bindings so `ImportImage` exists for the frontend.

- [ ] **Step 1: Add the mutex field in `app.go`**

In the `App` struct, add after the `collectionDir` field:

```go
	collectionDir string                   // abs path of the open collection; "" when none
	dirMu         sync.RWMutex             // guards collectionDir for the /media asset handler (HTTP goroutine)
```

- [ ] **Step 2: Guard the two `collectionDir` writes**

In `startup`, change:

```go
	a.collectionDir = cfg.CollectionDir
```

to:

```go
	a.dirMu.Lock()
	a.collectionDir = cfg.CollectionDir
	a.dirMu.Unlock()
```

In `OpenCollection`, change:

```go
	a.openFilePath = ""
	a.collectionDir = dir
```

to:

```go
	a.openFilePath = ""
	a.dirMu.Lock()
	a.collectionDir = dir
	a.dirMu.Unlock()
```

- [ ] **Step 3: Add `mediaDir()` and `ImportImage()`**

Add these methods to `app.go` (near `OpenCollection`). Ensure the import block includes `"texere/internal/media"` and `"fmt"` and `"path/filepath"` (add any missing):

```go
// mediaDir returns <collectionDir>/media, or "" when no collection is open.
// Unexported so it is not surfaced as a Wails binding; read by the asset handler
// (main.go) on the HTTP goroutine, hence the mutex.
func (a *App) mediaDir() string {
	a.dirMu.RLock()
	defer a.dirMu.RUnlock()
	if a.collectionDir == "" {
		return ""
	}
	return filepath.Join(a.collectionDir, "media")
}

// ImportImage opens the OS file dialog, copies the chosen image into the
// collection's media/ directory, and returns {asset, mime}. Returns (nil, nil)
// when the user cancels, and an error when no collection is open or the file is
// not a supported image.
func (a *App) ImportImage() (map[string]any, error) {
	a.dirMu.RLock()
	dir := a.collectionDir
	a.dirMu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("no collection open")
	}
	sel, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Add image",
		Filters: []runtime.FileFilter{
			{DisplayName: "Images (*.jpg, *.png, *.gif, *.webp)", Pattern: "*.jpg;*.jpeg;*.png;*.gif;*.webp"},
		},
	})
	if err != nil {
		return nil, err
	}
	if sel == "" {
		return nil, nil // cancelled
	}
	asset, err := media.Import(dir, sel)
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": asset.Name, "mime": asset.Mime}, nil
}
```

- [ ] **Step 4: Wire the handler in `main.go`**

Add `"texere/internal/media"` to the imports, and change the `AssetServer` block:

```go
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
```

to:

```go
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: media.FileServer(app.mediaDir),
		},
```

- [ ] **Step 5: Build (compiles Go + regenerates JS bindings)**

Run: `go build ./...`
Expected: compiles with no errors.

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`; regenerates `frontend/wailsjs/go/main/App.{js,d.ts}` to include `ImportImage`.

- [ ] **Step 6: Confirm the binding was generated**

Run: `grep -n "ImportImage" frontend/wailsjs/go/main/App.d.ts`
Expected: a line declaring `export function ImportImage():Promise<...>`.

- [ ] **Step 7: Commit**

```bash
git add app.go main.go frontend/wailsjs/
git commit -m "feat(notebook): ImportImage binding + /media asset handler"
```

---

## Task 3: `image-view.js` — rotation + crop → CSS (pure)

**Files:**
- Create: `frontend/src/lib/image-view.js`

**Interfaces:**
- Produces: `imageStyles({ naturalW, naturalH, rotation, crop }) → { viewport, pan, img }` (three CSS strings). Consumed by `ImageCell.svelte` (Task 4).

- [ ] **Step 1: Create the module**

Create `frontend/src/lib/image-view.js`:

```js
// imageStyles maps an image cell's natural size + non-destructive edit metadata
// (rotation in 90° steps, normalized crop of the ROTATED frame) to CSS for three
// nested layers:
//   <div style={viewport}>        // clips to the crop; aspect-ratio = crop box
//     <div style={pan}>           // the full rotated image, panned so the crop fills the viewport
//       <img style={img}>         // the natural image, rotated to fill the rotated frame
//
// All sizes are percentages, so it is resolution-independent, and it never
// distorts (each layer's aspect matches its content). crop is {x,y,w,h} fractions
// 0..1 of the rotated frame, or null for the whole image.
export function imageStyles({ naturalW, naturalH, rotation = 0, crop = null }) {
  const W = naturalW || 1
  const H = naturalH || 1
  const r = ((Math.round(rotation / 90) * 90) % 360 + 360) % 360
  const swap = r === 90 || r === 270
  const RW = swap ? H : W // rotated-frame dimensions
  const RH = swap ? W : H
  const c = crop && crop.w > 0 && crop.h > 0 ? crop : { x: 0, y: 0, w: 1, h: 1 }

  const viewport = `position:relative;overflow:hidden;width:100%;` +
    `aspect-ratio:${c.w * RW} / ${c.h * RH};`
  const pan = `position:absolute;` +
    `left:${(-c.x / c.w) * 100}%;top:${(-c.y / c.h) * 100}%;` +
    `width:${100 / c.w}%;height:${100 / c.h}%;`
  const img = swap
    ? `position:absolute;top:50%;left:50%;` +
      `width:${(W / H) * 100}%;height:${(H / W) * 100}%;` +
      `transform:translate(-50%,-50%) rotate(${r}deg);display:block;`
    : `display:block;width:100%;height:100%;transform:rotate(${r}deg);`

  return { viewport, pan, img }
}
```

- [ ] **Step 2: Reason through the four rotations (no JS test runner — verify by inspection)**

Confirm by hand (documented here so the reviewer can check):
- `r=0`, no crop: viewport aspect `W/H`; pan `0,0,100%,100%`; img fills, `rotate(0)`. ✔ shows the whole image.
- `r=180`: same boxes, `rotate(180)`. ✔
- `r=90`, `W=200,H=100`: `swap` → `RW=100,RH=200`; viewport aspect `100/200` (portrait ✔); img `width=200%,height=50%` of pan, centered, `rotate(90)` → a 200×100 (natural aspect) image whose rotated footprint fills the portrait pan. ✔ no distortion.
- crop `{0.25,0,0.5,1}` at `r=0`, `W=200,H=100`: viewport aspect `100/100`; pan `left=-50%,width=200%` → the middle half fills the viewport. ✔

- [ ] **Step 3: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds (module is standalone; it is wired into a component in Task 4).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/image-view.js
git commit -m "feat(image): pure rotation+crop CSS helper (image-view)"
```

---

## Task 4: `ImageCell.svelte`

**Files:**
- Create: `frontend/src/components/notebook/ImageCell.svelte`

**Interfaces:**
- Consumes: `imageStyles` (Task 3); the shared cell props; `cell` shape `{id, type:'image', asset, mime, naturalW, naturalH, rotation, crop}`.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/notebook/ImageCell.svelte`:

```svelte
<script>
  import { imageStyles } from '../../lib/image-view.js'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused

  let missing = false
  let cropping = false
  let sel = null       // { x, y, w, h } in fractions of the crop-mode image, during a drag
  let imgEl           // the crop-mode <img>, for rect math
  let dragging = false
  let start = null

  $: styles = imageStyles(cell)
  $: src = `/media/${cell.asset}`
  // In crop mode we show the FULL rotated image (ignoring the current crop) so a
  // new crop is always selected over the whole rotated frame.
  $: cropStyles = imageStyles({ ...cell, crop: null })

  function persist() { onChange(cell) }

  function onLoad(e) {
    missing = false
    if (!cell.naturalW || !cell.naturalH) {
      cell.naturalW = e.target.naturalWidth
      cell.naturalH = e.target.naturalHeight
      persist()
    }
  }
  function rotate(delta) {
    cell.rotation = (((cell.rotation || 0) + delta) % 360 + 360) % 360
    cell.crop = null // rotating invalidates a crop taken in the old frame
    persist()
  }
  function reset() {
    cell.rotation = 0
    cell.crop = null
    persist()
  }
  function enterCrop() { cropping = true; sel = null }
  function cancelCrop() { cropping = false; sel = null; dragging = false }
  function applyCrop() {
    if (sel && sel.w > 0.02 && sel.h > 0.02) { cell.crop = { ...sel }; persist() }
    cancelCrop()
  }

  function frac(e) {
    const r = imgEl.getBoundingClientRect()
    return {
      x: Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)),
      y: Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)),
    }
  }
  function down(e) { if (readOnly) return; dragging = true; start = frac(e); sel = { x: start.x, y: start.y, w: 0, h: 0 } }
  function move(e) {
    if (!dragging) return
    const p = frac(e)
    sel = {
      x: Math.min(start.x, p.x), y: Math.min(start.y, p.y),
      w: Math.abs(p.x - start.x), h: Math.abs(p.y - start.y),
    }
  }
  function up() { dragging = false }
</script>

<div class="im-cell">
  {#if !readOnly}
    <div class="im-bar">
      <button on:click={() => rotate(-90)} title="Rotate left">↶</button>
      <button on:click={() => rotate(90)} title="Rotate right">↷</button>
      {#if cropping}
        <button on:click={applyCrop} title="Apply crop">✓ Apply</button>
        <button on:click={cancelCrop} title="Cancel crop">✕ Cancel</button>
      {:else}
        <button on:click={enterCrop} title="Crop">⛶ Crop</button>
      {/if}
      <button on:click={reset} title="Reset rotation and crop">Reset</button>
      <span class="im-spacer"></span>
      <button class="im-del" on:click={onDelete} title="Delete cell">✕</button>
    </div>
  {/if}

  {#if missing}
    <div class="im-missing">image not found: {cell.asset}</div>
  {:else if cropping}
    <div class="im-cropwrap">
      <div style={cropStyles.viewport}>
        <div style={cropStyles.pan}>
          <img bind:this={imgEl} {src} style={cropStyles.img} alt=""
            on:pointerdown={down} on:pointermove={move} on:pointerup={up} on:pointercancel={up} draggable="false" />
        </div>
      </div>
      {#if sel}
        <div class="im-sel" style="left:{sel.x * 100}%;top:{sel.y * 100}%;width:{sel.w * 100}%;height:{sel.h * 100}%;"></div>
      {/if}
    </div>
  {:else}
    <div style={styles.viewport}>
      <div style={styles.pan}>
        <img {src} style={styles.img} alt="" on:load={onLoad} on:error={() => (missing = true)} draggable="false" />
      </div>
    </div>
  {/if}
</div>

<style>
  .im-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .im-bar { display: flex; align-items: center; gap: 6px; padding: 5px 8px; border-bottom: 1px solid #1c2a3a; flex-wrap: wrap; }
  .im-bar button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
  .im-spacer { flex: 1; }
  .im-del { background: none !important; border: none !important; color: #6b7a8a !important; }
  .im-del:hover { color: #ff8080 !important; }
  .im-cell > div:not(.im-bar):not(.im-cropwrap) { max-height: 70vh; }
  .im-cell img { max-width: 100%; }
  .im-cropwrap { position: relative; max-height: 70vh; touch-action: none; cursor: crosshair; }
  .im-sel { position: absolute; border: 1.5px solid #7dd3fc; background: rgba(125, 211, 252, 0.15); pointer-events: none; }
  .im-missing { padding: 20px; color: #ff8080; font-size: 13px; }
</style>
```

- [ ] **Step 2: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds. (The component is not yet reachable from `cellRegistry`, so it is still tree-shaken; Task 5 wires it in — that build is the real compile gate.)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/notebook/ImageCell.svelte
git commit -m "feat(notebook): image cell component (display + rotate/crop/reset)"
```

---

## Task 5: Register `image` cell + NotebookPane button (compile gate)

**Files:**
- Modify: `frontend/src/lib/notebook.js`, `frontend/src/components/notebook/NotebookPane.svelte`

**Interfaces:**
- Consumes: `ImageCell` (Task 4), `newImageCell` (this task), `ImportImage` binding (Task 2).

- [ ] **Step 1: Register in `notebook.js`**

Add the import alongside the others:

```js
import ImageCell from '../components/notebook/ImageCell.svelte'
```

Add the factory (after `newSketchCell`):

```js
export function newImageCell(asset, mime) {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'image', asset, mime, naturalW: 0, naturalH: 0, rotation: 0, crop: null }
}
```

Add to `cellRegistry`:

```js
  sketch: SketchCell,
  image: ImageCell,
```

- [ ] **Step 2: Wire `NotebookPane.svelte`**

Extend the bindings import to include `ImportImage`:

```js
  import { OpenNotebook, GetNotebook, SaveNotebook, ImportImage } from '../../../wailsjs/go/main/App'
```

Extend the notebook.js import to include `newImageCell`:

```js
  import { newDraftCell, newLLMCell, newGraphCell, newComputeCell, newSketchCell, newImageCell, cellRegistry } from '../../lib/notebook.js'
```

Add the handler (after `addSketch`):

```js
  async function addImage() {
    try {
      const ref = await ImportImage()
      if (!ref) return // cancelled
      cells = [...cells, newImageCell(ref.asset, ref.mime)]
      scheduleSave()
    } catch (e) { error = String(e?.message ?? e) }
  }
```

Add the button after the "+ Sketch cell" button:

```svelte
      <button class="nb-add" on:click={addSketch}>+ Sketch cell</button>
      <button class="nb-add" on:click={addImage}>+ Image cell</button>
```

- [ ] **Step 3: Build (real compile gate — pulls ImageCell + image-view into the bundle)**

Run: `cd frontend && npm run build`
Expected: build succeeds; `ImageCell.svelte` and `image-view.js` now compile into the bundle.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/notebook.js frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): register image cell + '+ Image cell' button"
```

---

## Task 6: Manual test plan + full verification

**Files:**
- Create: `docs/specs/2026-07-24-processing-notebook-slice5-manual-test-plan.md`

- [ ] **Step 1: Write the manual test plan**

Create the file:

```markdown
# Processing Notebook — Slice 5 (Image cell) Manual Test Plan

Prereqs: open a collection and an extract for processing.

## TC-1 Import + display
1. Click "+ Image cell"; pick a jpg or png.
   - EXPECT: the OS file dialog appears (filtered to images); after choosing, the image displays in the cell.

## TC-2 Rotate
1. Click ↷ (rotate right) four times.
   - EXPECT: the image turns 90° each click and returns to the original after four; no distortion; the cell reshapes to the rotated aspect.

## TC-3 Crop
1. Click ⛶ Crop; drag a rectangle over the image; click ✓ Apply.
   - EXPECT: the cell now shows only the cropped region (correct area, no distortion).
2. Click ⛶ Crop again; ✕ Cancel.
   - EXPECT: no change to the current crop.

## TC-4 Reset
1. After rotating and cropping, click Reset.
   - EXPECT: the image returns to its original orientation and full frame.

## TC-5 Cancel / no collection
1. Click "+ Image cell" and cancel the dialog.
   - EXPECT: no cell is added, no error.

## TC-6 Persistence across restart
1. Import an image, rotate + crop it, then restart the app and reopen the extract.
   - EXPECT: the image re-renders with the same rotation and crop and is still editable.

## TC-7 Dedup + on-disk assets
1. Import the same file twice.
   - EXPECT: two cells, but `<collectionDir>/media/` contains a single file for that image (same sha256 name).

## TC-8 Missing asset
1. Delete the file from `<collectionDir>/media/` (or rename it) while a cell references it; reopen the notebook.
   - EXPECT: the cell shows an "image not found" placeholder, no crash.
```

- [ ] **Step 2: Backend suite + full build**

Run: `go test ./...`
Expected: all PASS (includes `internal/media`).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`.

- [ ] **Step 3: Execute the manual test plan**

Walk TC-1 … TC-8 in `./build/bin/texere`.

- [ ] **Step 4: Commit**

```bash
git add docs/specs/2026-07-24-processing-notebook-slice5-manual-test-plan.md
git commit -m "docs(test-plan): Processing Notebook Slice 5 manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** on-disk content-hashed import + path-safe server (T1), `ImportImage` binding + AssetServer handler + mutex-guarded `collectionDir` (T2), pure rotation+crop CSS helper (T3), image cell with import/display/rotate/crop/reset + missing-asset handling (T4), register + button compile gate (T5), manual plan + full verification (T6). Watched inbox, HEIC, destructive edits, media GC are out of scope by design.
- **Placeholder scan:** no TBD/TODO; every code step contains complete code; Go steps follow TDD (tests first, run-fail, implement, run-pass).
- **Type consistency:** `media.Asset{Name,Mime}` → `ImportImage` returns `{asset,mime}` → `newImageCell(asset,mime)` → cell `{asset,mime,naturalW,naturalH,rotation,crop}` read by `imageStyles({naturalW,naturalH,rotation,crop})` and `ImageCell`. `mediaDir()` (unexported) is passed as `app.mediaDir` to `media.FileServer(dirFn func() string)`; `FileServer` joins `dirFn()` with the sanitized base name. `crop` is `{x,y,w,h}` fractions of the rotated frame everywhere it appears.
- **Compile-sequencing note:** `image-view.js` (T3) and `ImageCell.svelte` (T4) are tree-shaken until T5 registers the cell; T5's `npm run build` is the real frontend compile gate, and T6's `make build` embeds it. The `ImportImage` JS binding is generated by T2's `make build` (verified in T2 Step 6) before T5 imports it.
```
