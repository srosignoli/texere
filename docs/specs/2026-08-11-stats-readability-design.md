# Stats panel: units, meaning, and readable charts

**Date:** 2026-08-11
**Status:** Proposed — awaiting approval
**Feature area:** Study mode → Stats focus

## Summary

Give every number a unit and a one-line meaning, and rebuild all seven charts on
Chart.js so they carry axes, units and hover values. Return the three weekly
series labelled from Go so axis labels cannot drift from the buckets they
describe.

## Motivation

The Stats focus shipped with correct arithmetic and unreadable presentation.
Three problems, reported from use:

- **`Burden: 2.6`** — jargon with no unit. A reader cannot tell whether 2.6 is
  cards, hours, or a score, let alone that it estimates steady-state daily load.
- **`Sources: 1 / 5`** — reads naturally as "1 of 5 sources has something
  queued". It actually means "1 of your 5 sources is finished". The bare
  fraction invites the misreading.
- **Seven charts with no axes, no units and no hover values.** The reading-time
  chart and the processed-per-week chart are visually identical twelve-bar
  charts of different quantities in different cards.

## Grounding in the current implementation

- **`chart.js@^4.5.1` is already a dependency** (`frontend/package.json`) and
  already used by `ComputeCell.svelte:4,45` via `import Chart from
  'chart.js/auto'`. The original statistics spec's "no charting library"
  constraint was wrong on its own terms; the library is already in the bundle,
  so adopting it here costs nothing new.
- **`ComputeCell.svelte:43-47`** establishes the lifecycle pattern: a Svelte
  action, `new Chart(node, config)`, returning `{ destroy() { c.destroy() } }`.
  It has no `update`, which is fine here — `StatsPanel` renders its charts
  inside `{#if s}`, so the action runs once with final data, and switching focus
  destroys and recreates the whole component.
- **`StatsPanel.svelte`** renders seven `<svg>` charts and thirteen metric
  lines — fourteen readouts, since the two streaks share one line. Its `-1` sentinel guards (`has()`) and the two-day `settled()` rule are
  correct and must survive unchanged.
- **`internal/learning/stats.go`** already returns `ReviewsPerDay` as
  `[]DayCount{Day, Count}` — a labelled series. The three weekly series
  (`CardsPerWeek`, and `ActivityWeeks` for read and process) return bare
  `[]int`.
- **`frontend/src/lib/stats-format.js`** exports `barHeights`, `sparkPath`,
  `humanDuration`, `fiBand`. The first two exist only to hand-draw SVG.

## Decisions

- **Chart.js for all seven charts**, following `ComputeCell`'s action pattern.
  Rejected: hand-rolling axes, tick labels and hit-testing for tooltips onto the
  existing SVG, which is a meaningful amount of geometry to get right and to
  test, for a library the project already ships.
- **Every metric gets label, value-with-unit, and a one-line meaning.** Rejected:
  units alone, which fixes `2.6` → `2.6 reviews/day` but still leaves "burden"
  unexplained; and hover-only explanations, which are found only by someone who
  already suspects they are missing something.
- **The three weekly series become labelled, from Go.** `[]WeekCount{WeekStart,
  Count}`, matching the existing `DayCount` precedent. Rejected: deriving week
  labels in the frontend, which duplicates the Go bucketing arithmetic in a
  second place and inherits the DST drift already recorded against it — the
  labels would eventually disagree with the buckets they name.
- **`barHeights` and `sparkPath` are deleted**, with their tests. They exist
  only to draw SVG by hand and become dead on the day the charts move over.
  `humanDuration` and `fiBand` stay.
- **The `-1` sentinel guards and the two-day rule are untouched.** They are the
  two properties the previous review found defects in; this change is
  presentation only and must not disturb them.

## Architecture

### `frontend/src/lib/stats-charts.js` (new)

Pure config builders returning Chart.js config objects. Pure because building a
config never imports Chart.js — which is what makes them testable under
`node --test`, where this repo has no component harness.

```
barConfig({ labels, values, yLabel, unit, color }) -> object
lineConfig({ labels, values, yLabel, unit, color }) -> object
```

Both apply the dark palette (`#2a3a4a` grid and borders, `#8899aa` ticks and
titles), set `scales.y.title` to `yLabel`, and install a tooltip label callback
rendering `"<value> <unit>"`. `barConfig` is used by six charts; `lineConfig` by
the 84-day reviews series.

### `StatsPanel.svelte`

Replaces each `<svg>` with `<canvas use:chart={config}>`, where `chart` is the
three-line action copied from `ComputeCell`'s shape. Metric lines become a
`<div class="sp-stat">` of three elements: label, value, meaning.

The fourteen readouts and their new wording (the streaks split onto their own
lines):

| Metric | Value | Meaning |
|---|---|---|
| Forgetting index | `11%` | Share of reviews you failed. SuperMemo's healthy range is 7–13%. |
| Retention | `89%` | Share you recalled successfully — the other side of the forgetting index. |
| Leeches | `7 cards` | Cards you have failed five times or more. |
| Burden | `2.6 reviews/day` | Estimated steady-state load once every card settles into its interval. |
| Daily time | `~16 min` | Burden multiplied by your average time per review. |
| Backlog | `12 read · 34 process · 84 recall` | Waiting for you right now. |
| Current streak | `12 days` | Consecutive days with at least one review. |
| Longest streak | `41 days` | Your best run so far. |
| Memorized | `1,284 cards` | Cards that have been reviewed at least once. |
| Sources finished | `1 of 5` | Documents you have marked done. |
| Extracts | `34 open of 212` | Passages taken from your reading, and how many still await processing. |
| Cards | `1,284` | Active cards in the collection. |
| Oldest unprocessed extract | `23 days` | How long the oldest untouched passage has waited. |
| Last 30 days | `41 extracts · 67 cards` | What you have created this month. |

### Backend

`ActivityWeeks` and `CardsPerWeek` return `[]WeekCount` instead of `[]int`:

```go
type WeekCount struct {
	WeekStart string `json:"weekStart"` // "2006-01-02", local, the bucket's first day
	Count     int    `json:"count"`
}
```

Bucketing logic is unchanged — only the return shape and the label. `StatsView`'s
`CardsPerWeek`, `ReadMinsWeek` and `ProcPerWeek` change type accordingly.

### Axes

| Chart | x | y |
|---|---|---|
| Grade mix | Again / Hard / Good / Easy | reviews |
| Difficulty | band (1–3, 3–5, 5–7, 7–9, 9–10) | cards |
| Forecast | date, today → +13 | cards due |
| Reviews per day | date, 84 days | reviews |
| Cards per week | week starting | cards |
| Reading per week | week starting | minutes |
| Processed per week | week starting | extracts |

## Error handling

| Case | Behaviour |
|---|---|
| Empty series (all zeros) | Chart renders with axes and a flat baseline; the axis labels carry the meaning the bars cannot |
| A `-1` sentinel | Unchanged — the metric shows "Not enough data yet" and no chart is drawn for it |
| Log younger than two days | Unchanged — the caption replaces the chart |
| Chart.js throws on a malformed config | The action is per-canvas, so one failed chart cannot blank the panel |

## Testing

**JS:** `stats-charts.js` — axis titles carry the right unit; the tooltip
callback renders `"12 reviews"`; an empty series still produces a valid config
with labels; the dark palette is applied to grid, ticks and titles; `barConfig`
and `lineConfig` differ only in type.

**Go:** the same bucketing assertions as today, restated against `WeekCount` —
oldest first, the last bucket being the week containing `now`, quiet weeks
present as zeros, `read` summed as minutes and other kinds counted as rows — plus
that each entry's `WeekStart` is the local date of its bucket's first day.

**Manual:** its own test-plan document. Hovering every chart shows a value with
its unit; the two twelve-bar charts are now distinguishable by their axis titles;
burden reads as a rate; and the sources line can no longer be misread.

## Out of scope

- Any change to how a statistic is computed. This is presentation only.
- Zoom, pan, click-through or export on the charts.
- Per-chapter or per-card-type breakdowns.
- Revisiting the deferred findings from the statistics review (DST bucket drift,
  missing "Not enough data yet" captions on the habit panel, the 101% rounding
  pair). They are recorded in the ledger and are not what this change is about.
