# Editor Theming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The CodeMirror editor follows the app's theme — chrome and syntax highlighting both.

**Architecture:** Pure CSS-variable substitution in two library modules. `EditorView.theme` and `HighlightStyle.define` both emit CSS text and pass `var(…)` through untouched, so the editor rethemes through the cascade with no subscription, no rebuild and no JavaScript threading. Syntax colours get a `--n4l-syntax-*` namespace; Blue keeps its exact values, Dark and Light are Solarized-derived and adjusted to clear AA.

**Tech Stack:** Svelte 3.49, CodeMirror 6, Vite 3, `node --test`. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-08-17-editor-theme-design.md`

## Global Constraints

- **Blue must render exactly as it does today.** Every Blue token value is byte-identical to the literal it replaces. This is the constraint the whole themes migration held and it holds here.
- **No new dependencies. No Go changes. No component changes** — this touches two files under `frontend/src/lib/`, plus `style.css`, plus tests and a doc.
- **Syntax roles live in `--n4l-syntax-*`, chrome reuses the existing role tokens.** Mint a chrome role only where no existing role's Blue value is byte-identical to the literal being replaced. Do not reuse a role whose value merely looks close — near-identical literals stay distinct, as throughout this migration.
- **Every syntax role must exist in all three theme blocks.** Verify by parsing `style.css`, not by eye.
- **Every Light and Dark syntax value must measure ≥4.5:1** against that theme's editor background (`--surface`: `#f6f7f9` Light, `#16171a` Dark). Assert this in the test suite with a contrast function — do not trust the table in this plan.
- **Provenance and `# comment` stay visually distinct**, with comment the dimmer of the two in every theme. Blue renders them `#6b7280` and `#4b5563`; a naive Solarized mapping collapses both to `base1`.
- **Every test must be proven able to fail.** Each test step names the production change that breaks it; make that change, watch it fail for that reason, then revert, and record the captured output. This project has twice caught negative controls that could not have produced the failure they claimed — capture from real runs.

---

### Task 1: The syntax token layer

**Files:**
- Modify: `frontend/src/style.css` (three theme blocks)
- Modify: `frontend/src/lib/n4l-highlight.js:21-30`
- Create: `frontend/src/lib/n4l-syntax.test.js`

**Interfaces:**
- Produces, for Task 2 and for the manual pass: ten CSS roles named `--n4l-syntax-heading`, `-keyword`, `-arrow-code`, `-arrow-target`, `-ditto`, `-marker`, `-provenance`, `-label`, `-comment`, `-invalid`.

The role names are deliberately N4L's vocabulary rather than CodeMirror's tag names: `arrow-code` and `arrow-target` say what they mean to someone reading a note, where `operatorKeyword` and `string` do not.

- [ ] **Step 1: Write the failing tests**

Create `frontend/src/lib/n4l-syntax.test.js`. This suite reads `style.css` as text and inspects the CSS that `HighlightStyle` generates — both are reachable from `node --test`, which is what makes this feature testable at all.

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { n4lHighlightStyle } from './n4l-highlight.js'

const CSS = readFileSync(new URL('../style.css', import.meta.url), 'utf8')

const ROLES = [
  'heading', 'keyword', 'arrow-code', 'arrow-target', 'ditto',
  'marker', 'provenance', 'label', 'comment', 'invalid',
]

// Blue is today's palette and must not shift by a single byte.
const BLUE = {
  heading: '#7dd3fc', keyword: '#c084fc', 'arrow-code': '#67e8f9',
  'arrow-target': '#86efac', ditto: '#94a3b8', marker: '#fde68a',
  provenance: '#6b7280', label: '#a78bfa', comment: '#4b5563', invalid: '#f59e0b',
}

function block(theme) {
  const re = theme === 'blue'
    ? /:root\s*\{([\s\S]*?)\n\}/
    : new RegExp(`:root\\[data-theme="${theme}"\\]\\s*\\{([\\s\\S]*?)\\n\\}`)
  const m = CSS.match(re)
  assert.ok(m, `no ${theme} block in style.css`)
  return m[1]
}

function value(theme, role) {
  const m = block(theme).match(new RegExp(`--n4l-syntax-${role}\\s*:\\s*(#[0-9a-fA-F]{3,8})\\s*;`))
  return m && m[1].toLowerCase()
}

function ratio(a, b) {
  const lum = (h) => {
    const c = [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255)
      .map((v) => (v > 0.03928 ? ((v + 0.055) / 1.055) ** 2.4 : v / 12.92))
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]
  }
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p)
  return (x + 0.05) / (y + 0.05)
}

test('every syntax role is defined in all three themes', () => {
  for (const theme of ['blue', 'dark', 'light']) {
    for (const role of ROLES) {
      assert.ok(value(theme, role), `--n4l-syntax-${role} missing from ${theme}`)
    }
  }
})

// The migration's standing constraint: Blue does not move.
test('Blue keeps the exact values it has today', () => {
  for (const role of ROLES) {
    assert.equal(value('blue', role), BLUE[role], role)
  }
})

test('every Light and Dark syntax colour clears AA against its editor background', () => {
  const bg = { light: '#f6f7f9', dark: '#16171a' }
  for (const theme of ['light', 'dark']) {
    for (const role of ROLES) {
      const r = ratio(value(theme, role), bg[theme])
      assert.ok(r >= 4.5, `${theme} ${role} is ${r.toFixed(2)}:1, want >= 4.5`)
    }
  }
})

// Blue renders these as two distinct greys, comment the dimmer. A naive
// Solarized mapping sends both to base1 and loses the distinction.
test('provenance and comment stay distinct, comment dimmer', () => {
  const bg = { blue: '#1b2636', light: '#f6f7f9', dark: '#16171a' }
  for (const theme of ['blue', 'light', 'dark']) {
    const p = value(theme, 'provenance'), c = value(theme, 'comment')
    assert.notEqual(p, c, `${theme}: provenance and comment are the same colour`)
    assert.ok(ratio(c, bg[theme]) < ratio(p, bg[theme]),
      `${theme}: comment should be dimmer than provenance`)
  }
})

// The gate no visual check would reliably catch: the generated CSS must carry
// no literal colour at all.
test('the highlight style emits only var() colours', () => {
  const rules = n4lHighlightStyle.module.rules.join('\n')
  assert.match(rules, /var\(--n4l-syntax-/)
  assert.doesNotMatch(rules, /#[0-9a-fA-F]{3,8}/, `literal colour in generated CSS:\n${rules}`)
})
```

Note the last test needs `n4lHighlightStyle` exported from `n4l-highlight.js`. It currently is not — export it alongside `n4lHighlighting`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test-js`
Expected: FAIL — the roles do not exist in `style.css`, and `n4lHighlightStyle` is not exported.

- [ ] **Step 3: Add the roles to `style.css`**

Blue takes today's exact values. Light and Dark take the derived Solarized values below, each already measured ≥4.60:1 against that theme's editor background:

```css
/* :root — Blue */
--n4l-syntax-heading: #7dd3fc;
--n4l-syntax-keyword: #c084fc;
--n4l-syntax-arrow-code: #67e8f9;
--n4l-syntax-arrow-target: #86efac;
--n4l-syntax-ditto: #94a3b8;
--n4l-syntax-marker: #fde68a;
--n4l-syntax-provenance: #6b7280;
--n4l-syntax-label: #a78bfa;
--n4l-syntax-comment: #4b5563;
--n4l-syntax-invalid: #f59e0b;
```

| role | Light | Dark |
|---|---|---|
| heading | `#2075b1` | `#268bd2` |
| keyword | `#6267c0` | `#767bc8` |
| arrow-code | `#207c75` | `#2aa198` |
| arrow-target | `#687700` | `#859900` |
| ditto | `#586e75` | `#6b868e` |
| marker | `#8e6b00` | `#b58900` |
| label | `#cd2d7b` | `#d84e91` |
| invalid | `#c34815` | `#e05318` |

**`provenance` and `comment` are yours to derive.** Both come from Solarized's grey ramp (`base01 #586e75`, `base1 #93a1a1`), both must clear 4.5:1, and comment must stay dimmer than provenance. Note that "dimmer" inverts between themes: on Light a dimmer grey is *lighter*, on Dark it is *darker*, and both still have to clear the bar — that is the constraint that makes this the one row worth deriving rather than tabulating. Report the two values you chose with their measured ratios.

Add a comment above the Blue block recording that Dark and Light are Solarized-derived rather than Solarized, and why: Solarized's published accents measure 2.48:1 to 4.30:1 against its own `base3` background, so the hues were kept and the lightness moved until each role cleared AA.

- [ ] **Step 4: Point the highlight style at the roles**

In `frontend/src/lib/n4l-highlight.js`, each tag's `color` becomes its `var(--n4l-syntax-…)`, and `n4lHighlightStyle` gains an `export`. Update the palette comment at the top of the file — it currently documents the Blue hex values as if they were the whole story.

Tag-to-role mapping, from the existing definitions: `heading`→heading, `keyword`→keyword, `operatorKeyword`→arrow-code, `string`→arrow-target, `meta`→ditto, `annotation`→marker, `comment`→provenance, `labelName`→label, `lineComment`→comment, `invalid`→invalid.

Note the trap in that mapping: CodeMirror's `comment` tag is N4L's **provenance**, and CodeMirror's `lineComment` is N4L's **`# comment`**. Getting them the wrong way round swaps two greys and nothing will look obviously broken.

`fontWeight` and `fontStyle` on each tag stay exactly as they are.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS, output pristine.

- [ ] **Step 6: Prove the tests can fail**

Four controls. Make each, run `make test-js`, confirm the named test fails for the named reason, revert before the next.

1. Change Blue's `--n4l-syntax-heading` to `#7dd3fb` (one byte).
   Expected: `Blue keeps the exact values it has today` fails.
2. Delete `--n4l-syntax-marker` from the Light block.
   Expected: `every syntax role is defined in all three themes` fails.
3. Set Light's `--n4l-syntax-arrow-target` back to Solarized's published `#859900`.
   Expected: `every Light and Dark syntax colour clears AA` fails at about 2.99:1. This control also demonstrates *why* the palette was adjusted.
4. In `n4l-highlight.js`, put one tag's literal back — `{ tag: tags.heading, color: '#7dd3fc' }`.
   Expected: `the highlight style emits only var() colours` fails, printing the offending rule.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/style.css frontend/src/lib/n4l-highlight.js frontend/src/lib/n4l-syntax.test.js
git commit -m "feat(editor): theme the N4L syntax highlighting"
```

---

### Task 2: The editor chrome

**Files:**
- Modify: `frontend/src/lib/n4l-editor.js:19-42` (`n4lTheme`)
- Modify: `frontend/src/style.css` if a chrome role must be minted
- Modify: `frontend/src/lib/n4l-syntax.test.js` (one added test)

**Interfaces:**
- Consumes: the role tokens in `style.css`, including anything Task 1 added.
- Produces: nothing — this is the leaf.

- [ ] **Step 1: Write the failing test**

Append to `frontend/src/lib/n4l-syntax.test.js`:

```js
import { n4lTheme } from './n4l-editor.js'

// Same gate as the highlight style: the editor's chrome must emit no literal
// colour. Note this asserts on colour literals only — lengths, radii and the
// box-shadow's rgba() are not colours in a theme role's sense.
test('the editor theme emits only var() colours', () => {
  const rules = n4lTheme().module.rules.join('\n')
  assert.match(rules, /var\(--/)
  const literals = rules.match(/#[0-9a-fA-F]{3,8}\b/g) ?? []
  assert.deepEqual(literals, [], `literal colours in editor chrome: ${literals.join(', ')}`)
})
```

If `n4lTheme()` does not expose `.module.rules`, find the equivalent by inspecting the returned extension — the probe in the spec confirms the generated rules are reachable; adapt the accessor rather than weakening the assertion.

- [ ] **Step 2: Run it to verify it fails**

Run: `make test-js`
Expected: FAIL, listing the 22 literals currently in `n4lTheme`.

- [ ] **Step 3: Migrate the chrome**

Replace each literal in `n4lTheme` with the role whose Blue value is **byte-identical**. From the current definitions, the mapping is mostly direct — `#1b2636`→`--surface`, `#e8eaf0`→`--text`, `#161f2e`→`--surface-bar`, `#2a3a4a`→`--border`, `#7dd3fc`→`--accent-text`, `#1e3a5a`→`--accent`, `#1a2535`→`--surface-overlay`, `#8899aa`→`--text-muted`, `#5a7a9a`→`--accent-dim`, `#b0bec5`→`--text-row`, `#67e8f9`→`--n4l-syntax-arrow-code`.

Several have no byte-identical role — `#1d2e42` (active line and its gutter), `#854d0e` (search match), `#4a5a6a` (gutter text), `#141d2b` (completion section header). **Mint a role for each in all three theme blocks** rather than approximating: this migration's rule is that a near-match is not a match, because using one changes how Blue renders.

The `boxShadow`'s `rgba(0,0,0,0.55)` is not a themed colour and stays as it is — a drop shadow is dark in every theme. Say so in your report rather than leaving it unexplained.

- [ ] **Step 4: Run the tests**

Run: `make test-js`
Expected: PASS, all suites.

- [ ] **Step 5: Prove the new test can fail**

Put one chrome literal back — `'&': { height: '100%', background: '#1b2636' }`.
Expected: `the editor theme emits only var() colours` fails, naming `#1b2636`.

- [ ] **Step 6: Verify Blue is unchanged**

Run `cd frontend && npx vite build` — clean, and paste the warning list.

Then confirm every substitution is byte-identical: for each role you used, check its Blue value in `style.css` equals the literal it replaced. Write this as a throwaway script over your diff and paste its output. Every role minted must exist in all three blocks — verify by parsing.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/n4l-editor.js frontend/src/style.css frontend/src/lib/n4l-syntax.test.js
git commit -m "feat(editor): theme the editor chrome"
```

---

### Task 3: Manual verification

**Files:**
- Create: `docs/superpowers/specs/2026-08-17-editor-theme-manual-test-plan.md`

Dark and Light are gated out of the picker (`frontend/src/lib/themes.js` marks them unavailable), so you cannot select them through the UI. Set the theme by hand — `document.documentElement.dataset.theme = 'light'` is not reachable without devtools, so edit `~/.config/texere/config.json`'s `theme` field between launches, which the store honours on startup. Record the file's original contents and restore them byte-identically when you finish.

- [ ] **Step 1: Build and run against a copy of the collection**

Copy the collection to `"$CLAUDE_JOB_DIR/tmp/collection-editor"` and open the copy, so nothing touches the user's notes or `learning.db`. Build with `make build`.

- [ ] **Step 2: Check every editor, in every theme**

Four components build editors, and each is reached differently: `SourceTab.svelte` (the Note tab), `ReadingView.svelte` (Study → Read), `notebook/N4LDraftCell.svelte` (a draft cell in the notebook), `ProcessEditor.svelte` (Study → Process). Open all four in Blue, Dark and Light.

Use a note that exercises the grammar — a chapter heading, a `::` context line, arrows with codes and targets, a ditto, a `# @card` marker, an `@label`, a `(from)` provenance line, a `#` comment and an ALL CAPS reminder — so every one of the ten syntax roles actually renders. If no note in the collection has all of them, add one to the copy.

- [ ] **Step 3: The two surfaces that only appear on interaction**

Trigger the **autocomplete tooltip** (type an arrow code) and the **vim command line** (in an editor with vim enabled) in each theme. These are the easiest to miss, because they are invisible until you act.

- [ ] **Step 4: Confirm Blue is untouched**

Compare the Note tab in Blue against a build of the commit before Task 1, built in a temporary `git worktree` in a separate directory — never move HEAD on this checkout, and remove the worktree when done. The editor should be pixel-identical.

- [ ] **Step 5: Read the colours back, do not just look**

For each theme, screenshot an editor showing all ten roles and **read the screenshots back**. Sample the actual rendered pixel colour of each syntax role and compare it to the token's value in `style.css`. A role that silently falls back to CodeMirror's default will look plausible and be wrong — sampling catches that where an impression will not.

- [ ] **Step 6: Write up the result**

Record every case with a pass/fail and a one-line observation, following the shape of `docs/superpowers/specs/2026-08-12-node-filters-manual-test-plan.md`. A failure is a finding. Note the collection copy was left in the job directory.

- [ ] **Step 7: Commit**

```bash
git add docs/superpowers/specs/2026-08-17-editor-theme-manual-test-plan.md
git commit -m "docs(test-plan): editor theming"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Ten `--n4l-syntax-*` roles, three themes | 1 |
| Blue unchanged | 1 (test + control 1), 2 step 6, 3 step 4 |
| Light and Dark ≥4.5:1, asserted not trusted | 1 (test + control 3) |
| Provenance and comment distinct, comment dimmer | 1 (test + step 3) |
| Highlight style emits only `var()` | 1 (test + control 4) |
| Chrome reuses existing roles; mint only on no byte-identical match | 2 step 3 |
| Chrome emits only `var()` | 2 (test + control 5) |
| Editor rethemes through the cascade, no JS threading | Architecture; verified 3 step 2 |
| All four editor components, all three themes | 3 step 2 |
| Autocomplete tooltip and vim panel | 3 step 3 |
| No Go changes, no new dependencies | Global Constraints |

**Type consistency:** the ten role names are identical in the spec table, Task 1's `ROLES` array, Task 1's `BLUE` map, the `style.css` declarations and the `var()` references in `n4l-highlight.js`. `n4lHighlightStyle` and `n4lTheme` are the two exports the tests import.

**Known risk this plan accepts:** the `.module.rules` accessor used by both gate tests is CodeMirror's internal shape, not a documented API, and could change across a major version. That is acceptable — the dependency is pinned, and the test failing loudly on an upgrade is a better outcome than having no gate at all. Task 2 step 1 tells the implementer to adapt the accessor rather than weaken the assertion if it differs.
