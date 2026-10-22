# Process focus notebook — manual test plan

**Design:** `2026-08-03-process-focus-notebook-design.md`
**Build:** `make dev`

**This document is the verification gate, not a supplement.** Every task in this
feature's plan was implemented by a headless agent. None of them could run
`make dev` or open a Wails window, and this repository has no Svelte component
test harness. The focus header, the empty state, and the pane switching described
below were reasoned about from source and unit-tested only in the pieces that
could be pulled into plain Go or JS modules. **None of the rendered UI has ever
been watched running.** Treat a failure here as a new finding, not a surprise.

Automated coverage: `content.NodeSpans` (dittos, id-less nodes, consecutive
nodes, the `# @id` marker line, the empty-non-nil contract) and
`PeekNotebook`/`FileNodeSpans` — including that peeking creates nothing — via
`make test`; `nodeIdAtLine`'s edge cases via `make test-js`. Everything below is
what those cannot reach: the caret-following behaviour itself, which exists only
in the running app.

## Before you start

**Do not hand-write `# @id` markers into a file to fake extracts.** An id in the
file does not register an extract in `learning.db`, and several cases below
depend on the file and the database agreeing. Build the fixture through the app.

1. Create a `.n4l` file in your collection containing this literal text:

```
- Focus Test

:: mechanics ::

Newton's first law says a body stays at rest or in uniform motion unless acted on
    " by an external force

The second law relates force to mass and acceleration

The third law says every action has an equal and opposite reaction

:: notes ::

This is a plain note I typed and never promoted

Another plain note, also with no id
```

2. Promote three of the `:: mechanics ::` lines to extracts through the normal
   flow, so the study queue and the file agree.
3. Enter Process mode on the **first** extract, let its notebook open, add one
   cell so it is visibly non-empty, then click **Later**.
4. Enter Process mode on the **second** extract. This is your starting state:
   you are processing extract 2, extract 1 has a notebook, extract 3 does not.

Cases marked **[DB]** are invisible in the UI and require querying SQLite. The
database is at `<collectionDir>/learning.db`.

## TC-1 The queued extract opens as before

Enter Process mode on an extract.

**Expect:** the right pane shows that extract's notebook, exactly as before this
feature. **No focus header is displayed** — what you are reading and what you are
processing are the same.

## TC-2 The pane follows the caret to another extract with a notebook

Move the caret into extract 1, which has a notebook.

**Expect:** the pane switches to extract 1's notebook — the cell you added in
setup is visible — and a header appears naming both subjects.

## TC-3 An extract with no notebook shows the empty state

Move the caret into extract 3, which has never been processed.

**Expect:** `No notebook for this line.` — not extract 1's notebook, and not an
error banner.

## TC-4 A line that cannot carry a notebook shows the same empty state

Move the caret onto each of: a blank line, the chapter title `- Focus Test`, the
context marker `:: notes ::`, and one of the plain notes with no `# @id`.

**Expect:** the empty state in every case. No error, and no flicker of a
previously shown notebook.

## TC-5 Returning to the queued extract restores it

Move the caret back into extract 2, the one you are processing.

**Expect:** its notebook returns and the focus header disappears.

## TC-6 The action bar never retargets

With the caret sitting in **extract 1**, click **Promote**.

**Expect:** it acts on **extract 2** — the one named as "processing" in the
header — not the one you are reading. Repeat with **Later**: the queue advances
past extract 2, not extract 1.

*(This is the guarantee that keeps the study queue safe. If it fails, an extract
you merely looked at can be resolved without you asking.)*

## TC-7 A peeked notebook is editable and persists

With the caret in extract 1, add a cell and type in it. Move the caret away, then
back.

**Expect:** the edit is still there, saved to **extract 1's** notebook. Return to
extract 2 and confirm its notebook is unchanged.

## TC-8 **[DB]** Browsing creates nothing — the most important check

Note the count first:

```
sqlite3 <collectionDir>/learning.db 'SELECT count(*) FROM notebooks;'
```

Now move the caret through every extract and plain line in the file, including
several that have no notebook. Re-run the count.

**Expect:** unchanged.

*(If this fails the feature inverts: every node you visit gains a notebook, the
file browser starts marking those lines as having notebooks, and a visited
extract begins showing one. It is invisible on screen, which is why it is here.)*

## TC-9 The staleness window, and that it self-corrects

Put the caret in extract 3 and note what shows. Insert several blank lines
**above** it **without saving**, then move the caret back into extract 3.

**Expect:** possibly the wrong notebook or the empty state. The span map is stale
between saves and this is **accepted behaviour, not a bug**. Now save (Ctrl+S)
and move the caret again.

**Expect:** correct again. The window is bounded by the next save.

*(This case exists so a future reader does not "fix" it, and so its blast radius
stays known.)*

## TC-10 Push still records against the right notebook

With the caret in extract 1, push an N4L draft from that notebook into the file.

**Expect:** the text is inserted and no error appears. The push is recorded
against the notebook that pushed it — extract 1's, the one you were reading.

## TC-11 Rapid caret movement does not show a stale notebook

Move the caret quickly between extract 1 and extract 3, several times.

**Expect:** the pane settles on wherever the caret finally rests — never on the
other one. A brief empty state while a lookup is in flight is fine; **the wrong
notebook is not**.

*(Backend replies can arrive out of order; the view discards any that no longer
match the focused node.)*

## TC-12 A file region with no id-bearing nodes at all

Move the caret into the `:: notes ::` section, which contains only plain notes.

**Expect:** the empty state throughout, no errors, and no console noise.

## TC-13 The focus header is actually visible

With the caret in extract 1, look at the top of the right-hand pane.

**Expect:** the header strip is **visible on screen**, above the notebook, with a
divider beneath it.

*(The notebook pane is absolutely positioned and originally covered this header
completely. The app looked entirely healthy with the header invisible, so
confirming it renders is not the same as confirming it is seen — read the text
off the screen.)*

## TC-14 The header shows a short id, not a filesystem path

Read the header text.

**Expect:** something like `Processing second-law · reading first-law`. It must
**not** contain a path such as
`/home/you/notes/focus-test.n4l:mechanics:id:first-law`.

## TC-15 The Vim toggle does not strand the pane

With the caret in extract 1 (so the pane shows extract 1's notebook and the
header is up), click the **VIM** toggle.

**Expect:** the editor's cursor resets to the top of the file, **and the pane
follows** — showing whatever node line 1 belongs to, or the empty state. It must
**not** keep showing extract 1's notebook.

*(This was a real bug: rebuilding the editor constructs a new CodeMirror view,
and construction alone never fires the update listener. Nothing automated guards
it.)*

## TC-16 An extract whose line no longer matches still reports on load

Edit the file outside the app so the queued extract's recorded line is out of
range — delete most of the file's lines and save — then enter Process mode on
that extract.

**Expect:** the app opens without error and the pane shows a notebook or the
empty state. It must **not** hang on a blank right-hand pane forever waiting for
a caret report that never comes.

*(This exercises the fallback branch where the editor only focuses and dispatches
no selection event.)*

## TC-17 A push records provenance even though it moves the caret

With the caret in extract 1, push an N4L draft from that notebook into the file.

**Expect:** the text is inserted, and the push is recorded against **extract 1's**
notebook — the one that pushed it — even though inserting moves the caret and can
change which node is focused mid-save.

*(`insertAndSave` moves the caret itself, so this is not an exotic race: it was
reachable on an ordinary push with no user input at all. The notebook id is now
captured before the save begins.)*

## TC-18 Saving corrects a stale span map

This is the other half of TC-9 and is worth doing as its own case, because until
recently only a *push* refreshed the map, never a plain save.

Insert several blank lines above an extract, press **Ctrl+S**, then move the
caret into that extract.

**Expect:** the correct notebook for that extract. Repeat using **Make card** and
the action bar's **Later** instead of Ctrl+S — each of these also writes the file
and must leave the map correct.

## TC-19 A caret move cancels an in-flight cell — known limitation

In a peeked notebook, start something long-running: send an LLM turn, or run a
shell cell with a slow command. While it is still running, press an arrow key to
move the caret onto a different node.

**Expect:** the pane switches and the in-flight work is **cancelled** — the LLM
turn stops and its partial output and your prompt are lost; a shell run is
killed; a recording audio cell stops.

*(Known and accepted. The pane is deliberately remounted per focused node, and
teardown cancels. Cell *text* is debounce-persisted and survives; in-flight work
does not. Recorded so it is not mistaken for a crash — and so that anyone who
finds it unacceptable changes the remount strategy deliberately rather than
patching a symptom.)*
