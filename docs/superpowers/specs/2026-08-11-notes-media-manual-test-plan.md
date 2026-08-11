# Inline notes media — manual test plan

**Design:** `2026-08-11-notes-media-design.md`
**Build:** `make dev`

**This document is the verification gate for the interactive parts.** This
repository has **no CodeMirror plugin / Svelte component test harness**, so
`n4l-media-view.js` — the `ViewPlugin`, the `MediaWidget`, and the blob cache —
has never been watched running in a real webview. Treat any failure below as a
new finding.

What *is* automated: `n4l-media.test.js` covers `mediaKind` (every supported
extension, case-insensitive, an unknown extension, no extension, extension vs.
full filename), `mediaMime` (kind → MIME type, feeding the `Blob` a real
content type so `<video>`/`<audio>` will actually play it), and `findMedia`
(one match, several matches in order, a quoted non-media string ignored, an
unquoted path ignored, an unterminated quote surviving without hanging). None
of that reaches the DOM: whether an `<img>`/`<video>`/`<audio>` actually
appears below a line, whether a video actually seeks, whether a chip actually
reads `media not found: …`, and whether the main editor is actually untouched
are unverified by anything automated. Cases 3 and 12 below are the two with no
substitute.

## Why this feature exists

Notes taken while reading routinely reference assets on disk — a diagram, a
clip, a recording — and until now the note showed only the path, exactly as
typed. This plan exists to confirm that a path to an image, video or audio
file now shows the asset itself, not just its name.

## Things that are deliberate — do not report them as bugs

- **A remote asset makes a request to its host when the note opens.** A
  `http(s)://` URL now renders, which means opening a note fetches from that
  host. That is the intended behaviour, not a leak.
- **Remote media cannot work offline; local media can.** With no network a
  remote image shows the `media not found` chip while a local one beside it
  still renders. That difference is the clearest sign the two routes are
  genuinely different.
- **There is no folder rule.** Any path under the collection works — `media/`,
  `assets/`, anything nested. Earlier drafts of this document used `assets/` in
  every example, which made it look like a rule. It never was.
- **Caching is the browser's, not ours.** Nothing is held in JS. A file created
  after you saw its chip appears on the next render; a file whose contents you
  replace may serve from the HTTP cache until reload.
- **Quoted prose that happens to end in a filename gets a chip.** `"see the
  diagram in overview.png"` is detected as an image, fails to load, and shows
  `media not found`. A consequence of extension-only detection.
- **PDFs are deliberately not treated as media.** `"refs/paper.pdf"` renders
  as plain text with no widget, on purpose — PDFs have a source pane of their
  own elsewhere in the app.
- **There is no way to add, remove or replace an asset from this pane.** It
  renders whatever the note text already references. Adding media means
  typing (or pasting) a path to a file that is already on disk.
- **A file replaced on disk at the same path may keep showing the old
  bytes for the life of the window.** Loads are cached per path and the cache
  is never evicted, so if you swap the file under an already-rendered path
  without restarting the app, the widget can keep showing what it first
  loaded. This is a known, deferred limitation, not something to file fresh —
  though if you hit it, it is worth a one-line note in your report just to
  confirm it's the same shape.

## Before you start

You need a collection open, a reading in it (any medium — text, PDF or web;
the notes pane exists regardless), and **real image, video and audio files
copied under the collection directory**. Paths written in a notes line are
resolved by `GetSourceBytes` **relative to the collection directory itself**
— not relative to the notes file, and not restricted to `sources/`. Put them
somewhere unambiguous, e.g. a new `assets/` folder at the collection root:

```bash
mkdir -p <collectionDir>/assets
cp ~/Pictures/some-photo.png <collectionDir>/assets/diagram.png
cp ~/Videos/some-clip.mp4   <collectionDir>/assets/clip.mp4
cp ~/Music/some-track.mp3   <collectionDir>/assets/clip.mp3
```

Use a video that actually runs a few seconds — case 3 needs somewhere to
seek to — and audio/video that are not silent, so "plays" is actually
checkable by ear.

Open the reading in Study (Ctrl+Shift+S), and in its **notes pane** (the
CodeMirror editor on the right of the reading source), paste this block.
Each line is deliberately built to exercise one case below; the case numbers
in the comments are just for your reference and can be deleted before you're
done:

```
 A diagram of the render pipeline (illustrated by) "assets/diagram.png"
 The same diagram, a different arrow name (has an image) "assets/diagram.png"
 A recording of the walkthrough (has a video) "assets/clip.mp4"
 Background audio from the session (has an audio) "assets/clip.mp3"
 A reference I have not scanned yet (source) "refs/paper.pdf"
 A phrase worth remembering (note) "a bright thing"
 A typo I have not fixed yet (illustrated by) "assets/diagrm.png"
 Two assets referenced from one line (shows) "assets/diagram.png" (and) "assets/clip.mp4"
```

(Case 10 needs the image line duplicated another dozen or so times below the
block — see that case for why.)

## 1. An image renders below its line — the text line is untouched above it

Look at the first line of the pasted block, `... (illustrated by)
"assets/diagram.png"`.

**Expect:** the image appears in a block **below** that line. The text line
itself is still exactly where it was, above the image, reading the same as
before — the path was not hidden, replaced, or dimmed.

## 2. The text stays editable — click into the path and type

Click into the path text on the diagram line (e.g. right after `diagram`) and
type a character, then delete it.

**Expect:** the line behaves like any other CodeMirror line — caret lands
where you clicked, typing inserts normally, syntax highlighting is unaffected.
Nothing about the presence of the image below changes how the line above it
edits.

## 3. Video renders, plays, and **seeks** — no automated substitute

Seeking now comes from `http.ServeFile`'s range support rather than a blob URL,
so this is a regression check on the whole loading mechanism, not just on video.


Look at the `... (has a video) "assets/clip.mp4"` line. Play the video, then
**drag the scrubber** to a point partway through — not just click play/pause.

**Expect:** the video plays with visible transport controls, and dragging the
scrubber actually jumps playback to that point — not stuck at 0:00, not
silently snapping back to where it was. **This is the one property that
matters most in this plan.** Video is loaded as a `Blob` and handed an object
URL specifically so the browser can issue HTTP range requests against it;
that's the entire justification in the design for not using a base64 data
URL, which cannot be range-requested and would leave the scrubber inert. If
seeking does not work, that decision has regressed and it is a priority
finding.

## 4. Audio renders with controls and plays

Look at the `... (has an audio) "assets/clip.mp3"` line. Press play.

**Expect:** an audio control spanning most of the notes pane's width (not a
tiny default-sized widget), and sound actually plays.

## 5. The arrow name is irrelevant — only the extension matters

Compare the first two lines of the block: `(illustrated by) "assets/diagram.png"`
and `(has an image) "assets/diagram.png"`.

**Expect:** both render the identical image widget below their respective
lines. Nothing about which arrow was used changes whether or how the media
renders — detection is purely by the target's file extension.

## 6. A non-media target renders as plain text — no widget, no gap

Look at the `(source) "refs/paper.pdf"` line and the `(note) "a bright
thing"` line.

**Expect:** both render exactly as plain N4L text, same as any other line in
the file. No widget, no blank space reserved below either line, no failed
network request, no `media not found` chip — a PDF path and a non-path string
are simply not media as far as this feature is concerned, by design.

## 7. A wrong path shows the miss chip — the text above stays editable

Look at the `(illustrated by) "assets/diagrm.png"` line (the deliberate typo
— `diagrm`, not `diagram`).

**Expect:** below that line, a small **muted, italic** chip reading exactly
`media not found: assets/diagrm.png`. The text line above it is completely
untouched — click into `diagrm`, insert the missing `a`, and confirm the line
still edits normally. Once the path resolves, the chip should be replaced by
the real image on the next redraw (moving the caret or making any edit is
enough to trigger one).

## 8. Typing a path character by character does not flicker or stall

Go to the end of the block and start a new line: type `A test line (has an
image) "assets/d` slowly, one keystroke at a time, then keep going to finish
`iagram.png"`.

**Expect:** no flicker, no visible stall or lag as each character lands, and
no widget appears (correctly — there's no closing quote and no valid
extension yet) until the path is complete and the quote is closed. Failures
along the way (an incomplete or wrong path) are cached, so a half-typed path
is attempted once rather than fetched again on every keystroke — you should
not see repeated network/IPC-style hitching as you type.

## 9. Several media referenced from one line each render

Look at the last line of the block: `(shows) "assets/diagram.png" (and)
"assets/clip.mp4"`.

**Expect:** **two** widgets stack below that single line — the image, then
the video — both fully functional (the video still plays and seeks). One line
referencing multiple assets is not a special case; each quoted media path
gets its own widget.

## 10. A long note with many assets still scrolls smoothly, and no image dominates the pane

Duplicate the diagram line (`... "assets/diagram.png"`) another dozen-plus
times below the existing block, so the note has 15+ rendered images stacked
down the pane. Scroll from top to bottom and back.

**Expect:** scrolling stays smooth — no long freeze, no visible tearing, no
runaway lag that gets worse the more you scroll. No single image exceeds
roughly a third of the pane's visible height (the shipped cap is 240px tall,
`max-width: 100%`, so a note with many assets stays navigable rather than
turning into one giant picture). If you can reach devtools (Wails dev mode),
scroll quickly past images that haven't finished loading yet and back before
they settle — this exercises a widget being destroyed mid-load; nothing
should throw into the console.

## 11. The main note editor and the notebook overlay still show plain text — the scoping check

Open the **same reading notes file** in the main editor: in the top band's
Files/Chapters column, find the entry for `reading/<the reading's
title>.n4l` and open it into the **Note** tab (the main N4L editor at the
bottom of the workbench — the one you use for source `.n4l` files generally,
not the Study/reading surface). Separately, open the notebook overlay (click
a node's notebook-dot indicator in the editor gutter) on any file.

**Expect:** in both places, every line you pasted — including the media
paths — renders as **plain text only**. No image, video or audio widget
anywhere. This is by design: the feature is wired in only via `extra:
[n4lMedia()]` inside `ReadingView.svelte`'s notes editor, and the main editor
and notebook overlay share the same base extensions without it. (Optional
dev-side cross-check, not required to complete the case: `grep -rn
"n4lMedia" frontend/src/` should show it only in `n4l-media-view.js` — its
own definition — and in `ReadingView.svelte`.)

## 12. A playing video does not restart when you type elsewhere in the note

Start the `assets/clip.mp4` video playing and let it run a few seconds so
its current time is clearly past 0:00. Without touching the video itself,
click into a **different** line in the notes pane (e.g. the phrase line) and
type a character, then delete it.

**Expect:** the video **keeps playing from where it was** — it does not
restart from 0:00, pause, or flicker/reload. This is what the widget's `eq()`
override exists to prevent: without it, CodeMirror would treat every
document change as a reason to tear down and rebuild the widget, which would
recreate the `<video>` element (and restart playback) on every keystroke
anywhere in the note, not just on the video's own line. If the video jumps
back to the start when you type elsewhere, that protection has regressed and
it is a priority finding.

## 13. An asset the notebook wrote under `media/` renders

The case that prompted this change. Use the notebook to import an image, or
find an existing file under `<collectionDir>/media/`, and reference it from a
reading's notes file:

```
diagram (has an image) "media/<the-file-name>"
```

**Expect:** it renders, exactly like one under `assets/`. There is no blessed
folder.

## 14. A remote image renders

```
plot (has an image) "https://upload.wikimedia.org/wikipedia/commons/1/17/Gaussian_distribution.png"
```

**Expect:** the image renders. It is fetched from that host when the note opens.

## 15. Offline, remote fails while local still works

Put a local image and the remote one from case 14 in the same note. Disconnect
the network and reopen the reading.

**Expect:** the local image still renders; the remote one shows
`media not found: https://…`. If the local one *also* fails, the local route has
regressed — they are served by completely different mechanisms.

## 16. A path escaping the collection now shows the chip

```
outside (has an image) "../../photo.png"
```

**Expect:** `media not found: ../../photo.png`. **This is a deliberate change**
— it used to load, because the old loader cleaned `..` and read the file. The
`/collection/` route refuses to leave the collection directory.

## 17. A filename with a space renders

Copy an image to `<collectionDir>/my folder/a b.png` and reference it:

```
spaced (has an image) "my folder/a b.png"
```

**Expect:** it renders. Each path segment is URL-encoded, which is what makes a
space — or a `#` or `?` in a filename — survive the trip to the server.

## What I most want to hear about

- **Case 13 first.** An asset the notebook wrote under `media/` rendering in a
  reading note is the thing that prompted this change, and the thing I got
  wrong by documenting only `assets/` examples.
- **Cases 14 and 15 together.** Remote media working, and failing *differently*
  from local media when offline, is what proves the two routes are separate
  rather than one path that happens to work.
- **Case 3's seeking.** It now depends on `http.ServeFile`'s range support. If a
  video will not scrub, the whole loading mechanism has regressed, not just
  video.
- **Case 16.** It used to load and now does not. If you were relying on
  referencing files outside the collection, say so — that is a decision worth
  revisiting rather than a bug.
