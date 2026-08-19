# LLM settings move into the settings menu — design

**Date:** 2026-08-19
**Branch:** `054-llm-settings-in-settings-menu` (from `main`)
**Completes:** an item deferred by `2026-08-12-themes-design.md`

## The problem

LLM configuration lives behind a `⚙ Models` button inside the notebook pane
(`NotebookPane.svelte:110`). That made sense when the notebook was the only
thing that called an LLM. It no longer is — four surfaces now do:

| Surface | Use |
|---|---|
| `components/AskBox.svelte` | translates a natural-language query into a graph query, in the OmniBar |
| `components/LLMChatPane.svelte` | the chat in Study |
| `components/notebook/LLMCell.svelte` | notebook LLM cells |
| `components/notebook/TranscribeBar.svelte` | speech-to-text |

So to configure the model that translates a *graph search*, you must open a
notebook. That is the defect.

This was anticipated. The themes design named it twice: as the problem
motivating a global settings surface — *"There is no global settings surface.
The only one is `⚙ Models`"* (line 45) — and as explicitly deferred work —
*"Moving `⚙ Models` out of the notebook pane into the new popover, though the
popover is designed to receive it later"* (line 197). This feature is that
later.

## What moves, and what does not

`LLMSettings.svelte` is a 222-line overlay with three tabs — **LLM Providers**,
**Speech-to-text**, **Skills** — and full CRUD behind each: name, kind, base
URL, model, API key, env-var fallback, active-profile radios, add / edit /
delete. All three tabs move together as one unit; they are one surface today
and splitting them is not this feature's business.

`AskBox`'s query-profile selector (`GetQueryLLMProfile` / `SetQueryLLMProfile`)
**stays where it is.** It is a per-feature override of the global active
profile, not global config, and it belongs beside the feature it overrides.
Hoisting it into a global menu would misrepresent its scope.

## The constraint that shapes the design

**The settings popover cannot host the overlay.** `SettingsMenu.svelte`
dismisses on `pointerdown` anywhere outside its `root` element, and on Escape.
If a launcher mounted `LLMSettings` inside the popover's DOM, the first click
*within the overlay* would register as "outside" and close the popover — taking
the overlay with it.

So the overlay is mounted at `App.svelte` level and driven by a store, exactly
as `ShortcutsPanel` was in feature 053: `App.svelte` owns the mount,
`MiddleBand`/`SettingsMenu` owns the trigger, and neither can own the state.
`stores/workbench.js` already holds `logPanelOpen`, `omnibarMode` and
`shortcutsOpen` for this reason; `llmSettingsOpen` joins them.

## A latent bug this move exposes

`LLMSettings.svelte:172-173` sets its backdrop to `z-index: 40` and its modal to
`41`. The surfaces it will now share a stacking context with:

| Surface | z-index |
|---|---|
| `LLMSettings` backdrop / modal | **40 / 41** |
| `SettingsMenu` popover | 60 |
| `StudySurface` | 60 |
| `GraphExplorer` | 70 |
| `ShortcutsPanel` | 211 |

Mounted inside the notebook pane nothing competes, so 41 is invisible today.
Launched from the settings popover it would render **underneath the popover that
opened it**. Raising it above 70 is part of this change, not a follow-up.

## The change

1. **`stores/workbench.js`** — add `llmSettingsOpen`, a plain `writable(false)`,
   beside `shortcutsOpen`. It must be a real writable: a subscribe-only alias
   with no `update` is what made the toolbar's Search button blank the whole
   webview in feature 051.
2. **`SettingsMenu.svelte`** — a second `<fieldset class="sm-section">` with
   `<legend class="sm-legend">Models</legend>`, matching the existing Appearance
   section, containing a single launcher button. Clicking it opens the overlay
   and closes the popover.
3. **`App.svelte`** — mount `<LLMSettings>` on `$llmSettingsOpen`, beside the
   existing `ShortcutsPanel` mount.
4. **`LLMSettings.svelte`** — raise the backdrop and modal z-index above 70,
   and take `onClose` from the store rather than a prop if that reads more
   cleanly with the new owner.
5. **`NotebookPane.svelte`** — remove the import (line 8), the `showSettings`
   state (21), the `⚙ Models` button (110), the conditional mount (156), and the
   `.nb-gear` style (163) if nothing else uses it.

**Section naming.** The legend reads **Models**, matching today's button, even
though the overlay also holds Speech-to-text and Skills. Continuity beats
precision here; if the tab set grows again, rename then.

## Testing

The automated guards cannot help: nothing here is a colour, an icon or a
keyboard shortcut. What *is* mechanically checkable is thin and worth having
anyway — `NotebookPane.svelte` no longer references `LLMSettings`, and
`llmSettingsOpen` exists and is a writable. Both are text checks in
`frontend/src/lib/`, where `make test-js` will find them.

Everything real is manual, in **all three themes** (Blue, Dark, Light — feature
052 made the latter two real):

1. The **Models** section appears in the settings popover beneath Appearance.
2. Its launcher opens the overlay, and the overlay renders **above** the popover
   — the specific thing the z-index fix is for.
3. All three tabs still work, and a profile edit still saves and persists.
4. The notebook pane no longer shows `⚙ Models`, and the notebook still works.
5. `AskBox`'s own query-profile selector is untouched and still functions.

## The one real risk

Removing the notebook gear is a **user-visible regression for anyone with
muscle memory for it**, and a desktop app has no deprecation path — the button
is simply gone next launch. That is the accepted cost of a single entry point.
It is worth stating plainly rather than discovering in use.

## Out of scope

- Redesigning the popover into a full settings modal with inline sections. That
  is where this is eventually heading, and a launcher row does not block it.
- Splitting Speech-to-text or Skills out of the LLM overlay.
- Changing what any of the three tabs do.
