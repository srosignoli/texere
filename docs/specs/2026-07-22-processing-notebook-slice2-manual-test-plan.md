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
