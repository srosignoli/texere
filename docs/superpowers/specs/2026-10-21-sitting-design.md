# Sitting — design

Date: 2026-10-21 · Branch: `103-sitting` · Vocabulary: `CONTEXT.md` (Session, Source, Extract, Sitting, Turn)

## Problem

Ending a Sitting is policy, and it lives in a Svelte component no test can reach:

- the end sequence (progress → reading time → notes → reschedule) is written three times in
  `ReadingView.svelte`: `flush()` (105-115), `close()` (117-136) and `onDestroy` (182-194). A `closed`
  flag exists only so the copies don't log the same reading time twice;
- the peek rule is enforced at five call sites in Svelte (48, 54, 109, 127, 188). The Go side gets
  only a bool (`CreateExtract(…, advanceProgress bool)`), and `stats_methods.go:12-14` explains its
  own safety by pointing at "the call site";
- **a peek reschedules the Source.** All three paths skip progress and time during a peek, but still
  call `RescheduleReading(id, 'continue')`. Glancing at an Extract's passage pushes the Source's due date
  out and grows its interval ×1.5;
- **the notes save overwrites a fresh progress marker.** `UpdateReadingProgress` writes `# @progress`
  into the notes file, then `flushNotes()` saves the editor buffer, which still holds the old marker,
  over it. The database stays right; the file goes stale. PDF and web panes write the marker on every
  page turn, so a mid-Sitting Ctrl+S does the same;
- `StudySurface.svelte:45-62` keeps a `pendingFlush` ticket and a `freshReading()` re-fetch so a
  document switch cannot read a position before the fire-and-forget end has written it;
- every failure is swallowed (`catch { /* best-effort */ }`) and the reader closes anyway, including
  a failed notes save;
- an app quit with a Source open may log no reading time and leave the Source unscheduled, because
  `onDestroy` does not reliably run.

The Study Queue (next on the roadmap) also needs a backend signal for "a Source Turn ended", and
today there is none.

## Decisions (agreed)

1. **Package `internal/sitting`.** It owns every open Sitting of the current Session. It must not
   import the Wails runtime.
2. **The backend owns the Sitting.** Its interface is:
   - `Begin(sourceID, peekAnchor string) → (sittingID, Source)`: the Source as it is now, with resume
     position, notes path and progress. `peekAnchor == ""` means a normal Sitting.
   - `Progress(sittingID, position)`: database only. No file write. A no-op for a peek.
   - `End(sittingID, position, action, notes *string) → finalNotes`: `action ∈ {continue, later, done}`.
     `notes == nil` means the buffer is unchanged or unavailable.
3. **Ownership and wiring.** `collection.Session` owns one `*sitting.Tracker`, built in `Open` and
   reached through `Handles.Sittings()`. `Session.Close` (and therefore a Collection switch) calls
   `Tracker.EndAll()`, which ends each open Sitting as `continue` with `notes == nil`, *before* the
   learning store closes. The tracker takes its dependencies when constructed (learning store,
   `SelfWrite`, the notes saver), so `sitting` does not import `collection`.
4. **Notes are saved through the same path as `SaveFile`.** The body of `App.saveFile` (atomic write,
   collection-wide `# @id` injection, read back the final content) moves to `internal/content` as one
   function. `App.SaveFile` and the tracker both call it. The `SelfWrite` bracket stays with the caller.
5. **End order lives in the module:** save notes → write the `# @progress` marker → log reading
   time → reschedule. The marker is written **once per Sitting**, at End, after the notes save, so the
   notes save can no longer overwrite it.
6. **A peek records nothing** (no progress, no marker, no reading time, no reschedule) **except Done.**
   Done is an explicit statement about the Source, so it marks the Source done even during a peek.
7. **Ending is idempotent and scoped by id.** Ending an unknown or already-ended Sitting is a no-op
   that returns no error. Because each Sitting has its own id, an old `ReadingView`'s `onDestroy` can
   never end a newer Sitting.
8. **One lock, at most one open Sitting per Session.** Begin, Progress and End are serialised.
   `Begin` while another Sitting is open ends that one first, as `continue` with `notes == nil`. A
   Begin can therefore never read a position before an earlier End has written it.
9. **Failure policy.** If the notes save fails, End returns the error and the Sitting **stays open**;
   the view shows the error and does not close. Closing again after that error asks "Close without
   saving notes?" and, on confirm, ends with `notes == nil`, so a save that keeps failing can't trap the
   reader. Once the notes are saved, marker, time and reschedule are best-effort: logged, never returned.
10. **Reading time** is wall-clock from Begin to End, measured in the backend. Zero seconds is not
    logged, as today.
11. **`CreateExtract(sittingID, text, position)`** replaces `CreateExtract(id, text, position,
    advanceProgress)`. The binding gets the Source from the Sitting and advances progress through
    `Progress`, so a peek's Extract is still made but never moves the Source. It no longer writes the
    marker (decision 5).
12. **Bindings are thin adapters:** `BeginSitting`, `SittingProgress`, `EndSitting`. These are
    **deleted**: `UpdateReadingProgress`, `LogReadingSession`, `RescheduleReading`. `ReadingView` is
    their only caller.
13. **Frontend deletions:** in `ReadingView`, the `closed` flag, `openedAt`, the five peek guards, and
    the three copies of the end sequence (`flush()`, `close()` and `onDestroy` each become one
    `EndSitting` call). In `StudySurface`, `freshReading()` and `pendingFlush`. `switchSeq` stays:
    "last click decides what is displayed" is a view concern.
14. **Tests** live in `internal/sitting`, against a real temporary Collection, through
    Begin/Progress/End only. The behaviour the deleted bindings' tests cover (in
    `reading_methods_test.go`, `stats_methods_test.go`, `collection_copy_test.go`) moves there. New
    cases:
    - a peek leaves due, interval, position and the activity log untouched;
    - Done during a peek marks the Source done;
    - the marker survives a notes save at End;
    - End twice logs reading time once;
    - Begin while a Sitting is open ends it;
    - a failed notes save leaves the Sitting open;
    - `EndAll` on Session close.

## Out of scope

- "Later" scheduling exactly like "continue" (`ScheduleReading` treats them the same). The action is
  passed through unchanged.
- `SetReadingProjLen`: the total length a pane reports belongs to the Source, not to a Sitting.
- The Study Queue. A Source Turn is begun by the view through `BeginSitting`, the same way as from the
  Read tab. The queue never begins Sittings.
- Stored-relative path handling across the learning seam (architecture candidate 3). The tracker
  resolves paths through `Store.Resolve`, as the bindings do today.
