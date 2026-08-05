# Note editor notebook — manual test plan

**Design:** `2026-08-04-note-editor-notebook-design.md`
**Build:** `make dev`

**This document is the verification gate, not a supplement.** Every task in this
feature was implemented by a headless agent. None could run `make dev` or open a
Wails window, and this repository has no Svelte component test harness — so
`SourceTab.svelte`, which holds all of the new behaviour, has **zero automated
coverage**. The gutter dots, the overlay, and every guard below were reasoned
about from source. **None of it has ever been watched running.** Treat a failure
here as a new finding, not a surprise.

Automated coverage: `NotebooksForNodes` (exact-id lookup, the colon-in-a-path
leak, earliest-wins dedup) via `make test`; `NotebookMarks` (marks only nodes
that have a notebook, marks survive a text edit, creates nothing, empty-non-nil)
via `make test`. All of it is backend. Everything below is what those cannot
reach.

## Two things that changed meaning — do not report these as bugs

- **A gutter dot now means "this node has a notebook."** It used to mean "text on
  this line was pushed out of a notebook." Your existing dots will move: dots on
  lines you pushed into disappear, dots appear on nodes that have notebooks.
- **Push provenance is no longer displayed anywhere.** It is still recorded in
  `notebook_links` — `RecordPush` is untouched — but nothing reads it for the
  gutter. This is a deliberate trade, recorded in the design.

## Before you start

```
cp -r <collectionDir> <collectionDir>.bak
sqlite3 <collectionDir>/learning.db 'SELECT count(*) FROM notebooks;'   # note it
```

You need a `.n4l` file with at least: two extracts that have notebooks, one
id-bearing node with **no** notebook, and a plain note with no `# @id`. The
easiest way to get notebooks is to process two extracts in Process mode — opening
one there creates its notebook.

Cases marked **[DB]** are invisible in the UI.

## TC-1 A notebook makes a dot appear — with the file already open

Order matters here. Do it exactly.

1. **First** open the file in the note editor and leave it open.
2. Now process one of its extracts in Process mode, which creates that extract's
   notebook.
3. Return to the note editor **without switching files** and without toggling
   Vim.

**Expect:** a purple dot has appeared in the gutter on **that node's line** — the
line holding the node's text, not the `# @id` marker line above it.

*(This is the originally reported symptom and the reason the feature exists.
Opening the file **afterwards** would pass even with the bug, because the dot
would be fetched fresh on load — so the test only means something in this order.
The note editor stays mounted while Process mode is open, so the dot has to
arrive by notification, not by reload.)*

## TC-2 The dot survives editing the node's text

Edit the text of a node that has a dot — change several words. Save.

**Expect:** the dot is still there, on the (possibly moved) line.

*(This is the whole reason for re-keying. The old scheme matched on
`(chapter, node_text)`, so any edit silently dropped the dot — including the line
rewrite that "Make card" performs.)*

## TC-3 Nodes without notebooks have no dot

Look at an id-bearing node you have never processed, and at a plain note with no
`# @id`.

**Expect:** no dot on either.

## TC-4 Clicking a dot opens the overlay — and moves the caret

Put the caret somewhere far away, then click a dot on a distant line.

**Expect:** the overlay opens showing **that node's** notebook, **and the caret
moves to the clicked line.**

*(The caret move is load-bearing, not a convenience. Without it the overlay and
the caret disagree, and the same-line guard would then never re-fire for the line
you are actually on — see TC-5, which is the case that catches it.)*

## TC-5 The overlay cannot freeze on a node you have left

This is the sharpest case in the plan. Do it exactly.

1. Put the caret on a line — call it **A** — that has no notebook.
2. Click a dot on a **different, distant** line **B**.
3. Now type a few characters, or press an arrow key, staying near where the caret
   now is.

**Expect:** the overlay tracks where you are now. It must **not** keep showing
B's notebook while you edit elsewhere.

*(Before this was fixed, clicking B's dot left the caret on A, so the caret
report's same-line guard compared against A forever and never re-resolved. The
overlay showed B's notebook indefinitely while you typed somewhere else — exactly
the defect this feature exists to remove.)*

## TC-6 The overlay follows the caret between notebooks

With the overlay open, move the caret into another node that **has** a notebook.

**Expect:** the overlay switches to that node's notebook.

## TC-7 The overlay hides on a node with no notebook

Move the caret to an id-bearing node with no notebook, then to a plain note with
no `# @id`, then to a blank line.

**Expect:** the overlay **hides entirely** in all three cases. Not an error, not
the previous notebook, not an empty pane frame.

## TC-8 And comes back

Move the caret back into a node that has a notebook.

**Expect:** the overlay reappears with that notebook.

## TC-9 Close is sticky

Click **✕ Close**. Now move the caret across several nodes that have notebooks.

**Expect:** the overlay stays shut the whole time. It reopens only when you click
a dot again.

*(Without stickiness, Close would degrade to a flicker — the next caret move
would reopen what you just dismissed.)*

## TC-10 Resume (edit) works and persists

With the overlay showing a notebook, click **Resume (edit)**, change a cell, then
move the caret away and back.

**Expect:** the edit is there, saved to **that node's** notebook.

## TC-11 Edit rights do not follow you

With the overlay in Resume (edit) mode, move the caret to a **different** node
that has a notebook.

**Expect:** the new notebook opens **read-only** — the Resume button is showing
again. You must not silently retain edit rights on a notebook you never granted
them for.

## TC-12 A save does not flash the wrong notebook

With the overlay open on a node, make an edit and press **Ctrl+S**. Watch the
overlay closely at the moment of saving.

**Expect:** no flicker to a different node's notebook.

*(A save replaces the document and resets the caret, and the caret report is
debounced 150ms. Before this was fixed the span map was re-resolved against the
last *reported* line, so for ~150ms after every save the overlay described the
node the caret had just left.)*

## TC-13 Vim toggle does not strand the overlay

With the overlay open on a node partway down the file, click **VIM**.

**Expect:** the editor rebuilds, the caret resets to the top, **and the overlay
follows** — showing whatever line 1's node has, or hiding. It must not keep
showing the pre-toggle notebook.

*(Rebuilding constructs a new editor view, and construction alone never fires the
update listener — only an explicit dispatch does.)*

## TC-14 A newly-injected `# @id` becomes addressable immediately

Write a new note by hand that will be given an id — e.g. mark it as an extract —
and let the app inject the `# @id` (this reloads the file silently). Without
doing anything else, put the caret on that node and click its dot once it has a
notebook.

**Expect:** the node resolves. It must not be invisible to the overlay until some
unrelated event refreshes things.

*(The silent reload replaces the whole document. If the span map were not
refreshed there, a node that had only just become addressable would stay
unreachable.)*

## TC-15 External reload keeps the map correct

With the file open, modify it outside the app so the external-modification banner
appears, then click **Reload**.

**Expect:** dots and the overlay reflect the new content — line numbers have
moved and everything still lines up.

## TC-16 **[DB]** Browsing creates nothing — the most important check

Note the count:

```
sqlite3 <collectionDir>/learning.db 'SELECT count(*) FROM notebooks;'
```

Now move the caret through every node in the file, including many with no
notebook, and open/close the overlay several times. Re-run the count.

**Expect:** unchanged.

*(This is invisible on screen. If it fails, every node you look at acquires a
notebook, dots appear everywhere, and the feature inverts.)*

## TC-16b Switching files does not carry the overlay across

With the overlay open on a node, switch to a different file in the browser.

**Expect:** the overlay is closed and the gutter shows the new file's dots. It
must not stay open showing the previous file's notebook.

## TC-16c A cell edit survives moving away quickly

With the overlay in Resume (edit) mode, type in a cell and then move the caret to
another node **within about half a second** — faster than the 600ms save
debounce. Come back.

**Expect:** the edit is still there.

*(Cells persist on a debounce and the pane is torn down when the focused node
changes. Whether the pending save flushes before teardown has not been observed
running — this case exists to find out.)*

## TC-17 Process mode is unaffected

Open Process mode and confirm its notebook pane, its own gutter dots, and the
action bar all behave as they did.

**Expect:** no change. This feature touched only the note editor.

## What I most want to hear about

- **Anything in TC-4 through TC-9.** That is the whole user-facing behaviour and
  none of it has been observed running.
- **TC-5 specifically.** It was a Critical found in review; it would look
  perfectly fine in casual use and only appears if you click a distant dot and
  then keep typing where you were.
- **Whether the dot's new meaning is what you actually want in daily use** — you
  will see dots in different places than before, and losing the pushed-text
  marker is a real trade rather than a pure win.
