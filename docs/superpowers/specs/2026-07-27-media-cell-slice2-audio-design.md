# Media cell — slice 2: pronunciation and general audio

**Date:** 2026-07-27
**Status:** Approved design
**Feature area:** Processing Notebook — media cells

## Summary

Extend the Media cell to a second kind: **audio**. Search Lingua Libre for a
word's **pronunciation** (or Commons for general audio), preview candidates in
the results list, pick one, and it is fetched and stored content-addressed like
any other asset. **Promote to N4L** writes `has pronunciation (pron)` or
`has audio (aud)` depending on the provider.

Video remains out of scope; it is slice 3.

## Motivation

When learning a word you want to hear it, ideally from several speakers, and
keep the recording attached to the concept. Slice 1 delivered that loop for
pictures. Audio is the same loop with a different payload, and the cell was
built to grow it.

## Grounding in the current implementation

Everything below was verified against the live APIs and the current code before
this design was written.

- **`internal/media` already stores audio.** `ImportAudio`, `SaveAudio`,
  `audioMimeByExt`, `audioExtByMime`, and the `/media/<name>` file server's
  audio content-types all exist for the `AudioCell`. Only `FetchRemote` is
  image-only.
- **`FetchRemote` cannot accept audio as written.** It sniffs with
  `http.DetectContentType` and validates against the image-only `extByMime`.
  Two independent reasons that fails for audio: containers sniff unreliably
  (`import.go` says so, which is why `ImportAudio` keys off the extension), and
  Commons serves Lingua Libre `.wav` as **`Content-Type: audio/x-wav`** while
  `audioMimeByExt` maps `.wav` to `audio/wav` — so header-based validation
  rejects a perfectly good file.
- **The kind→component map and `ProviderInfo.Kinds` already exist** from slice
  1, specifically so a second kind could be added without restructuring.
  `MediaCell` picks its renderer from `views[cell.kind]`, and
  `MediaSearchPanel` already filters providers by kind.
- **The cell shape already carries `kind`.** No migration of saved notebooks.
- **`AudioCell` already discriminates `MediaError` codes** to distinguish a
  load failure from a codec failure, with a comment noting WebKitGTK/WSLg often
  lacks audio codecs. That handling is reused rather than reinvented.
- **Commons search returns the wrong thing for a naive audio query.**
  `manzana filetype:audio` returns a tango, a street-vendor recording, and a
  full sentence. Pronunciations live in Lingua Libre and need a shaped query.
- **Openverse's audio endpoint works anonymously but indexes music** (Jamendo).
  For *manzana* it returns "No hablemos de manzanas". It is excluded from audio.

## Decisions (from brainstorming)

- **Audio now, video later.** Audio is the image loop with a different payload;
  video breaks three of slice 1's assumptions at once — it is referenced rather
  than stored, its search needs an API key, and it renders as an embed. It gets
  its own design conversation rather than being appended here.
- **Pronunciation belongs in the Media cell, not `AudioCell`.** Same split as
  Scan vs Media for images: `AudioCell` records your own voice and promotes the
  *text* via STT; the Media cell's payload is the *recording itself*, promoted
  as a reference. Settled in slice 1 and not reopened.
- **Two audio providers, not one.** A purpose-built pronunciation provider, and
  generic Commons audio for everything else. A naive query cannot serve both:
  the shaping that finds pronunciations is exactly what excludes birdsong.
- **A language is required, and a curated map supplies it.** Lingua Libre
  filenames key on a Wikidata code (`LL-Q1321` = Spanish). A `name → Qcode` map
  of ~30 languages, ordered by recording volume, with a raw `Q…` code accepted
  in the same control so an unlisted language is never a dead end. The choice
  persists on the cell; new cells default to the last used, because a learner
  stays in one language for weeks.
- **Language scoping is necessary but not sufficient — an exact-word filter is
  also required.** Verified: `LL-Q150 chat` returns *femme*, *bigenre*, *quoi*
  because the speaker is named "Nattes à chat"; `LL-Q1860 pie` ranks *pie-eyed*,
  *pie house*, *pie safe* above the bare word. Relevance ranking alone is unfit.
- **Preview in the results list; download only on pick.** For images the
  thumbnail *is* the preview; audio has no equivalent, and choosing among six
  speakers means hearing them. Downloading every candidate would write a dozen
  permanent files per search.
- **Kind-specific arrows, defaulted by provider.** `has pronunciation (pron)`
  and `has audio (aud)`. Note `SSTconfig` already declares an unrelated
  `is pronounced as (pronas)` whose reverse is `pron-of`; that almost certainly
  means a phonetic rendering in text rather than a stored recording, so the two
  coexist and the new arrow takes the free reverse code `pron-rec`. Reusing
  `pron-of` would silently overwrite the existing relation, since the arrow
  registry is keyed by code. Provenance sets a **default in editable text**, not
  a hidden affordance — the distinction that made provenance-driven behaviour
  wrong in slice 1 does not apply, because the output is a draft you are already
  reviewing.
- **Audio is keyed by URL extension, never by sniffing or Content-Type.**
  Forced by the `audio/x-wav` finding above.

## Architecture

### Search layer — `internal/mediasearch`

Two new providers implementing the existing `Provider` interface.

**`Pronunciation`** — Lingua Libre via the Commons API already in use.

- Query `LL-{Qcode} {word}` over `gsrnamespace=6`, `iiprop=url|mime|size`
  (`size` also yields `duration`).
- **Exact-word filter:** keep only titles ending `-{word}.{ext}`,
  case-insensitively. This drops the speaker-name collisions and the compound
  words documented above.
- **Fallback:** when the filter leaves nothing, return the unfiltered results
  rather than an empty grid, so a word with no exact recording still shows
  near-misses.
- **Filename parsing** of `LL-{Q} ({iso})-{speaker}-{word}.{ext}` to populate
  language, speaker, and word. The format is ambiguous under naive splitting —
  both speaker and word may contain hyphens — so parsing anchors on the
  `LL-{Q} ({iso})-` prefix and the `.{ext}` suffix.
- `Kinds() == ["audio"]`.

**`CommonsAudio`** — query `{word} filetype:audio`, no filtering,
`Kinds() == ["audio"]`.

**Provider IDs are `pronunciation` and `commons-audio`.** These are persisted on
the cell and are what `buildMediaFact` keys on to choose the arrow, so they are
part of the contract, not display labels. (Slice 1's existing IDs are `commons`
and `openverse`; file import and pasted URLs set `file` and `url`.)

`Openverse.Kinds()` stays `["image"]`, so it simply does not appear in the audio
provider dropdown. No new filtering mechanism is needed.

**`Result` gains four fields** — `Lang`, `Speaker`, `DurationMs`, `Mime`, with
JSON tags `lang`, `speaker`, `durationMs`, `mime`. They are empty for images, so
nothing existing changes. They drive the audio row, which must convey what a
thumbnail conveys for an image:

```
▶  manzana        spa · Rdrg109 · 1.2s · wav
```

Format is shown because `CommonsAudio` returns `.ogg` that WebKitGTK may not
decode, and that should be visible before clicking rather than after.

**Language map** — a `name → Qcode` table in the search package, ~30 entries
ordered by Lingua Libre recording volume. A value already matching `Q\d+` is
passed through unchanged.

### Storage — `internal/media`

`FetchRemote` becomes kind-aware:

```go
func FetchRemote(collectionDir, rawURL, kind string) (Asset, error)
```

Shared guards — User-Agent, `io.LimitReader`, non-2xx rejection — stay in one
place; only validation branches:

- `kind == "image"` — sniff with `http.DetectContentType` against `extByMime`.
  Unchanged behaviour.
- `kind == "audio"` — key off the **URL path's extension** against the existing
  `audioMimeByExt`, matching `ImportAudio`.

The size cap becomes per-kind: **8 MB** image (unchanged), **25 MB** audio — a
Lingua Libre clip is ~100 KB, but a general Commons recording can run minutes.

### Bindings

`FetchMediaURL(rawURL, kind string)` gains the kind and forwards it.
`SearchMedia` is unchanged; it already takes `kind`.

### Frontend

- **`MediaSearchPanel`** gains a **kind toggle** (Image | Audio) that sets
  `cell.kind`, re-filters the provider dropdown through `Kinds`, and clears
  stale results; and, when the kind is audio, a **language control**.
- **Results render as rows for audio**, not tiles. A single shared `<audio>`
  element drives the ▶ preview, so only one clip plays at a time. Preview
  streams directly from `upload.wikimedia.org`, which serves media without
  hotlink protection — image thumbnails already load from that host.
- **`media/MediaAudioView.svelte`** — `<audio controls src="/media/{asset}">`,
  registered as `views.audio`. `MediaCell` itself barely changes; this is the
  seam the kind→component map was built for.
- **The crop toolbar is gated on `cell.kind === 'image'`.** It is currently
  unconditional in the filled state, and Crop on a sound file is nonsense.
- `+ Media` still creates an image cell; the toggle switches kind.

### N4L

Added to `SSTconfig/arrows-EP-3.sst`, matching its column alignment:

```
 + has pronunciation (pron)     - has pronunciation recording of (pron-rec)
 + has audio         (aud)      - is audio for                   (aud-for)
```

`buildMediaFact` derives the code from the cell instead of hardcoding `pic`:

| kind | provider | arrow |
|---|---|---|
| `image` | any | `pic` |
| `audio` | `pronunciation` | `pron` |
| `audio` | anything else (Commons audio, file import, pasted URL) | `aud` |

Promoting a pronunciation emits:

```n4l
manzana
"  (pron) media/<hash>.wav
```

The Promote button states which arrow it will write. Because the output is an
editable draft cell, a wrong default costs one keystroke.

## Error handling

- **Codec failures are named, not silent.** Preview and playback reuse
  `AudioCell`'s `MediaError`-code discrimination, separating "failed to load"
  from "this webview cannot decode this format". A dead ▶ button with no
  explanation is the worst outcome, and `.ogg` from `CommonsAudio` makes it
  likely on Linux.
- **Empty results and provider errors stay distinct**, as in slice 1.
- **The exact-word filter never produces an empty grid on its own** — the
  fallback guarantees that an empty result set means the *provider* returned
  nothing.
- Stale searches are still discarded by generation counter; `no collection open`
  still guards the fetch; a missing asset still renders as such.

## Testing

The tests use the **real failure cases found during design** as fixtures, not
invented ones.

**Go — `internal/mediasearch`:**

- Exact-word filter: a `chat` fixture carrying the "Nattes à chat" speaker
  collision, asserting those results are dropped; a `pie` fixture with
  `pie-eyed` / `pie house` / `pie chart` / `pie`, asserting only the bare word
  survives; an all-filtered-out fixture asserting the unfiltered fallback fires.
- Filename parsing: including a speaker containing a hyphen and a word
  containing a hyphen.
- Query shaping: `LL-Q1321 manzana`; `{word} filetype:audio`.
- Language map: name → Qcode, and a raw `Q1321` passed through.

**Go — `internal/media`:**

- `FetchRemote` with `kind == "audio"`: accepts by extension; a fixture serving
  **`Content-Type: audio/x-wav`** proving the header cannot reject a valid file;
  unknown-extension rejection; the 25 MB cap; and image behaviour unchanged.

**JS:** `buildMediaFact` across all three arrow cases, with the existing image
assertions unchanged.

**Live-API check using production parameters** — the same language code and
limit the app actually sends. Slice 1's Openverse failure was caused by a
verification curl using `page_size=2` while the app sent 24.

**A manual test plan**, per the convention every slice follows.

## OPEN: local audio does not play in the Wails window on Linux

**Status: unresolved, diagnosed but not proven. Found in manual testing 2026-07-27.**

Downloaded audio does not play from the cell on Linux/WSL. Every attempt — `.wav`
and `.ogg`, from a picked search result or a pasted URL — fails with
`MediaError 4` (`MEDIA_ERR_SRC_NOT_SUPPORTED`), surfaced as *"format/codec not
supported in this webview"*. **The message is misleading: this is not a codec
problem.**

What is established:

- The stored files are valid and unremarkable — verified with `file(1)` as
  RIFF/WAVE 16-bit PCM mono (48 kHz and 44.1 kHz) and Ogg Vorbis. 16-bit PCM WAV
  is about as universally supported as audio gets.
- **Preview works.** The results list plays the *same bytes* from
  `https://upload.wikimedia.org/...` without trouble, which rules out both the
  obvious environment-specific explanations: the audio sink works, and the
  GStreamer decoders are present.
- Images from the same `/media/<name>` path have worked since slice 1.

Probable cause: Wails v2.12 hands custom-`Handler` responses to WebKitGTK
through a `g_unix_input_stream` — a **pipe**
(`pkg/assetserver/webview/responsewriter_linux.go:75-85`). A pipe is not
seekable. Image decoders read sequentially and are unaffected; media elements
must seek (demuxer headers, playback seeking), and a non-seekable source makes
WebKit's media pipeline fail with exactly this error. Preview escapes it by
loading a real `https://` URL through WebKit's own HTTP stack. The Linux
webview's start URL is `wails://wails/` (`internal/frontend/desktop/linux/frontend.go:116`)
in both `wails dev` and a built binary.

This is **not specific to this slice**. It affects any local audio in the app on
Linux, including the pre-existing `AudioCell`, whose comment about WebKitGTK
"lacking the codecs" appears to be an earlier misdiagnosis of the same thing.

**The experiment that would settle it** — it changes exactly one variable, since
the same webview, machine and files are used, and only the scheme changes:

```
wails dev -frontenddevserverurl http://localhost:34115
```

(`frontend.go:175` shows the start URL is overridable this way; check
`wails dev -help` for the exact flag in your version.)

- Audio plays → the transport is confirmed as the cause. Native Linux will fail
  too, and a fix is needed.
- Audio still fails → the diagnosis above is wrong and the investigation should
  restart rather than a workaround being built on it.

Browsing the dev server in Chrome or Firefox is **not** a valid substitute —
those bring their own media stacks and would not isolate the variable.

If a fix is wanted, the candidates are: read the bytes through a Wails binding
and play from a `URL.createObjectURL` Blob (seekable in memory; a ~100 KB
pronunciation clip is trivial, a multi-MB recording less so), or serve `media/`
from a real localhost HTTP server. Both would also fix `AudioCell`.

Untested on native Linux and on Windows. Windows uses WebView2 with a different
response mechanism and is expected to work, but a green result there would not
identify which variable mattered.

## Out of scope for this slice

- **Video and `embedURL`** — slice 3. The field stays unpopulated.
- **Keyed providers**, including Forvo for pronunciation and YouTube.
- **Rendering audio on the study card or in the graph.** `RecallCard` renders
  `front`/`back` as plain text, so a promoted `pron` fact displays as a path
  there, exactly as `pic` does today. Making the study surface play media is its
  own slice.
- **Audio trimming** — there is no crop equivalent for sound in this slice.
- **Multi-asset boards** collecting a picture and a pronunciation under one
  concept.
- **Openverse audio**, for the reason given above.
