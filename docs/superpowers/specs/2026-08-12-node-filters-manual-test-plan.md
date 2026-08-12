# Node column quick filters — manual test plan (executed)

**Design:** `2026-08-12-node-filters-design.md`
**Build:** `make build && ./build/bin/texere` (built at commit `19510f6`)
**Comparison build:** commit `8046355` (the commit before Task 1), built in a
throwaway `git worktree` outside the repository and removed afterwards. `HEAD`
on the working checkout was never moved.
**Run date:** 2026-08-12 — every case below was **executed**, not described.
Each carries the result actually observed and the screenshot it was read from.

**This document is the only verification this feature gets.** This repository
has **no Svelte component test harness**, so `ColumnNodes.svelte` — the filter
bar's five buttons, the `activeFilter.match` wiring, and the three row-render
branches that now switch on `hasDetail` — has **zero automated coverage**.
What *is* automated is one layer down: `node-filters.test.js` covers
`hasArrows`, `isText`, `isLinked`, `isExtract`, `isCard`, `hasDetail` and the
`NODE_FILTERS` table as pure functions. Everything below is what that cannot
reach: whether the right nodes appear in the column, and whether any row's
appearance changed.

## How this run was set up

- The collection under test is a **copy** of the user's real collection, made
  with `cp -r` into the job scratch directory
  (`~/.claude/jobs/ad91db08/tmp/collection-filters`). The real collection at
  `…/SStorytime-main/examples/test` and its `learning.db` were never opened by
  the app during this run. `~/.config/texere/config.json` was pointed at the
  copy for the duration and **restored afterwards** (verified: it reads the real
  path again).
- The scratch notes file is `node-filter-check.n4l` **in that copy only**. Per
  the controller's instruction it was never created in the user's real
  collection, so there is nothing there to have cleaned up — confirmed by `ls`:
  the real collection has no such file and no file in it was modified today.
- Its chapter/marker shape was copied from the collection's own `test3.n4l`
  (`- <chapter>`, then `# @id <shortid>` / `# @card` lines above the node) and,
  for the sequence block, from `Mary.n4l`
  (`+:: _sequence_ , … ::` … `-:: _sequence_ ::`).
- **Recall (Ctrl+Shift+R), Study and Build Graph (Ctrl+G) were deliberately
  never opened.** All three call `learning.Sync`, which runs
  `InjectMissingIDs`/`InjectMissingExtractIDs` and would have written an
  `# @id` onto the deliberately id-less card (F5) and onto every prose node,
  destroying the very distinctions under test. The scratch file was checked on
  disk after the run and is byte-identical to what was written: F5 still has no
  `# @id`.
- The app was driven with synthetic X11 input (XTEST) on `DISPLAY=:0` and each
  state was captured by grabbing the app window with `XGetImage`. Root-window
  capture on this WSLg display returns black, so every screenshot is a grab of
  the window itself. Screenshots were read back as images; crops were enlarged
  3–4× to read the chevron and quote glyphs.

### The ten test nodes

`node-filter-check.n4l`, chapter **filter check**. Parsing was confirmed ahead
of the run with a throwaway `internal/content` test (written, run, deleted; the
repo is clean). Field values are what the parser produced:

| # | Line as written | `ID` | node `Arrows` | ditto `Arrows` | `StudyMeta` |
|---|---|---|---|---|---|
| F1 | `plain prose with no structure at all` | — | 0 | 0 | no |
| F2 | `a node with an arrow (means) a linked target` | — | 1 | 0 | no |
| F3 | `# @id` + `an extract with no arrows` | filt0001 | 0 | 0 | no |
| F4 | `# @id` + `# @card` + `a card with no arrows >> the answer side` | filt0002 | 0 | 0 | **yes** |
| F5 | `# @card` + `a card with no id at all >> rescued by the design` | **—** | 0 | 0 | **yes** |
| F6 | `# @id` + `# @card` + card line, then `" (note) …` ditto | filt0003 | 0 | 1 | **yes** |
| F7 | `# @id` + `# @card` + `a promoted card (from) "the source sentence…"` | filt0004 | 1 | 0 | **yes** |
| F8 | `# @id` + `an extract that also carries an arrow`, then `" (note) …` | filt0005 | 0 | 1 | no |
| F9 | `step one of the sequence, no arrows` (inside `_sequence_`) | — | 0 | 0 | no |
| F10 | `step two of the sequence (means) a linked step` (inside `_sequence_`) | — | 1 | 0 | no |

Two shapes here are worth calling out because they are the only ways a card can
carry an arrow at all. The parser takes a study-marked line **literally** and
does not parse arrows from it (`parser.go`, `if sm != nil && !hasFromArrowToken`),
so a card gets arrows only via **a ditto line** (F6) or via the
`(from) "anchor"` token that `PromoteExtract` writes (F7). A naive
`# @card` + `(means) x` line would have produced a card with **zero** arrows and
would have silently failed to test the overlap. Both real shapes are covered.

Expected membership, derived from the table above and the five predicates:

| | All | Text | Linked | Extracts | Cards |
|---|---|---|---|---|---|
| F1 prose | ✓ | ✓ | | | |
| F2 arrow | ✓ | | ✓ | | |
| F3 extract, no arrows | ✓ | **✓** | | **✓** | |
| F4 card, no arrows | ✓ | | | | ✓ |
| F5 card, no `@id` | ✓ | | | | **✓** |
| F6 card + ditto arrow | ✓ | | **✓** | | **✓** |
| F7 promoted card | ✓ | | **✓** | | **✓** |
| F8 extract + ditto arrow | ✓ | | ✓ | ✓ | |
| F9 sequence step, no arrows | ✓ | ✓ | | | |
| F10 sequence step + arrow | ✓ | | ✓ | | |
| **count** | **10** | **3** | **5** | **2** | **4** |

## Result summary

**11 of 11 cases pass. No failures, no bugs found.** Screenshot paths below are
relative to `~/.claude/jobs/ad91db08/tmp/`.

| # | Case | Result |
|---|---|---|
| TC-1 | **All** — every node, exactly as before the change | **PASS** |
| TC-2 | **Text** — prose only, no arrow rows, no cards | **PASS** |
| TC-3 | **Linked** — every node carrying an arrow, on the node or a ditto | **PASS** |
| TC-4 | **Extracts** — `# @id` nodes that are not cards | **PASS** |
| TC-5 | **Cards** — every study-marked node, including the one with no `# @id` | **PASS** |
| TC-6 | **Overlap 1** — extract with no arrows in **both** Text and Extracts | **PASS** |
| TC-7 | **Overlap 2** — card carrying arrows in **both** Linked and Cards | **PASS** |
| TC-8 | **Rendering** — a `# @card` with no arrows still shows a chevron | **PASS** |
| TC-9 | **Rendering** — arrows expand, prose keeps `"`, sequences keep `(n)` + stem | **PASS** |
| TC-10 | **Narrow column** — the five buttons at the column's minimum width | **PASS** |
| TC-11 | **Context filter × quick filter** — the two compose as an AND | **PASS** |

## TC-1 All — every node in the chapter

**Result: PASS.** Ten rows, in file order, all ten test nodes present
(`shotf-03-all.png`, enlarged as `zoom-03-new-all.png`). "Exactly as before this
change" is not asserted from memory — it was measured against the `8046355`
build in TC-8 below.

## TC-2 Text — prose only

**Result: PASS.** Exactly three rows: F1 `plain prose with no structure at all`,
F3 `an extract with no arrows`, F9 `(1) step one of the sequence, no arrows`
(`shotf-04-text.png`). No arrow-bearing node and no card appears. Note that F3
is here *and* under Extracts — that is TC-6, not a leak.

## TC-3 Linked — anything carrying an arrow

**Result: PASS.** Exactly five rows: F2, F6, F7, F8, F10
(`shotf-05-linked.png`). Both ditto-only-arrow nodes (F6, F8) are included, so
`hasArrows` really does look through `Dittos`, and both cards that carry arrows
(F6, F7) are here — the second half of TC-7. F4 and F5, cards with no arrows at
all, are correctly absent.

## TC-4 Extracts — `# @id` without a study marker

**Result: PASS.** Exactly two rows: F3 `an extract with no arrows` and F8
`an extract that also carries an arrow` (`shotf-06-extracts.png`). F8's presence
is the point of the whole change: it has an `# @id` **and** an arrow, and the
old predicate would have excluded it. F4, F6 and F7 all carry `# @id` but are
study-marked, and are correctly absent.

**The bug this replaced, observed directly.** On the `8046355` build the same
chapter under **Extracts** returns F1, F3 and F9 — the three prose nodes, i.e.
what is now called **Text** — and does **not** contain F8
(`shotf-18-pre-extracts.png`, enlarged as `zoom-18.png`). The old filter never
looked at `# @id`. Confirmed at source: `ColumnNodes.svelte:19` on that commit
reads `function isExtract(node) { return !hasArrows(node) && !node.StudyMeta }`.

## TC-5 Cards — every study-marked node, `# @id` or not

**Result: PASS.** Exactly four rows: F4, **F5 `a card with no id at all >>
rescued by the de…`**, F6, F7 (`shotf-07-cards.png`). F5 is the case the design
deliberately rescued: a hand-written `# @card` with no `# @id`. Under a literal
"`@id` AND `@card`" rule it would have appeared under no filter at all. It is
here, and the file on disk still shows it with no injected id.

## TC-6 Overlap 1 — an extract with no arrows is in both Text and Extracts

**Result: PASS.** F3 `an extract with no arrows` appears in the **Text** list
(`shotf-04-text.png`, second row) **and** in the **Extracts** list
(`shotf-06-extracts.png`, first row). Both screenshots were read; the node is in
both, not one. The lenses overlap as designed.

## TC-7 Overlap 2 — a card carrying arrows is in both Linked and Cards

**Result: PASS**, on both of the two shapes a card can take.

- F6 `a card that carries a ditto arrow >> the ans…` — present in **Linked**
  (`shotf-05-linked.png`, second row) and in **Cards** (`shotf-07-cards.png`,
  third row).
- F7 `a promoted card` — the `(from) "…"` shape `PromoteExtract` actually
  writes — present in **Linked** (fourth row) and in **Cards** (fourth row).

Neither is missing from either list.

## TC-8 Rendering regression — a `# @card` with no arrows still shows a chevron

This is the specific breakage the `isStructured` split could have caused: the
filter predicate and the row-render switch used to be the same function, and if
the renderer had been pointed at the narrower `isLinked` instead of `hasDetail`,
a card with no arrows would have flipped to the flat quote-style row.

**Result: PASS.** F4 `a card with no arrows >> the answer side` renders with a
`▶` chevron on the new build (`zoom-03-new-all.png`), clicking it flips to `▼`
and opens an empty children block (`shotf-09-expand-f4-card-no-arrows.png`,
`zoom-11-seq.png`). The same node renders with a `▶` chevron on the `8046355`
build (`zoom-17-pre-all.png`). Same for F5, the id-less card.

**Measured, not eyeballed.** The two "All" screenshots were diffed pixel by
pixel over the node-list region (`diff-new-vs-pre.png`). The entire difference
is a **uniform 1-pixel horizontal shift of every glyph** — outlines light up,
glyph interiors do not, and every row, chevron, quote mark, sequence number and
stem sits in the same place. The cause was then pinned down rather than assumed:
scanning a scanline for the column borders puts them at x=438/729 in the new
shot and x=439/730 in the comparison shot, because the splitters were restored
by hand between the two runs and landed 1px apart. No row changed render branch.

## TC-9 Rendering — the other three row shapes

**Result: PASS**, all four sub-cases, identical on both builds.

- **Arrows expand.** F2 `a node with an arrow` opens to `means → a linked
  target`; F6 opens to `note/remark → the arrow lives on this ditto line`
  (`shotf-08-expand-f2.png`, `shotf-10-expand-f6.png`).
- **Prose keeps the quote row.** F1 and F3 render with no chevron and the muted
  serif `"` icon (`zoom-11-seq.png`).
- **Sequences keep their number and stem.** F9 shows `(1)` in blue with the
  timeline stem beneath it and a quote-style row; F10 shows `(2)` with a
  chevron (`shotf-11-sequence.png`, `zoom-11-seq.png`).

Worth knowing, **not a regression**: the `(n)` sequence number is the node's
position among the *displayed* nodes, so under **Linked** the node F10 labelled
`(2)` under All is labelled `(1)`. `seqNumMap` has always been computed over
`displayNodes`, and the `8046355` build behaves identically — its **Structured**
list shows the same node as `(1)` (`zoom-19.png`). Unchanged by this feature;
recorded here so a future reader does not mistake it for one.

## TC-10 The filter bar at the column's minimum width

**Result: PASS.** Both splitters were dragged to their `max={420}` stops,
leaving the nodes column — which is the flex remainder — at roughly **174px**,
its minimum at this window size. The bar **wraps to two rows**: `All · Text ·
Linked` on the first, `Extracts · Cards` on the second. All five labels are
fully rendered, none clipped, nothing overflows the column
(`shotf-12-narrow.png`, enlarged as `zoom-12-bar.png`).

Task 2 step 5 did add `flex-wrap: wrap` — `ColumnNodes.svelte:331` — and it is
load-bearing. On the `8046355` build, whose `.quick-filter-bar` has no
`flex-wrap`, the third button is cut off mid-word at the column edge
("Structure…") rather than wrapping (`shotf-14-pre-start.png`). With five
buttons and no wrap, two would have been unreachable.

**Not established:** whether the window itself can be made narrower than
1100px. Synthetic drags on the window frame's right edge and corner were ignored
by this WSLg window manager (geometry unchanged after both attempts), so the
narrowest column reachable in this run was the splitter-imposed ~174px. Below
that the wrap would presumably continue to three rows; that is an inference.

## TC-11 The context filter and the quick filter compose

Not in the brief; checked because the quick filter stacks on a **derived** store
(`quickFiltered = $filteredNodes.filter(activeFilter.match)`) and a stale or
double-applied filter would show here.

**Result: PASS.** With the context tag `filter check steps` selected, **All**
narrows to F9 and F10 (`shotf-21-ctx-all.png`). Applying **Text** on top leaves
F9 alone (`shotf-22-ctx-text.png`); applying **Cards** leaves nothing and the
column shows the `No nodes` empty hint rather than falling back to the unfiltered
list (`shotf-23-ctx-cards.png`). The two filters intersect, in that order:
context first, quick filter second.

Also spot-checked: **A–Z** composes with a quick filter. Under **Linked** with
A–Z on, the same five nodes appear, alphabetically ordered
(`shotf-24-linked-az.png`).

## Not covered by this run

- **Unknown `quickFilter` id falls back to `all`.** `activeFilter = NODE_FILTERS
  .find(…) ?? NODE_FILTERS[0]` cannot be reached from the UI — `quickFilter` is
  only ever assigned from the table's own ids. Code-level only.
- **Nodes with a `# @study` or `# @cloze` marker.** Only `# @card` was used.
  `isCard` tests `StudyMeta` rather than the marker kind, and the parser sets
  `StudyMeta` for all three, so the behaviour follows — but it is an inference
  from the code, not something this run observed.
- **Window widths below 1100px** (see TC-10).

## Nothing was changed in the product

No production code was touched for this run. The only repository change is this
document. The comparison worktree was removed (`git worktree remove`) and `HEAD`
on the working checkout never moved. The scratch collection copy, the scratch
`.n4l`, the screenshots, the throwaway X11 driver and the crop/diff helpers all
live under `~/.claude/jobs/ad91db08/tmp/` and outside the repository; the
collection copy was **never merged back**.
