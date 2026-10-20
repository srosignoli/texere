# Changelog

Notable changes per release. Dates are tag dates.

Tags from v0.2.0 onward carry a `v` prefix, which Go tooling expects; `0.1.0`
predates that and is left as it is.

## v0.2.0 — unreleased

Two large features landed — the processing notebook and incremental reading —
along with themes, an LLM integration and the first packaged binaries.

### Added

- **Processing notebook.** A cell-based overlay on any note: N4L draft, LLM,
  graph, compute, shell, sketch, image, audio, video and scan cells. Cells are
  named, foldable, and keep a preview line when folded. Audio cells transcribe
  to N4L; scan cells crop on drag-release; media cells search the web for an
  image and promote it to a picture fact.
- **Incremental reading (Phase 2).** A sources sidebar with extract trees, PDF,
  text, web and LLM sources, and media and LaTeX in the Read tab.
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

- A malformed `<...>` token, a plain `(then)` arrow, and the Search button each
  crashed or blanked the app. They no longer do.
- Study wrote outside the open collection; identity keys did not survive a moved
  collection. Both are now collection-relative.
- A race in the file watcher when a file was added.
- Dialogs are drawn by the app, so the theme reaches them. Prose is no longer
  centred by default.
- A line with several arrows is a chain, as in upstream N4L: `A (r1) B (r2) C`
  now builds A→B and B→C. It used to build A→B and A→C, in the graph, in
  search and in the explorer.
- References (`$label.N`, `$PREV.N`, `$THIS.N`) are replaced by the items they
  name, as upstream N4L does. They used to stay in the graph as literal
  `$PREV.2` nodes. A label on a ditto line (`@rule " (rule) …`) now names that
  line, not its parent node.

### Internal

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
