# Incremental Reading — Phase 2b (Process & Unify) Design

**Date:** 2026-06-21
**Status:** Design approved, pending spec review
**Parent strategy:** `docs/superpowers/specs/2026-06-20-incremental-reading-design.md` (Phase 2, slice 2b)
**Builds on:** Phase 1 recall spine (`008`) + Phase 2a read & extract (`009`).

## 1. Context & slice

Phase 2 is cut into three slices: 2a (read & extract, done), **2b (this doc) — process & unify**, 2c (PDF/web adapters + explorer "Study this" bridge). 2b adds extract scheduling, the Process face, and folds the three current overlays into one Study surface with a Read·Process·Recall focus switch over an interleaved queue.

**Current state it composes:** Phase 1 left `StudyMode.svelte` (Ctrl+Shift+R, Recall over `items`); 2a left `SourcesPanel.svelte` + `ReadingView.svelte` (Ctrl+Shift+E, reading over `reading_elements`). `learning.db` is at schema v2 (`items`, `review_logs`, `reading_elements`). Extracts are `(from)` nodes in `reading/*.n4l` with **no scheduling state yet**.

## 2. Decisions (from brainstorming)

| # | Decision |
|---|---|
| 1 | **Extract discovery = register at creation + reconcile on sync.** 2a's `CreateExtract` registers the extract immediately (by running the classify pass on that one file); Build-Graph `Sync` reconciles (picks up hand-authored `(from)` nodes; retires promoted/deleted ones). |
| 2 | **One Study surface; the Sources library folds into the Read focus.** A single entry opens a surface whose default ("Next") serves the next due element of any kind; Read/Process/Recall are filters. Both old shortcuts map into it. |
| 3 | **Promotion is immediate.** Process "Make card"/"Cloze" writes the marker, saves, and re-syncs that one file so the recall item exists in the same session; the extract retires. |
| 4 | **Split is deferred.** 2b Process actions = Make card / Cloze / Refine (edit) / Later / Dismiss. |

## 3. Scope & non-goals

**In scope (2b):** the `extracts` table (schema v3) + deferral schedule + discovery; the Process face (focused editor + action bar) with immediate promotion; the unified `StudySurface` overlay (Next/Read/Process/Recall) composing `ReadingView` (Read), a new `ProcessView` (Process), and a `RecallCard` refactored out of `StudyMode` (Recall); the interleaved "what's next" queue.

**Out of scope:** Split (deferred); the exact 20/15/65 percentile mix + load-balancing (Phase 3 — 2b uses a simple due-ordered interleave with light kind-rotation); PDF/web adapters and the explorer "Study this" bridge (2c); priority-history transparency, the FSRS optimizer, and the advanced stats dashboard (Phase 3).

## 4. Data model

### 4.1 `extracts` table (schema v3)

```sql
CREATE TABLE extracts (
  id              TEXT PRIMARY KEY,   -- the extract node's qualified NodeID (same base as items)
  file_path       TEXT NOT NULL,
  text            TEXT NOT NULL,      -- display snippet
  from_anchor     TEXT,              -- the (from) "...char=N" value
  due             INTEGER,           -- unix; NULL = due now (new)
  deferral_count  INTEGER NOT NULL DEFAULT 0,
  active          INTEGER NOT NULL DEFAULT 1,
  created_at      INTEGER NOT NULL
);
CREATE INDEX idx_extracts_due ON extracts(due) WHERE active = 1;
```

`schema_meta.version` advances 2→3; the v3 step only `CREATE TABLE IF NOT EXISTS`s the new table + index (additive; existing tables untouched; idempotent re-open). The stepwise `migrate()` (added in 2a) gains a `case 2: ddlV3`.

### 4.2 Keying & the three element kinds

An extract's PK is the node's **qualified NodeID** (`filepath:chapter:id:shortid`) — the same base items key off (`NodeID::cardType::discriminator`). This lets reconcile match "this node is now a card": for a node that gained `# @card`/`# @cloze`, the extract row (`id == NodeID`) is retired and item rows (`NodeID::…`) are created. A node is classified by markers:

| Node shape | Kind |
|---|---|
| `StudyMeta != nil` (`# @card`/`# @cloze`/`# @study`) | **item** (existing `GenerateCards`) |
| no card markers, carries a `(from)` arrow | **extract** (upsert into `extracts`) |
| otherwise | neither |

## 5. Scheduling & the loop

**Extract schedule** (`ScheduleExtract(prev, action, now)`, machinery hidden): `next_due = now + base × 1.5^deferral_count` (base = 1 day), capped at 90 days.
- **Later** → `deferral_count++`, reschedule.
- **Dismiss** → `active = 0` (retired; the node stays in the file).
- **Make card / Cloze** (promote) → `active = 0`; the node becomes an item.

**Promotion (immediate):** `PromoteExtract(id, kind)` →
1. `content.InsertMarkerAboveNode(file, nodeLine, "# @card"|"# @cloze")`, saved atomically.
2. Re-run the classify/sync pass on that one file → `GenerateCards` creates the item(s) now; the extract row retires (its node now has `StudyMeta`).
3. The card is recallable in the same session.

**Interleaved queue:** `NextElement(focus)` returns the next due element + its kind across `reading_elements` (due to read), `extracts` (due to process), `items` (due to recall). `focus ∈ {auto, read, process, recall}`: `auto` picks the most-overdue across kinds with a light kind-rotation to avoid tunnel vision; the others restrict to one kind. `StudyCounts()` returns the three due counts for the switch badges. The precise mix/percentile machinery is deferred.

## 6. Architecture / components

| Unit | Responsibility | New/changed |
|---|---|---|
| `internal/learning/extracts.go` | `Extract` struct, `ScheduleExtract`, and `Store` CRUD (`UpsertExtract`, `RetireExtract`, `ListExtracts`, due query). Schema bump in `db.go` (`ddlV3`, `schemaVersion=3`, `case 2`). | **new** + `db.go` extend |
| `internal/learning/sync.go` | Extend the classify pass to upsert/retire extracts alongside items; expose a single-file variant for register-at-creation + promote re-sync. | extend |
| `internal/learning/queue.go` | `StudyCounts()` + `NextElement(focus)` interleave. | **new** |
| `internal/content/writeback.go` | `InsertMarkerAboveNode(path, nodeLine, marker)`. | extend |
| `reading_methods.go` / new `study_methods.go` | Bindings: `GetStudyCounts`, `GetNextElement`, `GetExtractQueue`, `PromoteExtract`, `DeferExtract`, `DismissExtract`; `CreateExtract` gains register-at-creation. | extend/new |
| `frontend` `components/ProcessView.svelte` | Process face: focused N4L editor on the extract (reused arrow autocomplete, EditCard-style) + action bar (Make card/Cloze/Refine/Later/Dismiss) + `(from)` peek + "Open in explorer". | **new** |
| `frontend` `components/RecallCard.svelte` | The card front→reveal→grade UI refactored out of `StudyMode` for reuse. | **new** (refactor) |
| `frontend` `components/StudySurface.svelte` + `stores/study.js` | The unified overlay: focus switch (Next/Read/Process/Recall) composing `ReadingView` + `SourcesPanel` (Read), `ProcessView` (Process), `RecallCard` (Recall); drives `GetNextElement`/`GetStudyCounts`. | **new** + store extend |
| `frontend` `App.svelte` | `Ctrl+Shift+R` → `StudySurface` (Next); `Ctrl+Shift+E` → `StudySurface` on Read. Remove the three standalone overlay mounts (folded in). | modify |

**Reuse:** `ReadingView` (2a) and `SourcesPanel` (2a) embed unchanged; `StudyMode`'s grading UI moves into `RecallCard` (StudyMode is retired as a standalone). `ProcessView` reuses the EditCard editor pattern + arrow autocomplete. Promotion reuses the Phase-1 dirty-buffer invariant (`savingPath` + `file:ids-injected` clean-reload).

## 7. Error handling / edge cases

- **Promote while the reading file is open in Read:** reuse the Phase-1 invariant — flush the notes buffer first, `savingPath`-suppress, and `file:ids-injected` clean-reload so the marker + item land without clobbering unsaved edits.
- **Empty queues:** each focus shows an empty state; "Next" shows "nothing due."
- **Promotion of a `# @cloze` extract whose text has no `{braces}`:** the marker is written but `GenerateCards` yields no card (Phase-1 behavior) — the extract still retires; surface a soft hint ("add `{braces}` to cloze").
- **Schema v3 on a v2 DB:** additive migration; no data loss; idempotent.
- **Reconcile keying:** because an extract's PK is the node's qualified NodeID, a node renamed/moved (chapter-title edit) retires the old extract and creates a new one (same limitation noted in Phase 1/2a — acceptable).
- **Char-anchor contract:** extracts carry the 2a `(from) "...char=N"` rune-offset anchor unchanged; Process's `(from)` peek/"Open in explorer" use it as-is.

## 8. Testing

- **Go unit:** `extracts` schema-v3 migrate-from-v2 + CRUD + `ScheduleExtract` (deferral growth, cap 90, Later/Dismiss/promote-retire); discovery classification (`(from)` node → extract; node that gains `# @card` → extract retired + item created); `InsertMarkerAboveNode` (marker lands directly above the target node line); `NextElement` interleave ordering + `StudyCounts`.
- **Manual quickstart matrix:** read → extract (appears in Process queue immediately) → Process "Make card" → it leaves Process and is recallable in Recall the same session; Later defers; Dismiss retires; the Next default rotates across kinds; Ctrl+Shift+R opens Next, Ctrl+Shift+E opens Read; the Sources library is reachable within Read.

## 9. Deferred to 2c / Phase 3 (recorded)

- Split; PDF/web adapters; explorer "Study this" bridge (2c).
- The exact 20/15/65 mix + load-balancing/auto-postpone; priority-history transparency; FSRS optimizer; advanced stats dashboard (Phase 3).
- **Cross-slice review checkpoint:** per the strategy doc §8, run an opus a+b+c coherence review at the end of 2c before merging the Phase-1→2 stack.
