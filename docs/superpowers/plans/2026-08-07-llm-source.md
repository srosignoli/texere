# LLM Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A fourth way to acquire a reading source — ask a configured LLM, refine over a few turns, then save the exchange as a markdown document that joins the reading list.

**Architecture:** A pure JS function shapes the message array into an article. A chat pane drives the existing `streamTurn` helper and renders in the reading area. One Go binding writes the file and registers the reading, modelled on `ImportWebSource`.

**Tech Stack:** Go 1.25, Wails v2, Svelte 3.59, `node --test`.

## Global Constraints

- **The saved document is the transcript shaped as an article**: each user question becomes a `##` heading, its answer the body. Not a `You:`/`Assistant:` transcript, and no second LLM call to rewrite it.
- **`Medium: "llm"`, never `"text"`.** `ReadingView.svelte:174,176` branches only on `'pdf'` and `'web'`, so anything else already routes to `TextSourcePane` — the new medium costs no pane work and lets the sidebar mark the document as model-generated. That matters here because a model-written study document can be confidently wrong and is about to become flashcards.
- **A failed save never loses the conversation.** The chat stays intact and the error is shown in the pane.
- **The chat lives in `StudySurface`, not inside the reading area's `{#key}` block**, so it survives switching documents. It dies with Study mode, after a confirm.
- **Switching model mid-chat affects only the next turn.** Nothing is re-run. Every model used is recorded in the document footer.
- **No control the user cannot use:** `+ LLM` is not rendered at all when no LLM profile is configured, following `AskBox.svelte`.
- Go: `gofmt`, imports grouped stdlib / external / internal. JS/Svelte: 2-space indent, **no semicolons**, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.
- `wails` is not on PATH; it is at `/home/simone/go/bin/wails`.
- `git add frontend/wailsjs` as a bare directory exits non-zero because `runtime/` is gitignored. Add explicit file paths and confirm with `git show --stat`.
- `TestWatchCollectionAdded` in `internal/content` is a **known pre-existing fsnotify flake** that fails intermittently under load on `main` too. Ignore it; never "fix" it.

## MANDATORY: negative-control every test you write

This project has shipped tests that passed against the exact bug they were written to catch, and a whole feature whose primary control did nothing because no one ran it. Before committing, for each behaviour asserted: break the production code in the specific way the test exists to catch, run the test, confirm it **fails with a message naming the real problem**, then restore and re-run green. Report the actual failure output for the most load-bearing assertion.

Two traps seen on the last feature, both of which produced a *silent* pass:
- A sabotage that does not compile is **not** a negative control. If removing the code under test breaks the build, change the sabotage (e.g. look up a key that cannot match) until it compiles and the test genuinely fails.
- A fixture that cannot distinguish the bug is not a test. If the control does not fail, say so rather than moving on — the fixture is the defect.

## File Structure

- `frontend/src/lib/chat-markdown.js` — shaping logic, dependency-free.
- `frontend/src/lib/chat-markdown.test.js` — its tests.
- `reading_methods.go` — add `CreateLLMSource`.
- `reading_methods_test.go` — its tests.
- `frontend/src/components/LLMChatPane.svelte` — new; the chat surface.
- `frontend/src/components/SourcesSidebar.svelte` — `+ LLM`, the live-chat row, the `llm` tag.
- `frontend/src/components/StudySurface.svelte` — chat state and rendering.
- `docs/superpowers/specs/2026-08-07-llm-source-manual-test-plan.md` — the manual gate.

---

### Task 1: `chatToMarkdown`

**Files:**
- Create: `frontend/src/lib/chat-markdown.js`
- Create: `frontend/src/lib/chat-markdown.test.js`

**Interfaces:**
- Consumes: nothing — dependency-free by design.
- Produces:
  - `chatToMarkdown(title, messages, models, date = new Date()) -> string`
  - `defaultTitle(messages) -> string`

`messages` is `[{ role: 'user'|'assistant', content: string }]`. `models` is an array of model id strings. This lives in `lib/` because the repo has **no Svelte component test harness** — logic left in a `.svelte` file has no tests at all.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/chat-markdown.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { chatToMarkdown, defaultTitle } from './chat-markdown.js'

const D = new Date('2026-08-07T12:00:00Z')

test('one exchange becomes a heading and a body', () => {
  const md = chatToMarkdown('Spaced repetition', [
    { role: 'user', content: 'How does SM-2 pick the next interval?' },
    { role: 'assistant', content: 'It multiplies by an ease factor.' },
  ], ['claude-opus-5'], D)
  assert.match(md, /^# Spaced repetition\n/)
  assert.match(md, /## How does SM-2 pick the next interval\?/)
  assert.match(md, /It multiplies by an ease factor\./)
})

test('several exchanges keep their order', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'first' },
    { role: 'assistant', content: 'answer one' },
    { role: 'user', content: 'second' },
    { role: 'assistant', content: 'answer two' },
  ], [], D)
  assert.ok(md.indexOf('## first') < md.indexOf('## second'), 'questions must stay in order')
  assert.ok(md.indexOf('answer one') < md.indexOf('## second'), 'each answer follows its own question')
})

// The document is a study source, not a record of a conversation in progress.
test('a question still awaiting its answer is dropped', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'answered' },
    { role: 'assistant', content: 'the answer' },
    { role: 'user', content: 'still pending' },
  ], [], D)
  assert.match(md, /## answered/)
  assert.doesNotMatch(md, /still pending/, 'an unanswered question must not become an empty section')
})

test('a multi-line question collapses to a single heading line', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'line one\nline two' },
    { role: 'assistant', content: 'a' },
  ], [], D)
  assert.match(md, /## line one line two/)
  assert.doesNotMatch(md, /## line one\nline two/, 'a newline inside a heading would break the structure')
})

test('markdown inside an answer passes through untouched', () => {
  const body = '- bullet\n\n```js\nconst x = 1\n```\n\n## an inner heading'
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'q' },
    { role: 'assistant', content: body },
  ], [], D)
  assert.ok(md.includes(body), 'answer bodies must not be re-escaped or reflowed')
})

test('the footer lists each model once, in first-use order', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'q' },
    { role: 'assistant', content: 'a' },
  ], ['claude-opus-5', 'gpt-4o', 'claude-opus-5'], D)
  assert.match(md, /\*Generated with claude-opus-5, gpt-4o — 2026-08-07\*/)
})

test('no models means no footer', () => {
  const md = chatToMarkdown('T', [
    { role: 'user', content: 'q' },
    { role: 'assistant', content: 'a' },
  ], [], D)
  assert.doesNotMatch(md, /Generated with/)
})

test('an empty conversation still produces a titled document', () => {
  const md = chatToMarkdown('Just a title', [], [], D)
  assert.match(md, /^# Just a title/)
})

test('defaultTitle takes the first line of the first question, trimmed', () => {
  assert.equal(defaultTitle([{ role: 'user', content: 'What is spaced repetition?\nmore' }]),
    'What is spaced repetition?')
  assert.equal(defaultTitle([]), '')
})

test('defaultTitle caps a long question at 60 characters', () => {
  const long = 'x'.repeat(200)
  assert.equal(defaultTitle([{ role: 'user', content: long }]).length, 60)
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/chat-markdown.test.js`
Expected: FAIL — cannot resolve `./chat-markdown.js`.

- [ ] **Step 3: Implement**

Create `frontend/src/lib/chat-markdown.js`:

```js
// Shapes an LLM conversation into a markdown study document.
//
// Questions become headings and answers become bodies, rather than a
// You:/Assistant: transcript: the result is a source you will read
// incrementally and extract from, and your half-formed questions should not be
// part of the text you study.
//
// Kept out of the component because this repo has no Svelte component test
// harness — logic left in a .svelte file has no tests at all.

/**
 * @param {string} title
 * @param {Array<{role: string, content: string}>} messages
 * @param {string[]} models  model ids used during the chat, in first-use order
 * @param {Date} date
 * @returns {string}
 */
export function chatToMarkdown(title, messages, models, date = new Date()) {
  const list = Array.isArray(messages) ? messages : []
  const out = ['# ' + String(title ?? '').trim(), '']

  for (let i = 0; i < list.length; i++) {
    if (list[i]?.role !== 'user') continue
    const answer = list[i + 1]
    // Drop a question whose answer has not arrived rather than saving an empty
    // section — this is a study document, not a conversation in progress.
    if (answer?.role !== 'assistant') continue
    out.push('## ' + oneLine(list[i].content), '', String(answer.content ?? '').trim(), '')
  }

  const used = [...new Set((models ?? []).filter(Boolean))]
  if (used.length) {
    // Provenance, for the same reason the medium is "llm": a reader months from
    // now should be able to see this was written by a model, and by which.
    // toISOString is UTC, so a save near local midnight can name the next day —
    // acceptable for a provenance line, and it keeps the output deterministic.
    out.push('---', '*Generated with ' + used.join(', ') + ' — ' + date.toISOString().slice(0, 10) + '*', '')
  }
  return out.join('\n')
}

/** First line of the first question, capped — a starting point to edit. */
export function defaultTitle(messages) {
  const first = (Array.isArray(messages) ? messages : []).find((m) => m?.role === 'user')
  if (!first) return ''
  // The first LINE, not the whole question collapsed. A multi-line question's
  // later lines are supporting detail; the title should be the thing you asked.
  // (chatToMarkdown's heading differs deliberately — there the whole question
  // becomes the heading, folded onto one line.)
  return oneLine(String(first.content ?? '').split('\n')[0]).slice(0, 60)
}

// A heading cannot contain a newline, and stray runs of whitespace read badly
// in a table of contents.
function oneLine(s) {
  return String(s ?? '').replace(/\s+/g, ' ').trim()
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `node --test frontend/src/lib/chat-markdown.test.js` then `make test-js`
Expected: all PASS.

- [ ] **Step 5: Negative controls (report both)**

1. Change `if (answer?.role !== 'assistant') continue` to `if (false) continue`. The unanswered-question test must FAIL naming `still pending`. Restore.
2. Remove `.replace(/\s+/g, ' ')` from `oneLine`. The multi-line-question test must FAIL. Restore.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/chat-markdown.js frontend/src/lib/chat-markdown.test.js
git commit -m "feat(llm-source): shape a chat into a markdown study document"
```

---

### Task 2: `CreateLLMSource`

**Files:**
- Modify: `reading_methods.go`
- Test: `reading_methods_test.go`
- Regenerate: `frontend/wailsjs/go/main/App.d.ts`, `App.js`

**Interfaces:**
- Consumes: existing `slugify`, `uniqueDest`, `toReadingView`, `content.CreateReadingNotes`, `learning.ReadingElement`.
- Produces: `func (a *App) CreateLLMSource(title, markdown string) (*ReadingView, error)`.

Modelled on `ImportWebSource` (`reading_methods.go:274`) — read it first and follow its shape, including its cleanup-on-failure behaviour.

`ProjLen` is `utf8.RuneCountInString(markdown)`. That is exact, not an approximation: `source.Projection` returns the raw file contents for any extension other than `.html`/`.htm` (`internal/source/projection.go:23-28`), so a `.md` file's projection *is* its text.

- [ ] **Step 1: Write the failing test**

Append to `reading_methods_test.go`:

```go
func TestCreateLLMSourceRegistersAModelGeneratedReading(t *testing.T) {
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := &App{collectionDir: dir, learnStore: st,
		registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}}

	md := "# Spaced repetition\n\n## How does SM-2 work?\n\nIt multiplies by an ease factor.\n"
	v, err := a.CreateLLMSource("Spaced repetition", md)
	if err != nil {
		t.Fatalf("CreateLLMSource: %v", err)
	}
	// "llm", not "text": the sidebar has to be able to say this was written by a
	// model. ReadingView routes anything that is not pdf/web to the text pane,
	// so this costs no pane work.
	if v.Medium != "llm" {
		t.Errorf("medium = %q, want \"llm\"", v.Medium)
	}
	if v.Title != "Spaced repetition" {
		t.Errorf("title = %q", v.Title)
	}
	if !strings.HasSuffix(v.RefPath, ".md") {
		t.Errorf("refPath = %q, want a .md source", v.RefPath)
	}

	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(v.RefPath)))
	if err != nil {
		t.Fatalf("source file not written: %v", err)
	}
	if string(body) != md {
		t.Errorf("source contents differ from what was passed in")
	}
	if _, err := os.Stat(v.NotesPath); err != nil {
		t.Errorf("notes file not scaffolded: %v", err)
	}

	got, ok, err := st.GetReading(v.ID)
	if err != nil || !ok {
		t.Fatalf("reading not registered: %v ok=%v", err, ok)
	}
	// A .md projection is the raw file, so this must match exactly — a wrong
	// ProjLen makes every % in the sidebar wrong for this document.
	if got.ProjLen != utf8.RuneCountInString(md) {
		t.Errorf("projLen = %d, want %d", got.ProjLen, utf8.RuneCountInString(md))
	}
}

func TestCreateLLMSourceSuffixesACollidingTitle(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	t.Cleanup(func() { st.Close() })
	a := &App{collectionDir: dir, learnStore: st,
		registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}}

	first, err := a.CreateLLMSource("Same title", "# a\n\ntext\n")
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.CreateLLMSource("Same title", "# b\n\nother text\n")
	if err != nil {
		t.Fatal(err)
	}
	if first.RefPath == second.RefPath {
		t.Fatalf("both saved to %q — the second overwrote the first", first.RefPath)
	}
	if first.NotesPath == second.NotesPath {
		t.Fatalf("both scaffolded %q", first.NotesPath)
	}
}

func TestCreateLLMSourceRefusesEmptyInput(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	t.Cleanup(func() { st.Close() })
	a := &App{collectionDir: dir, learnStore: st,
		registry: &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}}

	if _, err := a.CreateLLMSource("   ", "# a\n\ntext\n"); err == nil {
		t.Error("an empty title must be refused")
	}
	if _, err := a.CreateLLMSource("A title", "   \n"); err == nil {
		t.Error("an empty body must be refused")
	}
	// A refusal must not leave a half-made source behind.
	entries, _ := os.ReadDir(filepath.Join(dir, "sources"))
	if len(entries) != 0 {
		t.Errorf("refused calls left %d file(s) in sources/", len(entries))
	}
}
```

`reading_methods_test.go` already imports `os`, `path/filepath`, `strings`, `testing`, `time`, `texere/internal/content` and `texere/internal/learning`. Add `unicode/utf8`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test . -run TestCreateLLMSource -v`
Expected: FAIL — `a.CreateLLMSource undefined`.

- [ ] **Step 3: Implement**

Add to `reading_methods.go`, directly after `ImportWebSource`:

```go
// CreateLLMSource saves an LLM conversation as a markdown reading source.
//
// Medium is "llm" rather than "text" on purpose. ReadingView routes anything
// that is not "pdf" or "web" to the text pane, so the new medium costs no pane
// work, and it lets the sidebar mark the document as model-generated. That
// distinction matters more here than for other sources: a PDF's provenance is
// self-evident and a web article keeps its SourceURL, while a model-written
// study document can be confidently wrong and is about to become flashcards.
//
// ProjLen is the markdown's rune count, which is exact rather than an estimate:
// source.Projection returns raw file contents for every extension except
// .html/.htm, so a .md file's projection is its text.
func (a *App) CreateLLMSource(title, markdown string) (*ReadingView, error) {
	if a.learnStore == nil || a.collectionDir == "" {
		return nil, errors.New("no collection open")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("CreateLLMSource: title is empty")
	}
	if strings.TrimSpace(markdown) == "" {
		return nil, errors.New("CreateLLMSource: nothing to save")
	}
	slug := slugify(title)
	srcRel, srcAbs := uniqueDest(a.collectionDir, "sources", slug+".md")
	if err := os.WriteFile(srcAbs, []byte(markdown), 0o644); err != nil {
		return nil, fmt.Errorf("CreateLLMSource: save: %w", err)
	}
	_, notesAbs := uniqueDest(a.collectionDir, "reading", slug+".n4l")
	refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
	if err != nil {
		os.Remove(srcAbs) // best-effort: don't litter sources/ on a failed create
		return nil, fmt.Errorf("CreateLLMSource: notes: %w", err)
	}
	r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
		Position: "char=0", ProjLen: utf8.RuneCountInString(markdown), IntervalDays: 1, Medium: "llm"}
	if err := a.learnStore.UpsertReading(r); err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("CreateLLMSource: register: %w", err)
	}
	v := toReadingView(r)
	return &v, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test . -run TestCreateLLMSource -v` then `make test`
Expected: PASS (3 tests).

- [ ] **Step 5: Negative controls (report both)**

1. Change `Medium: "llm"` to `Medium: "text"`. `TestCreateLLMSourceRegistersAModelGeneratedReading` must FAIL naming the medium. Restore.
2. Move both empty-input guards to *after* the `os.WriteFile` call. `TestCreateLLMSourceRefusesEmptyInput` must FAIL on the leftover-files assertion. Restore.

- [ ] **Step 6: Regenerate bindings and commit**

```bash
/home/simone/go/bin/wails generate module
git add reading_methods.go reading_methods_test.go \
  frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js
git commit -m "feat(llm-source): save a conversation as a markdown reading source"
git show --stat HEAD
```

Confirm the two binding files appear in the stat output.

---

### Task 3: `LLMChatPane.svelte`

**Files:**
- Create: `frontend/src/components/LLMChatPane.svelte`

**Interfaces:**
- Consumes: `chatToMarkdown`, `defaultTitle` (Task 1); `CreateLLMSource` (Task 2); the existing `streamTurn` from `frontend/src/lib/llm.js` and `GetLLMConfig` from the bindings.
- Produces: a component taking `export let chat` (use with `bind:chat`) and emitting `created` with the new `ReadingView`.

`chat` is `{ messages: [{role, content}], profileId: string|null, models: string[] }`.

Read `frontend/src/components/notebook/LLMCell.svelte` first — this is a simplified version of the same pattern, without skills, attachments or notebook persistence. In particular reuse its `abandonTurn` behaviour: a failed turn drops the trailing user message and restores its text to the input, because a transcript ending in a dangling user turn is rejected by native Anthropic on the next send.

There is **no Svelte component test harness** in this repo. Do not add one; this task is verified by the build plus the reasoning you state in your report.

- [ ] **Step 1: Create the component**

```svelte
<script>
  // Ask a model, refine over a few turns, then save the exchange as a markdown
  // reading source. This is a source-acquisition channel in the same role as
  // + PDF and + Web — not a chat tool that happens to save. Asking about a
  // passage you are already reading is the notebook's LLM cell, not this.
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import { GetLLMConfig, CreateLLMSource } from '../../wailsjs/go/main/App'
  import { streamTurn } from '../lib/llm.js'
  import { chatToMarkdown, defaultTitle } from '../lib/chat-markdown.js'

  export let chat

  const dispatch = createEventDispatcher()

  // The output is study material, not conversation. Without this the model
  // writes replies — hedges, restatements of the question, offers to go on —
  // which read badly as a document and waste extraction effort.
  const SYSTEM = [
    'You are producing study material that will be read incrementally and turned into flashcards.',
    'Answer in complete, factual prose, using short paragraphs and sub-headings where they help.',
    'Do not address the reader conversationally, do not restate the question, and do not offer follow-ups.',
  ].join(' ')

  let profiles = []
  let activeId = ''
  let input = ''
  let partial = ''
  let streaming = false
  let turn = null
  let error = ''
  let saving = false
  let lastSent = ''

  $: canCreate = chat.messages.some((m) => m.role === 'assistant') && !streaming && !saving

  onMount(async () => {
    try {
      const c = await GetLLMConfig()
      profiles = c.profiles ?? []
      activeId = c.activeProfileId ?? ''
    } catch { /* settings not configured; the host hides the entry point anyway */ }
  })
  onDestroy(() => turn?.cancel())

  // The model id, not the profile name: the document's footer should say what
  // actually answered, which outlives a profile someone renames or deletes.
  function modelFor(id) {
    return profiles.find((p) => p.id === (id || activeId))?.model ?? ''
  }

  function setProfile(id) {
    chat.profileId = id || null
    chat = chat
  }

  function send() {
    const text = input.trim()
    if (!text || streaming) return
    lastSent = text
    chat.messages = [...chat.messages, { role: 'user', content: text }]
    chat = chat
    input = ''
    error = ''
    partial = ''
    streaming = true

    turn = streamTurn({
      profileId: chat.profileId,
      system: SYSTEM,
      messages: chat.messages,
      onDelta: (t) => { partial += t },
      onDone: () => finish(partial),
      onError: (m) => { error = m; abandonTurn() },
    })
  }

  function finish(text) {
    if (text) {
      chat.messages = [...chat.messages, { role: 'assistant', content: text }]
      // Recorded per completed turn, so a chat answered by two models says so.
      const m = modelFor(chat.profileId)
      if (m && !chat.models.includes(m)) chat.models = [...chat.models, m]
      chat = chat
    }
    partial = ''
    streaming = false
    turn = null
  }

  function abandonTurn() {
    // Drop the unanswered trailing user message: native Anthropic rejects two
    // consecutive user roles on the next send. Its text goes back in the input
    // so the turn can simply be retried.
    if (chat.messages.length && chat.messages[chat.messages.length - 1].role === 'user') {
      chat.messages = chat.messages.slice(0, -1)
      chat = chat
    }
    if (lastSent && !input) input = lastSent
    partial = ''
    streaming = false
    turn = null
  }

  function stop() { turn?.cancel(); abandonTurn() }

  async function create() {
    if (!canCreate) return
    const title = window.prompt('Title for this document:', defaultTitle(chat.messages))
    if (title === null) return           // cancelled
    if (!title.trim()) { error = 'A title is required.'; return }
    saving = true
    error = ''
    try {
      const md = chatToMarkdown(title, chat.messages, chat.models)
      const v = await CreateLLMSource(title, md)
      dispatch('created', v)
    } catch (e) {
      // Never lose the conversation to a failed save.
      error = String(e?.message ?? e)
    } finally {
      saving = false
    }
  }
</script>

<div class="lc-root">
  <div class="lc-bar">
    <span class="lc-title">✦ Ask a model</span>
    <select class="lc-prof" value={chat.profileId || ''} title="Model — changing it affects the next turn only"
            on:change={(e) => setProfile(e.target.value)}>
      <option value="">Default model</option>
      {#each profiles as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <button class="lc-create" on:click={create} disabled={!canCreate}
            title="Save this conversation as a document in the reading list">
      {saving ? 'Creating…' : 'Create document'}
    </button>
  </div>

  <div class="lc-log">
    {#each chat.messages as m}
      <div class="lc-msg {m.role}"><span class="lc-role">{m.role === 'user' ? 'You' : 'Model'}</span>{m.content}</div>
    {/each}
    {#if partial}<div class="lc-msg assistant"><span class="lc-role">Model</span>{partial}</div>{/if}
    {#if !chat.messages.length && !partial}
      <div class="lc-empty">Ask for the material you want to study. When the answers are good, press Create document and it joins your reading list.</div>
    {/if}
  </div>

  {#if error}<div class="lc-error">{error}</div>{/if}

  <div class="lc-input">
    <textarea bind:value={input} rows="3" placeholder="Ask something…" disabled={streaming}
              on:keydown={(e) => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); send() } }}
    ></textarea>
    {#if streaming}
      <button class="lc-send" on:click={stop}>Stop</button>
    {:else}
      <button class="lc-send" on:click={send} disabled={!input.trim()} title="Ctrl+Enter">Send</button>
    {/if}
  </div>
</div>

<style>
  .lc-root { position: absolute; inset: 0; display: flex; flex-direction: column; background: #0f1724; }
  .lc-bar { display: flex; align-items: center; gap: 8px; padding: 8px 10px;
    border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .lc-title { color: #7dd3fc; font-weight: 600; flex: 1; font-size: 13px; }
  .lc-prof { background: #101826; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px;
    font-size: 11px; padding: 2px 4px; }
  .lc-create { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px;
    padding: 3px 9px; cursor: pointer; font-size: 11px; }
  .lc-create:disabled { opacity: 0.5; cursor: default; }
  .lc-log { flex: 1; min-height: 0; overflow: auto; padding: 10px; }
  .lc-empty { color: #5d7186; font-size: 13px; padding: 20px 4px; max-width: 46em; }
  .lc-msg { white-space: pre-wrap; font-size: 13px; line-height: 1.55; margin-bottom: 12px; max-width: 46em; }
  .lc-msg.user { color: #cdd6e0; }
  .lc-msg.assistant { color: #9fb3c8; }
  .lc-role { display: block; font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em;
    color: #5d7186; margin-bottom: 2px; }
  .lc-error { color: #ff8080; padding: 6px 10px; font-size: 12px; border-top: 1px solid #2a3a4a; }
  .lc-input { display: flex; gap: 8px; padding: 8px 10px; border-top: 1px solid #2a3a4a; background: #131c29; }
  .lc-input textarea { flex: 1; min-width: 0; resize: vertical; background: #0b1220; color: #e8eaf0;
    border: 1px solid #2a3a4a; border-radius: 6px; padding: 6px 8px; font-size: 13px;
    font-family: inherit; outline: none; }
  .lc-send { align-self: flex-end; background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a;
    border-radius: 6px; padding: 4px 12px; cursor: pointer; font-size: 12px; }
  .lc-send:disabled { opacity: 0.5; cursor: default; }
</style>
```

- [ ] **Step 2: Verify**

Run: `cd frontend && npx vite build`
Expected: succeeds, and **no unused-CSS-selector warning naming any `.lc-*` selector**. Such a warning means markup and styles disagree. Pre-existing warnings about other components are fine.

Then `make test-js` (it must stay green; this task adds no JS tests).

- [ ] **Step 3: State the reasoning in your report**

For each, say what a user would see if it were wrong:
- `abandonTurn` dropping the trailing user message.
- `canCreate` requiring at least one `assistant` message.
- the `catch` in `create()` not clearing `chat`.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/LLMChatPane.svelte
git commit -m "feat(llm-source): a chat surface that saves itself as a document"
```

---

### Task 4: Wire it into Study mode

**Files:**
- Modify: `frontend/src/components/SourcesSidebar.svelte`
- Modify: `frontend/src/components/StudySurface.svelte`

**Interfaces:**
- Consumes: `LLMChatPane` (Task 3); the sidebar's existing `dispatch('open', …)`; `StudySurface`'s existing `switchTo(r, anchor)`, `readingView.flush()` and `sidebar.refresh()`.
- Produces: no new exports.

Two hazards, both of which have already cost this codebase a commit:

1. **`.ss-body` is `position: relative` and its children are `position: absolute; inset: 0`.** `LLMChatPane`'s root is already `position: absolute; inset: 0`, and it must render inside `.ss-reading` (which is `position: relative`), not directly in `.ss-body`.
2. **Svelte instruments an assignment only when the template references the variable.** `chat` is read in the markup below, so it is fine — but if you add any state that is only ever read inside a helper function, mutating it will schedule no re-render at all.

- [ ] **Step 1: Add the sidebar entry points**

In `SourcesSidebar.svelte`, add `GetLLMConfig` to the **existing** multi-line
import from `'../../wailsjs/go/main/App'` (do not add a second import statement
from the same module):

```js
  import {
    ImportTextSource, ImportPdfSource, ImportWebSource, GetReadingList, GetReadingExtracts, GetLLMConfig,
  } from '../../wailsjs/go/main/App'
```

Add state and load it in the existing `onMount(load)` path by extending `load()`:

```js
  let hasLLM = false
```

Inside `load()`, after the existing two awaits, add:

```js
    // Rendered only when a profile exists: a user who has never configured a
    // provider should not see a control they cannot use (as AskBox does).
    try { hasLLM = ((await GetLLMConfig())?.profiles ?? []).length > 0 } catch { hasLLM = false }
```

Add two props so the host can show the live chat:

```js
  export let chatLive = false
```

In the import row, after the `+ Web` button:

```svelte
      {#if hasLLM}
        <button class="sb-btn" on:click={() => dispatch('newChat')} title="Ask a model and save the answers as a document">+ LLM</button>
      {/if}
```

Directly above `<ul class="sb-list">`, add the live-chat row:

```svelte
    {#if chatLive}
      <button class="sb-chatrow" on:click={() => dispatch('openChat')}>✦ Chat in progress</button>
    {/if}
```

And its style, beside `.sb-doc`:

```css
  .sb-chatrow { display: block; width: 100%; text-align: left; background: #16243a;
    border: none; border-bottom: 1px solid #2a3a4a; color: #7dd3fc; font-size: 12px;
    padding: 6px 10px; cursor: pointer; }
```

Finally, mark model-generated documents. `SourcesSidebar.svelte:112-113` currently reads:

```svelte
              {#if row.reading.done}<span class="sb-tag done">done</span>
              {:else if dueNow(row.reading)}<span class="sb-tag due">due</span>{/if}
```

Add a separate `{#if}` directly beneath it — not an `{:else if}`, since a
document can be both model-generated and due:

```svelte
              {#if row.reading.medium === 'llm'}<span class="sb-tag llm">llm</span>{/if}
```

with:

```css
  .sb-tag.llm { background: #2c2340; color: #b9a3e3; }
```

- [ ] **Step 2: Hold the chat in `StudySurface`**

Add the import beside the others:

```js
  import LLMChatPane from './LLMChatPane.svelte'
```

Add state beside `let sidebar = null`:

```js
  // The chat lives here, not inside the reading area's {#key} block, so it
  // survives reading a document and coming back. It dies with Study mode.
  let chat = null
```

Add the handlers beside `switchTo`:

```js
  async function newChat() {
    if (chat && chat.messages.length && !window.confirm('Discard the chat in progress?')) return
    await readingView?.flush?.()
    peekAt = null
    activeReading.set(null)
    chat = { messages: [], profileId: null, models: [] }
  }

  async function openChat() {
    await readingView?.flush?.()
    peekAt = null
    activeReading.set(null)
  }

  // The saved document behaves like any other source from here on.
  async function chatCreated(v) {
    chat = null
    await sidebar?.refresh()
    if (v) await switchTo(v, null)
  }
```

Wire the sidebar's new events and the `chatLive` prop:

```svelte
        <SourcesSidebar
          bind:this={sidebar}
          chatLive={chat !== null}
          on:newChat={newChat}
          on:openChat={openChat}
          on:open={(e) => switchTo(e.detail, null)}
          on:peek={(e) => switchTo(e.detail.reading, e.detail.anchor)} />
```

Then render the chat in the reading area. Replace the contents of `<div class="ss-reading">` with:

```svelte
        <div class="ss-reading">
          {#if $activeReading}
            {#key ($activeReading.id ?? '') + '|' + (peekAt ?? '')}
              <ReadingView bind:this={readingView} reading={$activeReading} {peekAt}
                onClose={() => { peekAt = null; activeReading.set(null); sidebar?.refresh(); enter('next') }} />
            {/key}
          {:else if chat}
            <LLMChatPane bind:chat on:created={(e) => chatCreated(e.detail)} />
          {:else}
            <div class="ss-empty">Pick a document to read.</div>
          {/if}
        </div>
```

`$activeReading` is checked first so opening a document always wins over a parked chat; `openChat` clears `activeReading`, which is what brings the chat back.

- [ ] **Step 3: Confirm the chat is not lost on close**

`switchTo` already nulls nothing but `peekAt`, so parking the chat by opening a document leaves `chat` intact. Verify by reading: `grep -n "chat = null" frontend/src/components/StudySurface.svelte` must return exactly **one** hit — inside `chatCreated`. More than one means some path is silently discarding the conversation.

- [ ] **Step 4: Verify**

Run: `make test`, `make test-js`, and `cd frontend && npx vite build`
Expected: all pass, with **no unused-CSS-selector warning naming `.sb-chatrow` or `.sb-tag.llm`**.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/SourcesSidebar.svelte frontend/src/components/StudySurface.svelte
git commit -m "feat(llm-source): + LLM entry point and a parked chat in Study mode"
```

---

### Task 5: Manual test plan

**Files:**
- Create: `docs/superpowers/specs/2026-08-07-llm-source-manual-test-plan.md`

Match the house format of `docs/superpowers/specs/2026-08-06-study-sources-sidebar-manual-test-plan.md`: a preamble stating what is automated and what is not, prereqs, numbered cases with explicit setup and observable results, `**[DB]**` on anything invisible, and a closing "what I most want to hear about".

- [ ] **Step 1: Write the document**

Read `LLMChatPane.svelte`, `SourcesSidebar.svelte` and `StudySurface.svelte` as built before writing, so every case matches what actually shipped rather than what this plan predicted.

Cover at least:

1. `+ LLM` is absent when no LLM profile is configured, and present once one is.
2. A first question streams an answer; the Stop button ends a turn mid-stream and restores the question to the input.
3. A provider error shows in the pane, drops the pending question, and the same question can be retried.
4. Switching model mid-chat: the next answer uses the new model, earlier answers are untouched.
5. **The chat survives reading.** With a chat of two or more turns, click a document, read it, then click `✦ Chat in progress` — every turn is still there and you can keep going.
6. `Create document` is disabled until at least one answer has completed.
7. Creating: the title prompt is prefilled from the first question, the new document appears in the sidebar with an `llm` tag, and it opens.
8. The saved markdown reads as an article — each question a `##` heading — and the footer names every model used. **[DB]** `SELECT id, title, medium, proj_len FROM reading_elements WHERE medium='llm';`
9. A question typed but not yet answered is **not** in the saved file.
10. The new document behaves like any other source: read it, `%` advances, Ctrl+E makes an extract, and the extract appears in its tree.
11. Two documents created with the same title both survive, under different filenames.
12. Starting a second chat while one is in progress asks before discarding.

State explicitly, so a tester does not file them as bugs: the chat is deliberately lost when Study mode closes; changing model does not re-run earlier answers; and the document is an ordinary markdown file afterwards, editable in the editor.

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-08-07-llm-source-manual-test-plan.md
git commit -m "docs(test-plan): LLM source"
```

---

## Self-Review

**Spec coverage.** `chatToMarkdown` shaping and the dropped trailing question → Task 1. `CreateLLMSource`, `Medium: "llm"`, collision suffixing, empty-input refusal → Task 2. The chat surface, model switching, the system prompt, Stop, failed-turn rollback, Create-disabled-until-answered, and save failure leaving the chat intact → Task 3. `+ LLM` hidden without a profile, the live-chat row, the `llm` tag, the chat surviving a document switch, and the confirm on discard → Task 4. Manual coverage of everything with no harness → Task 5.

**Placeholders.** None: every code step carries literal code, every test its assertions.

**Type consistency.** `chat` is `{ messages, profileId, models }` in Tasks 3 and 4. `chatToMarkdown(title, messages, models, date?)` and `defaultTitle(messages)` are defined in Task 1 and called with those exact shapes in Task 3. `CreateLLMSource(title, markdown) -> ReadingView` is defined in Task 2, called in Task 3, and its result flows into Task 4's `chatCreated(v)` → `switchTo(v, null)`, which matches the existing `switchTo(r, anchor)` signature. The sidebar's new events `newChat`/`openChat` and prop `chatLive` are declared in Task 4 Step 1 and consumed in Step 2.

**Two things deliberately not built.** There is no draft persistence, so quitting the app loses a chat — the spec rejected it as unjustified until the feature proves itself, and the manual plan tells the tester so. And nothing seeds a chat from a passage being read: that is the notebook's LLM cell, and keeping it out means this feature touches no source pane and cannot destabilise the peek/progress guarantees.
