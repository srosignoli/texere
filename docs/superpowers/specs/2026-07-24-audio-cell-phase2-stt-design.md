# Audio cell — Phase 2: STT config + Transcribe → N4L draft

**Date:** 2026-07-24
**Status:** Approved design
**Feature area:** Processing Notebook (incremental reading) — Audio cell
**Phase:** 2 of 2. Phase 1 (audio cell: import/record/playback/storage) is merged.

## Summary

Add speech-to-text to the audio cell. A **Transcribe** button (with a per-cell
model dropdown) uploads the stored audio to a configurable, engine-agnostic
**OpenAI-compatible `/audio/transcriptions`** endpoint (local whisper server or
cloud) and inserts the returned text as a new N4L draft cell — reusing the 6a
`onInsert` path. STT providers are configured exactly like LLM providers, in a
new **Speech-to-text** tab of the ⚙ Models panel.

Because transcription **uploads the stored file** (no local audio decode), this
works even on the WSLg/WebKitGTK dev environment where playback/record are
blocked by the missing media backend.

## Decisions (from brainstorming)

- **Engine-agnostic OpenAI-compatible endpoint** (`POST <baseUrl>/audio/
  transcriptions`, multipart `model` + `file` → `{text}`). Local (whisper.cpp/
  faster-whisper/LocalAI) or cloud (OpenAI) — same wire shape.
- **STT config mirrors the LLM config**, reusing the `LLMProfile`/`LLMConfig`
  types and the generic `findProfile`/`upsertProfile`/`deleteProfile` helpers —
  no new config types.
- **STT `Kind` is fixed `openai-compatible`** (Anthropic has no audio API); the
  STT editor hides the Kind selector.
- **API key is optional for STT** — local servers need no auth; a blank key ⇒ no
  `Authorization` header (differs from LLM, which requires a key).
- **Transcribe output → a new N4L draft cell** (reuses 6a's `onInsert`).
- **No Test button for STT** — validation is transcribing a real clip.
- Non-streaming: one request returns the full transcript.

## Scope

**In scope**

- `AppConfig.STT LLMConfig`; STT profile CRUD bindings (`GetSTTConfig`,
  `SaveSTTProfile`, `DeleteSTTProfile`, `SetActiveSTTProfile`).
- `internal/stt` provider: multipart POST to `/audio/transcriptions`, bearer auth
  when a key is set, `{text}` (or plain-text) response.
- `TranscribeAudio(profileId, asset) (string, error)` binding.
- A **Speech-to-text** tab in the ⚙ Models settings, reusing the profile
  list/editor/presets against the STT bindings (Kind hidden, no Test, key optional).
- A Transcribe row on the audio cell (model dropdown + button) → N4L draft.

**Out of scope**

- Language / prompt / timestamp / translation params; streaming or partial
  transcripts; diarization; Anthropic-kind STT.
- A Test button for STT profiles.

## Config (Go)

- `AppConfig` gains `STT LLMConfig \`json:"stt"\`` alongside `LLM`.
- **Reuse** `LLMProfile` (`id, name, kind, baseUrl, model, apiKey, apiKeyEnv`) and
  the `findProfile`/`upsertProfile`/`deleteProfile` helpers (they operate on
  `*LLMConfig`). For STT, `kind` is always `openai-compatible`.
- **Key resolution (optional):** a `resolveKeyOptional(p LLMProfile) (string, error)`
  that returns `""` (no error) when neither `apiKey` nor `apiKeyEnv` is set, still
  resolves the env var when `apiKeyEnv` is set, and errors only if a named env var
  is empty. STT uses this; LLM keeps the strict `resolveKey`.

## Provider — `internal/stt` (pure stdlib)

```go
// Transcribe uploads audio to an OpenAI-compatible /audio/transcriptions endpoint
// and returns the transcript text. apiKey may be "" (no Authorization header) for
// local servers. filename gives the multipart part a sensible name/extension.
func Transcribe(ctx context.Context, baseURL, apiKey, model string, audio []byte, filename string) (string, error)
```

- Build a `multipart/form-data` body: field `model`, file field `file`
  (filename = the asset name, so the extension signals the format), audio bytes.
- `POST <trimRightSlash(baseURL)>/audio/transcriptions`; `Authorization: Bearer
  <key>` only when `key != ""`.
- Non-200 → error with a trimmed body snippet (mirrors the `llm` providers).
- Response parse: JSON `{ "text": "…" }`; if JSON decode fails, fall back to the
  trimmed raw body (servers configured for `response_format=text`).
- Uses the shared `httpClient` (as the llm providers do) with the passed `ctx`.

## Bindings — `stt_methods.go` (mirror the LLM ones)

- `GetSTTConfig() LLMConfig` → `cfg.STT`.
- `SaveSTTProfile(p LLMProfile) error` → `upsertProfile(&cfg.STT, p)` + save.
- `DeleteSTTProfile(id string) error` → `deleteProfile(&cfg.STT, id)` + save.
- `SetActiveSTTProfile(id string) error` → validate + set `cfg.STT.ActiveProfileID`.
- `TranscribeAudio(profileId, asset string) (string, error)`:
  - resolve the STT profile: `id` or `cfg.STT.ActiveProfileID`; error if none.
  - `key, err := resolveKeyOptional(p)`.
  - read the file: `dir := a.mediaDir()` (Slice-5, unexported); error if `""`;
    `data, err := os.ReadFile(filepath.Join(dir, filepath.Base(asset)))`
    (`filepath.Base` guards against traversal).
  - `text, err := stt.Transcribe(ctx, p.BaseURL, key, p.Model, data, asset)`.
  - return `text`.

## Settings UI — Speech-to-text tab

- The ⚙ Models modal gains a tab toggle at the top: **LLM Providers | Speech-to-text**.
- The STT tab reuses the same profile **list + editor + presets** UI, wired to the
  STT bindings (`GetSTTConfig`/`SaveSTTProfile`/`DeleteSTTProfile`/
  `SetActiveSTTProfile`), with: the **Kind selector hidden** (profiles saved with
  `kind: 'openai-compatible'`), **no Test button**, and the API-key field labeled
  "optional (local servers)". STT presets: **OpenAI Whisper**
  (`https://api.openai.com/v1`, `whisper-1`) and **Local whisper**
  (`http://localhost:8080/v1`, `whisper-1`, no key).
- Implementation may parametrize the existing component or add a sibling
  `STTSettings` — the plan decides; the UX is the two tabs above.

## Audio cell — Transcribe

- `AudioCell` declares `export let onInsert = () => {}` (already passed by
  `NotebookPane` since 6a; currently ignored).
- When `cell.asset` is set (and not `readOnly`): a **Transcribe** row —
  a model `<select>` from `GetSTTConfig().profiles` (bound to
  `cell.transcribeProfileId`, persisted via `onChange`) + a **Transcribe** button.
- Click → `busy` state → `await TranscribeAudio(cell.transcribeProfileId ?? '',
  cell.asset)` → on success `onInsert(text)` (inserts an N4L draft below) when
  non-empty; errors shown inline. Non-streaming (no `streamTurn`).
- If no STT profiles are configured, the dropdown is empty and Transcribe is
  disabled with a hint to configure one in ⚙ Models.

## Error handling

- No collection / no `mediaDir` → error surfaced inline.
- No STT profile configured → Transcribe disabled with a hint.
- Endpoint/auth/model error or non-200 → the provider error string shown inline.
- Missing asset on disk → read error surfaced.
- Empty transcript → no draft inserted.

## Testing

- **Go (`internal/stt`):** a table/httptest test — `Transcribe` posts multipart
  with `model` + `file` and parses a `{text}` JSON response; a plain-text response
  falls back to the raw body; a non-200 returns an error; no `Authorization`
  header when `apiKey==""` and a bearer header when set. (Same httptest style as
  the `llm` provider tests.)
- **Go (config/bindings):** `resolveKeyOptional` returns `""` for a keyless
  profile, the env value when `apiKeyEnv` is set, and errors on an empty named env
  var; STT CRUD reuses the covered `upsert/delete/find` helpers.
- **Frontend:** no JS test runner — `cd frontend && npm run build` + a manual test
  plan: add a Local-whisper STT profile in the new tab; on an imported audio cell,
  pick the model and Transcribe → an N4L draft appears with the text; error on a
  bad endpoint; no-profile disables Transcribe.
- Full build: `go test ./...` and `make build`.

## Success criteria

- STT profiles are configured in a Speech-to-text tab that mirrors the LLM setup
  (add/edit/delete/active, presets), with keyless local endpoints supported.
- Transcribe on an audio cell uploads the stored file to the selected endpoint and
  inserts the transcript as an editable N4L draft — working on WSL (no local decode).
- No new Go/npm dependencies; no CGo; audio bytes are read from `media/` and
  uploaded, never stored in `cells_json`.

## Dependencies

- **New:** none. `internal/stt` is standard-library (`mime/multipart`,
  `net/http`, `encoding/json`), reusing the shared `httpClient`. Frontend adds no
  dependency.
- **Reused:** the `LLMProfile`/`LLMConfig` types + profile helpers; `loadConfig`/
  `saveConfig`; the ⚙ Models settings component; Slice-5 `mediaDir()`; the 6a
  `onInsert` → N4L-draft path; Phase-1 audio cell + `media/` store.
