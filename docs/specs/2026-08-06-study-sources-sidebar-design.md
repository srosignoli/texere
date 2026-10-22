# Study read panel: a sources sidebar with extract trees

**Date:** 2026-08-06
**Status:** Proposed — awaiting approval
**Feature area:** Study mode → Read focus

## Summary

The document list becomes a collapsible **left sidebar** that stays visible while
you read. Each document can be expanded to show the extracts taken from it, in
document order. Clicking a document opens it; clicking an extract jumps to the
passage it came from **without moving your reading progress**. A search box
filters on document titles *and* extract text.

## Motivation

Today the read focus has two disjoint states. With no document open,
`SourcesPanel` is a 340px panel pinned to the right with empty space beside it.
Open a document and the list disappears entirely, so switching documents means
closing the one you are in.

Nothing shows what you have already extracted from a document. The extracts
exist — they carry the offset they were taken from — but the only way to reach
one is through the processing queue, which surfaces them by due date rather than
by source. There is no way to ask "what have I pulled out of this article?"

## Grounding in the current implementation

Surveyed before designing:

- **`StudySurface.svelte:103`** renders `SourcesPanel` when no reading is active,
  and `ReadingView` when one is. `SourcesPanel` is used nowhere else.
- **`SourcesPanel.svelte:85`** — `position: absolute; right: 0; width: 340px`.
- **`ReadingView.svelte:150-160`** — `.reading-body` is a flex row of
  `.reading-source` and `.reading-notes`, both `flex: 1`.
- **Positions are unit-prefixed**, not always characters. `reading_methods.go:61`
  documents `char=N` as a 0-based rune offset into the text projection, but
  `PdfSourcePane.svelte:51` dispatches `page=N`. An extract's anchor carries
  whichever unit its medium uses.
- **`Extract`** (`internal/learning/extracts.go:11`) has `FilePath` — the *notes*
  file the extract node lives in — and `FromAnchor`, which points back at the
  source. `ReadingView` (the Go struct) carries both `RefPath` and `NotesPath`,
  so a reading's extracts are those whose `FilePath` equals its `NotesPath`.
- **Reading progress is persisted in three places**, which matters for the peek:
  1. `ReadingView.svelte:84` — `close()` pulls `pane.getProgress()`.
  2. `ReadingView.svelte:130` — `onDestroy` does the same when not already closed.
  3. `ReadingView.svelte:42` — `onPaneProgress` writes whatever a pane pushes.
     The comment at `:83` says "PDF/web push progress as the user navigates", so
     for those media progress is written continuously, not only on leave.

## Decisions

- **The sidebar is on the left, collapsible in one click.** The reading area sits
  to its right and takes the full width when collapsed. Rejected: keeping the
  list on the right, which reads as a secondary inspector rather than the
  navigation it actually is, and left the reading area unbalanced.
- **The list stays visible while reading.** That is the point of the change: you
  can switch documents, or glance at what you have extracted, without closing
  what you are in.
- **The tree is two levels: document → its extracts.** Rejected: SuperMemo's
  arbitrary nesting, which requires being able to read an extract as its own
  source. Texere has no such affordance, so nesting would be a second feature
  underneath this one and deserves its own spec.
- **Search matches titles and extract text.** A document survives if it or any of
  its extracts match, and a surviving document auto-expands showing only the
  matching extracts. Rejected: title-only filtering, which cannot find a
  half-remembered passage when you have forgotten which document it came from —
  the case that actually motivates a search box.
- **Jumping to an extract is a peek and does not move your progress.** This is
  the least obvious decision and the one most likely to be got wrong. Progress is
  pulled *on leave*, so a naive jump means glancing at an old extract silently
  rewinds a document you had read most of the way through, with nothing on screen
  to tell you. Clicking the document *title* still resumes and saves normally.
- **Extracts are fetched once for the whole list, not per document.** Search has
  to match text in documents that are still collapsed, so the data has to be
  there before any tree is opened.

## Architecture

### `SourcesSidebar.svelte` (new, replaces `SourcesPanel.svelte`)

Owns the list, the search box, the expansion state and the collapse toggle.
Emits two things: open a document, and peek an extract.

```
┌ Sources ────┐┌ Reading ──────────────────┐      collapsed:
│ [search…]   ││                            │      ┌┐┌──────────────────────┐
│ ▾ Doc A 40% ││  source text  │  notes     │      │││  source  │  notes    │
│   ⤷ extract ││               │            │      │▸│          │           │
│ ▸ Doc B 12% ││               │            │      └┘└──────────────────────┘
└─────────────┘└────────────────────────────┘
```

Three distinct click targets per row, which is why the triangle exists as its own
control rather than the whole row toggling:

| click | does |
|---|---|
| the disclosure triangle | expands/collapses that document's extracts |
| the document title | opens it for reading, resuming at saved progress |
| an extract | opens it and jumps to that passage, as a peek |

The `%`, the `due`/`done` tags and the `+ Text / + PDF / + Web` imports all carry
over unchanged. Collapsed, the sidebar is a narrow rail with a chevron.

### Backend — one new binding

**`GetReadingExtracts() (map[string][]ReadingExtractView, error)`** keyed by
reading id, where `ReadingExtractView{ID, Text, Anchor string}`.

For each reading, the extracts whose `FilePath` equals its `NotesPath`, ordered
by their anchor's numeric value so the tree reads in document order. `Anchor` is
passed through **verbatim** — `char=1173` or `page=12` — because the unit depends
on the medium and only the pane knows what to do with it.

Returning every reading's extracts in one call, rather than a per-document
lookup, is what makes search over collapsed documents possible.

### Peeking without losing your place

`ReadingView` gains one prop, `peekAt` (an anchor string, or null). When set, the
pane opens at that anchor instead of the reading's saved position, and **all
three persist paths are suppressed for the lifetime of that peek**:

- `close()` skips `UpdateReadingProgress`
- `onDestroy` skips it too
- `onPaneProgress` ignores pushes

Suppressing only the first would work for text and markdown and fail silently for
PDF and web, which push progress as you navigate. Rescheduling and notes flushing
are unaffected — a peek still saves notes you typed.

Opening the same document by its title afterwards clears the peek and resumes
normal behaviour.

## Consequences

- **The reading area is narrower by the sidebar's width** unless collapsed. The
  one-click collapse exists for exactly that.
- **A peeked document behaves differently from an opened one**, which is a real
  distinction to learn. It is the safe direction: the failure mode of getting it
  wrong is silent lost progress.
- **All extracts are held in memory** while the panel is open. Fine for
  collections of the size this app targets; a collection with tens of thousands
  of extracts would want a server-side search instead.
- **The tree shows only what sync has discovered.** Extracts appear in the
  `extracts` table via `learning.Sync`, so one created moments ago shows after
  the next sync, exactly as it does in the processing queue today.

## Testing

**Go unit:** `GetReadingExtracts` groups by notes path and not by source path;
orders by anchor value numerically, not lexically (so `char=9` precedes
`char=100`); passes `page=` anchors through unchanged; and returns an empty map
rather than nil for a collection with no readings.

**JS unit:** the filter is a pure function in `frontend/src/lib/` — this repo has
no Svelte component harness, so logic left in the component would be untested.
Cases: title match, extract match, both, neither, case-insensitive, empty query
returns everything unexpanded, and a match inside a collapsed document expands it.

**Manual:** its own test-plan document. The load-bearing case is the peek — read
a document part-way, note the `%`, peek an early extract, close, and confirm the
`%` is unchanged. Repeat for a **PDF**, where progress is pushed rather than
pulled and a partial implementation would fail.

## Out of scope

- Nested extracts (extract-of-extract) and reading an extract as its own source.
- Editing or deleting extracts from the tree.
- Any change to the processing queue, to `ProcessView`, or to how extracts are
  created.
- Server-side search.
- Reordering or grouping documents beyond the existing list order.
