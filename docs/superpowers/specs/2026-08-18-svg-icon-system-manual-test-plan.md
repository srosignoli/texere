# SVG icon system — manual test plan (executed)

**Spec:** `2026-08-18-svg-icon-system-design.md`
**Plan:** `../plans/2026-08-18-svg-icon-system.md` (Task 5)
**Build:** `make build && ./build/bin/texere`, built at commit `ab6d90f`
**Run date:** 2026-08-18. Every case below was **executed**, not described.

## What needed proving

`icons.test.js` proves each path is well-formed, that every `<Icon name=…>`
resolves, and that no astral-plane emoji remain in the source. None of that
proves an icon is *visible in the running app*, and there is no Svelte component
test harness. This document is that evidence.

## How this run was set up

- The app ran against a **copy** of the user's real collection in the job scratch
  directory, launched with `XDG_CONFIG_HOME` pointed at a scratch config naming
  the copy. `~/.config/texere/config.json` was never modified; it was diffed
  against a pre-run backup afterwards and is identical. The real collection's
  82-file manifest (mtime + size) is likewise identical to its pre-run state.
- This box has no screenshot tool and no PIL, so capture used the `XGetImage`
  and XTEST drivers written for feature 049's manual test plan.
- The icon geometry itself was proved before any of this, in Task 1: all nine
  rendered at 72px in a throwaway page, screenshotted through headless Chrome
  (module imports are CORS-blocked over `file://`, so the page was generated
  from the real `ICONS` data with the SVG inlined) and read back.

## Results

| # | Case | Result |
|---|---|---|
| 1 | Nine icons read as their names | **PASS** |
| 2 | Toolbar: Save, Search, Explorer, Read, Study | **PASS** |
| 3 | OmniBar search icon, both modes | **PASS** |
| 4 | SketchCell expand control, both states | **PASS** |
| 5 | Study empty state | **PASS (with a note)** |
| 6 | `+ Card` survives, `💬` gone | **PASS** |
| 7 | Icons follow the theme | **NOT VERIFIABLE** — see below |

### 1. The geometry reads correctly — PASS

All nine were checked against their names at 72px: floppy disk (`save`),
magnifier (`search`), wire globe (`globe`), open book (`book-open`), book spines
(`books`), diamond (`graph`), outward corner brackets (`expand`), inward corner
brackets (`collapse`), party popper (`celebrate`).

The plan flagged this geometry as transcribed from memory and therefore
untrusted, and told the implementer to expect to redraw some. **None needed
redrawing.** That is a better outcome than the plan assumed, not a skipped step.

### 2. The toolbar — PASS

Save, Search, Explorer, Read and Study all draw shapes. Before this feature the
same five rendered as tofu boxes; the 049 screenshots show them as `▯`.

`↩` (undo) and `⚙` (settings) still render as text symbols, which is correct —
they are BMP, they were never broken, and the spec puts them out of scope.

The Explorer button was captured in both its disabled state (no chapter
selected, icon dim) and its enabled state (chapter selected, icon bright).

### 3. OmniBar, both modes — PASS

- Ctrl+K → fuzzy mode: **magnifier**, badge reads FUZZY.
- Ctrl+Shift+K → graph mode: **diamond**, badge reads GRAPH.

Both draw. Before this feature the fuzzy branch was `🔍` (tofu) and the graph
branch `◈` (fine), so toggling turned the icon into a box.

**This case produced a fix.** The icon shown in *graph* mode had been named
`fuzzy` in Task 1 — named after the state it is never displayed in. No test
could catch that: a backwards name resolves exactly as well as a correct one.
Renamed to `graph`, matching both the badge and the `omnibarMode` store value
(commit `4ce5fa6`).

### 4. SketchCell expand control, both states — PASS

Collapsed: **outward** corner brackets + "Expand". Expanded (filling the
window): **inward** corner brackets + "Collapse". Both draw, and the pair reads
as opposites at a glance.

Before this feature the expanded branch was `🗕` (tofu) and the collapsed branch
`⛶` (fine) — the same half-rendering defect as the OmniBar.

### 5. Study empty state — PASS, with a note

"Nothing due." followed by the celebrate icon. It draws.

**Note, for the user's judgement rather than a defect:** at the default 16px the
popper is small and sits tight against the full stop, so it reads more as a
speck than as a flourish. The emoji it replaced was larger and more legible. If
the empty state is meant to feel celebratory, this icon wants roughly 20px and a
little left margin. Left as-is because icon size in a decorative empty state is
a taste call, not a correctness one.

### 6. `+ Card` survives, `💬` gone — PASS

Hovering a node row in Column 3 shows a single `+ Card` button. The `💬` button
is gone.

Both halves were also asserted mechanically after the deletion: 0 `Discuss`
stubs remain, all 4 `+ Card` buttons survive, and all 4 `hover-actions` divs
still stand.

### 7. Icons follow the theme — NOT VERIFIABLE on this build

The plan called for capturing the toolbar in light and dark and comparing. **The
app has no such toggle yet.** The Appearance menu offers one theme, "Blue", and
states "Dark and Light are in progress." There is nothing to switch between, so
the comparison could not be made and is not claimed.

Partial evidence does exist. The Explorer icon renders **dim when the button is
disabled and bright when enabled** — the icon changed with the button's text
colour, which is what `stroke="currentColor"` is for. That demonstrates the
mechanism works; it does not demonstrate it across two themes, which remains
untested until the themes ship.

## An artifact worth recording

The Process pane showed `node "8vlaho1t" not found` throughout this run. That is
neither an icon defect nor a regression.

The scratch collection was migrated to schema 8 (short identity keys) during
feature 049's verification, but this branch is based on `main`, which predates
049 and still computes path-shaped node IDs. Short keys in the database cannot
match path-shaped keys computed from the files, so nothing resolves. It is an
artifact of reusing one scratch collection across two branches, and it will
disappear once 049 merges. Icon verification was unaffected: every surface under
test renders independently of whether an extract resolves.

## A pre-existing bug found while testing

**The Search button in the toolbar crashes the UI.** Clicking it throws
`TypeError: fF.update is not a function` and the whole webview goes blank, with
only the error text left.

`MiddleBand.svelte:62` calls `omnibarOpen.update(v => !v)`, but
`stores/workbench.js:143` defines `omnibarOpen` as
`{ subscribe: omnibarMode.subscribe }` — a subscribe-only alias with no `update`.

This predates the icon work: at `main` (`446e2eb`) both lines are byte-identical,
and this branch's diff to `MiddleBand.svelte` touches only button markup, the
`Icon` import and three `aria-label`s. It was found because converting the button
was the first reason anyone had clicked it.

Ctrl+K, the documented shortcut, works — it goes through `omnibarMode` directly.
Only the button is broken.

**Not fixed here**, because it is unrelated to this feature and deserves its own
change. The fix is one line: `omnibarMode.update(m => (m ? null : 'fuzzy'))`, or
give `omnibarOpen` a real `update`.

## Cleanup

- The app was stopped and the icon-proof harness deleted in Task 1.
- The real collection and `~/.config/texere/config.json` were verified unchanged.
- The scratch collection, capture drivers and screenshots live in the job scratch
  directory and are not part of the repository.
