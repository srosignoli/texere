# Incremental Reading — Manual End-to-End Test Plan (Phase 1 + 2)

**Date:** 2026-06-22
**Covers:** Phase 1 (recall spine) + Phase 2a/2b/2c (read · extract · process · unify · PDF/web)
**Audience:** a tester driving the built app by hand.
**Grounded in:** `frontend/src/App.svelte`, `components/*.svelte`, `reading_methods.go`, `study_methods.go`, `learning_methods.go` (not the spec docs — shortcuts/bindings verified against source).

---

## 0. Setup & conventions

### Build & launch
```bash
make dev        # Wails hot-reload, or:
make build && ./build/bin/texere
```
Open (or create) a collection directory containing `.n4l` files. A `reading/` subdir and a `sources/` subdir get created on first import; `learning.db` is created at the collection root (machine-local, gitignored).

### Keyboard shortcuts (verified in `App.svelte`)
| Shortcut | Action |
|---|---|
| **Ctrl/Cmd+G** | Build Graph (parse + inject `# @id`s + sync items/extracts into `learning.db`) |
| **Ctrl/Cmd+Shift+R** | Standalone **Recall** overlay (`StudyMode`) |
| **Ctrl/Cmd+Shift+S** | Unified **Study surface** on the **Next** queue (`StudySurface`) |
| **Ctrl/Cmd+Shift+E** | Study surface on the **Read** focus |
| **Ctrl/Cmd+Shift+M** | Toggle log panel (watch for errors here) |
| **Esc** | Close log panel / overlay |

### In-card keys (`RecallCard`)
- **Space / Enter** → reveal answer
- **1 / 2 / 3 / 4** → grade **Again / Hard / Good / Easy** (only after reveal)

### DB inspection helper
Use any pure-SQLite client (e.g. `sqlite3 <collection>/learning.db`). Key queries are collected in **Appendix A**. Keep the **log panel (Ctrl+Shift+M)** open during testing — import/sync/promote errors surface there as toasts and log lines.

### Pass/fail
Each case lists **Steps** and **Expected**. A case **passes** only if every Expected bullet holds. Record the build commit (`git rev-parse --short HEAD`) and OS with results.

---

## Phase 1 — Recall spine (FSRS)

### TC-1.1 Card discovery via Build Graph
**Steps**
1. In a `.n4l` file add a card:
   ```
   # @card
   What is the capital of France? >> Paris
   ```
2. Save. Press **Ctrl+G** (Build Graph).

**Expected**
- No error toast; log panel shows a successful parse/sync.
- A `# @id` line is injected above the node and the editor reloads cleanly (no unsaved-buffer clobber).
- `SELECT count(*) FROM items;` increased by 1 (a basic card split on `>>`).
- The new `items` row has `active=1` and `due IS NULL` (a new card).

### TC-1.2 Cloze card generation
**Steps**
1. Add a cloze:
   ```
   # @cloze
   The mitochondrion is the {{powerhouse}} of the {{cell}}.
   ```
2. Save, **Ctrl+G**.

**Expected**
- Items are created for the cloze (one per blanked span per Phase-1 behavior).
- A cloze with **no** `{{braces}}` (or only single `{ }`) creates **no** item (negative check — add one and confirm count does not rise).

### TC-1.3 Recall overlay shows a card + reveal
**Steps**
1. Press **Ctrl+Shift+R**.

**Expected**
- `StudyMode` runs `SyncStudyItems()` then loads the due queue; a card front is shown.
- Pressing **Space** reveals the back; the four grade buttons (Again/Hard/Good/Easy) appear.
- If nothing is due, an explicit empty state ("nothing due") shows — not a blank panel.

### TC-1.4 Grading advances FSRS and persists (core)
**Steps**
1. With a new card shown, note its `id` (Appendix A.2 — newest item).
2. Reveal (Space) and press **3** (Good).
3. Re-query the row and the review log (Appendix A.2, A.3).

**Expected**
- The card leaves the queue; the next card (or empty state) shows.
- `items` row now has: `reps=1`, `due` set to a **future** unix time, `stability>0`, `state` advanced, `last_review` set.
- A new `review_logs` row exists for that `item_id` with `grade=3`, non-null `elapsed_ms`, an `old_state`, and `new_stability`.

### TC-1.5 Grade differentiation (Again < Easy)
**Steps**
1. Create two fresh identical new cards, grade one **Again (1)** and one **Easy (4)**.
2. Compare their `due` values.

**Expected**
- `due(Again) < due(Easy)` — Again reschedules sooner; Easy pushes further out.

### TC-1.6 Second review carries prior state forward (not reset)
**Steps**
1. Take the card from TC-1.4 (now `reps=1`). Force it due: `UPDATE items SET due=strftime('%s','now')-1 WHERE id='<id>';`
2. Reopen Recall (Ctrl+Shift+R), grade **Good** again.

**Expected**
- `reps=2` (not back to 1).
- New `due` interval is **longer** than the TC-1.4 interval (stability accumulated — confirms FSRS reconstructs prior card state rather than treating each review as new).
- A second `review_logs` row exists for that `id`.

### TC-1.7 Build-Graph dirty-buffer invariant
**Steps**
1. Open a reading/card file, make an **unsaved** edit, then press **Ctrl+G**.

**Expected**
- `# @id` injection does not discard the unsaved edit silently; the editor reloads via the `file:ids-injected` clean-reload path with edits preserved/flushed (no FSRS-history-losing buffer clobber).

---

## Phase 2a — Read & extract (text/markdown)

### TC-2a.1 Import a text/markdown source
**Steps**
1. Open Sources (it appears inside the Study surface Read focus; or Ctrl+Shift+E). Click **+ Text**, pick a `.txt`/`.md` file.

**Expected**
- File is copied into `sources/`; a `reading/<…>.n4l` notes scaffold is created (`# @source`, `# @progress char=…`).
- A row appears in `reading_elements` with `medium='text'` (Appendix A.4).
- The source opens immediately in `ReadingView` via the `TextSourcePane`; `0%` progress initially.

### TC-2a.2 Create an extract from a selection
**Steps**
1. In the open text source, select a passage. Trigger Extract.

**Expected**
- A `(from)` node is written **inline** into the reading notes `.n4l` with anchor `char=N` (rune offset, not byte/UTF-16).
- The extract is registered immediately (register-at-creation): it appears in the **Process** queue this session without a full Build Graph.
- `SELECT * FROM extracts WHERE active=1;` shows the new row with `from_anchor` like `"<refPath> char=<N>"` (Appendix A.5).

### TC-2a.3 Progress + resume
**Steps**
1. Scroll/advance in the source, then close it (Read focus → close).
2. Reopen the same source.

**Expected**
- `# @progress char=N` is written on leave; reopening resumes at that offset.
- The source-list `percent` reflects position (>0%).

---

## Phase 2b — Process & unify

### TC-2b.1 Unified surface focus switch
**Steps**
1. Press **Ctrl+Shift+S**.

**Expected**
- Tabs **Next / Read / Process / Recall** are present (verified in `StudySurface`).
- Each tab shows a count badge from `GetStudyCounts` (read/process/recall due counts).
- **Next** auto-rotates: it serves Read if reading is due, else Process, else Recall; empty → "nothing due".

### TC-2b.2 Extract appears in Process immediately after creation
**Steps**
1. Create an extract (TC-2a.2), then switch to the **Process** tab.

**Expected**
- The just-created extract is the/an item in the Process queue (no Build Graph needed).
- `ProcessView` shows the extract text in an editable N4L editor plus the action bar: **→ Make card**, **{ } Cloze**, **⏳ Later**, **✕ Dismiss**.

### TC-2b.3 Promote → Make card (immediate, recallable same session)
**Steps**
1. In Process on an extract, click **→ Make card**.
2. Switch to the **Recall** tab (or Ctrl+Shift+R).

**Expected**
- `# @card` is inserted directly above the node and the file saved; that one file re-syncs.
- The `extracts` row for that id has `active=0` (retired).
- A new `items` row (keyed off the same NodeID base) exists with `active=1` and is recallable **this session**.

### TC-2b.4 Promote → Cloze with no braces (soft hint)
**Steps**
1. On an extract whose text has no `{braces}`, click **{ } Cloze**.

**Expected**
- The `# @cloze` marker is written and the extract retires (`active=0`), but `GenerateCards` yields no item.
- A soft hint surfaces (e.g. "add `{braces}` to cloze") — not a hard error.

### TC-2b.5 Later defers (geometric backoff)
**Steps**
1. On an extract, note `deferral_count` and `due`. Click **⏳ Later**. Re-query.

**Expected**
- `deferral_count` increments by 1; `due` ≈ `now + 1day × 1.5^deferral_count` (cap 90 days). Row stays `active=1`.

### TC-2b.6 Dismiss retires
**Steps**
1. On an extract, click **✕ Dismiss**. Re-query.

**Expected**
- `extracts` row `active=0`; it leaves the Process queue. The `(from)` node **remains** in the `.n4l` file (only the schedule is retired).

### TC-2b.7 Reconcile on Build Graph
**Steps**
1. Hand-author a `(from)` node in a reading file (no card markers). Save, **Ctrl+G**.
2. Separately, hand-add `# @card` above an existing extract node, save, **Ctrl+G**.

**Expected**
- The hand-authored `(from)` node is picked up as a new `extracts` row.
- The node that gained `# @card` has its `extracts` row retired and an `items` row created (reconcile).

### TC-2b.8 Mixed-marker idempotency (regression)
**Steps**
1. In ONE file, place both a `# @card` node and a `# @`-less `(from)` extract node, neither with `# @id` yet. Save, **Ctrl+G** twice.

**Expected**
- Both id-injection passes succeed without corrupting line anchoring (regression for the two-pass `.Line` shift bug). Re-running Build Graph is idempotent — no duplicate items/extracts, no shifted markers.

---

## Phase 2c — PDF & web adapters

### TC-2c.1 Import a PDF
**Steps**
1. Sources panel → **+ PDF**, choose a `.pdf`.

**Expected**
- PDF copied into `sources/` (suffix on name collision); notes scaffold + `reading_elements` row with `medium='pdf'`, `# @progress page=1`.
- `PdfSourcePane` renders page 1 (pdf.js canvas + selectable text layer); page nav (prev/next/jump) works.
- On first load total pages reported via `SetReadingProjLen` so `percent` is sane (page 1 of N ≈ low %, not always 0%).

### TC-2c.2 PDF extract with page anchor
**Steps**
1. Select text on page N in the PDF, Extract.

**Expected**
- `(from)` anchor is `"<refPath> page=<N>"` (page, not char).
- Extract appears in Process. A selection spanning two pages anchors to its **start** page.

### TC-2c.3 PDF resume
**Steps**
1. Navigate to page K, close the source, reopen.

**Expected**
- `# @progress page=K` written on leave; reopen resumes at page K. Out-of-range page clamps into range.

### TC-2c.4 Import a web article (security-critical)
**Steps**
1. Sources panel → **+ Web**, paste an article URL at the prompt.

**Expected**
- Fetch → Readability extract → sanitize → images inlined → self-contained `sources/<slug>.html` saved; `reading_elements` row `medium='web'`, `source_url=<original URL>`.
- `WebSourcePane` renders the rich article **with its images visible inline**.
- **Security:** open the saved `sources/<slug>.html` in a text editor and confirm:
  - **no `<script>`**, no `on*=` handler attributes, no `<iframe>/<object>/<embed>/<style>/<link>`;
  - **no remote `src=`/`href=` resource loads** — every `<img>` is a `data:` URI (so opening the file makes zero network requests).

### TC-2c.5 Web extract char anchor + "Open original"
**Steps**
1. Select text in the rendered article, Extract.
2. Click **Open original**.

**Expected**
- `(from)` anchor is `"<refPath> char=<N>"` where N is a **rune offset into the rendered article text** (the pane reports rendered-text length via `SetReadingProjLen`, so the char basis matches what's displayed).
- "Open original" opens `source_url` in the system browser (`BrowserOpenURL`).

### TC-2c.6 Import error atomicity
**Steps**
1. **+ Web** with a bad/unreachable URL (e.g. `https://example.invalid/x`) or a non-2xx page.

**Expected**
- An import-error toast; **nothing partial** is left behind — no orphan `sources/` file, no notes scaffold, no `reading_elements` row.

### TC-2c.7 Corrupt / password-protected PDF
**Steps**
1. Import a deliberately corrupt or password-protected `.pdf`.

**Expected**
- `PdfSourcePane` shows an error state; the element/notes are not silently dropped (graceful, not a crash).

---

## Cross-phase integration (the full loop)

### TC-INT.1 Read → Extract → Process → Recall (one pass, mixed media)
**Steps**
1. Import a **web** article and a **PDF**.
2. From each, create an extract (char anchor from web, page anchor from PDF).
3. In **Process**, promote the web extract via **Make card** and the PDF extract via **Cloze** (give it `{braces}` first by editing in the Process editor).
4. Go to **Recall** and grade each new card **Good**.
5. Inspect the DB.

**Expected**
- Both extracts end `active=0` in `extracts`; two new `active=1` `items` rows exist and were graded (each has a `review_logs` row, `reps=1`, future `due`).
- The originating `(from)` nodes still carry their correct `char=`/`page=` anchors in the `.n4l` files.
- The **Next** queue, after all promotions/grades, reflects the new state (no stale duplicates).

### TC-INT.2 Schema migration v1→v4 on an existing DB
**Steps**
1. Back up `learning.db`. If you have a pre-Phase-2c DB, open it with the current build; else verify via Appendix A.6.

**Expected**
- `schema_meta.version = 4`; `reading_elements` has `medium` (default `'text'`) and `source_url` (default `''`) columns; existing rows preserved (additive, no data loss); reopening is idempotent.

### TC-INT.3 Standalone vs unified entry coherence
**Steps**
1. Open **Ctrl+Shift+R** (standalone StudyMode) and grade a card.
2. Open **Ctrl+Shift+S** → Recall tab.

**Expected**
- Both surfaces share the same item queue/grade logic (both built on `RecallCard`); a card graded in one is reflected (gone/ rescheduled) in the other.

---

## Appendix A — DB inspection queries

Run against `<collection>/learning.db`:

```sql
-- A.1 item count
SELECT count(*) FROM items;

-- A.2 newest / specific item scheduling state
SELECT id, active, reps, lapses, state, stability, difficulty, due, last_review
FROM items ORDER BY rowid DESC LIMIT 5;

-- A.3 review history for an item
SELECT item_id, reviewed_at, grade, elapsed_ms, old_state, new_stability, card_type
FROM review_logs ORDER BY rowid DESC LIMIT 5;

-- A.4 reading elements (medium dispatch + source_url)
SELECT id, file_path, medium, source_url, due, deferral_count, active FROM reading_elements;

-- A.5 active extracts (the Process queue source of truth)
SELECT id, file_path, substr(text,1,40) AS snippet, from_anchor, due, deferral_count, active
FROM extracts WHERE active=1 ORDER BY (due IS NULL), due;

-- A.6 schema version + columns
SELECT version FROM schema_meta;
PRAGMA table_info(reading_elements);  -- expect medium, source_url present
```

**Due-queue semantics to remember:** a card/element/extract is "due" when `active=1 AND (due IS NULL OR due <= now)`; `due IS NULL` means brand-new. After grading, a future `due` removes it from the queue until that time.

---

## Appendix B — What this plan does NOT cover (deferred, out of scope)

These are recorded as Phase-3 / later follow-ups and should **not** be treated as failures here:
- Explorer **"Study this"** bridge; Process **Split** action.
- FSRS **optimizer** (weight retraining) — the app uses default FSRS v6 weights; `elapsed_ms` is logged but does not drive intervals yet.
- The exact **20/15/65 queue mix** + priority transparency, auto-postpone/load-balancing, and the advanced **stats dashboard**.
- Web article **re-fetch/refresh**; PDF highlight persistence beyond extract; audio/video media; scroll-resume refinements for char media.
