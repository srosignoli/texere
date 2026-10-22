# Complete Dark and Light Themes Implementation Plan

**Goal:** Dark and Light become selectable themes that paint the whole app correctly, and stay that way.

**Architecture:** Replace literal colours in component CSS with the existing `var(--token)` vocabulary, enforced by a ratcheting guard test that starts permissive and tightens file by file. The token sets already exist and are symmetric — `:root` declares 148 custom properties, and `[data-theme="dark"]` and `[data-theme="light"]` each override the same 132 — so this is substitution against a known vocabulary, not colour design.

**Tech Stack:** Svelte 3.49, Vite, `node:test` (stdlib). No new dependencies.

**Spec:** `docs/specs/2026-08-18-complete-themes-design.md`

## Global Constraints

- **`make test-js` runs `node --test frontend/src/lib/*.test.js`.** A test outside `frontend/src/lib/` ending in `.test.js` is silently never run.
- **The test runner cannot parse `.svelte`.** Components are checked by reading them as text, the way `n4l-syntax.test.js` reads `style.css`.
- **Never introduce a new literal colour.** Every colour added by this work is `var(--token)`. If no token fits, add one to all three theme blocks in `style.css` rather than inlining a value.
- **The counts below are measured, not estimated.** After stripping comments and exempting shadow declarations: **201 colours across 36 files**. The spec's "~236 across 40" was a raw regex count including comment false positives — the spec predicted this, and the guard's comment-stripping is what removes them.
- **Do not convert the 16 `--explorer-*` data colours to per-theme values.** They encode data (edge kind, node role, territory identity), not chrome, and are deliberately shared. Task 7 decides from screenshots whether any needs a light override.
- Do not open or modify the user's real collection at `/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`. Use a copy, and launch with `XDG_CONFIG_HOME` pointed at a scratch config.

---

### Task 1: The guard, as a ratchet

Build the instrument first, and make it pass immediately by listing every unconverted file as pending. Each later task deletes entries from that list. The suite is green at every commit and the list is a visible worklist.

**Files:**
- Create: `frontend/src/lib/theme-tokens.test.js`

**Interfaces:**
- Produces, for Tasks 2-6: a `PENDING` array of repo-relative paths (from `frontend/src/`). Removing a path from `PENDING` makes the guard enforce zero literal colours in that file.

- [ ] **Step 1: Write the test**

Create `frontend/src/lib/theme-tokens.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'

const SRC = new URL('../', import.meta.url)

// Comments are stripped before scanning. n4l-escape.js:96 contains the prose
// "incident #4312 - prod", which is not a colour; without this the guard would
// demand its conversion forever.
function stripComments(s) {
  return s
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/[^\n]*/g, '$1')
}

const COLOUR = /(#[0-9a-fA-F]{3,8}\b|\brgba?\([^)]*\))/g

// A colour inside a shadow declaration is a depth cue, not a surface: it stays
// dark in every theme. n4l-editor.js states the rule already. Both spellings
// are matched — CSS uses box-shadow, EditorView.theme({}) uses boxShadow — and
// a quoted value is consumed whole so a match cannot run past it into a second
// declaration on the same line.
const SHADOW = /(?:box-shadow|boxShadow|text-shadow|textShadow|filter)\s*:\s*(?:'[^']*'|"[^"]*"|[^;\n]*)/gi

// Colours exempt for a reason other than being a shadow, as file -> literals.
// Every entry needs a comment saying why. An entry without one is unconverted
// work wearing a disguise.
const ALLOW = {
  // Fallbacks for when getComputedStyle returns nothing (a test environment
  // with no real document). Documented at the top of Theme.js. The 0x… values
  // beside them are Three.js hex integers, not CSS colours, and COLOUR does not
  // match them — which is correct, they are not styling anything.
  'components/explorer/Theme.js': ['#e8eaf0', '#0e0f12', 'rgba(200, 214, 230, 0.7)'],
}

// Files not yet converted. Shrinks to nothing across Tasks 2-6; when it is
// empty, Task 6 deletes it and the constant below.
const PENDING = [
  'App.svelte',
  'components/ArrowPalette.svelte',
  'components/AskBox.svelte',
  'components/ColumnContextTags.svelte',
  'components/ColumnNodes.svelte',
  'components/EmptyState.svelte',
  'components/ExplorerLegend.svelte',
  'components/ExplorerSidebar.svelte',
  'components/ExplorerStats.svelte',
  'components/GraphExplorer.svelte',
  'components/LLMSettings.svelte',
  'components/LogPanel.svelte',
  'components/MiddleBand.svelte',
  'components/MiniMap.svelte',
  'components/NodeCard.svelte',
  'components/OmniBar.svelte',
  'components/QueryAutocomplete.svelte',
  'components/QueryHelpPanel.svelte',
  'components/ReadTab.svelte',
  'components/ReminderPanel.svelte',
  'components/SettingsMenu.svelte',
  'components/SourceTab.svelte',
  'components/SourcesSidebar.svelte',
  'components/Splitter.svelte',
  'components/StatsPanel.svelte',
  'components/explorer/EditCard.svelte',
  'components/explorer/NodeMeshes.js',
  'components/notebook/ImageCell.svelte',
  'components/notebook/LLMCell.svelte',
  'components/notebook/MediaSearchPanel.svelte',
  'components/notebook/ShellCell.svelte',
  'components/notebook/SketchCell.svelte',
  'components/notebook/media/MediaImageView.svelte',
  'components/notebook/media/MediaVideoView.svelte',
  'lib/bake.js',
  'lib/sketch-render.js',
  'lib/stats-charts.js',
]

function sourceFiles() {
  return readdirSync(SRC, { recursive: true })
    .map(String)
    .filter((f) => (f.endsWith('.svelte') || f.endsWith('.js')) && !f.endsWith('.test.js'))
}

function offenders(file) {
  const src = stripComments(readFileSync(new URL(file, SRC), 'utf8'))
  const shadows = [...src.matchAll(SHADOW)].map((m) => [m.index, m.index + m[0].length])
  const allowed = ALLOW[file] ?? []
  return [...src.matchAll(COLOUR)]
    .filter((m) => !shadows.some(([a, b]) => m.index >= a && m.index < b))
    .filter((m) => !allowed.includes(m[0]))
    .map((m) => m[0])
}

test('no literal colours outside the pending list', () => {
  const files = sourceFiles()
  // A file-walking test that stops walking passes forever while checking
  // nothing. Feature 050 demonstrated exactly that; this is the guard against it.
  assert.ok(files.length > 50, `scanned only ${files.length} files; the walk is broken`)

  const bad = []
  for (const f of files) {
    if (PENDING.includes(f)) continue
    const found = offenders(f)
    if (found.length) bad.push(`${f}: ${found.join(', ')}`)
  }
  assert.deepEqual(bad, [], `literal colours must be var(--token):\n${bad.join('\n')}`)
})

// The pending list is a worklist, not a dumping ground: an entry that no longer
// has any literal colour is stale and must be removed, or the list stops
// describing what is left to do.
test('every pending file still has something pending', () => {
  for (const f of PENDING) {
    assert.ok(offenders(f).length > 0, `${f} is clean — remove it from PENDING`)
  }
})

// Proves the comment-stripping works, so the #4312 false positive cannot come back.
test('a colour-shaped string in a comment is not a colour', () => {
  const src = stripComments(`// incident #4312 - prod\n<!-- #abcdef -->\n/* #123456 */\nx: #fedcba;`)
  assert.deepEqual(src.match(COLOUR), ['#fedcba'])
})

// Proves the shadow exemption covers both spellings.
test('shadow declarations are exempt in CSS and in JS', () => {
  const css = 'box-shadow: 0 6px 24px rgba(0,0,0,0.55); color: #ff0000;'
  const js = "boxShadow: '0 6px 24px rgba(0,0,0,0.55)', color: '#ff0000'"
  for (const src of [css, js]) {
    const shadows = [...src.matchAll(SHADOW)].map((m) => [m.index, m.index + m[0].length])
    const left = [...src.matchAll(COLOUR)]
      .filter((m) => !shadows.some(([a, b]) => m.index >= a && m.index < b))
      .map((m) => m[0])
    assert.deepEqual(left, ['#ff0000'], `shadow exemption wrong for: ${src}`)
  }
})
```

- [ ] **Step 2: Run it**

Run: `make test-js`
Expected: PASS, 4 new tests. Every file with a literal colour is pending, so the first test has nothing to complain about; the second confirms no pending entry is already clean.

- [ ] **Step 3: Prove each test can fail**

1. Remove `'components/LogPanel.svelte'` from `PENDING`.
   Expected: `no literal colours outside the pending list` fails, listing LogPanel's 13 colours.
2. Add `'components/Icon.svelte'` to `PENDING` (it has no colours).
   Expected: `every pending file still has something pending` fails with "Icon.svelte is clean — remove it from PENDING".
3. Delete the `.replace(/(^|[^:])\/\/[^\n]*/g, '$1')` line from `stripComments`.
   Expected: `a colour-shaped string in a comment is not a colour` fails, having matched `#4312`.
4. Change `SHADOW` to match only `box-shadow` (drop the `boxShadow` alternative).
   Expected: `shadow declarations are exempt in CSS and in JS` fails on the JS case.
5. Change the `files.length > 50` assertion to `>= 0` and point `SRC` at an empty directory.
   Expected: the first test PASSES while scanning nothing — the reason that assertion exists.

Revert each, and record the captured output.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/theme-tokens.test.js
git commit -m "test(themes): ratcheting guard against literal colours"
```

---

### Task 2: The explorer cluster

The largest concentration: 86 of the 201 colours, in 8 files. All are DOM chrome — the 3D scene already resolves colour from CSS variables at call time.

**Files:**
- Modify: `frontend/src/components/GraphExplorer.svelte` (32)
- Modify: `frontend/src/components/ExplorerSidebar.svelte` (26)
- Modify: `frontend/src/components/MiniMap.svelte` (11)
- Modify: `frontend/src/components/ExplorerStats.svelte` (7)
- Modify: `frontend/src/components/ExplorerLegend.svelte` (3)
- Modify: `frontend/src/components/explorer/Theme.js` (3 — allowlisted, see below)
- Modify: `frontend/src/components/explorer/EditCard.svelte` (2)
- Modify: `frontend/src/components/explorer/NodeMeshes.js` (2)
- Modify: `frontend/src/lib/theme-tokens.test.js` (remove these from `PENDING`)

- [ ] **Step 1: Convert the DOM chrome**

For each `.svelte` file above, replace every literal in its `<style>` block with the token whose role matches. The vocabulary is in `frontend/src/style.css` under `:root`. The common mappings, verified against the current Blue values:

| Literal seen | Role | Token |
|---|---|---|
| `#e8eaf0` | primary text | `var(--text)` |
| `#a8c4ff` | accent-coloured text | `var(--accent-text)` |
| `#4a7bd0` | accent border / fill | `var(--accent)` |
| `rgba(12, 14, 20, 0.9)` | floating panel behind content | `var(--surface-overlay)` |
| `rgba(20, 24, 32, 0.92)` | raised panel | `var(--surface-raised)` |

Do not guess from this table alone — read the surrounding rule and pick by role. Where a literal has no matching token, add one to **all three** theme blocks in `style.css` (`:root`, `[data-theme="dark"]`, `[data-theme="light"]`) rather than inlining it, and say in your report which tokens you added and why.

`rgba(0, 0, 0, 0.6)` used as a scrim over content is the one case to think twice about: if it exists to keep text readable over arbitrary content beneath, it stays dark in every theme and belongs in `ALLOW` with that reason. If it is merely a panel background, it converts.

- [ ] **Step 2: Fix NodeMeshes.js**

`components/explorer/NodeMeshes.js` hardcodes two 3D colours:

```js
const CYCLE_GLYPH_COLOR = '#f4b545'      // amber
const CYCLE_GLYPH_HALO = '#1a1306'
```

`Theme.js` documents itself as the single source of truth for explorer colour and resolves every value at call time; these two bypass it, which makes that claim untrue today. Move them behind resolvers in `Theme.js` in the same style as its existing ones — reading a CSS variable at call time, with a hardcoded fallback — and add `--explorer-cycle-glyph` and `--explorer-cycle-halo` to `:root` in `style.css`.

Treat them as data colours: declare them in `:root` only, alongside the other `--explorer-*` entries, not per theme.

Their fallbacks then belong in `ALLOW` under `Theme.js` for the same reason as the existing three.

- [ ] **Step 3: Update the guard**

Delete these 8 paths from `PENDING`. Add the two new `Theme.js` fallbacks to its `ALLOW` entry.

- [ ] **Step 4: Run**

Run: `make test-js`
Expected: PASS. The guard now enforces zero literals in all 8 files.

Run: `cd frontend && npx vite build`
Expected: succeeds.

- [ ] **Step 5: Look at it**

Build and open the explorer on a scratch collection in **all three themes**. The 3D scene, the sidebar, the legend, the minimap and the stats panel must each be readable. Screenshot each theme and read the screenshots back.

This is the first task where a wrong token becomes visible, and the explorer is the densest surface in the app — do not skip to the commit.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components frontend/src/style.css frontend/src/lib/theme-tokens.test.js
git commit -m "feat(themes): tokenise the graph explorer's DOM chrome"
```

---

### Task 3: The sketch trio

Small in count, largest in consequence: one of these files writes a persisted artifact.

**Files:**
- Modify: `frontend/src/components/notebook/SketchCell.svelte`
- Modify: `frontend/src/lib/sketch-render.js`
- Modify: `frontend/src/lib/bake.js`
- Modify: `frontend/src/lib/theme-tokens.test.js`

- [ ] **Step 1: Make the default pen theme-aware**

`SketchCell.svelte` holds:

```js
const COLORS = ['#e8eaf0', '#f59e0b', '#7dd3fc', '#ff6b6b', '#10b981']
```

`#e8eaf0` is near-white: correct ink on a dark canvas, invisible on a light one. Only the first swatch changes. Resolve it from a new `--sketch-ink` token (near-white in Blue and Dark, near-black in Light) read at call time, so switching theme changes the pen a new stroke will use.

The four accents stay literal and go in `ALLOW` with the reason: they read on both backgrounds, and per-theme accents would make a saved sketch look different from how it was drawn.

- [ ] **Step 2: Make the stroke fallback agree**

`lib/sketch-render.js` renders a stroke with no colour as `s.color ?? '#e8eaf0'`. That is the same decision in a second place. Use the same `--sketch-ink` resolution, so a legacy stroke without a stored colour draws in the current theme's ink rather than always near-white.

- [ ] **Step 3: Decide what baking does**

`lib/bake.js` sets `SKETCH_BG = '#0b1220'`, commented "dark bg preserves contrast for light strokes".

Baking writes a file. If a sketch is drawn in Light (dark ink) and baked onto a dark background, the ink vanishes in the saved image — permanently, and after the fact.

Bake onto the background the strokes were drawn against: resolve the surface from the theme at bake time, the same way the ink is resolved. A sketch baked in Light gets a light background and its dark strokes stay visible.

State in your report what you chose and why. This is the one decision in this plan that can silently destroy user content, so it deserves a sentence of its own.

- [ ] **Step 4: Update the guard and run**

Remove the three files from `PENDING`; add the four accent literals to `ALLOW` under `SketchCell.svelte`.

Run: `make test-js` — expected PASS.

- [ ] **Step 5: Verify by drawing**

In a scratch collection, open a notebook, add a Sketch cell and draw in **Light** and in **Dark**. In each: the default pen must be visible against the canvas. Then bake in each and open the baked image — the strokes must still be visible in the saved file. Screenshot all four states and read them back.

A test cannot catch invisible ink. Only drawing can.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/notebook/SketchCell.svelte frontend/src/lib frontend/src/style.css
git commit -m "feat(themes): the sketch pen and its baked background follow the theme"
```

---

### Task 4: Stats and logs

26 colours across three files, and the only place where chrome and data sit side by side.

**Files:**
- Modify: `frontend/src/components/LogPanel.svelte` (13)
- Modify: `frontend/src/components/StatsPanel.svelte` (10)
- Modify: `frontend/src/lib/stats-charts.js` (3)
- Modify: `frontend/src/lib/theme-tokens.test.js`

- [ ] **Step 1: Split stats-charts.js by kind**

```js
const TICK = '#8899aa'
const GRID = '#2a3a4a'
function config(type, { labels, values, yLabel, unit, color = '#7ecfa0' }) {
```

`TICK` and `GRID` are chart chrome — axis labels and gridlines. They must follow the theme: on a light background a `#2a3a4a` gridline is nearly black, and `#8899aa` tick text is low contrast. Convert both to tokens read at call time (the charts are drawn, not styled, so they need the same `getComputedStyle` approach `Theme.js` uses).

The default series colour `#7ecfa0` is **data**. It identifies a series. It stays literal and goes in `ALLOW` with that reason.

- [ ] **Step 2: Convert LogPanel and StatsPanel chrome**

Same substitution as Task 2. Log severity colours (if any read as red/amber/green for error/warn/ok) map to `--danger`, `--warn` and `--ok`, which already exist in all three themes — do not invent new tokens for them.

- [ ] **Step 3: Update the guard and run**

Remove the three files from `PENDING`; add `#7ecfa0` to `ALLOW` under `stats-charts.js`.

Run: `make test-js` — expected PASS.
Run: `cd frontend && npx vite build` — expected success.

- [ ] **Step 4: Look at it**

Open Study → Stats in Light and Dark. Axis labels, gridlines and series must all be legible in both. Screenshot each and read them back. Charts are drawn to a canvas, so a missed colour here does not merely look wrong — it can render an axis invisible.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/LogPanel.svelte frontend/src/components/StatsPanel.svelte frontend/src/lib
git commit -m "feat(themes): chart and log chrome follow the theme; series colours stay data"
```

---

### Task 5: The long tail

The remaining 24 files, 82 colours, 1–20 each. Mechanical, but it is most of the app's surface area.

**Files:**
- Modify: the 24 paths still in `PENDING` after Tasks 2-4 — `OmniBar.svelte` (19), `App.svelte` (9), `SourcesSidebar.svelte` (6), `ColumnContextTags.svelte`, `ColumnNodes.svelte`, `QueryHelpPanel.svelte`, `SourceTab.svelte`, `notebook/MediaSearchPanel.svelte` (5 each), `NodeCard.svelte` (4), `ArrowPalette.svelte`, `LLMSettings.svelte`, `MiddleBand.svelte` (3 each), `QueryAutocomplete.svelte`, `ReminderPanel.svelte`, `notebook/ImageCell.svelte`, `notebook/media/MediaImageView.svelte` (2 each), and `AskBox.svelte`, `EmptyState.svelte`, `ReadTab.svelte`, `SettingsMenu.svelte`, `Splitter.svelte`, `notebook/LLMCell.svelte`, `notebook/ShellCell.svelte`, `notebook/media/MediaVideoView.svelte` (1 each)
- Modify: `frontend/src/lib/theme-tokens.test.js`

- [ ] **Step 1: Convert, largest first**

Work down by count. Commit after `OmniBar.svelte` and `App.svelte` individually — they are big enough that a bad token in either is worth isolating in history — then the rest together.

Same rule throughout: pick by role, not by matching the old hex. Where nothing fits, add a token to all three theme blocks and report it.

- [ ] **Step 2: Empty the pending list**

`PENDING` should now be `[]`.

- [ ] **Step 3: Run**

Run: `make test-js`
Expected: `every pending file still has something pending` passes trivially over an empty list, and `no literal colours outside the pending list` now enforces the whole tree.

Run: `cd frontend && npx vite build` — expected success.

- [ ] **Step 4: Prove the guard now covers everything**

Add `color: #ff0000;` to any component's `<style>` block.
Expected: the guard fails naming that file and `#ff0000`. This is the moment the ratchet becomes a real constraint rather than a bookkeeping list — confirm it.

Revert, and record the output.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(themes): tokenise the remaining components"
```

---

### Task 6: Flip the switches

**Files:**
- Modify: `frontend/src/lib/themes.js`
- Modify: `frontend/src/components/SettingsMenu.svelte:70`
- Modify: `frontend/src/lib/theme-tokens.test.js`

- [ ] **Step 1: Make both themes available**

In `themes.js`:

```js
export const THEMES = Object.freeze([
  Object.freeze({ id: 'blue', label: 'Blue', available: true }),
  Object.freeze({ id: 'dark', label: 'Dark', available: true }),
  Object.freeze({ id: 'light', label: 'Light', available: true }),
])
```

Rewrite the comment above it. It currently explains why Dark and Light are gated and names blockers that no longer exist — CodeMirror was finished by feature 047, and the explorer's 3D scene resolved colour from CSS variables all along. Leaving stale reasoning in place is how the next person inherits a wrong mental model. Say instead that `available` remains as the mechanism for gating a future theme, and that all three ship today.

If `available` now has no false values anywhere, say so in your report and ask before deleting the field — `AVAILABLE_THEMES` is exported and may have other consumers.

- [ ] **Step 2: Delete the note**

Remove `<p class="sm-note">Dark and Light are in progress.</p>` from `SettingsMenu.svelte:70`, and its `.sm-note` style if nothing else uses it.

- [ ] **Step 3: Delete the ratchet scaffolding**

`PENDING` is empty. Delete the constant, the `PENDING.includes(f)` skip, and the `every pending file still has something pending` test. Keep the guard itself, the allowlist, and the three regex tests — those are permanent.

- [ ] **Step 4: Run**

Run: `make test-js`
Expected: PASS.

Run existing theme tests — `themes.js` has coverage; confirm nothing there asserted `available: false`:

```bash
node --test frontend/src/lib/*.test.js
```

Expected: PASS. If a test asserted Dark was unavailable, it was encoding the gate rather than the behaviour — update it and say which.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/themes.js frontend/src/components/SettingsMenu.svelte frontend/src/lib/theme-tokens.test.js
git commit -m "feat(themes): Dark and Light are selectable"
```

---

### Task 7: Verification in both themes

**Files:**
- Create: `docs/specs/2026-08-18-complete-themes-manual-test-plan.md`

Nothing above proves the app is *usable* in Light. The guard proves no literals remain; it cannot prove the right token was chosen. `--text-muted` where `--text-secondary` belonged passes every test and looks wrong forever.

- [ ] **Step 1: Set up**

Build, and open a **copy** of the real collection with `XDG_CONFIG_HOME` pointed at a scratch config. Feature 049's manual test plan describes the `XGetImage` capture and XTEST input drivers; rebuild them if they are not still in the scratch directory. Note that XTEST modifiers need real keysym names — `Control_L`, not `ctrl`.

- [ ] **Step 2: Capture every surface in Dark and Light**

Both themes, each screenshot read back:

1. Toolbar and the three columns
2. The note editor, including syntax highlighting
3. Study — Read, Process, Recall, Stats
4. Graph explorer — 3D scene, DOM overlay, sidebar, legend, minimap
5. Notebook — the cell list and at least one of each of Sketch, Compute and Media
6. The Appearance menu, showing three selectable themes
7. A native `<select>` popup and a scrollbar — the 2026-08-12 test plan flags both as light-mode hazards

- [ ] **Step 3: Switch themes with the explorer open**

Open the explorer, then change theme without closing it. The scene must repaint. `Theme.js` resolves colour at call time specifically so this works, and its comment records that eagerly-evaluated constants once pinned the palette to whichever theme loaded first. Confirm the regression has not returned.

- [ ] **Step 4: Decide the explorer data colours**

Look at the territory and edge colours on the light background. They are shared across themes by design. If any lacks contrast on light, add a `[data-theme="light"]` override for **that colour only** and say which and why. If they all read fine, say that — and change nothing.

- [ ] **Step 5: Write up the result**

Every case with a pass/fail and a one-line observation, in the shape of `docs/specs/2026-08-17-relative-identity-keys-manual-test-plan.md`. Record every token you added, and any surface you could not reach.

- [ ] **Step 6: Commit**

```bash
git add docs/specs/2026-08-18-complete-themes-manual-test-plan.md
git commit -m "docs(test-plan): complete Dark and Light themes"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Convert ~200 literals to tokens | 2, 3, 4, 5 |
| Allowlist: shadows | 1 (rule-based) |
| Allowlist: documented fallbacks | 1, 2 |
| Allowlist: data-encoding colours | 3 (pen accents), 4 (series colour) |
| Guard test, with vacuity assertion | 1 |
| Guard ignores comments (`#4312`) | 1 |
| SketchCell theme-aware default pen | 3 |
| `sketch-render.js` / `bake.js` agree with it | 3 |
| `NodeMeshes.js` moved behind `Theme.js` | 2 |
| `stats-charts.js` split chrome vs data | 4 |
| Explorer data colours decided from evidence | 7 |
| Flip both to `available: true` | 6 |
| Delete the "in progress" note | 6 |
| Both themes, every major surface | 7 |
| Live theme switch with explorer open | 7 |
| `<select>` popups and scrollbars | 7 |
| Out of scope: OS following, custom themes | not implemented anywhere |

**Type consistency:** `PENDING` (array of `string`), `ALLOW` (object mapping `string` to `string[]`), `offenders(file) -> string[]`, `sourceFiles() -> string[]`, `stripComments(s) -> string` and the `COLOUR` / `SHADOW` regexes are introduced in Task 1 and referenced with those exact names in Tasks 2-6.

**Corrected during self-review:** the `ALLOW` entry for `Theme.js` first listed three invented hex values. The real fallbacks are `#e8eaf0`, `#0e0f12` and `rgba(200, 214, 230, 0.7)`. Had that shipped, Task 1 would have failed on its first run with three phantom allowances and three unexplained offenders — so check any literal this plan quotes against the file before trusting it.

**Verified before writing, not assumed:** the guard's regexes were prototyped against the real tree. Comment-stripping takes 236 raw matches down to 220 and eliminates the `n4l-escape.js:96` false positive; the shadow exemption takes it to **201 across 36 files**, and correctly drops `n4l-editor.js` to zero once `SHADOW` matches JS camelCase `boxShadow` as well as CSS `box-shadow`. The kebab-only version left `n4l-editor.js` at 1 — an error this plan would otherwise have shipped.

**Known weakness this plan accepts:** the guard cannot distinguish a right token from a wrong one, and roughly 200 substitutions are being made by judgement. Task 7 is the only thing that catches a wrong choice, and it catches it by someone looking at a screenshot. The tasks are ordered so the densest and most consequential surfaces (explorer, sketch, charts) are looked at in their own tasks rather than swept into the long tail, because those are where a wrong token does the most damage.
