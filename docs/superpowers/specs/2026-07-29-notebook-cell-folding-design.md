# Notebook cell folding

**Date:** 2026-07-29
**Status:** Approved design
**Feature area:** Processing Notebook — cell chrome

## Summary

Fold a stacked cell from its header. A folded cell hides its body and shows one
muted line of content-derived preview text under the name, so the cell stays
identifiable while taking three lines instead of three hundred. The notebook
header gains a Fold all / Unfold all toggle. Fold state persists with the
notebook.

## Motivation

A notebook with a sketch, two scans and a long tutor transcript is taller than
several screens, and reaching the cell you want means scrolling past everything
you don't. The cell-headers slice named every cell; the chrome-consolidation
slice put reorder and delete in that header. Both listed folding as out of
scope — the cell-headers design at line 144, the chrome-consolidation design at
line 158. This is that follow-up.

## Grounding in the current implementation

Surveyed before designing:

- **The header already exists** as `.nb-cellhead` in `NotebookPane.svelte`, a
  flex row of `cellLabel(cell)` · spacer · `▲ ▼` · `✕`, with a single
  `{#if !readOnly}` around the three controls.
- **Several cells keep real state outside the saved `cell` object**, reachable
  only while the component is mounted:

  | component | component-only state | `onDestroy` |
  |---|---|---|
  | `ComputeCell` | `logs`, `value`, `error`, `chartConfigs` — all of its output | `run?.cancel()` |
  | `LLMCell` | `partial` (streaming text), `input` (typed, unsent) | `turn?.cancel()` |
  | `AudioCell` | `rec` (`MediaRecorder`), `stream`, `elapsed` | stops all mic tracks |

  `ComputeCell` is the sharpest case: `chartConfigs` is populated by a run and
  never written to `cell`, so unmounting a Compute cell discards its results.
- **Assets are content-hashed.** `internal/media/import.go:48-50` stores every
  imported file as `sha256hex + ext` and discards the original name, so an image
  cell's `asset` carries no human-readable identity.
- **`AudioCell.svelte:134` has a private `fmt(ms)`** producing `m:ss`.
- **Cells are saved as one opaque JSON blob** —
  `SaveNotebook(nbId, JSON.stringify(cells))` — so a new per-cell field needs no
  schema change and no migration.
- **`scheduleSave()` already early-returns when `readOnly`**
  (`NotebookPane.svelte:35`).
- **`SketchCell.svelte:31` initialises `strokes` from `cell.strokes` once**, in a
  plain `let`, not under `$:`.
- **There is still no Svelte component test harness.** `frontend/src/lib/*.test.js`
  are pure-JS modules under Node's test runner; `cell-label.js`'s header comment
  records why components are out of reach.

## Decisions (from brainstorming)

- **A folded cell shows a one-line preview, not just its name.** Header-only is
  tighter, but three sketch cells all reading `SKETCH` are indistinguishable —
  precisely the confusion the headers were added to remove. A clipped peek of
  the real rendering keeps visual types recognisable but requires the cell to
  lay out at full cost, which is what folding is trying to avoid.
- **Folded cells stay mounted and are hidden with CSS, not unmounted.** Folding
  is a view state and must never destroy work. Unmounting would erase a Compute
  cell's output, cancel an in-flight LLM stream along with any typed-but-unsent
  input, and stop an active recording. Guarding against that would need a busy
  signal from all eight cell components into `NotebookPane` — real new coupling
  across the whole cell surface — and would still lose the Compute output in the
  idle case. The cost accepted in exchange: a folded notebook costs the same to
  render as an unfolded one. **This feature buys readability, not speed.**
- **Fold state persists on the cell** as `cell.folded`. The notebook worth
  folding is the long one you return to, and losing your folds on every reopen
  would defeat that. An absent key is falsy, so existing notebooks open
  unfolded.
- **A Fold all / Unfold all toggle in the notebook header.** Open a long
  notebook, fold everything, unfold the two cells you came for — that is the
  skim workflow the feature is for, and per-cell-only makes it thirty clicks.
- **The fold toggle is visible in read-only.** The other three header controls
  are hidden there because they mutate the document. Folding changes what you
  are looking at, not what the notebook means, and a read-only notebook is the
  one you are reading rather than editing.
- **The image preview is geometry, not a filename.** Content-hashing leaves an
  image cell with no identifying text; `naturalW×naturalH` is the only honest
  thing it holds.

## Architecture

### `frontend/src/lib/cell-preview.js` (new)

Exports `cellPreview(cell)` and `formatDuration(ms)`. Pure and
dependency-free — the same constraint recorded in `cell-label.js`: `notebook.js`
imports nine `.svelte` files for `cellRegistry`, which Node's test runner cannot
parse, so nothing reachable from it is unit-testable.

A separate module from `cell-label.js` rather than a second export there. The
two answer different questions from different data: `cellLabel` reads
`type`/`mode`/`kind` and always returns something; `cellPreview` reads eight
different content fields and is expected to return `''`.

| type | preview source | example |
|---|---|---|
| `n4l` | first non-empty line of `cell.text` | `Ovid (wrote) Metamorphoses` |
| `llm` | first `role: 'user'` message, then `· N msgs` | `"explain the aorist tense…" · 6 msgs` |
| `graph` | `cell.query` | `Ovid \| +2` |
| `compute` | first non-empty line of `cell.code` | `print(math.sqrt(16))` |
| `sketch` | `cell.strokes.length`, singular at 1 | `47 strokes` |
| `image` | `naturalW×naturalH` | `1024×768` |
| `audio` | `formatDuration(cell.durationMs)` | `0:14` |
| `media` | `cell.title` \|\| `cell.subject` \|\| `cell.query` | `Aeneid Book VI reading` |

- **Empty means empty.** Any cell whose preview resolves to `''` — a fresh
  sketch (0 strokes), an unqueried graph cell, an image whose `naturalW` is
  still 0 — renders **no preview element at all**. A `(empty)` placeholder would
  be noise on exactly the cells carrying no information.
- **Unknown types return `''`.** Unlike `cellLabel`, which falls back to the raw
  type string so a header is never blank, there is no sensible content guess for
  a type this module does not know.
- **Truncation is split across two layers.** `cellPreview` caps its return at
  120 characters; CSS does the visible ellipsis at the pane's real width. The JS
  cap is not cosmetic — it stops a 40 KB draft's first line being written into
  the DOM for a cell you folded specifically to stop looking at. CSS handles the
  rest because only the browser knows the current pane width.

`formatDuration(ms)` is `AudioCell`'s `fmt` moved here, and `AudioCell` imports
it. The header preview and the `.au-dur` chip render the same cell's duration a
few pixels apart; two private copies could drift into showing two different
numbers for one recording. Moving it also gives the formatter the test coverage
it has none of today.

### `frontend/src/components/notebook/NotebookPane.svelte`

**Per-cell toggle.**

```js
function toggleFold(i) {
  cells[i].folded = !cells[i].folded
  cells = cells
  scheduleSave()          // no-ops when readOnly
}
```

**The header row** becomes a disclosure button plus the existing controls:

```
▸ SKETCH                          ▲ ▼    ✕
  47 strokes
```

- The **chevron and the name are one `<button>`**, on the left. Left is the
  disclosure convention, and it puts the new frequently-clicked control at the
  opposite end of the strip from `✕` — the mis-click protection the
  consolidation slice bought with the `.nb-del` margin (comment at
  `NotebookPane.svelte:139-143`) is not spent. The name is part of the hit
  target because this app supports pen input as a first-class case
  (`SketchCell.accept()` dispatches on `pointerType`; the existing comment notes
  pen gets no hover preview to catch an aim error), and a 9px chevron is a poor
  pen target. Carries `aria-expanded`.
- **`▸` folded, `▾` unfolded** — plain glyphs, matching `▲▼ ✕ ⚙`. Per-type icons
  remain out of scope.
- **Outside the `{#if !readOnly}` guard**, per the decision above. A read-only
  header is `▸ SKETCH` and nothing else.

**Markup.** `{@const}` is available (Svelte `^3.49.0`; added in 3.46) and valid
as a direct child of `{#each}`:

```svelte
{#each cells as cell, i (cell.id)}
  {@const preview = cell.folded ? cellPreview(cell) : ''}
  <div class="nb-cellwrap">
    <div class="nb-cellhead">…fold button…{#if !readOnly}…▲▼ ✕…{/if}</div>
    {#if preview}<div class="nb-preview">{preview}</div>{/if}
    <div class="nb-cellbody" class:nb-folded={cell.folded}>
      <svelte:component … />
    </div>
  </div>
{/each}
```

The `.nb-cellbody` wrapper is new and unavoidable: `class:` cannot reach through
`svelte:component` unless every cell forwards a class prop. It is a plain block
div, and `.nb-cellwrap` stopped being a flex container in the consolidation
slice, so it is layout-neutral.

**Fold all.** In `.nb-head`, beside `⚙ Models`, labelled by what it will do:

```js
$: allFolded = cells.length > 0 && cells.every((c) => c.folded)
function foldAll() {
  const v = !allFolded
  for (const c of cells) c.folded = v
  cells = cells
  scheduleSave()
}
```

Mutating in place rather than mapping to spread copies **matters because cells
stay mounted**. Components hold non-reactive references into their prop —
`SketchCell.svelte:31` initialises `strokes` from `cell.strokes` exactly once,
not under `$:`. Replacing every cell object underneath a stack of live
components is a class of bug worth not opening at all. It is also the same shape as
`toggleFold`. The keyed `{#each … (cell.id)}` sees unchanged ids either way, so
nothing remounts.

Shown in read-only too, where it works and simply does not persist.

### CSS

- `.nb-fold` — the disclosure button. Needs `font`, `color`, `letter-spacing`
  and `text-transform` set to `inherit`, since buttons do not inherit
  typography from `.nb-cellhead`. `background: none; border: none`, `min-width:
  0` so the name can still ellipsis. Hover to `#7dd3fc`, matching
  `.nb-reorder button:hover`.
- `.nb-cellname` keeps its existing `overflow: hidden; text-overflow: ellipsis;
  white-space: nowrap`.
- `.nb-preview` — **styled as content, not chrome.** `.nb-cellhead` is uppercase
  and letter-spaced because it names a type; the preview quotes what the user
  wrote, so it inherits none of that. Original case, no tracking, smaller and
  dimmer than the name (`#46586a`), indented to align under the name past the
  chevron, single-line with `text-overflow: ellipsis`.
- `.nb-folded` — `display: none`.

## The guard this feature depends on

`SketchCell.onResize()` (`SketchCell.svelte:69-76`) persists the wrapper's
measured height into `cell.height` and saves it. Under `display: none`,
`clientHeight` reads **0**, and the only thing preventing a zeroed height from
being written and saved for every folded sketch is the truthiness test in
`if (h && h !== cell.height)`.

That guard exists today for unrelated reasons and becomes load-bearing here. It
gets a comment saying so, and a case in the manual test plan. The behaviour is
self-healing: on unfold the observer fires again with a real height and
re-renders.

## Consequences

- **A folded notebook costs the same to render as an unfolded one.** Accepted
  deliberately: folding buys readability, not speed. If a notebook ever grows
  large enough that mount cost, rather than height, is the problem, the fix is
  selective unmounting of the types with no component-only state — a different
  feature with a different justification.
- **Fold toggles dirty the notebook** and schedule a save, so folding a cell
  updates the file's mtime. Consistent with every other cell edit.
- **Every stacked cell now has a body wrapper div.** Anything later reaching for
  a cell's DOM from `NotebookPane` sees one more level. `ContextCell` is
  unaffected — it is rendered above the `{#each}` and gains neither a header nor
  a wrapper.

## Testing

**Unit** — `frontend/src/lib/cell-preview.test.js`, under the existing Node
runner:

- each of the eight types produces its documented preview;
- the empty case for every type that has one (blank `text`, no messages, empty
  `query`, empty `code`, 0 strokes, `naturalW` 0, `durationMs` 0, media with
  none of title/subject/query) returns `''`;
- `sketch` is singular at 1 stroke, plural otherwise;
- `media` prefers `title`, then `subject`, then `query`;
- `llm` picks the first **user** message, not the first message, and appends the
  total message count;
- multi-line `text`/`code` yields the first **non-empty** line, leading blank
  lines skipped;
- an over-length line is capped at 120 characters;
- an unknown type and a null/type-less cell return `''`;
- `formatDuration` — 0, sub-minute zero-padding (`0:07`), exactly a minute
  (`1:00`), over ten minutes, and a non-finite/undefined input.

**Manual** — its own test-plan document, matching prior slices:

- every one of the eight stacked types folds and unfolds from its header, and
  shows the documented preview when folded;
- a fresh cell of each type folds to header-only, with no preview line and no
  empty stub;
- fold state survives closing and reopening the notebook, and a restart;
- a read-only notebook shows `▸ NAME` and no other header control; folding works
  there and is not persisted;
- Fold all folds every cell and relabels itself Unfold all; Unfold all reverses
  it; the label is correct on a notebook opened with all cells already folded;
- **nothing is destroyed by folding:** run a Compute cell that emits a chart,
  fold it, unfold it — logs, value and chart are still there; start an LLM
  answer, fold mid-stream, unfold — the stream is still running and completes;
  type into an LLM input without sending, fold, unfold — the text is still
  there; start a recording, fold, unfold — still recording, and the duration is
  correct on stop;
- fold a sketch, unfold it — the drawing returns at its original height, not
  collapsed; restart and confirm the saved height is unchanged;
- reorder and delete still work on a folded cell, from its header;
- an existing notebook saved before this change opens with every cell unfolded.

## Out of scope

- Undo for delete (still).
- Per-type icons (still).
- Drag-to-reorder (still).
- Auto-folding on open — by cell count, height, or age.
- Unmounting folded cells for performance.
- Folding the `ContextCell`, which is rendered above the stack and has no header.
- A keyboard shortcut for fold/unfold.
