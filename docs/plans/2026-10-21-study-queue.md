# Study Queue Implementation Plan

**Goal:** Replace Next's fixed read → process → recall order with a tested, per-Collection Mix of Turns, so a reading or Extract backlog can no longer starve Cards.

**Architecture:** A `learning.Queue`, owned by the Session, keeps the current Turn and a per-Session tally of Turns. It picks the kind furthest below its Mix share and serves that kind in `due ASC` order. The Mix lives in a new `study_settings` table. StudySurface's Next tab renders whatever `NextTurn()` returns, and the "is this Source due" rule moves to Go as `ReadingView.isDue`.

**Tech Stack:** Go 1.25, SQLite (`internal/learning`), Wails v2, Svelte 3.49.

**Spec:** `docs/specs/2026-10-21-study-queue-design.md`. Decisions are cited as **D1–D10**.

## Global Constraints

- Branch `105-study-queue`. In a worktree: copy `frontend/dist` in, symlink `frontend/node_modules` and exclude it locally. `frontend/wailsjs` is tracked but gitignored, so stage it with `git add -u`.
- `internal/learning` must not import `internal/sitting` (D1).
- The default Mix is exactly Source 20 / Extract 15 / Card 65. Ties go Card → Extract → Source.
- The "due" rule is the existing SQL, for every kind: `active=1 AND (due IS NULL OR due <= now)` for Cards and Extracts, and `done=0 AND (due IS NULL OR due <= now)` for Sources.
- Vocabulary is from `CONTEXT.md`: Turn, Mix, Study Queue, Study Element, Source, Extract, Card.
- `make test`, `make test-js`, `make lint` and `cd frontend && npx vite build` (with no `A11y:` lines) pass at every commit.

## Review Focus

1. **A Source served by the queue whose Sitting has not begun yet**, with Next re-asked before the reader mounts. Expected: the same Turn comes back and nothing is counted twice. *(Test in Task 2.)*
2. **The current Turn's element deleted, or deactivated by a sync.** Expected: the Turn counts as ended and a new one is served, with no error. *(Test in Task 2.)*
3. **A Mix with every due kind at weight 0** (e.g. 0/0/65 with no Cards due). Expected: nothing is served ("Nothing due"), even though Sources are due. *(Test in Task 2.)*
4. **The Collection switches while Next is open.** Expected: the new Session's tally starts at zero, and `NextTurn` never returns an element from the old Collection. *(Test in Task 3.)*
5. **Grading a Card in Next when the next Card due is the one just graded** (a lapse due again at once). Expected: the Turn ends (its due date moved) and the Mix chooses again. *(Test in Task 2.)*

---

### Task 1: The Mix in `study_settings`, and `ReadingView.isDue`

**Files:**
- Modify: `internal/learning/db.go` (`schemaVersion = 10`; a `case 9` that creates `study_settings(key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
- Create: `internal/learning/mix.go`, `internal/learning/mix_test.go`
- Modify: `reading_methods.go` (`ReadingView.IsDue bool json:"isDue"`, set in `toReadingView`), `reading_methods_test.go`

**Interfaces:**
- Produces:
  ```go
  type Mix struct { Source, Extract, Card int }   // json: source, extract, card
  var DefaultMix = Mix{Source: 20, Extract: 15, Card: 65}
  func (s *Store) LoadMix() (Mix, error)          // DefaultMix when unset or all zeros
  func (s *Store) SaveMix(m Mix) error            // error on any negative value
  func ReadingDue(r ReadingElement, now time.Time) bool   // !r.Done && (r.Due.IsZero() || !r.Due.After(now))
  ```
  The Mix is stored under the key `next_mix` as JSON.

- [ ] **Step 1: Write the failing tests:**
  - `TestLoadMixDefaultsWhenUnset` (== DefaultMix);
  - `TestSaveMixRoundTrips` (5/0/95);
  - `TestLoadMixAllZerosIsTheDefault`;
  - `TestSaveMixRejectsNegatives` (an error, and the stored value unchanged);
  - `TestMigrateV10CreatesStudySettings` (the table exists, version 10);
  - `TestReadingDue`: never scheduled → true, due in the past → true, due tomorrow → false, done → false;
  - `reading_methods_test.go` `TestGetReadingListMarksDueSources`: two rows, one due and one due tomorrow; `isDue` true/false.
- [ ] **Step 2:** `go test ./internal/learning ./ -run 'Mix|ReadingDue|MarksDue|V10'`. Expected: FAIL (undefined).
- [ ] **Step 3: Implement.** `toReadingView` sets `IsDue: learning.ReadingDue(r, time.Now())`.
- [ ] **Step 4:** `go test ./...`. Expected: PASS. Existing tests that assert `schemaVersion` still pass, because they compare against the constant.
- [ ] **Step 5:** Commit `feat(learning): a per-Collection Mix in study_settings; ReadingView says whether a Source is due`.

---

### Task 2: `learning.Queue`

**Files:**
- Modify: `internal/learning/queue.go`, `internal/learning/queue_test.go`

**Interfaces:**
- Consumes: Task 1's `LoadMix`, plus `DueReading`, `DueExtracts`, `DueQueue`, `GetReading`.
- Produces:
  ```go
  type Kind string
  const (KindSource Kind = "source"; KindExtract Kind = "extract"; KindCard Kind = "card")
  type Turn struct { Kind Kind; Source *ReadingElement; Extract *Extract; Card *CardView }
  type QueueDeps struct {
      Store       *Store
      OpenSitting func() (sourceID string, ok bool) // nil = none; peeks excluded by the caller
      Now         func() time.Time                  // nil = time.Now
  }
  func NewQueue(d QueueDeps) *Queue
  func (q *Queue) Next() (*Turn, error)            // nil, nil = nothing due
  func (s *Store) StillDue(k Kind, id string, now time.Time) (bool, error) // the due SQL for one row; false when the row is gone
  ```
  `Queue` holds `mu sync.Mutex`, `current *Turn`, `served map[Kind]int` and `n int`.

- [ ] **Step 1: Write the failing tests.** Use a real Store on `t.TempDir()`, a settable clock, and a fixture that seeds N due Sources, Extracts and Cards. A helper ends a Turn by moving its element's due date into the future.
  - `TestNextServesTheMixOverTwentyTurns`: with plenty of everything due and the default Mix, ending each Turn before the next, 20 Turns come out as Source 4, Extract 3, Card 13.
  - `TestFirstTurnIsACardOnATie`: a fresh Queue → KindCard.
  - `TestNextReturnsTheCurrentTurnUntilItEnds`: `Next` twice without ending gives the same element, and the tally counts 1 (Review Focus 1, for a Source Turn).
  - `TestATurnWhoseElementVanishedHasEnded`: delete the current Card's row (or set `active=0`) → `Next` serves a new Turn, with no error (Review Focus 2).
  - `TestAnOpenSittingResumesFirst`: `OpenSitting` returns `r1`, which is not the Mix's pick → KindSource with `r1`.
  - `TestKindsWithNothingDueAreSkipped`: only Extracts due → KindExtract every time.
  - `TestWeightZeroIsNeverServed`: Mix 0/0/65 and only Sources due → nil (Review Focus 3).
  - `TestNothingDueIsNil`.
  - `TestAMixChangeAppliesFromTheNextTurn`: after 5 Turns, `SaveMix(Mix{Source:100})` → every new Turn is a Source; the tally is not reset.
  - `TestAGradedCardDueAgainAtOnceStillEndsTheTurn` (Review Focus 5): the current Card's due date is set to `now+1s`, then the clock advances 2s → `Next` treats it as a new Turn (`n` increments) and lets the Mix choose.
  - `TestStudyCountsAndDueReading` stays unchanged: `DueReading` stays (D9).
- [ ] **Step 2:** `go test -race ./internal/learning -run 'Next|Turn|Sitting|Weight|Nothing|Mix'`. Expected: FAIL (undefined).
- [ ] **Step 3: Implement** `Next`, under `mu`:
  1. If `current != nil` and `StillDue(current)`, return `current`.
  2. Otherwise, if `OpenSitting` reports an id whose row `ReadingDue`s, serve that Source.
  3. Otherwise, for each kind with weight > 0, fetch its first due element (`DueReading`, `DueExtracts`, `DueQueue`, each with limit 1). Over the kinds that have one, pick the largest `w_k/W·(n+1) − s_k`, ties going Card → Extract → Source. Increment `served` and `n`, and store it as `current`.

  Card paths are not resolved here; that stays with the binding.
- [ ] **Step 4:** `go test -race ./internal/learning`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(learning): the Study Queue serves Turns by Mix share, one current Turn at a time`.

---

### Task 3: Session wiring and bindings

**Files:**
- Modify: `internal/sitting/tracker.go` (+ test): `func (t *Tracker) OpenSource() (string, bool)`, which returns the open Sitting's Source unless it is a peek
- Modify: `internal/collection/session.go` (+ test): `queue *learning.Queue`, built in `Open` with `OpenSitting: s.sittings.OpenSource`; `func (h Handles) Queue() *learning.Queue`
- Create: `queue_methods.go`, `queue_methods_test.go`
- Regenerate: `frontend/wailsjs`

**Interfaces:**
- Produces:
  ```go
  type TurnView struct {
      Kind    string             `json:"kind"`
      Source  *ReadingView       `json:"source,omitempty"`
      Extract *ExtractView       `json:"extract,omitempty"`
      Card    *learning.CardView `json:"card,omitempty"`
  }
  func (a *App) NextTurn() (*TurnView, error)   // nil when nothing is due
  func (a *App) GetStudyMix() (learning.Mix, error)
  func (a *App) SetStudyMix(m learning.Mix) error
  ```
  Paths are resolved as the existing bindings do: `toReadingView` with an absolute NotesPath, ExtractView's FilePath, and `resolveCardViewPaths`.

- [ ] **Step 1: Write the failing tests:**
  - `TestOpenSourceIgnoresPeeks` (sitting);
  - `TestOpenSourceReportsTheOpenSitting` (sitting);
  - `TestNextTurnServesACardFirst` (root, via `newTestApp`);
  - `TestNextTurnResumesAnOpenSitting` (root: `BeginSitting` on a Source, then `NextTurn` → that Source);
  - `TestStudyMixRoundTripsThroughTheBindings`;
  - `TestANewSessionStartsANewTally` (root, Review Focus 4): Turns served in Collection A; switch to Collection B; the first `NextTurn` serves from B, and the tie rule (Card first) holds again.
- [ ] **Step 2:** `go test ./... -run 'OpenSource|NextTurn|StudyMix|NewTally'`. Expected: FAIL.
- [ ] **Step 3: Implement**, then run `wails generate module`.
- [ ] **Step 4:** `go test -race ./... && make lint`. Expected: PASS. `App.d.ts` has `NextTurn`, `GetStudyMix` and `SetStudyMix`.
- [ ] **Step 5:** Commit `feat(study): NextTurn and the Mix bindings; the Session owns the Study Queue`.

---

### Task 4: The Next tab renders the Turn; the Mix in Stats

**Files:**
- Modify: `frontend/src/components/StudySurface.svelte` (`enter`, `nextKind`, `loadRead`, `onProcessAdvance`, `onRecallGrade`, and the Next branches of the template)
- Modify: `frontend/src/components/SourcesSidebar.svelte:42` (use `r.isDue`)
- Modify: `frontend/src/components/StatsPanel.svelte` (a "Next mix" section)
- Modify: `CHANGELOG.md`, `ROADMAP.md` (remove "Unified study queue" from Next)

**Interfaces:**
- Consumes: `NextTurn`, `GetStudyMix` and `SetStudyMix` (Task 3), and `ReadingView.isDue` (Task 1).

- [ ] **Step 1: StudySurface.**
  - Add `let turn = null`. `enter('next')` calls `turn = await NextTurn()`, and then:
    - `source`: `activeReading.set(turn.source)` unless a Source with that id is already active;
    - `extract`: `processId = turn.extract.id`;
    - `card`: `recallCard = turn.card`, `recallShownAt = Date.now()`.
  - `enter` for the other tabs keeps its loaders.
  - `loadRead` uses `r.isDue` instead of the due lambda.
  - Delete `nextKind` and its comment.
  - Template: the Next tab picks its branch from `turn?.kind` ('source' → the read area, 'extract' → ProcessView, 'card' → RecallCard, none → "Nothing due").
  - In Next, `onProcessAdvance` and `onRecallGrade` call `enter('next')` and ignore `GradeItem`'s returned Card. The reader's `onClose` already calls `enter('next')`.
- [ ] **Step 2: SourcesSidebar.** Replace the due lambda (:42) with `r.isDue`, and delete the comment that explains the NULL=0 encoding.
- [ ] **Step 3: StatsPanel.**
  - A "Next mix" `sp-card` with three labelled `<input type="number" min="0">` for Source, Extract and Card, loaded from `GetStudyMix()`.
  - A Save button calls `SetStudyMix` and shows its error, if any.
  - A line shows the shares as rounded percentages, e.g. "Source 20% · Extract 15% · Card 65%".
  - The inputs need visible labels, with no `A11y:` warnings.
- [ ] **Step 4:** `make test-js && cd frontend && npx vite build`. Expected: PASS, with no `A11y:` lines. `grep -n "nextKind\|r.due === 0\|r.due \* 1000" frontend/src/components/*.svelte` finds nothing.
- [ ] **Step 5: Manual check** in `make dev`, with a Collection that has due Sources, Extracts and Cards:
  1. Opening Study → Next shows a Card first.
  2. Working through Turns → Sources and Extracts are interleaved with Cards, not served all first.
  3. Leaving Next for another tab and coming back → the same Turn.
  4. Setting the Mix to 0/0/100 in Stats → Next serves only Cards.
  5. Closing a Source served by Next → Next moves on.
- [ ] **Step 6:**
  - `CHANGELOG.md`, under v0.2.0 → Added: "**Next mixes the queue.** Sources, extracts and cards take turns by a mix you set per collection in Stats (default 20 / 15 / 65), so a reading backlog no longer holds back reviews that are due."
  - `ROADMAP.md`: remove the "Unified study queue (§6.4)" paragraph from Next.
- [ ] **Step 7:** Commit `feat(study): Next serves Turns from the Study Queue; the Mix is set in Stats`.
