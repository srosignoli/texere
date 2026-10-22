# Keyboard Shortcuts Help Panel Implementation Plan

**Goal:** A `?` panel listing every keyboard shortcut worth memorising, kept honest by a test that fails when the panel and the source disagree.

**Architecture:** A pure data module (`lib/shortcuts.js`) rendered by a panel component, plus a guard test that scans the source in both directions — no phantom entries, no undocumented bindings. The split follows the line this repo already tests on: `make test-js` runs `node:test` over `frontend/src/lib/*.test.js`, and there is no Svelte component harness.

**Tech Stack:** Svelte 3.49, Vite, `node:test` (stdlib). No new dependencies.

**Spec:** `docs/specs/2026-08-19-shortcuts-help-panel-design.md`

## Global Constraints

- **`make test-js` runs `node --test frontend/src/lib/*.test.js`.** A test outside `frontend/src/lib/` ending in `.test.js` is silently never run.
- **The test runner cannot parse `.svelte`.** Components are checked by reading them as text.
- **Never introduce a literal colour.** `frontend/src/lib/theme-tokens.test.js` fails the suite on any hex, `rgb()`, `hsl()` or named colour outside its allowlist. The panel must use existing `var(--token)` values from `frontend/src/style.css`; if none fits, add a token to **all three** theme blocks (`:root`, `[data-theme="dark"]`, `[data-theme="light"]`).
- **The measured inventory is 19 sites, 13 distinct combos, 12 of them modified** (the 13th is the unmodified `Escape` in `EditCard.svelte`), verified against this tree while writing this plan. If your scan finds a materially different number, the extractor is wrong — investigate before adjusting the expected count.
- Do not open or modify the user's real collection at `/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`. Use a copy.

---

### Task 1: The shortcut data

The panel's content, as plain data, with only the assertions a data module can carry. Task 2 is what checks it against reality.

**Files:**
- Create: `frontend/src/lib/shortcuts.js`
- Create: `frontend/src/lib/shortcuts.test.js`

**Interfaces:**
- Produces, for Tasks 2-3: `export const SHORTCUT_GROUPS`, an array of `{ title: string, items: Item[] }` where `Item` is `{ keys: string[], label: string, site: string[] }`.
- `keys` is the display form, already split — e.g. `['Ctrl/Cmd', 'Shift', 'S']`.
- `site` is ALWAYS an array, even for one site. Several shortcuts have three or four; see the spec's Scope table.

- [ ] **Step 1: Write the failing tests**

Create `frontend/src/lib/shortcuts.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { SHORTCUT_GROUPS } from './shortcuts.js'

test('every group has a title and at least one item', () => {
  assert.ok(SHORTCUT_GROUPS.length > 0, 'no groups at all')
  for (const g of SHORTCUT_GROUPS) {
    assert.ok(typeof g.title === 'string' && g.title.length, 'group without a title')
    assert.ok(Array.isArray(g.items) && g.items.length, `${g.title}: no items`)
  }
})

test('every item has keys, a label, and at least one site', () => {
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      assert.ok(Array.isArray(it.keys) && it.keys.length, `${g.title}: item without keys`)
      assert.ok(typeof it.label === 'string' && it.label.length, `${g.title}: item without a label`)
      // site is an array even when there is one site: Ctrl/Cmd+S has four.
      assert.ok(Array.isArray(it.site), `${g.title}/${it.label}: site must be an array`)
      assert.ok(it.site.length > 0, `${g.title}/${it.label}: no site`)
    }
  }
})

test('no shortcut is listed in two groups', () => {
  const seen = new Map()
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      const combo = it.keys.join('+')
      // Ctrl/Cmd+S legitimately appears twice — save a file, flush notes — so
      // the key is combo+label, not combo alone.
      const k = `${combo}|${it.label}`
      assert.ok(!seen.has(k), `${k} appears in both ${seen.get(k)} and ${g.title}`)
      seen.set(k, g.title)
    }
  }
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `node --test frontend/src/lib/shortcuts.test.js`
Expected: FAIL — cannot resolve `./shortcuts.js`.

- [ ] **Step 3: Write the data module**

Create `frontend/src/lib/shortcuts.js`. Transcribe the spec's Scope table exactly — the labels there were read from each handler, not inferred from the key name, and two of them (`Ctrl/Cmd+Shift+R`, `Ctrl/Cmd+Shift+M`) are not guessable.

```js
// The keyboard shortcuts the help panel lists, as plain data so `node --test`
// can cover them — the test runner cannot parse .svelte, so anything testable
// lives outside a component (the same reason cell-label.js and icons.js exist).
//
// `site` is the file(s) implementing each binding. It is what makes
// shortcuts.test.js's guard possible: the guard verifies this claim against the
// source, in both directions. It is an array because one user-facing shortcut
// routinely has several implementations — Ctrl/Cmd+S is four sites, Ctrl/Cmd+Enter
// is three, because the same gesture is wired up per surface.
export const SHORTCUT_GROUPS = [
  {
    title: 'Global',
    items: [
      { keys: ['Ctrl/Cmd', 'G'], label: 'Build graph', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'K'], label: 'Search', site: ['components/OmniBar.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'K'], label: 'Graph query', site: ['components/OmniBar.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'S'], label: 'Open Study', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'E'], label: 'Open Read', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'R'], label: 'Open Recall overlay', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'M'], label: 'Toggle log panel', site: ['App.svelte'] },
      { keys: ['F1'], label: 'This panel', site: ['App.svelte'] },
    ],
  },
  {
    title: 'Editor',
    items: [
      { keys: ['Ctrl/Cmd', 'S'], label: 'Save file', site: [
        'components/SourceTab.svelte',
        'components/ProcessEditor.svelte',
        'components/explorer/EditCard.svelte',
      ] },
    ],
  },
  {
    title: 'Study & Read',
    items: [
      { keys: ['Ctrl/Cmd', 'E'], label: 'Extract selection', site: ['components/ReadingView.svelte'] },
      { keys: ['Ctrl/Cmd', 'S'], label: 'Save notes', site: ['components/ReadingView.svelte'] },
    ],
  },
  {
    title: 'Explorer',
    items: [
      { keys: ['Ctrl/Cmd', 'Shift', 'K'], label: 'Focus query bar', site: ['components/GraphExplorer.svelte'] },
      { keys: ['Alt', '←'], label: 'History back', site: ['components/GraphExplorer.svelte'] },
      { keys: ['Alt', '→'], label: 'History forward', site: ['components/GraphExplorer.svelte'] },
    ],
  },
  {
    title: 'Notebook cells',
    items: [
      { keys: ['Ctrl/Cmd', 'Enter'], label: 'Run cell / send', site: [
        'components/notebook/ShellCell.svelte',
        'components/notebook/LLMCell.svelte',
        'components/LLMChatPane.svelte',
      ] },
    ],
  },
]
```

Note `Escape` and `Space`/`Enter` from the spec's table are deliberately absent — see Task 2 Step 3 for why, and put them back only if that step's reasoning says to.

- [ ] **Step 4: Run to verify they pass**

Run: `make test-js`
Expected: PASS. Run the whole suite, not just this file.

- [ ] **Step 5: Prove the tests can fail**

1. Give one item `site: 'App.svelte'` (a string, not an array).
   Expected: `every item has keys, a label, and at least one site` fails with "site must be an array".
2. Duplicate one item into a second group verbatim.
   Expected: `no shortcut is listed in two groups` fails naming both groups.
3. Empty one group's `items`.
   Expected: `every group has a title and at least one item` fails naming that group.

Revert each, and record the captured output.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/shortcuts.js frontend/src/lib/shortcuts.test.js
git commit -m "feat(shortcuts): the keyboard shortcut inventory as testable data"
```

---

### Task 2: The guard

The instrument that keeps the panel honest. This is the task most likely to find that Task 1's data is wrong — that is what it is for.

**Files:**
- Modify: `frontend/src/lib/shortcuts.test.js`

**Interfaces:**
- Consumes: `SHORTCUT_GROUPS` from Task 1.

- [ ] **Step 1: Add the extractor and both directions**

Append to `frontend/src/lib/shortcuts.test.js`. This extractor was prototyped against this tree; the exact patterns matter and three of them exist to fix bugs a naive version has:

```js
import { readdirSync, readFileSync } from 'node:fs'

const SRC = new URL('../', import.meta.url)

// DOM handlers: `if ((e.ctrlKey || e.metaKey) && e.key === 'k')`.
const KEY = /\bkey\s*===\s*'([^']+)'/
// The leading `!` capture matters: OmniBar has `&& !e.shiftKey && e.key === 'k'`,
// and reading that negation as a REQUIRED modifier turns Ctrl+K into
// Ctrl+Shift+K — which is a different shortcut ten lines below it.
const MOD = /(!?)\s*\w+\.(ctrlKey|metaKey|shiftKey|altKey)\b/g
// CodeMirror keymaps: `{ key: 'Mod-s', run: … }`. Requiring `run:` on the same
// line is what rejects ordinary object keys — ExplorerLegend.svelte has
// `key: 'contains'`, `key: 'leadsto'` and four more that are data, not bindings.
const CM = /\bkey:\s*'([^']+)'\s*,\s*run:/g

// Source writes `(e.key === 'e' || e.key === 'E')` and CodeMirror writes 'Mod-s';
// one canonical case so the two syntaxes agree that Ctrl/Cmd+S is one shortcut.
// Source also writes the DOM literal 'ArrowLeft'/'ArrowRight', while the data
// in shortcuts.js displays the glyph '←'/'→' — fold the former to the latter
// here too, so a third representational difference doesn't read as a missing
// shortcut. (The data is the display form; the source is not — see the KEY
// regex above, which never sees 'Ctrl/Cmd' either, only ctrlKey/metaKey.)
const canonKey = (k) => {
  if (k === 'ArrowLeft') return '←'
  if (k === 'ArrowRight') return '→'
  return k.length === 1 ? k.toUpperCase() : k
}

function comboFromDom(line) {
  const mods = [...line.matchAll(MOD)]
  const k = line.match(KEY)
  if (!mods.length || !k) return null
  const req = new Set(mods.filter((m) => m[1] !== '!').map((m) => m[2]))
  const parts = []
  // ctrlKey||metaKey is the cross-platform idiom — one key, not two modifiers.
  if (req.has('ctrlKey') || req.has('metaKey')) parts.push('Ctrl/Cmd')
  if (req.has('shiftKey')) parts.push('Shift')
  if (req.has('altKey')) parts.push('Alt')
  return [...parts, canonKey(k[1])].join('+')
}

function comboFromCm(spec) {
  const bits = spec.split('-')
  const key = bits.pop()
  const parts = []
  if (bits.includes('Mod') || bits.includes('Ctrl') || bits.includes('Cmd')) parts.push('Ctrl/Cmd')
  if (bits.includes('Shift')) parts.push('Shift')
  if (bits.includes('Alt')) parts.push('Alt')
  return [...parts, canonKey(key)].join('+')
}

function sourceSites() {
  const files = readdirSync(SRC, { recursive: true })
    .map(String)
    .filter((f) => (f.endsWith('.svelte') || f.endsWith('.js')) && !f.endsWith('.test.js'))
  const sites = []
  for (const f of files) {
    readFileSync(new URL(f, SRC), 'utf8').split('\n').forEach((line, i) => {
      const d = comboFromDom(line)
      // `kind` records which regex family found this site. It exists so the
      // vacuity check below can assert on each family separately — a combined
      // floor can't tell "CM broke entirely" from "DOM alone already clears
      // it," which is exactly how a `run:` typo silently zeroed CodeMirror
      // extraction (and 3 of Ctrl/Cmd+S's 4 sites with it) and still passed.
      if (d) sites.push({ combo: d, file: f, line: i + 1, kind: 'dom' })
      for (const m of line.matchAll(CM)) {
        sites.push({ combo: comboFromCm(m[1]), file: f, line: i + 1, kind: 'cm' })
      }
    })
  }
  return sites
}

// Only MODIFIER shortcuts are compared. Unmodified list-navigation arrows,
// and unmodified Escape/Space/Enter (close log panel, close Study, reveal
// answer), are out of the spec's Scope table by design — see the comment
// above `SHORTCUT_GROUPS` in shortcuts.js for why they're absent from the
// data too. A modifier-based scan also structurally cannot see them (no
// modifier survives `comboFromDom`/`comboFromCm`), so they're narrowed out of
// `documented()` here regardless of the scope decision.
const documented = () => {
  const set = new Map()
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) set.set(it.keys.join('+'), it)
  }
  return set
}

test('the scan finds the shortcuts known to exist', () => {
  const sites = sourceSites()
  // Measured on this tree: 15 DOM-handler sites, 4 CodeMirror keymap sites.
  // Asserted per family, not as one combined floor of 15 or 19: a combined
  // floor is exactly the value DOM extraction alone already clears, so it
  // cannot detect the CM family going to zero — which happened when `run:`
  // was mistyped in review and all 6 tests stayed green, silently losing
  // Ctrl/Cmd+S's CodeMirror sites (3 of its 4) along with it.
  const dom = sites.filter((s) => s.kind === 'dom').length
  const cm = sites.filter((s) => s.kind === 'cm').length
  assert.ok(dom >= 15, `found only ${dom} DOM-handler sites; the DOM scan is broken`)
  assert.ok(cm >= 4, `found only ${cm} CodeMirror keymap sites; the CM scan is broken`)
})

test('every documented shortcut exists at a site it names', () => {
  // Compares the FULL combo (modifiers included), not just the trailing key
  // letter — a bare-key substring check would pass `Build graph` pointed at
  // any file that happens to mention 'g' or 'G' anywhere, modifiers or not.
  const sites = sourceSites()
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      if (it.keys.join('+') === 'F1') continue // added by this feature; Task 3 wires it
      const combo = it.keys.join('+')
      for (const f of it.site) {
        const combosInFile = sites.filter((s) => s.file === f).map((s) => s.combo)
        assert.ok(
          combosInFile.includes(combo),
          `${g.title}/${it.label}: ${f} has no site extracting ${combo} ` +
            `(found: ${combosInFile.join(', ') || 'none'})`,
        )
      }
    }
  }
})

test('every modifier shortcut in source is documented', () => {
  const docs = documented()
  const undocumented = []
  for (const s of sourceSites()) {
    if (!s.combo.includes('+')) continue // unmodified: contextual, out of scope
    if (!docs.has(s.combo)) undocumented.push(`${s.combo} at ${s.file}:${s.line}`)
  }
  assert.deepEqual(undocumented, [],
    `shortcuts in source but missing from the help panel:\n${undocumented.join('\n')}`)
})
```

- [ ] **Step 2: Run it**

Run: `make test-js`

This may FAIL, and that is informative rather than a problem. Two outcomes are expected and each has a different response:

- **`every modifier shortcut in source is documented` fails** naming a combo Task 1 omitted → Task 1's data is incomplete. Add the missing entry, using the label read from the handler at that site.
- **`every documented shortcut exists at a site it names` fails** → either the site path is wrong, or the combo the scan extracts from that site does not match the documented combo (modifiers included). Fix whichever is actually wrong; do not weaken the assertion to make it pass.

Report which outcome you hit and what you changed.

- [ ] **Step 3: Decide the unmodified keys**

The spec's Scope table lists `Escape` (close log panel), `Escape` (close Study) and `Space` / `Enter` (reveal answer). Task 1 left them out because the guard's second direction only compares modifier combos, so they would be documented but unguarded — a claim nothing verifies.

Choose, and justify in your report:
- **Leave them out**, treating Escape-to-close and Space-to-reveal as conventions users already know, consistent with excluding arrow navigation; or
- **Put them in**, accepting that those specific entries are unguarded, and say so in a comment beside them so nobody later assumes the guard covers everything the panel shows.

Either is defensible. An undocumented decision is not.

- [ ] **Step 4: Prove the guard can fail**

1. Add `if ((e.ctrlKey || e.metaKey) && e.key === 'j') { /* test */ }` to `App.svelte`.
   Expected: `every modifier shortcut in source is documented` fails naming `Ctrl/Cmd+J at App.svelte:<line>`.
2. Change one item's `site` to a file that does not implement it — e.g. point `Build graph` at `components/OmniBar.svelte`.
   Expected: `every documented shortcut exists at a site it names` fails.
3. Point `SRC` at a directory with no source files.
   Expected: `the scan finds the shortcuts known to exist` fails. Without it the other two would pass while checking nothing.

Verify each sabotage actually applied before trusting its result — a substitution that silently fails to match produces a green run that looks like a passing control. Revert each and record the output.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/shortcuts.test.js frontend/src/lib/shortcuts.js
git commit -m "test(shortcuts): guard the panel against the source, both directions"
```

---

### Task 3: The panel

**Files:**
- Create: `frontend/src/components/ShortcutsPanel.svelte`
- Modify: `frontend/src/components/MiddleBand.svelte` (the `?` button, beside `SettingsMenu`)
- Modify: `frontend/src/App.svelte` (F1, in the existing global `keydown` handler)
- Create: `frontend/src/stores/shortcuts.js` (or add to an existing store module — see Step 1)

**Interfaces:**
- Consumes: `SHORTCUT_GROUPS` from Task 1.
- Produces: a writable `shortcutsOpen` store, so both the button and F1 drive the same state.

- [ ] **Step 1: Add the open/closed store**

`App.svelte` owns F1 and `MiddleBand.svelte` owns the button, so the state cannot live in either. Follow the existing pattern: `stores/workbench.js` holds `logPanelOpen` and `omnibarMode` for exactly this reason. Put `shortcutsOpen` alongside them rather than creating a new module for one boolean — and say in your report which you chose.

- [ ] **Step 2: Write the panel**

Create `frontend/src/components/ShortcutsPanel.svelte`. It renders `SHORTCUT_GROUPS`: a heading per group, and per item the keys as separate `<kbd>` elements plus the label.

Requirements:
- Closes on `Escape`, matching `SettingsMenu.svelte`, `QueryHelpPanel.svelte` and the log panel.
- Uses only existing `var(--token)` values. `theme-tokens.test.js` fails the suite on any literal colour. `QueryHelpPanel.svelte` is the closest existing panel — read it for which tokens a panel of this kind uses.
- `role="dialog"` and an `aria-label`, as `StudySurface.svelte` does.

- [ ] **Step 3: Wire the `?` button**

In `MiddleBand.svelte`, beside `<SettingsMenu />`. The 2026-08-12 themes design placed the gear "where the pair will sit" — this is the pair. Match the gear's styling; give it `aria-label="Keyboard shortcuts"` and a `title` naming F1, the way the other toolbar buttons advertise their shortcuts.

Feature 050 replaced the UI's emoji with SVG icons and its guard forbids astral-plane characters, so use an `<Icon>` if a suitable name exists in `lib/icons.js`, or a plain `?` character — which is ASCII and therefore fine. Do not add an emoji.

- [ ] **Step 4: Wire F1**

In `App.svelte`'s existing `handleKeydown`, alongside the other global shortcuts. F1 needs no modifier and no is-this-a-text-field guard — that is why the spec chose it over `?`.

Follow the surrounding style exactly: the existing handlers call `e.preventDefault()` and `e.stopPropagation()`. F1 has a browser default (help) in some environments, so `preventDefault` matters.

- [ ] **Step 5: Run**

Run: `make test-js`
Expected: PASS, including `theme-tokens.test.js` (no literal colours) and the Task 2 guard, whose `F1` entry is now real.

Run: `cd frontend && npx vite build`
Expected: succeeds.

- [ ] **Step 6: Commit**

```bash
git add frontend/src
git commit -m "feat(shortcuts): a help panel on ? and F1"
```

---

### Task 4: Verification

**Files:**
- Create: `docs/specs/2026-08-19-shortcuts-help-panel-manual-test-plan.md`

- [ ] **Step 1: Build and open a scratch collection**

`make build` from the repo root. Run `./build/bin/texere` with `XDG_CONFIG_HOME=/tmp/claude-1000/-home-simone-projects-texere/45775468-79e5-4a04-b088-48b273096d37/scratchpad/xdg` — a scratch config pointing at a COPIED collection. Never the user's real one.

Helpers in that scratchpad: `xcap.py texere <out.png>`, `xin.py click texere <x> <y>`, `xin.py key texere <keysym> [Control_L …]` (modifiers need real keysym names).

- [ ] **Step 2: Verify the panel**

Read every screenshot back with the Read tool and describe what you saw.

1. The `?` button appears beside the `⚙`.
2. Clicking it opens the panel; every group renders with its shortcuts.
3. **F1 opens it** from the main window.
4. `Escape` closes it.
5. **All three themes** — Blue, Dark and Light. Feature 052 made Light real, and a new panel is exactly the kind of surface built against one theme and checked against none. Switch via the gear's Appearance menu.

- [ ] **Step 3: Spot-check that the panel tells the truth**

Pick three shortcuts from three different groups and actually press them. The panel says what they do; confirm they do it. A help panel that is internally consistent but wrong about the app is the failure mode no test catches — the guard proves a binding EXISTS at a site, never that the label describes what it does.

- [ ] **Step 4: Write up the result**

Every case with a pass/fail and a one-line observation, in the shape of `docs/specs/2026-08-17-relative-identity-keys-manual-test-plan.md`. Record anything you could not reach.

- [ ] **Step 5: Commit**

```bash
git add docs/specs/2026-08-19-shortcuts-help-panel-manual-test-plan.md
git commit -m "docs(test-plan): keyboard shortcuts help panel"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| `lib/shortcuts.js` pure data module | 1 |
| `site` is an array (multi-site bindings) | 1 (schema), 2 (guard honours it) |
| Five groups, labels read from handlers | 1 |
| Guard: no phantom entries | 2 |
| Guard: no undocumented bindings | 2 |
| Extractor handles DOM + CodeMirror syntaxes | 2 |
| Negated modifier is not a required one | 2 (the `(!?)` capture) |
| `ctrlKey \|\| metaKey` renders as one `Ctrl/Cmd` | 2 |
| Vacuity assertion | 2 |
| Panel component | 3 |
| `?` button in the reserved slot | 3 |
| F1 trigger | 3 |
| `Escape` closes | 3 |
| Renders in all three themes | 4 |
| Out of scope: rebinding, per-OS display, list navigation | not implemented anywhere |

**Type consistency:** `SHORTCUT_GROUPS` is `{ title: string, items: { keys: string[], label: string, site: string[] }[] }[]` in Task 1 and consumed with exactly those field names in Tasks 2 and 3. `shortcutsOpen` is introduced in Task 3 Step 1 and used by both Step 3 and Step 4.

**Verified before writing, not assumed:** the extractor was prototyped against this tree through three iterations, and each found a real bug. A modifier-and-key scan reported 15 DOM sites; adding CodeMirror keymaps naively brought in 6 false positives from `ExplorerLegend.svelte`'s `key: 'contains'` data (fixed by requiring `run:` on the same line); and `Ctrl/Cmd+S` split into two combos until both syntaxes shared one case-normalisation (fixed, now 4 sites under one combo). Final measured inventory: **19 sites, 13 distinct combos, 12 of them modified.**

**Known weakness this plan accepts:** the guard proves a binding exists at a site and that no modifier shortcut is undocumented. It cannot prove a LABEL is true — "Build graph" beside `Ctrl/Cmd+G` passes whatever that handler actually does. Task 3 Step 3 exists solely to catch that, by pressing three of them, and it is the only check on the panel's honesty about meaning rather than existence.
