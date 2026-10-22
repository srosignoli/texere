# Combined test pass — three stacked branches

**Branch:** `034-card-keeps-provenance` (tip of the stack; contains all three)
**Build:** `make dev`

Three independent pieces of work were rebased into one linear stack so they can be
tested in a single sitting. They touch disjoint files and were verified together
on the combined tip: Go suite green, `make test-js` 140/140, `vite build` clean,
`gofmt` clean.

| Commits | What | Its test plan |
|---|---|---|
| `2a8b895`..`7c68abb` | Process-mode notebook follows the caret | `2026-08-03-process-focus-notebook-manual-test-plan.md` |
| `2be5452`..`e5bd81f` | Unterminated quote no longer swallows the file | `2026-08-04-unterminated-quote-manual-test-plan.md` |
| `8345c13`..`8097ab3` | A promoted card keeps its `(from)` reference | `2026-08-04-card-provenance-manual-test-plan.md` |

## Before you start

```
cp -r <collectionDir> <collectionDir>.bak
sqlite3 <collectionDir>/learning.db 'SELECT count(*) FROM notebooks;'   # note it
sqlite3 <collectionDir>/learning.db 'SELECT count(*) FROM items WHERE active=1;'
sqlite3 <collectionDir>/learning.db 'SELECT count(*) FROM extracts WHERE active=1;'
```

Two of these plans ask you to compare counts before and after, and one asks you to
move `learning.db` aside. Take the backup.

## Suggested order

Run it in this sequence rather than plan-by-plan. It moves from disposable
fixtures to your real data, and puts the one destructive case last.

**1 — Parser, on throwaway files** (quote plan TC-1, TC-2, TC-3, TC-6)
Synthetic files only; nothing of yours is at risk. TC-3 is the regression check
that closed multi-line quotes still join into one node — the property most at
risk from that fix.

**2 — Card provenance, on one real extract** (provenance plan TC-1..TC-4)
Make a card from an extract that has a `(from)` anchor. **Complete the flow with
"✓ Create card"** — that step is what runs the sync, and skipping it is what sent
us down the wrong trail last time. TC-2 (the anchor must not be in the card's
back) is the one that matters most.

**3 — Process notebook** (focus plan TC-1..TC-7, TC-10..TC-16)
The bulk of the work. TC-13 (the header is genuinely visible, not covered by the
notebook pane) and TC-15 (the Vim toggle does not strand the pane) are the two
with no automated coverage at all.

**4 — Database checks** (focus plan TC-8, quote plan TC-5)
Re-run the counts from above. `notebooks` must **not** have grown from browsing —
that is the single most important assertion across all three plans, and it is
invisible in the UI. `items`/`extracts` **may** grow, if the quote fix recovered
content that was previously hidden.

**5 — Destructive, last** (provenance plan TC-5)
Move `learning.db` aside, reopen, and confirm the `(from)` reference is still in
the file. This is the case that was impossible to pass before, and the whole
justification for that change.

## Known limitations, already documented — not bugs to report

- A caret move cancels in-flight cell work (LLM turn, shell run, recording) — focus plan TC-19.
- The span map is briefly stale while typing, bounded by the next save — focus plan TC-9.
- Nothing links a card's `(from)` anchor back to the source document — provenance plan TC-6 asks you to record what you see; no navigation was added.

## What I most want to hear about

- **Anything in the focus plan's rendered UI.** None of it has ever been watched running — there is no Svelte component harness in this repo, so the header, the empty state and the pane switching were reasoned about from source. That plan is the first real observation, not a regression check.
- **`unknown arrow: "from"` in the error list** (provenance plan TC-7). If it appears, your registry has no `from` definition and every extract has been generating that error all along. Not caused by these changes; worth scoping separately.
- **New `unterminated quote` errors on files where the quoting looks deliberate to you** — that would mean a real authoring form exists which the fix now flags.
