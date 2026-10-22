# Read Tab Math Implementation Plan

**Goal:** In the Read tab, LaTeX delimited by `\(…\)` or `\[…\]` renders as typeset mathematics, in every note, with no per-note setting.

**Architecture:** A pure function splits a string into prose and math segments. A small component renders prose as text and math through KaTeX. `NodeCard` routes its three raw interpolations through it.

**Tech Stack:** Svelte 3.59, KaTeX (installed in Task 2; the registry currently serves 0.18.4), `node --test`.

## Global Constraints

- **`$` is NEVER a math delimiter.** All 89 `$` occurrences in the real collection are N4L variable references — `$user1.1`, `$question.1`. Supporting `$…$`, which most renderers do by default, would corrupt every one of them. Only `\(…\)` and `\[…\]` are math.
- **`n4l-math.js` must not import KaTeX.** That is what keeps it runnable under `node --test`, the only automated coverage this feature can have.
- **An unclosed delimiter renders as prose**, never swallowing the rest of the string. An unterminated-quote bug of exactly this shape has already been shipped once in this area.
- **Only KaTeX's own output may bypass escaping.** Prose segments go through ordinary Svelte interpolation and stay escaped. Never pass raw note text to `{@html}`.
- **KaTeX runs with `throwOnError: false`** so malformed LaTeX renders in red rather than breaking the card.
- **Read tab only.** `NodeCard` is reached only from `ReadTab.svelte`; do not wire math into any other component.
- **Do not change `InlineMedia` or the media wiring.** It sits beside `arrow.Target` and is unrelated.
- JS/Svelte: 2-space indent, **no semicolons**, single quotes.
- Run `make test-js` and `cd frontend && npx vite build` before each commit.

## MANDATORY: negative-control every test you write

Across this codebase's recent branches, **nine** plausible-looking negative controls turned out to prove nothing — fixtures sampling only one side of a boundary, a regex sabotage that was not a sabotage, a containment check the standard library pre-empted. Every one was caught only because someone ran the control and read the output.

Before committing, for each behaviour asserted: break the production code in the specific way the test exists to catch, run the test, confirm it **fails with a message naming the real problem**, then restore and re-run green. Report the actual failure output.

- A sabotage that does not parse is **not** a control. Change it until it runs and the test genuinely fails.
- If a control does not make its test fail, **say so plainly** and name the assertion that was supposed to catch it. Do not weaken a test until something goes red.

## File Structure

- `frontend/src/lib/n4l-math.js` — splitting prose from math, dependency-free.
- `frontend/src/lib/n4l-math.test.js` — its tests.
- `frontend/src/components/MathText.svelte` — renders segments; the only `{@html}` in the feature.
- `frontend/src/components/NodeCard.svelte` — three interpolations routed through `MathText`.
- `frontend/package.json` — adds `katex`.
- `docs/specs/2026-08-11-read-tab-math-manual-test-plan.md`

---

### Task 1: Splitting prose from math

**Files:**
- Create: `frontend/src/lib/n4l-math.js`
- Create: `frontend/src/lib/n4l-math.test.js`

**Interfaces:**
- Consumes: nothing — dependency-free by design.
- Produces: `splitMath(text) -> [{ type: 'text' | 'inline' | 'display', value: string }]`

`value` for a math segment is the LaTeX **without** its delimiters, because that
is what KaTeX takes. For a text segment it is the prose verbatim.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/n4l-math.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { splitMath } from './n4l-math.js'

test('a string with no math is one text segment', () => {
  assert.deepEqual(splitMath('just some prose'), [{ type: 'text', value: 'just some prose' }])
})

test('inline math is extracted without its delimiters', () => {
  assert.deepEqual(splitMath('\\( x = 1 \\)'), [{ type: 'inline', value: ' x = 1 ' }])
})

test('display math is extracted without its delimiters', () => {
  assert.deepEqual(splitMath('\\[ y = 2 \\]'), [{ type: 'display', value: ' y = 2 ' }])
})

// The real shape in moon.n4l: prose and both kinds of math in one string.
test('prose and math interleave in order', () => {
  const got = splitMath('The momentum \\(L\\) obeys \\[ L = I\\omega \\] where \\(I\\) is inertia.')
  assert.deepEqual(got, [
    { type: 'text', value: 'The momentum ' },
    { type: 'inline', value: 'L' },
    { type: 'text', value: ' obeys ' },
    { type: 'display', value: ' L = I\\omega ' },
    { type: 'text', value: ' where ' },
    { type: 'inline', value: 'I' },
    { type: 'text', value: ' is inertia.' },
  ])
})

test('the real arrow target from moon.n4l', () => {
  assert.deepEqual(splitMath('\\( I = 8.1 \\times 10^{37} kgm^2\\)'),
    [{ type: 'inline', value: ' I = 8.1 \\times 10^{37} kgm^2' }])
})

// Braces inside a formula must not be mistaken for anything structural.
test('braces nested in a formula survive', () => {
  assert.deepEqual(splitMath('\\[ \\frac{mM}{m+M} \\]'),
    [{ type: 'display', value: ' \\frac{mM}{m+M} ' }])
})

// The bug this exists to prevent: an opener with no closer must not consume the
// rest of the string. An unterminated quote did exactly that once here.
test('an unclosed inline delimiter stays prose', () => {
  assert.deepEqual(splitMath('before \\( x = 1 and no closer'),
    [{ type: 'text', value: 'before \\( x = 1 and no closer' }])
})

test('an unclosed display delimiter stays prose', () => {
  assert.deepEqual(splitMath('before \\[ x = 1 and no closer'),
    [{ type: 'text', value: 'before \\[ x = 1 and no closer' }])
})

// $ introduces an N4L variable. Treating it as math would corrupt every
// variable reference in the collection.
test('a dollar variable is left completely alone', () => {
  assert.deepEqual(splitMath('Start (next) $question.1'),
    [{ type: 'text', value: 'Start (next) $question.1' }])
  assert.deepEqual(splitMath('$a.1 and $b.1'), [{ type: 'text', value: '$a.1 and $b.1' }])
})

test('an empty or missing string yields no segments', () => {
  assert.deepEqual(splitMath(''), [])
  assert.deepEqual(splitMath(null), [])
  assert.deepEqual(splitMath(undefined), [])
})

test('two formulas of the same kind in one string', () => {
  assert.deepEqual(splitMath('\\(a\\) then \\(b\\)'), [
    { type: 'inline', value: 'a' },
    { type: 'text', value: ' then ' },
    { type: 'inline', value: 'b' },
  ])
})

// An empty formula is degenerate but must not produce a broken segment.
test('an empty formula yields an empty value, not a missing segment', () => {
  assert.deepEqual(splitMath('\\(\\)'), [{ type: 'inline', value: '' }])
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/n4l-math.test.js`
Expected: FAIL — cannot resolve `./n4l-math.js`.

- [ ] **Step 3: Implement**

Create `frontend/src/lib/n4l-math.js`:

```js
// Splitting a note string into prose and LaTeX.
//
// Delimiters are `\(…\)` for inline and `\[…\]` for display, and nothing else.
// In particular `$…$` is NOT math: `$` introduces a variable in N4L, and every
// one of the 89 `$` occurrences in the real collection is a variable reference
// like `$user1.1`. Supporting the conventional dollar delimiter would corrupt
// all of them.
//
// Dependency-free — it must never import KaTeX — so it runs under `node --test`,
// which is the only automated coverage this feature can have.

const OPENERS = [
  { open: '\\(', close: '\\)', type: 'inline' },
  { open: '\\[', close: '\\]', type: 'display' },
]

/**
 * @param {string} text
 * @returns {{type: 'text'|'inline'|'display', value: string}[]}
 */
export function splitMath(text) {
  const s = String(text ?? '')
  if (!s) return []

  const out = []
  let prose = ''
  let i = 0

  while (i < s.length) {
    const d = OPENERS.find((o) => s.startsWith(o.open, i))
    if (!d) {
      prose += s[i]
      i++
      continue
    }
    const end = s.indexOf(d.close, i + d.open.length)
    if (end < 0) {
      // No closer: the opener is ordinary text. Consuming to end-of-string here
      // would swallow the rest of the note, which is the failure this guards.
      prose += s[i]
      i++
      continue
    }
    if (prose) { out.push({ type: 'text', value: prose }); prose = '' }
    out.push({ type: d.type, value: s.slice(i + d.open.length, end) })
    i = end + d.close.length
  }

  if (prose) out.push({ type: 'text', value: prose })
  return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `node --test frontend/src/lib/n4l-math.test.js` then `make test-js`
Expected: all PASS.

- [ ] **Step 5: Negative controls (report all three)**

1. In the no-closer branch, replace the two lines with
   `out.push({ type: d.type, value: s.slice(i + d.open.length) }); i = s.length`.
   Both unclosed-delimiter tests must FAIL. Restore.
2. Add `{ open: '$', close: '$', type: 'inline' }` to `OPENERS`. The dollar-variable
   test must FAIL, showing `$a.1 and $b.1` split into segments. Restore.
3. Change the math segment's value to `s.slice(i, end + d.close.length)`, i.e.
   keep the delimiters. The real-arrow-target test must FAIL. Restore.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/n4l-math.js frontend/src/lib/n4l-math.test.js
git commit -m "feat(read): split note text into prose and LaTeX segments"
```

---

### Task 2: Rendering the segments

**Files:**
- Create: `frontend/src/components/MathText.svelte`
- Modify: `frontend/package.json` (adds `katex`)

**Interfaces:**
- Consumes: `splitMath` (Task 1).
- Produces: a component taking `export let text`.

- [ ] **Step 1: Install KaTeX**

```bash
cd frontend && npm install --save katex
```

Confirm it landed in `dependencies`, not `devDependencies` — it ships in the
bundle:

```bash
node -p "require('./package.json').dependencies.katex"
```

- [ ] **Step 2: Create the component**

Create `frontend/src/components/MathText.svelte`:

```svelte
<script>
  // Renders a note string with its LaTeX typeset.
  //
  // This is the ONLY place in the feature where escaping is bypassed, and it is
  // bypassed only for a string KaTeX itself produced from the LaTeX source.
  // Prose segments go through ordinary interpolation and stay escaped — never
  // pass raw note text to {@html}.
  //
  // KaTeX defaults to trust:false, so it emits no scripts and no javascript:
  // links even from hostile input.
  import katex from 'katex'
  import 'katex/dist/katex.min.css'
  import { splitMath } from '../lib/n4l-math.js'

  export let text

  $: segments = splitMath(text)

  // throwOnError:false renders bad LaTeX in red with its source showing, rather
  // than throwing and taking the whole card down.
  const render = (value, displayMode) =>
    katex.renderToString(value, { displayMode, throwOnError: false })
</script>

{#each segments as seg}
  {#if seg.type === 'text'}{seg.value}
  {:else if seg.type === 'inline'}{@html render(seg.value, false)}
  {:else}{@html render(seg.value, true)}
  {/if}
{/each}
```

- [ ] **Step 3: Verify the build**

Run: `cd frontend && npx vite build`
Expected: succeeds. KaTeX's CSS and fonts are pulled in by the
`katex/dist/katex.min.css` import, which is what makes formulas render offline —
without it the glyphs fall back to browser defaults and look wrong rather than
failing loudly.

Then `make test-js`, which must stay green: this task adds no JS tests, because
`MathText` imports KaTeX and so cannot load under `node --test`. That is exactly
why `splitMath` is a separate module.

- [ ] **Step 4: Report the reasoning**

State what a user would see if each of these were wrong: `throwOnError` left at
its default, the CSS import omitted, and prose segments passed through `{@html}`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/MathText.svelte frontend/package.json frontend/package-lock.json
git commit -m "feat(read): render LaTeX segments with KaTeX"
```

---

### Task 3: Wiring it into the Read tab

**Files:**
- Modify: `frontend/src/components/NodeCard.svelte`

**Interfaces:**
- Consumes: `MathText` (Task 2).
- Produces: no new exports.

`NodeCard` currently interpolates three fields raw. All three carry math in the
real collection.

- [ ] **Step 1: Route the three interpolations through `MathText`**

Add the import beside the existing `InlineMedia` one:

```js
  import MathText from './MathText.svelte'
```

Then replace, at `:23`:

```svelte
  <p class="node-text"><MathText text={node.Text} /></p>
```

at `:36` and again at `:48` — both the node-arrow and the ditto-arrow branch:

```svelte
          <dd class="arrow-dd"><MathText text={arrow.Target} /><InlineMedia path={arrow.Target} /></dd>
```

and at `:42`:

```svelte
            <dd class="arrow-dd ditto-text"><MathText text={ditto.Text} /></dd>
```

**Leave `<InlineMedia>` exactly as it is.** A media path contains no math and a
formula is not a file, so the two never interact — but reordering or nesting
them would change the media layout for no reason.

Leave the `isReminder` test at `:12` alone: it inspects `node.Text` for an
all-caps reminder and is unrelated to rendering.

- [ ] **Step 2: Verify**

Run: `make test-js` and `cd frontend && npx vite build`
Expected: both succeed, with **no unused-CSS-selector warning naming
`.node-text`, `.arrow-dd` or `.ditto-text`** — such a warning would mean the
markup and styles no longer agree.

Then confirm the wiring and the scoping:

- `grep -c "MathText" frontend/src/components/NodeCard.svelte` — must be **5**: the import plus four call sites.
- `grep -rl "MathText" frontend/src/components/` — must list **only** `NodeCard.svelte` and `MathText.svelte`. Anything else means math leaked out of the Read tab.
- `grep -c "InlineMedia" frontend/src/components/NodeCard.svelte` — must still be **4**: the import, the node-text loop, and the two arrow branches.

- [ ] **Step 3: Check it against the real collection**

The file `moon.n4l` in the collection at
`/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test` is dense
with formulas and is the fixture the manual plan uses. Build and run:

```bash
make build && ./build/bin/texere
```

Open that file and switch to the **Read** tab. Report what you see for the
`Moment of Inertia of the Earth` row — this is the first point in the feature
where anything is observable.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/NodeCard.svelte
git commit -m "feat(read): typeset math in node text, ditto text and arrow targets"
```

---

### Task 4: Manual test plan

**Files:**
- Create: `docs/specs/2026-08-11-read-tab-math-manual-test-plan.md`

Match the house format of
`docs/specs/2026-08-11-read-tab-media-manual-test-plan.md`. Read the
shipped components before writing.

- [ ] **Step 1: Write the document**

Use `moon.n4l` as the fixture — it is real, already in the collection, and dense
with the constructs that matter. Cover:

1. An inline formula in an arrow target renders typeset, not as source —
   `Moment of Inertia of the Earth` is the row to look at.
2. Display math `\[…\]` renders centred on its own line.
3. Prose and math interleaved in one string render in order, with the prose
   still reading as a sentence.
4. The specific constructs present in the file all render correctly: `\times`,
   `10^{37}`, `\frac{mM}{m+M}`, `\sqrt{GM}`, `\dot\omega_E`, `{\rm TOT}`,
   `\mu`, `\omega`.
5. Malformed LaTeX — add `(has value) "\( \frac{1 \)"` to a note — renders in
   red showing its source, and **the rest of the card still renders**.
6. An unclosed delimiter — `(note) \( x = 1` with no closer — renders literally
   as text, and does not swallow anything after it.
7. **A note containing `$variables` renders them unchanged.** Any file using
   `$user1.1`-style references is the fixture. This is the regression check on
   the single most dangerous thing this feature could get wrong.
8. A note with no math at all renders exactly as it did before.
9. Media and math coexist: a card with both a `(pic)` target and a `(has value)`
   formula shows both.
10. The Note tab still shows raw LaTeX — the scoping check.
11. Formulas render with the machine offline, which is what bundling KaTeX's
    fonts buys.

State explicitly, so a tester does not file them: `$…$` is deliberately not a
math delimiter and never will be here; and there is no way to copy a formula's
source out of the rendered output.

- [ ] **Step 2: Commit**

```bash
git add docs/specs/2026-08-11-read-tab-math-manual-test-plan.md
git commit -m "docs(test-plan): LaTeX math in the Read tab"
```

---

## Self-Review

**Spec coverage.** `\(…\)` and `\[…\]` only, `$` excluded → Task 1, asserted by a
test and a negative control. Detection as the delimiters → `splitMath`'s
single-text-segment case. KaTeX bundled with fonts → Task 2's CSS import.
`throwOnError: false` → Task 2. Unclosed delimiter as prose → Task 1, two tests
and a control. Only KaTeX output through `{@html}` → Task 2's component
structure. Read tab only → Task 3's grep. The three raw interpolations → Task 3,
all four call sites enumerated.

**Placeholders.** None: every code step carries literal code and every test its
assertions. Task 4 is prose because it edits prose, and it enumerates each case.

**Type consistency.** `splitMath` returns `{type, value}` in Task 1 and is
destructured as `seg.type`/`seg.value` in Task 2. `MathText` takes `text` in
Task 2 and is called with `text={…}` at four sites in Task 3. `InlineMedia`'s
`path` prop is untouched.

**One thing worth flagging.** Task 3's grep expects `MathText` five times — one
import and four call sites — but `NodeCard` has only three *fields* carrying
math. The fourth is because `arrow.Target` appears twice, once for node arrows
and once for ditto arrows. A reader counting fields rather than call sites would
think the grep is wrong.
