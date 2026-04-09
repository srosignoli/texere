# Implementation Plan: Project Scaffold

**Branch**: `001-project-scaffold` | **Date**: 2026-04-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-project-scaffold/spec.md`

## Summary

Establish the Texere repository as a buildable, testable, lint-clean Wails v2 + Svelte
desktop application with a three-layer Go package structure reflecting the constitutional
separation of concerns (content / graph / learning state). The scaffold produces a single
binary that opens a placeholder "Texere" window, cross-compiles to Windows from Linux via
mingw-w64, passes all Go tests and linter checks, and runs fully automated on GitHub Actions.

## Technical Context

**Language/Version**: Go 1.21 (minimum enforced via `go.mod` directive)
**Primary Dependencies**: Wails v2, Svelte + Vite (frontend), golangci-lint (dev tooling)
**Storage**: N/A at scaffold stage — package stubs only (`internal/content`, `internal/graph`, `internal/learning`)
**Testing**: `go test ./...` via `make test`; scaffold includes one App smoke test
**Target Platform**: Linux amd64 (native build); Windows amd64 (cross-compiled from Linux via mingw-w64)
**Project Type**: Desktop application (Wails v2 — Go backend + OS webview frontend)
**Performance Goals**: Build < 5 min; binary launch < 5 s; test suite < 60 s (SC-001–SC-003)
**Constraints**: Zero external runtime dependencies; single binary; CGo permitted only for Wails webview bridge (justified below)
**Scale/Scope**: Single-user desktop app; scaffold stage — no data model, no user features

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Source-First & N4L Fidelity | ✅ PASS | Scaffold creates `internal/content/` stub; no N4L parsing implemented; no proprietary formats introduced |
| II. Self-Contained Binary | ✅ PASS (exception tracked) | Wails CGo webview is constitutionally approved app framework; all storage layers are pure Go; see Complexity Tracking |
| III. Separation of Concerns | ✅ PASS | `internal/content/`, `internal/graph/`, `internal/learning/` established from day one; layer rules documented in quickstart |
| IV. Pleasure-Driven Learning | N/A | Scaffold feature — no user-facing study functionality |
| V. Transparency & Portability | ✅ PASS | All build steps in Makefile + quickstart.md; no opaque scripts; binary is a plain file the user can run directly |

**Post-Phase 1 re-check**: All gates still pass. Data model confirms layer boundaries are enforced by package structure. Build contract formalises transparency.

## Project Structure

### Documentation (this feature)

```text
specs/001-project-scaffold/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── build-contract.md   # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
texere/
├── main.go                        # Wails entry point (wails.Run)
├── app.go                         # App struct with startup/domReady/shutdown stubs
├── app_test.go                    # Smoke test: TestAppCreation
├── wails.json                     # Wails project config (name: texere)
├── go.mod                         # module texere, go 1.21
├── go.sum
├── Makefile                       # build, build-windows, test, lint, dev, clean
├── .golangci.yml                  # Linter config (govet, errcheck, staticcheck, gofmt)
│
├── internal/
│   ├── content/
│   │   └── doc.go                 # Package content — Layer I: N4L file I/O
│   ├── graph/
│   │   └── doc.go                 # Package graph — Layer II: knowledge graph (BadgerDB)
│   └── learning/
│       └── doc.go                 # Package learning — Layer III: FSRS learning state (SQLite)
│
├── frontend/
│   ├── src/
│   │   ├── App.svelte             # Placeholder: "Texere" title + minimal styling
│   │   └── main.js
│   ├── package.json
│   └── vite.config.js
│
├── build/
│   ├── appicon.png
│   └── windows/
│       └── wails.exe.manifest
│
├── .github/
│   └── workflows/
│       └── ci.yml                 # Jobs: build-and-test (Linux), build-windows
│
└── docs/
    └── quickstart.md              # Developer getting-started guide
```

**Structure Decision**: Wails v2 generates `main.go`, `app.go`, `wails.json`, `frontend/`,
and `build/`. The `internal/` tree with three constitutional layers is injected on top.
`docs/quickstart.md` replaces the Wails-generated README for developer onboarding.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| CGo required (Wails webview bridge) | Wails v2 uses OS-native webview (WebKitGTK on Linux, WebView2 on Windows) which requires CGo for the Go binding | Pure-Go alternatives (Fyne, gio) do not support the full Wails/Svelte stack mandated by the constitution; a custom pure-Go webview would be a larger project than the product itself |
| mingw-w64 required for Windows cross-compilation | CGo cross-compilation from Linux to Windows requires a cross-compiler | Docker-based builds violate Constitution §II; native Windows CI runners are an option but add cost and complexity at scaffold stage; mingw-w64 is a standard `apt` package available in 1 command |
