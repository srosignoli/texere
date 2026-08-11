# Study statistics: a fourth focus — manual test plan

**Design:** `2026-08-11-study-statistics-design.md`
**Build:** `make dev`

**This document is the verification gate for the interactive parts.** This
repository has **no Svelte component test harness**, so `StatsPanel.svelte`
and the Stats branch of `StudySurface.svelte` have **zero automated
coverage** and have never been watched running. Treat any failure below as a
new finding.

What *is* automated: `internal/learning/stats_test.go` covers
`QualityStatsFor`, `HabitStatsFor`, `WorkloadStatsFor` and `PipelineStatsFor`
against seeded temp databases — a forgetting index from a known grade mix,
the 30-day window boundary, average latency ignoring untimed reviews, streaks
across a deliberate gap, burden as a sum of interval reciprocals excluding
never-reviewed items, forecast bucketing from today, leech and difficulty
buckets, and the `-1` sentinels on an empty collection. `activity_test.go`
covers `LogActivity`, `ActivitySince` and `ActivityWeeks` (local week
bucketing, minutes for `read`, row counts for everything else).
`stats_methods_test.go` covers `LogReadingSession` (a zero-second session
writes nothing), that `rescheduleExtract` and `PromoteExtract` each write
their own `process` row with the right `detail`, and that `GetStatistics`
assembles all four panels and every fixed-length series without error on an
empty collection. `stats-format.test.js` covers `barHeights`, `sparkPath`,
`humanDuration` and `fiBand`'s 7%/13% boundaries in isolation.

**None of that touches the wiring between two of those tested functions.**
See case 5 below — it is the one case in this plan with no substitute.

## Three things that are deliberate — do not report them as bugs

- **Statistics are a snapshot, not live.** They load once when you enter the
  Stats tab. Grading a card, finishing a reading session or processing an
  extract while Stats is *not* the active tab will not be reflected until you
  leave the tab and come back — see case 12.
- **Reading history before this feature does not exist and cannot be
  backfilled.** `activity_log` (schema v6) starts empty on every existing
  collection. The two log-backed charts (weekly reading minutes, weekly
  processed count) will show "collecting since …" until you generate fresh
  activity — there is no way to make them show your past reading.
- **`Next` never rotates to Stats.** `nextKind()` only ever returns `read`,
  `process`, `recall` or `empty`. Stats is reached only by clicking its tab
  directly — that is intentional, not a missed case.

## Before you start

You need a collection with Study mode reachable (Ctrl+Shift+S), and ideally
one with some existing cards, extracts and reading sources so you are not
building everything from nothing. Useful to have going in:

- at least one reading source with unread content, for case 8
- at least one extract sitting unprocessed, for case 9
- at least one card due or overdue, for cases 3–5

**[DB]** items read `<collectionDir>/learning.db` with `sqlite3`. Useful
tables for this feature:

```
sqlite3 <collectionDir>/learning.db \
  "SELECT kind, ref_id, at, detail, amount FROM activity_log ORDER BY at DESC LIMIT 10;"
sqlite3 <collectionDir>/learning.db \
  "SELECT grade, reviewed_at, elapsed_ms FROM review_logs ORDER BY reviewed_at DESC LIMIT 10;"
sqlite3 <collectionDir>/learning.db \
  "SELECT active, due, last_review, lapses, difficulty, created_at FROM items;"
sqlite3 <collectionDir>/learning.db \
  "SELECT active, created_at FROM extracts;"
```

## 1. The Stats tab appears and opens four panels

Open Study (Ctrl+Shift+S). Look at the tab bar.

**Expect:** five tabs — Next, Read, Process, Recall, Stats — in that order.
Click **Stats**.

**Expect:** the tab highlights active and the body shows four cards:
**Learning quality**, **Workload**, **Habit**, **Pipeline health**, laid out
in a responsive grid (resize the window narrower and confirm the cards
reflow to fewer columns rather than overflowing).

## 2. A fresh collection shows "not enough data", never `0%` or a broken chart

Open (or create) a collection with no review history, no active items, and
an empty `activity_log` — freshly initialized, nothing studied yet. Open
Stats.

**Expect:** every metric that depends on history reads **"Not enough data
yet"** in italics — forgetting index, retention, burden, daily time, oldest
open extract. Nothing shows a bare `0%`, `NaN`, or `Infinity`. Every chart
area renders (an empty or flat SVG, not a hole in the layout) — grade-mix
bars, difficulty bars, the forecast bars, the reviews-per-day sparkline and
the cards-per-week bars should all appear as their (empty) shapes rather
than being missing entirely. The two log-backed charts (reading minutes,
processed count) show "Collecting … since today" instead of an empty chart,
because a first Read or Process action already exists implicitly — see case
11 for the exact wording rule.

## 3. Forgetting index matches a hand-count

Note the current values in **[DB]** `SELECT grade, COUNT(*) FROM review_logs
WHERE reviewed_at >= strftime('%s','now','-30 days') GROUP BY grade;` (grade
1 = Again). Go to Recall and grade five or six cards, pressing **Again** on
at least one and something else on the rest. Return to Stats (leave the tab
and click it again, so it reloads — see case 12).

**Expect:** Forgetting index = (Again count ÷ total reviews in the last 30
days) × 100, rounded to the nearest whole percent, matching a hand count of
what you just graded plus whatever was already in the 30-day window.
Retention reads as `100 − forgetting index`.

**[DB]** `SELECT grade, COUNT(*) FROM review_logs WHERE reviewed_at >=
strftime('%s','now','-30 days') GROUP BY grade;` — grade 1 is Again.

## 4. Band colouring crosses at 7% and 13%

With the forgetting index visible from case 3, keep grading cards to move it
across the two thresholds (or, if it's easier, mostly-Again a batch to push
it above 13%, then mostly non-Again to pull it back under 7%).

**Expect:** the forgetting-index line's colour changes at the boundaries —
below 7% one colour ("low" — too easy, reviewing more than necessary), 7–13%
inclusive another ("ok" — the healthy band), above 13% a third ("high" —
losing material). Confirm the colour actually differs at each band; do not
just infer it from the JS unit test, which only exercises `fiBand()` in
isolation and never renders anything.

## 5. Workload's daily-time estimate is wired to the real average latency — the load-bearing case

**This is the one gap with no automated test.** `GetStatistics`
(`stats_methods.go`) computes `Quality.AvgLatencyMs` from `review_logs` and
passes that single number into `WorkloadStatsFor(now, avgLatencyMs)`, which
multiplies it by burden to get `dailyMinutes`. Every Go test for
`WorkloadStatsFor` calls it directly with a hand-picked literal
(`WorkloadStatsFor(now, 10000)`, `WorkloadStatsFor(now, 1000)`, …), and
`TestGetStatisticsAssemblesEveryPanel` — the only test that goes through
`GetStatistics` itself — runs on an **empty** collection where burden is
already `-1` and the multiplication never happens. Nothing in the suite
proves the real latency figure, rather than zero or a leftover stale value,
actually reaches the Workload card. This case is that proof, and it must be
done on a collection with **both** scheduled items **and** graded reviews —
neither alone is enough.

1. Make sure you have several active cards with real intervals (a `due` in
   the future and a `last_review` in the past — i.e., cards you have
   reviewed before, not brand-new ones). If needed, grade a handful of Recall
   cards now so they get scheduled.
2. **[DB]** Record the average latency yourself:
   `SELECT AVG(elapsed_ms) FROM review_logs WHERE reviewed_at >=
   strftime('%s','now','-30 days');` — call this **L** (milliseconds).
3. **[DB]** Record burden's inputs: `SELECT due, last_review FROM items
   WHERE active=1 AND last_review IS NOT NULL;` and hand-compute **B** = Σ
   1/max(1, (due−last_review)/86400) over those rows — or read Burden
   straight off the Workload card once you're in Stats, since case-3-style
   hand-checks aren't the point here.
4. Open Stats and read the **Workload** card's **Burden** and **Daily time**
   figures.
5. Compute B × L / 1000 / 60 (burden × average latency in ms, converted to
   minutes) and compare it to the displayed daily time (the display rounds
   into `Xh Ym` / `Xm` via `humanDuration`).

**Expect:** the displayed daily time is **non-zero** and lands within
rounding of your hand computation — not implausibly tiny (as it would be if
zero latency were silently substituted) and not `Infinity`/`NaN`. If burden
is a healthy-looking number but daily time reads "Not enough data yet" or an
implausible value while you do have both reviewed cards and scheduled items,
that is exactly the failure this case exists to catch — report it as a
priority finding, since it is the only check anywhere on this hand-off.

## 6. The forecast's first bar equals today's due count

Look at the Workload card's forecast chart (14 daily bars) and separately
switch to the Recall tab to read its due-count badge (or **[DB]** count
directly). Return to Stats.

**Expect:** the forecast's first bar (today) is proportional to the same
count of active items due today shown on the Recall badge / counted in the
sidebar's `read`/`process`/`recall` counts. Grade or reschedule a card so
today's due count changes by one, leave Stats and return.

**Expect:** the first bar updates to match the new count.

**[DB]** `SELECT COUNT(*) FROM items WHERE active=1 AND due <=
strftime('%s','now');` (overdue items are folded into "due today" —
`WorkloadStatsFor` clamps a negative day-index to 0).

## 7. Streak increments after studying on a new day, and shows zero after a miss

This one is slow to verify live across real calendar days; treat it as a
**[DB]**-checkable case rather than something to force through the UI in one
sitting.

Grade at least one card today. Open Stats and read **Current streak** on the
Habit card.

**Expect:** if you also studied yesterday, current streak is at least 2; if
today is the first day in a while, current streak is 1. Longest streak is
never smaller than current streak.

**[DB]** confirm the day-by-day input directly:
```
sqlite3 <collectionDir>/learning.db \
  "SELECT date(reviewed_at, 'unixepoch', 'localtime') AS day, COUNT(*) \
   FROM review_logs GROUP BY day ORDER BY day DESC LIMIT 10;"
```
Walk backward from today: current streak is the count of consecutive days
present in that list starting at today (0 if today is missing — a streak
already broken is not current). Longest streak is the longest run of
consecutive days anywhere in the full history, not just the tail shown here.

## 8. Reading a document logs a session; peeking an extract does not

**[DB]** first, note the current row count: `SELECT COUNT(*) FROM
activity_log WHERE kind='read';`

1. Open a document from Read, read for at least a minute (scroll or page so
   there's real engagement), then close it (`×` or Esc, or switch to another
   tab — any of the three leave-paths in `ReadingView`).
2. Go to Stats and note the reading-time figures (weekly reading minutes bar
   for the current week, once logging has started — see case 11 if this is
   your very first session).
3. Go back to Read, expand a document's extract tree in the sidebar, and
   **click an extract to peek it** (not the title — this opens the document
   without touching reading progress, per the sources-sidebar feature).
   Close it after a few seconds.
4. Return to Stats.

**Expect:** after step 1, exactly one new `read` row appears with `amount`
approximately equal to the seconds you spent (**[DB]** `SELECT kind, amount,
detail FROM activity_log ORDER BY at DESC LIMIT 5;`). After step 3 (the
peek), **no new row appears** — the row count and the reading-time chart are
unchanged from step 2. A session of well under a second (open and
immediately close) also logs nothing — `LogReadingSession` ignores
`seconds <= 0`.

## 9. Processing an extract — promote, dismiss and defer — each logs its own outcome

Take three different extracts (or one extract for each, in sequence, using
Undo/re-extract as needed) and process one of each way from the Process tab:
promote one to a card, dismiss one, and defer ("later") one.

**Expect:** three new rows in `activity_log` with `kind='process'`, one each
with `detail` exactly `promote`, `dismiss`, `later` (not `defer` — that is
the scheduler's own vocabulary, verbatim). Promotion has its own write-point
in the code (`PromoteExtract` does not funnel through `rescheduleExtract`),
so confirm specifically that the promote case is not silently missing
relative to the other two.

**[DB]** `SELECT ref_id, detail, at FROM activity_log WHERE kind='process'
ORDER BY at DESC LIMIT 5;`

## 10. The funnel counts match the sidebar and the Process badge

Note the current Process badge count on the Study tab bar and the extract
list in the sources sidebar. Open Stats and read the Pipeline health card's
**Extracts: N open / M total** and the Workload card's backlog line
(`Process: N`).

**Expect:** the "open" / "Process" figures agree with the Process tab's
badge number and with a manual count of unprocessed extracts in the
sidebar. Process one extract, leave Stats, come back.

**Expect:** the open count drops by exactly one and the total stays the
same (processing changes `active`, not row existence).

## 11. Log-backed charts show "collecting since …" until they have data

On a collection where `activity_log` is still empty (or freshly emptied —
use a fresh collection if you don't want to touch existing data), open
Stats.

**Expect:** the Habit card's reading-time chart and the Pipeline card's
processed-per-week chart both show **"Collecting reading time since
today"** / **"Collecting processing history since today"** instead of a
chart. Log one `read` session (case 8) or one `process` action (case 9),
then leave Stats and return.

**Expect:** the corresponding message is replaced by its bar chart, with
one bar reflecting the new activity. The other chart (whichever kind you
did *not* just log) still shows its own "collecting" message if it still
has zero rows of that kind. This is a per-`loggingSince` toggle, not
per-chart — both charts key off the same overall `loggingSince` value from
`ActivitySince()`, which is the earliest of *either* kind — so once **any**
activity has been logged, both charts switch out of "collecting" mode even
if one of them still has no data of its own kind yet. If you see one chart
render as empty axes (not a "collecting" message) while its own kind
genuinely has zero rows, note that — it means `loggingSince` came from the
other kind's activity, which is expected, not a bug.

## 12. Leaving Stats and returning reloads the numbers

While Stats is open, note a value (e.g. Burden or the Process backlog
count). Switch to Recall and grade a card, or switch to Process and process
an extract — anything that changes a number Stats displays. **Do not**
revisit Stats yet.

**Expect:** if you could see Stats and the study surface simultaneously you
would notice nothing updates there — statistics are a snapshot from load
time, not live (see the deliberate-behaviours note above).

Now click the **Stats** tab.

**Expect:** the numbers you changed are now reflected — not the stale
values from before. Then, with Stats already the active tab, click the
**Stats** tab again (the already-active tab).

**Expect:** nothing happens — no reload, no flicker, no fresh network call.
`enter('stats')` re-sets the same store value, and Svelte's store `set`
does not notify subscribers when the value is unchanged, so `StatsPanel` is
not destroyed and recreated on a same-tab click. This is why case 3 and case
5 above tell you to leave the tab and come back rather than just re-clicking
Stats.

## What I most want to hear about

- **Case 5, the latency hand-off.** This is the only place in the entire
  test suite — automated or manual — that distinguishes `WorkloadStatsFor`
  receiving the real average latency from receiving zero or a leftover
  value. Every Go test drives `WorkloadStatsFor` with a literal it chose
  itself, and the one test that calls `GetStatistics` end-to-end runs on an
  empty collection where the multiplication is skipped entirely. If the
  daily-time figure doesn't track your hand computation, this is the bug
  that was shipped without a test to catch it.
- **Case 8's peek guard.** Whether a peek genuinely logs nothing under real
  usage, not just under the Go-side `LogReadingSession(id, 0)` unit test —
  the guard lives in `ReadingView.svelte`'s three leave-paths, which have no
  automated coverage at all.
- **Case 9's promote path.** It is a separate write-point from `later` and
  `dismiss` by design (see the design doc's "Grounding" section) — exactly
  the kind of asymmetry that is easy to wire correctly for two paths and
  forget for the third.
- **Anything that reads as `0%`, `NaN`, or a missing chart area anywhere**,
  even outside the fresh-collection case in case 2 — those are precisely
  the failure modes the `-1` sentinel and the `has()` guard in
  `StatsPanel.svelte` exist to prevent, and a single missed spot would look
  like a real result rather than an obvious bug.
