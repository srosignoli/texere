# Natural-Language Query Translation Implementation Plan

**Goal:** In graph mode, an optional button turns typed text into a searchN4L query using an LLM profile you choose, writing the result into the query bar unrun and editable.

**Architecture:** Verified few-shot examples live in Go and are proven against a real N4L corpus by an integration test. A Wails binding exposes them; a dependency-free JS module renders them plus the existing `query-grammar.js` into a system prompt, and validates what comes back. A second binding runs the one-shot LLM call.

**Tech Stack:** Go 1.25, Wails v2, Svelte 3.49, `sstorytime-kv` v0.6.0, `node --test`.

## Global Constraints

- **Only what texere executes.** The prompt teaches the ten directives in `frontend/src/lib/query-grammar.js` and nothing else. Never `\theme`, `\bookmarks`, `\dna`, `\terms`, `\concepts` — they parse, route to a node search, return nothing, and are indistinguishable from "no results".
- **Translation is explicit, never inferred.** No heuristic decides text "looks like a question". Enter always runs the literal text.
- **The generated query is never auto-run.** It lands in the bar for inspection.
- **The feature is absent without an LLM profile.** Gate on `profiles.length > 0`, the same condition `LLMCell.svelte:51` uses.
- **One home for the grammar.** `query-grammar.js` is the single source of truth; never restate a directive's meaning in Go or in a prompt string literal.
- Go: `gofmt`, imports grouped stdlib / external / internal. JS/Svelte: 2-space indent, **no semicolons**, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.
- `wails` is not on PATH; it is at `/home/simone/go/bin/wails`.
- `git add frontend/wailsjs` (bare dir) exits non-zero because `frontend/wailsjs/runtime/` is gitignored. Add explicit file paths and confirm with `git log`.
- `TestWatchCollectionAdded` in `internal/content` is a **known pre-existing fsnotify flake** that fails intermittently under load on `main` too. Ignore it; never "fix" it.

## MANDATORY: negative-control every test you write

This project has shipped tests that passed against the exact bug they were written to catch. Before committing, for each behaviour asserted: break the production code in the specific way the test exists to catch, run the test, confirm it **fails with a message naming the real problem**, then restore and re-run green. Report the actual failure output for the most load-bearing assertion. "Verified" without the output will be sent back.

## Deviation from the spec, and why

The spec put `EXAMPLES` in `query-prompt.js` with the Go test reading the JS list. **The examples live in Go instead**, exposed by a `ListQueryExamples` binding. Node 22 requires `with { type: 'json' }` for JSON imports while this repo's Vite 3 predates import attributes, so a shared JSON file is fragile, and having Go parse a `.js` file is worse. Owning them in Go puts them where they are verified, and the frontend consumes them the same way it already consumes `queryHints`.

## File Structure

- `internal/search/testdata/corpus/` — nine N4L files copied from `sstorytime-kv/examples`.
- `internal/search/corpus_test.go` — builds the corpus once, asserts every example returns results.
- `query_examples.go` — the verified examples, and the `ListQueryExamples` binding.
- `search_llm_methods.go` — `TranslateQuery`, `GetQueryLLMProfile`, `SetQueryLLMProfile`.
- `search_llm_methods_test.go` — their tests.
- `app.go` — one new `AppConfig` field.
- `frontend/src/lib/query-prompt.js` — `buildQueryPrompt`, `stripToQuery`, `validateQuery`.
- `frontend/src/lib/query-prompt.test.js` — their tests.
- `frontend/src/components/OmniBar.svelte` — the button, picker and refusal UI.
- `docs/specs/2026-08-05-nl-query-translation-manual-test-plan.md` — the manual gate.

---

### Task 1: The corpus fixture and its build test

**Files:**
- Create: `internal/search/testdata/corpus/` (nine files, copied)
- Create: `internal/search/corpus_test.go`

**Interfaces:**
- Consumes: `graph.BuildFromCollection`, `content.LoadRegistry`, `search.NewSearchService`.
- Produces: `buildCorpus(t) *search.SearchService` for Task 2.

The corpus is the foundation everything else is verified against. Copy from `/home/simone/projects/sstorytime-kv/examples`.

- [ ] **Step 1: Copy the nine files**

```bash
cd /home/simone/projects/texere
mkdir -p internal/search/testdata/corpus
for f in NDA.n4l chinese.n4l kubernetes.n4l brains.n4l music-catalogue.n4l \
         doors.n4l astronomy.n4l Mary.n4l integral.n4l; do
  cp "/home/simone/projects/sstorytime-kv/examples/$f" internal/search/testdata/corpus/
done
ls -la internal/search/testdata/corpus/
```

Expected: nine files, ~336KB total. `NDA.n4l` ~173K and `chinese.n4l` ~143K dominate.

Also copy the registry, so the test does not depend on a checkout outside the repo:

```bash
mkdir -p internal/search/testdata/corpus_sstconfig
cp /home/simone/projects/sstorytime-kv/SSTconfig/*.sst internal/search/testdata/corpus_sstconfig/
ls internal/search/testdata/corpus_sstconfig/
```

- [ ] **Step 2: Write the build test**

Create `internal/search/corpus_test.go`:

```go
package search_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"texere/internal/content"
	"texere/internal/graph"
	"texere/internal/search"
)

// The corpus is nine files from sstorytime-kv/examples, chosen for domain
// spread and route coverage rather than size. It exists so the few-shot
// examples fed to the LLM are proven to RETURN RESULTS, not merely to parse: a
// query can be syntactically valid, semantically sensible, and still return
// nothing for pure data reasons, and only running it catches that.
//
// Built once per package run — ~0.9s — and shared, because rebuilding per test
// would dominate the suite.
var (
	corpusOnce sync.Once
	corpusSvc  *search.SearchService
	corpusErr  error
)

func corpus(t *testing.T) *search.SearchService {
	t.Helper()
	corpusOnce.Do(func() {
		dir, err := os.MkdirTemp("", "corpus")
		if err != nil {
			corpusErr = err
			return
		}
		src := filepath.Join(testFixturesDir(t), "corpus")
		entries, err := os.ReadDir(src)
		if err != nil {
			corpusErr = err
			return
		}
		for _, e := range entries {
			b, rerr := os.ReadFile(filepath.Join(src, e.Name()))
			if rerr != nil {
				corpusErr = rerr
				return
			}
			if werr := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); werr != nil {
				corpusErr = werr
				return
			}
		}
		reg, _ := content.LoadRegistry(filepath.Join(testFixturesDir(t), "corpus_sstconfig"))
		if _, berr := graph.BuildFromCollection(dir, reg); berr != nil {
			corpusErr = berr
			return
		}
		corpusSvc = search.NewSearchService(dir, reg)
	})
	if corpusErr != nil {
		t.Fatalf("corpus build: %v", corpusErr)
	}
	return corpusSvc
}

func TestCorpusBuilds(t *testing.T) {
	svc := corpus(t)
	res, err := svc.Search(`\chapter any`)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Listings) == 0 {
		t.Fatal(`\chapter any returned no listings — the corpus did not ingest`)
	}
	t.Logf("corpus chapters: %d", len(res.Listings))
}

// The corpus legitimately references arrows absent from SSTconfig
// (extract-fr, has-frag, "consists of"). Those produce parse errors without
// blocking ingestion, so the build must NOT fail on them — failing would make
// this test hostage to upstream's authoring choices.
func TestCorpusToleratesUnknownArrows(t *testing.T) {
	svc := corpus(t)
	res, err := svc.Search("pod")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Fatal(`"pod" returned nothing; ingestion is broken despite unknown arrows`)
	}
}
```

`testFixturesDir` already exists in `internal/search/integration_test.go` — reuse it, do not redefine it.

- [ ] **Step 3: Run**

Run: `go test ./internal/search/ -run 'TestCorpus' -v`
Expected: both PASS. The build log prints ~0.9s; chapters ≥ 5.

- [ ] **Step 4: Negative control**

Delete `internal/search/testdata/corpus/kubernetes.n4l`, re-run. `TestCorpusToleratesUnknownArrows` must FAIL (`"pod"` returns nothing). Restore the file, confirm `git status --porcelain`, re-run green. Report the failure output.

- [ ] **Step 5: Commit**

```bash
git add internal/search/testdata/corpus internal/search/testdata/corpus_sstconfig internal/search/corpus_test.go
git commit -m "test(search): add a real N4L corpus for verifying query examples"
```

---

### Task 2: The verified examples and their binding

**Files:**
- Create: `query_examples.go`
- Modify: `internal/search/corpus_test.go` (append)
- Regenerate: `frontend/wailsjs/go/main/App.d.ts`, `App.js`, `models.ts`

**Interfaces:**
- Consumes: `corpus(t)` (Task 1).
- Produces, for the frontend: `ListQueryExamples() []QueryExample` where `QueryExample{Question, Query string}`.

**Every example below was run against this exact corpus and returned what the comment says.** Do not add, remove or reword a query without re-running the test.

- [ ] **Step 1: Write the examples**

Create `query_examples.go`:

```go
package main

// QueryExample is one worked question→query pair shown to the LLM.
type QueryExample struct {
	Question string `json:"question"`
	Query    string `json:"query"`
}

// queryExamples are the few-shot examples fed to the query-translation prompt.
//
// Every one is verified by TestQueryExamplesReturnResults against the N4L
// corpus in internal/search/testdata/corpus. That test is the point: an example
// that returns nothing teaches the model to produce queries that return
// nothing, and a query can be syntactically valid, semantically sensible and
// still empty for pure data reasons. Composing these from the grammar alone
// produced three that looked right and matched no data at all.
//
// They teach SHAPE, not content. The corpus is not the user's collection —
// their real chapter and context names are supplied separately from queryHints.
var queryExamples = []QueryExample{
	{"what do my notes say about pods?", "pod"},
	{"pods in the kubernetes chapter", `pod \chapter Kubernetes`},
	{"everything in the neuroscience chapter", `\notes \chapter neuroscience`},
	{"brain waves in the oscillations context", `waves \context oscillations`},
	{"which context tags mention oscillations", `\context oscillations`},
	{"memory notes in neuroscience", `\notes \chapter neuroscience \context memory`},
	{"how do I get from start to target 1?", `\from start \to target 1`},
	{"what does door lead to?", `\from door`},
	{"the exact phrase thank you", `"thank you"`},
	{"brain but not wave", `brain&!wave`},
	{"terms starting with neuro", `neuro:*`},
	{"the exact word pod, not pods", `!pod!`},
	{"what chapters are there?", `\chapter any`},
	{"what arrows are defined?", `\arrow any`},
	{"things near pod, one hop out", `pod \radius 1`},
	{"tell me the story of Mary", `\story Mary`},
	{"the steps in the astronomy sequence", `\sequence \chapter astronomy`},
	{"what sequences are there?", `\story any`},
	{"notes on differentiating under the integral", `\notes \context "differentiate under integral"`},
}

// ListQueryExamples exposes the verified examples to the frontend, which folds
// them into the system prompt. They live in Go because that is where the corpus
// test can prove them.
func (a *App) ListQueryExamples() []QueryExample {
	out := make([]QueryExample, len(queryExamples))
	copy(out, queryExamples)
	return out
}
```

- [ ] **Step 2: Write the failing verification test**

Append to `internal/search/corpus_test.go`. It must live in `package search_test` and cannot import `package main`, so the queries are restated here **and pinned to the Go list by Step 3's guard**:

```go
// Every query the prompt teaches must return something on the corpus. The
// expected kind is asserted too: a query that starts returning listings where
// it used to return nodes has changed meaning, and the prompt's question would
// no longer describe it.
func TestQueryExamplesReturnResults(t *testing.T) {
	svc := corpus(t)
	const (
		nodes = "nodes"
		list  = "listings"
		notes = "notes"
		chain = "structural"
	)
	for _, tc := range []struct{ query, want string }{
		{"pod", nodes},
		{`pod \chapter Kubernetes`, nodes},
		{`\notes \chapter neuroscience`, notes},
		{`waves \context oscillations`, nodes},
		{`\context oscillations`, list},
		{`\notes \chapter neuroscience \context memory`, notes},
		{`\from start \to target 1`, chain},
		{`\from door`, chain},
		{`"thank you"`, nodes},
		{`brain&!wave`, nodes},
		{`neuro:*`, nodes},
		{`!pod!`, nodes},
		{`\chapter any`, list},
		{`\arrow any`, list},
		{`pod \radius 1`, nodes},
		{`\story Mary`, chain},
		{`\sequence \chapter astronomy`, chain},
		{`\story any`, chain},
		{`\notes \context "differentiate under integral"`, notes},
	} {
		t.Run(tc.query, func(t *testing.T) {
			res, err := svc.Search(tc.query)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			got := map[string]int{
				nodes: len(res.Nodes), list: len(res.Listings),
				notes: len(res.Notes), chain: len(res.Structural),
			}
			if got[tc.want] == 0 {
				t.Fatalf("expected %s, got nodes=%d listings=%d notes=%d structural=%d — "+
					"this example teaches the model a query that returns nothing",
					tc.want, got[nodes], got[list], got[notes], got[chain])
			}
		})
	}
}
```

- [ ] **Step 3: Pin the two lists together**

The list above and `queryExamples` in `query_examples.go` must not drift. Add to `query_examples_test.go` (package `main`):

```go
package main

import (
	"os"
	"strings"
	"testing"
)

// The corpus verification lives in package search_test and cannot import this
// package, so it restates the queries. This guard fails when the two drift —
// without it, an example could be added here and never verified, which is
// exactly the hole the corpus test exists to close.
func TestEveryExampleIsVerifiedByTheCorpusTest(t *testing.T) {
	b, err := os.ReadFile("internal/search/corpus_test.go")
	if err != nil {
		t.Fatalf("read corpus test: %v", err)
	}
	body := string(b)
	for _, ex := range queryExamples {
		if !strings.Contains(body, "{`"+ex.Query+"`,") && !strings.Contains(body, `{"`+ex.Query+`",`) {
			t.Errorf("example %q is not verified by TestQueryExamplesReturnResults — add it there", ex.Query)
		}
	}
}
```

- [ ] **Step 4: Run**

Run: `go test ./internal/search/ -run TestQueryExamples -v` and `go test . -run TestEveryExampleIsVerified -v`
Expected: all 19 subtests PASS; the guard PASSES.

- [ ] **Step 5: Negative controls (two, report both)**

1. Add `{"nonsense", "zzzznotathing"}` to `queryExamples` and re-run the guard. It must FAIL naming that query. Remove it.
2. Change `{`\story Mary`, chain}` to `{`\story Mary`, nodes}` in the corpus test and re-run. It must FAIL saying it expected nodes. Restore.

- [ ] **Step 6: Regenerate bindings and commit**

```bash
/home/simone/go/bin/wails generate module
git add query_examples.go query_examples_test.go internal/search/corpus_test.go \
  frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js frontend/wailsjs/go/models.ts
git commit -m "feat(search): verified query examples for the translation prompt"
git log --oneline -1
```

---

### Task 3: The prompt module

**Files:**
- Create: `frontend/src/lib/query-prompt.js`
- Create: `frontend/src/lib/query-prompt.test.js`

**Interfaces:**
- Consumes: `DIRECTIVES`, `MATCHING`, `COMBINING` from `./query-grammar.js`; examples and hints passed in as arguments.
- Produces: `buildQueryPrompt(examples, hints)`, `stripToQuery(raw)`, `validateQuery(text)`.

Dependency-free: it must not import from `wailsjs`. That is not stylistic — `frontend/wailsjs/runtime/` is gitignored, so a module importing it cannot be loaded by Node and would be untestable.

- [ ] **Step 1: Write the failing tests**

Create `frontend/src/lib/query-prompt.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { DIRECTIVES } from './query-grammar.js'
import { buildQueryPrompt, stripToQuery, validateQuery } from './query-prompt.js'

const EX = [{ question: 'what about pods?', query: 'pod' }]
const HINTS = { chapter: ['Kubernetes notes'], context: ['pods'], arrow: ['leadsto'] }

test('the prompt teaches every directive we execute', () => {
  const p = buildQueryPrompt(EX, HINTS)
  for (const d of DIRECTIVES) assert.ok(p.includes(d.token), `missing ${d.token}`)
})

// The whole point of confining the model: upstream syntax we do not route must
// never appear, or it emits queries that parse, return nothing, and look
// exactly like "no results".
test('the prompt never mentions syntax texere cannot execute', () => {
  const p = buildQueryPrompt(EX, HINTS)
  for (const bad of ['\\theme', '\\bookmarks', '\\dna', '\\terms', '\\concepts']) {
    assert.ok(!p.includes(bad), `prompt leaks ${bad}`)
  }
})

test('the prompt carries the examples and the live vocabulary', () => {
  const p = buildQueryPrompt(EX, HINTS)
  assert.ok(p.includes('what about pods?'))
  assert.ok(p.includes('Kubernetes notes'))
  assert.ok(p.includes('pods'))
  assert.ok(p.includes('leadsto'))
})

test('long hint lists are capped and the truncation is stated', () => {
  const many = Array.from({ length: 100 }, (_, i) => `chapter-${i}`)
  const p = buildQueryPrompt(EX, { chapter: many, context: [], arrow: [] })
  assert.ok(p.includes('chapter-0'))
  assert.ok(!p.includes('chapter-99'), 'list was not capped')
  assert.match(p, /some of/i, 'truncation must be stated or the model infers the rest do not exist')
})

test('empty hint lists are omitted, not rendered as bare headings', () => {
  const p = buildQueryPrompt(EX, { chapter: [], context: [], arrow: [] })
  assert.ok(!/chapters:\s*\n\s*\n/i.test(p))
})

test('stripToQuery unwraps what models actually return', () => {
  assert.equal(stripToQuery('pod \\chapter Kubernetes'), 'pod \\chapter Kubernetes')
  assert.equal(stripToQuery('```\npod\n```'), 'pod')
  assert.equal(stripToQuery('```text\npod\n```'), 'pod')
  assert.equal(stripToQuery('`pod`'), 'pod')
  assert.equal(stripToQuery('Query: pod'), 'pod')
  assert.equal(stripToQuery('   pod   \n'), 'pod')
})

test('validateQuery accepts every canonical token and alias', () => {
  for (const d of DIRECTIVES) {
    assert.ok(validateQuery(`${d.token} x`).ok, `rejected ${d.token}`)
    for (const a of d.aliases) assert.ok(validateQuery(`${a} x`).ok, `rejected alias ${a}`)
  }
})

test('validateQuery rejects upstream-only directives and names them', () => {
  const r = validateQuery('\\dna \\chapter NDA')
  assert.equal(r.ok, false)
  assert.deepEqual(r.unknown, ['\\dna'])
  assert.deepEqual(validateQuery('\\theme dark \\bookmarks').unknown, ['\\theme', '\\bookmarks'])
})

test('a query with no directives at all is valid', () => {
  assert.ok(validateQuery('recipe fish soup').ok)
  assert.ok(validateQuery('"thank you"').ok)
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/query-prompt.test.js`
Expected: FAIL — cannot resolve `./query-prompt.js`.

- [ ] **Step 3: Implement**

Create `frontend/src/lib/query-prompt.js`:

```js
// Builds the system prompt that turns a natural-language question into a
// searchN4L query, and checks what comes back.
//
// The grammar is NOT restated here — it is rendered from query-grammar.js,
// which already drives the autocomplete and the help panel and is grounded in
// sstorytime-kv source. A hand-written copy would be a third description of the
// same language, drifting from the other two without anything failing.
//
// Dependency-free on purpose: this repo has no Svelte component harness, and a
// module importing wailsjs cannot be loaded by Node at all, so logic left in a
// .svelte file has no tests.

import { DIRECTIVES, MATCHING, COMBINING } from './query-grammar.js'

// How many live values of each kind to show. Bounded because a large collection
// has hundreds of context tags and the prompt would dwarf the question.
const HINT_CAP = 40

const KNOWN = new Set(DIRECTIVES.flatMap((d) => [d.token, ...d.aliases]))

function hintBlock(label, values) {
  if (!Array.isArray(values) || values.length === 0) return ''
  const shown = values.slice(0, HINT_CAP)
  // Say so when truncated: otherwise the model infers that an unlisted chapter
  // does not exist and avoids a perfectly good one.
  const head = values.length > shown.length
    ? `Some of the ${label} in this collection:`
    : `The ${label} in this collection:`
  return `${head}\n${shown.map((v) => `  ${v}`).join('\n')}\n\n`
}

export function buildQueryPrompt(examples, hints) {
  const h = hints || {}
  const directives = DIRECTIVES.map((d) => {
    const alias = d.aliases.length ? ` (also ${d.aliases.join(', ')})` : ''
    const arg = d.arg ? ` ${d.arg}` : ''
    return `  ${d.token}${arg}${alias}\n    ${d.summary}\n    e.g. ${d.example}`
  }).join('\n')

  const matching = MATCHING.map((m) => `  ${m.title}: ${m.body}`).join('\n')

  const worked = (examples || [])
    .map((e) => `  Q: ${e.question}\n  A: ${e.query}`)
    .join('\n')

  return [
    'You translate a natural-language question into a single search query for an',
    'N4L/SSTorytime knowledge graph.',
    '',
    'These are the ONLY directives that work. Anything else is silently ignored',
    'and returns nothing, so never invent one:',
    directives,
    '',
    'How text matches:',
    matching,
    '',
    `How they combine: ${COMBINING.rule}`,
    `  e.g. ${COMBINING.example} — ${COMBINING.explanation}`,
    '',
    'Worked examples:',
    worked,
    '',
    hintBlock('chapters', h.chapter) +
    hintBlock('context tags', h.context) +
    hintBlock('arrow codes', h.arrow) +
    'Prefer a name from those lists over inventing one.',
    '',
    'Reply with the query string and nothing else — no explanation, no quotes',
    'around the whole query, no code fences.',
  ].join('\n')
}

// Models wrap output in fences or prefix it with a label despite instructions.
// Without this the "query only" contract holds exactly as long as the model
// chooses to cooperate.
export function stripToQuery(raw) {
  let s = String(raw ?? '').trim()
  const fence = s.match(/^```[a-zA-Z]*\n([\s\S]*?)\n?```$/)
  if (fence) s = fence[1].trim()
  if (s.startsWith('`') && s.endsWith('`') && s.length > 1) s = s.slice(1, -1).trim()
  s = s.replace(/^query:\s*/i, '')
  return s.trim()
}

// Checks vocabulary, not arity: it will not catch an argument passed to a flag
// that takes none. It exists to stop upstream-only syntax reaching the bar.
export function validateQuery(text) {
  const unknown = []
  for (const m of String(text ?? '').matchAll(/\\[a-zA-Z]+/g)) {
    if (!KNOWN.has(m[0]) && !unknown.includes(m[0])) unknown.push(m[0])
  }
  return { ok: unknown.length === 0, unknown }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `node --test frontend/src/lib/query-prompt.test.js`
Expected: all PASS. Then `make test-js` — the whole suite must stay green.

- [ ] **Step 5: Negative controls (report both)**

1. Append `'\\dna lists discovered terms'` to the prompt array. "never mentions syntax texere cannot execute" must FAIL. Remove.
2. Change `HINT_CAP` to `1000`. "long hint lists are capped" must FAIL. Restore.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/query-prompt.js frontend/src/lib/query-prompt.test.js
git commit -m "feat(search): build the query-translation prompt from the grammar"
```

---

### Task 4: The translation binding and the remembered profile

**Files:**
- Modify: `app.go` (one field)
- Create: `search_llm_methods.go`
- Create: `search_llm_methods_test.go`
- Regenerate: the three `wailsjs` files

**Interfaces:**
- Consumes: `providerFor` (`llm_methods.go:63`), `loadConfig`/`saveConfig`, `llm.Request`, `llm.Message`.
- Produces: `TranslateQuery(profileID, question, system string) (string, error)`, `GetQueryLLMProfile() string`, `SetQueryLLMProfile(id string) error`.

- [ ] **Step 1: Add the config field**

In `app.go`, inside `AppConfig`, after `STT LLMConfig`:

```go
	// QueryLLMProfileID selects which LLM profile translates a natural-language
	// question into a graph query. Empty means "the active profile". It lives on
	// AppConfig rather than LLMConfig because LLMConfig is reused verbatim for
	// STT, where this would be meaningless.
	QueryLLMProfileID string `json:"queryLlmProfileId"`
```

- [ ] **Step 2: Write the failing tests**

Create `search_llm_methods_test.go`:

```go
package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"texere/internal/llm"
)

type fakeProvider struct {
	chunks []string
	err    error
}

func (f fakeProvider) Stream(_ context.Context, _ llm.Request, onDelta func(string) error) error {
	for _, c := range f.chunks {
		if err := onDelta(c); err != nil {
			return err
		}
	}
	return f.err
}

// The reply arrives in chunks; the caller wants one string. Accumulating in Go
// keeps the frontend from reassembling deltas for a single value.
func TestTranslateQueryAccumulatesTheStream(t *testing.T) {
	got, err := translateWith(fakeProvider{chunks: []string{"pod ", "\\chapter ", "Kubernetes"}}, "m", "sys", "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `pod \chapter Kubernetes` {
		t.Fatalf("got %q, want the three chunks joined", got)
	}
}

func TestTranslateQuerySurfacesProviderErrors(t *testing.T) {
	_, err := translateWith(fakeProvider{chunks: []string{"x"}, err: errors.New("rate limited")}, "m", "sys", "q")
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err = %v, want the provider's error surfaced", err)
	}
}

func TestQueryLLMProfileRoundTrips(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := &App{}
	if got := a.GetQueryLLMProfile(); got != "" {
		t.Fatalf("absent field should read as empty, got %q", got)
	}
	if err := a.SetQueryLLMProfile("prof-1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := a.GetQueryLLMProfile(); got != "prof-1" {
		t.Fatalf("got %q, want prof-1 to survive a reload", got)
	}
}
```

If `XDG_CONFIG_HOME` does not redirect `os.UserConfigDir` on this platform, report it as NEEDS_CONTEXT rather than weakening the test to assert nothing.

- [ ] **Step 3: Run to verify it fails**

Run: `go test . -run 'TranslateQuery|QueryLLMProfile' -v`
Expected: FAIL — `translateWith`, `GetQueryLLMProfile`, `SetQueryLLMProfile` undefined.

- [ ] **Step 4: Implement**

Create `search_llm_methods.go`:

```go
package main

import (
	"context"
	"strings"

	"texere/internal/llm"
)

// translateQueryMaxTokens is small on purpose: the answer is one query line, and
// a low cap makes a runaway model fail fast rather than bill for an essay.
const translateQueryMaxTokens = 256

// translateWith is the testable core: one non-streamed turn, accumulated.
// Split from TranslateQuery so a stub provider can drive it without a config
// file or a real endpoint.
func translateWith(prov llm.Provider, model, system, question string) (string, error) {
	var sb strings.Builder
	err := prov.Stream(context.Background(), llm.Request{
		Model:     model,
		System:    system,
		Messages:  []llm.Message{{Role: "user", Content: question}},
		MaxTokens: translateQueryMaxTokens,
	}, func(text string) error {
		sb.WriteString(text)
		return nil
	})
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}

// TranslateQuery turns a natural-language question into a graph query using the
// given profile ("" = the active one). One shot, not streamed: the answer is a
// single line, so the frontend awaits a value instead of reassembling deltas.
//
// The system prompt is passed IN rather than built here, so the query grammar
// keeps exactly one home — frontend/src/lib/query-grammar.js. Rebuilding it in
// Go would be the drift this design exists to avoid.
func (a *App) TranslateQuery(profileID, question, system string) (string, error) {
	prov, model, err := a.providerFor(profileID)
	if err != nil {
		return "", err
	}
	return translateWith(prov, model, system, question)
}

// GetQueryLLMProfile reports which profile translates queries. Empty means the
// active profile, which providerFor already resolves.
func (a *App) GetQueryLLMProfile() string {
	cfg, _ := loadConfig()
	return cfg.QueryLLMProfileID
}

// SetQueryLLMProfile remembers the choice across restarts.
func (a *App) SetQueryLLMProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	cfg.QueryLLMProfileID = id
	return saveConfig(cfg)
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test . -run 'TranslateQuery|QueryLLMProfile' -v`
Expected: all PASS.

- [ ] **Step 6: Negative control**

Change `translateWith` to `return sb.String()[:0], nil`. `TestTranslateQueryAccumulatesTheStream` must FAIL showing an empty string. Restore. Report the output.

- [ ] **Step 7: Regenerate bindings and commit**

```bash
/home/simone/go/bin/wails generate module
git add app.go search_llm_methods.go search_llm_methods_test.go \
  frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js frontend/wailsjs/go/models.ts
git commit -m "feat(search): one-shot query translation with a remembered profile"
git log --oneline -1
```

---

### Task 5: The OmniBar affordance

**Files:**
- Modify: `frontend/src/components/OmniBar.svelte`

**Interfaces:**
- Consumes: `TranslateQuery`, `ListQueryExamples`, `GetQueryLLMProfile`, `SetQueryLLMProfile`, `GetLLMConfig` from `'../../wailsjs/go/main/App'`; `buildQueryPrompt`/`stripToQuery`/`validateQuery` from `'../lib/query-prompt.js'`; the existing `queryHints` store.

No unit test — this repo has no Svelte component harness. Verify statically and via the build, and state in your report the user-visible symptom if each guard were wrong.

- [ ] **Step 1: Imports and state**

Extend the App import with `TranslateQuery, ListQueryExamples, GetQueryLLMProfile, SetQueryLLMProfile, GetLLMConfig`, and add:

```js
  import { buildQueryPrompt, stripToQuery, validateQuery } from '../lib/query-prompt.js'
```

Add beside the other state:

```js
  // Natural-language translation. Absent entirely when no LLM profile exists —
  // not disabled, not a tooltip: a user who has never configured a provider
  // should not see a control they cannot use.
  let llmProfiles = []
  let queryProfileId = ''
  let examples = []
  let translating = false
  let refusal = null      // { query, unknown: string[] } while a bad query is held back
  $: canTranslate = llmProfiles.length > 0

  // A SECOND onMount, not merged into the existing one. The existing call is a
  // sync arrow whose return value Svelte treats as the cleanup function; making
  // it async would return a Promise instead and silently break that teardown.
  // Svelte allows any number of onMount registrations.
  onMount(async () => {
    try { const c = await GetLLMConfig(); llmProfiles = c.profiles ?? [] } catch { /* not configured */ }
    try { queryProfileId = (await GetQueryLLMProfile()) ?? '' } catch { /* default */ }
    try { examples = (await ListQueryExamples()) ?? [] } catch { /* prompt degrades to grammar only */ }
    // A profile deleted since it was chosen must fall back, not persist a stale
    // name — providerFor resolves an unknown id to an error, not to the default.
    if (queryProfileId && !llmProfiles.some((p) => p.id === queryProfileId)) queryProfileId = ''
  })

  function setQueryProfile(id) {
    queryProfileId = id || ''
    SetQueryLLMProfile(queryProfileId).catch(() => {})
  }

  async function translate() {
    const question = query.trim()
    if (!question || translating) return
    translating = true
    refusal = null
    try {
      const raw = await TranslateQuery(queryProfileId, question, buildQueryPrompt(examples, $queryHints))
      const q = stripToQuery(raw)
      const v = validateQuery(q)
      if (!v.ok) { refusal = { query: q, unknown: v.unknown }; return }
      acceptTranslation(q)
    } catch (e) {
      appendLog([{ severity: 'error', source: 'search', message: `translate: ${String(e?.message ?? e)}` }])
    } finally {
      translating = false
    }
  }

  // Never auto-run: the query lands in the bar so it can be read and edited.
  // Running it would hide both the syntax and any mistake in it.
  function acceptTranslation(q) {
    refusal = null
    query = q
    inputEl?.focus()
  }
```

**Do not merge this into the existing `onMount`** at `OmniBar.svelte:160`. That
one is `onMount(() => document.addEventListener(...))` — a sync arrow whose
return value Svelte uses as the cleanup function. Making it `async` would return
a Promise instead, and the listener would never be removed. Register a second
`onMount`; Svelte supports as many as you like.

Verified present in this file already, so do not re-declare them: `onMount`
(`:2`), `appendLog` (`:7`), `inputEl` (`:16`), `queryHints` (`:11`).

- [ ] **Step 2: The button and picker**

In the markup, immediately after the existing `<input>`, add:

```svelte
        {#if isGraphMode && canTranslate}
          <select class="q-prof" value={queryProfileId}
                  on:change={(e) => setQueryProfile(e.target.value)} title="Model used to translate">
            <option value="">Default model</option>
            {#each llmProfiles as p}<option value={p.id}>{p.name}</option>{/each}
          </select>
          <button class="q-ask" on:click={translate} disabled={translating}
                  title="Turn this question into a query">
            {translating ? '…' : '✨'}
          </button>
        {/if}
```

- [ ] **Step 3: The refusal**

After the input row:

```svelte
  {#if refusal}
    <div class="q-refusal">
      <div><b>Not usable here:</b> {refusal.unknown.join(', ')} — upstream searchN4L syntax texere does not execute.</div>
      <code>{refusal.query}</code>
      <div class="q-refusal-actions">
        <button on:click={translate}>Retry</button>
        <button on:click={() => acceptTranslation(refusal.query)}>Use anyway</button>
        <button on:click={() => (refusal = null)}>Dismiss</button>
      </div>
    </div>
  {/if}
```

`Use anyway` exists because the validator can lag a library upgrade; being wrong should not be a dead end.

- [ ] **Step 4: Styles**

```css
  .q-prof { background: #101826; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; font-size: 11px; padding: 2px 4px; }
  .q-ask { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; cursor: pointer; }
  .q-ask:disabled { opacity: 0.6; cursor: default; }
  .q-refusal { padding: 6px 10px; border-top: 1px solid #2a3a4a; background: #101826; color: #e8eaf0; font-size: 12px; }
  .q-refusal code { display: block; margin: 4px 0; color: #fbbf24; }
  .q-refusal-actions { display: flex; gap: 6px; }
  .q-refusal-actions button { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 11px; }
```

- [ ] **Step 5: Verify**

Run: `make test`, `make test-js`, `cd frontend && npx vite build`
Expected: all pass; **no unused-CSS-selector warning naming `.q-ask`, `.q-prof` or `.q-refusal`** — such a warning means the markup was not added.

Then confirm by grep that Enter still runs literal text: the keydown handler must be untouched, and `translate()` must be reachable only from the button.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/OmniBar.svelte
git commit -m "feat(search): translate a question into a query from the omnibar"
```

---

### Task 6: Manual test plan

**Files:**
- Create: `docs/specs/2026-08-05-nl-query-translation-manual-test-plan.md`

Match the house format of `docs/specs/2026-08-04-note-editor-notebook-manual-test-plan.md`: a preamble stating what is automated and what is not, prereqs, numbered cases with explicit setup and observable results, `**[DB]**` on anything invisible.

- [ ] **Step 1: Write the document**

Cover at least:

1. With **no** LLM profile configured, no button and no picker appear; graph search is unchanged.
2. With a profile, the button appears in graph mode only — not in fuzzy mode.
3. A question becomes a query **in the bar, not run**. Nothing searches until Enter.
4. Enter on literal query text still runs it, untouched — translation never intercepts.
5. The generated query is editable before running.
6. The picker lists profiles, defaults to "Default model", and the choice **survives a restart**.
7. Deleting the chosen profile falls back to the default rather than erroring.
8. A question naming one of *your* real chapters produces a query using that chapter's actual name — this is what the live hints are for.
9. A provider error (bad key, unreachable endpoint) surfaces in the log without breaking search.
10. `Retry` re-asks; `Use anyway` puts a refused query in the bar; `Dismiss` clears it.
11. Translating with an empty input does nothing.
12. Fuzzy mode, `GraphQueryCell` and the explorer's query bar are all unaffected.

State explicitly, so a tester does not report them as bugs: the query is **not** run automatically; a valid query may still return nothing, because validation checks vocabulary and not whether your collection holds matching data.

- [ ] **Step 2: Commit**

```bash
git add docs/specs/2026-08-05-nl-query-translation-manual-test-plan.md
git commit -m "docs(test-plan): natural-language query translation"
```

---

## Self-Review

**Spec coverage.** Prompt generated from the grammar → Task 3. Only-what-we-execute → Task 3's two prompt tests plus `validateQuery`. Verified examples → Tasks 1-2. Live vocabulary → Task 3's hint tests, wired in Task 5. One-shot binding → Task 4. Profile selection that persists → Task 4 config + Task 5 picker. Explicit, never-auto-run → Task 5 Steps 1 and 5. Absent without a profile → Task 5 `canTranslate`. Refusal with override → Task 5 Step 3.

**Placeholders.** None: every code step carries literal code, every test its assertions.

**Type consistency.** `QueryExample{Question, Query}` (Go, tags `question`/`query`) is consumed as `e.question`/`e.query` in Task 3's prompt builder and passed through unchanged in Task 5. `buildQueryPrompt(examples, hints)` takes them in that order in Tasks 3 and 5. `validateQuery` returns `{ ok, unknown }`, destructured as such in Task 5. `hints` is `{chapter, context, arrow}`, matching the `queryHints` store.

**Two risks called out where they bite.** Task 2's guard exists because the corpus test cannot import `package main`, so the query list is stated twice; without the guard an example could be added and never verified. Task 4's config test depends on `XDG_CONFIG_HOME` redirecting `os.UserConfigDir`, and the task says to escalate rather than weaken the test if it does not.

**One deliberate gap.** `\page` and `\limit` have no worked example — `\limit` is prepended automatically by the OmniBar, and `\page` only makes sense inside a `\notes` view. Both are still described in the prompt from `DIRECTIVES`.
