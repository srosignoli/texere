# LaTeX math in the Read tab

**Date:** 2026-08-11
**Status:** Proposed — awaiting approval
**Feature area:** Workbench → Read tab

## Summary

In the Read tab, LaTeX delimited by `\(…\)` or `\[…\]` renders as typeset
mathematics. It applies to every note automatically — there is no per-note
setting, because the delimiters themselves are the signal.

## Motivation

Notes about physics and mathematics carry formulas in LaTeX. The Read tab is
where notes are read rather than edited, and it shows them as raw source:

```
Moment of Inertia of the Earth (has value) "\( I = 8.1 \times 10^{37} kgm^2\)"
```

Reading `10^{37}` and `\times` as symbols rather than as a quantity is exactly
the friction this tab exists to remove.

## Grounding in the current implementation

Measured against the collection at
`/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`:

- **The delimiters in use are `\(…\)` and `\[…\]`.** `moon.n4l` uses both, often
  in the same string, mixed with prose.
- **`$` is never math here.** All 89 occurrences of `$` in the collection are
  N4L variable references — `$user1.1`, `$question.1`, `$metadata.1`. Treating
  `$…$` as a delimiter, as many renderers do by default, would corrupt every one
  of them.
- **The parser strips the surrounding N4L quotes.** The arrow above arrives as
  `arrow.Target = \( I = 8.1 \times 10^{37} kgm^2\)` — clean LaTeX, single
  backslashes, no wrapper quotes to strip in the frontend.
- **Math appears in three fields of a parsed node**, all of which `NodeCard`
  currently interpolates raw: `node.Text` (`:23`), `arrow.Target` (`:36`, `:48`)
  and `ditto.Text` (`:42`).
- **`mathjs` is already a dependency but is not a renderer** — it evaluates
  expressions. Typesetting needs a separate library.
- `NodeCard` is reached only from `ReadTab.svelte`, so changing it cannot affect
  any other surface.

## Decisions

- **Delimiters are `\(…\)` for inline and `\[…\]` for display, and nothing
  else.** In particular `$…$` and `$$…$$` are **not** supported, because `$`
  introduces a variable in N4L. This is a deliberate departure from the common
  default and the single most important constraint in this design.
- **Detection is the delimiters.** A note "contains math" exactly when a
  delimited span is present; a string without one renders as it does today. No
  flag, no heuristic, no per-note setting.
- **KaTeX, bundled with its fonts.** Synchronous, so a card renders in one pass;
  self-contained, so it works offline, which matters for a desktop app. Rejected:
  MathJax, which is heavier and asynchronous, and a CDN, which would make
  formulas fail without a network.
- **Malformed LaTeX shows itself rather than breaking the card.** KaTeX runs
  with `throwOnError: false`, which renders the offending source in red.
- **An unclosed delimiter is prose.** `\( x` with no `\)` renders literally
  rather than consuming the rest of the string. This mirrors a bug already made
  once on this feature area, where an unterminated quote silently swallowed a
  line.
- **Only KaTeX's own output bypasses escaping.** Prose segments render through
  ordinary interpolation and stay escaped. KaTeX generates its HTML from the
  LaTeX source with `trust: false` by default, so it emits no scripts and no
  `javascript:` links.
- **Read tab only.** The notebook's cells and the graph explorer's labels are out
  of scope, confirmed explicitly rather than assumed.

## Architecture

### `frontend/src/lib/n4l-math.js` (new)

```
splitMath(text) -> [{ type: 'text' | 'inline' | 'display', value: string }]
```

Pure and dependency-free — it must not import KaTeX — so it runs under
`node --test`, which is the only automated coverage available here.

Scans left to right for `\(…\)` and `\[…\]`, emitting the prose between them as
`text` segments. A string with no delimiter yields a single `text` segment. An
unclosed delimiter is emitted as `text` from the opener onward.

### `frontend/src/components/MathText.svelte` (new)

Takes `text`, calls `splitMath`, and renders each segment: `text` through
ordinary interpolation, `inline` and `display` through
`katex.renderToString(value, { displayMode, throwOnError: false })` inside
`{@html}`.

This is the only place in the feature where escaping is bypassed, and it is
bypassed only for a string KaTeX itself produced.

### `frontend/src/components/NodeCard.svelte`

Three interpolations become `<MathText>`: `node.Text`, `ditto.Text`, and
`arrow.Target` in both the node-arrow and ditto-arrow branches. The existing
`<InlineMedia>` beside each target is untouched — a media path contains no math
and a formula is not a file, so the two never interact.

## Error handling

| Case | Behaviour |
|---|---|
| Malformed LaTeX | KaTeX renders the source in red; the rest of the card is unaffected |
| Unclosed `\(` or `\[` | The remainder renders as prose |
| No math in the string | One text segment — identical to today's output |
| A `$variable` | Untouched; `$` is not a delimiter |
| Empty or null text | No segments, nothing rendered |

## Testing

**JS** (`node --test`): `splitMath` for inline only, display only, prose
interleaved with both, several formulas in one string, an unclosed `\(`, an
unclosed `\[`, a `$user1.1` left intact, braces nested inside a formula, an
empty string, and a string with no math at all.

**Manual:** its own test-plan document, run against `moon.n4l`, which is real and
dense with formulas. It covers rendering fidelity for the constructs actually
present — `\times`, `10^{37}`, `\frac`, `\sqrt`, `\dot\omega_E`, `{\rm TOT}` —
plus display math centring, malformed input, and a note containing `$variables`
rendering unchanged.

## Out of scope

- `$…$` and `$$…$$` delimiters, permanently, for the reason above.
- Math anywhere but the Read tab.
- Editing or previewing formulas as you type.
- Numbered equations, cross-references, or a macro definitions file.
- Copying a formula's LaTeX source from the rendered output.
