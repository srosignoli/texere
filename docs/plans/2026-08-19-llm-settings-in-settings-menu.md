# LLM Settings in the Settings Menu — Implementation Plan

**Goal:** LLM, speech and skills configuration is reachable from the global settings menu instead of a gear buried in the notebook pane.

**Architecture:** The existing `LLMSettings` overlay is unchanged in what it does; only its owner moves. A `llmSettingsOpen` store lets `SettingsMenu` trigger it while `App.svelte` mounts it — the same split `ShortcutsPanel` uses, and necessary because the popover dismisses on outside-click and would take a nested overlay with it.

**Tech Stack:** Svelte 3.49, Vite, `node:test` (stdlib). No new dependencies.

**Spec:** `docs/specs/2026-08-19-llm-settings-in-settings-menu-design.md`

## Global Constraints

- **`make test-js` runs `node --test frontend/src/lib/*.test.js`.** A test outside `frontend/src/lib/` ending in `.test.js` is silently never run.
- **The test runner cannot parse `.svelte`.** Components are checked by reading them as text.
- **Never introduce a literal colour.** `frontend/src/lib/theme-tokens.test.js` fails the suite on any hex, `rgb()`, `hsl()` or named colour outside its allowlist. Use existing `var(--token)` values; if none fits, add a token to **all three** theme blocks in `frontend/src/style.css`.
- **No emoji.** `frontend/src/lib/icons.test.js` fails on any astral-plane character. `⚙` (U+2699) and `✕` (U+2715) are BMP and fine.
- **`llmSettingsOpen` must be a real `writable`.** A subscribe-only alias with no `update`/`set` is what made the toolbar's Search button throw and blank the entire webview in feature 051.
- Do not open or modify the user's real collection at `/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`. Use a copy.

---

### Task 1: The new route

After this task the settings menu can open the overlay. The notebook gear still works too — two doors, both functional. Task 2 removes the old one, so nothing is unreachable in between.

**Files:**
- Modify: `frontend/src/stores/workbench.js`
- Modify: `frontend/src/components/LLMSettings.svelte:172-173` (z-index), `:96` (header text)
- Modify: `frontend/src/App.svelte`
- Modify: `frontend/src/components/SettingsMenu.svelte`

**Interfaces:**
- Produces, for Task 2: `export const llmSettingsOpen` — a `writable(false)` in `stores/workbench.js`.

- [ ] **Step 1: Add the store**

In `frontend/src/stores/workbench.js`, beside `shortcutsOpen` (line 161):

```js
/**
 * llmSettingsOpen — the LLM / speech / skills overlay.
 *
 * Lives here rather than in either component because neither can own it:
 * SettingsMenu triggers it, App.svelte mounts it. Mounting it inside the
 * popover is not an option — SettingsMenu dismisses on `pointerdown` anywhere
 * outside its root element, so the first click *inside* a nested overlay would
 * read as "outside" and close both.
 */
export const llmSettingsOpen = writable(false)
```

- [ ] **Step 2: Fix the z-index before anything can see it**

`frontend/src/components/LLMSettings.svelte:172-173` sets the backdrop to `z-index: 40` and the modal to `41`. Mounted inside the notebook pane nothing competes, so this looks fine today. Launched from the settings popover it would render **underneath the popover that opened it**.

Current stacking neighbours: `SettingsMenu` popover 60, `StudySurface` 60, `GraphExplorer` 70, `ShortcutsPanel` 211.

Raise the backdrop and modal above 70. Use values that leave room and do not collide with `ShortcutsPanel`'s 211 — e.g. 200 and 201. Add a brief comment saying what it must clear and why, so the next person moving a surface does not have to re-derive it.

- [ ] **Step 3: Mount it from App.svelte**

`LLMSettings` does not self-gate — it renders its backdrop and modal unconditionally, which is why `NotebookPane.svelte:156` wraps it in `{#if}`. Keep that shape.

Add the import beside the other component imports, add `llmSettingsOpen` to the existing store import on line 5, and mount it immediately after `<ShortcutsPanel />` (line 418):

```svelte
    {#if $llmSettingsOpen}
      <LLMSettings onClose={() => llmSettingsOpen.set(false)} />
    {/if}
```

`LLMSettings` already takes `onClose` as a prop with a default (`:9`), so it needs no change to work with a new owner.

Placement note: `<ShortcutsPanel />` sits inside the `{#if}` branch that renders the workbench, with `<EmptyState>` in the `{:else}`. `SettingsMenu` lives in `MiddleBand`, which is in that same branch — so the launcher and the mount are co-gated and neither can be reached without the other. Put the mount in that branch, not outside it.

- [ ] **Step 4: Add the launcher section**

In `frontend/src/components/SettingsMenu.svelte`, after the existing Appearance `<fieldset>` (which ends around line 71), add a second section using the same markup shape:

```svelte
      <fieldset class="sm-section">
        <legend class="sm-legend">LLM, Speech &amp; Skills</legend>
        <button
          type="button"
          class="sm-launch"
          on:click={() => { llmSettingsOpen.set(true); close() }}
        >Configure…</button>
      </fieldset>
```

`close()` is the component's existing dismiss function (`:13`); call it without arguments — the `refocus: true` form exists for keyboard dismissal and would pull focus back to the gear while the overlay is opening.

Import `llmSettingsOpen` from `../stores/workbench.js`.

Style `.sm-launch` with existing tokens only. `--control`, `--control-border`, `--text` and `--control-raised-hover` are all already used by comparable buttons; read the file's existing `.sm-choice` styles first and match their scale so the section does not look bolted on.

- [ ] **Step 5: Make the overlay's own header agree**

`LLMSettings.svelte:96` renders `<span>Models</span>` as the overlay title. The menu that opens it now says "LLM, Speech & Skills", and the overlay itself has three tabs. Change the header to match the section name so the two surfaces agree.

- [ ] **Step 6: Run**

Run: `make test-js`
Expected: PASS, including `theme-tokens.test.js` (no literal colour introduced) and `icons.test.js` (no emoji).

Run: `cd frontend && npx vite build`
Expected: succeeds.

- [ ] **Step 7: See it work**

Build and open a scratch collection:

```bash
make build
XDG_CONFIG_HOME=/tmp/claude-1000/-home-simone-projects-texere/45775468-79e5-4a04-b088-48b273096d37/scratchpad/xdg ./build/bin/texere
```

That scratch config points at a COPIED collection — never the user's real one. Capture with `python3 …/scratchpad/xcap.py texere <out.png>`, click with `…/xin.py click texere <x> <y>`. The gear is at roughly x=990, y=360.

Confirm, reading each screenshot back:
1. The popover shows **LLM, Speech & Skills** beneath Appearance.
2. Its button opens the overlay.
3. **The overlay renders above the popover** — this is what Step 2 exists for. If the popover is visible on top of, or through, the overlay, the z-index is still wrong.

- [ ] **Step 8: Commit**

```bash
git add frontend/src
git commit -m "feat(settings): reach LLM, speech and skills config from the settings menu"
```

---

### Task 2: Remove the old door

**Files:**
- Modify: `frontend/src/components/notebook/NotebookPane.svelte` — lines 8, 21, 110, 156, 163
- Create: `frontend/src/lib/settings-wiring.test.js`

- [ ] **Step 1: Write the failing tests**

These are thin by nature — the automated guards in this repo cover colours, icons and shortcuts, and none of those is what changed here. What *is* mechanically checkable is that the old route is gone and the new one is wired. Create `frontend/src/lib/settings-wiring.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { llmSettingsOpen } from '../stores/workbench.js'

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8')

// A subscribe-only alias with no `set` is what made the toolbar's Search button
// throw and blank the whole webview in feature 051. Both call sites use set().
test('llmSettingsOpen is a real writable', () => {
  assert.equal(typeof llmSettingsOpen.subscribe, 'function')
  assert.equal(typeof llmSettingsOpen.set, 'function')
  assert.equal(typeof llmSettingsOpen.update, 'function')
})

test('the notebook pane no longer owns LLM settings', () => {
  const src = read('../components/notebook/NotebookPane.svelte')
  assert.ok(!src.includes('LLMSettings'), 'NotebookPane still references LLMSettings')
  assert.ok(!src.includes('nb-gear'), 'NotebookPane still has the gear button or its style')
})

test('the settings menu opens it and App mounts it', () => {
  assert.ok(read('../components/SettingsMenu.svelte').includes('llmSettingsOpen'),
    'SettingsMenu does not reference llmSettingsOpen')
  const app = read('../App.svelte')
  assert.ok(app.includes('LLMSettings'), 'App.svelte does not mount LLMSettings')
  assert.ok(app.includes('llmSettingsOpen'), 'App.svelte does not read llmSettingsOpen')
})
```

- [ ] **Step 2: Run to verify the second test fails**

Run: `node --test frontend/src/lib/settings-wiring.test.js`
Expected: `the notebook pane no longer owns LLM settings` FAILS — Task 1 left the gear in place deliberately. The other two should already pass, since Task 1 wired them.

If either of the others fails, Task 1 is incomplete — fix that before removing anything.

- [ ] **Step 3: Remove the gear**

In `frontend/src/components/notebook/NotebookPane.svelte`, delete:
- line 8 — `import LLMSettings from '../LLMSettings.svelte'`
- line 21 — `let showSettings = false`
- line 110 — the `<button class="nb-gear" …>⚙ Models</button>`
- line 156 — `{#if showSettings}<LLMSettings … />{/if}`
- line 163 — the `.nb-gear` style rule. Verified: `nb-gear` appears exactly twice in `frontend/src`, both in this file (the button and this rule), so both go.

Match on content rather than line number — earlier deletions will shift the later ones.

- [ ] **Step 4: Run to verify they pass**

Run: `make test-js`
Expected: PASS, whole suite.

Run: `cd frontend && npx vite build`
Expected: succeeds. A leftover reference to a deleted import surfaces here.

- [ ] **Step 5: Prove the tests can fail**

1. Re-add `let showSettings = false` to `NotebookPane.svelte`.
   Expected: still passes — the test greps for `LLMSettings` and `nb-gear`, not that. This tells you the test's actual reach; note it in your report rather than widening the test.
2. Re-add the `nb-gear` button line.
   Expected: `the notebook pane no longer owns LLM settings` fails with "still has the gear button or its style".
3. Remove `llmSettingsOpen` from `SettingsMenu.svelte`'s import.
   Expected: `the settings menu opens it and App mounts it` fails.

Verify each sabotage actually applied before trusting its result — a substitution that silently fails to match produces a green run that looks like a passing control. Revert each and record the output.

- [ ] **Step 6: Commit**

```bash
git add frontend/src
git commit -m "refactor(notebook): the notebook no longer owns LLM settings"
```

---

### Task 3: Verification

**Files:**
- Create: `docs/specs/2026-08-19-llm-settings-in-settings-menu-manual-test-plan.md`

Nothing above proves the overlay still *works* from its new owner. The tests check wiring exists, not that a profile saves.

- [ ] **Step 1: Set up**

`make build`, then run `./build/bin/texere` with `XDG_CONFIG_HOME` pointed at the scratch config as in Task 1 Step 7 — a COPIED collection, never the user's real one. Capture and input helpers are in that same scratchpad directory (`xcap.py`, `xin.py`; modifiers need real keysym names such as `Control_L`).

- [ ] **Step 2: Verify in all three themes**

Blue, Dark and Light — feature 052 made the latter two real, and a new section is exactly the kind of surface built against one theme and checked against none. Switch via the gear's Appearance section. Read every screenshot back and describe what you saw.

1. The **LLM, Speech & Skills** section renders beneath Appearance, and does not look bolted on.
2. Its button opens the overlay, and the overlay renders **above** the popover.
3. The overlay's own header reads "LLM, Speech & Skills", agreeing with the section.

- [ ] **Step 3: Verify the overlay still functions**

The move must not have broken what it moved:
1. All three tabs — **LLM Providers**, **Speech-to-text**, **Skills** — still open.
2. Add or edit a profile and save it. Reopen the overlay and confirm it persisted. This is the check that matters: the overlay now has a different parent and a different close path.
3. Closing works both ways it can — the `✕` and the backdrop click.

- [ ] **Step 4: Verify nothing else regressed**

1. The notebook pane no longer shows `⚙ Models`, and the notebook still opens and runs a cell.
2. `AskBox`'s own query-profile selector (OmniBar graph mode, `Ctrl+Shift+K`) is untouched and still lists profiles. It was deliberately left in place as a per-feature override.

- [ ] **Step 5: Write up the result**

Every case with a pass/fail and a one-line observation, in the shape of `docs/specs/2026-08-17-relative-identity-keys-manual-test-plan.md`. Record anything you could not reach. An honest "not verified" is worth more than a claimed pass.

- [ ] **Step 6: Commit**

```bash
git add docs/specs/2026-08-19-llm-settings-in-settings-menu-manual-test-plan.md
git commit -m "docs(test-plan): LLM settings in the settings menu"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| `llmSettingsOpen` store, real writable | 1 (added), 2 (asserted) |
| Popover cannot host the overlay → App owns the mount | 1 |
| z-index raised above 70 | 1 (fixed), 3 (verified visually) |
| `LLM, Speech & Skills` section with launcher | 1 |
| Overlay header agrees with the section | 1 |
| Notebook gear removed | 2 |
| All three tabs move as a unit | not split anywhere |
| `AskBox` query-profile selector untouched | 3 (verified), not modified |
| Manual verification in all three themes | 3 |

**Type consistency:** `llmSettingsOpen` is a `writable(false)`, imported from `../stores/workbench.js` by `SettingsMenu.svelte` and `./stores/workbench.js` by `App.svelte` (different relative depths — check both). `LLMSettings`'s `onClose` prop is unchanged.

**Verified before writing, not assumed:** `frontend/src/stores/workbench.js` imports only `svelte/store` and IS importable under `node --test` — checked by running a throwaway test that imported `shortcutsOpen` and asserted its methods, so Task 2's test file will load rather than failing at import. `SettingsMenu`'s outside-click dismissal (`pointerdown`, `:31`) is what rules out nesting; `LLMSettings` has no internal gate, so the `{#if}` wrapper is required; `LLMSettings` z-index 40/41 sits below the popover's 60 and the explorer's 70; `LLMSettings` takes `onClose` as a defaulted prop, so it needs no change to accept a new owner; `ShortcutsPanel` is mounted inside the workbench branch and `SettingsMenu` lives in `MiddleBand` within that same branch, so launcher and mount are co-gated.

**Known weakness this plan accepts:** Task 2's tests are grep-shaped and shallow — they prove a string is absent and two strings are present, not that the wiring works. Step 5's first control makes that explicit rather than hiding it. The real verification is Task 3, and specifically the save-and-reopen check, because a moved overlay with a new parent and a new close path is exactly where persistence quietly breaks.
