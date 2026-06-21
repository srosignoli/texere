# Incremental Reading — Strategy & Design

**Date:** 2026-06-20
**Status:** Design approved, pending spec review
**Scope:** Strategy + process model for SuperMemo-style incremental reading in Texere, plus a phased decomposition. The first implementation plan covers **Phase 1 (the recall spine)** only.

## 1. Problem

Texere should support incremental reading in the spirit of SuperMemo: read a source a little at a time, pull out fragments ("extracts"), develop those fragments into connected understanding, and retain them through spaced repetition. The hard part is modelling this evolving, scheduled process when the substrate is **plain `.n4l` text files** (source of truth, versioned in git) plus a rebuildable knowledge graph and a learning database — not a monolithic application database like SuperMemo's.

## 2. Goals & non-goals

**Goals**
- A read → extract → develop → recall loop that lives inside Texere's existing N4L editor, graph, and read view.
- Keep `.n4l` the human-readable source of truth; keep the model's "machinery" (intervals, priorities, scheduling math) invisible to the user.
- Reuse what exists (parser/writeback, editor + arrow autocomplete, graph explorer) and add only what's missing (a scheduling engine, a source layer, a study surface).
- Respect the no-CGo constitution: all new dependencies pure Go.

**Non-goals (deferred, see §8)**
- Audio/video sources, topic/curation scheduling, the FSRS optimizer, priority-history transparency, the advanced stats dashboard, an embedded graph panel in Process, and portability/sync of scheduling state.

## 3. Decisions (from brainstorming)

| # | Decision |
|---|---|
| 1 | **Source model:** reference the original via `(from)` as the universal backbone (read it in a medium-aware pane; extracts are N4L nodes). "Import as N4L reading-doc" (text2N4L-style) is an explicit on-demand action for text sources, **deferred past Phase 2**. |
| 2 | **Medium-agnostic `Source`:** every source = `{ original artifact, text projection, typed anchor }` with a per-medium adapter. New media are additive adapters. |
| 3 | **UX = two visible piles, machinery hidden.** One **Study surface** with a **Read · Process · Recall** focus control. No mix sliders, A-factor knobs, or priority percentiles in the UI. |
| 4 | **Process = focused N4L authoring**, not just buttons: the editor opened on the extract so the user develops it with typed arrows / ditto lines / links, **plus** a thin study-action bar (Make card / Cloze / Refine / Split / Later / Dismiss). No embedded graph in Process for v1; the orbit peek is deferred. Pure exploration stays in the existing graph explorer, reached via an "Open in explorer" escape hatch (and the explorer gets a reciprocal "Study this" action — Phase 2). |
| 5 | **State durability:** earned scheduling state (due dates, FSRS params, deferral counts, reading position, review logs) lives in **`learning.db` (SQLite), local-only, gitignored**, keyed by an auto-assigned `# @id`. Content stays in `.n4l` (versioned). Portability is a later concern. |
| 6 | **N4L representation** uses existing markers (§5.3) — no new syntax. |
| 7 | **Phase-1 media:** PDF + local text/HTML + web articles (URL). Audio/video deferred. |
| 8 | **Phasing:** recall spine first, then the reading/extract loop, then enhancements. |
| 9 | **Pinned dependencies:** pure-Go FSRS (`github.com/open-spaced-repetition/go-fsrs`) and pure-Go SQLite (`modernc.org/sqlite`). No CGo. |

**Spec correction:** `Texere_Specification_v1.3.md` calls `learning.db` "rebuildable" and gitignores it. That is true for the BadgerDB graph but **false for review history**, which is earned and not reconstructable from `.n4l`. The doc should be corrected: `learning.db` is gitignored and **machine-local (not rebuildable)**; losing it loses scheduling history. (Acceptable for now per Decision 5.)

## 4. The model

### 4.1 Three element kinds, distinguished by markers

| Kind | What it is in `.n4l` | Scheduled to |
|---|---|---|
| **Reading element** | a referenced source: `# @ref "sources/x.pdf"` + `# @progress <anchor>` | resurface to *continue reading* |
| **Extract** | a node carrying a `(from) "<anchor>"` arrow, auto-assigned `# @id`, usually under `_sequence_`, with no card markers yet | resurface to *process/develop* |
| **Item (card)** | an extract (or any node) that has `# @card` / `# @cloze` (or sits inside a `# @study` block) | FSRS recall |

The scheduler classifies a node purely from its markers, so the three kinds are emergent from the text, not a separate registry.

### 4.2 The medium-agnostic `Source`

```
Source := { uri, medium, original, projection(), anchorScheme }
  projection() -> text stream the user reads/extracts from
  anchor       -> typed location embedded in (from) strings
```

| Medium (Phase 1) | original | projection | anchor in `(from)` |
|---|---|---|---|
| PDF | the PDF in `sources/` | extracted text per page (pdf.js) | `kahneman.pdf p.20` |
| Local text / HTML | the file in `sources/` | the text itself | `notes.md ¶12` / `char=4500` |
| Web article | saved HTML in `sources/` (fetched from URL) | Readability-extracted article text | `web char=4500` |

### 4.3 State boundary

- **`.n4l` (git, source of truth):** extracts, cards, `(from)` anchors, `# @id`, `# @progress`, all study markers.
- **`learning.db` (SQLite, local-only, gitignored):** item scheduling/FSRS state, review logs, reading positions, deferral counts. Join key: `# @id`.
- The only writebacks to `.n4l` are the ones the parser already supports: assigning `# @id`, adding `# @card`/`# @cloze`/`# @study`, and updating `# @progress`.

## 5. The workflow

### 5.1 Study surface

A single surface with a **Read · Process · Recall** focus switch. The queue interleaves all due elements underneath; the switch filters what is surfaced. "Done reading for now" = click Process or Recall; the source pane disappears.

- **Read face:** medium-aware source pane beside the N4L notes. Selecting text → **Extract**: creates a `(from)` node (auto `# @id`) in the notes and bumps `# @progress` on the source. Leaving reschedules the reading element.
- **Process face:** the N4L **editor** focused on the extract (develop it with typed arrows, ditto lines, links via arrow autocomplete + arrow palette) **plus** a thin study-action bar (Make card / Cloze / Refine=edit / Split / Later / Dismiss) and the `(from)` peek + "Open in explorer" affordances. Marking `@card`/`@cloze` promotes the extract to an item in place.
- **Recall face:** FSRS grading — front → reveal → four grades (Again/Hard/Good/Easy). "View source" jumps to the `(from)` anchor.

### 5.2 Scheduling (machinery hidden; sane fixed defaults)

- **Items:** FSRS v6, desired retention 0.9, four grades.
- **Extracts:** `next = now + base × 1.5^deferral_count`, capped at 90 days (base = 1 day). "Later" increments `deferral_count`; "Make card/Cloze" removes it from the extract queue (it becomes an item); "Dismiss" retires it.
- **Reading elements:** resurface with a simple growing interval (default: next day; "Later" pushes it out) until the source is marked done.
- **Unified queue:** interleaved; an internal default mix (≈20% reading / 15% extracts / 65% items) governs ordering. None of these numbers are user-facing.

## 6. Architecture

| Concern | Location | Status |
|---|---|---|
| Markers, `(from)`, auto `# @id`, `# @progress` writeback | `internal/content/` | **exists** (`parser.go`, `writeback.go`) — minor gaps only |
| Knowledge graph / explorer (link targets, "Open in explorer") | `internal/graph/`, `internal/explorer/` | **exists** |
| N4L editor + arrow autocomplete (Read/Process panes) | `frontend` `SourceTab` / `EditCard` | **exists** — reuse |
| **Scheduling engine** (SQLite, FSRS, queue) | `internal/learning/` | **new** (empty today) |
| **Source layer** (Source abstraction + adapters) | `internal/source/` (new) + frontend pdf.js | **new — Phase 2** |
| **Study surface** (Read/Process/Recall, reader pane, recall UI) | `frontend` (new mode) | **new** |
| Wails bindings | `*_methods.go` | **new** |

**Data stores:** `learning.db` (SQLite via `modernc.org/sqlite`) for scheduling; existing BadgerDB graph and Bleve index unchanged; `.n4l` files unchanged in role.

## 7. Phase 1 — The recall spine (this is what the first plan implements)

**Outcome:** working spaced repetition over cards authored in N4L today (no sources/extracts yet). This is the engine extracts and reading elements plug into later.

### 7.1 Scope

- **Card generation (lean subset):**
  - **Cloze cards** — a node or ditto line marked `# @cloze` (or `{braces}` inside a `# @study` block): one card per brace group; front = line with that group hidden, back = full line.
  - **Basic forward arrow cards** — a studyable arrow on a `# @card`/`# @study` node: front = `"<node text> — <arrow long name>?"`, back = the arrow target.
  - **Deferred to a later phase:** reverse (paired-arrow) cards and `_sequence_` recall cards. The schema must not preclude them (store a `card_type` column).
- **Item lifecycle / discovery:** on Build Graph (and on demand), scan parsed files for studyable nodes → ensure each has a `# @id` (writeback if missing) → upsert item rows in `learning.db`. Items whose markers were removed are **retired** (flagged inactive; review logs kept), never hard-deleted.
- **Queue:** due items, `WHERE due <= now ORDER BY due` (priority ordering deferred); a configurable daily cap is out of scope for Phase 1.
- **Recall UI:** a Study mode entry; the Recall face (front → reveal → four grades); each grade calls FSRS and persists the new state + a review log.
- **No** Read/Process faces, no source layer, no extract scheduling in Phase 1.

### 7.2 `learning.db` schema (Phase 1)

```sql
CREATE TABLE items (
  id           TEXT PRIMARY KEY,   -- "<# @id>::<card_type>::<discriminator>"
  node_id      TEXT NOT NULL,      -- the node's # @id
  file_path    TEXT NOT NULL,
  card_type    TEXT NOT NULL,      -- 'cloze' | 'basic'  (room for 'reverse','sequence')
  front        TEXT NOT NULL,
  back         TEXT NOT NULL,
  active       INTEGER NOT NULL DEFAULT 1,
  -- FSRS state
  stability    REAL,
  difficulty   REAL,
  due          INTEGER,            -- unix seconds; NULL = new
  last_review  INTEGER,
  reps         INTEGER NOT NULL DEFAULT 0,
  lapses       INTEGER NOT NULL DEFAULT 0,
  state        INTEGER NOT NULL DEFAULT 0, -- FSRS state enum
  created_at   INTEGER NOT NULL
);
CREATE INDEX idx_items_due ON items(due) WHERE active = 1;

CREATE TABLE review_logs (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id     TEXT NOT NULL,
  reviewed_at INTEGER NOT NULL,
  grade       INTEGER NOT NULL,   -- 1..4
  elapsed_ms  INTEGER,
  old_state   INTEGER,
  new_stability REAL,
  card_type   TEXT
);
CREATE INDEX idx_logs_item ON review_logs(item_id);

CREATE TABLE schema_meta ( version INTEGER NOT NULL );
```

A small migration runner keyed off `schema_meta.version` creates/upgrades the DB at open.

### 7.3 Backend surface (Wails bindings, new `*_methods.go`)

- `SyncStudyItems() -> {added, retired, total, err}` — discover items from parsed files, writeback `# @id`, upsert/retire rows.
- `GetDueQueue(limit int) -> []CardView` — due items (id, front, back, card_type, node provenance).
- `GradeItem(itemID string, grade int, elapsedMs int) -> CardView|done` — apply FSRS, persist state + log, return the next card or a done signal.
- `GetStudyStats() -> {due, new, total}` — minimal counts for the Recall entry badge.

### 7.4 FSRS

Wrap `github.com/open-spaced-repetition/go-fsrs` behind a small `internal/learning` interface (`Schedule(card, grade, now) -> card`) so the optimizer and weight tuning can be added later without touching callers. Default weights, retention 0.9.

### 7.5 Testing (Phase 1)

Project convention = Go unit tests + a manual quickstart matrix.
- **Go unit:** schema create/migrate; item discovery from a fixture `.n4l` (cloze + basic arrow cards; `# @id` assigned; marker removal retires items, logs kept); FSRS scheduling (new → again/good progressions; due math); queue ordering by `due`.
- **Manual quickstart matrix:** mark a node `# @cloze`, Build, enter Study, see the card, grade it, confirm it reschedules and the log row appears; remove the marker, re-sync, confirm the item retires but history remains.

## 8. Phase 2 and beyond (sketch — separate spec/plan later)

- **Phase 2 — Reading & extracts (headline loop):** `internal/source/` adapters (PDF / local text-HTML / web URL); reading-element + extract scheduling; the Read face (source pane + notes, Extract → `(from)` node) and Process face (focused editor + study-action bar); the Study surface with the Read/Process/Recall focus control and the interleaved queue; explorer "Study this" bridge.
- **Phase 3+ (later):** audio/video transcript adapters; orbit peek in Process; reverse + sequence cards; topic/curation scheduling (spec §6.2); priority management + `priority_history` transparency; FSRS optimizer; advanced stats dashboard; "import as N4L reading-doc"; portability/sync of scheduling state.

**Review checkpoint (do not skip):** each slice (2a/2b/2c) gets its own opus final whole-branch review at its end. Those are scoped per-slice and CANNOT see cross-slice seams. So **at the end of 2c (before merging the Phase-1→2 stack), run an additional opus cross-slice (a+b+c) coherence review** scoped to `main..HEAD`, explicitly checking the integration seams: 2b's Process face operating on 2a-created extracts; the unified Study surface folding the reading queue into the FSRS item queue; and whether 2b/2c relocation honors the 2a rune-offset `char=` anchor contract. (Optionally `/code-review ultra` for a deeper multi-agent pass.)

## 9. Error handling & edge cases (Phase 1)

- **DB open/migrate failure:** surface via the existing log-panel/error-toast channel; Study mode shows an error state rather than crashing.
- **Node edited/deleted after reviews:** discovery retires the item (`active = 0`); review logs are retained for future stats. If the same `# @id` reappears, the item is reactivated with its prior FSRS state.
- **`# @id` writeback vs. a dirty editor buffer:** sync writes `# @id` via the existing atomic writeback path; if the file is open and dirty, defer the writeback and warn (reuse the feature-007 "background edited" notice pattern), so the user's unsaved edits are never clobbered.
- **Cloze with no `{braces}`:** produces no card; not an error (a `# @cloze` marker with no braces is a no-op, optionally surfaced as a soft validation hint).
- **First review of a new item:** FSRS new-card defaults; `due = NULL` means "new", surfaced after due cards up to any future cap.

## 10. Open questions (recorded, not blocking Phase 1)

- Breadth of card-type generation (reverse/sequence) and exactly how paired-arrow reverse cards map — revisit when Phase 1's basic/cloze path is proven.
- Portability/sync of scheduling state (sidecar vs. committed DB) — revisit if multi-machine use becomes a need.
- Daily review caps / load balancing — deferred until real review volumes exist.
