# One meaning for a notebook dot — design

Date: 2026-10-21 · Branch: `104-notebook-dots` · Vocabulary: `CONTEXT.md` (Notebook, Push, Extract)

## Problem

Two systems draw the same dot in the gutter, and they mean different things:

- **The note editor** (`SourceTab.svelte:43`) uses `NotebookMarks`: a dot means *this node has a Notebook*. The mark is keyed by node id and survives edits, and clicking the dot opens the Notebook.
- **Process mode** (`ProcessEditor.svelte:48`) uses `GetNotebookLinks`: a dot means *this text was pushed out of Notebook X*. The mark is keyed by `(chapter, node_text)`, so any edit to the line drops it. Clicking it does nothing (`notebookDotsGutter(() => {})`, :179).

`notebook_methods.go:347-352` says `NotebookMarks` "replaces" `NotebookLinkMark`, but the old system is still live:
- `RecordPush` (:315) writes `notebook_links` after every Push from ProcessView (`ProcessView.svelte:80`), and `GetNotebookLinks` (:403) is that table's only reader;
- `SourceTab` listens for `notebook:links-changed` (:435), but only to refresh its own `NotebookMarks`.

Resolving the caret to a node and then to that node's Notebook is written twice: `SourceTab.svelte:76-142` and `ProcessView.svelte:16-57`. The two copies have drifted apart:

| | SourceTab | ProcessView |
|---|---|---|
| Node spans couldn't be read | `null` | falls back to the queued Extract's id |
| When it asks the backend | only once the overlay is open | always |
| Debounce (150 ms) | in SourceTab | in ProcessEditor (:30-37) |
| Ignores a late reply for a node already left | on both success and error | on success only (:56) |
| Re-checks when a Notebook is created | yes (`notebook:changed`, :442-450) | no |

Neither copy can be tested: both are Svelte components.

`ProcessView.svelte:22` says node ids are fully qualified, `<path>:<chapter>:id:<short>`. They are bare short ids (`internal/content/nodeid.go`, `notebook_methods.go:260-261`), so its `shortId()` slice does nothing.

## Decisions (agreed)

1. **A dot means one thing in every editor: this node has a Notebook.** Process mode moves to `NotebookMarks`.
2. **A dot click moves the caret to that line.** The Notebook view follows the caret, as in the note editor, so there is no second way to open a Notebook.
3. **Process-mode dots refresh** on load, after every save (a save shifts lines, and a Push saves), and on `notebook:changed`.
4. **Push provenance is removed.** Deleted:
   - bindings: `RecordPush`, `GetNotebookLinks`;
   - store methods: `AddNotebookLinks`, `LinksForFile`;
   - types: `learning.NotebookLink`, `NotebookLinkMark`;
   - the event and its emitter: `notebook:links-changed`, `notebookLinksChanged`;
   - the push-key matcher that serves `RecordPush` (`notebook_methods.go` ~:270-310);
   - callers: ProcessView's `RecordPush` call and SourceTab's listener;
   - the tests that cover all of these.
5. **Schema v9 drops `notebook_links`** and its index. Existing rows are deleted permanently.
   - The v7 relative-path step must tolerate the table being absent, because a database re-migrated from an earlier version may no longer have it.
   - Fresh databases still pass through ddlV5, then v9 drops the table again. Old migration steps stay as they were.
6. **One caret→Notebook module**, `frontend/src/lib/caret-notebook.js`, with no Svelte imports, so Node's test runner can reach it. It owns:
   - the 150 ms caret debounce;
   - resolving the line to a node via `nodeIdAtLine`;
   - re-checking when spans arrive or change;
   - ignoring a late reply, on success and on error;
   - a forced re-check when a Notebook is created.

   The two real differences become caller options:
   - `fallbackId`: used when the spans are empty. Process passes the queued Extract.
   - `ownId`: an id the caller shows itself and that is never peeked. Process passes the queued Extract.
   - `shown`: when false, nothing is peeked. The note editor peeks only while its overlay is armed.
7. **SourceTab and ProcessView both use the module.** ProcessEditor's debounce goes, and it reports every caret move.
8. **The stale "fully qualified" comment and `shortId()` go.**

## Out of scope

- Push provenance keyed by node id. If it is ever wanted, it gets designed on its own.
- `OpenNotebook` / `GetNotebook` / `PeekNotebook` rebuilding `NotebookView` three times (architecture review, finding 5). Not part of this change.
- The notebook cell schema (architecture candidate 6).
