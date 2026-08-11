# Stats readability — manual test plan

**Design:** `2026-08-11-stats-readability-design.md`
**Build:** `make dev`

**This document is the verification gate for the interactive parts.** As with
the panel itself, `StatsPanel.svelte` has **no Svelte component test
harness**, so nothing below has been watched running end to end in a browser.
Treat any failure as a new finding.

What *is* automated: `stats-charts.test.js` covers `barConfig`/`lineConfig` in
isolation — the y-axis title carries the right text, the tooltip callback
renders `"12 reviews"`, an empty series still produces a config with a visible
axis, the dark palette reaches ticks/grid/title, `beginAtZero` is set, the
legend is off, a line gets a visible stroke and a bar does not, and hover
targets the whole column (`interaction: {mode:'index', intersect:false}`). On
the Go side, `stats_test.go` and `activity_test.go` still cover the same
arithmetic as before — this feature changes no computation — and now assert
against `WeekCount.WeekStart` instead of a bare index (`TestCardsPerWeekLabelsEachBucket`
and the `ActivityWeeks` tests). None of that reaches `StatsPanel.svelte`
itself: whether a canvas actually renders in a live webview, whether hovering
it actually surfaces a tooltip, and whether leaving and returning to the tab
actually tears down the old Chart.js instance are unverified by anything
automated. Cases 4 and 10 below are the two with no substitute.

## Three things that are deliberate — do not report them as bugs

- **The meanings are verbose on purpose.** "Share of reviews you failed.
  SuperMemo's healthy range is 7–13%." is a full sentence, not a two-word
  label, because "forgetting index" and "burden" are jargon a first-time
  reader has no reason to already know. If a meaning reads as over-explained,
  that is the intent, not an oversight.
- **The charts do not zoom, pan or click through.** Hovering shows a value
  and nothing else responds to a click or a drag. Out of scope for this
  change by design (see the design doc's "Out of scope").
- **Nothing about how a statistic is computed changed in this release.**
  Every number should read exactly as it did before — same forgetting index,
  same burden, same forecast counts, same streaks — only how each is
  *presented* changed. If a value itself looks arithmetically wrong, that is
  a pre-existing defect to report separately, not something introduced here.

## Before you start

You need a collection reachable from Study (Ctrl+Shift+S). Two collections
make this plan easiest to run in full:

- one with **existing** review history, scheduled cards, sources and
  extracts — ideally a few weeks old, so the weekly charts have real bars and
  the two-day log-backed charts (case 9) have a chance of being settled
- one **fresh** collection with nothing in it, for case 8

**[DB]** items read `<collectionDir>/learning.db` with `sqlite3`. Useful for
this plan:

```
sqlite3 <collectionDir>/learning.db \
  "SELECT kind, datetime(MIN(at),'unixepoch','localtime') FROM activity_log GROUP BY kind;"
sqlite3 <collectionDir>/learning.db \
  "SELECT COUNT(*) FROM reading_elements WHERE done=1; SELECT COUNT(*) FROM reading_elements;"
sqlite3 <collectionDir>/learning.db \
  "SELECT COUNT(*) FROM review_logs; SELECT COUNT(*) FROM items WHERE active=1 AND last_review IS NOT NULL;"
```

## 1. Every readout carries a label, a unit and a meaning — burden reads as a rate

Open Study → Stats on the collection with history.

**Expect:** all fourteen readouts (Forgetting index, Retention, Leeches,
Burden, Daily time, Backlog, Current streak, Longest streak, Memorized,
Sources finished, Extracts, Cards, Oldest unprocessed extract, Last 30 days)
render as three lines each — an uppercase grey label, a bold value, and a
smaller grey one-line meaning underneath. Read **Burden** specifically.

**Expect:** it reads like `2.6 reviews/day` — a rate, with its unit attached
— not a bare `2.6`. Read **Daily time** next to it.

**Expect:** it reads as a duration (`16m` or `1h 4m` via `humanDuration`), not
a raw count of minutes.

## 2. "Sources finished" can no longer be read as "has something queued"

Look at the Pipeline health card's first line.

**Expect:** the label reads **Sources finished**, the value reads `N of M`
(e.g. `1 of 5`), and the meaning underneath reads **"Documents you have
marked done."** — wording that cannot be mistaken for "N of M has something
in the queue". Cross-check `N` and `M` against **[DB]** `SELECT COUNT(*) FROM
reading_elements WHERE done=1;` and `SELECT COUNT(*) FROM reading_elements;`.

## 3. All seven charts have a labelled y-axis carrying a unit

Work through the panel top to bottom: grade mix and difficulty (Learning
quality), forecast (Workload), reviews-per-day and cards-per-week (Habit),
reading-per-week (Habit, if settled) and processed-per-week (Pipeline health,
if settled) — seven canvases across the four cards, two of which may instead
show a "Collecting…" caption (case 9, not a bug if so).

**Expect:** every chart that renders has a vertical axis title running up the
left edge naming what it measures and its unit — `reviews`, `cards`, `cards
due`, `minutes` or `extracts` as appropriate. None of the seven is a bare
shape with no axis text.

## 4. Hovering any bar or point shows its value with the unit — no automated substitute

Hover a bar in the grade-mix chart, then a bar in the forecast chart, then a
point along the reviews-per-day line — for the line, hover anywhere in the
chart's vertical band above/below the stroke, not only directly on the drawn
line itself.

**Expect:** a tooltip appears reading `"<value> <unit>"`, e.g. `12 reviews` or
`3 cards`, on all three. The line chart in particular should not require
pixel-hunting to land on the tooltip — hovering anywhere in that column
should trigger it (`interaction: {mode:'index', intersect:false}`). This is
the one property nothing automated checks end to end: `stats-charts.test.js`
calls the tooltip callback directly with a fake `ctx.parsed.y`; nothing
renders a real canvas and hovers it. A missing, blank, or unit-less tooltip
anywhere on any of the seven charts is a priority finding.

## 5. Reading-per-week and processed-per-week are now distinguishable

These two charts are visually the same shape — twelve bars — over different
quantities in different cards (Habit vs. Pipeline health). If neither is
showing its "Collecting…" caption (case 9), compare them directly.

**Expect:** the Habit card's chart's y-axis reads **`minutes`** and the
Pipeline card's reads **`extracts`**. Hover a bar in each — the reading
chart's tooltip reads `"<n> minutes"`, the processed chart's reads `"<n>
extracts"`.

## 6. The three weekly charts' x-axis labels are real week-start dates, ending on today

Look at the x-axis under cards-per-week, and — if settled — reading-per-week
and processed-per-week. Read the rightmost label on each.

**Expect:** every label is an ISO date (`2026-07-28`), one week apart from
its neighbour, and the **rightmost label on all three charts is today's
date**. The weekly buckets are rolling 7-day windows ending at the moment you
loaded Stats, not calendar (Mon–Sun) weeks — the last bucket's start is
exactly `midnight(now)`, so "current week" on this panel always literally
means "the date you're looking at it".

**[DB]** for the two activity-log-backed charts, confirm the log's own start
date is what it claims to be (used together with case 9's caption wording,
not the chart's date domain, which always spans exactly 12 weeks regardless
of when the log started):
```
sqlite3 <collectionDir>/learning.db \
  "SELECT kind, datetime(MIN(at),'unixepoch','localtime') FROM activity_log GROUP BY kind;"
```

## 7. The forecast's x-axis reads today → +13d, and its first bar matches Recall

Look at the Workload card's forecast chart (14 bars).

**Expect:** the x-axis labels read `today`, `+1d`, `+2d`, … `+13d` — not raw
dates, not `Day 0`/`Day 1`. Hover the `today` bar for its exact count, then
switch to the Recall tab and read its due-count badge (or **[DB]** `SELECT
COUNT(*) FROM items WHERE active=1 AND due <= strftime('%s','now');` — note
this deliberately excludes never-reviewed items, which have no `due` at all
but are still folded into "due today" by `WorkloadStatsFor`, so the true
comparison is against Recall's on-screen badge, not this query alone).

**Expect:** the `today` bar's value matches the Recall badge, including any
never-reviewed cards.

## 8. Regression check — a fresh collection still shows "Not enough data yet", never `-1` or `0%`

**This is the case that matters most in this plan.** The five sentinel
readouts (Forgetting index, Retention, Burden, Daily time, Oldest unprocessed
extract) have already regressed twice on this feature — once as a bare `-1`,
once as a false `0%` — and this change rewrote every line of markup that
renders them. Do this one carefully, line by line, rather than skimming the
card.

1. Open (or create) a collection with **no review history, no scheduled
   items, and an empty `activity_log`** — freshly initialized, nothing
   studied yet. **[DB]** confirm before you start: `SELECT COUNT(*) FROM
   review_logs;` and `SELECT COUNT(*) FROM items WHERE active=1 AND
   last_review IS NOT NULL;` should both return `0`.
2. Open Study (Ctrl+Shift+S) → **Stats**.
3. Read the **Learning quality** card's **Forgetting index** and
   **Retention** lines.
4. Read the **Workload** card's **Burden** and **Daily time** lines.
5. Read the **Pipeline health** card's **Oldest unprocessed extract** line.

**Expect, for all five:** the value reads the exact string **"Not enough
data yet"**, in the same italic grey as every other sentinel. None of the
five reads `-1`, `-1%`, `0%`, `NaN`, `Infinity`, or a blank. Every chart on
the page still renders its axes and an empty or flat shape — no chart area is
missing outright, and nothing throws a visible error into the layout.
Separately confirm the two log-backed charts read **"Collecting reading time
since today"** and **"Collecting processing history since today"**, not the
sentinel wording above and not an empty chart — that caption is a different
code path (`settled()`, not `has()`; see case 9).

## 9. The two log-backed charts wait until they have two days of history, each on its own clock

On the fresh collection from case 8 (or any collection whose `activity_log`
is less than two days old for one or both kinds), open Stats and look at the
Habit card's reading-time slot and the Pipeline card's processed-per-week
slot.

**Expect:** both show a caption instead of a chart — **"Collecting reading
time since &lt;date&gt;"** and **"Collecting processing history since
&lt;date&gt;"** — where `<date>` is the day the first `read` or `process` row
of that kind was written, or "today" if no row of that kind exists yet at
all. **[DB]** confirm the date matches:
```
sqlite3 <collectionDir>/learning.db \
  "SELECT kind, datetime(MIN(at),'unixepoch','localtime') FROM activity_log GROUP BY kind;"
```
Now process one extract from the Process tab (this writes only a `process`
row), leave Stats and return.

**Expect:** the processed-per-week caption's date reflects that write if it
was the first `process` row ever logged; the reading-time caption is
**unaffected** — still showing whatever it showed before, unmoved by the
process event. The two charts must not share one flag: an earlier build did,
and a single dismissal made the reading chart wrongly assert twelve weeks of
zero reading from a log seconds old. Both charts remain captions — not empty
twelve-bar charts — until two full days have passed since each kind's own
first row.

## 10. Charts survive leaving Stats and returning — no leak, no duplicate, no blank box

**This is the other case with no automated substitute.** The chart action in
`StatsPanel.svelte` (`function chart(node, config) { const c = new
Chart(node, config); return { destroy() { c.destroy() } } }`) creates a
`Chart` instance on mount and destroys it on teardown. If that `destroy()`
call were missing, wrong, or skipped, each visit to Stats would leave the
previous run's Chart.js instances alive, still bound to canvases Svelte has
since removed from the DOM — and Chart.js eventually throws `"Canvas is
already in use. Chart with ID '…' must be destroyed…"` when a new instance
is attached to a canvas element whose old registration was never cleared.
Nothing in `stats-charts.test.js` touches this, because it never constructs a
real `Chart` — it only inspects the plain config object the builder returns.

1. Open Study → Stats. Confirm all charts render (case 3).
2. Switch to another tab (Next, Read, Process or Recall).
3. Switch back to Stats.
4. Repeat steps 2–3 at least **five times** in a row, reasonably quickly.

**Expect:** every chart renders correctly on every return — same axes, same
data, nothing missing, nothing showing stale data from a previous visit, no
visibly doubled or overlapping canvas, no growing lag as you repeat the
cycle. If you can reach devtools (Wails dev mode), watch the console for an
error mentioning `"Canvas is already in use"` — that is the specific symptom
of a missing or broken `destroy()`, and five round trips is generally enough
to surface it if teardown is broken.

## 11. The panel still scrolls within its own area, clear of the Study tab bar

With Stats open and the window sized so the content is taller than the
visible area (shrink the window, or zoom in), scroll down through the four
cards.

**Expect:** the panel scrolls inside its own boundary — the Study tab bar
(Next, Read, Process, Recall, Stats) stays fixed above it and is never
covered, pushed out of view, or scrolled along with the cards underneath it.

## What I most want to hear about

- **Whether the three original complaints are actually answered.** That is
  the entire reason this feature exists:
  - Does **Burden** now read as a rate you can act on (`2.6 reviews/day`),
    rather than an unexplained bare number? (case 1)
  - Does **"Sources finished — 1 of 5 — Documents you have marked done"**
    read unambiguously as "1 of your 5 sources is done", with no way left to
    misread it as "1 of 5 has something in the queue"? (case 2)
  - Do all seven charts now carry a unit on the axis, and show the value with
    its unit on hover — at the point under the mouse, without hunting along a
    thin line to land the tooltip? (cases 3, 4, 5)
- **Case 8, the regression check.** This exact property — the five sentinels
  reading "Not enough data yet" rather than `-1` or `0%` on a fresh
  collection — has broken twice before on this feature, and every line that
  renders it was rewritten in this change. If any of the five looks wrong,
  it is the single most important finding in this plan.
- **Case 10, chart teardown.** Whether repeated round trips into and out of
  Stats produce any visible artifact or a console error. This is the only
  place a missing `destroy()` would show up, and nothing automated would
  catch it.
