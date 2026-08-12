# Themes: Blue, Dark, Light

**Date:** 2026-08-12
**Status:** Proposed — awaiting approval
**Feature area:** Application-wide appearance

## Summary

The app gains three themes — **Blue** (today's palette, the default), **Dark**
and **Light** — chosen from a `⚙` popover at the right end of the action bar and
persisted in `config.json`.

## Motivation

Every colour in the app is a blue-family literal written into a component's
`<style>` block. There is no way to work in a light room, and no way to use the
app beside light-themed tools without a jarring switch.

## Grounding in the current implementation

Measured against the tree at `30f378d`:

- **924 hard-coded colour literals across 56 components.** 172 distinct hex
  values plus 21 distinct `rgba()`. Of the hex values, **40 are used five times
  or more** and **81 appear exactly once**.
- **The only CSS variables are the graph explorer's** (`frontend/src/style.css:16-60`)
  — edge, node, territory, ground and label colours. They exist because Three.js
  cannot read a component's scoped CSS, not as a general theme layer.
- **`color-scheme: dark` on `:root` is load-bearing.** The comment at
  `style.css:17-20` records why: WebKitGTK uses it to theme native `<select>`
  popups and scrollbars, and without it a select renders light-on-light and
  ignores CSS styling of the native popup.
- **Three.js resolves those variables at scene-build time** (`style.css:11-12`),
  so an explorer already on screen will not restyle when the variables change.
- **`html` is painted before Svelte mounts** — `style.css:2` sets
  `background-color: rgba(27, 38, 54, 1)`. A stored light theme would therefore
  flash blue on every launch.
- **`index.html` already carries inline `<script>` blocks** for error handling,
  so a pre-paint bootstrap script matches the file's existing shape.
- **`AppConfig`** (`app.go:22-33`) is the persisted user config, loaded and saved
  by `loadConfig`/`saveConfig` (`methods.go:28,47`). `loadConfig` returns an
  empty struct on a missing, unreadable or corrupt file, so an absent field is
  already the first-run path. `UpdateRecentArrows` (`methods.go:294`) is the
  existing pattern for a small binding that mutates one field.
- **There is no global settings surface.** The only one is `⚙ Models`
  (`NotebookPane.svelte:110`), which opens `LLMSettings` and is scoped to the
  notebook pane.
- **The action bar's left-hand buttons live inside
  `{#if $workbenchStore.activeTab === 'note'}`** (`MiddleBand.svelte:111-140`).
  Only the spacer and the reminder badge sit outside that guard, which makes the
  bar's right end the app's one persistent global zone.

## Decisions

- **Three themes: Blue, Dark, Light. Blue is the default.** Blue is what every
  surface was designed against and remains the app's identity; Dark is a neutral
  near-black; Light is the new capability. A first run, an upgrade from a config
  with no theme field, and an unrecognised value all resolve to Blue.
- **A semantic token layer, not a palette swap.** ~15 role tokens — surface,
  surface-raised, surface-sunken, border, border-strong, text, text-muted,
  text-faint, accent, accent-hover, accent-text, ok, warn, danger — defined on
  `:root` and overridden under `:root[data-theme="dark"]` and
  `[data-theme="light"]`. Switching sets one attribute on `<html>`.
- **Blue's token values are exactly today's literals.** Every migration step is
  therefore a visual no-op until the picker lands, and "nothing changed" is a
  test that can actually be run — by pixel-diffing a build against the previous
  commit, the technique already proven in
  `2026-08-12-node-filters-manual-test-plan.md`.
- **Semantic colours stay literal.** An error red, the amber reminder banner and
  the chart series colours mean the same thing in every theme; routing them
  through the theme layer would be theming for its own sake. They are instead
  checked for contrast against a light surface. The judgement of which is which
  belongs to whoever migrates the file — it cannot be made from a global list.
- **`color-scheme` flips with the theme**, or native controls break in light
  mode.
- **The explorer's semantic hues do not change.** Edge colours encode arrow
  meta-types and territory colours distinguish contexts; both must stay legible
  and stable across themes. Only ground, label, label-halo and the plane colours
  get per-theme values.
- **A theme change rebuilds an open explorer.** If a rebuild proves expensive,
  the fallback is to apply the theme when the explorer next opens and say so in
  the UI — never to leave it silently stale.
- **`config.json` is authoritative, `localStorage` is the pre-paint mirror.**
  The duplication exists for exactly one reason: a synchronous read is available
  before first paint and a Wails binding is not. The frontend reconciles from Go
  on startup and writes both on every change.
- **The control is a `⚙` popover at the right end of the action bar**, outside
  the tab guard, where the planned `?` will sit beside it. Rejected: three
  toolbar buttons, which spend permanent space in a bar that already wraps at
  narrow widths, on a setting changed twice a year; and putting the theme inside
  the `?` menu, which conflates "how does this work" with "how should this
  behave".

## Architecture

### `internal` / Go

`AppConfig` gains one field:

```go
Theme string `json:"theme"` // "blue" | "dark" | "light"; "" = blue (first run)
```

Two bindings, following `UpdateRecentArrows`:

```go
func (a *App) GetTheme() string        // normalised; never returns ""
func (a *App) SetTheme(theme string) error  // rejects anything else
```

`SetTheme` validates rather than trusting its caller — an arbitrary string
written into `config.json` would become a `data-theme` attribute value.

### `frontend/src/lib/themes.js` (new)

Pure and dependency-free, so it runs under `node --test`.

```
THEMES         -> [{ id: 'blue'|'dark'|'light', label: string }]
isValidTheme(v) -> boolean
normalizeTheme(v) -> 'blue' | 'dark' | 'light'   // unknown, empty, null → 'blue'
```

`normalizeTheme` is the single place the default lives, so Go, the bootstrap
script and the picker cannot disagree about what an unset theme means.

### `frontend/src/style.css`

Gains the role tokens for all three themes, and moves the explorer's
ground/label/plane variables under the same per-theme blocks. `html`'s
background becomes `var(--surface)`.

### `frontend/index.html`

An inline script before the module script:

```html
<script>
  try {
    var t = localStorage.getItem('texere.theme')
    if (t === 'dark' || t === 'light') document.documentElement.dataset.theme = t
  } catch (e) {}
</script>
```

Blue needs no attribute — it is `:root`'s own values — so the script only acts
for the two themes that differ, and a `localStorage` exception (private mode, a
cleared store) silently leaves the default.

### `frontend/src/stores/theme.js` (new)

A Svelte store holding the active theme: reconciles from `GetTheme()` on
startup, and on change sets `document.documentElement.dataset.theme`, writes
`localStorage`, calls `SetTheme()`, and notifies the explorer.

### `frontend/src/components/SettingsMenu.svelte` (new)

The `⚙` popover: three theme choices, marked up as a radio group so the current
one is announced, not merely coloured. Rendered in `MiddleBand.svelte` after the
spacer, beside the reminder badge.

## Error handling

| Case | Behaviour |
|---|---|
| `config.json` missing, corrupt or has no theme | Blue, via `loadConfig`'s existing first-run path |
| `SetTheme` called with an unknown value | Rejected; the stored theme is unchanged |
| `localStorage` unavailable or throws | The bootstrap silently no-ops; Go's value still applies after mount, with a possible one-frame flash |
| `GetTheme` fails at startup | Blue, and the failure is logged rather than swallowed |
| A theme change while the explorer is open | The scene rebuilds; if it cannot, the UI says the explorer will update when reopened |

## Testing

**Go:** `SetTheme` accepts each of the three values and rejects an unknown one,
an empty one and a very long one; the value round-trips through
`saveConfig`/`loadConfig`; `GetTheme` returns Blue for a config with no theme
field.

**JS** (`node --test`): `normalizeTheme` for each valid id, an unknown string,
`''`, `null`, `undefined` and a non-string; `isValidTheme` agreeing with it; and
`THEMES` carrying exactly the three ids in display order.

**Visual regression, per migration pass:** build, pixel-diff against a build of
the previous commit, expect no difference. This is the gate that makes a
900-literal migration safe.

**Manual:** its own test-plan document — all three themes across every major
surface, native `<select>` popups and scrollbars in light mode, the theme
surviving a restart, no flash on launch, and an explorer open across a switch.

## Out of scope

- A custom or user-authored theme, and any colour picker.
- Following the OS theme automatically.
- The `?` help menu itself — this feature only positions the `⚙` where the pair
  will sit.
- Moving `⚙ Models` out of the notebook pane into the new popover, though the
  popover is designed to receive it later.
- Theming the CodeMirror editor's syntax highlighting beyond its chrome.
