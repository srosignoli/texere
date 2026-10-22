# Plain `(then)` arrow no longer kills sequences — manual test plan

**Fix:** `2f8fe15` — `internal/search/arrows.go`
**Build:** `make dev`

## What was wrong

Writing `A (then) B` as an ordinary relationship — anywhere in any `.n4l` file
in the collection — **silently emptied `\story` and `\sequence` for the entire
collection**. No error, no warning, and the sequence nodes stayed findable by
ordinary text search, so nothing pointed at the cause.

`(then)` is a natural thing to write. If your notes contain one, sequence search
has not been working, and there was no way to tell.

**Mechanism.** `augmentUnknownArrows` harvests arrow codes that appear in data
but not in `SSTconfig` into the registry, and runs immediately before
`RegisterArrows`. A collection merely *using* `(then)` therefore had `then`
registered from that harvest, so the auto-registration of the implicit sequence
pair returned `-1` and `InsertInverseArrowDirectory` was skipped. Without the
inverse wiring the sequence walk follows nothing.

Automated coverage: `internal/search/sequence_plain_arrow_test.go` — a real
sequence plus a five-line file containing one `(then)` arrow; asserts
`\story <name>` and `\story any` still return chains, and that the sequence
nodes are ingested either way (so a failure distinguishes the route from
ingestion). Verified by negative control: removing the reserved-code skip fails
both. Also verified against nine real `sstorytime-kv` example files, where
`\story any` went from **0 chains to 8**.

## Prereqs

Back up your collection: `cp -r <collectionDir> <collectionDir>.bak`.

The interesting cases need a collection that has **both** a `:: _sequence_ ::`
block and a `(then)` arrow written outside one. If you do not have the latter,
TC-2 shows you how to add it.

## TC-1 Your existing sequences still work

Build the graph (**Ctrl+G**), then run `\story any` and `\sequence`.

**Expect:** the sequences you expect to see. This is the baseline — if it is
empty here, check TC-4 before assuming the fix failed.

## TC-2 A plain `(then)` arrow no longer breaks them — the actual bug

Note what `\story any` returns. Now add this to any note file in the collection,
outside any `:: _sequence_ ::` block:

```
- Scratch

:: testing ::

Disc 1, Part 1 (then) Disc 1, Part 2
```

Save, rebuild the graph (**Ctrl+G**), and run `\story any` again.

**Expect:** the same chains as before. Adding that line must change nothing.

*(Before this fix, that one line emptied `\story` and `\sequence` across the
whole collection.)*

## TC-3 The plain arrow is still a normal relationship

With the TC-2 file still present, search for `Disc 1, Part 1`.

**Expect:** the node is found and its `(then)` relationship to `Disc 1, Part 2`
is visible in the graph, like any other arrow. The fix suppresses nothing — it
stops a plain arrow from *shadowing* the reserved sequence arrow.

## TC-4 **The one worth doing on your real notes**

Search your collection for existing `(then)` usages:

```
grep -rn "(then)" <collectionDir> --include=*.n4l
```

If any turn up **outside** a `:: _sequence_ ::` block, then sequence search has
been silently broken for you until now. Rebuild the graph and re-run `\story
any`.

**Expect:** chains you have not been seeing. Record how many — that number is
how much this bug was costing you.

## TC-5 Sequences survive a restart

Close and reopen the app, then run `\story any` without rebuilding.

**Expect:** the same chains. The fix touches arrow registration, which runs on
every lazy open, so a repeat open must not undo it.

*(The second half of the fix exists for exactly this: resolving the existing
pointer instead of giving up when the insert reports "already present".)*

## TC-6 Nothing else about search changed

Run a few ordinary queries — plain text, `\chapter`, `\context`, `\notes`,
`\from … \to …`.

**Expect:** unchanged from before. This fix is confined to registering the
implicit sequence arrow pair.

## What I most want to hear about

- **TC-4's count.** If your notes already contained a stray `(then)`, how many
  sequences reappeared?
- Any collection where `\story any` is *still* empty after a rebuild — that
  would be a second cause this fix does not address.
