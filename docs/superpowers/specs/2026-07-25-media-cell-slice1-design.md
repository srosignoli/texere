# Media cell — slice 1: web image search → notebook cell → N4L fact

**Date:** 2026-07-25
**Status:** Approved design
**Feature area:** Processing Notebook — media cells

## Summary

Add a **Media cell** to the processing notebook: search the web for a picture,
pick one, and it is fetched, content-addressed, and stored in the collection's
`media/` directory like any other asset. A **Promote to N4L** action writes a
fact referencing the asset (`<subject>` — `has picture` → `media/<hash>.jpg`)
into an N4L draft cell below, using the notebook's existing insert path.

This slice covers **images only**. The cell is designed to grow pronunciation
audio and video in later slices; the acquisition pipeline is type-agnostic by
construction.

## Motivation

When studying — a language especially — you routinely want a picture of the
word, its pronunciation, or a short clip. Today that means leaving the app,
searching in a browser, saving a file, and importing it. The notebook already
owns every other step of the processing loop, so the lookup belongs there too.

## Grounding in the current implementation

- `internal/media` already provides content-addressed storage (`store()`,
  sha256, dedup under `<collectionDir>/media/`) and a traversal-safe
  `/media/<name>` file server for the webview. Remote fetch is a third
  acquisition path beside `Import` / `ImportAudio` / `SaveAudio`.
- `internal/source/web.go` already fetches remote images safely: `getWithUA`
  sends a descriptive User-Agent (Go's default is 403'd by Wikipedia and many
  others), `io.LimitReader` caps the body, and the MIME is validated. The new
  fetch mirrors this rather than inventing a pattern.
- `frontend/src/lib/notebook.js` exposes a `cellRegistry` keyed by cell `type`;
  adding a type is mechanical.
- `NotebookPane.svelte:96` already wires `onInsert(text)` to
  `insertAfter(i, newDraftCell(text))`. Promotion needs no new write path.
- `internal/content/parser.go:857` (`stripLineComment`) deliberately protects
  `://`, so URL-shaped tokens survive in node text. Not needed for slice 1's
  relative paths, but it means remote references stay open as an option.
- `internal/content/unknown_arrows.go` synthesizes arrows that appear in notes
  but are undeclared in `SSTconfig`, so `has picture` functions even in a
  collection whose config has not been updated.
- `llm_config.go` / `stt_methods.go` establish the profile pattern (id, name,
  kind, base URL, model, `apiKey` or `apiKeyEnv`) that keyed search providers
  will reuse in a later slice.

## Decisions (from brainstorming)

- **A separate `media` cell, not an extension of `ImageCell`.** The existing
  image cell exists to OCR scanned paper notes: its payload is the *text*, and
  its `TranscribeBar` (`ImageCell.svelte:90`) promotes an LLM transcription into
  an N4L draft. A study illustration's payload is the *image itself*, and it
  promotes a reference. Acquisition, editing affordances, promotion semantics,
  and intent all differ. Rather than explain with labels or warnings when
  Transcribe should not be pressed, the affordance is simply absent.
- **Naming disambiguates by purpose, not by format.** "Media" beside "Image" is
  unclear, because an image *is* media. The existing button is relabelled
  **`+ Scan`** (paper notes, OCR) and the new one is **`+ Media`** (pictures,
  later pronunciation and video). Persisted cell `type` strings remain
  `"image"` and `"media"` — the label changes, the serialized value must not, or
  existing notebooks fail to resolve against `cellRegistry`.
- **One cell type for all media kinds, one asset per cell.** The acquisition
  pipeline (search → pick → fetch → cap → store) is identical across kinds; only
  rendering differs, and those bodies are small enough to split into
  sub-components chosen by a kind→component map. Three sibling cells would
  triplicate the pipeline to vary a MIME prefix.
- **Search lives inline in the cell, not in a modal.** Every other cell in the
  notebook does its work inline — `GraphQueryCell` in particular already pairs a
  query bar with results. Keeping the query *in* the cell also means it persists,
  so an image can be re-searched and swapped later, which matters when the first
  picture you choose for a word turns out not to be the one that sticks.
- **Paste-URL is a peer acquisition path, not a fallback bolt-on.** It shares the
  entire fetch path with search results. It is also what keeps the feature usable
  when no provider covers the word — which, for language study, will be common.
- **Provenance is stored in the cell, not written into the N4L.** `sourcePage`
  and `license` stay available where they are needed without putting machine
  bookkeeping into prose that is read by hand.
- **Kind-specific arrows.** `has picture (pic)` now; `has audio`, `has video`
  later. This preserves the ability to query for gaps — words that still lack a
  pronunciation — which a single generic `has media` would erase.
- **Results are never persisted.** Cells are serialized wholesale into
  `cellsJSON` by `SaveNotebook`; caching result grids would bloat every notebook
  for no benefit.

## Architecture

### Backend

**New package `internal/mediasearch`** — provider-agnostic search. No storage
concerns, no knowledge of collections.

```go
// Result is one search hit, normalized across providers.
type Result struct {
    Kind       string // "image" (later "audio", "video")
    ThumbURL   string
    FullURL    string
    Title      string
    SourcePage string
    License    string
}

type Provider interface {
    ID() string   // "commons", "openverse"
    Name() string // display label
    Search(ctx context.Context, query, kind string, limit int) ([]Result, error)
}
```

Slice 1 ships two keyless providers:

- **Wikimedia Commons** — MediaWiki API, `generator=search` over the File
  namespace with `prop=imageinfo`. Strong for concrete nouns; a descriptive
  User-Agent is required by policy.
- **Openverse** — anonymous access, rate-limited; broader everyday coverage.

Keyed providers (Pexels, YouTube, Google CSE) are a later slice and reuse the
`LLMProfile` config pattern. The `Provider` interface is the seam.

**Extended `internal/media`** — one new function:

```go
// FetchRemote downloads url, validates it, and stores it like any other asset.
func FetchRemote(collectionDir, url string) (Asset, error)
```

It mirrors `source.fetchImage`'s guards — User-Agent header, `io.LimitReader`
size cap, non-2xx rejection, MIME checked against the existing `extByMime`
allow-list — then calls the existing `store()`, inheriting content-addressing
and dedup. It declares its own `userAgent` const rather than importing
`internal/source`, keeping the two packages independent.

**New bindings** in `media_methods.go`:

- `ListMediaProviders() []ProviderInfo` — where
  `ProviderInfo{ID, Name string; Kinds []string}` lets the panel populate its
  provider select and, later, hide providers that cannot serve a given kind.
- `SearchMedia(providerID, query, kind string, limit int) ([]Result, error)`
- `FetchMediaURL(url string) (map[string]any, error)` → `{asset, mime}`

`FetchMediaURL` serves both "clicked a search result" and "pasted a URL"; they
are the same operation.

### Frontend

- **`components/notebook/MediaCell.svelte`** — two states. Assetless, it renders
  the acquisition panel. With an asset, it renders the media plus a toolbar of
  *Replace*, *Crop*, *Promote to N4L*. It never mounts `TranscribeBar`.
- **`components/notebook/MediaSearchPanel.svelte`** — provider select, query box,
  results grid, *Import file*, *Paste URL*. Split out so `MediaCell` stays small.
- **`components/notebook/media/MediaImageView.svelte`** — the `<img>`, styled
  through the existing shared `image-view.js`. Chosen by a kind→component map so
  a later `MediaAudioView` slots in beside it without touching `MediaCell`.
- **`lib/media-fact.js`** — `buildMediaFact({subject, asset})`, a pure function
  returning the N4L text. Extracted so it is testable without mounting Svelte.
- **`lib/notebook.js`** — `newMediaCell()` and `media: MediaCell` in
  `cellRegistry`.
- **`NotebookPane.svelte`** — add `+ Media`; relabel `+ Image cell` → `+ Scan`.

`ImageCell.svelte`, `bake.js`, and `TranscribeBar.svelte` are untouched.

### Cell shape

```js
{
  id, type: 'media',
  kind: 'image',        // discriminator; later 'audio' | 'video'
  asset: '', mime: '',  // local, content-addressed
  embedURL: '',         // reserved for video (reference-not-stored); unused in slice 1
  crop: null,
  query: '', subject: '',
  provider: '', sourcePage: '', license: '',
  naturalW: 0, naturalH: 0,
}
```

`kind` plus the `asset` / `embedURL` split is specified now, though slice 1 only
ever populates the local branch, so video does not later force a migration.

### Config

Add `has picture (pic)` to `SSTconfig/arrows-EP-3.sst` — a property relation, so
the EP ("expresses") spacetime type. Declaring it buys clean typing;
`unknown_arrows.go` means undeclared use still functions.

## Data flow

1. `+ Media` appends an assetless cell, which renders `MediaSearchPanel`.
2. Query and provider → `SearchMedia(provider, query, "image", 24)` → a grid of
   `ThumbURL`s.
3. Clicking a result calls `FetchMediaURL(FullURL)`. The cell records `asset`,
   `mime`, `provider`, `sourcePage`, `license`; keeps `query`; and defaults
   `subject` to the query — the thing you searched for is the thing the fact is
   about. `onChange` persists; the cell flips to its filled state.
4. *Import file* reuses the existing `ImportImage()` binding unchanged.
   *Paste URL* is the same `FetchMediaURL` call, the URL being its own
   provenance.
5. The filled state offers crop and *Replace*; *Replace* returns to the panel
   with the query intact.
6. *Promote to N4L* renders `buildMediaFact` and calls the existing `onInsert`:

```n4l
manzana
    has picture   media/a3f9c1….jpg
```

`subject` is editable before promoting. Pushing that draft to a file uses the
push flow that already exists — this slice adds no write path.

## Error handling

- **No collection open** — both new bindings error, matching `ImportImage`.
- **Empty results and provider failure are distinct states.** "No results for
  *manzana*" and "Commons unreachable" have different remedies and are rendered
  differently; the query is preserved either way so retry costs one click.
- **Fetch failures name the cause** — non-2xx, over the size cap (stating the
  cap), or wrong MIME.
- **Hotlink-protected results** that 403 on direct fetch get real copy pointing
  at *Paste URL*, because this will happen often enough to deserve better than a
  generic failure.
- **Stale searches are cancelled** when the query changes and late results are
  discarded — the discipline `TranscribeBar` uses via `turn.cancel()` and
  `IsoGraphView` uses for stale frames.
- **Missing asset on render** reuses `ImageCell`'s `missing` pattern.
- No secrets exist in this slice (both providers are keyless). When keyed
  providers arrive, request URLs must never be logged.

## Testing

**Go** — mirroring `internal/source/web_test.go` and `internal/llm/*_test.go`:

- `internal/mediasearch`: `httptest` servers returning canned Commons and
  Openverse payloads; assert normalization into `[]Result`, including missing
  optional fields and empty result sets.
- `media.FetchRemote`: happy path stores and dedups; oversize rejected; non-image
  MIME rejected; non-2xx rejected. (`media_test.go` already imports `httptest`.)

**JS** — alongside the existing `lib/node-source.test.js`:

- `buildMediaFact` unit tests: normal subject, subject containing spaces, empty
  subject.

**Manual test plan** — a companion doc, per the convention every slice follows.

## Out of scope for this slice

- Pronunciation audio and video cells (later slices).
- Keyed providers: Pexels, YouTube, Google CSE.
- Rendering `has picture` on the study card or in the graph. `RecallCard.svelte`
  renders `card.front` / `card.back` as plain text with no `{@html}`, so a
  promoted reference currently displays as a literal path. Making the study
  surface render media is its own slice.
- Multi-asset "boards" collecting several assets under one concept.
- Thumbnail caching or offline search.
- **Rotate.** Scans need it, illustrations do not, and its absence keeps the two
  cells' toolbars visibly different.
