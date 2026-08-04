# Unterminated quote recovery — manual test plan

**Fix:** `47ff368` — `internal/content/parser.go`, `internal/content/errors.go`
**Build:** `make dev`

## What changed and why it matters

A `'...'` or `"..."` block that reached end of file without closing used to be
joined into a single line and processed as one node. Every line below the
opening quote was absorbed as literal text — node text, `# @id` and `# @card`
markers, arrows — so **one stray leading quote made the rest of the file
invisible**: no nodes, no ids, no cards, no extracts. No `ParseError` was
reported, so nothing surfaced it, and the file looked perfectly normal in the
editor.

The parser now recovers: the buffered lines are parsed individually, exactly as
they would have been read had the quote never opened, and the opening line is
reported as `unterminated quote`.

Automated coverage: `internal/content/unterminated_quote_test.go` — recovery for
both quote forms, and a regression test pinning that **closed** multi-line blocks
still join into one node. Verified by negative control: restoring the old
join-at-EOF makes both recovery tests fail.

**The risk this plan exists to check** is the regression direction, not the fix.
The fix is unit-tested; what unit tests cannot tell you is whether any file in
*your* collection relied on the old joining behaviour, or whether the new error
now floods the error list on files that were previously silent.

## Prereqs

Back up your collection before TC-4 (`cp -r <collectionDir> <collectionDir>.bak`),
since that case asks you to look at real notes.

## TC-1 A stray leading quote no longer hides the rest of the file

Create a file with this literal content:

```
- Quote Test

"an unclosed quotation

# @id keepme
This note is below the stray quote

Another note further down
```

Open it.

**Expect:** the notes below the quote are visible as separate nodes — the file
does **not** collapse into one blob of text. An `unterminated quote` error is
reported against **line 3**, the line that opened it.

## TC-2 The error names the opening line, not the end of the file

Using the TC-1 file, read the error's line number.

**Expect:** it points at the opening quote (line 3), not at the last line.

*(The whole difficulty of this bug was that nothing told you where to look. An
error pointing at EOF would repeat that failure in a quieter way.)*

## TC-3 A closed multi-line quote still works — the regression check

Create a file with this literal content:

```
- Quote Test

"a quotation that
spans two physical lines"

a following note
```

**Expect:** the quotation is **one** node whose text spans both lines, exactly as
before this fix, and **no** `unterminated quote` error. Repeat with `'` single
quotes instead of `"` — same result.

*(This is the property most at risk. If it fails, the fix has broken legitimate
multi-line quoting.)*

## TC-4 Your real collection — the point of the whole exercise

Open your existing collection and check the error list across your `.n4l` files.

**Expect:** any file that previously ended in an unterminated quote now reports
one `unterminated quote` error, and **content below that line that was previously
missing now appears** — nodes, `# @id` markers, and any cards or extracts they
carry.

**Mark clearly if you see either of these**, both of which are findings:

- A file reporting `unterminated quote` where the quoting looks deliberate to you
  — that means a real authoring form exists which this fix now flags.
- A sudden crop of new errors on many files — that would suggest unterminated
  quotes are common in practice and the error needs to be quieter or smarter.

## TC-5 **[DB]** Recovered content becomes real study material

If TC-4 surfaced any previously-hidden `# @card` or `(from)` nodes, note the
counts, then run a sync (entering Study mode triggers one):

```
sqlite3 <collectionDir>/learning.db 'SELECT count(*) FROM items WHERE active=1;'
sqlite3 <collectionDir>/learning.db 'SELECT count(*) FROM extracts WHERE active=1;'
```

**Expect:** counts may **increase**, because material that was invisible to the
parser is now readable. That is the fix working.

*(An increase here is the clearest evidence of how much this bug was costing. If
nothing was hidden in your collection, the counts stay flat and that is fine.)*

## TC-6 Editing across the recovery point still saves correctly

In the TC-1 file, edit a note *below* the stray quote and save.

**Expect:** the edit persists and the file is not reordered or mangled. The
parser change is read-only, but this confirms writeback still lines up with the
recovered line numbers.
