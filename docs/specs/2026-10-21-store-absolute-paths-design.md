# The learning Store speaks absolute paths — design

Date: 2026-10-21 · Branch: `107-store-absolute-paths` · Vocabulary: `CONTEXT.md` (Collection, Source, Extract, Card) · Architecture review candidate 3

## Problem

The learning Store keeps three path columns relative to the Collection: `items.file_path`, `extracts.file_path` and `reading_elements.notes_path`. It hands them out in that stored form, so every caller has to convert them with `Store.Resolve` and `Store.Rel`. About 20 call sites do, and each one chose its own failure policy:

| What the caller does | Sites | When a stored path won't resolve |
|---|---|---|
| Lists rows | `reading_methods.go:204, 557`, `notebook_methods.go:224` | skips the row, logs |
| File I/O on one element | `reading_methods.go:311`, `study_methods.go:98, 120`, `sitting/tracker.go:68` | returns an error |
| Display only | `learning_methods.go:33` (`resolveCardViewPaths`), `study_methods.go:57`, `queue_methods.go:30, 38` | **keeps the raw stored value**, a relative path in a field the rest of the boundary treats as absolute |
| Writes | `upsertReadingRelative` ×4 (`reading_methods.go:45-58`), `learning.Sync` (`sync.go:38`) | fails |

The comments record what this has cost. One says "this is the exact incident site — an unresolved stored path here silently rewrites the # @progress marker" (the deleted `UpdateReadingProgress`). Another says "otherwise a relative value can never equal an absolute one and this lookup silently stops matching anything" (`notebook_methods.go:220-223`).

## Decisions (agreed)

1. **Every Store read returns `NotesPath` / `FilePath` absolute.** The conversion happens in the Store's row scanners:
   - `scanReading` and `scanExtract`;
   - a new `scanCard` used by `DueQueue` and `GetCardView`.

   Every read method is covered by construction rather than by each method remembering.
2. **Every Store write takes an absolute path** and stores the relative form:
   - the upserts: `UpsertReading`, `UpsertItem`, `UpsertExtract`;
   - the retirements: `RetireMissing`, `RetireMissingExtracts`.

   A path already relative to the Collection is accepted too, which keeps test fixtures that seed `"reading/x.n4l"` working. A path outside the Collection is an error.
3. **One failure policy, in the Store.** A row whose stored path does not resolve under the Collection:
   - **Single-row reads** (`GetReading`, `GetExtract`, `GetCardView`) return an error that names the row.
   - **Many-row reads** (`ListReading`, `DueReading`, `DueExtracts`, `DueQueue`) skip it and log it.
   - A due query with a limit fills the limit from the rows that do resolve, so one bad row cannot make the Study Queue think nothing is due.
   - No raw stored value crosses the Store's interface.
4. **`Resolve` and `Rel` become private** (`resolve`, `rel`), used only inside `internal/learning`.
5. **Deleted:**
   - the ~20 binding conversions, `upsertReadingRelative` and `resolveCardViewPaths`;
   - `learning.Sync`'s own `Rel` call;
   - the Sitting Tracker's `Resolve` call.
6. **Unchanged:**
   - `RefPath` stays a Collection-relative reference that the frontend passes back to `GetSourceBytes` / `GetSourceHTML` / `GetSourceProjection`, which resolve it through `Handles.Path`;
   - the migrations (`migrateRelativePaths`, `migrateIdentityKeys`) keep reading and writing raw column values.
7. **The Store's own methods are the test seam**, against a real database. The existing binding and Sitting tests stay unchanged and keep passing, which is the evidence that callers see the same behaviour.

## Out of scope

- One module to register a Source behind the four import bindings (architecture candidate 5). This change only stops them converting the notes path.
- Changing what is stored. The columns stay relative to the Collection, so a Collection can still be moved or copied.
