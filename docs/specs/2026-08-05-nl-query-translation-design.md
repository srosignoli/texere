# Natural-language questions into graph queries

**Date:** 2026-08-05
**Status:** Proposed — awaiting approval
**Feature area:** Graph search (`OmniBar`) + LLM

## Summary

In graph mode, an optional **ask box above the query bar** turns a plain-language
question into a searchN4L query. It draws on the same LLM profiles the LLM cell uses, and you
pick which of them translates — a choice that persists. The generated query is
written **into the query bar**, where you read and edit it. The bar searches as
you type, so it runs from there like any query you had typed yourself.

Translation is never automatic. Enter always runs the literal text as a query,
exactly as today. The button is the only trigger, and it is not rendered at all
when no LLM profile exists.

## Motivation

The query language is powerful and unfamiliar. `\notes \context "differentiate
under integral"` is not guessable, and the autocomplete only helps once you know
a directive exists. A natural-language route lowers the entry cost without
taking the query language away — the generated query stays visible and editable,
so it teaches the syntax rather than hiding it.

## Grounding in the current implementation

Surveyed before designing:

- **`frontend/src/lib/query-grammar.js` is already the single source of truth**
  for this language. Its header states it is grounded in
  `sstorytime-kv` source rather than guessed, and it exports `DIRECTIVES` (10
  tokens: `\chapter \context \notes \from \to \sequence \arrow \limit \page
  \radius`), `MATCHING` (operator semantics) and `COMBINING` (how they compose).
  **Both the autocomplete dropdown and the help panel are generated from it.**
- **Three nested vocabularies exist**, and they are not the same thing:
  1. The official docs include CLI-only directives — `\theme`, `\bookmarks`,
     `\dna`, `\terms`, `\concepts`.
  2. The library's `DecodeSearchField` parses ~40 keywords.
  3. **Texere routes on eight fields** — `From`, `To`, `Sequence`, `Name`,
     `Chapter`, `Context`, `PageNr`, `Arrows` (`internal/search/search.go`
     `classifyQuery`, four routes) — plus texere's own `\radius`, which is **not**
     official syntax.
  `query-grammar.js` already encodes vocabulary 3, which is the one that does
  anything here.
- **`OmniBar.svelte`** has two modes (`isFuzzyMode` / `isGraphMode`), prepends
  `\limit N` when absent, and calls `SearchGraph` at `:324`.
- **`providerFor(id)`** (`llm_methods.go:63`) resolves a profile, falling back to
  `cfg.LLM.ActiveProfileID` when passed `""`, and returns
  `errors.New("no LLM profile configured")` when there is none.
- **`LLMCell.svelte:51`** gates its UI on `hasProvider = profiles.length > 0`.
- `SearchGraph` is also called by `GraphQueryCell.svelte` and
  `compute-host.js`; neither is touched.

## Decisions

- **The prompt is generated from `query-grammar.js`, never hand-written.** A
  hand-written grammar would become a third description of the language,
  drifting silently from the autocomplete and the help panel. Generating it
  means adding a directive teaches the LLM for free, and the prompt can never
  claim syntax we do not route.
- **The LLM may emit only what texere executes.** Rejected: teaching the full
  official syntax. `\dna \chapter NDA` parses, routes to a node search, returns
  nothing, and is indistinguishable from "no results" — a confidently wrong
  answer is worse than a refusal.
- **Translation is explicit, never inferred.** No heuristic decides that text
  "looks like a question". Enter runs what you typed.
- **The generated query lands in the bar and runs from there like any other.**
  An earlier draft of this design said it must never auto-run, on the reasoning
  that running it would hide the query and any mistake in it. That reasoning was
  wrong about this bar: `OmniBar` already searches as you type
  (`OmniBar.svelte:356-373`), so the query is visible in the bar *and* its
  results appear — which tells you immediately whether the translation actually
  worked. Suppressing that would make a translated query behave differently from
  one you typed, for no gain. The expensive listing queries — `\notes`,
  `\story`, `\sequence`, `\from` — still wait for Enter, exactly as they
  already do (`isListingQuery`, `:314-317`), so a generated full-DB scan is not
  fired on your behalf.
- **Unknown directives are refused, with an override.** The message names what
  was rejected and why; `Use anyway` exists because the validator may lag a
  library upgrade, and being wrong should not be a dead end.
- **One-shot, not streamed.** The answer is one line. The backend accumulates
  the provider stream and returns the finished string rather than making the
  frontend reassemble deltas for a single value.
- **You choose which model translates, and the choice sticks.** The profile
  *set* is shared with the LLM cell — no second place to configure providers —
  but which of them handles translation is its own persisted preference,
  defaulting to the active profile. Translating a query is a narrow, repetitive
  task that a small fast model often does well, while the LLM cell may want a
  stronger one; forcing both through one profile makes that choice impossible.
  Rejected: a separate provider configuration for search, which would double the
  settings surface rather than just the selection.

## Architecture

### `frontend/src/lib/query-prompt.js` (new, dependency-free)

Three pure functions, all testable under `node --test` — which matters because
this repo has no Svelte component harness, so logic left in a `.svelte` file has
no tests at all.

- **`buildQueryPrompt(hints)`** → the system prompt, rendered from `DIRECTIVES`,
  `MATCHING`, `COMBINING`, the worked `EXAMPLES` below, and `hints` — the user's
  real chapter, context and arrow names. Ends with the output contract: emit the
  query string and nothing else.
- **`stripToQuery(raw)`** → the model's reply reduced to a bare query. Models
  routinely wrap output in code fences or add a sentence of explanation despite
  instructions; this removes fences, surrounding backticks, a leading
  `Query:` label, and surrounding whitespace. Without it the contract holds only
  as long as the model cooperates.
- **`validateQuery(text)`** → `{ ok, unknown: string[] }`. Every `\token` in the
  text is checked against the canonical tokens and aliases in `DIRECTIVES`.
  Unknown ones are returned for the refusal message.
- **`EXAMPLES`** → the worked question→query pairs (below).

### Why a grammar reference alone is not enough

`DIRECTIVES` describes each directive in isolation. That teaches the *grammar*
but never demonstrates the *task* — a question arriving as prose and a query
leaving as one line. Two things close that gap, and the second matters more.

**Worked examples, verified against a real corpus.** `EXAMPLES` is a list of
question→query pairs, and **a test runs every one of them against a graph built
from real upstream N4L files and asserts it returns results**. An example that
returns nothing teaches the model to produce queries that return nothing, which
is the failure this feature would otherwise produce most often.

The corpus is nine files copied from `sstorytime-kv/examples` into
`internal/search/testdata/corpus/` (~336KB, builds in under a second), chosen for
domain spread and route coverage rather than size:

| file | size | domain | exercises |
|---|---|---|---|
| `NDA.n4l` | 173K | legal | large flat text |
| `chinese.n4l` | 143K | language | CJK, phrases |
| `kubernetes.n4l` | 6.2K | ops | chapter + context |
| `brains.n4l` | 4.5K | science | contexts, `\notes` |
| `music-catalogue.n4l` | 3.6K | catalogue | properties |
| `doors.n4l` | 401B | physics | paths, cones |
| `astronomy.n4l` | 1.1K | astronomy | active `_sequence_` |
| `Mary.n4l` | 519B | poem | `\story` by name |
| `integral.n4l` | 1.8K | mathematics | quoted multi-word context |

`MobyDickNotes.n4l` (1.8M) and `Darwin.n4l` (1.6M) were verified to build and
query correctly (4.4s, 14288 nodes) but are excluded: 3.4MB of repo weight and
4.4s on every suite run, for no query coverage the six above do not already give.

The registry comes from upstream `SSTconfig` via `content.LoadRegistry` — 563
arrows, no hand-built stubs.

**The verified table** — every row was run against this corpus and returned what
is shown:

| question | query | returns |
|---|---|---|
| what do my notes say about pods? | `pod` | 5 nodes |
| pods in the kubernetes chapter | `pod \chapter Kubernetes` | 5 nodes |
| everything in the neuroscience chapter | `\notes \chapter neuroscience` | 1 note |
| brain waves in the oscillations context | `waves \context oscillations` | 5 nodes |
| which context tags mention oscillations | `\context oscillations` | 2 listings |
| memory notes in neuroscience | `\notes \chapter neuroscience \context memory` | 1 note |
| how do I get from start to target 1? | `\from start \to target 1` | 30 chains |
| what does door lead to? | `\from door` | 17 chains |
| the exact phrase thank you | `"thank you"` | 8 nodes |
| brain but not wave | `brain&!wave` | 5 nodes |
| terms starting with neuro | `neuro:*` | 1 node |
| the exact word pod, not pods | `!pod!` | 1 node |
| what chapters are there? | `\chapter any` | 5 listings |
| what arrows are defined? | `\arrow any` | 591 listings |
| things near pod, one hop out | `pod \radius 1` | 5 nodes |
| tell me the story of Mary | `\story Mary` | 1 chain |
| the steps in the astronomy sequence | `\sequence \chapter astronomy` | 1 chain |
| what sequences are there? | `\story any` | 5 chains |
| notes on differentiating under the integral | `\notes \context "differentiate under integral"` | 1 note |

That covers all four routes — node search, listing, notes, structural — every
directive the prompt teaches except `\page` and `\limit`, and the
`&`/`!`/`:*`/`!x!`/phrase operators.

**What this verification caught**, which is the argument for doing it at all:

- **`\sequence` had no working example** until the corpus was extended. On the
  first six files, `\sequence`, `\sequence \chapter NDA` and `\sequence
  \context NDA` all returned nothing — `NDA.n4l` carries a `:: _sequence_ ,
  NDA::` line, but a second `::` context immediately replaces it before any node
  appears, so no node is ever inside the sequence block. Adding `astronomy.n4l`,
  `Mary.n4l` and `integral.n4l` (3.4KB together, all with `_sequence_` active
  over real nodes) closed it: `\story Mary` → 1 chain, `\story any` → 5 chains.
  `\story Mary` is also the example upstream's own docs use.
- **A well-formed query can be empty for data reasons alone.** `brain \context
  oscillations` returns nothing while `waves \context oscillations` returns 5
  nodes — the "brain" nodes live in a different context, so the intersection is
  genuinely empty. Nothing is wrong with the query. This is the strongest
  argument for verifying examples rather than composing them from the grammar:
  they can be syntactically perfect, semantically sensible, and still teach the
  model a shape that yields nothing.
- **Earlier drafts of this table were fiction.** `\chapter astronomy` before
  `astronomy.n4l` was in the corpus; `\sequence "create a pod"`, passing an
  argument to a flag declared `valueKind: 'none'`; and a multi-word `|deep
  purple|` where `MATCHING` declares `|…|` as wrapping a word. All three came
  from the *official* upstream docs — valid there, wrong here.
- **A claim I made and had to retract.** I first recorded that a quoted
  multi-word context tag matches nothing. It does not: `\context "brain waves"`
  returns 2 listings and `\notes \context "differentiate under integral"`
  returns a note. The empty result I had seen came from the text term, not the
  quoting. Recorded because the wrong version was already written into this
  document and would have taught the prompt to avoid a form that works.

**The examples teach shape, not content.** They are proven against the corpus,
which is not the user's collection — `\chapter Kubernetes` is a demonstration of
form. The real names come from the next section.

**The user's real vocabulary, which is the bigger lever.** Examples teach shape;
they cannot tell the model that this collection has a chapter called
`Incremental learning - SuperMemo Help` and no chapter called `physics`. Without
that, `\chapter physics` is a plausible-looking guess that returns nothing — and
"the query looked right but found nothing" is exactly the failure this feature
would otherwise produce most often.

The app already fetches these: the `queryHints` store
(`frontend/src/stores/queryHints.js`) holds `{ chapter, context, arrow }`, the
user's real values, populated from `ListChapters` / `ListContextTags` /
`ListArrowCodes` for the autocomplete. `buildQueryPrompt` takes that snapshot and
lists them, instructing the model to prefer an existing name over inventing one.

The lists are unbounded, so each is **capped at 40 entries** in the prompt. When
truncated the prompt says so explicitly — otherwise the model would infer that an
unlisted chapter does not exist and avoid a perfectly good one. Empty lists (no
graph built yet) are omitted rather than rendered as an empty heading.

### Backend — the translation binding

**`TranslateQuery(profileID, question, system string) (string, error)`** in a
new `search_llm_methods.go`.

It calls `providerFor(profileID)` — so an empty id uses the active profile and a
missing profile yields the existing "no LLM profile configured" error — then runs
one `Stream` call with the system prompt and the question, accumulating deltas
into a buffer, and returns the accumulated text. It emits no Wails events; the
frontend awaits a value.

The system prompt is passed **in** rather than built in Go, so the grammar has
exactly one home. The alternative — duplicating `DIRECTIVES` into Go — is the
drift this design exists to avoid.

### Backend — remembering the chosen model

`AppConfig` (`app.go:22`) is JSON at `os.UserConfigDir()/texere/config.json`, so
a new field costs nothing and is backward compatible — absent means empty means
"use the active profile".

```go
QueryLLMProfileID string `json:"queryLlmProfileId"`
```

It goes on `AppConfig` rather than on `LLMConfig`, because `LLMConfig` is reused
verbatim for `STT` (`app.go:26`) where a query-translation profile would be
meaningless.

Two thin bindings: **`GetQueryLLMProfile() string`** and
**`SetQueryLLMProfile(id string) error`**. The id is passed straight to
`TranslateQuery`, and `providerFor` already treats `""` as "the active profile"
(`llm_methods.go:68-70`), so the default needs no special case anywhere.

### `OmniBar.svelte`

In graph mode only, and only when `profiles.length > 0`, a **second input row**
sits above the query bar, with the profile `<select>` and the button beside it — the same shape
`LLMCell.svelte:184` already uses, where a blank option means "active profile".
Changing it calls `SetQueryLLMProfile`, so the choice survives a restart.

Pressing the button:

1. captures the current text as the question,
2. calls `TranslateQuery(selectedProfileId, question, buildQueryPrompt($queryHints))`,
3. runs `stripToQuery` then `validateQuery` on the reply,
4. on success replaces the input text with the query, leaving it unrun and
   focused; on failure shows the refusal with `Retry` and `Use anyway`.

While in flight the button shows a busy state and is disabled. Errors surface in
the existing log panel the way graph errors already do.

Nothing else about search changes. Enter still runs the literal text.

## Consequences

- **Costs a call per press.** Explicit invocation makes that visible and
  intentional; nothing happens in the background.
- **The prompt grows with the grammar.** Ten directives is small; if the grammar
  grew large the prompt would need trimming, which is a problem worth having.
- **Generated queries can be wrong while valid.** Validation checks vocabulary,
  not intent — `\chapter physics` when you meant `\context physics` passes. This
  is why the query is shown and not run.
- **Quality degrades before the graph is built.** With no `sst_data`, the hint
  lists are empty and the model has only the examples to work from, so it will
  guess names. Acceptable: graph search does not work then either.
- **The prompt grows with the collection.** Bounded by the 40-per-list cap, but a
  collection with hundreds of context tags gives the model a partial view. The
  cap is a starting point, not a tuned value.
- **Quality depends on the profile you pick.** A small local model will produce
  worse queries than a frontier one, and the same button will feel different on
  each — which is the reason the choice is yours and is remembered.
- **Deleting the selected profile falls back silently.** `providerFor` resolves
  an unknown id to a "no LLM profile configured" error, so the picker must treat
  a missing id as blank rather than showing a stale name.
- **`\radius` is texere-only.** The prompt teaches it because the app executes
  it; a query copied to upstream searchN4L would not understand it.

## Testing

**JS unit (`query-prompt.test.js`)** — the substance, since none of this can be
tested through the component:

- `buildQueryPrompt` mentions every token in `DIRECTIVES` and no token outside
  it. This is the test that pins "only what texere executes": adding `\dna` to
  the prompt by hand would fail it.
- **Every query in `EXAMPLES` passes `validateQuery`.** Stops a worked example
  teaching unsupported *syntax*. Note it checks vocabulary, not arity — it would
  not catch an argument passed to a flag declared `valueKind: 'none'`.
- `buildQueryPrompt` includes the chapter, context and arrow names it is given;
  caps each list at 40 and says so when it truncates; and omits a list entirely
  when empty rather than emitting a bare heading.
- `stripToQuery` unwraps a fenced block, a fenced block with a language tag,
  surrounding backticks, a `Query:` prefix, and leading/trailing whitespace; and
  leaves a bare query untouched.
- `validateQuery` accepts every canonical token and every alias; rejects `\dna`,
  `\theme`, `\bookmarks`; reports multiple unknowns; and treats a query with no
  directives at all as valid (plain text is a legal query).

**Go integration (`internal/search`)** — the load-bearing test, and the reason
the examples can be trusted:

- Build a graph from `testdata/corpus/` with the upstream `SSTconfig` registry,
  once per run (~0.9s), then run **every query in `EXAMPLES`** and assert each
  returns a non-empty result of the expected kind (nodes / listings / notes /
  structural). An example that stops returning results — because the corpus
  changed, the library was upgraded, or a route regressed — fails here rather
  than silently degrading the prompt.
- The examples live in `query-prompt.js` (JS) and the assertion runs in Go, so
  the two must be kept in step. The Go test reads the JS list rather than
  duplicating it; a duplicated copy would drift, which is the failure this whole
  design is organised against.
- Assert the build itself succeeds and record `UnknownArrows` as informational.
  The corpus legitimately references arrows absent from `SSTconfig`
  (`extract-fr`, `has-frag`, `consists of`), which produce parse errors without
  blocking ingestion — failing on them would make the test hostage to upstream.

**Go unit** — `TranslateQuery` returns the accumulated text from a stubbed
provider; surfaces the provider's error; and returns the existing "no LLM
profile configured" error when none is set. `SetQueryLLMProfile` round-trips
through the config file, and an absent field reads back as `""`.

**Manual** — its own test-plan document, matching prior slices: the button is
absent with no profile and present with one; a question becomes an editable
query that is not run; Enter still runs literal text unchanged; an unsupported
directive is refused and `Use anyway` overrides; a provider error surfaces
without breaking search; the picker defaults to the active profile, a chosen
profile survives a restart, and deleting the chosen profile falls back to the
active one instead of erroring.

## Out of scope

- Executing the generated query automatically.
- Any change to fuzzy mode, to `GraphQueryCell`, or to `compute-host.js`.
- ~~The explorer's own query bar~~ — done, see the 2026-08-06 revision below.
- A separate provider *configuration* for search — the profile set stays shared;
  only which profile to use is a search preference.
- Explaining results, summarising them, or answering the question directly. The
  output is a query, nothing else.
- Teaching upstream-only syntax (`\theme`, `\bookmarks`, `\dna`, `\terms`,
  `\concepts`).


## Revision — the ask box was split out of the query bar (2026-08-06)

The design originally put the button *inside* the query bar, so the question was
typed into the same input as the query. In use that fights itself: the omnibar
searches as you type and opens the directive autocomplete, so typing *"what is in
my kubernetes notes?"* ran a literal multi-word search and dropped a match list
over the input before the button could be reached.

The ask input is now its own row above the query bar. It has no `on:input`, no
autocomplete and no search — nothing typed there is ever queried. Enter
translates, stopping propagation so it cannot reach the panel handler that would
run the query bar's search or accept an autocomplete item. The query bar itself
is untouched.

This also removed a short-lived "translated from …" line. That existed because
the question disappeared the moment the query replaced it; with the question
still on screen above, it was redundant.

Worth recording plainly: this was offered as an option during the original
brainstorming and not chosen, and the in-bar version had to be built and used
before the problem was obvious. The translation quality was never the issue —
*"what is in my kubernetes notes?"* → `\notes \chapter Kubernetes notes` was
correct from the first try, using a real chapter name from the live hints.


## Revision — the ask box is shared, and the explorer has one (2026-08-06)

The explorer's always-visible query bar was originally out of scope, on the
reasoning that one surface should prove the shape first. It has, so it now has an
ask box too.

Rather than duplicate the block, the input, model picker, button and the
refusal/error cards were extracted into `AskBox.svelte`. It owns the LLM call,
the prompt, `stripToQuery`/`validateQuery` and every piece of state, renders
nothing when no profile is configured, and emits the finished query. Each host
decides what happens next:

- **`OmniBar`** drops it into its query bar, which searches as you type.
- **`GraphExplorer`** fills its query bar and stops. That surface never runs a
  query without an explicit submit, and a generated `\notes` scan — a full-DB
  scan — should not be the exception.
- **`GraphQueryCell`** (notebook) does the same, persisting through the cell's
  existing `onChange` path. Hidden when the notebook is read-only, since the
  query cannot be edited there.

All three surfaces that have a query bar now have an ask box, and
`TranslateQuery`, `buildQueryPrompt`, `validateQuery` and `stripToQuery` appear
in exactly one file.

`OmniBar` lost 127 lines in the process, and neither host now references
`TranslateQuery`, `buildQueryPrompt` or `validateQuery`. The alternative was two
copies of prompt-building, validation and refusal handling that had to change
together forever, which is exactly the drift this design was organised against
for the grammar itself.
