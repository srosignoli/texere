# Notebook cell schema — design

Date: 2026-10-22 · Branch: `109-notebook-cell-schema` · Vocabulary: `CONTEXT.md` (Notebook) · Architecture review candidate 6

## Problem

The schema of a Notebook's cells exists only implicitly. Go stores cells as opaque JSON (`notebook_methods.go`, `CellsJSON string`), so the schema is whatever the frontend writes:

- **The factories can't be tested.** All nine factories live in `lib/notebook.js`, which also imports nine `.svelte` components for `cellRegistry`. Node's test runner can't load those components, so nothing in that file can be tested (see the comments heading `cell-label.js` and `cell-preview.js`).
- **The media reset copies the schema by hand.** `MediaCell.onPick` (`components/notebook/MediaCell.svelte:30-53`) resets 14 fields by hand, mirroring `newMediaCell`. It also sets `durationMs`, which the factory never defines, so the two have already drifted apart.
- **The LLM cell names cells differently from their headers.** Its "Attach context" list (`LLMCell.svelte:196`, written out twice) builds labels with its own conditionals ("draft", "image"), while the headers say "N4L draft" and "Scan". The list of attachable types is also written out twice in that line.
- **Failed autosaves are invisible.** `NotebookPane.svelte` saves with `SaveNotebook(...).catch(() => {})` in both `scheduleSave` and `onDestroy`. A failing autosave is never shown, and the edits exist only in memory.

## Decisions (agreed)

1. **`lib/cell-schema.js`**, with no `.svelte` or `wailsjs` imports, holds:
   - the nine factories (`newDraftCell` … `newShellCell`), moved unchanged with their names kept;
   - `pickMediaAsset(cell, result, ref)`, the asset swap `MediaCell.onPick` does today;
   - `ATTACHABLE_TYPES`: `['n4l', 'llm', 'graph', 'compute', 'sketch', 'image']`.

   `lib/notebook.js` keeps only `cellRegistry`.
2. **`newMediaCell` defines `durationMs: 0`.** Saved notebooks are unaffected, since every pick already writes the field.
3. **The attach list uses `ATTACHABLE_TYPES` and `cellLabel`.** "draft" becomes "N4L draft" and "image" becomes "Scan", the same word as the cell's header.
4. **`lib/autosave.js`: `createAutosave({ save, delayMs = 600, onError, onSaved, timers })`**, returning `{ schedule(), flush(), cancel() }`. `NotebookPane` uses it:
   - a failed save shows "Notebook not saved: <error>" in the existing error banner;
   - the next successful save clears it;
   - edits stay in memory;
   - a failure on close (`flush()` in `onDestroy`) goes to `console.error`, since nothing can show it.
5. **Tests (Node).**
   - **Factories:** each returns its type, ids are unique, and the media factory has every field `pickMediaAsset` writes.
   - **`pickMediaAsset`:** its behaviour.
   - **Autosave:** one save per burst, errors reported, recovery, and `flush()`.

   The Svelte wiring is checked with the build and by hand.

## Out of scope

- The per-cell `persist()` wrappers: deleting them would only move the code.
- A Go-side schema for cells: they stay opaque JSON.
- Profile loading copied across `LLMCell`, `TranscribeBar` and `AudioCell` (architecture review finding 6), which is left for later.
