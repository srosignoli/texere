# Processing Notebook — Slice 2: LLM Cell (provider-agnostic)

**Date:** 2026-07-22
**Status:** Approved design
**Feature area:** Processing Notebook (incremental reading)
**Slice:** 2 of ~6 (see the Slice 1 design for the full decomposition)

## Summary

Add an **LLM cell** to the processing notebook: a multi-turn conversation cell
that reasons with the user about an extract, in two modes — **ask** (a light
thinking-partner) and **tutor** (a Socratic coach based on the Feynman Technique
and Wozniak's 20 Rules of Knowledge Formulation that critiques the user's own
card drafts but never writes cards for them). Responses stream token-by-token.

The LLM backend is **provider-agnostic**: Anthropic, OpenAI, Google, OpenRouter,
or any local server. Providers are configured as **named profiles** stored
globally; one is active by default, and an individual cell may override it.
There is currently **no LLM code in the repository** — this slice introduces it.

This slice retires the `TutorTab.svelte` placeholder and its bottom-band `tutor`
tab: the LLM cell is now the only LLM surface.

## Decisions made during brainstorming

Do not silently reopen these during implementation:

- **Configuration model: named profiles, one active.** Multiple provider
  profiles are defined once and switched between; keys are entered once per
  profile.
- **Granularity: global config, per-cell override allowed.** The profile set and
  active default live globally (not per-notebook); each LLM cell defaults to the
  active profile but may pick any defined profile.
- **Client architecture: `Provider` interface + OpenAI-compatible adapter +
  native Anthropic adapter.** The universal OpenAI-compatible client covers
  OpenAI, OpenRouter, Google's compat endpoint, and local servers; Anthropic
  uses its native `/v1/messages` API.
- **API key storage: plaintext in `config.json` (chmod 0600), with an env-var
  reference option.** A profile stores either a literal key or the name of an
  environment variable that holds it.
- **Conversation model: multi-turn thread per cell.** Each LLM cell holds a
  running conversation that persists.
- **Ask mode uses a light framing system prompt** (not empty).
- **Retire the Tutor tab entirely** (not repurposed for settings).

## Scope

**In scope (one spec, two build phases):**

- **Phase 2a — Provider infrastructure**
  - `internal/llm` package: `Provider` interface, OpenAI-compatible adapter,
    native Anthropic adapter, with unit tests against mock SSE streams.
  - Global LLM profile config in `AppConfig`; key resolution (literal / env var).
  - Wails methods: profile CRUD, set-active, and **Test connection**.
  - An **LLM Settings** panel (list/add/edit/delete/set-active/test) with
    provider presets.
- **Phase 2b — LLM cell**
  - The `llm` cell component: multi-turn transcript, streaming, ask/tutor toggle,
    per-cell profile override, attach-context control.
  - Streaming transport (Wails events) with cancel.
  - Context assembly (pinned context cell + attached cells).
  - Retire `TutorTab.svelte` and the `tutor` bottom-band tab; add "+ LLM cell".

**Out of scope (later slices):** multimodal attachments / image blocks / handwriting
transcription (Slice 6); graph-query, compute, sketch, image cells (Slices 3–5);
`\notebooks` search; leech feed-forward; tool use / function calling; token/cost
accounting.

## Architecture & component boundaries

```
internal/llm/                     # NEW package — provider-agnostic client
├── llm.go        # Message, Request, Delta types; Provider interface; New(profile) factory
├── openai.go     # openai-compatible adapter (OpenAI/OpenRouter/Google-compat/local)
├── anthropic.go  # native Anthropic /v1/messages adapter
├── sse.go        # shared SSE line reader
└── *_test.go     # httptest-backed adapter tests

llm_methods.go                    # NEW Wails methods (repo root, package main)
                                  #   config CRUD + TestLLMProfile + StartLLMTurn/CancelLLMTurn
app.go / methods.go               # extend AppConfig with LLMConfig; key resolution helper

frontend/src/
├── components/notebook/LLMCell.svelte   # NEW cell component
├── components/LLMSettings.svelte        # NEW settings panel
├── lib/notebook.js                      # register 'llm' in cellRegistry; newLLMCell()
├── lib/llm.js                           # NEW: prompts (ask framing, tutor), context assembly, stream client
└── components/notebook/NotebookPane.svelte  # add "+ LLM cell"; pass context to cells
```

Removed: `frontend/src/components/TutorTab.svelte` and the `tutor` tab wiring in
`BottomBand.svelte`.

The `llm` cell slots into the existing `cellRegistry` (Slice 1), so
`NotebookPane` renders it without special-casing. `LLMCell` needs the pinned
context payload and the sibling cells (for attachment); `NotebookPane` passes
both down.

## Config & profiles (global, `config.json`)

`AppConfig` (in `app.go`) gains an `LLM` field:

```go
type LLMProfile struct {
    ID        string `json:"id"`                  // stable, generated
    Name      string `json:"name"`                // display, e.g. "Claude Sonnet"
    Kind      string `json:"kind"`                // "openai-compatible" | "anthropic"
    BaseURL   string `json:"baseUrl"`             // provider base URL
    Model     string `json:"model"`               // model id
    APIKey    string `json:"apiKey,omitempty"`    // literal key (plaintext), OR
    APIKeyEnv string `json:"apiKeyEnv,omitempty"` // name of env var holding the key
}

type LLMConfig struct {
    Profiles        []LLMProfile `json:"profiles"`
    ActiveProfileID string       `json:"activeProfileId"`
}

type AppConfig struct {
    CollectionDir string    `json:"collectionDir"`
    RecentArrows  []string  `json:"recentArrows"`
    LLM           LLMConfig `json:"llm"`
}
```

- **Key resolution** (`resolveKey(p LLMProfile) (string, error)`): if `APIKeyEnv`
  is set, return `os.Getenv(APIKeyEnv)` (error if empty); else return `APIKey`
  (error if empty). Keys are never logged.
- `config.json` is written with mode `0600` by `saveConfig` only (do **not**
  change the shared `atomicWriteText`, which also writes `.n4l` note files).
- **Presets** offered in the Settings UI when adding a profile (prefill
  `Kind`+`BaseURL`; the user supplies name, model, key):
  - Anthropic — `anthropic`, `https://api.anthropic.com`
  - OpenAI — `openai-compatible`, `https://api.openai.com/v1`
  - Google (Gemini, OpenAI-compat) — `openai-compatible`,
    `https://generativelanguage.googleapis.com/v1beta/openai`
  - OpenRouter — `openai-compatible`, `https://openrouter.ai/api/v1`
  - Local (Ollama) — `openai-compatible`, `http://localhost:11434/v1`

Exact preset URLs are verified against provider docs / the `claude-api` skill at
implementation time.

## Provider abstraction (`internal/llm`)

```go
type Message struct {
    Role    string // "system" | "user" | "assistant"
    Content string
}

type Request struct {
    Model       string
    System      string     // system prompt (mode framing + assembled context)
    Messages    []Message  // the conversation (user/assistant turns)
    MaxTokens   int
    Temperature float32
}

// Stream streams the assistant response, invoking onDelta for each text chunk.
// It returns when the stream ends or ctx is cancelled. Errors carry the
// provider's message where available.
type Provider interface {
    Stream(ctx context.Context, req Request, onDelta func(string) error) error
}

func New(p LLMProfile, key string) (Provider, error) // dispatch on p.Kind
```

- **openai-compatible** (`openai.go`): `POST {BaseURL}/chat/completions`,
  `Authorization: Bearer <key>`, body `{model, messages:[{role:"system",...},
  ...], stream:true, max_tokens, temperature}`; parse SSE `data:` lines,
  extract `choices[0].delta.content`, stop on `data: [DONE]`.
- **anthropic** (`anthropic.go`): `POST {BaseURL}/v1/messages`,
  headers `x-api-key: <key>`, `anthropic-version: <pinned>`; body `{model,
  system, messages:[{role:"user"|"assistant",...}], stream:true, max_tokens}`;
  parse SSE events, emit text from `content_block_delta` / `text_delta`.
  System prompt goes in the top-level `system` field (Anthropic messages are
  user/assistant only).
- Both use an `http.Client` with a sensible timeout, following the
  `internal/source/web.go` pattern, and a shared `sse.go` line scanner.

The exact Anthropic model IDs, streaming event names, and `anthropic-version`
value are taken from the `claude-api` skill at implementation time — not from
memory.

## Streaming transport (Wails)

`llm_methods.go` (package main):

- `StartLLMTurn(req StartTurnReq) (string, error)` — `req` carries the resolved
  or overriding `profileId`, the `system` string, and the `messages`. Generates
  a `streamId`, launches a goroutine that builds a `Provider` and calls
  `Stream`, emitting:
  - `llm:delta` `{streamId, text}` per chunk,
  - `llm:done` `{streamId}` on success,
  - `llm:error` `{streamId, message}` on failure.
  Cancel funcs are stored in `map[string]context.CancelFunc` guarded by a mutex.
- `CancelLLMTurn(streamId string)` — cancels the context (Stop button).

`LLMCell` calls `StartLLMTurn`, subscribes to the three events filtered by its
`streamId`, appends deltas to the in-progress assistant bubble, and commits the
message on `llm:done`.

## LLM cell (data model + behavior)

Cell object in `cells_json`:

```json
{ "id": "...", "type": "llm", "mode": "ask",
  "profileId": null,
  "messages": [ { "role": "user", "content": "..." },
                { "role": "assistant", "content": "..." } ],
  "attachments": [ "<cellId>", ... ] }
```

- **Multi-turn transcript**: user/assistant bubbles; an input box + Send; a Stop
  button while streaming. Autosaved with the notebook (same debounce as Slice 1).
  Streaming deltas update the last (in-progress) assistant bubble; the message is
  only persisted on `llm:done`. A cancelled turn discards its partial text.
- **Mode toggle**: ask | tutor (see below).
- **Per-cell profile override**: a selector defaulting to the active profile;
  `profileId: null` means "use active".
- **Attach context**: a small multiselect of other cells in the notebook whose
  ids go in `attachments`. Slice 2 attachables are N4L draft cells (their text)
  and other LLM cells (their transcript).

### Context assembly (`frontend/src/lib/llm.js`)

`Request.System` is composed, in order:

1. The **mode system prompt** (ask framing or tutor prompt).
2. A **context block**: always the pinned context cell (extract text, tags,
   radius-1 orbit, source passage), then each attached cell's content, each under
   a clear header.

The cell's `messages` array is the conversation. Nothing from the notebook is
sent except the explicit context block and the conversation.

### Mode prompts

**Ask (light framing):**

> You are a thinking partner helping the user reason about and connect knowledge
> while they process reading extracts into notes. Be concise and substantive.
> Use the provided context (the extract, its neighbours, and source) when
> relevant. Do not fabricate; say when you are unsure.

**Tutor (Socratic; Feynman + Wozniak; never writes cards):**

> You are a Socratic learning tutor. Your goal is to strengthen the user's
> understanding and help them formulate their OWN spaced-repetition cards — never
> to write finished cards for them.
>
> Method — Feynman Technique: make the user explain concepts in plain language;
> probe where the explanation is vague, circular, or incomplete; ask "why" and
> "how do you know".
>
> Method — Wozniak's Rules of Knowledge Formulation, especially: prefer the
> minimum information per item; build on the basics before the complex; avoid
> sets and enumerations; make each question specific and unambiguous; encourage
> the user's own wording; keep context and sources.
>
> Behaviour: ask one focused question at a time. When the user drafts a card,
> critique it against the rules (Is it minimal? Unambiguous? Does it test one
> thing?) and suggest how THEY could improve it — do not hand them a finished
> front/back. If asked to "just write the card", decline and guide them to write
> it. Be encouraging but rigorous. Use the provided context when relevant.

Exact final wording is refined during implementation; the principles above are
fixed.

## Settings UI + retiring the Tutor tab

- **LLM Settings panel** (`LLMSettings.svelte`): lists profiles; add (from a
  preset or blank), edit, delete, set-active; each profile has a **Test
  connection** button that issues a tiny 1-token completion and shows success or
  the provider's error verbatim. Reached from a gear in the notebook pane header
  and from the LLM cell's "Configure a provider" link (following the existing
  `ExplorerSidebar` gear-toggle pattern). Keys are shown masked with a reveal
  toggle.
- **Retire** `TutorTab.svelte` and remove the `tutor` tab button + pane from
  `BottomBand.svelte` (and any `activeTab === 'tutor'` handling). The LLM cell is
  the only LLM surface.
- **Cell insertion**: add a **"+ LLM cell"** button to `NotebookPane` beside
  "+ N4L draft cell".

## Error handling

- **No active profile / unresolved key**: the cell shows "Configure an LLM
  provider" linking to Settings; no request is sent.
- **API / network / 4xx / 429 errors**: surfaced via `llm:error` and rendered in
  the cell with a **Retry**; the failed turn is not persisted as an assistant
  message. Provider error messages are shown verbatim (helpful for wrong
  key/model/URL).
- **Cancel**: `CancelLLMTurn` stops the stream; the partial assistant text is
  discarded.
- **Timeouts**: enforced by the `http.Client`; a timeout is reported as an
  `llm:error`.
- Keys are never written to logs or emitted in error payloads.

## Testing

- **Go unit tests** (no real API calls): OpenAI and Anthropic adapters against
  `httptest` servers replaying canned SSE streams → assert deltas parse and
  concatenate correctly, including a mid-stream `[DONE]`/`message_stop` and a
  provider error body; `New()` dispatch on `Kind`; `resolveKey` for literal vs
  env-var (incl. the missing-env error); profile CRUD + set-active persistence in
  a temp `config.json`.
- **Frontend**: `npm run build` plus a manual test plan (in the Slice 1 style):
  configure a profile, Test connection, ask-mode streaming, tutor-mode critique
  of a card draft, per-cell override, attach a draft cell, Stop mid-stream, and a
  bad-key error.

## Success criteria

- A profile can be configured for each provider family and **Test connection**
  passes (or reports a clear error).
- An **ask** cell streams a reply token-by-token from the active profile.
- A **tutor** cell challenges a drafted card and declines to write it.
- A **per-cell override** uses a different model than the active profile.
- **Attaching** an N4L draft cell includes its text in the model's context.
- **Stop** cancels a stream mid-response.
- A **bad key** surfaces a clear, provider-supplied error without crashing.
- Profiles and cell conversations **persist across restart**.
- The **Tutor tab is gone**; the LLM cell is the only LLM surface.

## Dependencies

Existing: `AppConfig`/`config.json` (global settings), Wails events/bindings, the
`internal/source/web.go` HTTP pattern, Slice 1's `cellRegistry` / `NotebookPane`
/ context payload. New third-party dependencies: **none** (standard library
`net/http` + `encoding/json` + `bufio` for SSE). The `claude-api` skill is an
implementation-time reference for Anthropic specifics (model IDs, streaming
events, `anthropic-version`).
