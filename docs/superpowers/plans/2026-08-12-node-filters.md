# Node Column Filters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the nodes column's three quick filters with five — All · Text · Linked · Extracts · Cards — backed by predicates that mean what their labels say.

**Architecture:** Frontend only. The filter predicates move out of `ColumnNodes.svelte`, where nothing can test them, into a new pure `frontend/src/lib/node-filters.js` covered by `node --test`. A `NODE_FILTERS` table drives both the buttons and the filtering so the two cannot disagree. The component's `isStructured` splits in two: the filter takes the narrow `hasArrows` meaning, while the row renderer moves to `hasDetail`, which preserves today's behaviour exactly.

**Tech Stack:** Svelte 3.49, Vite 3, `node --test` for the pure JS. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-08-12-node-filters-design.md`

## Global Constraints

- **No Go changes.** No parser change, no change to `# @id` / `# @card` semantics, no change to promotion. Every field this feature needs already exists on `Node` (`internal/content/types.go:22-35`).
- **No new dependencies.**
- **No row's rendering may change.** The only visible change is the filter bar itself. `hasDetail` must accept exactly what `isStructured` accepts today — `hasArrows(node) || !!node.StudyMeta` — so a `# @card` node with no arrows still renders as a collapsible row rather than flipping to the flat quote style.
- **The five filters are overlapping lenses, not exclusive buckets.** A node may match several. A card carrying arrows appears under both Linked and Cards; an extract with no arrows appears under both Text and Extracts. Do not add precedence to make the counts sum to All.
- **`Cards` means "has a study marker"** — `!!node.StudyMeta` — not "has `# @id` and `# @card`". `# @cloze` and `# @study` also set `StudyMeta` (`internal/content/parser.go:388-412`), and a hand-written `# @card` need not carry an `# @id`; the literal rule would make such a node invisible in every filter.
- **`Extracts` means `node.ID !== '' && !node.StudyMeta`** — an `@id` is what makes something an extract, and a promoted one is a Card instead.
- **Pure logic goes in `frontend/src/lib/`, not in a component.** `make test-js` runs `node --test frontend/src/lib/*.test.js`; a module importing a `.svelte` file, a store, `katex` or `wailsjs` cannot load there.
- **Every test must be proven able to fail.** Each test step names the production change that breaks it; make that change, watch the test fail for that reason, then revert. A test that passes against a gutted implementation is worse than no test.
- **Filter labels are exactly** `All`, `Text`, `Linked`, `Extracts`, `Cards` — and filter ids exactly `all`, `text`, `linked`, `extracts`, `cards`, in that order.

---

### Task 1: The filter predicates

**Files:**
- Create: `frontend/src/lib/node-filters.js`
- Create: `frontend/src/lib/node-filters.test.js`

**Interfaces:**
- Consumes: nothing. The module is self-contained and imports nothing.
- Produces, for Task 2:
  - `hasArrows(node) -> boolean`
  - `isText(node) -> boolean`
  - `isLinked(node) -> boolean`
  - `isExtract(node) -> boolean`
  - `isCard(node) -> boolean`
  - `hasDetail(node) -> boolean`
  - `NODE_FILTERS -> [{ id: string, label: string, match: (node) => boolean }]`

A node here is the parser's shape, serialised through Wails with Go field names: `{ ID, Text, Line, Arrows, Dittos, StudyMeta }`. `Arrows` is an array on the node; each entry of `Dittos` may carry its own `Arrows`. `ID` is `''` when there is no `# @id`. `StudyMeta` is `null` when the node is not studyable.

- [ ] **Step 1: Write the failing tests**

Create `frontend/src/lib/node-filters.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  hasArrows, isText, isLinked, isExtract, isCard, hasDetail, NODE_FILTERS,
} from './node-filters.js'

// Node shapes mirroring the parser's output (Go field names, via Wails).
const plain      = { ID: '', Text: 'just a line', Arrows: [], Dittos: [], StudyMeta: null }
const bare       = { ID: '', Text: 'no collections at all' }
const rootArrow  = { ID: '', Text: 'a', Arrows: [{ Code: 'is', Target: 'b' }], Dittos: [], StudyMeta: null }
const dittoArrow = { ID: '', Text: 'a', Arrows: [], Dittos: [{ Text: '', Arrows: [{ Code: 'is', Target: 'b' }] }], StudyMeta: null }
const dittoOnly  = { ID: '', Text: 'a', Arrows: [], Dittos: [{ Text: 'more prose', Arrows: [] }], StudyMeta: null }
const extract    = { ID: 'e1', Text: 'an extracted sentence', Arrows: [], Dittos: [], StudyMeta: null }
const extractArr = { ID: 'e2', Text: 'an extract that grew an arrow', Arrows: [{ Code: 'is', Target: 'b' }], Dittos: [], StudyMeta: null }
const card       = { ID: 'c1', Text: 'front >> back', Arrows: [], Dittos: [], StudyMeta: { CardType: 'basic' } }
const cardNoID   = { ID: '', Text: 'hand written >> card', Arrows: [], Dittos: [], StudyMeta: { CardType: 'basic' } }
const cardArrows = { ID: 'c2', Text: 'front >> back', Arrows: [{ Code: 'is', Target: 'b' }], Dittos: [], StudyMeta: { CardType: 'cloze' } }

test('hasArrows sees arrows on the node and on a ditto', () => {
  assert.equal(hasArrows(rootArrow), true)
  assert.equal(hasArrows(dittoArrow), true)
  assert.equal(hasArrows(plain), false)
  assert.equal(hasArrows(dittoOnly), false)
})

// A node arriving without Arrows/Dittos at all must not throw.
test('hasArrows tolerates missing collections', () => {
  assert.equal(hasArrows(bare), false)
})

test('isText is prose with no arrows and no study marker', () => {
  assert.equal(isText(plain), true)
  assert.equal(isText(dittoOnly), true)
  assert.equal(isText(rootArrow), false)
  assert.equal(isText(card), false)
})

// An extract is structurally prose. Overlapping lenses: it is BOTH.
test('isText includes an extract that carries no arrows', () => {
  assert.equal(isText(extract), true)
  assert.equal(isExtract(extract), true)
})

test('isLinked is any node carrying an arrow', () => {
  assert.equal(isLinked(rootArrow), true)
  assert.equal(isLinked(dittoArrow), true)
  assert.equal(isLinked(plain), false)
})

// The overlap the spec calls out: a card with arrows must appear under Linked,
// which is the one place someone hunting for arrows would look.
test('isLinked includes a card that carries arrows', () => {
  assert.equal(isLinked(cardArrows), true)
  assert.equal(isCard(cardArrows), true)
})

test('isExtract needs an @id and no study marker', () => {
  assert.equal(isExtract(extract), true)
  assert.equal(isExtract(extractArr), true)
  assert.equal(isExtract(plain), false)
  assert.equal(isExtract(card), false)
})

test('isCard is any study-marked node, with or without an @id', () => {
  assert.equal(isCard(card), true)
  assert.equal(isCard(cardNoID), true)
  assert.equal(isCard(cardArrows), true)
  assert.equal(isCard(extract), false)
  assert.equal(isCard(plain), false)
})

// A hand-written card with no @id must be reachable somewhere. Under the
// literal "@id AND @card" rule it would appear in no filter at all.
test('a study marker without an @id is still a card and is not an extract', () => {
  assert.equal(isCard(cardNoID), true)
  assert.equal(isExtract(cardNoID), false)
})

// hasDetail is the RENDER switch, not a filter. It must keep accepting exactly
// what the old isStructured accepted: arrows OR a study marker. If it ever
// narrows to hasArrows, a card with no arrows silently loses its collapsible
// row.
test('hasDetail accepts arrows or a study marker', () => {
  assert.equal(hasDetail(rootArrow), true)
  assert.equal(hasDetail(dittoArrow), true)
  assert.equal(hasDetail(card), true)
  assert.equal(hasDetail(cardNoID), true)
  assert.equal(hasDetail(plain), false)
  assert.equal(hasDetail(extract), false)
})

test('NODE_FILTERS is the bar, in order, with unique ids', () => {
  assert.deepEqual(NODE_FILTERS.map((f) => f.id), ['all', 'text', 'linked', 'extracts', 'cards'])
  assert.deepEqual(NODE_FILTERS.map((f) => f.label), ['All', 'Text', 'Linked', 'Extracts', 'Cards'])
  assert.equal(new Set(NODE_FILTERS.map((f) => f.id)).size, NODE_FILTERS.length)
})

// 'all' carries a real match so the caller filters unconditionally and never
// special-cases it.
test('every filter has a callable match, and all matches everything', () => {
  for (const f of NODE_FILTERS) assert.equal(typeof f.match, 'function', f.id)
  const all = NODE_FILTERS.find((f) => f.id === 'all')
  for (const n of [plain, rootArrow, extract, card, cardNoID, bare]) {
    assert.equal(all.match(n), true)
  }
})

test('each filter entry applies its own predicate', () => {
  const match = (id) => NODE_FILTERS.find((f) => f.id === id).match
  assert.equal(match('text')(plain), true)
  assert.equal(match('text')(rootArrow), false)
  assert.equal(match('linked')(rootArrow), true)
  assert.equal(match('linked')(plain), false)
  assert.equal(match('extracts')(extract), true)
  assert.equal(match('extracts')(card), false)
  assert.equal(match('cards')(card), true)
  assert.equal(match('cards')(extract), false)
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test-js`
Expected: FAIL — `Cannot find module './node-filters.js'`, because the module does not exist yet.

- [ ] **Step 3: Write the implementation**

Create `frontend/src/lib/node-filters.js`:

```js
// The nodes column's quick filters.
//
// These predicates lived inside ColumnNodes.svelte, where this repo's
// `node --test` cannot reach them — which is how the old "Extracts" filter came
// to mean "no arrows and no study marker", never looking at # @id at all.
//
// The five filters are overlapping LENSES, not exclusive buckets: a node may
// match several, because "has arrows" and "was extracted" are different
// questions about the same node.
//
// Dependency-free: no Svelte, no stores. A node is the parser's shape as it
// arrives through Wails — { ID, Text, Arrows, Dittos, StudyMeta }.

/** An arrow on the node itself, or on any of its ditto continuation lines. */
export function hasArrows(node) {
  return (node?.Arrows?.length > 0) ||
         (node?.Dittos?.some((d) => d.Arrows?.length > 0) ?? false)
}

/** Prose: no N4L structure and not studyable. */
export function isText(node) {
  return !hasArrows(node) && !node?.StudyMeta
}

/** Carries at least one arrow. Named for the toolbar's +link button. */
export function isLinked(node) {
  return hasArrows(node)
}

/** Extracted from a source: has a # @id and has not been promoted to a card. */
export function isExtract(node) {
  return !!node?.ID && !node?.StudyMeta
}

/**
 * Studyable. Deliberately "has a study marker" rather than "# @id AND # @card":
 * # @cloze and # @study set StudyMeta too, and a hand-written # @card need not
 * carry an # @id — under the literal rule such a node would appear in no filter
 * at all.
 */
export function isCard(node) {
  return !!node?.StudyMeta
}

/**
 * The ROW RENDERER's switch, not a filter: does this node have anything to
 * expand? This is the old isStructured, kept verbatim. The filter bar no longer
 * uses it — Linked is the narrower hasArrows — and the two must not be merged
 * again, or a card with no arrows loses its collapsible row.
 */
export function hasDetail(node) {
  return hasArrows(node) || !!node?.StudyMeta
}

/** The filter bar, in display order. One table drives both buttons and filtering. */
export const NODE_FILTERS = [
  { id: 'all',      label: 'All',      match: () => true },
  { id: 'text',     label: 'Text',     match: isText },
  { id: 'linked',   label: 'Linked',   match: isLinked },
  { id: 'extracts', label: 'Extracts', match: isExtract },
  { id: 'cards',    label: 'Cards',    match: isCard },
]
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS, every test in both lib test files, output pristine.

- [ ] **Step 5: Prove the tests can fail**

Four separate controls. Make each change, run `make test-js`, confirm the named test fails, then revert it before the next one.

1. In `hasArrows`, drop the ditto clause — `return node?.Arrows?.length > 0`.
   Expected: `hasArrows sees arrows on the node and on a ditto` fails on `dittoArrow`.
2. In `isCard`, require an id — `return !!node?.ID && !!node?.StudyMeta`.
   Expected: `isCard is any study-marked node, with or without an @id` and `a study marker without an @id is still a card and is not an extract` both fail on `cardNoID`. This is the exact rule the spec rejected, so the control also proves the tests pin that decision.
3. In `hasDetail`, narrow it to `return hasArrows(node)`.
   Expected: `hasDetail accepts arrows or a study marker` fails on `card` — the regression this split could cause.
4. In `NODE_FILTERS`, give `text` the `isLinked` match.
   Expected: `each filter entry applies its own predicate` fails on `match('text')(plain)`.

If any control passes, the test is not testing what it claims. Fix the test before continuing.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/node-filters.js frontend/src/lib/node-filters.test.js
git commit -m "feat(nodes): add the five node-filter predicates, tested"
```

---

### Task 2: Wire the column to the table

**Files:**
- Modify: `frontend/src/components/ColumnNodes.svelte` — `:9` (state comment), `:15-20` (delete local predicates), `:62-66` (filtering), `:136-141` (the bar), `:161` and `:209` (render switch)

**Interfaces:**
- Consumes: everything Task 1 produces.
- Produces: nothing — this is the leaf.

This repo has no Svelte component harness, so this task has no automated test of its own; its logic was tested in Task 1 and its behaviour is verified in Task 3. Read your own diff carefully — that is the real gate here.

- [ ] **Step 1: Import the lib and delete the local predicates**

At the top of the `<script>` block, beside the existing store import:

```js
  import { hasDetail, NODE_FILTERS } from '../lib/node-filters.js'
```

Then delete the local `hasArrows`, `isExtract` and `isStructured` functions (`:15-20`) together with their comment. `allDittoRows`, `toggleExpand` and `selectNode` stay exactly as they are.

- [ ] **Step 2: Filter through the table**

Replace the `quickFiltered` block (`:62-66`):

```js
  // Quick filter stacks on top of the context-filtered derived store. The
  // table's 'all' entry matches everything, so there is no special case here.
  $: activeFilter = NODE_FILTERS.find((f) => f.id === quickFilter) ?? NODE_FILTERS[0]
  $: quickFiltered = $filteredNodes.filter(activeFilter.match)
```

and update the declaration comment at `:9` to name the five ids:

```js
  let quickFilter = 'all'           // one of NODE_FILTERS[].id
```

- [ ] **Step 3: Render the bar from the table**

Replace the three hand-written buttons (`:138-140`) with:

```svelte
    {#each NODE_FILTERS as f (f.id)}
      <button class="qf-btn" class:active={quickFilter === f.id} on:click={() => (quickFilter = f.id)}>{f.label}</button>
    {/each}
```

Keep the surrounding `<div class="quick-filter-bar">` and the `qf-btn` / `active` class names exactly as they are — the styling is unchanged.

- [ ] **Step 4: Move the render switch to hasDetail**

At `:161` and `:209`, change `isStructured(node)` to `hasDetail(node)`. These are the only two call sites. Nothing else about those blocks changes.

- [ ] **Step 5: Check the bar still fits**

The bar goes from three buttons to five in a column whose width is user-adjustable down to 120px (`TopBand.svelte:44`). Build and look at it at the narrowest width. If the buttons overflow their row, add `flex-wrap: wrap` to `.quick-filter-bar` and nothing else — do not shorten a label, do not drop a filter, and do not introduce a scroller. If it already fits, change no CSS at all and say so.

- [ ] **Step 6: Verify the build**

Run: `cd frontend && npx vite build`
Expected: builds with no errors and no new warnings. Paste the warning list into your report rather than characterising it.

Run: `make test-js`
Expected: unchanged from Task 1 — this task adds no tests.

Then read your diff and confirm all of:
- `isStructured` no longer exists anywhere in the file, and `hasDetail` is at both former call sites.
- No `<style>` rule changed except a possible `flex-wrap` from Step 5.
- The five labels read exactly `All`, `Text`, `Linked`, `Extracts`, `Cards`.
- `allDittoRows`, `seqNumMap`, the cursor tracking and the hover actions are untouched.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/ColumnNodes.svelte
git commit -m "feat(nodes): five quick filters — All, Text, Linked, Extracts, Cards"
```

---

### Task 3: Manual verification

**Files:**
- Create: `docs/superpowers/specs/2026-08-12-node-filters-manual-test-plan.md`

**Interfaces:**
- Consumes: the built app from Tasks 1 and 2.
- Produces: a filled-in test plan with a pass/fail per case.

- [ ] **Step 1: Build and run against a copy of the collection**

The real collection is at `/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`. Copy it — `cp -r … "$CLAUDE_JOB_DIR/tmp/collection"` — and open the copy, so nothing this task does can touch the user's notes or their `learning.db`.

```bash
make build && ./build/bin/texere
```

Pick a chapter with a mix of content. If no single chapter has all five kinds, add the missing ones to the copy: a plain prose line, a line with an arrow, a `# @id` node, and a `# @card` node — and one hand-written `# @card` with **no** `# @id`, which is the case the spec deliberately rescued.

- [ ] **Step 2: Check each filter**

| # | Filter | Expect |
|---|---|---|
| 1 | All | Every node in the chapter, exactly as before this change |
| 2 | Text | Prose only — no arrow rows, no cards |
| 3 | Linked | Every node carrying an arrow, on the node or on a ditto |
| 4 | Extracts | `# @id` nodes that are not cards |
| 5 | Cards | Every study-marked node, **including** the one with no `# @id` |

- [ ] **Step 3: Check the overlaps**

The lenses deliberately overlap. Confirm both:
- An extract with no arrows appears under **both** Text and Extracts.
- A card carrying arrows appears under **both** Linked and Cards.

If either is missing from one of its two filters, that is a failure — record it.

- [ ] **Step 4: Confirm no row's rendering changed**

This is the regression the `isStructured` split could cause. With All selected, compare against the pre-change build (`git stash` or a second checkout):
- A node with arrows still shows a chevron and expands to its arrow list.
- **A `# @card` node with no arrows still shows a chevron**, rather than flipping to the flat quote-style row.
- A plain prose node still shows the quote-style row with its `"` icon.
- Sequence nodes still show their `(n)` number and timeline stem.

- [ ] **Step 5: Check the bar at the narrowest column width**

Drag the nodes column to its minimum. Record whether the five buttons fit, and whether Step 5 of Task 2 added `flex-wrap`.

- [ ] **Step 6: Write up the result**

Record every case above with a pass/fail and a one-line observation in `docs/superpowers/specs/2026-08-12-node-filters-manual-test-plan.md`, following the shape of `2026-08-12-recall-media-math-manual-test-plan.md`. A failure is a finding, not something to smooth over. Note that the scratch collection copy lives in the job directory and was never merged back.

- [ ] **Step 7: Commit**

```bash
git add docs/superpowers/specs/2026-08-12-node-filters-manual-test-plan.md
git commit -m "docs(test-plan): node column filters"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Five filters, exact labels and ids | 1 (`NODE_FILTERS`), 2 (the bar) |
| Overlapping lenses, not exclusive buckets | 1 (two overlap tests), 3 step 3 |
| `Cards` = has a study marker, `@id` not required | 1 (`isCard`, control 2), 3 step 2 case 5 |
| `Extracts` = `@id` without a study marker | 1 (`isExtract`) |
| `Text` = no arrows, no study marker | 1 (`isText`) |
| `Linked` = at least one arrow | 1 (`isLinked`, `hasArrows`) |
| `isStructured` splits; rendering unchanged | 1 (`hasDetail`, control 3), 2 step 4, 3 step 4 |
| Predicates in `lib/`, `node --test` covered | 1 |
| Filter table drives both buttons and filtering | 1 (`NODE_FILTERS`), 2 steps 2-3 |
| Unknown `quickFilter` falls back to `all` | 2 step 2 (`?? NODE_FILTERS[0]`) |
| Missing `Arrows`/`Dittos` does not throw | 1 (`bare` node test) |
| No Go changes, no new dependencies | Global Constraints |

**Type consistency:** `hasArrows`, `isText`, `isLinked`, `isExtract`, `isCard`, `hasDetail` and `NODE_FILTERS` keep the same names across Task 1's implementation, Task 1's tests and Task 2's import. Task 2 imports only `hasDetail` and `NODE_FILTERS` — the individual predicates reach the component through the table's `match`, so there is one path from a filter id to its behaviour, not two. Node fields are Go-cased (`ID`, `Arrows`, `Dittos`, `StudyMeta`), matching `internal/content/types.go:22-35`.

**Scope:** two new lib files, one component, one doc. Small enough for one plan.
