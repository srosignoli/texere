# Source Register Implementation Plan

**Goal:** One `source.Register` replaces the four copies of the Source import pipeline, and a failed import no longer leaves a stray notes file.

**Spec:** `docs/specs/2026-10-21-source-register-design.md`. Decisions are cited as **D1–D7**.

## Constraints

- Branch `108-source-register`, in a worktree. Copy `frontend/dist`, symlink and locally exclude `frontend/node_modules`, and run `wails generate module` so `wailsjs/runtime` exists for the build.
- No visible change to names, titles, positions or lengths (D4). The existing binding tests are the evidence and stay unchanged.
- Test-first, one slice at a time, at `source.Register` (D7).
- `make test`, `make lint`, `make test-js` and the frontend build pass at every commit.

## Seam

`source.Register(collectionDir, st, im)` against `t.TempDir()` with a real `learning.Store`.

---

### Task 1: `source.Register`

**Files:** a new `internal/source/register.go` and `internal/source/register_test.go`. `uniqueDest` and `copyFile` move here (unexported).

**Slices** (one test, then the code for it):
1. `TestRegisterCopiesAPickedTextFile`: `CopyFrom` a temp `k.txt` with the content "some text".
   - `sources/k.txt` exists with that content, and `reading/k.n4l` exists.
   - The row has title "k", `RefPath` "sources/k.txt", an absolute `NotesPath`, `Position` "char=0", `ProjLen` 9 and medium "text".
   - The row is stored (`GetReading` finds it).
2. `TestRegisterTakesThePdfDefaults`: `Medium` "pdf" → `Position` "page=1", `ProjLen` 0.
3. `TestRegisterWritesGeneratedContent`: `Data` markdown, `Title` "My Notes", `FileName` "my-notes.md", `NotesName` "my-notes.n4l", medium "llm" → files at those names, title "My Notes", `ProjLen` = the markdown's rune count.
4. `TestRegisterMeasuresAWebArticleByItsText`: `Data` = HTML containing a `<script>`, medium "web" → `ProjLen` = runes of `TextFromHTML`, and `SourceURL` is kept.
5. `TestRegisterSuffixesCollidingNames`: register `k.txt` twice → the second is `sources/k-2.txt`, `reading/k-2.n4l`, title "k-2".
6. `TestAFailedRegisterLeavesNoFiles`: close the Store first, so storing the row fails → an error, and neither `sources/` nor `reading/` holds a new file (the leftover-notes bug, D5).
7. `TestRegisterNeedsContent`: neither `Data` nor `CopyFrom` → an error, and no files.

**Commit:** `feat(source): Register brings a Source into a Collection in one place`.

---

### Task 2: The four bindings use `Register`

**Files:** `reading_methods.go` (`importTextFrom`, `CreateLLMSource`, `importPdfFrom`, `ImportWebSource`; delete the moved `uniqueDest` and `copyFile`).

**Steps:**
1. Each binding builds an `Import` and calls `source.Register(h.Dir(), h.Learning(), im)`, then `toReadingView`.
   - **text and PDF:** `CopyFrom: picked`, `FileName: filepath.Base(picked)`, `Medium` "text" / "pdf".
   - **LLM:** `Data`, `Title`, `FileName: slug+".md"`, `NotesName: slug+".n4l"`, `Medium` "llm".
   - **Web:** the same with ".html", plus `SourceURL: art.URL`, inside the existing check that the Collection did not change.

   Error prefixes stay per binding (`ImportTextSource: …`).
2. `go test -race ./...`. Expected: PASS with **no test edits outside `internal/source`**, in particular `import_seam_test.go` and the `TestImportWeb*` / `TestCreateLLMSource*` tests in `reading_methods_test.go`.
3. `make lint`, `make test-js`, and the frontend build. `grep -n "uniqueDest\|copyFile\|CreateReadingNotes" reading_methods.go` finds nothing.

**Commit:** `refactor(reading): the four Source imports register through source.Register`.

## Manual check

In `make dev`: import a text file, a PDF and a web URL, and save an LLM chat as a Source. Each one appears in the sidebar and opens at the start.
