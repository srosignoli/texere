# Processing Notebook — Slice 5: Image cell (manual insert) + on-disk assets

**Date:** 2026-07-24
**Status:** Approved design
**Feature area:** Processing Notebook (incremental reading)
**Slice:** 5 of ~6 (see the Slice 1 design for the full decomposition)

## Summary

Add an **image cell**: pick an image with the OS file dialog, copy it into an
on-disk `media/` directory under the collection, and display it in the notebook
via a Wails asset handler (`<img src="/media/…">`). The cell supports
**non-destructive** editing — 90° rotation and a crop rectangle stored as
metadata in `cells_json`; the original file on disk is never modified. This is
the first notebook cell that stores binary **on-disk assets** (previous cells,
including the sketch, persist entirely inside `cells_json`).

Scope trimmed during brainstorming: the **watched inbox** (fsnotify + debounce)
originally bundled into this slice is **dropped** — images enter only via a
manual "+ Image cell" file picker. HEIC and destructive edits are out of scope.

## Decisions made during brainstorming

- **Manual insert only.** No fsnotify inbox this slice. A "+ Image cell" button
  opens the OS file picker; the chosen file is copied into `media/`.
- **Asset handler serving.** A Wails `AssetServer` `Handler` streams
  `/media/<file>` from `<collectionDir>/media/`, falling through to the embedded
  frontend for everything else. The `<img src>` is a URL, so the browser caches
  and lazy-decodes (lighter than base64 for large phone photos).
- **Non-destructive rotate + crop.** `rotation` (90° steps) and a normalized
  `crop` rect are stored on the cell; the file is untouched, so edits are fully
  reversible. Slice 6 will bake rotation+crop to a bitmap when pushing to the LLM.
- **Content-hashed filenames** (`<sha256>.<ext>`) — importing the same image
  twice reuses one file (automatic dedup).
- **Formats:** jpg/png/gif/webp (browser-native). **HEIC out of scope** (not
  webview-decodable; a pure-Go HEIC decoder is heavy and the native one needs
  CGo, which the project forbids).
- **No manual resize handle** — an image fits the notebook width, height by the
  rotated/cropped aspect, capped at a max height.
- **Deleting a cell** removes only the reference; the file stays in `media/`
  (orphan). Media garbage-collection is deferred.

## Scope

**In scope**

- An `image` cell type in the notebook `cellRegistry`, with a "+ Image cell"
  button in `NotebookPane`.
- `ImportImage()` Wails binding: OS file dialog (filtered to supported image
  types) → copy into `media/` under a content-hashed name → return the reference.
- A Wails asset handler serving `/media/<file>` from the collection's `media/`.
- Non-destructive **rotate** (↶/↷, 90° steps) and **crop** (drag a rectangle),
  plus **Reset**, stored as metadata; re-rendered on reopen.
- Graceful handling of a missing asset file, no open collection, and a cancelled
  dialog.

**Out of scope (later slices)**

- Watched inbox / fsnotify / debounce (dropped from this slice).
- Attaching an image to the LLM cell / baking rotation+crop to a bitmap for a
  multimodal request (Slice 6).
- HEIC, arbitrary-angle rotation, brightness/contrast/filters, multi-image
  galleries, media garbage collection, drag-and-drop import.

## Data model (`cells_json`)

```json
{
  "id": "…",
  "type": "image",
  "asset": "ab12cd….jpg",
  "mime": "image/jpeg",
  "naturalW": 4032,
  "naturalH": 3024,
  "rotation": 0,
  "crop": null
}
```

- `asset` — filename under `<collectionDir>/media/`. The only pointer to the
  bytes; `cells_json` never contains image data.
- `mime` — detected content type (used for the `<img>`/handler; also for Slice 6).
- `naturalW` / `naturalH` — the source pixel dimensions, captured on the frontend
  from `img.naturalWidth/Height` on first load and persisted (so the server never
  has to decode the image; no image-decode Go dependency).
- `rotation` — `0 | 90 | 180 | 270`, clockwise degrees applied for display.
- `crop` — `null`, or `{ x, y, w, h }` as fractions `0..1` of the **rotated**
  frame (rotation is conceptually applied first, then the crop selects a sub-rect
  of what the user sees). `null` means the whole (rotated) image.

## Architecture & components

```
main.go                                   # wire the AssetServer Handler (media.FileServer)
app.go                                    # ImportImage() binding; MediaDir() (mutex-guarded)
internal/media/                           # NEW pure-Go package
  ├── import.go                           #   Import(collectionDir, srcPath) → (Asset, error)
  ├── server.go                           #   FileServer(dirFn func() string) http.Handler
  └── media_test.go                       #   hashing, ext, path-traversal, mime-reject tests
frontend/src/
  ├── lib/image-view.js                   # NEW pure: {naturalW,naturalH,rotation,crop} → { viewport, img } CSS
  ├── components/notebook/ImageCell.svelte# NEW: <img src="/media/…"> + rotate/crop/reset toolbar + crop overlay
  ├── lib/notebook.js                     # register 'image'; newImageCell()
  └── components/notebook/NotebookPane.svelte  # "+ Image cell" button
```

### `internal/media` (pure Go, no new deps)

- `type Asset struct { Name, Mime string }`.
- `Import(collectionDir, srcPath string) (Asset, error)`:
  - Read `srcPath`; sniff MIME with `http.DetectContentType` (stdlib). Reject if
    not `image/{jpeg,png,gif,webp}` → error (defense even though the dialog filters).
  - `sum := sha256.Sum256(data)`; `name := hex(sum) + extForMime(mime)`
    (`.jpg/.png/.gif/.webp`).
  - `os.MkdirAll(filepath.Join(collectionDir, "media"), 0o755)`; write the file if
    absent (dedup — identical bytes reuse the same name). Return `Asset{name, mime}`.
- `FileServer(dirFn func() string) http.Handler`:
  - On `GET /media/<file>`: `base := path.Base(cleanReqPath)`; reject if `base`
    contains a separator, is `.`/`..`, or `dirFn()` is empty → 404. Serve
    `filepath.Join(dirFn(), base)` with `http.ServeFile` (sets content-type,
    caching, range). Non-`/media/` paths → 404 (falls back to the embedded assets
    via the Wails asset server, which invokes this handler only on a miss).
  - Path-traversal safe: only a sanitized base name under the media dir is ever
    served.

### `app.go`

- Add `dirMu sync.RWMutex` guarding `collectionDir` (currently read/written without
  a lock; the handler runs on the HTTP goroutine). Lock the writes at `startup`
  and `OpenCollection`; add `MediaDir() string` (RLock → `filepath.Join(
  collectionDir, "media")`, or `""` when no collection).
- `ImportImage() (map[string]any, error)` binding:
  - If `MediaDir()` is `""` → error "no collection open".
  - `path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
    Title:"Add image", Filters:[]runtime.FileFilter{{DisplayName:"Images",
    Pattern:"*.jpg;*.jpeg;*.png;*.gif;*.webp"}}})`. Empty `path` (cancelled) →
    return `(nil, nil)`.
  - `asset, err := media.Import(collectionDir, path)`; on success return
    `{ "asset": asset.Name, "mime": asset.Mime }`.

### `main.go`

- `AssetServer: &assetserver.Options{ Assets: assets, Handler: media.FileServer(app.MediaDir) }`.
  The handler is a fallback for requests the embedded FS doesn't serve, so
  `/media/…` reaches it while the SPA and its assets keep loading from `embed`.

### `image-view.js` (pure, frontend)

- `imageStyles({ naturalW, naturalH, rotation, crop })` → `{ viewport, img }`
  where each is a CSS string (or style object). Computes:
  - rotated frame dims (swap W/H for 90/270),
  - the crop sub-rect (defaults to the full rotated frame when `crop` is null),
  - a `viewport` (`position:relative; overflow:hidden; aspect-ratio:<cropW/cropH>`)
    and an `img` transform (`rotate(rotation)` + translate/scale) that places the
    cropped region to fill the viewport.
- Pure and deterministic (unit-reasoned in the plan with the four rotation cases,
  with and without crop). No DOM access.

### `ImageCell.svelte`

- Props: the shared cell set (`cell, onChange, onDelete, readOnly, context, cells,
  onPush`); `context/cells/onPush` unused this slice.
- Renders `<img src={"/media/" + cell.asset}>` inside a viewport styled by
  `imageStyles(cell)`. `on:load` fills `cell.naturalW/naturalH` from
  `img.naturalWidth/Height` if unset and persists once. `on:error` shows a
  "image not found" placeholder (asset deleted externally).
- Toolbar (hidden when `readOnly`): **↶** / **↷** (rotate ∓90°, wrapping 0..270),
  **Crop** (enter crop mode), **Reset** (clear rotation+crop), **✕** delete.
- **Crop mode:** overlay a draggable selection rectangle on the displayed image;
  **Apply** converts the pixel rect (in displayed → rotated-frame space) to a
  normalized `crop` and persists; **Cancel** discards. One commit per apply.
- Persist (`onChange(cell)`) on rotate, crop-apply, reset, and the one-time
  natural-size fill — never during a crop drag.

## Display sizing

- The viewport fits the notebook content width; height follows the rotated/cropped
  aspect. A CSS `max-height` cap (e.g. `70vh`) scales tall scans down uniformly.
  No stored height, no manual resize handle.

## Persistence

- The cell object (asset ref + edit metadata) flows through the existing
  `onChange(cell) → updateCellObj → debounced SaveNotebook(cells_json)` path.
  Bytes are never in `cells_json`. Metadata is tiny; writes are per completed edit.

## Error handling

- **No collection open:** `ImportImage` returns an error → the frontend shows a
  toast; no cell added.
- **Dialog cancelled:** `ImportImage` returns `(nil, nil)` → no-op.
- **Non-image / unreadable file:** `media.Import` returns an error → toast.
- **Missing asset at display time:** `<img on:error>` → placeholder; the cell and
  its metadata remain (the file may reappear or be re-imported).
- **Path traversal / bad request to the handler:** 404, never escapes `media/`.

## Testing

- **Go (`internal/media`):** unit tests for content-hash naming + extension per
  MIME, dedup (same bytes → same name, written once), non-image rejection, and
  `FileServer` path-traversal rejection (`/media/../secret`, separators, empty
  dir) plus a successful serve of a file under `media/`.
- **Frontend:** no JS test runner — verify with `cd frontend && npm run build`
  plus a manual test plan:
  - Import a jpg/png; it displays; reopen the notebook → still there.
  - Rotate ↶/↷ through all four orientations; Crop a region and Apply; Reset.
  - Cancel the dialog (no cell); import with no collection open (toast).
  - Delete an image cell; confirm the notebook saves (file remains on disk).
  - Restart the app, reopen the extract → image + rotation + crop re-render.
- Full build: `go test ./...` and `PATH=… make build` embed and serve it.

## Success criteria

- "+ Image cell" imports via the OS dialog; the image displays from `/media/…`.
- Rotation (90° steps) and crop apply non-destructively, persist in `cells_json`,
  and re-render after an app restart; Reset returns to the original.
- The original file in `media/` is never modified; re-importing the same image
  does not duplicate it.
- No new Go dependencies (pure stdlib) and no CGo; the asset handler is
  path-traversal safe.

## Dependencies

- **New:** none. `internal/media` uses only the standard library
  (`crypto/sha256`, `net/http`, `os`, `path`/`path/filepath`). Frontend adds no
  npm dependency.
- **Reused:** Slice 1's `cellRegistry` / `NotebookPane` / `cells_json` persistence
  and the `onChange(cell) → updateCellObj → SaveNotebook` path; Wails runtime
  file dialog + `AssetServer`; the collection-directory plumbing in `app.go`.
