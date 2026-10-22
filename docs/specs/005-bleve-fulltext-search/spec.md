# Feature Specification: Replace text matching with Bleve full-text search engine

**Feature Branch**: `005-bleve-fulltext-search`
**Created**: 2026-04-24
**Status**: Draft
**Input**: User description: "Replace the text-matching step inside the searchN4L pipeline with a Bleve-backed full-text index. All `\` commands continue to work unchanged — only the plain-text portion of a query is routed through Bleve. Build graph (Ctrl+G) rebuilds the index alongside BadgerDB. Translate existing query modifiers (`!exact!`, `|exact|`, `(accents)`, `brain&!notes`, `\"fish soup\"`, `strange<->kind`, `flo:*`) to the equivalent Bleve query primitives."

## Clarifications

### Session 2026-04-25

- Q: Where should the Bleve integration live (library vs consumer vs split)? → A: Library-owned — SSTorytime-KV owns Bleve index, translation, and query API; Texere consumes it.
- Q: How should results be ordered (Bleve score vs stable insertion order)? → A: Bleve relevance score orders the outer result list (candidate nodes for `routeNodeSearch`; seed candidates for path/sequence resolution). Hop-count ordering for paths and sequence-chain ordering remain unchanged. `NodePtr` is the deterministic tie-breaker.
- Q: What analyzer strategy should the Bleve index use? → A: Dual-field analyzed index — `text_en` (`standard` analyzer + ASCII-fold + English stemmer), `text_cjk` (`cjk` analyzer with bigram tokenizer), plus `text_raw` (keyword, lowercased) for exact-match (`!A!`/`|A|`) and prefix (`flo:*`) queries. Analyzed-field queries are disjoined across `text_en` and `text_cjk`.
- Q: What should the overlay do when a collection has no `bleve/` index (pre-upgrade state)? → A: Surface an actionable error in the overlay ("Press Ctrl+G to build the search index") and return zero results until the user rebuilds. No auto-trigger and no substring-matcher fallback.
- Q: How should the system handle malformed operator syntax in a query (e.g., unclosed quote, trailing `&`, invalid slop)? → A: Hard error to the overlay with a parse-error message indicating position; return zero results. Operator parsing only activates in clearly-querified positions (no surrounding whitespace, recognised delimiters), so free-form text containing `&` / `<->` / `:*` separated by whitespace is treated as literal tokens and does not error.

## Pre-Spec Research: Current Text-Matching Implementation

A read of the SSTorytime-KV source **and** the Texere consumer code reveals a finding that materially affects the scope of this replacement — reported up front so the user can confirm before committing to a plan.

### Where text matching happens today

There are **two distinct text-match sites**, not one:

| # | Call site | File:Line | What it does |
|---|-----------|-----------|--------------|
| 1 | `SearchService.searchNodes` (Texere) | `internal/search/search.go:270` | Iterates **every node** via `BadgerKV.IterateNodes`, applies chapter filter, then filters each term with either exact-equality (for `!A!`/`|A|`) or `strings.Contains(normalizeForSearch(n.S), term)` (for plain / `(accents)`). This is the **primary** text-search path driven by the overlay. |
| 2 | `BadgerKV.SearchNodesByName` (SSTorytime-KV) | `kv_badger.go:463` | Prefix-scans the `text:` secondary index, applies case-insensitive `strings.Contains(text, query)`. Called by `SolveNodePtrs` (path endpoint resolution, `store_paths.go:42`) and by `SearchService.searchStructural` for sequences (`search.go:526`). Also wrapped by `GetDBNodePtrMatchingNCCS` (`store_indexes.go:60`), which Texere does **not** currently use for the main node search. |

Both sites do case-insensitive substring matching. Texere's `normalizeForSearch` additionally Unicode-accent-folds at site 1; site 2 does not.

### Where text matching sits in the query pipeline

```
overlay query string
  ↓
SST.DecodeSearchField(query)                # search.go:121, parses \ commands into SearchParameters
  ↓ produces SearchParameters{Name, From, To, Chapter, Context, Arrows, Sequence, Range, ...}
  ↓
SearchService.classifyQuery(params)         # routes by which fields are populated
  ├─→ routeNodeSearch  → searchNodes(params, ...)       ← SITE 1 (IterateNodes + strings.Contains)
  ├─→ routeStructural  → searchStructural(params, ...)  ← SITE 2 (SearchNodesByName for sequences; SolveNodePtrs for \from/\to endpoints)
  ├─→ routeListing     → searchListing(...)
  └─→ routeNotes       → searchNotes(...)
  ↓
orbit expansion (GetNodeOrbit) + context filter + result formatting
```

### Key facts

- The `\` command parser (`DecodeSearchField`) already strips modifiers like `!A!`, `|A|`, and `(fangzi)` into `params.Name` via `SST.IsExactMatch`, but their *exact-match* vs *accent-fold* semantics are **re-applied in Texere**, not in the library. The library's `SearchNodesByName` always substring-matches case-insensitively regardless of the original delimiter.
- `params.Name` is a `[]string` of tokens — multi-word search text is split on whitespace and currently evaluated with **OR** semantics in Texere (`searchNodes`).
- Boolean operators `&`, `|`, `!` at token level are **not** parsed or applied today. `"fish soup"` (phrase), `strange<->kind` (proximity), `flo:*` (prefix) have **no dedicated handling** either — they fall through as literal substrings.
- Orbit expansion, context filtering (`SST.MatchContexts`), chapter filter, and result formatting (`GraphNode` struct, `StructuralChain` struct) all run **after** the text match completes and are independent of how the match was produced.
- Build graph (`internal/graph/builder.go:BuildFromCollection`) writes to `<collectionDir>/sst_data/` (BadgerDB) via `IdempDBAddNode` / `Edge` / `SavePageMap`. No text index exists beyond BadgerDB's own `text:` secondary key. Bleve would be a new, parallel index written during the same pass.

### What this means for scope

The feature description says "replace the function that receives the text portion of a searchN4L query and returns matching nodes". In practice that is **not a single function** — the text match is implemented once in SSTorytime-KV (`SearchNodesByName`, used for path endpoints and sequence lookup) and once again in Texere (`searchNodes`, used for the primary `routeNodeSearch` flow). Replacing only the library function would leave the overlay's main search path untouched.

The spec below therefore scopes the replacement across **both** sites: a Bleve-backed matcher owned by SSTorytime-KV, plus wiring in Texere so `searchNodes`, `SolveNodePtrs`, and the sequence path all consume it. The user's success criteria (stemming, boolean logic, phrase/proximity/prefix match, accent folding) require this wider scope — they cannot be satisfied by the library change alone.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Stemming and multi-language matching in node search (Priority: P1)

As a note-taker querying my N4L collection through the Build-graph overlay, when I search for `running` I want to also get nodes containing `run`, `runs`, and `runner`, and when I search for a Chinese word alongside English notes I want the Chinese token to analyze correctly rather than be treated as a single opaque substring.

**Why this priority**: This is the headline improvement. Without stemming and proper tokenization, users get zero hits on morphological variants and on mixed-language notes — the most common disappointment with the current substring matcher. All other Bleve features (phrase, proximity, prefix) are refinements layered on this foundation.

**Independent Test**: Build graph on a collection containing mixed English/Chinese vocabulary notes, issue a plain-text query, and verify that (a) a query for `run` returns nodes containing `running`/`runs`/`runner`, (b) a Chinese query term returns nodes containing that token even when surrounded by English text.

**Acceptance Scenarios**:

1. **Given** a collection with nodes `"I was running in the park"` and `"he runs every morning"`, **When** the user searches `run`, **Then** both nodes appear in the results.
2. **Given** a collection with the node `"房子 means house"`, **When** the user searches `房子`, **Then** the node appears in results.
3. **Given** a collection with the node `"fángzi"`, **When** the user searches `(fangzi)`, **Then** the node appears in results (accent-insensitive match via parentheses syntax).
4. **Given** a query that matched under the old substring matcher, **When** the same query runs under Bleve, **Then** the node still appears in results (no regression on queries that already worked).

---

### User Story 2 — Boolean, phrase, proximity, and prefix operators (Priority: P1)

As a power user, I want to express targeted queries using boolean combinations, exact phrases, word adjacency, and prefix wildcards — the same operator grammar SSTorytime's PostgreSQL backend offers.

**Why this priority**: Tied for P1 with stemming. The presence of these operators is the justification for adopting a real search engine rather than a smarter substring matcher. Users who already know the `&`, `|`, `!`, `"…"`, `<->`, `<N>`, `:*`, `!…!`, `|…|`, `(…)` grammar should see them work identically.

**Independent Test**: For each operator syntax, craft a query whose result set under substring-matching is demonstrably wrong (either missing expected hits or returning obvious false positives), then verify that Bleve produces the correct set.

**Acceptance Scenarios**:

1. **Given** nodes `"brain research"`, `"brain notes"`, and `"notes about music"`, **When** the user searches `brain&!notes`, **Then** only `"brain research"` is returned.
2. **Given** nodes `"fish soup recipe"` and `"fish curry and soup"`, **When** the user searches `"fish soup"`, **Then** only `"fish soup recipe"` is returned (phrase match, not bag-of-words).
3. **Given** a node `"she had a strange kind demeanour"`, **When** the user searches `strange<->kind`, **Then** the node is returned (adjacency, slop=0).
4. **Given** a node `"a strange but kind woman"`, **When** the user searches `strange<2>woman`, **Then** the node is returned (slop=2).
5. **Given** nodes `"flower"`, `"floor"`, `"flora"`, and `"frog"`, **When** the user searches `flo:*`, **Then** the first three are returned and `"frog"` is not.
6. **Given** nodes `"A"`, `"Apple"`, `"Banana"`, **When** the user searches `!A!`, **Then** only the node whose text is exactly `"A"` is returned.
7. **Given** nodes `"A"`, `"a"`, `"Apple"`, **When** the user searches `!A!`, **Then** the nodes whose text is exactly `"A"` and exactly `"a"` are both returned (case-insensitive exact match); `"Apple"` is not.
8. **Given** the query `word1 | word2`, **When** evaluated against nodes where some contain `word1`, some contain `word2`, and some contain both, **Then** all three groups appear in results (disjunction).

---

### User Story 3 — Text match composes with `\` commands exactly as before (Priority: P1)

As a user issuing combined queries like `notes about chinese \context restaurant \limit 20`, I want the Bleve matcher to operate only on the plain-text fragment and leave every `\` command's behaviour unchanged — the same chapter filter, the same context filter, the same orbit expansion, the same result cap, the same `\from`/`\to` path traversal, the same `\sequence` lookup, the same `\notes` / `\chapter` / `\arrow` listings.

**Why this priority**: This is a zero-regression guarantee. It blocks adoption if violated: users trust their existing queries, and every `\` command represents a distinct use case. The feature is worthless if it breaks any of them.

**Independent Test**: Run every documented `\` command once under the current implementation, capture results, run the same commands under Bleve, and diff. Any difference is a regression to investigate.

**Acceptance Scenarios**:

1. **Given** the query `chinese \context restaurant`, **When** executed, **Then** the result set contains exactly the nodes whose text matches "chinese" (via Bleve) AND that have at least one link whose context matches "restaurant" (unchanged behaviour).
2. **Given** the query `\from "chapter 1" \to "chapter 5" \paths`, **When** executed, **Then** path traversal operates identically to today — endpoints resolve via text lookup (now Bleve-backed), path enumeration via the existing in-memory NODE_DIRECTORY.
3. **Given** the query `\sequence my-sequence-marker`, **When** executed, **Then** sequence lookup behaves as today, using Bleve only for the token-match step.
4. **Given** the query `\chapter recipes \arrow then`, **When** executed (no text portion), **Then** no Bleve query is issued and results are identical to today.
5. **Given** `\limit 5`, **When** any query applies the cap, **Then** the cap is applied to the final result set after orbit expansion, as today.
6. **Given** the query matches Texere's `routeListing` or `routeNotes` classifier (which today do not text-match nodes), **When** executed, **Then** no Bleve call occurs and behaviour is unchanged.

---

### User Story 4 — Build graph keeps the index in sync (Priority: P2)

As a user who edits `.n4l` files and presses Ctrl+G to rebuild, I want my next search to reflect the current contents of my notes without any extra step — the Bleve index is rebuilt during Build graph alongside BadgerDB, transparently.

**Why this priority**: One step below P1 because existing users already expect Build graph to refresh search state; this preserves that contract. Failure surfaces as stale results, not a crash.

**Independent Test**: Add a new node to a `.n4l` file, press Ctrl+G, search for a word that only appears in that new node — the node must appear. Delete a node, Ctrl+G, search — the node must no longer appear.

**Acceptance Scenarios**:

1. **Given** a fresh collection with no prior indexes, **When** the user presses Ctrl+G, **Then** both `sst_data/` (BadgerDB) and `bleve/` (Bleve) directories are created under the collection folder.
2. **Given** an existing collection that was built before this feature shipped, **When** the user presses Ctrl+G, **Then** the `bleve/` directory is created fresh and populated with every node.
3. **Given** a node is removed from a `.n4l` file and Ctrl+G is pressed, **When** the user searches for text that only appeared in that node, **Then** zero results are returned (the Bleve index is rebuilt, not appended).
4. **Given** Build graph completes successfully, **When** the user views the toast / status message, **Then** it reports completion; any node-count metric it reports today is unchanged.

---

### User Story 5 — `bleve/` directory excluded from version control (Priority: P3)

As a user who checks their notes collection into git, I don't want the Bleve index files committed — the index is a derived artefact that can be rebuilt from `.n4l` sources at any time.

**Why this priority**: Lowest priority because it's a one-line `.gitignore` addition and only matters the first time a user runs `git status` after building. Low impact, but leaving it out is a papercut that would immediately annoy users.

**Independent Test**: In a fresh clone, run Build graph, run `git status`, verify `bleve/` is not listed as untracked.

**Acceptance Scenarios**:

1. **Given** the repository `.gitignore`, **When** the user views it after this feature ships, **Then** it contains an entry excluding `bleve/` (or the equivalent path pattern used for per-collection index directories).
2. **Given** a collection directory stored inside the repo's tree, **When** Build graph creates `bleve/`, **Then** `git status` does not list those files as untracked.

---

### Edge Cases

- **Empty text portion**: Query consists only of `\` commands (e.g., `\chapter recipes`). No Bleve query is issued; routing is unchanged.
- **Text portion produces zero Bleve hits but `\` commands could produce hits**: The overall query still returns zero results (text-AND-`\`-filter semantics are unchanged from today — the text match remains a required filter when present).
- **Query mixes bare text and modifiers**: e.g., `brain notes "fish soup"`. Each token is parsed in order and translated to the appropriate Bleve primitive, then combined per the existing OR-between-name-tokens behaviour (unless explicit `&`/`|` operators override it).
- **`\limit N` smaller than hit count**: Bleve returns more than N hits; the pipeline caps to N after orbit expansion, matching current behaviour.
- **Collection has a `sst_data/` but no `bleve/`** (pre-upgrade state, and user opens overlay without pressing Ctrl+G): The search feature must fail gracefully — surface a "rebuild index to enable search" message rather than crash or silently return empty results.
- **Collection path contains Unicode or spaces**: The `bleve/` directory must be created at the same level as `sst_data/` regardless of path content.
- **Bleve index corrupted / unopenable**: Search falls back to an error state surfaced to the overlay; user remediation is to press Ctrl+G.
- **`!A!` with a single-character token**: Must match the exact string "A" and not "Apple", "And", or "a letter A". Bleve's default analyzer would drop single-character tokens; the exact-match path must use a term query against a non-analyzed field variant.
- **`(fangzi)` when the collection contains both `fangzi` and `fángzi`**: Both must match (accent folding is bidirectional).
- **Query parses as a Dirac-notation `<from|ctx|to>` form** (handled in `DecodeSearchField` at `SSTorytime.go:4233`): Not affected — Dirac rewriting runs before text matching and feeds `\from`/`\to`/`\context`, none of which go through Bleve.
- **Malformed operator syntax** (unclosed quote, unbalanced paren, trailing `&`, `<-1>` slop): hard parse error returned to the overlay with a position indicator; zero results.
- **Operator-looking characters in literal positions** (e.g., `chemistry C & S reaction`): treated as plain literal tokens; no operator parsing, no parse error.

---

## Requirements *(mandatory)*

### Functional Requirements

**Scope — what changes and what does not**

- **FR-001**: The text-matching logic at `internal/search/search.go:270` (`SearchService.searchNodes`, currently `IterateNodes` + `strings.Contains`) MUST be replaced with a call into a new SSTorytime-KV API that returns Bleve-matched `[]NodePtr`. The surrounding chapter filter, context filter, orbit expansion, and `GraphNode` formatting MUST remain in Texere and behaviourally identical.
- **FR-002**: The text-matching logic in `SearchNodesByName` (SSTorytime-KV `kv_badger.go:463`) — called by `SolveNodePtrs` (path endpoint resolution) and by `SearchService.searchStructural` for sequences — MUST be replaced with a Bleve-backed lookup that returns the same `[]NodePtr` result type.
- **FR-002a**: SSTorytime-KV MUST own the Bleve dependency, the index schema, the index lifecycle (create / rebuild / open / close), and the query-translation layer (operator grammar → Bleve primitives). Texere MUST consume this through a typed Go API and MUST NOT import `github.com/blevesearch/bleve/v2` directly.
- **FR-003**: The query parser `DecodeSearchField` (SSTorytime.go:4166), the `\` command set (`\from`, `\to`, `\paths`, `\sequence`, `\chapter`, `\context`, `\arrow`, `\notes`, `\limit`, and every other `CMD_*` constant), and the four-way query classifier (`routeNodeSearch`, `routeStructural`, `routeListing`, `routeNotes`) MUST remain unchanged.
- **FR-004**: The `routeListing` and `routeNotes` paths MUST NOT issue Bleve queries — they do not text-match nodes today.
- **FR-005**: The `\limit N` cap MUST continue to apply after orbit expansion, exactly as today.

**Query translation**

- **FR-006**: The system MUST provide a query-translation layer that accepts the plain-text portion of a searchN4L query (the `SearchParameters.Name` token list after `DecodeSearchField` has run) plus any extracted modifiers, and produces a Bleve query object.
- **FR-007**: Plain text tokens without operators MUST translate to a match query using the collection's language analyzer, with disjunction (OR) between tokens — preserving today's OR-across-`params.Name` semantics.
- **FR-008**: A double-quoted phrase (`"fish soup"`) MUST translate to a phrase query that matches the exact ordered sequence of tokens.
- **FR-009**: Bang-bounded (`!A!`) or pipe-bounded (`|A|`) tokens MUST translate to an exact-term query against a non-analyzed field variant so that single-character and punctuation-exact matches succeed.
- **FR-010**: Parenthesized tokens (`(fangzi)`) MUST translate to a match query that is case- and accent-folded, matching both accented and un-accented forms in the collection.
- **FR-011**: The infix operator `&` MUST translate to conjunction, `|` to disjunction, and `!` (as a prefix unary) to a must-not clause. Operator precedence MUST match PostgreSQL `ts_vector` conventions (`!` binds tighter than `&`, which binds tighter than `|`).
- **FR-012**: The proximity operator `<->` MUST translate to a phrase query with slop=0 (strict adjacency), and `<N>` MUST translate to a phrase query with slop=N.
- **FR-013**: The prefix operator `word:*` MUST translate to a prefix query on the stem `word`.
- **FR-014**: Translation MUST be case-insensitive for every primitive, including the exact-match path (`!…!` / `|…|`). Implementation lowercases on both sides — the `text_raw` keyword field is lowercased at index time, and term/prefix queries lowercase the input before lookup. This preserves today's behaviour, where the substring matcher at `internal/search/search.go:270` ran case-insensitively via `normalizeForSearch`.
- **FR-015**: When a query mixes multiple primitives at the top level without explicit boolean operators (e.g., `brain "fish soup" flo:*`), each primitive MUST be combined with disjunction (OR), preserving today's default-OR semantics for `params.Name`.
- **FR-015a**: Operator parsing MUST only activate in clearly-querified positions — operators directly adjacent to operands without surrounding whitespace (e.g., `brain&!notes`, `flo:*`, `<2>`), or inside recognised delimiters (`"…"`, `!…!`, `|…|`, `(…)`). Operator-like characters in literal positions (e.g., `chemistry C & S reaction`, `effect of <ions> on pH`) MUST be treated as plain text tokens and MUST NOT trigger operator parsing or parse errors.
- **FR-015b**: When a query contains malformed operator syntax (e.g., unclosed quote, unbalanced parenthesis, trailing operator, invalid proximity slop), the system MUST return a parse error indicating the problematic position and MUST return zero results. The system MUST NOT silently drop the malformed fragment, fall back to literal-text search of the whole string, or attempt partial parsing.

**Indexing**

- **FR-016**: During Build graph (`BuildFromCollection`), each ingested node MUST be indexed into a Bleve index whose files live in `<collectionDir>/bleve/`, alongside the existing `<collectionDir>/sst_data/` BadgerDB.
- **FR-017**: Each Bleve document MUST contain enough information to (a) match against the node's text, (b) resolve back to the node's BadgerDB `NodePtr`, (c) filter by chapter (since chapter filter applies post-match today, but a Bleve pre-filter is an allowed optimization), and (d) record the source file path (for future use; not required by any query today).
- **FR-018**: The Bleve index MUST be rebuilt from scratch on each Build graph invocation, matching the existing contract for BadgerDB (`os.RemoveAll(dbPath)` at `builder.go:37`).
- **FR-019**: If Build graph fails partway, the Bleve index on disk MUST be left in a state that causes the next Build graph to recover (either a consistent previous build, or a cleanly-cleared directory). Partially-written indexes that appear healthy but return wrong results are unacceptable.
- **FR-020**: Each indexed document MUST contain three text-bearing fields covering the supported query primitives: `text_en` (custom analyzer: unicode tokenizer + lowercase + ASCII-folding + English Snowball stemmer) for stemmed Latin-script matching with case and accent folding; `text_cjk` (Bleve `cjk` analyzer with bigram tokenizer) for CJK content; and `text_raw` (keyword field, lowercased) for exact-match (`!A!`/`|A|`) and prefix (`flo:*`) queries. Analyzed-field queries (plain tokens, phrases, proximity, accent-fold parens) MUST be issued as a disjunction across `text_en` and `text_cjk`. Per-node or per-chapter analyzer selection is out of scope; the same three-field schema applies to every node.

**Runtime behaviour**

- **FR-021**: When `SearchService.searchNodes` receives a query whose `params.Name` is empty (wildcard-only search), the system MUST bypass the Bleve index and behave exactly as today (currently: returns every node that passes chapter + context filters up to the limit).
- **FR-022**: When the Bleve index does not exist (collection built before this feature shipped, or never built), the overlay MUST surface a clear, actionable error message instructing the user to press Ctrl+G to build the search index, and MUST return zero results until the index exists. The system MUST NOT auto-trigger Build graph and MUST NOT fall back to a substring matcher. `\` commands that do not require text matching (e.g., `\chapter` listings, `\notes` lookups) are unaffected and continue to work without an index.
- **FR-023**: The result data structures returned by `searchNodes` (`[]GraphNode`), `searchStructural` (`[]StructuralChain`), and `SolveNodePtrs` (`[]NodePtr`) MUST remain unchanged. Downstream consumers (orbit expansion, context filtering, JSON marshalling to the overlay) MUST work without modification.
- **FR-023a**: For `routeNodeSearch`, the outer result list MUST be ordered by Bleve relevance score (descending), with `NodePtr` as a deterministic tie-breaker. For path queries (`\paths`), seed candidates returned to the path enumerator MUST be ordered by Bleve score, but path-internal ordering MUST remain governed by `sortPathsByLength` (`store_paths.go:32`). For sequence queries (`\sequence`), seed candidates MUST be ordered by Bleve score, but sequence-chain ordering MUST remain governed by the in-memory `(then)`-arrow walk in `GetSequenceContainers` / `GetLongestAxialPath`. Bleve scoring MUST NOT reorder items inside any single path or sequence chain.

**Repo hygiene**

- **FR-024**: The repository `.gitignore` MUST exclude `bleve/` (scoped so it applies to every collection directory, mirroring the current treatment of `sst_data/`).

### Key Entities

- **Bleve Document (per node)**: Represents one indexed N4L node. Fields: `text_en` (analyzed with `standard` + ASCII-fold + English stemmer), `text_cjk` (analyzed with the `cjk` bigram analyzer), `text_raw` (keyword field, lowercased only — used for exact-match `!…!`/`|…|` and prefix `word:*` queries), `chapter` (keyword), `source_file` (keyword, recorded for future use), and the node's BadgerDB `NodePtr` (stored so matches can be cross-referenced back to the graph).
- **Bleve Index Directory**: A filesystem directory at `<collectionDir>/bleve/` holding the Bleve index files. Created and reset by Build graph. Git-ignored. Opened read-only by the search service.
- **Translated Query**: The Bleve query object produced by the translation layer from a `SearchParameters.Name` token list plus inline operator syntax. Internally a tree of Bleve primitives (match, phrase, term, prefix, conjunction, disjunction, must-not). Not user-visible; exists only as a runtime bridge between the `\` command parser and the Bleve index.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every query that produced results under the current implementation produces at least those same results under Bleve (zero regressions on the "did work" set; additional hits from stemming are acceptable and expected).
- **SC-002**: A `brain&!notes` query against a collection where some nodes contain "brain" alone, some contain "brain" and "notes" together, and some contain "notes" alone, returns exactly the first group — verified by spot-check.
- **SC-003**: A `"fish soup"` phrase query against a collection containing both the phrase and the disjoint words "fish" / "soup" returns only the phrase hits.
- **SC-004**: A `strange<->kind` query returns nodes where the two words are adjacent; a `strange<2>woman` query tolerates up to 2 words between them.
- **SC-005**: A `(fangzi)` query returns nodes containing `fángzi`, `Fángzi`, or `FANGZI`, and vice versa — tested against a representative accented-Latin and CJK sample.
- **SC-006**: A `flo:*` query returns nodes containing `flow`, `floor`, `flora`, and does not return nodes containing only `frog`.
- **SC-007**: A `!A!` query against a collection containing nodes `"A"`, `"Apple"`, and `"arrows"` returns only the node whose text is exactly `"A"`.
- **SC-008**: Build graph duration on a representative collection (1 000–5 000 nodes) increases by no more than 200 ms versus the pre-Bleve baseline.
- **SC-009**: The on-disk Bleve index for a 5 000-node collection occupies under 50 MB. (Rough proportionality check; precise target depends on average node text length.)
- **SC-010**: Every `\` command documented in the existing user guide returns results identical in content and ordering to the pre-Bleve implementation when the text portion is empty. When the text portion is non-empty, results are identical on queries whose text portion does not rely on new Bleve-only features (stemming / boolean / phrase / proximity / prefix).
- **SC-011**: On a fresh clone, running Build graph then `git status` does not list any Bleve index files as untracked.
- **SC-012**: A user who opens the overlay with a pre-Bleve collection (no `bleve/` directory) sees a clear actionable error message, not a crash or silent empty result.
- **SC-013**: A query with an unclosed quote or unbalanced paren produces a parse-error message in the overlay indicating the problematic position; a query containing literal `&` / `<->` / `:*` separated by whitespace runs without error and matches as plain text.

---

## Assumptions

- **Single-writer contract preserved**: BadgerDB today requires that no search service be open while Build graph runs (`builder.go` comment at line 24–26). Bleve has the same requirement, and the existing "close search, rebuild, reopen" sequence in Texere will cover both.
- **Language analyzer choice is a v1 decision, not a user-configurable one**: A single Bleve analyzer (or a simple mapping by file/chapter) is acceptable for v1; per-query language selection is out of scope. The plan phase picks the specific analyzer.
- **SSTorytime-KV can grow a new public API**: The feature ships a new Bleve-backed search API in SSTorytime-KV, consumed by Texere. The module is published at `github.com/srosignoli/sstorytime-kv` and already under this user's control, so a minor version bump is available.
- **Pure-Go constraint stands**: Texere's constitution requires pure-Go dependencies. Bleve (`github.com/blevesearch/bleve/v2`) is pure Go; this assumption is verified at the plan phase but stated here for traceability.
- **CJK analyzer is required, not optional**: Bleve's CJK support lives in `github.com/blevesearch/bleve/v2/analysis/lang/cjk` (pure Go). The dual-field index decided in clarification Q3 makes this a hard dependency; falling back to Unicode-segmentation-only matching is no longer an option.
- **`bleve/` next to `sst_data/` is the right location**: Per-collection, rebuildable, mirrors the existing convention. No central/shared Bleve index is contemplated.
- **Overlay UX for "index missing" is a standard error toast**: No new UI component is required; the existing error-surfacing mechanism in the overlay is reused (see the `GraphError` field at `search.go:117`).
- **Text-AND-`\`-filter semantics are preserved**: When a query has both a text portion and `\` filters, results must satisfy both — matching today's behaviour. Bleve is not allowed to "improve" this into a score-based combined ranking.
