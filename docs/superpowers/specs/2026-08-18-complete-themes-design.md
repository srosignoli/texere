# Complete Dark and Light themes — design

**Date:** 2026-08-18
**Branch:** `052-complete-themes` (from `main`)
**Follows:** `2026-08-12-themes-design.md` (theme infrastructure, shipped Blue),
`2026-08-17-editor-theme-design.md` (the CodeMirror chrome)

## The problem

The Appearance menu advertises three themes and lets you pick one. Dark and Light
are listed as unavailable, under the note "Dark and Light are in progress."

Both themes are real. They are valid ids, they normalize to themselves, they have
complete token sets, and they paint correctly if you hand-edit `config.json`.
What stops them being offered is `available: false` in `frontend/src/lib/themes.js`
and one true fact: around 236 colours are still written as literals in component
CSS, so any surface using one keeps its Blue-era appearance no matter which theme
is active. On Dark that is a mild mismatch. On Light it is dark text on dark
panels — unreadable.

This feature closes that: convert the literals, flip both switches, delete the
note.

## What is already done

The `themes.js` comment gating Dark and Light names its blockers as "CodeMirror,
the graph explorer's DOM overlay, SketchCell's default pen, and more". Two of
those are stale, and the third is one line:

| Named blocker | Actual state |
|---|---|
| CodeMirror | **Done.** `lib/n4l-editor.js` uses 30 `var(--…)` references and holds 2 literals, both `rgba(0,0,0,0.55)` drop shadows that a comment already marks as intentionally unthemed. Feature 047 finished this. |
| Graph explorer | **Half done, and the half that is done is the hard half.** `components/explorer/Theme.js` already resolves every colour from CSS variables *at call time*, deliberately, with a comment explaining that eagerly-evaluated constants used to pin the palette to whichever theme was loaded first. The 3D scene is theme-reactive today. What is not converted is the **DOM overlay** — 38 literals in `GraphExplorer.svelte`'s `<style>` block, plus its sidebars. |
| SketchCell pen | One line: `const COLORS = ['#e8eaf0', …]`. |

The token vocabulary is also ready. `:root` declares 148 custom properties;
`[data-theme="dark"]` and `[data-theme="light"]` each override exactly the same
132. This is substitution work against an existing vocabulary, not new colour
design.

## Scope

**In scope:** the ~236 literal colours across 40 non-test files, the SketchCell
pen default, an automated guard against regression, and flipping both themes to
`available: true`.

The distribution is concentrated — six files hold over half:

| File | literals |
|---|---|
| `components/GraphExplorer.svelte` | 38 |
| `components/ExplorerSidebar.svelte` | 27 |
| `components/OmniBar.svelte` | 20 |
| `components/StatsPanel.svelte` | 13 |
| `components/LogPanel.svelte` | 13 |
| `components/MiniMap.svelte` | 12 |
| 34 further files | 1–9 each |

**Out of scope**, carried forward from the 2026-08-12 design: following the OS
theme automatically, user-authored themes, and any colour picker. A fourth
"Auto" state would interact with both the saved preference and the pre-paint
bootstrap in `index.html`; that is its own feature.

## What does not convert

Not every literal is a theming defect. Two categories stay, and the guard test
carries an explicit allowlist for them:

- **Shadows.** Around 34 declarations, mostly `rgba(0,0,0,…)` in `box-shadow`.
  `n4l-editor.js` already sets the precedent: "boxShadow's rgba(0,0,0,0.55) is a
  drop shadow, not a themed colour — it stays dark under the tooltip in every
  theme." A shadow is a depth cue, not a surface.
- **Scrims** that must stay dark in every theme to keep content readable beneath
  them.
- **Documented fallbacks**, specifically the 3 literals in `explorer/Theme.js`,
  which exist so the explorer still renders when `getComputedStyle` returns
  nothing (a test environment with no real document).
- **Data-encoding colours**, on the same reasoning as the `--explorer-*` tokens
  below: a chart series colour identifies a series, not a surface. `lib/stats-charts.js`
  is the case in point, and it is *mixed* — `TICK` (`#8899aa`) and `GRID`
  (`#2a3a4a`) are chart chrome and must convert; the default series colour
  (`#7ecfa0`) is data and may stay. Splitting that file's literals by which kind
  they are is real work, not a bulk substitution.

Every allowlisted colour carries a comment saying why it is exempt. A colour
without that comment is not exempt — it is unconverted work.

### Three cases the bulk count hides

Counting literals with a regex flattens distinctions that matter. Three found
while writing this spec:

- **`components/explorer/NodeMeshes.js`** hardcodes `CYCLE_GLYPH_COLOR`
  (`#f4b545`) and `CYCLE_GLYPH_HALO` (`#1a1306`) for the 3D scene. `Theme.js`
  describes itself as the single source of truth for explorer colour and resolves
  everything at call time; these two bypass it. They are genuine conversion work,
  and they mean that claim is currently untrue.
- **`lib/bake.js`** sets `SKETCH_BG = '#0b1220'`, commented "dark bg preserves
  contrast for light strokes". A baked sketch is a *persisted artifact*, so this
  interacts with the pen decision: bake dark ink drawn on a light canvas onto a
  dark background and the contrast inverts. Whatever the pen does, baking must
  agree with it. `lib/sketch-render.js`'s `s.color ?? '#e8eaf0'` fallback is the
  same decision in a third place.
- **`lib/n4l-escape.js:96`** is a **false positive**: `#4312` appears inside a
  prose comment (`incident #4312 - prod`). The ~236 figure includes matches like
  this, so the guard must not flag them — it needs to ignore comments, or require
  a CSS-property context, and its own tests must include a commented `#4312` to
  prove it does.

## The guard

Converting 236 literals is worthless if the 237th lands next week. There is no
Svelte component test harness, so nothing would notice.

A `node:test` spec (`frontend/src/lib/theme-tokens.test.js`, where `make test-js`
will find it) scans every non-test `.svelte` and `.js` under `src/` for literal
hex and `rgb()`/`rgba()` colours and fails on any not covered by the allowlist.
This is the same instrument that closed the icon feature — there, a scan for
astral-plane codepoints; here, a scan for raw colours — and it is what makes the
result durable rather than a one-off cleanup.

It must also assert it scanned a non-zero number of files. A file-walking test
that silently stops walking passes forever while checking nothing; that exact
failure mode was demonstrated during feature 050 and is the reason this sentence
exists.

**What the guard cannot do:** prove the *right* token was chosen. `--text-muted`
where `--text-secondary` belonged converts cleanly, passes the guard, and looks
subtly wrong for good. Only looking catches that, which is what the verification
section is for.

## Two judgement calls

**The SketchCell pen.** `COLORS[0]` is `#e8eaf0`, near-white — correct ink on a
dark canvas, invisible on a light one. Only the default swatch becomes
theme-aware; the four accents (`#f59e0b`, `#7dd3fc`, `#ff6b6b`, `#10b981`) stay
literal, because they read on both backgrounds. This is the smallest change that
fixes the actual defect, and it avoids a per-theme palette that would make a
saved sketch look different from how it was drawn.

The decision does not live in one file. `sketch-render.js` falls back to
`#e8eaf0` for a stroke with no colour, and `bake.js` renders onto a fixed dark
`#0b1220`. All three must agree, or a sketch drawn in Light will bake to
invisible ink. Baking is the one that matters most: it writes a file.

**The explorer's data colours.** Sixteen tokens — `--explorer-edge-*`,
`--explorer-node-*`, `--explorer-territory-0…7` — are declared in `:root` and
deliberately *not* overridden per theme. They encode data (edge kind, node role,
territory identity), not chrome, so a graph keeps its meaning across a theme
switch.

They stay shared. If the light-mode screenshots show a territory or edge colour
that lacks contrast on a light background, *that* colour gets a light override —
decided from evidence rather than pre-emptively. Guessing which of sixteen data
colours needs adjusting, before seeing one on a light canvas, would be inventing
work.

## Verification

Both themes, every major surface, screenshots read back:

1. Toolbar and the three columns
2. The note editor (its chrome is already themed; this confirms it against a
   converted background)
3. Study — Read, Process, Recall, Stats
4. The graph explorer: 3D scene, DOM overlay, sidebar, legend, minimap
5. Notebook cells, including a sketch with the default pen on both canvases
6. The Appearance menu itself, showing three selectable themes

Light mode gets the closer read, because that is where a missed literal stops
being a shade and becomes unreadable text. The 2026-08-12 manual test plan
already flags native `<select>` popups and scrollbars as light-mode hazards;
both are re-checked here.

Theme switching is also verified live, not only at launch: the explorer's
`Theme.js` resolves colours at call time specifically so a switch repaints an
open scene, and that behaviour has a regression history worth re-confirming.

## Risk

This is a large, mostly mechanical change with weak automated coverage by
nature — the guard proves no literals remain, not that the substitutions were
right. The mitigation is that a wrong token is *visible*: it renders as the wrong
shade in a screenshot someone reads. The failure this design most wants to avoid
is not a wrong token but an unlooked-at surface, which is why verification
enumerates surfaces rather than sampling them.
