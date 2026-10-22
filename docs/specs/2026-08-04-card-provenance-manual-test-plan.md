# Card keeps its (from) provenance — manual test plan

**Fix:** `f170638` — `internal/content/promote.go`, `study_methods.go`
**Build:** `make dev`

## What changed and why

"Make card" rebuilt the extract's node line as `<front> >> `, discarding the
`(from) "sources/… char=N"` anchor that recorded where the passage came from.
That was deliberate — provenance survives on the retired `extracts` row — but the
reasoning is backwards for this app: the `.n4l` file is the durable store and the
database is derived, so a rebuilt or lost `learning.db` destroyed the reference
permanently, with nothing in the file able to restore it.

The anchor is now written as a `(from)` **ditto line** beneath the seeded card:

```
# @id disf212z
# @card
This article describes the fastest avenue: >> Incremental Learning.
    " (from) "sources/incremental-learning-supermemo-help.html char=1173"
```

It cannot sit on the node line itself — `basicCards` splits `node.Text` on the
first `>>`, so an anchor there would land inside the card's **back**.

Automated coverage: `internal/content/promote_provenance_test.go` and the updated
`TestSeedBasicCardNode` — the seeded output byte-for-byte, the no-anchor path
unchanged, and a round-trip proving `node.Text` excludes the ditto while
`nodeHasFromArrow` still finds it. Verified by negative control: removing the
ditto write makes two tests fail.

## Prereqs

An extract created by incremental reading from a source document, so it carries a
real `(from)` anchor. Back up your collection first if you care about the file.

## TC-1 The anchor survives Make card

Process an extract that has a `(from) "sources/…"` anchor. Click **→ Make card**.

**Expect:** the file now shows `# @card`, the seeded `<passage> >> ` line, **and a
ditto line beneath it carrying the original `(from) "sources/… char=N"`**. The
anchor text is identical to what was on the extract line before.

## TC-2 The card's back does not contain the anchor

Fill in the back and click **✓ Create card**, then review it.

**Expect:** the back is exactly what you typed. It must **not** contain
`(from)`, a `sources/…` path, or a `char=` offset.

*(This is the property the ditto placement exists to protect. If the anchor ever
moves onto the node line, it lands here and every card is polluted.)*

## TC-3 The card still materialises

After **✓ Create card**, enter Study mode.

**Expect:** the card appears in the recall queue as before. The added ditto line
must not prevent the node from generating a card.

## TC-4 A hand-written note promoted to a card is unchanged

Write an ordinary note by hand (no `(from)` anchor), mark it as an extract, then
promote it with **Make card**.

**Expect:** `# @card` plus the `<text> >> ` line and **no** stray ditto line. The
output is exactly what it was before this change.

## TC-5 The reference is still readable after a rebuilt database

This is the whole point of the fix, and the one case that was previously
impossible to pass.

1. Complete TC-1 so a card carries its ditto anchor.
2. Close the app. Move `learning.db` aside (`mv learning.db learning.db.bak`).
3. Reopen the app and let it re-sync.

**Expect:** the card is rebuilt from the file **and the `(from)` line is still
there in the note**, naming the source document and character offset. Before this
fix the anchor was gone from the file and the retired extract row went with the
database, so the passage's origin was unrecoverable.

## TC-6 Navigating back to the source

With the card's node in view, check whether the `(from)` anchor is usable — does
clicking or following it reach the source document at the right offset?

**Expect:** unknown — record what you observe. This fix restores the *data*; it
does not add navigation. If nothing links back, that is a follow-up worth
scoping, not a failure of this change.

## TC-7 Watch for `unknown arrow: "from"` in the error list

Check the parse errors for a file containing these new ditto lines.

**Expect:** whatever your `.sst` registry already produced for the `(from)` lines
the extractor writes on ordinary extracts — this fix does not change it either
way.

**If you do see `unknown arrow: "from"`**, that means your arrow registry has no
`from` definition, and every extract in your collection has been generating that
error all along. `(from)` is emitted by the app itself (`content/reading.go`) and
given special handling in the parser, so it arguably ought to be registered by
default. Report it and I will scope it separately — it is not caused by this
change, but this change adds more lines that trigger it.
