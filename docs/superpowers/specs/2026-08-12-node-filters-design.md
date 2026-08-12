# Node column filters

**Date:** 2026-08-12
**Status:** Proposed — awaiting approval
**Feature area:** Workbench → nodes column

## Summary

The nodes column's quick-filter bar goes from three filters to five —
**All · Text · Linked · Extracts · Cards** — and the predicates behind them
start meaning what their labels say.

## Motivation

Today's bar offers All, Extracts and Structured. Two of the three are wrong:

- **"Extracts" does not select extracts.** `ColumnNodes.svelte:19` defines it as
  `!hasArrows(node) && !node.StudyMeta` — it never looks at `# @id`. Every
  ordinary prose line that was never extracted appears in it, and a genuine
  extract carrying an arrow does not.
- **"Structured" conflates two different things** — `hasArrows(node) ||
  !!node.StudyMeta`. A `# @card` node with no arrows is not structured in any
  N4L sense, but it lands there.

There is also no way to ask the two questions the collection is actually
organised around: *what have I extracted?* and *what have I turned into cards?*

## Grounding in the current implementation

- **`Node` carries everything needed** (`internal/content/types.go:22-35`):
  `ID` is `""` unless a `# @id` marker precedes the node; `StudyMeta` is `nil`
  unless the node is studyable; `Arrows` and `Dittos` hold the N4L structure.
  No parser change is required.
- **`StudyMeta` is not only `# @card`.** `# @cloze` and `# @study` set it too
  (`internal/content/parser.go:388-412`), as do study blocks (`:1069`).
- **`isStructured` does double duty.** It is the filter predicate *and* the
  render-mode switch at `ColumnNodes.svelte:161` and `:209`, choosing between a
  collapsible arrow-list row and the flat quote-style row.
- **The predicates live inside the component**, where this repo's `node --test`
  cannot reach them — which is how the "Extracts" mislabel survived.
- **`quickFilter` is local component state** and is referenced nowhere else in
  the frontend. The active filter already resets on restart.

## Decisions

- **Five filters: All, Text, Linked, Extracts, Cards.**
- **They are overlapping lenses, not exclusive buckets.** Each button asks an
  independent question, so a card that carries arrows appears under both Linked
  and Cards, and an extract with no arrows appears under both Text and Extracts.
  Text describes a node's *structure*; Extracts describes its *provenance*; they
  answer different questions and a node can honestly be both. Rejected: a
  precedence order making the counts sum to All, which would hide a card with
  arrows from the Linked lens — the one place someone looking for arrows would
  expect to find it.
- **"Text" for prose, "Linked" for arrows.** `Linked` matches the `+link` button
  already in the toolbar: the same word for the same concept in two places.
  Rejected: keeping "Structured" for the narrower meaning, which would carry the
  old expectation across the change.
- **Cards means "has a study marker", not "has `# @id` and `# @card`".**
  Promotion injects an `@id`, but a hand-written `# @card` need not have one,
  and under the literal rule such a node would appear in *no* filter at all —
  invisible in every lens. Extracts keeps `@id` in its definition, because an
  `@id` is what makes something an extract.
- **The rendering does not change.** `isStructured` splits: the filter takes the
  narrow `hasArrows` meaning, while the row renderer moves to `hasDetail`, which
  keeps today's exact `hasArrows || StudyMeta` behaviour. A card with no arrows
  still renders as a collapsible row.
- **The filter list is data, not markup.** One table drives both the buttons and
  the filtering, so the two cannot disagree about what a filter means.

## Architecture

### `frontend/src/lib/node-filters.js` (new)

Pure and dependency-free — no Svelte, no stores — so it runs under
`node --test`, the only automated coverage available for frontend logic here.

```
hasArrows(node) -> boolean   // an arrow on the node or on any ditto
isText(node)    -> boolean   // no arrows, no study marker
isLinked(node)  -> boolean   // at least one arrow
isExtract(node) -> boolean   // has # @id, no study marker
isCard(node)    -> boolean   // has a study marker
hasDetail(node) -> boolean   // render switch: arrows or a study marker
NODE_FILTERS    -> [{ id, label, match }]
```

`NODE_FILTERS` is ordered as the bar reads: `all`, `text`, `linked`,
`extracts`, `cards`. `all`'s `match` accepts everything, so the caller filters
unconditionally and never special-cases it.

`hasDetail` lives here rather than in the component precisely because it is not
a filter: pinning it in tests is what stops it drifting back into the filter
predicate it used to share.

### `frontend/src/components/ColumnNodes.svelte`

- Deletes the local `hasArrows`, `isExtract` and `isStructured`; imports from
  the lib.
- The filter bar renders from `NODE_FILTERS` instead of three hand-written
  buttons.
- `quickFiltered` looks the active filter up in the table and applies its
  `match`, falling back to `all` if `quickFilter` somehow names no filter.
- `:161` and `:209` call `hasDetail` where they called `isStructured`.

## Error handling

| Case | Behaviour |
|---|---|
| A node missing `Arrows`/`Dittos` entirely | Treated as having none; no throw |
| `quickFilter` names no known filter | Falls back to `all` rather than crashing on `undefined.match` |
| A filter matches nothing | The existing "No nodes" hint, unchanged |
| A study marker with no `# @id` | Appears under Cards — the case the literal rule would have hidden |

## Testing

**JS** (`node --test`), against node shapes built to mirror the parser's output:
each predicate over a plain node, a node with arrows on the root, a node with
arrows only on a ditto, an `@id` node with and without a study marker, a study
node without an `@id`, and a node carrying arrows *and* a study marker. Plus:
`NODE_FILTERS` ids are unique and its `all` entry matches everything; and
`hasDetail` still accepts exactly what the old `isStructured` accepted, which is
the regression this split could cause.

**Manual:** its own test-plan document, run against the real collection — each
of the five filters, the overlap cases stated above, and confirmation that no
row's rendering changed.

## Out of scope

- Persisting the active filter across restarts.
- Counts or badges on the filter buttons.
- Filtering anywhere but the nodes column.
- Any change to the parser, to `# @id`/`# @card` semantics, or to promotion.
- The `+ Card` and `💬` hover actions, which are still `console.log` stubs.
