# Media Cell — Slice 3 (Video) Manual Test Plan

Prereqs: open a collection with a processing notebook; a network connection for
the search/fetch/embed cases (all cases except TC-12); the arrow declared in
`SSTconfig/arrows-EP-3.sst` (`vid`/`vid-for`) must be present for TC-9 to
resolve correctly (this branch adds it).

**Run TC-1 first and do not proceed past it if it fails — see its EXPECT.**

## TC-1 Platform risk: remote video plays at all — HALT ON FAILURE

1. Add a Media cell, switch the kind to **Video**, leave provider on the
   default (Wikimedia Commons), search `photosynthesis`, and click one of the
   result tiles.
   - EXPECT: the tile dims while the pick resolves, then the cell leaves the
     acquisition panel and renders `MediaVideoView`'s `<video controls>`
     element with a remote `src` (`cell.embedURL`, the Commons file URL), and
     the video plays back with picture and sound in the Wails window.
   - **If it fails** — the video frame shows the `.vv-err` banner (e.g.
     "Video: format/codec not supported in this webview (MediaError 4)." or
     "Video failed to load (MediaError 2).") instead of playing, or the frame
     stays black with no controls response — **STOP THE PASS.** Record the
     exact `.vv-err` text (or console error) and do not continue to TC-2. This
     slice's entire design assumes a remote-`src` `<video>` plays in this
     webview (the same mechanism slice 2's audio preview relies on), and it
     ships no fallback because avoiding local storage of video is the point of
     the slice. A failure here means every other case in this plan is moot.

*(Deferred to human verification — this agent has no GUI and cannot exercise
`<video>` playback.)*

## TC-2 Kind toggle

1. On a fresh Media cell's acquisition panel, note the Image/Audio kind
   buttons (`ms-on` highlight on the selected one, per slice 1/2).
2. Click **Video**.
   - EXPECT: a third kind button reading "Video" is present alongside Image
     and Audio; clicking it applies the same `ms-on` highlight style as the
     other two kinds (`MediaSearchPanel.svelte`'s `.ms-btn.ms-on` rule, not a
     separate video-only style); the language `<select>` (audio-only) is not
     shown; the provider dropdown repopulates (see TC-3).
3. Search Commons for `photosynthesis` (kind Video) so results are on screen,
   then click **Image**.
   - EXPECT: the video results grid disappears and no results are shown at all
     (not even a stale "No results" message) — `setKind()` resets both
     `results` and `searched` on every kind change, regardless of direction.
4. Switch back to **Video**.
   - EXPECT: results are empty again (the same reset), and the provider
     dropdown reverts to the video provider list (TC-3).

## TC-3 Provider list for video

1. With kind set to Video, open the provider dropdown.
   - EXPECT: exactly three entries, in this order: "Wikimedia Commons",
     "PeerTube", "Internet Archive" (`internal/mediasearch/registry.go`'s
     `Providers()` order, filtered to `kinds` containing `"video"`). None of
     the image-only or audio-only providers ("Openverse",
     "Pronunciation (Lingua Libre)", "Commons audio") appear.
2. Switch to Image, then Audio, and open each provider dropdown in turn.
   - EXPECT: neither "PeerTube" nor "Internet Archive" appears for Image or
     Audio — the video providers are exclusive to the Video kind, matching how
     Openverse and the audio providers are already kind-exclusive (slice 2
     TC-2).

## TC-4 Result tiles

1. With kind Video, provider "Wikimedia Commons", search `photosynthesis`.
   - EXPECT: a grid of tiles (`.ms-vgrid`, wider cells than the image grid's
     `.ms-grid` — `minmax(160px, 1fr)` vs `minmax(96px, 1fr)`), each showing a
     thumbnail image, a duration badge in the top-right corner (e.g. `1:30`),
     and a title beneath the thumbnail.
2. Switch provider to "PeerTube", search `photosynthesis`.
   - EXPECT: tiles render with thumbnails that resolve correctly even though
     the API returns a host-relative `thumbnailPath` — the provider code joins
     it with `https://` + `account.host` before returning `thumbUrl`
     (`internal/mediasearch/peertube.go`); a tile whose account host was empty
     in the API response would legitimately show no thumbnail rather than a
     broken image, per the same code's comment.
3. Switch provider to "Internet Archive", search `photosynthesis`.
   - EXPECT: tiles render with thumbnails, durations (where available) and
     titles as in steps 1–2.
4. For all three providers, confirm no tile is a bare square with a stretched
   or cropped-to-1:1 thumbnail — video tiles use a 16:9 image and `aspect-ratio:
   auto` on the tile itself (`.ms-vhit`/`.ms-vhit img`), not the image grid's
   1:1 `.ms-hit`/`.ms-hit img` rules.

## TC-5 PeerTube pick

1. From a PeerTube video results grid (TC-4 step 2), click a tile whose
   instance is known to publish a progressive (non-HLS-only) file.
   - EXPECT: the tile dims (`ms-loading`) while `ResolveVideoPlayURL` runs
     (fetching the instance's own file list), then the cell fills and the
     video plays in `MediaVideoView`'s `<video>` element.
2. Click a tile from an instance that publishes HLS only (`.m3u8`, no
   progressive file) — if none is known in advance, keep picking PeerTube
   results until one resolves this way.
   - EXPECT: **no blank player** — an error appears in the acquisition panel
     reading exactly `mediasearch: peertube: no playable file (this instance
     publishes HLS only)` (from `ResolvePlayURL` in
     `internal/mediasearch/peertube.go`, surfaced via the panel's `err` div);
     the cell stays in its acquisition (unfilled) state.

## TC-6 Archive pick

1. From an Internet Archive video results grid, click a tile.
   - EXPECT: the tile dims while `ResolvePlayURL` fetches the item's file list,
     then the video plays.
2. Note that the Archive corpus returned by this query is uneven: the search
   term `photosynthesis` commonly surfaces `youtube-*`-identifier items (mirror
   uploads of YouTube videos) as top hits rather than original Archive content.
   - EXPECT: this is expected and not a bug — the query only guarantees
     `NOT collection:tvarchive`; a YouTube-mirror hit still resolves and plays
     like any other Archive item, it just is not "genuine" archival footage.

## TC-7 Set start here

1. On a filled video cell (from TC-1, TC-5, or TC-6), let the video play or
   scrub the native `<video>` control bar to a specific moment (e.g. 0:45),
   then click the **Set start here** button beneath the player.
   - EXPECT: the button is present (it renders only when `cell.playerKind !==
     'iframe'`, i.e. any file-backed pick); after clicking, the label to its
     right (`start Ns`) updates immediately to the rounded whole-second value
     of the player's current time (`MediaVideoView`'s `setStartHere()` calls
     `onStart(videoEl.currentTime)`, and `MediaCell.onStart` rounds and floors
     at 0).
2. Reload the notebook (restart the app or reopen the extract).
   - EXPECT: the cell still renders filled, and on load the player seeks to the
     stored `startSec` automatically — `onLoaded()` sets
     `videoEl.currentTime = cell.startSec` once the metadata loads, but only if
     the drift exceeds 1 second (so a video that already starts near that
     point does not visibly re-seek).

## TC-8 Promote

1. On a filled video cell that has a title (from a search-result pick, which
   sets `cell.title` from the provider's result), fill the subject field (or
   confirm it auto-filled from the search query) and mark a start time (TC-7).
   - EXPECT: the Promote button reads exactly `Promote to N4L (vid)`
     (`mediaArrow` returns the `vid` code whenever `cell.kind === 'video'`).
2. Click "Promote to N4L (vid)".
   - EXPECT: a new N4L draft cell is inserted directly below the Media cell,
     containing **exactly two** lines under the subject:
     ```
     <subject>
     "  (vid) <sourcePage-with-provider-time-param>
     "  (note) <title> @ <m:ss or h:mm:ss>
     ```
     The clock is joined with `@`, **never wrapped in parentheses**: N4L reads
     `(...)` as arrow syntax, so a parenthesised clock is silently consumed by
     the parser and never reaches the graph. Verified: with `@` the note's
     target arrives intact as e.g. `Dr. Binocs Show — Photosynthesis @ 3:43`.
     — e.g. for a Commons pick, the `(vid)` target ends in `#t=<seconds>`; for
     a non-Commons file pick, it ends in `?start=<seconds>` or
     `&start=<seconds>` if the URL already has a query string
     (`buildVideoURL` in `frontend/src/lib/media-fact.js`). If the start time
     is 0, no time parameter is appended and the `(note)` line's title carries
     no clock suffix.
3. Pick (or paste) a video with **no title** — e.g. a paste-URL video (TC-11)
   where `cell.title` stays `''`.
   - EXPECT: the promoted draft contains **only** the `(vid)` line under the
     subject — no `(note)` line at all, since `buildMediaFact` only appends
     the note line when `cell.title` is non-empty after trimming.

## TC-9 Push and graph

1. From the promoted draft cell in TC-8 step 2, push it to an N4L file using
   the existing push flow.
   - EXPECT: the file is written with the subject line followed by the
     `(vid)` and (if present) `(note)` ditto lines exactly as shown in TC-8.
2. Build the graph (Ctrl+G), open the graph explorer, and find the promoted
   subject node.
   - EXPECT: a `has video` edge is present, resolved to its declared long name
     and reverse (`has video` / `is video for`, from the `vid`/`vid-for`
     arrow just added to `SSTconfig/arrows-EP-3.sst`), not a synthesized or
     unknown arrow. *(Deferred to human verification — see the report; this
     agent verified the parse step directly with a temporary unit test
     instead — see the report's parser round-trip output — but cannot run the
     graph explorer's GUI.)*

## TC-10 YouTube paste

1. With kind Video, paste a `https://www.youtube.com/watch?v=<id>` (or
   `https://youtu.be/<id>`) URL into the "…or paste a video or YouTube URL"
   field and click Fetch.
   - EXPECT: no download occurs (video is referenced, never stored); the cell
     fills and `MediaVideoView` renders an `<iframe>` (not a `<video>` element)
     pointed at `https://www.youtube-nocookie.com/embed/<id>`, which plays
     inline.
   - EXPECT: **"Set start here" is absent** — the button only renders when
     `cell.playerKind !== 'iframe'`, and a YouTube paste sets `playerKind:
     'iframe'`.
2. There is no dedicated start-time input in this UI, and Set start here is
   absent for YouTube (step 1). So paste a YouTube URL that already has a
   time parameter **typed into it directly**, e.g.
   `https://www.youtube.com/watch?v=<id>&t=90s` (the form YouTube's own "Copy
   video URL at current time" produces), click Fetch, fill the subject, and
   Promote.
   - EXPECT: the promoted `(vid)` line's URL is the pasted URL **verbatim**,
     `&t=90s` included — a YouTube pick always resets `cell.startSec` to `0`
     (`MediaCell.onPick`, unconditional for every kind), so `buildVideoURL`'s
     `if (!t) return url` branch returns the pasted string completely
     unmodified rather than re-deriving or duplicating a time parameter. This
     is the sense in which "a typed start time is honoured": whatever the user
     already typed into the URL survives untouched, because the app never
     rewrites a YouTube URL whose computed start is zero.
3. Paste a non-YouTube-shaped URL that nonetheless contains `youtube.com` in
   an unexpected position, or a genuine watch URL with a malformed/short video
   id.
   - EXPECT: if no id can be extracted by the regex (6+ URL-safe characters
     after `v=` or after `youtu.be/`), the iframe is not rendered; instead the
     cell shows "Not a recognised video link: `<url>`" — `ytId` stays `''`, so
     `embedSrc` is also `''`, falling to the `{:else}` branch in
     `MediaVideoView.svelte`.

## TC-11 Non-YouTube paste

1. With kind Video, paste a direct `.mp4` or `.webm` URL (not YouTube) into
   the paste field and click Fetch.
   - EXPECT: no download occurs; the cell fills with `playerKind: 'file'` and
     `MediaVideoView` renders the native `<video>` element (`isFile` is true
     whenever `playerKind !== 'iframe'`), sourced directly from the pasted URL.
   - EXPECT: **Set start here is present** (unlike TC-10), since this pick is
     file-backed, not an iframe.
2. Paste a URL with no scheme (e.g. a bare path or a title accidentally pasted
   into the field) and click Fetch.
   - EXPECT: an error appears reading "Paste an http(s) video or YouTube URL."
     and nothing is picked — `fetchPasted()` rejects any pasted video string
     that does not match `^https?://` before it ever reaches `onPick`.

## TC-12 Import file is hidden for video

1. With kind set to Video, look at the row below the search bar.
   - EXPECT: no "Import file…" button is present — only the paste-URL input
     and its "Fetch" button (`MediaSearchPanel.svelte`'s `{#if kind !==
     'video'}` guard around the Import button).
2. Switch kind to Image, then to Audio, and check the same row each time.
   - EXPECT: "Import file…" is present for both Image and Audio, unchanged
     from slices 1 and 2, filtered to image or audio file types respectively.

## TC-13 Dead link

1. With kind Video, paste a syntactically valid but unreachable
   `https://…nonexistent-host…/dead.mp4`-style URL and click Fetch.
   - EXPECT: the cell fills anyway (video is only referenced, and the paste
     path never checks reachability before calling `onPick`); the `<video>`
     element attempts to load `src` and fails, and `MediaVideoView`'s
     `.vv-err` banner appears with text from `mediaErrorMessage` (e.g. "Video
     failed to load (MediaError 2)." or a codec-flavoured message, depending
     on how the webview classifies the network failure) — **not** a blank or
     frozen frame with no explanation.

## TC-14 Regression: image media cell unaffected

1. Search Wikimedia Commons for `apple` on an Image-kind Media cell, pick a
   result, apply a crop, and click "Promote to N4L".
   - EXPECT: search, pick, and crop behave exactly as in slices 1–2; the
     Promote button reads `Promote to N4L (pic)` and emits the `(pic)` ditto
     line unchanged.

## TC-15 Regression: audio media cell unaffected

1. Set kind to Audio, search a pronunciation, preview a row, pick it, and
   promote.
   - EXPECT: preview, pick, and the `(pron)`/`(aud)` promotion behave exactly
     as in slice 2 (see `2026-07-27-media-cell-slice2-manual-test-plan.md`
     TC-3, TC-6, TC-10) — nothing in this slice touched
     `MediaAudioView.svelte`'s rendering or the audio branches of
     `MediaSearchPanel.svelte` beyond adding the sibling Video kind button and
     the shared (and unused, for audio) `onStart` prop.
2. Open (or build) a notebook containing a Scan cell and a separate Audio
   cell (record + Transcribe).
   - EXPECT: both behave exactly as before this slice — neither
     `ImageCell.svelte` nor `AudioCell.svelte` was touched.

## TC-16 Regression: existing image/audio notebooks still render

1. Open a notebook saved before this slice, containing image and/or audio
   Media cells (`cell.kind` of `"image"`, `"audio"`, or absent).
   - EXPECT: each renders in its existing view (`MediaImageView` or
     `MediaAudioView`) exactly as before — `views[cell.kind]` in
     `MediaCell.svelte` is unchanged apart from adding the `video` entry, and
     `hasContent(cell)` still reports true for these cells via `cell.asset`
     (the new `cell?.embedURL` check is additive, only relevant to video
     cells that have no `asset`).

## Known risk (see TC-1)

This slice's entire approach — resolving a play URL lazily and pointing a
`<video>` element at a remote CDN/instance URL, or an `<iframe>` at YouTube,
rather than downloading and storing anything locally — assumes a remote-`src`
`<video>` plays in the Wails window on Linux. Audio preview (slice 2) already
demonstrates that a remote `<audio src>` plays fine through this same webview,
and the known Linux playback failure is specific to *local* files served
through Wails' non-seekable asset pipe — but this has **not been verified for
`<video>` specifically**, and this slice ships **no fallback**, because
avoiding local video storage (and the local-playback bug that comes with it) is
exactly what the slice is for. TC-1 exists to test this assumption first and
must halt the rest of the pass if it fails.

## TC-17 Typed start time (added after the first manual pass)

The implementation originally shipped only the **Set start here** button, which
is absent on the YouTube path — leaving no way at all to mark a moment on a
YouTube cell. The typed field the design called for was missing.

1. On a **YouTube** cell (paste a `youtube.com/watch?v=…` link), find the start
   field beside "Watch on YouTube".
   - EXPECT: the field is present, and **Set start here** is absent.
2. Type `3:43` and press Enter (or click away).
   - EXPECT: the field shows `3:43` and it survives a notebook reload.
3. Click "Watch on YouTube".
   - EXPECT: the browser opens the video **at 3:43**, i.e. the URL ends `&t=223s`.
4. Type nonsense (`abc`) and press Enter.
   - EXPECT: the field reverts to `3:43`. It must NOT reset to `0:00` — that
     would silently discard the marked moment.
5. Enter `223` (bare seconds) and then `1:02:03`.
   - EXPECT: accepted as 3:43 and 1:02:03 respectively.
6. On a **file** video (a search result), use **Set start here**, then type a
   different value in the field.
   - EXPECT: both routes work and agree; the field refreshes after the button
     is used, and "Open in browser" opens at whichever value is current.
7. Promote after setting a typed start.
   - EXPECT: the `(vid)` URL carries the time parameter and the `(note)` line
     ends `@ <clock>`.
