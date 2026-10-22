# Processing Notebook — Slice 4 (Sketch cell) Manual Test Plan

Prereqs: open an extract for processing.

> **Platform note:** On Linux/WebKitGTK a pen tablet (e.g. XP-Pen) is reported
> to the webview as `pointerType: "mouse"` with no real pressure — it is
> indistinguishable from a mouse, and palm rejection / hardware pressure are not
> available there. The "mouse/pen" toggle therefore defaults ON so the mouse and
> the tablet both draw out of the box. True `pen`/`touch` pointer types (and thus
> pen-only palm rejection) only appear on platforms that report them, e.g.
> Windows/WebView2 touchscreens.

## TC-1 Draw (default)
1. Add a Sketch cell (the "mouse/pen" toggle is ON by default); drag on the canvas with the mouse or the pen tablet.
   - EXPECT: a smooth stroke appears; width varies slightly (simulated pressure).

## TC-2 Pen-only toggle / palm rejection
1. Turn the "mouse/pen" toggle OFF; try to draw with the mouse (or, on Linux, the tablet).
   - EXPECT: nothing draws — only a real `pen` pointer draws in this mode. On Linux/WebKitGTK the tablet reports as `mouse`, so it will NOT draw with the toggle off (expected); on Windows/WebView2 a pen still draws and touch never draws.

## TC-3 Tools + colors
1. Pick Highlight + amber; draw over an existing pen stroke.
   - EXPECT: a translucent amber highlight that darkens where it overlaps (multiply).
2. Switch pen colors and draw.
   - EXPECT: strokes render in the selected color; the size slider changes width.

## TC-4 Eraser (stroke-erase)
1. Select Erase; drag across a stroke.
   - EXPECT: the whole stroke it touches disappears (not a partial rub-out).

## TC-5 Undo / Redo / Clear
1. Draw 3 strokes; Undo twice; Redo once.
   - EXPECT: strokes remove/re-add in order.
2. Erase a stroke, then Undo.
   - EXPECT: the erased stroke returns.
3. Clear, then Undo.
   - EXPECT: all strokes return.

## TC-6 Resize
1. Drag the bottom edge of the canvas taller, then shorter.
   - EXPECT: the canvas resizes; existing strokes stay put.

## TC-7 Persistence
1. Draw a sketch, restart the app, reopen the extract.
   - EXPECT: the sketch re-renders at the same height and is still editable (draw more, undo works).

## TC-8 Pressure (tablet, optional)
1. On a pen tablet (Windows Ink / WebView2), vary pen pressure.
   - EXPECT: stroke width tracks pressure. On Linux/WebKitGTK, width is velocity-simulated but strokes always draw.
