# Research: Unified Search Bar with SSTorytime-KV Graph Integration

**Branch**: `004-unified-search-graph` | **Date**: 2026-04-17

---

## 1. SSTorytime-KV Go API

### Decision: Use `OpenBadgerStore` + `DecodeSearchField` pipeline

**Rationale**: The upstream `Open()` function calls `os.Exit(-1)` on failure, which would crash the Wails process. `OpenBadgerStore(path string) (*BadgerKV, error)` is the safe entry point for embedded use — it returns a standard `error` and does not call `os.Exit`. All other search functions accept `PoSST` which wraps the `*BadgerKV`.

**Alternatives considered**: Using `Open()` — rejected because it terminates the process on any DB error. Using a subprocess — rejected (violates self-contained binary principle).

---

### Decision: Keep PoSST open for the collection lifetime

**Rationale**: Opening/closing BadgerDB per query adds ~10–50ms of overhead (file locking, manifest read). Since the constitution's orbit query budget is <1ms (BadgerDB key lookup), the DB must be open. The `SearchService` struct holds `PoSST` open from collection load to collection change/shutdown.

**Alternatives considered**: Open per query — rejected (latency). Open lazily on first search — acceptable but add connection state field.

---

### Search Pipeline

There is no single exported `SearchN4L` function. The pipeline is:

```
query string
  → DecodeSearchField(query) → SearchParameters
  → route by query type:
      plain text / context / chapter → GetDBNodePtrMatchingNCCS(...)
      \chapter any / \in → GetChaptersByChapContext(...)
      \notes → GetDBPageMap(...)
      \arrow → GetDBArrowByName(...) / GetDBArrowBySTType(...)
      \from + \to / \paths → GetPathsAndSymmetries(...)
      \sequence / \seq → GetSequenceContainers(...)
  → GetNodeOrbit(sst, nptr, "", limit) → [ST_TOP][]Orbit (for Tier 2 node cards)
```

### Key Types

| Type | Key Fields | Usage |
|------|-----------|-------|
| `SearchParameters` | Name, From, To, Chapter, Context, Arrows, Limit, Sequence, Stats | Output of `DecodeSearchField` |
| `Node` | `S` (text), `Chap` (chapter), `NPtr` (pointer), `L` (graph-internal level — NOT source line) | Core result vertex |
| `NodePtr` | `Class int`, `CPtr ClassedNodePtr` | Stable graph identity for orbit lookup |
| `Orbit` | `Arrow` (arrow code), `Text` (neighbor text), `Radius`, `Ctx` (context), `STindex` | One neighbor in the orbit |
| `Story` | `Chapter`, `Axis []NodeEvent` | Sequence query result |
| `PageMap` | `Chapter`, `Line int`, `Path []Link` | Note page result |
| `ArrowDirectory` | `Long`, `Short`, `Ptr` | Arrow definition result |

### Query Type Classification

After `DecodeSearchField`:

| Condition | Query Type | Handler |
|-----------|-----------|---------|
| `params.From != nil \|\| params.To != nil \|\| len(params.Name)==0 with paths` | Structural (path) | `GetPathsAndSymmetries` |
| `params.Sequence == true` | Structural (sequence) | `GetSequenceContainers` |
| `params.Chapter != "" && params.Name == nil` | Listing (TOC) | `GetChaptersByChapContext` |
| `params.Stats == true` | Listing (notes/page) | `GetDBPageMap` |
| `len(params.Arrows) > 0 && params.Name == nil` | Listing (arrows) | `GetDBArrowBySTType` |
| Default | Node search | `GetDBNodePtrMatchingNCCS` |

---

## 2. BadgerDB Path Convention

**Decision**: DB path is `<collectionDir>/sst_data`

**Rationale**: The upstream `Open()` hardcodes `"sst_data"` relative to `cwd`. The Ctrl+G (Build graph) feature will write to `<collectionDir>/sst_data`. The `SearchService` checks `os.Stat(dbPath)` to determine availability.

**Check existence**:
```go
func (s *SearchService) IsReady() bool {
    _, err := os.Stat(s.dbPath)
    return err == nil
}
```

---

## 3. Existing OmniBar (Tier 1)

**Decision**: Extend `OmniBar.svelte` in-place; do not replace or wrap it.

**Rationale**: `OmniBar.svelte` already implements: Ctrl+K toggle, click-outside backdrop, autofocus, loading state, `fileCache` (node text → filePath + line via `Node.Line`), arrow-key navigation, Enter-to-navigate, Escape-to-close. The Tier 2 addition is purely additive — a new result section below the existing three, plus a debounce + backend call.

**Key existing data**: `fileCache` maps `filePath → ParsedFile`. Each `Node` in `ParsedFile` has `Line` (source line, 1-indexed) and `Text`. This IS the in-memory index for source-location enrichment — no separate index needed in the backend.

**Frontend enrichment pattern**:
```js
// After receiving graph results from backend:
function enrichWithSource(graphResult) {
  for (const [filePath, pf] of Object.entries(fileCache)) {
    for (const ch of pf.Chapters ?? []) {
      for (const node of ch.Nodes ?? []) {
        if (node.Text === graphResult.nodeText) {
          return { ...graphResult, filePath, line: node.Line, chapterTitle: ch.Title }
        }
      }
    }
  }
  return { ...graphResult, filePath: null, line: null }
}
```

---

## 4. Debounce Implementation

**Decision**: 200ms debounce in Svelte using `setTimeout`/`clearTimeout` pattern.

**Rationale**: Already validated by existing `bep/debounce` in go.mod (used by Wails internally). Frontend debounce is simpler — no additional dependency needed.

```js
let debounceTimer = null
$: {
  clearTimeout(debounceTimer)
  const q = query
  debounceTimer = setTimeout(() => dispatchGraphSearch(q), 200)
}
```

---

## 5. Tab Key — No-Op

**Decision**: Intercept Tab in `handleKeydown`; call `e.preventDefault()`, take no further action.

**Rationale**: Default browser Tab behavior would move focus out of the overlay. Preventing default keeps focus in the overlay without triggering graph explorer (future feature).

---

## 6. Node Source Location — Frontend Cross-Reference

**Decision**: Source location (filePath, line) is resolved entirely in the frontend by cross-referencing `graphResult.NodeText` against `fileCache`.

**Rationale**: The user confirmed that neither file path nor line number is stored in BadgerDB. The frontend `fileCache` (built lazily when OmniBar opens) already contains all nodes with their `Line` and `Text` fields from the N4L parser. The backend returns only graph data (node text, orbit, chapter, context); the frontend does the join.

**Duplicate node text**: If multiple nodes share the same text, all matching locations are returned as sub-items. The first match (by file/chapter order) is the default; others shown as alternatives.

---

## 7. Structural Query Display

**Decision**: Structural results (`\from\to`, `\paths`, `\sequence`) rendered as a flat plain-text list with arrow labels between nodes.

**Format example**:
```
brain → [means] → learning → [e.g.] → spaced repetition
```

**Rationale**: Cytoscape.js graph explorer is future; plain text is the lowest-fidelity display that conveys the chain. Enter on a structural result item navigates to that node's source line (if enrichable); non-enrichable items are skip-navigable.

---

## 8. Modifier Pass-Through: Exact-Match and Accent-Insensitive (T027 Verification)

**Finding**: `DecodeSearchField` parses `!term!`, `|term|`, and `(term)` tokens, but their enforcement is backend-specific.

### Exact-match (`!term!` / `|term|`)

`IsExactMatch(s string) (bool, string)` strips the delimiters and lowercases the term — the cleaned string is placed in `params.Name`. However, `SearchNodesByName` in BadgerDB (`kv_badger.go:448`) performs a **case-insensitive substring match** (`strings.Contains(text, query)`) regardless of whether the original token had delimiters. The `!…!` / `|…|` exact-match semantic is implemented in the PostgreSQL backend via `tsvector` exact matching, not in the BadgerDB path.

**Consequence for v0.1.0**: Exact-match delimiters are accepted without error (delimiters are stripped) but the result set is the same as a plain substring query. No user-visible breakage — results may include more matches than strict exact-match would produce.

### Accent-insensitive (`(term)`)

`UnParen(s)` detects the `(…)` wrapping and strips it, passing the bare term to the text index scan. The PostgreSQL backend uses `sst_unaccent()` (a custom unaccent function) for this. The BadgerDB index stores node text as-is (no normalization). Querying `(fangzi)` will find nodes containing the literal substring `fangzi`; accented variants (`fāngzi`, etc.) will not match unless the node text was stored without accents.

**Consequence for v0.1.0**: Parentheses are silently stripped; query behaves as a plain substring search. No error; accent-insensitive broadening is not performed.

**Decision**: No workaround applied — this is a v0.1.0 SSTorytime-KV constraint. Both modifier types pass through cleanly (no panics, no errors) and degrade gracefully to substring search. Document in user-facing help when a `\help` command is added.
