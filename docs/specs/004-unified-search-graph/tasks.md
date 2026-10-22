# Tasks: Unified Search Bar with SSTorytime-KV Graph Integration

**Input**: Design documents from `/docs/specs/004-unified-search-graph/`  
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/search-binding.md ✅, quickstart.md ✅

**Tests**: Not requested in spec — no test tasks generated.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add SSTorytime-KV dependency and create the internal/search package skeleton.

- [X] T001 Add `github.com/srosignoli/sstorytime-kv` dependency by running `go get github.com/srosignoli/sstorytime-kv` from the repo root; verify the import appears in `go.mod` and `go.sum`
- [X] T002 Create directory `internal/search/` (run `mkdir -p internal/search`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Go types, SearchService skeleton, App wiring, and Wails binding stub. ALL must be complete before any user story can be implemented.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T003 Create `internal/search/types.go` with all result types as specified in `docs/specs/004-unified-search-graph/data-model.md`: `SearchResult`, `GraphNode`, `OrbitEntry`, `StructuralChain`, `ChainNode`, `ListingItem`; ensure all slice fields are initialized as non-nil empty slices (`[]GraphNode{}` etc.) when zero-valued, enforced via a `NewSearchResult()` constructor
- [X] T004 Create `internal/search/search.go` with `SearchService` struct (`dbPath string`, `store *badger.BadgerKV`, `pst sst.PoSST`, `mu sync.Mutex`); implement `NewSearchService(collectionDir string) *SearchService` (sets `dbPath = filepath.Join(collectionDir, "sst_data")`), `IsReady() bool` (calls `os.Stat(s.dbPath)`), `Close()` (calls `sst.Close(s.pst)` if store is non-nil), and a stub `Search(query string) (*SearchResult, error)` that returns `NewSearchResult()` with `GraphUnavailable: true`; import `github.com/srosignoli/sstorytime-kv` as `sst`
- [X] T005 Add `searchSvc *search.SearchService` field to the `App` struct in `app.go`; add `a.searchSvc = search.NewSearchService(cfg.CollectionDir)` in `startup()` after the successful collection load block (after `a.initialSums = summaries`); add the same call in `OpenCollection()` after `a.openFilePath = ""`; add `if a.searchSvc != nil { a.searchSvc.Close() }` in `shutdown()`; add `import "texere/internal/search"` to `app.go`
- [X] T006 Create `search_methods.go` (package main) with the `SearchGraph(query string) (*search.SearchResult, error)` method on `App`: if `a.searchSvc == nil` return `search.NewSearchResult()` with `GraphUnavailable: true`; otherwise delegate to `a.searchSvc.Search(query)`; import `"texere/internal/search"`
- [X] T007 Run `go build ./...` to verify the project compiles with the new package; fix any import cycle or type mismatch errors before proceeding
- [X] T008 Run `wails generate bindings` to regenerate `frontend/wailsjs/go/main/App.js`, `App.d.ts`, and `frontend/wailsjs/go/models.ts` with the new `SearchGraph` export and `search.*` types; verify `SearchGraph` appears in `App.js`

**Checkpoint**: Foundation ready — `SearchGraph` is callable from the frontend but returns `GraphUnavailable: true`. OmniBar user story work can now begin.

---

## Phase 3: User Story 1 — Quick Navigation via Fuzzy File Search (Priority: P1) 🎯 MVP

**Goal**: The existing OmniBar fuzzy search (chapters, contexts, nodes) already works. This phase adds the two missing items from the spec: Tab key no-op and standardizing the node result cap to 10.

**Independent Test**: Open OmniBar (Ctrl+K), type any word, verify fuzzy results appear instantly; arrow keys navigate; Enter jumps the editor to the correct node; Escape closes; clicking outside closes; Tab does NOT move focus out of the overlay.

- [X] T009 [US1] Add Tab key handling to `handleKeydown` in `frontend/src/components/OmniBar.svelte`: add `if (e.key === 'Tab') { e.preventDefault(); return }` as the first guard in the function body, before the existing Escape/Arrow/Enter checks (FR-011)
- [X] T010 [US1] In `frontend/src/components/OmniBar.svelte`, change the nodes result cap in the `results` reactive block from `nodes.splice(10)` to `nodes.splice(10)` (already 10 — verify it is 10 and leave as-is); verify chapters cap is at most 10 and contexts cap is at most 10; update the `// Nodes — up to 10` comments to reference FR-019 for traceability

**Checkpoint**: User Story 1 is fully functional. All existing fuzzy search behaviour is verified; Tab is now a no-op.

---

## Phase 4: User Story 2 — Graph-Powered Node Search (Priority: P2)

**Goal**: Implement the Tier 2 graph search. Backend opens BadgerDB lazily, calls SSTorytime-KV for plain text / context / chapter queries, and returns `GraphNode` results. Frontend adds debounce (200ms), "Searching…" state, and result cards showing orbit neighbors.

**Independent Test**: Build graph (Ctrl+G), open OmniBar, type a node text substring, wait 200ms — Tier 2 shows graph results with node text, orbit neighbors (arrow code + direction + neighbor text), chapter, and context tags. Enter on a graph result jumps the editor to that node's source line.

- [X] T011 [US2] Implement the lazy-open logic in `internal/search/search.go` `Search()`: acquire `s.mu.Lock()` at the start; if `s.store == nil`, call `IsReady()` — if false, return `NewSearchResult()` with `GraphUnavailable: true`; if true, call `sst.OpenBadgerStore(s.dbPath)` — on error return `NewSearchResult()` with `GraphError: err.Error()`; on success set `s.store = kv` and `s.pst = sst.PoSST{KV: kv}` then unlock; wrap the subsequent search logic in a `defer func() { if r := recover(); r != nil { result.GraphError = fmt.Sprintf("search panic: %v", r) } }()` to catch SSTorytime-KV panics
- [X] T012 [US2] Implement `parseAndRouteQuery(query string, params sst.SearchParameters) queryRoute` helper in `internal/search/search.go` that classifies `SearchParameters` into one of: `routeNodeSearch`, `routeStructural`, `routeListing` — for Phase 4 only `routeNodeSearch` needs to be implemented; structural and listing can return `routeUnimplemented` for now
- [X] T013 [US2] Implement the `routeNodeSearch` branch in `internal/search/search.go` `Search()`: call `sst.DecodeSearchField(query)` to get `params`; call `sst.GetDBNodePtrMatchingNCCS(s.pst, params.Name[0], params.Chapter, params.Context, nil, false, 10)` to get `[]sst.NodePtr`; for each `NodePtr` fetch the `Node` via `sst.GetDBNodeByPtr` (or equivalent), get its orbit via `sst.GetNodeOrbit(s.pst, nptr, "", 8)`, and build a `GraphNode` with `NodeText = node.S`, `Chapter = node.Chap`, `Orbit` populated from orbit entries at radius 1; cap total nodes at 10; return in `SearchResult.Nodes`
- [X] T014 [US2] Add `OrbitEntry` population in `internal/search/search.go`: for each `Orbit` entry at radius 1 from `GetNodeOrbit`, determine direction by checking `STindex` (even STindex = outbound, odd = inbound per SSTorytime-KV convention); set `ArrowCode` from `orbit.Arrow`, `NeighborText` from `orbit.Text`; look up `ArrowLong` via `sst.GetDBArrowByPtr` if needed; cap at 8 entries per node
- [X] T015 [P] [US2] Add Tier 2 state variables to `frontend/src/components/OmniBar.svelte`: add `let graphResults = null`, `let graphLoading = false`, `let debounceTimer = null`; add an import for `SearchGraph` from `../../wailsjs/go/main/App`
- [X] T016 [US2] Add reactive debounce block in `frontend/src/components/OmniBar.svelte`: `$: { clearTimeout(debounceTimer); const q = query.trim(); if (q) { graphLoading = true; graphResults = null; debounceTimer = setTimeout(() => dispatchGraphSearch(q), 200) } else { graphResults = null; graphLoading = false } }`; implement `async function dispatchGraphSearch(q)` that calls `SearchGraph(q)`, sets `graphResults = result`, sets `graphLoading = false`; wrap in try/catch (set `graphLoading = false` on error)
- [X] T017 [US2] Implement `function enrichGraphNodes(nodes)` in `frontend/src/components/OmniBar.svelte` that cross-references each `GraphNode.NodeText` against the existing `fileCache` to find `filePath` and `line`; iterate `Object.entries(fileCache)` → chapters → nodes; return enriched array `[{ ...gn, filePath, line, chapterTitle }]`; if no match, set `filePath: null, line: null`
- [X] T018 [US2] Add Tier 2 result section to the `{#if query.trim()}` block in `frontend/src/components/OmniBar.svelte` — below the existing node results: render a `<div class="result-label">Graph</div>` section; if `graphLoading` show `<div class="graph-loading">Searching…</div>`; if `graphResults?.GraphUnavailable` show `<div class="graph-unavailable">Build your graph first (Ctrl+G) to enable graph search.</div>`; if `graphResults?.GraphError` show the error message; otherwise render each enriched `GraphNode` as a result button showing node text (primary), orbit summary (2-3 neighbors inline), chapter and context tags (hint); style consistently with existing result items
- [X] T019 [US2] Extend `flatResults` in `frontend/src/components/OmniBar.svelte` to include enriched graph nodes (type `'graph-node'`): append them after the existing `nodes` entries; extend `navigate()` to handle `type === 'graph-node'` by calling `workbenchStore.update` with `selectedChapterKey`, `pendingScrollLine: item.line`, `selectedNodeLine: item.line`, `activeTab: 'note'`; if `item.filePath` is null, show an error toast (use existing pattern) without navigating; extend `handleKeydown` Enter branch to handle graph nodes

**Checkpoint**: User Stories 1 and 2 are both functional. Graph results appear in Tier 2 with orbit display. Enter navigates to the correct line.

---

## Phase 5: User Story 3 — Structural & Listing Queries (Priority: P3)

**Goal**: Extend `Search()` to route `\from\to`, `\paths`, `\sequence`, `\chapter any`, `\notes`, `\context`, `\arrow` queries to the correct SSTorytime-KV functions and return `StructuralChain` / `ListingItem` results. Extend OmniBar Tier 2 to display them.

**Independent Test**: With graph built, type `\chapter any` → all chapters listed; `\arrow means` → arrow definition shown; `\notes brain` → note page lines shown; `\from brain \to learning` → chain displayed as plain text; Enter on a chain node (if enrichable) navigates to its source line.

- [X] T020 [US3] Implement `routeStructural` branch in `internal/search/search.go`: detect when `params.From != nil` or `params.Sequence == true`; for path queries call `sst.GetPathsAndSymmetries(s.pst, startSet, endSet, params.Chapter, params.Context, nil, nil, 1, 6)`; for sequence queries call `sst.GetSequenceContainers(s.pst, nodePtrs, nil, nil, 10)`; convert each `[]sst.Link` chain into a `StructuralChain` by resolving node text via `sst.GetDBNodeByPtr` for each `Link.Dst`; populate `ChainNode.ArrowCode` from `sst.GetDBArrowByPtr(s.pst, link.Arr).Short`; return in `SearchResult.Structural`
- [X] T021 [US3] Implement `routeListing` branch in `internal/search/search.go` for four sub-types: `\chapter any` / `\contents` / `\toc` → call `sst.GetChaptersByChapContext(s.pst, params.Chapter, params.Context, 20)`, return as `ListingItem{Kind: "chapter", Title: chapterName}`; `\notes [chapter]` → call `sst.GetDBPageMap(s.pst, params.Chapter, params.Context, 0)`, return as `ListingItem{Kind: "note-line", Title: pm.Alias, Detail: pm.Chapter}`; `\arrow [term]` → call `sst.GetDBArrowByName(s.pst, params.Name[0])` or `sst.GetDBArrowBySTType(...)`, return as `ListingItem{Kind: "arrow", Title: ad.Short, Detail: ad.Long}`; `\context [term]` → use `GetChaptersByChapContext` with context filter, return as `ListingItem{Kind: "context", Title: tag, Detail: chapterName}`
- [X] T022 [US3] Extend Tier 2 display in `frontend/src/components/OmniBar.svelte` for `StructuralChain`: render each chain as a collapsible item (chain summary as button, full node sequence shown inline as `A → [arrow] → B → [arrow] → C`); each chain node is independently selectable in `flatResults` as type `'graph-structural'`; extend `navigate()` for `'graph-structural'` using the same enrichment + fileCache lookup as `'graph-node'`
- [X] T023 [US3] Extend Tier 2 display in `frontend/src/components/OmniBar.svelte` for `ListingItem`: render listing items under a section label matching their kind ("Chapters", "Arrows", "Contexts", "Notes"); type `'graph-listing'`; extend `navigate()` for `'graph-listing'` (navigate if `item.NodeText` non-empty, otherwise no-op)
- [X] T024 [US3] Update `parseAndRouteQuery` classifier in `internal/search/search.go` to replace the `routeUnimplemented` stubs with the correct `routeStructural` / `routeListing` returns based on the full condition table in `docs/specs/004-unified-search-graph/research.md` §1 "Query Type Classification"

**Checkpoint**: All query types produce visible results in the overlay. Structural chains display as plain-text sequences. Listing commands show chapter/arrow/context catalogs.

---

## Phase 6: User Story 4 — Result Limiting and Exact/Accent-Insensitive Matching (Priority: P3)

**Goal**: Ensure `\limit N`, `!exact!`, `|exact|`, and `(accent-insensitive)` modifiers are correctly passed through and applied. `DecodeSearchField` already handles these in `SearchParameters`; this phase wires them into the result cap logic and verifies pass-through.

**Independent Test**: Query `supermarket \limit 3` returns ≤3 graph results; `!supermarket!` returns only exact matches; `(fangzi)` matches accented variants.

- [X] T025 [US4] In `internal/search/search.go` `Search()`, extract `limit` from `params.Range` after `DecodeSearchField` (SSTorytime-KV sets `SearchParameters.Range` from `\limit N`); if `params.Range > 0` use it as the node cap instead of the default 10; pass `limit` as the final argument to `GetDBNodePtrMatchingNCCS` and `GetPathsAndSymmetries` instead of the hardcoded `10`
- [X] T026 [US4] In `frontend/src/components/OmniBar.svelte`, add FR-019 Tier 2 cap: after receiving `graphResults.Nodes`, slice to the first 10 entries before enrichment unless the query contains a `\limit` token (detect with `query.includes('\\limit')`); if `\limit` is present, keep all returned nodes up to 50 as a safety cap
- [X] T027 [US4] Verify pass-through of exact-match (`!…!`, `|…|`) and accent-insensitive (`(…)`) modifiers: these are handled internally by `DecodeSearchField` before reaching `GetDBNodePtrMatchingNCCS` — confirm by running `make dev`, opening the overlay, and testing each modifier syntax with a known node; document any SSTorytime-KV version constraints in `research.md` if behaviour differs

**Checkpoint**: All four user stories are complete and independently testable.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Build validation, style consistency, and spec documentation update.

- [X] T028 [P] Run `make build` and `make build-windows` to verify both Linux and Windows cross-compilation succeed with the new `github.com/srosignoli/sstorytime-kv` dependency; fix any CGo or platform-specific issues (pure Go dependency — should be clean)
- [X] T029 [P] Add CSS styles for the new Tier 2 elements in `frontend/src/components/OmniBar.svelte`: `.graph-loading`, `.graph-unavailable`, `.graph-error`, `.orbit-summary`, `.chain-display` — use the existing colour palette (`#1a2535` background, `#5a7a9a` secondary text, `#e8eaf0` primary text) and match the result-item spacing
- [X] T030 Run the quickstart verification checklist from `docs/specs/004-unified-search-graph/quickstart.md` §"Step 6: Verify" in `make dev` mode: test each scenario (Tier 1 regression, Tier 2 unavailable, Tier 2 available, navigation, structural, listing); confirm all pass before marking the feature complete

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 — **BLOCKS all user stories**
- **US1 (Phase 3)**: Depends on Phase 2 — minimal changes to existing OmniBar
- **US2 (Phase 4)**: Depends on Phase 2 — can start in parallel with US1 after Phase 2
- **US3 (Phase 5)**: Depends on Phase 4 (needs Search() routing infrastructure)
- **US4 (Phase 6)**: Depends on Phase 4 (needs Search() limit wiring)
- **Polish (Phase 7)**: Depends on all user story phases

### User Story Dependencies

- **US1 (P1)**: Independent after Phase 2 — no graph needed; OmniBar already works
- **US2 (P2)**: Independent after Phase 2 — builds on top of Search() stub
- **US3 (P3)**: Depends on US2 (Search routing infrastructure must exist)
- **US4 (P3)**: Depends on US2 (limit applies to Search() node cap)

### Within Each Phase

- T003 and T004 can run in parallel (different files)
- T005, T006 must follow T003/T004 (import the new package)
- T007 must follow T005/T006 (compile check)
- T008 must follow T007 (bindings from compiled code)
- T015 can run in parallel with T011–T014 (frontend state vars, no backend dependency)

### Parallel Opportunities

- T003 [P] `internal/search/types.go` and T004 `internal/search/search.go` — different files
- T015 [P] Frontend state vars — no dependency on T011–T014 backend changes
- T028 [P] Build check and T029 [P] CSS styles — independent polish tasks

---

## Parallel Example: Phase 2 (Foundational)

```
Parallel batch 1:
  Task T003: Create internal/search/types.go
  Task T004: Create internal/search/search.go (stub)

Sequential:
  Task T005: Wire App struct (depends on T003+T004)
  Task T006: Create search_methods.go (depends on T003)
  Task T007: go build ./... (depends on T005+T006)
  Task T008: wails generate bindings (depends on T007)
```

## Parallel Example: Phase 4 (US2)

```
Parallel batch 1:
  Task T011: Lazy-open logic in search.go
  Task T015: Frontend state vars in OmniBar.svelte

Sequential after T011:
  Task T012: parseAndRouteQuery classifier
  Task T013: routeNodeSearch implementation
  Task T014: OrbitEntry population

Sequential after T015 + T013+T014:
  Task T016: Debounce + dispatchGraphSearch
  Task T017: enrichGraphNodes function
  Task T018: Tier 2 display section
  Task T019: flatResults + navigate() extension
```

---

## Implementation Strategy

### MVP First (User Story 1 + Foundation Only)

1. Complete Phase 1: Setup (T001–T002)
2. Complete Phase 2: Foundational (T003–T008) — CRITICAL
3. Complete Phase 3: User Story 1 (T009–T010)
4. **STOP and VALIDATE**: Fuzzy search works; Tab is no-op; foundation compiles
5. Ship or demo P1

### Incremental Delivery

1. Phase 1 + Phase 2 → Foundation ready, SearchGraph callable (returns unavailable)
2. Phase 3 → US1 complete (fuzzy search fully spec-compliant)
3. Phase 4 → US2 complete (graph search live) — **main feature value delivered**
4. Phase 5 → US3 complete (structural + listing queries)
5. Phase 6 → US4 complete (modifiers wired)
6. Phase 7 → Polish + build validation

---

## Notes

- [P] tasks = different files, no shared state — safe to run in parallel
- [Story] label maps each task to its user story for traceability
- `make dev` hot-reloads both Go and Svelte — use it throughout development
- SSTorytime-KV search functions may panic — all Search() calls must be wrapped in `recover()`
- After any Go type change, re-run `wails generate bindings` to keep frontend types in sync
- The `fileCache` in OmniBar.svelte is the in-memory index for source-location enrichment — no separate index needed in the backend
- T027 is a manual verification step — no code change required if pass-through works correctly
