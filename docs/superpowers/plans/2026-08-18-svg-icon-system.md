# SVG Icon System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the nine astral-plane emoji in the UI with inline SVG icons, so the app stops depending on an emoji font it does not ship.

**Architecture:** A pure data module (`src/lib/icons.js`) holding path geometry, and a thin renderer (`src/components/Icon.svelte`) that turns a name into an `<svg>` with `stroke="currentColor"`. The split follows the line the repo already tests on: `make test-js` runs `node:test` over `src/lib/*.test.js` only, and there is no Svelte component harness.

**Tech Stack:** Svelte 3.49, Vite, `node:test` (stdlib). No new dependencies — Lucide path geometry is copied as strings, not installed.

**Spec:** `docs/superpowers/specs/2026-08-18-svg-icon-system-design.md`

## Global Constraints

- **No new npm dependencies.** The app is strictly offline and self-contained; only the path geometry is copied, with attribution.
- **`make test-js` runs `node --test frontend/src/lib/*.test.js`.** Tests only run if they live in `frontend/src/lib/` and end in `.test.js`. A test placed anywhere else is silently never run.
- **The test runner cannot parse `.svelte`.** Anything a test needs to check inside a component must be checked by reading the file as text, the way `n4l-syntax.test.js` already reads `style.css`.
- **Icons inherit color, never set it.** `stroke="currentColor"` and no `fill`. Hardcoding a color breaks the light/dark theme.
- **Leave `+ Card` alone.** Each `hover-actions` block in `ColumnNodes.svelte` holds two dead `console.log` stubs: the `💬` button this plan deletes, and a `+ Card` button beside it. `+ Card` renders correctly and is not an icon, so it is out of scope. Do not delete it, and do not delete the enclosing `<div class="hover-actions">` — it still has a child.
- **The path data in Task 1 was transcribed from memory of the Lucide set and is NOT trusted.** A wrong path renders as garbage rather than failing a test, so Task 1 contains a visual proof step. Do not skip it and do not wire any icon into the UI before it passes.
- Do not open or modify the user's real collection at `/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`.

---

### Task 1: The icon data module

The geometry comes first and gets *looked at* before anything consumes it.

**Files:**
- Create: `frontend/src/lib/icons.js`
- Create: `frontend/src/lib/icons.test.js`
- Create (temporary): `frontend/icon-proof.html`

**Interfaces:**
- Produces, for Tasks 2-4: `export const ICONS`, an object whose keys are the nine icon names and whose values are `{ viewBox: string, paths: string[] }`.
- The nine names, used verbatim in later tasks: `save`, `search`, `globe`, `book-open`, `books`, `fuzzy`, `expand`, `collapse`, `celebrate`.

- [ ] **Step 1: Write the failing tests**

Create `frontend/src/lib/icons.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { ICONS } from './icons.js'

// The nine names the components reference. Spelled out rather than derived from
// ICONS itself: a test that reads its expectations out of the thing under test
// passes no matter what that thing contains.
const EXPECTED = [
  'save', 'search', 'globe', 'book-open', 'books',
  'fuzzy', 'expand', 'collapse', 'celebrate',
]

test('every expected icon exists', () => {
  for (const name of EXPECTED) {
    assert.ok(ICONS[name], `missing icon: ${name}`)
  }
})

test('ICONS holds exactly the expected names', () => {
  assert.deepEqual(Object.keys(ICONS).sort(), [...EXPECTED].sort())
})

test('every icon has a viewBox and at least one non-empty path', () => {
  for (const [name, icon] of Object.entries(ICONS)) {
    assert.match(icon.viewBox, /^0 0 \d+ \d+$/, `${name}: bad viewBox`)
    assert.ok(Array.isArray(icon.paths), `${name}: paths must be an array`)
    assert.ok(icon.paths.length > 0, `${name}: no paths`)
    for (const d of icon.paths) {
      assert.equal(typeof d, 'string', `${name}: path must be a string`)
      assert.ok(d.trim().length > 0, `${name}: empty path`)
      // A path that does not start with a move command is malformed, and the
      // browser draws nothing rather than complaining.
      assert.match(d.trim(), /^[Mm]/, `${name}: path does not start with a move`)
    }
  }
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `node --test frontend/src/lib/icons.test.js`
Expected: FAIL — cannot resolve `./icons.js`.

- [ ] **Step 3: Write the data module**

Create `frontend/src/lib/icons.js`:

```js
// SVG icon geometry, kept as plain data so `node --test` can cover it — the
// test runner cannot parse .svelte, so anything testable has to live outside a
// component (the same reason cell-label.js exists).
//
// Geometry is adapted from Lucide (https://lucide.dev), ISC licensed. Only the
// path strings are copied; Lucide is deliberately NOT a dependency, because the
// app ships offline and self-contained.
//
// All icons are 24x24 outline paths meant to be stroked, never filled. Icon.svelte
// applies stroke="currentColor" so they follow the theme.
export const ICONS = {
  save: {
    viewBox: '0 0 24 24',
    paths: [
      'M15.2 3a2 2 0 0 1 1.4.6l3.8 3.8a2 2 0 0 1 .6 1.4V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z',
      'M17 21v-7a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v7',
      'M7 3v4a1 1 0 0 0 1 1h7',
    ],
  },
  search: {
    viewBox: '0 0 24 24',
    paths: [
      'M11 3a8 8 0 1 0 0 16 8 8 0 0 0 0-16z',
      'm21 21-4.35-4.35',
    ],
  },
  globe: {
    viewBox: '0 0 24 24',
    paths: [
      'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20z',
      'M2 12h20',
      'M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z',
    ],
  },
  'book-open': {
    viewBox: '0 0 24 24',
    paths: [
      'M12 7v14',
      'M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z',
    ],
  },
  books: {
    viewBox: '0 0 24 24',
    paths: [
      'M4 4v16',
      'M8 4v16',
      'M12 5v15',
      'm16 6 4 14',
    ],
  },
  fuzzy: {
    viewBox: '0 0 24 24',
    paths: [
      'M12 2.7 21.3 12 12 21.3 2.7 12z',
    ],
  },
  expand: {
    viewBox: '0 0 24 24',
    paths: [
      'M8 3H5a2 2 0 0 0-2 2v3',
      'M21 8V5a2 2 0 0 0-2-2h-3',
      'M3 16v3a2 2 0 0 0 2 2h3',
      'M16 21h3a2 2 0 0 0 2-2v-3',
    ],
  },
  collapse: {
    viewBox: '0 0 24 24',
    paths: [
      'M8 3v3a2 2 0 0 1-2 2H3',
      'M21 8h-3a2 2 0 0 1-2-2V3',
      'M3 16h3a2 2 0 0 1 2 2v3',
      'M16 21v-3a2 2 0 0 1 2-2h3',
    ],
  },
  celebrate: {
    viewBox: '0 0 24 24',
    paths: [
      'M5.8 11.3 2 22l10.7-3.79',
      'M11 13c1.93 1.93 2.83 4.17 2 5-.83.83-3.07-.07-5-2-1.93-1.93-2.83-4.17-2-5 .83-.83 3.07.07 5 2z',
      'M15 3v2',
      'M19 5l-1.5 1.5',
      'M21 10h-2',
    ],
  },
}
```

- [ ] **Step 4: Run to verify they pass**

Run: `node --test frontend/src/lib/icons.test.js`
Expected: PASS, 3 tests.

- [ ] **Step 5: Look at the icons**

The tests above prove each path is *well-formed*, not that it *looks like the thing it is named after*. The geometry was transcribed from memory and is the least trustworthy part of this plan, so prove it with your eyes before anything depends on it.

Create `frontend/icon-proof.html`:

```html
<!doctype html>
<meta charset="utf-8">
<title>icon proof</title>
<style>
  body { background:#16202c; color:#d8e0ea; font:14px sans-serif; padding:24px; }
  ul { list-style:none; display:flex; flex-wrap:wrap; gap:28px; padding:0; }
  li { display:flex; flex-direction:column; align-items:center; gap:8px; width:120px; }
  svg { width:72px; height:72px; }
</style>
<ul id="out"></ul>
<script type="module">
  import { ICONS } from './src/lib/icons.js'
  const NS = 'http://www.w3.org/2000/svg'
  document.getElementById('out').innerHTML = Object.entries(ICONS).map(([name, i]) => `
    <li>
      <svg viewBox="${i.viewBox}" fill="none" stroke="currentColor"
           stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        ${i.paths.map(d => `<path d="${d}"/>`).join('')}
      </svg>
      <span>${name}</span>
    </li>`).join('')
</script>
```

Serve it and look at it:

```bash
cd frontend && npx vite --port 5199
# open http://localhost:5199/icon-proof.html
```

Screenshot the page and **read the screenshot back**. Check each of the nine against its name: a magnifier for `search`, an open book for `book-open`, a stack of book spines for `books`, a diamond for `fuzzy`, corner brackets pointing outward for `expand` and inward for `collapse`, a floppy disk for `save`, a wire globe for `globe`, a popper burst for `celebrate`.

**Any icon that does not read as its name is a defect in this plan, not in your work.** Replace its path data with correct Lucide geometry (or your own), re-run the proof, and say in your report which ones you had to fix and what was wrong with them. Do not proceed while any icon looks like garbage — the whole point of this feature is that these are visible.

- [ ] **Step 6: Prove the tests can fail**

1. Delete the `books` entry from `ICONS`.
   Expected: `every expected icon exists` fails with "missing icon: books", and `ICONS holds exactly the expected names` fails.
2. Change one `paths` array to `[]`.
   Expected: `every icon has a viewBox and at least one non-empty path` fails with "no paths".
3. Change one path string to start with `L` instead of `M`.
   Expected: the same test fails with "path does not start with a move".

Revert each after observing it, and record the captured output in your report.

- [ ] **Step 7: Remove the proof harness and commit**

`icon-proof.html` is scaffolding, not product. Delete it — the screenshots from Step 5 are the record.

```bash
rm frontend/icon-proof.html
git add frontend/src/lib/icons.js frontend/src/lib/icons.test.js
git commit -m "feat(icons): SVG icon geometry as a testable data module"
```

---

### Task 2: The Icon component, and the toolbar

The component gets a real consumer in the same task, because a renderer with no call site cannot be verified — there is no component test harness.

**Files:**
- Create: `frontend/src/components/Icon.svelte`
- Modify: `frontend/src/components/MiddleBand.svelte:124`, `:126`, `:135`, `:139`, `:140`
- Modify: `frontend/src/lib/icons.test.js` (add the typo guard)

**Interfaces:**
- Consumes: `ICONS` from Task 1.
- Produces, for Tasks 3-4: `<Icon name={string} size={number} label={string} />`, imported as `import Icon from './Icon.svelte'` (from `components/`) or `'../Icon.svelte'` (from `components/notebook/`). `size` defaults to `16`; `label` defaults to `''`, meaning decorative.

- [ ] **Step 1: Write the failing test**

Add to `frontend/src/lib/icons.test.js`:

```js
import { readdirSync, readFileSync } from 'node:fs'

const COMPONENTS = new URL('../components/', import.meta.url)

function svelteSources() {
  const files = readdirSync(COMPONENTS, { recursive: true })
    .map(String)
    .filter((f) => f.endsWith('.svelte'))
  return files.map((f) => [f, readFileSync(new URL(f, COMPONENTS), 'utf8')])
}

// Names reach <Icon> two ways: as a literal (name="save") and inside an
// expression (name={expanded ? 'collapse' : 'expand'}). Both must be checked,
// so the expression branch pulls every quoted string out of the braces.
function iconNamesIn(src) {
  const names = new Set()
  for (const m of src.matchAll(/<Icon\b[^>]*?\bname=(?:"([^"]+)"|\{([^}]*)\})/g)) {
    if (m[1]) names.add(m[1])
    else for (const q of m[2].matchAll(/'([^']+)'/g)) names.add(q[1])
  }
  return names
}

test('every <Icon name=…> in a component exists in ICONS', () => {
  let checked = 0
  for (const [file, src] of svelteSources()) {
    for (const name of iconNamesIn(src)) {
      checked++
      assert.ok(ICONS[name], `${file} uses unknown icon "${name}"`)
    }
  }
  // Without this the test passes when the regex silently matches nothing —
  // which is exactly what a renamed prop or a changed tag would cause.
  assert.ok(checked > 0, 'found no <Icon> usages at all; the scan is broken')
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/icons.test.js`
Expected: FAIL — "found no `<Icon>` usages at all", because no component uses `Icon` yet.

- [ ] **Step 3: Write the component**

Create `frontend/src/components/Icon.svelte`:

```svelte
<script>
  import { ICONS } from '../lib/icons.js'

  export let name
  export let size = 16
  // Accessible name. Empty means decorative — correct when a visible text label
  // sits beside the icon, wrong when the icon is the button's only content.
  export let label = ''

  // Fail soft. A mistyped name must not blank a toolbar or throw inside a
  // button's render; icons.test.js catches the typo at test time instead.
  $: icon = ICONS[name]
  $: if (!icon) console.warn(`Icon: unknown name "${name}"`)
</script>

{#if icon}
  <svg
    viewBox={icon.viewBox}
    width={size}
    height={size}
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
    role={label ? 'img' : undefined}
    aria-label={label || undefined}
    aria-hidden={label ? undefined : 'true'}
  >
    {#each icon.paths as d}
      <path {d} />
    {/each}
  </svg>
{/if}

<style>
  /* Sits on the text baseline next to a label, and never stretches in a flex row. */
  svg { display: inline-block; vertical-align: -0.125em; flex: none; }
</style>
```

Svelte drops attributes whose value is `undefined`, so a decorative icon emits `aria-hidden="true"` and no `role`/`aria-label`, and a labelled one the reverse.

- [ ] **Step 4: Convert the toolbar**

In `frontend/src/components/MiddleBand.svelte`, add to the imports at the top of `<script>` (after the `SettingsMenu` import):

```js
import Icon from './Icon.svelte'
```

Then replace the five buttons. The three icon-only buttons gain an `aria-label`, because today their meaning lives only in `title`, which screen readers treat inconsistently:

```svelte
<button class="btn" on:click={handleSave} title="Save (Ctrl+S)" aria-label="Save"><Icon name="save" /></button>
```

```svelte
<button class="btn" on:click={handleSearch} title="Search (Ctrl+K / Cmd+K)" aria-label="Search"><Icon name="search" /></button>
```

```svelte
    <button
      class="btn"
      on:click={handleOpenExplorer}
      disabled={!explorerLaunchQuery}
      title={explorerLaunchQuery ? 'Open 2.5D Graph Explorer' : 'Select a chapter to enable'}
      aria-label="Open 2.5D Graph Explorer"
    ><Icon name="globe" /></button>
```

The two labelled buttons keep their visible text, so their icons stay decorative — no `label` prop, no `aria-label`:

```svelte
    <button class="btn" on:click={handleOpenRead} title="Read & import sources — + Text / PDF / Web (Ctrl+Shift+E)"><Icon name="book-open" /> Read</button>
    <button class="btn" on:click={handleOpenStudy} title="Study — Read · Process · Recall queue (Ctrl+Shift+S)"><Icon name="books" /> Study</button>
```

Leave `handleUndo`'s `↩` button between them untouched: it is a BMP symbol that renders correctly and sits in no ternary.

- [ ] **Step 5: Run to verify it passes**

Run: `node --test frontend/src/lib/icons.test.js`
Expected: PASS, 4 tests. The guard now finds 5 usages.

Run: `cd frontend && npx vite build`
Expected: build succeeds. A Svelte syntax error in `Icon.svelte` shows up here and nowhere else.

- [ ] **Step 6: Prove the guard can fail**

1. Change one call site to `<Icon name="savee" />`.
   Expected: `every <Icon name=…> in a component exists in ICONS` fails with `MiddleBand.svelte uses unknown icon "savee"`.
2. Rename the prop in the guard's regex from `name` to `icon` (simulating a renamed prop the guard no longer tracks).
   Expected: the test fails on `found no <Icon> usages at all`, not silently passes. This is the control that matters — it proves the guard notices when it has stopped guarding.

Revert both, and record the output.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/Icon.svelte frontend/src/components/MiddleBand.svelte frontend/src/lib/icons.test.js
git commit -m "feat(icons): Icon component, and SVG icons in the toolbar"
```

---

### Task 3: The two mixed ternaries

These are the second defect from the spec: each control pairs an astral emoji with a BMP symbol, so one state renders and the other shows a box.

**Files:**
- Modify: `frontend/src/components/OmniBar.svelte:955`
- Modify: `frontend/src/components/notebook/SketchCell.svelte:195`

**Interfaces:**
- Consumes: `Icon` from Task 2, and the names `search`, `fuzzy`, `expand`, `collapse` from Task 1.

- [ ] **Step 1: Convert OmniBar**

Add to the imports in `OmniBar.svelte`'s `<script>`:

```js
import Icon from './Icon.svelte'
```

Replace line 955:

```svelte
      <span class="search-icon" aria-hidden="true">{isFuzzyMode ? '🔍' : '◈'}</span>
```

with:

```svelte
      <span class="search-icon" aria-hidden="true"><Icon name={isFuzzyMode ? 'search' : 'fuzzy'} /></span>
```

The wrapping `<span>` keeps its class (`.search-icon` is styled) and its existing `aria-hidden`, so the icon stays decorative — correct, because the input beside it carries the label.

- [ ] **Step 2: Convert SketchCell**

`SketchCell.svelte` is in `components/notebook/`, so the import path has one more level:

```js
import Icon from '../Icon.svelte'
```

Replace line 195:

```svelte
      <button class="sk-expand" on:click={toggleExpand} title="Expand to fill the window (Esc to exit)">{expanded ? '🗕 Collapse' : '⛶ Expand'}</button>
```

with:

```svelte
      <button class="sk-expand" on:click={toggleExpand} title="Expand to fill the window (Esc to exit)"><Icon name={expanded ? 'collapse' : 'expand'} /> {expanded ? 'Collapse' : 'Expand'}</button>
```

The visible word stays, so the icon is decorative. Note the text ternary is now separate from the icon ternary — both switch on the same `expanded`.

- [ ] **Step 3: Run to verify**

Run: `node --test frontend/src/lib/icons.test.js`
Expected: PASS. The guard now resolves `search`, `fuzzy`, `expand` and `collapse` out of the two expression forms — which is the branch of `iconNamesIn` that Task 2 could not exercise, because Task 2's call sites were all literals.

Run: `cd frontend && npx vite build`
Expected: succeeds.

- [ ] **Step 4: Prove the expression branch is really being checked**

Change the SketchCell call site to `name={expanded ? 'collapsee' : 'expand'}`.
Expected: the guard fails with `notebook/SketchCell.svelte uses unknown icon "collapsee"`. If it passes, `iconNamesIn` is not reading inside `{...}` and Task 3's two call sites are unguarded — fix the regex before continuing.

Revert, and record the output.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/OmniBar.svelte frontend/src/components/notebook/SketchCell.svelte
git commit -m "fix(icons): both states of the fuzzy and expand toggles render"
```

---

### Task 4: Empty states, dead buttons, and the acceptance check

**Files:**
- Modify: `frontend/src/components/StudySurface.svelte:210`
- Modify: `frontend/src/components/RecallCard.svelte:29`
- Modify: `frontend/src/components/ColumnNodes.svelte` (delete four buttons)
- Modify: `frontend/src/lib/icons.test.js` (add the acceptance check)

**Interfaces:**
- Consumes: `Icon` from Task 2, `celebrate` from Task 1.

- [ ] **Step 1: Write the failing acceptance test**

This is the property the whole feature exists to establish. Add to `frontend/src/lib/icons.test.js`:

```js
const SRC = new URL('../', import.meta.url)

// Astral-plane emoji (U+1F300–U+1FAFF) are the ones with no glyph in the fonts
// a bare Linux install ships, so they render as tofu boxes. BMP symbols like ⚙
// and ↩ are fine and deliberately not matched here.
const ASTRAL = /[\u{1F300}-\u{1FAFF}]/u

test('no astral-plane emoji remain in the UI source', () => {
  const files = readdirSync(SRC, { recursive: true })
    .map(String)
    .filter((f) => (f.endsWith('.svelte') || f.endsWith('.js')) && !f.endsWith('.test.js'))
  assert.ok(files.length > 0, 'scanned no files; the walk is broken')
  const offenders = []
  for (const f of files) {
    const src = readFileSync(new URL(f, SRC), 'utf8')
    for (const [i, line] of src.split('\n').entries()) {
      if (ASTRAL.test(line)) offenders.push(`${f}:${i + 1}: ${line.trim()}`)
    }
  }
  assert.deepEqual(offenders, [], `emoji left in the UI:\n${offenders.join('\n')}`)
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/icons.test.js`
Expected: FAIL, listing the seven remaining emoji lines — `StudySurface.svelte:210`, `RecallCard.svelte:29`, and the four `💬` lines in `ColumnNodes.svelte`.

- [ ] **Step 3: Convert the two empty states**

In `StudySurface.svelte`, add `import Icon from './Icon.svelte'` to the `<script>` imports, then replace line 210:

```svelte
      <div class="ss-empty">Nothing due. 🎉</div>
```

with:

```svelte
      <div class="ss-empty">Nothing due. <Icon name="celebrate" /></div>
```

In `RecallCard.svelte`, add `import Icon from './Icon.svelte'` to the `<script>` imports, then replace line 29:

```svelte
  <div class="rc-center"><p>Nothing due. 🎉</p></div>
```

with:

```svelte
  <div class="rc-center"><p>Nothing due. <Icon name="celebrate" /></p></div>
```

Both are decorative: the sentence beside them already says what they mean.

- [ ] **Step 4: Delete the four dead buttons**

In `ColumnNodes.svelte`, delete this line in each of the four `hover-actions` blocks (near lines 166, 195, 219 and 255 — the indentation differs between the first two and the last two, so match on content, not column):

```svelte
                    <button class="ha-btn" on:click|stopPropagation={() => console.log('Discuss', node.Line)}>💬</button>
```

Delete **only** that line. The `+ Card` button above it is also a `console.log` stub, but it renders correctly, is not an icon, and is out of this feature's scope — leave it, and leave the enclosing `<div class="hover-actions">`, which still has a child.

- [ ] **Step 5: Run to verify they pass**

Run: `make test-js`
Expected: PASS. Run the whole suite here, not just this file — this is the first task that touches components other tests read.

Run: `cd frontend && npx vite build`
Expected: succeeds.

Confirm the deletion count:

```bash
grep -c "console.log('Discuss'" frontend/src/components/ColumnNodes.svelte || echo 0
```

Expected: `0`. And confirm `+ Card` survived:

```bash
grep -c "Promote to Card" frontend/src/components/ColumnNodes.svelte
```

Expected: `4`.

- [ ] **Step 6: Prove the acceptance test can fail**

Put a single `💬` back into any `.svelte` file under `src/`.
Expected: `no astral-plane emoji remain in the UI source` fails, naming that file and line.

Also delete the `assert.ok(files.length > 0)` line and point `SRC` at a directory with no matching files.
Expected: without that assertion the test passes while scanning nothing. Restore both, and record what you saw — this is the guard against the test quietly becoming vacuous.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/StudySurface.svelte frontend/src/components/RecallCard.svelte frontend/src/components/ColumnNodes.svelte frontend/src/lib/icons.test.js
git commit -m "feat(icons): SVG empty states; drop the dead Discuss buttons"
```

---

### Task 5: Verification in the running app

Nothing above proves an icon is *visible in the app*. There is no component test harness, so this is where that gets established.

**Files:**
- Create: `docs/superpowers/specs/2026-08-18-svg-icon-system-manual-test-plan.md`

- [ ] **Step 1: Build and open a scratch collection**

```bash
make build
```

Do **not** point the app at the user's real collection. Copy it to a scratch directory and launch with `XDG_CONFIG_HOME` set to a scratch config naming the copy, as feature 049's manual test plan did — that leaves `~/.config/texere/config.json` untouched rather than modifying and restoring it.

- [ ] **Step 2: Capture each converted surface**

This box has no screenshot tool and no PIL. Feature 049's manual test plan describes the `XGetImage` capture and XTEST input drivers written for it; rebuild them if they are not still in the scratch directory.

Capture and **read back**:

1. The toolbar — Save, Search, Explorer, Read, Study. No boxes.
2. The OmniBar search icon in **both** states (toggle fuzzy mode). Both must draw.
3. The SketchCell expand control in **both** states. Both must draw.
4. The Study empty state, showing the celebrate icon.
5. A node row in Column 3, confirming `+ Card` is still there and the `💬` button is gone.

- [ ] **Step 3: Confirm the icons follow the theme**

Switch between light and dark theme and re-capture the toolbar. The icons must change color with the text. If they do not, `stroke="currentColor"` is not reaching them — check for a `stroke` or `color` override on `.btn`.

- [ ] **Step 4: Write up the result**

Every case with a pass/fail and a one-line observation, in the shape of `docs/superpowers/specs/2026-08-17-relative-identity-keys-manual-test-plan.md`. Record which icons, if any, you had to redraw in Task 1 Step 5, and what was wrong with them.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-08-18-svg-icon-system-manual-test-plan.md
git commit -m "docs(test-plan): SVG icon system"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| `src/lib/icons.js` pure data module | 1 |
| Nine named icons | 1 |
| Lucide attribution, no dependency | 1 |
| `Icon.svelte` with `name`/`size`/`label` | 2 |
| `stroke="currentColor"`, theme-following | 2 (built), 5 (verified) |
| Unknown name fails soft | 2 |
| MiddleBand five call sites | 2 |
| `aria-label` on the three icon-only buttons | 2 |
| OmniBar + SketchCell mixed ternaries | 3 |
| Empty states (StudySurface, RecallCard) | 4 |
| Delete four `💬` buttons | 4 |
| `icons.test.js` data tests | 1 |
| Typo guard over `.svelte` sources | 2 (literals), 3 (expressions) |
| Astral-codepoint acceptance check | 4 |
| Manual screenshots, both ternary states | 5 |
| Light/dark theme check | 5 |
| Out of scope: `⚙`, `⟳`, `↩`, font-family stacks | stated in Global Constraints and Task 2 Step 4 |

**Type consistency:** `ICONS` is `{ [name]: { viewBox: string, paths: string[] } }` in Task 1 and consumed with exactly those field names in Task 2's component and Task 1's tests. `<Icon name size label />` is introduced in Task 2 and used with the same prop names in Tasks 3 and 4. The nine icon names are spelled identically in Task 1's `EXPECTED`, Task 1's `ICONS`, and every call site in Tasks 2-4.

**Known weakness this plan accepts:** the path geometry in Task 1 was written from memory of the Lucide set, and no automated test can tell a correct magnifier from a wrong one. Task 1 Step 5 exists solely to catch that with human (or screenshot-reading) judgement, and the plan says outright that a bad icon there is the plan's defect rather than the implementer's. If Step 5 is skipped, this feature can ship nine well-formed paths that draw nonsense and every test will still pass.
