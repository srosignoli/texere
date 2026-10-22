# Notebook Dots Implementation Plan

**Goal:** Make a gutter dot mean only "this node has a Notebook" in every editor, delete the text-keyed push-provenance system, and resolve caret→Notebook in one tested module.

**Architecture:** Process mode switches from `GetNotebookLinks` to `NotebookMarks`. The provenance path is deleted end to end (bindings, store, event, table), and schema v9 drops `notebook_links`. A new `frontend/src/lib/caret-notebook.js` takes over the debounce, span resolution and stale-reply logic that `SourceTab` and `ProcessView` each implement today, with the two real differences as options.

**Tech Stack:** Go 1.25, SQLite (`internal/learning`), Wails v2, Svelte 3.49, `node:test`.

**Spec:** `docs/specs/2026-10-21-notebook-dots-design.md`. Decisions are cited as **D1–D8**.

## Global Constraints

- Branch `104-notebook-dots`. In a worktree, copy `frontend/dist` in and link `frontend/node_modules` (gitignored; the symlink also needs a local exclude).
- `frontend/wailsjs` is gitignored but tracked: stage it with `git add -u`, never `git add <dir>`.
- `frontend/src/lib/caret-notebook.js` must import nothing from `.svelte` files or `wailsjs`. Its backend call is passed in.
- The debounce is exactly 150 ms.
- Vocabulary is from `CONTEXT.md`: Notebook, Push, Extract.
- `make test`, `make test-js`, `make lint` and `cd frontend && npx vite build` (with no `A11y:` lines) pass at every commit.

## Review Focus

1. **An old database at schema 6 or 7 upgrading.** Expected: v7 rewrites the `notebook_links` paths while the table still exists, then v9 drops it, and Open succeeds. *(Test in Task 1.)*
2. **The idempotency test's re-run from version 6 over a v9 database**, where `notebook_links` is gone. Expected: no error. *(Test in Task 1.)*
3. **A caret move, then a slow peek, then another move, with the first reply failing late.** Expected: the failure does not clear the Notebook of the node now under the caret. *(Test in Task 2.)*
4. **A file whose spans cannot be read** (an empty list). Expected: Process falls back to the queued Extract, and the note editor shows nothing, with no peek. *(Test in Task 2.)*
5. **A Push in Process mode.** Expected: the file saves, the dots refresh at the shifted lines, and no `RecordPush` call is made. *(Manual check in Task 3.)*

---

### Task 1: One dot meaning; provenance and its table removed

**Files:**
- Modify: `notebook_methods.go`, removing `notebookLinksChanged` (:249-255), `pushedNodeKeys` (~:270-310), `RecordPush` (:312-337), `NotebookLinkMark` (:339-344), `GetNotebookLinks` (:401-458), and the `NotebookLinkMark` mention in the `NotebookMark` comment (:347-352)
- Modify: `internal/learning/notebooks.go` (remove `NotebookLink`, `AddNotebookLinks` and `LinksForFile`, :76-120), `internal/learning/db.go` (`schemaVersion = 9`, plus a `case 8` that drops the table), `internal/learning/paths.go` (`migrateColumnToRelative` skips a missing table)
- Modify tests:
  - `notebook_methods_test.go`: delete `TestPushedNodeKeys` and `TestRecordPushAndGetLinks`;
  - `internal/learning/notebooks_test.go`: delete the `AddNotebookLinks`/`LinksForFile` test;
  - `internal/learning/db_test.go`, `internal/learning/paths_test.go`: see Step 1.
- Modify: `frontend/src/components/ProcessEditor.svelte` (dots from `NotebookMarks`, a click moves the caret, refresh on `notebook:changed`), `ProcessView.svelte` (drop the `RecordPush` call), `SourceTab.svelte` (drop the `notebook:links-changed` listener, :435-437, and `cancelLinksChanged`)
- Regenerate: `frontend/wailsjs` (`wails generate module`)

**Interfaces:**
- Produces: schema version 9 with no `notebook_links` table. No `RecordPush` or `GetNotebookLinks` bindings.

- [ ] **Step 1: Write and adjust the failing tests:**
  - `db_test.go` `TestMigrate_V5_NotebookTables` becomes `TestNotebookTablesAfterMigrate`: `notebooks` exists, `notebook_links` does **not**, and the version is 9.
  - New `db_test.go` `TestMigrateV9DropsNotebookLinks`: a database at schema 8 (built with ddlV1..V6 and `version=8`) that has one `notebook_links` row. After `Open`, the table is absent and `schema_meta.version == 9`.
  - `paths_test.go` `TestMigrationRewritesAllFourColumns`: keep seeding `notebook_links` at schema 6, and remove its post-Open check. Rename it `TestMigrationRewritesThePathColumns`. A comment says the table is dropped at v9, after v7 has rewritten it.
  - `paths_test.go` `TestMigrationIsIdempotent`: remove `notebook_links` from `dump()`. The re-run from version 6 must return no error now that the table is gone (Review Focus 2).
- [ ] **Step 2:** `go test ./internal/learning`. Expected: FAIL. The table still exists, the version is 8, and the idempotency re-run passes only because the table is still there.
- [ ] **Step 3: Implement.**
  - `case 8:` runs `DROP INDEX IF EXISTS idx_nblinks_file; DROP TABLE IF EXISTS notebook_links;`.
  - `migrateColumnToRelative` checks `sqlite_master` for the table first and returns nil when it is absent.
  - Delete the Go symbols listed under Files, then the frontend callers.
  - ProcessEditor:
    - `refreshDots()` calls `NotebookMarks(target.filePath)`;
    - `notebookDotsGutter(onDotClick)` moves the selection to the start of the line and focuses the view, so the caret report follows;
    - an `EventsOn('notebook:changed', refreshDots)` is registered in `onMount` and cancelled in `onDestroy`;
    - the existing refreshes after load and save stay.
- [ ] **Step 4:** `go test -race ./... && make lint`. Expected: PASS. Then run `wails generate module`, `make test-js` and `cd frontend && npx vite build`. Expected: PASS, with no `A11y:` lines. `grep -rn "RecordPush\|GetNotebookLinks\|links-changed\|NotebookLink\b" --include=*.go --include=*.svelte --include=*.js . | grep -v wailsjs` finds nothing.
- [ ] **Step 5:** Commit `feat(notebook): a dot means the node has a Notebook, everywhere; push provenance and notebook_links go`.

---

### Task 2: `lib/caret-notebook.js`

**Files:**
- Create: `frontend/src/lib/caret-notebook.js`, `frontend/src/lib/caret-notebook.test.js`

**Interfaces:**
- Consumes: `nodeIdAtLine` from `./node-span.js`.
- Produces:
  ```js
  // peek(id) → Promise<NotebookView|null>. onFocus({ id, notebook }) is called whenever the
  // focused node or its Notebook changes; notebook is null for none / not peeked / ownId.
  export function createCaretNotebook({ peek, onFocus, fallbackId = null, ownId = null, shown = true, delayMs = 150 })
  // → {
  //   caret(line)               — debounced by delayMs; repeated same-line reports are ignored
  //   resolve(line, force=false) — immediate; force re-peeks the same node
  //   setSpans(spans, line?)    — replaces the map and resolves at line (default: last line)
  //   setShown(bool)            — true resolves the current line with force; false clears the notebook
  //   refresh()                 — resolve(last line, true): a Notebook was created
  //   reset()                   — a new file: clears spans, focus, last line and any pending timer
  //   destroy()                 — cancels the pending timer
  // }
  ```

- [ ] **Step 1: Write the failing tests**, using a fake `peek` (deferred promises) and a fake timer (`delayMs: 0` plus `await` on a tick, or inject a `setTimeout`):
  - `resolves the node under the caret and peeks its notebook`: spans `[{id:'a',fromLine:1,toLine:3}]`, `resolve(2)` → `peek('a')` once, and `onFocus({id:'a', notebook})`.
  - `a late reply for a node already left is ignored`: resolve to a, then resolve to b; a's peek resolves last; the final focus is b's notebook.
  - `a late failure for a node already left does not clear the current notebook` (Review Focus 3): a's peek rejects after b's has resolved; the focus is still b's notebook.
  - `empty spans fall back to fallbackId`: `fallbackId:'x'`, `setSpans([])`, `resolve(5)` → focus id `'x'`.
  - `empty spans without a fallback focus nothing and peek nothing` (Review Focus 4).
  - `ownId is focused but never peeked`: `ownId:'x'` → `onFocus({id:'x', notebook:null})`, no `peek` call.
  - `nothing is peeked until shown`: `shown:false`, `resolve(2)` → focus id `'a'`, notebook null, and no peek. Then `setShown(true)` → a peek of a.
  - `the same node is not re-peeked unless forced`: two `resolve` calls on lines inside a → one peek. `refresh()` → a second peek.
  - `caret reports are debounced, and the last line wins`: `caret(1)`, `caret(2)`, `caret(3)` inside the delay → one resolve, at 3.
  - `setSpans re-resolves at the last caret line`: `caret(2)` before any spans → nothing focused; `setSpans(spans)` → a is focused.
  - `reset forgets the file`: after `reset()`, a pending debounced caret never fires.
- [ ] **Step 2:** `node --test frontend/src/lib/caret-notebook.test.js`. Expected: FAIL (module not found).
- [ ] **Step 3: Implement** `createCaretNotebook` in `caret-notebook.js`. A focus token (the id it was asked for) is compared after every `await`, on both the resolve and the reject path.
- [ ] **Step 4:** `make test-js`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(notebook): caret→Notebook resolution in one tested module`.

---

### Task 3: SourceTab and ProcessView use the module

**Files:**
- Modify: `frontend/src/components/SourceTab.svelte`: the caret state, `loadSpans`, `resolveCaret` and `reportCaret` (:74-142), the file-switch reset (:170-178), the `notebook:changed` re-resolve (:446-450) and the `onDestroy` timer clear (:465)
- Modify: `frontend/src/components/ProcessView.svelte`: `spans`/`focusedId`/`peeked` and `resolve` (:16-57); delete `shortId` and its comment (:21-23) and use the id directly where it was shown
- Modify: `frontend/src/components/ProcessEditor.svelte`: `reportCaret` dispatches `caret` on every line change, with no timer (:26-37)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `createCaretNotebook` (Task 2).

- [ ] **Step 1: SourceTab.**
  - Build one `createCaretNotebook({ peek: PeekNotebook, shown: false, onFocus })`. `onFocus` sets `focusedId` and `focusedNb`, and resets `reopenReadOnly` when the id changes.
  - Arming the overlay calls `setShown(true)`, and Close calls `setShown(false)`.
  - `loadSpans(path)` calls `setSpans(spans, actualCaretLine)`, `reportCaret` calls `caret(line)`, and a file switch calls `reset()`.
  - The `notebook:changed` handler calls `refresh()` when armed, and `onDestroy` calls `destroy()`.
- [ ] **Step 2: ProcessView.** Build `createCaretNotebook({ peek: PeekNotebook, fallbackId: extractId, ownId: extractId, onFocus })`. `onFocus` sets `focusedId`, `peeked` and `peekedFor`. `onCaret` calls `caret(line)`, `loadSpans` calls `setSpans`, and it subscribes to `notebook:changed` → `refresh()`. Delete `shortId`.
- [ ] **Step 3: ProcessEditor.** Remove `caretTimer` and dispatch `caret` directly. The same-line guard stays.
- [ ] **Step 4:** `make test-js && cd frontend && npx vite build`. Expected: PASS, with no `A11y:` lines. `grep -n "caretTimer\|shortId" frontend/src/components/{SourceTab,ProcessView,ProcessEditor}.svelte` finds nothing.
- [ ] **Step 5: Manual check** in `make dev`:
  1. In the note editor, click a dot → the overlay opens on that node and moving the caret follows; Close stays closed.
  2. In Process mode → the dots match the note editor's for the same file, and clicking one moves the caret and shows that Notebook.
  3. Push a draft → the file saves and the dots land at the shifted lines (Review Focus 5).
  4. Create a Notebook on a node in Process mode → its dot appears in both editors without a reload.
- [ ] **Step 6:** Add a `CHANGELOG.md` line under v0.2.0 → Fixed: "Process mode's notebook dots vanished on any edit and did nothing when clicked. Every editor's dot now means the node has a notebook, and clicking it moves to that node."
- [ ] **Step 7:** Commit `refactor(notebook): SourceTab and ProcessView resolve the caret through one module`.
