# Processing Notebook — Slice 2 (LLM Cell) Implementation Plan

**Goal:** Add a provider-agnostic, streaming, multi-turn LLM cell (ask + tutor modes) to the processing notebook, configured via global named profiles (Anthropic / OpenAI / Google / OpenRouter / local), and retire the Tutor tab.

**Architecture:** A new pure-Go `internal/llm` package defines a `Provider` interface with two raw-`net/http` adapters — a universal OpenAI-compatible client and a native Anthropic `/v1/messages` client — behind one SSE reader. Global LLM profiles live in `AppConfig` (`config.json`). Wails methods do config CRUD, a connection test, and a streaming turn that emits `llm:delta`/`llm:done`/`llm:error` events, which the Svelte `LLMCell` consumes.

**Tech Stack:** Go 1.22 (stdlib `net/http`, `encoding/json`, `bufio`, `context`), Svelte 3.49, Wails v2.12. No new third-party dependencies.

## Global Constraints

- Go: standard `gofmt`; imports grouped stdlib / external / internal.
- **No new third-party dependencies** (Go or npm). The Anthropic adapter uses native raw `net/http` against `/v1/messages` — NOT the Anthropic Go SDK and NOT an OpenAI-compatible shim. This is the approved design (uniform `Provider` interface, zero new deps); do not "upgrade" it to the SDK.
- Default Anthropic model string is `claude-opus-4-8`; **never append a date suffix** to model IDs. Anthropic current models reject `temperature`/`top_p`/`top_k`/`budget_tokens` (400) — the adapters send plain streamed chat with no sampling or thinking params.
- Provider profiles are global (`config.json` under `os.UserConfigDir()/texere/`), one active + optional per-cell override. Keys are plaintext OR an env-var reference; the config file is written `0600`.
- Frontend has no JS test runner; frontend tasks verify with `cd frontend && npm run build` plus a manual test plan.
- Notebooks/cells persist via Slice 1's autosave; the LLM cell stores its conversation in `cells_json`.
- Run `wails` via `PATH="$PATH:$(go env GOPATH)/bin"` (Makefile convention); `wails generate module` regenerates bindings.

## Design note carried from the spec

The `claude-api` skill's default is the official Anthropic SDK; the approved spec deliberately chose native raw HTTP for a uniform provider-agnostic interface with no new dependencies. The Anthropic wire facts used below (endpoint, headers, `anthropic-version: 2023-06-01`, `content_block_delta`/`text_delta` streaming events, `message_stop`, required `max_tokens`, `claude-opus-4-8`) come from that skill and are current as of this plan.

---

## File Structure

**Backend (Go)**
- `internal/llm/llm.go` — create: `Message`, `Request` types; `Provider` interface; `New(kind, baseURL, apiKey)` factory.
- `internal/llm/sse.go` — create: `scanSSEData` reader.
- `internal/llm/sse_test.go` — create.
- `internal/llm/openai.go` — create: OpenAI-compatible adapter.
- `internal/llm/openai_test.go` — create.
- `internal/llm/anthropic.go` — create: native Anthropic adapter.
- `internal/llm/anthropic_test.go` — create.
- `llm_config.go` — create (package main): `LLMProfile`/`LLMConfig` types; pure helpers `resolveKey`, `upsertProfile`, `deleteProfile`.
- `llm_config_test.go` — create.
- `app.go` — modify: add `LLM LLMConfig` to `AppConfig`; add `llmStreams`/`llmMu` fields; init in `startup`.
- `methods.go` — modify: `saveConfig` writes `0600`.
- `llm_methods.go` — create (package main): config CRUD, `TestLLMProfile`, `StartLLMTurn`, `CancelLLMTurn`.

**Frontend (Svelte/JS)**
- `frontend/src/lib/llm.js` — create: mode prompts, context assembly, streaming client.
- `frontend/src/components/LLMSettings.svelte` — create.
- `frontend/src/components/notebook/LLMCell.svelte` — create.
- `frontend/src/lib/notebook.js` — modify: register `llm` in `cellRegistry`; add `newLLMCell`.
- `frontend/src/components/notebook/NotebookPane.svelte` — modify: pass context + sibling cells to cells; add "+ LLM cell"; add a "⚙ Models" gear opening `LLMSettings`.
- `frontend/src/components/BottomBand.svelte` — modify: remove the `tutor` tab + `TutorTab` import/pane.
- `frontend/src/components/TutorTab.svelte` — delete.
- `docs/specs/2026-07-22-processing-notebook-slice2-manual-test-plan.md` — create.

---

## Phase 2a — Provider infrastructure

## Task 1: SSE reader + LLM types + Provider interface

**Files:**
- Create: `internal/llm/llm.go`, `internal/llm/sse.go`
- Test: `internal/llm/sse_test.go`

**Interfaces:**
- Produces:
  - `type Message struct { Role string; Content string }` (JSON tags `role`,`content`)
  - `type Request struct { Model, System string; Messages []Message; MaxTokens int }`
  - `type Provider interface { Stream(ctx context.Context, req Request, onDelta func(string) error) error }`
  - `func New(kind, baseURL, apiKey string) (Provider, error)` — dispatch on `kind` ("openai-compatible" | "anthropic"); returns error for unknown kinds. (Adapters added in Tasks 2–3; for Task 1 `New` returns `errors.New("llm: unknown provider kind "+kind)` for everything.)
  - `func scanSSEData(r io.Reader, onData func(data string) (stop bool, err error)) error` — reads an SSE body line by line; for each line beginning with `data:` (after trimming one optional leading space) it calls `onData` with the payload; blank lines and non-`data:` lines (e.g. `event:`) are ignored; stops when `onData` returns `stop==true` or at EOF.

- [ ] **Step 1: Write the failing test**

Create `internal/llm/sse_test.go`:

```go
package llm

import (
	"strings"
	"testing"
)

func TestScanSSEData(t *testing.T) {
	body := "event: start\n" +
		"data: {\"a\":1}\n" +
		"\n" +
		"data: [DONE]\n" +
		"data: after-done\n"
	var got []string
	err := scanSSEData(strings.NewReader(body), func(data string) (bool, error) {
		got = append(got, data)
		return data == "[DONE]", nil
	})
	if err != nil {
		t.Fatalf("scanSSEData: %v", err)
	}
	if len(got) != 2 || got[0] != `{"a":1}` || got[1] != "[DONE]" {
		t.Fatalf("unexpected payloads: %#v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/ -run TestScanSSEData -v`
Expected: FAIL — package/`scanSSEData` undefined.

- [ ] **Step 3: Implement**

Create `internal/llm/llm.go`:

```go
// Package llm is a provider-agnostic streaming chat client. It speaks two wire
// formats behind one Provider interface: the OpenAI-compatible
// /chat/completions API (OpenAI, OpenRouter, Google's compat endpoint, local
// servers) and the native Anthropic /v1/messages API.
package llm

import (
	"context"
	"errors"
)

// Message is one turn in a conversation.
type Message struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// Request is a single streamed completion request.
type Request struct {
	Model     string
	System    string // system prompt (mode framing + assembled context)
	Messages  []Message
	MaxTokens int
}

// Provider streams an assistant response, invoking onDelta for each text chunk.
// It returns when the stream ends or ctx is cancelled; errors carry the
// provider's message where available.
type Provider interface {
	Stream(ctx context.Context, req Request, onDelta func(string) error) error
}

// New builds a Provider for the given kind. baseURL and apiKey are the resolved
// endpoint and secret for the profile.
func New(kind, baseURL, apiKey string) (Provider, error) {
	switch kind {
	case "openai-compatible":
		return &openaiProvider{baseURL: baseURL, apiKey: apiKey}, nil
	case "anthropic":
		return &anthropicProvider{baseURL: baseURL, apiKey: apiKey}, nil
	default:
		return nil, errors.New("llm: unknown provider kind " + kind)
	}
}
```

Create `internal/llm/sse.go`:

```go
package llm

import (
	"bufio"
	"io"
	"strings"
)

// scanSSEData reads an SSE stream and calls onData with the payload of each
// "data:" line. Non-data lines (event:, id:, comments) and blank lines are
// skipped. It stops when onData returns stop==true, at EOF, or on a read error.
func scanSSEData(r io.Reader, onData func(data string) (stop bool, err error)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // allow long data lines
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimPrefix(data, " ")
		stop, err := onData(data)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return sc.Err()
}
```

> Note: `New` references `openaiProvider`/`anthropicProvider` which are added in Tasks 2–3. For this task to compile, temporarily add empty stubs at the bottom of `llm.go`:
> ```go
> type openaiProvider struct{ baseURL, apiKey string }
> func (p *openaiProvider) Stream(ctx context.Context, req Request, onDelta func(string) error) error { return errors.New("not implemented") }
> type anthropicProvider struct{ baseURL, apiKey string }
> func (p *anthropicProvider) Stream(ctx context.Context, req Request, onDelta func(string) error) error { return errors.New("not implemented") }
> ```
> Tasks 2 and 3 move these to `openai.go`/`anthropic.go` and implement them. Delete the stubs from `llm.go` when you do.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llm/ -run TestScanSSEData -v`
Expected: PASS. Also `go build ./...` — no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/llm.go internal/llm/sse.go internal/llm/sse_test.go
git commit -m "feat(llm): Provider interface + SSE reader"
```

---

## Task 2: OpenAI-compatible adapter

**Files:**
- Create: `internal/llm/openai.go` (move the `openaiProvider` stub here)
- Modify: `internal/llm/llm.go` (remove the `openaiProvider` stub)
- Test: `internal/llm/openai_test.go`

**Interfaces:**
- Consumes: `scanSSEData`, `Request`, `Message`.
- Produces: `openaiProvider.Stream` — `POST {baseURL}/chat/completions`, `Authorization: Bearer <key>`, JSON body `{model, messages, stream:true, max_tokens}` where `messages` is the system message (if any) followed by `req.Messages`; parses SSE `data:` lines as `{"choices":[{"delta":{"content":"..."}}]}`, calls `onDelta` for each non-empty content, stops on `[DONE]`; on non-200 returns the response body text as the error.

- [ ] **Step 1: Write the failing test**

Create `internal/llm/openai_test.go`:

```go
package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth header = %q", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := &openaiProvider{baseURL: srv.URL, apiKey: "k"}
	var sb strings.Builder
	err := p.Stream(context.Background(),
		Request{Model: "m", System: "sys", Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 16},
		func(s string) error { sb.WriteString(s); return nil })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if sb.String() != "Hello" {
		t.Fatalf("got %q, want Hello", sb.String())
	}
}

func TestOpenAIStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()
	p := &openaiProvider{baseURL: srv.URL, apiKey: "k"}
	err := p.Stream(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8}, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("want error containing 'bad key', got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/ -run TestOpenAI -v`
Expected: FAIL — stub returns "not implemented".

- [ ] **Step 3: Implement**

Remove the `openaiProvider` stub from `llm.go`. Create `internal/llm/openai.go`:

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type openaiProvider struct {
	baseURL string
	apiKey  string
}

func (p *openaiProvider) Stream(ctx context.Context, req Request, onDelta func(string) error) error {
	msgs := make([]Message, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, Message{Role: "system", Content: req.System})
	}
	msgs = append(msgs, req.Messages...)

	body, _ := json.Marshal(map[string]any{
		"model":      req.Model,
		"messages":   msgs,
		"stream":     true,
		"max_tokens": req.MaxTokens,
	})
	url := strings.TrimRight(p.baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("llm: openai %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	return scanSSEData(resp.Body, func(data string) (bool, error) {
		if data == "[DONE]" {
			return true, nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return false, nil // ignore keep-alives / non-JSON lines
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			if err := onDelta(chunk.Choices[0].Delta.Content); err != nil {
				return true, err
			}
		}
		return false, nil
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llm/ -run TestOpenAI -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/openai.go internal/llm/openai_test.go internal/llm/llm.go
git commit -m "feat(llm): OpenAI-compatible streaming adapter"
```

---

## Task 3: Native Anthropic adapter

**Files:**
- Create: `internal/llm/anthropic.go` (move the `anthropicProvider` stub here)
- Modify: `internal/llm/llm.go` (remove the `anthropicProvider` stub)
- Test: `internal/llm/anthropic_test.go`

**Interfaces:**
- Consumes: `scanSSEData`, `Request`, `Message`.
- Produces: `anthropicProvider.Stream` — `POST {baseURL}/v1/messages`, headers `x-api-key: <key>`, `anthropic-version: 2023-06-01`, `content-type: application/json`; JSON body `{model, max_tokens, messages, stream:true}` plus `system` when non-empty (`messages` is `req.Messages` unchanged — Anthropic messages are user/assistant only); parses SSE `data:` lines as Anthropic events, emitting text from `content_block_delta`→`text_delta`, stopping on `message_stop`, and returning an error on an `error` event; on non-200 returns the response body text.

- [ ] **Step 1: Write the failing test**

Create `internal/llm/anthropic_test.go`:

```go
package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "k" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n\n")
		io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"lo\"}}\n\n")
		io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	p := &anthropicProvider{baseURL: srv.URL, apiKey: "k"}
	var sb strings.Builder
	err := p.Stream(context.Background(),
		Request{Model: "claude-opus-4-8", System: "sys", Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 16},
		func(s string) error { sb.WriteString(s); return nil })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if sb.String() != "Hello" {
		t.Fatalf("got %q, want Hello", sb.String())
	}
}

func TestAnthropicStreamErrorEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"overloaded\"}}\n\n")
	}))
	defer srv.Close()
	p := &anthropicProvider{baseURL: srv.URL, apiKey: "k"}
	err := p.Stream(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8}, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "overloaded") {
		t.Fatalf("want error containing 'overloaded', got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/ -run TestAnthropic -v`
Expected: FAIL — stub returns "not implemented".

- [ ] **Step 3: Implement**

Remove the `anthropicProvider` stub from `llm.go`. Create `internal/llm/anthropic.go`:

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type anthropicProvider struct {
	baseURL string
	apiKey  string
}

func (p *anthropicProvider) Stream(ctx context.Context, req Request, onDelta func(string) error) error {
	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   req.Messages, // user/assistant only
		"stream":     true,
	}
	if req.System != "" {
		payload["system"] = req.System
	}
	body, _ := json.Marshal(payload)

	url := strings.TrimRight(p.baseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("llm: anthropic %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	return scanSSEData(resp.Body, func(data string) (bool, error) {
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false, nil
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				if err := onDelta(ev.Delta.Text); err != nil {
					return true, err
				}
			}
		case "message_stop":
			return true, nil
		case "error":
			return true, fmt.Errorf("llm: anthropic: %s", ev.Error.Message)
		}
		return false, nil
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llm/ -run TestAnthropic -v`
Expected: PASS. Then `go test ./internal/llm/ -v` — all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/anthropic.go internal/llm/anthropic_test.go internal/llm/llm.go
git commit -m "feat(llm): native Anthropic /v1/messages streaming adapter"
```

---

## Task 4: LLM config types + pure helpers + 0600 config write

**Files:**
- Create: `llm_config.go` (package main)
- Modify: `app.go` (add `LLM LLMConfig` to `AppConfig`), `methods.go` (`saveConfig` → `0600`)
- Test: `llm_config_test.go`

**Interfaces:**
- Produces:
  - `type LLMProfile struct { ID, Name, Kind, BaseURL, Model, APIKey, APIKeyEnv string }` (JSON tags: `id,name,kind,baseUrl,model,apiKey,apiKeyEnv`; `apiKey`/`apiKeyEnv` `omitempty`).
  - `type LLMConfig struct { Profiles []LLMProfile; ActiveProfileID string }` (JSON: `profiles`, `activeProfileId`).
  - `func resolveKey(p LLMProfile) (string, error)`
  - `func upsertProfile(cfg *LLMConfig, p LLMProfile)` — replace by ID or append; if it was the first profile, set `ActiveProfileID`.
  - `func deleteProfile(cfg *LLMConfig, id string)` — remove by ID; if it was active, set active to the first remaining (or "").
  - `func findProfile(cfg *LLMConfig, id string) (LLMProfile, bool)`
- Consumes: none.

- [ ] **Step 1: Write the failing test**

Create `llm_config_test.go`:

```go
package main

import (
	"os"
	"testing"
)

func TestResolveKey(t *testing.T) {
	if _, err := resolveKey(LLMProfile{}); err == nil {
		t.Fatal("empty profile should error")
	}
	if k, err := resolveKey(LLMProfile{APIKey: "lit"}); err != nil || k != "lit" {
		t.Fatalf("literal: %q %v", k, err)
	}
	os.Setenv("TEXERE_TEST_KEY", "envval")
	defer os.Unsetenv("TEXERE_TEST_KEY")
	if k, err := resolveKey(LLMProfile{APIKeyEnv: "TEXERE_TEST_KEY", APIKey: "ignored"}); err != nil || k != "envval" {
		t.Fatalf("env precedence: %q %v", k, err)
	}
	if _, err := resolveKey(LLMProfile{APIKeyEnv: "TEXERE_MISSING_ENV"}); err == nil {
		t.Fatal("missing env should error")
	}
}

func TestProfileCRUD(t *testing.T) {
	cfg := &LLMConfig{}
	upsertProfile(cfg, LLMProfile{ID: "a", Name: "A"})
	if len(cfg.Profiles) != 1 || cfg.ActiveProfileID != "a" {
		t.Fatalf("first upsert should set active: %+v", cfg)
	}
	upsertProfile(cfg, LLMProfile{ID: "b", Name: "B"})
	upsertProfile(cfg, LLMProfile{ID: "a", Name: "A2"}) // update, not append
	if len(cfg.Profiles) != 2 {
		t.Fatalf("update should not append: %+v", cfg.Profiles)
	}
	if p, ok := findProfile(cfg, "a"); !ok || p.Name != "A2" {
		t.Fatalf("update lost: %+v", p)
	}
	deleteProfile(cfg, "a") // active was "a" → falls back to "b"
	if _, ok := findProfile(cfg, "a"); ok {
		t.Fatal("delete failed")
	}
	if cfg.ActiveProfileID != "b" {
		t.Fatalf("active should fall back to b, got %q", cfg.ActiveProfileID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestResolveKey|TestProfileCRUD' -v`
Expected: FAIL — undefined types/functions.

- [ ] **Step 3: Implement**

Create `llm_config.go`:

```go
package main

import (
	"errors"
	"fmt"
	"os"
)

// LLMProfile is one configured provider: a name, a wire "kind", endpoint,
// model, and either a literal API key or the name of an env var holding it.
type LLMProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "openai-compatible" | "anthropic"
	BaseURL   string `json:"baseUrl"`
	Model     string `json:"model"`
	APIKey    string `json:"apiKey,omitempty"`
	APIKeyEnv string `json:"apiKeyEnv,omitempty"`
}

// LLMConfig is the global set of profiles plus the active default.
type LLMConfig struct {
	Profiles        []LLMProfile `json:"profiles"`
	ActiveProfileID string       `json:"activeProfileId"`
}

// resolveKey returns the profile's secret: the env var if APIKeyEnv is set,
// else the literal APIKey. Never logs the value.
func resolveKey(p LLMProfile) (string, error) {
	if p.APIKeyEnv != "" {
		v := os.Getenv(p.APIKeyEnv)
		if v == "" {
			return "", fmt.Errorf("env var %q is empty", p.APIKeyEnv)
		}
		return v, nil
	}
	if p.APIKey == "" {
		return "", errors.New("no API key set for profile")
	}
	return p.APIKey, nil
}

func findProfile(cfg *LLMConfig, id string) (LLMProfile, bool) {
	for _, p := range cfg.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return LLMProfile{}, false
}

// upsertProfile replaces a profile with the same ID, or appends it. The first
// profile added becomes the active default.
func upsertProfile(cfg *LLMConfig, p LLMProfile) {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].ID == p.ID {
			cfg.Profiles[i] = p
			return
		}
	}
	cfg.Profiles = append(cfg.Profiles, p)
	if cfg.ActiveProfileID == "" {
		cfg.ActiveProfileID = p.ID
	}
}

// deleteProfile removes a profile by ID; if it was active, the active default
// falls back to the first remaining profile (or "").
func deleteProfile(cfg *LLMConfig, id string) {
	out := cfg.Profiles[:0]
	for _, p := range cfg.Profiles {
		if p.ID != id {
			out = append(out, p)
		}
	}
	cfg.Profiles = out
	if cfg.ActiveProfileID == id {
		cfg.ActiveProfileID = ""
		if len(cfg.Profiles) > 0 {
			cfg.ActiveProfileID = cfg.Profiles[0].ID
		}
	}
}
```

In `app.go`, add the field to `AppConfig`:

```go
type AppConfig struct {
	CollectionDir string    `json:"collectionDir"`
	RecentArrows  []string  `json:"recentArrows"`
	LLM           LLMConfig `json:"llm"`
}
```

In `methods.go` `saveConfig`, change the file mode from `0o644` to `0o600`:

```go
	if err := os.WriteFile(p, data, 0o600); err != nil {
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run 'TestResolveKey|TestProfileCRUD' -v`
Expected: PASS. Then `go build ./...` — no errors.

- [ ] **Step 5: Commit**

```bash
git add llm_config.go llm_config_test.go app.go methods.go
git commit -m "feat(config): global LLM profiles + key resolution; config.json 0600"
```

---

## Task 5: Wails methods — config CRUD, Test connection, streaming turn

**Files:**
- Create: `llm_methods.go` (package main)
- Modify: `app.go` (add `llmStreams`/`llmMu` fields + init in `startup`)
- Test: `llm_methods_test.go`

**Interfaces:**
- Consumes: `loadConfig`/`saveConfig`, `resolveKey`/`upsert`/`delete`/`findProfile`, `llm.New`, `runtime.EventsEmit`, `a.ctx`.
- Produces:
  - `func (a *App) GetLLMConfig() LLMConfig`
  - `func (a *App) SaveLLMProfile(p LLMProfile) error`
  - `func (a *App) DeleteLLMProfile(id string) error`
  - `func (a *App) SetActiveProfile(id string) error`
  - `func (a *App) TestLLMProfile(id string) error` — resolves the profile, builds a provider, runs a tiny streamed turn ("ping", `max_tokens` 16), returns nil on success or the provider error.
  - `func (a *App) StartLLMTurn(req StartTurnReq) (string, error)` where `type StartTurnReq struct { ProfileID, System string; Messages []llm.Message }` (JSON `profileId`,`system`,`messages`). Streams via a goroutine emitting `llm:delta {streamId,text}`, `llm:done {streamId}`, `llm:error {streamId,message}`. Returns the `streamId`.
  - `func (a *App) CancelLLMTurn(streamID string)`
  - `func newStreamID() string`

- [ ] **Step 1: Write the failing test**

Create `llm_methods_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestNewStreamID(t *testing.T) {
	a, _ := newStreamID(), newStreamID()
	_ = a
	x, y := newStreamID(), newStreamID()
	if x == y {
		t.Fatalf("ids should differ: %q", x)
	}
	if !strings.HasPrefix(x, "llm-") {
		t.Fatalf("unexpected id: %q", x)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestNewStreamID -v`
Expected: FAIL — `undefined: newStreamID`.

- [ ] **Step 3: Implement**

In `app.go`, add fields to `App`:

```go
	llmStreams map[string]context.CancelFunc // active LLM turns, keyed by streamId
	llmMu      sync.Mutex                     // guards llmStreams
```

In `startup`, initialize the map (near the top, after `a.ctx = ctx`):

```go
	a.llmStreams = map[string]context.CancelFunc{}
```

Create `llm_methods.go`:

```go
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/llm"
)

const llmMaxTokens = 4096 // per-turn response cap for the chat cell

func newStreamID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "llm-" + hex.EncodeToString(b)
}

// GetLLMConfig returns the global LLM profile configuration.
func (a *App) GetLLMConfig() LLMConfig {
	cfg, _ := loadConfig()
	return cfg.LLM
}

// SaveLLMProfile inserts or updates a profile and persists the config.
func (a *App) SaveLLMProfile(p LLMProfile) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	upsertProfile(&cfg.LLM, p)
	return saveConfig(cfg)
}

// DeleteLLMProfile removes a profile by ID.
func (a *App) DeleteLLMProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	deleteProfile(&cfg.LLM, id)
	return saveConfig(cfg)
}

// SetActiveProfile sets the active default profile.
func (a *App) SetActiveProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if _, ok := findProfile(&cfg.LLM, id); !ok {
		return errors.New("profile not found")
	}
	cfg.LLM.ActiveProfileID = id
	return saveConfig(cfg)
}

// providerFor resolves a profile (by id, or the active default when id=="")
// and builds a llm.Provider plus the resolved model.
func (a *App) providerFor(id string) (llm.Provider, string, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, "", err
	}
	if id == "" {
		id = cfg.LLM.ActiveProfileID
	}
	p, ok := findProfile(&cfg.LLM, id)
	if !ok {
		return nil, "", errors.New("no LLM profile configured")
	}
	key, err := resolveKey(p)
	if err != nil {
		return nil, "", err
	}
	prov, err := llm.New(p.Kind, p.BaseURL, key)
	return prov, p.Model, err
}

// TestLLMProfile runs a tiny streamed turn to validate a profile's
// endpoint/key/model, returning nil on success or the provider error.
func (a *App) TestLLMProfile(id string) error {
	prov, model, err := a.providerFor(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := false
	err = prov.Stream(ctx, llm.Request{
		Model:     model,
		Messages:  []llm.Message{{Role: "user", Content: "ping"}},
		MaxTokens: 16,
	}, func(string) error { got = true; cancel(); return nil })
	// A cancel after the first token surfaces as context.Canceled — treat a
	// stream that produced output as success.
	if got {
		return nil
	}
	return err
}

// StartTurnReq is the payload for StartLLMTurn.
type StartTurnReq struct {
	ProfileID string        `json:"profileId"`
	System    string        `json:"system"`
	Messages  []llm.Message `json:"messages"`
}

// StartLLMTurn streams an assistant response in a goroutine, emitting
// llm:delta / llm:done / llm:error events keyed by the returned streamId.
func (a *App) StartLLMTurn(req StartTurnReq) (string, error) {
	prov, model, err := a.providerFor(req.ProfileID)
	if err != nil {
		return "", err
	}
	streamID := newStreamID()
	ctx, cancel := context.WithCancel(context.Background())

	a.llmMu.Lock()
	a.llmStreams[streamID] = cancel
	a.llmMu.Unlock()

	go func() {
		defer func() {
			a.llmMu.Lock()
			delete(a.llmStreams, streamID)
			a.llmMu.Unlock()
			cancel()
		}()
		err := prov.Stream(ctx, llm.Request{
			Model:     model,
			System:    req.System,
			Messages:  req.Messages,
			MaxTokens: llmMaxTokens,
		}, func(text string) error {
			runtime.EventsEmit(a.ctx, "llm:delta", map[string]string{"streamId": streamID, "text": text})
			return nil
		})
		if err != nil && ctx.Err() == nil {
			runtime.EventsEmit(a.ctx, "llm:error", map[string]string{"streamId": streamID, "message": err.Error()})
			return
		}
		runtime.EventsEmit(a.ctx, "llm:done", map[string]string{"streamId": streamID})
	}()
	return streamID, nil
}

// CancelLLMTurn stops an in-flight turn (the Stop button).
func (a *App) CancelLLMTurn(streamID string) {
	a.llmMu.Lock()
	cancel := a.llmStreams[streamID]
	a.llmMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
```

Add `"context"` and `"sync"` to `app.go`'s import block if not already present (`context` is; add `sync` only if missing — it is already imported for `savingMu`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestNewStreamID -v`
Expected: PASS. Then `go build ./...` and `go test ./...` — Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add llm_methods.go app.go llm_methods_test.go
git commit -m "feat(app): LLM config CRUD, TestLLMProfile, streaming StartLLMTurn/CancelLLMTurn"
```

---

## Task 6: Regenerate Wails bindings

**Files:** Modify (generated): `frontend/wailsjs/go/main/App.js`, `App.d.ts`, `frontend/wailsjs/go/models.ts`

- [ ] **Step 1: Regenerate**

Run: `PATH="$PATH:$(go env GOPATH)/bin" wails generate module`

- [ ] **Step 2: Verify**

Run: `grep -oE "GetLLMConfig|SaveLLMProfile|DeleteLLMProfile|SetActiveProfile|TestLLMProfile|StartLLMTurn|CancelLLMTurn" frontend/wailsjs/go/main/App.js | sort -u`
Expected: all seven names present.

- [ ] **Step 3: Commit**

```bash
git add frontend/wailsjs/go/main/App.js frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/models.ts
git commit -m "chore(bindings): regenerate wailsjs for LLM methods"
```

---

## Task 7: LLM Settings panel

**Files:**
- Create: `frontend/src/components/LLMSettings.svelte`

**Interfaces:**
- Consumes: `GetLLMConfig`, `SaveLLMProfile`, `DeleteLLMProfile`, `SetActiveProfile`, `TestLLMProfile` from `../wailsjs/go/main/App`.
- Produces: `LLMSettings` component with an `onClose` prop; a modal listing profiles (add from preset / edit / delete / set active / Test connection). Emits nothing else; it reads/writes config directly.

- [ ] **Step 1: Create the panel**

Create `frontend/src/components/LLMSettings.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import { GetLLMConfig, SaveLLMProfile, DeleteLLMProfile, SetActiveProfile, TestLLMProfile } from '../wailsjs/go/main/App'

  export let onClose = () => {}

  const PRESETS = [
    { name: 'Anthropic',  kind: 'anthropic',         baseUrl: 'https://api.anthropic.com',                          model: 'claude-opus-4-8' },
    { name: 'OpenAI',     kind: 'openai-compatible',  baseUrl: 'https://api.openai.com/v1',                          model: 'gpt-4o' },
    { name: 'Google',     kind: 'openai-compatible',  baseUrl: 'https://generativelanguage.googleapis.com/v1beta/openai', model: 'gemini-2.0-flash' },
    { name: 'OpenRouter', kind: 'openai-compatible',  baseUrl: 'https://openrouter.ai/api/v1',                       model: '' },
    { name: 'Local (Ollama)', kind: 'openai-compatible', baseUrl: 'http://localhost:11434/v1',                       model: 'llama3.1' },
  ]

  let profiles = []
  let activeId = ''
  let editing = null   // the profile being added/edited
  let testMsg = {}     // profileId -> status string
  let error = ''

  onMount(load)
  async function load() {
    try { const c = await GetLLMConfig(); profiles = c.profiles ?? []; activeId = c.activeProfileId ?? '' }
    catch (e) { error = String(e?.message ?? e) }
  }

  function addFrom(preset) {
    editing = { id: `p-${Date.now()}`, name: preset.name, kind: preset.kind, baseUrl: preset.baseUrl, model: preset.model, apiKey: '', apiKeyEnv: '' }
  }
  function edit(p) { editing = { ...p } }
  async function save() {
    try { await SaveLLMProfile(editing); editing = null; await load() }
    catch (e) { error = String(e?.message ?? e) }
  }
  async function del(id) { try { await DeleteLLMProfile(id); await load() } catch (e) { error = String(e?.message ?? e) } }
  async function setActive(id) { try { await SetActiveProfile(id); activeId = id } catch (e) { error = String(e?.message ?? e) } }
  async function test(id) {
    testMsg = { ...testMsg, [id]: 'testing…' }
    try { await TestLLMProfile(id); testMsg = { ...testMsg, [id]: '✓ ok' } }
    catch (e) { testMsg = { ...testMsg, [id]: '✕ ' + String(e?.message ?? e) } }
  }
</script>

<div class="ov-backdrop" on:click={onClose}></div>
<div class="ov-modal">
  <div class="ov-head"><span>LLM Providers</span><button on:click={onClose}>✕</button></div>
  {#if error}<div class="ov-error">{error}</div>{/if}

  {#if editing}
    <div class="ov-form">
      <label>Name<input bind:value={editing.name} /></label>
      <label>Kind
        <select bind:value={editing.kind}>
          <option value="openai-compatible">openai-compatible</option>
          <option value="anthropic">anthropic</option>
        </select>
      </label>
      <label>Base URL<input bind:value={editing.baseUrl} /></label>
      <label>Model<input bind:value={editing.model} placeholder="e.g. claude-opus-4-8" /></label>
      <label>API key<input type="password" bind:value={editing.apiKey} placeholder="leave blank to use an env var" /></label>
      <label>…or env var name<input bind:value={editing.apiKeyEnv} placeholder="e.g. ANTHROPIC_API_KEY" /></label>
      <div class="ov-actions"><button class="primary" on:click={save}>Save</button><button on:click={() => (editing = null)}>Cancel</button></div>
    </div>
  {:else}
    <ul class="ov-list">
      {#each profiles as p (p.id)}
        <li>
          <label class="ov-active"><input type="radio" checked={p.id === activeId} on:change={() => setActive(p.id)} /></label>
          <div class="ov-meta"><b>{p.name}</b><span>{p.kind} · {p.model || '(no model)'}</span></div>
          <button on:click={() => test(p.id)}>Test</button>
          <button on:click={() => edit(p)}>Edit</button>
          <button on:click={() => del(p.id)}>Delete</button>
          {#if testMsg[p.id]}<span class="ov-test">{testMsg[p.id]}</span>{/if}
        </li>
      {/each}
    </ul>
    <div class="ov-presets">
      <span>Add:</span>
      {#each PRESETS as preset}<button on:click={() => addFrom(preset)}>{preset.name}</button>{/each}
    </div>
  {/if}
</div>

<style>
  .ov-backdrop { position: fixed; inset: 0; background: rgba(0,0,0,0.5); z-index: 40; }
  .ov-modal { position: fixed; top: 8%; left: 50%; transform: translateX(-50%); width: 560px; max-width: 92vw; max-height: 80vh; overflow: auto; z-index: 41; background: #0e1622; border: 1px solid #2a3a4a; border-radius: 10px; color: #cdd6e2; }
  .ov-head { display: flex; justify-content: space-between; align-items: center; padding: 10px 14px; border-bottom: 1px solid #26374a; color: #7dd3fc; font-weight: 600; }
  .ov-head button { background: none; border: none; color: #6b7a8a; cursor: pointer; }
  .ov-error { color: #ff8080; padding: 6px 14px; font-size: 12px; }
  .ov-list { list-style: none; margin: 0; padding: 8px 12px; }
  .ov-list li { display: flex; align-items: center; gap: 8px; padding: 6px 0; border-bottom: 1px solid #1b2735; }
  .ov-meta { display: flex; flex-direction: column; flex: 1; min-width: 0; }
  .ov-meta span { color: #5a7a9a; font-size: 11px; }
  .ov-test { font-size: 11px; color: #93c5fd; }
  .ov-list button, .ov-presets button, .ov-actions button { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 3px 9px; cursor: pointer; font-size: 12px; }
  .ov-actions button.primary, .primary { background: #244a72; }
  .ov-presets { display: flex; gap: 6px; align-items: center; padding: 10px 14px; border-top: 1px solid #26374a; flex-wrap: wrap; }
  .ov-form { display: flex; flex-direction: column; gap: 8px; padding: 12px 14px; }
  .ov-form label { display: flex; flex-direction: column; gap: 3px; font-size: 12px; color: #93c5fd; }
  .ov-form input, .ov-form select { background: #0b1220; border: 1px solid #2a3a4a; border-radius: 6px; color: #e8eaf0; padding: 5px 8px; font-size: 13px; }
  .ov-actions { flex-direction: row; display: flex; gap: 8px; }
</style>
```

- [ ] **Step 2: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/LLMSettings.svelte
git commit -m "feat(notebook): LLM Providers settings panel"
```

---

## Phase 2b — LLM cell

## Task 8: LLM cell library (prompts, context assembly, stream client) + registry

**Files:**
- Create: `frontend/src/lib/llm.js`
- Modify: `frontend/src/lib/notebook.js`

**Interfaces:**
- Consumes: `StartLLMTurn`, `CancelLLMTurn` from `../../wailsjs/go/main/App`; `EventsOn` from `../../wailsjs/runtime/runtime`.
- Produces (in `llm.js`):
  - `ASK_SYSTEM`, `TUTOR_SYSTEM` (strings)
  - `buildSystem(mode, context, attachedCells)` → string (mode framing + context block)
  - `streamTurn({ profileId, system, messages, onDelta, onDone, onError })` → returns `{ streamId, cancel }`
- Produces (in `notebook.js`): `newLLMCell()`; `cellRegistry.llm = LLMCell`.

- [ ] **Step 1: Create `llm.js`**

Create `frontend/src/lib/llm.js`:

```js
import { StartLLMTurn, CancelLLMTurn } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

export const ASK_SYSTEM =
  'You are a thinking partner helping the user reason about and connect knowledge ' +
  'while they process reading extracts into notes. Be concise and substantive. Use ' +
  'the provided context (the extract, its neighbours, and source) when relevant. Do ' +
  'not fabricate; say when you are unsure.'

export const TUTOR_SYSTEM =
  'You are a Socratic learning tutor. Your goal is to strengthen the user’s understanding ' +
  'and help them formulate their OWN spaced-repetition cards — never to write finished cards for them.\n\n' +
  'Method — Feynman Technique: make the user explain concepts in plain language; probe where ' +
  'the explanation is vague, circular, or incomplete; ask "why" and "how do you know".\n\n' +
  'Method — Wozniak’s Rules of Knowledge Formulation, especially: prefer the minimum information ' +
  'per item; build on basics before the complex; avoid sets and enumerations; make each question ' +
  'specific and unambiguous; encourage the user’s own wording; keep context and sources.\n\n' +
  'Behaviour: ask one focused question at a time. When the user drafts a card, critique it against ' +
  'the rules (Is it minimal? Unambiguous? Does it test one thing?) and suggest how THEY could improve ' +
  'it — do not hand them a finished front/back. If asked to "just write the card", decline and guide ' +
  'them to write it. Be encouraging but rigorous. Use the provided context when relevant.'

// buildSystem composes the system prompt: mode framing + a context block built
// from the pinned context cell and any attached cells' text.
export function buildSystem(mode, context, attachedCells) {
  const parts = [mode === 'tutor' ? TUTOR_SYSTEM : ASK_SYSTEM]
  const ctx = []
  if (context?.extractText) ctx.push(`# Extract\n${context.extractText}`)
  if (context?.tags?.length) ctx.push(`# Tags\n${context.tags.join(', ')}`)
  if (context?.orbit?.length) {
    ctx.push('# Neighbours\n' + context.orbit.map((o) => `- ${o.arrowLong} ${o.neighborText}`).join('\n'))
  }
  if (context?.source?.passage) ctx.push(`# Source passage\n${context.source.passage}`)
  for (const c of attachedCells ?? []) {
    if (c.type === 'n4l' && c.text) ctx.push(`# Attached N4L draft\n${c.text}`)
    else if (c.type === 'llm' && c.messages?.length) {
      ctx.push('# Attached conversation\n' + c.messages.map((m) => `${m.role}: ${m.content}`).join('\n'))
    }
  }
  if (ctx.length) parts.push('--- Context ---\n' + ctx.join('\n\n'))
  return parts.join('\n\n')
}

// streamTurn starts a streamed turn and wires the delta/done/error events,
// filtered to this turn's streamId. Returns a cancel() that stops the stream
// and unsubscribes.
export function streamTurn({ profileId, system, messages, onDelta, onDone, onError }) {
  let offDelta = () => {}
  let offDone = () => {}
  let offError = () => {}
  let sid = null

  StartLLMTurn({ profileId: profileId || '', system, messages })
    .then((streamId) => {
      sid = streamId
      offDelta = EventsOn('llm:delta', (p) => { if (p?.streamId === sid) onDelta(p.text) })
      offDone = EventsOn('llm:done', (p) => { if (p?.streamId === sid) { cleanup(); onDone() } })
      offError = EventsOn('llm:error', (p) => { if (p?.streamId === sid) { cleanup(); onError(p.message) } })
    })
    .catch((e) => onError(String(e?.message ?? e)))

  function cleanup() { offDelta(); offDone(); offError() }
  function cancel() { if (sid) CancelLLMTurn(sid); cleanup() }
  return { cancel }
}
```

- [ ] **Step 2: Register the cell type**

In `frontend/src/lib/notebook.js`, add the import and factory + registry entry:

```js
import LLMCell from '../components/notebook/LLMCell.svelte'
```

```js
export function newLLMCell(mode = 'ask') {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'llm', mode, profileId: null, messages: [], attachments: [] }
}
```

```js
export const cellRegistry = {
  context: ContextCell,
  n4l: N4LDraftCell,
  llm: LLMCell,
}
```

(The `LLMCell` component is created in Task 9; `npm run build` in this task will fail until Task 9 exists, so do Steps 3–5 of Task 9 before building. To keep this task independently committable, create a minimal placeholder `LLMCell.svelte` now — `<script>export let cell</script><div>llm</div>` — and replace it in Task 9. Commit the placeholder with this task.)

- [ ] **Step 3: Create the LLMCell placeholder**

Create `frontend/src/components/notebook/LLMCell.svelte`:

```svelte
<script>
  export let cell
</script>
<div>llm cell placeholder</div>
```

- [ ] **Step 4: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/llm.js frontend/src/lib/notebook.js frontend/src/components/notebook/LLMCell.svelte
git commit -m "feat(notebook): llm cell library (prompts, context assembly, stream client) + registry"
```

---

## Task 9: LLMCell component

**Files:**
- Modify: `frontend/src/components/notebook/LLMCell.svelte` (replace the placeholder)

**Interfaces:**
- Consumes: `buildSystem`, `streamTurn` from `../../lib/llm.js`; `GetLLMConfig` from `../../wailsjs/go/main/App`.
- Props: `cell` (the llm cell object), `context` (pinned context payload), `cells` (sibling cells, for attach), `onChange(cell)`, `onDelete()`, `readOnly`.
- Persists conversation by calling `onChange(cell)` after each completed turn / mode / profile / attachment change.

- [ ] **Step 1: Replace the placeholder**

Replace `frontend/src/components/notebook/LLMCell.svelte` with:

```svelte
<script>
  import { onMount, onDestroy } from 'svelte'
  import { GetLLMConfig } from '../../wailsjs/go/main/App'
  import { buildSystem, streamTurn } from '../../lib/llm.js'

  export let cell
  export let context = null
  export let cells = []
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false

  let profiles = []
  let input = ''
  let streaming = false
  let partial = ''
  let turn = null
  let error = ''

  onMount(async () => { try { const c = await GetLLMConfig(); profiles = c.profiles ?? [] } catch { /* settings not configured */ } })
  onDestroy(() => turn?.cancel())

  $: attached = (cell.attachments ?? []).map((id) => cells.find((c) => c.id === id)).filter(Boolean)
  $: hasProvider = profiles.length > 0

  function persist() { onChange(cell) }
  function setMode(m) { cell.mode = m; cell = cell; persist() }
  function setProfile(id) { cell.profileId = id || null; cell = cell; persist() }
  function toggleAttach(id) {
    const a = new Set(cell.attachments ?? [])
    if (a.has(id)) a.delete(id); else a.add(id)
    cell.attachments = [...a]; cell = cell; persist()
  }

  function send() {
    const text = input.trim()
    if (!text || streaming) return
    cell.messages = [...cell.messages, { role: 'user', content: text }]
    cell = cell
    input = ''
    error = ''
    partial = ''
    streaming = true
    const system = buildSystem(cell.mode, context, attached)
    turn = streamTurn({
      profileId: cell.profileId,
      system,
      messages: cell.messages,
      onDelta: (t) => { partial += t },
      onDone: () => { finish(partial) },
      onError: (m) => { error = m; streaming = false; partial = ''; turn = null },
    })
  }

  function finish(text) {
    if (text) { cell.messages = [...cell.messages, { role: 'assistant', content: text }]; cell = cell; persist() }
    partial = ''
    streaming = false
    turn = null
  }

  function stop() { turn?.cancel(); streaming = false; partial = ''; turn = null }
  function onKey(e) { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); send() } }
</script>

<div class="llm-cell">
  <div class="llm-bar">
    <div class="modes">
      <button class:active={cell.mode === 'ask'} on:click={() => setMode('ask')} disabled={readOnly}>Ask</button>
      <button class:active={cell.mode === 'tutor'} on:click={() => setMode('tutor')} disabled={readOnly}>Tutor</button>
    </div>
    <select class="prof" value={cell.profileId || ''} on:change={(e) => setProfile(e.target.value)} disabled={readOnly} title="Model profile">
      <option value="">active default</option>
      {#each profiles as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <span class="spacer"></span>
    {#if !readOnly}<button class="del" on:click={onDelete} title="Delete cell">✕</button>{/if}
  </div>

  {#if !readOnly && cells.filter((c) => c.id !== cell.id && (c.type === 'n4l' || c.type === 'llm')).length}
    <div class="attach">
      <span>Attach context:</span>
      {#each cells.filter((c) => c.id !== cell.id && (c.type === 'n4l' || c.type === 'llm')) as c}
        <label><input type="checkbox" checked={(cell.attachments ?? []).includes(c.id)} on:change={() => toggleAttach(c.id)} /> {c.type === 'n4l' ? 'draft' : 'llm'} {c.id.slice(-4)}</label>
      {/each}
    </div>
  {/if}

  <div class="transcript">
    {#each cell.messages as m}
      <div class="msg {m.role}"><span class="who">{m.role}</span><div class="body">{m.content}</div></div>
    {/each}
    {#if streaming}<div class="msg assistant"><span class="who">assistant</span><div class="body">{partial}<span class="caret">▋</span></div></div>{/if}
    {#if error}<div class="llm-error">{error}</div>{/if}
  </div>

  {#if !readOnly}
    {#if !hasProvider}
      <div class="llm-hint">No LLM provider configured — open ⚙ Models to add one.</div>
    {:else}
      <div class="composer">
        <textarea bind:value={input} on:keydown={onKey} placeholder={cell.mode === 'tutor' ? 'Explain your understanding, or draft a card…' : 'Ask…'} rows="2" disabled={streaming}></textarea>
        {#if streaming}<button class="stop" on:click={stop}>Stop</button>{:else}<button class="send" on:click={send}>Send</button>{/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .llm-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .llm-bar { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid #1c2a3a; }
  .modes button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 10px; cursor: pointer; font-size: 12px; }
  .modes button.active { background: #244a72; color: #e8eaf0; }
  .prof { background: #0b1220; border: 1px solid #2a3a4a; border-radius: 6px; color: #cdd6e2; font-size: 12px; padding: 2px 6px; }
  .spacer { flex: 1; }
  .del { background: none; border: none; color: #6b7a8a; cursor: pointer; }
  .del:hover { color: #ff8080; }
  .attach { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; padding: 6px 8px; border-bottom: 1px solid #1c2a3a; color: #5a7a9a; font-size: 11px; }
  .attach label { color: #93c5fd; }
  .transcript { padding: 8px; display: flex; flex-direction: column; gap: 8px; max-height: 340px; overflow-y: auto; }
  .msg { display: flex; flex-direction: column; gap: 2px; }
  .msg .who { font-size: 10px; letter-spacing: .06em; text-transform: uppercase; color: #5a7a9a; }
  .msg .body { white-space: pre-wrap; font-size: 13px; }
  .msg.user .body { color: #e8eaf0; }
  .msg.assistant .body { color: #b9c6d6; }
  .caret { color: #7dd3fc; }
  .llm-error { color: #ff8080; font-size: 12px; }
  .llm-hint { color: #b58a3a; font-size: 12px; padding: 8px; }
  .composer { display: flex; gap: 6px; padding: 8px; border-top: 1px solid #1c2a3a; }
  .composer textarea { flex: 1; background: #0b1220; border: 1px solid #2a3a4a; border-radius: 6px; color: #e8eaf0; font-size: 13px; padding: 6px 8px; resize: vertical; }
  .send, .stop { align-self: flex-end; border: 1px solid #2a3a4a; border-radius: 6px; padding: 6px 12px; cursor: pointer; font-size: 12px; }
  .send { background: #1e3a5a; color: #e8eaf0; }
  .stop { background: #5a1e1e; color: #ffd7d7; }
</style>
```

- [ ] **Step 2: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/notebook/LLMCell.svelte
git commit -m "feat(notebook): LLM cell (multi-turn ask/tutor, streaming, profile override, attach)"
```

---

## Task 10: Wire LLM cell into NotebookPane

**Files:**
- Modify: `frontend/src/components/notebook/NotebookPane.svelte`
- Modify: `frontend/src/components/notebook/N4LDraftCell.svelte` (accept & ignore `context`/`cells` props to avoid unknown-prop warnings)

**Interfaces:**
- Consumes: `newDraftCell`, `newLLMCell`, `cellRegistry` from `../../lib/notebook.js`; `LLMSettings` from `../LLMSettings.svelte`.
- Produces: the pane renders each cell via `<svelte:component this={cellRegistry[cell.type]} ... context={context} cells={cells} />`; adds "+ LLM cell" and a "⚙ Models" button that opens `LLMSettings`.

- [ ] **Step 1: Pass context + siblings to cells and add controls**

In `NotebookPane.svelte`:

- Import additions:
```js
  import { newDraftCell, newLLMCell, cellRegistry } from '../../lib/notebook.js'
  import LLMSettings from '../LLMSettings.svelte'
```
- Add state + handlers in `<script>`:
```js
  let showSettings = false
  function addLLM() { cells = [...cells, newLLMCell('ask')]; scheduleSave() }
```
- Replace the cell render (the `<svelte:component this={cellRegistry[cell.type]} ...>` from Slice 1) so it also passes `context` and `cells`:
```svelte
          <svelte:component this={cellRegistry[cell.type]} {cell} {readOnly} {context} cells={cells}
            onChange={(t) => (typeof t === 'string' ? updateCell(i, t) : updateCellObj(i, t))}
            onPush={(t) => onPush(t)}
            onDelete={() => deleteCell(i)} />
```
- Add `updateCellObj` (LLM cells call `onChange(cell)` with the whole object; draft cells call `onChange(text)`):
```js
  function updateCellObj(i, obj) { cells[i] = obj; cells = cells; scheduleSave() }
```
- In the header (`.nb-head`), add a gear button:
```svelte
    <button class="nb-gear" on:click={() => (showSettings = true)} title="LLM providers">⚙ Models</button>
```
- Next to "+ N4L draft cell", add:
```svelte
      <button class="nb-add" on:click={addLLM}>+ LLM cell</button>
```
- At the end of the markup, mount the settings modal:
```svelte
{#if showSettings}<LLMSettings onClose={() => (showSettings = false)} />{/if}
```
- Add a style for `.nb-gear` (mirror `.nb-title` sizing):
```css
  .nb-gear { margin-left: auto; background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
```

- [ ] **Step 2: Silence unknown-prop warnings on N4LDraftCell**

In `N4LDraftCell.svelte`, add two ignored props near the other `export let` lines so the shared `<svelte:component>` prop set doesn't warn:

```js
  export let context = null   // unused; present so the shared cell prop set matches
  export let cells = []       // unused
```

- [ ] **Step 3: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 4: Manual smoke test**

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build && ./build/bin/texere`
Verify: open an extract → notebook shows "+ N4L draft cell", "+ LLM cell", and "⚙ Models"; adding an LLM cell renders the ask/tutor bar; ⚙ Models opens the settings panel.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/notebook/NotebookPane.svelte frontend/src/components/notebook/N4LDraftCell.svelte
git commit -m "feat(notebook): mount LLM cell + Models settings in NotebookPane"
```

---

## Task 11: Retire the Tutor tab

**Files:**
- Modify: `frontend/src/components/BottomBand.svelte`
- Delete: `frontend/src/components/TutorTab.svelte`

**Interfaces:** none (removes a tab).

- [ ] **Step 1: Remove the tab wiring**

In `BottomBand.svelte`:
- Delete the import line `import TutorTab from './TutorTab.svelte'`.
- Delete the tab button line: `<button class="tab-btn" class:active={activeTab === 'tutor'} on:click={() => setTab('tutor')}>Tutor</button>`.
- Delete the pane block for the tutor tab (the `{:else if activeTab === 'tutor'}` branch and its `<TutorTab />` content, keeping the surrounding `{#if}`/`{/if}` structure valid).

Then delete the component:

```bash
git rm frontend/src/components/TutorTab.svelte
```

- [ ] **Step 2: Verify the frontend compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds; no reference to `TutorTab` or `activeTab === 'tutor'` remains (`grep -rn "TutorTab\|'tutor'" frontend/src` returns nothing).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/BottomBand.svelte
git commit -m "refactor(ui): retire the Tutor tab (LLM cell is the only LLM surface)"
```

---

## Task 12: Manual test plan + full verification

**Files:**
- Create: `docs/specs/2026-07-22-processing-notebook-slice2-manual-test-plan.md`

- [ ] **Step 1: Write the manual test plan**

Create the file:

```markdown
# Processing Notebook — Slice 2 (LLM Cell) Manual Test Plan

Prereqs: an API key for at least one provider (Anthropic/OpenAI/OpenRouter) or a
local OpenAI-compatible server (Ollama). Open an extract for processing.

## TC-1 Configure a provider
1. Notebook pane → ⚙ Models → Add → pick a preset (e.g. Anthropic).
2. Enter a model (e.g. `claude-opus-4-8`) and paste the key (or set the env-var name).
3. Save → click Test.
   - EXPECT: "✓ ok" (or a clear provider error for a bad key/model).
4. Restart the app; reopen ⚙ Models.
   - EXPECT: the profile persisted; it is the active default.

## TC-2 Ask mode streaming
1. Add an LLM cell (mode = Ask). Type a question; Cmd/Ctrl+Enter or Send.
   - EXPECT: the reply streams in token-by-token; on completion it becomes a fixed assistant bubble.
2. Send a follow-up.
   - EXPECT: multi-turn — the model answers with the prior turn in context.

## TC-3 Tutor mode
1. Switch the cell to Tutor. Draft a card front/back and ask it to review.
   - EXPECT: it critiques the card (minimal? unambiguous?) and does NOT write a finished card; asks a probing question.

## TC-4 Per-cell override
1. Add a second profile (different model). On one LLM cell, pick it in the profile dropdown; leave the other on "active default".
   - EXPECT: each cell uses its selected model (verify via the reply / provider dashboard).

## TC-5 Attach context
1. Add an N4L draft cell with some text. In an LLM cell, check it under "Attach context". Ask about it.
   - EXPECT: the reply reflects the attached draft's content.

## TC-6 Stop + errors
1. Send a long prompt; click Stop mid-stream.
   - EXPECT: streaming halts; the partial is discarded (no assistant bubble committed).
2. Edit the profile to a bad key; send.
   - EXPECT: a clear error is shown; no assistant bubble is committed.

## TC-7 Persistence + Tutor tab gone
1. Have a conversation, restart the app, reopen the extract.
   - EXPECT: the LLM cell's conversation persisted.
2. Look at the bottom band.
   - EXPECT: there is no "Tutor" tab.
```

- [ ] **Step 2: Backend suite + lint + full build**

Run: `go test ./...`
Expected: all PASS.

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`. (`make lint` may show the pre-existing failures noted in Slice 1; confirm no new issues in `internal/llm/`, `llm_config.go`, `llm_methods.go` via `golangci-lint run ./internal/llm/... .`.)

- [ ] **Step 3: Execute the manual test plan**

Run: `./build/bin/texere` and walk TC-1 … TC-7 against a real provider.

- [ ] **Step 4: Commit**

```bash
git add docs/specs/2026-07-22-processing-notebook-slice2-manual-test-plan.md
git commit -m "docs(test-plan): Processing Notebook Slice 2 manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** provider infra (T1–T3, T5), global profiles + key resolution + 0600 (T4), Test connection (T5), streaming transport (T5), settings UI + presets (T7), llm cell library incl. ask framing + tutor prompt + context assembly (T8), multi-turn ask/tutor cell with streaming + per-cell override + attach (T9), NotebookPane integration + "+ LLM cell" + ⚙ Models (T10), retire Tutor tab (T11), manual test plan + verification (T12). Deferred items (multimodal, other cell types, `\notebooks` search, leech feed-forward, tool use, cost accounting) are out of scope by design.
- **Provider fidelity:** Anthropic uses native `/v1/messages` (not an OpenAI shim, not the SDK) per the approved spec; wire details (headers, `anthropic-version`, `content_block_delta`/`text_delta`, `message_stop`, `claude-opus-4-8`, no sampling/thinking params) are from the `claude-api` skill.
- **Type consistency:** `LLMProfile`/`LLMConfig` JSON tags (`baseUrl`,`apiKey`,`apiKeyEnv`,`activeProfileId`) match the Svelte reads; `StartTurnReq` fields (`profileId`,`system`,`messages`) match `streamTurn`; `llm.Message` (`role`,`content`) is shared across adapters and the frontend; events `llm:delta`/`llm:done`/`llm:error` carry `{streamId,text|message}` consistently between `llm_methods.go` and `llm.js`.
- **Known limitation:** the LLM cell always sends the full context block each turn (no per-turn caching); acceptable for Slice 2. Adaptive thinking, prompt caching, and tool use are deliberately omitted.
```
