# Implementation Plan: Replace text matching with Bleve full-text search engine

**Branch**: `005-bleve-fulltext-search` | **Date**: 2026-04-25 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/005-bleve-fulltext-search/spec.md`

## Summary

Add a Bleve-backed full-text matcher to SSTorytime-KV (library-owned per clarification Q1) and re-wire Texere's two text-match sites (`searchNodes` and the `SearchNodesByName`-driven path/sequence resolution) to consume it. The library grows an `Index` type that owns Bleve lifecycle (open/rebuild/close), a `TranslateQuery(string) (Query, error)` parser that maps the user-facing operator grammar (`!exact!`, `|exact|`, `(accents)`, `&`, `|`, `!`, `"…"`, `<->`, `<N>`, `:*`) to Bleve primitives, and a `SearchByQuery(query) []NodePtr` API. Indexing happens during `BuildFromCollection` alongside BadgerDB writes; the index lives in `<collectionDir>/bleve/`. Texere's `searchNodes` switches from `IterateNodes`+`strings.Contains` to a Bleve seed lookup followed by the existing chapter/context filters and orbit expansion.

## Technical Context

**Language/Version**: Go 1.22 (backend only — this feature ships no frontend changes beyond an error-toast string update)
**Primary Dependencies**:
- `github.com/blevesearch/bleve/v2` (new — pure Go, BSD-3-Clause)
- `github.com/blevesearch/bleve/v2/analysis/lang/cjk` (new — pure Go, included with Bleve)
- `github.com/blevesearch/bleve/v2/analysis/token/asciifolding` (new — pure Go, included with Bleve)
- Existing: SSTorytime-KV (will get a minor version bump to expose Bleve API), Wails v2.12, BadgerDB (via SSTorytime-KV)

**Storage**:
- Existing: BadgerDB at `<collectionDir>/sst_data/` (unchanged contract)
- New: Bleve index at `<collectionDir>/bleve/` (created/reset during Build graph, opened read-only during search)

**Testing**: `go test ./...` (unit tests for translator, integration tests for index build + query roundtrip); `make build` and `make build-windows` for cross-compile; `make dev` for manual UI smoke

**Target Platform**: Linux amd64 + Windows amd64 (cross-compiled via existing CI)
**Project Type**: Desktop app (Wails v2, Go + OS webview) — same project type as 004
**Performance Goals**:
- Build graph total time on a 5 000-node collection: ≤ 200 ms over the pre-Bleve baseline (SC-008)
- Bleve query for `routeNodeSearch` p95 < 50 ms on a 5 000-node index (no constitution budget for this; chosen to stay well under the 100 ms Tier 2 search target inherited from feature 004)
- Build graph end-to-end stays inside the constitution's 100–500 ms budget for collections up to ~10 000 nodes

**Constraints**:
- Pure Go only (no CGo) — Bleve and CJK analyzer satisfy this
- No network calls, no external services
- Single binary deployment unchanged
- Index size ≤ 50 MB at 5 000 nodes (SC-009)

**Scale/Scope**: Personal knowledge collections, typically 100–14 000 nodes (existing `internal/search/search.go` comment notes ~14k nodes in practice). Bleve scales well past this; no horizontal-scale work required.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Source-First & Full N4L Fidelity | ✅ PASS | Bleve index is derived from `.n4l` files at Build graph time. No `.n4l` files are written or modified. Index loss is recoverable by Ctrl+G. |
| II. Self-Contained Binary | ✅ PASS | Bleve and its CJK + asciifolding sub-packages are pure Go (verified at Phase 0). No new external runtime dependencies, no DB server, no network. Cross-compiles without extra tooling. |
| III. Separation of Concerns | ✅ PASS | The Bleve index is a third *derived* artefact alongside BadgerDB, sitting in the Knowledge Graph layer. It is not canonical; it is rebuildable from the Content layer. No knowledge content is stored only in Bleve. |
| IV. Pleasure-Driven Learning | N/A | Search/discovery feature; no study scheduling logic affected. Indirectly improves learning by making notes findable. |
| V. Transparency & Portability | ✅ PASS | The `bleve/` directory is plain files inside the user's collection folder. No silent state changes; explicit Build graph is the only writer. Index is git-ignored (FR-024) so users can still zip/move/sync collections without bloat. |
| Performance — Build graph 100–500 ms | ⚠️ WATCH | Bleve indexing adds work during the same Build graph pass. SC-008 caps the overhead at 200 ms; the constitutional budget allows up to 500 ms total. For large collections (>10 000 nodes) we'll need to verify; documented as a Phase 0 research item. |
| Performance — Orbit query <1 ms | ✅ PASS | Bleve only replaces text matching, not orbit traversal. Orbit walks the in-memory NODE_DIRECTORY exactly as today. |
| Dependency Discipline (pure Go) | ✅ PASS | `github.com/blevesearch/bleve/v2` and its analyzer sub-packages are pure Go. Verified in research.md. |

**Post-design re-check**: No new violations introduced after Phase 1 design. The library API surface is small (one new exported type + four methods); the consumer change is local to two functions. See "Post-Design Constitution Re-check" at the bottom of this file.

## Project Structure

### Documentation (this feature)

```text
specs/005-bleve-fulltext-search/
├── plan.md                       # This file
├── research.md                   # Phase 0 — Bleve API choices, analyzer config, ordering, error model
├── data-model.md                 # Phase 1 — index document schema, Go types, query AST
├── quickstart.md                 # Phase 1 — dev setup, manual verification steps
├── contracts/
│   └── sstorytime-kv-bleve-api.md  # Phase 1 — new public Go API surface in SSTorytime-KV
├── checklists/
│   └── requirements.md           # From /speckit.specify
└── tasks.md                      # Phase 2 output (/speckit.tasks — NOT created here)
```

### Source Code

This feature touches two repositories: SSTorytime-KV (library, owns Bleve) and Texere (consumer, calls library API).

```text
# ── SSTorytime-KV (github.com/srosignoli/sstorytime-kv) ─────────────────────────
pkg/SSTorytime/
├── bleve_index.go                # NEW — Index type: Open, Build, Close, Search
├── bleve_translate.go            # NEW — TranslateQuery(string) → Bleve query AST
├── bleve_translate_test.go       # NEW — translator unit tests (operator grammar)
├── bleve_index_test.go           # NEW — round-trip index/search tests
├── kv_badger.go                  # MODIFY — SearchNodesByName delegates to Index
├── store_paths.go                # MODIFY — SolveNodePtrs takes optional Index
└── store_indexes.go              # MODIFY — GetDBNodePtrMatchingNCCS delegates to Index

# ── Texere (github.com/srosignoli/texere — this repo) ───────────────────────────
internal/search/
├── search.go                     # MODIFY — searchNodes uses Index seed query;
│                                 #          searchStructural sequence path uses Index;
│                                 #          IsReady checks both sst_data/ and bleve/
├── search_test.go                # MODIFY — add Bleve-path coverage
└── unit_test.go                  # MODIFY — add operator-grammar regressions

internal/graph/
└── builder.go                    # MODIFY — open Index alongside BadgerDB; index each
                                  #          node in Pass 1; close Index after build.
                                  #          Reset bleve/ dir in Cleanup section.

frontend/src/components/
└── OmniBar.svelte                # MODIFY — extend "graph not built" message to also
                                  #          cover "search index missing" (FR-022).

.gitignore                        # MODIFY — add `bleve/` and `*.bleve/` patterns.

specs/005-bleve-fulltext-search/  # This feature's spec dir (already created)
```

**Structure Decision**: Library-owned Bleve integration per clarification Q1. The library grows two new files (`bleve_index.go` for lifecycle + query API, `bleve_translate.go` for the operator-grammar parser) plus tests. The library's existing `SearchNodesByName` and `GetDBNodePtrMatchingNCCS` become thin wrappers that delegate to the new `Index.Search` when a Bleve index is available, falling back to the existing substring matcher only as a transition path during the migration window — but **at v1.0 of this feature**, callers must always pass an Index, and the substring matcher is removed (clarification Q4: no fallback). Texere's changes are local to two files in `internal/search/` and one in `internal/graph/`.

The CodeMirror search side of Texere (`OmniBar.svelte`) gets a one-line message update, no structural change. No new packages, no new Wails bindings.

## Complexity Tracking

No constitution violations after Phase 1 design. No complexity tracking entry required.

The "WATCH" item under Performance — Build graph budget is monitored (not violated) and surfaced as the v1 measurement task in Phase 2 (`/speckit.tasks`).

## Post-Design Constitution Re-check

After completing data-model, contracts, and quickstart:

- **Source-First** holds: index is rebuildable; no canonical data lives in Bleve.
- **Self-Contained Binary** holds: dependency audit in `research.md` confirms pure Go for `bleve/v2` plus the two analyzer sub-packages.
- **Separation of Concerns** holds: the new `Index` type sits in the Knowledge Graph layer, parallel to BadgerDB. No content or learning-state coupling.
- **Transparency & Portability** holds: `bleve/` is plain files; git-ignored; user can zip/move the collection. Empty / missing index produces an explicit user-visible message (clarification Q4).
- **Performance** budgets unchanged on hot paths (orbit query, parse). Build graph overhead is bounded by SC-008.

No new violations.
