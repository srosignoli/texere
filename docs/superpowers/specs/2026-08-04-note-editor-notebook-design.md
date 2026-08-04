# The note editor's notebook follows the caret

**Date:** 2026-08-04
**Status:** Proposed — awaiting approval
**Feature area:** Note editor (`SourceTab`) — notebook overlay and gutter dots

## Summary

Two changes to the note editor, driven by one report: the gutter dot did not
mean what it appeared to mean, and the read-only notebook overlay kept showing a
notebook for a note you were no longer on.

1. **A gutter dot now means "this node has a notebook."** It is keyed by node id.
2. **Once opened, the overlay follows the caret** — showing the focused node's
   notebook, hiding when that node has none. Closing it keeps it closed until
   another dot is clicked.

No schema migration. No `# @id` written into notes.

## Motivation

Reported: "notebook creation doesn't create the purple bullet point any more",
and "if I select a note without a notebook the read-only window should not be
visible".

The first is a mismatch between meaning and expectation rather than a
regression. `GetNotebookLinks` reads `notebook_links`, and the only writer is
`RecordPush` (`internal/learning/notebooks.go:88`). A dot has therefore always
meant *"text on this line was pushed out of a notebook"* — provenance — never
*"this node has a notebook"*. Notebook creation has never produced a dot.

Worse, links are matched by `(chapter, node_text)` (`notebook_methods.go:333,
344`), so **any edit to a line silently drops its dot**. Promoting an extract to
a card rewrites the line, which alone would explain dots disappearing.

The second is the same defect 032 fixed for Process mode: `SourceTab` opens the
overlay on a dot click (`SourceTab.svelte:32-34`) and holds `reopenId` until
explicitly closed (`:46-49`), so it keeps displaying one note's notebook while
you read another. In an application whose purpose is binding notes to specific
nodes, showing the wrong binding is worse than showing none.

## Grounding in the current implementation

- `SourceTab.svelte:284` installs `notebookDotsGutter(onDotClick)`; `:38-44`
  refreshes marks from `GetNotebookLinks`.
- `:46-50` holds `reopenId` / `reopenReadOnly`, with `openNotebookReadOnly`,
  `closeReopen` and `resumeReopen`.
- `:355-366` renders the overlay, keyed `{#key reopenId + reopenReadOnly}`.
- `notebooks.extract_node_id` is **already node-id keyed** and carries an index
  (`db.go:170`). Node ids are `<path>:<chapter>:id:<short>`.
- 032 shipped the machinery this needs: `FileNodeSpans`, `nodeIdAtLine`
  (`frontend/src/lib/node-span.js`), and `PeekNotebook` — which returns `nil,
  nil` for a node with no notebook and **never creates one**.
- `ProcessEditor` already demonstrates debounced caret reporting.

## Decisions

- **The dot means "has a notebook", keyed by node id.** Chosen over re-keying
  `notebook_links`, because pushed nodes have no id — `InjectMissingIDs` only
  marks study nodes and `InjectMissingExtractIDs` only nodes with a `(from)`
  arrow — so re-keying would require writing an `# @id` into notes on every
  push. That reverses an explicit constraint from the 032 design ("keeps the
  feature from ever writing an `# @id` into a user's notes") and needs a schema
  migration and backfill. Node ids are stable across edits, so the new keying
  fixes the disappearing-dot problem for free.
- **Push provenance keeps being recorded but stops driving the gutter.**
  `RecordPush` and `notebook_links` are left alone. The data is the only record
  of where pushed text came from, and its capture was only just repaired. That
  it currently has no reader is noted as a loose end, not resolved here.
- **The overlay opens on a dot click, then follows the caret.** Rejected:
  following the caret unconditionally, which would make the notebook appear
  while you are simply writing.
- **Closing is sticky.** Close means closed until you click another dot —
  otherwise moving the caret would reopen what you just dismissed.
- **The feature is read-only.** It uses `PeekNotebook`, so browsing creates
  nothing, exactly as in Process mode.

## Architecture

### The rule

| overlay state | caret on | overlay shows |
|---|---|---|
| never opened | anything | nothing |
| opened via a dot | node with a notebook | that node's notebook |
| opened via a dot | node without one, or a line with no node | nothing |
| closed by the user | anything | nothing, until another dot click |

Hiding rather than unmounting-and-remounting is not a distinction the user can
observe; the pane is remounted per notebook exactly as `ProcessView` does.

### Backend — one new binding

**`NotebookMarks(filePath string) ([]NotebookMark, error)`** where
`NotebookMark{Line int; NotebookID string}` — the lines of nodes that have a
notebook.

Implementation: parse the file, take each node's `ID`, and look up notebooks in
one query rather than one per node. `extract_node_id` begins with the file path,
so a single `WHERE extract_node_id LIKE ? || ':%'` on the indexed column
retrieves the file's notebooks; the result is joined to node ids in memory.

`GetNotebookLinks` is left in place and simply stops being used by the gutter.

### Frontend — `SourceTab.svelte`

Gains what `ProcessView` already has, and nothing more:

```
spans        ← FileNodeSpans(filePath), refetched after each save
caretLine    ← debounced updateListener, 150ms, same-line guard
focusedId    ← nodeIdAtLine(spans, caretLine)
focusedNb    ← PeekNotebook(focusedId)   // null is the normal case
armed        ← true on dot click, false on Close
```

The overlay renders iff `armed && focusedNb`. A dot click sets `armed` and
resolves the clicked line, so clicking still opens immediately.

`resumeReopen` ("Resume (edit)") keeps working, applying to whichever notebook is
currently shown.

**Reuse, not reimplementation.** The caret listener is the same shape as
`ProcessEditor`'s; if it needs a third copy later it should be extracted, but two
call sites do not yet justify the abstraction.

## Consequences

- **The dot changes meaning.** Existing dots for pushed text disappear; dots for
  nodes with notebooks appear. Users who read dots as provenance will see a
  different picture — this is the point, but it is a visible change.
- **Push provenance becomes invisible.** Recorded, never displayed. If that
  matters it needs its own affordance.
- **The overlay moves as you navigate.** Deliberate, and the reason the feature
  exists.
- **The span map goes stale while typing**, bounded by the next save — the same
  accepted trade-off as 032, with the same self-correcting behaviour.

## Testing

**Go unit:** `NotebookMarks` over a file with a node that has a notebook, a node
that does not, a node with no id, and a file with no nodes; plus a case proving
it creates nothing (query the store afterwards).

**JS unit:** none new — `nodeIdAtLine` is already covered by 032.

**Manual:** its own test-plan document, matching prior slices. Dot appears when a
notebook is created; dot survives an edit to the line's text (the old keying's
failure); overlay follows the caret; hides on a node without a notebook; Close
stays closed; Resume (edit) still works; and no notebook row is created by
browsing, verified in `learning.db`.

## Out of scope

- Any schema migration, or re-keying `notebook_links`.
- Injecting `# @id` into notes.
- A separate marker for push provenance.
- Creating a notebook from the note editor.
- Changes to Process mode.
