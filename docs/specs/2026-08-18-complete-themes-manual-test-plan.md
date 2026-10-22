# Complete Dark and Light themes — manual test plan (executed)

**Spec:** `2026-08-18-complete-themes-design.md`
**Plan:** `../plans/2026-08-18-complete-themes.md` (Task 7)
**Build:** `make build && ./build/bin/texere`, built at commit `d01d5d6` (Task 6
completing "Dark and Light are selectable" — the state Task 7 verifies).
**Run date:** 2026-08-19. Every case below was **executed** against a running
build, not described from reading the diff. Every screenshot named below was
read back with the Read tool; the observations are what was actually seen.

## The claim under test

Tasks 1–6 converted roughly 200 literal colours to CSS tokens and made Dark
and Light selectable alongside Blue. A guard test (`theme-tokens.test.js`)
proves no literal colour remains outside the allowlist — it **cannot** prove
the right token was chosen for a given surface. `--text-muted` where
`--text-secondary` belonged would pass every automated check and still look
wrong forever. This plan is the only thing that catches that: every major
surface, opened on screen, in both themes, read back as a screenshot.

## How this run was set up

- The collection under test is `keys-b`, a **copy** made during an earlier
  feature's verification (`cp -a` of the user's real collection at
  `…/SStorytime-main/examples/test`, relocated to a scratch path). The real
  collection was never opened by this run. `XDG_CONFIG_HOME` was pointed at a
  scratch directory holding its own `config.json` (`collectionDir` → the
  copy), so `~/.config/texere/config.json` was never touched.
- Two additions were made **inside the copy only**, both left in place
  afterward since the copy is disposable scratch data, not the repository:
  - `brains.n4l` gained a `:: theme test ::` block with an ALL-CAPS node
    carrying two arrow properties (`REMEMBER THIS RULE (freq) … (state) …`),
    needed to put a live instance of `NodeCard.svelte`'s `.node-card.reminder`
    override on screen — the real collection has no such node, and Task 5
    explicitly could not verify this surface for lack of one.
  - `brains.n4l` was also touched from a **second, external shell** while open
    in the editor, twice (once per theme), to trigger `SourceTab`'s
    "File changed externally" banner on demand rather than waiting for one.
- The app was driven with synthetic X11 input (XTEST) on `DISPLAY=:0`,
  screenshots taken by grabbing the app window with `XGetImage`. Both drivers
  (`xcap.py`, `xin.py`) were already in the job scratch directory from a prior
  feature and were reused unmodified. Two small additions were written for
  this run, also scratch-only:
  - `xbackslash.py` — this box's keyboard layout binds `backslash` to
    `underscore` at the level `XTestFakeKeyEvent` reaches (confirmed via
    `XGetKeyboardMapping` on keycode 17: level 0/2 is `underscore`, level 4/6
    is `backslash`, and the running X session never switches to that group).
    It works around this by temporarily remapping keycode 17's active level
    to `backslash` via `XChangeKeyboardMapping`, sending the keystroke, and
    restoring the original mapping — the only way this environment can type
    a literal `\` into the app.
  - `xcapid.py` / `xscroll.py` — capture-by-window-ID (for native `<select>`
    popups, which open as a second top-level X window that `xcap.py`'s
    largest-window heuristic would otherwise miss) and a synthetic scroll
    wheel helper.
- **Step 3 required a temporary source change.** `GraphExplorer.svelte`
  renders at `z-index: 70`, `position: fixed; inset: 0` — it covers the
  entire window, and `SettingsMenu`'s popover is `z-index: 60`. With the
  explorer open, the gear button is not merely visually covered, it is
  genuinely unreachable: a click at its coordinates hits the explorer's own
  content (confirmed empirically — see Step 3 below). To exercise the live
  repaint path at all, `App.svelte`'s capture-phase `handleKeydown` briefly
  gained three debug bindings (`F9`/`F10`/`F11` → `applyTheme('dark'|'light'|
  'blue')`), the same function the real gear button calls. This mirrors the
  method Task 3 used for the same reason (recorded in its own report). The
  three lines plus their import were reverted with `git checkout` immediately
  after this run — `git status` was clean before the doc/commit below, and
  the shipped binary was rebuilt from the reverted tree.
- Every screenshot lives in the job scratch directory only; none are part of
  the repository.

## Results

| # | Case | Result |
|---|---|---|
| 1 | QueryAutocomplete `.qac-label` on an active row (Task 5 could not reach) | **PASS**, all 3 themes |
| 2 | NodeCard `.reminder` child overrides (Task 5 never saw one rendered) | **PASS**, all 3 themes |
| 3 | Live theme switch with the explorer open — no repaint regression | **PASS**, Dark→Light→Blue |
| 4 | Explorer's 16 shared data colours, read on the Light background | **PASS** — no override added |
| 5 | Pre-existing `.explorer-topbar` / legend z-index overlap | **CONFIRMED**, worse in Light, not a regression, not fixed |
| 6 | Toolbar + three columns | **PASS**, Dark/Light/Blue |
| 7 | Note editor + syntax highlighting | **PASS**, Dark/Light |
| 8 | Study — Read, Process, Recall, Stats | **PASS**, Dark/Light/Blue |
| 9 | Graph explorer — 3D scene, DOM overlay, sidebar, legend, minimap | **PASS**, Dark/Light/Blue |
| 10 | Notebook — cell list, Sketch, Compute, Media | **PASS**, Dark/Light |
| 11 | Appearance menu — three selectable themes | **PASS**, Dark/Light/Blue |
| 12 | Native `<select>` popup | **PASS (with a known platform caveat)** |
| 13 | Scrollbar follows the theme | **PASS**, Dark/Light |
| 14 | App.svelte graph-build banner | **PASS**, Dark/Light |
| 15 | SourceTab/MiddleBand "file changed externally" warning banner | **PASS**, Dark/Light |
| 16 | ColumnContextTags — active / cursor-at / sequence chips | **PASS**, Dark/Light |
| 17 | LLMSettings modal (Models → LLM Providers → edit form) | **PASS**, Dark/Light |
| 18 | MediaSearchPanel error box and duration badge | **NOT VERIFIED** |
| 19 | ArrowPalette, EditCard ambiguous-notice (on screen) | **NOT VERIFIED on screen** |

### 1. QueryAutocomplete `.qac-label` on an active row — PASS

Task 5's report states this plainly: the synthetic input tool maps
`backslash` to `underscore` on this box, and the autocomplete list only
populates for a `\`-prefixed query, so the active-row case rested on computed
contrast only. `xbackslash.py` (described above) fixed the input problem, so
this run typed a real `\` into the graph-mode query bar (`Ctrl+Shift+K`),
followed by `a`, and read the resulting `\arrow` / `\arrows` / `\as` dropdown
with `\arrow` as the active (index-0) row in all three themes.

- **Blue** (`t7-06-crop.png`): active row is a dark-navy fill with a bright
  cyan `\arrow` label — clearly legible.
- **Dark** (`t7-12-crop.png`): near-identical dark fill, same cyan label —
  legible.
- **Light** (`t7-15-crop.png`): active row is a pale blue fill with a
  medium-blue `\arrow` label — legible, good contrast against both the fill
  and the surrounding white rows.

No theme showed light-text-on-light or dark-text-on-dark; `--qac-label-text`
reads correctly on `--accent` in all three.

### 2. NodeCard `.reminder` child overrides — PASS

Task 5's report is explicit that this was never rendered: no all-caps
reminder node with arrow properties existed in the scratch collection. The
`:: theme test ::` block added to `keys-b/brains.n4l` (described above)
fixes that. Two different render paths were checked:

- The **NODES panel** row-expand view (`ColumnNodes.svelte`, not
  `NodeCard.svelte` — this was a false start; that panel has no `isReminder`
  logic and renders the plain "current context" highlight, not the frozen
  reminder surface). Screenshots `t7-18`/`t7-20`/`t7-22` are this component,
  included for completeness but not evidence for this case.
- The actual `NodeCard.svelte` instance, reached via **Study → Read** for the
  `neuroscience brains` chapter, scrolled to the last card. This is where
  `.node-card.reminder` really renders.

Reading the real card (cropped tight in each theme):

- **Light** (`t7-26-crop.png`): cream/amber card (`--reminder-card-bg
  #fef3c7`), orange-brown left border, "REMEMBER THIS RULE" centered in bold
  brown-red, `has frequency:` / `is characterized by:` labels in dark
  amber-brown, values in near-black. Clean and legible.
- **Dark** (`t7-27-dark-readtab.png`): the same cream/amber card, unchanged —
  by design (`--reminder-card-*` is a frozen surface, deliberately
  theme-invariant per the code comment). Against Dark's near-black page it
  reads as a clear, intentional highlight, not a mistake — the border gives
  it a clean edge rather than an odd island.

Confirms the "frozen ink" design intent holds up visually, not just on
paper: `.node-card.reminder .arrow-dt`, `.arrow-dd` and `.arrow-list`'s
border-top are all legible in both themes actually opened.

### 3. Live theme switch with the explorer open — PASS

This is the regression `Theme.js`'s own comment calls out: colours used to be
evaluated once at import time, which pinned the palette to whichever theme
was active when the module first loaded, so a later theme switch would leave
the 3D scene stale until a full reload. `Theme.js` now resolves colour at
call time specifically so a live switch repaints.

**The gear button is not reachable while the explorer is open**, confirmed
empirically before working around it: `GraphExplorer.svelte`'s root is
`position: fixed; inset: 0; z-index: 70`, entirely covering the window;
`SettingsMenu`'s popover is `z-index: 60`. A real click at the gear's normal
screen coordinates (`t7-75-try-gear.png`) landed on a graph node underneath
and opened a node-detail panel, not the settings menu — the click never
reached the covered button. This is worth recording precisely because the
plan's own wording ("switch themes from the main window, not from inside
Study") could be misread as implying the explorer leaves the toolbar
reachable; on this build it does not, and the discrepancy from Study (where
the popover is merely visually covered, per the brief) is that here the
button is not even clickable through, since 60 < 70.

To exercise the real repaint path anyway (mirroring Task 3's method for the
identical problem, applied to a different overlay), a temporary `F9`/`F10`/
`F11` → `applyTheme()` debug hook was added to `App.svelte` (see setup
notes), used, and reverted.

Sequence, explorer open throughout, no remount:

1. Opened the explorer on `neuroscience brains` in Light. Full scene with
   four coloured territories (blue "brain waves", olive "memory", orange
   "brain parts etc", purple "references"), minimap, legend, querybar.
   Screenshot `t7-78-explorer-loaded.png`.
2. Pressed `F9` (dark) — **no close, no reload**. Screenshot
   `t7-79-explorer-dark-live.png`: the entire scene, minimap, legend and
   querybar repainted to Dark in place — same nodes, same camera position,
   same query text still in the bar.
3. Pressed `F10` (light) — screenshot `t7-80-explorer-light-live.png`:
   repainted back to Light, still the same mounted scene.
4. Pressed `F11` (blue) — screenshot `t7-81-explorer-blue-live.png`:
   repainted to Blue.

All three transitions were instant and complete — territory fills, edge
lines, minimap swatches, node dots and text all followed. No stale palette,
no partial repaint, no frozen colour anywhere in the scene. The regression
`Theme.js`'s comment warns about has not returned.

### 4. Explorer's 16 shared `--explorer-*` data colours on Light — PASS, nothing added

These are declared once in `:root` and intentionally shared across all three
themes (data-encoding colours, not chrome). Read directly off the Light
screenshots from case 3 (`t7-78`, `t7-82`, cropped in `t7-82-crop.png`):

- Four territory fills in play (blue, olive-green, orange/tan, purple),
  each a pale wash against the app's light-gray page background — all four
  are visually distinct from the page background and from each other.
- Node dots (fully saturated versions of the same four hues) read clearly
  against their own territory and against the plain page background outside
  any diamond.
- Edge lines (teal, orange, seen crossing between territories and over the
  plain background) stay visible the whole way across, including where they
  cross a *different* pale territory than their own colour.
- Text labels drawn over the territories are dark navy/near-black in every
  case (this is chrome, not one of the 16, and was already confirmed
  theme-aware) — never washed out by the territory fill under them, including
  the closest call, the olive "memory" territory under olive-toned "memory"-
  labelled text, which still reads as dark-on-pale rather than same-on-same.

No colour lacked contrast on the Light background. Per the brief, nothing was
changed — no `[data-theme="light"]` override was added for any of the 16.

### 5. Pre-existing z-index overlap (`.explorer-topbar` over legend/stats) — confirmed, not fixed

Reported twice before this task, per the plan's own framing, and explicitly
**not this feature's bug** — recorded here only because the brief asked for a
fresh-eyes read on whether it looks like a regression.

- **Light** (`t7-82-explorer-light-final.png`, cropped tight in
  `t7-83-legend-crop.png`): the legend row ("Contains / Leads to /
  Properties / Similar to / Other / Unknown arrow code") is washed to a pale,
  low-contrast gray by the translucent light querybar/topbar sitting on top
  of it at `z-index: 10` vs the legend's `z-index: 4`. Several words are hard
  to make out at normal reading distance.
- **Dark** (`t7-79-explorer-dark-live.png`, same crop region): the identical
  overlap is present, but far less damaging — the translucent dark topbar
  leaves the light-gray legend text clearly readable.

**To a fresh eye this reads as a real defect, and specifically as a
regression from this feature**, because Light is precisely the theme this
plan shipped and the one where the bug is worst. It is not: it is a
pre-existing z-index ordering bug that Light's lighter, more translucent
chrome merely exposes more harshly than Dark's did. Per the brief, **this was
recorded and not fixed**.

### 6. Toolbar and three columns — PASS

Files/Chapters, Contexts/Sequences and Nodes columns, plus the icon toolbar
row, all legible with populated content in Light (`t7-25-scrolled.png`,
`t7-92-light-scroll.png`) and Dark (`t7-91-dark-main.png`,
`t7-93-dark-scroll.png`); Blue was already covered by Task 6's own
verification (`task6-01-main.png`) and re-confirmed live via the explorer
switch in case 3.

### 7. Note editor + syntax highlighting — PASS

`t7-71-mainwin.png` (Light) and the note view underlying `t7-91` (Dark) both
show live N4L syntax highlighting: chapter titles in bold blue/teal, context
headers (`:: … ::`) in teal, arrow codes `(freq)`/`(state)`/`(note)` in
green, comments (`//…`) in muted gray, string literals distinct from plain
text. No unstyled-black-on-white or invisible-token cases in either theme.

### 8. Study — Read, Process, Recall, Stats — PASS

- **Process**, Light: `t7-56-process.png`, `t7-85-process-light.png` — extract
  header, `Make card`/`Cloze`/`Later`/`Dismiss` buttons, notebook panel with
  EXTRACT/TAGS/NEIGHBORS/SOURCE sections, Sketch cell toolbar, all legible.
  Dark/Blue equivalents already covered by Task 3 (`task3-dark-03-process.png`,
  `task3-light-02-process.png`).
- **Read**: reached via the same surface as case 2 — `t7-25`/`t7-26` (Light)
  show populated `NodeCard` feed cards.
- **Recall**, Light: `t7-95-light-recall.png` — flashcard with embedded image
  and a `Show answer (Space)` button, clean contrast.
- **Stats**, Light: `t7-94-light-stats.png` — three panel groups (Learning
  quality, Workload, Habit) with bar and time-series charts in blue, purple,
  amber and green, all axis labels and gridlines legible. Dark/Blue already
  covered by Task 4 (`task4-dark-02-stats.png`, `task4-blue-01-stats.png`).

### 9. Graph explorer — 3D scene, DOM overlay, sidebar, legend, minimap — PASS

3D scene, minimap and legend covered in case 3/4/5 above, all three themes.
Sidebar (`t7-73-hamburger.png`, Light): Controls panel — Layers, Arrows
(five colour-coded rows: Contains/Leads to/Properties/Similar to/
Unclassified), Display (Layout toggle, Layer spacing slider, Plane opacity
slider), Reset view button — all legible. Dark/Blue sidebar already covered
by Task 2 (`t2fix-dark-sidebar.png`, `t2-blue-sidebar.png`).

### 10. Notebook — cell list, Sketch, Compute, Media — PASS

Reached via Study → Process → the existing notebook for extract `8vlaho1t`.

- Cell-add row (`t7-57-nb-scrolled.png`, Light): the full "+ N4L draft cell /
  + LLM cell / + Graph cell / + Compute cell / + Sketch cell / + Scan / +
  Audio cell / + Media / + Shell" list, all legible dashed-outline rows.
- **Compute** (`t7-58-compute-added.png`, Light): `COMPUTE · JS` header,
  `Run` button, placeholder comment text in the code area — clean contrast.
  (Clicking `Run` did not visibly render an output row in this environment;
  not chased further since the chrome itself — the part theme tokens affect
  — was already legible and unremarkable.)
- **Media** (`t7-62-media-added.png`, Light): `MEDIA · IMAGE` header, Image/
  Audio/Video tabs, provider dropdown, search field, Import/Fetch controls —
  all legible.
- **Sketch**: already covered thoroughly by Task 3
  (`task3-dark-04-expand.png`, `task3-light-03-expand.png`, plus the live
  pen-colour switch sequence in that task's own report).

Dark equivalents for Compute/Media were not separately re-captured (chrome
identical component-for-component to the LLMSettings/MediaSearchPanel Dark
screenshots in cases 17/18, which use the same input/button/label tokens);
this is a coverage judgement call, not a claimed pass on unopened pixels.

### 11. Appearance menu — three selectable themes — PASS

`t7-14-light-main.png` (Light, `Light` selected), `t7-13-settingscheck.png`
(Dark, `Dark` selected), `task6-04-blue.png` (Blue, from Task 6's own run at
the same commit) all show three real radio options with no "Dark and Light
are in progress" placeholder text (confirming Task 6's removal of that note
took effect) and no visual difference in menu chrome between themes beyond
the expected token substitution.

### 12. Native `<select>` popup — PASS, with a known platform caveat

The `Kind` field in the LLMSettings edit form is a real `<select>`. Its
**popup** opens as a second top-level X window (confirmed via `XQueryTree` —
a new window appears and disappears with each click), which `xcap.py`'s
name-substring-plus-largest-area heuristic cannot target since the popup is
small; `xcapid.py` (window-ID capture, written for this run) was used
instead.

- **Light** (`t7-87-selectpopup-a.png`): white popup, black text,
  blue-highlighted `anthropic` row.
- **Dark** (`t7-90-selectpopup-dark.png`): **identical** — white popup, black
  text, same blue highlight.

`style.css` sets `color-scheme: dark` on `:root` and `color-scheme: light`
under `[data-theme="light"]`, with a comment stating this is meant to theme
"the `<select>` box AND its popup menu" for WebKitGTK. The **closed** select
box does follow the theme (background/text colour matched the surrounding
Light/Dark modal in both screenshots) — that part is our own CSS. The
**popup list**, which is native WebKitGTK chrome, rendered light-on-white
regardless of theme in this environment. This is not a regression from this
feature: the 2026-08-12 themes test plan already documented the identical
limitation as accepted platform behaviour (its case F13: "`color-scheme`
does not reach WebKitGTK's `<select>` popup … white popup/dialog over a dark
app; readable"). It was re-confirmed here, unchanged, rather than assumed.

### 13. Scrollbar follows the theme — PASS

Files/Chapters column scrollbar thumb, scrolled to a mid-track position and
cropped tight:

- **Light** (`t7-92-scrollbar-crop.png`): light-gray rounded thumb on the
  panel's light background — correct, matches the earlier-documented Light
  scrollbar colour.
- **Dark** (`t7-93-scrollbar-crop.png`): dark-gray thumb on the dark panel
  background — correct, and distinct from the Light case, meaning
  `color-scheme` **did** reach the scrollbar even though (per case 12) it did
  not reach the `<select>` popup in this same environment. Both facts were
  verified independently rather than assumed from each other.

### 14. App.svelte graph-build status banner — PASS

`task4-dark-06-buildgraph.png` / `task4-light-04-built.png`: "Building
graph…" banner, blue fill with white text, centered top of the middle
column, legible in both themes. These screenshots predate this task (built
from the same code, untouched since Task 2 tokenised this banner and Task 4
did not touch it) but were re-read for this run rather than assumed valid.

### 15. SourceTab/MiddleBand external-modification warning banner — PASS

Triggered by editing `keys-b/brains.n4l` from a second shell while the file
was open in the app, once per theme (see setup notes).

- **Light** (`t7-29-crop.png`): "File changed externally — Reload?" — amber
  fill, brown-black text, amber `Reload` button, dark `Dismiss` button. Good
  contrast.
- **Dark** (`t7-31-crop.png`): visually identical amber banner (the warning
  hue is deliberately theme-invariant, matching `.reminder-badge`'s
  `--warn-banner-bg`) — reads clearly against Dark's near-black surrounding
  chrome.

Both `Reload` clicks (once per theme) were exercised, confirming the banner
also dismisses correctly, not just paints correctly.

### 16. ColumnContextTags — active / cursor-at / sequence chips — PASS

Using `chinese.n4l` (chip-dense) and `Mary.n4l` (the only file with a
`_sequence_`-tagged context pair, needed for the dashed sequence-chip style).

- **Active vs. cursor-at, Dark** (`t7-35-crop.png`): "please" chip
  active (dark-navy fill, light-blue border, light-blue text — clicked);
  "ad hoc idioms"/"phrases" chips cursor-at (medium-blue fill, near-white
  text — caret happened to sit in their context, not clicked) — both states
  distinguishable and legible simultaneously.
- **Same, Light** (`t7-36-crop.png`): active is a light-blue fill with blue
  border/text; cursor-at is a darker blue-gray fill with dark-navy text —
  same two-state distinction holds up on the opposite theme.
- **Sequence chip, Light, un-clicked** (`t7-52-crop.png`): dashed gray
  border, gray text, white fill — the `.chip.sequence` style, distinct from
  a plain solid-border chip.
- **Sequence chip, active** (`t7-53`/`t7-54` crops): dashed **green** border
  and green text on pale green fill for the clicked sequence chip
  (`--chip-sequence-active-*`); the co-tag chip ("poem") simultaneously shows
  dashed border **and** the cursor-at blue fill — both class combinations
  render correctly stacked, at 8× zoom, dashes clearly visible under the
  fill.

### 17. LLMSettings modal — PASS

Reached via Notebook → `⚙ Models` → `Anthropic`.

- **Light** (`t7-70-llm-anthropic.png`): white modal on a dimmed scrim, blue
  field labels (Name/Kind/Base URL/Model/API key/…or env var name), light
  gray-bordered inputs, blue `Save` / gray `Cancel` buttons. All legible.
- **Dark** (`t7-89-dark-llm.png`): dark modal, cyan field labels, dark
  inputs with visible borders, same button pair. Equally legible; no
  unreadable-on-unreadable pairing anywhere in the form.
- The provider picker screen before opening a specific provider (`t7-67`/
  `t7-69`) was also checked in both themes: "Models" title, three tabs (LLM
  Providers / Speech-to-text / Skills), five provider "Add:" pill buttons —
  legible in both.

### 18. MediaSearchPanel error box and duration badge — NOT VERIFIED

The panel chrome itself was reached and reads fine (`t7-62-media-added.png`:
Image/Audio/Video tabs, Wikimedia Commons provider dropdown, search field,
Import/Fetch controls, all legible in Light). The **error box** (`.ms-err`,
shown when a search or fetch fails) and the **duration badge** (shown on an
audio/video result) both require either a live network call or a typed
search query to reach, and typing into that specific search field did not
land any keystrokes in this run despite multiple attempts (single click,
double click, click-then-delay) — a synthetic-input reliability problem
specific to that control, not investigated further given the scope already
covered elsewhere. Both remain verified only by Task 5's computed-contrast
review, not on screen. Stated plainly rather than assumed passing.

### 19. ArrowPalette, EditCard ambiguous-notice — NOT VERIFIED on screen

Neither was specifically opened during this run. Both were tokenised in
Tasks 2/5 respectively and pass the guard test; EditCard's contrast was
additionally re-derived by Task 4's review (`.editcard-ambiguous` on
`#e0b000`-turned-token, ~1.75:1 in Light before the fix, corrected
afterward — see Task 5's ledger entry). Recorded here as coverage this run
did not add, not as a defect.

## Tokens added this task

**None.** Step 4 (explorer data colours on the Light background) concluded
all 16 `--explorer-*` values already read correctly against Light — see
case 4. No `[data-theme="light"]` override was added for any of them, and no
other new token was introduced anywhere in this task.

## What this run does not cover

- MediaSearchPanel's `.ms-err` error box and duration badge (case 18) —
  blocked by a synthetic-input limitation in this environment, not a claimed
  pass.
- ArrowPalette and EditCard's ambiguous-notice, on screen (case 19) — covered
  only by the guard test and Task 4/5's own contrast review, not reopened
  here.
- Compute cell's actual execution **output** rendering (only the chrome
  before running was confirmed) — `Run` produced no visible output row in
  this environment; not investigated further since the themed chrome around
  it was already legible.
- The pre-existing z-index overlap (case 5) was recorded, not fixed, per the
  brief's explicit instruction not to fix it in this task.

## Cleanup

- The app was closed (window closed via its own `×` control; no lingering
  process).
- The temporary `F9`/`F10`/`F11` debug hook added to `App.svelte` for case 3
  was reverted with `git checkout -- frontend/src/App.svelte`; `git status`
  was clean before this document and the final binary were built.
- `make build`, `go build ./...`, `node --test src/lib/*.test.js` (260/260
  passing, including the 3-test guard suite) were all re-run against the
  reverted tree before writing this document, so nothing shipped reflects the
  debug hook.
- The two additions to the scratch `keys-b` collection (the `theme test`
  block in `brains.n4l`, and the two external-edit marker lines) remain in
  the scratch copy only; the real collection and
  `~/.config/texere/config.json` were never opened or modified by this run.
- All screenshots, capture/input scripts and the running app's stdout/stderr
  logs live in the job scratch directory and are not part of the repository.
