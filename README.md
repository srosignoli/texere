# Texere

A desktop app for N4L plain-text notes: an editor, a knowledge graph,
incremental reading and spaced repetition in one window.

Your notes stay plain text on disk. The graph and the scheduling state are
derived from them and can be rebuilt at any time — nothing you write is locked
inside a database.

Built on [SSTorytime-KV](https://github.com/srosignoli/sstorytime-kv) with
[Bleve](https://blevesearch.com) for full-text search, wrapped in
[Wails](https://wails.io) v2. Linux and Windows.

## Status

0.x, and pre-1.0 deliberately: the on-disk formats — collection layout, the
BadgerDB graph store, the SQLite learning database — may still change between
minor releases, and there is no migration path yet. Keep backups of collections
you care about.

[ROADMAP.md](ROADMAP.md) is what's next. [CHANGELOG.md](CHANGELOG.md) is what
has shipped.

## What you do with it

**Write.** An N4L editor with arrow autocomplete, a compliance linter, and a
radius-1 minimap of the node under the caret. Media and LaTeX render inline.

**Ask.** Build the graph (Ctrl+G) and query it — by word, chapter, context tag,
path between two nodes, or sequence. With an LLM profile configured, an ask box
turns a plain question into a query you can read and edit before it runs. The
full syntax is in [docs/query-language.md](docs/query-language.md).

**Explore.** A 2.5D isometric view of the graph, with in-place node editing that
lands back on the right line of the right file.

**Study.** Three loops over one collection — read sources and extract from them,
process extracts into cards, recall the cards on an FSRS schedule. Statistics
cover quality, habit, workload and the read/process/recall pipeline.

**Process.** A notebook overlay attached to any note, with cells for drafting
N4L, sketching, running a shell command, computing, querying the graph, and
importing images, audio, video or scans — including speech-to-text.

## Install

Download the archive for your platform from
[Releases](https://github.com/srosignoli/texere/releases) and check it against
`checksums.txt`.

- **Linux** — `texere_<version>_linux_amd64.tar.gz`. Needs
  `libwebkit2gtk-4.1-0` and `libgtk-3-0`.
- **Windows** — `texere_<version>_windows_amd64.zip`. Single executable.

## Build from source

```bash
make deps        # Linux system libraries
make wails-cli   # Wails CLI, built with the toolchain go.mod asks for
make build       # → build/bin/texere
make test        # go test ./...
```

Full setup, cross-compilation and the test matrix are in
[docs/quickstart.md](docs/quickstart.md).

## Documentation

| | |
|---|---|
| [docs/quickstart.md](docs/quickstart.md) | Developer setup, build, test, project layout |
| [docs/query-language.md](docs/query-language.md) | Query syntax and the ask box |
| [docs/demos/README.md](docs/demos/README.md) | Scripted demo recordings |
| [docs/Texere_Specification_v1.3.md](docs/Texere_Specification_v1.3.md) | The long-form design document. Aspirational — much of it is unbuilt, and ROADMAP.md says which parts are queued |
| [specs/](specs/) | Per-feature specs, plans and tasks |

## Licence

Not yet chosen. Until one is added, no permissions are granted beyond reading
the source.
