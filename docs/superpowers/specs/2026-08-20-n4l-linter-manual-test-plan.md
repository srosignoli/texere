# N4L linter — manual test plan (executed)

> **Note added 2026-08-24.** This document records a test run made *before*
> the `contentSQStart` apostrophe fix. The counts below (107 defects across 7
> files, 106/6 after the in-app repair) were correct for the code as it stood
> that day. 85 of those defects were parser false positives — see the
> correction in `2026-08-20-n4l-linter-design.md`. Re-running the same steps
> today yields **22 defects across 5 files**, with `Darwin.n4l` and `NDA.n4l`
> unmarked. The observations about UI behaviour — marker appearance, grouping,
> navigation, live re-lint on save — all still hold.


**Spec:** `2026-08-20-n4l-linter-design.md`
**Plan:** `../plans/2026-08-20-n4l-linter.md` (Task 5)
**Build:** `make build && ./build/bin/texere`, built at commit `d21d12f`
(`fix(lint): Problems tab review round 1 — dup keys, missing re-lint, arrow
truncation, tab badge`), the tip of `055-n4l-linter` at the time of this run.
**Run date:** 2026-08-23. Every case below was **executed**, not described.

## The claim under test

The parser already classifies every line it cannot make sense of, but until
this feature nothing surfaced that to the user except a scrollable Log tab
mixed in with routine parse chatter. The design's bet is a three-way split:
**defects** (the parser read something other than what was written — misread
content) get a red `!` on the file in the tree and a dedicated Problems tab
entry; **vocabulary** (an undeclared arrow — the note is fine, it just won't
become a graph edge yet) is aggregated across the whole collection and shown
separately, and **never marks a file**. If vocabulary findings marked files
the same way defects do, the feature would flag 39 of 53 files as broken when
only 7 actually have misread content — the marker would be noise, not signal.
This run's job is to check that split holds in the running app, that fixing a
defect actually clears both surfaces, and that the pre-existing Log tab is
untouched.

## How this run was set up

- The collection under test, `lint-copy`, is a **fresh copy** of the user's
  real collection at `…/SStorytime-main/examples/test`, made with `cp -a`
  into the job scratch directory immediately before this run (a stale copy
  from an earlier feature's testing existed in the scratch directory but was
  not reused, to rule out any edits it might already carry).
- **The real collection was never opened by the app.** A full manifest of all
  83 files under it (path + size + mtime) was taken before and after this run
  and diffed: **identical**.
- `~/.config/texere/config.json` was **not touched**. The app was launched
  with `XDG_CONFIG_HOME` pointed at a scratch directory holding its own
  `config.json`, naming `lint-copy` as `collectionDir` (`os.UserConfigDir`
  honours `XDG_CONFIG_HOME`). Theme switches during this run wrote to that
  scratch config only.
- The app was driven with synthetic X11 input (XTEST) on `DISPLAY=:0`, using
  the `xcap.py` / `xin.py` helpers already in the scratch directory from prior
  runs, plus two written for this run: `xscroll.py` (wheel events via
  `XTestFakeButtonEvent` buttons 4/5 — the panels involved are longer than one
  screenful and `xin.py` had no scroll support) and `xtype.py` (types a
  literal string key-by-key via XTEST, used to edit a note in place). Every
  screenshot named below was read back as an image; the observations recorded
  are what was actually seen in them.
- **Independent, non-UI verification.** Alongside the driven UI, a throwaway
  test (`internal/content/zz_task5_verify_test.go`, removed before commit —
  never part of this branch's history) called `LintCollection` directly
  against `lint-copy` and printed the aggregate. This is the same production
  function the UI calls through `App.LintCollection`, so it is not an
  independent reimplementation, but it does rule out any bug that exists only
  in the Svelte rendering layer (wrong count displayed, wrong file
  attribution in the panel, stale cache) by comparing the Go struct directly
  against what the screen showed.

## Results

| # | Case | Result |
|---|---|---|
| 1 | Baseline: `filesLinted`, defect counts per file, vocabulary top arrows | **PASS** |
| 2 | Vocabulary-only file is not marked (`CFEngine.n4l`) | **PASS** |
| 3 | Fixing a defect and saving clears both the marker and the Problems row | **PASS** |
| 4 | Log tab unchanged: entries, `errors`/`warnings` filters | **PASS** |
| 5 | Marker + Problems tab legible in Light theme | **PASS** |
| 6 | Marker + Problems tab legible in Dark theme | **PASS** |
| 7 | Marker + Problems tab legible in Blue theme | **PASS** |
| 8 | Long arrow names wrap instead of truncating | **PASS** |

### 1. Baseline — PASS

Opening `lint-copy` auto-ran the lint (no user action beyond Open). The
Problems tab badge read **107**, matching the brief's baseline exactly.

Scrolling the Defects panel end to end and cross-referencing the `!N` marker
on each file in the tree gave, file by file:

| file | defects (UI) | defects (Go, independent) |
|---|---|---|
| Darwin.n4l | 74 | 74 |
| NDA.n4l | 10 | 10 |
| chinese.n4l | 18 | 18 |
| json_example_1.n4l | 2 | 2 |
| branches.n4l | 1 | 1 |
| knowledge.n4l | 1 | 1 |
| unicode.n4l | 1 | 1 |
| **sum** | **107** | **107** |

Exactly the 7 files the brief names, exactly the counts it gives. No other
file in the tree carried a red `!` (checked by scrolling the full 53-file
tree top to bottom — screenshots `shot-01-initial.png` through
`shot-12-filetree-nda5.png`).

The Vocabulary panel read **158** rows, top two `has-frag` **8,591** uses (3
files) and `extract-fr` **8,315** uses (3 files) — both screen and independent
Go run agree to the exact use count (`shot-03-problems.png`).

The Go run additionally gave the set-based numbers the design leans on:
**39 of 53 files have some finding, 7 of those have a defect** (32 files have
vocabulary findings only, 6 files have both, 1 has only a defect and no
undeclared arrow) — matching the brief's "39 of 53" framing exactly.

### 2. A vocabulary-only file is not marked — PASS

`CFEngine.n4l` was picked because the independent Go run placed it in the
vocabulary-file set with zero defects. In the running app
(`shot-35-top-tree-blue.png`, Blue theme) it sits between `branches.n4l !1`
and `chinese.n4l !18` in the tree — both neighbours carry the red marker,
`CFEngine.n4l` carries none. This is the property the whole design rests on:
a file that only uses an undeclared arrow reads as normal, not broken.

### 3. Fixing a defect and saving clears both surfaces — PASS

`knowledge.n4l` line 126 read `a material agent, thing or person (!same)
$PREV.1` — the file's one defect, an unresolved `$PREV.1` reference
(`shot-19-editor-line126b.png`, defect shown red-underlined in the editor).
Working inside the app's own editor (not an external edit): selected and
deleted `$PREV.1` with Backspace, typed `an immaterial agent or concept` in
its place (the line above establishes that phrase as the antecedent), then
`Ctrl+S`.

Immediately after save (`shot-24-after-save.png`):

- `knowledge.n4l` in the file tree lost its `!1` marker — plain text, same as
  a file with no findings.
- Reopening the Problems tab (`shot-26-problems-list-after-fix.png`): tab
  badge **106** (was 107), "DEFECTS 106 across 6 files" (was 107 across 7),
  `knowledge.n4l` no longer listed. Vocabulary stayed at **158** — untouched,
  as expected, since the edit fixed a reference, not an arrow declaration.

The independent Go run, re-executed against the now-saved file on disk with
`-count=1` (no cache), reproduced the same **106 defects across 6 files**,
with `knowledge.n4l` absent from the per-file list — confirming the on-disk
write and the re-lint agree, not just the two views of the same stale state.

### 4. Log tab unchanged — PASS

Saving `knowledge.n4l` also re-parsed it for the ordinary parse log, which is
what the Log tab has always shown (unrelated to this feature — it is the
pre-existing per-save parse warning stream). The tab read **44** entries, all
`unknown arrow: "..."` lines tagged `PARSE`, file:line prefixed
(`shot-36-log-tab.png`). Clicking `errors` produced "No entries match the
current filter." (all 44 are warning-level, not error-level —
`shot-37-log-errors-filter.png`); clicking `warnings` showed all 44
(`shot-38-log-warnings-filter.png`). Both filters and the log stream behave
exactly as they did before this branch — the Problems tab is additive, not a
replacement.

### 5–7. All three themes — PASS

The settings gear menu (`APPEARANCE`) was used to switch Light → Dark → Blue.
In each theme the Problems tab (`DEFECTS 106`, `Darwin.n4l 74` in red,
`VOCABULARY 158`, teal arrow names, grey usage bars) and the file-tree marker
(`json_example_1.n4l !2`, seen in every theme's screenshot) stayed legible —
red-on-white in Light, red-on-near-black in Dark, red-on-navy in Blue. No
colour token collided with its background in any theme.

- Light: `shot-25-problems-after-fix.png`, `shot-26-problems-list-after-fix.png`
- Dark: `shot-28-dark-problems.png`
- Blue: `shot-29-blue-problems.png`, `shot-30-blue-closed.png`,
  `shot-35-top-tree-blue.png`

### 8. Long arrow names wrap — PASS

The Go run flagged arrows over the old ~17-character truncation width, e.g.
`ctx PoSST,arrow ArrowPtr` (24 chars), `ctx PoSST, org NodePtr, dst Link,
sttype int` (41 chars). In the Blue-theme vocabulary list
(`shot-34-vocab-wrap2.png`) these rendered as genuine multi-line wraps, not
single-line truncation with an ellipsis:

```
ctx PoSST, org NodePtr, dst
Link, sttype int

ctx PoSST, start NodePtr,
sttype,depth int,limit int

ctx PoSST,chap string,cn
[]string,page int

ctx PoSST,from Node,link
Link,to Node
```

Every character of every name checked was visible — no `…` appeared anywhere
in the vocabulary panel across roughly 40 rows inspected while scrolling.

## What this run does not cover

- **The 31-character arrow name** (`"$1, 'éçàêè...', 'ecaee...'"`, from the
  Go-side length scan) was not individually located and screenshotted; the
  four wrapped rows shown above (24–41 chars) are taken as sufficient
  evidence the wrap, not a fixed-width truncation, is what is rendering.
- **The one defect-file-with-no-vocabulary-finding** (6 of the 7 defect files
  also appear in the vocabulary set; 1 does not) was not identified by name —
  irrelevant to any pass/fail here, noted only because the number came up
  during the Go-side breakdown.
- **No automated coverage was added.** This is expected and stated in the
  plan's self-review: the repo has no Svelte component harness, and this run
  — screenshots plus the independent `LintCollection` calls — is the
  intended check. `go test ./...` (all packages, including the now-removed
  verification test while it existed) and `node --test
  frontend/src/lib/*.test.js` (273 tests, including `theme-tokens.test.js`
  and `icons.test.js`) were both run clean at the end of this session, but
  neither exercises the marker or the Problems tab.
- **Watch-triggered re-lint** (an external process editing a file while the
  app is open) is explicitly out of scope per the plan and was not tested.
- **The declare-arrow action and auto-fix** are explicitly out of scope per
  the plan and do not exist to test.

## Cleanup

- The app was stopped (`pkill -f build/bin/texere`).
- `internal/content/zz_task5_verify_test.go`, the throwaway verification
  harness, was deleted before this document was committed — `git status`
  confirms a clean tree apart from this file.
- `lint-copy`, the scratch `XDG_CONFIG_HOME` tree, `xscroll.py` and
  `xtype.py` live in the job scratch directory only, not in the repository.
- The user's real collection was verified byte-identical (manifest diff,
  above) and `~/.config/texere/config.json` was never opened by this run.
