# One way to register a Source — design

Date: 2026-10-21 · Branch: `108-source-register` · Vocabulary: `CONTEXT.md` (Collection, Source) · Architecture review candidate 5

## Problem

Four bindings in `reading_methods.go` register a Source: `importTextFrom` (:111), `CreateLLMSource` (:234), `importPdfFrom` (:326) and `ImportWebSource` (:355). Each writes out the same five steps:
1. place the source file in `sources/` under a unique name;
2. derive the title;
3. create the notes file in `reading/` under a unique name;
4. build the row;
5. store it, removing the source file on failure.

They differ only in small ways:

| | text | PDF | LLM | web |
|---|---|---|---|---|
| File placed by | copying the picked file | copying the picked file | writing the markdown | writing the fetched HTML |
| File names from | the picked file's name | the picked file's name | the title, slugified | the title, slugified |
| Start position | `char=0` | `page=1` | `char=0` | `char=0` |
| Length (`ProjLen`) | runes of `source.Projection(file)` | 0 (the pane reports it later) | runes of the markdown (= the projection of a `.md` file) | `TextFromHTML` (documented identical to `Projection` of the saved file) |

**Bug:** when storing the row fails, all four remove the source file but leave the notes file in `reading/`. That leftover is an ordinary N4L file in the Collection, so the graph and the linter pick it up as a stray note.

Testing any of this today means driving the App against a real Collection (`import_seam_test.go`, `reading_methods_test.go`).

## Decisions (agreed)

1. **`source.Register(collectionDir, store, Import) → (learning.ReadingElement, error)`** in `internal/source`. The package already owns reading a Source (its projection), and it imports nothing internal, so depending on `learning` creates no circular import.
2. **`Import`** carries what differs between callers:
   - `Medium`;
   - `Title`: "" for a picked file, which takes its title from the placed file's name without its extension;
   - `FileName`: the name to start from in `sources/`;
   - `NotesName`: "" means the placed file's name without its extension, plus `.n4l`;
   - `Data` or `CopyFrom`: the content;
   - `SourceURL`.
3. **`Register` works out what used to be spelled four times:**
   - unique names in `sources/` and `reading/`, with a `-2`, `-3`, … suffix;
   - the start position: `page=1` for `pdf`, otherwise `char=0`;
   - the length: runes of `Projection` of the placed file, or 0 for `pdf`;
   - `IntervalDays: 1`.

   It returns the stored row, its `NotesPath` absolute.
4. **Naming is unchanged.** Picked files keep their own name and take their title from it. Generated content keeps its human title, and its file names are slugified. Callers pass `FileName` / `NotesName` exactly as they build them today.
5. **A failed import leaves nothing behind.** Any failure after a file is written removes every file `Register` wrote.
6. **The four bindings become thin adapters.**
   - The dialogs stay in the bindings, along with the network fetch and the check that the Collection did not change during it, and `CreateLLMSource`'s input checks.
   - `uniqueDest` and `copyFile` move into `internal/source`.
   - `slugify` stays with the bindings that build generated names.
7. **The test seam is `Register`**, against a temporary Collection with a real Store. The existing binding and import-seam tests stay unchanged and keep passing.

## Out of scope

- Images and audio for notebook cells (`ImportImage`, `ImportAudio`): media assets, not Sources.
- What a Source is stored as (unchanged), and naming policy (unchanged, D4).
