# Media in the Read tab — manual test plan

**Design:** `2026-08-11-read-tab-media-design.md`
**Build:** `make build && ./build/bin/texere`

**This document is the verification gate for everything visual.** This
repository has no Svelte component harness, so `InlineMedia.svelte` and
`NodeCard.svelte`'s wiring have **zero automated coverage** and have never been
watched running outside a real window. Treat any failure below as a new finding.

What *is* automated: `n4l-media.test.js` covers `mediaKind` (every extension,
uppercase, unknown, empty, no extension, and extension-versus-substring so
`notes-about-png.txt` is not an image while `archive.txt.png` is) and `srcFor`
(a remote URL passed through whatever its case, a local path routed through
`/collection/`, per-segment encoding so a space, `#` or `?` survives). On the Go
side, `internal/media` covers the `/collection/` route: nested paths under both
`media/` and `assets/`, escapes and an unopened collection refused, `.mp4` and
`.webm` typed as video while `.mp3`/`.wav` stay audio, and `resolveUnder` tested
directly including `a..b/c.png`.

## Four things that are deliberate — do not report them as bugs

- **Only the Read tab renders media.** The Note tab and Study mode's reading
  notes pane are raw N4L editors and deliberately show the path as text. An
  earlier build rendered media in both; it was removed on purpose.
- **The path stays visible above the media.** A wrong path is then fixable in
  place instead of silently absent.
- **A remote asset makes a request to its host when the note is displayed**, and
  cannot work offline while a local one still can.
- **A page URL is not media.** `(vid) https://archive.org/details/SVS-3387`
  has no media extension and stays plain text. Only direct file URLs render.

## Before you start

You need a collection with real assets. Copy an image, an `.mp4` and an `.mp3`
anywhere under the collection directory — there is no required folder — and add
lines to a note. The shape below is the one most N4L is actually written in: a
subject, then ditto lines whose leading `"` means "same subject as above".

```
apple image
"  (pic) media/<your-image>.jpg

lecture
"  (has a video) assets/<your-clip>.mp4
"  (has an audio) assets/<your-recording>.mp3
```

Then open the **Read** tab (the one without line numbers, where arrow codes
appear as their descriptions).

## 1. An image renders under its arrow row

**Expect:** beneath the row reading *has a picture* (or whatever `(pic)` resolves
to) and its path, the image itself. The path text stays visible.

## 2. It works on a ditto line

The example above is already a ditto — the `"` is a continuation marker, not a
quote. **Expect:** it renders. Detection reads the parsed arrow target, so the
ditto makes no difference.

## 3. The arrow name is irrelevant

Change `(pic)` to `(illustrated by)`, or to `(banana)`. **Expect:** identical
rendering. Only the extension decides.

## 4. A target with no media extension stays plain text

```
paper (source) refs/paper.pdf
note (means) a bright thing
```

**Expect:** both render as text rows with no element and no gap beneath them.

## 5. Video renders and **seeks**

**Expect:** a video element with controls; it plays, and dragging the scrubber
seeks. Seeking comes from `http.ServeFile`'s range support — if it will not
scrub, the serving mechanism has regressed, not just video.

## 6. Audio renders and plays

**Expect:** an audio control, full width up to about 420px, that plays.

## 7. Media named in the node's own text still renders

```
here is a diagram assets/<your-image>.jpg
```

**Expect:** it renders. This path predates the arrow-target support and must
keep working — though note it also means a sentence ending in a filename will
render, which is a known consequence of extension-only detection.

## 8. A remote image renders

```
plot (pic) https://upload.wikimedia.org/wikipedia/commons/1/17/Gaussian_distribution.png
```

**Expect:** the image renders, fetched from that host when the note is shown.

## 9. Offline, remote fails while local still works

Put a local image and the remote one in the same note, disconnect the network,
and reopen the Read tab.

**Expect:** the local image still renders; the remote one does not. If the local
one *also* fails, the local route has regressed — they are served by completely
different mechanisms, and that difference is the point of this case.

## 10. A path outside the collection does not load

```
outside (pic) ../../photo.png
```

**Expect:** no image. The `/collection/` route refuses to leave the collection
directory. **[shell]** confirm the file really is there and readable, so the
refusal is containment rather than a missing file:
`ls -l "$(dirname "$(dirname "<collectionDir>")")/photo.png"`

## 11. An asset the notebook wrote under `media/` renders

Import an image through the notebook, or take an existing file from
`<collectionDir>/media/`, and reference it. **Expect:** it renders. There is no
blessed folder — `media/`, `assets/` and anything nested all work.

## 12. A filename containing a space renders

Copy an image to `<collectionDir>/my folder/a b.png` and reference it.
**Expect:** it renders. Each path segment is URL-encoded, which is what makes a
space — or a `#` or `?` — survive the trip to the server.

## 13. The Note tab and the Study notes pane show plain text — the scoping check

Open the same file in the **Note** tab, and open a reading in Study mode
(Ctrl+Shift+S → Read) whose notes reference media.

**Expect:** in both, the path renders as ordinary text with no media and no
extra vertical space. This is deliberate; both are source editors.

## What I most want to hear about

- **Case 1 first** — media appearing under the row that describes it is the
  whole feature.
- **Case 5's seeking**, which exercises the serving mechanism rather than the
  video element.
- **Cases 8 and 9 together** — remote working, and failing *differently* from
  local when offline, is what proves the two routes are genuinely separate.
- **Case 13.** Media was rendered in both those editors at one point and removed.
  If it reappears, the scoping has regressed.
