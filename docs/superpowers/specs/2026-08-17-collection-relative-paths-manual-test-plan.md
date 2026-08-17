# Collection-relative paths — manual test plan (executed)

**Design:** `2026-08-17-collection-relative-paths-design.md`
**Build:** `make build && ./build/bin/texere`, built on branch
`048-collection-relative-paths` at commit `d6d9138`.
**Run date:** 2026-08-17 — every case below was **executed**, not described.
Each carries the result actually observed and the screenshot it was read from.

**Why this document exists.** Task 3's report says plainly that a missed call
site is type-correct and behaves normally until a collection moves, so neither
the compiler nor any single test can prove the conversion complete. This run is
the only check that exercises the real database, the real note files and the
real UI against a collection that has actually been copied.

## The rule this run was executed under

**The user's real collection at
`/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test` was never
opened by the app, and was never written to.** The feature exists because a
verification run against a copy once reset a reading position in that directory
(`# @progress char=11555` → `char=0`). Every file's mtime, size and SHA-256 was
recorded before the run and re-checked after it.

**Result, stated explicitly: all 82 files are byte-identical and carry their
original mtimes. Nothing in the real collection changed.**

```
$ diff real-before.txt real-after.txt          # mtime + size, 82 files
$ diff real-before-sha.txt real-after-sha.txt  # sha256, 82 files
                                               # both empty — identical
```

Its five reading notes still read `char=4988`, `char=554`, `char=3`,
`char=0`, `page=6` — the values they held before this run began.

## How this run was set up

- `cp -r` of the real collection into `~/.claude/jobs/ad91db08/tmp/paths-a`
  (82 files, `learning.db` at **schema version 6**, all four path columns
  absolute). `paths-b` is a copy of `paths-a` taken *after* `paths-a` had been
  migrated — so it is a collection carrying a database written for a different
  directory, which is the defect's exact shape.
- `~/.config/texere/config.json` points at the real collection, so it was
  repointed **before the binary was ever launched** and a guard asserted the
  file no longer mentioned `SStorytimeFork` each time. It was restored
  afterwards and verified byte-identical by SHA-256
  (`033f46f0…c17e14c` before and after).
- The app was driven with synthetic X11 input (XTEST) on `DISPLAY=:0` and each
  state captured by grabbing the app window with `XGetImage`; root-window
  capture on this WSLg display returns black. Every screenshot was read back as
  an image.
- No production code was changed. Screenshot paths below are relative to
  `~/.claude/jobs/ad91db08/evidence/`.

## Result summary

**6 of 10 cases pass. Four fail, on two distinct defects, neither of them a
regression introduced by this branch — but one of them means the feature does
not deliver what its motivation promises.**

| # | Case | Result |
|---|---|---|
| TC-1 | Migration to schema 7 runs on a real database | **PASS** |
| TC-2 | All four columns hold collection-relative values | **PASS** |
| TC-3 | `reading_elements.ref_path` is untouched | **PASS** |
| TC-4 | A copied collection writes reading progress to **itself** | **PASS** |
| TC-5 | The original collection is byte-identical afterwards | **PASS** |
| TC-6 | A renamed note fails visibly in **Process** | **PASS** |
| TC-7 | A renamed note in **Read** | **FAIL** — unhandled rejection wipes the UI |
| TC-8 | Reading list, reading view, editor, nodes, Recall on a settled collection | **PASS** |
| TC-9 | **Process** / extracts on a copied collection | **FAIL** — stale absolute path inside the row's identity key |
| TC-10 | Newly written rows are location-independent | **FAIL** — new identity keys are still absolute |

## TC-1 The migration runs, on a real database

**Result: PASS.** `paths-a` opened with `learning.db` at version 6. After the
app opened it once, `select version from schema_meta` returns **7**
(`c1-01-open-paths-a.png` shows the collection loaded, file tree populated).
Re-opening the same directory later in the run did not re-run it — the version
guard `if v >= schemaVersion { return nil }` held, and the column values below
were unchanged on the second open.

## TC-2 The four columns hold relative values

**Result: PASS.** Every row in all four columns was rewritten; **zero rows in
any of the four still begin with `/`**.

| Column | Rows | Still absolute | Sample after |
|---|---|---|---|
| `items.file_path` | 15 | **0** | `reading/spaced-repetition.n4l`, `test3.n4l` |
| `extracts.file_path` | 20 | **0** | `reading/incremental-learning-supermemo-help.n4l` |
| `notebook_links.file_path` | 25 | **0** | `reading/spaced-repetition.n4l` |
| `reading_elements.notes_path` | 5 | **0** | `reading/what-is-the-tdd-methology-in-software-development.n4l` |

Before the run the same rows read
`/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test/reading/…`.
`rehome`'s longest-suffix search recovered every one of the 65 rows, including
the top-level `test3.n4l`, which has only one segment to match on.

## TC-3 `ref_path` is untouched

**Result: PASS.** All five `reading_elements.ref_path` values are string-for-string
what they were before the migration — `sources/spaced-repetition.html`,
`sources/SW_SmartMS42_Maintenance_Supervision10.pdf`,
`sources/2026-06-22-incremental-reading-manual-test-plan.md`,
`sources/incremental-learning-supermemo-help.html`,
`sources/what-is-the-tdd-methology-in-software-development.md`. The column the
design named as already correct was not "fixed" into something else.

## TC-4 The copied collection writes to itself — the defect

This is the case the feature exists for. `paths-b` is a copy of an already
migrated `paths-a`: same notes, same database, different directory.

**Result: PASS.** **Study → Read** listed all five sources with their progress
percentages (`c2-01-read-list.png`), so `notes_path` resolved against the *open*
collection rather than failing. Opening **Spaced repetition** rendered the
source document beside its `.n4l` notes, which showed `# @progress char=0`
(`c2-02-doc-open.png`). Scrolling well into the document
(`c2-03-scrolled.png`) and leaving it wrote the new position:

```
paths-b/reading/spaced-repetition.n4l:5:  # @progress char=14902
paths-a/reading/spaced-repetition.n4l:5:  # @progress char=0      ← unchanged
```

Re-opening `paths-b` later restored the document at `char=14902` / 57%
(`c3-01-doc.png`), so the position round-trips through the file rather than
being written and lost.

Other write paths were exercised in the same session: grading a card in
**Recall** appended to `review_logs` (37 → 38 rows) and the card rendered with
its media image resolved from `paths-b/media/` (`c2-07-recall.png`,
`c2-08-answer.png`); **Later** on the Process queue advanced it and bumped
`deferral_count`; creating an extract wrote a new `extracts` row.

## TC-5 The original collection is byte-identical afterwards

**Result: PASS.** `paths-a`'s 82 files were fingerprinted by mtime, size and
SHA-256 immediately before `paths-b` was opened, and again after the app quit:

```
$ diff a-before.txt a-after.txt          # mtime + size — empty
$ diff a-before-sha.txt a-after-sha.txt  # sha256       — empty
```

Not one file changed, including `learning.db`. Against the pre-fix code this is
precisely what failed: the copy's database still named `paths-a`'s files, and
Study read and wrote them. This is the regression the Go test in Task 3
reproduces, confirmed here on real data through the real UI.

## TC-6 A renamed note fails visibly in Process

`paths-b/reading/spaced-repetition.n4l` was renamed to
`…-RENAMED.n4l`, leaving rows pointing at a file that no longer exists.

**Result: PASS.** The Process pane shows, in place of the editor and in red:

> `parse /home/simone/.claude/jobs/ad91db08/tmp/paths-b/reading/spaced-repetition.n4l failed`

(`c3-08-process-renamed.png`). It names the file, the app did not crash, did not
hang, and did not silently skip to another row — the pane stays on the failed
item and the surrounding chrome (tabs, notebook panel, buttons) is intact.

Two things are worth reading off that string. The path is resolved **under the
open collection** (`…/paths-b/…`), which is the containment guard doing its job:
a pre-fix build would have named `…/SStorytimeFork/…/test/…`. And the wording is
"parse … failed" for a file that does not exist, where "no such file" would say
more; that is a message-quality nit, not a defect.

## TC-7 A renamed note in Read — **FAIL**

**Result: FAIL.** Opening the same reading item from **Study → Read** replaces
the *entire application view* with a bare red line
(`c3-10-open-renamed-doc.png`):

> `UNHANDLED REJECTION: GetFileContent: open /home/simone/.claude/jobs/ad91db08/tmp/paths-b/reading/spaced-repetition.n4l: no such file or directory`

The window is then dead: clicking and `Escape` change nothing
(`c3-11-after-rejection.png`), the process is still alive but the UI never comes
back, and only restarting the app recovers. The message is readable and names
the file — the resolution half of the requirement is met — but "does not crash"
is not met in any sense a user would recognise.

**Cause, at source:** `frontend/src/components/ReadingView.svelte:72` and `:140`
both do

```js
const text = (await GetFileContent(reading.notesPath)) ?? ''
```

with no `try`/`catch` and no `.catch`, so a rejected binding call escapes as an
unhandled promise rejection. Sibling call sites already guard this —
`SourceTab.svelte:193` and `:365`, `ProcessEditor.svelte`, and
`explorer/EditCard.svelte:115` all catch and log — so `ReadingView` is the
outlier rather than the house style.

**Not introduced by this branch.** The missing `catch` predates it. But the
design's own acceptance line is "confirm an unresolvable row produces a readable
error", and on the Read path it produces a dead window instead. Left as a
finding, not fixed, per this task's constraints.

## TC-8 A settled collection is unaffected

`paths-a`, reopened after its migration and never moved since.

**Result: PASS**, across every surface checked.

- **Reading list** — all five sources, with progress and the `done` badge
  (`c4-03-a-read.png`).
- **Reading view** — **Spaced repetition** opens with the source document
  rendered beside its notes, `# @ref "sources/spaced-repetition.html"` resolved
  (`c4-06-a-doc.png`).
- **Recall** — a card renders front and back, with its `media/` image loaded
  (`c4-04-a-recall.png`).
- **Main window and editor** — file tree, chapters, contexts and the node column
  all populate; opening `reading/spaced-repetition.n4l` from the tree loads the
  file and lists its nodes (`c4-07-a-main.png`, `c4-09-a-filetree2.png`,
  `c4-10-a-note-open.png`).
- **Node quick filters** — **Cards** returns the file's three study-marked nodes,
  **Extracts** returns the one `# @id`-without-marker node
  (`c4-11-a-extracts-filter.png`, `c4-12-a-cards-filter.png`). Both are computed
  from file content, not from the database, and both are correct for this file.

## TC-9 Process on a copied collection — **FAIL**

**Result: FAIL.** In `paths-b`, **Study → Next** and **Study → Process** show,
for every pre-existing extract in the queue:

> `node "/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test/reading/incremental-learning-supermemo-help.n4l:Incremental learning - SuperMemo Help:id:8vlaho1t" not found in /home/simone/.claude/jobs/ad91db08/tmp/paths-b/reading/incremental-learning-supermemo-help.n4l`

(`c2-05-next.png`, `c2-06-process.png`; a second extract gives the same shape
with a different short id, `c3-05-process2.png`.) The *file* half resolved
correctly — the app opened the right file in the right collection. The *node*
half did not: the row's identity key still carries the original absolute path.

**Cause.** The schema-7 migration rewrites four `file_path`-style columns
(`internal/learning/paths.go:84-97`). Four **other** columns store a node key of
the form `<absolute path>:<chapter>:id:<short>`, and none of them is migrated:

| Column | Rows | Still holding the original absolute path |
|---|---|---|
| `items.id` | 15 | **15** |
| `items.node_id` | 15 | **15** |
| `extracts.id` | 20 | **20** |
| `notebooks.extract_node_id` | 8 | **8** |

`notebook_methods.go:361-401` (`NotebookMarks`) rebuilds each node's id from the
file it just parsed — i.e. from the *current* path — and looks it up against
`notebooks.extract_node_id`, so notebooks attached to nodes are unreachable in a
copied collection for the same reason. No notebook marker appeared on
`reading/spaced-repetition.n4l` in `paths-a` (`c4-10-a-note-open.png`) despite
a `notebooks` row existing for a node in that file.

**This is a gap in the feature, not merely a pre-existing bug.** The design's
motivation is "a user who moves or copies their collection will have Study read
and write the old location", and its *Out of scope* list names `ref_path`, the
repair UI, the graph store and the Bleve index — it does not mention identity
keys at all. After this branch, a copied collection no longer damages the
original, which is the serious half; but its extracts, its card→node links and
its notebooks are still broken by the move. **Process** in particular is dead
for every legacy row.

**Confirmed by control, not inferred.** An extract created *in* `paths-b`
(`…/paths-b/reading/spaced-repetition.n4l:Spaced repetition:id:z1m43l5c`)
renders perfectly in the same Process pane — notes file, extract text, source
context and an editable notebook (`c3-07-later4.png`). Same pane, same file,
same session: the only difference is whether the id matches the current
location.

## TC-10 New rows are still written with absolute identity keys — **FAIL**

**Result: FAIL.** The extract created during TC-9's control was written as:

| Field | Value |
|---|---|
| `file_path` | `reading/spaced-repetition.n4l` — **relative**, the fix working |
| `id` | `/home/simone/.claude/jobs/ad91db08/tmp/paths-b/reading/spaced-repetition.n4l:Spaced repetition:id:z1m43l5c` — **absolute** |

So this is not only a backlog of legacy rows: every extract, card and notebook
created after the fix still embeds the collection's absolute path in its key,
and will break the next time the collection moves. A migration that rewrote the
four id columns once would not hold; the key format itself is the issue.

## Not established by this run

- **A collection that has genuinely never moved.** The only such collection is
  the user's real one, and this task forbids opening it — correctly. `paths-a`
  is a copy, so its identity keys were already stale before the app ever saw it,
  which is why TC-9's symptom appears there too. That a stationary collection's
  Process pane works is an inference from the key format (its ids would embed
  its own path), not an observation.
- **Windows path separators.** `rehome` documents that a value written with
  backslashes will not be segmented and will fail safe. This run is Linux-only.
- **Symlink escape.** `collectionpath.Resolve`'s symlink handling has unit tests
  in Task 1; no symlinked collection was constructed here.
- **`sst_data/` and `bleve/`.** Out of scope by design (rebuilt, not migrated);
  Build Graph was not run.

## Nothing was changed in the product

No production code was touched. The only repository change is this document. The
collection copies, screenshots and the throwaway X11 driver live under
`~/.claude/jobs/ad91db08/` and outside the repository; neither copy was merged
back. `~/.config/texere/config.json` is byte-identical to its pre-run state.
