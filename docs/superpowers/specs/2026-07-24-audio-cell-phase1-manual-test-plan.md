# Audio cell — Phase 1 Manual Test Plan

Prereqs: open a collection and an extract for processing.

## TC-1 Import + playback
1. Click "+ Audio cell"; click "+ Import audio"; pick an mp3 or wav.
   - EXPECT: the file dialog is filtered to audio; after choosing, an <audio> player appears and plays the clip; the duration shows.

## TC-2 Record where unsupported (WebKitGTK/WSL)
1. On a fresh Audio cell, click "● Record".
   - EXPECT (on Linux/WebKitGTK): a clear "recording isn’t available in this webview — import an audio file instead" message; the Record button disables; Import still works. No crash.
   - EXPECT (on a Windows/WebView2 build, if available): a mic-permission prompt; after allowing, "■ Stop" with a running timer; on Stop the clip is saved and plays back.

## TC-3 Replace
1. On a loaded cell, click "Replace"; import a different file.
   - EXPECT: the cell swaps to the new clip.

## TC-4 Delete cell
1. Click ✕ on an audio cell.
   - EXPECT: the cell is removed; the notebook saves (the media file remains on disk — orphan, consistent with images).

## TC-5 Persistence across restart
1. Import an audio clip, restart the app, reopen the extract.
   - EXPECT: the audio cell re-loads and still plays.

## TC-6 Dedup + missing asset
1. Import the same file into two cells — <collectionDir>/media/ holds one file for it.
2. Delete that file from disk while a cell references it; reopen the notebook.
   - EXPECT: an "audio not found" placeholder, no crash.
