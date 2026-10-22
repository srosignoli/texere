# Notebook cell type headers

**Date:** 2026-07-28
**Status:** Approved design
**Feature area:** Processing Notebook — cell chrome

## Summary

Give every cell in the notebook stack a slim header naming its type, rendered
once by `NotebookPane`'s per-cell wrapper rather than by the nine cell
components. Where a type has meaningful variants, the header appends a detail —
`Media · video`, `Compute · js`, `LLM · tutor`.

## Motivation

Once cells stack up, telling them apart is hard. Only two of the nine types
identify themselves today, and they do it inconsistently.

## Grounding in the current implementation

Surveyed before designing:

- **Two cells already name themselves**, not one: `MediaCell` shows `Media`
  (`me-label`) and `AudioCell` shows `🎙 Audio` (`au-title`). The rest show only
  controls, or nothing.
- **Three cells' toolbars are gated on `!readOnly`** — `N4LDraftCell`,
  `ImageCell` and `SketchCell` render their bar inside `{#if !readOnly}`. In a
  read-only notebook those cells have **no** bar at all, so a label placed
  inside one would disappear exactly when the reader is browsing rather than
  editing.
- **`ContextCell` is not a stacked cell.** `NotebookPane` renders it once above
  the `{#each}` loop, so it is not part of the pile being scanned. Note `context`
  *does* appear in `cellRegistry`, but no factory creates one and none is ever
  pushed into `cells`; the entry is vestigial. Should one ever appear there, the
  unknown-type fallback below gives it a readable header rather than a blank
  one, so no special case is needed.
- `ComputeCell` already surfaces `js` in its own bar, establishing that a
  type-plus-detail label is natural here.

## Decisions (from brainstorming)

- **The header lives in `NotebookPane`'s wrapper, not in the cell components.**
  One implementation instead of nine; consistent by construction; immune to each
  cell's own `readOnly` gating; and a new cell type gets a header for free by
  adding a name beside its registry entry. The cells stay unaware of it.
- **A full-width strip above the cell**, rather than a badge in the left gutter
  or a chip on the border. The complaint is identification at a glance, and a
  strip is the only option that never requires decoding an abbreviation. The
  cost is roughly 20px of height per cell, accepted deliberately.
- **Type name plus a detail where the type has variants.** A plain type name
  would leave three stacked `Media` cells indistinguishable — a picture, a
  pronunciation and a video all reading `Media` — which is the very confusion
  this feature exists to remove. The detail costs nothing extra: it comes from
  data the cell already holds.
- **Names match the creation button.** The header for an `image` cell reads
  `Scan` because the button that creates it reads `+ Scan`. Same word for
  creating and for identifying.
- **The two existing in-cell labels are removed** so nothing is doubled.
- **Controls are not consolidated into the header.** Moving the delete `✕` or
  the `▲▼` reorder buttons in would touch all nine shipped components for a
  change that was not asked for, putting working cells at risk without
  addressing the actual annoyance. Left as a possible separate pass.

## Architecture

### `frontend/src/lib/notebook.js`

One exported pure function beside the existing `cellRegistry`, so the naming
table sits next to the type→component table it mirrors:

```js
cellLabel(cell) -> string   // e.g. "Media · video", "N4L draft"
```

Keeping it pure and outside the component makes it unit-testable without
mounting Svelte, matching how `buildMediaFact` and `mediaArrow` are handled.

| `cell.type` | header | detail source |
|---|---|---|
| `n4l` | `N4L draft` | — |
| `llm` | `LLM · <mode>` | `cell.mode` (`ask` \| `tutor`) |
| `graph` | `Graph` | — |
| `compute` | `Compute · <language>` | `cell.language` |
| `sketch` | `Sketch` | — |
| `image` | `Scan` | — |
| `audio` | `Audio` | — |
| `media` | `Media · <detail>` | see below |

**Media detail** is `cell.kind` (`image`, `audio`, `video`), except that an
audio media cell whose `provider` is `pronunciation` reads `pronunciation`
instead of `audio` — the more informative word, and the one a language learner
is actually scanning for.

**Fallbacks.** An unknown `type` yields the raw type string rather than an empty
header, so a future or hand-edited cell is still identifiable. A missing detail
field appends nothing rather than a dangling separator — notably `kind`, absent
on media cells saved before the audio slice.

### `frontend/src/components/notebook/NotebookPane.svelte`

A `.nb-cellhead` div inside the existing `.nb-cellwrap`, above `.nb-cell`,
rendering `{cellLabel(cell)}`. It sits outside every `readOnly` condition, so it
shows for readers as well as editors.

Styling follows the existing muted-chrome palette: small type, low-contrast
foreground, a quiet background distinct from the cell body so the strip reads as
chrome rather than content.

### Removals

- `MediaCell.svelte` — the `me-label` span (`Media`).
- `AudioCell.svelte` — the `au-title` span (`🎙 Audio`). Its duration chip stays.

No other cell component changes.

## Consequences

Cells that keep their own toolbar show two stacked bars: the muted name strip,
then their controls. This is the accepted cost of putting the label in the
wrapper rather than inside each cell; the strip is visually quieter, so the two
should read as chrome-then-controls rather than as competing bars. If it proves
noisy in use, the fix is the control consolidation listed as out of scope.

## Testing

**JS unit tests** for `cellLabel`, alongside the existing `lib/*.test.js`:

- every type's name, including that `image` reads `Scan`;
- the three detail cases — `LLM · tutor`, `Compute · js`, `Media · video`;
- a media cell of each kind, and the pronunciation special case;
- a media cell with no `kind` (saved before the audio slice) appending nothing
  rather than a dangling separator;
- an unknown type falling back to the raw type string.

**Manual:** the strip appears on every stacked cell, in both editable and
read-only notebooks; `ContextCell` has none; no cell shows a doubled name.

## Out of scope

- Moving delete or reorder controls into the header.
- Any header for `ContextCell`.
- Icons. `AudioCell`'s 🎙 is removed rather than generalised; adding a per-type
  icon set is a separate visual decision.
- Collapsing or folding cells from the header.
