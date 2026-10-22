# Notebook cell chrome consolidation

**Date:** 2026-07-28
**Status:** Approved design
**Feature area:** Processing Notebook — cell chrome

## Summary

Move the `▲▼` reorder gutter and the eight per-cell delete buttons into the
cell header strip added by the cell-headers slice. The header becomes a flex
row — name on the left, `▲ ▼` and `✕` on the right — and the left gutter
disappears, so every cell gains its width.

## Motivation

Every stacked cell currently has three zones of chrome: a reorder gutter to its
left, a name strip above it, and its own toolbar. Two of those hold one control
each. The cell-headers design listed this consolidation as the follow-up pass.

## Grounding in the current implementation

Surveyed before designing:

- **All eight stacked types carry a delete button**, each with its own class
  (`.del-btn`, `.del`, `.gq-del`, `.cc-del`, `.sk-del`, `.im-del`, `.au-del`,
  and `MediaCell`'s shared `.me-btn`), each right-aligned, each wrapped in its
  own `{#if !readOnly}`. Eight copies of one control.
- **"One bar instead of two" is not reachable.** Six types have real
  type-specific controls that must stay in their own bar — Sketch's pen and
  colour row, Image's rotate/crop, Graph's query input, LLM's mode and profile,
  Compute's Run, N4L's Push. What consolidation removes is the *third* zone, the
  gutter. Only Audio and Media drop to a single bar.
- **`.au-bar` and `.me-bar` both have padding and a `border-bottom`.** Removing
  their delete button empties them in states that are not corner cases: a
  freshly added `+ Audio` cell has no asset, so no duration chip and no Replace;
  a freshly added `+ Media` cell has no licence and no content. Both would render
  an ~11px stub with a rule under it.
- **`ComputeCell` shows `js` in a `.cc-lang` span**, which the header now also
  shows as `Compute · js`.
- **There is no Svelte component test harness.** `frontend/src/lib/*.test.js`
  are pure-JS modules under Node's test runner; the `cell-label.js` header
  comment records why components are out of its reach.

## Decisions (from brainstorming)

- **Controls always visible, not hover-revealed.** A hover-only strip is the
  quietest to read, but there is no hover state for the pen-tablet use this app
  explicitly supports — `SketchCell` has dedicated pointer-type handling for it.
- **`▲▼` stay as buttons; no drag-to-reorder.** Drag needs drop targets,
  autoscroll and a keyboard fallback that would mean keeping `▲▼` anyway. It is
  a feature, not a consolidation.
- **Delete is protected by separation only** — a gap between the `▲▼` pair and
  `✕`, plus the existing red hover. The two controls are neighbours for the
  first time and `deleteCell` has no undo, so the risk is real; but an undo
  toast is new retained state that wants its own slice, and a confirm taxes
  every deliberate delete of what is meant to be a disposable cell.
- **The header keeps its floating treatment** — no background, outside the cell
  border. The reasoning already recorded in `.nb-cellhead`'s comment still
  holds: a filled strip would have to match nine cells' own backgrounds, radii
  and borders to avoid looking detached. It would also put a filled bar directly
  above six cells' own filled bars, which is the competing-bars problem the
  cell-headers design flagged. These controls belong to the wrapper, not to the
  cell, and sitting outside the cell's border says so.

## Architecture

### `frontend/src/components/notebook/NotebookPane.svelte`

`.nb-cellhead` becomes a flex row:

```
{cellLabel(cell)}   ·spacer·   ▲ ▼    ✕
```

wrapped in a single `{#if !readOnly}` around the three buttons, replacing the
eight guards spread across the cell components and the one around the gutter.
The gap between the `▲▼` pair and `✕` is wider than the gap within the pair —
it is the whole of the mis-click protection, so it must be visible rather than
nominal.

- `✕` calls `deleteCell(i)` directly. The `onDelete` prop is no longer passed to
  `svelte:component`.
- `▲`/`▼` keep the existing `move(i, ±1)`, and are `disabled` on the first and
  last cell respectively. `move()` already no-ops at the ends; a dead button was
  forgivable at 9px in a gutter and is not once it is a prominent control.
- The `.nb-reorder` div and its styles are deleted. `.nb-cellwrap` stops being a
  flex container and `.nb-cell` loses its `flex: 1; min-width: 0`.
- Button styling reuses what the removed controls already used: the muted
  `#46586a` with a `#7dd3fc` hover from `.nb-reorder button`, and a `#ff8080`
  hover on `✕` as in every cell today.

### The eight cell components

Each loses its delete button, that button's CSS rule, and its now-unused
`export let onDelete`. Four also lose a spacer that no longer pushes anything:

| component | removed | spacer |
|---|---|---|
| `N4LDraftCell` | `.del-btn` | none — `margin-left: auto` went with the button |
| `LLMCell` | `.del` | `.spacer` removed |
| `GraphQueryCell` | `.gq-del` | none |
| `ComputeCell` | `.cc-del` | `.cc-spacer` **kept** — Run stays right-aligned |
| `SketchCell` | `.sk-del` | `.sk-spacer` removed |
| `ImageCell` | `.im-del` | `.im-spacer` removed |
| `AudioCell` | `.au-del` | `.au-spacer` **kept** — Replace stays right-aligned |
| `MediaCell` | the `.me-btn` delete | `.me-spacer` **kept** — Replace/Cancel stay right |

### Empty-bar suppression

- `AudioCell` — `.au-bar` is non-empty exactly when `cell.asset` is set, since
  both the duration chip and Replace require it. Wrapped in `{#if cell.asset}`.
- `MediaCell` — `.me-bar` is wrapped in
  `{#if cell.license || (!readOnly && hasContent(cell))}`. `hasContent` is
  already imported there from `media-fact.js` and already unit-tested.

No new predicate module: both conditions are a single expression over data the
component already holds.

### Removals

- `ComputeCell.svelte` — the `.cc-lang` span (`js`), now doubled by the header's
  `Compute · js`. Same doubling removed from `MediaCell` and `AudioCell` in the
  cell-headers slice.

## Consequences

Six types still show two rows of chrome — the floating name-and-controls strip,
then their own toolbar. That is unchanged by this pass; what goes is the gutter
and the eightfold duplication of one button.

Delete and reorder become adjacent for the first time. A mis-aimed `▼` that
lands on `✕` destroys a sketch or a transcribed scan with no undo, within the
600ms debounce. Mitigated by spacing only, deliberately. If it bites in use, the
fix is an undo toast as its own slice.

## Testing

No new pure logic, so no new unit tests. The only logic touched — `cellLabel`
and `hasContent` — is already covered by `cell-label.test.js` and
`media-fact.test.js`. Standing up a Svelte component harness for a markup move
would be a larger change than the feature.

**Manual** (its own test-plan document, matching prior slices):

- the header shows name plus `▲ ▼   ✕` on all eight stacked types; the gutter is
  gone and cells are visibly wider;
- a read-only notebook shows the name only, with no controls, on all eight;
- `▲` is disabled on the first cell and `▼` on the last; reorder works between;
- delete works from the header for each of the eight types;
- a fresh `+ Audio` and a fresh `+ Media` cell show no empty stub bar; once a
  recording exists the duration bar returns, and a licensed media asset still
  shows its licence;
- a Compute cell shows `js` once, in the header.

## Out of scope

- Undo for delete.
- Collapsing or folding cells from the header.
- Per-type icons.
- Drag-to-reorder.
