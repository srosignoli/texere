# Keyboard shortcuts help panel — manual test plan (executed)

**Spec:** `2026-08-19-shortcuts-help-panel-design.md` (referenced by the task
brief; not re-derived here)
**Plan:** `.superpowers/sdd/2026-08-19-shortcuts-help-panel/` (Task 4)
**Build:** `make build && ./build/bin/texere`, built at commit `9ea6a0d`
(`feat(shortcuts): a help panel on ? and F1`)
**Run date:** 2026-08-19. Every case below was **executed**, not described.

## The claim under test

Tasks 1–3 built `lib/shortcuts.js` (the shortcut inventory as data),
`shortcuts.test.js` (a guard that scans the source for modifier key
combinations and checks they're documented, and vice versa), and
`ShortcutsPanel.svelte` (a slide-in panel listing every group, opened by a
`?` button or F1). The guard can prove a binding **exists** at a claimed
site and that no modifier shortcut is **undocumented**. It cannot prove a
label describes what the binding actually **does** — "Build graph" next to
`Ctrl/Cmd+G` passes review whatever that handler runs. This plan exists to
catch exactly that gap, plus the two things no automated test can touch at
all: F1 (exempted from the guard by name) and how the panel actually
renders pixel-for-pixel in three themes.

## How this run was set up

- The app was pointed at `keys-b`, a **copy** of the user's real collection
  (itself already a copy from feature 049's manual run — never the original
  path) via `XDG_CONFIG_HOME` set to a scratch directory holding its own
  `config.json`. The real collection at
  `…/SStorytimeFork/SStorytime-main/examples/test` was never opened by this
  run — confirmed by not touching it at all during the session; no process
  in this run held a handle to it.
- The app was driven with synthetic X11 input (`xin.py` for clicks/keys,
  `xcap.py` for window capture) on `DISPLAY=:0`, both living in the job
  scratch directory only. Every screenshot named below was read back with
  the Read tool; the observations recorded are what was seen in them, not
  assumed from the action taken.
- `node --test frontend/src/lib/shortcuts.test.js` was run before driving
  the UI: all 6 guard tests pass (site-exists, no-undocumented-modifier,
  no-duplicate-group-membership, vacuity, and the two scan-integrity
  checks). This run does not repeat what that guard already covers; it
  covers what the guard structurally cannot.

## Results

| # | Case | Result |
|---|---|---|
| 1 | `?` button appears beside `⚙` | **PASS** |
| 2 | Clicking `?` opens the panel; every group renders | **PASS** |
| 3 | F1 opens the panel from the main window | **PASS** |
| 4 | Escape closes the panel | **PASS** |
| 5 | Panel renders correctly — Light theme | **PASS** |
| 6 | Panel renders correctly — Dark theme | **PASS** |
| 7 | Panel renders correctly — Blue theme | **PASS** |
| 8 | Explorer entries show `Alt ←` / `Alt →` with real glyphs | **PASS** |
| 9 | `Ctrl/Cmd+G` "Build graph" does what it says (Global) | **PASS** |
| 10 | `Ctrl/Cmd+S` "Save file" does what it says (Editor) | **PASS** |
| 11 | `Ctrl/Cmd+Enter` "Run cell / send" does what it says (Notebook cells) | **PASS** |
| 12 | `Alt+←` / `Alt+→` "History back/forward" (Explorer) | **PARTIAL — see below** |
| 13 | Label check: `Ctrl/Cmd+Shift+S` "Open Study" | **MISLABELLED — it toggles** |
| 14 | Label check: `Ctrl/Cmd+Shift+R` "Open Recall overlay" | **MISLABELLED — it toggles** |
| 15 | Label check: `Ctrl/Cmd+Shift+M` "Toggle log panel" | **PASS — label matches behaviour** |

### 1–2. The `?` button and the panel it opens — PASS

`t4-00-main.png`: startup screen (Light theme, default), toolbar row with
`? ⚙` at the right end (`?` at approx. x=955, `⚙` at x=990, y=360).

`t4-01-panel-light.png`: clicking `?` opens a right-side slide-in panel
titled "Keyboard shortcuts" with a `✕` close button. All five groups
render with their headers and rows: **GLOBAL** (Build graph, Search, Graph
query, Open Study, Open Read, Open Recall overlay, Toggle log panel, This
panel), **EDITOR** (Save file), **STUDY & READ** (Extract selection, Save
notes), **EXPLORER** (Focus query bar, History back, History forward), and
**NOTEBOOK CELLS** (visible at the bottom edge, confirmed to render fully
in a later screenshot). Every row shows `kbd`-styled key chips and a
right-aligned label.

### 3. F1 opens the panel — PASS

The panel was closed via Escape (`t4-02-after-escape.png` — panel gone,
`?` button shows a focus ring), keyboard focus was moved into the editor
by clicking it, then F1 was pressed. `t4-03-f1-open.png` shows the panel
open again, with the editor's cursor visible behind it — proof F1 fired
from the main window, not from residual focus on the `?` button. This is
the only check in this project that exercises F1 at all: `shortcuts.js`'s
own comment names it "the one exception, exempted by name" from the
guard's modifier-combo scan.

### 4. Escape closes the panel — PASS

Demonstrated twice: once explicitly in case 3's setup (`t4-02`), and
implicitly every time the panel was reopened for a new theme in this run.

### 5–7. All three themes — PASS

Switched via the gear icon's Appearance menu (`t4-04-settings.png` shows
the Blue/Dark/Light radio group, Light selected by default).

- **Light** (`t4-01-panel-light.png`): white panel surface, near-black
  text, light-gray `kbd` chips with a visible border — legible throughout.
- **Dark** (`t4-06-panel-dark.png`, after selecting Dark in
  `t4-05-dark-main.png`): near-black panel surface, off-white text, cyan
  group headers, `kbd` chips with a subtle border distinguishable from the
  background. No unstyled/invisible text.
- **Blue** (`t4-08-panel-blue.png`, after selecting Blue in
  `t4-07-blue-main.png`): navy panel surface, light text, the same chip
  and header styling scaled to the theme's palette. No unstyled/invisible
  text.

No literal colours were introduced or needed for this check — the panel's
`--surface-overlay` / `--text` / `--help-*` tokens already carry the
theme, and all three renders confirm that binding held.

### 8. Explorer glyphs — PASS

Visible in all three theme screenshots above (each shows the full panel
including the Explorer group): the two rows read **`Alt ←`** and
**`Alt →`** — real arrow characters rendered as `kbd` chips next to `Alt`,
not the strings `ArrowLeft` / `ArrowRight`. Confirmed identically in
Light, Dark and Blue.

### 9. `Ctrl/Cmd+G` "Build graph" — PASS

With a chapter loaded, `Ctrl+G` was pressed. `t4-09-buildgraph.png` shows
a "Building graph…" toast appear immediately; three seconds later
`t4-10-buildgraph-done.png` shows it replaced by "Graph ready — 22406
nodes, 23191 links (12364ms)". The label describes exactly what happened.

### 10. `Ctrl/Cmd+S` "Save file" — PASS

A space was typed at the end of a line in the open chapter. The file tree
row for that chapter grew an orange dirty dot (`t4-29-editor-dirty.png`).
`Ctrl+S` was pressed; `t4-30-editor-saved.png` shows the dot gone and the
editor re-scrolled to the top (consistent with a re-parse-after-save). The
label matches the observed effect.

### 11. `Ctrl/Cmd+Enter` "Run cell / send" — PASS

A Shell·bash cell was added to the open notebook (Study → Process →
Notebook → Shell), `echo shortcut4` was typed into it
(`t4-38-shell-typed.png`), and `Ctrl+Enter` was pressed.
`t4-39-shell-ran.png` shows output `shortcut4` with `exit 0 · 0.0s`
appended below the cell. The label matches: the binding ran the cell.

### 12. `Alt+←` / `Alt+→` "History back / History forward" — PARTIAL

Source (`GraphExplorer.svelte`) unambiguously wires `Alt+ArrowLeft` /
`Alt+ArrowRight` to `historyBack()` / `historyForward()`, which call
`performReanchor(id, { source: 'history' })` — the same re-anchor pipeline
used by clicking a search result while the explorer is open. This was
exercised as follows:

- Opened the 2.5D Graph Explorer on the "Astronomy by the numbers poem"
  chapter (`t4-16-explorer-open.png`).
- Used the in-explorer search (`Ctrl+K`) to jump to a different node
  ("By law of phy6 great") in the same result set — a reanchor via
  `source: 'omnibar'` (`t4-20-search-typed.png` → `t4-21-reanchored.png`).
- Pressed `Alt+←` then `Alt+→` (`t4-23-altleft.png`, `t4-25-altright-fixed.png`)
  — both fired without error and the app remained responsive.
- Closed the explorer and confirmed via the main workbench: the NODES
  panel highlighted "By law of phy6 great" and the editor auto-scrolled to
  its line (`t4-28-explorer-x-closed.png`) — proof the reanchor mechanism
  itself persisted state correctly.

What this does **not** prove: a clean visual back-then-forward round trip.
The test chapter's whole result set (25 nodes across two small diamonds)
renders inside one fit-to-bounds camera frame regardless of which node is
the current anchor, so pressing `Alt+←` produced no perceptible camera
change in the screenshots, and neither did `Alt+→` afterwards. Since the
net effect of back-then-forward is indistinguishable from a no-op in this
data, this run cannot rule out "the keys are wired to a no-op" purely from
pixels — only source inspection plus the absence of any error rules that
out. **An honest "not fully visually verified" rather than a claimed
pass.** A future run with two results that are spatially far apart in the
layout (e.g., a query spanning multiple chapters) would make this
decisive.

One input-syntax mistake happened during this case and is recorded rather
than hidden: `xin.py key texere Alt_L Right` was first run with the
modifier and key in the wrong argument order (the tool's syntax is
`key <window> <keysym> [modifier...]`), which sends the physical keys in
an order that does not set `e.altKey` on the `ArrowRight` keydown. This
was caught before drawing any conclusion from it and re-run correctly as
`xin.py key texere Right Alt_L` for the screenshot cited above.

### 13–14. Toggle labels — MISLABELLED (both), confirmed by direct observation

Per the task brief, this was pressed and observed rather than inferred
from the diff alone.

- **`Ctrl/Cmd+Shift+S`, labelled "Open Study":** pressed once — Study
  surface opened on the Next queue (`t4-11-shiftS-open.png`, "Nothing
  due."). Pressed **again** — Study closed and the main workbench
  returned (`t4-12-shiftS-closed.png`). This is a toggle, not an
  open-only action.
- **`Ctrl/Cmd+Shift+R`, labelled "Open Recall overlay":** pressed once —
  the Recall overlay opened showing a due card (`t4-13-shiftR-open.png`).
  Pressed **again** — it closed back to the main workbench
  (`t4-14-shiftR-closed.png`). Also a toggle.
- **Source confirms the mechanism** (`App.svelte`'s `handleKeydown`):
  both handlers read `studySurfaceOpen.update((v) => !v)` /
  `studyOpen.update((v) => !v)` — genuine toggles, not conditionals that
  only ever set `true`.

**Recommendation:** rename these two labels to "Toggle Study" and "Toggle
Recall overlay" for consistency with the correctly-labelled
`Ctrl/Cmd+Shift+M` ("Toggle log panel", case 15). Per the task brief this
is **not** applied here — `shortcuts.js` is data the Task 2 guard covers
for key/site correctness only, not label wording, so an unreviewed label
edit inside a verification task would be out of process. The brief traces
this inconsistency to the design spec's own Scope table, not to any
implementation task, so the fix belongs wherever that table gets revised.

### 15. `Ctrl/Cmd+Shift+M` "Toggle log panel" — PASS, label matches

Pressed once — log panel opened at the bottom of the main window showing
17749 entries / 4 warnings (`t4-40-logpanel-open.png`). Pressed again —
panel closed, editor visible again (`t4-41-logpanel-closed.png`). The
label already says "Toggle" and the behaviour matches; this is the
reference case the other two should be renamed to match.

## Addendum: Ctrl/Cmd+F and the rest of the CodeMirror editor family (post-review fix)

A user reported, before merge, that `Ctrl+F` (opens the editor's search panel)
was missing from the panel. Investigation found the gap was structural, not a
one-off oversight: `SourceTab.svelte` and `lib/n4l-editor.js` wire up
CodeMirror's `searchKeymap` and `historyKeymap` by **spreading the whole
array** (`...searchKeymap`), not as `{ key: '...', run: ... }` literals — the
only shape shortcuts.test.js's CM regex could see. Every binding in those two
keymaps was invisible to the guard by construction.

Fixed by: adding eight entries to the Editor group (`Ctrl/Cmd+F` open search,
`F3` find next, `Escape` close search, `Ctrl/Cmd+D` select next occurrence,
`Ctrl/Cmd+Shift+L` select all occurrences, `Ctrl/Cmd+Alt+G` go to line,
`Ctrl/Cmd+Z` undo, `Ctrl+Y`/`Cmd+Shift+Z` redo — the last one genuinely
platform-specific, not a `Ctrl/Cmd` row), each carrying a `viaKeymap` field;
and teaching shortcuts.test.js to detect any `...xKeymap` spread in the tree
and require it declared in a new `KEYMAPS` registry in shortcuts.js, with a
substantive reason per entry (not a bare allowlist). `defaultKeymap`'s
ordinary editing conventions (Ctrl+A, Home/End, arrow movement) were left
undocumented deliberately — out of scope per the existing "shortcuts worth
memorising" rule. `extraKeymap` is not a library keymap at all (a local
function parameter carrying bindings documented individually elsewhere) and
is declared as such.

**The `Ctrl/Cmd+G` clash, recorded, not changed:** CodeMirror's own
`searchKeymap` binds `Mod-g` to find-next, but `App.svelte`'s global keydown
handler binds `Ctrl/Cmd+G` to Build graph in the **capture phase** with
`stopPropagation()` (see the comment at `App.svelte:100-105`), so
`searchKeymap`'s `Mod-g` never reaches the editor — deliberately, per that
comment, not a bug. The panel therefore documents **F3** as the working
find-next key, not `Ctrl/Cmd+G`. No key routing was changed by this fix.

**Verified in the running app** (`make build && ./build/bin/texere`, pointed
at the `keys-b` copy collection via `XDG_CONFIG_HOME`, same setup as the rest
of this plan — the real collection at
`…/SStorytimeFork/SStorytime-main/examples/test` was never opened):

- F1 opens the panel; the Editor group now lists all nine rows (Save file
  plus the eight above), matching `lib/shortcuts.js`.
- With focus in the editor, `Ctrl+F` was pressed for real (via synthetic
  X11 input) and the CodeMirror search panel opened at the bottom of the
  editor — a "Find" field, next/previous/all, match case/regexp/by word
  toggles, and a replace row — confirming the user's report is now both
  documented and true. (One input-syntax mistake happened first: `xin.py`'s
  modifier argument must be a real X keysym name like `Control_L`, not the
  string `ctrl` — the first attempt silently typed a literal `f` into the
  document instead of triggering the shortcut, caught immediately from the
  screenshot and undone with `Ctrl+Z` before retrying correctly.)
- The `shortcuts.test.js` guard was proven to bite: a fake `...lintKeymap`
  spread was added to `lib/n4l-editor.js`, `node --test` failed naming
  `...lintKeymap at lib/n4l-editor.js:72`, and the change was reverted
  (confirmed clean via `git diff`).

## What this run does not cover

- **Every shortcut in the panel individually.** Task 4's brief calls for
  three spot-checks across three groups, not exhaustive re-verification —
  the guard already proves every documented combo exists at its claimed
  site. This run added `Ctrl/Cmd+G`, `Ctrl/Cmd+S`, and `Ctrl/Cmd+Enter` as
  the three, plus the toggle-label checks and a partial Explorer
  History check as extra scope.
- **Ctrl/Cmd+K "Search", Ctrl/Cmd+Shift+K "Graph query", Ctrl/Cmd+Shift+E
  "Open Read", Ctrl/Cmd+E "Extract selection", Ctrl/Cmd+S "Save notes"
  (Study & Read), Ctrl/Cmd+Shift+K "Focus query bar" (Explorer).** Not
  pressed in this run. `Ctrl/Cmd+K` was used incidentally while setting up
  case 12 (to reach the explorer's search), which is weak indirect
  evidence it opens something, but it was not evaluated against its
  specific label.
- **A visually decisive Explorer History back/forward round trip.** See
  case 12 — the mechanism is confirmed by source and by one side-effect
  (workbench state after a reanchor), but a clean two-screenshot "camera
  moved back, then moved forward again" pair was not obtained because the
  test collection's graph is too compact to show it. Recorded as PARTIAL
  rather than PASS.
- **Windows/macOS key rendering** (the panel always shows `Ctrl/Cmd`,
  regardless of host OS) — out of scope per the spec's own Scope table,
  not re-litigated here.

## Cleanup

- The app was stopped (`pkill -f build/bin/texere`) after the run.
- `keys-b`'s `config.json` still points `XDG_CONFIG_HOME`'s scratch config
  at the copied collection, not the real one; confirmed by reading it back
  after the app exited.
- The real collection at
  `…/SStorytimeFork/SStorytime-main/examples/test` was not opened this
  run — verified by inspection, no diff to take since nothing in this run
  ever pointed at it.
- Screenshots, the shell scratch scripts (`xin.py`, `xcap.py`) and the
  scratch `XDG_CONFIG_HOME` tree live in the job scratch directory and are
  not part of the repository.
