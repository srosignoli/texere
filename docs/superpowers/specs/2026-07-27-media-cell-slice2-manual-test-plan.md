# Media Cell — Slice 2 (Audio) Manual Test Plan

Prereqs: open a collection with a processing notebook; a network connection for
the search/fetch cases (TC-1, 2, 3, 4, 5, 7, 8, 13, 14); the arrows declared in
`SSTconfig/arrows-EP-3.sst` (`pron`/`pron-rec`, `aud`/`aud-for`) must be present
for TC-10/TC-11 to resolve correctly (this branch adds them).

## TC-1 Kind toggle

1. Click `+ Media` to add a new Media cell.
   - EXPECT: the acquisition panel shows the Image/Audio kind buttons with
     **Image** selected (the `ms-on` highlight), the provider dropdown defaults
     to Wikimedia Commons, and no language selector is visible.
2. Search for `apple` on the default (Commons) provider so results are on
   screen.
3. Click **Audio**.
   - EXPECT: the provider dropdown's option list changes to exactly
     "Pronunciation (Lingua Libre)" and "Commons audio" (Wikimedia Commons and
     Openverse are gone); the dropdown auto-selects "Pronunciation (Lingua
     Libre)" (the first audio-capable provider); a language `<select>` appears
     next to the kind buttons; the Commons image results grid from step 2 is
     gone and the panel shows no results (not even a stale "No results"
     message, since `searched` is reset to false).
   - EXPECT: the search box placeholder changes to "Search for a
     pronunciation…" and the paste field placeholder changes to "…or paste an
     audio URL" (both depend on `kind`; see TC-3, TC-13).
4. Click **Image** again.
   - EXPECT: the provider dropdown reverts to Wikimedia Commons/Openverse, the
     language selector disappears, and results are still empty (switching
     kind always clears results, regardless of direction).

## TC-2 Openverse absent from audio

1. With kind set to Image, open the provider dropdown.
   - EXPECT: "Openverse" is listed (alongside "Wikimedia Commons").
2. Switch kind to Audio and open the provider dropdown.
   - EXPECT: "Openverse" is not present; only "Pronunciation (Lingua Libre)"
     and "Commons audio" are listed.

## TC-3 Pronunciation search

1. Set kind to Audio, language to Spanish, provider "Pronunciation (Lingua
   Libre)", query `manzana`, and search.
   - EXPECT: a list of result rows appears (not a thumbnail grid); each row
     shows a ▶ preview button and, to its right, a label and a meta line
     joining (with " · ") the language code `spa`, a speaker name, a duration
     like `1.2s`, and a format tag `wav` — e.g. `spa · Eavqwiki · 1.2s · wav`.
     Several distinct speakers/rows appear.

## TC-4 The exact-word filter

1. Set language to French, provider Pronunciation, query `chat`, search.
   - EXPECT: every result row's label is the word "chat" (a recording of
     *chat*); no row's label is "femme" or "quoi" even though a contributor
     named "Nattes à chat" has recordings under those words. (The exact-word
     filter matches the recorded *word*, not text appearing anywhere in the
     filename, including the speaker's name.)
2. Set language to English, provider Pronunciation, query `pie`, search.
   - EXPECT: at least one row's label is the bare word "pie"; rows labelled
     "pie-eyed" or "pie chart" are not shown when an exact "pie" recording
     exists in the result set (the exact-match filter suppresses compounds
     once any exact hit is present).

## TC-5 Fallback

1. Pick a language/word combination unlikely to have an exact Lingua Libre
   recording (e.g. language English, query a rare or invented word that still
   returns some Commons search hits containing the substring, such as a long
   compound word).
   - EXPECT: if the search returns pages but none is an exact recording of the
     word, the grid still shows those near-miss rows rather than "No results
     for …" — the provider returns everything the search matched when no exact
     hit exists (see `internal/mediasearch/pronunciation.go`'s fallback to
     `all`). If the search returns zero pages at all, "No results for …" is
     the correct and expected outcome — this case only exercises the
     near-miss fallback, not a guaranteed non-empty grid for any input.

## TC-6 Preview

1. On an Audio search results list (e.g. from TC-3), click the ▶ button on a
   row.
   - EXPECT: the row's icon becomes ■; audio plays through the browser's
     hidden shared `<audio>` element; nothing is written to
     `<collection>/media/` (only `pick()`, not preview, downloads).
2. While it is playing, click the ▶ button on a different row.
   - EXPECT: the first row's icon reverts to ▶ (its playback stops) and the
     second row's icon becomes ■ and it plays.
3. Click the ■ button on the currently playing row.
   - EXPECT: playback stops and the icon reverts to ▶.
4. Let a preview play to the end without clicking anything.
   - EXPECT: on the `ended` event the icon reverts to ▶ automatically.

## TC-7 Codec failure is named

1. Search Commons audio (provider "Commons audio") for a query likely to
   surface a `.ogg`-encoded result (Commons stores most non-Lingua-Libre audio
   as Ogg Vorbis/Opus), then click ▶ on a row that turns out to be undecodable
   in this webview (WebKitGTK/WSLg commonly cannot decode certain Ogg codecs).
   - EXPECT: no silent failure and no dead ▶ button — an error line reading
     "Preview: format/codec not supported in this webview (MediaError 3)." or
     "…(MediaError 4)." appears below the play/pick rows (from
     `mediaErrorMessage`), and the preview `<audio>` element's `previewURL` is
     cleared (the row's icon reverts to ▶).
2. Pick that same result anyway (click its row, not the ▶).
   - EXPECT: `pick()` still downloads and stores it (picking never touches the
     preview `<audio>` element), and the resulting filled cell's own player
     (TC-8, TC-9) may show the identical decode error since it uses the same
     underlying webview and the same `mediaErrorMessage` helper.

## TC-8 Pick and persist

1. From a Pronunciation or Commons-audio results list, click a row (not its ▶).
   - EXPECT: the row dims (`ms-loading`) while `FetchMediaURL` runs, then the
     cell leaves the acquisition panel and renders `MediaAudioView`: an
     `<audio controls>` element sourced from `/media/<asset>`.
2. Reload the notebook (restart the app or reopen the extract).
   - EXPECT: the same `<audio>` player still renders, pointed at the same
     asset (the asset name persisted on the cell).

## TC-9 No crop controls on an audio cell

1. On the filled audio cell from TC-8, look at the cell's toolbar.
   - EXPECT: no **Crop** or **Clear crop** button is present — only the
     subject field and "Promote to N4L (…)" (and Replace, in the cell's header
     bar).
2. On a filled *image* Media cell (from a Commons/Openverse pick), look at its
   toolbar.
   - EXPECT: **Crop** is present (and **Clear crop** once a crop has been
     applied), unchanged from slice 1.

## TC-10 Promote

1. On the audio cell from TC-8, if it was picked via the Pronunciation
   provider, check the subject field and the Promote button.
   - EXPECT: subject is pre-filled with the search query (e.g. "manzana");
     the button reads exactly `Promote to N4L (pron)`.
2. Click "Promote to N4L (pron)".
   - EXPECT: a new N4L draft cell is inserted directly below the Media cell,
     containing exactly:
     ```
     manzana
     "  (pron) media/<assetname>.wav
     ```
3. Add another Media cell, switch to Audio, pick a result via "Commons audio"
   instead of Pronunciation.
   - EXPECT: the Promote button reads `Promote to N4L (aud)`, and promoting
     emits `"  (aud) media/<assetname>.<ext>` under the subject line.

## TC-11 Push and graph

1. From either promoted draft cell in TC-10, push it to an N4L file using the
   existing push flow.
   - EXPECT: the file is written with the two lines (subject + ditto arrow
     line) appended as shown in TC-10.
2. Build the graph (Ctrl+G), open the graph explorer, and find the promoted
   subject node.
   - EXPECT: a `has pronunciation` edge (for a `pron` fact) or `has audio`
     edge (for an `aud` fact) is present, resolving to its declared long
     name/reverse (`has pronunciation` / `has pronunciation recording of`, or
     `has audio` / `is audio for`) rather than a synthesized/unknown arrow.
     *(Deferred to human verification — see report; not exercised by this
     agent, which cannot run the GUI.)*

## TC-12 Language selection: last-chosen language carries to a new cell

1. Clear any remembered language first (in the webview's devtools/console,
   `localStorage.removeItem('texere.lastLanguage.v1')`), or note whatever
   language is currently remembered so step 4's expectation can be adjusted.
   Add a Media cell, switch kind to Audio.
   - EXPECT: the language selector defaults to "French" — the first entry in
     the language list (`internal/mediasearch/languages.go`), since this
     cell's `lang` starts empty and nothing is remembered yet.
2. Change the language to Spanish and search.
   - EXPECT: the change is persisted on this cell's own `lang` field, and also
     recorded as the last-chosen language in `localStorage`
     (`texere.lastLanguage.v1`, via `setLastLanguage()` in
     `frontend/src/lib/last-language.js`).
3. Reload the notebook (restart the app or reopen the extract).
   - EXPECT: reopening, this same cell's kind is still Audio and its language
     selector still shows Spanish — the choice is persisted on the cell
     itself and restored on load (this per-cell persistence is unchanged).
4. Add a **second**, brand-new Media cell and switch it to Audio.
   - EXPECT: its language selector defaults to "Spanish", **not** French — a
     new cell's `lang` field starts empty, but `MediaSearchPanel.svelte`'s
     `onMount` now seeds it from `getLastLanguage()` when a remembered name is
     present and still exists in the current language list, falling back to
     the list's first entry only when nothing is remembered or the remembered
     name is no longer valid.
5. Change the second cell's language to Italian.
   - EXPECT: the first cell (from steps 1–3) still shows Spanish — an existing
     cell's already-set `lang` is never overwritten by the remembered value;
     only a cell whose `lang` starts empty consults it on mount.

## TC-13 Import file for audio (audio-filtered dialog)

1. Set kind to Audio. Find a direct `.wav` URL (e.g. a Commons upload URL) and
   paste it into the "…or paste an audio URL" field; click Fetch.
   - EXPECT: the file is fetched (via `FetchMediaURL(url, 'audio')`) and the
     cell fills with the audio player (TC-8); the Promote button reads
     `Promote to N4L (aud)`, not `(pron)` (a pasted URL's provider is `'url'`,
     never `'pronunciation'`).
2. With kind set to Audio, click "Import file…".
   - EXPECT: the OS file dialog opens titled "Add audio" and filtered to audio
     types (`*.webm;*.ogg;*.oga;*.wav;*.mp3;*.m4a;*.mp4;*.aac;*.flac`) —
     `MediaSearchPanel.svelte`'s `importFile()` now calls the `ImportAudio`
     Wails binding when `kind === 'audio'`, not `ImportImage`.
3. Select a local `.wav` file in that dialog.
   - EXPECT: the file is copied into `<collection>/media/`, the acquisition
     panel is replaced by `MediaAudioView`'s `<audio controls>` element
     (as in TC-8), and it plays back correctly.
   - EXPECT: the subject field is empty (a file import supplies no query/
     title) and the Promote button reads `Promote to N4L (aud)`, **not**
     `(pron)` — `importFile()`'s `onPick` call sets `provider: 'file'`, and
     `mediaArrow` only returns `pron` when `provider === 'pronunciation'`.
4. With kind set to Image, click "Import file…".
   - EXPECT: the OS dialog is filtered to image types
     (`*.jpg;*.jpeg;*.png;*.gif;*.webp`), unchanged from slice 1 —
     `importFile()` calls `ImportImage` whenever `kind !== 'audio'`.

## TC-14 Oversize and wrong-extension

1. Set kind to Audio, paste the URL of an audio file larger than 25 MB, and
   Fetch.
   - EXPECT: an error naming the cap, e.g. "media: audio larger than 25 MB";
     nothing is stored and the cell stays in its acquisition state.
2. Paste a URL ending in an unsupported audio extension (e.g. `.mid` or a URL
   with no recognizable audio extension) and Fetch.
   - EXPECT: an error such as `media: unsupported audio extension ".mid"` —
     audio is validated by the URL's extension, not by sniffing content, so
     an unsupported extension is rejected before any decode is attempted.

## TC-15 Regression: image media cell unaffected

1. Search Wikimedia Commons for `apple` on an Image-kind Media cell, pick a
   result, apply a crop, and click "Promote to N4L".
   - EXPECT: search, pick, and crop all behave exactly as in slice 1 (see
     `2026-07-25-media-cell-slice1-manual-test-plan.md` TC-2, TC-3, TC-11);
     the Promote button reads `Promote to N4L (pic)` and emits the `(pic)`
     ditto line unchanged.
2. Open a notebook saved before this slice that contains an image Media cell
   (no `kind` field, or `kind: "image"`).
   - EXPECT: it still renders as an image cell with Crop available (per
     `mediaArrow`'s comment: "`kind` is absent on cells saved before audio
     existed, and means image" — but note `mediaArrow` treats *absent* kind
     as image only inside `buildMediaFact`/`mediaArrow`; the cell's own
     `views[cell.kind]` lookup in `MediaCell.svelte` requires `cell.kind` to
     be the literal string `"image"` to select `MediaImageView` — a truly
     `undefined` kind would render nothing there. In practice every
     persisted media cell has had `kind: "image"` since this field's
     introduction, so this is not expected to be observable; call it out to
     the tester only if a truly kind-less cell is found in an old notebook).

## TC-16 Regression: Scan cell and Audio cell unaffected

1. Open (or build) a notebook containing a Scan (`+ Scan`, image) cell and a
   separate Audio cell (record + Transcribe), alongside a Media cell.
   - EXPECT: the Scan cell's Transcribe flow and the Audio cell's
     record/playback/Transcribe flow behave exactly as before this slice —
     neither `ImageCell.svelte` nor `AudioCell.svelte` was touched by this
     slice (only `MediaCell.svelte`, `MediaSearchPanel.svelte`, the new
     `MediaAudioView.svelte`, and the media-fact/media-error/notebook.js
     library files changed).
