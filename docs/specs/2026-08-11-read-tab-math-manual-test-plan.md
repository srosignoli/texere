# LaTeX math in the Read tab — manual test plan

**Design:** `2026-08-11-read-tab-math-design.md`
**Build:** `make build && ./build/bin/texere`

**This document is the verification gate for everything visual.** `splitMath`
has thorough coverage in `frontend/src/lib/n4l-math.test.js` (inline only,
display only, interleaved prose, unclosed delimiters, braces nested in a
formula, `$variable` references, empty/missing text). Nothing downstream of it
does: `MathText.svelte`'s KaTeX call and `NodeCard.svelte`'s four call sites
have zero automated coverage and have never been watched rendering in a real
window. Treat any failure below as a new finding.

## Two things that are deliberate — do not report them as bugs

- **`$…$` is never math, and never will be here.** Every `$` in this
  collection introduces an N4L variable (`$user1.1`, `$question.1`,
  `$metadata.1`). `\(…\)` and `\[…\]` are the only delimiters recognised.
- **There is no way to copy a formula's source out of the rendered output.**
  KaTeX replaces the LaTeX text with typeset HTML; the source is gone from the
  page. To recover it, look at the Note tab.

## Before you start

Open the collection at
`/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test` — it's
real, already on disk, and dense with the constructs this feature has to get
right. Two files in it matter:

- **`moon.n4l`** — a physics note thick with formulas (`\times`, `10^{37}`,
  `\frac{mM}{m+M}`, `\sqrt{GM}`, `\dot\omega_E`, `{\rm TOT}`, `\mu`, `\omega`),
  already confirmed to render correctly in a GUI check during an earlier task.
  This plan re-checks it as the acceptance gate, not as a first look.
- **`json_example_4.n4l`** — ordinary JSON-shaped notes with `$user1.1`-style
  variables, including one line with three of them.

Then open the **Read** tab (the one without line numbers, where arrow codes
appear as their descriptions) on each file as the cases call for it.

## 1. An inline formula in an arrow target renders typeset

Open `moon.n4l` in the Read tab and find the row for **Moment of Inertia of
the Earth**.

**Expect:** the `(has value)` row shows typeset mathematics — an italic *I*,
an equals sign, "8.1", a multiplication sign, "10" with a superscript "37",
"kgm" with a superscript "2" — not the literal characters `\times` or `^{37}`.

## 2. Display math renders centred on its own line

In the same file, find the paragraph containing `\[ L_{\rm TOT} = I\omega_{\rm
earth} + \mu R^2 \omega_{\rm moon} \]`.

**Expect:** that formula breaks onto its own line, horizontally centred,
visually distinct from the surrounding prose — this is `\[…\]`'s display mode,
as opposed to the inline formulas sitting mid-sentence around it.

## 3. Prose and math interleave in order, and the prose still reads as a sentence

Look at the same paragraph as a whole: "The total angular momentum `\(L\)` of
the Earth-Moon system is approximately constant. `\[ L_{\rm TOT} = … \]` where
`\(\mu\)` is the reduced mass `\[ \mu = \frac{mM}{m+M} = 7.4 \times 10^{22} kg
\]`".

**Expect:** reading top to bottom, it comes out as: prose, an inline *L*,
more prose ending in "constant.", a centred display formula, prose "where", an
inline *μ*, prose "is the reduced mass", a second centred display formula. No
segment is out of order, dropped, or duplicated, and the prose parts still
read as English sentences with the formulas standing in for the symbols they
replace.

## 4. Every construct in the file typesets correctly

Read down the whole of `moon.n4l` in the Read tab and check each of these
renders as the mathematics it names, not as its LaTeX source:

| Source | Expect |
|---|---|
| `\times` | a multiplication sign (×) |
| `10^{37}` | "10" with a superscript "37" |
| `\frac{mM}{m+M}` | a vertical fraction, `mM` over `m+M` |
| `\sqrt{GM}` | a radical sign over `GM` |
| `\dot\omega_E` | a lowercase omega with a dot above it and a subscript E |
| `{\rm TOT}` | upright (non-italic) "TOT" as a subscript |
| `\mu` | lowercase mu (μ) |
| `\omega` | lowercase omega (ω) |

**Expect:** all typeset correctly; nothing shows a LaTeX command as literal
text.

## 5. Bare parentheses inside a formula are safe — verified, not suspected

This was raised in review as a possible break: N4L uses `(code)` for arrow
codes, so a formula containing literal parentheses might confuse the parser
into treating them as one. It doesn't. `moon.n4l` already contains
`\left( \frac{\omega_E}{\omega_M} \right)` inside a `\[…\]` block — find the
paragraph starting "But `\[ \dot \omega_E = …`" and read to its end, where
`\frac{dR}{dt} = - \frac{2I}{\mu R} \left( \frac{\omega_E}{\omega_M} \right)
\cdot \left( \frac{\dot T}{T} \right)` sits inside one display formula.

**Expect:** the whole expression, parentheses included, renders as one
typeset formula — no arrow breaks mid-formula, nothing after `\left(` gets cut
off or reinterpreted. This case exists as insurance against a future change to
the N4L parser or `splitMath`, not because anything is currently suspected
broken.

## 6. Malformed LaTeX renders in red showing its source, and the rest of the card still renders

Add a line to a note (any file — a scratch note is fine):

```
has value (has value) "\( \frac{1 \)"
```

**Expect:** that arrow's target shows in red monospace text reading
`\frac{1` roughly — its raw source, not a blank space and not a crashed card.
Everything else on the card (its node text, its other arrows) renders
normally above and below it.

## 7. An unclosed delimiter renders literally and swallows nothing after it

Add:

```
note (note) \( x = 1
```

**Expect:** the target shows literally as the text `\( x = 1` — no red error
styling, because this isn't malformed LaTeX, it's plain prose that happens to
contain a backslash-paren. If there's anything after it on the same string
(add more text after `x = 1` to check), that text still shows too — the
opener must not consume the rest of the string.

## 8. Two-or-more `$variable` references on one line render unchanged — the case that matters most

Open `json_example_4.n4l` in the Read tab and find the sequence line:

```
$user1.1 (then) $user2.1 (then) $user3.1
```

**Expect:** `$user1.1`, `$user2.1` and `$user3.1` all display exactly as
written — literal dollar signs, literal text, no typesetting, no red error
box, nothing missing. This is the single most dangerous thing this feature
could have got wrong: a naive implementation might treat `$` as a math
delimiter (it's the conventional one) and either swallow everything between
the first two `$`s on the line or throw a KaTeX error on `user1.1 (then)
$user2.1 (then)`. A line with only one `$` can't catch that failure, because
there's no second `$` for a broken implementation to pair against — hence a
line with three.

## 9. A note with no math renders exactly as it did before

Open any note with no `\(` or `\[` in it — most of the collection.

**Expect:** node text and arrow targets look exactly as they did before this
feature shipped: plain text, no KaTeX markup, no red boxes, no stray spacing.
`splitMath` yields a single `text` segment for these, which is a pass-through.

## 10. Media and math coexist on the same card

Find or create a card with both a `(pic)` (or other media) target and a
`(has value)` formula target — for example, add a `(pic)` arrow to one of the
`moon.n4l` constant rows alongside its existing `(has value)` formula.

**Expect:** both render: the image (or video/audio control) under its row,
the formula typeset under its own row. Neither interferes with the other —
`InlineMedia` and `MathText` render side by side off the same arrow target
without one consuming what the other needs.

## 11. The Note tab still shows raw LaTeX — the scoping check

Open `moon.n4l` in the **Note** tab (the source editor, with line numbers).

**Expect:** every formula shows as its literal LaTeX source — `\times`,
`10^{37}`, backslashes and all — with no typesetting anywhere. This is
deliberate: the Note tab is a source editor, and math rendering is scoped to
the Read tab only.

## 12. Formulas render with the machine offline

Disconnect the network, then open (or reopen) `moon.n4l` in the Read tab.

**Expect:** every formula still typesets, identically to case 4. KaTeX's
fonts are bundled into the app rather than loaded from a CDN, so this has no
network dependency to fail. **[shell]** confirm the fonts really did ship,
rather than the app happening to have a warm cache from a previous run:
`find frontend/dist/assets -iname '*.woff*' -o -iname '*.ttf' | wc -l` should
report several dozen font files.

## What I most want to hear about

- **Whether formulas actually read as mathematics** — not "does something
  other than the raw LaTeX show up," but whether a physicist glancing at
  **Moment of Inertia of the Earth** (case 1) or the angular-momentum
  paragraph (case 3) would recognise it as the equation it is. That's the
  entire point of the feature; every other case is a guard against it
  breaking.
- **Case 8.** If the `$user1.1`/`$user2.1`/`$user3.1` line shows anything
  other than three intact variable references, stop and report immediately —
  that's the regression this whole design exists to prevent, and it would
  mean every `$`-bearing note in a real collection is silently corrupted.
- **Case 5.** It's marked verified rather than suspected precisely so that a
  clean pass here is reassuring and not just expected; report it either way.
- **Case 6's "rest of the card still renders."** A red error box is fine; a
  card that goes blank or throws past it is not.
