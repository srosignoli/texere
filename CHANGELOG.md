# Changelog

Notable changes per release. Dates are tag dates.

Tags from v0.2.0 onward carry a `v` prefix, which Go tooling expects; `0.1.0`
predates that and is left as it is.

## v0.2.0 — unreleased

Two large features landed — the processing notebook and incremental reading —
along with themes, an LLM integration and the first packaged binaries.

Release candidates: `v0.2.0-rc.1` (2026-10-14), `v0.2.0-rc.2` (2026-10-22).
rc.2 adds the mixed study queue, a round of fixes to reading and the notebook,
and an architecture refactor that moves policy out of the views and bindings
into tested modules (see Internal).

### Added

- **Processing notebook.** A cell-based overlay on any note: N4L draft, LLM,
  graph, compute, shell, sketch, image, audio, video and scan cells. Cells are
  named, foldable, and keep a preview line when folded. Audio cells transcribe
  to N4L; scan cells crop on drag-release; media cells search the web for an
  image and promote it to a picture fact.
- **Incremental reading (Phase 2).** A sources sidebar with extract trees, PDF,
  text, web and LLM sources, and media and LaTeX in the Read tab.
- **Next mixes the queue.** Sources, extracts and cards take turns by a mix you
  set per collection in Stats (default 20 / 15 / 65), so a reading backlog no
  longer holds back reviews that are due.
- **Study.** Statistics with readable charts, media and math on flashcards, and
  card provenance back to the line that produced it.
- **Ask the graph in plain language.** An LLM turns a question into a query,
  which lands editable in the query bar. Five node-column filters.
- **N4L compliance linter**, and declared `text2N4L` arrows.
- **Themes** — Blue, Dark and Light, covering the editor. SVG icons replace
  emoji that some systems could not render. A keyboard shortcut panel, and LLM
  configuration moved into the settings menu.
- **Releases.** Tagging `v*` now builds Linux and Windows binaries and publishes
  them with checksums.

### Fixed

- Process mode's notebook dots vanished on any edit and did nothing when
  clicked. Every editor's dot now means the node has a notebook, and clicking it
  moves to that node.
- Peeking at an extract's passage pushed its source's next reading out, as if
  you had read it. A peek now records nothing; Done during a peek still counts.
- Editing a source's notes while reading reset its `# @progress` marker to where
  the sitting began. A failed notes save no longer closes the reader silently:
  it stays open with the error, and closing again offers to give the notes up.
- A malformed `<...>` token, a plain `(then)` arrow, and the Search button each
  crashed or blanked the app. They no longer do.
- Study wrote outside the open collection; identity keys did not survive a moved
  collection. Both are now collection-relative.
- A race in the file watcher when a file was added.
- Dialogs are drawn by the app, so the theme reaches them. Prose is no longer
  centred by default.
- A failed source import left its notes file in `reading/`, where the graph and
  the linter took it for a stray note. A failed import now leaves nothing behind.
- A notebook whose autosave failed said nothing, and its edits existed only in
  memory. It now shows "Notebook not saved: …" until a save succeeds.
- The LLM cell's "Attach context" list named cells differently from their
  headers ("draft", "image"). It now uses the header names ("N4L draft", "Scan").
- In a collection copied from elsewhere, an extract or card whose stored path
  still pointed at the original showed up with that raw path. It is now left
  out of the lists and logged; nothing is ever read or written outside the open
  collection.
- A line with several arrows is a chain, as in upstream N4L: `A (r1) B (r2) C`
  now builds A→B and B→C. It used to build A→B and A→C, in the graph, in
  search and in the explorer.
- References (`$label.N`, `$PREV.N`, `$THIS.N`) are replaced by the items they
  name, as upstream N4L does. They used to stay in the graph as literal
  `$PREV.2` nodes. A label on a ditto line (`@rule " (rule) …`) now names that
  line, not its parent node.

### Internal

- **Architecture refactor.** Policy that lived in Svelte views and Wails bindings
  moved into modules with tests at their interface:
  - **Sitting** (`internal/sitting`): ending a reading (the order of its writes,
    the peek rule, idempotence) in one backend module. ReadingView reports what
    happened instead of re-implementing it three times.
  - **Study Queue** (`internal/learning/queue.go`): Next's scheduling, out of
    `StudySurface.svelte`. Whether a source is due is decided in Go only.
  - **The learning store speaks absolute paths**: every read returns them and
    every write takes them, with one policy for rows outside the collection,
    replacing about twenty per-binding conversions.
  - **`source.Register`**: the one way a source enters a collection, behind the
    text, PDF, web and LLM imports.
  - **Frontend logic in `frontend/src/lib/`**, testable without Svelte: the
    notebook cell schema, the notebook autosave, caret→notebook resolution, and
    the reader's end and switch rules.
  - Notebook dots: the text-keyed push-provenance system was removed; every dot
    means "this node has a notebook".
  - The media bindings return a typed `AssetRef`.
- **learning.db schema v10.** v9 drops `notebook_links` (push provenance that
  nothing read any more) and v10 adds `study_settings` (the mix). Collections
  upgrade on open. A collection opened by this version should not be opened by
  an older one.
- **Docs.** Specs live in `docs/specs/`, plans in `docs/plans/`, and the
  project's founding principles in `docs/principles.md`. The spec-kit scaffolding
  they came from is removed, and `CONTEXT.md` gains the study and notebook
  vocabulary (Sitting, Turn, Mix, Study Queue, Notebook, Push, …).
- The pre-commit hook's demo-shim check silently passed whenever more than one
  generated binding was staged. It now checks them all.

- A selector contract covering every surface, plus a recording kit that drives
  the real app under Playwright for demo videos. A hook keeps the demo seam out
  of commits, and release builds ship neither the seam nor devtools.
- `make lint-frontend` fails the build on any Svelte a11y warning.
- `make wails-cli` installs the Wails CLI with the toolchain `go.mod` asks for.
  A CLI built by an older Go type-checks this module in-process and logs
  `package requires newer Go version` on every build.

## 0.1.0 — 2026-06-19

The isometric 3D graph explorer with in-place node editing (pin → Enter edit
card), unified search, Bleve full-text search, the N4L parser and editor, and
the SSTorytime-KV knowledge graph.
