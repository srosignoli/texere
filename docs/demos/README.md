# Scripted demo recordings

Drive the real application from a script and record the result as video.

## How it works

`wails dev` serves the app to an ordinary browser at `http://localhost:34115`
with the Go backend attached over its IPC websocket. So Chromium renders the
real app — real `.n4l` files, real BadgerDB, real FSRS scheduling — while
Playwright drives it by accessible name and records the screen.

Two consequences worth knowing:

- The recording shows **Chromium**, not the WebKitGTK webview the shipped
  binary uses. Fonts and small rendering details differ.
- Native file dialogs are owned by the Go process and are invisible to the
  browser, which is why the demo build exposes `DemoImportTextSource`. See
  `demo_seam.go`.

## Running

```bash
npm install            # in this directory, once
npm run seed           # create the fixture and point the app at it
make dev-demo          # from the repo root, in another terminal
npm run study-session  # once :34115 is up
npm run restore        # put your own collection back, remove the fixture
```

**`npm run seed` must run before the app starts.** App.svelte decides whether a
collection is open from `GetInitialSummaries()`, a snapshot taken at `startup()`.
Calling `OpenCollection` afterwards updates the Go side but leaves the UI on
whatever it booted with, and reloading the page re-reads the same snapshot — so
an app started before seeding renders only the "Open collection" button, and
that button opens a native dialog the browser cannot reach.

`seed` saves your current `collectionDir` inside the fixture and `restore` puts
it back. Nothing deletes the fixture except `restore`: an earlier version of
this harness removed it on exit, which left the app booting into a directory
that no longer existed — a state it cannot recover from without picking a
collection by hand.

`make dev-demo` compiles with `-tags demo` and reverts the regenerated
`frontend/wailsjs/` bindings on exit. If you run `wails dev` by hand instead,
revert them yourself — `TestGeneratedBindingsCarryNoDemoShims` fails if you
forget.

Output lands in `$TEXERE_DEMO_OUT` (default: a temp directory). Convert to mp4:

```bash
ffmpeg -i out/study/*.webm -c:v libx264 -pix_fmt yuv420p -crf 24 session.mp4
```

## Writing a scenario

Two rules keep long recordings reliable:

1. **Wait on conditions, never on sleeps.** Every beat below waits for real
   state — `GetStudyCounts().process > 0` — before the next begins. A slow LLM
   call or graph build then stretches the take instead of desyncing it.
2. **Address controls by accessible name.** `button[aria-label="Make card"]`,
   not `.pv-btn:nth-child(3)`. Those names are asserted by
   `frontend/src/lib/study-path-labels.test.js`, so a rename breaks a test
   rather than silently breaking a recording.

3. **Never point the harness at a real collection.** `seed` creates a throwaway
   one under `$TMPDIR` and the scenario imports documents and materialises
   cards into it. Run `restore` when you are done.
