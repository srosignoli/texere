# texere Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-04-17

## Active Technologies
- Go 1.22 (enforced via `go.mod`) + `github.com/fsnotify/fsnotify` v1.7 (pure Go, cross-platform inotify/ReadDirectoryChanges/kqueue); standard library `hash/fnv` for node IDs; `bufio` for streaming line reads (002-n4l-parser)
- None (parser is read-only; SQLite writes happen in `internal/learning` on save events triggered by the caller) (002-n4l-parser)
- Go 1.22 (backend) + TypeScript/JavaScript, Svelte 3.49, Vite 3 (frontend) + Wails v2.12, CodeMirror 6 (`@codemirror/state`, `@codemirror/view`, `@codemirror/language`, `@codemirror/autocomplete`), `fsnotify` v1.9 (already in `go.mod`) (003-editor-core)
- `.n4l` plain-text files only; no DB layer for this feature (003-editor-core)
- Go 1.22 (backend) + JavaScript/Svelte 3.49 (frontend) + Wails v2.12, SSTorytime-KV (`github.com/srosignoli/sstorytime-kv`, published module), CodeMirror 6, fsnotify v1.9 (004-unified-search-graph)
- BadgerDB via SSTorytime-KV (`<collectionDir>/sst_data/`); read-only from search feature perspective (004-unified-search-graph)

- Go 1.21 (minimum enforced via `go.mod` directive) + Wails v2, Svelte + Vite (frontend), golangci-lint (dev tooling) (001-project-scaffold)

## Project Structure

```text
texere/
├── main.go / app.go       # Wails entry point + App struct
├── internal/content/      # Layer I: N4L file I/O
├── internal/graph/        # Layer II: knowledge graph (BadgerDB)
├── internal/learning/     # Layer III: FSRS learning state (SQLite)
├── frontend/src/          # Svelte UI
├── build/bin/             # Compiled binaries (git-ignored)
└── .github/workflows/     # GitHub Actions CI
```

## Commands

```bash
make build           # Build Linux amd64 binary → build/bin/texere
make build-windows   # Cross-compile Windows amd64 → build/bin/texere.exe
make test            # go test ./...
make lint            # golangci-lint run ./...
make dev             # Wails hot-reload dev server
make clean           # Remove build artifacts
```

## Code Style

- Go: standard `gofmt` formatting; imports grouped (stdlib / external / internal)
- No CGo except via Wails framework (webview bridge only)
- All new dependencies MUST be pure Go (no CGo) — exceptions require Complexity Tracking entry in the relevant plan

## Recent Changes
- 004-unified-search-graph: Added Go 1.22 (backend) + JavaScript/Svelte 3.49 (frontend) + Wails v2.12, SSTorytime-KV (`github.com/srosignoli/sstorytime-kv`, published module), CodeMirror 6, fsnotify v1.9
- 003-editor-core: Added Go 1.22 (backend) + TypeScript/JavaScript, Svelte 3.49, Vite 3 (frontend) + Wails v2.12, CodeMirror 6 (`@codemirror/state`, `@codemirror/view`, `@codemirror/language`, `@codemirror/autocomplete`), `fsnotify` v1.9 (already in `go.mod`)
- 002-n4l-parser: Added Go 1.22 (enforced via `go.mod`) + `github.com/fsnotify/fsnotify` v1.7 (pure Go, cross-platform inotify/ReadDirectoryChanges/kqueue); standard library `hash/fnv` for node IDs; `bufio` for streaming line reads


<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
