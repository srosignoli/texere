# Contract: Wails IPC — Explorer Layout

**Date**: 2026-05-04
**Branch**: `006-isometric-graph-explorer`

This document defines the three Wails-bound methods this feature adds to `App` (in `app.go`), their semantics, error model, and the lifecycle of the `ResultSet` they manipulate. JSON shapes are formalized in [layout-types.md](./layout-types.md).

All methods are bound by Wails so the frontend invokes them as TypeScript functions on `window.go.main.App`. Names match the Go method names.

---

## 1. `ExplorerOpen(query string) (LayoutResult, error)`

**Purpose**: Run a graph query against SSTorytime-KV (delegating to feature 004's existing pipeline), cache the materialized subgraph as the active `ResultSet`, compute the initial layout for the default anchor (FR-008), and return the rendered scene.

**Inputs**:
- `query` — the same query string format accepted by feature 004's `SearchGraph`. Examples: `"chapter brain"`, `"\\context restaurant"`, `"supermarket"`.

**Behavior**:
1. If the SSTorytime-KV graph has not been built or the search service is not ready: return a `LayoutResult` with empty arrays and `Diagnostics.GraphError = "Build your graph first (Ctrl+G) to enable graph search."`. No error is returned at the IPC level — the frontend renders the empty state with the message.
2. Otherwise, run the query, materialize a `ResultSet`, evict any prior cached `ResultSet`, choose the default anchor per FR-008, run a `LayoutPass`, and return a `LayoutResult`.
3. If `len(query)` is zero or whitespace-only: return an `IPC error` (`"empty query"`) — this is a programmer error, not a user-visible state.
4. If the result set exceeds the ceiling (500 nodes / 1500 edges): clamp deterministically (lowest-NodePtr-first eviction), set `Diagnostics.Clamped = true`, and populate `Diagnostics.Truncated` with the dropped count.

**Outputs**: `LayoutResult` JSON (see layout-types.md).

**Side effects**:
- Replaces the active `ResultSet` cache.
- Logs the query, layout time, and clamp counts to the existing app log.

**Latency budget**: ≤2 s end-to-end including the BadgerDB query, including the >500-node clamping path.

---

## 2. `ExplorerReanchor(resultID string, anchorID string) (LayoutResult, error)`

**Purpose**: Re-run the layout pass against the cached `ResultSet` with a new anchor — no BadgerDB query, no re-classification.

**Inputs**:
- `resultID` — the opaque ID returned by `ExplorerOpen`.
- `anchorID` — stringified `SST.NodePtr` of a node already in that result.

**Behavior**:
1. If `resultID` does not match the active `ResultSet.ID`: return an IPC error `"stale resultId"`. The frontend treats this as a signal to call `ExplorerOpen` afresh.
2. If `anchorID` is not present in the result: return an IPC error `"anchor not in result"`. (Frontend should never send this in normal flow because click targets are restricted to rendered nodes; this is a defense-in-depth check.)
3. If `anchorID` is the current anchor: short-circuit and return a `LayoutResult` whose `Diagnostics.LayoutMs == 0` and whose positions are byte-identical to the prior pass. (This makes FR-025a a Go-level guarantee, not just a frontend convention.)
4. Otherwise: run a `LayoutPass` and return the resulting `LayoutResult`.

**Outputs**: `LayoutResult` JSON.

**Side effects**: None (the cached `ResultSet` is unchanged; only the layout transform is recomputed).

**Latency budget**: ≤100 ms compute. The 1 s end-to-end re-anchor budget (SC-010) accommodates ~900 ms of frontend animation.

---

## 3. `ExplorerClose() error`

**Purpose**: Drop the active `ResultSet` cache and any associated state.

**Inputs**: none.

**Behavior**:
1. If no active result: no-op, return nil.
2. Otherwise: clear the cache.

**Outputs**: nil error on success.

**Side effects**: Frees memory held by the cached subgraph.

**Latency budget**: <5 ms.

---

## 4. Error model

All three methods return Go errors only for **programmer-error** conditions (empty query, stale ID, anchor not in result). User-visible failure modes (graph not built, query returned nothing, ceiling exceeded) are **non-error** outcomes communicated through `LayoutResult.Diagnostics`. The frontend renders accordingly (empty state, hint banner, truncation badge).

This split keeps the JS-side code path small: `await ExplorerOpen(q)` always returns a renderable `LayoutResult` in the normal case; thrown errors indicate frontend bugs.

---

## 5. Concurrency

The `ExplorerService` (in `internal/explorer/service.go`) protects its single-result cache with a mutex. Concurrent IPC calls from the frontend are serialized at the service level — there is no use case for parallel layout passes within a single Wails app. Layout itself is pure-functional and lock-free; only the cache slot is guarded.

---

## 6. Versioning

The `LayoutResult` JSON shape is owned by this feature. Adding fields is backward-compatible (frontend tolerates unknown fields). Renaming or removing fields is a breaking change and requires a frontend update in the same PR.

There is no API version field; binary compatibility is enforced by the build system (Wails ships frontend and backend together).

---

## 7. Lifecycle diagram

```
                        +-----------------------+
                        |       no result       |
                        +-----------------------+
                                |   ^
                  ExplorerOpen  |   |  ExplorerClose
                                v   |
                        +-----------------------+
                        |   active ResultSet    |
                        | (one cached subgraph) |
                        +-----------------------+
                          |    ^             ^
                          |    | ExplorerOpen|
            ExplorerReanchor   | (new query) | ExplorerOpen
            (same resultID)    |             | (same query — still evicts old result)
                          v    |             |
                        same active result, new layout pass
```

`ExplorerOpen` always evicts; the explorer never juggles multiple cached results. Anchor history is a frontend concern (see [data-model.md §2.5](../data-model.md#25-anchor-history-frontend-only)).
