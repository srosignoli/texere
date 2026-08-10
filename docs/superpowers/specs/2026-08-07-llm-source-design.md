# LLM as a source: chat your way to a study document

**Date:** 2026-08-07
**Status:** Proposed — awaiting approval
**Feature area:** Study mode → Read focus → sources

## Summary

A fourth way to acquire a reading source, beside `+ Text`, `+ PDF` and `+ Web`:
ask a configured LLM, refine the answers over a few turns, then save the
exchange as a markdown document that joins the reading list like any other.

## Motivation

The reading library can only hold material that already exists somewhere — a
file on disk, a PDF, a web article. Yet plenty of what you want to study has no
single good source: a comparison across three papers, a summary of a topic you
only need the shape of, an explanation pitched at your level.

The app already talks to LLMs in the notebook's LLM cell and in query
translation, so the capability is there; what is missing is a route from a
model's answer into the incremental-reading loop, where it can be read,
extracted from, and turned into cards.

## Grounding in the current implementation

Surveyed before designing:

- **`llm_methods.go:115`** — `StartLLMTurn(StartTurnReq{ProfileID, System,
  Messages})` streams a response in a goroutine, emitting `llm:delta`,
  `llm:done` and `llm:error` keyed by a returned `streamId`.
  `CancelLLMTurn(streamID)` (`:153`) stops one.
- **`LLMCell.svelte`** already drives that API as a multi-turn chat with a
  profile picker (`:184`), a message array (`:90`), and a failed-turn rollback
  that drops the pending user message (`:140-141`). The chat surface this
  feature needs is a variation on code that already works.
- **`ImportWebSource` (`reading_methods.go:274`)** is the exact shape of what
  the backend must do: synthesise content in memory, write it under `sources/`
  with a slugged unique name, scaffold `reading/<slug>.n4l` through
  `content.CreateReadingNotes`, and register a `ReadingElement` whose `ProjLen`
  comes from the text projection.
- **`ReadingView.svelte:174,176`** branches on `medium === 'pdf'` and
  `'web'` only; every other medium falls through to `TextSourcePane`. A new
  medium therefore costs no pane work.
- **`ImportTextSource`** already accepts `*.md`, so markdown is an understood
  source format with a working projection.
- **`AskBox.svelte`** renders nothing at all when no LLM profile is configured.
  This feature follows that rule rather than showing a dead control.
- **`StudySurface.svelte`** holds `peekAt` and a `switchTo(r, anchor)` that
  awaits `readingView.flush()` before changing what the reading area shows.
  Anything else that takes over that area must go through the same path.

## Decisions

- **The saved document is the transcript shaped as an article**: each user
  question becomes a `##` heading and its answer the body. Rejected: a verbatim
  `You:`/`Assistant:` transcript, which bakes your half-formed questions into
  text you will later read and extract from; and a second LLM call to rewrite
  the conversation into prose, which costs time at save and silently drops
  detail you may have asked for on purpose.
- **The document is marked as model-generated** — `Medium: "llm"`, surfaced as
  an `llm` tag on its sidebar row. A PDF's provenance is self-evident and a web
  article keeps its `SourceURL`; an LLM document saved as plain text would be
  indistinguishable from a file you wrote yourself. That distinction matters
  more here than elsewhere, because a model-written study document can be
  confidently wrong and you will be turning it into flashcards.
- **The chat survives switching documents, and dies with Study mode.** It lives
  in `StudySurface`'s state rather than inside the reading area's `{#key}`
  block, so you can read something, come back, and keep going. Rejected: a
  draft persisted to disk, which needs a storage format, a listing, and a
  policy for drafts you never return to — none of it justified until the
  feature has proven itself.
- **Switching model mid-chat affects only the next turn.** Earlier answers
  stand; nothing is re-run behind your back. Every model used is recorded in
  the document's footer, so a document answered by two models says so.
- **The system prompt frames the output as study material** — complete,
  structured, factual prose rather than a conversational reply — because the
  result is going to be read incrementally and turned into cards.
- **No passage-seeded chat.** Asking about something you are reading is what
  the notebook's LLM cell is for. This feature is a source-acquisition channel,
  in the same role as importing a PDF or a Wikipedia article.

## Architecture

Three units with clear boundaries.

### `frontend/src/lib/chat-markdown.js` (new)

```
chatToMarkdown(title, messages, models) -> string
```

Pure and dependency-free, so it runs under `node --test`. This repo has **no
Svelte component test harness**, so shaping logic left inside a component would
have no tests at all.

Produces:

```markdown
# Spaced repetition intervals

## How does SM-2 pick the next interval?

SM-2 multiplies the previous interval by an ease factor…

## What happens on a lapse?

The interval resets to 1 day…

---
*Generated with claude-opus-5, gpt-4o — 2026-08-07*
```

A trailing user message with no answer yet is dropped rather than saved as an
empty section. Answer bodies pass through untouched, markdown and all.

### `frontend/src/components/LLMChatPane.svelte` (new)

The chat surface, rendered where `ReadingView` normally sits. Takes the chat
object as a prop and emits changes; owns no persistence. Reuses the pattern
`LLMCell.svelte` already proves: `StartLLMTurn`, the three `llm:*` events, a
Stop button through `CancelLLMTurn`, a profile `<select>`, and the rollback
that drops a pending user message when a turn fails.

Its one addition is a **Create document** button, enabled once at least one
answer has completed. It asks for a title, prefilled with the first line of the
first user message trimmed to 60 characters — a starting point to edit, not a
guess to accept blindly.

### `CreateLLMSource(title, markdown string) (*ReadingView, error)` (new binding)

Modelled directly on `ImportWebSource`:

1. slug the title; `uniqueDest` a `sources/<slug>.md`
2. write the markdown
3. `content.CreateReadingNotes(notesAbs, srcRel, title)`
4. `UpsertReading` with `Medium: "llm"`, `Position: "char=0"`, and `ProjLen`
   from `utf8.RuneCountInString` of the markdown
5. clean up the source file if any later step fails, as the import methods do

Rejects an empty title or empty markdown rather than creating a source with
nothing in it.

### Wiring in `StudySurface.svelte`

- `chat` state: `null`, or `{ messages, profileId, models }`.
- `+ LLM` in the sidebar's import row, rendered only when a profile exists.
- A `✦ Chat` row at the top of the sidebar list while a chat is live; clicking
  it returns to the chat.
- Opening the chat goes through the existing `readingView?.flush?.()` path, so
  the document you were reading still records its progress.
- On Create, the returned reading is opened, and the chat is cleared.

## Error handling

| Case | Behaviour |
|---|---|
| No LLM profile configured | `+ LLM` is not rendered at all |
| Stream error | Pending turn dropped, error shown, retry available |
| Create with no completed answer | Button disabled |
| `CreateLLMSource` fails | Error shown, **chat left intact** — never lose the conversation to a failed save |
| Closing Study with unsaved turns | Confirm first |
| Title collides with an existing source | `uniqueDest` suffixes `-2`, `-3`, … |

## Testing

**Go:** `CreateLLMSource` writes the `.md` where expected, scaffolds the notes
file, registers a reading with `Medium: "llm"` and a `ProjLen` matching the
markdown's rune count, suffixes a colliding slug, and refuses an empty title or
body without leaving a stray file behind.

**JS:** `chatToMarkdown` — a single turn, several turns in order, a trailing
unanswered question dropped, markdown inside an answer passed through
unmodified, the model footer listing each model once, and an empty message list.

**Manual:** its own test-plan document, covering what has no harness — the
streaming itself, switching model mid-chat, the chat surviving a document
switch, the `llm` tag appearing on the new row, and the new document being
readable and extractable like any other source.

## Out of scope

- Starting a chat from a passage you are reading — the notebook's LLM cell.
- Resuming a chat after the app restarts.
- Giving the model access to your collection.
- Editing the saved document from the chat; it is an ordinary markdown source
  and the editor already handles it.
- Regenerating or extending a document that has already been created.
