# Location-independent identity keys

**Date:** 2026-08-17
**Status:** Proposed — awaiting approval
**Feature area:** `internal/content` node identity, `internal/learning` schema

## Summary

Node and card identity keys embed the absolute file path. Copying a collection
therefore leaves every stored key unmatchable, so the Process pane is dead for
legacy extracts and their notebooks are unreachable. The key becomes the short
ID that already identifies the node, and short-ID issuance becomes
collection-wide so it stays unique without the path's namespace.

## Motivation

The previous feature made stored *paths* collection-relative, which stopped
Study writing outside the open collection — the data-damaging half of the
problem. Its manual verification then found the other half: identity keys embed
the same absolute path and were untouched.

```
/home/simone/…/examples/test/reading/x.n4l:x:id:73dhx9t2
/home/simone/…/examples/test/reading/x.n4l:x:id:73dhx9t2::basic::0
```

On a copied collection nothing matches these, so extracts lose their Process
pane and notebooks become unreachable. New rows still write absolute keys, so a
migration alone would not hold — the generator has to change too.

## Grounding in the current implementation

Measured against the collection at
`/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`:

- **`NodeID(filepath, chapterTitle, shortID)`** (`internal/content/nodeid.go:21`)
  returns `<filepath>:<chapterTitle>:id:<shortID>`. Called from
  `writeback.go:215`, `parser.go:542` and `promote.go:160`.
- **The short ID already identifies the node.** `GenerateShortID`
  (`nodeid.go:29`) produces 8 cryptographically random base-36 characters, and
  the value is persisted in the note as `# @id`. It survives a move; the path
  does not.
- **Nothing parses an ID apart.** No `Split(":id:")`, no index arithmetic
  anywhere in the tree — IDs are opaque strings compared for equality and used
  as map and column keys. Their content is therefore free to change.
- **Six columns carry these keys**, across five tables:

  | column | rows | embedding a path |
  |---|---|---|
  | `items.id` | 15 | 15 |
  | `items.node_id` | 15 | 15 |
  | `extracts.id` | 20 | 20 |
  | `review_logs.item_id` | 37 | 37 |
  | `notebooks.extract_node_id` | 8 | 8 |
  | `activity_log.ref_id` | 5 | 1 |

  `review_logs.item_id` and `notebooks.extract_node_id` are foreign references —
  rewriting a key means rewriting them in the same transaction.
  `activity_log.ref_id` is polymorphic: only some rows hold an identity key.
- **Item keys are `<nodeID>::<cardType>::<discriminator>`** — e.g.
  `…:id:73dhx9t2::basic::0`.
- **Short-ID uniqueness is enforced per file only.** `existingIDs(lines)`
  (`writeback.go:122`) scans one file's lines; all three injection functions
  (`InjectMissingIDs`, `InjectMissingExtractIDs`, `InjectNodeID`) use it that
  way. Across files, uniqueness rests on randomness alone — harmless today
  because the path namespaces the key.
- **The collection is currently clean**: 20 of 20 short IDs distinct in the
  database, and no repeated `# @id` in any `.n4l` file.
- **`notebook_methods.go:375` records that splitting a path back out of an ID is
  ambiguous**, because a colon is legal in a filename.

## Decisions

- **The node key becomes the short ID; the card key becomes
  `<short>::<cardType>::<discriminator>`.** The path and chapter were redundant
  for identity and are the only parts that break on a move.
- **The migration splits on the *last* `:id:`.** The general ambiguity
  `notebook_methods.go` warns about does not bite here: the short-ID alphabet is
  `[0-9a-z]`, so it can contain no colon, so the final `:id:` is always the real
  marker regardless of what appears in the path or chapter title. This is the
  single fact that makes a mechanical migration safe.
- **All six columns move in one transaction**, as schema version 8, so keys and
  their foreign references can never diverge.
- **`activity_log.ref_id` is matched on shape, not rewritten blanket.** Only
  values containing `:id:` are identity keys; the rest are other kinds of
  reference and must be left alone.
- **`learning.db` is copied to `learning.db.bak-v7` before the migration runs.**
  A transaction makes the migration all-or-nothing, but it cannot undo a
  *successful* migration whose logic was wrong. One file copy, once, on one
  upgrade.
- **Short-ID issuance becomes collection-wide.** Removing the path removes a
  namespace: afterwards, two nodes sharing a short ID would merge their study
  state. At 36⁸ ≈ 2.8×10¹² keys the collision probability is roughly 1.8×10⁻⁵ at
  ten thousand nodes — small, but a merge of study history is not a failure
  worth accepting when the fix is a set membership test.
- **The injectors take a `taken` set; they do not consult the database.**
  `internal/content` must not depend on `internal/learning`, and the store would
  not be authoritative anyway — a note can carry an `# @id` with no database row.
  Callers build the set from a cheap scan for `# @id` markers, without parsing.
  `sync.go` already walks every file and can accumulate it; the single-file save
  path pays one scan.

## Architecture

### `internal/content`

```go
// NodeID returns the node's stable identifier: the short ID itself.
func NodeID(shortID string) string

func InjectMissingIDs(parsed *ParsedFile, taken map[string]bool) ([]IDInjection, error)
func InjectMissingExtractIDs(parsed *ParsedFile, taken map[string]bool) ([]IDInjection, error)
func InjectNodeID(path string, nodeLineNumber int, taken map[string]bool) (*IDInjection, error)

// CollectionShortIDs scans every .n4l file under dir for "# @id" markers.
func CollectionShortIDs(dir string) (map[string]bool, error)
```

`NodeID` keeping its name and losing its parameters means the compiler finds
every call site — the opposite of the previous feature's silent-omission risk.

### `internal/learning`

Schema version 8. Before the version-7 case's work is committed, the store
copies the database file to `learning.db.bak-v7`. The migration then rewrites,
in one transaction:

```
items.id, items.node_id, extracts.id,
review_logs.item_id, notebooks.extract_node_id, activity_log.ref_id
```

using `shortKey(old) string` — everything after the last `:id:` — leaving any
value without that marker untouched.

## Error handling

| Case | Behaviour |
|---|---|
| A key has no `:id:` marker | Left untouched — already migrated, or not an identity key |
| The migration fails partway | Transaction rolls back; `learning.db.bak-v7` remains |
| Two rows collapse to the same key | The transaction fails on the primary-key constraint rather than silently merging — see Testing |
| A short ID is already taken at injection | A new one is generated |
| The collection scan fails | Injection returns the error rather than issuing a possibly-duplicate ID |

## Testing

**Referential integrity is the property that matters.** Before and after the
migration, every `review_logs.item_id` must match an `items.id` and every
`notebooks.extract_node_id` an `extracts.id`. Asserted by counting orphans on
both sides of the migration — zero before, zero after — not by inspecting a
sample.

**The collapse case deserves its own test.** Seed two rows whose keys differ only
in their path but share a short ID, and assert the migration fails loudly rather
than silently merging two nodes' study history. This is the failure the
collection-wide uniqueness change exists to prevent, and it must be visible if it
ever occurs.

**Unit:** `shortKey` for a plain node key, a card key with its `::basic::0`
suffix, a path containing a colon, a chapter title containing `:id:`, a value
already migrated, and a value with no marker at all. `CollectionShortIDs` over a
nested directory, and an injection that skips a taken ID.

**End-to-end, the claim being made:** migrate a collection, copy it elsewhere,
and confirm an extract's Process pane and its notebook work on the copy. That is
precisely what fails today.

**Against real data:** run the migration on a copy of the user's database — 100
rows across six columns — and verify referential integrity and that the app
opens the migrated copy normally.

## Out of scope

- The four `file_path`/`notes_path` columns, already made relative.
- `reading_elements.ref_path`, already relative — though the readers that join it
  without the containment guard remain a known gap.
- Changing the short ID's length or alphabet.
- Deduplicating short IDs that already collide in an existing collection; the
  migration reports such a collision rather than resolving it.
