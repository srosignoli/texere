# Notebook Cell Chrome Consolidation Implementation Plan

**Goal:** Move the `▲▼` reorder gutter and the eight per-cell delete buttons into the notebook cell header strip, so each stacked cell has two zones of chrome instead of three.

**Architecture:** `NotebookPane.svelte`'s `.nb-cellhead` becomes a flex row holding the cell name plus `▲ ▼` and `✕`, all under one `{#if !readOnly}`. The `.nb-reorder` gutter column is deleted and `.nb-cellwrap` stops being a flex container. The eight cell components each shed their delete button, its CSS and their `onDelete` prop. Two bars that go empty as a result (`AudioCell`, `MediaCell`) get suppressed, and `ComputeCell`'s now-doubled `js` label is removed.

**Tech Stack:** Svelte 3.49, Vite 3. Frontend unit tests are pure-JS modules under Node's test runner (`node --test frontend/src/lib/*.test.js`).

**Spec:** `docs/specs/2026-07-28-notebook-cell-chrome-consolidation-design.md`

## Global Constraints

- **No new unit tests.** This slice adds no pure logic. The only logic it
  touches — `cellLabel` and `hasContent` — is already covered by
  `frontend/src/lib/cell-label.test.js` and `frontend/src/lib/media-fact.test.js`.
  There is no Svelte component test harness and this slice does not add one.
- **Two automated gates run at every commit:**
  - `make test-js` — five test files must stay green (regression only).
  - `cd frontend && npm run build` — must complete **with no
    `Unused CSS selector` warnings**. This is the real check that CSS cleanup
    kept pace with markup removal; Svelte emits that warning for any rule whose
    selector no longer matches, so a leftover `.xx-del` rule fails the gate.
- **Do not add:** undo for delete, cell collapse/fold, per-type icons, or
  drag-to-reorder. All four are explicitly out of scope.
- **Do not restyle the header strip.** It keeps its floating treatment — no
  background, outside the cell border. This is a recorded decision, not an
  oversight; the reasoning lives in the comment above `.nb-cellhead`.
- **Line numbers in this plan are as of the branch tip and shift as you edit.**
  Match on the quoted text, not the line number.

---

### Task 1: Header gains the reorder and delete controls

Puts `▲ ▼ ✕` into `.nb-cellhead` and deletes the gutter. `onDelete` is still
passed to the cells in this task, so their own `✕` keeps working — the two
deletes coexist for one commit. That redundancy is deliberate: it keeps this
task's end state fully working, and Task 2 removes the duplicates.

**Files:**
- Modify: `frontend/src/components/notebook/NotebookPane.svelte` (markup ~85–102, styles ~125–134)

**Interfaces:**
- Consumes: `cellLabel(cell)` from `frontend/src/lib/cell-label.js`; the existing
  `move(i, dir)` and `deleteCell(i)` in `NotebookPane.svelte`.
- Produces: header CSS classes `.nb-cellname`, `.nb-headspacer`, `.nb-del`, and a
  restyled `.nb-reorder`. Task 2 relies on `deleteCell(i)` already being wired
  to the header `✕`.

- [ ] **Step 1: Replace the `{#each}` block markup**

Find this block (starts line 85) and replace it whole:

```svelte
    {#each cells as cell, i (cell.id)}
      <div class="nb-cellwrap">
        {#if !readOnly}
          <div class="nb-reorder">
            <button on:click={() => move(i, -1)} title="Move up">▲</button>
            <button on:click={() => move(i, 1)} title="Move down">▼</button>
          </div>
        {/if}
        <div class="nb-cell">
          <div class="nb-cellhead">{cellLabel(cell)}</div>
          <svelte:component this={cellRegistry[cell.type]} {cell} {readOnly} {context} cells={cells}
            onChange={(t) => (typeof t === 'string' ? updateCell(i, t) : updateCellObj(i, t))}
            onPush={(t) => onPush(t)}
            onDelete={() => deleteCell(i)}
            onInsert={(text) => insertAfter(i, newDraftCell(text))} />
        </div>
      </div>
    {/each}
```

with:

```svelte
    {#each cells as cell, i (cell.id)}
      <div class="nb-cellwrap">
        <div class="nb-cellhead">
          <span class="nb-cellname">{cellLabel(cell)}</span>
          {#if !readOnly}
            <span class="nb-headspacer"></span>
            <div class="nb-reorder">
              <button on:click={() => move(i, -1)} disabled={i === 0} title="Move up">▲</button>
              <button on:click={() => move(i, 1)} disabled={i === cells.length - 1} title="Move down">▼</button>
            </div>
            <button class="nb-del" on:click={() => deleteCell(i)} title="Delete cell">✕</button>
          {/if}
        </div>
        <svelte:component this={cellRegistry[cell.type]} {cell} {readOnly} {context} cells={cells}
          onChange={(t) => (typeof t === 'string' ? updateCell(i, t) : updateCellObj(i, t))}
          onPush={(t) => onPush(t)}
          onDelete={() => deleteCell(i)}
          onInsert={(text) => insertAfter(i, newDraftCell(text))} />
      </div>
    {/each}
```

Three things changed beyond adding the buttons: the `.nb-cell` div is gone (it
existed only as the gutter's flex sibling), the header moved out of it to sit
directly under `.nb-cellwrap`, and `▲`/`▼` are now `disabled` at the ends.
`move()` already no-ops there; a dead button was forgivable at 9px in a gutter
and is not once it is a prominent control.

- [ ] **Step 2: Replace the styles**

Find these rules (starting line 125) and replace them whole:

```css
  .nb-cellwrap { display: flex; gap: 6px; }
  .nb-reorder { display: flex; flex-direction: column; gap: 2px; padding-top: 4px; }
  .nb-reorder button { background: none; border: none; color: #46586a; cursor: pointer; font-size: 9px; }
  .nb-reorder button:hover { color: #7dd3fc; }
  .nb-cell { flex: 1; min-width: 0; }
  /* Reads as chrome, not content: the same muted, letter-spaced treatment
     AudioCell already used for its own name, dimmed further. Deliberately has
     no background — nine cell types have their own borders and radii, and a
     filled strip would have to match each one to avoid looking detached. */
  .nb-cellhead { font-size: 10px; letter-spacing: .06em; text-transform: uppercase; color: #5d7186; padding: 0 2px 3px; }
```

with:

```css
  /* Reads as chrome, not content: the same muted, letter-spaced treatment
     AudioCell already used for its own name, dimmed further. Deliberately has
     no background — nine cell types have their own borders and radii, and a
     filled strip would have to match each one to avoid looking detached. The
     reorder and delete controls belong to this wrapper rather than to the cell,
     and sitting outside the cell's border is how that reads honestly. */
  .nb-cellhead { display: flex; align-items: center; font-size: 10px; letter-spacing: .06em; text-transform: uppercase; color: #5d7186; padding: 0 2px 3px; }
  .nb-cellname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .nb-headspacer { flex: 1; }
  .nb-reorder { display: flex; gap: 2px; }
  .nb-reorder button { background: none; border: none; color: #46586a; cursor: pointer; font-size: 9px; padding: 0 2px; }
  .nb-reorder button:hover:not(:disabled) { color: #7dd3fc; }
  .nb-reorder button:disabled { opacity: .35; cursor: default; }
  /* The gap before ✕ is the whole of the mis-click protection: ▲▼ and delete
     are neighbours for the first time here, and deleteCell has no undo. Keep it
     visible — do not collapse it into the .nb-reorder gap. */
  .nb-del { margin-left: 14px; background: none; border: none; color: #46586a; cursor: pointer; font-size: 11px; padding: 0 2px; }
  .nb-del:hover { color: #ff8080; }
```

`.nb-cellwrap` keeps its class on the div (it is the keyed `{#each}` element)
but no longer needs a rule: the cells' own `margin-bottom: 10px` already
provides the spacing the flex gap was not supplying.

- [ ] **Step 3: Run the automated gates**

```bash
cd /home/simone/projects/texere && make test-js
cd /home/simone/projects/texere/frontend && npm run build
```

Expected: `make test-js` passes all five files. `npm run build` completes with
**no `Unused CSS selector` warning** — in particular none for `.nb-cell`, which
you just deleted along with its div.

- [ ] **Step 4: Verify in the running app**

```bash
cd /home/simone/projects/texere && make dev
```

Open a node's notebook and add three cells of different types. Confirm:
- each header reads `NAME` on the left and `▲ ▼   ✕` on the right;
- there is no gutter to the left of the cells, and cells are wider than before;
- `▲` is greyed on the first cell, `▼` on the last, and neither responds;
- `▲▼` reorder the middle cell both ways;
- the header `✕` deletes a cell;
- each cell still shows its own `✕` too — expected, Task 2 removes it.

- [ ] **Step 5: Commit**

```bash
cd /home/simone/projects/texere
git add frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): move reorder and delete into the cell header

The gutter column goes; the header strip becomes a flex row holding the
name plus the reorder pair and delete. Delete is separated from the
reorder pair by a deliberate gap — they are neighbours for the first
time and deleteCell has no undo. The cells' own delete still works and
is removed next.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: Strip the eight per-cell delete buttons

Removes the duplicate. Each component loses its delete button, that button's CSS
rules and its now-unused `export let onDelete`; three components also lose a
spacer that no longer pushes anything.

**Files:**
- Modify: `frontend/src/components/notebook/N4LDraftCell.svelte`
- Modify: `frontend/src/components/notebook/LLMCell.svelte`
- Modify: `frontend/src/components/notebook/GraphQueryCell.svelte`
- Modify: `frontend/src/components/notebook/ComputeCell.svelte`
- Modify: `frontend/src/components/notebook/SketchCell.svelte`
- Modify: `frontend/src/components/notebook/ImageCell.svelte`
- Modify: `frontend/src/components/notebook/AudioCell.svelte`
- Modify: `frontend/src/components/notebook/MediaCell.svelte`
- Modify: `frontend/src/components/notebook/NotebookPane.svelte`

**Interfaces:**
- Consumes: the header `✕` wired to `deleteCell(i)` in Task 1.
- Produces: no component accepts `onDelete` any more. Task 3 edits `.au-bar`,
  `.me-bar` and `.cc-lang`, all of which this task leaves in place.

**Which spacers go and which stay** — a spacer is only dead if `✕` was the last
thing to its right:

| component | delete to remove | spacer |
|---|---|---|
| `N4LDraftCell` | `.del-btn` | none — its `margin-left: auto` goes with the button |
| `LLMCell` | `.del` | remove `.spacer` |
| `GraphQueryCell` | `.gq-del` | none exists |
| `ComputeCell` | `.cc-del` | **keep** `.cc-spacer` — Run/Stop stays right-aligned |
| `SketchCell` | `.sk-del` | remove `.sk-spacer` |
| `ImageCell` | `.im-del` | remove `.im-spacer` |
| `AudioCell` | `.au-del` | **keep** `.au-spacer` — Replace stays right-aligned |
| `MediaCell` | the `.me-btn` delete | **keep** `.me-spacer` — Replace/Cancel stay right, and the class is used a second time in `.me-tools` |

- [ ] **Step 1: `N4LDraftCell.svelte`**

Delete the prop (line 11):

```svelte
  export let onDelete = () => {}
```

Delete the button (line 44):

```svelte
      <button class="del-btn" on:click={onDelete} title="Delete cell">✕</button>
```

Delete both rules (lines 55–56):

```css
  .del-btn { margin-left: auto; background: none; color: #6b7a8a; border: none; cursor: pointer; font-size: 12px; }
  .del-btn:hover { color: #ff8080; }
```

- [ ] **Step 2: `LLMCell.svelte`**

Delete the prop (line 11):

```svelte
  export let onDelete = () => {}
```

Delete both of these lines (115–116):

```svelte
    <span class="spacer"></span>
    {#if !readOnly}<button class="del" on:click={onDelete} title="Delete cell">✕</button>{/if}
```

Delete all three rules (lines 163–165):

```css
  .spacer { flex: 1; }
  .del { background: none; border: none; color: #6b7a8a; cursor: pointer; }
  .del:hover { color: #ff8080; }
```

- [ ] **Step 3: `GraphQueryCell.svelte`**

Delete the prop (line 11):

```svelte
  export let onDelete = () => {}
```

Delete the button (line 117), leaving the `Run` button and its `{#if !readOnly}`
wrapper intact:

```svelte
      <button class="gq-del" on:click={onDelete} title="Delete cell">✕</button>
```

Delete both rules (lines 141–142):

```css
  .gq-del { background: none; border: none; color: #6b7a8a; cursor: pointer; }
  .gq-del:hover { color: #ff8080; }
```

- [ ] **Step 4: `ComputeCell.svelte`**

Delete the prop (line 8):

```svelte
  export let onDelete = () => {}
```

Delete the button (line 57). Keep the `{#if running}…Run/Stop…{/if}` line above
it and keep `.cc-spacer`:

```svelte
      <button class="cc-del" on:click={onDelete} title="Delete cell">✕</button>
```

Delete both rules (lines 80–81):

```css
  .cc-del { background: none; border: none; color: #6b7a8a; cursor: pointer; }
  .cc-del:hover { color: #ff8080; }
```

- [ ] **Step 5: `SketchCell.svelte`**

Delete the prop (line 9):

```svelte
  export let onDelete = () => {}
```

Delete both of these lines (193–194):

```svelte
      <span class="sk-spacer"></span>
      <button class="sk-del" on:click={onDelete} title="Delete cell">✕</button>
```

Delete all three rules (lines 215–217):

```css
  .sk-spacer { flex: 1; }
  .sk-del { background: none !important; border: none !important; color: #6b7a8a !important; }
  .sk-del:hover { color: #ff8080 !important; }
```

- [ ] **Step 6: `ImageCell.svelte`**

Delete the prop (line 8):

```svelte
  export let onDelete = () => {}
```

Delete both of these lines (96–97). Do **not** touch the `✕ Cancel` button at
line 91 — that cancels a crop, not the cell:

```svelte
      <span class="im-spacer"></span>
      <button class="im-del" on:click={onDelete} title="Delete cell">✕</button>
```

Delete all three rules (lines 130–132):

```css
  .im-spacer { flex: 1; }
  .im-del { background: none !important; border: none !important; color: #6b7a8a !important; }
  .im-del:hover { color: #ff8080 !important; }
```

- [ ] **Step 7: `AudioCell.svelte`**

Delete the prop (line 8):

```svelte
  export let onDelete = () => {}
```

Delete the button (line 163). Keep `.au-spacer` on line 161:

```svelte
    {#if !readOnly}<button class="au-del" on:click={onDelete} title="Delete cell">✕</button>{/if}
```

Delete both rules (lines 204–205):

```css
  .au-del { background: none !important; border: none !important; color: #6b7a8a !important; }
  .au-del:hover { color: #ff8080 !important; }
```

- [ ] **Step 8: `MediaCell.svelte`**

Delete the prop (line 11):

```svelte
  export let onDelete = () => {}
```

Delete the button (line 101). There is no CSS rule to remove — it shared the
`.me-btn` class with Replace and Cancel, which stay:

```svelte
    {#if !readOnly}<button class="me-btn" on:click={onDelete} title="Delete cell">✕</button>{/if}
```

- [ ] **Step 9: Stop passing `onDelete` from `NotebookPane.svelte`**

Delete this line from the `<svelte:component>` call:

```svelte
          onDelete={() => deleteCell(i)}
```

- [ ] **Step 10: Confirm no `onDelete` remains**

```bash
cd /home/simone/projects/texere && grep -rn "onDelete" frontend/src/
```

Expected: **no output.** Any hit is a missed edit.

- [ ] **Step 11: Run the automated gates**

```bash
cd /home/simone/projects/texere && make test-js
cd /home/simone/projects/texere/frontend && npm run build
```

Expected: five test files green; build completes with **no
`Unused CSS selector` warning**. A warning naming any `*-del` or `*spacer` class
means a rule outlived its markup — go back and delete it.

- [ ] **Step 12: Verify in the running app**

```bash
cd /home/simone/projects/texere && make dev
```

Add one cell of each of the eight types. Confirm:
- no cell shows a `✕` inside its own bar; each shows exactly one, in the header;
- the header `✕` actually deletes, for all eight types;
- `ComputeCell`'s Run/Stop is still right-aligned, and `AudioCell`'s and
  `MediaCell`'s Replace still are;
- `ImageCell`'s crop mode still offers its `✕ Cancel` button;
- `SketchCell`'s and `ImageCell`'s toolbars are now left-packed with no trailing
  gap, which is expected.

- [ ] **Step 13: Commit**

```bash
cd /home/simone/projects/texere
git add frontend/src/components/notebook/
git commit -m "refactor(notebook): drop the eight per-cell delete buttons

One control implemented eight times, now implemented once in the
wrapper. Three spacers go with them — LLM, Sketch and Image had delete
as their only right-aligned item. Compute, Audio and Media keep theirs
for Run and Replace.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: Suppress the bars that Task 2 emptied, and the doubled `js`

`.au-bar` and `.me-bar` both carry `padding` plus a `border-bottom`, so an empty
one renders as an ~11px stub with a rule under it. Task 2 empties both in states
that are not corner cases: **every freshly added `+ Audio` cell** (no asset, so
no duration chip and no Replace) and **every freshly added `+ Media` cell** (no
licence, no content).

**Files:**
- Modify: `frontend/src/components/notebook/AudioCell.svelte`
- Modify: `frontend/src/components/notebook/MediaCell.svelte`
- Modify: `frontend/src/components/notebook/ComputeCell.svelte`

**Interfaces:**
- Consumes: `hasContent(cell)` from `frontend/src/lib/media-fact.js`, already
  imported at the top of `MediaCell.svelte` and already unit-tested in
  `frontend/src/lib/media-fact.test.js`. Do not re-import or reimplement it.
- Produces: nothing further depends on this task.

- [ ] **Step 1: `AudioCell.svelte` — gate `.au-bar` on `cell.asset`**

The bar is non-empty exactly when `cell.asset` is set: the duration chip needs
it, and so does Replace. Replace this block (post-Task-2 state, around line 159):

```svelte
  <div class="au-bar">
    {#if cell.asset}<span class="au-dur">{fmt(cell.durationMs)}</span>{/if}
    <span class="au-spacer"></span>
    {#if !readOnly && cell.asset}<button on:click={replace} title="Replace">Replace</button>{/if}
  </div>
```

with:

```svelte
  {#if cell.asset}
    <div class="au-bar">
      <span class="au-dur">{fmt(cell.durationMs)}</span>
      <span class="au-spacer"></span>
      {#if !readOnly}<button on:click={replace} title="Replace">Replace</button>{/if}
    </div>
  {/if}
```

The two inner `cell.asset` checks are now implied by the outer one, so they go
rather than being restated.

- [ ] **Step 2: `MediaCell.svelte` — gate `.me-bar`**

Replace this block (post-Task-2 state, around line 93):

```svelte
  <div class="me-bar">
    {#if cell.license}<span class="me-lic">{cell.license}</span>{/if}
    <span class="me-spacer"></span>
    {#if !readOnly && hasContent(cell) && !replacing}
      <button class="me-btn" on:click={replace} title="Pick a different asset">Replace</button>
    {/if}
    {#if !readOnly && replacing && hasContent(cell)}
      <button class="me-btn" on:click={() => (replacing = false)} title="Keep the current asset">Cancel</button>
    {/if}
  </div>
```

with:

```svelte
  {#if cell.license || (!readOnly && hasContent(cell))}
    <div class="me-bar">
      {#if cell.license}<span class="me-lic">{cell.license}</span>{/if}
      <span class="me-spacer"></span>
      {#if !readOnly && hasContent(cell) && !replacing}
        <button class="me-btn" on:click={replace} title="Pick a different asset">Replace</button>
      {/if}
      {#if !readOnly && replacing && hasContent(cell)}
        <button class="me-btn" on:click={() => (replacing = false)} title="Keep the current asset">Cancel</button>
      {/if}
    </div>
  {/if}
```

The inner conditions stay as they are: Replace and Cancel are mutually exclusive
on `replacing`, so the outer guard cannot subsume them.

- [ ] **Step 3: `ComputeCell.svelte` — remove the doubled `js`**

The header now reads `Compute · js`, so the in-bar label repeats it. This is the
same doubling removed from `MediaCell` and `AudioCell` in the cell-headers slice.

Delete this line (line 53):

```svelte
    <span class="cc-lang">js</span>
```

and its rule (line 76):

```css
  .cc-lang { color: #7dd3fc; font-size: 11px; font-family: monospace; }
```

Keep `.cc-spacer` — Run/Stop still needs to sit right.

- [ ] **Step 4: Run the automated gates**

```bash
cd /home/simone/projects/texere && make test-js
cd /home/simone/projects/texere/frontend && npm run build
```

Expected: five test files green; build completes with **no
`Unused CSS selector` warning** — in particular none for `.cc-lang`, and none
for `.au-dur`, `.au-spacer`, `.me-lic` or `.me-spacer`, which are still reachable
inside their new `{#if}` blocks.

- [ ] **Step 5: Verify in the running app**

```bash
cd /home/simone/projects/texere && make dev
```

Confirm:
- a fresh `+ Audio` cell shows **no** bar above its recording controls — no stub,
  no stray horizontal rule;
- after recording, the bar returns with the duration on the left and Replace on
  the right;
- a fresh `+ Media` cell shows **no** bar above the search panel;
- after picking a licensed asset, the bar returns with the licence and Replace;
- a Compute cell shows `js` once, in the header, and Run is still right-aligned.

- [ ] **Step 6: Commit**

```bash
cd /home/simone/projects/texere
git add frontend/src/components/notebook/
git commit -m "fix(notebook): hide the bars that emptied when delete moved out

A fresh Audio or Media cell had nothing left in its bar, and both bars
carry padding and a border-bottom, so each rendered as a stub with a
rule under it. Compute's 'js' goes too — the header says it now.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: Manual test plan

The slice's verification artifact, matching every prior notebook slice. There is
no automated coverage of this change beyond "it compiles and nothing regressed",
so this document is the real gate.

**Files:**
- Create: `docs/specs/2026-07-28-notebook-cell-chrome-consolidation-manual-test-plan.md`

**Interfaces:**
- Consumes: the finished behaviour from Tasks 1–3.
- Produces: nothing.

- [ ] **Step 1: Write the test plan**

Follow the house format — a `## TC-N <name>` heading per case, numbered steps,
and an indented `- EXPECT:` clause naming the concrete thing to see. Cases:

- **TC-1 Header layout, all eight types.** Add one cell of each of `+ N4L draft`,
  `+ LLM`, `+ Graph`, `+ Compute`, `+ Sketch`, `+ Scan`, `+ Audio`, `+ Media`.
  EXPECT: every header shows its name uppercase on the left and `▲ ▼` then a
  visible gap then `✕` on the right; the gap before `✕` is clearly wider than the
  gap between `▲` and `▼`; no cell has a gutter to its left.
- **TC-2 Cells are wider.** Compare against `main`. EXPECT: each cell's content
  starts flush with the header's left edge, having reclaimed the gutter's width.
- **TC-3 End-of-list buttons.** EXPECT: `▲` is dimmed and inert on the first
  cell, `▼` dimmed and inert on the last; with one cell only, both are dimmed.
- **TC-4 Reorder.** Move a middle cell up and down with `▲▼`. EXPECT: the cell
  moves, its header name moves with it, and its content (a sketch's strokes, an
  LLM's transcript) is preserved rather than remounted empty.
- **TC-5 Delete, all eight types.** EXPECT: the header `✕` removes the cell; no
  cell shows a second `✕` in its own bar.
- **TC-6 Read-only notebook.** Reopen a notebook read-only. EXPECT: every header
  shows its name and **no** controls at all — no `▲▼`, no `✕` — on all eight
  types, including `N4LDraftCell`, `ImageCell` and `SketchCell`, whose own bars
  are hidden entirely in read-only.
- **TC-7 Empty-bar suppression, Audio.** Add `+ Audio` without recording.
  EXPECT: no bar and no horizontal rule between the header and the recording
  controls. Then record: EXPECT the bar returns with duration left, Replace
  right.
- **TC-8 Empty-bar suppression, Media.** Add `+ Media`. EXPECT: no bar above the
  search panel. Pick a licensed Commons asset: EXPECT the bar returns with the
  licence left and Replace right. Reopen read-only: EXPECT the bar still shows
  the licence, with no Replace.
- **TC-9 Compute label.** EXPECT: `js` appears once, in the header as
  `COMPUTE · JS`, and Run/Stop is still right-aligned in the cell's own bar.
- **TC-10 Image crop cancel survives.** Enter crop mode on a `+ Scan` cell.
  EXPECT: the `✕ Cancel` button is present and cancels the crop — it was not
  collected by the delete-button sweep.
- **TC-11 Persistence.** Reorder and delete cells, wait past the 600ms debounce,
  close and reopen the notebook. EXPECT: the order and deletions persisted.

- [ ] **Step 2: Execute the plan against `make dev` and record results**

Run every case. Note any that cannot be exercised without a GUI, marking them
`*(Deferred to human verification)*` as prior plans do rather than claiming a
pass.

- [ ] **Step 3: Commit**

```bash
cd /home/simone/projects/texere
git add docs/specs/2026-07-28-notebook-cell-chrome-consolidation-manual-test-plan.md
git commit -m "docs(test-plan): cell chrome consolidation

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

## Spec coverage

| Spec requirement | Task |
|---|---|
| Header becomes a flex row, name + `▲ ▼` + `✕` | 1 |
| Single `{#if !readOnly}` replacing nine guards | 1, 2 (step 9) |
| Visible gap between `▲▼` and `✕` as the sole mis-click protection | 1 |
| `▲`/`▼` disabled at the ends | 1 |
| `.nb-reorder` gutter deleted; `.nb-cellwrap` no longer flex | 1 |
| `✕` calls `deleteCell(i)` directly | 1 |
| Header keeps its floating treatment (no background) | 1 (constraint) |
| Eight delete buttons + CSS + `onDelete` props removed | 2 |
| Three dead spacers removed, three kept | 2 |
| `onDelete` no longer passed to `svelte:component` | 2 (step 9) |
| `.au-bar` gated on `cell.asset` | 3 |
| `.me-bar` gated on `cell.license \|\| (!readOnly && hasContent(cell))` | 3 |
| `.cc-lang` removed | 3 |
| No new unit tests; manual test plan is the verification | 4 |
| Out of scope: undo, collapse, icons, drag | Global Constraints |
