# Theming the N4L editor

**Date:** 2026-08-17
**Status:** Proposed — awaiting approval
**Feature area:** CodeMirror editors (Note tab, reading view, notebook draft cell, process editor)

## Summary

The CodeMirror editor's chrome and syntax highlighting move onto CSS custom
properties, so the editor follows the app's theme. Syntax colours get their own
`--n4l-syntax-*` namespace, with Blue unchanged and Dark and Light derived from
Solarized's hues, darkened or lightened until every role clears WCAG AA.

## Motivation

The themes feature migrated 59 components onto ~118 semantic role tokens, but its
file inventory was built from `.svelte` files. The editor's colours live in two
JavaScript modules and were never touched, so **the largest surface in the app
stays a navy slab in Dark and Light** — 22 literals in `n4l-editor.js`'s
`n4lTheme` and 10 in `n4l-highlight.js`'s `n4lHighlightStyle`.

## Grounding in the current implementation

- **`n4lTheme(fontSize)`** (`frontend/src/lib/n4l-editor.js:19`) is an
  `EditorView.theme({…})` covering the editor surface, content, gutters, cursor,
  selection, active line, search match, the vim panel and the autocomplete
  tooltip. It is applied inside `n4lBaseExtensions` (`:72`), so every editor gets
  it.
- **`n4lHighlightStyle`** (`frontend/src/lib/n4l-highlight.js:21-30`) defines ten
  tags: heading, keyword, operatorKeyword, string, meta, annotation, comment,
  labelName, lineComment, invalid. Its module-level `n4lHighlighting` export is
  built once at import.
- **Four components build editors** through `n4lBaseExtensions`:
  `ProcessEditor.svelte`, `ReadingView.svelte`, `notebook/N4LDraftCell.svelte`,
  `SourceTab.svelte`.
- **Both layers accept CSS variables.** Verified by probe, not assumed:

  ```
  HighlightStyle.define([{ tag: tags.heading, color: 'var(--n4l-heading)' }])
    → [".ͼo {color: var(--n4l-heading); font-weight: bold;}"]
  ```

  `EditorView.theme` builds without error from the same. Both emit CSS text;
  neither validates or parses the value.
- **The theme is applied by setting `data-theme` on `<html>`**, and 118 role
  tokens are already defined for all three themes in `frontend/src/style.css`.

## Decisions

- **Substitution only — no JavaScript threading.** Because both layers emit CSS,
  the editor rethemes through the cascade with no subscription, no rebuild and no
  `retheme()`. This is the opposite of the Chart.js work, where values are baked
  into JS objects, and it makes this feature far smaller than it appears.
- **Chrome reuses the existing role tokens.** `--surface`, `--surface-bar`,
  `--border`, `--accent-text`, `--accent`, `--surface-overlay`, `--text`,
  `--text-muted` and friends already carry the right meanings. New roles are
  minted only where no existing Blue value matches byte-for-byte.
- **Syntax gets its own `--n4l-syntax-*` namespace.** These encode N4L grammar —
  green *means* arrow target, cyan *means* arrow code — not app chrome. Kept
  separate for the same reason `--explorer-*` is: a future tweak to `--ok` for a
  status chip must not silently recolour every arrow target.
- **Blue keeps its exact current ten values.** The Note tab looks identical after
  this lands, consistent with the constraint the whole migration held.
- **Dark and Light borrow Solarized's hues, adjusted to clear AA.** Solarized is
  deliberately built around uniform perceptual relationships rather than WCAG
  thresholds: measured against Solarized's *own* `base3` background, eight of its
  nine accents fall between 2.48:1 and 4.30:1. Adopting its backgrounds would not
  help — blue measures 3.41:1 on `#fdf6e3` and 3.43:1 on our `#f6f7f9`. So the
  hues are kept and the lightness moved until each role reaches ≥4.5:1.
  **The honest label is Solarized-derived, not Solarized.**
- **Solarized's backgrounds are not adopted.** The editor's chrome is already
  expressible in our role tokens, and a warm cream editor among cool-white panels
  would read as a different application.
- **Provenance and `# comment` stay distinguishable.** Blue renders them as two
  greys — `#6b7280` and `#4b5563`, comment the dimmer. A naive Solarized mapping
  sends both to `base1` and collapses that. Both derive from the grey ramp, and
  comment stays the dimmer of the two in every theme.

## Architecture

### `frontend/src/style.css`

Ten `--n4l-syntax-*` roles in each of the three theme blocks:

| role | Blue (unchanged) | Light | Dark |
|---|---|---|---|
| `heading` | `#7dd3fc` | `#2075b1` | `#268bd2` |
| `keyword` | `#c084fc` | `#6267c0` | `#767bc8` |
| `arrow-code` | `#67e8f9` | `#207c75` | `#2aa198` |
| `arrow-target` | `#86efac` | `#687700` | `#859900` |
| `ditto` | `#94a3b8` | `#586e75` | `#6b868e` |
| `marker` | `#fde68a` | `#8e6b00` | `#b58900` |
| `provenance` | `#6b7280` | *(grey ramp)* | *(grey ramp)* |
| `label` | `#a78bfa` | `#cd2d7b` | `#d84e91` |
| `comment` | `#4b5563` | *(dimmer grey)* | *(dimmer grey)* |
| `invalid` | `#f59e0b` | `#c34815` | `#e05318` |

Every Light and Dark value above is derived and measured at ≥4.60:1 against that
theme's editor background (`--surface`: `#f6f7f9` Light, `#16171a` Dark). Dark
keeps five of Solarized's published values exactly — heading, arrow-code,
arrow-target, marker and the light grey — because only four accents failed there.
The two grey roles are left to the implementation to derive, subject to staying
distinct and both clearing AA.

### `frontend/src/lib/n4l-highlight.js`

Each tag's `color` becomes `var(--n4l-syntax-…)`. Nothing else changes; the
module-level `n4lHighlighting` export stays as it is, because a CSS variable
re-resolves without the module being re-evaluated.

### `frontend/src/lib/n4l-editor.js`

Each literal in `n4lTheme` becomes `var(--…)`. `fontSize` remains the only
per-editor knob.

## Error handling

| Case | Behaviour |
|---|---|
| A `--n4l-syntax-*` role is undefined | The declaration is invalid and CodeMirror's own default colour applies — degraded, not broken. The test below prevents it reaching main. |
| A theme block omits a role | Caught by the completeness test, as elsewhere in this feature. |
| An editor mounts before the theme is applied | The bootstrap sets `data-theme` before first paint; a variable resolves at paint time, so there is no stale-value window. |

## Testing

**The test that bites, and is only possible because of the probe:** the generated
CSS is reachable from JavaScript, so `node --test` can assert that **every colour
the editor emits is a `var(--…)` and no hex literal survives** in either module.
That fails the moment someone reintroduces a literal — something no amount of
visual checking would catch reliably.

Plus: the ten syntax roles present in all three theme blocks; Blue's ten values
byte-identical to today's; and each Light and Dark value at ≥4.5:1 against that
theme's editor background, asserted with a contrast function in the test rather
than trusted from this document.

**Manual:** its own test-plan document, covering all four components that build
editors, in all three themes, plus the autocomplete tooltip and the vim panel —
the two surfaces easiest to miss because they appear only on interaction.

## Out of scope

- The Chart.js palette (series, `.sp-value`, axes, gridlines) — its own task.
- The remaining Light defects found by the themes verification pass: SketchCell's
  default pen, the explorer's DOM overlay, and the contrast failures in
  `ColumnNodes`, `LogPanel`, `ColumnContextTags` and `SourcesSidebar`.
- Re-enabling Dark and Light in the picker, which stays gated until those land.
- A user-configurable syntax palette.
