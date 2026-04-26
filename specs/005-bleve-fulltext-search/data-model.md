# Phase 1 Data Model: Replace text matching with Bleve full-text search engine

**Feature**: 005-bleve-fulltext-search
**Date**: 2026-04-25

This document defines:

1. The Bleve **document mapping** (per-node schema written to the index).
2. The **Go types** added to SSTorytime-KV (`Index`, internal AST nodes for the translator).
3. The **Texere-side type changes** (`SearchService` field additions; new error mode).

No persistent data structures change in Texere or BadgerDB. The Bleve index is a new derived store at `<collectionDir>/bleve/`.

---

## 1. Bleve index document mapping

**Index name**: implicit (single index per directory).
**Document ID**: stringified `NodePtr` (hex-encoded, fixed-width). Allows direct use as a Bleve document key and supports re-indexing of the same node by the same ID.

### Document fields

| Field | Type | Analyzer | Indexed | Stored | Purpose |
|---|---|---|---|---|---|
| `text_en` | text | Custom analyzer: `unicode` tokenizer → `to_lower` → `asciifolding` → `stemmer_en_snowball` | yes | no | Latin-script matching with case fold, accent fold, and English stemming (FR-007, FR-010, FR-014) |
| `text_cjk` | text | `cjk` (CJK bigram tokenizer) | yes | no | CJK matching (FR-007) |
| `text_raw` | keyword | `keyword` (no tokenization), lowercased | yes | no | Exact-term (`!A!`/`|A|`) and prefix (`flo:*`) queries (FR-009, FR-013) |
| `chapter` | keyword | `keyword` | yes | no | Optional pre-filter; chapter filter still applies post-Bleve in Texere |
| `source_file` | keyword | `keyword` | no | yes | Recorded for future use; not queryable today (FR-017d) |
| `node_ptr` | keyword | `keyword` | no | yes | The `NodePtr` itself, stored alongside the document so `SearchByQuery` can return `[]NodePtr` without re-deriving from the document ID |

**Notes**:

- `Stored=false` for the analyzed text fields keeps the index small. We never need to retrieve the original text via Bleve — Texere already has it in `BadgerKV.IterateNodes`/`GetNode`.
- Filter order matters: `to_lower` first, then ASCII-folding, then the English Snowball stemmer. Result: `Naïve` → `naïve` → `naive` → `naiv`; `Fángzi` → `fángzi` → `fangzi` → `fangzi`. Both query directions match (FR-010, FR-014, SC-005).
- The CJK analyzer's bigram tokenizer means `房子` indexes as the bigram `房子` and the unigrams `房`, `子`. A query for `房子` matches; a query for `房` matches the unigram. This satisfies SC-005's CJK acceptance scenario.

### Bleve index mapping (Go construction)

Built once in `NewBleveIndex` / `OpenBleveIndex`:

```go
indexMapping := bleve.NewIndexMapping()

// text_en: standard + asciifolding
err := indexMapping.AddCustomAnalyzer("text_en_analyzer", map[string]any{
    "type":          "custom",
    "tokenizer":     "unicode",
    "token_filters": []string{"to_lower", "asciifolding", "stemmer_en_snowball"},
})

// text_cjk: built-in cjk analyzer
// (registered via _ "github.com/blevesearch/bleve/v2/analysis/lang/cjk")

docMapping := bleve.NewDocumentMapping()
docMapping.AddFieldMappingsAt("text_en",      bleve.NewTextFieldMapping().withAnalyzer("text_en_analyzer"))
docMapping.AddFieldMappingsAt("text_cjk",     bleve.NewTextFieldMapping().withAnalyzer("cjk"))
docMapping.AddFieldMappingsAt("text_raw",     bleve.NewTextFieldMapping().withAnalyzer("keyword"))
docMapping.AddFieldMappingsAt("chapter",      bleve.NewKeywordFieldMapping())
docMapping.AddFieldMappingsAt("source_file",  bleve.NewKeywordFieldMapping())
docMapping.AddFieldMappingsAt("node_ptr",     bleve.NewKeywordFieldMapping())

indexMapping.AddDocumentMapping("_default", docMapping)
```

(The above is illustrative; exact API spelling matches `bleve/v2`'s as of the version pinned in `go.mod`.)

---

## 2. Go types — SSTorytime-KV (library)

### 2.1 `Index` (public)

```go
// Index wraps a Bleve full-text index over the SSTorytime node corpus.
// Lifecycle: create with NewBleveIndex during Build graph, or open with
// OpenBleveIndex during search. Always Close() when done.
type Index struct {
    bleveIdx bleve.Index
    path     string
}
```

**Methods** (full contract in `contracts/sstorytime-kv-bleve-api.md`):

- `NewBleveIndex(path string) (*Index, error)` — fresh index; removes any pre-existing dir.
- `OpenBleveIndex(path string) (*Index, error)` — opens existing index for reading.
- `(*Index) AddNode(ptr NodePtr, text, chapter, sourceFile string) error`
- `(*Index) NewBatch() *IndexBatch` — for amortised writes during Build graph.
- `(*Index) ApplyBatch(b *IndexBatch) error`
- `(*Index) SearchByQuery(queryText string, limit int) ([]NodePtr, error)`
- `(*Index) Close() error`

### 2.2 `IndexBatch` (public)

```go
// IndexBatch accumulates AddNode calls and commits them in a single Bleve write.
// Created by Index.NewBatch(); applied via Index.ApplyBatch().
type IndexBatch struct {
    inner *bleve.Batch
}
```

Methods: `(*IndexBatch) Add(ptr NodePtr, text, chapter, sourceFile string)`.

### 2.3 Translator AST (internal — package-private)

These types live in `bleve_translate.go` and are not exported. They exist so the parser produces a strongly-typed AST that the lowering step then translates to Bleve primitives.

```go
type queryNode interface{ qNode() }

type bareToken    struct{ Text string }
type phraseToken  struct{ Text string }
type exactToken   struct{ Text string }              // !A! or |A|
type accentToken  struct{ Text string }              // (fangzi)
type prefixToken  struct{ Stem string }              // flo:*
type proximity    struct{ A, B string; Slop int }    // strange<->kind, strange<2>woman
type conjunction  struct{ Children []queryNode }     // a&b
type disjunction  struct{ Children []queryNode }     // a|b
type mustNot      struct{ Child queryNode }          // !a (prefix unary)
```

A single function `lower(node queryNode) bleve.Query` walks this tree and emits the Bleve primitives listed in `research.md § R4`.

### 2.4 Translator error type

```go
type ParseError struct {
    Pos     int    // byte offset into the original query string
    Message string // human-readable description
}

func (e *ParseError) Error() string { /* "parse error at position N: …" */ }
```

`SearchByQuery` returns `*ParseError` when the input is malformed (FR-015b). Callers can type-assert to recover the position for UI presentation.

---

## 3. Go types — Texere (consumer)

### 3.1 `SearchService` field additions

```go
type SearchService struct {
    // ... existing fields unchanged ...
    blevePath  string         // <collectionDir>/bleve  (set in NewSearchService)
    bleveIdx   *SST.Index     // opened lazily alongside BadgerDB
}
```

**`NewSearchService`** sets `blevePath = filepath.Join(collectionDir, "bleve")`.

**`IsReady`** (semantic change) returns true only when both `sst_data/` and `bleve/` directories exist:

```go
func (s *SearchService) IsReady() bool {
    if _, err := os.Stat(s.dbPath); err != nil { return false }
    if _, err := os.Stat(s.blevePath); err != nil { return false }
    return true
}
```

**Lazy-open** opens BadgerDB first, then Bleve. If BadgerDB opens but Bleve fails, `result.GraphError` carries the Bleve error string and `result.GraphUnavailable` is set so the overlay surfaces the Q4 message.

### 3.2 `SearchResult` — no schema change

`SearchResult.GraphUnavailable` already exists (feature 004) and is reused for the missing-Bleve-index case. No new fields. No frontend type regen needed beyond a possibly-tweaked toast message (`OmniBar.svelte`).

---

## 4. Validation rules

| Rule | Source | Where enforced |
|---|---|---|
| Document ID uniqueness | `NodePtr` is unique by construction (BadgerDB sequence) | `Index.AddNode` — `bleve.Index.Index()` is replace-on-conflict |
| `text_en`, `text_cjk`, `text_raw` all index the same source string | FR-020 | `Index.AddNode` — single source `text` argument fan-outs to three fields |
| Empty query short-circuits | FR-021 | `SearchService.searchNodes` — checks `len(params.Name) == 0` before calling Bleve |
| Limit ≥ 0 | API hygiene | `Index.SearchByQuery` — negative limits return an error |
| Index path must be absolute (or rooted at `<collectionDir>`) | FR-016 | `NewBleveIndex` — `filepath.Abs` + sanity check |

---

## 5. State transitions

The Bleve index has three on-disk states:

```
[NONE]   ──open()──▶ NotFound error  (Texere reports GraphUnavailable per Q4)
   │
   └─NewBleveIndex(path)──▶ [BUILDING] (writable, single writer = builder.go)
                                 │
                                 └─Close()──▶ [READY]
                                                 │
                                                 ├─OpenBleveIndex(path)──▶ [READY] (read-only)
                                                 │                              │
                                                 │                              └─Close()──▶ [READY]
                                                 │
                                                 └─NewBleveIndex(same path)──▶ [BUILDING] (rebuild from scratch)
```

There is no `[CORRUPTED]` recovery path — if the index is unopenable, Texere surfaces a clear error and the user runs Build graph to recreate it (matches the BadgerDB recovery story).

---

## 6. Concurrency invariants

- **Single writer at a time**: only `BuildFromCollection` writes. `SearchService` opens read-only.
- **Search may run concurrently with itself**: Bleve's read API is safe for concurrent calls; Texere already serialises through `SearchService.mu` per-instance, so this is unchanged.
- **No concurrent build + search**: enforced by Texere's existing pattern of closing `SearchService` before invoking `BuildFromCollection`. See research R8.
