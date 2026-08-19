# Keyboard shortcuts help panel — design

**Date:** 2026-08-19
**Branch:** `053-shortcuts-help-panel` (from `main`)
**Related:** `2026-08-19-guard-tests-are-a-symptom.md` (why this feature's guard
is the third of its kind, and what would remove it)

## The problem

Texere has around twenty keyboard shortcuts and no way to discover them. A few
are advertised in `title` tooltips on toolbar buttons — `Save (Ctrl+S)`,
`Search (Ctrl+K / Cmd+K)`, `Study — … (Ctrl+Shift+S)` — but only for the
handful of actions that happen to have a button. Everything else is invisible:
`Ctrl+G` builds the graph, `Ctrl+Shift+M` toggles the log panel, `Alt+←`
navigates explorer history, `Ctrl+E` extracts a selection while reading. None
of those announce themselves anywhere.

The 2026-08-12 themes design anticipated this. It placed the `⚙` settings
button where a pair would sit and listed "the `?` help menu itself" as out of
scope, "this feature only positions the `⚙` where the pair will sit". This
feature fills that slot.

## The complication

**There is no central keymap.** Every binding is an inline check inside one of
~20 `keydown` handlers, and they are written in two unrelated syntaxes:

- DOM handlers — `if ((e.ctrlKey || e.metaKey) && e.key === 'k')` — 15 sites
- CodeMirror keymaps — `{ key: 'Mod-s', run: … }` — 3 sites

So a help panel is necessarily a *second* description of facts that already
live in the source, and it will drift the first time someone adds a binding.
Nothing would notice.

Three ways to handle that were considered:

1. **A hand-maintained list.** Honest today, quietly wrong in three months.
2. **A central registry the handlers dispatch from.** Cannot drift by
   construction, because the panel and the dispatcher read one table. This is
   the right long-term answer and is recorded as such in the companion note —
   but it means rewriting every keyboard path in the app, which is not a short
   capability.
3. **A data module plus a guard test that cross-checks it against the source.**
   Chosen. It is the same instrument that closed features 050 and 052, it is
   small, and it fails loudly when the two descriptions disagree.

## Architecture

Three pieces, split along the line this repo already tests on: `make test-js`
runs `node:test` over `frontend/src/lib/*.js`, and there is no Svelte component
harness.

### `frontend/src/lib/shortcuts.js` — pure data

```js
export const SHORTCUT_GROUPS = [
  { title: 'Global', items: [
    { keys: ['Ctrl/Cmd', 'G'], label: 'Build graph', site: 'App.svelte' },
    …
  ]},
  …
]
```

The `…` above stands for the remaining entries; the full inventory is the table
under Scope. What this fixes is the module's shape — a flat list of groups, each
a list of items with `keys`, `label` and `site` — which is what the component and
the guard both depend on.

Each entry carries the keys, a human label, and the source that implements it.
`site` is what makes the guard possible: it is the claim the test verifies.

**`site` must accept an array, not only a string.** This is not an edge case —
it is the common case. `Ctrl/Cmd+S` in the editor is three CodeMirror keymaps
(`SourceTab.svelte:384`, `ProcessEditor.svelte:174`,
`explorer/EditCard.svelte:49`), and `Ctrl/Cmd+Enter` is three DOM handlers
(`notebook/ShellCell.svelte:119`, `notebook/LLMCell.svelte:151`,
`LLMChatPane.svelte:165`). One user-facing shortcut routinely has several
implementations, because the same gesture is wired up per surface. The guard's
"no undocumented bindings" direction must therefore treat a documented binding
as covering *every* site it lists, or those extra sites read as undocumented and
the test fails on correct code.

Five groups, in the order a user meets them: **Global**, **Editor**,
**Study & Read**, **Explorer**, **Notebook cells**.

### `frontend/src/components/ShortcutsPanel.svelte` — the panel

Renders the groups. Opens two ways:

- the **`?` button** beside the `⚙`, filling the slot the themes feature reserved
- **F1**, from anywhere

F1 rather than `?` (Shift+/): `?` is a character users type into notes and query
fields, so binding it needs an is-this-a-text-field guard that F1 does not. F1
is unbound today and means nothing else in a text field.

Closes on `Escape`, matching every other panel in the app.

### `frontend/src/lib/shortcuts.test.js` — the guard

Checks **both directions**, because each catches a different mistake:

- **No phantom entries.** Every documented binding is found at the site it
  names. Catches a shortcut that was removed or renamed while the panel kept
  advertising it.
- **No undocumented bindings.** Every shortcut found in source is either
  documented or listed in an explicit `CONTEXTUAL` allowlist with a reason.
  This is the direction that matters: it fires when someone adds a binding and
  forgets the panel.

The extractor must handle both syntaxes, and both were prototyped against the
real tree before this design was written. Two corrections came out of that,
either of which would have shipped as a wrong panel:

- **A negated modifier is not a required one.** `OmniBar.svelte:144` reads
  `(e.ctrlKey || e.metaKey) && !e.shiftKey && e.key === 'k'`. A naive scan
  reports `Ctrl+Shift+K`; the real binding is `Ctrl+K`, and `Ctrl+Shift+K` is a
  *different* shortcut ten lines below.
- **`ctrlKey || metaKey` is one key, not two.** It is the cross-platform idiom
  and renders as a single `Ctrl/Cmd`.

## Scope

**In scope:** the shortcuts worth memorising — global ones, and those that
define a mode. The inventory below is complete as of this design, with every
label read from the handler rather than inferred from the key.

| Group | Keys | Action | Site |
|---|---|---|---|
| Global | `Ctrl/Cmd+G` | Build graph | `App.svelte:105` |
| Global | `Ctrl/Cmd+K` | Search (fuzzy) | `OmniBar.svelte:144` |
| Global | `Ctrl/Cmd+Shift+K` | Graph query | `OmniBar.svelte:154` |
| Global | `Ctrl/Cmd+Shift+S` | Toggle Study | `App.svelte:124` |
| Global | `Ctrl/Cmd+Shift+E` | Open Read | `App.svelte:132` |
| Global | `Ctrl/Cmd+Shift+R` | Toggle Recall overlay | `App.svelte:117` |
| Global | `Ctrl/Cmd+Shift+M` | Toggle log panel | `App.svelte:111` |
| Global | `F1` | This panel | *new* |
| Editor | `Ctrl/Cmd+S` | Save | `SourceTab.svelte:384`, `ProcessEditor.svelte:174`, `explorer/EditCard.svelte:49` (CodeMirror keymaps) |
| Study & Read | `Ctrl/Cmd+E` | Extract selection | `ReadingView.svelte:138` |
| Study & Read | `Ctrl/Cmd+S` | Save notes | `ReadingView.svelte:139` |
| Explorer | `Ctrl/Cmd+Shift+K` | Focus query bar | `GraphExplorer.svelte:1316` |
| Explorer | `Alt+←` / `Alt+→` | History back / forward | `GraphExplorer.svelte:1354,1360` |
| Notebook | `Ctrl/Cmd+Enter` | Run cell / send | `notebook/ShellCell.svelte:119`, `notebook/LLMCell.svelte:151`, `LLMChatPane.svelte:165` |

**Out of scope:** rebinding or customising shortcuts; per-OS display beyond the
`Ctrl/Cmd` idiom already present in the source; and generic list navigation
(arrows, Tab, Enter, Escape inside pickers), which goes in the `CONTEXTUAL`
allowlist rather than the panel. Users do not look up how to arrow through a
list, and listing those would bury the entries that matter.

Also out of scope, narrowed after this table was first drafted: `Escape`
(close log panel, close Study) and `Space`/`Enter` (reveal answer). These are
unmodified, contextual keys of the same kind as list navigation — not global
or mode-defining — and the shipped panel does not list them, on the same
"don't bury the entries that matter" reasoning as the carve-out above. `F1` is
the sole binding; `?` was considered (see below) but never wired up, so the
row above lists `F1` only.

## Two things this surfaced, recorded rather than fixed

**The app has two study entry points.** `Ctrl/Cmd+Shift+R` toggles `studyOpen`,
which mounts `StudyMode.svelte` — described in `stores/study.js` as "the Recall
(Study) overlay". `Ctrl/Cmd+Shift+S` toggles `studySurfaceOpen`, which mounts
`StudySurface.svelte`, the unified Read / Process / Recall / Stats surface with
its own Recall tab. Both are live and mounted from `App.svelte`. This design
documents both honestly, because that is what is true — but a help panel listing
two overlapping ways to reach Recall is worth someone deciding about. That
decision is not this feature's.

**`Ctrl/Cmd+S` means two different things.** In the editor it saves the file
(CodeMirror keymap); in ReadingView it flushes notes. Both are correct in
context and neither should change; the panel groups them separately so the
difference is visible rather than confusing.

## Testing

Automated, via `make test-js`:

- the data module's shape — every group has a title and at least one item;
  every item has keys, a label and a site
- the guard, both directions, over both syntaxes
- a vacuity assertion: the scan must find at least the number of sites known to
  exist, so a broken extractor fails loudly instead of silently reporting
  "nothing undocumented"

Manual, because there is no component harness: the panel opens from both the `?`
button and F1, closes on `Escape`, and renders correctly in **all three themes** —
Blue, Dark and Light. Feature 052 made Light real, and a new panel is exactly
the kind of surface that gets built against one theme and checked against none.

## What the guard cannot do

It sees only the two syntaxes it knows. A binding written a third way is
invisible to both the guard and the panel — which is precisely how `Ctrl+S`, the
app's most advertised shortcut, was missing from the first prototype until the
CodeMirror form was added. The vacuity assertion limits the blast radius; it
does not eliminate it. The companion note records what would.
