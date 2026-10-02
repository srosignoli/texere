# Roadmap

Direction, not commitment. No dates.

An item moves to `specs/NNN-*/` when it gets designed, and to
[CHANGELOG.md](CHANGELOG.md) when it ships. Small work — the kind that becomes a
numbered branch and a merge commit — does not appear here.

The long-form [specification](docs/Texere_Specification_v1.3.md) describes far
more than is built. Section numbers below point into it.

## Next

**Unified study queue (§6.4).** `Next` currently walks a fixed cascade —
read, then process, then recall — decided in a reactive statement in
`StudySurface.svelte`. Reading and extract backlogs therefore starve recall,
which is the one queue with a real deadline: FSRS cards lose retention if they
are reviewed late. Replace it with a scored queue in `internal/learning/queue.go`
holding a configurable mix, so scheduling policy stops living in the view layer.

**FSRS parameter optimisation (§6.1).** `internal/learning/fsrs.go` hardcodes
`fsrs.DefaultParam()`. Every review is already written to `review_logs` with
grade, elapsed time and prior state — the optimiser's input, collected and never
read back. Fit the weights once there is enough history, and make desired
retention configurable instead of fixed at 0.9.

## Later

**Leech detection (§7.1).** `items` has a `lapses` column and no `priority`.
Five or more lapses should raise priority and flag the card for reformulation.
The smallest useful slice of automatic priority management, and the one worth
doing first.

**Topic scheduling (§6.2).** Reading elements are scheduled; curation is not.
Signals are staleness, unprocessed ratio and child-card health, surfacing the
note and opening the editor at the relevant section.

**Full automatic priority management (§7).** Prerequisite boosting, stability
demotion, deferral decay, and a `priority_history` that records every adjustment
with a readable reason. Depends on the queue above.

## Not scheduled

Large enough to need their own specs before they mean anything.

- **Knowledge Architect tab (§9)** — a tutor over your own collection.
- **Neural review mode (§12)**.
- **Version control and sync (§14)**.
- **macOS builds.** Needs a `macos-latest` job and code signing.

## Not planned

- A mobile client. The editor and the explorer both assume a keyboard and a
  large window.
- A hosted or multi-user version. Collections are local directories, and the
  design leans on that.
- Importers for other flashcard formats. N4L is the point.

## Deliberately not on this list

"Improve the UI." It happens continuously, one branch at a time, and a roadmap
entry for it would be unmeasurable. Structural UI work appears here only when a
feature forces it — the unified queue will, because one interleaved stream needs
a different surface than four tabs.
