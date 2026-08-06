# Study read panel: sources sidebar with extract trees — manual test plan

**Design:** `2026-08-06-study-sources-sidebar-design.md`
**Build:** `make dev`

**This document is the verification gate for the interactive parts.** This
repository has **no Svelte component test harness**, so `SourcesSidebar.svelte`,
the read-focus branch of `StudySurface.svelte`, and the three `peekAt` guards in
`ReadingView.svelte` have **zero automated coverage** and have never been
watched running together. Treat any failure below as a new finding.

What *is* automated: `reading_methods_test.go` covers `GetReadingExtracts` —
grouping by notes path rather than source path, ordering by anchor value
numerically (`char=9` before `char=100`), passing `page=` anchors through
unchanged, and returning `{}` rather than `nil` for a collection with no
readings. `sources-filter.test.js` covers `filterSources` in nine cases: empty
query, title match, extract match, matches across several documents,
case-insensitivity, no match, a reading with no extracts, and that expansion
only happens on an extract-driven survivor. Everything below is what those
cannot reach — mainly the two click targets, the collapse, the peek, and how
the three fit together on screen.

## Three things that are deliberate — do not report them as bugs

- **A peeked document does not move your reading progress.** Closing it, or
  opening a different document, leaves the `%` exactly where it was before you
  peeked. That is the whole point of the feature — see TC-7 and TC-8.
- **The tree does not live-update while you are reading.** `extractsByReading`
  is loaded on mount and reloaded when a document is opened or closed from the
  sidebar — not while `ReadingView` sits open. Create an extract mid-read and
  the sidebar's copy of that document's tree will not show it until you leave.
- **A passage already promoted to a card still appears, tagged `card`.** The
  tree is not the processing queue: it records everything you took from a
  document, because its job is getting you back to the passage. Clicking one
  peeks it exactly like any other. No sync is needed for any of this — the tree
  is parsed from the `.n4l` itself, so hand-edits show up on the next reload.

## Peek navigation works in all three media — but verify it per medium

`ReadingView.svelte` passes the peeked anchor to all three panes as `startAt`
(lines 158, 160, 162). Each pane parses it with its **own** unit: `PdfSourcePane`
reads `page=N`, `TextSourcePane` and `WebSourcePane` read `char=N`. The units
must not be crossed, and the panes reach the anchor by different routes — the
PDF re-renders a page, the other two scroll a projection — so TC-6 and TC-8 ask
you to confirm the landing position separately for text and for PDF rather than
assuming one implies the other.

An anchor whose unit does not match its pane fails safe: the regex finds
nothing, and the pane opens at the top (page 1, or offset 0). If a peek lands
you at the very start of a document instead of at the passage, that is the
symptom to report.

## Before you start

You need a collection with Study mode reachable (Ctrl+Shift+S). Useful to have
going in:
- a **text or markdown** reading, read part-way, with at least one extract
  taken from early in it (make one with Ctrl+E if you don't have one)
- a **PDF** reading, similarly progressed with an early extract
- one reading with **no** extracts at all

The `+ Text` / `+ PDF` / `+ Web` buttons and Ctrl+E while reading (TC-4/TC-12)
will produce whatever you're missing.

**[DB]** items read `<collectionDir>/learning.db` with `sqlite3`, e.g.:
```
sqlite3 <collectionDir>/learning.db \
  "SELECT id, title, position, proj_len, due, done FROM reading_elements;"
sqlite3 <collectionDir>/learning.db \
  "SELECT id, file_path, from_anchor, active FROM extracts WHERE active=1;"
```

## TC-1 The sidebar is on the left

Open Study (Ctrl+Shift+S), switch to the **Read** tab.

**Expect:** a ~300px panel titled "Sources" on the **left**, with a search box,
an import row (`+ Text` / `+ PDF` / `+ Web`), and the document list. Each row
shows a `▸`/`▾` triangle, the title, a `%`, and a `due` tag (undone, and either
never scheduled or past due) or a `done` tag (finished) — never both.

## TC-2 Collapse and reopen in one click

Click `‹` at the top-right of the sidebar bar. Then click the `›` chevron on
the resulting rail.

**Expect:** collapsed, the sidebar becomes a narrow rail and the reading area
(if a document is open) takes the full width — nothing is left empty beside
it. Before collapsing, type something into the search box and manually expand
a document's tree; after reopening, **both are still there** — the search text
and the expanded tree survive the round trip, because collapsing only swaps
which markup renders and does not reset the component's own state.

## TC-3 The list stays visible while a document is open

Open a document by its title. With it open, click a **different** document's
title in the still-visible sidebar.

**Expect:** the sidebar never disappears while reading. The reading area swaps
to the newly-clicked document without you closing the first one — no prompt,
no confirmation. (The first document's notes and reschedule are still saved in
the background as it unmounts.)

## TC-4 The triangle expands to extracts, in document order

Pick a document with two or more extracts taken at different points (or make
two now with Ctrl+E — see TC-8's setup for how). Click its `▸` triangle, **not**
the title.

**Expect:** only the tree opens — the document itself does not start reading;
`▸` becomes `▾`. Extracts are listed earliest-anchor-first (e.g. an extract at
`char=120` above one at `char=980`; `page=2` above `page=9`), each showing its
extract text (full text on hover via the `title` attribute). If you just
created one of these extracts, it will not appear until you leave the document
once — see the "does not live-update" note above.

## TC-5 The title opens at saved progress

Open a document by its title, scroll/page some way in, close it (`×` or Esc),
and note the `%`.

**Expect:** clicking the title again reopens at the same position you left —
not the top, not a peeked passage.

**[DB]** `position` in `reading_elements` for that id matches what you scrolled
to.

## TC-6 Clicking an extract opens the document at that passage (text)

Use a **text or markdown** document; PDF is checked separately in TC-8.
Expand its tree and note the text of an extract taken from early in the
document. Click that **extract**, not the title.

**Expect:** the document opens scrolled so that early passage sits at/near the
top of the pane — not wherever you'd last read to.

## TC-7 The peek, for a text document — the load-bearing case

Open a text/markdown document by its **title**. Scroll a genuine way down.
Close it (`×` or Esc) and write down the `%` shown in its sidebar row — call
it **P**.

1. Expand that document's tree.
2. Click an extract taken from **early** in the document — well before where
   you'd scrolled to.
3. Confirm the pane opened scrolled to that early passage, not to where you'd
   left off.
4. Close it (`×` or Esc) **without** clicking the title first.
5. Read the `%` on the sidebar row now.

**Expect:** the `%` after step 5 is **exactly P** — unchanged, even though the
pane just showed you a much earlier position than P.

**[DB]** `SELECT position, proj_len FROM reading_elements WHERE id='<id>';` —
`position` is the same `char=N` you had before the peek, not the extract's
anchor.

## TC-8 The peek, for a PDF — the case a partial implementation fails

Open a PDF document by its **title**, page forward into the middle of it,
close it (`×` or Esc), and write down the `%` — call it **P**. You need one
extract from an early page: if you don't have one, open it by title, select
text on an early page, press Ctrl+E, then close and reopen the sidebar's tree
to confirm it's there.

1. Expand the tree and click that early extract to peek it.
2. **Confirm it opens on the extract's page**, not the middle page you left
   off at. `PdfSourcePane` parses `page=N` out of the anchor; landing on page 1
   would mean the anchor's unit did not match.
3. Page forward and back several times
   with `‹`/`›`. Each page render pushes a `progress` event — this is the step
   that has no text-document equivalent.
4. Close (`×` or Esc).
5. Read the `%` on the sidebar row.

**Expect:** the `%` after step 5 is still **exactly P**, unmoved by all the
paging you did during the peek. A guard that only covers `close()`/`onDestroy`
and forgets `onPaneProgress` passes TC-7 and fails here, because PDF pushes
progress on every page turn instead of handing it over once on leave.

**[DB]** `position` in `reading_elements` is still the page you closed on
*before* the peek (step 0), not any page reached during steps 1–3.

## TC-9 Opening by title after a peek resumes and saves normally

Right after TC-7 or TC-8, same document.

1. Click the document's **title**, not an extract.
2. Confirm it reopens at **P** — the saved position — not the peeked passage.
3. Scroll/page further, then close.

**Expect:** the `%` now reflects the new position. Clicking the title clears
`peekAt` (`StudySurface.svelte`'s `on:open` sets it to `null`), so this open
persists like any other.

**[DB]** `position` has changed from what it was in TC-7/TC-8.

## TC-10 Search over titles and extract text

a. Type text that only appears in a document's **title**.
**Expect:** that document is listed; if you hadn't manually expanded it, its
tree stays closed — a title match has nothing in the tree to show.

b. Clear the box, then type text that only appears inside an **extract**.
**Expect:** the owning document is listed and its tree **auto-expands**,
showing only the matching extract(s) — not the rest of that document's
extracts.

c. Type something that matches nothing anywhere.
**Expect:** "Nothing matches."

## TC-11 A document with no extracts

Expand the triangle of a reading that has never had anything extracted from
it.

**Expect:** "No extracts yet." in place of the extract list.

## TC-12 Importing still works via all three buttons

Click `+ Text` (pick a `.txt`/`.md`/`.html`), `+ PDF` (pick a `.pdf`), `+ Web`
(paste a URL into the prompt).

**Expect:** each import adds a new row to the list immediately — no manual
refresh needed — and opens that document for reading. Cancelling a file
dialog does nothing: no row appears, nothing errors.

## TC-13 Peeking a second extract in the already-open document jumps

With a **text** document open via a peek (mid-TC-7 works, or start fresh), go
back to the still-visible sidebar and click a **different** extract from the
same document's tree — without closing the reading first.

**Expect:** the pane re-scrolls to the newly-clicked passage right away. (The
reading id didn't change, but `StudySurface`'s `{#key}` includes `peekAt` in
its key, so a changed anchor still remounts the view.)

This holds for all three media. For a **PDF** the pane re-renders the
extract's page rather than scrolling.

## TC-14 A search-expanded row can still be collapsed

With a search active that auto-expanded a row (from TC-10), click that row's
`▾` triangle.

**Expect:** the tree closes even though the query still matches, and clicking
again reopens it. A manual click overrides the search's expansion rather than
being swallowed by it.

## TC-15 Extracting during a peek does not rewind the document

The peek's second safety property, and a separate code path from TC-7/TC-8 —
this one advances progress from the Go side, so the guards those cases exercise
do not cover it.

1. Open a text document by its **title**, read a genuine way down, close it,
   and write down the `%` — call it **P**.
2. Expand its tree and **peek** an extract from early in the document.
3. Select a sentence in that early passage and press **Ctrl+E** (or the ✂
   Extract button).
4. Close the document.

**Expect:** the new extract exists — it appears in the tree once it reloads —
and the `%` is still **exactly P**. Before this was fixed, step 3 rewound the
document to the peeked passage in both the database and the `# @progress` line,
with nothing on screen to show it.

**[DB]** `position` in `reading_elements` is unchanged, and the notes file's
`# @progress` line still holds the pre-peek value.

## TC-16 Switching documents and coming back keeps your place

The interaction the sidebar exists to make possible — do not skip it.

1. Open document **A** by its title and read a genuine way down. Do **not**
   close it.
2. Click document **B**'s title in the still-visible sidebar.
3. Click **A**'s title again.

**Expect:** A reopens where you left it in step 1, not at the top. Then close A
and confirm its `%` still reflects how far you had read. Before this was fixed,
step 3 reopened A at its stale cached position and closing it wrote that stale
value back over the good one.

**[DB]** `position` for A after step 3 matches what it was at the end of step 1.

## TC-17 Promoted passages stay in the tree

Take a document where you have already promoted at least one extract to a card
(`# @card` in its notes file), and expand its tree.

**Expect:** the promoted passages are listed alongside the unpromoted ones, each
with a small `card` tag, and clicking one peeks it like any other. Check a
passage whose text is long enough to be cut off with an ellipsis — the tag must
still be there, at the right edge, with the text truncating before it rather
than pushing it out of view. Count the
rows against the `# @id` paragraphs in the notes file — every paragraph that
carries a `(from)` anchor should be there, plus the document's own node should
not be.

A paragraph that is missing here has lost its `(from)` anchor in the file *and*
has no row left in the `extracts` table — nothing records where it came from, so
there is nowhere to jump to. Report which ids, if any.

## What I most want to hear about

- **TC-7 and TC-8's `%` results.** That's the entire safety property this
  feature depends on — a peek that silently rewinds a half-read document is
  the failure mode the whole design exists to prevent.
- **TC-17's count.** Whether the tree now matches the `(from)` paragraphs in
  your notes files. This is the fix for the count mismatch you reported.
- **TC-15 and TC-16.** Both are fixes for defects found only in the final
  review, after the feature was otherwise finished. Neither has automated
  coverage on the UI side, and both fail silently when wrong.
- **TC-6/TC-8's landing position.** Whether a PDF peek really lands on the
  extract's page. PDF and text reach the anchor by different routes, so one
  working does not imply the other.
- Anything that makes the search-auto-expand and manual-expand states look
  inconsistent to you in practice, even if it matches what's described above —
  that's a sign the distinction itself needs rethinking, not just the wording.
