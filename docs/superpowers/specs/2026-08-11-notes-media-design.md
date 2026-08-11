# Inline media in the reading notes pane

**Date:** 2026-08-11
**Status:** Proposed — awaiting approval
**Feature area:** Study mode → Read focus → the notes editor

## Summary

In the notes pane beside a source, a quoted target that names an image, video or
audio file renders as the media itself, below the line that references it. The
text stays put and stays editable.

## Motivation

Notes taken while reading routinely point at assets on disk — a diagram, a
lecture clip, a recorded phrase. Today they are paths. To see one you leave the
app, find the file and open it elsewhere, which is enough friction that the
media may as well not be linked.

## Grounding in the current implementation

- **The notes pane is a CodeMirror editor**, not rendered output
  (`ReadingView.svelte:142`, `new EditorView`). "Formatting" today means
  syntax highlighting from `n4lBaseExtensions`. Showing media therefore means a
  CodeMirror decoration, not a template change.
- **`n4lBaseExtensions(opts)` already accepts `extra` and spreads it**
  (`frontend/src/lib/n4l-editor.js:63,73`). Nothing in that file has to change:
  the reading pane passes its own extension through `extra`, which scopes the
  feature to that one editor by construction rather than by a flag.
- **`GetSourceBytes(refPath)` (`reading_methods.go:383`)** already returns
  base64 for any file resolved against the collection directory, and is already
  used by `PdfSourcePane`. No new binding is needed.
- **`internal/source/web.go:132`** already inlines images as data URLs when
  saving a web article, so data-URL media is an established shape in the app —
  but see the object-URL decision below.
- The app has `MediaImageView`, `MediaVideoView` and `MediaAudioView`, but they
  belong to the notebook's *remote* media search (`media_methods.go` —
  providers, URLs, `FetchMediaURL`). They solve a different problem and are not
  reused here.

## Decisions

- **Detection is by the target's file extension, not the arrow's name.** Arrow
  names are free text the author writes: `has an image` would work while
  `illustrated by`, `screenshot`, or a typo would silently not. The extension is
  a property of the thing being pointed at. Rejected: matching a configured list
  of media arrows, and requiring both to agree.
- **Media renders below the line, never replacing it.** The pane is an editor.
  Hiding the path would make a broken link invisible and unfixable, and would
  make the text awkward to edit. Rejected: replacing the quoted path with the
  media, which is more compact and worse.
- **Object URLs, not data URLs.** A data URL defeats range requests, so a video
  cannot be seeked. The bytes still cross the Wails bridge as base64 either way;
  what changes is the retained form and, decisively, that seeking works.
- **Loads are cached per path, successes and failures alike.** Without a cache,
  typing a path renders and fetches once per keystroke.
- **Scope is the reading notes pane only.** The main editor and the notebook
  overlay keep plain text. This falls out of passing the extension through
  `extra` rather than adding a flag to the shared builder.
- **A failed load shows a chip, not a broken element.** `media not found: <path>`
  in muted text, so the failure names itself.

## Architecture

### `frontend/src/lib/n4l-media.js` (new)

Two pure exports, dependency-free so they run under `node --test` — this repo
has no CodeMirror or Svelte component harness, so logic left inside the plugin
would be untested.

```
mediaKind(path) -> 'image' | 'video' | 'audio' | null
mediaMime(path) -> string          // '' when the extension is unknown
findMedia(lineText) -> [{ path, kind, from, to }]
```

`findMedia` scans a single line for double-quoted spans and returns those whose
contents have a media extension, with offsets relative to the line. Extensions:

| kind | extensions |
|---|---|
| image | `.png` `.jpg` `.jpeg` `.gif` `.webp` `.svg` `.bmp` |
| video | `.mp4` `.webm` `.mov` `.m4v` |
| audio | `.mp3` `.wav` `.ogg` `.m4a` `.flac` |

Matching is case-insensitive. A quoted string with no media extension, and any
unquoted text, are ignored.

### `frontend/src/lib/n4l-media-view.js` (new)

The CodeMirror side: a **`StateField`** that walks the document, calls
`findMedia`, and emits one block `Decoration.widget` per match, positioned at
the end of that line with `side: 1` and `block: true`.

It must be a `StateField`, not a `ViewPlugin`. CodeMirror only lets directly
provided decoration sets affect vertical layout; a `ViewPlugin` registers its
decorations as a *function*, and a block decoration from that path throws
`RangeError: Block decorations may not be specified via plugins` — out of the
`EditorView` constructor when a note already contains a media path, taking the
notes pane down with it. A `StateField` has no viewport, so it scans the whole
document rather than the visible ranges; reading notes files are small.

The widget's `toDOM` renders `<img>`, `<video controls>` or `<audio controls>`
into a container capped at 240px tall, and asynchronously fills `src` from the
cache. `mediaMime` types the `Blob`: a `<video>` handed a typeless blob will not
play.

A module-level `Map` caches `path -> Promise<string|null>` — one object URL per
path, made once and never revoked, or null for a file that could not be read.
Cached failures matter as much as successes: a half-typed path must not be
retried on every keystroke.

The cache is built per `n4lMedia()` call, so it lives as long as one editor
rather than the window. Module-level it would be keyed on the relative path
alone while `GetSourceBytes` resolves against the *live* collection directory —
so a second collection with the same relative path would show the first
collection's file — and a cached failure would outlive its own fix.

The widget declares an `estimatedHeight` and calls `view.requestMeasure()` once
the media fires `load`/`loadedmetadata`. Appending the element schedules a
measure, but at that moment it has no intrinsic size; decoding is what grows it,
and decoding mutates no DOM, so without this the height map under-reports every
media block and the line numbers beside it drift.

One URL per path rather than one per widget: CodeMirror reuses a widget's DOM
when `eq()` matches but installs the *new* instance and drops the old one
without calling `destroy()`, so a per-widget URL is orphaned whenever the end of
a media line is edited — which is the ordinary case, since the path is usually
last on the line. The cache already pins the decoded bytes for the session.

Decorations rebuild on document change.

### `ReadingView.svelte`

One line: `extra: [n4lMedia()]` added to its existing `n4lBaseExtensions({...})`
call. Nothing else in the component changes, and `n4l-editor.js` is untouched.

## Error handling

| Case | Behaviour |
|---|---|
| File missing or unreadable | Muted `media not found: <path>` chip; the text line is unaffected |
| Path is not media | No widget, no fetch — the line renders as it does today |
| A codec the webview cannot play | The browser's own broken-media state inside the capped box; the path is still readable above it |
| Very large file | Loaded once and cached. **Memory is not bounded** — the whole file crosses the bridge as base64 and is retained as a `Blob` for the session, and the load fires merely because the line scrolled into view. A note pointing at a multi-gigabyte clip will hurt. No size guard; recorded rather than solved |

`GetSourceBytes` resolves its argument under the collection directory without a
containment check, so a note authored with `"../outside.png"` would read a file
outside the collection. This is a single-user desktop app reading files the user
already owns, and the binding already behaves this way for reading sources, so
this change does not add a boundary — it is recorded here because the new caller
passes paths from note text rather than from app-managed metadata.

## Testing

**JS unit** (`node --test`): `mediaKind` for every listed extension, uppercase
extensions, an unknown extension, an empty string, and a path with no extension.
`findMedia` for a line with one match, several matches, a quoted non-media
string, an unquoted path, and an unterminated quote — the last matters because
N4L files in this collection have been broken by stray quotes before, and the
scanner must not hang or mis-span on one.

**Manual**: its own test-plan document, covering what no harness here reaches —
that an image appears below its line, that video and audio get working controls
and seek, that a wrong path shows the chip while leaving the text editable, that
editing a path updates the render without a fetch per keystroke, and that the
main note editor still shows plain text.

## Out of scope

- Rendering media anywhere but the reading notes pane.
- Remote URLs. Only files under the collection directory.
- Editing, cropping, replacing or removing an asset from the pane.
- A picker or drag-and-drop for adding media to a note.
- PDFs, which have a source pane of their own.
- Thumbnail generation or caching to disk.
