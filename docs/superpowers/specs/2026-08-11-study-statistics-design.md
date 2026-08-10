# Study statistics: a fourth focus

**Date:** 2026-08-11
**Status:** Proposed — awaiting approval
**Feature area:** Study mode → a new Stats focus

## Summary

A fourth tab beside Read, Process and Recall, showing four panels: learning
quality, workload, habit, and pipeline health. Plus a small activity log for the
two things the database cannot currently reconstruct.

## Motivation

The Study bar already shows three counts, and they answer only "what is waiting
for me right now". Nothing answers whether the learning is *working* — whether
the forgetting index sits in the healthy band, whether tomorrow's workload is
sustainable, whether material is flowing from reading through to cards or
silently piling up in the extract queue.

`review_logs` has been recording every grade, its latency and the resulting
stability since schema v1, and nothing has ever read it back.

## Grounding in the current implementation

Surveyed before designing:

- **`review_logs`** (`internal/learning/db.go`, ddlV1) holds `item_id`,
  `reviewed_at`, `grade`, `elapsed_ms`, `old_state`, `new_stability`,
  `card_type`, written on every grade by `store.go:132`. It is the source for
  forgetting index, grade mix, latency, streaks and reviews-per-day.
- **`items`** holds `stability`, `difficulty`, `due`, `last_review`, `reps`,
  `lapses`, `state`, `created_at`, `active` — enough for burden, the due
  forecast, difficulty distribution, leeches and cards-created-over-time.
- **`extracts`** holds `created_at`, `deferral_count`, `active`. Creation over
  time is derivable; **processing is not** — `ApplyExtractSchedule`
  (`extracts.go:66`) flips `active` without recording when.
- **`reading_elements`** holds `last_read`, a single overwritten timestamp.
  There is **no reading history of any kind**.
- **Processing needs TWO write-points, not one.** `rescheduleExtract`
  (`study_methods.go:142`) handles `later` and `dismiss`, but `PromoteExtract`
  (`study_methods.go:92`) is a separate path that seeds a card and re-syncs the
  file without ever calling `ApplyExtractSchedule`. `ScheduleExtract`'s doc
  comment listing `promote` among its actions is misleading: nothing routes a
  promotion through it. Missing that second write-point would leave the pipeline
  panel showing every dismissal and no promotions — precisely backwards.
- **`StudyCounts` (`queue.go:12`)** already returns the read/process/recall
  backlog and is reused rather than reimplemented.
- **`StudySurface.svelte`** renders its tabs from a literal list of
  `[key, label]` pairs and switches on `$studyFocus`; a fourth entry follows the
  existing pattern.
- Grades are the FSRS four-point scale — `GradeAgain`, `GradeHard`, `GradeGood`,
  `GradeEasy` (`fsrs.go:10`).

## Decisions

- **A fourth Study tab, not a new top-level view.** Statistics elaborate the
  counts already in that bar, and you think about your learning in the session
  where you do it. Rejected: a separate `Ctrl+Shift` surface like the graph
  explorer, which costs another shortcut and another place to look for content
  visited occasionally. If the panels outgrow the tab, promoting them later is
  a move, not a rewrite.
- **The activity log records only what is unrecoverable.** Cards created is
  already `items.created_at` and extracts created is already
  `extracts.created_at`; logging those would duplicate existing data and
  introduce a second, disagreeing source of truth. Only two kinds are needed:
  `read` sessions and `process` outcomes.
- **A peek logs nothing.** Clicking an extract in the sources sidebar opens the
  document without touching reading progress; it must not register as a reading
  session either, or "time spent reading" counts glances.
- **Charts are inline SVG.** No charting library — this project keeps its
  dependency surface small, and bars, sparklines and a heatmap are a few dozen
  lines of path arithmetic. The arithmetic lives in a testable pure module, not
  in the component.
- **Every panel must survive an empty collection.** Forgetting index divides by
  a review count, burden divides by an interval, streaks need days. Each shows
  "Not enough data yet" rather than `0%` or `NaN`.
- **Forgetting curves are out of scope.** SuperMemo's central chart plots
  retention against retrievability, which here means self-joining each review
  log to its predecessor to recover the interval and prior stability. That is a
  feature in itself and deserves its own spec once the simpler panels prove
  useful.

## Architecture

### Schema v6 — `activity_log`

```sql
CREATE TABLE IF NOT EXISTS activity_log (
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  kind   TEXT NOT NULL,                 -- 'read' | 'process'
  ref_id TEXT NOT NULL DEFAULT '',      -- reading id, or extract id
  at     INTEGER NOT NULL,              -- unix seconds
  detail TEXT NOT NULL DEFAULT '',      -- 'later'|'dismiss'|'promote'
  amount INTEGER NOT NULL DEFAULT 0     -- read: session seconds
);
CREATE INDEX IF NOT EXISTS idx_activity_at ON activity_log(at);
```

`amount` is deliberately one integer rather than a JSON blob: the only
quantitative thing worth recording per session is its length, and a blob would
invite storing more without a schema decision.

Two write-points:

- **`read`** — `ReadingView`'s existing flush/close path, which already knows it
  is leaving a document. It reports seconds since mount. Guarded by the same
  `peekAt` check as the three progress-persist paths, so a peek logs nothing.
- **`process`** — `rescheduleExtract` (`study_methods.go:141`), recording the
  action verbatim as `detail`. The three values are exactly the strings
  `ScheduleExtract` already accepts — `later`, `dismiss`, `promote` — so the log
  cannot drift from the scheduler's own vocabulary.

### `internal/learning/stats.go`

Owns every query and computation and returns one `Statistics` struct. Pure
enough to test against a seeded temp database: given known rows, each number is
predictable.

| Field | Computed as |
|---|---|
| Forgetting index | `Again` reviews ÷ all reviews, last 30 days |
| Retention | `100 − forgetting index` |
| Grade mix | Count per grade, last 30 days |
| Leeches | Active items with `lapses ≥ 5` |
| Difficulty buckets | Active items by FSRS `difficulty` (1–10) in five buckets of two |
| Burden | `Σ 1/interval` over active items, `interval = max(1, due − last_review)` in days |
| Avg latency | Mean `elapsed_ms`, last 30 days |
| Daily minutes | `burden × avg latency` |
| Due forecast | Active item counts per day, next 14 days |
| Backlog | `StudyCounts` |
| Reviews per day | Grouped `reviewed_at`, last 84 days |
| Streaks | Consecutive days with ≥ 1 review — current and longest |
| Memorized | Active items with `reps > 0` |
| Cards per week | Grouped `items.created_at`, last 12 weeks |
| Funnel | Reading, extract and item counts with their unfinished shares |
| Oldest unprocessed | `now − min(extracts.created_at)` where `active = 1` |
| Read minutes per week | `activity_log` `kind='read'`, last 12 weeks |
| Processed per week | `activity_log` `kind='process'`, last 12 weeks |
| Logging since | `min(activity_log.at)`, or null |

### `frontend/src/lib/stats-format.js`

Pure and dependency-free, therefore testable under `node --test`: bucketing a
series into fixed-width bars, generating a sparkline `path` `d` attribute,
humanising a duration, and formatting the forgetting index against its target
band. The repo has **no Svelte component test harness**, so anything left in the
component is untested.

### `StatsPanel.svelte` and the tab

Four cards in a responsive grid, each rendering its own inline SVG. The tab is a
fourth `[key, label]` entry in `StudySurface`'s existing list, with the panel as
a new branch beside the read/process/recall branches. Statistics load once when
the tab is entered — not polled — and reload when re-entered.

## Error handling

| Case | Behaviour |
|---|---|
| No reviews yet | Learning-quality and habit panels show "Not enough data yet" |
| No active items | Burden and forecast show "Not enough data yet"; no division |
| `last_review` null (never reviewed) | Excluded from burden rather than counted as interval 0 |
| Activity log younger than 2 days | Log-backed charts show "collecting since &lt;date&gt;" |
| `GetStatistics` fails | One error line in the panel; the other tabs keep working |

## Testing

**Go:** each computation against a seeded temp database, where the expected
number is arithmetic rather than a snapshot — a forgetting index from a known
grade mix, a burden from known intervals, a streak across a deliberate gap,
forecast bucketing across a day boundary, and an empty collection producing
zero values and no `NaN`, no divide-by-zero, no panic. Separately, that a peek
writes no `read` row while a real reading session writes exactly one, and that
each of the three extract actions writes its own `detail`.

**JS:** `stats-format.js` — bar bucketing including an empty series and a single
point, sparkline path arithmetic, duration humanising across seconds/minutes/
hours, and the target-band classification at its 7% and 13% boundaries.

**Manual:** its own test-plan document — the tab, the four panels against a real
collection, the empty states on a fresh collection, and that reading a document
then reopening Stats shows the session while peeking an extract does not.

## Out of scope

- Forgetting curves and retrievability analysis.
- Per-chapter, per-file or per-card-type breakdowns.
- Export to CSV or clipboard.
- Knowledge-graph statistics (node, link and chapter counts) — they describe the
  collection's shape, not your learning.
- Acting on what you see: no promoting, suspending or editing leeches from the
  panel.
- Backfilling reading history, which cannot be reconstructed.
