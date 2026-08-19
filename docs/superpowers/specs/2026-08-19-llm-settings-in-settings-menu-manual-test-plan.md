# LLM settings in the settings menu — manual test plan (executed)

**Spec:** `2026-08-19-llm-settings-in-settings-menu.md`
**Design:** `2026-08-19-llm-settings-in-settings-menu-design.md`
**Task brief:** `.superpowers/sdd/2026-08-19-llm-settings-in-settings-menu/task-3-brief.md` (Task 3)
**Build:** `make build && ./build/bin/texere`, built at commit `96cf9b4`
**Run date:** 2026-08-19. Every case below was **executed**, not described.

## The claim under test

Tasks 1–2 moved the `⚙ Models` launcher out of the notebook pane and into a new
"LLM, Speech & Skills" section of the global settings popover, raised
`LLMSettings`' z-index from 40/41 to 200/201 so it can render above the
popover (60), and had `App.svelte` mount the overlay directly rather than the
notebook owning it. The automated tests added alongside that move
(`frontend/src/lib/settings-wiring.test.js`) are grep-shaped: they prove a
string is present or absent, not that the wiring works end to end. This run
is the real check, in particular of persistence — the overlay now has a
different parent and a different close path than it did before Task 1, and a
moved overlay with a new close path is exactly where persistence quietly
breaks without anything catching it.

## How this run was set up

- The collection under test, `keys-b`, is a **copy** of the user's real
  collection at `…/SStorytimeFork/SStorytime-main/examples/test`, made in an
  earlier session by `cp -a` into the job scratch directory, at a different
  path. The real collection was never opened by this run: the scratch
  `config.json`'s `collectionDir` names only the scratch path, confirmed by
  grepping the scratch `XDG_CONFIG_HOME` tree for the real collection's path
  and finding no match.
- `~/.config/texere/config.json` was **not touched**. The app was launched
  with `XDG_CONFIG_HOME` pointed at a scratch directory holding its own
  `config.json`, which `os.UserConfigDir` honours.
- No real API key was entered anywhere. The saved profile's key field holds
  the literal placeholder string `sk-test-placeholder`.
- The app was driven with synthetic X11 input (XTEST) via `xin.py`/`xcap.py`
  in the job scratch directory (no `xdotool`, no PIL on this box; `xcap.py`
  uses `XGetImage` with a stdlib PNG encoder). Every screenshot named below
  was read back as an image; the observations are what was read from them.
  Crops used to check small regions closely were made with the scratch
  directory's `pngcrop.py`.
- One quirk worth recording: the X server's keyboard layout is AZERTY, not
  QWERTY — sending the raw keycode for keysym `3` (or any digit) without a
  `Shift_L` modifier produces the unshifted AZERTY symbol (e.g. `"`) instead
  of a digit, and sending an uppercase-letter keysym without `Shift_L`
  produces the lowercase letter (both keysyms share one physical key, and
  `XTestFakeKeyEvent` sends the raw keycode's level-0 symbol). The profile
  name typed below (`verify-profile-checkpoint`) uses only lowercase letters
  and hyphens to sidestep this rather than fight it.

## Results

| # | Case | Result |
|---|---|---|
| 1 | Section renders beneath Appearance, not bolted on | **PASS** |
| 2 | Configure… opens the overlay; overlay above the popover | **PASS** |
| 3 | Overlay header agrees with the section label | **PASS** |
| 4 | All three tabs open | **PASS** |
| 5 | Save a profile, reopen — the check that matters | **PASS** |
| 6 | Closing works both ways (✕ and backdrop click) | **PASS** |
| 7 | All three themes (Blue, Dark, Light) | **PASS** |
| 8 | Notebook gear is gone; notebook still runs a cell | **PASS** |
| 9 | AskBox's own profile selector still lists profiles | **PASS** |

### 1. The section renders beneath Appearance — PASS

`shot: t3-01-settingsopen.png`. Clicking the gear (990, 360) opens a popover
with an `APPEARANCE` fieldset (Blue/Dark/Light radios) followed immediately
by an `LLM, SPEECH & SKILLS` fieldset holding a single `Configure…` button.
Same legend/label typography and spacing as Appearance — it reads as a
second section of the same menu, not a foreign control dropped in.

### 2. Configure… opens the overlay above the popover — PASS

`shot: t3-02-overlayopen.png`. Clicking `Configure…` opens the "LLM, Speech &
Skills" overlay; the settings popover is gone from view in the same
screenshot. This is *by design*, not a gap in the z-index check: `SettingsMenu`'s
click handler is `() => { llmSettingsOpen.set(true); close() }` (`SettingsMenu.svelte:78`)
— it closes its own popover synchronously in the same handler that opens the
overlay, so the two are never both open through the normal click path and
there is no frame in which they visibly overlap to photograph.

That makes the z-index ordering itself unobservable through a simple
side-by-side screenshot, so it was verified behaviourally instead: with the
overlay open, a click at the exact coordinates of the gear button (990, 360)
— which sits *behind* the full-viewport `.ov-backdrop` (`z-index: 200`) —
was sent (`shot: t3-03-zorder-click-gear.png`, taken after the click). Two
things confirm the backdrop won the hit-test rather than the gear button
underneath it: the overlay **closed** (the backdrop's own `on:click={onClose}`
fired), and the settings popover did **not** open (the gear's own `on:click={toggle}`
did not fire — if it had, both would be visible: an open popover and a closed
overlay showing through it). That is direct proof the overlay's stacking
context sits above the popover's layer at the coordinates in question, which
is what the 200/201 vs 60 z-index change was for. This is stronger than a
static screenshot in one sense (it proves interception of input, not just
paint order) but was reached indirectly because the primary UI path makes
literal simultaneous rendering impossible to trigger.

### 3. Overlay header agrees with the section label — PASS

Same screenshot as above (`t3-02-overlayopen.png`): the overlay's own header
reads "LLM, Speech & Skills", byte-identical to the popover's fieldset legend.

### 4. All three tabs open — PASS

- LLM Providers: `t3-04-overlay-reopened.png` — empty list, five presets
  (Anthropic, OpenAI, Google, OpenRouter, Local (Ollama)).
- Speech-to-text: `t3-15-stt-tab.png` — empty list, two presets (OpenAI
  Whisper, Local whisper).
- Skills: `t3-16-skills-tab.png` — two built-in skills listed (Socratic
  tutor, Thinking partner), each with a Duplicate button, plus the skills
  folder path and an "Open skills folder" button.

### 5. Save a profile, reopen — PASS (the check that matters)

This is Step 3.2 from the brief, run at three levels of increasing strength:

1. **Add + save.** Clicked the Anthropic preset (`t3-05-add-anthropic-form.png`,
   pre-filled `kind=anthropic`, `baseUrl=https://api.anthropic.com`,
   `model=claude-opus-4-8`), renamed it to `verify-profile-checkpoint`
   (`t3-08-name-final.png`) and set the API key field to the placeholder
   `sk-test-placeholder` (masked as 19 dots in `t3-09-apikey-typed.png`, the
   correct length for that string), then clicked Save.
   `t3-10-saved.png` shows the profile listed, marked active (filled radio),
   with Test/Edit/Delete controls — the save round-tripped through the Go
   backend and back into the list.
2. **Reopen in the same session.** Closed the overlay (✕), reopened it via
   gear → Configure… — `t3-12-reopened-insession.png` shows the same profile
   still listed and still active.
3. **Reopen after a full app restart.** `pkill`ed the process, confirmed no
   `build/bin/texere` process remained, relaunched with the same
   `XDG_CONFIG_HOME`, and reopened the overlay —
   `t3-14-reopen-after-restart.png` shows `verify-profile-checkpoint` still
   listed and still active. This is the strongest form of the check: it rules
   out an in-memory store that merely *looked* persisted because the Svelte
   component never remounted.
4. **On disk.** The scratch `config.json` (read directly, not through the
   UI) shows the full profile under `llm.profiles`, with `activeProfileId`
   pointing at it and the placeholder key stored verbatim — confirming the
   save actually reached the config file the new mount point reads from.

### 6. Closing works both ways — PASS

- `✕`: `t3-11-closed-via-x.png` — overlay gone, back to the collection view,
  clicked at (766, 81).
- Backdrop click: `t3-13-closed-via-backdrop.png` — clicked at (150, 600),
  well outside the modal's bounds, overlay closed the same way.

### 7. All three themes — PASS

- **Blue** (default): section and overlay shown above (`t3-01`, `t3-02`).
- **Dark**: switched via the Appearance radio (`t3-18-dark-main.png` — popover
  and section render correctly against the dark surface); overlay
  (`t3-19-dark-overlay.png`) shows the saved profile, fully legible. The
  modal surface color in Dark (`#0d0e0e`) is visually close to Blue's
  (`#0e1622`) — checked in `frontend/src/style.css` (lines 211–212 vs
  800–801) and confirmed **intentional**, not a missed token: a comment at
  `style.css:299` records that Dark was deliberately kept close to Blue for
  this surface. Not a defect.
- **Light**: switched the same way (`t3-20-light-main.png` — popover on a
  light background, section legible); overlay (`t3-21-light-overlay.png`)
  flips to a light modal (`#e8ebee` background, dark text per
  `style.css:1068-1069`), fully legible, saved profile still shown — also
  confirms the profile persisted across a theme change, not just a restart.

### 8. Notebook gear is gone; notebook still runs a cell — PASS

Opened Study → Process on an extract with an existing notebook
(`t3-24-process.png`). The notebook header shows `Notebook` and a `▸ Fold
all` button — **no `⚙ Models` button**, confirming Task 2's removal is live
in this build (source-verified too: `git show 66285c2` deletes the
`.nb-gear` button, its handler, its `showSettings` state, and the inline
`<LLMSettings>` mount from `NotebookPane.svelte`).

The notebook still functions: a pre-existing Shell cell (`echo shortcut4`,
left over from an earlier session against this same scratch collection) was
run via its Run button. `t3-25-cell-ran.png` shows the output block
`shortcut4` / `exit 0 · 0.0s` appended below the cell — the notebook cell
pipeline still executes.

*Caveat on this screenshot's provenance:* an earlier scratch-directory
screenshot, `task1-06-notebookgear.png` (timestamped 22:49, five minutes
**before** the 66285c2 commit at 22:55), still shows the `⚙ Models` button.
That file predates the removal and was not treated as evidence here — it was
cropped and inspected only to rule out a false alarm before re-verifying
against a fresh build of the current commit, which does not show the gear.

### 9. AskBox's own profile selector still lists profiles — PASS

Opened the OmniBar in graph mode with `Ctrl+Shift+K` (`t3-26-askbox-graphmode.png`
— `GRAPH` badge visible, AskBox's `Ask in plain language…` row rendered).
Per `AskBox.svelte`, this row renders `{#if canTranslate}` only when
`llmProfiles.length > 0` — its mere presence is already evidence the profile
list loaded a non-empty result. The adjacent `<select class="ask-prof">`
(`GetLLMConfig` on mount, `AskBox.svelte:40`) initially reads "Default
model"; native `<select>` popups don't render inside `xcap.py`'s captured
window on this WebKitGTK build (a known limitation from earlier sessions), so
rather than rely on the popup, focus was left on the closed control and a
native type-ahead `v` keypress was sent. `t3-30-typeahead-v.png` shows the
box's displayed value jump to `verify-profile-checkpoint` — the exact profile
saved in case 5, appearing correctly in a component this feature explicitly
left untouched as a per-feature override. This also incidentally
re-confirms persistence from a second, independent code path. Restored to
"Default model" (`d` type-ahead) and closed the OmniBar afterward.

## What this run does not cover

- **Delete / Edit of a profile.** Add-and-persist was exercised; the
  Edit/Delete buttons visible in `t3-10-saved.png` were not individually
  driven. Both are unchanged by this feature (no code in the Tasks 1–2 diff
  touches `LLMSettings.svelte`'s save/delete logic itself — only its host and
  z-index), so this is a low-risk gap, not an unknown.
- **Test button** (`t3-10-saved.png`) was not clicked — it would need a real
  endpoint or a network-failure path, and the task brief scopes this run to
  wiring/persistence, not provider connectivity.
- **Literal simultaneous popover+overlay screenshot.** As explained in case
  2, the normal UI path makes this impossible to produce (the popover closes
  itself the instant the overlay opens), so the z-index claim was verified
  by input-interception instead of by a side-by-side image. If a future
  regression reopens the popover without closing it (e.g. a bug reintroducing
  nesting), this run would not directly show what that looks like — only that
  today's implementation doesn't reach that state.
- **STT and Skills tab save flows.** Tab-open was confirmed for all three
  (case 4); a full add/save/reopen cycle was only run for the LLM Providers
  tab, since that is the tab the brief's Step 3.2 names explicitly and the
  save/reopen code path (`apiFor(tab).save` / `.get`) is shared across tabs.

## Cleanup

- The app was fully stopped (`pkill`, confirmed no process remained) after
  the run.
- The saved placeholder profile (`verify-profile-checkpoint`,
  `sk-test-placeholder`) was left in the scratch collection's config —
  harmless, scratch-only, no real credential.
- The scratch `XDG_CONFIG_HOME` tree, all screenshots, and the input/capture
  helpers live in the job scratch directory and are not part of the
  repository.
- The user's real collection at `…/SStorytimeFork/SStorytime-main/examples/test`
  and `~/.config/texere/config.json` were never opened or written by this
  run, per the setup notes above.
