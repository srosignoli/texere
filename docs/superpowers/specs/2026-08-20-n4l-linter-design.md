# N4L linter — design

**Date:** 2026-08-20
**Branch:** `055-n4l-linter` (from `main`)

## The problem

N4L is for casual notes you refine later. That is the point, and strict syntax
enforcement would destroy it. But a file the parser misreads silently degrades
three things: the node column shows the wrong nodes, arrows fail to link, and
the graph build produces fewer edges than the author intended. Today none of
that is visible unless you run Build Graph and read the log.

The guardrail wanted is therefore **not** a style checker. It reports
consequences: *what did the parser actually do with what you wrote.*

## What the collection actually contains

Measured against the real collection (53 files, 63 MB) with the existing
parser, before any of this was designed:

> **Note (2026-08-24):** the figures in this section were originally
> double-counted. The baseline script summed both return values of
> `ParseFile(path, reg) (*ParsedFile, []ParseError)`, but
> `internal/content/parser.go:161` returns `pf, pf.Errors` — the second
> return value *is* `pf.Errors`, the same slice, not a distinct set. Every
> occurrence was therefore added twice. This affects only the *occurrence*
> counts below (total findings, `unknown arrow`, `unterminated quote`,
> `unresolved reference`). The set-based figures — 53 files linted, 158
> distinct undeclared arrow names, 107 defects confined to exactly 7 files
> (since revised to 22 across 5 — see the correction below the table),
> 39 of 53 files with some finding, the per-arrow `has-frag`/`extract-fr`
> counts, and the 145 ms timing — do not sum occurrences across the doubled
> slice, so they were unaffected and are unchanged here. Independently
> reproduced three times against a copy of the real collection; see
> `.superpowers/sdd/2026-08-20-n4l-linter/baseline-correction-report.md`.

- **Full-collection parse: 145 ms.** Cost is not a design constraint.
- **17,592 problems across 39 of 53 files.** Reported naively that is a marker
  on 74% of files and a list nobody reads.

They collapse to far fewer *causes*:

| Kind | Occurrences | Distinct causes |
|---|---|---|
| `unknown arrow` | 17,485 (99.4%) | **158 arrow names** |
| `unterminated quote` | 87 → **2** (see below) | 7 files → **1** |
| `unresolved reference` | 20 | `$PREV.n`, `$THIS.n` |

> **Correction, 2026-08-24 — 85 of the 87 `unterminated quote` findings were
> a parser bug, not file defects.** `contentSQStart`
> (`internal/content/parser.go`) opened a multi-line single-quoted block on
> *any* `'` anywhere in an `@label` line, so every possessive and contraction
> in labelled prose — `Man's`, `five years' work` — opened a block that the
> next blank line closed as unterminated. `maskSpecial`, in the same file,
> already encoded the correct rule ("an opening `'` is not preceded by an
> ASCII letter") and `contentSQStart`'s own doc comment already claimed to
> exclude "regular node text with embedded apostrophes"; the `@label` branch
> simply never implemented it. Fixed test-first; the two survivors are
> genuine unclosed blocks in `json_example_1.n4l`.
>
> `Darwin.n4l` produced 74 of the false reports and `NDA.n4l` another 10.
> Both are output of SSTorytime's `text2N4L`, whose `Sanitize()` replaces
> only `(` and `)` — N4L's arrow metacharacters — and leaves apostrophes in
> sentence text verbatim by design. Both files were valid N4L throughout and
> now carry no marker. Parsed output never differed (`Darwin.n4l`: 3204 nodes
> before and after): `recoverQuotedBlock` had always reparsed the buffered
> lines, so only the *report* was wrong, never the content.
>
> **Revised defect baseline: 22 defects across 5 files** (was 107 across 7).
> The vocabulary figures are untouched — still 158 distinct undeclared arrow
> names across 39 files.

Two arrows account for 16,906 of the occurrences — `has-frag` (8,591) and
`extract-fr` (8,315) — used systematically across three bulk-imported files.
They are not typos and not prose misread as syntax. The remaining 156 are the
author's real vocabulary: `contains`, `url`, `then`, `implements`, `leads to`,
`depends on`, `consists of`, `see also`, `expresses`.

**This measurement is the design.** A linter reporting 17,485 line-level
findings would be worthless; one reporting "158 arrows are undeclared" is a
finite, fixable list.

## Principles

**Surface the parser; do not write a linter.** All seven problem kinds already
exist as `Err*` constants in `internal/content/errors.go`, produced at nine
sites. A separate ruleset would be a second definition of N4L that can disagree
with the first — and the first is the one that actually builds the graph.

**Group by cause, not occurrence.** 17,592 occurrences reduce to 158 undeclared
arrow names plus a handful of defect kinds confined to 7 files — that is what
"cause" means here, and it is what the tool reports.

**Two categories, because they are different problems:**

- **Defects** — the parser produced something other than what was written.
  `unterminated quote`, `unresolved reference`, `malformed marker`,
  `invalid priority`, `encoding`. Per file, per line. These drive the marker.
- **Vocabulary** — `unknown arrow`. Aggregated by arrow name across the whole
  collection, with a use count and the files it appears in. **Never marks a
  file.** This is precisely the casual-notes case the feature must protect: an
  undeclared arrow means "this relation will not become a graph edge", not
  "your file is broken".

## The marker

A red `!` beside a filename means **this file's content is being misread** —
defects only. On the real collection that is **7 files of 53**, not 39. A marker
that appears rarely is one that gets believed.

`ColumnChapters.svelte:67-68` already renders a per-file `⚠` badge for
`ScanError` (unreadable file). The marker reuses that slot rather than inventing
a second per-file status affordance.

## Why the Problems tab is a tab, not a log filter

`stores/logPanel.js` holds `logEntries` as an **append-only** history, written
by `appendLog` from several call sites and filtered by severity in the UI.

Lint findings are **current state**: re-derived on every run, and a finding must
disappear the moment the file is fixed. Appending them to the log would
duplicate every finding on every re-lint and leave fixed problems visible
forever. So the lint result gets its own store, replaced wholesale, and its own
tab. The existing Log tab is untouched.

## The change

**Backend — one pass, two outputs.**

`LintCollection(dir)` walks the collection, calls the existing `ParseFile` per
file, and classifies each `ParseError` by matching its `Msg` prefix against the
seven `Err*` constants. It returns per-file defects (file, line, kind, message,
remediation) and collection-level vocabulary entries (arrow name, use count,
files).

The classification is a table over constants that already exist. It adds no
detection logic, so the linter cannot disagree with the parser about what N4L
is.

**Remediation text lives in Go, beside the error kinds** — a `Remediation(kind)`
in `internal/content`, next to `errors.go`. Not in the UI. The parser already
writes consequence-first messages (`"quoted block opened here is never closed;
its lines were read individually"`); remediation supplies the "what to do" half.
Keeping both in one file is what stops the advice drifting from the detection.

**Frontend — three touchpoints:**

1. `ColumnChapters.svelte` — the red `!`, driven by a file's defect count,
   in the existing `⚠` slot.
2. `LogPanel.svelte` — a tab bar: **Log** (existing, unchanged) and
   **Problems** (new). Problems shows defects grouped by file, and a vocabulary
   section listing undeclared arrows by use count.
3. A store holding the current lint result, replaced wholesale on each run.

**Triggers:** collection open, and save. 145 ms measured, so no incremental
invalidation machinery — the complexity of tracking which file changed is not
worth 145 ms.

## Testing

The lint classification and aggregation are pure functions over `ParseError`
values, so they are unit-testable in Go without a collection.

**Four of the seven kinds do not occur in the real collection at all** —
`orphan ditto`, `invalid priority`, `malformed marker` and `encoding` are zero.
Their classification and remediation text would otherwise ship untested against
any real input. Each needs a synthetic fixture `.n4l` in the Go test suite that
actually provokes it, or the advice for those four is a guess.

Frontend has no component harness, so the tab and the marker are verified
manually, in all three themes.

## Out of scope

- **A "declare this arrow" action** that writes into SSTconfig. This is the
  obvious next step and the vocabulary data shape is deliberately built to
  support it — arrow name, count, files — but writing to SSTconfig is a
  different feature with its own failure modes.
- Any new syntax rule, or any check the parser does not already perform.
- Auto-fixing anything in a `.n4l` file.
- Linting on file-watch events. Open and save are enough; a burst of external
  writes should not trigger repeated full-collection passes.
