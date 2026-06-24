# Pure Text Card Authoring — Design

**Date:** 2026-06-25
**Status:** Approved (brainstorm) — ready for implementation plan
**Area:** `internal/content` (parser), `internal/learning` (card generation), manual test plan

---

## 1. Problem & motivation

Manual test TC-1.1 ("Card discovery via Build Graph") fails: a `# @card` node with a
`(means)` arrow produces **no** `items` row. Root cause (confirmed against the running
registry): basic-card generation only emits a card when the arrow's
`ArrowDef.Studyable == true`, but **nothing in production ever sets `Studyable`**. The
field (`internal/content/types.go`, commented *"From SSTconfig comment metadata"*) is
populated only in unit-test fixtures; `loadArrows` in `registry.go` never assigns it. So
every basic card is silently dropped in the real app, while unit tests pass because they
hand-build registries with `Studyable: true`.

Rather than retrofit a studyability convention onto SSTconfig (no such metadata exists,
and the `note` vs `means` distinction the tests encode cannot be expressed at metatype
granularity), we **decouple cards from arrows entirely**. Cards become explicit author-written
text. This deletes the `Studyable` dependency — and the bug — outright.

### Guiding principle

Cards and knowledge-graph edges become **orthogonal**, chosen per line by the author:

- `What is the capital of France? >> Paris` — a flashcard (recall), no graph edge.
- `France (means) Paris` — a real SSTorytime graph edge, no card.
- Want both? Write both lines.

Nothing forces a card and an edge to be the same line anymore. The only capability
*removed* is arrow-derived basic cards; sequence cards were already parsed-but-never-generated
and stay that way.

---

## 2. Authoring model & syntax

| Marker | Node syntax | Produces |
|---|---|---|
| `# @card` | `front text >> back text` | one card: front → back |
| `# @card reverse` | `front text >> back text` | two cards: front→back **and** back→front |
| `# @card no-reverse` | `front text >> back text` | one card (explicit single; harmless) |
| `# @cloze` | `… {{hidden}} … {{more}} …` | one card per `{{ }}` group |

Rules:

- **`>>` separator:** split `node.Text` on the **first** `>>`. Front = left side (trimmed),
  back = right side (trimmed). Later `>>` occurrences stay literally in the back text.
- **`# @study` / `# @study reverse` blocks** continue to work: they mark the following node
  lines as `>>` basic / reverse cards (the block form of the same thing).
- **`# @study sequence`** stays parsed-but-dormant (no generation) — unchanged from today,
  out of scope.
- **Cloze braces are `{{ }}` only** (hard switch). A single `{x}` is literal text, not a blank.
- An arrow line with **no** card marker (`France (means) Paris`) is purely a graph edge,
  exactly as today.

---

## 3. Parser changes (`internal/content`)

### 3.1 Skip arrow extraction on card/cloze nodes (narrowed for `(from)` anchors)

Card text must not pass through `extractArrows`, or parenthetical prose
(`What is the capital (city) of France?`) gets consumed as a bogus `(city)` arrow with the
rest of the line as its target — breaking the card.

In `parseNode`, **before** calling `extractArrows`, inspect the pending study meta
(`state.pendingStudy` / `state.studyBlock`). Skip arrow extraction — taking the **whole
trimmed line** as literal card text — when the node is study-marked **and does not carry a
`(from)` arrow**:

- `node.Text` = full trimmed line (with `>>` kept literal — see §3.3).
- `node.Arrows` = empty.

**Why the `(from)` exception:** an extract node anchors to its source via a `(from)` arrow
(`passage (from) "<refPath> char=N"`). That anchor must stay parsed/separate even when the
node also gains a `# @cloze` marker during promotion, so the anchor text never leaks into
the cloze passage. Concretely: a study node *with* a `(from)` arrow keeps arrow extraction
(anchor separated, `node.Text` = passage); a study node *without* one skips it (literal card
text). Basic-card promotion (§8) explicitly strips the `(from)` anchor so the seeded `>>`
card falls into the "no `(from)`" branch.

A study marker (without `from`) on a line means "this is a card, not a relationship."
Consequence: such a node ingests into the graph as a lone node with no edges — already true
for cloze today, and intended under the new philosophy.

### 3.2 Cloze: `{ }` → `{{ }}`

Update `extractClozeTargets` (content) and `braceGroups` (learning) to match `{{ … }}`
only. Match outermost `{{ … }}` pairs left-to-right. A single `{x}` is literal text. An
unmatched `{{` yields no blank.

### 3.3 Keep `>>` literal

The parser does **not** special-case `>>`. It stays in `node.Text`, and the split happens
only in the card generator (§4). Graph/search therefore see the raw line (`… >> …` shows in
the explorer) — accepted as a minor cosmetic cost in exchange for parser simplicity.

### 3.4 No new markers

`# @card`, `# @card reverse`, `# @cloze`, `# @study*` already parse. The malformed-marker
path (`# @card foo` → `ErrMalformedMarker`) is untouched. We change only what text a node
carries and how braces are read.

---

## 4. Card generation (`internal/learning/cards.go`)

Rewrite `GenerateCards`. This removes the arrow dependency and the `Studyable` bug.

- **Basic** (`CardTypeBasic`): split `node.Text` on the first `>>`.
  - Both sides non-empty → one `GeneratedCard{Front: left, Back: right, CardType: basic, Discriminator: "0"}`.
  - No `>>`, or either side empty after trim → **no card** (see §5).
- **Reverse** (`CardTypeReverse`): same split, emit **two** cards —
  `{Front: left, Back: right, Discriminator: "0"}` and
  `{Front: right, Back: left, Discriminator: "1"}`. Same no-`>>` rule.
- **Cloze** (`CardTypeCloze`): existing logic, but `braceGroups` / back-rendering operate on
  `{{ }}`. Back = line with braces removed; front = chosen group shown as `[…]`; one card per
  group; `Discriminator` = group index.

**Signature change:** drop the `reg *content.SSTRegistry` parameter from `GenerateCards`
(no longer needed by any card type). Update callers (`Sync`).

**Deletions:**

- `basicCards(...)` arrow-iteration logic and its registry/arrow lookups.
- `ArrowDef.Studyable` field in `types.go` (now unused in production and tests).
- `Studyable: …` lines in `cards_test.go` / `sync_test.go` fixtures.

**Item identity:** keying stays `NodeID + CardType + Discriminator`, so Build Graph remains
idempotent and reverse's two cards get stable, distinct ids.

---

## 5. Error handling & edge cases

- **`# @card` / `# @card reverse` with no `>>`** (or an empty side): emit **no card**, **no
  hard error** — `Sync` skips it, exactly as a brace-less cloze silently yields nothing
  today. The node still parses and still ingests into the graph. A user-facing "add `>>`"
  hint is **out of scope** (no hint channel exists today; adding one is a separate
  enhancement).
- **Multiple `>>`**: split on the first only; later `>>` stay literally in the back text.
- **`# @cloze` with no `{{ }}`**: no card (unchanged behavior, now keyed on double braces).
- **Malformed marker** (`# @card foo`): existing `ErrMalformedMarker` path untouched.
- **Idempotency:** re-running Build Graph yields identical `NodeID + CardType + Discriminator`
  keys → no duplicate items, no churn (TC-2b.8 stays green).
- **Retirement:** removing `>>` from a card line (so it no longer generates) retires its item
  via the existing `RetireMissing` path; review logs preserved.

---

## 6. Testing

**Unit tests (TDD):**

- `internal/learning/cards_test.go` — rewrite for the new generator: `>>` basic split,
  `# @card reverse` → two cards, no-`>>` → zero cards, `{{ }}` cloze (one card per group,
  back strips braces), single `{ }` → not a blank. Remove `Studyable` fixtures.
- `internal/learning/sync_test.go` — fixtures to `>>` / `{{ }}`; drop `Studyable`. Assert
  items created end-to-end, idempotent re-sync, retirement when `>>` removed.
- `internal/content/parser_test.go` — card/cloze nodes skip arrow extraction (parenthetical
  prose stays in `node.Text`, `node.Arrows` empty); `{{ }}` cloze-target extraction;
  arrow-only lines still parse as edges.
- **Regression test for the original bug:** load the real `SSTconfig`, author a
  `# @card … >> …`, run `GenerateCards`, assert exactly one item — the test that would have
  caught TC-1.1 from the start.

**Doc migration:**

- `docs/superpowers/specs/2026-06-22-incremental-reading-manual-test-plan.md` — TC-1.1
  (`>>` basic), TC-1.2 (`{{ }}` cloze), TC-2b.3 / TC-2b.4 (promote paths) to new syntax.
- Note the syntax in the Phase-1 plan/spec where card authoring is described.

---

## 8. Promote-extract interaction (seed `>>` on Make card)

A raw extract is a single anchored passage with no front/back, so "promote → Make card"
cannot produce a basic `>>` card directly. Behavior:

- **Promote → Cloze** (`# @cloze`): unchanged. Insert the marker; the node keeps its
  `(from)` anchor (so arrow extraction is retained per §3.1) and `node.Text` stays the clean
  passage. The author adds `{{ }}` in the Process editor; the cloze card generates on the
  next sync.
- **Promote → Make card** (`# @card`): the node is **rewritten** to seed a basic card —
  the `(from) "<anchor>"` portion is dropped and the line becomes `<passage> >> ` (back
  empty), with `# @card` inserted above. The extract's `# @id` is preserved, so the eventual
  item keys off the **same NodeID base**. The `(from)` anchor is dropped from the file
  (its provenance remains in the now-retired `extracts` row). With the back empty, **no item
  is generated yet**; the Process editor stays open with the cursor after `>> ` for the
  author to type the back. On save **plus a sync** the card materializes.
  - `SaveFile` does **not** sync learning items, so the Process view triggers
    `SyncStudyItems()` after the author confirms the filled-in card (a new "✓ Create card"
    action), then advances.

New backend surface: `content.SeedBasicCardNode(path, nodeLine, front)` — atomic rewrite
that replaces the node line with `front + " >> "` (preserving indentation) and inserts
`# @card` above it. `PromoteExtract(id, "card")` calls it with the extract's stored text;
`PromoteExtract(id, "cloze")` keeps using `InsertMarkerAboveNode`.

This supersedes the prior "promote buttons unchanged" assumption. TC-2b.3 changes: promote →
Make card now seeds an editable card rather than immediately creating a recallable item.

## 9. Out of scope (unchanged)

- FSRS scheduling and the recall/grading spine.
- Sequence-card generation (still parsed, still dormant).
- Reading / extract / import flows (other than the promote rewrite in §8).
- Any user-facing soft-hint channel for malformed (no-`>>`) cards.
