# Notebook Cell Folding Implementation Plan

**Goal:** Let a stacked notebook cell be folded from its header — body hidden, one line of content-derived preview shown in its place — so a long notebook can be skimmed.

**Architecture:** One new pure module, `frontend/src/lib/cell-preview.js`, exporting `cellPreview(cell)` and `formatDuration(ms)`. `NotebookPane` owns everything else: a disclosure button in the header strip it already renders, a `.nb-cellbody` wrapper that gets `display: none` when folded, and a Fold all toggle in the notebook header. **Folded cells stay mounted** — they are hidden with CSS, never unmounted.

**Tech Stack:** Svelte 3.49, `node:test`, Vite 3.

## Global Constraints

- **Folded cells MUST stay mounted.** Hide the body with `display: none`. Never wrap `<svelte:component>` in `{#if !cell.folded}` or any equivalent. `ComputeCell` holds its entire output (`logs`, `value`, `error`, `chartConfigs`) in component-local state that is never written to `cell`; `LLMCell` holds `partial` and unsent `input`; `AudioCell` holds a live `MediaRecorder`. All three have `onDestroy` handlers that cancel or stop work. Unmounting on fold destroys user data.
- **Fold state is `cell.folded`,** a boolean on the cell object. Absent key reads as unfolded, so notebooks saved before this change open unfolded. Cells are persisted as one JSON blob (`SaveNotebook(nbId, JSON.stringify(cells))`) — there is no schema to migrate and no Go change anywhere in this plan.
- **Mutate `folded` in place; never replace cell objects.** Components hold non-reactive references into their prop — `SketchCell.svelte:31` initialises `strokes` from `cell.strokes` in a plain `let`, not under `$:`. Use `cells[i].folded = …; cells = cells`, not `cells.map((c) => ({ ...c, folded: v }))`.
- **The fold controls are visible in read-only notebooks.** They sit outside every `{#if !readOnly}` guard. Folding does not change what a notebook means, and `scheduleSave()` already early-returns on `readOnly` (`NotebookPane.svelte:35`), so no new carve-out is needed — it simply does not persist.
- **Do not touch the `▲▼` reorder buttons or the `✕` delete button,** their placement, or their `{#if !readOnly}` guard. The gap before `✕` is deliberate mis-click protection documented at `NotebookPane.svelte:139-143`.
- **Image, audio and video media behaviour must not regress** — all three shipped and are in use.
- Follow the existing style of the files edited: 2-space indent, no semicolons, the same muted CSS palette.
- Run `make test-js` and `cd frontend && npx vite build` before each commit.

---

### Task 1: `cellPreview`

**Files:**
- Create: `frontend/src/lib/cell-preview.js`
- Create: `frontend/src/lib/cell-preview.test.js`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `cellPreview(cell) -> string` — one line of content for a folded cell, `''` when there is nothing worth quoting. Task 3 imports it.
  - `formatDuration(ms) -> string` — `m:ss`. Task 2 imports it.

**Why its own module, and NOT `cell-label.js` or `notebook.js`.** `notebook.js`
statically imports nine `.svelte` components for `cellRegistry`; Node's test
runner cannot parse `.svelte`, so a `node:test` file can never import anything
reachable from it. That rules out `notebook.js`, and it is the same reason
`cell-label.js` exists as its own file.

It is separate from `cell-label.js` because the two answer different questions
from different data. `cellLabel` reads `type`/`mode`/`kind` and **always**
returns something, since a blank header reads as a rendering bug. `cellPreview`
reads eight different content fields and is **expected** to return `''`, which
the caller renders as no element at all. Keeping them apart also leaves
`cell-label.test.js` untouched.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/cell-preview.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cellPreview, formatDuration } from './cell-preview.js'

test('a text-bearing cell previews its first non-empty line', () => {
  assert.equal(cellPreview({ type: 'n4l', text: 'Ovid (wrote) Metamorphoses' }), 'Ovid (wrote) Metamorphoses')
  assert.equal(cellPreview({ type: 'compute', code: 'print(math.sqrt(16))' }), 'print(math.sqrt(16))')
  assert.equal(cellPreview({ type: 'graph', query: 'Ovid | +2' }), 'Ovid | +2')
})

// A draft that opens with blank lines still has content worth showing.
test('leading blank lines are skipped, and only the first real line is shown', () => {
  assert.equal(cellPreview({ type: 'n4l', text: '\n\n  Ovid\nMetamorphoses' }), 'Ovid')
  assert.equal(cellPreview({ type: 'compute', code: '\n\nprint(1)\nprint(2)' }), 'print(1)')
})

// The count says how much conversation is hidden; the first *user* message says
// what it was about. The first message in the array is always the user's today,
// but preferring by role keeps this honest if that ever changes.
test('an LLM cell previews its opening question and how many messages follow', () => {
  const cell = {
    type: 'llm',
    messages: [
      { role: 'user', content: 'explain the aorist tense' },
      { role: 'assistant', content: 'The aorist is…' },
    ],
  }
  assert.equal(cellPreview(cell), '"explain the aorist tense" · 2 msgs')
})

test('an LLM cell picks the first user message, not the first message', () => {
  const cell = {
    type: 'llm',
    messages: [
      { role: 'system', content: 'you are a tutor' },
      { role: 'user', content: 'decline rosa' },
    ],
  }
  assert.equal(cellPreview(cell), '"decline rosa" · 2 msgs')
})

test('one message is a msg, not msgs', () => {
  const cell = { type: 'llm', messages: [{ role: 'user', content: 'hi' }] }
  assert.equal(cellPreview(cell), '"hi" · 1 msg')
})

test('a sketch previews its stroke count, singular at one', () => {
  assert.equal(cellPreview({ type: 'sketch', strokes: [1, 2, 3] }), '3 strokes')
  assert.equal(cellPreview({ type: 'sketch', strokes: [1] }), '1 stroke')
})

// Assets are content-hashed (internal/media/import.go stores sha256hex+ext and
// discards the original name), so there is no filename to fall back to.
test('an image previews its geometry, since it has no name to show', () => {
  assert.equal(cellPreview({ type: 'image', naturalW: 1024, naturalH: 768 }), '1024×768')
})

test('an audio cell previews its duration', () => {
  assert.equal(cellPreview({ type: 'audio', durationMs: 14000 }), '0:14')
  assert.equal(cellPreview({ type: 'audio', durationMs: 605000 }), '10:05')
})

test('a media cell prefers title, then subject, then query', () => {
  assert.equal(cellPreview({ type: 'media', title: 'T', subject: 'S', query: 'Q' }), 'T')
  assert.equal(cellPreview({ type: 'media', title: '', subject: 'S', query: 'Q' }), 'S')
  assert.equal(cellPreview({ type: 'media', title: '', subject: '', query: 'Q' }), 'Q')
})

// The caller renders no element at all for '', so a cell with nothing in it
// folds to its header alone rather than to a placeholder.
test('a cell with no content previews as the empty string', () => {
  assert.equal(cellPreview({ type: 'n4l', text: '' }), '')
  assert.equal(cellPreview({ type: 'n4l', text: '\n  \n' }), '')
  assert.equal(cellPreview({ type: 'llm', messages: [] }), '')
  assert.equal(cellPreview({ type: 'graph', query: '' }), '')
  assert.equal(cellPreview({ type: 'compute', code: '' }), '')
  assert.equal(cellPreview({ type: 'sketch', strokes: [] }), '')
  assert.equal(cellPreview({ type: 'image', naturalW: 0, naturalH: 0 }), '')
  assert.equal(cellPreview({ type: 'audio', durationMs: 0 }), '')
  assert.equal(cellPreview({ type: 'media', title: '', subject: '', query: '' }), '')
})

// Unlike cellLabel, which falls back to the raw type so a header is never
// blank, there is no sensible content guess for a type this module never saw.
test('an unknown or absent type previews as the empty string', () => {
  assert.equal(cellPreview({ type: 'hologram' }), '')
  assert.equal(cellPreview({}), '')
  assert.equal(cellPreview(null), '')
  assert.equal(cellPreview(undefined), '')
})

// Not cosmetic: CSS does the visible ellipsis at the pane's real width, which
// is the only place that width is known. This cap stops a huge draft's first
// line reaching the DOM for a cell folded specifically to stop looking at it.
test('an over-long line is capped at 120 characters', () => {
  const long = 'x'.repeat(500)
  assert.equal(cellPreview({ type: 'n4l', text: long }).length, 120)
  assert.equal(cellPreview({ type: 'graph', query: long }).length, 120)
})

test('formatDuration renders m:ss with a padded seconds field', () => {
  assert.equal(formatDuration(0), '0:00')
  assert.equal(formatDuration(7000), '0:07')
  assert.equal(formatDuration(60000), '1:00')
  assert.equal(formatDuration(605000), '10:05')
})

test('formatDuration treats junk as zero rather than rendering NaN', () => {
  assert.equal(formatDuration(undefined), '0:00')
  assert.equal(formatDuration(null), '0:00')
  assert.equal(formatDuration(NaN), '0:00')
  assert.equal(formatDuration(-5), '0:00')
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `make test-js`
Expected: FAIL — `Cannot find module '.../frontend/src/lib/cell-preview.js'`

- [ ] **Step 3: Write the implementation**

Create `frontend/src/lib/cell-preview.js`:

```js
// One line of content-derived text for a folded notebook cell. Deliberately its
// own dependency-free module: notebook.js statically imports nine .svelte
// components for cellRegistry, which Node's test runner cannot parse, so
// nothing reachable from there is unit-testable.
//
// Separate from cell-label.js on purpose. cellLabel reads type/mode/kind and
// always returns something, because a blank header reads as a rendering bug.
// cellPreview reads eight different content fields and is expected to return
// '' — the caller then renders no preview element at all, so a cell with
// nothing in it folds to its header alone rather than to a placeholder.

// Caps what reaches the DOM. This is not the visible truncation: CSS does that,
// at the pane's real width, which is the only place the width is known. The cap
// stops a 40 KB draft's first line being written into the DOM for a cell the
// user folded specifically to stop looking at.
const MAX = 120

function clip(s) {
  const t = String(s ?? '').trim()
  return t.length > MAX ? t.slice(0, MAX) : t
}

// The first line with something on it — a draft that opens with blank lines
// still has content worth previewing.
function firstLine(text) {
  for (const line of String(text ?? '').split('\n')) {
    const t = line.trim()
    if (t) return clip(t)
  }
  return ''
}

// m:ss. Exported because AudioCell renders the same cell's duration a few
// pixels below this preview; two private copies could drift into showing two
// different numbers for one recording. Non-finite and negative input floor to
// zero rather than rendering "NaN:NaN".
export function formatDuration(ms) {
  const n = Number(ms)
  const s = Math.floor((Number.isFinite(n) && n > 0 ? n : 0) / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// The count says how much conversation is hidden; the opening question says
// what it was about. Selected by role rather than by position so a future
// system message cannot become the preview.
function llmPreview(cell) {
  const msgs = Array.isArray(cell.messages) ? cell.messages : []
  const head = clip(msgs.find((m) => m?.role === 'user')?.content)
  if (!head) return ''
  return `"${head}" · ${msgs.length} ${msgs.length === 1 ? 'msg' : 'msgs'}`
}

function sketchPreview(cell) {
  const n = Array.isArray(cell.strokes) ? cell.strokes.length : 0
  return n ? `${n} ${n === 1 ? 'stroke' : 'strokes'}` : ''
}

// Assets are content-hashed — internal/media/import.go stores sha256hex+ext and
// discards the original filename — so geometry is the only identifying thing an
// image cell persists. Zero until the <img> has loaded once.
function imagePreview(cell) {
  const w = Number(cell.naturalW) || 0
  const h = Number(cell.naturalH) || 0
  return w && h ? `${w}×${h}` : ''
}

export function cellPreview(cell) {
  if (!cell) return ''
  switch (cell.type) {
    case 'n4l': return firstLine(cell.text)
    case 'llm': return llmPreview(cell)
    case 'graph': return clip(cell.query)
    case 'compute': return firstLine(cell.code)
    case 'sketch': return sketchPreview(cell)
    case 'image': return imagePreview(cell)
    case 'audio': return cell.durationMs ? formatDuration(cell.durationMs) : ''
    case 'media': return clip(cell.title || cell.subject || cell.query)
    default: return ''
  }
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS — all `cell-preview.test.js` tests, and `cell-label.test.js` and the other `lib/*.test.js` files still passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/cell-preview.js frontend/src/lib/cell-preview.test.js
git commit -m "feat(notebook): cellPreview — one line of content for a folded cell"
```

---

### Task 2: `AudioCell` uses the shared duration formatter

**Files:**
- Modify: `frontend/src/components/notebook/AudioCell.svelte` (import block; the private `fmt`; the `.au-dur` call site)

> Line numbers below are as of the start of this task. Step 1 inserts a line, so
> everything after it shifts by one — anchor on the quoted code, not the number.

**Interfaces:**
- Consumes: `formatDuration(ms) -> string` from `frontend/src/lib/cell-preview.js` (Task 1).
- Produces: nothing new.

`AudioCell` has a private `fmt(ms)` producing exactly the `m:ss` that
`formatDuration` produces. Once folding ships, the header preview and the
`.au-dur` chip render the same cell's duration a few pixels apart, and two
private copies can drift into showing two different numbers for one recording.
Removing the copy also gives the formatter the test coverage it has none of
today.

- [ ] **Step 1: Add the import**

In `frontend/src/components/notebook/AudioCell.svelte`, after the existing
`media-error.js` import:

```js
  import { mediaErrorMessage } from '../../lib/media-error.js'
  import { formatDuration } from '../../lib/cell-preview.js'
```

- [ ] **Step 2: Delete the private formatter**

Remove this whole function — the only definition of `fmt` in the file — entirely:

```js
  function fmt(ms) {
    const s = Math.floor((ms || 0) / 1000)
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
  }
```

- [ ] **Step 3: Update the call site**

Inside `.au-bar`, change `<span class="au-dur">{fmt(cell.durationMs)}</span>` to:

```svelte
      <span class="au-dur">{formatDuration(cell.durationMs)}</span>
```

- [ ] **Step 4: Verify no other caller remains**

Run: `grep -n "fmt(" frontend/src/components/notebook/AudioCell.svelte`
Expected: no output. If anything matches, that call site was missed — update it to `formatDuration` before continuing.

- [ ] **Step 5: Verify the build and tests**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS, build succeeds with no unresolved-import error.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/notebook/AudioCell.svelte
git commit -m "refactor(audio-cell): share one duration formatter with the cell preview"
```

---

### Task 3: Fold a cell from its header

**Files:**
- Modify: `frontend/src/components/notebook/NotebookPane.svelte` (imports; `toggleFold`; the `{#each}` block; styles)
- Modify: `frontend/src/components/notebook/SketchCell.svelte` (`onResize`, currently lines 69-76)

> Line numbers below are as of the start of this task. Steps 1 and 2 insert
> lines, so later anchors shift — match on the quoted code, not the number.

**Interfaces:**
- Consumes: `cellPreview(cell) -> string` from `frontend/src/lib/cell-preview.js` (Task 1); the existing `cellLabel(cell)` from `cell-label.js`.
- Produces: `cell.folded` — a boolean on the cell object, absent meaning unfolded. Task 4 reads and writes it.

There is no Svelte component test harness in this repo — `frontend/src/lib/*.test.js`
are pure-JS modules under `node --test`, and `cell-label.js`'s header comment
records why components are out of reach. So this task has no failing test to
write. Verification is the build, the unchanged unit tests, and the explicit
manual checks in Step 6, which are the ones that would catch a regression.

- [ ] **Step 1: Add the import and the toggle**

In `NotebookPane.svelte`, add to the imports directly after the `cell-label.js`
line:

```js
  import { cellLabel } from '../../lib/cell-label.js'
  import { cellPreview } from '../../lib/cell-preview.js'
```

Add `toggleFold` beside the other cell mutators, directly after `deleteCell`:

```js
  // Mutated in place, never replaced with a spread copy: folded cells stay
  // mounted, and components hold non-reactive references into this object —
  // SketchCell initialises `strokes` from `cell.strokes` in a plain `let`.
  // scheduleSave() no-ops when readOnly, so folding a read-only notebook works
  // and simply does not persist.
  function toggleFold(i) {
    cells[i].folded = !cells[i].folded
    cells = cells
    scheduleSave()
  }
```

- [ ] **Step 2: Replace the `{#each}` block**

Replace the entire `{#each cells as cell, i (cell.id)}` … `{/each}` block inside
`.nb-scroll` — from the `{#each}` line through its matching `{/each}`, the whole
of the current `.nb-cellwrap` markup — with:

```svelte
    {#each cells as cell, i (cell.id)}
      {@const preview = cell.folded ? cellPreview(cell) : ''}
      <div class="nb-cellwrap">
        <div class="nb-cellhead">
          <button class="nb-fold" on:click={() => toggleFold(i)}
            aria-expanded={!cell.folded}
            title={cell.folded ? 'Unfold cell' : 'Fold cell'}>
            <span class="nb-chev">{cell.folded ? '▸' : '▾'}</span>
            <span class="nb-cellname">{cellLabel(cell)}</span>
          </button>
          {#if !readOnly}
            <span class="nb-headspacer"></span>
            <div class="nb-reorder">
              <button on:click={() => move(i, -1)} disabled={i === 0} title="Move up">▲</button>
              <button on:click={() => move(i, 1)} disabled={i === cells.length - 1} title="Move down">▼</button>
            </div>
            <button class="nb-del" on:click={() => deleteCell(i)} title="Delete cell">✕</button>
          {/if}
        </div>
        {#if preview}<div class="nb-preview">{preview}</div>{/if}
        <div class="nb-cellbody" class:nb-folded={cell.folded}>
          <svelte:component this={cellRegistry[cell.type]} {cell} {readOnly} {context} cells={cells}
            onChange={(t) => (typeof t === 'string' ? updateCell(i, t) : updateCellObj(i, t))}
            onPush={(t) => onPush(t)}
            onInsert={(text) => insertAfter(i, newDraftCell(text))} />
        </div>
      </div>
    {/each}
```

Three things to get right here:

- **The `.nb-cellbody` wrapper is required.** `class:` cannot reach through
  `<svelte:component>` unless every cell component forwards a class prop. It is
  a plain block div and `.nb-cellwrap` is no longer a flex container, so it is
  layout-neutral.
- **`<svelte:component>` stays outside every `{#if}`.** Hiding is `display: none`
  on the wrapper. See the Global Constraints — unmounting destroys user data.
- **The fold button is outside `{#if !readOnly}`;** `▲▼` and `✕` stay inside it,
  unchanged.

`{@const}` requires Svelte 3.46+ and must be an immediate child of a block.
`package.json` pins `"svelte": "^3.49.0"`, and it sits directly inside `{#each}`,
so both conditions hold.

- [ ] **Step 3: Add the styles**

In the `<style>` block of `NotebookPane.svelte`, add directly after the existing
`.nb-cellname` rule:

```css
  /* A button inherits no typography, so every property .nb-cellhead sets has to
     be restored by hand or the name stops matching the strip it sits in.
     min-width:0 keeps this flex child shrinkable so .nb-cellname can ellipsis.
     The name is inside the hit target on purpose: this app supports pen input
     as a first-class case (SketchCell dispatches on pointerType), and a 9px
     chevron is a poor pen target. It is on the LEFT so that the one frequently
     clicked control in this strip is as far as possible from ✕ — see the
     mis-click note on .nb-del below. */
  .nb-fold { display: flex; align-items: center; gap: 4px; min-width: 0; background: none; border: none; margin: 0; padding: 2px 4px 2px 0; cursor: pointer; font: inherit; color: inherit; letter-spacing: inherit; text-transform: inherit; }
  .nb-fold:hover { color: #7dd3fc; }
  .nb-chev { flex: none; font-size: 8px; }
  /* Content, not chrome. The strip above is uppercase and letter-spaced because
     it names a type; this quotes what the user actually wrote, so it inherits
     none of that. It is 11px against the header's 10px and still reads as
     subordinate — lowercase x-height at 11px sits below cap height at 10px —
     while staying legible as prose. Indented to align under the name. */
  .nb-preview { padding: 0 2px 4px 16px; font-size: 11px; color: #46586a; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .nb-folded { display: none; }
```

- [ ] **Step 4: Mark the guard in `SketchCell`**

Folding makes an existing truthiness test in `SketchCell` load-bearing. Replace
the whole `function onResize()` in `SketchCell.svelte` with:

```js
  function onResize() {
    // Never persist the fullscreen container height as the cell's inline height.
    if (!expanded) {
      // The `h &&` below is load-bearing beyond its original purpose. A folded
      // cell is display:none, so clientHeight reads 0 here, and without the
      // truthiness test every folded sketch would have its saved height zeroed
      // and written to disk. Do NOT tidy this to `h !== cell.height`.
      const h = Math.round(wrap.clientHeight)
      if (h && h !== cell.height) { cell.height = h; onChange(cell) }
    }
    render()
  }
```

- [ ] **Step 5: Verify the build and tests**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS, build succeeds. A Svelte compile error mentioning `{@const}`
means it was not placed as a direct child of `{#each}` — fix the placement rather
than removing it.

- [ ] **Step 6: Verify by hand**

Run: `make dev`, open a notebook, and confirm each of these. The last three are
the point of the whole design — do not skip them.

1. Every cell header shows `▾ NAME`; clicking the chevron **or the name** folds
   it to `▸ NAME` plus one preview line, and clicking again restores it.
2. A fresh `+ Sketch cell` folds to header-only — no preview line, no empty stub.
3. Fold a cell, close and reopen the notebook: it is still folded.
4. Open a notebook read-only. The header shows `▸ NAME` with no `▲▼` and no `✕`;
   folding works there.
5. **Compute:** run a cell that emits output and a chart, fold it, unfold it —
   logs, value and chart are all still there.
6. **LLM:** send a question, fold the cell mid-stream, unfold — the answer is
   still arriving and completes. Separately, type into the input without
   sending, fold, unfold — the text is still there.
7. **Sketch:** draw, fold, unfold — the drawing returns at its original height,
   not collapsed. Restart the app and confirm the height is still right.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/notebook/NotebookPane.svelte frontend/src/components/notebook/SketchCell.svelte
git commit -m "feat(notebook): fold a cell from its header"
```

---

### Task 4: Fold all / Unfold all

**Files:**
- Modify: `frontend/src/components/notebook/NotebookPane.svelte` (`allFolded` and `foldAll`; the `.nb-head` block; `.nb-gear` and new `.nb-foldall` styles)

> Task 3 has already shifted every line number in this file. Anchor on the
> quoted code throughout.

**Interfaces:**
- Consumes: `cell.folded` (Task 3).
- Produces: nothing later tasks rely on.

- [ ] **Step 1: Add the state and the handler**

In `NotebookPane.svelte`, after `toggleFold`:

```js
  $: allFolded = cells.length > 0 && cells.every((c) => c.folded)

  // In place, for the same reason as toggleFold — see its comment. Replacing
  // every cell object underneath a stack of live components is a class of bug
  // worth not opening. The keyed {#each} sees unchanged ids, so nothing
  // remounts either way.
  function foldAll() {
    const v = !allFolded
    for (const c of cells) c.folded = v
    cells = cells
    scheduleSave()
  }
```

- [ ] **Step 2: Add the button to the notebook header**

Replace the whole `<div class="nb-head">` … `</div>` block — the one holding
`.nb-title` and `.nb-gear`, at the top of `.nb-pane` — with:

```svelte
  <div class="nb-head">
    <span class="nb-title">Notebook</span>
    {#if readOnly}<span class="nb-ro">read-only</span>{/if}
    <span class="nb-headspacer"></span>
    {#if cells.length}
      <button class="nb-foldall" on:click={foldAll}>{allFolded ? '▾ Unfold all' : '▸ Fold all'}</button>
    {/if}
    <button class="nb-gear" on:click={() => (showSettings = true)} title="LLM providers">⚙ Models</button>
  </div>
```

The explicit `.nb-headspacer` replaces `.nb-gear`'s `margin-left: auto`. Two
right-hand buttons both carrying `margin-left: auto` would split the free space
between them instead of grouping, and leaving the auto margin on `.nb-gear`
alone would let the gear drift left to sit against the title whenever the
notebook is empty and Fold all is not rendered. The spacer is correct in both
cases, and `.nb-headspacer` is already defined in this file.

- [ ] **Step 3: Update the styles**

Change the existing `.nb-gear` rule to drop its `margin-left: auto`, and add
`.nb-foldall` beside it:

```css
  .nb-gear { background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
  .nb-foldall { background: none; color: #5d7186; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
  .nb-foldall:hover { color: #7dd3fc; border-color: #2a4a6a; }
```

`.nb-foldall` is quieter than `.nb-gear` — it is a view control, not a way into
settings, and the muted `#5d7186` is the same colour the cell headers use.

- [ ] **Step 4: Verify the build and tests**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS, build succeeds.

- [ ] **Step 5: Verify by hand**

Run `make dev` and confirm:

1. Fold all folds every cell and the button relabels itself to Unfold all;
   Unfold all reverses it.
2. Folding the last remaining unfolded cell by hand flips the label to Unfold
   all on its own.
3. Reopening a notebook whose cells are all folded shows Unfold all immediately.
4. An empty notebook (no cells) shows no Fold all button, and `⚙ Models` is
   still right-aligned rather than sitting next to the title.
5. Fold all works in a read-only notebook and does not persist.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): fold all, for skimming a long notebook"
```

---

### Task 5: Manual test plan document

**Files:**
- Create: `docs/specs/2026-07-29-notebook-cell-folding-manual-test-plan.md`

**Interfaces:**
- Consumes: the behaviour built in Tasks 1-4.
- Produces: nothing consumed by code.

Every prior slice in this feature area shipped a manual test plan alongside its
design, because there is no Svelte component harness and the interesting
behaviour is all in the components. This one carries the destructive-fold cases
that are the whole justification for the CSS-hide decision.

- [ ] **Step 1: Write the document**

Create `docs/specs/2026-07-29-notebook-cell-folding-manual-test-plan.md`:

```markdown
# Notebook cell folding — manual test plan

**Design:** `2026-07-29-notebook-cell-folding-design.md`
**Build:** `make dev`

Unit coverage: `frontend/src/lib/cell-preview.test.js` via `make test-js`
covers `cellPreview` and `formatDuration` exhaustively. Everything below is what
that cannot reach.

## TC-1 Every stacked type folds and previews

For each of the eight stacked types — N4L draft, LLM, Graph, Compute, Sketch,
Scan, Audio, Media — add a cell, give it content, and fold it.

**Expect:** the header reads `▸ NAME`, the body disappears, and one muted line
appears beneath the header showing that cell's content: the draft's first line,
the LLM's opening question and message count, the graph query, the first line of
code, the stroke count, the image's pixel dimensions, the recording's duration,
the media title/subject/query. Clicking the chevron **or the name** unfolds it.

## TC-2 An empty cell folds to its header alone

Add a fresh cell of each type and fold it **without** giving it content.

**Expect:** header only. No preview line, no blank row, no `(empty)` placeholder.

## TC-3 Fold survives reopening and restart

Fold two cells in a notebook of five. Close it, reopen it. Then quit the app,
relaunch, and reopen it.

**Expect:** the same two cells are folded both times; the other three are not.

## TC-4 A notebook saved before this change opens unfolded

Open a notebook last saved by a build without folding.

**Expect:** every cell is unfolded and every header shows `▾`. No error toast.

## TC-5 Read-only

Open a notebook read-only (the reopen-by-`notebookId` path).

**Expect:** headers show `▸`/`▾` and the name, and **no** `▲▼` and **no** `✕`.
Folding and Fold all both work. Fold a cell, close, reopen read-only — it is
unfolded again, because read-only does not save.

## TC-6 Folding never destroys work

This is the case the design chose CSS-hiding over unmounting for. All four must
pass.

1. **Compute:** run a cell whose code calls `print(...)` and `chart(...)`. With
   output and a chart visible, fold the cell, then unfold it.
   **Expect:** logs, the `⇒` value and the rendered chart are all still there.
   *(Unmounting would have shown an empty output pane — `chartConfigs` is never
   written to the cell.)*
2. **LLM mid-stream:** send a question and fold the cell **while the answer is
   still streaming**. Wait, then unfold.
   **Expect:** the answer completed and is in the transcript. It was not
   truncated or cancelled.
3. **LLM unsent input:** type into an LLM cell's textarea without pressing Send.
   Fold, unfold.
   **Expect:** the typed text is still in the textarea.
4. **Audio mid-recording:** start recording, fold the cell, wait ~5s, unfold,
   stop.
   **Expect:** still recording when unfolded; the saved clip is the full length,
   and the duration chip agrees with the header preview.

## TC-7 A folded sketch keeps its height

Draw in a sketch cell, note its height, fold it, unfold it.

**Expect:** the drawing returns at its original height and is fully visible, not
collapsed to a sliver. Restart the app, reopen: still the original height.

*(A folded cell is `display: none`, so `SketchCell.onResize` measures
`clientHeight` as 0. The `h &&` truthiness guard is what stops that zero being
saved — this case exists to catch anyone removing it.)*

## TC-8 Fold all

1. In a notebook with cells, click **Fold all**.
   **Expect:** every cell folds; the button becomes **Unfold all**.
2. Click **Unfold all**.
   **Expect:** every cell unfolds; the button becomes **Fold all**.
3. Fold every cell but one by hand, then fold that last one.
   **Expect:** the button flips to **Unfold all** by itself.
4. Reopen a notebook whose cells were all folded.
   **Expect:** the button reads **Unfold all** immediately.
5. Open a notebook with no cells.
   **Expect:** no Fold all button, and `⚙ Models` still sits at the right edge.

## TC-9 The other header controls still work

On a folded cell: click `▲` and `▼`, and click `✕`.

**Expect:** reorder moves the folded cell and it stays folded; delete removes it.
`▲` is still disabled on the first cell and `▼` on the last. The gap between the
`▲▼` pair and `✕` is unchanged.

## TC-10 Audio duration is reported identically in both places

Record a clip of ~1:05 and fold the cell, then unfold it.

**Expect:** the folded preview and the `.au-dur` chip in the unfolded bar show
the same string. *(They now share `formatDuration`.)*
```

- [ ] **Step 2: Commit**

```bash
git add docs/specs/2026-07-29-notebook-cell-folding-manual-test-plan.md
git commit -m "docs(test-plan): notebook cell folding"
```

---

## Spec coverage

Every requirement in the design maps to a task:

| Spec requirement | Task |
|---|---|
| `cell-preview.js` as its own pure module; separate from `cell-label.js` | 1 |
| The eight-row preview table, incl. image geometry over filename | 1 |
| Empty preview → `''`; unknown type → `''` | 1 |
| 120-char cap in JS, ellipsis in CSS | 1 |
| `formatDuration` extracted, `AudioCell` imports it | 1, 2 |
| `toggleFold`, `cell.folded`, persistence via existing JSON blob | 3 |
| Chevron + name as one left-hand `<button>`, `aria-expanded` | 3 |
| Fold toggle outside `{#if !readOnly}` | 3 |
| `.nb-cellbody` wrapper + `display: none`, never unmounted | 3 |
| `.nb-preview` styled as content, not chrome | 3 |
| The `SketchCell.onResize` guard gets a comment | 3 |
| `allFolded` / `foldAll`, mutating in place | 4 |
| Fold all in `.nb-head`, shown in read-only | 4 |
| Unit tests for all eight types, empties, and `formatDuration` | 1 |
| Manual test plan document | 5 |

Nothing in the design's out-of-scope list is built: no delete undo, no per-type
icons, no drag-to-reorder, no auto-folding, no unmounting of folded cells, no
`ContextCell` header or fold (it is rendered above the `{#each}` and Task 3 only
touches the loop body), and no keyboard shortcut.
