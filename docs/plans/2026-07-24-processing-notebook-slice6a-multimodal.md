# Processing Notebook — Slice 6a: Multimodal attach + transcription — Implementation Plan

**Goal:** Let sketch/image cells feed the LLM — attach them to an LLM cell (chat about the picture) and one-click Transcribe them into an editable N4L draft (text + Mermaid for diagrams).

**Architecture:** Add an optional `Images` field to `llm.Message`; the Anthropic/OpenAI providers emit a string-or-array `content` accordingly (text-only turns unchanged). A frontend `bake.js` renders a sketch (strokes→PNG) or image cell (rotation+crop→JPEG) to a downscaled base64 image. Attach bakes attached cells into the first user message; a shared `TranscribeBar` runs a one-shot vision turn and inserts the result as an N4L draft via a new `onInsert` callback.

**Tech Stack:** Go stdlib (providers); Svelte 3.49 + Canvas API; existing `renderStrokes`, `/media` handler, `StartLLMTurn`/`streamTurn`/`GetLLMConfig`. No new deps, no CGo.

## Global Constraints

- No new npm or Go dependencies; no CGo.
- **Text-only LLM turns must serialize byte-identically to today** (regression-safe): a message with no images still sends `content` as a plain string.
- **No image bytes in the notebook DB** — `cells_json` stores only attachment cell-ids (as today) and `transcribeProfileId`; images are baked from source cells at send time.
- Images ride **once, on the first user message** of a request.
- Frontend has no JS test runner; frontend tasks verify with `cd frontend && npm run build` + a manual test plan. Go uses `go test ./...`.
- `Image` JSON shape is `{mediaType, data}` (base64, no `data:` prefix), matching `bake.js` output and `llm.Image`.
- Provider capability is not gated; a non-vision model's API error surfaces via the existing `llm:error` path.

---

## File Structure

- `internal/llm/llm.go` — MODIFY: add `Image` type + `Images` on `Message`.
- `internal/llm/wire.go` — CREATE: `anthropicMessages` / `openaiMessages` builders.
- `internal/llm/anthropic.go`, `internal/llm/openai.go` — MODIFY: use the builders.
- `internal/llm/wire_test.go` — CREATE.
- `frontend/src/lib/bake.js` — CREATE: `bakeSketch`, `bakeImage`.
- `frontend/src/lib/llm.js` — MODIFY: add `TRANSCRIBE_SYSTEM`.
- `frontend/src/components/notebook/LLMCell.svelte` — MODIFY: attach picker + async send with images.
- `frontend/src/components/notebook/TranscribeBar.svelte` — CREATE.
- `frontend/src/components/notebook/SketchCell.svelte`, `ImageCell.svelte` — MODIFY: mount `TranscribeBar`, add `onInsert` prop.
- `frontend/src/components/notebook/NotebookPane.svelte` — MODIFY: `insertAfter` + pass `onInsert`.
- `docs/specs/2026-07-24-processing-notebook-slice6a-manual-test-plan.md` — CREATE (Task 5).

---

## Task 1: Go — multimodal message type + provider serialization (TDD)

**Files:**
- Modify: `internal/llm/llm.go`, `internal/llm/anthropic.go`, `internal/llm/openai.go`
- Create: `internal/llm/wire.go`, `internal/llm/wire_test.go`

**Interfaces:**
- Produces: `llm.Image{MediaType, Data string}`, `Message.Images []Image`, and internal `anthropicMessages`/`openaiMessages`. Consumed by the frontend (which sends `images` on message JSON) in Tasks 3–4.

- [ ] **Step 1: Write the failing tests**

Create `internal/llm/wire_test.go`:

```go
package llm

import (
	"reflect"
	"testing"
)

func TestAnthropicMessagesTextOnly(t *testing.T) {
	got := anthropicMessages([]Message{{Role: "user", Content: "hi"}})
	want := []map[string]any{{"role": "user", "content": "hi"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestAnthropicMessagesWithImage(t *testing.T) {
	got := anthropicMessages([]Message{{
		Role: "user", Content: "what is this?",
		Images: []Image{{MediaType: "image/png", Data: "AAAA"}},
	}})
	want := []map[string]any{{
		"role": "user",
		"content": []map[string]any{
			{"type": "text", "text": "what is this?"},
			{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "AAAA"}},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestOpenAIMessagesWithImage(t *testing.T) {
	got := openaiMessages([]Message{{
		Role: "user", Content: "caption",
		Images: []Image{{MediaType: "image/jpeg", Data: "BBBB"}},
	}})
	want := []map[string]any{{
		"role": "user",
		"content": []map[string]any{
			{"type": "text", "text": "caption"},
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64,BBBB"}},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestOpenAIMessagesTextOnly(t *testing.T) {
	got := openaiMessages([]Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "u"}})
	want := []map[string]any{{"role": "system", "content": "sys"}, {"role": "user", "content": "u"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/llm/ -run 'Messages'`
Expected: FAIL (undefined `anthropicMessages`/`openaiMessages`, `Image`).

- [ ] **Step 3: Add the types in `llm.go`**

In `internal/llm/llm.go`, replace the `Message` struct and add `Image`:

```go
type Image struct {
	MediaType string `json:"mediaType"` // "image/png" | "image/jpeg"
	Data      string `json:"data"`      // base64, no data: prefix
}

type Message struct {
	Role    string  `json:"role"`
	Content string  `json:"content"`
	Images  []Image `json:"images,omitempty"` // user turns only
}
```

- [ ] **Step 4: Create `wire.go`**

Create `internal/llm/wire.go`:

```go
package llm

// anthropicMessages builds the Anthropic /v1/messages "messages" array: a plain
// string content for text-only messages, or a content-block array when a message
// carries images (text block first, then image blocks).
func anthropicMessages(msgs []Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		if len(m.Images) == 0 {
			out = append(out, map[string]any{"role": m.Role, "content": m.Content})
			continue
		}
		blocks := make([]map[string]any, 0, len(m.Images)+1)
		if m.Content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
		}
		for _, im := range m.Images {
			blocks = append(blocks, map[string]any{
				"type":   "image",
				"source": map[string]any{"type": "base64", "media_type": im.MediaType, "data": im.Data},
			})
		}
		out = append(out, map[string]any{"role": m.Role, "content": blocks})
	}
	return out
}

// openaiMessages builds the OpenAI /chat/completions "messages" array with the
// same string-or-parts rule (image_url data URIs for images).
func openaiMessages(msgs []Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		if len(m.Images) == 0 {
			out = append(out, map[string]any{"role": m.Role, "content": m.Content})
			continue
		}
		parts := make([]map[string]any, 0, len(m.Images)+1)
		if m.Content != "" {
			parts = append(parts, map[string]any{"type": "text", "text": m.Content})
		}
		for _, im := range m.Images {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": "data:" + im.MediaType + ";base64," + im.Data},
			})
		}
		out = append(out, map[string]any{"role": m.Role, "content": parts})
	}
	return out
}
```

- [ ] **Step 5: Wire the builders into the providers**

In `internal/llm/anthropic.go`, change the payload line:

```go
		"messages":   req.Messages, // user/assistant only
```

to:

```go
		"messages":   anthropicMessages(req.Messages),
```

In `internal/llm/openai.go`, change:

```go
	body, _ := json.Marshal(map[string]any{
		"model":      req.Model,
		"messages":   msgs,
		"stream":     true,
		"max_tokens": req.MaxTokens,
	})
```

to:

```go
	body, _ := json.Marshal(map[string]any{
		"model":      req.Model,
		"messages":   openaiMessages(msgs),
		"stream":     true,
		"max_tokens": req.MaxTokens,
	})
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/llm/`
Expected: PASS (new tests + existing provider tests still green — text-only serialization unchanged).

- [ ] **Step 7: Commit**

```bash
git add internal/llm/
git commit -m "feat(llm): optional image blocks on messages (Anthropic + OpenAI)"
```

---

## Task 2: `bake.js` — sketch/image → base64 image

**Files:**
- Create: `frontend/src/lib/bake.js`

**Interfaces:**
- Produces: `bakeSketch(cell) → Promise<{mediaType,data}>`, `bakeImage(cell) → Promise<{mediaType,data}>`. Consumed by `LLMCell` (Task 3) and `TranscribeBar` (Task 4).

- [ ] **Step 1: Create the module**

Create `frontend/src/lib/bake.js`:

```js
// bake.js — render a sketch or image cell to a downscaled base64 image for
// multimodal LLM turns. Returns { mediaType, data } with data = base64 (no prefix).
import { renderStrokes } from './sketch-render.js'

const MAX_EDGE = 1568 // Anthropic's recommended max long edge
const SKETCH_BG = '#0b1220' // dark bg preserves contrast for light strokes

function loadImage(src) {
  return new Promise((resolve, reject) => {
    const im = new Image()
    im.onload = () => resolve(im)
    im.onerror = () => reject(new Error('image not found: ' + src))
    im.src = src
  })
}

export async function bakeSketch(cell) {
  const strokes = cell.strokes ?? []
  if (!strokes.length) throw new Error('Sketch is empty — nothing to send.')
  const lw = cell.logicalW || 800
  const lh = cell.logicalH || 600
  const s = Math.min(1, MAX_EDGE / Math.max(lw, lh))
  const w = Math.max(1, Math.round(lw * s))
  const h = Math.max(1, Math.round(lh * s))
  const strokeCanvas = document.createElement('canvas')
  renderStrokes(strokeCanvas.getContext('2d'), strokes, { width: w, height: h, dpr: 1, logicalW: lw, logicalH: lh })
  const out = document.createElement('canvas')
  out.width = w
  out.height = h
  const ctx = out.getContext('2d')
  ctx.fillStyle = SKETCH_BG
  ctx.fillRect(0, 0, w, h)
  ctx.drawImage(strokeCanvas, 0, 0)
  return { mediaType: 'image/png', data: out.toDataURL('image/png').split(',')[1] }
}

// Rotate the context so drawing the natural image at (0,0) fills the rotated
// frame RW×RH (r ∈ {0,90,180,270} clockwise). Mirrors image-view.js geometry.
function applyRotation(ctx, r, RW, RH) {
  if (r === 90) { ctx.translate(RW, 0); ctx.rotate(Math.PI / 2) }
  else if (r === 180) { ctx.translate(RW, RH); ctx.rotate(Math.PI) }
  else if (r === 270) { ctx.translate(0, RH); ctx.rotate(3 * Math.PI / 2) }
}

export async function bakeImage(cell) {
  const img = await loadImage(`/media/${cell.asset}`)
  const W = img.naturalWidth
  const H = img.naturalHeight
  const r = (((Math.round((cell.rotation || 0) / 90) * 90) % 360) + 360) % 360
  const swap = r === 90 || r === 270
  const RW = swap ? H : W
  const RH = swap ? W : H
  const c = cell.crop && cell.crop.w > 0 && cell.crop.h > 0 ? cell.crop : { x: 0, y: 0, w: 1, h: 1 }
  const cw = c.w * RW
  const ch = c.h * RH
  const s = Math.min(1, MAX_EDGE / Math.max(cw, ch))
  const outW = Math.max(1, Math.round(cw * s))
  const outH = Math.max(1, Math.round(ch * s))
  const canvas = document.createElement('canvas')
  canvas.width = outW
  canvas.height = outH
  const ctx = canvas.getContext('2d')
  ctx.scale(s, s)               // output px → rotated-frame px
  ctx.translate(-c.x * RW, -c.y * RH) // pan so the crop origin maps to (0,0)
  applyRotation(ctx, r, RW, RH) // rotate the frame, then draw the natural image at 0,0
  ctx.drawImage(img, 0, 0)
  return { mediaType: 'image/jpeg', data: canvas.toDataURL('image/jpeg', 0.85).split(',')[1] }
}
```

- [ ] **Step 2: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds (module standalone; consumed in later tasks).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/bake.js
git commit -m "feat(notebook): bake sketch/image cells to base64 images"
```

---

## Task 3: Attach sketch/image to the LLM cell

**Files:**
- Modify: `frontend/src/components/notebook/LLMCell.svelte`

**Interfaces:**
- Consumes: `bakeSketch`/`bakeImage` (Task 2); the `Images` field understood by the Go providers (Task 1).

- [ ] **Step 1: Import the bakers**

Add to the imports in `LLMCell.svelte`:

```js
  import { bakeSketch, bakeImage } from '../../lib/bake.js'
```

- [ ] **Step 2: Add sketch/image to the attach picker**

In the attach markup, change BOTH the `{#if}` guard and the `{#each}` filter from:

```svelte
  {#if !readOnly && cells.filter((c) => c.id !== cell.id && (c.type === 'n4l' || c.type === 'llm' || c.type === 'graph' || c.type === 'compute')).length}
```
```svelte
      {#each cells.filter((c) => c.id !== cell.id && (c.type === 'n4l' || c.type === 'llm' || c.type === 'graph' || c.type === 'compute')) as c}
```

to include `sketch`/`image` (both occurrences):

```svelte
  {#if !readOnly && cells.filter((c) => c.id !== cell.id && ['n4l', 'llm', 'graph', 'compute', 'sketch', 'image'].includes(c.type)).length}
```
```svelte
      {#each cells.filter((c) => c.id !== cell.id && ['n4l', 'llm', 'graph', 'compute', 'sketch', 'image'].includes(c.type)) as c}
```

And extend the label expression in the picker `<label>` so sketch/image get a name (change the existing ternary that ends with `: 'llm'`):

```svelte
 {c.type === 'n4l' ? 'draft' : c.type === 'graph' ? 'graph' : c.type === 'compute' ? 'compute' : c.type === 'sketch' ? 'sketch' : c.type === 'image' ? 'image' : 'llm'} {c.id.slice(-4)}
```

- [ ] **Step 3: Make `send()` async and inject baked images into the first user message**

Replace the whole `send()` function with:

```js
  async function send() {
    const text = input.trim()
    if (!text || streaming) return
    lastSent = text
    cell.messages = [...cell.messages, { role: 'user', content: text }]
    cell = cell
    input = ''
    error = ''
    partial = ''
    streaming = true

    // Bake attached sketch/image cells; attach them once to the first user message.
    let sendMessages = cell.messages
    try {
      const images = []
      for (const c of attached) {
        if (c.type === 'sketch') images.push(await bakeSketch(c))
        else if (c.type === 'image') images.push(await bakeImage(c))
      }
      if (images.length) {
        const idx = cell.messages.findIndex((m) => m.role === 'user')
        sendMessages = cell.messages.map((m, i) => (i === idx ? { ...m, images } : m))
      }
    } catch (e) {
      // Bake failed — undo the just-added user message and abort.
      cell.messages = cell.messages.slice(0, -1)
      cell = cell
      error = String(e?.message ?? e)
      streaming = false
      return
    }

    const system = buildSystem(cell.mode, context, attached)
    turn = streamTurn({
      profileId: cell.profileId,
      system,
      messages: sendMessages,
      onDelta: (t) => { partial += t },
      onDone: () => { finish(partial) },
      onError: (m) => { error = m; abandonTurn() },
    })
  }
```

(`buildSystem` already ignores cell types it has no text branch for, so attached sketch/image cells add nothing to the system prompt — they ride as images only.)

- [ ] **Step 4: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 5: Manual smoke**

`PATH="$PATH:$(go env GOPATH)/bin" make build` then `./build/bin/texere`: add a sketch and an LLM cell, draw something, attach the sketch to the LLM cell, ask "what did I draw?" — confirm the model responds about the drawing. (Requires a configured vision profile.)

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/notebook/LLMCell.svelte
git commit -m "feat(notebook): attach sketch/image cells to an LLM cell as images"
```

---

## Task 4: Transcribe (shared bar + cells + onInsert)

**Files:**
- Modify: `frontend/src/lib/llm.js`, `frontend/src/components/notebook/NotebookPane.svelte`, `SketchCell.svelte`, `ImageCell.svelte`
- Create: `frontend/src/components/notebook/TranscribeBar.svelte`

**Interfaces:**
- Consumes: `bakeSketch`/`bakeImage` (Task 2), `streamTurn`/`GetLLMConfig`, `newDraftCell`.

- [ ] **Step 1: Add `TRANSCRIBE_SYSTEM` to `llm.js`**

Add near the other system-prompt constants in `frontend/src/lib/llm.js`:

```js
export const TRANSCRIBE_SYSTEM =
  'You transcribe images of handwritten or printed notes into clean text. ' +
  'Transcribe all text faithfully, preserving structure (headings, lists, indentation) as Markdown. ' +
  'If the image contains a diagram, flowchart, chart, mind-map, or graph, reconstruct it as a fenced ' +
  '```mermaid code block that best represents its structure, in addition to any surrounding text. ' +
  'Output only the transcription — no preamble or commentary.'
```

- [ ] **Step 2: Create `TranscribeBar.svelte`**

Create `frontend/src/components/notebook/TranscribeBar.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import { GetLLMConfig } from '../../../wailsjs/go/main/App'
  import { streamTurn, TRANSCRIBE_SYSTEM } from '../../lib/llm.js'

  export let cell
  export let bake            // async (cell) => { mediaType, data }
  export let onInsert = () => {} // (text) => insert an N4L draft below
  export let onChange = () => {} // persist the cell (profile choice)
  export let readOnly = false

  let profiles = []
  let busy = false
  let err = ''

  onMount(async () => { try { const c = await GetLLMConfig(); profiles = c.profiles ?? [] } catch { /* not configured */ } })

  function setProfile(id) { cell.transcribeProfileId = id || null; cell = cell; onChange(cell) }

  async function transcribe() {
    if (busy) return
    busy = true
    err = ''
    let img
    try { img = await bake(cell) } catch (e) { err = String(e?.message ?? e); busy = false; return }
    let acc = ''
    streamTurn({
      profileId: cell.transcribeProfileId,
      system: TRANSCRIBE_SYSTEM,
      messages: [{ role: 'user', content: 'Transcribe this image.', images: [img] }],
      onDelta: (t) => { acc += t },
      onDone: () => { busy = false; if (acc.trim()) onInsert(acc) },
      onError: (m) => { err = m; busy = false },
    })
  }
</script>

{#if !readOnly}
  <div class="tb">
    <select value={cell.transcribeProfileId || ''} on:change={(e) => setProfile(e.target.value)} disabled={busy || !profiles.length} title="Model for transcription">
      <option value="">active default</option>
      {#each profiles as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <button on:click={transcribe} disabled={busy || !profiles.length}>{busy ? 'Transcribing…' : 'Transcribe'}</button>
    {#if err}<span class="tb-err">{err}</span>{/if}
  </div>
{/if}

<style>
  .tb { display: flex; align-items: center; gap: 6px; padding: 5px 8px; border-top: 1px solid #1c2a3a; }
  .tb select, .tb button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; font-size: 12px; cursor: pointer; }
  .tb button:disabled, .tb select:disabled { opacity: 0.5; cursor: default; }
  .tb-err { color: #ff8080; font-size: 11px; }
</style>
```

- [ ] **Step 3: Add `insertAfter` + `onInsert` in `NotebookPane.svelte`**

Add the function near `updateCellObj`:

```js
  function insertAfter(i, newCell) { cells = [...cells.slice(0, i + 1), newCell, ...cells.slice(i + 1)]; scheduleSave() }
```

And extend the `<svelte:component>` prop list (it already passes `onChange`/`onPush`/`onDelete`) with:

```svelte
            onInsert={(text) => insertAfter(i, newDraftCell(text))}
```

(`newDraftCell` is already imported in `NotebookPane`.)

- [ ] **Step 4: Mount `TranscribeBar` in `SketchCell.svelte`**

Add imports:

```js
  import TranscribeBar from './TranscribeBar.svelte'
  import { bakeSketch } from '../../lib/bake.js'
```

Add the prop (with the other `export let`s):

```js
  export let onInsert = () => {}
```

Add the bar immediately after the toolbar `</div>` (the `.sk-bar` block), inside `{#if !readOnly}` is NOT needed — `TranscribeBar` self-guards on `readOnly`:

```svelte
  <TranscribeBar {cell} bake={bakeSketch} {onInsert} onChange={() => onChange(cell)} {readOnly} />
```

Place it just before `<div class="sk-wrap" ...>`.

- [ ] **Step 5: Mount `TranscribeBar` in `ImageCell.svelte`**

Add imports:

```js
  import TranscribeBar from './TranscribeBar.svelte'
  import { bakeImage } from '../../lib/bake.js'
```

Add the prop:

```js
  export let onInsert = () => {}
```

Add the bar right after the toolbar (`.im-bar`) block, before the image display block:

```svelte
  <TranscribeBar {cell} bake={bakeImage} {onInsert} onChange={() => onChange(cell)} {readOnly} />
```

- [ ] **Step 6: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds (pulls `TranscribeBar` into both cells).

- [ ] **Step 7: Manual smoke**

`make build` then `./build/bin/texere`: on a sketch of handwriting, pick a vision model, click Transcribe → a new N4L draft cell appears below with the text; on a photo of a flowchart, confirm a ```mermaid block.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/lib/llm.js frontend/src/components/notebook/TranscribeBar.svelte frontend/src/components/notebook/NotebookPane.svelte frontend/src/components/notebook/SketchCell.svelte frontend/src/components/notebook/ImageCell.svelte
git commit -m "feat(notebook): Transcribe sketch/image to an N4L draft (model picker + Mermaid)"
```

---

## Task 5: Manual test plan + full verification

**Files:**
- Create: `docs/specs/2026-07-24-processing-notebook-slice6a-manual-test-plan.md`

- [ ] **Step 1: Write the manual test plan**

Create the file:

```markdown
# Processing Notebook — Slice 6a (Multimodal) Manual Test Plan

Prereqs: open an extract; configure at least one vision-capable LLM profile (e.g. an Anthropic Claude profile) in ⚙ Models.

## TC-1 Attach a sketch to an LLM cell
1. Add a Sketch cell; draw a simple shape. Add an LLM cell.
2. In the LLM cell's attach row, check the sketch; ask "what did I draw?".
   - EXPECT: the model answers describing the drawing.

## TC-2 Attach an image
1. Add an Image cell; import a photo. Attach it to an LLM cell; ask about it.
   - EXPECT: the model answers about the photo (rotation/crop are reflected).

## TC-3 Transcribe handwriting → N4L draft
1. On a sketch of handwritten words, pick a model, click Transcribe.
   - EXPECT: a new N4L draft cell appears directly below with the transcribed text.

## TC-4 Transcribe a diagram → Mermaid
1. Draw/photograph a small flowchart; Transcribe.
   - EXPECT: the draft contains a ```mermaid code block approximating the diagram.

## TC-5 Model picker
1. Change the dropdown to a different profile; Transcribe again.
   - EXPECT: the request uses the selected model (different result/latency).

## TC-6 Errors
1. Transcribe with a non-vision model.
   - EXPECT: an error message; no draft inserted.
2. Text-only LLM chat still works (attach nothing; converse) — regression check.

## TC-7 Persistence
1. Attach a sketch, chat, restart the app, reopen.
   - EXPECT: the conversation persists; no image bytes bloat (the notebook still opens fast); re-asking re-bakes the image.
```

- [ ] **Step 2: Backend suite + full build**

Run: `go test ./...`
Expected: all PASS (includes `internal/llm` wire tests).

Run: `PATH="$PATH:$(go env GOPATH)/bin" make build`
Expected: builds `build/bin/texere`.

- [ ] **Step 3: Execute the manual test plan**

Walk TC-1 … TC-7 in `./build/bin/texere`.

- [ ] **Step 4: Commit**

```bash
git add docs/specs/2026-07-24-processing-notebook-slice6a-manual-test-plan.md
git commit -m "docs(test-plan): Processing Notebook Slice 6a manual test plan"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** message type + provider serialization with regression-safe text path (T1), canvas baking of sketch/image with rotation+crop and downscale (T2), attach picker + images-once-on-first-user-message (T3), model-picker + Transcribe + Mermaid prompt + N4L-draft output via `onInsert` (T4), manual plan + full verification (T5). Deferred (Mermaid-to-picture, 6b/6c/6d, DB-stored images) out of scope.
- **Placeholder scan:** no TBD/TODO; every code step is complete.
- **Type consistency:** `llm.Image{MediaType,Data}` ↔ `bake.js` `{mediaType,data}` ↔ message JSON `images:[…]`; `anthropicMessages`/`openaiMessages` output shapes match the T1 tests; `streamTurn({profileId,system,messages})` unchanged (images ride on message objects); `onInsert(text)` → `newDraftCell(text)` in `NotebookPane`; `transcribeProfileId` persisted via the cells' `onChange`.
- **Compile-sequencing:** `bake.js` (T2) and `TranscribeBar` (T4) are tree-shaken until imported by the cells; T3/T4 builds pull them in, and T5's `make build` embeds everything. The Go `Images` field (T1) is required before the frontend sends `images`.
```
