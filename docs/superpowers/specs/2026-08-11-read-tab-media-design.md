# Media in the Read tab

**Date:** 2026-08-11
**Status:** Shipped — describes what is in the tree
**Feature area:** Workbench → Read tab

Supersedes `2026-08-11-notes-media-design.md` and
`2026-08-11-notes-media-http-design.md`, both of which describe a CodeMirror
implementation that was built, found to be in the wrong surface, and deleted.

## Summary

In the Read tab, a node that points at an image, video or audio file shows the
media itself beneath the row naming it. Local files are served by the Go asset
server; `http(s)://` targets load from the web.

## Motivation

Notes routinely point at assets — a diagram, a lecture clip, a recorded phrase.
The Read tab is where notes are read rather than edited: it resolves arrow codes
to their descriptions, so `(pic)` reads as *has a picture*. It rendered the
target as a path, which meant leaving the app to see the thing being described.

## Grounding in the current implementation

- **`NodeCard.svelte`** renders each node: its text, then a `<dl>` of arrow
  entries where `<dt>` is the arrow's `LongName` and `<dd>` is the target. It is
  reached from `ReadTab.svelte`, which is one of the two tabs in `BottomBand`.
- **A parsed node carries its arrows structurally.** For
  `"  (pic) media/x.jpg` beneath `apple image`, the Go parser produces
  `node.Text = "apple image"` with one ditto whose arrow is
  `{Code: "pic", Target: "media/x.jpg"}`. Nothing needs to scan raw text: the
  target arrives as a field.
- **`NodeCard` already had inline media**, but only over whitespace-delimited
  tokens inside `node.Text` — never over arrow targets, which is where a path
  actually appears. It also resolved to `` `${collectionDir}/${token}` ``, a
  filesystem path a webview cannot fetch, so that path could never have loaded
  either.
- **`internal/media/server.go`** serves `<collectionDir>/media/` at
  `/media/<name>` for the notebook. `main.go` composes it with a second route
  behind a `http.ServeMux`.
- **Wails tries the embedded frontend assets first** and calls the configured
  `Handler` only when they do not contain the path
  (`pkg/assetserver/assethandler.go`), so composing handlers cannot swallow the
  app's own UI.

## Decisions

- **Detection is by the target's file extension, never the arrow's name.** Arrow
  names are free text the author writes: matching them would mean `has an image`
  works while `pic`, `illustrated by` or a typo silently does not. The extension
  is a property of the thing being pointed at.
- **Media renders beneath the row, and the path stays visible.** A wrong path is
  then diagnosable in place rather than silently absent.
- **Local files are served over HTTP, not read through a binding.**
  `http.ServeFile` gives native range requests, so video seeks with no work;
  the browser caches; nothing is retained in JS.
- **A remote URL loads automatically.** The cost is accepted: opening a note
  fetches from that host, and remote media cannot work offline while local media
  can. Rejected: a click-to-load gate, and caching remote bytes into the
  collection.
- **The `/collection/` route enforces containment**, which is stricter than the
  binding it replaced. A target resolving outside the collection directory 404s.
- **Only the Read tab renders media.** The Note tab and Study mode's reading
  notes pane are raw N4L editors; media there is noise. This was learned the
  expensive way — see below.

## Architecture

### `frontend/src/lib/n4l-media.js`

Two pure functions, dependency-free so they run under `node --test` — this repo
has no Svelte component harness.

```
mediaKind(path) -> 'image' | 'video' | 'audio' | null
srcFor(path)    -> string
```

| kind | extensions |
|---|---|
| image | `.png` `.jpg` `.jpeg` `.gif` `.webp` `.svg` `.bmp` |
| video | `.mp4` `.webm` `.mov` `.m4v` |
| audio | `.mp3` `.wav` `.ogg` `.m4a` `.flac` |

Matching is case-insensitive and looks only at the final extension, so
`notes-about-png.txt` is not an image and `archive.txt.png` is.

`srcFor` returns a `http(s)://` path unchanged. Anything else becomes
`/collection/` plus the path with **each segment** `encodeURIComponent`-ed —
per segment so `/` separators survive while a space, `#` or `?` inside a
filename does not truncate the URL.

### `frontend/src/components/InlineMedia.svelte`

Thin glue: takes a `path`, renders `<img>`, `<video controls>` or
`<audio controls>` when `mediaKind` recognises it, and nothing otherwise.
Capped at 240px tall.

### `frontend/src/components/NodeCard.svelte`

Renders `<InlineMedia path={arrow.Target} />` inside the `<dd>` of every arrow
entry — node arrows and ditto arrows alike — and for each media token found in
the node's own text.

### `internal/media/server.go`

```go
func CollectionServer(dirFn func() string) http.Handler
func resolveUnder(root, rel string) (string, bool)
```

`CollectionServer` serves `/collection/<relative-path>`. `resolveUnder` joins
and then requires the result to stay under the root.

Containment is a separate function because it cannot be tested through the
handler: `http.ServeFile` rejects any request whose URL path contains a `..`
element (`net/http/fs.go:815`) before the handler's own check runs, so a
handler-level test passes with the check deleted.

Content types get a narrower override than `FileServer`'s. `/media/` holds only
imported audio, so labelling `.mp4` and `.webm` audio there is right; this route
serves whatever a note references, where both are video.

## Error handling

| Case | Behaviour |
|---|---|
| Local file missing | Route 404s; the element shows its own broken state, path still readable above |
| Path escapes the collection | Route 404s |
| No collection open | Route 404s |
| Remote host unreachable, or offline | The element's own error state |
| Target is not media | No element at all — the path renders as plain text |

## Testing

**JS** (`node --test`): `mediaKind` over every extension, uppercase, unknown,
empty, no extension, and extension-versus-substring. `srcFor` for `http`,
`https`, uppercase `HTTPS://`, a nested local path, and filenames containing a
space, `#` and `?`.

**Go:** `CollectionServer` serves nested paths under `media/` and `assets/`,
404s escapes and an unopened collection, and types `.mp4`/`.webm` as video while
`.mp3`/`.wav` stay audio. `resolveUnder` is tested directly, including
`a..b/c.png` — `..` inside a filename is not a traversal.

**Manual:** `2026-08-11-read-tab-media-manual-test-plan.md`.

## What was built and discarded

Worth recording, because it was most of the work.

The first two implementations decorated the **CodeMirror** editor: a
`ViewPlugin`, then a `StateField`, emitting block widgets beneath each line,
loading bytes through a Go binding as base64, decoding to a `Blob` and holding
object URLs. Four real defects were found and fixed in it — block decorations
throw when provided from a `ViewPlugin`; an object URL is orphaned when
CodeMirror reuses a widget's DOM; decoding schedules no re-measure so the height
map drifts; a module-level cache keyed on a relative path outlives the
collection it was filled from.

All of it is deleted. It was correct, and it was in a surface nobody was reading
notes in. The `Read` tab renders through `NodeCard` and receives arrows already
parsed by Go, so the entire scanning-and-decorating layer was unnecessary.

The lesson is cheap to state and was expensive to learn: establish **which
surface** a feature belongs to before building it. Two rounds of review found
genuine bugs in code that should never have existed.

## Out of scope

- Media in the Note tab or Study mode's reading notes pane.
- Caching remote media into the collection.
- Page URLs that are not direct media files — `(vid) https://archive.org/details/X`
  has no media extension and stays plain text.
- Adding, replacing or removing an asset from the Read tab.
