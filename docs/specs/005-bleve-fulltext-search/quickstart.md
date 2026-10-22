# Quickstart: Bleve full-text search

**Feature**: 005-bleve-fulltext-search
**Date**: 2026-04-25
**Audience**: Developers implementing or verifying this feature.

This document walks through the steps to get the Bleve-backed search working
end-to-end on a developer machine, and the manual flows to exercise once the
code is in place. It is not a substitute for the unit/integration tests in
`research.md § R10`; it is the human-in-the-loop sanity check.

---

## 1. Repository layout assumption

Both repositories are checked out side by side:

```text
~/projects/
├── sstorytime-kv/        # github.com/srosignoli/sstorytime-kv
└── texere/               # this repo
```

During development, point Texere's `go.mod` at the local SSTorytime-KV checkout
with a `replace` directive:

```text
// go.mod (Texere)
replace github.com/srosignoli/sstorytime-kv => ../sstorytime-kv
```

Remove the `replace` line and bump the pinned version to `v0.4.0` once the
library is published.

---

## 2. SSTorytime-KV side: add Bleve

```bash
cd ~/projects/sstorytime-kv

# Add deps (pure Go — no CGo, no cross-compile tooling required)
go get github.com/blevesearch/bleve/v2@latest
go get github.com/blevesearch/bleve/v2/analysis/lang/cjk
go get github.com/blevesearch/bleve/v2/analysis/token/asciifolding

# Verify pure-Go (constitution principle II)
go list -deps -f '{{.ImportPath}} {{.CgoFiles}}' ./pkg/SSTorytime/... | grep -v '\[\]' || echo "all pure Go"

# Run tests
go test ./pkg/SSTorytime/...
```

New files:

- `pkg/SSTorytime/bleve_index.go` — `Index`, `IndexBatch`, lifecycle, search.
- `pkg/SSTorytime/bleve_translate.go` — operator-grammar parser + lowering.
- `pkg/SSTorytime/bleve_translate_test.go` — translator unit tests.
- `pkg/SSTorytime/bleve_index_test.go` — round-trip index/search tests.

Modified files:

- `pkg/SSTorytime/kv_badger.go` — `SearchNodesByName` deprecation wrapper.
- `pkg/SSTorytime/store_indexes.go` — `GetDBNodePtrMatchingNCCS` deprecation wrapper.

Tag and publish once tests are green:

```bash
git tag v0.4.0
git push origin v0.4.0
```

---

## 3. Texere side: wire it in

```bash
cd ~/projects/texere

# Pick up local SSTorytime-KV via the replace directive (or pin v0.4.0 once published)
go mod tidy

# Build & test
make test
make build
```

Modified files:

- `internal/graph/builder.go` — open `Index` alongside BadgerDB; call `Add` per
  node in Pass 1; `Close` after Pass 2.
- `internal/search/search.go` — `SearchService` gains `blevePath` and
  `bleveIdx`; `searchNodes` calls `SearchByQuery` instead of `IterateNodes`;
  `IsReady` checks both `sst_data/` and `bleve/`.
- `internal/search/search_test.go`, `internal/search/unit_test.go` — Bleve-path
  coverage and operator-grammar regressions.
- `frontend/src/components/OmniBar.svelte` — toast string update for the
  missing-index case (FR-022).
- `.gitignore` — add `bleve/` and `*.bleve/`.

---

## 4. First-run developer flow

```bash
# Start the dev server
make dev
```

Then in the running app:

1. **Open a collection** with at least one `.n4l` file containing English and
   CJK text (use the seed collection at `testdata/collections/mixed/` if it
   exists, otherwise create one with the editor).
2. **Trigger Build graph** (`Ctrl+G`). Verify the spinner completes and the
   status bar reports both `sst_data/` and `bleve/` written. Inspect on disk:

   ```bash
   ls -la <collectionDir>/
   # expect both: sst_data/   bleve/
   du -sh <collectionDir>/bleve/
   # expect well under 50 MB for collections under 5 000 nodes (SC-009)
   ```

3. **Open the search overlay** (`Ctrl+K` / `Ctrl+Space` per the existing
   binding) and run each of the queries below in turn. Each should return ≥1
   hit on a properly seeded collection.

---

## 5. Operator-grammar smoke matrix

Run these queries in the search overlay against the seeded mixed-language
collection. The expected result column says what to look for in the
result list — exact node IDs depend on your seed data.

| # | Query | Expects |
|---|---|---|
| 1 | `running` | Stems to `run` — also matches nodes containing `runs`, `runner`. SC-005 (English). |
| 2 | `房子` | CJK bigram match. SC-005 (CJK). |
| 3 | `"fish soup"` | Phrase match: only nodes with the exact two-word sequence. |
| 4 | `!Apple!` | Exact-text match. Only nodes whose entire text is "Apple" (case-insensitive). |
| 5 | <code>&#124;Apple&#124;</code> | Same as `!Apple!` (alternative syntax). |
| 6 | `(fangzi)` | Accent-insensitive: matches both `fangzi` and `fángzi`. SC-007. |
| 7 | `flo:*` | Prefix: matches `flow`, `flotation`, `floor`. |
| 8 | `strange<->kind` | Adjacent words: matches `strange kind` but not `strange of kind`. |
| 9 | `strange<2>woman` | Within 2 words: matches `strange wonderful woman`. |
| 10 | `brain & notes` | Both terms required (whitespace OK around `&`). |
| 11 | `brain&!notes` | `brain` present, `notes` absent. |
| 12 | `apple \| pear` | Either term. |
| 13 | `chemistry C & S reaction` | All four tokens default-OR — `C` and `S` are literals because of whitespace around `&` (FR-015a). |

### Negative cases

| # | Query | Expects |
|---|---|---|
| N1 | `"fish soup` | Toast: `parse error at position 0: unclosed quote` (FR-015b). |
| N2 | `brain&` | Toast: `parse error at position 6: operator '&' missing right operand`. |
| N3 | `strange<-1>kind` | Toast: `parse error at position 7: proximity slop must be non-negative`. |

### `\` command regressions

These must continue to work (US3 / FR-005). Spot-check a few:

| # | Query | Expects |
|---|---|---|
| C1 | `\paths from=apple to=pear` | Path resolution unaffected — `apple`/`pear` resolve via Bleve seed lookup, then existing path walk runs. |
| C2 | `\sequence start=intro` | Sequence walk via existing BadgerDB code path; no change. |
| C3 | `\chapter biology` | Chapter listing path unaffected. |
| C4 | `\notes about=mitosis` | Notes panel populates. |

---

## 6. Missing-index recovery

Verify the Q4 UX:

```bash
# Simulate a missing index (with the app closed)
rm -rf <collectionDir>/bleve/
```

Re-open the app. Type any text query into the search overlay. Expect a toast:

> Search index missing for this collection. Run Build graph (Ctrl+G) to create it.

Hit `Ctrl+G`, watch it rebuild, retry the query — should now return hits.

---

## 7. Performance sanity

```bash
# After a build on a 5 000-node collection
time make dev   # observe Build graph elapsed in the status bar
```

Acceptance:

- Build graph total time ≤ pre-Bleve baseline + 200 ms (SC-008). Use a tagged
  baseline build (pre-feature) for comparison.
- A handful of overlay queries against the same collection should return in
  well under 100 ms each (no constitutional budget for this; chosen to stay
  comfortably inside the existing Tier-2 search target from feature 004).
- `du -sh <collectionDir>/bleve/` ≤ 50 MB at 5 000 nodes (SC-009).

---

## 8. Cross-platform verification

```bash
# Linux amd64
make build

# Windows amd64 (cross-compile)
make build-windows

# Sanity check: both binaries produce a single self-contained file
file build/bin/texere build/bin/texere.exe
```

The `bleve/` directory created on Linux is portable to Windows and vice versa
(Bleve's Scorch format is endian-neutral; verified in `research.md § R2`).

---

## 9. What "done" looks like

- All translator unit tests pass (`go test ./pkg/SSTorytime/...`).
- Texere `searchNodes` and `searchStructural` tests pass with the new code path.
- All 13 queries in §5 return sensible results on the seeded collection.
- All three negative cases in §5 surface a toast with a position hint.
- All four `\`-command regressions in §5 still work.
- `make build` and `make build-windows` succeed.
- Build graph elapsed time on a 5 000-node collection within SC-008's budget.
- `bleve/` is git-ignored in both fresh clones.
