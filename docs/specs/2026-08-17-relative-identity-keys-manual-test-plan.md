# Location-independent identity keys — manual test plan (executed)

**Spec:** `2026-08-17-relative-identity-keys-design.md`
**Plan:** `../plans/2026-08-17-relative-identity-keys.md` (Task 4)
**Build:** `make build && ./build/bin/texere`, built at commit `d73a3eb`
**Comparison build:** commit `446e2eb` — the merge immediately before this
feature — built in a throwaway `git worktree` under the job scratch directory
and removed afterwards. `HEAD` on the working checkout was never moved.
**Run date:** 2026-08-18. Every case below was **executed**, not described.

## The claim under test

An identity key used to be `<path>:<chapter>:id:<short>`. Copy or move a
collection and every stored key names a path that no longer exists, so nothing
in `learning.db` can be matched back to the notes: the Process pane is dead and
notebooks are unreachable. This feature makes the key the short ID alone and
migrates existing databases (schema 8) so the keys survive relocation.

## How this run was set up

- The collection under test is a **copy** of the user's real collection at
  `…/SStorytime-main/examples/test`, made with `cp -a` into the job scratch
  directory. Three copies were used:
  - `keys-a` — copy of the real collection, migrated in place by opening it.
  - `keys-b` — copy of the **migrated** `keys-a`, at a different path. This is
    the moved-collection case the feature exists for.
  - `keys-c` — copy of the **original, unmigrated** collection at a different
    path. This is what a user copying their collection got under the old code,
    and it is the comparison build's input.
- **The real collection was never opened by either app.** A full manifest of all
  82 files (mtime + size) was taken before and after the run and diffed: it is
  **identical**. Its `learning.db` is still at **schema 6** and no
  `learning.db.bak-v7` was created beside it.
- `~/.config/texere/config.json` was **not modified**. Rather than point the
  user's config at a copy and restore it afterwards, each app was launched with
  `XDG_CONFIG_HOME` set to a scratch directory holding its own `config.json`,
  which `os.UserConfigDir` honours. The real config was diffed against a
  pre-run backup at the end: identical. (The scratch configs carry no LLM
  profiles, so no API key was copied into the scratch tree.)
- The app was driven with synthetic X11 input (XTEST) on `DISPLAY=:0` and each
  state captured by grabbing the app window with `XGetImage`. This box has no
  screenshot tool and no PIL, so both the capture (`xcap.py`, with a stdlib
  `zlib` PNG encoder) and the input driver (`xin.py`) were written for this run
  and live in the scratch directory only. Every screenshot named below was read
  back as an image; the observations are what was read from them.

## Results

| # | Case | Result |
|---|---|---|
| 1 | Migration on real data (`keys-a`) | **PASS** |
| 2 | The claim: copied collection works (`keys-b`) | **PASS** |
| 3 | A settled collection still works (`keys-a`) | **PASS** |
| 4 | New rows use the new format (`keys-b`) | **PASS** |
| 5 | Comparison build on a copied collection (`keys-c`) | **FAILS as expected** |

### 1. The migration on real data — PASS

`keys-a` was opened in the new build. The real collection was at **schema 6**,
not 7, so this Open ran the v7 relative-path migration and the new v8 in one
pass.

Observed afterwards:

- `schema_meta.version` = **8**.
- `learning.db.bak-v7` created beside the database, 278528 bytes.
- The backup holds the **pre-migration** state: `schema_meta.version` = 6 and
  `items.id` = `/home/simone/…/2026-06-22-incremental-reading-manual-test-plan.n4l:2026-06-22-incremental-reading-manual-test-plan:id:73dhx9t2::basic::0`.
- **Acceptance property — zero orphans on both sides:**

  ```sql
  select count(*) from review_logs where item_id not in (select id from items);   -- 0 before, 0 after
  select count(*) from notebooks where extract_node_id not in (select id from extracts); -- 0 before, 0 after
  ```

- Row counts unchanged: items 15, review_logs 37, extracts 20, notebooks 8,
  activity_log 5. Zero orphans is also satisfied by zero rows, so the counts
  were checked to rule that out.
- **No key anywhere still holds a path.** Counted per column, all zero:

  | column | rows still matching `%:id:%` |
  |---|---|
  | `items.id` | 0 |
  | `items.node_id` | 0 |
  | `extracts.id` | 0 |
  | `review_logs.item_id` | 0 |
  | `notebooks.extract_node_id` | 0 |
  | `activity_log.ref_id` | 0 |

- `activity_log` behaved as designed on real polymorphic data. It held two
  `read` rows (`m0uavtdu`, `jslkmpv9` — reading-element ids, no marker) and one
  `process` row carrying a full identity key. After migration the `read` rows
  are **byte-identical** and the `process` row is `yrz6nef7`.

**Observation, not a defect.** The backup is named `.bak-v7` but its
`schema_meta` reads **6**. `migrate()` stamps `schema_meta` once, after the
whole loop, so a database jumping 6 → 8 in one Open still reads 6 while the v8
step runs. Restoring the backup would replay v7 and v8, so it is functionally
restorable; the label is about which migration it precedes, not the number
inside it. This is a pre-existing property of `migrate()`, not something v8
introduced.

### 2. The claim being made — PASS

`keys-a` was copied to `keys-b` (a different path) and opened in the new build.

- The app started, loaded the collection and listed its files
  (`shot-keysb-01.png`).
- Study opened showing **Process 7** and **Recall 7**
  (`shot-keysb-02-study.png`).
- **The Process pane opens** (`shot-keysb-03-process.png`). The left pane shows
  the notes file with the extract at line 47 (`# @id 8vlaho1t`, "Typical
  learning at school puts an emphasis").
- **The notebook is reachable and populated** — same screenshot, right pane:
  EXTRACT "Typical learning at school puts an emphasis", TAGS `notes`,
  NEIGHBORS "follows on from sources/incremental-learning-supermemo-help.html
  char=3253", a SOURCE excerpt, and the full cell-add list.
- Cards render, including embedded media (`shot-keysb-04-recall.png`): a
  spaced-repetition card with its image, and a "Show answer (Space)" control.

Extract `8vlaho1t` is deliberately the one checked here: it is among the
extracts the comparison build reports as dead (case 5).

Backend-level, over every row rather than the one on screen:

| | `keys-a` | `keys-b` (the copy) |
|---|---|---|
| extracts reachable | 6/6 | **6/6** |
| cards reachable | 13/13 | **13/13** |
| notebooks reachable | 8/8 | **8/8** |
| orphaned review logs | 0 | **0** |

"Reachable" here is the Process pane's own test: take the stored id, resolve
the file, parse it, and find the node whose parsed ID equals that id — the
three steps of `App.GetExtractTarget`, whose third step is what a copied
collection used to fail.

Opening `keys-b` did not re-migrate it: still schema 8, and its
`learning.db.bak-v7` is the one inherited from `keys-a`.

### 3. A settled collection still works — PASS

`keys-a` was re-checked after the `keys-b` work: extracts 6/6, cards 13/13,
notebooks 8/8, orphaned review logs 0. A fix that repairs a copied collection
by breaking a stationary one would show up here.

### 4. New rows use the new format — PASS

A file with a `(from)` extract node and a `# @card` node was written into
`keys-b` and the real `learning.Sync` path run over it. Stored keys:

- extracts: `lexxoyzl`, `pitkpdgg`
- items: `5wwlbkht::basic::0`

No path, no `:id:` marker, and the card key is the
`<short>::<cardType>::<discriminator>` shape the spec calls for. The first
attempt at this case produced an extract row but **no** item row — the card
node lacked the `>>` that `basicCards` splits front/back on, so that half of
the check was vacuous. The fixture was fixed rather than the result accepted.

### 5. Comparison build on a copied collection — FAILS as expected

Without this control the four passes above prove nothing: they would look the
same if the bug had never existed. Commit `446e2eb` was built and pointed at
`keys-c`.

- **Process pane is dead** (`shot-keysc-02-process-OLD.png`). Where the new
  build shows the note and its extract, the old build shows only an error:

  > node "/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test/reading/incremental-learning-supermemo-help.n4l:Incremental learning - SuperMemo Help:id:8vlaho1t" not found in /tmp/…/keys-c/reading/incremental-learning-supermemo-help.n4l

  The stored key names the **old** path; the file is at the **new** one.
- **The notebook is empty** — same screenshot: EXTRACT "—", "No graph
  neighbors", "No linked source."
- Backend-level over every row: **extracts 0/6 reachable, cards 0/13
  reachable.** Every Process pane dead, every card unrenderable.

| | new build on `keys-b` | old build on `keys-c` |
|---|---|---|
| extracts reachable | 6/6 | **0/6** |
| cards reachable | 13/13 | **0/13** |

**One nuance, stated precisely.** Notebooks read 8/8 under the old build too.
That is not a contradiction: `GetNotebookByExtract` matches
`notebooks.extract_node_id` against the stored extract key, and both were
written with the same old path, so they still agree with each other. The
breakage is upstream — no id can be *derived from the copied files*, so the
route in is gone. It is accurate to say the notebook was unreachable in
practice; it is not accurate to say the notebook lookup itself was
location-dependent.

## What this run does not cover

- **Reading list.** Plan step 3 names it alongside Study and the notebook. The
  reading-element ids were confirmed intact at the database level
  (`activity_log` "read" rows byte-identical, `reading_elements` untouched by
  the migration — it is not one of the six columns), but the reading list was
  not driven in the UI.
- **The `O_EXCL` on the backup create.** It closes a TOCTOU window between the
  `Stat` and the create that only a second process can open; a single-threaded
  run cannot produce one.
- **Migrating a database that is already at schema 7.** The real collection was
  at 6, so the 7 → 8 path was exercised only by the unit tests in
  `internal/learning/identity_test.go`, not against real data.

## Cleanup

- Both apps stopped. The `git worktree` for `446e2eb` was removed.
- The throwaway verification harness (`zz_task4_verify_test.go`), the X11
  capture and input drivers, all three collection copies and the scratch
  `XDG_CONFIG_HOME` trees live in the job scratch directory and are not part of
  the repository.
- The user's real collection and `~/.config/texere/config.json` were verified
  unchanged, as recorded above.
