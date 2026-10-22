# Phase 0 Research: Replace text matching with Bleve full-text search engine

**Feature**: 005-bleve-fulltext-search
**Date**: 2026-04-25
**Status**: Complete — all NEEDS CLARIFICATION items resolved via the Q1–Q5 clarification session and the items below.

This document records the design-relevant decisions that drive Phase 1 (data model, contracts, quickstart) and Phase 2 (tasks). Five of the load-bearing decisions were already pinned in `spec.md § Clarifications` (Session 2026-04-25). The remainder concern dependency audit, library-vs-consumer API shape, build sequencing, and migration mechanics.

---

## R1. Bleve dependency audit (pure-Go check)

**Decision**: Adopt `github.com/blevesearch/bleve/v2` as a direct dependency of SSTorytime-KV. Pull in the CJK and ASCII-folding analyzer sub-packages explicitly so they participate in build/lint analysis.

**Rationale**: Constitution principle II requires pure Go. Bleve v2 and the analyzer sub-packages are pure Go (verified by inspecting `go.mod` of `blevesearch/bleve/v2` and the existing pure-Go projects that depend on it). No CGo. No external runtime services. License is BSD-3-Clause — compatible with the existing module licensing.

**Modules introduced**:

| Module | Why | Pure Go? |
|---|---|---|
| `github.com/blevesearch/bleve/v2` | Core index + query primitives | ✅ |
| `github.com/blevesearch/bleve/v2/analysis/lang/cjk` | CJK bigram analyzer (`text_cjk` field) | ✅ |
| `github.com/blevesearch/bleve/v2/analysis/token/asciifolding` | ASCII-fold filter for `text_en` accent matching | ✅ |
| `github.com/blevesearch/bleve_index_api` | Pulled in transitively by Bleve v2 | ✅ |
| `github.com/blevesearch/scorch_segment_api/v2` | Default Scorch index segment format | ✅ |

**Alternatives considered**:

- **Roll our own inverted index in `kv_badger.go`** — would avoid a dependency, but reimplementing tokenization, stemming, ASCII folding, CJK bigram, scoring, and operator parsing would dwarf the feature. Rejected.
- **Bluge** (Bleve fork by the original author) — newer, leaner API, but fewer pre-built analyzers and a smaller ecosystem. The CJK analyzer support is less mature. Rejected for v1; revisit if Bleve becomes a maintenance burden.
- **`zoekt`** (Sourcegraph) — code-search oriented, not general full-text. Rejected.

---

## R2. Index storage format

**Decision**: Use Bleve's default Scorch segment format. Place the index at `<collectionDir>/bleve/`. Open in read-only mode for queries; open read-write only inside `BuildFromCollection`.

**Rationale**: Scorch is Bleve's recommended default, supports concurrent readers, and writes with a single writer. It matches BadgerDB's single-writer/multi-reader model already in use, so the existing "close search → rebuild → reopen" sequence in Texere covers both stores without changes (clarification Q1 + assumption "Single-writer contract preserved").

**Alternatives considered**:

- **`upsidedown` (legacy)** — Bleve's older format, deprecated for new use. Rejected.
- **In-memory index rebuilt at app start** — would avoid disk artefacts but turn every app start into an O(N) scan. With 14 000-node collections that's hundreds of milliseconds of latency on every launch. Rejected.

---

## R3. Library API shape (SSTorytime-KV side)

**Decision**: Add a single new exported type and four methods to package `SSTorytime`:

```go
type Index struct { /* opaque */ }

// OpenBleveIndex opens an existing Bleve index for reading. Returns an error if
// the directory does not exist or the index is corrupted.
func OpenBleveIndex(path string) (*Index, error)

// NewBleveIndex creates a fresh Bleve index at path, removing any pre-existing
// directory at that location. Used during Build graph.
func NewBleveIndex(path string) (*Index, error)

// AddNode indexes a single node. Called once per node during Build graph.
// Idempotent — re-adding a node with the same NodePtr replaces the prior doc.
func (i *Index) AddNode(ptr NodePtr, text, chapter, sourceFile string) error

// Close flushes and closes the index. Safe to call multiple times.
func (i *Index) Close() error

// SearchByQuery parses queryText into a Bleve query and returns matching
// NodePtrs ordered by relevance (NodePtr ascending as the deterministic
// tie-breaker). limit caps results; 0 means no cap.
func (i *Index) SearchByQuery(queryText string, limit int) ([]NodePtr, error)
```

Plus one re-wiring decision: `SearchNodesByName(textName, limit)` (existing, called by `SolveNodePtrs` and `searchStructural`) takes a new optional first argument or grows a sibling `SearchNodesByQuery(idx *Index, query, limit)`. We choose the **sibling** approach to keep the existing method signature intact for any other downstream consumers and to make the migration in Texere a search-and-replace at the call sites.

**Rationale**: Minimal surface. The translator (R4) is internal — callers pass query strings, not pre-parsed Bleve queries. This keeps the operator grammar an SSTorytime-KV concern (clarification Q1) and prevents Texere from importing Bleve types directly (FR-002a).

**Alternatives considered**:

- **Expose Bleve `Query` types in the public API** — would let callers compose programmatic queries. Rejected: leaks the implementation, makes the API harder to evolve, contradicts FR-002a.
- **Replace `SearchNodesByName` in place** — would force every existing caller (SSTorytime, downstream users if any) to provide an `Index`. Migration risk too high for v1.

---

## R4. Operator-grammar translator design

**Decision**: Hand-written recursive-descent parser in `bleve_translate.go`. Tokenizer first (recognises the literal-vs-querified distinction from FR-015a), then a Pratt-style expression parser for the binary operators with PostgreSQL `ts_query` precedence (`|` < `&` < `!`-prefix).

**Rationale**:

- The grammar is small (≤10 productions) and stable. A hand-written parser is roughly 200 LOC, beats pulling in `participle` or `goyacc`, and gives precise position info for parse errors (FR-015b).
- The literal-vs-querified rule (FR-015a) requires lookahead at character level — easier in a hand-written tokenizer than in a regex-based one.
- The translator output is a Bleve query AST node directly, not an intermediate IR. No need for a third layer.

**Grammar (informal EBNF)**:

```
query        = expr
expr         = term ( "|" term )*           # disjunction
term         = factor ( "&" factor )*       # conjunction
factor       = "!" factor | atom
atom         = phrase | exact | accent_fold | prefix_atom | proximity | bare_token
phrase       = '"' WORD ( SPACE WORD )* '"'
exact        = "!" WORD "!" | "|" WORD "|"
accent_fold  = "(" WORD ( SPACE WORD )* ")"
prefix_atom  = WORD ":" "*"
proximity    = WORD ( "<->" | "<" DIGIT+ ">" ) WORD
bare_token   = WORD
WORD         = (any non-operator char run)
```

The translator emits Bleve primitives:

| Production | Bleve query |
|---|---|
| `bare_token` | `bleve.NewMatchQuery(text)` against `text_en` ⊕ `text_cjk` (disjunction) |
| `phrase` | `bleve.NewMatchPhraseQuery(text)` against `text_en` ⊕ `text_cjk` |
| `exact` | `bleve.NewTermQuery(strings.ToLower(text))` against `text_raw` |
| `accent_fold` | Same as `bare_token` (analyzer chain already accent-folds via ASCII-folding filter) |
| `prefix_atom` | `bleve.NewPrefixQuery(strings.ToLower(stem))` against `text_raw` |
| `proximity` | `bleve.NewMatchPhraseQuery("a b")` with `.Slop = N` against `text_en` |
| `&` (conjunction) | `bleve.NewConjunctionQuery(...)` |
| `\|` (disjunction) | `bleve.NewDisjunctionQuery(...)` |
| `!` (must-not) | wrapped in `bleve.NewBooleanQuery(must=nil, should=nil, mustNot=[child])` |

**Alternatives considered**:

- **Parse with `regexp`** — fast to write, fragile on edge cases (e.g., quote inside parentheses). Rejected.
- **Use Bleve's built-in `query_string` parser** — its grammar is similar but not identical to the user's PostgreSQL-style grammar. `<N>` proximity, `!…!`/`|…|` exact bounds, and `(accent)` parens are not native to Bleve's parser. Adapting it would be brittle. Rejected.

---

## R5. Indexing strategy in Build graph

**Decision**: Index each node in `BuildFromCollection`'s **Pass 1** (the node-creation pass), immediately after `IdempDBAddNode` returns the populated `NPtr`. Index is opened once at the start of the build, batched writes, closed at the end.

**Rationale**:

- Pass 2 (link creation) doesn't add new nodes, so indexing in Pass 1 covers everything.
- Bleve's batch API amortises per-write overhead. A single `index.Batch()` flushed once at end of Pass 1 keeps the SC-008 200 ms overhead realistic.
- Closing in `defer` mirrors the existing BadgerDB lifecycle in `builder.go`.

**Sequencing inside `BuildFromCollection`**:

```go
// Inside BuildFromCollection (sketch, not final code)
os.RemoveAll(dbPath)
os.RemoveAll(blevePath)              // NEW

bleveIdx, _ := SST.NewBleveIndex(blevePath)   // NEW
defer bleveIdx.Close()                          // NEW
batch := bleveIdx.NewBatch()                    // NEW

// ... existing kv = OpenBadgerStore(...)

for each chapter, each node {
    n := SST.IdempDBAddNode(sst, n)
    SST.CacheNode(n)
    batch.Add(n.NPtr, n.S, ch.Title, summary.FilePath)   // NEW
    // ... existing
}

bleveIdx.ApplyBatch(batch)                       // NEW (commits before Pass 2)
// ... Pass 2 link creation continues unchanged
```

**Alternatives considered**:

- **Re-iterate BadgerDB after Pass 2 to build the Bleve index** — simpler decoupling, but doubles I/O on the hot path. Rejected.
- **Build the Bleve index lazily on first search** — lazy work shifts latency to the user's first query and complicates the `IsReady` check. Rejected.

---

## R6. Search wiring in Texere

**Decision**: `SearchService` opens the Bleve index lazily alongside BadgerDB during the existing `IsReady` / lazy-open flow. Both must be present for `IsReady` to return true; if BadgerDB is present but Bleve is not, `IsReady` returns false and the overlay shows the Q4-resolved error message.

**Changes to `internal/search/search.go`**:

1. Add `bleveIdx *SST.Index` field to `SearchService`.
2. `IsReady()` checks both `sst_data/` and `bleve/` directories.
3. Lazy open opens the Bleve index after BadgerDB.
4. `searchNodes`:
   - If `params.Name` is empty (wildcard) — keep today's `IterateNodes` path (FR-021).
   - Otherwise — call `bleveIdx.SearchByQuery(joinName(params.Name), maxlimit*10)` to get seeds; fetch each seed's `Node` from BadgerDB; apply existing chapter + context filters; build orbit; cap at `limit`. Order is preserved from Bleve (FR-023a).
5. `searchStructural` sequence path: replace `s.pst.KV.SearchNodesByName(joined, limit)` with `bleveIdx.SearchByQuery(joined, limit)` (`searchStructural` then applies the existing sequence-marker filter).
6. `resolvePathEndpoints`: replace the `SST.SolveNodePtrs(...)` fallback with a Bleve seed lookup that's then filtered through the existing `textToDBPtr` exact-match preference.

**Result-ordering wire** (FR-023a): the `[]NodePtr` returned by `bleveIdx.SearchByQuery` is already in score order with `NodePtr` tie-breaker; downstream code preserves this order through the chapter/context filter loops by appending to a slice in iteration order rather than rebucketing by NodePtr.

**Alternatives considered**:

- **Open the Bleve index globally in `app.go`** — would parallel the BadgerDB lifecycle exactly but require a new `App` field. Marginally cleaner. Picked the `SearchService` approach because it co-locates the two indexes that always travel together.

---

## R7. Pre-Bleve collection migration

**Decision**: No automatic migration. Per clarification Q4, when a user opens a collection with `sst_data/` but no `bleve/`, `IsReady()` returns false and the overlay shows the actionable "Press Ctrl+G" message. After the user runs Build graph, both stores are consistent.

**Rationale**: The simplest correct behaviour. Auto-rebuild-on-first-query was rejected in Q4 because it would surprise users with a Build graph operation they didn't initiate.

**Code touch**: `OmniBar.svelte` already shows a "Build your graph first (Ctrl+G)" message when `GraphUnavailable` is true (feature 004). The same flag covers this case — no new UI primitive is needed; only the text might be tweaked to mention "search index" alongside "graph".

**Alternatives considered**: B (auto-rebuild) and C (substring fallback) — both rejected in Q4.

---

## R8. Concurrent search-while-indexing safety

**Decision**: Texere already enforces the "close search before Build graph" invariant for BadgerDB (`builder.go:24` comment: "The caller must close any open SearchService before calling this"). Reuse that exact invariant for Bleve. `BuildFromCollection` will continue to assume no SearchService is open; the only change is that the close-then-reopen cycle now owns one extra index handle.

**Rationale**: No behavioural change; the existing invariant covers Bleve as well. The `app.go` sequence around Build graph already calls `searchSvc.Close()` before invoking the builder.

**Alternatives considered**:

- **Per-store mutexes inside SearchService** — overkill for a single-user desktop app. Rejected.

---

## R9. Index size and build-time budget validation

**Decision**: SC-008 (≤ 200 ms build overhead) and SC-009 (≤ 50 MB index at 5 000 nodes) will be validated by a Phase 2 task that runs `make build` + a benchmark on a representative collection (the user's own collection if available, otherwise a synthetic 5k-node fixture). If budgets are exceeded, the fallback options are: (a) reduce the number of indexed fields (drop `text_cjk` for collections with no CJK content — but this requires content sniffing), or (b) flush the batch in chunks rather than at end-of-pass.

**Rationale**: Bleve's published benchmarks suggest ≤ 50 MB for 5 000 short documents is comfortable; the 200 ms overhead is plausible but unverified for this specific schema. Better to measure than guess.

**Alternatives considered**:

- **Skip the index size budget** — would leave SC-009 unverifiable. Rejected.
- **Set a tighter budget** — without measurement, a tighter budget would be arbitrary. Rejected.

---

## R10. Test strategy

**Decision**:

- **Unit tests** for the operator-grammar translator (`bleve_translate_test.go` in SSTorytime-KV): table-driven, one row per production. Covers happy path + every parse-error case enumerated in FR-015b.
- **Integration tests** for index lifecycle (`bleve_index_test.go` in SSTorytime-KV): build a 50-doc fixture, query each operator class, assert results.
- **Regression tests** in Texere (`internal/search/unit_test.go`, `internal/search/integration_test.go`): adapt existing tests so they assume an Index is present; add new cases covering each acceptance scenario in spec User Stories 1–3.
- **Performance**: benchmark in SSTorytime-KV (`bleve_index_bench_test.go`) for build-time and query-latency. Surfaced as `make bench` output, not a CI gate (yet).

**Rationale**: Existing repo convention (per CLAUDE.md "Testing — All public APIs and scheduling algorithms MUST have unit tests"). Integration coverage closes the gap between "translator produces correct AST" and "the AST returns the right NodePtrs".

**Alternatives considered**:

- **Property-based testing** for the parser — tempting but overkill for a 200-LOC grammar with hand-enumerated edge cases. Defer.

---

## Outstanding items deferred to Phase 2 (`/speckit.tasks`)

- Exact wording of the overlay "search index missing" message (Texere UI string).
- Choice of representative collection for SC-008 / SC-009 measurement.
- Whether to publish the SSTorytime-KV minor version bump as `v0.4.0` (additive) or `v0.4.0` with a deprecation notice on `SearchNodesByName`.
- Optional: `Index.Stats()` API for surfacing index size in a future "About this collection" panel.

None of the above blocks Phase 1 design. All are recorded in the deferred-items section of `tasks.md` once `/speckit.tasks` runs.
