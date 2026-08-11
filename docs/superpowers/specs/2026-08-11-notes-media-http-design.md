# Notes media over HTTP: any local path, and the web

**Date:** 2026-08-11
**Status:** Proposed — awaiting approval
**Feature area:** Study mode → Read focus → the notes editor

## Summary

Serve every referenced asset as an ordinary `src` the webview fetches: a
`http(s)://` URL directly, and any other path over a new `/collection/` route on
the Go asset server. The base64 binding, the blob cache and the object URLs go
away.

## Motivation

The inline-media feature renders assets referenced from a note. Two problems
surfaced in use:

- **It looked folder-limited.** It is not — `GetSourceBytes` joins any relative
  path onto the collection directory, so a notebook asset at `media/<sha>.png`
  already renders. Every example in the manual test plan said `assets/`, which
  is what created the impression.
- **A `http(s)://` URL does not render.** It is detected as media, then resolved
  under the collection directory, fails, and shows a `media not found` chip.

Fixing the second exposed that the loading mechanism itself was the wrong one.

## Grounding in the current implementation

- **An HTTP asset server already exists.** `internal/media/server.go`'s
  `FileServer` serves `<collectionDir>/media/` at `/media/<name>`, wired into
  Wails at `main.go:24` via `assetserver.Options.Handler`. The notebook's
  `ImageCell.svelte:22` uses it directly: ``src = `/media/${cell.asset}` ``. No
  base64, no bindings.
- **A 404 from that handler falls through to the embedded frontend assets.**
  `FileServer` already 404s every path outside `/media/`, and the app still
  loads — so composing handlers behind a `http.ServeMux` is safe.
- **`FileServer` is deliberately flat.** It serves `path.Base(r.URL.Path)` only,
  collapsing any directory component, which is why it cannot serve
  `assets/photo.png`.
- **`app.mediaDir()` (`app.go:243`)** reads `collectionDir` under `a.dirMu`
  because the HTTP handler runs on its own goroutine. Any sibling accessor needs
  the same lock.
- **`internal/media/media_test.go:61`** (`TestFileServerServesAndBlocksTraversal`)
  is the established shape for testing these handlers.
- The current media widget instead calls `GetSourceBytes`, base64-decodes in JS,
  builds a `Blob`, and makes an object URL — machinery that exists only because
  the asset server was not found.

## Decisions

- **Serve local assets over HTTP, not through a binding.** `http.ServeFile`
  gives native range requests, so a video seeks with no work at all; the browser
  caches; and nothing is retained in JS. This retires the previous spec's
  admission that memory is unbounded, rather than documenting it again.
- **A remote URL is used as `src` directly, and loads automatically.** A note
  containing an `https` image simply shows it. The cost is accepted: opening a
  note makes a request to that host, and remote media shows a broken state
  offline while local media still works. Rejected: a click-to-load placeholder,
  and fetch-and-cache-into-the-collection — both were live options, and
  automatic loading is what was asked for.
- **A new `/collection/` route rather than widening `/media/`.** `/media/` is
  flat by design and its base-name collapse is its traversal defence; widening
  it would weaken a route the notebook depends on.
- **The new route enforces containment, which tightens today's behaviour.**
  `GetSourceBytes` cleans `..` and happily reads outside the collection; the
  earlier spec recorded that as accepted-but-noted. `/collection/` 404s it
  instead. This is a deliberate behaviour change, not an incidental one.
- **`mediaMime` is deleted.** It existed to type a `Blob`; the server sets
  `Content-Type` now. Keeping a tested-but-unused function would be worse than
  removing it.
- **Detection is untouched.** `mediaKind` and `findMedia` keep working exactly as
  they do, and the `StateField`, the block-below-the-line placement, the 240px
  cap and the scoping to the notes pane all stay.

## Architecture

### `internal/media/server.go`

A second handler beside `FileServer`:

```go
func CollectionServer(dirFn func() string) http.Handler
```

Serving `/collection/<relative-path>` from `dirFn()`. It rejects a request when
no collection is open, and resolves the path with `filepath.Join` (which cleans
`..`) then requires the result to remain under the root — otherwise 404. Audio
content types get the same explicit override `FileServer` applies, because Go's
mime table labels `.webm` as video.

### `app.go` and `main.go`

`app.collectionRoot()` mirrors `mediaDir()`, returning `collectionDir` under
`a.dirMu` — the same lock, for the same reason: the handler runs on the HTTP
goroutine.

`main.go` composes both routes:

```go
mux := http.NewServeMux()
mux.Handle("/media/", media.FileServer(app.mediaDir))
mux.Handle("/collection/", media.CollectionServer(app.collectionRoot))
```

and passes `mux` as `assetserver.Options.Handler`.

### `frontend/src/lib/n4l-media.js`

Loses `mediaMime`. Gains:

```
srcFor(path) -> string
```

A remote path (`http://` or `https://`, case-insensitive) is returned unchanged.
Anything else becomes `/collection/` plus the path with each segment
`encodeURIComponent`-ed, so a filename containing a space, `#` or `?` resolves
rather than being cut short at the fragment or query.

### `frontend/src/lib/n4l-media-view.js`

`toDOM` becomes synchronous: create the element, set `src` from `srcFor`, append.
Deleted with it: the module-level cache, the `load` function threaded through
every widget, `bytesFrom`, the object URLs and the `GetSourceBytes` import.

Two listeners remain, both load-bearing:

- `error` replaces the element with the existing `media not found` chip. Without
  it a failed load shows the browser's broken-image icon instead.
- `load` / `loadedmetadata` calls `view.requestMeasure()`. Decoding is what
  grows the element to its cap and it mutates no DOM, so without this the height
  map under-reports every media block and the line numbers beside it drift.

## Error handling

| Case | Behaviour |
|---|---|
| Local file missing | Route 404s, `error` fires, `media not found: <path>` chip |
| Path escapes the collection | Route 404s, same chip — a change from today, where it loaded |
| No collection open | Route 404s, same chip |
| Remote host unreachable, or offline | `error` fires, same chip |
| Remote URL returns a non-media body | The element's own error state; the path stays readable above it |
| A codec the webview cannot play | The browser's broken-media state inside the capped box |

## Testing

**Go:** `CollectionServer` serves a nested path (`media/x.png` and
`assets/y.png` both, since conflating them is the bug that started this), 404s
`../outside.png`, 404s when `dirFn` returns empty, and sets a `Content-Type` a
webview will accept. Modelled on `TestFileServerServesAndBlocksTraversal`.

**JS:** `srcFor` for an `https` URL, an `http` URL, an uppercase `HTTPS://`, a
nested local path, a filename containing a space, one containing `#`, and one
containing `?` — the last three are why encoding is per-segment rather than
whole-string.

**Manual:** its own test-plan document. The case that prompted this leads: an
asset the notebook wrote under `media/` renders in a reading note. Then a remote
image renders, a remote image with the machine offline shows the chip, a video
seeks, and a path escaping the collection now shows the chip where it used to
load.

## Out of scope

- Caching remote media into the collection.
- A click-to-load gate on remote assets.
- Rendering media anywhere but the reading notes pane.
- Changing detection: which extensions count, and the arrow-agnostic rule.
- `GetSourceBytes` itself, which `PdfSourcePane` still uses and which keeps its
  current behaviour.
