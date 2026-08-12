# Media and math on flashcards — manual test plan (executed)

**Design:** `2026-08-12-recall-media-math-design.md`
**Build:** `make build && ./build/bin/texere` (built at commit `5436949`)
**Run date:** 2026-08-12 — all eight cases below were **executed**, not just
described. Each carries the result actually observed and the screenshot it was
read from.

**This document is the verification gate for everything visual in this
feature.** This repository has **no Svelte component test harness**, so
`RecallCard.svelte`'s new markup — `MathText` around both faces, `InlineMedia`
for each media token, `stripMediaTokens` on the text — has **zero automated
coverage**. What *is* automated is one layer down: `n4l-media.test.js` covers
`mediaKind`, `srcFor`, `mediaTokens` and `stripMediaTokens` as pure functions,
and `internal/media` covers the `/collection/` route that serves the bytes.
Everything below is what those cannot reach: whether the right thing appears on
screen, and whether the answer stays hidden until it is asked for.

## How this run was set up

- The collection under test is a **copy** of the user's real collection, made
  with `cp -r` into the job scratch directory
  (`~/.claude/jobs/ad91db08/tmp/collection`). The real collection and its
  `learning.db` were never touched: the scratch cards live only in the copy's
  own `learning.db`, which is throwaway. `~/.config/texere/config.json` was
  pointed at the copy for the duration of the run and **restored afterwards**.
- The scratch notes file is `recall-render-check.n4l` in that copy. Per the
  controller's instruction it was **not** created in the user's real collection
  and therefore was not deleted from it — it lives, with the copy, in the job
  directory and never entered the user's study queue.
- Its chapter/marker shape was copied from the collection's own `test3.n4l`
  (`- <chapter>`, then `# @id <shortid>` / `# @card` or `# @cloze` above each
  card-bearing line).
- The two media paths are real files in the collection's `media/` directory
  (`adbe92ae…f4b.jpg`, a photo; `18f0a984…527.png`, a line drawing), plus one
  deliberately absent path for case 8.
- Cards were materialised by the app itself: opening **Recall** (Ctrl+Shift+R)
  runs `SyncStudyItems`, which discovered all eight items. To make the eight
  appear in a known order ahead of the collection's twelve pre-existing items,
  the copy's `learning.db` had `due` timestamps rewritten **while the app was
  closed**. That touches scheduling only — nothing in the rendering path.
- The app was driven with synthetic X11 input (XTEST) on `DISPLAY=:0`, and each
  state was captured by grabbing the app window with `XGetImage` and read back
  as an image. Root-window capture on this WSLg display returns black, so every
  screenshot is a grab of the window itself.

The scratch file's card-bearing lines:

```
\( E = mc^2 \) >> mass-energy equivalence
the Bohr radius >> \( a_0 = \frac{4\pi\varepsilon_0\hbar^2}{m_e e^2} \)
what does this picture show >> media/adbe92ae…f4b.jpg
\( \nabla \cdot \mathbf{B} = 0 \) >> media/18f0a984…527.png
the Gauss law for magnetism is {{ \( \nabla \cdot \mathbf{B} = 0 \) }}
an unclosed formula \( x + 1 >> renders as prose
what is the capital of Italy >> Rome
a picture that is not there >> media/definitely-missing-file.png
```

## Result summary

**8 of 8 cases pass**, on **both** surfaces that host `RecallCard`: the
full-screen Recall overlay (`StudyMode.svelte`, Ctrl+Shift+R) and the Study
surface's **Recall** tab (`StudySurface.svelte`, Ctrl+Shift+S). No failures, no
bugs found. Screenshot paths below are relative to
`~/.claude/jobs/ad91db08/tmp/`.

| # | Case | Result |
|---|---|---|
| 1 | `E = mc^2` front typeset | **PASS** |
| 2 | Bohr radius back typeset, hidden until reveal | **PASS** |
| 3 | fruit/picture → image, no path text | **PASS** |
| 4 | formula → image, back is the image alone | **PASS** |
| 5 | cloze containing a formula | **PASS** |
| 6 | unclosed `\(` renders as prose | **PASS** |
| 7 | ordinary text-only card unchanged | **PASS** |
| 8 | missing media file | **PASS** |
| — | answer media not fetched before reveal | **PASS** (see below) |
| — | keyboard path (Space, `1`–`4`, reveal reset) | **PASS** |

## 1. `E = mc^2` on the front — typeset, not source

Front: `\( E = mc^2 \)`.

**Result: PASS.** The card shows *E = mc²* in KaTeX's serif italic with a real
superscript; no `\(`, `\)` or backslash anywhere on the card.
`shot-03-card1-front.png`. Revealing shows the plain-text back
"mass-energy equivalence" below the rule (`shot-04-card1-back.png`).

## 2. Bohr radius — the formula is on the back, and only after Show answer

Back: `\( a_0 = \frac{4\pi\varepsilon_0\hbar^2}{m_e e^2} \)`.

**Result: PASS.** Before reveal the card shows only "the Bohr radius" — no
formula, no source, nothing below the rule, and no rule at all
(`shot-05-card2-front.png`). After Space, `a₀ =` followed by a properly stacked
fraction: `4πε₀ℏ²` over `m_e e²` with the bar between them
(`shot-06-card2-back.png`).

## 3. Picture answer — the image, and no path text

`what does this picture show >> media/adbe92ae…f4b.jpg`.

**Result: PASS.** The front is prose only (`shot-07-card3-front.png`). After
reveal the photo renders beneath the rule at its natural aspect ratio, and the
64-character hash filename appears **nowhere** on the card
(`shot-08-card3-back.png`) — `stripMediaTokens` removed the token and the media
took its place, rather than being appended beside it.

## 4. Formula front, image answer — the back is the image alone

`\( \nabla \cdot \mathbf{B} = 0 \) >> media/18f0a984…527.png`.

**Result: PASS.** Front typeset as ∇ · **B** = 0 with the bold upright **B**
(`shot-09-card4-front.png`); the back is the line drawing on its own, no text
and no path (`shot-10-card4-back.png`). This card also arrived **unrevealed**
immediately after grading a card whose media had just been shown — the reveal
state resets per card.

## 5. Cloze containing a formula

`the Gauss law for magnetism is {{ \( \nabla \cdot \mathbf{B} = 0 \) }}`.

**Result: PASS.** Front: "the Gauss law for magnetism is […]" — the braces are
gone and the blanked span is the `[…]` marker (`shot-11-card5-front.png`).
Back: "the Gauss law for magnetism is ∇ · **B** = 0", the formula typeset
inline in the middle of the sentence, no `{{`/`}}` (`shot-12-card5-back.png`).

## 6. Unclosed `\(` — literal prose, not a broken card

`an unclosed formula \( x + 1 >> renders as prose`.

**Result: PASS.** The front reads literally `an unclosed formula \( x + 1`,
backslash and paren included, in the ordinary card font. The card is not blank,
the rule and Show answer button are in place, and the back reveals normally as
"renders as prose" (`shot-13-card6-front.png`, `shot-14-card6-back.png`).

## 7. Ordinary text-only card — unchanged

`what is the capital of Italy >> Rome`.

**Result: PASS.** Identical to the pre-change appearance: white front text
centred in the card, green back text under the rule, same spacing and same
button row as every other card in this run (`shot-15-card7-front.png`,
`shot-16-card7-back.png`). Text with neither math nor media is passed through
untouched.

## 8. Missing media file

`a picture that is not there >> media/definitely-missing-file.png`.

**Result: PASS.** The front is prose as usual (`shot-17-card8-front.png`).
After reveal, the back shows the webview's **broken-image placeholder** — an
empty bordered box with the small "?" glyph — and nothing else: no error, no
stack, no visible path. The four grade buttons are present and working: the
card was graded from this state with a **mouse click** on `Good (3)` and the
queue advanced normally to "Nothing due. 🎉" (`shot-18-card8-back.png`,
`shot-19-after-last-grade.png`).

Worth knowing, not a bug: the `<img>` keeps the path in its `alt` attribute, so
a very wide window may render the alt text inside the placeholder box. At the
tested size the box shows only the broken-image glyph.

## The answer's media is not fetched before the card is revealed

The property a screenshot cannot show. This build has **no webview inspector**
— right-clicking inside the webview produces no context menu
(`shot-24-rightclick.png`), so the DOM and Network panels were not available —
and the Go asset server has no request log to read. It was verified instead by
**file access time**, which is a direct observation of whether the server read
the bytes:

1. Both answer images had their `atime` set to `2020-01-01` before launch. The
   filesystem is mounted `relatime`, which updates `atime` on the next read
   when the stored value is over a day old (confirmed on a probe file first).
2. App launched, Recall opened, first card displayed: **both atimes still
   2020-01-01**.
3. Card 3 on screen, front only, not revealed: **still 2020-01-01** — the
   answer's `<img>` was never created, so nothing requested `/collection/…`.
4. Space pressed: the jpg's atime jumped to the reveal instant
   (`11:35:31`), while card 4's png — a card not yet reached — **stayed at
   2020-01-01**.
5. The same sequence repeated for card 4's png: unchanged while its front was
   displayed, updated (`11:36:13`) at reveal.
6. Repeated once more on the **other** surface (Study → Recall tab) with a
   fresh 2020 atime: unchanged while the front showed, updated at reveal
   (`11:43:04`).

Two different files, three separate reveals, on both surfaces — each read
happened at the reveal and never before it. Confidence: high for "the bytes
were not read before reveal", which is what matters. It is indirect evidence
about the DOM: it proves no fetch, and a fetch is the only thing an `<img>`
inside `{#if revealed}` could do that would leak the answer. A hypothetical
element carrying the answer *without* fetching would not be caught by this
method; nothing in the markup can produce one.

## Keyboard path

**Result: PASS.** Space reveals; `1`–`4` grade and advance (`3` and `4` both
exercised, plus a mouse click on `Good (3)`); every card that followed a grade
arrived **unrevealed**, including the two media cards and the cloze — the
`card?.id !== lastId` reset in `RecallCard.svelte` holds. Escape closes the
overlay as before.

## Nothing was changed in the product

No production code was touched for this run. The only repository change is this
document. The scratch collection copy, the scratch `.n4l`, the screenshots and
the throwaway X11 driver all live under `~/.claude/jobs/ad91db08/tmp/` and
outside the repository.
