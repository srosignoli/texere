# Processing Notebook — Slice 5 (Image cell) Manual Test Plan

Prereqs: open a collection and an extract for processing.

## TC-1 Import + display
1. Click "+ Image cell"; pick a jpg or png.
   - EXPECT: the OS file dialog appears (filtered to images); after choosing, the image displays in the cell.

## TC-2 Rotate
1. Click ↷ (rotate right) four times.
   - EXPECT: the image turns 90° each click and returns to the original after four; no distortion; the cell reshapes to the rotated aspect.

## TC-3 Crop
1. Click ⛶ Crop; drag a rectangle over the image; click ✓ Apply.
   - EXPECT: the cell now shows only the cropped region (correct area, no distortion).
2. Click ⛶ Crop again; ✕ Cancel.
   - EXPECT: no change to the current crop.

## TC-4 Reset
1. After rotating and cropping, click Reset.
   - EXPECT: the image returns to its original orientation and full frame.

## TC-5 Cancel / no collection
1. Click "+ Image cell" and cancel the dialog.
   - EXPECT: no cell is added, no error.

## TC-6 Persistence across restart
1. Import an image, rotate + crop it, then restart the app and reopen the extract.
   - EXPECT: the image re-renders with the same rotation and crop and is still editable.

## TC-7 Dedup + on-disk assets
1. Import the same file twice.
   - EXPECT: two cells, but `<collectionDir>/media/` contains a single file for that image (same sha256 name).

## TC-8 Missing asset
1. Delete the file from `<collectionDir>/media/` (or rename it) while a cell references it; reopen the notebook.
   - EXPECT: the cell shows an "image not found" placeholder, no crash.
