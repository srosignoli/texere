# Media Cell — Slice 1 Manual Test Plan

Prereqs: open a collection with a processing notebook; a network connection for
the search/fetch cases (TC-2, 3, 4, 6, 8, 9, 13); the ability to disconnect the
network for TC-5.

## TC-1 Buttons: `+ Scan` unchanged, `+ Media` new, both persist

1. In the notebook toolbar, confirm the old image-cell button now reads
   `+ Scan` (not `+ Image cell`).
   - EXPECT: clicking `+ Scan` opens the file picker exactly as before; import
     a scanned note image; the cell renders with its existing toolbar
     including **Transcribe**. Transcribe still promotes OCR text to a new
     N4L draft cell below it.
2. Click `+ Media`.
   - EXPECT: a new, empty Media cell appears at the end of the notebook,
     showing the acquisition panel (provider select, search box, Import
     file…, paste-URL row) — no image yet.
3. Restart the app (or close and reopen the extract) and reopen the notebook.
   - EXPECT: both the Scan cell (with its image) and the Media cell (empty,
     showing the acquisition panel) are still present in the same order.

## TC-2 Search, both providers

1. In a Media cell's acquisition panel, leave the provider on Wikimedia
   Commons (default), type `apple`, press Enter or click Search.
   - EXPECT: a thumbnail grid of results appears below; the button reads
     "Searching…" while in flight, then reverts.
2. Switch the provider dropdown to Openverse.
   - EXPECT: the panel re-searches automatically (no need to click Search
     again) and shows a different grid of `apple` results from Openverse.
3. Switch back to Commons.
   - EXPECT: results refresh to the Commons grid again.

## TC-3 Pick and persist; content-addressed dedup

1. From a search grid, click a result thumbnail.
   - EXPECT: the thumbnail dims briefly while fetching, then the cell flips
     to its filled state and renders the full picture; a `source` link
     appears at the bottom if the result had a source page.
2. Reload the notebook (restart the app or reopen the extract).
   - EXPECT: the same picture still renders (asset persisted on the cell).
3. Add a second Media cell, search the same query, and pick the identical
   picture again (e.g. the same Commons file).
   - EXPECT: it fetches and renders normally. Inspect
     `<collection>/media/` on disk (or via the file manager) — only one file
     exists for that picture's content hash; picking the same picture twice
     does not create a duplicate file.

## TC-4 Empty results

1. In a Media cell's search box, type `qwertyuiopasdfgh` and search.
   - EXPECT: a note reading something like "No results for
     "qwertyuiopasdfgh". Try another provider, or Paste URL." — not an error
     styling, not a crash.

## TC-5 Provider failure

1. Disconnect the network (turn off Wi-Fi/unplug, or block the app).
2. Search for any query.
   - EXPECT: an error message appears (e.g. a network/DNS failure string);
     the typed query remains in the search box (not cleared).
3. Reconnect the network and click Search again (query still present).
   - EXPECT: the search succeeds and results render normally.

## TC-6 Stale search

1. Type a query (e.g. `apple`) and search.
2. Before results return (or immediately after), change the query to
   something else (e.g. `banana`) and search again.
   - EXPECT: only the second query's ("banana") results are shown; the first
     query's results never flash on screen after the second search starts
     (no out-of-order overwrite).

## TC-7 Import file

1. In a Media cell's acquisition panel, click "Import file…" and choose a
   local JPEG from disk.
   - EXPECT: the cell fills with the imported picture; no `source` link is
     shown (no `sourcePage` for a local import).

## TC-8 Paste URL

1. Find a direct image URL (e.g. a `.jpg`/`.png` link) and paste it into the
   "…or paste an image URL" field; click Fetch (or press Enter).
   - EXPECT: the image is fetched and the cell fills with it.
2. Paste the URL of an HTML page (not an image) and Fetch.
   - EXPECT: an error naming an unsupported/wrong type (the sniffed MIME is
     not an image type).
3. Paste a URL that returns 404 and Fetch.
   - EXPECT: an error naming the failed status (e.g. "status 404").
4. Paste a URL known to hotlink-protect (403 on direct fetch, e.g. certain
   stock-photo or CDN pages) and Fetch.
   - EXPECT: the error text is the enriched 403 message pointing at Paste
     URL — "…this host blocks direct downloads. Open the source page and use
     Paste URL with the real image address." (This is the same message a
     blocked search-result click would show; here it demonstrates the direct
     paste path hits the same guard.)

## TC-9 Oversize

1. Paste the URL of (or otherwise fetch) an image larger than 8 MB.
   - EXPECT: an error naming the cap, e.g. "image larger than 8 MB"; nothing
     is stored and the cell stays in its acquisition state.

## TC-10 Replace

1. On a filled Media cell, click Replace.
   - EXPECT: the cell returns to the acquisition panel with the previous
     search query still filled in (not cleared).
2. Search again (optional) and then click Cancel instead of picking a new
   result.
   - EXPECT: the cell reverts to showing the existing (original) picture,
     unchanged.

## TC-11 Crop

1. On a filled Media cell, click Crop.
   - EXPECT: the view switches to the uncropped full frame with a "Drag to
     select a crop." hint; dragging on the image draws a selection rectangle.
2. Drag out a clear rectangle (more than ~2% of width/height in both
   dimensions) and release.
   - EXPECT: the crop applies immediately — the view shows the cropped
     region; a "Clear crop" button now appears in the toolbar.
3. Reload the notebook.
   - EXPECT: the crop is still applied after reload.
4. Click "Clear crop".
   - EXPECT: the full, uncropped picture reappears; "Clear crop" disappears
     from the toolbar.
5. Enter Crop mode again and perform a very small drag (a few pixels, under
   ~2% of the image in either dimension).
   - EXPECT: releasing the drag does not apply any crop (the tiny/accidental
     drag is ignored); the view stays uncropped (or keeps its prior crop).
6. Compare the Media cell's toolbar to the `+ Scan` image cell's toolbar.
   - EXPECT: the Media cell's toolbar has Replace/Crop/Clear crop/Promote to
     N4L but no **Rotate** control; the Scan cell still has Rotate.

## TC-12 Promote

1. On a filled Media cell that was populated via search, check the
   "subject" field.
   - EXPECT: it is pre-filled with the search query used to find the
     picture (e.g. searching "manzana" and picking a result pre-fills
     `subject` = "manzana").
2. Edit the subject field to a different value.
   - EXPECT: the field accepts the edit and the new value is used going
     forward (persists on reload).
3. Clear the subject field entirely (empty string).
   - EXPECT: the "Promote to N4L" button is disabled.
4. Restore a non-empty subject (e.g. "manzana") with a known asset name and
   click "Promote to N4L".
   - EXPECT: a new N4L draft cell is inserted directly below the Media cell,
     containing exactly:
     ```
     manzana
         has picture   media/<assetname>.jpg
     ```
     (subject line, then a line indented 4 spaces with `has picture`, three
     spaces, then `media/<asset>`, matching `buildMediaFact`'s output
     verbatim — no extra whitespace or trailing content beyond the final
     newline).

## TC-13 Push and graph

1. From the promoted draft cell (TC-12), push it to an N4L file using the
   existing push flow.
   - EXPECT: the file is written with the two lines from TC-12 appended
     under the subject.
2. Build the graph (Ctrl+G) and open the graph explorer; find the promoted
   subject node (e.g. "manzana").
   - EXPECT: a `has picture` edge is present in the explorer, resolving to
     its declared long name/reverse ("has picture" / "is a picture of")
     rather than being rendered as a synthesized/unknown arrow.
     *(Deferred to human verification — see report; not exercised by this
     agent, which cannot run the GUI.)*

## TC-14 No collection open

1. With no collection open (start the app fresh, or close the current
   collection if that's possible from the UI), attempt to reach a Media
   cell's acquisition panel and either pick a search result or fetch a
   pasted URL.
   - EXPECT: the action reports "no collection open" rather than crashing or
     silently failing. (`SearchMedia` itself needs no open collection, so
     search results may still render; the picking/fetch step is where the
     error surfaces.)

## TC-15 Read-only notebook

1. Open a notebook in read-only mode (e.g. via the read-only viewer path
   used for study review) that contains a Media cell.
   - EXPECT: no acquisition panel is shown for an assetless cell (just a
     "No asset." note); no per-cell toolbar (no Replace/Crop/Promote) is
     shown for a filled cell; no delete (✕) button appears anywhere in the
     cell; no `+ Scan` / `+ Media` (or any other `+ …`) add buttons appear
     at the bottom of the notebook.

## TC-16 Regression

1. Open (or build) a notebook containing at least one each of: Image
   (`+ Scan`) cell, Audio cell, Sketch cell, Graph cell, Compute cell, and
   LLM cell, alongside a new Media cell.
   - EXPECT: every existing cell type still renders and behaves as before
     (image with Transcribe, audio playback/recording, sketch canvas, graph
     query text/iso toggle, compute cell execution, LLM chat) — the addition
     of the Media cell and the `+ Scan` relabel introduce no regressions in
     any other cell type.
