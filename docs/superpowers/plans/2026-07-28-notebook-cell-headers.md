# Notebook Cell Type Headers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every stacked notebook cell a slim header naming its type, so a pile of cells can be scanned at a glance.

**Architecture:** One pure function, `cellLabel(cell)`, in its own dependency-free module `frontend/src/lib/cell-label.js`, returning the finished header text. `NotebookPane` renders it once per cell in the wrapper it already owns. No cell component gains anything; two that currently name themselves lose their now-duplicated labels.

**Tech Stack:** Svelte 3.49, `node:test`.

## Global Constraints

- **The header is rendered by `NotebookPane`, never by a cell component.** One implementation instead of nine, consistent by construction, and immune to each cell's own `readOnly` gating.
- **The header must be visible in read-only notebooks.** `N4LDraftCell`, `ImageCell` and `SketchCell` gate their own toolbars on `{#if !readOnly}` and have no bar at all when reading — which is exactly when identifying a cell matters. The header therefore sits outside every `readOnly` condition.
- **Names match the button that creates the cell.** An `image` cell reads `Scan` because the button reads `+ Scan`. Same word for creating and identifying.
- **No control consolidation.** Do not move the delete `✕` or the `▲▼` reorder buttons into the header, and do not otherwise edit the nine cell components beyond the two label removals in Task 3.
- **Image, audio and video media behaviour must not regress** — all three shipped and are in use.
- Follow the existing style of the files edited: 2-space indent, no semicolons, the same muted CSS palette.
- Run `make test-js` and `cd frontend && npx vite build` before each commit.

---

### Task 1: `cellLabel`

**Files:**
- Create: `frontend/src/lib/cell-label.js`
- Create: `frontend/src/lib/cell-label.test.js`

**Interfaces:**
- Consumes: nothing.
- Produces: `cellLabel(cell) -> string`, exported from `frontend/src/lib/cell-label.js`. Task 2 imports it from there.

**Why its own module, and NOT `notebook.js`.** `notebook.js` statically imports
nine `.svelte` components for `cellRegistry`. Node's test runner cannot parse
`.svelte`, so a `node:test` file can never import anything from that module.
`cellLabel` therefore lives in its own dependency-free file.

**Do not** attempt to make `notebook.js` importable from Node — its
`cellRegistry` must stay a plain, synchronously-populated object. `NotebookPane`
reads `cellRegistry[cell.type]` directly in `<svelte:component>`, and an object
populated later by a promise triggers no Svelte reactivity, so every cell would
render blank.

`make test-js` runs `node --test frontend/src/lib/*.test.js`, so a new `cell-label.test.js` in that directory is picked up automatically with no Makefile change.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/cell-label.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cellLabel } from './cell-label.js'

test('each stacked cell type gets the name of the button that creates it', () => {
  assert.equal(cellLabel({ type: 'n4l' }), 'N4L draft')
  assert.equal(cellLabel({ type: 'graph' }), 'Graph')
  assert.equal(cellLabel({ type: 'sketch' }), 'Sketch')
  assert.equal(cellLabel({ type: 'audio' }), 'Audio')
  // The button reads "+ Scan", so the header must too — not "Image".
  assert.equal(cellLabel({ type: 'image' }), 'Scan')
})

test('a detail is appended where the type has meaningful variants', () => {
  assert.equal(cellLabel({ type: 'llm', mode: 'tutor' }), 'LLM · tutor')
  assert.equal(cellLabel({ type: 'llm', mode: 'ask' }), 'LLM · ask')
  assert.equal(cellLabel({ type: 'compute', language: 'js' }), 'Compute · js')
})

// The whole point: three stacked Media cells must not read identically.
test('a media cell names what it actually holds', () => {
  assert.equal(cellLabel({ type: 'media', kind: 'image' }), 'Media · image')
  assert.equal(cellLabel({ type: 'media', kind: 'video' }), 'Media · video')
  assert.equal(cellLabel({ type: 'media', kind: 'audio' }), 'Media · audio')
})

// "pronunciation" is the word a language learner scans for; "audio" is not.
test('a pronunciation is named as one, not as audio', () => {
  assert.equal(
    cellLabel({ type: 'media', kind: 'audio', provider: 'pronunciation' }),
    'Media · pronunciation')
  assert.equal(
    cellLabel({ type: 'media', kind: 'audio', provider: 'commons-audio' }),
    'Media · audio')
})

// Media cells saved before the audio slice have no `kind`. A dangling
// separator would look like a rendering bug.
test('a missing detail appends nothing, not a dangling separator', () => {
  assert.equal(cellLabel({ type: 'media' }), 'Media')
  assert.equal(cellLabel({ type: 'llm' }), 'LLM')
  assert.equal(cellLabel({ type: 'compute' }), 'Compute')
})

test('an unknown type stays identifiable rather than blank', () => {
  assert.equal(cellLabel({ type: 'widget' }), 'widget')
  // context is vestigial in cellRegistry and never stacked, but if one ever
  // appeared it must still render something.
  assert.equal(cellLabel({ type: 'context' }), 'context')
})

test('the header is never empty', () => {
  assert.equal(cellLabel({}), 'Cell')
  assert.equal(cellLabel({ type: '' }), 'Cell')
  assert.equal(cellLabel(null), 'Cell')
  assert.equal(cellLabel(undefined), 'Cell')
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `make test-js`
Expected: FAIL — `Cannot find module '.../frontend/src/lib/cell-label.js'`.

- [ ] **Step 3: Write the implementation**

Create `frontend/src/lib/cell-label.js`. It imports nothing — that is the point:

```js
// Display name per stacked cell type, matching the button that creates it — the
// same word for creating and for identifying. `context` is deliberately absent:
// ContextCell is rendered above the stack rather than inside it, so it never
// needs a header; the fallback in cellLabel covers it if one ever appears.
const CELL_NAMES = {
  n4l: 'N4L draft',
  llm: 'LLM',
  graph: 'Graph',
  compute: 'Compute',
  sketch: 'Sketch',
  image: 'Scan',
  audio: 'Audio',
  media: 'Media',
}

// mediaDetail names what a media cell actually holds. Three stacked cells all
// reading "Media" — a picture, a pronunciation and a video — is precisely the
// confusion these headers exist to remove. A pronunciation is called that
// rather than "audio" because it is the word a learner is scanning for.
function mediaDetail(cell) {
  const kind = cell.kind || ''
  if (kind === 'audio' && cell.provider === 'pronunciation') return 'pronunciation'
  return kind
}

// cellLabel is the text of a stacked cell's header: the type's name, plus a
// detail when that type has meaningful variants. Kept pure and outside the
// component so it is unit-testable without mounting Svelte.
//
// An unknown type falls back to the raw type string, and a type-less cell to
// "Cell", so a header is never blank — a blank strip reads as a rendering bug.
// A missing detail appends nothing rather than a dangling separator.
export function cellLabel(cell) {
  const type = cell?.type ?? ''
  const name = CELL_NAMES[type] || type || 'Cell'
  let detail = ''
  if (type === 'llm') detail = cell.mode || ''
  else if (type === 'compute') detail = cell.language || ''
  else if (type === 'media') detail = mediaDetail(cell)
  return detail ? `${name} · ${detail}` : name
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `make test-js`
Expected: PASS — the seven new tests plus every pre-existing suite in `frontend/src/lib/`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/cell-label.js frontend/src/lib/cell-label.test.js
git commit -m "feat(notebook): cellLabel — type name plus a detail where it varies"
```

---

### Task 2: Render the header in `NotebookPane`

**Files:**
- Modify: `frontend/src/components/notebook/NotebookPane.svelte`

**Interfaces:**
- Consumes: `cellLabel(cell)` from `frontend/src/lib/cell-label.js` (Task 1).
- Produces: a `.nb-cellhead` element above every stacked cell.

**The placement trap — read this before editing.** The design doc says the header goes "inside the existing `.nb-cellwrap`", which is **wrong** and would produce a visible layout bug. `.nb-cellwrap` is a **horizontal flex row** (`display: flex; gap: 6px`) whose children are the `▲▼` reorder gutter and `.nb-cell`. A header added as a third child of that row would render *beside* the cell, not above it.

The header must go **inside `.nb-cell`**, as the first child, before `<svelte:component>`. `.nb-cell` is `flex: 1; min-width: 0` and stacks its children normally.

- [ ] **Step 1: Import the helper**

Add a new import line beside the existing `notebook.js` import. `cellLabel` lives in its own module, so this is a separate import, not an extension of the existing one:

```js
  import { cellLabel } from '../../lib/cell-label.js'
```

- [ ] **Step 2: Render the header inside `.nb-cell`**

Find the `.nb-cell` div (currently around line 92) and add the header as its first child, leaving the `<svelte:component>` and all its props untouched:

```svelte
        <div class="nb-cell">
          <div class="nb-cellhead">{cellLabel(cell)}</div>
          <svelte:component this={cellRegistry[cell.type]} {cell} {readOnly} {context} cells={cells}
```

Note it sits outside every `{#if !readOnly}` — three cells have no toolbar at all when reading, and identifying them then is the point of the feature.

- [ ] **Step 3: Style it**

Add beside the existing `.nb-cell` rule in the `<style>` block:

```css
  /* Reads as chrome, not content: the same muted, letter-spaced treatment
     AudioCell already used for its own name, dimmed further. Deliberately has
     no background — nine cell types have their own borders and radii, and a
     filled strip would have to match each one to avoid looking detached. */
  .nb-cellhead { font-size: 10px; letter-spacing: .06em; text-transform: uppercase; color: #5d7186; padding: 0 2px 3px; }
```

- [ ] **Step 4: Verify the build**

Run: `cd frontend && npx vite build` — must succeed.
Run: `make test-js` from the repo root — must pass.

Re-read your diff and confirm the header is inside `.nb-cell`, not a sibling of it, and that no `svelte:component` prop was altered.

Visual confirmation needs a GUI and is covered by Task 3's manual checks.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): render a type header above every stacked cell"
```

---

### Task 3: Remove the two duplicated in-cell labels

**Files:**
- Modify: `frontend/src/components/notebook/MediaCell.svelte`
- Modify: `frontend/src/components/notebook/AudioCell.svelte`

**Interfaces:**
- Consumes: the header from Task 2.
- Produces: nothing.

Two cells named themselves before this feature existed. With the header in place they would show their name twice.

- [ ] **Step 1: Remove `MediaCell`'s label**

Delete this line (currently line 93), leaving the rest of the `me-bar` — the licence chip, spacer, Replace/Cancel and delete buttons — untouched:

```svelte
    <span class="me-label">Media</span>
```

Delete its now-unused rule (currently line 141):

```css
  .me-label { font-size: 12px; color: #8aa; }
```

- [ ] **Step 2: Remove `AudioCell`'s label**

Delete this line (currently line 160), keeping the duration chip that follows it:

```svelte
    <span class="au-title">🎙 Audio</span>
```

Delete its now-unused rule (currently line 202):

```css
  .au-title { color: #7dd3fc; font-size: 11px; letter-spacing: .06em; text-transform: uppercase; }
```

- [ ] **Step 3: Confirm nothing else referenced them**

Run: `grep -rn "me-label\|au-title" frontend/src/`
Expected: no output. If either name still appears, remove that reference too rather than leaving a dead class.

- [ ] **Step 4: Verify the build and tests**

Run: `cd frontend && npx vite build` — must succeed.
Run: `make test-js` from the repo root — must pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/notebook/MediaCell.svelte frontend/src/components/notebook/AudioCell.svelte
git commit -m "refactor(notebook): drop the two in-cell names now the header shows them"
```

- [ ] **Step 6: Record the manual checks**

This change is too small to warrant its own manual test plan document — the repo keeps one per feature slice, and this is a single UI affordance. Record these checks in your report instead, for the human to walk:

1. Every stacked cell shows a header naming its type; the names read `N4L draft`, `LLM`, `Graph`, `Compute`, `Sketch`, `Scan`, `Audio`, `Media`.
2. An LLM cell reads `LLM · ask` and switches to `LLM · tutor` when the mode is changed.
3. A compute cell reads `Compute · js`.
4. Three media cells holding a picture, a pronunciation and a video read `Media · image`, `Media · pronunciation` and `Media · video` respectively — the case the feature exists for.
5. No cell shows its name twice; in particular the media and audio cells show it only in the header.
6. Open a **read-only** notebook: headers are still present on every cell, including the N4L draft, Scan and Sketch cells, which have no toolbar at all in that mode.
7. The context panel above the stack has no header.
8. Reordering with `▲▼` moves the header with its cell, and the header sits above the cell rather than beside the reorder buttons.

---

## Self-Review

**Spec coverage.** Every requirement maps to a task: `cellLabel` as a pure function beside `cellRegistry` → Task 1; the names table including `image` → `Scan` → Task 1; the media detail with the pronunciation special case → Task 1; the unknown-type and missing-detail fallbacks → Task 1; rendering in `NotebookPane`'s wrapper outside all `readOnly` gating → Task 2; the muted chrome styling → Task 2; removal of `me-label` and `au-title` → Task 3; `ContextCell` receiving no header → satisfied structurally, since it is rendered above the `{#each}` and Task 2 only touches the loop body. Everything in the spec's out-of-scope list is absent: no control consolidation, no `ContextCell` header, no icons, no collapsing.

**Type consistency.** `cellLabel` is defined in Task 1 and consumed in Task 2 under the same name and signature. `CELL_NAMES` keys match `cellRegistry`'s keys exactly, minus `context`, which the fallback covers. The detail field names — `mode`, `language`, `kind`, `provider` — match the cell shapes in `notebook.js`'s factories.

**One correction to the spec, applied in Task 2.** The design doc places the header "inside the existing `.nb-cellwrap`". That is wrong: `.nb-cellwrap` is a horizontal flex row containing the reorder gutter and `.nb-cell`, so a header placed there would render beside the cell rather than above it. Task 2 states the correct placement — first child of `.nb-cell` — and flags the trap explicitly, since following the spec literally would produce a visible layout bug.

**One deliberate refinement.** The spec calls for "a quiet background distinct from the cell body". Task 2 uses a background-less caption instead, because nine cell types carry their own borders and corner radii and a filled strip would have to match each to avoid looking detached. The muted, letter-spaced treatment still reads as chrome, and it reuses the look `AudioCell` already had for its own name.
