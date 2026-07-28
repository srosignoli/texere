# Media cell — slice 3: video, referenced and embedded

**Date:** 2026-07-28
**Status:** Approved design
**Feature area:** Processing Notebook — media cells

## Summary

Add the third and final media kind: **video**. Search three keyless,
learning-oriented providers — Wikimedia Commons, PeerTube (via SepiaSearch) and
the Internet Archive — pick a video, scrub to the moment that matters, press
**Set start here**, and promote it to an N4L fact that links straight to that
timestamp. YouTube is supported by pasted URL only; YouTube *search* is slice 4.

Unlike pictures and pronunciations, video is **referenced, never stored**.

## Motivation

A picture fixes a word and a recording fixes its sound; a video explains a
process. The material a learner wants — a NASA visualisation, a lecture, an
explainer — is long, so a reference to a whole video is nearly useless on
revision. The unit that matters is *this video at 3:43*.

## Grounding in the current implementation

Every provider claim below was verified against the live API before this design
was written; every arrow code was checked against the loaded registry.

- **The cell already anticipates this slice.** `kind` discriminates the
  renderer through a kind→component map, and `embedURL` has been reserved and
  unpopulated since slice 1 precisely for a referenced video.
- **`Result` already carries what video needs** — `ThumbURL`, `DurationMs`,
  `SourcePage`, `Title`, `FullURL` — from the image and audio slices.
- **`mediaErrorMessage`** (slice 2) already maps `MediaError` codes to honest
  wording and is reused rather than copied.
- **`vid` and `vid-for` are free**; `note`/`isnotefor` exist and are registered.
  `url`/`isurl` are avoided — `arrows-EP-3.sst:185` reserves them in a comment.
- **URL targets survive the N4L parser intact.** Verified by parsing four
  shapes — a plain file URL, `?start=223`, `?v=…&t=223s`, and `#t=223` — all
  with zero parse errors and the target preserved byte for byte.
- **All three providers return thumbnails and durations**, so video results are
  tiles with a duration badge rather than audio's rows.

## Decisions (from brainstorming)

- **Referenced and embedded, uniformly — never stored.** Commons and Archive
  videos are freely licensed and technically downloadable, so this is a real
  choice rather than a constraint. Three reasons decided it. Local media
  playback is currently broken in the Wails window on Linux (see the open point
  in the slice 2 design), so a stored video would be dead on the developer's own
  machine, while remote playback demonstrably works — that is why audio preview
  works. Video files are 10–500 MB against a ~100 KB pronunciation clip. And
  YouTube may not lawfully be downloaded, so embedding keeps every provider
  behaving identically instead of splitting into two code paths and two fact
  shapes.
  - **Accepted cost:** no offline study, and a dead link if the video is removed
    upstream — a genuine risk for small PeerTube instances.
- **Three keyless providers now; YouTube search in slice 4.** YouTube is an
  outlier on three axes at once — it is the only provider needing an API key,
  the only one that must be an `<iframe>`, and the only one where reading the
  playback position needs a separately-loaded player API. Bundling it would give
  this slice two render paths, two timestamp mechanisms and a settings surface.
  It gets its own slice, as audio did rather than being bolted onto images.
- **YouTube by pasted URL, though.** A pasted watch link is recognised and
  embedded, with manual timestamp entry. Cheap, and it means the feature is
  never a dead end for the largest corpus.
- **A start time, not a clip.** "Take me to the bit that matters" is most of the
  value. An end point with looping would need each provider's player API driven
  separately, which is three integrations and better designed after living with
  a start time.
- **The timestamp is captured from the player, not typed.** Because the three
  search providers expose direct file URLs, the player is *our own* `<video>`
  element, so `currentTime` is readable with no per-provider API. Scrub, click
  **Set start here**, done. A typed `mm:ss` field remains for override.
- **The fact carries a note line.** A bare URL is unreadable months later, so
  promotion emits the link plus a `(note)` line with the title and time. This
  departs from the "provenance stays out of the N4L" rule of slice 1, and does
  so knowingly: a title is not machine bookkeeping, it is what makes the line
  legible to a human.
- **The fact points at the watch page, not the file URL.** File URLs are CDN
  paths that rotate and expire; watch pages are durable, shareable, and open in
  a browser.
- **"Import file…" is hidden for video.** Nothing is stored locally in this
  slice, and a local file would hit the very playback bug being routed around.

## Architecture

### Search layer — `internal/mediasearch`

Three new providers, each `Kinds() == []string{"video"}`.

**`CommonsVideo`** — `{query} filetype:video` through the existing Commons
plumbing. Returns `video/webm` with `thumburl` (via `iiurlwidth`) and duration.

**`PeerTube`** — SepiaSearch (`https://sepiasearch.org/api/v1/search/videos`),
which federates across instances including education-ministry ones such as
`tube-sciences-technologies.apps.education.fr`. Returns title, duration, and a
**relative** `thumbnailPath` that must be joined with the instance host.

**`InternetArchive`** — `advancedsearch.php` with
`mediatype:movies AND NOT collection:tvarchive`. The exclusion is load-bearing:
without it the TV News Archive dominates (418 hits, all news captures); with it,
256 remain including UC Berkeley lecture webcasts. Thumbnails come from
`https://archive.org/services/img/<identifier>`. Its corpus is uneven even so —
the top hit for *photosynthesis* is a YouTube mirror — which is a known
characteristic, not a defect to fix.

**Provider IDs are contract, not display labels.** They are persisted on the
cell and are what `buildVideoURL` branches on to build a timestamped link, so a
mismatch produces a silently wrong URL rather than an error:

| ID | Source |
|---|---|
| `commons-video` | `CommonsVideo` |
| `peertube` | `PeerTube` (SepiaSearch) |
| `archive` | `InternetArchive` |
| `youtube` | a pasted `youtube.com/watch` or `youtu.be` link |
| `url` | any other pasted URL |

Earlier slices already use `commons`, `openverse`, `pronunciation`,
`commons-audio`, `file` and `url`; none collide. Note `youtube` is **not** a
provider in the registry — it is only ever set by the paste path, and exists so
`buildVideoURL` can emit `&t=223s` rather than `?start=223`.

**Play URLs resolve lazily, on pick.** Commons returns the file URL in search
results, but SepiaSearch returns only a watch page and Archive only an
identifier; resolving those eagerly would mean 20+ extra requests per search. A
second method on the video providers does it once, when a video is chosen:

```go
// ResolvePlayURL turns a search result into a URL a <video> element can play.
// Commons already knows it and returns it unchanged; PeerTube and Archive fetch
// the file list on demand.
ResolvePlayURL(ctx context.Context, r Result) (string, error)
```

`Result` needs no new fields: `FullURL` carries the play URL once resolved,
`SourcePage` the watch page.

**HLS-only instances are an expected outcome.** Some PeerTube instances publish
only `streamingPlaylists` (`.m3u8`), which a bare `<video>` cannot play. Those
results fail at pick with a plain "no playable file" message. This is reported,
not engineered around — HLS playback is out of scope.

### Bindings

- `SearchMedia(providerID, query, "video", limit)` — unchanged signature.
- `ResolveVideoPlayURL(providerID string, r mediasearch.Result) (string, error)`
  — new, called once on pick.

### Cell shape

```js
{
  id, type: 'media',
  kind: 'video',
  embedURL: '',      // the play URL (file) or the YouTube watch URL
  playerKind: '',    // 'file' | 'iframe'
  sourcePage: '',    // durable human watch page; what the N4L fact links to
  title: '',         // drives the (note) line
  startSec: 0,
  durationMs: 0,
  provider: '',
  query: '', subject: '',
  asset: '', mime: '', // stay empty for video — nothing is stored locally
}
```

**`asset` staying empty breaks an existing assumption.** `MediaCell` decides it
is "filled" with `{#if !cell.asset || replacing}`, which for video is never
true, so the cell would be stuck on the acquisition panel. The test becomes a
`hasContent(cell)` helper — `cell.asset || cell.embedURL` — used in both that
branch and the Promote guard.

### Frontend

- **`media/MediaVideoView.svelte`**, registered as `views.video`:
  - `playerKind === 'file'` → `<video controls preload="metadata" src={embedURL}>`.
    A remote `src`, which is what sidesteps the Linux pipe bug.
  - `playerKind === 'iframe'` → a `youtube-nocookie.com/embed/<id>` iframe.
  - On load, seek to `startSec`, so reopening a cell resumes at the mark.
- **Timestamp control** beneath the player: an `mm:ss` field plus a **Set start
  here** button reading `videoEl.currentTime`. For `playerKind === 'iframe'` the
  button is **absent** and the field is typed — the same "remove the affordance
  rather than explain it" principle used for Transcribe and Crop, and honest,
  since the button cannot work there.
- **Results are tiles** with thumbnail, duration badge and title. No in-list
  preview: thumbnails inform adequately, and autoplaying video while browsing
  would be unpleasant.
- **Paste URL:** a `youtube.com/watch` or `youtu.be` link becomes
  `playerKind: 'iframe'`; anything else is treated as a direct video file.
- **"Import file…" is hidden when the kind is video.**

### N4L

Added to `SSTconfig/arrows-EP-3.sst`, matching neighbouring column alignment:

```
 + has video (vid)   - is video for (vid-for)
```

Promotion emits two lines:

```n4l
photosynthesis
"  (vid) https://diler.tube/videos/watch/81840642?start=223
"  (note) Dr. Binocs Show — Photosynthesis (3:43)
```

`buildVideoURL({sourcePage, provider, startSec})` joins `media-fact.js` beside
`mediaArrow` as a pure, unit-tested function. It lives on the frontend because
it must also serve pasted YouTube links, which have no backend provider. Each
provider spells time differently — Commons `#t=223`, PeerTube `?start=223`,
Archive `?start=223`, YouTube `&t=223s`. When `startSec` is 0 no parameter is
appended; when there is no title the `(note)` line is omitted rather than
emitted empty.

## Error handling

- **HLS-only PeerTube result** — "no playable file" at pick; expected, not a bug.
- **`ResolvePlayURL` failure** — the cell stays on the acquisition panel with the
  reason shown.
- **Playback failure** — reuses `mediaErrorMessage` from slice 2; no third copy
  of the `MediaError` wording.
- **Dead upstream link** — an inherent risk of referencing rather than storing;
  reported as a load failure rather than appearing blank.
- Empty results and provider errors stay distinct; stale searches are still
  discarded by generation counter.

## Testing

**Go** — canned `httptest` fixtures per provider:

- Query shaping for all three, including Archive's `NOT collection:tvarchive`
  and Commons' `filetype:video`.
- Normalization into `Result`, including PeerTube's relative `thumbnailPath`
  being joined with its instance host.
- `ResolvePlayURL` for each: Commons as a no-op, PeerTube and Archive selecting
  a progressive file, and an **HLS-only PeerTube fixture** asserting the clear
  error rather than a panic or a silent empty string.

**JS** — `buildVideoURL` for all four link shapes including `startSec: 0`; the
two-line fact with and without a title; the `hasContent` helper for both an
asset-backed and an `embedURL`-backed cell.

**A live-API check using the parameters the app actually sends** — the specific
failure that shipped a broken provider in slice 1 and was nearly repeated in
slice 2.

**A manual test plan**, whose first case must be the risk below.

## Known risk

This design assumes a **remote-`src` `<video>` plays in the Wails window on
Linux**. That is strongly implied — audio preview works by exactly that
mechanism, and the known Linux failure is specific to *local* files served
through Wails' non-seekable pipe — but it is **not verified for video**. If it
proves false, this slice has no fallback, because local storage is the thing it
routes around. It is therefore the first case in the manual test plan, ahead of
anything else worth checking.

## Out of scope for this slice

- **YouTube search and its API key** — slice 4.
- **Clip end and looping** — needs each provider's player API.
- **Any local storage of video**, and therefore offline use.
- **Video on the study card.** `RecallCard` renders `front`/`back` as plain
  text, so a promoted `vid` fact shows as a URL there, exactly as `pic` and
  `pron` show as paths.
- **Reading playback position out of an iframe.**
- **HLS playback** — no `hls.js`.
