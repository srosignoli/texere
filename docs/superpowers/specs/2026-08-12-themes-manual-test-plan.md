# Themes (Blue / Dark / Light) — manual test plan (executed)

**Feature branch:** `046-themes`
**Build under test:** `make build && ./build/bin/texere`, built at commit `1009f7f`
**Comparison build:** commit `fe45713` — the last commit before the Task-3
component migration began — built in a throwaway `git worktree` outside the
repository and removed afterwards. `HEAD` on the working checkout was never
moved.
**Run date:** 2026-08-16 — every case below was **executed**, not described.
Each carries the result actually observed and the screenshot it was read from.

**This document is the only end-to-end verification this feature gets.** The
seven implementation tasks were each verified in isolation, mostly by static
reading of the diff plus a pixel-diff of a handful of surfaces. A pixel-diff
cannot see a dialog nobody opened, which is why this plan enumerates surfaces
rather than saying "check the app". What *is* automated one layer down is
`theme.test.js`'s coverage of `THEMES` / `isValidTheme` / `normalizeTheme` and
Go's `validThemes`; everything below is what that cannot reach — whether the
pixels on each surface are right in each of the three themes.

## How this run was set up

- The collection under test is a **copy** of the user's real collection, made
  with `cp -r` into the job scratch directory
  (`~/.claude/jobs/ad91db08/tmp/collection-verify`). The real collection at
  `…/SStorytime-main/examples/test` and its `learning.db` were never opened by
  the app during this run. `~/.config/texere/config.json` was pointed at the
  copy for the duration and **restored afterwards**, verified byte-identical by
  `sha256` (`033f46f0…`): no `theme` key, `collectionDir` back at the real
  path, both LLM profiles intact. The `texere.theme` row was also deleted from
  `~/.local/share/texere/localstorage/wails_wails_0.localstorage` so the
  browser mirror matches the "no theme key" starting state.
- Inside the copy only: `aaa-theme-check.n4l` was created as a deterministic
  fixture (a chapter with two all-caps reminder lines, so the `⚑` badge and the
  reminder panel exist at all — the real collection has no reminder anywhere —
  and one node carrying `(means)` so the nodes column's arrow tree can be
  expanded). It sorts first, so the chapter under test is always the top row.
  `learning.db` in the copy had its absolute paths rewritten to the copy, and
  one notebook was seeded with **one cell of each of the nine cell types**
  (`n4l`, `llm`, `graph`, `compute`, `sketch`, `image`, `media`, `shell`,
  `audio`), assembled from cells that already existed in the collection.
- The theme was set **in `config.json` before launch** for the per-theme
  sweeps, so each sweep also exercises the startup path (Go authoritative →
  `GetTheme()` → `activate`). Live switching through the `⚙` popover is tested
  separately (TC-20).
- The app was driven with synthetic X11 input (XTEST) on `DISPLAY=:0` and each
  state was captured by grabbing the app window with `XGetImage`. Root-window
  capture on this WSLg display returns black, so every screenshot is a grab of
  the window itself. Every screenshot referenced below was read back as an
  image; crops were enlarged 3–5× to read chips and small text, and colours
  were sampled per pixel and turned into WCAG contrast ratios rather than
  eyeballed.

### What "the arrow palette" turned out to be

`+link` calls `SourceTab.openArrowPicker()`, which inserts `(` and calls
CodeMirror's `startCompletion` — the palette **is** the CM6 autocomplete
tooltip, styled by `n4l-editor.js`'s `.cm-tooltip-autocomplete` rules.
`components/ArrowPalette.svelte` exists but is imported by nothing.

## Result summary

**23 cases: 12 PASS, 9 FAIL, 2 NOT ESTABLISHED.** Fourteen findings are
recorded: **twelve new theme defects** (F1–F12), one platform limitation (F13)
and one incidental non-theme observation (F14) — see § Findings. Screenshot
paths below are relative to `~/.claude/jobs/ad91db08/tmp/`.

| # | Case | Result |
|---|---|---|
| TC-1 | Workbench — three columns, both tabs, in all three themes | **FAIL** (F1, F2, F3) |
| TC-2 | Nodes column — expanded arrow tree and hover actions | **FAIL** (F1, F2, F3) |
| TC-3 | Read tab — node cards and reminder cards | **FAIL** (known K5, K-reminder-card) |
| TC-4 | Reminder badge and reminder panel | **PASS** (badge: known K-badge) |
| TC-5 | OmniBar with results open | **FAIL** (known K5 mode badge) |
| TC-6 | Log panel | **FAIL** (F4, F5) |
| TC-7 | Arrow picker (CM6 autocomplete) | **NOT ESTABLISHED** |
| TC-8 | Settings `⚙` popover itself | **PASS** |
| TC-9 | Graph explorer — canvas, planes, labels | **PASS** |
| TC-10 | Graph explorer — overlay chrome (ask box, query bar, legend, sidebar, stats, minimap) | **FAIL** (F8, known K3) |
| TC-11 | Study — Next / Read / Process / Recall / Stats | **PASS** for Next/Read/Recall; Stats **FAIL** (known K2 + F12) |
| TC-12 | Sources sidebar (Study → Read) | **FAIL** (F7) |
| TC-13 | Notebook — all nine cell types | **FAIL** (F9) |
| TC-14 | Models dialog (`LLMSettings`), all three tabs + edit form | **PASS** |
| TC-15 | Context/sequence chips | **FAIL** (F6) |
| TC-16 | CodeMirror editor pane | **FAIL** (known K1 — confirmed live) |
| TC-17 | Native `<select>` popup renders dark-on-light in **Light** | **PASS** |
| TC-18 | Scrollbars follow the theme | **PASS** |
| TC-19 | App opens in the theme stored in `config.json` | **PASS** |
| TC-20 | Live switching through the `⚙` popover, all six transitions | **PASS** |
| TC-21 | Unknown theme in `config.json` falls back to Blue without erroring | **PASS** |
| TC-22 | **Blue unchanged** vs the pre-migration build, surface by surface | **FAIL** (F11 — one regression) |
| TC-23 | No flash of Blue on launch | **NOT ESTABLISHED** (platform) |

---

## TC-1 Workbench — three columns, both tabs

**Result: FAIL.** In all three themes the chapters, contexts and nodes columns,
the tab strip, the action bar and its buttons render correctly and legibly:
`blue-02-workbench.png`, `dark-02-workbench.png`, `light-02-workbench.png`, all
read. Column background resolves to `#1b2636` / `#16171a` / `#f6f7f9` as the
token layer says it should.

The failure is inside the nodes column, in the rules TC-2 covers, plus the
editor pane below it (TC-16). Nothing else on this surface is wrong.

## TC-2 Nodes column — expanded arrow tree, hover actions

**Result: FAIL — three findings.** With a node's arrow tree expanded and the
row hovered (`light-03-nodes-expanded.png`, crop `light-03-crop.png` at 3×;
`02-crop.png` for the Blue reference):

- **F1** `.arrow-target { color: #86efac }` (`ColumnNodes.svelte:488`) — the
  arrow *target*, i.e. the actual content of an expanded row — measures
  **1.31:1** against Light's `--surface` `#f6f7f9`. Sampled from the rendered
  glyphs, not from the stylesheet.
- **F2** `.arrow-code { color: #5a9abd }` (`:475`) — **2.88:1** on Light.
- **F3** `.ha-btn { background: #1a2d45; color: #7ab0d4 }` (`:449-452`) — the
  `+ Card` / comment buttons render as a **dark navy chip on Light's white
  column** (visible in `light-03-crop.png`). Internally 5.97:1, so it is not
  illegible; it is a dark-theme control sitting in a light theme, and in Dark
  it is a blue-family chip in a theme whose stated rule is "no blue cast".

## TC-3 Read tab — node cards, reminder cards

**Result: FAIL — both already known.** `light-06-readtab.png`,
`dark-06-readtab.png` read.

- `NodeCard.svelte:113` `.arrow-dd { color: #86efac }` measured **1.40:1**
  against the card's white background in Light (histogram of the rendered
  glyphs at `light-06-readtab.png` 290,646). Known finding.
- `NodeCard.svelte:59-77`'s fixed light-amber reminder card
  (`#fef3c7` / `#1c1100` / `#92400e`) is a **glaring bright block** in Dark's
  near-black Read tab (`dark-06-readtab.png`). Known finding.

Everything else on the Read tab — card chrome, arrow labels, layout — is
correct in all three themes.

## TC-4 Reminder badge and reminder panel

**Result: PASS** for the panel; the badge is a known finding.
`light-05-reminderpanel.png`, `blue-05-reminderpanel.png` read. The panel is
fully tokenised: white surface, `--warn` `#b45309` reminder text on `#ffffff`
(≈4.9:1), themed Resolve buttons. The `⚑ 2` badge in the action bar is
`MiddleBand.svelte`'s `.reminder-badge` `background:#78350f; color:#fde68a`, a
dark-brown chip on Light's light bar — already recorded by Task 6.

**Incidental, not a theme defect:** the reminder panel does **not** close on
Escape. It stayed open across the rest of the first sweep and occluded the
right-hand third of the window, which is what forced this run's second and
third sweep passes.

## TC-5 OmniBar with results open

**Result: FAIL — known.** `light-08-omnibar.png` read: the shell, scrim, result
rows, section headers and right-hand context labels are all correct in Light.
The `FUZZY` mode badge is `#1e3a50` background with `#5a8aaa` text
(`OmniBar.svelte:1376`), sampled directly from the badge — a dark navy chip
inside Light's white omnibar. Known finding.

## TC-6 Log panel

**Result: FAIL — two new findings.** `light-09-logpanel.png`, crop
`light-09-crop.png` at 5×; `dark-09c.png` at 5×.

- **F4** `.count-warn { color: #d9b06a }` (`LogPanel.svelte:164`) — the `!1`
  warning counter measures **2.03:1** on Light's `#ffffff` log bar and is, in
  the enlarged crop, barely visible. Its siblings: `.count-err` `#d96a6a`
  **3.38:1**, `.count-info` and `.copy-badge` `#6a9aaa` **3.08:1**.
- **F5** `.filter.active { background:#1e3242; color:#d8c590;
  border-color:#c9a96e }` (`:188-190`) — the active filter renders as a
  **near-black pill among light outline pills** in Light, and as a distinctly
  **navy** pill in Dark (`dark-09c.png`). Sampled `#1e3242` on a `#ffffff` bar.

The log rows themselves, the severity bars and the panel body are correct in
all three themes.

## TC-7 Arrow picker (CM6 autocomplete)

**Result: NOT ESTABLISHED.** `+link` inserted `(` into the editor and typing
`me` plus `Ctrl+Space` did not produce a completion tooltip on this collection
(`blue-51c.png`, `blue-52c.png`, `blue-53c.png` — all read, no popup in any).
The palette could not be made to render, so it was not observed in any theme.
Its styling is nonetheless unthemed by inspection: `n4l-editor.js`'s
`.cm-tooltip-autocomplete` / `.cm-completionLabel` / `.cm-completionDetail` /
`.cm-completionSectionHeader` are hardcoded Blue literals — part of known
finding K1.

## TC-8 Settings `⚙` popover

**Result: PASS.** Read in Blue (`recover-02c.png` at 3×) and Light
(`live-openc.png` at 3×). Fully tokenised in both: correct surface, border,
legend, row text, and the checked radio tracks the active theme. Readable in
both. The only blemish is platform-level: the **unchecked** native radios draw
as white circles in Blue and Dark, because WebKitGTK renders the radio from the
GTK theme and only honours `accent-color` for the checked dot (F13).

## TC-9 Graph explorer — canvas, planes, labels

**Result: PASS.** `light-11-explorer.png`, `dark-50b-explorer.png`,
`blue-65-explorer.png` read. The canvas backdrop, the isometric plane grid and
tint, the floor labels and their halos, and the node/edge semantic hues all
follow the theme as designed: Light's canvas measures `#e5e8ec`
(`--explorer-canvas-bg`), the "Untagged" floor label is dark-on-light, and the
territory/edge hues are identical across themes as the spec requires.

## TC-10 Graph explorer — overlay chrome

**Result: FAIL — the largest new finding.** With the Controls sidebar open:
`light-27-explorer-sidebar.png` and `dark-50b-explorer.png`, both read.

**F8** — the explorer's entire DOM overlay layer is hardcoded dark and does not
follow the theme. In Light it renders as a set of **black slabs floating over a
near-white canvas**, sampled directly from the screenshot:

| Element | Component | Measured in Light |
|---|---|---|
| Ask box strip | `GraphExplorer.svelte` `.explorer-ask` `rgba(0,0,0,0.6)` | `#121416` |
| Query bar | `.explorer-querybar` + `input` `rgba(12,14,20,0.9)`, `color:#e8eaf0` | `#14161b` |
| Legend bar | `ExplorerLegend.svelte:71-84` `rgba(14,20,24,0.85)`, `#e8eaf0` | `#07090b` |
| Controls sidebar | `ExplorerSidebar.svelte` (26 literals; `rgba(20,24,32,0.94)`, `#334455`, `#e8eaf0`, `#aab4c2`, `#1f2630`, `#2a3340`, `#1a2028`, `#232a35`) | dark panel |
| Status/System readout | `ExplorerStats.svelte:123-172` `rgba(14,20,24,0.85)`, `#e8eaf0`, `#45607a` | dark panel |
| Node / edge / layer tooltips, freeze pill, empty card | `GraphExplorer.svelte` `rgba(20,24,32,0.92–0.94)`, `#334455`, `#a8c4ff`, `#aab4c2`, `#d8dde8` | dark panels |
| Edit-card scrim | `explorer/EditCard.svelte:146` `rgba(10,16,26,0.72)` | dark scrim |
| MiniMap | `MiniMap.svelte` (known K3) | `rgba(14,20,24,.92)` |

The already-tracked item covers only the MiniMap. The scope is the whole
overlay layer, and the `--explorer-scrim*` / `--text-on-scrim` tokens that
exist for exactly this purpose are not consumed by any of it. In Dark the same
panels look approximately right; the blue-family borders (`#334455`) and text
(`#a8c4ff`) remain a blue cast in a theme that forbids one.

A second-order effect is visible in `light-27-explorer-sidebar.png`: the ask
box's native `<select>` correctly renders **light** (`#f4f4f4`, following
`color-scheme`) while sitting inside a **black** slab.

## TC-11 Study — Next, Read, Process, Recall, Stats

**Result: PASS for four of five focuses; Stats FAIL.**

Read back: `blue-13-study-next.png` / `06-study.png` (Next, "Nothing due"),
`light-73-sources.png` (Read), `light-33-notebook-1.png`,
`dark-33-notebook-1.png` (Process), `light-70-recall-front.png`,
`light-71-recall-back.png` (Recall front/back, including the image card, the
KaTeX formula and the four grade buttons), `light-72-stats.png`,
`dark-72-stats.png`, `blue-24-stats.png` (Stats). The Study tab bar, the
Process toolbar, the extract/context card and the Recall surface are all
correctly themed in all three.

**Stats fails**, and beyond the already-known series/`.sp-value` problem:

- Known K2, now measured: the forgetting-index value `#e0b000` is **1.71:1**
  against Light's `--surface-panel` `#e9ecf0`.
- **F12** `stats-charts.js:8-9` `TICK = '#8899aa'` and `GRID = '#2a3a4a'` are
  Chart.js axis and gridline colours, not series colours, and are outside the
  known item's description. In **Dark** they render as visibly **blue**
  gridlines (`dark-72c.png`, enlarged 3× — the blue vertical rules are
  unmistakable) in the theme whose rule is "no blue cast at all". In Light the
  ticks measure **2.37:1**.

## TC-12 Sources sidebar (Study → Read)

**Result: FAIL.** `light-73-sources.png`, crop `light-73c.png` at 3×.

**F7** — `SourcesSidebar.svelte:188-190` `.sb-tag.due { background:#4a3a1a }`,
`.sb-tag.done { background:#1a3a2a }`, `.sb-tag.llm { background:#2c2340 }` are
**dark chips on Light's light source list**, clearly visible in the crop. The
file's own comment acknowledges these are literals kept for pairing; the
consequence in Light is that the two status chips are the darkest things on the
panel. The progress percentages beside them (`#4b9a73`, ≈3.6:1) come from a
token and are fine.

## TC-13 Notebook — all nine cell types

**Result: FAIL — one new finding; everything else correct.** Read:
`light-33-notebook-1.png` (n4l + llm), `probe-scroll2c.png` (graph + compute +
sketch), `light-45c.png` (scan/image + media), `light-46c.png` (shell + audio +
the add-cell rail), and `dark-33-notebook-1.png` for Dark. Cell chrome,
headers, fold controls, Push-to-source, the shell prompt and output area, the
audio error notice and the media attribution are all correctly themed in all
three themes.

**F9** — `SketchCell.svelte:15` `const COLORS = ['#e8eaf0', …]` and `let color =
COLORS[0]`. The **default pen colour is near-white**, and Light's sketch canvas
measures `#e4e7ec` (`--surface-sunken`): **1.03:1**. A user who opens a sketch
cell in Light and draws with the default pen draws **invisible ink**. The first
swatch in the palette row is likewise invisible against the toolbar. This is a
functional defect, not a cosmetic one; it is the only finding in this run that
loses user work rather than merely looking wrong.

## TC-14 Models dialog (`LLMSettings`)

**Result: PASS.** `light-47-llmsettings.png`, `light-48c.png` (edit form at 2×),
`dark-48c.png` read. Dialog surface, tab strip, provider rows, the Add row, and
the edit form's labels, text inputs and native `<select>` are all correctly
themed in Light and Dark, with the select control itself following
`color-scheme` (dark control with a white arrow in Dark, light control with a
grey arrow in Light).

## TC-15 Context / sequence chips

**Result: FAIL.** `light-80c.png` (inactive) and `light-81c.png` (active), both
3×.

**F6** — `ColumnContextTags.svelte:177-179` `.chip.sequence.active {
background:#1a3020; border-color:#4ade80; color:#86efac }` renders as a
**near-black green pill** in Light's contexts column the moment the chip is
selected; the inactive chip beside it is a correct light outline pill.
`.chip.cursor-at:not(.active)` (`:187-189`, `#1e3550` / `#b0c4d8`) has the same
shape. Internally legible (10.05:1 and 6.98:1), wrong theme.

## TC-16 CodeMirror editor pane

**Result: FAIL — known K1, confirmed live and measured.** The editor pane
measures `#1b2636` in **Dark** (`dark-02-workbench.png` at 550,700) and in
**Light** (`light-02-workbench.png`), i.e. it stays a navy slab in both. The
same is true of the notebook's N4L-draft mini editor (`#1b2636` at
`dark-33-notebook-1.png` 900,530) and of the Process focus's source pane. This
is the largest single surface in the app and it never changes.

## TC-17 Native `<select>` popup in Light

**Result: PASS.** The `Kind` select in the Models edit form was opened in Light
and the popup — which WebKitGTK renders as its own X window — was captured
separately (`light-49-popupwin.png`, enlarged as `light-49pc.png`). It renders
**dark text on white with a blue selection band: readable**. `color-scheme:
light` is doing its job.

**F13, recorded as a platform limitation, not a defect:** the *same* popup was
captured in **Dark** (`dark-49pc.png`) and is byte-identically light. The
popup menu is drawn by GTK and does not follow `color-scheme` at all, so in
Dark and Blue a white popup opens over a dark app. The same is true of
`window.confirm()` — the "Unsaved changes — switch anyway?" dialog is a white
GTK dialog in Dark (`dark-50-explorer.png`, read). Both are readable; neither
is reachable from CSS.

## TC-18 Scrollbars

**Result: PASS.** The chapters column's scrollbar track and thumb were sampled
across the same 12×290 region in all three sweeps: Blue `#1b2636` / `#1e3a5a` /
`#455d77`, Dark `#16171a` / `#2a2d33` / `#525459`, Light `#f6f7f9` / `#dbeafe` /
`#b4c0d1`. The scrollbars follow the theme, which is the other half of what
`color-scheme` controls.

## TC-19 App opens in the theme stored in `config.json`

**Result: PASS.** With `theme` written into `config.json` and the app launched
fresh: Dark opens at `#16171a` (`24-darkstart.png`), Light at `#f6f7f9`
(`25-lightstart.png`), Blue at `#1b2636` (`03-restart.png`). Each was then
driven through a full surface sweep in that theme without ever touching the
picker, so every screenshot in TC-1…TC-16 is also evidence for this case.

## TC-20 Live switching through the `⚙` popover

**Result: PASS.** From Light: → Blue `#1b2636` (`live-a.png`), → Dark `#16171a`
(`live-b.png`), → Light `#f6f7f9` (`live-c.png`), each read and each sampled.
`config.json`'s `theme` key followed every selection (`blue`, then `dark`, then
`light`), and the checked radio followed the active theme.

**Known limitation confirmed (K6):** with the graph explorer open, the `⚙` is
unreachable — the explorer is `fixed; inset:0; z-index:70` over the action bar,
and a click at the picker's coordinates lands on the explorer canvas
(`explorer-picker-blocked.png`, crop `epb.png` — plain isometric grid where the
popover should be).

## TC-21 Unknown theme in `config.json`

**Result: PASS.** `config.json` was hand-edited to `"theme": "solarized"` and
the app relaunched. It **opened in Blue** (`recover-01.png`, sampled
`#1b2636`), did not error, and the `⚙` popover showed **Blue** checked
(`recover-02c.png`) — `normalizeTheme` is the single default, as designed. The
invalid string is left in `config.json` untouched until the user picks a theme,
which follows from Task 6's deliberate choice to call `activate` (not
`applyTheme`) at startup so a launch never writes config.

## TC-22 Blue unchanged vs the pre-migration build

**Result: FAIL — one regression, everything else identical.** The `fe45713`
build and the `1009f7f` build were driven through the *same* scripted sequence
from a freshly launched instance, and the resulting frames diffed pixel by
pixel (tolerance: per-pixel channel sum > 6).

| Surface | Differing pixels | Where |
|---|---|---|
| Workbench, chapter loaded | 2044 / 951500 (0.21%) | `(960,406)-(1045,432)` only |
| Nodes column, arrow tree expanded | 2044 (0.21%) | same box |
| Read tab | 2044 (0.21%) | same box |
| OmniBar with results | **0** (0.00%) | — |
| Log panel | 2044 (0.21%) | same box |
| Graph explorer | 20123 (2.12%) | the plane grid lines |
| Graph explorer + sidebar | 16579 (1.74%) | the plane grid lines |

The `(960,406)-(1045,432)` box is the `⚙` button and the reminder badge — the
Task-6 addition, i.e. expected. Outside it, four of the six surfaces are
**pixel-identical**, and the OmniBar is identical including the badge box.

**F11 — the one real Blue regression.** The explorer's isometric plane grid
changed shade. `LayerPlanes.js` `planeColorsFor()` now takes the *Untagged*
floor's grid colour from `--explorer-plane-grid` (`#6b7c95` in Blue), where the
pre-migration build computed `brighten(COLOR_PLANE_TINT, 0.55)` =
`brighten(#1f2a3a, 0.55)` = **`#9a9fa6`**. Measured on screen at the same pixel:
`#222429` before, `#1b1f26` now. The change is deliberate and its reason is
sound — the current comment says `brighten()` on an already-pale tint washes out
to near-white in Light and the floor loses its grid — but Blue was not held
constant while making it. Holding Blue identical would have meant giving
Blue's `--explorer-plane-grid` the value `#9a9fa6`. Only the Untagged floor is
affected; user-tagged contexts still use `brighten(territory)` in both builds.

(Two now-dead tokens, `--explorer-extended-plane-grid` and
`--explorer-extended-plane-tint`, were also removed in the same range. They
were imported by `LayerPlanes.js` but never used, so their removal has no
pixel consequence — verified by grep on the `fe45713` tree.)

## TC-23 No flash of Blue on launch

**Result: NOT ESTABLISHED, deliberately.** Task 6 built a burst sampler
(~180 frames/s, a sample every ~5–6 ms against a ≥16 ms compositor frame),
found zero wrong-theme frames in any theme — and then ran a negative control
with the `localStorage` mirror deleted, which should have *produced* a flash,
and still saw none. On this WSLg / WebKitGTK setup the webview commits no frame
at all until the Svelte app has mounted, by which time `GetTheme()` has already
resolved. The window the pre-paint bootstrap exists to close is empty here, so
a green result on this platform is not evidence that the bootstrap works. This
run did not repeat the experiment and does not claim the check as coverage.
What it *does* claim, and did verify, is TC-19: the app opens in the stored
theme.

---

## Findings

Twelve new theme defects (F1–F12), one platform limitation (F13), one
incidental non-theme observation (F14), plus confirmation of six
already-recorded items. **None was fixed** — a scattered fix here would muddy
the record of what this pass established.

### New

| # | File : line | Token / literal | Theme | Measured |
|---|---|---|---|---|
| F1 | `ColumnNodes.svelte:488` | `.arrow-target { color:#86efac }` | Light | **1.31:1** on `--surface #f6f7f9` |
| F2 | `ColumnNodes.svelte:475` | `.arrow-code { color:#5a9abd }` | Light | **2.88:1** |
| F3 | `ColumnNodes.svelte:449-452` | `.ha-btn { background:#1a2d45; color:#7ab0d4 }` | Light, Dark | dark navy chip on white column; blue-family in Dark |
| F4 | `LogPanel.svelte:163-165, 196` | `.count-warn #d9b06a`, `.count-err #d96a6a`, `.count-info`/`.copy-badge #6a9aaa` | Light | **2.03:1**, 3.38:1, 3.08:1 on `#ffffff` |
| F5 | `LogPanel.svelte:188-190` | `.filter.active { background:#1e3242; color:#d8c590; border-color:#c9a96e }` | Light, Dark | near-black pill among light pills; navy in Dark |
| F6 | `ColumnContextTags.svelte:177-179, 187-189` | `.chip.sequence.active #1a3020/#4ade80/#86efac`; `.chip.cursor-at #1e3550/#b0c4d8` | Light | dark pill in a light column (10.05:1 / 6.98:1 internal) |
| F7 | `SourcesSidebar.svelte:188-190` | `.sb-tag.due #4a3a1a`, `.done #1a3a2a`, `.llm #2c2340` | Light | dark chips on a light list |
| F8 | `GraphExplorer.svelte` (~36 literals), `ExplorerSidebar.svelte` (26), `ExplorerLegend.svelte:71-84`, `ExplorerStats.svelte:123-172`, `explorer/EditCard.svelte:146` | `rgba(0,0,0,.6)`, `rgba(20,24,32,.92–.94)`, `rgba(12,14,20,.9)`, `rgba(14,20,24,.85)`, `#334455`, `#e8eaf0`, `#aab4c2`, `#a8c4ff`, `#1f2630` … | Light, Dark | ask box `#121416`, query bar `#14161b`, legend `#07090b` over `--explorer-canvas-bg #e5e8ec` |
| F9 | `SketchCell.svelte:15` | `COLORS[0] = '#e8eaf0'` — the **default pen** | Light | **1.03:1** on `--surface-sunken #e4e7ec` — invisible ink |
| F10 | `App.svelte:486-498` | `.graph-banner--building #2a4a6a/#7ecfff`, `--done #1a4a2a/#7ecfa0`, `--error #4a1a1a/#ff8080` | Light | fixed dark banners — **static only, not observed live** |
| F11 | `explorer/LayerPlanes.js` `planeColorsFor()` + `style.css:237` | Untagged plane grid: was `brighten(#1f2a3a,.55)` = `#9a9fa6`, now `--explorer-plane-grid #6b7c95` | **Blue** | on-screen `#222429` → `#1b1f26`; the only Blue pixel regression |
| F12 | `lib/stats-charts.js:8-9` | `TICK = '#8899aa'`, `GRID = '#2a3a4a'` | Dark, Light | visible blue gridlines in Dark; ticks **2.37:1** in Light |
| F13 | — (platform) | `color-scheme` does not reach WebKitGTK's `<select>` popup, `window.confirm()`, or unchecked native radios | Dark, Blue | white popup/dialog over a dark app; readable |
| F14 | `ReminderPanel` / `ArrowPalette.svelte` | not a theme defect: panel ignores Escape; `ArrowPalette.svelte` is imported nowhere | — | — |

### Already known, confirmed here

| Ref | What | Confirmation |
|---|---|---|
| K1 | CodeMirror editor entirely unthemed (`n4l-editor.js`, `n4l-highlight.js`) | pane measures `#1b2636` in Dark **and** Light (TC-16) |
| K2 | `StatsPanel` series and `.sp-value` illegible on Light | forgetting index **1.71:1** (TC-11) |
| K3 | `MiniMap.svelte` unthemed | dark panel over Light's canvas (TC-10) — now the smallest part of F8 |
| K4 | `style.css:588` `body { color: white }` | **not observed to bite** on any surface opened; remains latent |
| K5 | `NodeCard.svelte:113 #86efac`; `OmniBar.svelte:1376` mode badge | **1.40:1** (TC-3); `#1e3a50`/`#5a8aaa` (TC-5) |
| K6 | Theme cannot be changed with the explorer open | click lands on the canvas (TC-20) |
| — | `MiddleBand` `.reminder-badge #78350f/#fde68a`; `NodeCard` reminder card `#fef3c7` | dark-brown chip in Light (TC-4); bright block in Dark (TC-3) |

## What this run could not establish

- **The pre-paint bootstrap** (TC-23) — not falsifiable on WSLg/WebKitGTK.
- **The arrow picker** (TC-7) — the CM6 completion tooltip could not be made to
  render on this collection, in any theme.
- **`App.svelte`'s graph banners** (F10) — the build-graph banner appears for
  well under a second and was never captured in Light; the finding is from the
  stylesheet only.
- **The `Speech-to-text` and `Skills` tabs of the Models dialog** were opened
  only in Light's default (LLM Providers) state; their own controls were not
  individually enumerated.
- **Whether K4 (`body { color: white }`) ever bites** — no surface opened in
  this run inherited it. Absence of evidence.
