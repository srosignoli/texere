# Process mode: the notebook follows the caret

**Date:** 2026-08-03
**Status:** Approved design
**Feature area:** Process mode — notebook pane

## Summary

In Process mode the notebook pane follows the caret: it shows the notebook of
whatever node you are reading, or an empty state when that node has none. The
action bar does not follow — Promote, Cloze, Later and Dismiss keep acting on the
extract the study queue sent you to.

The feature is read-only. It creates no notebook and writes to no file.

## Motivation

`ProcessView` receives a fixed `extractId` from the study queue and passes it to
both the editor and the notebook pane. Moving the caret to another extract in the
same file changes nothing, so the right-hand pane keeps showing the *first*
extract's notebook while you read a different one, with nothing saying so.

In an application whose purpose is binding notes to specific nodes, displaying
the wrong binding is worse than displaying none.

## Grounding in the current implementation

Surveyed before designing:

- **`ProcessView.svelte:34`** renders `<NotebookPane nodeID={extractId} …>`.
  `extractId` is a prop from `StudySurface.svelte:107`
  (`{#key processId}<ProcessView extractId={processId} …>{/key}`) and is fixed for
  the whole processing session.
- **`ProcessEditor` does not track the caret.** It scrolls to the extract's line
  on load and dispatches on save, but nothing observes selection changes.
- **`OpenNotebook` CREATES when none exists** (`notebook_methods.go`):
  `GetNotebookByExtract(nodeID)` and, when absent, builds a `learning.Notebook`
  and calls `UpsertNotebook`.
- **`GetNotebookByExtract` is misnamed.** It is a plain lookup —
  `SELECT … FROM notebooks WHERE extract_node_id=?` — with no validation that the
  node is a registered extract. Notebooks are already keyed by *node id*.
- **Only study-marked nodes get an `# @id`.** `InjectMissingIDs`
  (`internal/content/writeback.go`) skips any node where `node.StudyMeta == nil`,
  so ordinary notes a user types carry no id at all.
- **`NotebookPane` has two load paths**: `nodeID` → `OpenNotebook` (creates), and
  `notebookId` → `GetNotebook` (reads). `readOnly` is an independent prop;
  `SourceTab.svelte:364` merely happens to pass both together.
- **There is no Svelte component test harness.** `frontend/src/lib/*.test.js` run
  under `node --test` via `make test-js`.

## Decisions (from brainstorming)

- **The notebook follows the caret; nothing else does.** Rejected: leaving the
  pane pinned and merely labelling the mismatch, which stops the current
  behaviour lying but does not do what was asked.
- **The action bar never retargets.** One principle covers every caret position,
  and the study queue can never be advanced or resolved by accident. Rejected:
  retargeting the whole view, where `Later`/`Dismiss` could resolve an extract the
  queue never offered.
- **The discriminator is "has an id", not "is an extract".** This came out of the
  user's observation that a `.n4l` file may contain anything, not only extracts.
  Since notebooks are keyed by node id with no extract validation, any id-bearing
  node can carry one; a node without an id has no handle and can carry none.
- **No "Start a notebook" anywhere.** The processing queue contains only extracts
  awaiting processing, so an extract with no notebook is simply one not yet
  processed — the queue is already the way to start one, and a second path would
  be redundant. This also keeps the feature from ever writing an `# @id` into a
  user's notes.
- **A peeked notebook is editable.** It is that node's own notebook and edits
  belong to it, so there is no cross-wiring.

## Architecture

### The rule

| caret is on | notebook pane | action bar |
|---|---|---|
| the queued extract | its notebook | queued extract |
| another node with an id that has a notebook | that notebook | queued extract |
| another node with an id, no notebook | empty state | queued extract |
| a node with no id, or not a node | empty state | queued extract |

The last two rows render the same empty state. Telling "an unprocessed extract"
apart from "a node that cannot carry a notebook" would need a further lookup to
decide whether the node is a registered extract, and the distinction changes
nothing the user can do from here.

### `notebook_methods.go` — two new bindings

**`PeekNotebook(nodeID string) (*NotebookView, error)`** — `OpenNotebook` without
the creation:

```go
nb, ok, err := a.learnStore.GetNotebookByExtract(nodeID)
if !ok {
    return nil, nil // absent is the normal case, not an error
}
```

A nil result marshals to JSON `null`, which the caller reads as "show the empty
state". **Absence must not be an error**: it is the common case, and an error
would light the pane's error banner every time the caret rests on a plain line.

**This binding is the load-bearing part of the design.** Reusing `OpenNotebook`
would create a notebook row for every node the caret passes over, and
`GetNotebookLinks` would then mark those lines in the file browser as having
notebooks. The feature would defeat itself: after one visit a node *would* have a
notebook, so it would start showing one.

**`FileNodeSpans(filePath string) ([]NodeSpan, error)`** where
`NodeSpan{ID string; FromLine, ToLine int}` — the line→id mapping the editor
lacks. A fold over an existing parse: each `Node` contributes `Line` through its
last ditto's line. Nodes with an empty `ID` are omitted, since they cannot key a
notebook.

### Why a map rather than a query per keystroke

Caret movement is constant. Asking the backend "which node is at line N?" on each
arrow key would be a stream of IPC and file parses for a read that changes
nothing. Fetching spans once per file and resolving locally makes caret movement
free.

**The staleness this accepts.** Editing shifts line numbers, so the map goes stale
between saves — insert five lines and every span below is wrong by five. Two
things make that tolerable: the map is refetched after each save, and the failure
mode is benign and visible (you briefly peek the wrong node's notebook) rather
than silent, and it self-corrects.

Rejected: deriving spans from the buffer in JavaScript to avoid staleness
entirely. That means reimplementing the parser's notion of where a node ends, in
a second language, with no test against the real parser.

### Frontend

**`ProcessEditor.svelte`** gains a debounced CodeMirror `updateListener` that
dispatches the caret's **line number** and nothing more. It stays ignorant of
nodes and notebooks.

**`ProcessView.svelte`** owns the rest: it holds the spans, resolves line→id
locally, peeks when the resolved id changes, and chooses what to render.

```
focused node === queued extract  →  <NotebookPane nodeID={extractId}>    (OpenNotebook — creates, as today)
peek returned a notebook         →  <NotebookPane notebookId={peeked}>   (GetNotebook — reads)
otherwise                        →  the empty state
```

**No new mode on `NotebookPane`.** Both paths already exist, and creation stays
exactly where it was, because the first branch is untouched. `{#key focusedNodeId}`
wraps the pane so switching remounts rather than reconciling two notebooks in one
instance.

**Debounce the peek, not only the caret** (~150ms). `assembleContext(nodeID)`
builds the orbit from the graph, so resting on a line should fetch once and
scrolling past fifty lines should fetch once at the end.

**The header** names both subjects when they differ — what is being processed,
what is being read. This is what keeps the anchored action bar honest rather than
merely quiet.

### `frontend/src/lib/node-span.js` (new, dependency-free)

`nodeIdAtLine(spans, line) -> string | null`. The only real logic on the frontend,
extracted so it is testable at all: this repo has no component test harness, so
logic left inside a `.svelte` file has no tests, and `llm.js` showed what happens
when a testable function sits beside an untestable import.

## Consequences

- **The right pane moves as you navigate.** A notebook you were mid-thought in
  slides away when you click elsewhere in the file. Cells persist on a 600ms
  debounce, so nothing is lost, but the movement is real.
- **You can edit another node's notebook while the action bar sits on the queued
  extract.** Deliberate: the header says so, and the edits go to the right
  notebook.
- **The span map is briefly wrong while typing.** Bounded by the next save.
- **Push provenance now follows the pane.** `ProcessView.handlePush` records
  `RecordPush(notebookId, …)` using the id captured from the pane's `on:ready`.
  With the pane remounting per focused node, that id tracks whichever notebook is
  displayed — which is correct, since that notebook is what pushed the text. Two
  details the implementation must get right: `on:ready` has to fire on every
  remount for the id to stay accurate, and the id must be **cleared when the empty
  state renders**, or a stale value from the previously shown notebook survives
  with no pane mounted. Nothing can push from an empty state today, so a stale id
  is currently unreachable rather than harmless — clear it anyway, because the
  next person to add an affordance there will not know it was load-bearing.

## Testing

**Go unit:**

- `FileNodeSpans` over a fixture containing a node with dittos, a node without an
  id (omitted), consecutive nodes, and a file with no nodes at all.
- `PeekNotebook`: returns the notebook when one exists; returns `nil, nil` — not
  an error — when it does not; and **creates nothing**, asserted by peeking a
  fresh node id and then confirming no row exists.

**JS unit (`node-span.test.js`):** `nodeIdAtLine` for a line on a node, a line
inside its dittos, a line between nodes, a line before the first node, a line past
the end (after an edit shortened the file), an empty span list, and a span list
with one entry.

**Manual** — its own test-plan document, matching prior slices: the pane follows
the caret across extracts; an id-less line shows the empty state; the action bar
stays on the queued extract and the header says so; Promote still targets the
queued extract while the caret is elsewhere; a peeked notebook is editable and
persists; **no notebook row is created by browsing** (verified in `learning.db`,
since this is invisible in the UI); and the staleness window — type several lines
above an extract, move the caret, observe the wrong notebook briefly, save, and
confirm it corrects.

## Out of scope

- Creating a notebook from the pane.
- Injecting `# @id` into id-less notes.
- Renaming `GetNotebookByExtract`.
- Any change to the study queue, to what the action bar targets, or to
  `SourceTab`'s use of `NotebookPane`.
- Distinguishing "unprocessed extract" from "cannot carry a notebook" in the
  empty state.
