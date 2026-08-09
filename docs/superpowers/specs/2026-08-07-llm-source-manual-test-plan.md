# LLM as a source — manual test plan

**Design:** `2026-08-07-llm-source-design.md`
**Build:** `make dev`

**This document is the verification gate for everything interactive.** This
repository has **no Svelte component test harness**, so `LLMChatPane.svelte` and
the Study-mode wiring have **zero automated coverage** and have never been
watched running. On the previous feature the interactive surface shipped with a
control that did nothing, and only a manual pass would have caught it. Treat any
failure below as a new finding.

What *is* automated: `chat-markdown.test.js` covers the document shaping in ten
cases — a single exchange, several in order, an unanswered trailing question
dropped, a multi-line question folded into one heading line, markdown inside an
answer passing through untouched, the model footer de-duplicated, no footer
without models, an empty conversation, and `defaultTitle`'s first-line and
60-character behaviour. `reading_methods_test.go` covers `CreateLLMSource`:
`Medium: "llm"`, the file contents, the scaffolded notes, an exact `ProjLen`,
a colliding title getting its own filename, and empty input refused without
leaving a stray file. Everything below is what those cannot reach.

## Three things that are deliberate — do not report them as bugs

- **The chat is lost when Study mode closes.** It is held in memory only. Both
  exits (`×` and Esc) ask first when there are turns to lose, but there is no
  draft on disk and no way to resume after quitting the app.
- **Changing the model does not re-run anything.** Earlier answers stand as
  they were; the new model answers the next turn onward. Every model used is
  named in the saved document's footer.
- **The saved file is an ordinary markdown source.** Once created it has no
  further link to the chat — edit it in the editor like any other document.

## Before you start

You need at least one LLM profile configured (Settings → LLM). Two profiles
with different models makes TC-4 and TC-8 much easier to judge.

**[DB]** items read `<collectionDir>/learning.db` with `sqlite3`:
```
sqlite3 <collectionDir>/learning.db \
  "SELECT id, title, medium, proj_len FROM reading_elements WHERE medium='llm';"
```

## TC-1 The entry point appears only when it can work

With **no** LLM profile configured, open Study (Ctrl+Shift+S) → Read.

**Expect:** the sidebar's import row shows `+ Text`, `+ PDF`, `+ Web` and **no
`+ LLM`**. Configure a profile, reopen Study, and `+ LLM` is now there.

## TC-2 A question streams an answer, and Stop ends it

Click `+ LLM`. Type a question and press Send (or Ctrl+Enter).

**Expect:** the answer streams in progressively under a `MODEL` label. Ask a
second, longer question and press **Stop** part-way.

**Expect:** streaming ends, the partial answer is discarded, and **the question
you asked is back in the input box** ready to resend — the transcript never
ends with a question nobody answered.

## TC-3 A provider error is recoverable

Force a failure — easiest is to edit the profile to an invalid API key, then
ask a question.

**Expect:** a red error line appears in the pane, the pending question returns
to the input, and the earlier turns are untouched. Fix the key and press Send
again: the same question goes through normally.

## TC-4 Switching model mid-chat affects only the next turn

With two profiles configured, ask a question, then change the model in the
top-right picker and ask a second question.

**Expect:** the first answer is unchanged — not regenerated, not reworded. The
second is answered by the new model. Nothing re-runs.

## TC-5 The chat survives reading — the load-bearing case

Build up a chat of **two or more** completed turns. Without saving it, click a
document in the sidebar and read a little. Then click **`✦ Chat in progress`**
at the top of the sidebar.

**Expect:** every turn is exactly as you left it and you can continue the
conversation. The document you were reading keeps its `%` — leaving it for the
chat persists progress the same way switching documents does.

## TC-6 Create is disabled until there is something to save

Open a fresh chat and look at the `Create document` button before asking
anything, and again while an answer is still streaming.

**Expect:** disabled in both cases. It enables once at least one answer has
completed.

## TC-7 Creating the document

With a chat of two or more turns, press **Create document**.

**Expect:** a title prompt, prefilled with the first line of your first
question (cut at 60 characters). Accept or edit it. The chat closes, the new
document appears in the sidebar list with an **`llm`** tag beside any `due`
tag, and it opens for reading.

**[DB]** a row appears with `medium = 'llm'` and a non-zero `proj_len`.

## TC-8 The saved file reads as an article

Open the new document's source file from `<collectionDir>/sources/`.

**Expect:** a `#` title, then each of your questions as a `##` heading with its
answer beneath. Not a `You:`/`Assistant:` transcript. At the end, a `---` rule
and a line naming **every** model that answered — if you switched models in
TC-4, both are listed, each once.

## TC-9 An unanswered question is not saved

Ask a question, let it answer. Type a second question and press Send, then
press **Stop** before any text arrives. Now press Create document.

**Expect:** the saved file contains the answered question only. The one you
stopped is nowhere in it — not as an empty section, not as a dangling heading.

## TC-10 The document behaves like any other source

Read the new document.

**Expect:** it scrolls and the `%` advances as you read; selecting text and
pressing Ctrl+E creates an extract; the extract appears in the document's tree
in the sidebar; closing and reopening resumes where you left off. Nothing about
it behaves differently from an imported `.md`.

## TC-11 Two documents can share a title

Create a document titled `Test note`. Create another with the **same** title.

**Expect:** both exist in the list, and both are readable with their own
content — the second did not overwrite the first.

**[DB]** two rows, different ids, different `ref_path` values.

## TC-12 Starting a second chat asks first

With an unsaved chat of at least one turn, click `+ LLM` again.

**Expect:** a confirm before the existing conversation is discarded.
Cancelling leaves the chat exactly as it was.

## TC-13 Closing Study asks first

With an unsaved chat of at least one turn, press **Esc**, and separately try
the **`×`** button.

**Expect:** both ask before discarding. Cancelling keeps you in Study with the
chat intact. This is the last chance to save a conversation — after this it is
gone, by design.

## What I most want to hear about

- **TC-5 and TC-13.** The chat is held only in memory, so these are the two
  places a conversation can be lost. TC-5 is the one that makes the feature
  usable at all; TC-13 is the one that makes losing it a choice.
- **TC-8's footer.** Whether the models named actually match what answered.
  This is the document's only provenance, and the `llm` tag plus that line are
  what stop a model-written study document from passing for one you wrote.
- **The system prompt's effect.** The model is told to write study material
  rather than converse. If answers still read like chat replies — hedging,
  restating your question, offering to continue — say so; the prompt is one
  string in `LLMChatPane.svelte` and easy to sharpen.
