# SVG icon system — design

**Date:** 2026-08-18
**Branch:** `050-svg-icon-system` (from `main` at `446e2eb`)

## The problem

Nine controls in the UI are labelled with astral-plane emoji written directly
into the Svelte source — `💾`, `🔍`, `🌐`, `📖`, `📚`, `🗕`, `🎉`. On a machine
with no emoji font installed they render as tofu boxes: the toolbar shows empty
rectangles where Save, Search, Explorer, Read and Study should be.

This was found while testing feature 049 on WSL. The cause is not the app:

```
fonts covering U+1F4D6 (📖):  Segoe UI Emoji, Segoe UI Symbol
fc-match "📖"              →  DejaVu Sans          ← selected, but has no such glyph
```

Only the Windows fonts visible through `/mnt/c` cover those codepoints, and
fontconfig does not select them. BMP symbols in the same toolbar — `⚙` (U+2699),
`⟳`, `↩` — are covered by DejaVu Sans and render correctly. That split
(astral fails, BMP works) is the signature of a missing emoji font.

A font install fixes this box. It does not fix the general case: the UI depends
on glyphs it does not ship, and no `font-family` stack in the frontend declares
an emoji fallback. Windows builds get Segoe UI Emoji and most Linux desktops
ship Noto Color Emoji, so this is a bare-environment failure rather than a
universal one — but it is a real dependency on the host, silently taken.

**SVG has no such dependency.** The shapes ship with the app.

### A second defect, found on the way

Two controls put an astral emoji and a BMP symbol in the same ternary, so one
state renders and the other does not — independent of any font install:

- `OmniBar.svelte:955` — `{isFuzzyMode ? '🔍' : '◈'}`
- `SketchCell.svelte:195` — `{expanded ? '🗕 Collapse' : '⛶ Expand'}`

The font gap was masking this. Toggling either control changes it from a glyph
to a box.

### Dead scaffolding

The four `💬` buttons in `ColumnNodes.svelte` (lines 166, 195, 219, 255) are
`on:click={() => console.log('Discuss', node.Line)}`. They log to the console
and do nothing else. There is no Discuss feature behind them.

## Scope

**In scope:** the nine emoji call sites that fail to render, the two BMP symbols
that share a ternary with one of them, and deletion of the four dead buttons.

**Out of scope:** the BMP symbols that render correctly and are not part of a
mixed ternary (`⚙`, `⟳`, `↩`, and the rest). Converting those is churn on
working code with no defect behind it. If the project later wants total
consistency, that is a separate, larger change.

**Also out of scope:** adding an emoji fallback to the `font-family` stacks.
Once the emoji are gone this buys nothing, and leaving the stacks alone keeps
the diff to the actual problem.

## Architecture

Two modules, split along the line the repository already tests on. `make
test-js` runs `node:test` over pure `.js` modules in `src/lib/`; there is no
Svelte component harness. So the data lives where it can be tested and the
component stays thin enough that its correctness is obvious by inspection.

### `src/lib/icons.js` — pure data

```js
export const ICONS = {
  save:        { viewBox: '0 0 24 24', paths: [ '…' ] },
  search:      { viewBox: '0 0 24 24', paths: [ '…' ] },
  globe:       { viewBox: '0 0 24 24', paths: [ '…' ] },
  'book-open': { viewBox: '0 0 24 24', paths: [ '…' ] },
  books:       { viewBox: '0 0 24 24', paths: [ '…' ] },
  fuzzy:       { viewBox: '0 0 24 24', paths: [ '…' ] },
  expand:      { viewBox: '0 0 24 24', paths: [ '…' ] },
  collapse:    { viewBox: '0 0 24 24', paths: [ '…' ] },
  celebrate:   { viewBox: '0 0 24 24', paths: [ '…' ] },
}
```

The `'…'` above stands for the actual path string in each case; the nine
geometries are chosen at implementation time, not specified here. What this
section fixes is the *shape* of the module — a flat map, string keys, path
strings, no behaviour — which is what the component and the tests depend on.

No Svelte import, no side effects. Path geometry is taken from Lucide
(ISC-licensed) and embedded as strings, with the licence and provenance recorded
in the module header. Lucide is not added as a dependency — only the geometry is
copied, which keeps the app self-contained as its offline constraint requires.

### `src/components/Icon.svelte` — thin renderer

Props:

| prop | type | default | meaning |
|---|---|---|---|
| `name` | string | — | key into `ICONS` |
| `size` | number | `16` | width and height in px |
| `label` | string | `''` | accessible name; empty means decorative |

Renders a single `<svg>` with `fill="none"`, `stroke="currentColor"`,
`stroke-width="2"`, `stroke-linecap="round"`, `stroke-linejoin="round"`, and one
`<path>` per entry.

`stroke="currentColor"` is the load-bearing choice: icons inherit the text
colour of whatever button holds them, so they follow the light/dark theme with
no extra CSS and no theme-aware icon variants. `GraphExplorer.svelte:1530`
already uses inline SVG with `currentColor`, so this follows existing practice
rather than introducing it.

When `label` is empty the `<svg>` carries `aria-hidden="true"`; when set, it
carries `role="img"` and `aria-label={label}`.

**Unknown name:** render nothing and `console.warn` once. A typo must not blank
the toolbar or throw inside a button's render. The typo itself is caught at test
time, not runtime — see Testing.

## Call-site changes

| File | Line | Now | After |
|---|---|---|---|
| `MiddleBand.svelte` | 124 | `💾` | `<Icon name="save" />` |
| `MiddleBand.svelte` | 126 | `🔍` | `<Icon name="search" />` |
| `MiddleBand.svelte` | 135 | `🌐` | `<Icon name="globe" />` |
| `MiddleBand.svelte` | 139 | `📖 Read` | `<Icon name="book-open" /> Read` |
| `MiddleBand.svelte` | 140 | `📚 Study` | `<Icon name="books" /> Study` |
| `OmniBar.svelte` | 955 | `{isFuzzyMode ? '🔍' : '◈'}` | `<Icon name={isFuzzyMode ? 'search' : 'fuzzy'} />` |
| `SketchCell.svelte` | 195 | `{expanded ? '🗕 Collapse' : '⛶ Expand'}` | `<Icon name={expanded ? 'collapse' : 'expand'} />` + text |
| `StudySurface.svelte` | 210 | `Nothing due. 🎉` | `Nothing due. <Icon name="celebrate" />` |
| `RecallCard.svelte` | 29 | `Nothing due. 🎉` | `Nothing due. <Icon name="celebrate" />` |
| `ColumnNodes.svelte` | 166, 195, 219, 255 | `💬` button | **deleted** |

Nine icons, nine surviving call sites, four buttons removed.

### Accessibility

Three buttons are icon-only and today convey their meaning through `title`
alone: Save (124), Search (126) and Explorer (135). Conversion is the moment to
give each an `aria-label` matching its existing `title` text, with the icon
`aria-hidden`. Read and Study keep their visible text, so their icons stay
decorative.

This is deliberately narrow. It fixes the three controls the change already
touches; it is not an accessibility pass over the app.

## Testing

Rendering has no automated harness — there is no Svelte component test setup —
so the automated tests cover the data and the wiring, and appearance is
confirmed by screenshot.

**`src/lib/icons.test.js`** (`node:test`, run by `make test-js`):

- every entry has a `viewBox` and at least one non-empty path string
- names are unique and match the documented set

**Typo guard, same file:** read every `.svelte` file under `src/`, extract each
`<Icon name="…" />` literal, and assert it exists in `ICONS`. A mistyped name is
the realistic failure mode of this design, and nothing else catches it — the
component deliberately fails soft at runtime. Names built from an expression
(the two ternaries) are not literals, so the test must also assert that the
literal set it found is non-empty, or a regex that silently matches nothing
would pass while testing nothing.

**Acceptance check, same file:** scan the frontend source for astral-plane
codepoints (`U+1F300`–`U+1FAFF`) and assert none remain. This is the property
the feature exists to establish, and it is one line.

**Manual, with screenshots read back:**

1. The toolbar — Save, Search, Explorer, Read, Study all show shapes, not boxes.
2. `OmniBar` fuzzy toggle — **both** states, since a half-rendering ternary is
   one of the defects being fixed.
3. `SketchCell` expand toggle — both states, same reason.
4. Study empty state — the celebrate icon.
5. Light and dark theme, to confirm `currentColor` actually follows the theme.

The XTEST input driver and `XGetImage` capture tooling written for feature 049's
manual test plan can be reused; this box has no screenshot tool and no PIL.

## What this does not fix

- Emoji elsewhere in the product surface — README, commit messages, any
  generated content — are untouched. Only the UI is in scope.
- The `font-family` stacks still declare no emoji fallback. Any emoji added to
  the UI in future will fail the same way. The typo-guard test's astral-codepoint
  check is what would catch that, and it is the reason to keep it.
- Icon *shapes* are a judgement call. The tests prove an icon renders and is
  wired to the right control; they cannot prove it reads as "Study". That is
  what the screenshots are for.
