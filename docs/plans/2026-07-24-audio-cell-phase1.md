# Audio cell — Phase 1 (import + record + playback + storage) — Implementation Plan

**Goal:** Add an `audio` notebook cell — import an audio file or record in-app (where the webview permits), store it on disk, and play it back. No STT yet (Phase 2).

**Architecture:** Generalize the Slice-5 `internal/media` store for audio (extension-derived, since audio containers sniff unreliably); add `ImportAudio`/`SaveRecording` bindings mirroring `ImportImage`; the `/media` handler sets explicit audio content-types. `AudioCell.svelte` records via `MediaRecorder` (graceful fallback on WebKitGTK) or imports, and plays `<audio src="/media/…" controls>`.

**Tech Stack:** Go stdlib (`internal/media`, bindings); Svelte 3.49 + browser `MediaRecorder`/`getUserMedia`/`<audio>`; existing `/media` handler + `cellRegistry`/`cells_json`. No new deps, no CGo.

## Global Constraints

- No new npm/Go dependencies; no CGo. `internal/media` stays standard-library.
- **Audio bytes never in `cells_json`** — the cell stores `{asset, mime, durationMs}`; bytes live under `<collectionDir>/media/` (content-hashed, dedup).
- **Audio extension is derived, not sniffed:** import uses the source filename extension; recording uses the `MediaRecorder` mimeType (codecs suffix stripped). Do NOT use `http.DetectContentType` for audio.
- Image `Import` behavior must be **unchanged** (regression) after the store refactor.
- Frontend has no JS test runner; frontend tasks verify with `cd frontend && npm run build` + a manual test plan. Go uses `go test ./...`.
- The `/media` handler stays path-traversal safe.
- Record degrades gracefully where `getUserMedia` is denied (WebKitGTK/WSLg) — a clear message, import still available; no crash.

---

## File Structure

- `internal/media/import.go` — MODIFY: extract `store()`; add `ImportAudio`, `SaveAudio`, audio ext/mime maps.
- `internal/media/server.go` — MODIFY: set audio `Content-Type` for known audio extensions.
- `internal/media/media_test.go` — MODIFY: add audio tests.
- `app.go` — MODIFY: `ImportAudio()` + `SaveRecording()` bindings.
- `frontend/src/components/notebook/AudioCell.svelte` — CREATE.
- `frontend/src/lib/notebook.js` — MODIFY: register `audio` + `newAudioCell()`.
- `frontend/src/components/notebook/NotebookPane.svelte` — MODIFY: "+ Audio cell" button.
- `docs/specs/2026-07-24-audio-cell-phase1-manual-test-plan.md` — CREATE (Task 5).

---

## Task 1: `internal/media` — audio storage (TDD)

**Files:**
- Modify: `internal/media/import.go`, `internal/media/server.go`, `internal/media/media_test.go`

**Interfaces:**
- Produces: `media.ImportAudio(collectionDir, srcPath) (Asset, error)`, `media.SaveAudio(collectionDir, data []byte, mime string) (Asset, error)`. Consumed by `app.go` (Task 2).

- [ ] **Step 1: Add the failing tests**

Append to `internal/media/media_test.go`:

```go
func TestImportAudioByExtension(t *testing.T) {
	col := t.TempDir()
	src := filepath.Join(t.TempDir(), "memo.MP3") // mixed case
	if err := os.WriteFile(src, []byte("not really mp3 but ext decides"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := ImportAudio(col, src)
	if err != nil {
		t.Fatalf("ImportAudio: %v", err)
	}
	if a.Mime != "audio/mpeg" || !strings.HasSuffix(a.Name, ".mp3") {
		t.Errorf("got name=%q mime=%q", a.Name, a.Mime)
	}
	if _, err := os.Stat(filepath.Join(col, "media", a.Name)); err != nil {
		t.Errorf("not written: %v", err)
	}
	// dedup
	b, _ := ImportAudio(col, src)
	if b.Name != a.Name {
		t.Errorf("dedup failed: %q vs %q", b.Name, a.Name)
	}
}

func TestImportAudioRejectsNonAudioExt(t *testing.T) {
	col := t.TempDir()
	src := filepath.Join(t.TempDir(), "note.txt")
	_ = os.WriteFile(src, []byte("x"), 0o644)
	if _, err := ImportAudio(col, src); err == nil {
		t.Fatal("expected rejection for .txt")
	}
}

func TestSaveAudioStripsCodecs(t *testing.T) {
	col := t.TempDir()
	a, err := SaveAudio(col, []byte("opusbytes"), "audio/webm;codecs=opus")
	if err != nil {
		t.Fatalf("SaveAudio: %v", err)
	}
	if a.Mime != "audio/webm" || !strings.HasSuffix(a.Name, ".webm") {
		t.Errorf("got name=%q mime=%q", a.Name, a.Mime)
	}
}

func TestSaveAudioRejectsUnknownMime(t *testing.T) {
	col := t.TempDir()
	if _, err := SaveAudio(col, []byte("x"), "audio/weird"); err == nil {
		t.Fatal("expected rejection for audio/weird")
	}
}
```

(The test file already imports `strings`, `os`, `path/filepath`, `testing`; if `strings` is missing, add it.)

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/media/ -run 'Audio'`
Expected: FAIL (undefined `ImportAudio`/`SaveAudio`).

- [ ] **Step 3: Refactor `import.go` and add audio functions**

Replace the whole body of `internal/media/import.go` with:

```go
// Package media stores notebook image and audio assets on disk under
// <collectionDir>/media/ and serves them to the webview. Assets are
// content-addressed (sha256), so identical bytes are stored once. Pure standard
// library, no CGo.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Asset is a stored file: its filename under media/ and MIME type.
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

// audioMimeByExt maps a lowercased audio file extension to its MIME type
// (used by ImportAudio and the /media content-type on playback).
var audioMimeByExt = map[string]string{
	".webm": "audio/webm", ".ogg": "audio/ogg", ".oga": "audio/ogg",
	".wav": "audio/wav", ".mp3": "audio/mpeg", ".m4a": "audio/mp4",
	".mp4": "audio/mp4", ".aac": "audio/aac", ".flac": "audio/flac",
}

// audioExtByMime maps a normalized audio MIME (no codecs suffix) to an extension
// (used by SaveAudio for recordings).
var audioExtByMime = map[string]string{
	"audio/webm": ".webm", "audio/ogg": ".ogg", "audio/wav": ".wav",
	"audio/mpeg": ".mp3", "audio/mp4": ".m4a", "audio/aac": ".aac", "audio/flac": ".flac",
}

// store writes data to <collectionDir>/media/<sha256><ext> (dedup) and returns
// the filename. Shared by image and audio storage.
func store(collectionDir string, data []byte, ext string) (string, error) {
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + ext
	dir := filepath.Join(collectionDir, "media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("media: mkdir: %w", err)
	}
	dst := filepath.Join(dir, name)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return "", fmt.Errorf("media: write: %w", err)
		}
	}
	return name, nil
}

// Import copies the image at srcPath into media/ (content-hashed). Non-image
// files are rejected (MIME sniffed).
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
	name, err := store(collectionDir, data, ext)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: name, Mime: mime}, nil
}

// ImportAudio copies the audio file at srcPath into media/, keyed by its file
// extension (audio containers sniff unreliably). Rejects non-audio extensions.
func ImportAudio(collectionDir, srcPath string) (Asset, error) {
	ext := strings.ToLower(filepath.Ext(srcPath))
	mime, ok := audioMimeByExt[ext]
	if !ok {
		return Asset{}, fmt.Errorf("media: unsupported audio extension %q", ext)
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return Asset{}, fmt.Errorf("media: read source: %w", err)
	}
	name, err := store(collectionDir, data, ext)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: name, Mime: mime}, nil
}

// SaveAudio stores recorded audio bytes. mime may carry a codecs suffix
// (e.g. "audio/webm;codecs=opus"), which is stripped to select the extension.
func SaveAudio(collectionDir string, data []byte, mime string) (Asset, error) {
	base := mime
	if i := strings.IndexByte(base, ';'); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	ext, ok := audioExtByMime[base]
	if !ok {
		return Asset{}, fmt.Errorf("media: unsupported audio mime %q", mime)
	}
	name, err := store(collectionDir, data, ext)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: name, Mime: base}, nil
}
```

- [ ] **Step 4: Set audio content-type in `server.go`**

In `internal/media/server.go`, immediately before the final `http.ServeFile(...)` line, add:

```go
		if ct, ok := audioMimeByExt[strings.ToLower(filepath.Ext(name))]; ok {
			w.Header().Set("Content-Type", ct) // Go's mime table calls .webm video/webm; be explicit for <audio>
		}
```

(`strings` and `path/filepath` are already imported in `server.go`.)

- [ ] **Step 5: Run to verify pass + no regression**

Run: `go test ./internal/media/`
Expected: PASS — the four new audio tests plus the existing image `Import`/`FileServer` tests (regression-safe: image path unchanged).

- [ ] **Step 6: Commit**

```bash
git add internal/media/
git commit -m "feat(media): audio storage (ImportAudio/SaveAudio) + audio content-type"
```

---

## Task 2: `app.go` — `ImportAudio` + `SaveRecording` bindings

**Files:**
- Modify: `app.go`

**Interfaces:**
- Consumes: `media.ImportAudio`, `media.SaveAudio` (Task 1).
- Produces: `ImportAudio()` and `SaveRecording(dataBase64, mime string)` Wails bindings, both returning `{asset, mime}` (or `null`/error), consumed by `AudioCell` (Task 3). `make build` regenerates the JS bindings.

- [ ] **Step 1: Add the bindings**

Add to `app.go` (near `ImportImage`). Ensure the imports include `encoding/base64` (add if missing):

```go
// ImportAudio opens the OS file dialog (audio filter), copies the chosen file
// into media/, and returns {asset, mime}. (nil, nil) on cancel; error if no
// collection is open or the type is unsupported.
func (a *App) ImportAudio() (map[string]any, error) {
	a.dirMu.RLock()
	dir := a.collectionDir
	a.dirMu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("no collection open")
	}
	sel, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Add audio",
		Filters: []runtime.FileFilter{
			{DisplayName: "Audio (*.webm, *.ogg, *.wav, *.mp3, *.m4a, *.aac, *.flac)", Pattern: "*.webm;*.ogg;*.oga;*.wav;*.mp3;*.m4a;*.mp4;*.aac;*.flac"},
		},
	})
	if err != nil {
		return nil, err
	}
	if sel == "" {
		return nil, nil
	}
	asset, err := media.ImportAudio(dir, sel)
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": asset.Name, "mime": asset.Mime}, nil
}

// SaveRecording decodes base64 audio bytes from an in-app recording and stores
// them in media/, returning {asset, mime}.
func (a *App) SaveRecording(dataBase64, mime string) (map[string]any, error) {
	a.dirMu.RLock()
	dir := a.collectionDir
	a.dirMu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("no collection open")
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return nil, fmt.Errorf("decode recording: %w", err)
	}
	asset, err := media.SaveAudio(dir, data, mime)
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": asset.Name, "mime": asset.Mime}, nil
}
```

- [ ] **Step 2: Build (compiles + regenerates bindings)**

Run: `go build ./...`
Expected: compiles clean.

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`; regenerates `frontend/wailsjs/go/main/App.{js,d.ts}` to include `ImportAudio` and `SaveRecording`.

- [ ] **Step 3: Confirm the bindings**

Run: `grep -nE "ImportAudio|SaveRecording" frontend/wailsjs/go/main/App.d.ts`
Expected: both declared.

- [ ] **Step 4: Commit**

```bash
git add app.go frontend/wailsjs/
git commit -m "feat(notebook): ImportAudio + SaveRecording bindings"
```

---

## Task 3: `AudioCell.svelte`

**Files:**
- Create: `frontend/src/components/notebook/AudioCell.svelte`

**Interfaces:**
- Consumes: `ImportAudio`, `SaveRecording` bindings (Task 2); cell shape `{id, type:'audio', asset, mime, durationMs}`.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/notebook/AudioCell.svelte`:

```svelte
<script>
  import { onDestroy } from 'svelte'
  import { ImportAudio, SaveRecording } from '../../../wailsjs/go/main/App'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused

  let recording = false
  let recUnavailable = false
  let recErr = ''
  let elapsed = 0
  let missing = false
  let rec = null
  let chunks = []
  let stream = null
  let timer = null
  let startedAt = 0

  function persist() { onChange(cell) }

  async function startRec() {
    recErr = ''
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    } catch (e) {
      recUnavailable = true
      recErr = 'Recording isn’t available in this webview — import an audio file instead.'
      return
    }
    const prefs = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus', 'audio/ogg', 'audio/mp4', '']
    let m = ''
    for (const p of prefs) { if (!p || (window.MediaRecorder && MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(p))) { m = p; break } }
    rec = m ? new MediaRecorder(stream, { mimeType: m }) : new MediaRecorder(stream)
    chunks = []
    rec.ondataavailable = (e) => { if (e.data && e.data.size) chunks.push(e.data) }
    rec.onstop = onStop
    rec.start()
    recording = true
    startedAt = Date.now()
    elapsed = 0
    timer = setInterval(() => { elapsed = Date.now() - startedAt }, 200)
  }

  function stopRec() { try { if (rec && rec.state !== 'inactive') rec.stop() } catch (_) {} }

  async function onStop() {
    clearInterval(timer)
    recording = false
    const durationMs = Date.now() - startedAt
    const mime = (rec && rec.mimeType) || 'audio/webm'
    if (stream) stream.getTracks().forEach((t) => t.stop())
    const blob = new Blob(chunks, { type: mime })
    try {
      const b64 = await blobToBase64(blob)
      const ref = await SaveRecording(b64, mime)
      cell.asset = ref.asset; cell.mime = ref.mime; cell.durationMs = durationMs
      missing = false; persist()
    } catch (e) { recErr = String(e?.message ?? e) }
  }

  function blobToBase64(blob) {
    return new Promise((resolve, reject) => {
      const r = new FileReader()
      r.onload = () => resolve(String(r.result).split(',')[1])
      r.onerror = () => reject(new Error('read failed'))
      r.readAsDataURL(blob)
    })
  }

  async function importAudio() {
    recErr = ''
    try {
      const ref = await ImportAudio()
      if (!ref) return
      cell.asset = ref.asset; cell.mime = ref.mime; cell.durationMs = 0
      missing = false; persist()
    } catch (e) { recErr = String(e?.message ?? e) }
  }

  function onMeta(e) {
    const d = Math.round((e.target.duration || 0) * 1000)
    if (d && d !== cell.durationMs) { cell.durationMs = d; persist() }
  }

  function replace() { cell.asset = ''; cell.mime = ''; cell.durationMs = 0; missing = false; persist() }

  function fmt(ms) {
    const s = Math.floor((ms || 0) / 1000)
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
  }

  onDestroy(() => { clearInterval(timer); if (stream) stream.getTracks().forEach((t) => t.stop()) })
</script>

<div class="au-cell">
  <div class="au-bar">
    <span class="au-title">🎙 Audio</span>
    {#if cell.asset}<span class="au-dur">{fmt(cell.durationMs)}</span>{/if}
    <span class="au-spacer"></span>
    {#if !readOnly && cell.asset}<button on:click={replace} title="Replace">Replace</button>{/if}
    {#if !readOnly}<button class="au-del" on:click={onDelete} title="Delete cell">✕</button>{/if}
  </div>

  {#if cell.asset}
    {#if missing}
      <div class="au-missing">audio not found: {cell.asset}</div>
    {:else}
      <audio class="au-player" src={"/media/" + cell.asset} controls on:loadedmetadata={onMeta} on:error={() => (missing = true)}></audio>
    {/if}
  {:else if !readOnly}
    <div class="au-empty">
      {#if recording}
        <button class="au-rec" on:click={stopRec}>■ Stop ({fmt(elapsed)})</button>
      {:else}
        <button class="au-rec" on:click={startRec} disabled={recUnavailable}>● Record</button>
        <button on:click={importAudio}>+ Import audio</button>
      {/if}
      {#if recErr}<div class="au-err">{recErr}</div>{/if}
    </div>
  {:else}
    <div class="au-missing">no audio</div>
  {/if}
</div>

<style>
  .au-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .au-bar { display: flex; align-items: center; gap: 8px; padding: 5px 8px; border-bottom: 1px solid #1c2a3a; }
  .au-title { color: #7dd3fc; font-size: 11px; letter-spacing: .06em; text-transform: uppercase; }
  .au-dur { color: #93c5fd; font-size: 11px; }
  .au-spacer { flex: 1; }
  .au-bar button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
  .au-del { background: none !important; border: none !important; color: #6b7a8a !important; }
  .au-del:hover { color: #ff8080 !important; }
  .au-empty { display: flex; align-items: center; gap: 8px; padding: 10px; flex-wrap: wrap; }
  .au-empty button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 4px 12px; cursor: pointer; font-size: 12px; }
  .au-empty button:disabled { opacity: .4; cursor: default; }
  .au-player { width: 100%; display: block; padding: 8px; }
  .au-err { color: #ff8080; font-size: 12px; width: 100%; }
  .au-missing { padding: 14px; color: #ff8080; font-size: 13px; }
</style>
```

- [ ] **Step 2: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds (component is tree-shaken until registered in Task 4).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/notebook/AudioCell.svelte
git commit -m "feat(notebook): audio cell (record/import + playback)"
```

---

## Task 4: Register `audio` + NotebookPane button (compile gate)

**Files:**
- Modify: `frontend/src/lib/notebook.js`, `frontend/src/components/notebook/NotebookPane.svelte`

**Interfaces:**
- Consumes: `AudioCell` (Task 3), `newAudioCell` (this task).

- [ ] **Step 1: Register in `notebook.js`**

Add the import alongside the others:

```js
import AudioCell from '../components/notebook/AudioCell.svelte'
```

Add the factory (after `newImageCell`):

```js
export function newAudioCell() {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'audio', asset: '', mime: '', durationMs: 0 }
}
```

Add to `cellRegistry`:

```js
  image: ImageCell,
  audio: AudioCell,
```

- [ ] **Step 2: Wire `NotebookPane.svelte`**

Extend the `notebook.js` import to include `newAudioCell`:

```js
  import { newDraftCell, newLLMCell, newGraphCell, newComputeCell, newSketchCell, newImageCell, newAudioCell, cellRegistry } from '../../lib/notebook.js'
```

Add the handler (after `addImage`):

```js
  function addAudio() { cells = [...cells, newAudioCell()]; scheduleSave() }
```

Add the button after the "+ Image cell" button:

```svelte
      <button class="nb-add" on:click={addImage}>+ Image cell</button>
      <button class="nb-add" on:click={addAudio}>+ Audio cell</button>
```

- [ ] **Step 3: Build (real compile gate — pulls AudioCell into the bundle)**

Run: `cd frontend && npm run build`
Expected: build succeeds; `AudioCell.svelte` now compiles into the bundle.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/notebook.js frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): register audio cell + '+ Audio cell' button"
```

---

## Task 5: Manual test plan + full verification

**Files:**
- Create: `docs/specs/2026-07-24-audio-cell-phase1-manual-test-plan.md`

- [ ] **Step 1: Write the manual test plan**

Create the file:

```markdown
# Audio cell — Phase 1 Manual Test Plan

Prereqs: open a collection and an extract for processing.

## TC-1 Import + playback
1. Click "+ Audio cell"; click "+ Import audio"; pick an mp3 or wav.
   - EXPECT: the file dialog is filtered to audio; after choosing, an <audio> player appears and plays the clip; the duration shows.

## TC-2 Record where unsupported (WebKitGTK/WSL)
1. On a fresh Audio cell, click "● Record".
   - EXPECT (on Linux/WebKitGTK): a clear "recording isn’t available in this webview — import an audio file instead" message; the Record button disables; Import still works. No crash.
   - EXPECT (on a Windows/WebView2 build, if available): a mic-permission prompt; after allowing, "■ Stop" with a running timer; on Stop the clip is saved and plays back.

## TC-3 Replace
1. On a loaded cell, click "Replace"; import a different file.
   - EXPECT: the cell swaps to the new clip.

## TC-4 Delete cell
1. Click ✕ on an audio cell.
   - EXPECT: the cell is removed; the notebook saves (the media file remains on disk — orphan, consistent with images).

## TC-5 Persistence across restart
1. Import an audio clip, restart the app, reopen the extract.
   - EXPECT: the audio cell re-loads and still plays.

## TC-6 Dedup + missing asset
1. Import the same file into two cells — <collectionDir>/media/ holds one file for it.
2. Delete that file from disk while a cell references it; reopen the notebook.
   - EXPECT: an "audio not found" placeholder, no crash.
```

- [ ] **Step 2: Backend suite + full build**

Run: `go test ./...`
Expected: all PASS (includes `internal/media` audio tests).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`.

- [ ] **Step 3: Execute the manual test plan**

Walk TC-1 … TC-6 in `./build/bin/texere`.

- [ ] **Step 4: Commit**

```bash
git add docs/specs/2026-07-24-audio-cell-phase1-manual-test-plan.md
git commit -m "docs(test-plan): Audio cell Phase 1 manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** audio storage (ext-derived import + mime-derived record, shared `store`, image `Import` unchanged) + audio content-type on playback (T1); `ImportAudio`/`SaveRecording` bindings (T2); audio cell with record (graceful WebKitGTK fallback), import, `<audio>` playback, replace/delete, duration, missing-asset (T3); register + button compile gate (T4); manual plan + full verification (T5). STT/transcription is Phase 2, out of scope.
- **Placeholder scan:** no TBD/TODO; every code step complete; Go steps follow TDD.
- **Type consistency:** `media.Asset{Name,Mime}` → `ImportAudio`/`SaveRecording` return `{asset,mime}` → `AudioCell` sets `cell.asset/mime/durationMs` (shape from `newAudioCell`). `SaveRecording(dataBase64, mime)` ↔ frontend `blobToBase64` + `rec.mimeType`. `audioMimeByExt`/`audioExtByMime` are the two directions (import vs record); `store(dir,data,ext)` shared by image and audio.
- **Compile-sequencing:** `AudioCell` (T3) is tree-shaken until T4 registers it; T4's `npm run build` is the real compile gate; the `ImportAudio`/`SaveRecording` JS bindings are generated by T2's `make build` before T3 imports them.
```
