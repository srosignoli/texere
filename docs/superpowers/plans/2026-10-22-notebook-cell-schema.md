# Notebook Cell Schema Implementation Plan

**Goal:** Put the cell schema and the notebook autosave where Node's test runner can reach them. Media picks, attach labels and failed saves then behave the way the schema says.

**Spec:** `docs/superpowers/specs/2026-10-22-notebook-cell-schema-design.md`. Decisions are cited as **D1–D5**.

## Constraints

- Branch `109-notebook-cell-schema`, in a worktree. Copy `frontend/dist`, symlink and locally exclude `frontend/node_modules`, and run `wails generate module` for `wailsjs/runtime`.
- `lib/cell-schema.js` and `lib/autosave.js` import nothing from `.svelte` files or `wailsjs`.
- Test-first, one slice at a time, with `node:test` (`make test-js`).
- `make test-js`, `make lint-frontend` and `make test` pass at every commit.

## Seams

- `lib/cell-schema.js`: the factories and `pickMediaAsset`.
- `lib/autosave.js`: `createAutosave`.

---

### Task 1: `cell-schema.js`

**Slices:**
1. `each factory makes a cell of its own type with a unique id`: the nine factories give their types (`n4l` … `shell`), and 20 calls give 20 distinct ids.
2. `a new media cell already has every field a pick writes`: every key `pickMediaAsset` sets on a cell is present in `newMediaCell()`. The test gets the keys by picking onto `{}` and reading `Object.keys`. This catches `durationMs`.
3. `a pick swaps the asset and forgets the old framing`: given a cell with a crop, a natural size, a video title and a start time → the new `asset`/`mime`/`kind`, with `crop` null, `naturalW`/`naturalH` 0, `startSec` 0, and the video fields taken from the result (or '').
4. `a pick takes the subject from the query only when there is none`: an empty subject gets the trimmed query, and an existing subject is kept.
5. `a pick keeps the provider when the result names none`: `result.provider` undefined → the old provider is kept.

Then `notebook.js` keeps only `cellRegistry` and imports nothing else. `NotebookPane` imports the factories from `cell-schema.js`. `MediaCell.onPick` calls `pickMediaAsset(cell, result, ref)` and keeps its own UI state (`replacing`, `cropping`, `persist()`). `LLMCell` uses `ATTACHABLE_TYPES` and `` `${cellLabel(c)} ${c.id.slice(-4)}` ``.

**Check:** `make test-js && make lint-frontend`. `grep -n "c.type === 'n4l' ?" frontend/src/components/notebook/LLMCell.svelte` finds nothing.

**Commit:** `refactor(notebook): the cell schema is one testable module`.

---

### Task 2: `autosave.js`

**Slices** (with injected `timers`, as `caret-notebook.js` does):
1. `a burst of changes saves once, after the delay`: `schedule()` ×3, then the timer fires → `save` is called once.
2. `a failed save is reported`: `save` rejects → `onError(err)`, and no `onSaved`.
3. `the next successful save reports recovery`: after a failure, schedule and fire → `onSaved()`.
4. `flush saves now when a save is pending, and only then`: pending → `save` is called at once and the timer is cancelled; nothing pending → no call.
5. `flush reports its failure to onError too`.

Then `NotebookPane`:
- `const autosave = createAutosave({ save: () => SaveNotebook(nbId, JSON.stringify(cells)), onError: (e) => error = 'Notebook not saved: ' + msg(e), onSaved: () => { if (error.startsWith('Notebook not saved')) error = '' } })`;
- `scheduleSave()` calls `autosave.schedule()` unless the notebook is `readOnly`;
- `onDestroy` calls `autosave.flush()` with an `onError` that logs to `console.error`. The pane is gone, so it replaces the banner handler by passing a flag or a second instance.

**Check:** `make test-js && make lint-frontend`. `grep -n "catch(() => {})" frontend/src/components/notebook/NotebookPane.svelte` finds no `SaveNotebook`.

**Commit:** `fix(notebook): a failed autosave is shown, not swallowed`.

## Manual check

In `make dev`:
1. Add each cell type → each appears.
2. In a media cell, search and pick a picture, crop it, pick another → the crop is gone.
3. An LLM cell's attach list reads "N4L draft 1a2b" and "Scan 9f3c".
4. Make the Collection's database read-only (`chmod a-w learning.db`) and edit a cell → "Notebook not saved: …" appears. Restore the permission and edit again → the banner clears.
