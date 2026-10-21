# Study Queue — design

Date: 2026-10-21 · Branch: `105-study-queue` · Vocabulary: `CONTEXT.md` (Study Element, Source, Extract, Card, Topic, Turn, Mix, Study Queue, Sitting, Session) · Roadmap: "Unified study queue (§6.4)"

## Problem

Next walks a fixed order, read → process → recall, decided by a reactive statement in `StudySurface.svelte:131-147` (`nextKind`). Any Source waiting to be read, or any Source already open, always wins. A reading or Extract backlog therefore starves Cards, the one kind with a real deadline: FSRS Cards lose retention when they are reviewed late.

The policy is spread across the view:
- the "is this Source due" rule is written in SQL (`queue.go:14-15`) and twice in JS (`StudySurface.svelte:116` and `SourcesSidebar.svelte:42`), and both JS copies rely on a NULL due date being encoded as 0;
- `enter('next')` loads all three kinds and lets `nextKind` choose;
- after a grade, `GradeItem` hands back the next due Card, which keeps Next on Cards for as long as any are due.

None of this can be tested.

## Decisions (agreed)

1. **A `learning.Queue`** in `internal/learning/queue.go`, owned by `collection.Session` next to the Sitting Tracker. It is built with the Store, a way to ask which Source has an open Sitting, and a clock. It must not import `internal/sitting`; the Sitting question comes in as a function.
2. **One operation: `Next() → Turn`.** A Turn is `{Kind, Source | Extract | Card}`, or nil when nothing is due. The binding is `NextTurn()`.
3. **The current Turn is kept, and re-asking returns it.** `Next` returns the current Turn, counted once, until it has ended. **A Turn has ended when its Study Element is no longer due.** Grading a Card, any Extract action (promote, defer, dismiss), and ending a Sitting all move the element's due date. The view can therefore call `NextTurn()` as often as it likes.
4. **An open Sitting resumes first.** When there is no current Turn, a Source with an open Sitting (not a peek) is served before the Mix is consulted.
5. **Otherwise the Mix decides the kind.** It serves the kind furthest below its share: the largest `w_k / W · (n + 1) − s_k`, where:
   - `w_k` is the kind's Mix weight;
   - `W` is the sum of the weights of the kinds that have something due and a weight above 0;
   - `n` is the number of Turns served this Session;
   - `s_k` is the number served of that kind.

   Kinds with nothing due, or with weight 0, are skipped. Ties go Card → Extract → Source. Within a kind, the order stays `due ASC`.
6. **The tally is per Session and in memory.** It starts at zero when a Collection opens and is not rebuilt from the logs. A Turn is counted when it is served.
7. **The Mix** is three non-negative whole numbers, Source / Extract / Card, turned into shares when used.
   - The default is 20 / 15 / 65.
   - 0 means that kind is never served. All zeros means the default.
   - It is set per Collection and stored in a new key/value table, `study_settings`, at schema **v10**.
   - It is read on every `Next`, so an edit applies from the next Turn on, against the same tally.
   - Bindings: `GetStudyMix()` and `SetStudyMix(mix)`. `SetStudyMix` rejects negative values.
8. **The view** renders whatever `NextTurn()` returns in the Next tab.
   - It calls `NextTurn()` on entering Next and after every Turn ends: a grade, an Extract action, or a Source closing.
   - In Next it ignores the Card `GradeItem` returns. The Recall tab keeps using it.
   - `nextKind` and the `enter('next')` loading of all three kinds are deleted.
   - The Read / Process / Recall tabs and their badges are unchanged.
9. **"Is this Source due" moves to Go.** `ReadingView` gains `isDue`, computed with the same rule as the SQL. Both JS checks are replaced by `r.isDue`. `Store.DueReading` stays, because the queue uses it.
10. **The Mix is edited** in a "Next mix" section of the Stats view: three number inputs and Save. Saving shows the shares it implies.

## Out of scope

- Topics as Study Elements (§6.2): a fourth kind slots into the same rule later.
- Time budgets, auto-postpone and randomisation (§6.4).
- Letting overdue Cards jump the Mix, and priority scoring (§7).
- "Later" scheduling the same as "continue" (unchanged from the Sitting work).
