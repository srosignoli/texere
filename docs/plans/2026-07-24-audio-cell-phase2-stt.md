# Audio cell — Phase 2 (STT config + Transcribe → N4L draft) — Implementation Plan

**Goal:** Transcribe an audio cell's recording via a configurable OpenAI-compatible `/audio/transcriptions` endpoint (local or cloud) and insert the text as an N4L draft.

**Architecture:** A pure-Go `internal/stt` provider does a multipart POST. STT reuses the `LLMProfile`/`LLMConfig` types via a new `AppConfig.STT`; STT bindings mirror the LLM ones; `resolveKeyOptional` allows keyless local endpoints. The ⚙ Models modal gains an **LLM Providers | Speech-to-text** tab. `AudioCell` gains a Transcribe row → `onInsert` → N4L draft.

**Tech Stack:** Go stdlib (`mime/multipart`, `net/http`, `encoding/json`); Svelte 3.49; reuses config helpers, `mediaDir()`, the 6a `onInsert` path. No new deps, no CGo.

## Global Constraints

- No new npm/Go dependencies; no CGo.
- STT reuses `LLMProfile`/`LLMConfig` + `findProfile`/`upsertProfile`/`deleteProfile`; `AppConfig` gains `STT LLMConfig \`json:"stt"\``.
- STT `Kind` is always `openai-compatible` (bindings force it); the STT editor hides the Kind selector; **no Test button** for STT.
- **API key optional for STT** — `resolveKeyOptional` returns `""` (no error) for a keyless profile; `stt.Transcribe` sends `Authorization` only when a key is set.
- Audio bytes are read from `media/` and uploaded — never stored in `cells_json`.
- Frontend has no JS test runner; frontend tasks verify with `cd frontend && npm run build` + a manual test plan. Go uses `go test ./...`.
- Non-streaming: one request returns the full transcript.

---

## File Structure

- `internal/stt/transcribe.go` — CREATE: `Transcribe(...)` + client.
- `internal/stt/transcribe_test.go` — CREATE.
- `app.go` — MODIFY: add `STT LLMConfig` to `AppConfig`.
- `llm_config.go` — MODIFY: add `resolveKeyOptional`.
- `llm_config_test.go` — CREATE (or append): `resolveKeyOptional` tests.
- `stt_methods.go` — CREATE: STT CRUD + `TranscribeAudio` bindings.
- `frontend/src/components/LLMSettings.svelte` — MODIFY: LLM/STT tabs.
- `frontend/src/components/notebook/AudioCell.svelte` — MODIFY: Transcribe row + `onInsert`.
- `docs/specs/2026-07-24-audio-cell-phase2-manual-test-plan.md` — CREATE (Task 5).

---

## Task 1: `internal/stt` — transcription provider (TDD)

**Files:**
- Create: `internal/stt/transcribe.go`, `internal/stt/transcribe_test.go`

**Interfaces:**
- Produces: `stt.Transcribe(ctx, baseURL, apiKey, model string, audio []byte, filename string) (string, error)`. Consumed by `TranscribeAudio` (Task 2).

- [ ] **Step 1: Write the failing tests**

Create `internal/stt/transcribe_test.go`:

```go
package stt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscribeJSON(t *testing.T) {
	var gotAuth, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = r.ParseMultipartForm(1 << 20)
		gotModel = r.FormValue("model")
		if f, _, err := r.FormFile("file"); err == nil {
			f.Close()
		}
		w.Write([]byte(`{"text":"hello world"}`))
	}))
	defer srv.Close()
	got, err := Transcribe(context.Background(), srv.URL, "sk-x", "whisper-1", []byte("AUDIO"), "a.wav")
	if err != nil || got != "hello world" {
		t.Fatalf("got %q err %v", got, err)
	}
	if gotAuth != "Bearer sk-x" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotModel != "whisper-1" {
		t.Errorf("model = %q", gotModel)
	}
}

func TestTranscribePlainTextFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("just plain text"))
	}))
	defer srv.Close()
	got, err := Transcribe(context.Background(), srv.URL, "", "m", []byte("A"), "a.webm")
	if err != nil || got != "just plain text" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestTranscribeNoKeyNoAuthHeader(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		w.Write([]byte(`{"text":"x"}`))
	}))
	defer srv.Close()
	_, _ = Transcribe(context.Background(), srv.URL, "", "m", []byte("A"), "a.wav")
	if hadAuth {
		t.Error("no Authorization header expected when key is empty")
	}
}

func TestTranscribeErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte("bad model"))
	}))
	defer srv.Close()
	if _, err := Transcribe(context.Background(), srv.URL, "", "m", []byte("A"), "a.wav"); err == nil {
		t.Fatal("expected error on 400")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/stt/`
Expected: FAIL (package/`Transcribe` undefined).

- [ ] **Step 3: Implement `transcribe.go`**

Create `internal/stt/transcribe.go`:

```go
// Package stt transcribes audio via an OpenAI-compatible /audio/transcriptions
// endpoint (local or cloud). Pure standard library, no CGo.
package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 120 * time.Second}

// Transcribe uploads audio to <baseURL>/audio/transcriptions and returns the
// transcript. apiKey may be "" (no Authorization header) for local servers.
// filename names the multipart file part; its extension signals the format.
func Transcribe(ctx context.Context, baseURL, apiKey, model string, audio []byte, filename string) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("model", model); err != nil {
		return "", err
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(audio); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	url := strings.TrimRight(baseURL, "/") + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stt: %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &out); err == nil && out.Text != "" {
		return out.Text, nil
	}
	return strings.TrimSpace(string(data)), nil // plain-text response
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/stt/`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add internal/stt/
git commit -m "feat(stt): OpenAI-compatible /audio/transcriptions provider"
```

---

## Task 2: STT config + `resolveKeyOptional` + bindings

**Files:**
- Modify: `app.go` (AppConfig), `llm_config.go` (resolveKeyOptional)
- Create: `llm_config_test.go`, `stt_methods.go`

**Interfaces:**
- Consumes: `stt.Transcribe` (Task 1); `LLMProfile`/`LLMConfig` + helpers.
- Produces: `GetSTTConfig`, `SaveSTTProfile`, `DeleteSTTProfile`, `SetActiveSTTProfile`, `TranscribeAudio(profileId, asset)` bindings, consumed by the frontend (Tasks 3–4). `make build` regenerates the JS bindings.

- [ ] **Step 1: Add `STT` to `AppConfig`**

In `app.go`, add the field to `AppConfig`:

```go
type AppConfig struct {
	CollectionDir string    `json:"collectionDir"`
	RecentArrows  []string  `json:"recentArrows"`
	LLM           LLMConfig `json:"llm"`
	STT           LLMConfig `json:"stt"`
}
```

- [ ] **Step 2: Add `resolveKeyOptional` + test**

Append to `llm_config.go`:

```go
// resolveKeyOptional is like resolveKey but permits a keyless profile: it returns
// "" and no error when neither APIKey nor APIKeyEnv is set. Used by STT, whose
// local endpoints (e.g. a whisper server) need no auth.
func resolveKeyOptional(p LLMProfile) (string, error) {
	if p.APIKeyEnv != "" {
		v := os.Getenv(p.APIKeyEnv)
		if v == "" {
			return "", fmt.Errorf("env var %q is empty", p.APIKeyEnv)
		}
		return v, nil
	}
	return p.APIKey, nil // "" is allowed
}
```

Create `llm_config_test.go`:

```go
package main

import (
	"os"
	"testing"
)

func TestResolveKeyOptional(t *testing.T) {
	if k, err := resolveKeyOptional(LLMProfile{}); err != nil || k != "" {
		t.Errorf("keyless: got %q err %v, want \"\" nil", k, err)
	}
	if k, err := resolveKeyOptional(LLMProfile{APIKey: "sk-1"}); err != nil || k != "sk-1" {
		t.Errorf("literal: got %q err %v", k, err)
	}
	os.Setenv("TEXERE_TEST_STT_KEY", "envval")
	defer os.Unsetenv("TEXERE_TEST_STT_KEY")
	if k, err := resolveKeyOptional(LLMProfile{APIKeyEnv: "TEXERE_TEST_STT_KEY"}); err != nil || k != "envval" {
		t.Errorf("env: got %q err %v", k, err)
	}
	if _, err := resolveKeyOptional(LLMProfile{APIKeyEnv: "TEXERE_TEST_STT_MISSING"}); err == nil {
		t.Error("expected error for empty named env var")
	}
}
```

- [ ] **Step 3: Create `stt_methods.go`**

Create `stt_methods.go`:

```go
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"texere/internal/stt"
)

// GetSTTConfig returns the speech-to-text provider config.
func (a *App) GetSTTConfig() LLMConfig {
	cfg, _ := loadConfig()
	return cfg.STT
}

// SaveSTTProfile inserts or updates an STT profile (kind is always
// openai-compatible) and persists the config.
func (a *App) SaveSTTProfile(p LLMProfile) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	p.Kind = "openai-compatible"
	upsertProfile(&cfg.STT, p)
	return saveConfig(cfg)
}

// DeleteSTTProfile removes an STT profile by ID.
func (a *App) DeleteSTTProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	deleteProfile(&cfg.STT, id)
	return saveConfig(cfg)
}

// SetActiveSTTProfile sets the active default STT profile.
func (a *App) SetActiveSTTProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if _, ok := findProfile(&cfg.STT, id); !ok {
		return errors.New("profile not found")
	}
	cfg.STT.ActiveProfileID = id
	return saveConfig(cfg)
}

// TranscribeAudio reads the stored asset and transcribes it via the given STT
// profile (or the active default when profileID==""), returning the text.
func (a *App) TranscribeAudio(profileID, asset string) (string, error) {
	cfg, err := loadConfig()
	if err != nil {
		return "", err
	}
	id := profileID
	if id == "" {
		id = cfg.STT.ActiveProfileID
	}
	p, ok := findProfile(&cfg.STT, id)
	if !ok {
		return "", errors.New("no speech-to-text profile configured")
	}
	key, err := resolveKeyOptional(p)
	if err != nil {
		return "", err
	}
	dir := a.mediaDir()
	if dir == "" {
		return "", errors.New("no collection open")
	}
	name := filepath.Base(asset) // guard against traversal
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	return stt.Transcribe(context.Background(), p.BaseURL, key, p.Model, data, name)
}
```

- [ ] **Step 4: Test + build (regenerates bindings)**

Run: `go test ./...`
Expected: PASS (new config test + stt tests + existing suite).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`; regenerates `frontend/wailsjs/go/main/App.{js,d.ts}` with the STT bindings.

- [ ] **Step 5: Confirm bindings**

Run: `grep -nE "GetSTTConfig|SaveSTTProfile|DeleteSTTProfile|SetActiveSTTProfile|TranscribeAudio" frontend/wailsjs/go/main/App.d.ts`
Expected: all five declared.

- [ ] **Step 6: Commit**

```bash
git add app.go llm_config.go llm_config_test.go stt_methods.go frontend/wailsjs/
git commit -m "feat(stt): STT profile config + TranscribeAudio binding"
```

---

## Task 3: Settings UI — LLM / Speech-to-text tabs

**Files:**
- Modify: `frontend/src/components/LLMSettings.svelte`

**Interfaces:**
- Consumes: the STT CRUD bindings (Task 2).

- [ ] **Step 1: Replace the `<script>` block**

Replace the entire `<script>…</script>` of `LLMSettings.svelte` with:

```svelte
<script>
  import { onMount } from 'svelte'
  import {
    GetLLMConfig, SaveLLMProfile, DeleteLLMProfile, SetActiveProfile, TestLLMProfile,
    GetSTTConfig, SaveSTTProfile, DeleteSTTProfile, SetActiveSTTProfile,
  } from '../../wailsjs/go/main/App'

  export let onClose = () => {}

  const LLM_PRESETS = [
    { name: 'Anthropic',  kind: 'anthropic',         baseUrl: 'https://api.anthropic.com',                          model: 'claude-opus-4-8' },
    { name: 'OpenAI',     kind: 'openai-compatible',  baseUrl: 'https://api.openai.com/v1',                          model: 'gpt-4o' },
    { name: 'Google',     kind: 'openai-compatible',  baseUrl: 'https://generativelanguage.googleapis.com/v1beta/openai', model: 'gemini-2.0-flash' },
    { name: 'OpenRouter', kind: 'openai-compatible',  baseUrl: 'https://openrouter.ai/api/v1',                       model: '' },
    { name: 'Local (Ollama)', kind: 'openai-compatible', baseUrl: 'http://localhost:11434/v1',                       model: 'llama3.1' },
  ]
  const STT_PRESETS = [
    { name: 'OpenAI Whisper', kind: 'openai-compatible', baseUrl: 'https://api.openai.com/v1', model: 'whisper-1' },
    { name: 'Local whisper',  kind: 'openai-compatible', baseUrl: 'http://localhost:8080/v1',  model: 'whisper-1' },
  ]

  // Per-tab binding + option set. Called with the current tab so async actions
  // never read a stale reactive value.
  function apiFor(t) {
    return t === 'stt'
      ? { get: GetSTTConfig, save: SaveSTTProfile, del: DeleteSTTProfile, setActive: SetActiveSTTProfile, test: null, presets: STT_PRESETS, hasKind: false }
      : { get: GetLLMConfig, save: SaveLLMProfile, del: DeleteLLMProfile, setActive: SetActiveProfile, test: TestLLMProfile, presets: LLM_PRESETS, hasKind: true }
  }

  let tab = 'llm'
  let profiles = []
  let activeId = ''
  let editing = null
  let testMsg = {}
  let error = ''

  onMount(load)
  async function load() {
    error = ''
    try { const c = await apiFor(tab).get(); profiles = c.profiles ?? []; activeId = c.activeProfileId ?? '' }
    catch (e) { error = String(e?.message ?? e) }
  }
  function setTab(t) { tab = t; editing = null; testMsg = {}; load() }

  function addFrom(preset) {
    editing = { id: `p-${Date.now()}`, name: preset.name, kind: preset.kind, baseUrl: preset.baseUrl, model: preset.model, apiKey: '', apiKeyEnv: '' }
  }
  function edit(p) { editing = { ...p } }
  async function save() {
    error = ''
    try { await apiFor(tab).save(editing); editing = null; await load() }
    catch (e) { error = String(e?.message ?? e) }
  }
  async function del(id) { error = ''; try { await apiFor(tab).del(id); await load() } catch (e) { error = String(e?.message ?? e) } }
  async function setActive(id) { error = ''; try { await apiFor(tab).setActive(id); activeId = id } catch (e) { error = String(e?.message ?? e) } }
  async function test(id) {
    const fn = apiFor(tab).test
    if (!fn) return
    testMsg = { ...testMsg, [id]: 'testing…' }
    try { await fn(id); testMsg = { ...testMsg, [id]: '✓ ok' } }
    catch (e) { testMsg = { ...testMsg, [id]: '✕ ' + String(e?.message ?? e) } }
  }
</script>
```

- [ ] **Step 2: Replace the markup (between `</script>` and `<style>`)**

Replace the markup block with (note the tab row, the `{#if apiFor(tab).hasKind}` Kind selector, the `{#if apiFor(tab).test}` Test button, and `apiFor(tab).presets`):

```svelte
<div class="ov-backdrop" on:click={onClose}></div>
<div class="ov-modal">
  <div class="ov-head"><span>Models</span><button on:click={onClose}>✕</button></div>
  <div class="ov-tabs">
    <button class:sel={tab === 'llm'} on:click={() => setTab('llm')}>LLM Providers</button>
    <button class:sel={tab === 'stt'} on:click={() => setTab('stt')}>Speech-to-text</button>
  </div>
  {#if error}<div class="ov-error">{error}</div>{/if}

  {#if editing}
    <div class="ov-form">
      <label>Name<input bind:value={editing.name} /></label>
      {#if apiFor(tab).hasKind}
        <label>Kind
          <select bind:value={editing.kind}>
            <option value="openai-compatible">openai-compatible</option>
            <option value="anthropic">anthropic</option>
          </select>
        </label>
      {/if}
      <label>Base URL<input bind:value={editing.baseUrl} /></label>
      <label>Model<input bind:value={editing.model} placeholder={tab === 'stt' ? 'e.g. whisper-1' : 'e.g. claude-opus-4-8'} /></label>
      <label>API key<input type="password" bind:value={editing.apiKey} placeholder={tab === 'stt' ? 'optional (local servers)' : 'leave blank to use an env var'} /></label>
      <label>…or env var name<input bind:value={editing.apiKeyEnv} placeholder="e.g. OPENAI_API_KEY" /></label>
      <div class="ov-actions"><button class="primary" on:click={save}>Save</button><button on:click={() => (editing = null)}>Cancel</button></div>
    </div>
  {:else}
    <ul class="ov-list">
      {#each profiles as p (p.id)}
        <li>
          <label class="ov-active"><input type="radio" checked={p.id === activeId} on:change={() => setActive(p.id)} /></label>
          <div class="ov-meta"><b>{p.name}</b><span>{p.kind} · {p.model || '(no model)'}</span></div>
          {#if apiFor(tab).test}<button on:click={() => test(p.id)}>Test</button>{/if}
          <button on:click={() => edit(p)}>Edit</button>
          <button on:click={() => del(p.id)}>Delete</button>
          {#if testMsg[p.id]}<span class="ov-test">{testMsg[p.id]}</span>{/if}
        </li>
      {/each}
    </ul>
    <div class="ov-presets">
      <span>Add:</span>
      {#each apiFor(tab).presets as preset}<button on:click={() => addFrom(preset)}>{preset.name}</button>{/each}
    </div>
  {/if}
</div>
```

- [ ] **Step 3: Add the tab style**

In the `<style>` block, add:

```css
  .ov-tabs { display: flex; gap: 6px; padding: 8px 14px 0; }
  .ov-tabs button { background: none; border: none; border-bottom: 2px solid transparent; color: #6b7a8a; cursor: pointer; padding: 4px 6px; font-size: 12px; }
  .ov-tabs button.sel { color: #7dd3fc; border-bottom-color: #7dd3fc; }
```

- [ ] **Step 4: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/LLMSettings.svelte
git commit -m "feat(settings): Speech-to-text tab in the Models panel"
```

---

## Task 4: Audio cell — Transcribe row

**Files:**
- Modify: `frontend/src/components/notebook/AudioCell.svelte`

**Interfaces:**
- Consumes: `GetSTTConfig`, `TranscribeAudio` (Task 2); the `onInsert` prop (already passed by `NotebookPane`).

- [ ] **Step 1: Imports, prop, and state**

Add to the imports in `AudioCell.svelte`:

```js
  import { onMount } from 'svelte'
  import { ImportAudio, SaveRecording, GetSTTConfig, TranscribeAudio } from '../../../wailsjs/go/main/App'
```

(Merge with the existing `wailsjs/go/main/App` import; `onMount` alongside the existing `onDestroy` import.)

Add the prop near the other `export let`s:

```js
  export let onInsert = () => {}
```

Add state near the other `let`s:

```js
  let sttProfiles = []
  let transcribing = false
  let transErr = ''
```

Load profiles on mount (add alongside the existing script logic, before `onDestroy`):

```js
  onMount(async () => { try { const c = await GetSTTConfig(); sttProfiles = c.profiles ?? [] } catch { /* not configured */ } })

  function setTransProfile(id) { cell.transcribeProfileId = id || null; cell = cell; onChange(cell) }

  async function transcribe() {
    if (transcribing) return
    transcribing = true
    transErr = ''
    try {
      const text = await TranscribeAudio(cell.transcribeProfileId || '', cell.asset)
      if (text && text.trim()) onInsert(text)
    } catch (e) { transErr = String(e?.message ?? e) }
    transcribing = false
  }
```

- [ ] **Step 2: Add the Transcribe row in the loaded state**

In the markup, inside the `{#if cell.asset}` branch, after the `<audio>`/missing block (still within the `cell.asset` branch, only when `!readOnly`), add:

```svelte
    {#if !readOnly}
      <div class="au-trans">
        <select value={cell.transcribeProfileId || ''} on:change={(e) => setTransProfile(e.target.value)} disabled={transcribing || !sttProfiles.length} title="Speech-to-text model">
          <option value="">active default</option>
          {#each sttProfiles as p}<option value={p.id}>{p.name}</option>{/each}
        </select>
        <button on:click={transcribe} disabled={transcribing || !sttProfiles.length}>{transcribing ? 'Transcribing…' : 'Transcribe'}</button>
        {#if !sttProfiles.length}<span class="au-hint">Configure a model in ⚙ Models → Speech-to-text</span>{/if}
        {#if transErr}<span class="au-err">{transErr}</span>{/if}
      </div>
    {/if}
```

- [ ] **Step 3: Add styles**

In the `<style>` block, add:

```css
  .au-trans { display: flex; align-items: center; gap: 6px; padding: 6px 8px; border-top: 1px solid #1c2a3a; flex-wrap: wrap; }
  .au-trans select, .au-trans button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; font-size: 12px; cursor: pointer; }
  .au-trans button:disabled, .au-trans select:disabled { opacity: .5; cursor: default; }
  .au-hint { color: #5a7a9a; font-size: 11px; }
```

- [ ] **Step 4: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 5: Manual smoke**

`PATH="$PATH:$(go env GOPATH)/bin" make build` then `./build/bin/texere`: add an STT profile (Local whisper or OpenAI Whisper) in ⚙ Models → Speech-to-text; on an imported audio cell, pick the model and Transcribe → confirm an N4L draft appears with the text (needs a reachable STT endpoint).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/notebook/AudioCell.svelte
git commit -m "feat(notebook): Transcribe audio cell to an N4L draft (STT)"
```

---

## Task 5: Manual test plan + full verification

**Files:**
- Create: `docs/specs/2026-07-24-audio-cell-phase2-manual-test-plan.md`

- [ ] **Step 1: Write the manual test plan**

Create the file:

```markdown
# Audio cell — Phase 2 (STT) Manual Test Plan

Prereqs: open an extract; have an OpenAI-compatible transcription endpoint
reachable (OpenAI `whisper-1`, or a local whisper server on `http://localhost:8080/v1`).

## TC-1 Configure an STT profile
1. Open ⚙ Models; click the "Speech-to-text" tab.
   - EXPECT: an empty STT list with presets "OpenAI Whisper" and "Local whisper"; no Kind selector in the editor; no Test button; the key field says "optional".
2. Add a preset, fill the key if needed, Save; mark it active.
   - EXPECT: it appears in the STT list and persists across reopening the panel.

## TC-2 Transcribe
1. On an audio cell that has a clip, pick the STT model and click Transcribe.
   - EXPECT: a "Transcribing…" state, then a new N4L draft cell appears below with the transcript text.

## TC-3 Model picker + keyless local
1. Add a keyless "Local whisper" profile; Transcribe with it.
   - EXPECT: works with no API key (no auth sent); the request hits the local endpoint.

## TC-4 Errors
1. Transcribe with a bad Base URL / model.
   - EXPECT: an inline error message; no draft inserted.
2. With no STT profiles configured, the Transcribe button is disabled with a hint.

## TC-5 LLM tab unaffected (regression)
1. Switch to the "LLM Providers" tab.
   - EXPECT: existing LLM profiles, the Kind selector, and the Test button all still work.
```

- [ ] **Step 2: Backend suite + full build**

Run: `go test ./...`
Expected: all PASS (includes `internal/stt` + `resolveKeyOptional`).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`.

- [ ] **Step 3: Execute the manual test plan**

Walk TC-1 … TC-5 in `./build/bin/texere`.

- [ ] **Step 4: Commit**

```bash
git add docs/specs/2026-07-24-audio-cell-phase2-manual-test-plan.md
git commit -m "docs(test-plan): Audio cell Phase 2 (STT) manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** `internal/stt` provider with bearer-optional + JSON/plain-text response (T1); `AppConfig.STT`, `resolveKeyOptional`, STT CRUD + `TranscribeAudio` reusing `mediaDir()` + `filepath.Base` traversal guard (T2); ⚙ Models LLM/STT tabs, Kind hidden + no Test for STT, key-optional label, STT presets (T3); audio-cell Transcribe row → `onInsert` N4L draft (T4); manual plan + full verification (T5). Out of scope (language/prompt params, streaming, diarization, Test button, Anthropic STT) omitted.
- **Placeholder scan:** no TBD/TODO; every code step complete; Go steps follow TDD.
- **Type consistency:** STT reuses `LLMProfile`/`LLMConfig`; `SaveSTTProfile` forces `kind='openai-compatible'`; `TranscribeAudio(profileId, asset)` → `stt.Transcribe(ctx, baseURL, key, model, data, name)`; frontend `TranscribeAudio(cell.transcribeProfileId||'', cell.asset)` → `onInsert(text)`; `GetSTTConfig().profiles` feeds the dropdown; `apiFor(tab)` swaps binding sets so the settings markup is shared.
- **Compile-sequencing:** the STT JS bindings are generated by T2's `make build` before T3/T4 import them; T4's `onInsert` is already wired from 6a. Existing LLM settings/behavior preserved (regression checked in TC-5).
```
