# Media and Math on Flashcards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render referenced media and typeset LaTeX on the front and back of a flashcard in Study mode's Recall focus.

**Architecture:** Frontend only. `NodeCard`'s local `mediaTokens` helper moves into `frontend/src/lib/n4l-media.js` beside `mediaKind` and `srcFor`, joined by a new `stripMediaTokens`; both are pure, so both get `node --test` coverage the local copy never had. `RecallCard.svelte` then renders its front and back through the existing `MathText` and `InlineMedia` components, with the media path token removed from the text rather than left beside the media.

**Tech Stack:** Svelte 3.49, Vite 3, `node --test` for the pure JS. KaTeX and the `/collection/` asset route already ship — nothing is added to `package.json` or `go.mod`.

**Spec:** `docs/superpowers/specs/2026-08-12-recall-media-math-design.md`

## Global Constraints

- **No Go changes.** No new Wails binding, no schema change, no change to card generation, scheduling or grading. The whole feature is three frontend files plus a test file and a doc.
- **No new dependencies.** `katex` and `n4l-media.js` are already in the tree.
- **`NodeCard`'s rendered output must not change.** It imports the shared helper instead of defining its own; the Read tab keeps showing a media path *beside* its media.
- **Media detection is by file extension only** — `mediaKind` decides, never an arrow name, never a directory name. Media may live anywhere the collection can reach, not only `media/`.
- **No task in this feature introduces a new `{@html}`.** Card text is author input and must reach the DOM through ordinary Svelte interpolation or through KaTeX's own output inside `MathText` — the only `{@html}` this feature touches. (`WebSourcePane.svelte` has its own, pre-existing and out of scope here.)
- **The card back stays inside `{#if revealed}`**, so an answer's image is not fetched, and cannot flash on screen, before the card is turned.
- **Pure logic goes in `frontend/src/lib/`, not in a component.** This repo has no Svelte component harness — `make test-js` runs `node --test frontend/src/lib/*.test.js`. A module that imports a `.svelte` file, `katex`, or `wailsjs` cannot load there.
- **Every test must be proven able to fail.** Each test step below names the production change that breaks it; make that change, watch the test fail for that reason, then revert it. A test that passes against a gutted implementation is worse than no test.

---

### Task 1: Shared media-token helpers

Move the token scanner out of `NodeCard` into the tested lib, and add the stripper the card needs.

**Files:**
- Modify: `frontend/src/lib/n4l-media.js` (append two exports)
- Modify: `frontend/src/lib/n4l-media.test.js` (append tests)
- Modify: `frontend/src/components/NodeCard.svelte:4` (import) and `:15-20` (delete the local function)

**Interfaces:**
- Consumes: `mediaKind(path) -> 'image'|'video'|'audio'|null`, already exported from the same module.
- Produces, for Task 2:
  - `mediaTokens(text) -> string[]` — the whitespace-delimited tokens of `text` that `mediaKind` recognises, in order, duplicates kept.
  - `stripMediaTokens(text) -> string` — `text` with those tokens removed. Returns `text` **unchanged** when it contains no media token.

- [ ] **Step 1: Write the failing tests**

Append to `frontend/src/lib/n4l-media.test.js`. Update the import on line 3 to
`import { mediaKind, srcFor, mediaTokens, stripMediaTokens } from './n4l-media.js'`.

```js
test('mediaTokens finds a path that is the whole string', () => {
  assert.deepEqual(mediaTokens('media/apple.jpg'), ['media/apple.jpg'])
})

test('mediaTokens finds a path surrounded by prose', () => {
  assert.deepEqual(mediaTokens('the fruit media/apple.jpg in profile'), ['media/apple.jpg'])
})

test('mediaTokens finds every path in the string, in order', () => {
  assert.deepEqual(
    mediaTokens('media/a.png then clips/b.mp4 then sound/c.mp3'),
    ['media/a.png', 'clips/b.mp4', 'sound/c.mp3'],
  )
})

test('mediaTokens ignores tokens that are not media', () => {
  assert.deepEqual(mediaTokens('see notes.txt and archive.zip'), [])
})

test('mediaTokens returns empty for prose, empty and nullish input', () => {
  assert.deepEqual(mediaTokens('just some prose'), [])
  assert.deepEqual(mediaTokens(''), [])
  assert.deepEqual(mediaTokens(null), [])
  assert.deepEqual(mediaTokens(undefined), [])
})

test('stripMediaTokens removes a path from the middle without doubling the space', () => {
  assert.equal(stripMediaTokens('the fruit media/apple.jpg in profile'), 'the fruit in profile')
})

test('stripMediaTokens leaves no leading or trailing space', () => {
  assert.equal(stripMediaTokens('media/apple.jpg is a fruit'), 'is a fruit')
  assert.equal(stripMediaTokens('a fruit media/apple.jpg'), 'a fruit')
})

test('stripMediaTokens empties a string that is only a path', () => {
  assert.equal(stripMediaTokens('media/apple.jpg'), '')
  assert.equal(stripMediaTokens('  clips/b.mp4  '), '')
})

test('stripMediaTokens removes every path', () => {
  assert.equal(stripMediaTokens('x media/a.png y clips/b.mp4 z'), 'x y z')
})

// The common card has no media at all. Reformatting its whitespace — collapsing
// the spacing inside a formula, say — would be a visible change to text this
// function has no business touching.
test('stripMediaTokens returns text with no media byte-for-byte unchanged', () => {
  assert.equal(stripMediaTokens('a  double  spaced  line '), 'a  double  spaced  line ')
  assert.equal(stripMediaTokens('\\( E = mc^2 \\)'), '\\( E = mc^2 \\)')
})

test('stripMediaTokens handles empty and nullish input', () => {
  assert.equal(stripMediaTokens(''), '')
  assert.equal(stripMediaTokens(null), '')
  assert.equal(stripMediaTokens(undefined), '')
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test-js`
Expected: FAIL — `TypeError: mediaTokens is not a function` (and the same for `stripMediaTokens`), because neither export exists yet.

- [ ] **Step 3: Write the implementation**

Append to `frontend/src/lib/n4l-media.js`:

```js
/**
 * The whitespace-delimited tokens of `text` that are media paths.
 *
 * Lived in NodeCard until the Recall panel needed it too. In a component it had
 * no tests at all — this repo runs `node --test` over this directory only.
 */
export function mediaTokens(text) {
  return String(text ?? '').split(/\s+/).filter((t) => mediaKind(t))
}

/**
 * `text` with its media paths removed, for surfaces that show the media in the
 * path's place rather than beside it.
 *
 * Text containing no media is returned untouched rather than re-joined: almost
 * every card is that case, and silently normalising its whitespace would edit
 * strings this function was not asked to change.
 */
export function stripMediaTokens(text) {
  const s = String(text ?? '')
  const parts = s.split(/\s+/)
  if (!parts.some((t) => mediaKind(t))) return s
  return parts.filter((t) => t && !mediaKind(t)).join(' ')
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS, all tests in the file, no warnings.

- [ ] **Step 5: Prove the tests can fail**

Three separate controls. Make each change, run `make test-js`, confirm the named test fails, then revert it before the next one.

1. In `mediaTokens`, change the filter to `.filter((t) => t)`.
   Expected: `mediaTokens ignores tokens that are not media` fails — it returns `['see','notes.txt','and','archive.zip']`.
2. In `stripMediaTokens`, delete the `if (!parts.some(...)) return s` line.
   Expected: `stripMediaTokens returns text with no media byte-for-byte unchanged` fails — the double spacing collapses to `'a double spaced line'`.
3. In `stripMediaTokens`, change the filter to `.filter((t) => t)`.
   Expected: `stripMediaTokens removes a path from the middle without doubling the space` fails — the path is still there.

If any control passes, the test is not testing what it claims. Fix the test before continuing.

- [ ] **Step 6: Point NodeCard at the shared helper**

In `frontend/src/components/NodeCard.svelte`, change the import on line 4 to:

```js
  import { mediaTokens } from '../lib/n4l-media.js'
```

and delete lines 15-20 — the comment block and the local `function mediaTokens`. Nothing in the markup changes: `{#each mediaTokens(node.Text) as token}` now calls the import.

`mediaKind` is no longer referenced directly in this component; make sure it is not left in the import list.

- [ ] **Step 7: Verify the Read tab is untouched**

Run: `cd frontend && npx vite build`
Expected: builds with no errors and no unused-import warnings.

The two implementations are identical apart from the `String()` coercion, so the Read tab's output cannot have changed. Confirm by reading the diff of `NodeCard.svelte`: it must contain only the import line and the deleted function — no change inside `<div class="node-card">` and no change to the `<style>` block.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/lib/n4l-media.js frontend/src/lib/n4l-media.test.js frontend/src/components/NodeCard.svelte
git commit -m "refactor(media): move mediaTokens into the tested lib, add stripMediaTokens"
```

---

### Task 2: Render media and math on the card

**Files:**
- Modify: `frontend/src/components/RecallCard.svelte` — imports, the `.rc-front` and `.rc-back` markup, and the `<style>` block

**Interfaces:**
- Consumes: `mediaTokens`, `stripMediaTokens` from Task 1; `MathText` (`export let text`) and `InlineMedia` (`export let path`), both already in `frontend/src/components/`.
- Produces: nothing — this is the leaf of the feature.

There is no component test harness, so this task is verified by a build plus the manual pass in Task 3. That makes reading the diff carefully the real gate.

- [ ] **Step 1: Add the imports**

At the top of the `<script>` block in `frontend/src/components/RecallCard.svelte`, above `export let card`:

```js
  import InlineMedia from './InlineMedia.svelte'
  import MathText from './MathText.svelte'
  import { mediaTokens, stripMediaTokens } from '../lib/n4l-media.js'
```

- [ ] **Step 2: Render the front and back**

Replace lines 29-30:

```svelte
      <div class="rc-front">{card.front}</div>
      {#if revealed}<hr /><div class="rc-back">{card.back}</div>{/if}
```

with:

```svelte
      <div class="rc-front">
        <MathText text={stripMediaTokens(card.front)} />
        {#each mediaTokens(card.front) as token}<InlineMedia path={token} />{/each}
      </div>
      {#if revealed}
        <hr />
        <div class="rc-back">
          <MathText text={stripMediaTokens(card.back)} />
          {#each mediaTokens(card.back) as token}<InlineMedia path={token} />{/each}
        </div>
      {/if}
```

The back stays inside `{#if revealed}`: Svelte does not create those elements until the block is entered, so the answer's `<img>` has no `src` to fetch and cannot appear before the card is turned. Do not hoist it out and hide it with CSS.

- [ ] **Step 3: Centre the media on the card**

`.rc-card` is `text-align: center`, but `InlineMedia` renders `display: block` with `margin: 6px 0`, so an image would sit hard against the left edge. Svelte's scoped styles do not reach into a child component, so this needs `:global`. Add to the `<style>` block:

```css
  .rc-card :global(.inline-media),
  .rc-card :global(.inline-audio) { margin: 6px auto; }
```

Scope it under `.rc-card` — a bare `:global(.inline-media)` would leak out of this component and re-centre the Read tab's media too.

- [ ] **Step 4: Verify the build**

Run: `cd frontend && npx vite build`
Expected: builds with no errors.

Then read the diff of `RecallCard.svelte` and confirm all of:
- `card.front` and `card.back` appear only inside `stripMediaTokens(...)` and `mediaTokens(...)` — neither is interpolated raw any more, and neither reaches `{@html}`.
- The `{#if revealed}` boundary still encloses the whole back, `<hr />` included.
- `onKey`, the `GRADES` list, the reveal button and the grade buttons are unchanged.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/RecallCard.svelte
git commit -m "feat(recall): render media and typeset math on flashcards"
```

---

### Task 3: Manual verification

The two preceding tasks have no automated coverage of what the user actually sees. This task creates the test content, runs the app, and records the result.

**Files:**
- Create: `docs/superpowers/specs/2026-08-12-recall-media-math-manual-test-plan.md`
- Create (in the collection under test, not in this repo): a scratch `.n4l` file with the cards below

**Interfaces:**
- Consumes: the built app from Tasks 1 and 2.
- Produces: a filled-in test plan with a pass/fail per case.

- [ ] **Step 1: Write the test content**

The collection is at `/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`. Create `recall-render-check.n4l` there, in whatever chapter/section form that collection already uses (open a neighbouring file and copy its header shape — do not invent one). The card-bearing lines:

```
\( E = mc^2 \) >> mass–energy equivalence

the Bohr radius >> \( a_0 = \frac{4\pi\varepsilon_0\hbar^2}{m_e e^2} \)

what does this fruit look like >> media/apple.jpg

\( \nabla \cdot \mathbf{B} = 0 \) >> media/apple.jpg

the Gauss law for magnetism is {{ \( \nabla \cdot \mathbf{B} = 0 \) }}

an unclosed formula \( x + 1 >> renders as prose
```

Use a media filename that actually exists in that collection — list the directory and substitute a real one rather than assuming `apple.jpg` is there.

- [ ] **Step 2: Build and run**

```bash
make build && ./build/bin/texere
```

Open the collection, run the notes through whatever step generates cards (Study → the processing/sync path already in the app), then open **Study → Recall**.

- [ ] **Step 3: Check each case**

| # | Card | Expect |
|---|---|---|
| 1 | `E = mc^2` front | Front typeset, not `\( … \)` source |
| 2 | Bohr radius | Back typeset, fraction stacked, revealed only after Show answer |
| 3 | fruit → image | Front reads as prose; back shows the image and **no path text** |
| 4 | formula → image | Front typeset, back is the image alone |
| 5 | cloze with a formula | The `{{ }}` are gone, the formula inside still typesets |
| 6 | unclosed `\(` | Renders as literal prose; the card is not blank or broken |
| 7 | any ordinary text-only card | Identical to before this change |
| 8 | a card whose media file is missing | Broken-image state only; the card and its buttons still work |

- [ ] **Step 4: Verify the answer does not leak**

On card 3, before pressing Show answer, open the webview devtools (right-click → Inspect, or the dev build's inspector) and confirm there is no `<img>` in the DOM for the back, and no request to `/collection/…` in the Network tab. Press Show answer and confirm both appear only then.

This is the one property that a screenshot cannot show and that a mistake here would silently break.

- [ ] **Step 5: Check the keyboard path still works**

Space reveals, `1`-`4` grade, and the next card arrives with `revealed` reset — a card with media must not appear pre-revealed.

- [ ] **Step 6: Write up the result**

Record each case above with a pass/fail and a one-line observation in
`docs/superpowers/specs/2026-08-12-recall-media-math-manual-test-plan.md`, following the shape of `2026-08-06-study-sources-sidebar-manual-test-plan.md`. A failure is a finding, not something to smooth over in the write-up.

- [ ] **Step 7: Remove the scratch notes**

Delete `recall-render-check.n4l` from the collection so it does not pollute the user's real reading queue, and note in the test plan that it was removed.

- [ ] **Step 8: Commit**

```bash
git add docs/superpowers/specs/2026-08-12-recall-media-math-manual-test-plan.md
git commit -m "docs(test-plan): recall card media and math"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| `mediaTokens` / `stripMediaTokens` in `n4l-media.js`, `node --test` covered | 1 |
| `NodeCard` imports the shared helper, behaviour unchanged | 1, steps 6-7 |
| `RecallCard` renders front and back through `MathText` | 2 |
| Media token replaced by its media, not appended | 2 (`stripMediaTokens` on the text) |
| Back media stays inside `{#if revealed}` | 2 step 2, verified 3 step 4 |
| Error handling: missing file, malformed LaTeX, no media and no math | 3 step 3, cases 6-8 |
| Testing: JS unit cases | 1 step 1 |
| Testing: manual test plan document | 3 |
| No Go changes, no new dependencies | Global Constraints |

**Type consistency:** `mediaTokens` and `stripMediaTokens` keep the same names and signatures in Task 1's implementation, Task 1's tests, Task 2's imports and Task 2's markup. `card.front` / `card.back` are lowercase, matching the existing `RecallCard` and the `CardView` the Wails binding returns. `MathText` takes `text`; `InlineMedia` takes `path`.

**Scope:** three files and two docs. Small enough for one plan.
