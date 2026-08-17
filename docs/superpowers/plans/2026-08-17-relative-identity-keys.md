# Location-Independent Identity Keys Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Node and card identity keys stop embedding the file path, so a copied or moved collection keeps its extracts, cards and notebooks reachable.

**Architecture:** `NodeID` returns the short ID that already identifies a node; card keys become `<short>::<cardType>::<discriminator>`. A schema-8 migration rewrites six columns across five tables in one transaction, splitting on the last `:id:` marker. Because removing the path removes a namespace, short-ID issuance becomes collection-wide.

**Tech Stack:** Go 1.25, SQLite via `modernc.org/sqlite`. No new dependencies, no frontend changes.

**Spec:** `docs/superpowers/specs/2026-08-17-relative-identity-keys-design.md`

## Global Constraints

- **Keys and their foreign references move together or not at all.** `review_logs.item_id` references `items.id`; `notebooks.extract_node_id` references `extracts.id`. One transaction.
- **Referential integrity is the acceptance property.** Zero orphans before, zero orphans after — asserted by counting, not by sampling.
- **A collapse must fail loudly.** If two rows migrate to the same key, the migration must abort rather than silently merge two nodes' study history.
- **`learning.db` is copied to `learning.db.bak-v7` before the migration's work is committed.** A transaction cannot undo a *successful* migration whose logic was wrong.
- **Split on the LAST `:id:`.** The short-ID alphabet is `[0-9a-z]`, so it contains no colon, so the final marker is always the real one whatever colons appear in the path or chapter title. A first-match split is wrong and a test proves it.
- **A value with no `:id:` marker is left untouched** — already migrated, or not an identity key. `activity_log.ref_id` is polymorphic and only some rows are keys.
- **`internal/content` must not import `internal/learning`.** The injectors take a `taken` set; they never consult the database.
- **Every test must be proven able to fail.** Each test step names the production change that breaks it; make it, watch the named test fail for that reason, revert, and record the captured output. This project has twice caught controls that could not have produced the failure they claimed.
- Do not open or modify the user's real collection at `/home/simone/projects/SStorytimeFork/SStorytime-main/examples/test`. Tests use `t.TempDir()`; the real-data check uses a copy.

---

### Task 1: The key format and its generator

Change the generator first. `NodeID` keeps its name and loses its parameters, so the compiler finds every call site — the opposite of the previous feature's silent-omission risk.

**Files:**
- Modify: `internal/content/nodeid.go`
- Modify: `internal/content/writeback.go:215`, `internal/content/parser.go:542`, `internal/content/promote.go:160`
- Modify: `internal/content/nodeid_test.go` (or create)

**Interfaces:**
- Produces, for Tasks 2-4: `func NodeID(shortID string) string`, returning the short ID unchanged, and `""` for an empty short ID as today.

- [ ] **Step 1: Write the failing tests**

```go
func TestNodeIDIsTheShortID(t *testing.T) {
	if got := NodeID("73dhx9t2"); got != "73dhx9t2" {
		t.Errorf("NodeID = %q, want the short id", got)
	}
}

// An untracked node has no id and cannot be scheduled — unchanged behaviour.
func TestNodeIDIsEmptyWithoutAShortID(t *testing.T) {
	if got := NodeID(""); got != "" {
		t.Errorf("NodeID(\"\") = %q, want \"\"", got)
	}
}

// The property the whole feature rests on: the key does not change when the
// collection moves.
func TestNodeIDDoesNotDependOnLocation(t *testing.T) {
	if NodeID("73dhx9t2") != NodeID("73dhx9t2") {
		t.Fatal("NodeID is not stable")
	}
}
```

The third test looks tautological once the signature changes — that is the point. Write it anyway, and note in your report that it is documentation of intent rather than a test that can fail; do **not** dress it up as a control.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/content/ -run NodeID`
Expected: FAIL to compile — `NodeID` takes three arguments.

- [ ] **Step 3: Change the generator and its call sites**

`NodeID(shortID string) string` returns `shortID`. Update the doc comment: the note about `@label` not being an identity anchor still applies and should stay.

Then fix the three call sites the compiler now rejects. At each, check whether the surrounding code was computing a path or chapter title *only* to build the ID — if so, that computation is now dead and should go with it. Report any you removed.

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/content/...`
Expected: PASS. Other tests in the package may fail if they assert on the old key format — those are legitimate updates, but **state each one you changed and why** rather than adjusting silently.

- [ ] **Step 5: Commit**

```bash
git add internal/content
git commit -m "feat(content): identity keys are the short id, not the path"
```

---

### Task 2: Collection-wide short-ID issuance

Removing the path removed a namespace. This restores the guarantee.

**Files:**
- Modify: `internal/content/writeback.go` (`existingIDs`, `InjectMissingIDs`, `InjectNodeID`)
- Modify: `internal/content/promote.go` (`InjectMissingExtractIDs`)
- Create: `internal/content/collectionids.go`
- Test: `internal/content/collectionids_test.go`

**Interfaces:**
- Produces:

```go
// CollectionShortIDs scans every .n4l file under dir for "# @id" markers.
func CollectionShortIDs(dir string) (map[string]bool, error)
```

and the three injectors gain a `taken map[string]bool` parameter.

- [ ] **Step 1: Write the failing tests**

```go
func TestCollectionShortIDsFindsMarkersInNestedDirectories(t *testing.T)
func TestCollectionShortIDsIgnoresNonN4LFiles(t *testing.T)
func TestCollectionShortIDsOnAnEmptyDirectory(t *testing.T)

// The point of the feature: an id already used ANYWHERE in the collection is
// never issued again.
func TestInjectionSkipsAnIDTakenElsewhereInTheCollection(t *testing.T)
```

The last one needs care to be meaningful. `GenerateShortID` is random, so you cannot predict what it will produce. Make it deterministic for the test — inject a generator, or seed `taken` with everything the alphabet could produce for a constrained case, or loop enough times that a duplicate would be overwhelmingly likely. **Choose an approach and say why it actually tests the guarantee**; a test that passes because a collision was improbable anyway tests nothing.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/content/ -run 'CollectionShortIDs|Injection'`
Expected: FAIL — `CollectionShortIDs` undefined.

- [ ] **Step 3: Implement**

`CollectionShortIDs` walks `dir` for `.n4l` files and matches `# @id ` lines without parsing — the existing `existingIDs` line logic, applied per file and unioned. Reuse the excluded-directory rules the scanner already applies if any exist; report what you found.

The injectors take `taken`, consult it alongside the current file's own IDs, and regenerate on a hit.

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/content/...`

- [ ] **Step 5: Update the callers**

`methods.go:223`, `internal/learning/sync.go:31` and `:78`. `sync.go` walks every file already — accumulate the set there rather than scanning twice, and say in your report which approach you took and what it costs.

Run: `go test ./...`

- [ ] **Step 6: Prove the tests can fail**

1. Have the injector ignore `taken`.
   Expected: `TestInjectionSkipsAnIDTakenElsewhereInTheCollection` fails.
2. Have `CollectionShortIDs` scan only the top directory, not subdirectories.
   Expected: `TestCollectionShortIDsFindsMarkersInNestedDirectories` fails.

- [ ] **Step 7: Measure the scan**

The single-file save path now pays a collection scan. Measure it against a copy of the real collection and report the number. If it is slow enough to be felt on save, **say so and stop** — that is a design problem to raise, not to engineer around inside this task.

- [ ] **Step 8: Commit**

```bash
git add internal/content methods.go internal/learning/sync.go
git commit -m "feat(content): issue short ids unique across the collection"
```

---

### Task 3: The migration

**Files:**
- Modify: `internal/learning/db.go` (`schemaVersion` 7 → 8, a `case 7:`)
- Create: `internal/learning/identity.go`
- Create: `internal/learning/identity_test.go`

**Interfaces:**
- Produces: `func shortKey(old string) string` — everything after the last `:id:`, or `old` unchanged when the marker is absent.

- [ ] **Step 1: Write the failing tests**

```go
func TestShortKeyOfANodeKey(t *testing.T) {
	got := shortKey("/home/u/notes/x.n4l:x:id:73dhx9t2")
	if got != "73dhx9t2" { t.Errorf("got %q", got) }
}

func TestShortKeyOfACardKey(t *testing.T) {
	got := shortKey("/home/u/notes/x.n4l:x:id:73dhx9t2::basic::0")
	if got != "73dhx9t2::basic::0" { t.Errorf("got %q", got) }
}

// A colon is legal in a filename; the last marker is still the real one,
// because the short-id alphabet has no colon.
func TestShortKeyWithAColonInThePath(t *testing.T) {
	got := shortKey("/home/u/a:b/x.n4l:x:id:73dhx9t2")
	if got != "73dhx9t2" { t.Errorf("got %q", got) }
}

// The adversarial case a first-match split gets wrong.
func TestShortKeyWithTheMarkerInsideTheChapterTitle(t *testing.T) {
	got := shortKey("/home/u/x.n4l:chapter:id:fake:id:73dhx9t2")
	if got != "73dhx9t2" { t.Errorf("got %q", got) }
}

func TestShortKeyLeavesAnAlreadyMigratedValue(t *testing.T) {
	if got := shortKey("73dhx9t2"); got != "73dhx9t2" { t.Errorf("got %q", got) }
}

func TestShortKeyLeavesAValueWithNoMarker(t *testing.T) {
	if got := shortKey("some-other-reference"); got != "some-other-reference" {
		t.Errorf("got %q", got)
	}
}
```

Then the migration, over a real database seeded at schema 7:

```go
// The acceptance property.
func TestMigrationLeavesNoOrphanedReferences(t *testing.T)
func TestMigrationRewritesAllSixColumns(t *testing.T)
func TestMigrationLeavesNonIdentityRefIDsAlone(t *testing.T)   // activity_log
func TestMigrationIsIdempotent(t *testing.T)
func TestMigrationBacksUpTheDatabase(t *testing.T)

// A collapse must be loud, never silent.
func TestMigrationFailsWhenTwoKeysCollapseToOne(t *testing.T)
```

`TestMigrationLeavesNoOrphanedReferences` counts, on both sides: `review_logs` rows whose `item_id` matches no `items.id`, and `notebooks` rows whose `extract_node_id` matches no `extracts.id`. Zero before, zero after.

Note the idempotency lesson from the previous feature: **a dump-equality check cannot detect a bug that converges to a stable wrong value.** Say what your idempotency test does and does not prove.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/learning/ -run 'ShortKey|Migration'`
Expected: FAIL — `shortKey` undefined.

- [ ] **Step 3: Implement**

`shortKey` uses `strings.LastIndex(old, ":id:")`. The backup is a file copy taken before the transaction's work is committed; name it `learning.db.bak-v7` beside the database. The `case 7:` rewrites the six columns:

```
items.id, items.node_id, extracts.id,
review_logs.item_id, notebooks.extract_node_id, activity_log.ref_id
```

Order matters if foreign keys are enforced — check whether `PRAGMA foreign_keys` is on in this store and say what you found; if it is, rewrite in an order that never leaves a dangling reference, or defer constraints for the transaction.

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/learning/...`

- [ ] **Step 5: Prove the tests can fail**

1. Use `strings.Index` instead of `LastIndex`.
   Expected: `TestShortKeyWithTheMarkerInsideTheChapterTitle` fails.
2. Skip `review_logs.item_id` in the migration.
   Expected: `TestMigrationLeavesNoOrphanedReferences` fails with 37 orphans.
3. Rewrite `activity_log.ref_id` unconditionally.
   Expected: `TestMigrationLeavesNonIdentityRefIDsAlone` fails.
4. Remove the backup step.
   Expected: `TestMigrationBacksUpTheDatabase` fails.

- [ ] **Step 6: Commit**

```bash
git add internal/learning
git commit -m "feat(learning): migrate identity keys off the file path"
```

---

### Task 4: Verification against real data and a moved collection

**Files:**
- Create: `docs/superpowers/specs/2026-08-17-relative-identity-keys-manual-test-plan.md`

**Do not open the user's real collection.** Copy it once to `"$CLAUDE_JOB_DIR/tmp/keys-a"` and work there. Record the real collection's file mtimes before and after and state the comparison explicitly — a previous run in this project damaged that directory, which is why this feature exists.

- [ ] **Step 1: The migration on real data**

Open `keys-a` in a build of this branch. Confirm schema version 8, that `learning.db.bak-v7` was created, that all six columns hold short-form keys, and — the acceptance property — that `review_logs` and `notebooks` have zero orphaned references:

```sql
select count(*) from review_logs where item_id not in (select id from items);
select count(*) from notebooks where extract_node_id not in (select id from extracts);
```

- [ ] **Step 2: The claim being made**

Copy the migrated `keys-a` to `keys-b`. Open `keys-b`. Confirm an extract's **Process pane opens** and its **notebook is reachable** — both are dead on a copied collection today, and this is the whole point of the feature. Screenshot both and read the screenshots back.

- [ ] **Step 3: A settled collection still works**

Open `keys-a` again and confirm Study, the notebook and the reading list behave normally. A fix that repairs a copied collection by breaking a stationary one is not a fix.

- [ ] **Step 4: New rows use the new format**

In `keys-b`, create a new extract and promote it to a card. Confirm the stored keys carry no path — otherwise the generator change did not take, and the next copy breaks again.

- [ ] **Step 5: Write up the result**

Every case with a pass/fail and a one-line observation, in the shape of `docs/superpowers/specs/2026-08-12-node-filters-manual-test-plan.md`.

- [ ] **Step 6: Commit**

```bash
git add docs/superpowers/specs/2026-08-17-relative-identity-keys-manual-test-plan.md
git commit -m "docs(test-plan): location-independent identity keys"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| `NodeID` returns the short ID | 1 |
| Card key `<short>::<type>::<disc>` | 3 (`shortKey` preserves the suffix) |
| Split on the last `:id:` | 3 (test + control 1) |
| Six columns, one transaction | 3 |
| Foreign references stay resolvable | 3 (test + control 2) |
| `activity_log.ref_id` matched on shape | 3 (test + control 3) |
| Backup before migrating | 3 (test + control 4) |
| A collapse fails loudly | 3 |
| Collection-wide short-ID issuance | 2 |
| `internal/content` does not import `internal/learning` | 2 (the `taken` parameter) |
| Real-data verification, moved-collection claim | 4 |

**Type consistency:** `NodeID(shortID string) string`, `CollectionShortIDs(dir string) (map[string]bool, error)`, `shortKey(old string) string`, and the three injectors' added `taken map[string]bool` are used with the same names and signatures in every task that touches them.

**Known risk this plan accepts:** Task 2's uniqueness test is only as good as the determinism the implementer builds into it, and the plan cannot specify that without knowing how they choose to inject the generator. The step says so and asks them to justify the approach rather than assert it — a test that passes because a random collision was unlikely anyway would be worse than no test, and this is the one place in the plan where that failure mode is available.
