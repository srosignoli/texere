# Stats Readability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every Stats number a unit and a one-line meaning, and rebuild all seven charts on Chart.js with axes, units and hover values.

**Architecture:** The three weekly series become labelled `[]WeekCount` from Go so axis labels cannot drift from their buckets. Pure config builders in a new JS module produce Chart.js configs; the component renders them through the Svelte action pattern `ComputeCell.svelte` already uses.

**Tech Stack:** Go 1.25, Wails v2, Svelte 3.59, `chart.js@^4.5.1` (already a dependency), `node --test`.

## Global Constraints

- **This change is presentation only.** No statistic's computation changes. If you find yourself editing an arithmetic expression in `stats.go` other than to reshape a return value, stop — that is out of scope.
- **The `-1` sentinel guards and the two-day `settled()` rule in `StatsPanel.svelte` must survive untouched.** They are the two properties the previous review found real defects in. A `-1` reaching the screen would tell someone who has never studied that their recall is perfect.
- **`chart.js` is already a dependency and already in the bundle** (`ComputeCell.svelte:4` imports `chart.js/auto`). Do not add, upgrade or pin a charting package.
- **Follow `ComputeCell.svelte:43-47` for chart lifecycle** — a Svelte action, `new Chart(node, config)`, returning `{ destroy() { c.destroy() } }`.
- **Config builders must not import Chart.js.** That is what keeps them testable under `node --test`, where this repo has no component harness.
- **Weekly bucketing arithmetic does not change** — only the return shape gains a label.
- Go: `gofmt`, imports grouped stdlib / external / internal. JS/Svelte: 2-space indent, **no semicolons**, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.
- `wails` is not on PATH; it is at `/home/simone/go/bin/wails`.
- `git add frontend/wailsjs` as a bare directory exits non-zero because `runtime/` is gitignored. Add explicit file paths and confirm with `git show --stat`.
- `TestWatchCollectionAdded` in `internal/content` is a **known pre-existing fsnotify flake**. Ignore it.

## MANDATORY: negative-control every test you write

On the branch this builds on, **six** plausible-looking negative controls turned out to prove nothing: fixtures sampling only 13 and 20 around a boundary at 13, intervals of exactly 2 and 10 days hiding integer division, a single seeded extract making all-vs-active indistinguishable. Every one was caught only because someone ran the control and looked at the result.

Before committing, for each behaviour asserted: break the production code in the specific way the test exists to catch, run the test, confirm it **fails with a message naming the real problem**, then restore and re-run green. Report the actual failure output.

- A sabotage that does not compile is **not** a control. Change it until it runs and the test genuinely fails.
- If a control does not make its test fail, **say so plainly** and name the assertion that was supposed to catch it. Do not weaken a test until something goes red.
- Vary your fixtures. Identical rows hide ordering and grouping bugs.

## File Structure

- `internal/learning/activity.go` — `ActivityWeeks` returns `[]WeekCount`.
- `internal/learning/stats.go` — `WeekCount` type; `CardsPerWeek` returns `[]WeekCount`.
- `internal/learning/activity_test.go`, `internal/learning/stats_test.go` — restated assertions.
- `stats_methods.go` — `StatsView`'s three weekly fields change type.
- `frontend/src/lib/stats-charts.js` — pure Chart.js config builders.
- `frontend/src/lib/stats-charts.test.js` — its tests.
- `frontend/src/lib/stats-format.js` — `barHeights` and `sparkPath` deleted.
- `frontend/src/lib/stats-format.test.js` — their tests deleted.
- `frontend/src/components/StatsPanel.svelte` — canvases and the metric rewrite.
- `docs/superpowers/specs/2026-08-11-stats-readability-manual-test-plan.md`

---

### Task 1: Labelled weekly series

**Files:**
- Modify: `internal/learning/stats.go`, `internal/learning/activity.go`
- Modify: `internal/learning/stats_test.go`, `internal/learning/activity_test.go`
- Modify: `stats_methods.go`, `stats_methods_test.go`
- Regenerate: `frontend/wailsjs/go/main/App.d.ts`, `App.js`, `models.ts`

**Interfaces:**
- Consumes: the existing `midnight(t)` helper in `stats.go`.
- Produces:

```go
type WeekCount struct {
	WeekStart string `json:"weekStart"` // "2006-01-02", local, the bucket's first day
	Count     int    `json:"count"`
}

func (s *Store) ActivityWeeks(kind string, weeks int, now time.Time) ([]WeekCount, error)
func (s *Store) CardsPerWeek(weeks int, now time.Time) ([]WeekCount, error)
```

`StatsView.CardsPerWeek`, `.ReadMinsWeek` and `.ProcPerWeek` become `[]learning.WeekCount`.

**The bucketing arithmetic does not change.** Both methods keep their existing `start`, their `idx := int(time.Unix(at, 0).Sub(start).Hours() / (24 * 7))`, their `weeks <= 0` guard before `make`, and — for `ActivityWeeks` — the seconds-to-minutes division for `kind == "read"`. You are adding a label to each bucket, nothing else.

- [ ] **Step 1: Write the failing tests**

In `internal/learning/activity_test.go`, replace the body assertions of `TestActivityWeeksBucketsReadMinutesOldestFirst` — the existing `got[3] != 10` / `got[1] != 20` / `got[2] != 0` checks — with:

```go
	if got[3].Count != 10 {
		t.Errorf("this week = %d minutes, want 10 — the last entry must be the current week", got[3].Count)
	}
	if got[1].Count != 20 {
		t.Errorf("two weeks ago = %d minutes, want 20", got[1].Count)
	}
	if got[2].Count != 0 {
		t.Errorf("last week = %d, want 0 — a quiet week must be a zero, not a missing entry", got[2].Count)
	}
	// The label is what the chart's x-axis shows. If it drifts from the bucket
	// it names, every bar is captioned with the wrong week.
	wantLast := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	if got[3].WeekStart != wantLast {
		t.Errorf("last bucket starts %q, want %q — the final bucket is the week containing now", got[3].WeekStart, wantLast)
	}
	wantFirst := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local).AddDate(0, 0, -21).Format("2006-01-02")
	if got[0].WeekStart != wantFirst {
		t.Errorf("first bucket starts %q, want %q", got[0].WeekStart, wantFirst)
	}
	// Labels must be strictly increasing: two buckets sharing a label would make
	// the axis unreadable and hide a bucketing collapse.
	for i := 1; i < len(got); i++ {
		if got[i].WeekStart <= got[i-1].WeekStart {
			t.Fatalf("bucket %d starts %q, not after %q", i, got[i].WeekStart, got[i-1].WeekStart)
		}
	}
```

Change `TestActivityWeeksCountsRowsForNonReadKinds`'s `got[1] != 2` to `got[1].Count != 2`, and `TestActivityWeeksIgnoresOtherKinds`'s `got[1] != 0` to `got[1].Count != 0`.

In `internal/learning/stats_test.go`, no existing test covers `CardsPerWeek`. Add:

```go
func TestCardsPerWeekLabelsEachBucket(t *testing.T) {
	st, _ := Open(t.TempDir())
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	lr := now.AddDate(0, 0, -1)
	due := now
	seedItem(t, st, "recent", &due, &lr, 0, 5, now)
	seedItem(t, st, "older", &due, &lr, 0, 5, now.AddDate(0, 0, -14))

	got, err := st.CardsPerWeek(4, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d weeks, want exactly 4", len(got))
	}
	if got[3].Count != 1 {
		t.Errorf("this week = %d, want 1", got[3].Count)
	}
	if got[1].Count != 1 {
		t.Errorf("two weeks ago = %d, want 1", got[1].Count)
	}
	if got[2].Count != 0 {
		t.Errorf("last week = %d, want 0 — a quiet week is a zero, not a gap", got[2].Count)
	}
	wantLast := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	if got[3].WeekStart != wantLast {
		t.Errorf("last bucket starts %q, want %q", got[3].WeekStart, wantLast)
	}
}
```

In `stats_methods_test.go`, `TestGetStatisticsAssemblesEveryPanel` checks the three series are 12 entries long via a `map[string][]int`. Change that map's type to `map[string][]learning.WeekCount` — the length assertion is unchanged and still load-bearing.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/learning/ ./ 2>&1 | head -20`
Expected: compile failures — `got[3].Count undefined (type int has no field or method Count)`.

- [ ] **Step 3: Implement**

In `internal/learning/stats.go`, beside `DayCount`:

```go
// WeekCount is one bucket of a weekly series. WeekStart labels it with the local
// date of the bucket's first day, so a chart axis and the bucket it describes
// come from the same arithmetic and cannot drift apart.
type WeekCount struct {
	WeekStart string `json:"weekStart"`
	Count     int    `json:"count"`
}
```

In `CardsPerWeek`, change the signature to `([]WeekCount, error)`, the empty guard to `return []WeekCount{}, nil`, and build the labelled skeleton before filling:

```go
	out := make([]WeekCount, weeks)
	start := midnight(now).AddDate(0, 0, -7*(weeks-1))
	for i := range out {
		out[i].WeekStart = start.AddDate(0, 0, 7*i).Format("2006-01-02")
	}
```

then change the accumulation from `out[idx]++` to `out[idx].Count++`.

Apply the same three changes to `ActivityWeeks`: signature `([]WeekCount, error)`, guard `return []WeekCount{}, nil`, the labelled skeleton after `start` is computed, `out[idx] += amount` → `out[idx].Count += amount`, `out[idx]++` → `out[idx].Count++`, and the minutes conversion `out[i] /= 60` → `out[i].Count /= 60`.

In `stats_methods.go`, change the three `StatsView` field types from `[]int` to `[]learning.WeekCount`. Nothing else in that file changes — the three assignments already take whatever the methods return.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/learning/ -run 'ActivityWeeks|CardsPerWeek' -v` then `make test`
Expected: PASS.

- [ ] **Step 5: Negative controls (report both)**

1. In `ActivityWeeks`, label every bucket with the window start: `out[i].WeekStart = start.Format("2006-01-02")`. `TestActivityWeeksBucketsReadMinutesOldestFirst` must FAIL on the strictly-increasing check. Restore.
2. In `CardsPerWeek`, offset the label by one week: `start.AddDate(0, 0, 7*i+7)`. `TestCardsPerWeekLabelsEachBucket` must FAIL naming the wrong last bucket. Restore.

- [ ] **Step 6: Regenerate bindings and commit**

```bash
/home/simone/go/bin/wails generate module
git add internal/learning/stats.go internal/learning/activity.go \
  internal/learning/stats_test.go internal/learning/activity_test.go \
  stats_methods.go stats_methods_test.go \
  frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js frontend/wailsjs/go/models.ts
git commit -m "feat(stats): label each weekly bucket with its start date"
git show --stat HEAD
```

Confirm `models.ts` gained a `WeekCount` class.

---

### Task 2: Chart config builders

**Files:**
- Create: `frontend/src/lib/stats-charts.js`
- Create: `frontend/src/lib/stats-charts.test.js`

**Interfaces:**
- Consumes: nothing — dependency-free by design.
- Produces:

```
barConfig({ labels, values, yLabel, unit, color }) -> object
lineConfig({ labels, values, yLabel, unit, color }) -> object
```

Both return a Chart.js v4 config object. **Do not import Chart.js in this file** — building a config is plain object construction, and keeping the import out is what lets these run under `node --test`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/stats-charts.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { barConfig, lineConfig } from './stats-charts.js'

const base = { labels: ['Mon', 'Tue'], values: [3, 7], yLabel: 'reviews', unit: 'reviews', color: '#7ecfa0' }

test('barConfig carries the data and the axis title', () => {
  const c = barConfig(base)
  assert.equal(c.type, 'bar')
  assert.deepEqual(c.data.labels, ['Mon', 'Tue'])
  assert.deepEqual(c.data.datasets[0].data, [3, 7])
  assert.equal(c.options.scales.y.title.text, 'reviews')
  assert.equal(c.options.scales.y.title.display, true)
})

test('lineConfig differs from barConfig only in type', () => {
  const b = barConfig(base)
  const l = lineConfig(base)
  assert.equal(l.type, 'line')
  assert.equal(b.type, 'bar')
  assert.deepEqual(l.data.labels, b.data.labels)
  assert.equal(l.options.scales.y.title.text, b.options.scales.y.title.text)
})

// The whole point of the change: hovering a bar must say what it is.
test('the tooltip renders the value with its unit', () => {
  const c = barConfig({ ...base, unit: 'reviews' })
  const label = c.options.plugins.tooltip.callbacks.label
  assert.equal(label({ parsed: { y: 12 } }), '12 reviews')
})

test('a singular value still reads correctly', () => {
  const c = barConfig({ ...base, unit: 'minutes' })
  assert.equal(c.options.plugins.tooltip.callbacks.label({ parsed: { y: 1 } }), '1 minutes')
})

// An empty collection must still produce axes, or the reader sees a blank box
// with no clue what it would have shown.
test('an empty series still produces a valid config with its axis title', () => {
  const c = barConfig({ labels: [], values: [], yLabel: 'cards', unit: 'cards', color: '#7ecfa0' })
  assert.deepEqual(c.data.labels, [])
  assert.deepEqual(c.data.datasets[0].data, [])
  assert.equal(c.options.scales.y.title.text, 'cards')
})

test('missing arrays degrade to empty rather than throwing', () => {
  const c = barConfig({ yLabel: 'cards', unit: 'cards' })
  assert.deepEqual(c.data.labels, [])
  assert.deepEqual(c.data.datasets[0].data, [])
})

// Chart.js defaults to dark-on-light. Without this every label is invisible
// against the panel, which reads as a missing axis rather than a styling bug.
test('the dark palette reaches ticks, grid and axis titles', () => {
  const c = barConfig(base)
  for (const axis of ['x', 'y']) {
    assert.equal(c.options.scales[axis].ticks.color, '#8899aa', `${axis} ticks`)
    assert.equal(c.options.scales[axis].grid.color, '#2a3a4a', `${axis} grid`)
    assert.equal(c.options.scales[axis].title.color, '#8899aa', `${axis} title`)
  }
})

test('the y axis starts at zero so bar heights stay comparable', () => {
  assert.equal(barConfig(base).options.scales.y.beginAtZero, true)
})

test('the legend is off — one dataset needs no key', () => {
  assert.equal(barConfig(base).options.plugins.legend.display, false)
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `node --test frontend/src/lib/stats-charts.test.js`
Expected: FAIL — cannot resolve `./stats-charts.js`.

- [ ] **Step 3: Implement**

Create `frontend/src/lib/stats-charts.js`:

```js
// Chart.js config builders for the Stats focus.
//
// Deliberately does NOT import Chart.js: a config is a plain object, and
// keeping the import out is what lets these run under `node --test`. This repo
// has no Svelte component harness, so a config built inside the component would
// have no tests at all.

const TICK = '#8899aa'
const GRID = '#2a3a4a'

/**
 * @param {{labels?: string[], values?: number[], yLabel: string, unit: string, color?: string}} o
 */
export function barConfig(o) {
  return config('bar', o)
}

/** Same as barConfig but drawn as a line — used for the 84-day review series. */
export function lineConfig(o) {
  return config('line', o)
}

function config(type, { labels, values, yLabel, unit, color = '#7ecfa0' }) {
  return {
    type,
    data: {
      labels: Array.isArray(labels) ? labels : [],
      datasets: [{
        data: Array.isArray(values) ? values : [],
        backgroundColor: color,
        borderColor: color,
        borderWidth: type === 'line' ? 1.5 : 0,
        pointRadius: 0,
        fill: false,
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: false,
      plugins: {
        // One dataset per chart, so a legend would only repeat the card's title.
        legend: { display: false },
        tooltip: {
          callbacks: {
            // The reason this feature exists: a bar must say what it is worth.
            label: (ctx) => `${ctx.parsed.y} ${unit}`,
          },
        },
      },
      scales: {
        x: {
          ticks: { color: TICK, maxRotation: 0, autoSkip: true },
          grid: { color: GRID },
          title: { color: TICK },
        },
        y: {
          beginAtZero: true,
          ticks: { color: TICK, precision: 0 },
          grid: { color: GRID },
          title: { display: true, text: yLabel, color: TICK },
        },
      },
    },
  }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `node --test frontend/src/lib/stats-charts.test.js` then `make test-js`
Expected: all PASS.

- [ ] **Step 5: Negative controls (report both)**

1. Change the tooltip callback to `(ctx) => String(ctx.parsed.y)`. The tooltip test must FAIL showing `'12'` against `'12 reviews'`. Restore.
2. Change the `y` axis `title.color` to `'#000000'`. The dark-palette test must FAIL naming `y title`. Restore.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/stats-charts.js frontend/src/lib/stats-charts.test.js
git commit -m "feat(stats): Chart.js config builders with units and dark axes"
```

---

### Task 3: The panel — canvases and metric wording

**Files:**
- Modify: `frontend/src/components/StatsPanel.svelte`
- Modify: `frontend/src/lib/stats-format.js`
- Modify: `frontend/src/lib/stats-format.test.js`

**Interfaces:**
- Consumes: `barConfig`, `lineConfig` (Task 2); `humanDuration` and `fiBand` from `stats-format.js`; the labelled `[]WeekCount` series (Task 1).
- Produces: no new exports.

**Three things must survive this rewrite untouched.** Each has already been a real defect on this feature:
- the `has()` guard on every `-1` sentinel (`forgettingIndex`, `retention`, `burden`, `dailyMinutes`, `oldestOpenDays`),
- the `settled()` two-day rule and its "Collecting … since &lt;date&gt;" captions,
- the panel root's `position: absolute; inset: 0; overflow: auto` — `.ss-body` is `position: relative` with absolutely-positioned children, so anything else renders the panel invisible beneath them.

- [ ] **Step 1: Delete the dead SVG helpers**

`barHeights` and `sparkPath` exist only to hand-draw SVG and become dead in this task. Remove both from `frontend/src/lib/stats-format.js`, and remove their tests from `frontend/src/lib/stats-format.test.js` — the tests named `bars scale so the tallest fills the height`, `an all-zero series gives zero-height bars, not NaN`, `a single point still scales`, `sparkPath spans the full width and starts with a move`, and `sparkPath needs at least two points`. Keep `humanDuration` and `fiBand` and their tests.

Then `grep -rn "barHeights\|sparkPath" frontend/src/` must return nothing.

- [ ] **Step 2: Add the chart action and swap the canvases**

In `StatsPanel.svelte`, add to the script:

```js
  import Chart from 'chart.js/auto'
  import { barConfig, lineConfig } from '../lib/stats-charts.js'

  // Same shape as ComputeCell's: create on mount, destroy on teardown. No
  // update() needed — this panel renders its charts inside {#if s}, so the
  // action runs once with final data, and leaving the tab destroys the whole
  // component.
  function chart(node, config) {
    const c = new Chart(node, config)
    return { destroy() { c.destroy() } }
  }

  const weekLabels = (series) => (series ?? []).map((w) => w.weekStart)
  const weekValues = (series) => (series ?? []).map((w) => w.count)
```

Replace each of the seven `<svg>` blocks with a canvas in a fixed-height wrapper. The seven configs, using the axis table from the spec:

```svelte
<!-- Learning quality -->
<div class="sp-chartbox"><canvas use:chart={barConfig({
  labels: ['Again', 'Hard', 'Good', 'Easy'], values: s.quality.gradeMix,
  yLabel: 'reviews', unit: 'reviews', color: '#7dd3fc' })}></canvas></div>

<div class="sp-chartbox"><canvas use:chart={barConfig({
  labels: ['1–3', '3–5', '5–7', '7–9', '9–10'], values: s.workload.difficulty,
  yLabel: 'cards', unit: 'cards', color: '#b9a3e3' })}></canvas></div>

<!-- Workload -->
<div class="sp-chartbox"><canvas use:chart={barConfig({
  labels: s.workload.forecast.map((_, i) => (i === 0 ? 'today' : `+${i}d`)),
  values: s.workload.forecast,
  yLabel: 'cards due', unit: 'cards', color: '#e0b000' })}></canvas></div>

<!-- Habit -->
<div class="sp-chartbox"><canvas use:chart={lineConfig({
  labels: (s.habit.reviewsPerDay ?? []).map((d) => d.day),
  values: (s.habit.reviewsPerDay ?? []).map((d) => d.count),
  yLabel: 'reviews', unit: 'reviews', color: '#7ecfa0' })}></canvas></div>

<div class="sp-chartbox"><canvas use:chart={barConfig({
  labels: weekLabels(s.cardsPerWeek), values: weekValues(s.cardsPerWeek),
  yLabel: 'cards', unit: 'cards', color: '#7ecfa0' })}></canvas></div>

<div class="sp-chartbox"><canvas use:chart={barConfig({
  labels: weekLabels(s.readMinsWeek), values: weekValues(s.readMinsWeek),
  yLabel: 'minutes', unit: 'minutes', color: '#7dd3fc' })}></canvas></div>

<!-- Pipeline -->
<div class="sp-chartbox"><canvas use:chart={barConfig({
  labels: weekLabels(s.procPerWeek), values: weekValues(s.procPerWeek),
  yLabel: 'extracts', unit: 'extracts', color: '#b9a3e3' })}></canvas></div>
```

The last two stay inside their existing `{#if settled(...)}` branches, unchanged.

Add the wrapper style — Chart.js needs a sized parent when `maintainAspectRatio` is false:

```css
  .sp-chartbox { position: relative; height: 140px; margin: 8px 0; }
```

- [ ] **Step 3: Rewrite the metric readouts**

Replace every `<p class="sp-metric">` / `<p class="sp-sub">` readout with a three-part block:

```svelte
<div class="sp-stat">
  <span class="sp-label">Burden</span>
  <span class="sp-value">{s.workload.burden.toFixed(1)} reviews/day</span>
  <span class="sp-meaning">Estimated steady-state load once every card settles into its interval.</span>
</div>
```

The fourteen readouts, verbatim — label, value expression, meaning:

| Label | Value | Meaning |
|---|---|---|
| Forgetting index | `{pct(s.quality.forgettingIndex)}` | Share of reviews you failed. SuperMemo's healthy range is 7–13%. |
| Retention | `{pct(s.quality.retention)}` | Share you recalled successfully — the other side of the forgetting index. |
| Leeches | `{s.workload.leeches} cards` | Cards you have failed five times or more. |
| Burden | `{s.workload.burden.toFixed(1)} reviews/day` | Estimated steady-state load once every card settles into its interval. |
| Daily time | `{humanDuration(s.workload.dailyMinutes)}` | Burden multiplied by your average time per review. |
| Backlog | `{s.workload.backlog.read} read · {s.workload.backlog.process} process · {s.workload.backlog.recall} recall` | Waiting for you right now. |
| Current streak | `{s.habit.currentStreak} days` | Consecutive days with at least one review. |
| Longest streak | `{s.habit.longestStreak} days` | Your best run so far. |
| Memorized | `{s.workload.memorized} cards` | Cards that have been reviewed at least once. |
| Sources finished | `{s.pipeline.sourcesDone} of {s.pipeline.sources}` | Documents you have marked done. |
| Extracts | `{s.pipeline.extractsOpen} open of {s.pipeline.extracts}` | Passages taken from your reading, and how many still await processing. |
| Cards | `{s.pipeline.cards}` | Active cards in the collection. |
| Oldest unprocessed extract | `{s.pipeline.oldestOpenDays} days` | How long the oldest untouched passage has waited. |
| Last 30 days | `{s.pipeline.extractsMade30} extracts · {s.pipeline.cardsMade30} cards` | What you have created this month. |

The five sentinel-guarded readouts — forgetting index, retention, burden, daily time, oldest unprocessed extract — keep their existing `{#if has(...)}` / `{:else}` structure, with the `{:else}` branch rendering the same three-part block whose value is `Not enough data yet`. The `class:ok` / `class:low` / `class:high` binding on the forgetting index moves to its `.sp-value`.

Styles:

```css
  .sp-stat { margin: 8px 0; }
  .sp-label { display: block; color: #8899aa; font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; }
  .sp-value { display: block; color: #e8eaf0; font-size: 15px; font-weight: 600; }
  .sp-value.ok { color: #7ecfa0; }
  .sp-value.low { color: #7dd3fc; }
  .sp-value.high { color: #e0b000; }
  .sp-meaning { display: block; color: #5d7186; font-size: 11px; line-height: 1.4; }
```

Remove the now-unused `.sp-metric` and `.sp-sub` rules **only if** nothing still uses them — the "Collecting … since" captions use `.sp-sub sp-nodata`, so `.sp-sub` stays.

- [ ] **Step 4: Verify**

Run: `make test`, `make test-js`, `cd frontend && npx vite build`.

Expected: all green, and **no unused-CSS-selector warning naming any `.sp-*` selector**. Such a warning means a style survived whose markup did not, or vice versa.

Then confirm the three properties that must not have regressed:
- `grep -c "has(" frontend/src/components/StatsPanel.svelte` — must still be **5**.
- `grep -c "settled(" frontend/src/components/StatsPanel.svelte` — must still be **2**.
- `grep -n "sp-root" frontend/src/components/StatsPanel.svelte` — must still be `position: absolute; inset: 0; overflow: auto`.
- `grep -rn "barHeights\|sparkPath" frontend/src/` — must return nothing.

- [ ] **Step 5: Report the symptoms**

State what a user would see if each were wrong: a missing `has()` guard, a `.sp-chartbox` without a fixed height, and the chart action without its `destroy()`.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/StatsPanel.svelte frontend/src/lib/stats-format.js frontend/src/lib/stats-format.test.js
git commit -m "feat(stats): units, plain-English meanings and Chart.js axes"
```

---

### Task 4: Manual test plan

**Files:**
- Create: `docs/superpowers/specs/2026-08-11-stats-readability-manual-test-plan.md`

Match the house format of `docs/superpowers/specs/2026-08-11-study-statistics-manual-test-plan.md`. Read the panel as built before writing.

- [ ] **Step 1: Write the document**

Cover:

1. Every readout shows a label, a value **with a unit**, and a one-line meaning. Specifically: burden reads as a rate (`2.6 reviews/day`), not a bare number.
2. "Sources finished — 1 of 5 — Documents you have marked done" can no longer be read as "1 of 5 has something queued".
3. All seven charts have a labelled y-axis carrying a unit.
4. Hovering any bar or point shows its value with the unit.
5. The reading-per-week and processed-per-week charts are now distinguishable — their y-axis titles read `minutes` and `extracts`.
6. The three weekly charts' x-axis dates are real week-start dates, and the last is the current week. **[DB]** compare against `SELECT kind, datetime(MIN(at),'unixepoch','localtime') FROM activity_log GROUP BY kind;`
7. The forecast's x-axis reads `today, +1d … +13d` and its first bar matches the Recall badge.
8. On a fresh collection, the five sentinel readouts still say "Not enough data yet" — **no `-1`, no `0%`**. This is a regression check on the most important property of the previous feature.
9. The two log-backed charts still show their "Collecting … since &lt;date&gt;" captions until two days of history exist.
10. Charts survive leaving Stats and returning — no duplicated canvas, no leak, no blank box.
11. The panel still scrolls within its own area and does not overlap the Study tab bar.

State explicitly, so a tester does not file them as bugs: the meanings are deliberately verbose because the terms are jargon; the charts do not zoom, pan or click through; and nothing about how a statistic is computed changed in this release.

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-08-11-stats-readability-manual-test-plan.md
git commit -m "docs(test-plan): stats readability"
```

---

## Self-Review

**Spec coverage.** Chart.js for all seven charts → Task 3, configs from Task 2. Label/unit/meaning on every readout → Task 3 Step 3, all fourteen enumerated. `Sources: 1/5` → "Sources finished — 1 of 5" in the same table. Labelled weekly series from Go → Task 1. `barHeights`/`sparkPath` deleted → Task 3 Step 1. Sentinel guards and the two-day rule preserved → Task 3's three-property check in Step 4. Axis table → Task 3 Step 2. Manual coverage → Task 4.

**Placeholders.** None: every code step carries literal code, every test its assertions, and the fourteen readouts are enumerated rather than described.

**Type consistency.** `WeekCount{WeekStart, Count}` is defined in Task 1 with JSON tags `weekStart`/`count`, and Task 3's `weekLabels`/`weekValues` read exactly `w.weekStart` and `w.count`. `barConfig`/`lineConfig` take `{labels, values, yLabel, unit, color}` in Task 2 and are called with those keys in Task 3. `DayCount`'s existing `day`/`count` tags are used unchanged for the reviews-per-day series.

**One thing deliberately not fixed.** The previous review noted that forgetting index and retention each round independently and can display as summing to 101%. This plan rewrites those exact two lines and still does not fix it — the spec puts it out of scope, and changing a displayed value is a computation change disguised as presentation. It stays in the ledger.
