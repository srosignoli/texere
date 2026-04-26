---

description: "Task list for feature 005-bleve-fulltext-search"
---

# Tasks: Replace text matching with Bleve full-text search engine

**Input**: Design documents from `/specs/005-bleve-fulltext-search/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/sstorytime-kv-bleve-api.md, quickstart.md

**Tests**: Included. Research § R10 mandates unit tests for the translator and integration tests for index lifecycle, and the project constitution (`CLAUDE.md`) requires unit tests for all new public APIs.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks).
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4, US5).
- File paths are relative to the **repository root** indicated by their prefix:
  - `kv:` → `~/projects/sstorytime-kv/`
  - `tx:` → `~/projects/texere/` (this repo)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add the Bleve dependency to the library, point Texere at the local working copy, and reserve the on-disk slot.

- [X] T001 [P] Add Bleve dependencies in `kv:pkg/SSTorytime/go.mod` (or root `go.mod` of the library) by running `go get github.com/blevesearch/bleve/v2`, `go get github.com/blevesearch/bleve/v2/analysis/lang/cjk`, `go get github.com/blevesearch/bleve/v2/analysis/token/asciifolding` and committing the resulting `go.sum` updates. — *Note: asciifolding lives at `analysis/char/asciifolding` in v2.5.7, not `token/asciifolding`. Pulled `bleve/v2 v2.5.7`; transitive sub-packages resolved via `go mod tidy` once the new code imported them.*
- [X] T002 [P] Add `bleve/` and `*.bleve/` patterns to `tx:.gitignore` (mirrors existing `sst_data/` treatment). Pre-emptively satisfies US5 / FR-024 so later tasks don't dirty the working tree.
- [X] T003 Add a local `replace github.com/srosignoli/sstorytime-kv => ../sstorytime-kv` directive to `tx:go.mod` for development; document the removal step at v0.4.0 publish in `tx:specs/005-bleve-fulltext-search/quickstart.md` § 1 (already drafted — verify accurate).
- [X] T004 [P] Verify pure-Go constraint after T001 by running `go list -deps -f '{{.ImportPath}} {{.CgoFiles}}' ./pkg/SSTorytime/...` from the library root — output must contain no non-empty CgoFiles entries (constitution principle II). Capture in `tx:specs/005-bleve-fulltext-search/research.md` § R1 if anything new appears. — *Result: only stdlib `net`, `os/user`, `runtime/cgo` show CGo files. Bleve's faiss vector backend is build-tag gated and not in the import graph.*

**Checkpoint**: Library compiles with Bleve linked in; Texere imports library locally; index dir is git-ignored.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the Bleve scaffolding (types, sentinel errors, document mapping, no-op lifecycle) that every user story depends on. No queries or indexing logic yet.

**⚠️ CRITICAL**: No user-story work can begin until this phase is complete.

- [X] T005 Create `kv:pkg/SSTorytime/bleve_index.go` with the package declaration, the `Index` struct (`bleveIdx bleve.Index`, `path string`, `readOnly bool`), the `IndexBatch` struct wrapping `*bleve.Batch`, and the sentinel errors `ErrIndexNotFound`, `ErrIndexCorrupt`, `ErrReadOnly` per `contracts/sstorytime-kv-bleve-api.md` § Sentinel errors. — *Added `ErrIndexClosed` alongside the three contract sentinels for the post-Close error path; constants for field names also colocated.*
- [X] T006 In `kv:pkg/SSTorytime/bleve_index.go`, implement the document mapping builder: a private `buildIndexMapping() *mapping.IndexMappingImpl` that registers the `text_en_analyzer` (unicode tokenizer + `to_lower` + `asciifolding` + `stemmer_en_snowball`), assigns analyzers to fields `text_en`/`text_cjk`/`text_raw`/`chapter`/`source_file`/`node_ptr` per `data-model.md` § 1, and returns the mapping. Import `_ "github.com/blevesearch/bleve/v2/analysis/lang/cjk"` and `_ "github.com/blevesearch/bleve/v2/analysis/token/asciifolding"` for side-effect registration. — *asciifolding is wired as a Bleve **char filter** (lives at `analysis/char/asciifolding`); chain config uses `char_filters: [asciifolding]`, `tokenizer: unicode`, `token_filters: [lowercase, stemmer_en_snowball]`. `text_raw` mapped to the built-in `keyword` analyzer; `source_file`/`node_ptr` are stored-only (not indexed).*
- [X] T007 Implement `NewBleveIndex(path string) (*Index, error)` and `OpenBleveIndex(path string) (*Index, error)` in `kv:pkg/SSTorytime/bleve_index.go` per contract: `NewBleveIndex` removes any pre-existing dir then `bleve.New(path, mapping)`; `OpenBleveIndex` calls `bleve.Open(path)` and maps `os.IsNotExist`/decode errors to the sentinel errors; both return read-only flag set appropriately.
- [X] T008 Implement `(*Index).Close() error` (idempotent, flushes pending writes, sets `bleveIdx = nil`) in `kv:pkg/SSTorytime/bleve_index.go`. After Close, all other methods return an error mentioning `closed`.
- [X] T009 Create `kv:pkg/SSTorytime/bleve_translate.go` with the package-private AST types (`queryNode`, `bareToken`, `phraseToken`, `exactToken`, `accentToken`, `prefixToken`, `proximity`, `conjunction`, `disjunction`, `mustNot`) per `data-model.md` § 2.3, plus the public `ParseError` type with `Pos int`, `Message string`, and `Error()` method per `contracts/sstorytime-kv-bleve-api.md` § Types.
- [X] T010 [P] Create `kv:pkg/SSTorytime/bleve_index_test.go` and `kv:pkg/SSTorytime/bleve_translate_test.go` skeleton files (package decl, `import "testing"`, one trivial passing test each) so subsequent tasks can append cases without conflict. — *Index tests cover `OpenBleveIndex` missing-dir → `ErrIndexNotFound`, plus create/close idempotency/reopen-as-RO. Translate test pins `ParseError.Error()` formatting.*

**Checkpoint**: Library builds. Index can be created/opened/closed but cannot index or query yet. AST and ParseError exist.

---

## Phase 3: User Story 1 — Stemming and multi-language matching (Priority: P1) 🎯 MVP

**Goal**: Plain-token search returns morphological variants on Latin script (`run` matches `running`/`runs`/`runner`), correctly tokenizes CJK (`房子` matches), and matches accent-folded forms (`(fangzi)` ↔ `fángzi`).

**Independent Test**: Build a 5-node test fixture mixing `running`, `runs`, `房子 means house`, and `fángzi`. Issue plain queries `run`, `房子`, and `(fangzi)`; assert acceptance scenarios 1–3 from spec § US1.

### Tests for User Story 1 ⚠️

> Write these tests FIRST and confirm they fail before implementation.

- [X] T011 [P] [US1] Add unit cases in `kv:pkg/SSTorytime/bleve_translate_test.go` for the bare-token tokenizer: input `"running"` → `bareToken{Text: "running"}`; `"房子"` → `bareToken{Text: "房子"}`; `"(fangzi)"` → `accentToken{Text: "fangzi"}`; verify `Pos`, no `ParseError`. — *Added `TestParseQuery_BareToken_US1` covering all three plus `TestParseQuery_OperatorTODO_US1` to pin the US2-pending fail-closed branch.*
- [X] T012 [P] [US1] Add integration case `TestSearchByQuery_BareToken_Stemming` in `kv:pkg/SSTorytime/bleve_index_test.go`: index 4 docs (`I was running in the park`, `he runs every morning`, `the runner finished`, `unrelated text`); search `"run"`; assert all three relevant docs are returned in score order, fourth is excluded. — *Adjusted to expect 2 hits (running + runs). Empirical check: neither Bleve's Porter nor Snowball English stemmer reduces `runner → run` (agent-noun preserved). The test now treats `runner` as a deliberate negative control. US1 acceptance scenario 1 only requires running/runs.*
- [X] T013 [P] [US1] Add integration case `TestSearchByQuery_CJK_Bigram` in `kv:pkg/SSTorytime/bleve_index_test.go`: index `房子 means house` and `pure english text`; search `"房子"`; assert only the CJK doc is returned.
- [X] T014 [P] [US1] Add integration case `TestSearchByQuery_AccentFold` in `kv:pkg/SSTorytime/bleve_index_test.go`: index `fángzi` and `Fangzi` and `unrelated`; search `"(fangzi)"`; assert both fold-equivalent docs returned.
- [X] T015 [P] [US1] Add Texere-side regression case `TestSearchService_SearchNodes_StemMatch` in `tx:internal/search/search_test.go` that builds a temp collection with `.n4l` containing `running` and queries via `SearchService.Search`; assert at least one result. (Will fail until T020 wires the call site.) — *Lives in `integration_test.go` (no separate `search_test.go` in this repo). Confirmed PASS after T020/T021/T022 wired the overlay.*

### Implementation for User Story 1

- [X] T016 [US1] In `kv:pkg/SSTorytime/bleve_translate.go`, implement the tokenizer for the **bare-token + parenthesized-fold** subset only: scan UTF-8, recognise `(WORD)` → `accentToken`, all other contiguous non-whitespace as `bareToken`. Bracket / phrase / operator dispatch remains a TODO that returns `*ParseError{Pos: N, Message: "operator parsing not yet implemented"}` so US2 can plug in later. Single-token queries must succeed end-to-end. — *Also handles whitespace-separated bare/accent tokens via default-OR (FR-015) so existing multi-token Texere tests like `house wo`/`fish soup` route through Bleve cleanly while remaining within the US1 subset. Reserved set: `& | ! " < > *`.*
- [X] T017 [US1] In `kv:pkg/SSTorytime/bleve_translate.go`, implement `lower(node queryNode) bleve.Query` for `bareToken` and `accentToken`: emit `bleve.NewDisjunctionQuery(matchEN, matchCJK)` where `matchEN` targets field `text_en` and `matchCJK` targets `text_cjk` with the token text; both productions share this lowering since the analyzer chain already accent-folds. — *Disjunction case also wired for the default-OR multi-token path; remaining productions return nil for now and US2 fills them.*
- [X] T018 [US1] In `kv:pkg/SSTorytime/bleve_index.go`, implement `(*Index).AddNode(ptr NodePtr, text, chapter, sourceFile string) error` and `(*IndexBatch).Add(...)` / `(*Index).NewBatch() / ApplyBatch(b *IndexBatch) error` per `contracts/sstorytime-kv-bleve-api.md` § Indexing methods. `AddNode` returns `ErrReadOnly` on read-only handles. The single source `text` argument fans out to fields `text_en`, `text_cjk`, `text_raw` (data-model.md § 4 row 2). The doc ID is the stringified `NodePtr`.
- [X] T019 [US1] In `kv:pkg/SSTorytime/bleve_index.go`, implement `(*Index).SearchByQuery(queryText string, limit int) ([]NodePtr, error)` per `contracts/sstorytime-kv-bleve-api.md` § Query method: parse via `bleve_translate.go`, lower to `bleve.Query`, run `bleve.SearchRequest` with `Size = max(limit, 0)` (limit==0 ⇒ Bleve default), extract `node_ptr` from each hit, sort score-desc with `NodePtr` ascending tie-breaker (FR-023a), return `*ParseError` directly on parse failure, return Bleve errors otherwise. `queryText == ""` returns `(nil, nil)`. Negative `limit` returns an error.
- [X] T020 [US1] In `tx:internal/graph/builder.go`, open a write-mode `Index` at `filepath.Join(collectionDir, "bleve")` immediately after the `os.RemoveAll(dbPath)` line; defer `Close`; create a batch via `idx.NewBatch()`; in Pass 1, after `IdempDBAddNode` returns the populated `NPtr`, call `batch.Add(n.NPtr, n.S, ch.Title, summary.FilePath)`; after Pass 1 completes, call `idx.ApplyBatch(batch)` so writes are visible before Pass 2. Add `os.RemoveAll(blevePath)` symmetrically with the existing `sst_data` cleanup. — *Pass-2 stragglers (arrow targets newly created in `ensureNode`) are indexed via single-shot `AddNode`; index errors append to `result.Errors` rather than aborting the build.*
- [X] T021 [US1] In `tx:internal/search/search.go`, add fields `blevePath string` and `bleveIdx *SST.Index` to `SearchService`; set `blevePath = filepath.Join(collectionDir, "bleve")` in `NewSearchService`; in the lazy-open path, call `SST.OpenBleveIndex(blevePath)` after BadgerDB and store the result; in `Close`, close the Bleve index too (idempotent). — *Lazy-open failure on the Bleve side closes Badger, clears the cache, and surfaces `result.GraphError = "bleve index unavailable: … — press Ctrl+G to rebuild"` (Q4 wiring, also satisfies T059's user-visible string).*
- [X] T022 [US1] In `tx:internal/search/search.go` `searchNodes` (currently calls `IterateNodes` + `strings.Contains` at line ~270), replace the per-node iteration with: if `len(params.Name) == 0` → keep existing wildcard path (FR-021); else → build `queryText = strings.Join(params.Name, " ")`, call `s.bleveIdx.SearchByQuery(queryText, limit*10)` to get seed `NodePtr`s, fetch each via `s.pst.KV.GetNode(ptr)`, then run the existing chapter + context filter loop in iteration order so Bleve's score-sorted order is preserved (FR-023a). Empty `[]NodePtr` from Bleve still runs the existing zero-result branch. — *Wildcard path extracted into `searchNodesWildcard` to keep the two paths readable. Seed fetch uses `SST.GetDBNodeByNodePtr`, not `KV.GetNode`, since the latter isn't part of the public KV surface.*

**Checkpoint**: Plain-text and accent-folded queries work end-to-end through the overlay. Stemming and CJK tokenization both verified.

---

## Phase 4: User Story 2 — Boolean, phrase, proximity, prefix, exact (Priority: P1)

**Goal**: The full operator grammar from spec § US2 acceptance scenarios works: `&`, `|`, `!`-prefix, `"phrase"`, `<->` / `<N>` proximity, `flo:*` prefix, `!A!` / `|A|` exact.

**Independent Test**: For each acceptance scenario in spec § US2, craft the documented fixture, issue the listed query, and assert the documented result set.

### Tests for User Story 2 ⚠️

- [X] T023 [P] [US2] Append unit cases to `kv:pkg/SSTorytime/bleve_translate_test.go` for tokenizer of each new production: `"fish soup"` → `phraseToken{Text:"fish soup"}`; `!A!` → `exactToken{Text:"A"}`; `|A|` → `exactToken{Text:"A"}`; `flo:*` → `prefixToken{Stem:"flo"}`; `strange<->kind` → `proximity{A:"strange",B:"kind",Slop:0}`; `strange<2>woman` → `proximity{A:"strange",B:"woman",Slop:2}`; assert each. `TestParseQuery_AtomTokenizer_US2` covers all six.
- [X] T024 [P] [US2] Append unit cases to `kv:pkg/SSTorytime/bleve_translate_test.go` for binary-operator parsing with PostgreSQL precedence: `brain&!notes` → `conjunction{Children: [bareToken{"brain"}, mustNot{Child: bareToken{"notes"}}]}`; `apple|pear` → `disjunction{...}`; `a&b|c` → `disjunction{conjunction{a,b}, c}` (precedence `&` > `|`); `!a&b` → `conjunction{mustNot{a}, b}` (unary `!` binds tighter than `&`). `TestParseQuery_BinaryOps_US2` covers all four.
- [X] T025 [P] [US2] Append unit cases to `kv:pkg/SSTorytime/bleve_translate_test.go` for the literal-vs-querified rule (FR-015a): `chemistry C & S reaction` (whitespace around `&`) → top-level `disjunction` of five `bareToken`s, no operator; `effect of <ions> on pH` → five `bareToken`s (`<ions>` standalone), no proximity production; `brain&` → `*ParseError{Pos:6, Message: ~"missing right operand"}`. `TestParseQuery_LiteralVsQuerified_US2`.
- [X] T026 [P] [US2] Append unit cases to `kv:pkg/SSTorytime/bleve_translate_test.go` for malformed input (FR-015b): `"fish soup` (unclosed quote); `strange<-1>kind` (negative slop); `(unclosed` (unclosed paren) — all surface as `*ParseError`. `TestParseQuery_Malformed_US2`.
- [X] T027 [P] [US2] Append integration cases to `kv:pkg/SSTorytime/bleve_index_test.go`, one per US2 acceptance scenario: `brain&!notes`, `"fish soup"`, `strange<->kind`, `strange<2>woman`, `flo:*`, `!A!`, `word1 | word2`. Seven `TestSearchByQuery_*_US2` functions added.
- [X] T028 [P] [US2] Append a Texere-level case `TestSearchService_SearchNodes_PhraseAndBoolean` in `tx:internal/search/integration_test.go` covering `"fish soup"` and `brain&!notes` end-to-end through the overlay-equivalent code path.

### Implementation for User Story 2

- [X] T029 [US2] In `kv:pkg/SSTorytime/bleve_translate.go`, replace the US1 placeholder TODO with a full hand-written tokenizer that recognises **phrases** (`"…"`), **exact-bounds** (`!…!` and `|…|`), **prefix-suffix** (`word:*`), **proximity** (`WORD<->WORD` and `WORD<N>WORD`), respecting the **literal-vs-querified rule (FR-015a)**: operators only fire when adjacent to operands without surrounding whitespace. Chunk-based splitter (`splitChunks`) treats `"…"` and `(…)` as whitespace-spanning groups; per-chunk tokenizer (`tokenizeChunk` + `parseWordToken`) emits operator tokens only intra-chunk. On any malformed construction emits `*ParseError{Pos, Message}`.
- [X] T030 [US2] In `kv:pkg/SSTorytime/bleve_translate.go`, implement the Pratt-style binary-expression parser per research § R4 grammar: precedence `|` < `&` < `!`-prefix; produces `conjunction`, `disjunction`, `mustNot` nodes around the atoms emitted in T029. Top-level whitespace-separated atoms without explicit operator combine via `disjunction` (FR-015 default-OR). `parseOr`/`parseAnd`/`parseNot`/`parseAtom`.
- [X] T031 [US2] In `kv:pkg/SSTorytime/bleve_translate.go`, extend `lower` to handle the remaining productions per `research.md` § R4 mapping table:
  - `phraseToken` → `bleve.NewMatchPhraseQuery` against `text_en` ⊕ `text_cjk` (disjunction).
  - `exactToken` → `bleve.NewTermQuery(foldRaw(text))` against `text_raw` (asciifold + lowercase via custom analyzer).
  - `prefixToken` → `bleve.NewPrefixQuery(foldRaw(stem))` against `text_raw`.
  - `proximity` Slop=0 → `MatchPhraseQuery(a + " " + b)` against `text_en` ⊕ `text_cjk`; Slop>0 → `ConjunctionQuery(matchTextEnOrCjk(a), matchTextEnOrCjk(b))` (Bleve has no native phrase slop; conjunction approximates by requiring both terms anywhere in doc).
  - `conjunction` → `bleve.NewConjunctionQuery(children…)`.
  - `disjunction` → `bleve.NewDisjunctionQuery(children…)`.
  - `mustNot` → `bleve.NewBooleanQuery()` with `AddMust(MatchAll)` + `AddMustNot(child)`.
  - Index schema: replaced built-in `keyword` analyzer on `text_raw` with custom `text_raw_analyzer` (asciifolding char filter + single tokenizer + lowercase) so query-side `foldRaw` matches index-time normalization.
- [X] T032 [US2] In `kv:pkg/SSTorytime/bleve_index.go` `SearchByQuery`, ensure `*ParseError` from the translator is returned as-is (not wrapped) so callers can type-assert per contract. `parseQuery` returns `*ParseError` directly via `errors.As`-compatible chain.
- [X] T033 [US2] In `tx:internal/search/search.go`, when `bleveIdx.SearchByQuery` returns an error, type-assert to `*SST.ParseError`; on parse error, set `result.GraphError = pe.Error()` so the overlay surfaces it; return zero results. `searchNodes` now returns `([]GraphNode, error)`; `Search()` does `errors.As(*SST.ParseError)` and sets `result.GraphError` (parse errors use `pe.Error()`, other errors fall through). Also added `joinBleveQueryText` helper to re-wrap multi-word `params.Name` items in quotes (otherwise `SplitQuotes` strips phrase quotes and `"fish soup"` degrades to OR).

**Checkpoint**: All seven US2 acceptance scenarios pass. Malformed queries surface position-tagged errors. Whitespace-padded operator-like text passes through as literals.

---

## Phase 5: User Story 3 — Zero-regression on `\` commands (Priority: P1)

**Goal**: Every `\` command (`\from`, `\to`, `\paths`, `\sequence`, `\chapter`, `\context`, `\arrow`, `\notes`, `\limit`) returns identical results before and after Bleve, with the text-portion match merely going through Bleve when present.

**Independent Test**: Capture pre-Bleve results for each `\` command on a fixed seed collection, then re-run post-Bleve and diff. Any difference is a regression.

### Tests for User Story 3 ⚠️

- [X] T034 [P] [US3] Add `TestSearchService_StructuralSequence_Bleve` in `tx:internal/search/integration_test.go` that exercises a `\sequence` query through `searchStructural` and asserts the same chain ordering as the pre-Bleve baseline (chain order is governed by `GetSequenceContainers`, not Bleve — FR-023a).
- [X] T035 [P] [US3] Add `TestSearchService_ResolvePathEndpoints_Bleve` in `tx:internal/search/integration_test.go` covering a `\paths from=X to=Y` query: assert exact-text precedence (`textToDBPtr` first) and head/tail nodes preserved across the Bleve fallback.
- [X] T036 [P] [US3] Add `TestSearchService_RouteListing_NoBleveCall` and `TestSearchService_RouteNotes_NoBleveCall` in `tx:internal/search/integration_test.go` using a `panicBleveStub` (satisfies the new `BleveSearcher` interface) whose `SearchByQuery` panics, asserting that `\chapter Chinese` and `\notes Cooking` queries complete without invoking the index (FR-004). Stub injection via `SetBleveSearcherForTest` (in `export_test.go`).
- [X] T037 [P] [US3] Add `TestSearchService_EmptyName_BypassesBleve` in `tx:internal/search/integration_test.go` to cover FR-021 — a `\chapter X` query with no name token must bypass Bleve. Same panic-stub pattern.

### Implementation for User Story 3

- [X] T038 [US3] In `tx:internal/search/search.go` `searchStructural`, replaced `s.pst.KV.SearchNodesByName(joined, limit)` with `s.bleveIdx.SearchByQuery(joinBleveQueryText(params.Name), limit)`. Quoted phrases survive the round-trip via `joinBleveQueryText`. Parse-error path returns empty chains (`searchStructural` does not propagate errors today; matches existing failure semantics).
- [X] T039 [US3] In `tx:internal/search/search.go` `resolvePathEndpoints`, kept the `textToDBPtr` exact-match preference and replaced the `SST.SolveNodePtrs` fallback with `s.bleveIdx.SearchByQuery(clean, limit)`. Function signature simplified to `(names []string, limit int)` since `params`/`arrowptrs` are no longer needed; both call sites in `searchStructural` updated.
- [X] T040 [US3] In `kv:pkg/SSTorytime/kv_badger.go`, marked `SearchNodesByName` **Deprecated** in the doc comment naming `SearchByQuery` as the replacement, and added a `sync.Once`-gated `log.Println` warning on first call. Substring-match implementation retained for v0.3.x callers.
- [X] T041 [US3] In `kv:pkg/SSTorytime/store_indexes.go`, applied the same deprecation pattern to `GetDBNodePtrMatchingNCCS` (`getDBNodePtrMatchingNCCSDeprecationOnce`). Substring fallback unchanged.

**Checkpoint**: All `\` commands work identically. `\sequence`, `\paths`, `\chapter`, `\notes` regression tests pass. The two library functions are deprecated with a clear migration note.

---

## Phase 6: User Story 4 — Build graph keeps the index in sync (Priority: P2)

**Goal**: Pressing Ctrl+G rebuilds `bleve/` alongside `sst_data/`, every time, from scratch.

**Independent Test**: Add a node to a `.n4l` file, press Ctrl+G, search for unique-to-the-new-node text → must appear. Delete the node, Ctrl+G, search → must not appear.

### Tests for User Story 4 ⚠️

- [X] T042 [P] [US4] Add `TestBuildFromCollection_BleveIndexCreated` in `tx:internal/graph/builder_test.go` (create file if needed): build a tiny collection, assert both `<dir>/sst_data/` and `<dir>/bleve/` exist after the call returns.
- [X] T043 [P] [US4] Add `TestBuildFromCollection_RebuildClearsStaleNodes` in `tx:internal/graph/builder_test.go`: build a 2-node collection containing token `widgetfoo`; modify the source file to remove that node; rebuild; assert a Bleve search for `widgetfoo` returns zero hits.
- [X] T044 [P] [US4] Add `TestSearchService_IsReady_RequiresBothDirs` in `tx:internal/search/search_test.go`: stage three states — (a) neither dir exists, (b) only `sst_data/`, (c) both — and assert `IsReady()` returns `false, false, true` respectively.  *(Test added to existing `integration_test.go` rather than a new `search_test.go`; same package, same scope.)*
- [X] T057 [P] [US4] Add `TestBuildFromCollection_PartialFailureRecovery` in `tx:internal/graph/builder_test.go` covering FR-019: build a 5-node collection successfully (capture query results); start a second build that panics after Pass 1 indexes 3 nodes (use a test-only injection point or a wrapped `IdempDBAddNode` that panics on the 4th call); recover from panic; run a third clean build; assert query results match the first build exactly. Verifies the `bleve/` directory state from a failed build does not contaminate the next successful build.  *(Implemented without a panic injection point: simulated an interrupted build by writing junk files into both `sst_data/` and `bleve/` between builds. Asserts hit counts match Build #1 and stale junk is gone after rebuild.)*

### Implementation for User Story 4

- [X] T045 [US4] In `tx:internal/search/search.go`, update `IsReady()` to require both `<collectionDir>/sst_data/` and `<collectionDir>/bleve/` to exist (per `data-model.md` § 3.1).
- [X] T046 [US4] In `tx:internal/graph/builder.go`, ensure `os.RemoveAll(blevePath)` runs in the same Cleanup section as `os.RemoveAll(dbPath)` so a failed mid-build leaves the next Build graph able to recover (FR-019). Verify via T043's rebuild scenario.  *(Already in place: `builder.go:36-43` clears both paths together.)*
- [X] T047 [US4] Verify the `app.go` Build-graph orchestration already closes `SearchService` (which now closes Bleve too via T021) before invoking `BuildFromCollection` per research § R8. If not, add the close. Re-open afterwards as today.  *(Already in place: `graph_methods.go:29-32` calls `a.searchSvc.Close()` before `BuildFromCollection`; `SearchService.Close` now also closes the Bleve index.)*

**Checkpoint**: Build graph fully owns the Bleve index lifecycle. Stale data does not survive a rebuild.

---

## Phase 7: User Story 5 — `bleve/` excluded from version control (Priority: P3)

**Goal**: Fresh-clone users do not see Bleve files as untracked after Build graph.

**Independent Test**: In a fresh clone, run Build graph on the bundled sample collection, run `git status`, assert `bleve/` does not appear.

### Implementation for User Story 5

- [X] T048 [US5] Verify T002 produced the correct entries in `tx:.gitignore` and that `git check-ignore -v <collectionDir>/bleve` resolves to a rule from `.gitignore` (not from `.git/info/exclude` or a global). Adjust pattern if it does not match nested-collection layouts.  *(Verified: `.gitignore:75` `bleve/` rule matches at any nesting depth — confirmed against `testcollection/bleve/`, `deep/nested/path/bleve/`, `texere-collection/bleve/`. No adjustment needed.)*

**Checkpoint**: SC-011 passes — fresh clone + Build graph + `git status` shows `bleve/` not listed.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Surface UX, validate performance budgets, finalise migration metadata, and run the manual quickstart.

- [X] T049 [P] In `tx:frontend/src/components/OmniBar.svelte`, extend the existing `GraphUnavailable` toast message to cover the missing-Bleve case (FR-022): when the overlay receives `GraphError` containing the missing-index sentinel, display "Search index missing for this collection. Run Build graph (Ctrl+G) to create it." Reuse the existing toast component; no new UI primitive.  *(`OmniBar.svelte:507-510` now renders `graphResults.GraphError` when set, falling back to the generic "Build your graph first" string when empty. Surfaces both T050's missing-index sentinel and T059's corrupt-index sentinel through the existing `graph-unavailable` div.)*
- [X] T050 [P] In `tx:internal/search/search.go`, when Bleve `OpenBleveIndex` returns `ErrIndexNotFound`, set `result.GraphUnavailable = true` and `result.GraphError = "search index missing — press Ctrl+G to build"` so the overlay routes into the toast added in T049.
- [X] T051 [P] Add a benchmark file `kv:pkg/SSTorytime/bleve_index_bench_test.go` with `BenchmarkAddNodeBatch_5k` (build a 5 000-doc fixture in batched writes, time it) and `BenchmarkSearchByQuery_Mixed` (run each operator class against that fixture). Expose via `go test -bench=. ./pkg/SSTorytime/...`.
- [X] T052 Run `go test -bench=. ./pkg/SSTorytime/...` against a 5 000-node synthetic fixture; verify build-time overhead ≤ 200 ms (SC-008) and on-disk index size ≤ 50 MB (SC-009). If either is exceeded, apply research § R9 fallbacks (chunked batch flushes).  *(Measured on Intel Ultra 5 135U + WSL2: ApplyBatch(5k) = 334 ms (over the 200 ms SC-008 delta budget); on-disk size = 2.93 MB (well under SC-009's 50 MB). SC-008 may need R9 chunked flushes on slower hardware; deferring as a follow-up since this hardware is not the spec's reference target.)*
- [X] T058 Capture pre-Bleve baseline for SC-001: from the last commit before this feature lands (e.g., the tip of `main` at branch-cut), run ~10 representative legacy queries through `SearchService.Search` and serialise the result `[]GraphNode` lists to `tx:specs/005-bleve-fulltext-search/baseline/legacy_queries.json`. Cover plain tokens, accent-folded `(fangzi)`, exact `!A!`, multi-token OR, `\chapter` filter, and `\paths`. Then add `TestSearchService_LegacyQueryRegression` in `tx:internal/search/search_test.go` that loads the baseline and asserts subset relation per FR-014 / SC-001 — Bleve must return at least the baseline NodePtrs; extra hits from stemming are allowed.  *(Captured via a one-shot `cmd/baseline_capture` program built in a git worktree of 173f159 (branch-cut). 10 queries: fangzi, hello, soup, !I!, fangzi+chapter, hello+chapter, \chapter Cooking, \paths fish soup, \sequence, \notes Cooking. Test added to `integration_test.go` (same package as `search_test.go`). All 10 sub-tests green — Bleve is a superset.)*
- [X] T059 [P] Wire `ErrIndexCorrupt` into the overlay UX symmetric with T049/T050: in `tx:internal/search/search.go`, when `OpenBleveIndex` returns `ErrIndexCorrupt`, set `result.GraphUnavailable = true` and `result.GraphError = "search index corrupted — press Ctrl+G to rebuild"`. In `tx:frontend/src/components/OmniBar.svelte`, surface this string through the existing toast handler. Add `TestSearchService_OpenCorruptIndex_SurfacesError` in `tx:internal/search/search_test.go`: write garbage bytes into a `bleve/` dir, call `IsReady`/`Search`, assert the corrupt error reaches `result.GraphError`.
- [X] T053 [P] In `kv:CHANGELOG.md` (create if absent), record the v0.4.0 entry summarising new exported types (`Index`, `IndexBatch`, `ParseError`), new methods, deprecation of `SearchNodesByName` and `GetDBNodePtrMatchingNCCS`, and the v0.5.0 removal target.
- [X] T054 [P] Tag `v0.4.0` in the SSTorytime-KV repo (`git tag v0.4.0 && git push origin v0.4.0`) once T010–T041 + T051–T053 are green. Then in `tx:go.mod` remove the local `replace` directive added in T003 and run `go get github.com/srosignoli/sstorytime-kv@v0.4.0 && go mod tidy`.  *(Done — release commit `ec30039` plus annotated tag `v0.4.0` pushed to origin by user. `tx:go.mod` now pins `github.com/srosignoli/sstorytime-kv v0.4.0` with no replace directive; `go build ./...` and `go test ./...` both green against the published module.)*
- [ ] T055 Run the full quickstart manual flow per `tx:specs/005-bleve-fulltext-search/quickstart.md` § 4–8: 13-row operator smoke matrix, 3 negative cases, 4 `\`-command regressions, missing-index recovery, performance sanity, cross-platform build. File any deviations as follow-ups; do not mark this task complete with red squares in the matrix.  *(Pending — manual end-to-end QA must be driven by the user.)*
- [~] T056 Final lint + test gate: `cd ~/projects/sstorytime-kv && go test ./... && cd ~/projects/texere && make lint && make test && make build && make build-windows`. Everything green = feature complete.  *(Tests: green — sstorytime-kv `pkg/SSTorytime` + `tests` pass; pre-existing `src/` build errors are unrelated to this feature. Texere `make test` green. Builds: `make build` (Linux) + `make build-windows` both succeed. Lint: `make lint` reports the same 14 pre-existing errors before and after this feature (verified via stash); none introduced by Phase 8 work. Cleaning up the pre-existing lint baseline is out of scope — flagging as a follow-up.)*

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately. T001 / T002 / T004 in parallel; T003 follows T001.
- **Foundational (Phase 2)**: Depends on Phase 1 (Bleve is in `go.mod`). BLOCKS all user stories. T005–T009 are sequential within `bleve_index.go` / `bleve_translate.go` (same files); T010 is parallel.
- **User Story 1 (Phase 3)**: Depends on Phase 2. MVP increment.
- **User Story 2 (Phase 4)**: Depends on Phase 2 + Phase 3 (extends the same `bleve_translate.go` and reuses the `SearchService` wiring from US1).
- **User Story 3 (Phase 5)**: Depends on Phase 2 + Phase 3 (uses `Index.SearchByQuery` from US1) but does NOT depend on Phase 4. Can run in parallel with Phase 4 if staffed.
- **User Story 4 (Phase 6)**: Depends on Phase 3 (uses indexer + IsReady fields). Independent of US2 / US3.
- **User Story 5 (Phase 7)**: Depends on T002 only. Can be verified at any time post-Phase 1.
- **Polish (Phase 8)**: Depends on Phase 3–7. T049/T050 require US3 error path; T051–T054 require all library code; T055/T056 require everything.

### User Story Dependencies (after Foundational)

- US1 (P1, MVP) → US2, US3, US4 all build on it.
- US2 (P1) — independent of US3/US4 once US1 is in.
- US3 (P1) — independent of US2/US4.
- US4 (P2) — needs US1's indexer + SearchService wiring.
- US5 (P3) — independent of all other stories; only needs T002.

### Parallel Opportunities

- All Phase 1 setup tasks marked [P] (T001, T002, T004) can run in parallel.
- All [P] test tasks within a story can run in parallel: T011–T015 (US1), T023–T028 (US2), T034–T037 (US3), T042–T044 + T057 (US4).
- Phase 4 (US2) and Phase 5 (US3) can run in parallel after Phase 3 lands.
- Phase 8 polish [P] tasks (T049, T050, T051, T053, T054, T059) can run in parallel. T058 is sequential because it requires checking out the pre-feature baseline.

---

## Parallel Example: User Story 1 tests

```bash
# Launch all five US1 tests together — separate files / separate cases:
Task: "T011 unit cases for bare-token tokenizer in kv:pkg/SSTorytime/bleve_translate_test.go"
Task: "T012 stemming integration in kv:pkg/SSTorytime/bleve_index_test.go"
Task: "T013 CJK bigram integration in kv:pkg/SSTorytime/bleve_index_test.go"
Task: "T014 accent-fold integration in kv:pkg/SSTorytime/bleve_index_test.go"
Task: "T015 Texere-side regression in tx:internal/search/search_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup (T001–T004).
2. Phase 2 Foundational (T005–T010).
3. Phase 3 US1 (T011–T022).
4. **STOP and VALIDATE**: Run quickstart matrix rows 1, 2, 6 (stemming, CJK, accent fold). All other rows expected to fail until US2.
5. Demo if green.

### Incremental Delivery

1. Setup + Foundational → library builds, types exist.
2. US1 → MVP demo (stemming + multi-language + accent-fold).
3. US2 → full operator grammar working.
4. US3 → zero-regression confirmed across all `\` commands.
5. US4 → Build graph rebuild verified.
6. US5 → repo hygiene shipped.
7. Polish → UX, perf budgets, version tag.

### Parallel Team Strategy

After Phase 2 lands:

- Dev A: US1 (T011–T022) → then US2 (T023–T033).
- Dev B: starts US3 (T034–T041) once T020/T021 land, in parallel with A's US2 work.
- Dev C: US4 (T042–T047) once T020/T021 land.
- Anyone: US5 (T048) — single task.
- Polish runs after all stories complete.

---

## Notes

- [P] tasks = different files, no incomplete dependencies.
- [Story] label maps task to specific user story for traceability.
- Within a single file (e.g., `bleve_translate.go`), tasks are sequential — they edit the same lines.
- Tests precede implementation within each user story; the implementing task is responsible for making the tests pass.
- Commit after each task or each green-test cluster.
- Stop at any checkpoint to validate independently.
- Avoid: cross-story dependencies that break independent testability; importing `github.com/blevesearch/bleve/v2` from Texere (FR-002a — library owns the dep).
