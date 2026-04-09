# texere Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-04-06

## Active Technologies

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

- 001-project-scaffold: Added Go 1.21 (minimum enforced via `go.mod` directive) + Wails v2, Svelte + Vite (frontend), golangci-lint (dev tooling)

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
