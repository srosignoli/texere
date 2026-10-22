# Audio cell — Phase 1: capture (import + record) + storage + playback

**Date:** 2026-07-24
**Status:** Approved design
**Feature area:** Processing Notebook (incremental reading) — new cell type
**Phase:** 1 of 2. Phase 2 (STT config + Transcribe → N4L draft) is a separate
spec.

## Summary

Add an **audio cell** so the user can keep a voice memo / spoken idea in a
notebook during processing. Audio enters the cell two ways: **import** an audio
file (file picker → on-disk asset — works on every platform) or **record**
in-app via `MediaRecorder` where the webview permits it. The recording is stored
on disk under `<collectionDir>/media/` (content-hashed, reusing Slice 5's store,
generalized for audio) and played back with a native `<audio controls>` element
served by the existing `/media` handler. `cells_json` stores only a reference +
metadata, never audio bytes.

Speech-to-text transcription is **Phase 2** (its own spec): an STT-provider
config mirroring the LLM config, and a Transcribe button that produces an N4L
draft. This phase ships capture, storage, and playback — independently useful.

## Motivation & the recording-feasibility finding

A mic-capture spike in the current build showed that in-app recording is
**blocked on Linux/WebKitGTK (WSLg)**: `MediaRecorder`/`getUserMedia` exist, but
WebKitGTK auto-denies the mic permission (`NotAllowedError`, no prompt) because
Wails v2.12 installs no permission-request handler and exposes no hook to grant
one. Unblocking it needs native GTK work (a constitution CGo exception) and WSLg
mic input is itself uncertain. On the **Windows** target (WebView2/Chromium),
`getUserMedia` works with a normal permission prompt, so recording is expected to
work there.

Therefore this design is **import-first with record-where-supported**: import
always works (the primary path on WSL today); the Record button attempts
`getUserMedia` and, on failure, degrades to a clear "recording unavailable on
this platform — import a file instead" message. No native/CGo work; nothing is
gated to a platform in the build.

## Decisions made during brainstorming

- **Import + record-where-supported**, degrading gracefully on WebKitGTK.
- **On-disk assets** in `<collectionDir>/media/`, content-hashed, reusing the
  Slice 5 media store generalized to audio. No audio bytes in `cells_json`.
- **Audio extension is derived, not sniffed** — from the source filename (import)
  or the `MediaRecorder` mimeType (record); `http.DetectContentType` is unreliable
  for audio containers.
- **STT/transcription deferred to Phase 2** (separate spec).

## Scope

**In scope**

- An `audio` cell type + "+ Audio cell" button in `NotebookPane`.
- **Import:** an `ImportAudio()` binding (OS file dialog filtered to audio) →
  copy into `media/` → return the reference.
- **Record:** `getUserMedia` + `MediaRecorder`; on stop, POST the bytes to a
  `SaveRecording(dataBase64, mime)` binding → `media/`. Graceful failure on
  WebKitGTK.
- **Playback:** `<audio src="/media/…" controls>`; the `/media` handler serves
  the correct audio content-type.
- Replace (re-import/re-record) and delete; a duration readout.

**Out of scope (Phase 2 / later)**

- STT config, `internal/stt` provider, the Transcribe button / N4L-draft output.
- Native WebKitGTK mic permission handler / WSLg mic enablement.
- Waveform visualization, trimming/editing, multi-clip, format conversion.

## Data model (`cells_json`)

```json
{ "id": "…", "type": "audio", "asset": "ab12….webm", "mime": "audio/webm", "durationMs": 14200 }
```

- `asset` — filename under `<collectionDir>/media/` (empty until imported/recorded).
- `mime` — the audio MIME (`audio/webm`, `audio/mpeg`, `audio/wav`, …), used for
  display and (Phase 2) transcription upload.
- `durationMs` — captured from the recorder timer or the `<audio>` element's
  `loadedmetadata` duration; for display. `0` when unknown.

## Architecture & components

```
internal/media/import.go     # MODIFY: extract a shared store(); add ImportAudio + SaveAudio (+ audio allowlist)
internal/media/server.go     # MODIFY (if needed): set audio content-types for known audio extensions
app.go                       # ImportAudio() + SaveRecording() bindings (mirror ImportImage)
internal/media/media_test.go # MODIFY: audio store/ext + dedup tests
frontend/src/
  ├── components/notebook/AudioCell.svelte   # NEW: record/import + <audio> playback + replace/delete
  ├── lib/notebook.js                        # register 'audio'; newAudioCell()
  └── components/notebook/NotebookPane.svelte# "+ Audio cell" button
```

### `internal/media` (generalized for audio)

- Refactor the hash-and-write core out of `Import` into
  `store(collectionDir string, data []byte, ext string) (name string, error)`
  (MkdirAll, `<sha256><ext>`, dedup) — `Import` keeps its image sniffing and calls
  `store`. No behavior change for images.
- Add an audio allowlist by **file extension** (import) and by **MIME** (record):
  `.webm .ogg .oga .wav .mp3 .m4a .mp4 .aac .flac` ↔ `audio/webm`, `audio/ogg`,
  `audio/wav`, `audio/mpeg`, `audio/mp4`, `audio/aac`, `audio/flac`.
- `ImportAudio(collectionDir, srcPath string) (Asset, error)`: read the file;
  derive `ext := strings.ToLower(filepath.Ext(srcPath))`; reject if not in the
  audio allowlist; `store(...)`; return `Asset{Name, Mime: mimeForExt(ext)}`.
- `SaveAudio(collectionDir string, data []byte, mime string) (Asset, error)`:
  normalize `mime` (strip `;codecs=…`); map to an allowed ext (reject otherwise);
  `store(...)`; return `Asset{Name, Mime}`. Used for recordings.

### `app.go` bindings (mirror `ImportImage`)

- `ImportAudio() (map[string]any, error)`: `mediaDir` guard; `runtime.OpenFileDialog`
  filtered to `*.webm;*.ogg;*.oga;*.wav;*.mp3;*.m4a;*.mp4;*.aac;*.flac`; cancel →
  `(nil, nil)`; else `media.ImportAudio(dir, sel)` → `{asset, mime}`.
- `SaveRecording(dataBase64, mime string) (map[string]any, error)`: guard;
  `base64.StdEncoding.DecodeString`; `media.SaveAudio(dir, data, mime)` →
  `{asset, mime}`.

### `AudioCell.svelte`

- Props: the shared cell set (`cell, onChange, onDelete, readOnly, context, cells,
  onPush`); `context/cells/onPush` unused this phase. (`onInsert` will be added in
  Phase 2 for transcription output.)
- **Empty state** (`!cell.asset`, not `readOnly`): a **● Record** button and a
  **+ Import audio** button.
  - **Record:** `await navigator.mediaDevices.getUserMedia({audio:true})`; pick a
    supported `MediaRecorder` mimeType; record with a running timer; **Stop** →
    build a `Blob`, base64-encode, call `SaveRecording(b64, mime)`; set
    `cell.asset/mime/durationMs`; persist. On `getUserMedia` rejection, set a
    `recUnavailable` flag → show "Recording isn't available in this webview —
    import an audio file instead." (Import still works.)
  - **Import:** `await ImportAudio()`; if a ref returns, set the cell + persist;
    read `durationMs` from the `<audio>` `loadedmetadata` once loaded.
- **Loaded state** (`cell.asset`): `<audio src={"/media/"+cell.asset} controls
  on:loadedmetadata>` + a duration label + **Replace** (re-run record/import) and
  the shared **✕** delete. `on:error` → "audio not found" placeholder (asset
  removed externally).
- Persist (`onChange(cell)`) only on completed actions (record saved, import done,
  replace, metadata fill) — never during recording.

### `notebook.js` / `NotebookPane.svelte`

- `newAudioCell()` → `{ id: …, type: 'audio', asset: '', mime: '', durationMs: 0 }`;
  register `audio: AudioCell`.
- Add `function addAudio() { cells = [...cells, newAudioCell()]; scheduleSave() }`
  and a "+ Audio cell" button after "+ Image cell".

## Playback content-type

`http.ServeFile` derives Content-Type from the extension via Go's `mime` table,
which is incomplete for some audio containers (e.g. `.webm` → `video/webm`). The
`/media` handler will set an explicit `Content-Type` for known audio extensions
(`.webm/.ogg/.oga` → `audio/*`, etc.) before serving, so `<audio>` plays reliably.
(Codec availability in WebKitGTK — e.g. Opus in webm — is a runtime concern
verified in the manual test; imported `mp3`/`wav`/`m4a` are the safe common case.)

## Error handling

- **No collection open:** `ImportAudio`/`SaveRecording` return an error → toast.
- **Dialog cancelled:** `ImportAudio` returns `(nil, nil)` → no-op.
- **Unsupported type:** `media.ImportAudio`/`SaveAudio` reject → toast.
- **Record denied / unsupported (WebKitGTK):** `getUserMedia` rejects →
  `recUnavailable` message; import remains available; no crash.
- **Missing asset at playback:** `<audio on:error>` → placeholder.
- **Path traversal:** unchanged — the `/media` handler already sanitizes.

## Testing

- **Go (`internal/media`):** unit tests for `ImportAudio` (ext allowlist,
  content-hash name, dedup, reject non-audio ext) and `SaveAudio` (mime→ext,
  codecs-suffix stripping, reject unknown mime); confirm image `Import` is
  unchanged (regression).
- **Frontend:** no JS test runner — `cd frontend && npm run build` + a manual test
  plan: import an mp3/wav → it plays; "+ Audio cell" then Record on WSL → the
  graceful "unavailable" message (and, on a WebView2/Windows build if available,
  an actual recording that plays back); Replace; delete; restart → the audio
  re-loads; delete the file on disk → placeholder.
- Full build: `go test ./...` and `make build`.

## Success criteria

- "+ Audio cell" imports an audio file that plays back in-cell; the asset persists
  and re-loads after restart; no audio bytes in `cells_json`.
- Record works where the webview permits it and degrades to a clear message where
  it doesn't (WebKitGTK/WSLg), with import always available.
- No new Go/npm dependencies; no CGo; the media store and `/media` handler remain
  path-traversal safe.

## Dependencies

- **New:** none. `internal/media` stays standard-library; the frontend uses the
  browser `MediaRecorder`/`getUserMedia`/`<audio>` APIs and the existing
  `cellRegistry`/`NotebookPane`/`cells_json` + `/media` handler.
- **Reused:** Slice 5's media store + asset handler + `ImportImage` binding
  pattern; the notebook cell registration/persistence path.
