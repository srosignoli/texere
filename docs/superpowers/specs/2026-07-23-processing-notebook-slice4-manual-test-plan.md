# Processing Notebook — Slice 4 (Sketch cell) Manual Test Plan

Prereqs: open an extract for processing.

## TC-1 Draw (mouse toggle)
1. Add a Sketch cell; turn ON the "mouse" toggle; drag on the canvas.
   - EXPECT: a smooth pen stroke appears; width varies slightly (simulated pressure).

## TC-2 Palm rejection / pen-only
1. Turn the "mouse" toggle OFF; try to draw with the mouse.
   - EXPECT: nothing draws (pen-only). (With a pen tablet, pen still draws; touch never draws.)

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
