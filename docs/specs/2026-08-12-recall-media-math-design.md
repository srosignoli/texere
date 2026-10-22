# Media and math on flashcards

**Date:** 2026-08-12
**Status:** Proposed — awaiting approval
**Feature area:** Study mode → Recall focus

## Summary

Flashcards render referenced images, video and audio, and typeset LaTeX, using
the same two components the Read tab already uses. On a card the media path is
**replaced** by its media rather than shown beside it.

## Motivation

The Read tab renders media and typeset mathematics; the Recall panel shows the
same content as raw text. A card whose answer is a diagram shows a file path,
and a card about angular momentum shows `\( L = I\omega \)` as source — which is
precisely the material where seeing the thing matters most.

## Grounding in the current implementation

- **A card's front and back are the node's own text.** `basicCards`
  (`internal/learning/cards.go:39`) splits on the first `>>`; `clozeCards`
  (`:87`) rewrites the same text. Nothing else is carried.
- **Therefore LaTeX already reaches a card verbatim.** `\( E = mc^2 \) >> mass–
  energy equivalence` is stored exactly as written. Only the rendering is
  missing — no change to the card model, the schema, or sync.
- **Arrows are NOT carried into a card.** A `(pic) media/apple.jpg` arrow on a
  card's node never reaches its front or back, even though that is how media
  reaches the Read tab. A path written *inside* the card text does survive.
- **Cloze uses double braces.** `stripBraces` (`cards.go:81`) removes only `{{`
  and `}}`, so LaTeX's single braces — `\frac{a}{b}` — pass through untouched.
  The two notations do not collide.
- **`RecallCard.svelte:29-30`** interpolates `card.front` and `card.back` raw.
  It is the only component that renders a card.
- **The back is already inside `{#if revealed}`**, so an answer's media will not
  load, or spoil the answer, before the card is turned.
- **`NodeCard.svelte:18-20`** defines `mediaTokens` locally — the only copy, and
  untested, because it lives in a component and this repo has no component
  harness.

## Decisions

- **Media comes from the card's own text.** Rejected: carrying a node's media
  arrows into the card, which would mean changing `GeneratedCard`, the `items`
  table and the sync path, and deciding whether an arrow's media belongs on the
  front or the back. Writing the path where you want it to appear is enough.
- **On a card, a media token is replaced by its media.** This diverges
  deliberately from the Read tab, where the path stays visible because a broken
  link needs to be fixable while reading. On a flashcard the path is noise; the
  picture is the answer.
- **`mediaTokens` moves into `n4l-media.js`.** It is needed in two components
  now, and moving it makes it testable for the first time. `NodeCard`'s
  behaviour is unchanged — it imports what it used to define.
- **Nothing changes on the Go side.** No new binding, no schema change, no
  change to how cards are generated.

## Architecture

### `frontend/src/lib/n4l-media.js`

Gains two exports beside `mediaKind` and `srcFor`:

```
mediaTokens(text)      -> string[]   // whitespace-delimited tokens that are media
stripMediaTokens(text) -> string     // the same text with those tokens removed
```

Both pure and dependency-free, so both run under `node --test`.
`stripMediaTokens` collapses the whitespace it leaves behind, so removing a
token from the middle of a sentence does not leave a double space, and removing
a trailing one does not leave a trailing space.

### `frontend/src/components/RecallCard.svelte`

Front and back each become the text with media stripped, rendered through
`MathText`, followed by an `InlineMedia` per token:

```svelte
<div class="rc-front">
  <MathText text={stripMediaTokens(card.front)} />
  {#each mediaTokens(card.front) as token}<InlineMedia path={token} />{/each}
</div>
```

and the same inside the existing `{#if revealed}` for the back.

### `frontend/src/components/NodeCard.svelte`

Deletes its local `mediaTokens` and imports the shared one. No behavioural
change — the Read tab keeps showing the path beside the media.

## Error handling

Nothing new. A missing file, a remote URL, malformed LaTeX and an unclosed
delimiter all behave exactly as they do in the Read tab, because they are the
same two components.

| Case | Behaviour |
|---|---|
| Card text is only a media path | The text renders empty; only the media shows |
| Media file missing | The element's own error state, as in the Read tab |
| Malformed LaTeX | Rendered in red showing its source |
| No media and no math | Identical to today's output |
| A media path has trailing punctuation, e.g. `see media/apple.jpg.` or `(media/apple.jpg)` | Not detected: the `.` or `)` is part of the token, so `mediaKind` returns null and the path renders as literal text. Pre-existing, shared with the Read tab — but more confusing on a card, where the design says the path should be noise. |

Authors writing a formula that *is* the card's answer should prefer display
math `\[ … \]` over inline `\( … \)`: inline typesets a `\frac` at text-style
size, which reads small on a card.

## Testing

**JS** (`node --test`): `mediaTokens` for a bare path, a path amid prose, several
in one string, none, and a non-media token left alone. `stripMediaTokens` for the
same inputs, plus that it leaves no doubled internal whitespace, no leading or
trailing space, and returns the string unchanged when there is nothing to strip.

**Manual:** its own test-plan document — math on a front, an image on a back,
both on one card, a card whose entire back is a path, a cloze card whose text
contains a formula, and confirmation that an answer's image does not load before
the card is revealed.

## Out of scope

- Carrying a node's media arrows into its cards.
- Any change to how cards are generated, scheduled or graded.
- Media or math in the card *editor* or the processing queue.
- Audio autoplay on reveal.
