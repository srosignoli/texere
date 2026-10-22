# Implementation Plan: Unified Search Bar with SSTorytime-KV Graph Integration

**Branch**: `004-unified-search-graph` | **Date**: 2026-04-17 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `/docs/superpowers/specs/004-unified-search-graph/spec.md`

## Summary

Extend the existing `OmniBar.svelte` overlay (which already implements Ctrl+K, fuzzy Tier 1 search, keyboard navigation, and click-outside dismissal) with a second Tier 2 section that queries the SSTorytime-KV BadgerDB. The backend wraps the SSTorytime-KV `DecodeSearchField` + search function pipeline in a new `internal/search` package, exposed as a single `SearchGraph(query)` Wails binding. The frontend debounces queries at 200ms, displays "Searching…" while in-flight, and cross-references returned node text against its existing `fileCache` to resolve source file path and line number for navigation.

## Technical Context

**Language/Version**: Go 1.22 (backend) + JavaScript/Svelte 3.49 (frontend)  
**Primary Dependencies**: Wails v2.12, SSTorytime-KV (`github.com/srosignoli/sstorytime-kv`, published module), CodeMirror 6, fsnotify v1.9  
**Storage**: BadgerDB via SSTorytime-KV (`<collectionDir>/sst_data/`); read-only from search feature perspective  
**Testing**: `go test ./...` (unit tests for `internal/search`); `make dev` for UI smoke tests  
**Target Platform**: Linux amd64 + Windows amd64 (cross-compiled, per existing CI)  
**Project Type**: Desktop app (Wails v2, Go + OS webview)  
**Performance Goals**: Overlay open <50ms; Tier 1 results <16ms/keystroke; Tier 2 results <100ms post-debounce; Enter navigation <50ms  
**Constraints**: Pure Go only (no CGo); SSTorytime-KV uses BadgerDB (pure Go) ✅; single binary  
**Scale/Scope**: Personal knowledge collection (~100–10,000 nodes typical); single-user, offline

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Source-First & Full N4L Fidelity | ✅ PASS | Search is read-only; no N4L files are written or modified |
| II. Self-Contained Binary | ✅ PASS | SSTorytime-KV is pure Go (BadgerDB); no external DB server; no network |
| III. Separation of Concerns | ✅ PASS | Search reads Content layer (fileCache, N4L parser) and Knowledge Graph layer (BadgerDB) independently; does not blur boundaries or store canonical content in BadgerDB |
| IV. Pleasure-Driven Learning | N/A | Search/navigation feature; no study scheduling logic |
| V. Transparency & Portability | ✅ PASS | Search results are non-destructive; no silent state changes |
| Performance — Orbit query <1ms | ✅ PASS | Constitution budget is ≥10× the 100ms Tier 2 target; BadgerDB key lookup is sub-millisecond |
| Dependency Discipline | ✅ PASS | `github.com/srosignoli/sstorytime-kv` is pure Go |

**Post-design re-check**: No new violations introduced. `SearchService` holds `PoSST` open for collection lifetime — this is a resource handle, not an external dependency.

## Project Structure

### Documentation (this feature)

```text
docs/superpowers/specs/004-unified-search-graph/
├── plan.md              # This file
├── research.md          # Phase 0 — SSTorytime-KV API, architecture decisions
├── data-model.md        # Phase 1 — Go types, frontend state additions
├── quickstart.md        # Phase 1 — dev setup and verification steps
├── contracts/
│   └── search-binding.md  # Phase 1 — SearchGraph Wails binding contract
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/search/
├── types.go             # SearchResult, GraphNode, OrbitEntry, StructuralChain, ListingItem
└── search.go            # SearchService: NewSearchService, IsReady, Search, Close

search_methods.go        # App.SearchGraph Wails binding (new file, package main)

app.go                   # Add searchSvc *search.SearchService field
                         # Add searchSvc init in startup() and OpenCollection()
                         # Add searchSvc.Close() in shutdown()

frontend/src/components/
└── OmniBar.svelte       # Extended: Tier 2 state, debounce, graph results display

frontend/wailsjs/go/
├── main/App.js          # Auto-regenerated (adds SearchGraph export)
├── main/App.d.ts        # Auto-regenerated
└── models.ts            # Auto-regenerated (adds search.* types)
```

**Structure Decision**: Single-project layout. Backend logic isolated in `internal/search/` (new package, no circular imports). Frontend change is additive to existing OmniBar component. New `search_methods.go` follows the existing pattern of `methods.go` for App-level Wails bindings.

## Complexity Tracking

No constitution violations. No complexity tracking entry required.
