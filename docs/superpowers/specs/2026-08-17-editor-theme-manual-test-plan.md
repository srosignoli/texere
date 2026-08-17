# Theming the N4L editor — manual test plan (executed)

**Design:** `2026-08-17-editor-theme-design.md`
**Build:** `make build && ./build/bin/texere` (built at commit `d7dc1c6`)
**Comparison build:** commit `24a0fea` (the commit before Task 1), built in a
throwaway `git worktree` at `/tmp/texere-pre-theme` and removed afterwards
(`git worktree remove`). `HEAD` on the working checkout was never moved.
**Run date:** 2026-08-17 — every case below was **executed**, not described.
Each carries the result actually observed and the measurement it was read from.

**This document, plus the pixel samples in it, is the only verification the
rendered result gets.** What is automated is one layer down: `n4l-syntax.test.js`
asserts that `n4lHighlightStyle` and all three `EditorView.theme()` blocks emit
only `var(--role)` and that every role exists in all three theme blocks. None of
that can tell you whether a rule ever matches an element, whether a role paints
at all, or what colour actually lands on the screen. That is what this run is
for — and it found four roles/rules that pass the automated gate and never
paint, which is precisely the failure mode the gate cannot see.

## How this run was set up

- **Nothing was changed in the product.** The only repository change is this
  document. All findings below are recorded, not fixed.
- The collection under test is a **copy** of the user's real collection
  (`cp -r` into `~/.claude/jobs/ad91db08/tmp/collection-editor`, then a second
  copy `collection-ed2` made with `tar --exclude` to drop `learning.db`,
  `sst_data` and `bleve` — see **F7**, which is why the second copy was needed).
  `~/.config/texere/config.json` was pointed at the copy for the duration and
  **restored byte-identically afterwards** (`sha256` before and after both
  `033f46f0686fb9aa8af911f0291a800190dbe1be4a5c9fe5937cbd3e2c17e14c`).
- **Dark and Light are gated out of the picker** (`frontend/src/lib/themes.js`
  marks them `available: false`), so they cannot be selected through the UI. The
  theme was set by writing `config.json`'s `theme` field between launches, which
  the store honours on startup. Confirmed working: the same binary, relaunched
  with only that one field changed, renders three different palettes.
- The app was driven with synthetic X11 input (XTEST) on `DISPLAY=:0` and each
  state captured by grabbing the app window with `XGetImage`. Root-window
  capture on this WSLg display returns black, so every screenshot is a grab of
  the window itself. The X keymap on this display carries neither `numbersign`
  nor `at` on any keycode at any shift level, so the driver binds them to a
  scratch keycode via `XChangeKeyboardMapping` before pressing — without that,
  `#` typed as `3` and `@` as `0`, which silently removed four of the nine roles
  from the typed test text. That was caught by reading the screenshots back.
- Screenshots were read back as images **and sampled**. For each role the
  sampler takes the modal colour of a text box (the background) and then the
  pixel in that box farthest from it in RGB — the glyph core. For anti-aliased
  strokes that never reach full coverage (the wavy underline, the gutter dot,
  the section header) the colour was recovered by solving
  `measured = α·C + (1−α)·bg` and checking α is consistent across all three
  channels.

### The test note

`theme-check.n4l` **in the copy only**, chapter **theme check**:

```
- theme check

:: editor theming, roles ::

# a plain comment line about the chapter below
# @label themecheck
# @source the editor theming spec, 2026-08-17
# @card
# @id thm00001
the heading above is a chapter (means) the chapter heading role
   (note) a second arrow with its own target
= (ditto) a ditto continuation line
REMEMBER TO CHECK ALL TEN ROLES
another node (from) "the source sentence this came from"
a node with a bogus arrow (zzqq) an unregistered target
```

Which tokens this actually produces was **measured before the run**, not
assumed: a throwaway Node script ran `n4lLanguage` over the file and walked
`highlightTree` with `n4lHighlightStyle`, printing every emitted class and the
text it covered. That is how **F1** (`--n4l-syntax-arrow-target` never fires)
was found, and how a first draft was corrected — an indented `` :: … :: `` line
produces no `keyword` token at all, because the tokenizer only matches `::` at
column 0.

Nine of the ten roles fire. The tenth, `arrow-target`, cannot (F1).

The same nine roles were put into the other three editors: the ReadingView notes
pane and the ProcessEditor share one `.n4l` in the copy
(`reading/spaced-repetition-2.n4l`, imported and edited through the UI during
the run), and the N4L draft cell was typed fresh in each theme.

## Result summary

**26 of 33 cases pass. 7 fail — 7 findings, none of them fixed.** Screenshot
paths below are relative to `~/.claude/jobs/ad91db08/tmp/et/`.

| # | Case | Result |
|---|---|---|
| TC-1 | **SourceTab** (Note tab), Blue — nine roles + chrome | **PASS** |
| TC-2 | **SourceTab**, Dark | **PASS** |
| TC-3 | **SourceTab**, Light | **PASS** |
| TC-4 | **ReadingView** notes pane, Blue | **PASS** |
| TC-5 | **ReadingView**, Dark | **PASS** |
| TC-6 | **ReadingView**, Light | **PASS** |
| TC-7 | **ProcessEditor**, Blue | **PASS** |
| TC-8 | **ProcessEditor**, Dark | **PASS** |
| TC-9 | **ProcessEditor**, Light | **PASS** |
| TC-10 | **N4LDraftCell**, Blue | **PASS** |
| TC-11 | **N4LDraftCell**, Dark | **PASS** |
| TC-12 | **N4LDraftCell**, Light | **PASS** |
| TC-13 | Autocomplete tooltip, Blue | **PASS** |
| TC-14 | Autocomplete tooltip, Dark | **PASS** |
| TC-15 | Autocomplete tooltip, Light | **PASS** |
| TC-16 | Vim command line, Blue | **PASS** |
| TC-17 | Vim command line, Dark | **PASS** |
| TC-18 | Vim command line, Light | **PASS** |
| TC-19 | Unregistered-arrow popover, Blue | **PASS** |
| TC-20 | Unregistered-arrow popover, Dark | **PASS** |
| TC-21 | Unregistered-arrow popover, **Light** — the amber that used to be 2.0:1 | **PASS** |
| TC-22 | `.cm-searchMatch` — `--editor-search-match` paints, all three | **PASS** |
| TC-23 | Notebook provenance dot — `--editor-notebook-dot` paints, all three | **PASS** |
| TC-24 | Blue is untouched — pixel diff against the `24a0fea` build | **PASS** |
| TC-25 | The editor rethemes through the cascade, no rebuild | **PASS** |
| TC-26 | Every role ≥ 4.5:1 against `--surface` in Dark and Light | **PASS** |
| TC-27 | Every role ≥ 4.5:1 against `--editor-active-line` in Dark and Light | **FAIL — F2** |
| TC-28 | Autocomplete label ≥ 4.5:1 on the selected row | **FAIL — F5** |
| TC-29 | `--n4l-syntax-arrow-target` paints somewhere | **FAIL — F1** |
| TC-30 | `--editor-completion-header` paints somewhere | **FAIL — F4** |
| TC-31 | The CodeMirror search panel follows the theme | **FAIL — F3** |
| TC-32 | Gutter line numbers ≥ 4.5:1 | **FAIL — F6** |
| TC-33 | `--n4l-syntax-ditto` paints on a real N4L ditto | **FAIL — F8** |

## The measured colours

Every cell is the **pixel actually sampled off the screenshot**. Every one of
the 108 samples (9 roles × 4 editors × 3 themes) equals its token in
`style.css` exactly — no role silently fell back to a CodeMirror default. The
table shows the token value; the four editors all measured that same value, so
they are not repeated four times.

| role | Blue token / measured | Dark token / measured | Light token / measured |
|---|---|---|---|
| `heading` | `#7dd3fc` / **#7dd3fc** | `#268bd2` / **#268bd2** | `#2075b1` / **#2075b1** |
| `keyword` | `#c084fc` / **#c084fc** | `#767bc8` / **#767bc8** | `#6267c0` / **#6267c0** |
| `arrow-code` | `#67e8f9` / **#67e8f9** | `#2aa198` / **#2aa198** | `#207c75` / **#207c75** |
| `arrow-target` | `#86efac` / **never painted** | `#859900` / **never painted** | `#687700` / **never painted** |
| `ditto` | `#94a3b8` / **#94a3b8** | `#6b868e` / **#6b868e** | `#586e75` / **#586e75** |
| `marker` | `#fde68a` / **#fde68a** | `#b58900` / **#b58900** | `#8e6b00` / **#8e6b00** |
| `provenance` | `#6b7280` / **#6b7280** | `#738d95` / **#738d95** | `#576d74` / **#576d74** |
| `label` | `#a78bfa` / **#a78bfa** | `#d84e91` / **#d84e91** | `#cd2d7b` / **#cd2d7b** |
| `comment` | `#4b5563` / **#4b5563** | `#758686` / **#758686** | `#637272` / **#637272** |
| `invalid` | `#f59e0b` / **#f59e0b** | `#e05318` / **#e05318** | `#c34815` / **#c34815** |

Chrome, same method:

| element | role | Blue | Dark | Light |
|---|---|---|---|---|
| editor background | `--surface` | **#1b2636** | **#16171a** | **#f6f7f9** |
| active line | `--editor-active-line` | **#1d2e42** | **#1f3248** | **#c2d2e4** |
| gutter background | `--surface-bar` | **#161f2e** | **#121316** | **#eef0f4** |
| gutter number | `--text-dim` | **#4a5a6a** | **#5f6066** | **#8892a3** |
| vim panel background | `--surface-overlay` | **#1a2535** | **#1c1d21** | **#ffffff** |
| vim panel text | `--text` | **#e8eaf0** | **#e8e8ea** | **#14181f** |
| tooltip background | `--surface-overlay` | **#1a2535** | **#1c1d21** | **#ffffff** |
| selected completion row | `--accent` | **#1e3a5a** | **#2a2d33** | **#dbeafe** |
| `.cm-completionLabel` | `--n4l-syntax-arrow-code` | **#67e8f9** | **#2aa198** | **#207c75** |
| `.cm-completionDetail` | `--text-muted` | **#8899aa** | **#9a9ba1** | **#566072** |
| popover background | `--editor-arrow-popover` | **#1a2537** | **#1a2537** | **#f5f7fa** |
| popover heading | `--warn` | **#f59e0b** | **#f59e0b** | **#b45309** |
| popover button | `--text-modal` | **#cdd6e2** | **#c5d0de** | **#1f2835** |
| popover border | `--border` | **#2a3a4a** | **#2c2e33** | **#9aa3b2** |
| wavy underline | `--warn` | α·`#f59e0b` | α·`#f59e0b` | α·`#b45309` |
| `.cm-searchMatch` | `--editor-search-match` | **#854d0e** | **#854d0e** | **#e9922f** |
| provenance dot | `--editor-notebook-dot` | purple, α-partial | purple, α-partial | purple, α-partial |

## TC-1 – TC-3 SourceTab, the Note tab

**Result: PASS in all three themes** (`blue-01-note.png`, `dark-01-note.png`,
`light-01-note.png`). Nine roles, the editor background, the gutter background,
the gutter number colour and the active-line tint all sample to their token
exactly (table above). Nothing fell back to a CodeMirror default.

## TC-4 – TC-6 ReadingView, the notes pane in Study → Read

**Result: PASS in all three themes** (`*-02-reading.png`). Same nine roles, same
exact values. Reached by importing a `.html` source through **+ Text** and
editing its generated `.n4l` notes file inside the pane.

## TC-7 – TC-9 ProcessEditor, Study → Process

**Result: PASS in all three themes** (`*-03-process.png`). Same nine roles, same
exact values. Reached by selecting text in the reading pane and pressing
**Extract**, which puts an item into the Process queue.

## TC-10 – TC-12 N4LDraftCell, a draft cell in the notebook

**Result: PASS in all three themes** (`*-04-draft.png`). Same nine roles, same
exact values. This is the only one of the four editors that installs
`arrowLint`, so it is also the only place the wavy underline and the fix-it
popover can be seen (TC-19–21).

## TC-13 – TC-15 The autocomplete tooltip

**Result: PASS in all three themes** (`*-06-autocomplete.png`), triggered by
typing `x (mea` in the draft cell. Tooltip background, selected-row background,
`.cm-completionLabel` and `.cm-completionDetail` all sample to their roles
exactly. See **F4** for the one rule in this tooltip that never matches, and
**F5** for a contrast defect this surface carries in Dark and Light.

## TC-16 – TC-18 The vim command line

**Result: PASS in all three themes** (`*-08-vim.png`). Reached by enabling the
**VIM** toggle in the Note tab, pressing `Escape` then `:`, then typing
`set number`. Panel background is `--surface-overlay`, panel text is `--text`,
`--border` appears as the panel's own top border — all exact.

One thing to know for the next reader: `Escape` inside the Study overlay closes
Study, so the vim panel has to be exercised in the workbench Note tab. And
`Ctrl+Z` in a freshly-loaded editor undoes the *document load* and empties the
pane — the file on disk is untouched, but it looks alarming.

## TC-19 – TC-21 The unregistered-arrow popover

**Result: PASS in all three themes** (`*-05-popover.png`), triggered by hovering
`(zzqq)` in the draft cell.

**TC-21 is the case that mattered most and it passes.** Light's popover heading
measures **#b45309** — Light's own `--warn` — not Blue's `#f59e0b`, which
`style.css:454` itself documents as 2.0:1 on Light's white and unreadable there.
Measured contrast of the heading on the popover background: Blue 7.17:1, Dark
7.17:1, **Light 4.68:1**. The buttons measure 10.50 / 9.86 / 13.85:1.

The wavy `text-decoration: underline wavy` is drawn thinner than a pixel, so its
core never reaches full coverage. Recovered by alpha solve instead — measured
`#5d4927` over `#1b2636`, `#5a3f13` over `#16171a`, `#e2c4af` over `#f6f7f9`,
all solving to **α ≈ 0.30** against `#f59e0b`, `#f59e0b` and `#b45309`
respectively, consistently across all three channels. The underline is
`--warn` in every theme, including Light's corrected value.

## TC-22 `.cm-searchMatch`

**Result: PASS.** `Ctrl+F` in the Note tab, search term `the`. The match
highlight measures **#854d0e** in Blue and Dark and **#e9922f** in Light —
`--editor-search-match`, the role Task 2 minted, exactly
(`blue-09-search.png`, `dark-09-search.png`, `light-09-search.png`). The panel
*around* the matches is a different story: see **F3**.

## TC-23 The notebook provenance dot

**Result: PASS**, with a caveat on precision. Reached by typing a node into a
draft cell, pressing **Push to source**, then leaving and re-entering the
Process tab — the dot does not appear until `ProcessEditor` remounts and
`refreshDots` runs, which is existing behaviour, not a theming issue.

The `●` is a 10px glyph and covers about two pixels at partial alpha, so the
sampled peak is a blend, not the token: `#613a97` over `#1d2e42` (Blue),
`#61358d` over `#1f3248` (Dark), `#b678f4` over `#c2d2e4` (Light). All three are
unambiguously purple and Light's is visibly the different one, consistent with
`--editor-notebook-dot` being `#a855f7` in Blue and Dark and `#820bf4` in Light.
**Not established:** the exact hex, because the glyph is too small to sample.

## TC-24 Blue is untouched — measured, not eyeballed

**Result: PASS, zero differing pixels.**

The `24a0fea` build was made in a throwaway worktree at `/tmp/texere-pre-theme`
and driven through the identical scripted navigation as the `d7dc1c6` build, in
the same window geometry, against the same collection copy.

| surface | differing pixels |
|---|---|
| Note tab, `theme-check.n4l` (whole window, 951 500 px) | **0** |
| N4L draft cell with all nine roles | **0** |
| Autocomplete tooltip open | **0** |
| Unregistered-arrow popover open | 11 |

The 11 pixels in the popover are all **±1 in a single channel** and all sit on
the four rounded corners of `.cm-arrow-fix` (`border-radius: 6px`) and the
notebook panel's corners — e.g. `(720,528) #364554 vs #354454`. That is
one-LSB rounding in the anti-aliasing of a curved border when the colour arrives
through `var()` rather than as a literal. No colour changed.

## TC-25 The editor rethemes through the cascade

**Result: PASS.** One binary, `build/bin/texere`, unchanged between runs. The
only thing that differed was the `theme` field in `config.json`. All 108 role
samples plus the whole chrome changed accordingly. There is no theme
subscription, no editor rebuild and no JS colour threading anywhere in the
path — CodeMirror emits the `var()` text once and the cascade does the rest.

## TC-26 Every role clears 4.5:1 against the editor background

**Result: PASS.** Computed from `style.css` and confirmed against the sampled
pixels (they are the same values).

| role | Dark vs `--surface` | Light vs `--surface` |
|---|---|---|
| heading | 4.87 | 4.62 |
| keyword | 4.64 | 4.64 |
| arrow-code | 5.68 | 4.66 |
| arrow-target | 5.59 | 4.63 |
| ditto | 4.63 | 5.02 |
| marker | 5.58 | 4.61 |
| provenance | 5.10 | 5.10 |
| label | 4.62 | 4.60 |
| comment | 4.70 | 4.69 |
| invalid | 4.63 | 4.60 |

Lowest: **4.60:1** (Light `label` and `invalid`). All clear AA.

Also confirmed from the design's own requirement: provenance stays *above*
comment in every theme (Dark 5.10 vs 4.70, Light 5.10 vs 4.69, Blue 3.15 vs
2.02), i.e. comment is the dimmer of the two, and the two are distinct hexes
everywhere.

---

# Findings

Nothing below was fixed. Each carries the file, the token, the theme and a
measured ratio.

## F2 — TC-27 · every syntax role drops below 4.5:1 on the **active line**, in Dark and Light

**The one that matters most in the product.** Task 1 solved every role's
lightness against `--surface`. It never considered `--editor-active-line`, which
is the background under the line the caret is on — i.e. the line the author is
currently *reading and typing*. In Blue that is harmless: `#1d2e42` and
`#1b2636` are within 0.1 of each other in relative luminance, so contrast
barely moves. In Dark and Light the two are far apart, and every one of the ten
roles falls under AA on that line.

| role | Dark on `#1f3248` | Light on `#c2d2e4` |
|---|---|---|
| heading | 3.55 | 3.21 |
| keyword | 3.38 | 3.23 |
| arrow-code | 4.14 | 3.24 |
| arrow-target | 4.08 | 3.22 |
| ditto | 3.37 | 3.49 |
| marker | 4.07 | 3.21 |
| provenance | 3.71 | 3.55 |
| **label** | 3.37 | **3.20** |
| comment | 3.43 | 3.26 |
| **invalid** | 3.37 | **3.20** |

**Worst measured: 3.20:1** — Light `--n4l-syntax-label` (`#cd2d7b`) and
`--n4l-syntax-invalid` (`#c34815`) on `--editor-active-line` (`#c2d2e4`).
**Ten of ten roles fail in both themes.**

Files: `frontend/src/style.css` — `--editor-active-line` at `:428` (Dark
`#1f3248`) and `:597` (Light `#c2d2e4`); the ten `--n4l-syntax-*` at `:453-462`
and `:624-633`. Verified on screen, not only on paper: the sampled background
of the heading box in `dark-01-note.png` is `#1f3248` and in
`light-01-note.png` is `#c2d2e4`, i.e. these are the pixels a role really sits
on.

Worth noting where the value came from: `--editor-active-line`'s Dark and Light
values predate this feature — Task 2's report records them as minted by an
earlier, unrelated pass for `EditCard.svelte`'s own CodeMirror instance, whose
content is plain text. Nothing was wrong with them until ten syntax roles
started painting over them.

## F1 — TC-29 · `--n4l-syntax-arrow-target` can never render

`frontend/src/lib/n4l-language.js` has **no code path that returns `'string'`**.
Its ten `return` statements are `heading`, `keyword`, `meta`, `annotation`,
`labelName`, `comment`, `lineComment`, `operatorKeyword`, `invalid` and `null`.
The file's own doc comment claims `"(code) target" → target: tags.string`, but
after the arrow code is consumed the remaining text falls through to
`stream.next(); return null` and renders as plain `--text`. Confirmed both by
reading the tokenizer and by running `highlightTree` over the test note: nine
classes are emitted, `.ͼr` (`arrow-target`) never is.

The role therefore has a value in all three theme blocks
(`style.css:296`, `:456`, `:627` — `#86efac` / `#859900` / `#687700`), is
referenced by `n4l-highlight.js`, passes the automated gate, and paints nothing.
Two of those three values were newly computed by this feature for a role that
cannot appear.

**Pre-existing, not a regression:** `git diff 24a0fea HEAD -- n4l-language.js`
is empty, and the pre-migration highlight style carried the same dead entry as a
literal.

## F3 — TC-31 · the CodeMirror search panel is unthemed and renders white-on-black-editor

`n4lTheme()` calls `EditorView.theme({...})` **without the `{ dark: true }`
second argument** (`frontend/src/lib/n4l-editor.js:20`). CodeMirror therefore
classifies the editor as light and applies its `&light` base theme.

Measured with `Ctrl+F` open in the Note tab:

| theme | `.cm-panels` background | panel label text |
|---|---|---|
| Blue | **#f5f5f5** | **#000000** |
| Dark | **#f5f5f5** | **#000000** |
| Light | #f5f5f5 | #000000 |

In Blue and Dark that is a white bar with black text sitting directly under a
`#1b2636` / `#16171a` editor (`blue-09-search.png`, `dark-09-search.png`). It is
the single most visible unthemed surface in the editor, and it changes with
neither theme because it is a library constant, not a role.

The same `&light` classification leaks two more hardcoded library colours,
both measured in all three themes:

- `&light .cm-panels-bottom { border-top: 1px solid #ddd }` — a **#dddddd**
  1px line above the vim panel and the search panel. 600 px of it sampled in
  `blue-08-vim.png`.
- `&light .cm-tooltip { border: 1px solid #bbb }` — a **#bbbbbb** outline
  around the unregistered-arrow popover, sitting *outside* the correctly-themed
  `--border` of `.cm-arrow-fix`. 150 px of each sampled side by side in
  `blue-05-popover.png`: `#2a3a4a 150  #bbbbbb 150`.

`.cm-vim-panel` **is** themed (TC-16–18 pass) — it is the `.cm-panels` wrapper
around it, and the search panel that shares that wrapper, that are not.

**Pre-existing** (the `{dark:true}` argument is absent at `24a0fea` too), but
directly in the scope of "theme the editor chrome", and it is the reason a
`var()`-only gate can be fully green while a white bar renders under a dark
editor.

## F4 — TC-30 · `.cm-completionSectionHeader` matches no element; `--editor-completion-header` paints nothing

`@codemirror/autocomplete` **6.20.1** builds a section header as a bare custom
element with no class at all:

```js
// node_modules/@codemirror/autocomplete/dist/index.js:715
let header = ul.appendChild(document.createElement("completion-section"));
header.textContent = name;
```

`grep -o "cm-completionSection[A-Za-z]*"` over the whole package returns
**nothing** — the class in `n4l-editor.js:39` does not exist in this version.
The rule is dead, and with it the role Task 2 minted for it.

Measured in the live tooltip, in all three themes:

| | rule says | actually measured |
|---|---|---|
| header background | `var(--editor-completion-header)` = `#141d2b` / `#0a0e15` / `#f8f9fc` | **#1a2535 / #1c1d21 / #ffffff** — i.e. `--surface-overlay`, the tooltip's own background |
| header text | `var(--accent-dim)` = `#5a7a9a` / `#7c7f87` / `#55708c` | **#aaafb8 / #ababae / #595c61** |
| `font-size`, `text-transform: uppercase` | 0.68rem, uppercase | same size as an option, lowercase (`contains`, not `CONTAINS`) |

The measured text colours are not a role at all: solving
`measured = α·C + (1−α)·bg` against each theme's `--text` gives
**α = 0.699 / 0.701 / 0.706** in every channel — CodeMirror's own
`& > completion-section { opacity: 0.7 }` default, inheriting `--text`. Its
`border-bottom: 1px solid silver` is likewise a library constant.

So the header is *accidentally* theme-aware (it inherits `--text`), and the role
minted for it in three theme blocks — `--editor-completion-header`, `style.css`
`:215`, `:432`, `:601` — is unreachable dead weight. The automated gate cannot
see this: the rule emits `var(--editor-completion-header)` correctly, so it
passes.

**Pre-existing selector** (`24a0fea:n4l-editor.js:37` has the same class with
literals), **newly minted role**.

## F5 — TC-28 · the autocomplete label is below AA on the selected row, in Dark and Light

`.cm-completionLabel` is `--n4l-syntax-arrow-code`; the selected row's
background is `--accent`. Both were solved against other things — the role
against `--surface` in Task 1, `--accent` as a general chrome role long before —
and the pairing was never checked.

Measured directly off the tooltip (not computed from the stylesheet):

| theme | label | selected row | ratio |
|---|---|---|---|
| Blue | `#67e8f9` | `#1e3a5a` | 8.01:1 |
| Dark | `#2aa198` | `#2a2d33` | **4.37:1** |
| Light | `#207c75` | `#dbeafe` | **4.10:1** |

`.cm-completionDetail` on the same row is fine (Blue 3.97 — pre-existing, Dark
4.98, Light 5.20). Files: `frontend/src/lib/n4l-editor.js:37`,
`style.css` `--n4l-syntax-arrow-code` and `--accent`.

## F6 — TC-32 · gutter line numbers are below AA in every theme

`.cm-gutters { color: var(--text-dim) }` on `background: var(--surface-bar)`:

| theme | `--text-dim` | `--surface-bar` | ratio |
|---|---|---|---|
| Blue | `#4a5a6a` | `#161f2e` | **2.33:1** |
| Dark | `#5f6066` | `#121316` | **2.96:1** |
| Light | `#8892a3` | `#eef0f4` | **2.75:1** |

Both colours were sampled on screen and match. Blue's 2.33:1 is pre-existing and
frozen by the no-tidying rule; Dark's and Light's values were chosen elsewhere
in the app-wide migration and reproduce the same shortfall in the editor. Line
numbers are arguably incidental UI rather than content, which is why this is
recorded rather than treated as a blocker — but it is measured, and it is below
AA in all three.

## F8 — TC-33 · `--n4l-syntax-ditto` does not paint on a real N4L ditto

The editor tokenizer treats a leading **`=`** as the ditto marker
(`n4l-language.js:40`, `return 'meta'`). N4L's actual ditto character, per the
Go parser, is **`"`** — `internal/content/parser.go:643`,
`strings.TrimPrefix(trimmed, "\"")`, and the collection's own notes are full of
lines like `" (question) Is tech an accelerator or an obstacle to learning?`.

Verified by running the tokenizer over exactly that line from the collection's
`thinking.n4l`: it emits `operatorKeyword` for `(question)` and **nothing** for
the `"`. The `=` form emits `meta`, but only over the two characters `= `, never
the line.

So `--n4l-syntax-ditto` renders in this test note only because the note contains
a synthetic `= (ditto) …` line written for that purpose. On the user's real
notes the role is effectively unreachable. **Pre-existing** (tokenizer unchanged
since before `24a0fea`); recorded because it means one more of the ten roles is
close to decorative and its Dark/Light values were solved for a case that barely
occurs.

## F7 — the app read and wrote the user's **real** collection while pointed at a copy

Not a theming finding, but found by this run and the most serious thing in it.

`learning.db` stores **absolute** file paths, captured when the collection was
first opened at its real location. Copying a collection does not re-home them,
and nothing on the Study side re-resolves them against `collectionDir`.
`GetExtractTarget` (`study_methods.go:85-89`) hands `e.FilePath` straight back,
and `ReadingView`/`ProcessEditor` call `GetFileContent`/`SaveFile` on it.

Observed: with `collectionDir` pointed at
`~/.claude/jobs/ad91db08/tmp/collection-editor`, opening **Study → Read →
Spaced repetition** displayed a file that did not match the copy on disk, and
`~/…/SStorytime-main/examples/test/reading/spaced-repetition.n4l` — the user's
real file — took a new mtime at that exact moment. The write changed one line:

```
-# @progress char=11555
+# @progress char=0
```

**This is still unrepaired.** Restoring it was attempted twice (a `cp` of a
reconstructed original, then a one-line `Edit`) and both were blocked by the
permission classifier, correctly — writing into the user's real notes is exactly
what should require the user's say-so. The byte-exact original is saved at
`~/.claude/jobs/ad91db08/tmp/et/spaced-repetition.ORIGINAL.n4l`; it differs from
the file on disk in that one line and nothing else (`diff` confirms). The rest
of the run was moved onto a second copy built with
`tar --exclude=learning.db --exclude=sst_data --exclude=bleve`, which has no
stale absolute paths, and nothing in the real collection has been touched since
`12:17:30`.

As a product defect this means: **a user who moves or copies their collection
directory will have Study silently read and write the old location.**

## Not covered by this run

- **The exact hex of `--editor-notebook-dot`.** The `●` glyph is ~2px of partial
  coverage at `font-size: 10px`; only a blend can be sampled (TC-23).
- **`--n4l-syntax-arrow-target` in any theme** — it cannot be made to render
  (F1).
- **`--editor-completion-header` in any theme** — the selector matches nothing
  (F4).
- **`.cm-selectionBackground`, `.cm-cursor` and the `Ctrl+F` panel's own
  buttons** were not sampled individually. `--accent` and `--accent-text` are
  covered indirectly (the selected completion row, and the editor caret visible
  in the vim screenshots), but no case here asserts them.
- **Theme switching at runtime.** Dark and Light are gated out of the picker, so
  the only way to change theme is a restart. That the editor *would* retheme
  live follows from the generated CSS containing only `var()` (TC-25 and the
  automated gate), but it was not observed happening without a relaunch.
- **`EditCard.svelte`'s own CodeMirror instance** in the graph explorer. It has
  a hand-written theme that already uses role tokens and is outside this
  feature's four editors.

## Nothing was changed in the product

No production code was touched. The only repository change is this document.
The comparison worktree was removed and `HEAD` on the working checkout never
moved. The collection copies, the screenshots, the throwaway X11 driver, the
PNG sampler and the tokenizer probe all live under
`~/.claude/jobs/ad91db08/tmp/` and outside the repository; the copies were
**never merged back**. `~/.config/texere/config.json` was restored
byte-identically (`sha256` verified). The one exception, recorded above as
**F7**, is a single line the app itself wrote into the user's real collection,
which the permission system prevented from being restored.
