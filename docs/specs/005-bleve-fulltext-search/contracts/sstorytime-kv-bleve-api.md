# Contract: SSTorytime-KV Bleve API

**Feature**: 005-bleve-fulltext-search
**Date**: 2026-04-25
**Type**: New public Go API surface in package `SSTorytime` (module `github.com/srosignoli/sstorytime-kv`).

This contract defines the API that Texere (and any other downstream consumer) calls to use the Bleve-backed full-text matcher. Per clarification Q1, the library owns the Bleve dependency, the index lifecycle, the operator-grammar translator, and the query API. Consumers MUST NOT import `github.com/blevesearch/bleve/v2` directly (FR-002a).

---

## Module & version

| Item | Value |
|---|---|
| Module | `github.com/srosignoli/sstorytime-kv` |
| Current Texere pin | `v0.3.0` |
| New version (post-feature) | `v0.4.0` (additive — no breaking changes, but enough to warrant a minor bump) |
| Backwards compat | `SearchNodesByName` and `GetDBNodePtrMatchingNCCS` remain callable with their existing signatures; their substring-match implementations may be removed in `v0.5.0` once all known consumers migrate. |

---

## Types

```go
package SSTorytime

// Index wraps a Bleve full-text index. One Index per collection directory.
//
// Lifecycle:
//   - NewBleveIndex creates a fresh index (used by Build graph).
//   - OpenBleveIndex opens an existing index for reading (used by search).
//   - Close releases resources. Idempotent.
//
// Concurrency:
//   - A single Index value is safe for concurrent SearchByQuery calls.
//   - Concurrent writes (AddNode / ApplyBatch) are NOT safe; Build graph
//     is single-writer.
//   - A read-only Index opened with OpenBleveIndex MUST NOT be passed to
//     AddNode / ApplyBatch (returns ErrReadOnly).
type Index struct { /* opaque */ }

// IndexBatch accumulates pending writes for a single bulk commit.
type IndexBatch struct { /* opaque */ }

// ParseError indicates malformed operator syntax in a query string.
// SearchByQuery returns *ParseError (not a wrapped error) when the input
// fails to parse. Callers can type-assert to extract the position for UI
// presentation.
type ParseError struct {
    Pos     int    // byte offset into the original query string
    Message string // human-readable description of the problem
}

func (e *ParseError) Error() string
```

### Sentinel errors

```go
var (
    ErrIndexNotFound = errors.New("bleve index not found at path")
    ErrIndexCorrupt  = errors.New("bleve index is corrupted or unreadable")
    ErrReadOnly      = errors.New("index opened read-only; writes not permitted")
)
```

---

## Constructors

### `NewBleveIndex(path string) (*Index, error)`

Creates a fresh Bleve index at `path`. If `path` already exists, it is removed first (mirrors `os.RemoveAll(dbPath)` in `builder.go:37`).

| Parameter | Type | Constraints |
|---|---|---|
| `path` | `string` | Non-empty filesystem path. Parent directory must exist. Path must be absolute or relative to the current working directory at call time. |

**Returns**:

- `*Index` — writable index handle. Caller MUST `Close` when done.
- `error` — non-nil if directory removal failed, parent directory missing, or Bleve initialisation failed.

**Side effects**: Removes `path` if present. Creates `path` and Bleve segment files inside it.

**Used by**: `BuildFromCollection` in Texere `internal/graph/builder.go`.

---

### `OpenBleveIndex(path string) (*Index, error)`

Opens an existing Bleve index at `path` for reading.

| Parameter | Type | Constraints |
|---|---|---|
| `path` | `string` | Path to a directory previously written by `NewBleveIndex`. |

**Returns**:

- `*Index` — read-only index handle. Caller MUST `Close` when done.
- `error` —
  - `ErrIndexNotFound` if `path` does not exist.
  - `ErrIndexCorrupt` if the directory exists but cannot be opened as a Bleve index.
  - Other I/O errors otherwise.

**Side effects**: None. No files are written.

**Used by**: `SearchService` lazy-open in Texere `internal/search/search.go`.

---

## Indexing methods

### `(*Index) AddNode(ptr NodePtr, text, chapter, sourceFile string) error`

Indexes a single node. Idempotent — re-adding a node with the same `NodePtr` replaces the prior document.

| Parameter | Type | Constraints |
|---|---|---|
| `ptr` | `NodePtr` | Must be a valid populated `NodePtr` (not the zero value). Used as document ID. |
| `text` | `string` | The node's text. May be any UTF-8 string. Empty strings are accepted but produce a document with no analyzed tokens. |
| `chapter` | `string` | Chapter name. May be empty. |
| `sourceFile` | `string` | Path to the source `.n4l` file. May be empty. Stored, not indexed. |

**Returns**: `error` —

- `ErrReadOnly` if the Index was opened with `OpenBleveIndex`.
- Bleve-internal error if the underlying segment write failed.

**Performance**: O(token count). Caller should use `NewBatch`/`ApplyBatch` for bulk writes during Build graph.

---

### `(*Index) NewBatch() *IndexBatch`

Returns a fresh `IndexBatch` for accumulating writes.

### `(*IndexBatch) Add(ptr NodePtr, text, chapter, sourceFile string)`

Same parameters as `AddNode`, but defers the write until `ApplyBatch` is called. Cannot fail (validation deferred to apply time).

### `(*Index) ApplyBatch(b *IndexBatch) error`

Commits a batch of pending writes. Single I/O round-trip rather than one per `Add`.

**Returns**: `error` if the underlying Bleve commit fails. On error, the batch is invalidated and must be discarded; partial commits are not visible.

---

## Query method

### `(*Index) SearchByQuery(queryText string, limit int) ([]NodePtr, error)`

Parses `queryText` into a Bleve query (per the operator grammar in `research.md § R4`) and returns matching `NodePtr`s.

| Parameter | Type | Constraints |
|---|---|---|
| `queryText` | `string` | The plain-text portion of a `searchN4L` query, after `\` commands have been stripped by `DecodeSearchField`. May be a single bare token, a multi-token phrase, or an operator expression. |
| `limit` | `int` | Maximum results to return. `0` means no cap (Bleve's default page size will be used). Negative values return an error. |

**Returns**:

- `[]NodePtr` — results ordered by Bleve relevance score (descending), with `NodePtr` ascending as the deterministic tie-breaker (FR-023a). Empty slice (length zero) when there are no hits.
- `error` —
  - `*ParseError` if `queryText` is syntactically malformed (FR-015b). The result slice is `nil` in this case.
  - Bleve-internal error if the index read failed.

### Empty input

`queryText == ""` returns `(nil, nil)` — no matches, no error. Callers (Texere `searchNodes`) check for empty `params.Name` before calling and never pass an empty string in normal use.

### Examples

| Input | Expected behaviour |
|---|---|
| `"running"` | Match query against `text_en \| text_cjk` for "running". With English stemmer, also matches `runs`, `runner`. |
| `"房子"` | Match query against `text_cjk` for the bigram `房子`; against `text_en` it analyzes to a Han-character token list and is unlikely to match Latin nodes. |
| `"\"fish soup\""` | Phrase query against `text_en \| text_cjk`. Returns nodes containing the exact two-word sequence. |
| `"!A!"` | Term query against `text_raw` for `"a"` (lowercased). Returns only nodes whose entire text lowercases to `"a"`. |
| `"flo:*"` | Prefix query against `text_raw` for `"flo"`. Returns nodes whose text starts with `"flo"` after lowercasing. |
| `"strange<->kind"` | Phrase query for `"strange kind"` with slop=0 against `text_en \| text_cjk`. |
| `"strange<2>woman"` | Phrase query for `"strange woman"` with slop=2. |
| `"brain&!notes"` | Conjunction(Match("brain"), MustNot(Match("notes"))). |
| `"word1 \| word2"` | Disjunction(Match("word1"), Match("word2")). |
| `"chemistry C & S reaction"` | Plain literal — operator-like `&` has whitespace on both sides, so per FR-015a it is treated as a literal token. Default-OR disjunction across all five tokens. |
| `"\"fish soup"` | Unclosed quote. Returns `*ParseError{Pos: 0, Message: "unclosed quote starting at position 0"}`. |
| `"brain&"` | Trailing operator. Returns `*ParseError{Pos: 6, Message: "operator '&' missing right operand"}`. |
| `"strange<-1>kind"` | Invalid slop. Returns `*ParseError{Pos: 7, Message: "proximity slop must be non-negative"}`. |

---

## Lifecycle method

### `(*Index) Close() error`

Flushes any pending writes (no-op for read-only handles), closes the underlying Bleve index, releases file handles. Idempotent — calling `Close` on an already-closed Index returns `nil`.

After `Close`, all other methods return an error.

---

## Re-wired existing functions

These library functions keep their existing signatures but gain Bleve-backed implementations:

### `SearchNodesByName(textName string, limit int) []NodePtr` — DEPRECATED

Callers must migrate to `(*Index).SearchByQuery`. Until removal in v0.5.0, this function:

- If a global `Index` has been registered with `RegisterDefaultBleveIndex(*Index)`, delegates to `SearchByQuery`.
- Otherwise, falls back to the legacy substring matcher and emits a `log.Println` deprecation warning **once per process**.

This deprecation path is for any downstream consumer outside Texere; **Texere will not use it** — it always opens an `Index` directly.

### `GetDBNodePtrMatchingNCCS(...)` — DEPRECATED, same migration path

---

## Texere consumption pattern

```go
// internal/search/search.go (sketch)

func (s *SearchService) lazyOpen() error {
    // ... open BadgerDB as today ...
    idx, err := SST.OpenBleveIndex(s.blevePath)
    if err != nil { return err }
    s.bleveIdx = idx
    return nil
}

func (s *SearchService) searchNodes(params SST.SearchParameters, ..., limit int) []GraphNode {
    if len(params.Name) == 0 {
        return s.searchNodesWildcard(params, limit)   // existing IterateNodes path
    }
    queryText := strings.Join(params.Name, " ")
    seeds, err := s.bleveIdx.SearchByQuery(queryText, limit*10)
    if err != nil {
        if pe, ok := err.(*SST.ParseError); ok {
            // surface to overlay; result.GraphError set by caller
            return nil
        }
        return nil
    }
    // … apply chapter + context filters; build orbit; cap at limit …
}
```

---

## Telemetry / observability

Out of scope for v1 (research § Outstanding). The library does not log query text. Callers that wish to log latency should wrap `SearchByQuery` themselves.

---

## Backwards compatibility

This release introduces no breaking changes to existing exported types or functions in `SSTorytime-KV`. New types (`Index`, `IndexBatch`, `ParseError`) are additive. Existing callers continue to compile and run; they merely miss the Bleve upgrade until they migrate.

The on-disk layout changes are additive: `<collectionDir>/sst_data/` is unchanged; `<collectionDir>/bleve/` is new. Pre-Bleve collections behave as Q4 specifies (overlay error → user runs Ctrl+G).
