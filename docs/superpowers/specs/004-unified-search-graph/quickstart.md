# Quickstart: Unified Search Bar with SSTorytime-KV Graph Integration

**Branch**: `004-unified-search-graph` | **Date**: 2026-04-17

---

## Prerequisites

- Go 1.22+ with the project building (`make build`)
- A collection with at least one `.n4l` file
- The graph built at least once (Ctrl+G) so `sst_data/` exists for Tier 2 testing

---

## Step 1: Add SSTorytime-KV dependency

```bash
cd /home/simone/projects/texere
go get github.com/srosignoli/sstorytime-kv
```

Verify it appears in `go.mod` and `go.sum`.

---

## Step 2: Create the search package

Create `internal/search/types.go` and `internal/search/search.go` per the data-model and contracts.

**Smoke test**:
```bash
go build ./internal/search/...
```

---

## Step 3: Add SearchService to App

In `app.go`:
- Add `searchSvc *search.SearchService` field to `App`
- In `OpenCollection`, after loading summaries: `a.searchSvc = search.NewSearchService(dir)`
- In `startup`, after successful collection load: same
- In `shutdown`: call `a.searchSvc.Close()` if non-nil

Add `SearchGraph` method in a new `search_methods.go`:
```go
func (a *App) SearchGraph(query string) (*search.SearchResult, error) {
    if a.searchSvc == nil || !a.searchSvc.IsReady() {
        return &search.SearchResult{GraphUnavailable: true,
            Nodes: []search.GraphNode{}, Listings: []search.ListingItem{},
            Structural: []search.StructuralChain{}}, nil
    }
    return a.searchSvc.Search(query)
}
```

---

## Step 4: Regenerate Wails bindings

```bash
wails generate bindings
```

Verify `frontend/wailsjs/go/main/App.js` now exports `SearchGraph`.

---

## Step 5: Extend OmniBar.svelte

1. Import `SearchGraph` from wailsjs
2. Add `graphResults`, `graphLoading`, `debounceTimer` state variables
3. Add reactive debounce block (200ms) that calls `SearchGraph(query)`
4. Add `enrichGraphNodes(results)` that cross-references `fileCache`
5. Add Tier 2 section to the results area (below existing node results)
6. Add Tab key handler (no-op, `e.preventDefault()`)
7. Extend `flatResults` with `graph-node`, `graph-structural`, `graph-listing` types
8. Handle `GraphUnavailable` and `GraphError` states in the Tier 2 display

---

## Step 6: Verify

**Tier 1 (regression)**: Open OmniBar with Ctrl+K, type any word → existing fuzzy results still appear.

**Tier 2 (unavailable)**: Without a built graph, Tier 2 shows "Build your graph first (Ctrl+G)."

**Tier 2 (available)**: After Ctrl+G, type a node text substring → graph results appear below fuzzy results within 100ms of pause.

**Navigation**: Arrow keys move through both tiers; Enter on a graph node jumps the editor to the correct line.

**Structural**: Type `\from brain \to spaced repetition` → result displays as a plain-text chain.

**Listing**: Type `\chapter any` → all chapters listed. Type `\arrow means` → arrow definition shown.

---

## Key File Locations

| File | Purpose |
|------|---------|
| `internal/search/types.go` | SearchResult, GraphNode, OrbitEntry, StructuralChain, ListingItem |
| `internal/search/search.go` | SearchService, Open/Close, Search, IsReady |
| `search_methods.go` | SearchGraph Wails binding on App |
| `frontend/src/components/OmniBar.svelte` | Tier 1 + Tier 2 overlay UI |
| `frontend/wailsjs/go/main/App.js` | Auto-generated binding (regenerate after Go changes) |
