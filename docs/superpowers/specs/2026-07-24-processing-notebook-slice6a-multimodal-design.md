# Processing Notebook — Slice 6a: Multimodal attach + transcription

**Date:** 2026-07-24
**Status:** Approved design
**Feature area:** Processing Notebook (incremental reading)
**Slice:** 6a of ~6 — the first cut of Slice 6 (Multimodal + glue), which is
decomposed into 6a (this), 6b push-as-image, 6c `\notebooks` search, 6d
learning-loop glue.

## Summary

Make sketch and image cells usable inside the reasoning loop. Two capabilities on
one foundation:

1. **Foundation — multimodal messages.** Extend the LLM message type so a user
   turn can carry **image blocks**. Sketch cells render to a PNG; image cells bake
   their rotation+crop to a bitmap; both are downscaled and base64-encoded, and
   translated per provider (Anthropic `image` source / OpenAI-compatible
   `image_url`).
2. **Attach (conversation).** Sketch/image cells become attachable to an LLM cell;
   their baked image is included in the request so you can *chat about* the
   drawing/photo.
3. **Transcribe (one-shot).** A model dropdown + **Transcribe** button on the
   sketch/image cell runs a one-shot vision turn (faithful text; Mermaid for
   diagrams) and drops the result into a new N4L draft cell below.

No image bytes are stored in the notebook DB — images are re-baked from the source
cells at send time.

## Decisions made during brainstorming

- **Transcribe output → a new N4L draft cell** inserted below the source cell
  (feeds the produce-N4L loop; holds the transcription and any `mermaid` block as
  editable text).
- **Model is the user's choice per transcription.** The sketch/image cell shows a
  dropdown of configured LLM profiles; the user picks the best OCR/vision model.
  No hard capability gate — a model that can't do vision surfaces its error.
- **Mermaid reconstruction** is requested in the transcription prompt: if the
  image contains a diagram/flowchart/chart/mind-map/graph, reconstruct it as a
  fenced `mermaid` code block alongside the text.
- **Images ride once on the first user message** of a request (the API is
  stateless; one copy keeps the image in context for the whole conversation
  without duplicating it per turn).
- **No base64 in the notebook DB** — the LLM cell stores only attachment cell-ids
  (as today); images are baked from the referenced cells at send time.

## Scope

**In scope**

- `llm.Message` (+ `StartTurnReq`) gains optional `Images []Image`; Anthropic and
  OpenAI-compatible providers emit the correct per-message content array.
- Frontend baking helpers: sketch strokes → PNG; image cell (rotation+crop) →
  JPEG; downscale to a max edge (~1568px), base64.
- Sketch/image cells added to the LLM cell attach picker; on send, attached
  sketch/image images are baked and attached to the first user message.
- A **Transcribe** button + profile dropdown on sketch and image cells; one-shot
  turn with a transcription system prompt; output → new N4L draft cell via a new
  `onInsert` callback in `NotebookPane`.

**Out of scope (later slices / deferred)**

- Rendering the returned Mermaid to a picture (it stays as text in the draft).
- Push-as-image (6b), `\notebooks` search (6c), learning-loop glue (6d).
- Image editing beyond the existing rotate/crop; PDF/multi-page; audio.
- Storing baked images in the DB, prompt-caching tuning, cost metering.

## Foundation — multimodal message type

`internal/llm/llm.go`:

```go
type Image struct {
    MediaType string `json:"mediaType"` // "image/png" | "image/jpeg"
    Data      string `json:"data"`      // base64 (no data: prefix)
}

type Message struct {
    Role    string  `json:"role"`
    Content string  `json:"content"`
    Images  []Image `json:"images,omitempty"` // user turns only
}
```

- **Anthropic provider** (`anthropic.go`): stop passing `req.Messages` verbatim.
  Build each message's `content`: a plain string when it has no images; otherwise
  an array `[{type:"text",text:Content}, {type:"image",source:{type:"base64",
  media_type:MediaType,data:Data}}, …]`.
- **OpenAI-compatible provider** (`openai.go`): build `content` as an array
  `[{type:"text",text:Content}, {type:"image_url",image_url:{url:"data:<MediaType>;
  base64,<Data>"}}, …]` when images are present; plain string otherwise.
- `StartTurnReq.Messages` already carries `[]llm.Message`, so images flow through
  `StartLLMTurn` unchanged structurally; only the providers' serialization changes.
- Text-only turns produce byte-identical requests to today (regression-safe).

## Baking (frontend, canvas)

New `frontend/src/lib/bake.js` (pure-ish; needs canvas + image load):

- `bakeSketch(cell) → { mediaType:'image/png', data }`: render `cell.strokes` with
  the existing `renderStrokes` into an offscreen canvas sized to the sketch's
  logical dimensions (capped at the max edge), on the sketch's **dark background**
  (preserves contrast for light strokes), `toDataURL('image/png')`, strip prefix.
- `bakeImage(cell) → { mediaType:'image/jpeg', data }`: load `/media/${cell.asset}`
  into an `Image`, draw into a canvas that applies `cell.rotation` + `cell.crop`
  (canvas transforms mirroring `image-view.js`'s geometry), downscaled to the max
  edge, `toDataURL('image/jpeg', 0.85)`.
- `MAX_EDGE = 1568` (Anthropic's recommended max long edge). Both are async
  (image load / decode). Missing asset → reject with an error surfaced to the UI.

## Attach (conversation)

- The LLM cell attach picker (`LLMCell.svelte`) adds `sketch` and `image` to its
  cell-type filter. Attaching stores the cell id in `cell.attachments` (as today).
- On **send**: text attachments still feed `buildSystem` (system prompt).
  Additionally, for each attached `sketch`/`image` cell, `bake*` it and collect the
  images; attach them to the **first user message** of `cell.messages` for this
  request (transiently, at send time — not persisted). `streamTurn` passes the
  messages (now possibly carrying `images`) to `StartLLMTurn`.
- The image is re-baked each request from the current source cell, so edits to the
  sketch/photo are reflected; nothing image-related is written to `cells_json`.

## Transcribe (one-shot)

- `SketchCell.svelte` and `ImageCell.svelte` gain a compact row: a **profile
  dropdown** (from `GetLLMConfig().profiles`, mirroring `LLMCell`) and a
  **Transcribe** button. The selected profile id persists on the cell
  (`transcribeProfileId`) so the choice is remembered.
- On click: `bake*` the cell → build one user message `{role:'user',
  content:'Transcribe this image.', images:[baked]}` → `streamTurn({ profileId,
  system: TRANSCRIBE_SYSTEM, messages })`. Accumulate deltas; on done, call
  `onInsert(newDraftCell(accumulatedText))` to insert an N4L draft cell directly
  below. A "transcribing…" state disables the button; errors → toast/inline.
- `TRANSCRIBE_SYSTEM` (new constant in `llm.js`): "You transcribe images of
  handwritten or printed notes into clean text. Transcribe all text faithfully,
  preserving structure (headings, lists, indentation) as Markdown. If the image
  contains a diagram, flowchart, chart, mind-map, or graph, reconstruct it as a
  fenced ```mermaid code block that best represents its structure, in addition to
  any surrounding text. Output only the transcription — no preamble."

## NotebookPane — `onInsert`

- Add `function insertAfter(i, newCell) { cells = [...cells.slice(0,i+1), newCell,
  ...cells.slice(i+1)]; scheduleSave() }` and pass `onInsert={(nc) =>
  insertAfter(i, nc)}` to each `svelte:component`. All existing cells ignore the
  unused prop; sketch/image cells use it for transcription output.

## Providers & capability

- Vision requires a vision-capable model. **Not gated** — the user selects the
  model; a non-vision model returns an API error surfaced via the existing
  `llm:error` toast path. Anthropic Claude profiles are vision-capable; local
  OpenAI-compatible endpoints vary.

## Error handling

- Missing/edited-away asset at bake time → error toast, no turn started.
- Bake failure (canvas/tainted/decode) → error toast.
- No profiles configured → the dropdown is empty and Transcribe is disabled with a
  hint to configure a model.
- Model without vision → provider/API error via `llm:error`.
- Empty transcription result → no draft cell inserted (or an empty one — insert
  only when non-empty).

## Testing

- **Go:** provider unit tests asserting the emitted request JSON — a text-only
  message still serializes `content` as a string (regression), and a message with
  `Images` serializes the correct Anthropic `content` array (text + `image`/
  base64) and OpenAI `image_url` data-URI. (httptest, as existing provider tests.)
- **Frontend:** no JS test runner — `cd frontend && npm run build` plus a manual
  test plan: attach a sketch and an image to an LLM cell and confirm the model
  answers about the picture; Transcribe a handwritten sketch and a photo of text →
  a new N4L draft cell appears with the text; transcribe a hand-drawn flowchart →
  a `mermaid` block; switch models via the dropdown; error on a non-vision model.
- Full build: `go test ./...` and `make build`.

## Success criteria

- A sketch or image attached to an LLM cell is seen by the model (it answers
  questions about the picture) across the conversation.
- Transcribe produces an editable N4L draft cell with faithful text, and a
  `mermaid` block for diagrams, using the model chosen in the dropdown.
- Text-only LLM turns are unchanged; no image bytes are persisted in the notebook.
- No new Go/npm dependencies; no CGo.

## Dependencies

- **New:** none. Providers use the standard library; baking uses the browser
  Canvas API and the existing `renderStrokes`/`image-view` geometry.
- **Reused:** Slice 2 LLM cell + `StartLLMTurn`/`streamTurn`/`GetLLMConfig`; Slice
  4 sketch (`renderStrokes`, logical dims); Slice 5 image (`/media` handler,
  rotation/crop); the `cellRegistry`/`NotebookPane`/`cells_json` persistence path.
