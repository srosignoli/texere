# Inline Notes Media Implementation Plan

> **SUPERSEDED.** This plan built a CodeMirror implementation that was later
> deleted: media renders in the **Read tab** through `NodeCard`, not by
> decorating an editor. `n4l-media-view.js` and `findMedia` no longer exist.
> Accurate as a record of what was executed; see
> `docs/specs/2026-08-11-read-tab-media-design.md` for the shipped
> system.

**Goal:** In the reading notes pane, a quoted target naming an image, video or audio file renders as the media itself, below the line that references it.

**Architecture:** A dependency-free module detects media paths in a line. A CodeMirror `ViewPlugin` turns each match into a block widget that loads the file through the existing `GetSourceBytes` binding and shows it. The plugin reaches the notes editor through `n4lBaseExtensions`'s existing `extra` hook, which scopes it to that one editor without changing shared code.

**Tech Stack:** Svelte 3.59, CodeMirror 6 (`@codemirror/view`), Wails v2, `node --test`.

## Global Constraints

- **Detection is by the target's file extension, never by the arrow's name.** Arrow names are free text the author writes — `has an image` would work while `illustrated by` or a typo silently would not.
- **Media renders BELOW the line and never replaces it.** This pane is an editor: hiding the path makes a broken link invisible and unfixable. The text must remain exactly where it was and stay editable.
- **Scope is the reading notes pane only.** The main editor and the notebook overlay keep plain text. This falls out of passing the plugin through `extra` — **do not add a flag to `n4lBaseExtensions`, and do not edit `frontend/src/lib/n4l-editor.js` at all.**
- **Object URLs, not data URLs.** Base64 inflates a file by a third, holds it all in memory as a string, and defeats seeking in `<video>`. Cache the `Blob`; each widget makes its own object URL and revokes it on destroy.
- **Loads are cached per path, failures included.** Without a cached failure, typing a path fires a load per keystroke.
- **A failed load shows a muted `media not found: <path>` chip**, never a broken-image element, and never disturbs the text line.
- JS/Svelte: 2-space indent, **no semicolons**, single quotes.
- Run `make test-js` and `cd frontend && npx vite build` before each commit.
- `TestWatchCollectionAdded` in `internal/content` is a **known pre-existing fsnotify flake**. Ignore it.

## MANDATORY: negative-control every test you write

On the branch this follows, **seven** plausible-looking negative controls turned out to prove nothing — fixtures sampling only values on one side of a boundary, intervals that divided evenly and hid integer division, a single seeded row that made two queries indistinguishable. Each was caught only because someone ran the control and read the output.

Before committing, for each behaviour asserted: break the production code in the specific way the test exists to catch, run the test, confirm it **fails with a message naming the real problem**, then restore and re-run green. Report the actual failure output.

- A sabotage that does not parse is **not** a control. Change it until it runs and the test genuinely fails.
- If a control does not make its test fail, **say so plainly** and name the assertion that was supposed to catch it. Do not weaken a test until something goes red.

## File Structure

- `frontend/src/lib/n4l-media.js` — detection and MIME lookup, dependency-free.
- `frontend/src/lib/n4l-media.test.js` — its tests.
- `frontend/src/lib/n4l-media-view.js` — the CodeMirror plugin, widget and blob cache.
- `frontend/src/components/ReadingView.svelte` — one line, passing the plugin through `extra`.
- `docs/specs/2026-08-11-notes-media-manual-test-plan.md`

---

### Task 1: Detecting media paths

**Files:**
- Create: `frontend/src/lib/n4l-media.js`
- Create: `frontend/src/lib/n4l-media.test.js`

**Interfaces:**
- Consumes: nothing — dependency-free by design.
- Produces:
  - `mediaKind(path) -> 'image' | 'video' | 'audio' | null`
  - `mediaMime(path) -> string` — `''` when the extension is unknown
  - `findMedia(lineText) -> [{ path, kind, from, to }]`, offsets relative to the line

`mediaMime` exists because the widget builds a `Blob`, and a `<video>` fed a
blob with no MIME type will not play. It is here rather than in the plugin so it
is covered by tests — this repo has no CodeMirror harness.

This module must import nothing: anything importing from `wailsjs` cannot load
under `node --test`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/n4l-media.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mediaKind, mediaMime, findMedia } from './n4l-media.js'

test('mediaKind recognises every supported extension', () => {
  for (const p of ['a.png', 'a.jpg', 'a.jpeg', 'a.gif', 'a.webp', 'a.svg', 'a.bmp']) {
    assert.equal(mediaKind(p), 'image', p)
  }
  for (const p of ['a.mp4', 'a.webm', 'a.mov', 'a.m4v']) {
    assert.equal(mediaKind(p), 'video', p)
  }
  for (const p of ['a.mp3', 'a.wav', 'a.ogg', 'a.m4a', 'a.flac']) {
    assert.equal(mediaKind(p), 'audio', p)
  }
})

test('mediaKind is case-insensitive', () => {
  assert.equal(mediaKind('PHOTO.PNG'), 'image')
  assert.equal(mediaKind('Clip.MP4'), 'video')
})

test('mediaKind rejects everything else', () => {
  for (const p of ['a.pdf', 'a.txt', 'a.n4l', 'noextension', '', 'trailing.']) {
    assert.equal(mediaKind(p), null, JSON.stringify(p))
  }
})

// A .png in the middle of a name is not the extension. Matching anywhere in the
// string would render "notes-about-png.txt" as an image.
test('mediaKind looks at the extension, not the whole path', () => {
  assert.equal(mediaKind('notes-about-png.txt'), null)
  assert.equal(mediaKind('mp4-encoding-guide.md'), null)
  assert.equal(mediaKind('assets/my.png.backup'), null)
  // The discriminating case: only the LAST dot's suffix is the extension.
  // Reading from the first dot gives 'txt.png' here and misses a real image.
  assert.equal(mediaKind('archive.txt.png'), 'image')
})

test('mediaMime maps each kind to a playable type', () => {
  assert.equal(mediaMime('a.png'), 'image/png')
  assert.equal(mediaMime('a.jpg'), 'image/jpeg')
  assert.equal(mediaMime('a.jpeg'), 'image/jpeg')
  assert.equal(mediaMime('a.svg'), 'image/svg+xml')
  assert.equal(mediaMime('a.mp4'), 'video/mp4')
  assert.equal(mediaMime('a.mp3'), 'audio/mpeg')
  assert.equal(mediaMime('a.m4a'), 'audio/mp4')
  assert.equal(mediaMime('a.txt'), '')
})

test('findMedia returns a quoted media path with its offsets', () => {
  const line = 'sunset (has an image) "assets/sunset.jpg"'
  const got = findMedia(line)
  assert.equal(got.length, 1)
  assert.equal(got[0].path, 'assets/sunset.jpg')
  assert.equal(got[0].kind, 'image')
  // Offsets must span the path itself, not the surrounding quotes.
  assert.equal(line.slice(got[0].from, got[0].to), 'assets/sunset.jpg')
})

test('findMedia ignores a quoted string that is not media', () => {
  assert.deepEqual(findMedia('note (means) "a bright thing"'), [])
  assert.deepEqual(findMedia('paper (source) "refs/paper.pdf"'), [])
})

// The arrow name is deliberately irrelevant — that is the whole detection rule.
test('findMedia does not care what the arrow is called', () => {
  assert.equal(findMedia('x (illustrated by) "a/b.png"').length, 1)
  assert.equal(findMedia('x (banana) "a/b.png"').length, 1)
})

test('findMedia finds several matches on one line, in order', () => {
  const got = findMedia('x (a) "one.png" (b) "two.mp3"')
  assert.deepEqual(got.map((m) => m.path), ['one.png', 'two.mp3'])
  assert.deepEqual(got.map((m) => m.kind), ['image', 'audio'])
})

test('findMedia ignores an unquoted path', () => {
  assert.deepEqual(findMedia('x (has an image) assets/sunset.jpg'), [])
})

// N4L files in this collection have been broken by stray quotes before. An
// unterminated quote must yield nothing rather than swallowing the rest of the
// line or hanging.
test('findMedia survives an unterminated quote', () => {
  assert.deepEqual(findMedia('x (has an image) "assets/sunset.jpg'), [])
  assert.deepEqual(findMedia('"" (a) "b.png"'), [{ path: 'b.png', kind: 'image', from: 8, to: 13 }])
})

test('findMedia handles an empty line', () => {
  assert.deepEqual(findMedia(''), [])
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/n4l-media.test.js`
Expected: FAIL — cannot resolve `./n4l-media.js`.

- [ ] **Step 3: Implement**

Create `frontend/src/lib/n4l-media.js`:

```js
// Finding media assets referenced from an N4L line.
//
// Detection is by the target's file EXTENSION, never by the arrow's name.
// Arrow names are free text the author writes, so matching them means
// "has an image" works while "illustrated by", "screenshot" or a typo silently
// does not. The extension is a property of the thing being pointed at.
//
// Dependency-free so it runs under `node --test`: this repo has no CodeMirror
// or Svelte component harness, so logic left inside the plugin has no tests.

const MIME = {
  png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif',
  webp: 'image/webp', svg: 'image/svg+xml', bmp: 'image/bmp',
  mp4: 'video/mp4', webm: 'video/webm', mov: 'video/quicktime', m4v: 'video/x-m4v',
  mp3: 'audio/mpeg', wav: 'audio/wav', ogg: 'audio/ogg', m4a: 'audio/mp4', flac: 'audio/flac',
}

const KIND = {
  image: ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp'],
  video: ['mp4', 'webm', 'mov', 'm4v'],
  audio: ['mp3', 'wav', 'ogg', 'm4a', 'flac'],
}

/** 'image' | 'video' | 'audio' | null */
export function mediaKind(path) {
  const ext = extensionOf(path)
  if (!ext) return null
  for (const kind of Object.keys(KIND)) {
    if (KIND[kind].includes(ext)) return kind
  }
  return null
}

/**
 * The Blob's MIME type. A <video> handed a blob with no type will not play, so
 * this is load-bearing rather than cosmetic. '' for anything unrecognised.
 */
export function mediaMime(path) {
  return MIME[extensionOf(path)] ?? ''
}

/**
 * Every double-quoted media path on one line, with offsets relative to it.
 * @returns {{path: string, kind: string, from: number, to: number}[]}
 */
export function findMedia(lineText) {
  const out = []
  // [^"\n]* rather than .*? so an unterminated quote simply fails to match
  // instead of swallowing the rest of the line.
  const re = /"([^"\n]*)"/g
  let m
  while ((m = re.exec(String(lineText ?? ''))) !== null) {
    const kind = mediaKind(m[1])
    if (!kind) continue
    out.push({ path: m[1], kind, from: m.index + 1, to: m.index + 1 + m[1].length })
  }
  return out
}

// The final dot's suffix, lowercased. A name with no dot, or a trailing dot,
// has no extension — "notes-about-png.txt" must not read as an image.
function extensionOf(path) {
  const s = String(path ?? '')
  const dot = s.lastIndexOf('.')
  if (dot < 0 || dot === s.length - 1) return ''
  return s.slice(dot + 1).toLowerCase()
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `node --test frontend/src/lib/n4l-media.test.js` then `make test-js`
Expected: all PASS.

- [ ] **Step 5: Negative controls (report all three)**

1. In `extensionOf`, use `indexOf('.')` instead of `lastIndexOf('.')`. The "extension, not the whole path" test must FAIL on `assets/my.png.backup`. Restore.
2. In `findMedia`, make the closing quote optional — `/"([^"\n]*)"?/g`. The unterminated-quote test must FAIL. (Do **not** use `/"(.*?)"/g`: `.` excludes newlines and the lazy quantifier stops at the first quote, so on single-line input it behaves identically to `[^"\n]*` and sabotages nothing.) Restore.
3. In `findMedia`, drop the `+ 1` from both offsets. The offset test must FAIL, showing the slice including a quote. Restore.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/n4l-media.js frontend/src/lib/n4l-media.test.js
git commit -m "feat(notes): detect media paths by extension"
```

---

### Task 2: Rendering the media

**Files:**
- Create: `frontend/src/lib/n4l-media-view.js`
- Modify: `frontend/src/components/ReadingView.svelte`

**Interfaces:**
- Consumes: `mediaKind`, `mediaMime`, `findMedia` (Task 1); the existing `GetSourceBytes(refPath) -> Promise<string>` binding, which returns base64 for a file resolved under the collection directory.
- Produces: `n4lMedia() -> Extension`, a CodeMirror extension passed through `n4lBaseExtensions`'s `extra`.

**Do not edit `frontend/src/lib/n4l-editor.js`.** It already destructures `extra = []` and spreads it into the returned extension list (`:63`, `:73`). Passing the plugin through `extra` is what scopes this to the notes pane; a flag would not.

Follow the existing `ViewPlugin.fromClass` shape at `n4l-editor.js:111-117` — `constructor(view)`, `update(u)` rebuilding on `docChanged || viewportChanged`, and `{ decorations: (v) => v.decorations }`.

There is NO CodeMirror or Svelte component test harness in this repo. Do not add one; this task is verified by the build, the greps in Step 4, and the reasoning you state.

- [ ] **Step 1: Create the plugin**

Create `frontend/src/lib/n4l-media-view.js`:

```js
// Renders media referenced from an N4L line, as a block widget BELOW that line.
//
// Below, never replacing: this pane is an editor, and hiding the path would
// make a broken link invisible and unfixable, and the text awkward to edit.

import { Decoration, EditorView, ViewPlugin, WidgetType } from '@codemirror/view'
import { RangeSetBuilder } from '@codemirror/state'
import { GetSourceBytes } from '../../wailsjs/go/main/App'
import { findMedia, mediaMime } from './n4l-media.js'

// path -> Promise<Blob|null>. Failures are cached too: without that, typing a
// path fires a fresh load on every keystroke.
//
// A Blob rather than an object URL, because one URL shared between widgets
// would break every other widget the moment the first one revoked it. Each
// widget makes its own URL from this blob and revokes only that.
const blobs = new Map()

function loadBlob(path) {
  if (blobs.has(path)) return blobs.get(path)
  const p = GetSourceBytes(path)
    .then((b64) => new Blob([bytesFrom(b64)], { type: mediaMime(path) }))
    .catch(() => null)
  blobs.set(path, p)
  return p
}

function bytesFrom(b64) {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

class MediaWidget extends WidgetType {
  constructor(path, kind) {
    super()
    this.path = path
    this.kind = kind
    this.url = null
  }

  // Without this CodeMirror rebuilds the widget on every update, which would
  // restart a playing video on each keystroke.
  eq(other) { return other.path === this.path && other.kind === this.kind }

  toDOM() {
    const box = document.createElement('div')
    box.className = 'n4l-media'
    loadBlob(this.path).then((blob) => {
      if (!blob) {
        const miss = document.createElement('span')
        miss.className = 'n4l-media-missing'
        miss.textContent = `media not found: ${this.path}`
        box.appendChild(miss)
        return
      }
      this.url = URL.createObjectURL(blob)
      const el = document.createElement(this.kind === 'image' ? 'img' : this.kind)
      if (this.kind !== 'image') el.controls = true
      el.src = this.url
      box.appendChild(el)
    })
    return box
  }

  destroy() {
    if (this.url) URL.revokeObjectURL(this.url)
    this.url = null
  }

  // The widget owns its own async content; CodeMirror must not diff into it.
  ignoreEvent() { return true }
}

function mediaDecorations(view) {
  const b = new RangeSetBuilder()
  for (const { from, to } of view.visibleRanges) {
    for (let pos = from; pos <= to;) {
      const line = view.state.doc.lineAt(pos)
      for (const m of findMedia(line.text)) {
        b.add(line.to, line.to, Decoration.widget({
          widget: new MediaWidget(m.path, m.kind), side: 1, block: true,
        }))
      }
      pos = line.to + 1
    }
  }
  return b.finish()
}

// The 240px cap keeps a note with twenty assets navigable.
const theme = EditorView.theme({
  '.n4l-media': { padding: '4px 0 8px 0' },
  '.n4l-media img, .n4l-media video': {
    maxHeight: '240px', maxWidth: '100%', display: 'block', borderRadius: '4px',
  },
  '.n4l-media audio': { width: '100%', maxWidth: '420px', display: 'block' },
  '.n4l-media-missing': { color: '#8899aa', fontStyle: 'italic', fontSize: '11px' },
})

export function n4lMedia() {
  return [
    ViewPlugin.fromClass(
      class {
        constructor(view) { this.decorations = mediaDecorations(view) }
        update(u) { if (u.docChanged || u.viewportChanged) this.decorations = mediaDecorations(u.view) }
      },
      { decorations: (v) => v.decorations },
    ),
    theme,
  ]
}
```

- [ ] **Step 2: Wire it into the notes pane**

In `ReadingView.svelte`, add the import beside the others. Note the path is
`n4l-media-view.js`, the plugin — not `n4l-media.js`, which holds only the pure
detection functions:

```js
  import { n4lMedia } from '../lib/n4l-media-view.js'
```

Then add one key to the existing `n4lBaseExtensions({...})` call at `:146`:

```js
        extensions: n4lBaseExtensions({
          getArrowDefs: () => arrowDefsRef,
          getRecent: () => recentCodesRef,
          onArrowSelected: handleArrowSelected,
          extra: [n4lMedia()],
        }),
```

- [ ] **Step 3: Verify the build**

Run: `make test-js` and `cd frontend && npx vite build`
Expected: both succeed. `make test-js` must still pass — this task adds no JS tests, and `n4l-media-view.js` imports `wailsjs`, so it is deliberately **not** loadable under `node --test`. That is why Task 1 exists.

- [ ] **Step 4: Confirm the scoping**

- `grep -n "n4lMedia" frontend/src/components/ReadingView.svelte` — exactly two hits, the import and the `extra`.
- `grep -rn "n4lMedia" frontend/src/ | grep -v "n4l-media-view.js"` — must show **only** `ReadingView.svelte`. Another component would mean the feature leaked out of the notes pane.
- `git diff --stat frontend/src/lib/n4l-editor.js` — must be empty. That file is not to be touched.

- [ ] **Step 5: Report the symptoms**

State what a user would observe if each were wrong: a widget without `eq()`, a shared object URL revoked by one widget, and a failure that is not cached.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/n4l-media-view.js frontend/src/components/ReadingView.svelte
git commit -m "feat(notes): render linked media below its line"
```

---

### Task 3: Manual test plan

**Files:**
- Create: `docs/specs/2026-08-11-notes-media-manual-test-plan.md`

Match the house format of `docs/specs/2026-08-11-stats-readability-manual-test-plan.md`. Read the shipped plugin before writing.

You will need a collection with real assets. Say so in a "Before you start"
section: copy a `.png`, an `.mp4` and an `.mp3` under the collection directory,
and add lines to a reading's notes file pointing at them.

- [ ] **Step 1: Write the document**

Cover:

1. An image path in a notes line renders the image below that line, and the text line is still there, unchanged, above it.
2. The text remains editable — click into the path, type, and the line behaves like any other.
3. A video renders with controls, plays, and **seeks** when you drag the scrubber. Seeking is the specific reason blobs are used rather than base64 data URLs; if it does not seek, that decision has regressed.
4. Audio renders with controls and plays.
5. The arrow name is irrelevant: `(illustrated by) "a.png"` renders exactly like `(has an image) "a.png"`.
6. A non-media target — `"refs/paper.pdf"`, `"a bright thing"` — renders as plain text with no widget and no gap.
7. A wrong path shows `media not found: <path>` in muted italic, and the text line above it is untouched and still editable so the path can be fixed in place.
8. Typing a path character by character does not flicker or stall — failures are cached, so a half-typed path is attempted once, not once per keystroke.
9. Several media on one line each render.
10. A long note with many assets still scrolls smoothly, and no image exceeds roughly a third of the pane height.
11. **The main note editor still shows plain text** — open the same file there and confirm no media renders. This is the scoping check.
12. Playing a video and then typing elsewhere in the note does not restart it. That is what the widget's `eq()` prevents.

State explicitly, so a tester does not file them as bugs: only files under the collection directory render, remote URLs do not; PDFs are deliberately not treated as media; and there is no way to add or remove an asset from this pane — it renders what the note references.

- [ ] **Step 2: Commit**

```bash
git add docs/specs/2026-08-11-notes-media-manual-test-plan.md
git commit -m "docs(test-plan): inline notes media"
```

---

## Self-Review

**Spec coverage.** Extension-based detection → Task 1. Rendering below the line, never replacing → Task 2's block widget at `line.to` with `side: 1`. Object URLs with per-widget revoke → Task 2's blob cache. Cached failures → same. Scope limited to the notes pane → Task 2 Steps 2 and 4, including a grep that fails if the feature leaks. The `media not found` chip → Task 2's `toDOM`. The 240px cap → Task 2's theme. Manual coverage → Task 3.

**Placeholders.** None: every code step carries literal code and every test its assertions.

**Type consistency.** `findMedia` returns `{path, kind, from, to}` in Task 1 and is destructured as `m.path`/`m.kind` in Task 2. `mediaMime(path) -> string` feeds the `Blob` type. `n4lMedia()` is defined in Task 2 and imported from `n4l-media-view.js` — not from `n4l-media.js`, which holds only the pure functions.

**One thing the spec did not mention.** `mediaMime` is not in the spec. It follows from the object-URL decision: a `<video>` handed a typeless blob will not play. It is in the pure module so it is tested rather than buried in the widget.
