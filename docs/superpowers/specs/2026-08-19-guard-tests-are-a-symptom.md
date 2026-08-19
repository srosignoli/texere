# The guard tests are a symptom — refactorings worth doing

**Status:** open, not scheduled. A note, not a plan.
**Raised:** 2026-08-19, while designing the shortcuts help panel.

## The pattern

Three features in a row have shipped a source-scanning guard test. Each was
the right call at the time, and each exists for the same reason: **the thing it
guards has no single source of truth**, so a regex over the source is the only
way to notice drift.

| Guard | Forbids / checks | Exists because |
|---|---|---|
| `frontend/src/lib/icons.test.js` (feature 050) | astral-plane emoji anywhere in the UI; every `<Icon name=…>` resolves | icon names are strings passed as props — nothing type-checks them |
| `frontend/src/lib/theme-tokens.test.js` (feature 052) | literal hex / `rgb()` / `hsl()` / named colours outside an allowlist | CSS accepts a literal anywhere a token belongs; nothing prevents it |
| the shortcuts guard (planned, feature 053) | every binding in source is documented, and every documented binding exists | bindings are inline `e.key === '…'` checks in ~20 files, plus CodeMirror `key: 'Mod-s'` keymaps |

Two of these are already ~93 and ~227 lines, and a third is coming. All three
independently reimplement the same machinery: walk `frontend/src`, read each
`.svelte`/`.js` as text, strip comments, run a regex, subtract an allowlist,
assert the walk was not vacuous.

## Why this is worth fixing rather than repeating

A guard test is a **lint against a design gap**. It reports the gap; it does not
close it. Every one of them shares the same three weaknesses, and this session
hit all three:

- **It only sees the patterns it knows.** The shortcuts extractor missed
  `Ctrl+S` entirely until it was taught CodeMirror's `key: 'Mod-s'` form — and
  that is the app's most advertised shortcut. A fourth syntax would be invisible
  again.
- **It can pass while checking nothing.** Every guard needs a vacuity assertion
  because a file-walking test that stops walking is silently green. Feature 050
  demonstrated exactly that failure, deliberately, and it is the reason the
  assertion is in all of them.
- **Its allowlist rots.** `ALLOW` entries are justified once and never
  re-examined. `theme-tokens.test.js` lost the "every pending file still has
  something pending" companion test when the ratchet was removed, and nothing
  replaced it for `ALLOW`. A stale entry now pre-authorises re-introducing the
  very thing the guard forbids.

## What would actually remove each guard

Ordered by ratio of benefit to disruption.

**1. Shortcuts — a central keymap registry.** The highest-value one, and the
only one where the guard can be deleted outright rather than weakened. Today the
panel and the handlers are two descriptions of the same facts, and the guard
exists solely to keep them agreeing. If the handlers *dispatch from* the same
table the panel renders, they cannot disagree — the drift is impossible rather
than detected. This was considered for feature 053 and rejected as too large for
a "short capability" (~20 files, every keyboard path), which is the right call
for that feature and the wrong one forever.

**2. Icons — a build-time or type-level check.** `<Icon name="save" />` is a
string prop. A generated union type, or a Svelte-aware lint, would catch a typo
where it is written instead of in a test that greps for it. Lower value than
(1): the current guard is small, and the failure it prevents is cosmetic.

**3. Colours — probably keep the guard.** Worth recording that this one may be
irreducible. CSS has no mechanism that forbids a literal where a token belongs;
stylelint's `declaration-property-value-allowed-list` comes closest and would
replace the regex with a maintained tool, but it is a dependency and this app is
deliberately dependency-light. If the guard stays, give it back the companion
test the ratchet removal deleted: assert every `ALLOW` entry still corresponds
to a literal that is actually present, so a stale exemption fails loudly.

**4. If none of the above happen, at least factor the machinery.** The walk,
the comment-stripping, and the vacuity assertion are the same in all three. One
`lib/source-scan.js` used by all of them would cut the duplication and mean a
bug found in one scanner is fixed everywhere — the `!e.shiftKey` negation bug
and the `#4312`-in-a-comment false positive were both found in one guard and
would have recurred in the next.

## What is not being claimed

The guards are not a mistake. They caught real defects this session that no
other mechanism would have: unreachable icon names, ~200 unconverted colours,
and a `PENDING` list that had gone stale. The argument here is only that a lint
which must exist forever is evidence of a structural gap, and that at three
instances the pattern is established enough to be worth naming.
