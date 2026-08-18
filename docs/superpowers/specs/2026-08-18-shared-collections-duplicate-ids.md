# Duplicate identity keys across collections — a known gap

**Status:** open. Not scheduled, not a defect in anything shipped.
**Raised:** 2026-08-18, while reviewing feature 049 before merge.
**Relates to:** `2026-08-17-relative-identity-keys-design.md`

## The short version

Since feature 049, a node's identity key is its short ID alone. Inside one
collection those IDs are unique by construction. Across two collections they are
unique only by luck, and nothing checks. Copy a `.n4l` file from one collection
into another and two different nodes can end up sharing a key — at which point
their study history silently merges.

This is fine today because Texere assumes one collection. It becomes a real
problem the moment collections are shared, merged, or synced.

## Why one collection is safe

`generateUniqueID` (`internal/content/writeback.go`) does not gamble. Each
candidate is checked against the current file's IDs *and* against
`CollectionShortIDs(root)` — every `# @id` marker under the collection root — and
regenerated on a hit, up to ten times, then erroring rather than issuing a
duplicate.

The ID space is `36^8 = 2,821,109,907,456`, so the birthday bound is comfortable
anyway, but it is not what makes this safe. The check is:

| IDs in a collection | P(collision) **if nothing checked** |
|---|---|
| 1,000 | 1 in 5.6 million |
| 10,000 | 1 in 56,000 |
| 100,000 | 1 in 565 |

With the check, zero.

## Why two collections are not

Two things combine:

1. **The `# @id` marker lives in the file text**, so it travels with the file.
2. **The injectors only run for nodes that lack an ID.** `InjectMissingIDs` skips
   any node where `node.ID != ""` (`writeback.go`). An ID that arrives already
   written is never re-validated against the collection it arrives in.

Collection A and collection B each issued IDs without knowledge of the other.
Nothing prevented them both from issuing `73dhx9t2`. Copy the file, and B now
holds two nodes with one key.

What happens next is the bad part. `UpsertItem` and `UpsertExtract`
(`internal/learning/`) look the row up **by ID**, find one, and `UPDATE` it. The
incoming node does not create a row — it overwrites the resident node's text and
file path while inheriting its `due`, `reps`, `lapses`, `state`, `created_at` and
its entire `review_logs` history.

No error, no warning, no log line. Two unrelated ideas become one card carrying
the wrong schedule.

## Nothing detects it today

There is no steady-state duplicate check anywhere in `internal/content` or
`internal/learning`. The schema-8 migration has a collapse guard that refuses to
merge two keys into one, but that runs **once**, at upgrade, and only over keys
that already carry a path prefix. It cannot see duplicates that arrive later.

## What a fix would look like

The machinery already exists; it is a matter of using it for detection rather
than only for issuance.

`CollectionShortIDs` currently returns a `map[string]bool` and so cannot report a
marker seen twice — the second write is indistinguishable from the first. A
variant that counts occurrences, or records the file and line of each, would make
duplicates visible for the cost of one extra field.

From there the open questions are product ones, not technical:

- **When to check.** On collection open is cheap (one scan, measured at 6.7 ms
  over the real collection). On save it would be redundant, since the scan
  already happens there.
- **What to do about it.** Refusing to open a collection is too harsh for
  something the user may not have caused. Surfacing it — a warning naming both
  files and the shared ID — respects that the user is the one who knows which
  node should keep the key.
- **Whether to offer repair.** Reissuing an ID is not a local edit: the marker in
  the file and the rows in `learning.db` must move together, which is the same
  keys-and-references problem the schema-8 migration solved. Worth doing only if
  sharing collections becomes a real workflow.

## What is *not* affected

Renaming or moving a file **inside** one collection is safe and was the point of
049. The key no longer contains the path, so the next `Sync` finds the same ID,
matches the same row and repairs `file_path` in place — preserving the schedule
and every review log. Under the old scheme the ID changed with the path, creating
a fresh row with no history and orphaning the old one.

Copying or moving a whole collection is likewise safe: every ID inside it moves
together, and the set stays internally consistent. Only mixing files from
*separately-issued* collections creates the hazard described here.
