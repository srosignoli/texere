# Natural-language questions into graph queries

**Date:** 2026-08-05
**Status:** Proposed — awaiting approval
**Feature area:** Graph search (`OmniBar`) + LLM

## Summary

In graph mode, an optional button turns the text you have typed into a
searchN4L query using the LLM profile already configured for the LLM cell. The
generated query is written **into the query bar, not executed** — you read it,
edit it, and run it yourself.

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
- **The generated query is not auto-run.** It lands in the bar for inspection.
- **Unknown directives are refused, with an override.** The message names what
  was rejected and why; `Use anyway` exists because the validator may lag a
  library upgrade, and being wrong should not be a dead end.
- **One-shot, not streamed.** The answer is one line. The backend accumulates
  the provider stream and returns the finished string rather than making the
  frontend reassemble deltas for a single value.
- **No new configuration.** It reuses the LLM cell's profiles and active
  profile. Rejected: a separate profile for search, which doubles the settings
  surface for no benefit.

## Architecture

### `frontend/src/lib/query-prompt.js` (new, dependency-free)

Three pure functions, all testable under `node --test` — which matters because
this repo has no Svelte component harness, so logic left in a `.svelte` file has
no tests at all.

- **`buildQueryPrompt()`** → the system prompt, rendered from `DIRECTIVES`,
  `MATCHING` and `COMBINING`. Each directive contributes its token, aliases,
  argument shape, summary and example. It ends with the output contract: emit
  the query string and nothing else.
- **`stripToQuery(raw)`** → the model's reply reduced to a bare query. Models
  routinely wrap output in code fences or add a sentence of explanation despite
  instructions; this removes fences, surrounding backticks, a leading
  `Query:` label, and surrounding whitespace. Without it the contract holds only
  as long as the model cooperates.
- **`validateQuery(text)`** → `{ ok, unknown: string[] }`. Every `\token` in the
  text is checked against the canonical tokens and aliases in `DIRECTIVES`.
  Unknown ones are returned for the refusal message.

### Backend — one new binding

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

### `OmniBar.svelte`

In graph mode only, and only when `profiles.length > 0`, a button sits at the
right of the input. Pressing it:

1. captures the current text as the question,
2. calls `TranslateQuery('', question, buildQueryPrompt())`,
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
- **Quality depends on the profile.** A small local model will produce worse
  queries than a frontier one, and the same button will feel different on each.
- **`\radius` is texere-only.** The prompt teaches it because the app executes
  it; a query copied to upstream searchN4L would not understand it.

## Testing

**JS unit (`query-prompt.test.js`)** — the substance, since none of this can be
tested through the component:

- `buildQueryPrompt` mentions every token in `DIRECTIVES` and no token outside
  it. This is the test that pins "only what texere executes": adding `\dna` to
  the prompt by hand would fail it.
- `stripToQuery` unwraps a fenced block, a fenced block with a language tag,
  surrounding backticks, a `Query:` prefix, and leading/trailing whitespace; and
  leaves a bare query untouched.
- `validateQuery` accepts every canonical token and every alias; rejects `\dna`,
  `\theme`, `\bookmarks`; reports multiple unknowns; and treats a query with no
  directives at all as valid (plain text is a legal query).

**Go unit** — `TranslateQuery` returns the accumulated text from a stubbed
provider; surfaces the provider's error; and returns the existing "no LLM
profile configured" error when none is set.

**Manual** — its own test-plan document, matching prior slices: the button is
absent with no profile and present with one; a question becomes an editable
query that is not run; Enter still runs literal text unchanged; an unsupported
directive is refused and `Use anyway` overrides; a provider error surfaces
without breaking search.

## Out of scope

- Executing the generated query automatically.
- Any change to fuzzy mode, to `GraphQueryCell`, or to `compute-host.js`.
- The explorer's own query bar — same grammar, and worth doing later, but this
  slice proves the shape in one place first.
- A separate LLM profile for search.
- Explaining results, summarising them, or answering the question directly. The
  output is a query, nothing else.
- Teaching upstream-only syntax (`\theme`, `\bookmarks`, `\dna`, `\terms`,
  `\concepts`).
