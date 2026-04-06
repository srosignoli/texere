# Research: Project Scaffold

**Branch**: `001-project-scaffold`
**Phase**: 0 — Research
**Date**: 2026-04-06

## Decision 1: Application Framework Init Structure

**Decision**: Use `wails init -n texere -t svelte` as the starting point, then layer
the constitutional `internal/` package structure on top of the generated scaffold.

**Rationale**: Wails v2 with the Svelte template is the constitutionally mandated
framework. The generated structure provides `main.go`, `app.go`, `wails.json`,
`frontend/` (Vite + Svelte), and `build/`. Injecting `internal/content/`,
`internal/graph/`, and `internal/learning/` packages into this structure costs
nothing and enforces the three-layer separation from the first commit.

**Alternatives considered**:
- Manual project structure without `wails init`: rejected — would require re-implementing
  Wails wiring that the init command provides correctly.
- Separate `cmd/` and `pkg/` layout: rejected — overkill for a single-binary desktop
  app with no library consumers.

---

## Decision 2: Go Toolchain Version Enforcement

**Decision**: Target **Go 1.21+** and use the `go 1.21` directive in `go.mod`.

**Rationale**: Go 1.21 introduced the `toolchain` directive, making `go.mod` the
authoritative minimum-version declaration. If a developer runs an older toolchain,
Go itself emits a clear error: `note: module requires Go 1.21`. This satisfies
FR-008 (build MUST fail with a human-readable error) with zero additional scripting.
Wails v2 requires Go 1.18+; 1.21 is a safe, modern floor.

**Alternatives considered**:
- Shell script version check in Makefile: redundant with go.mod enforcement; adds
  maintenance overhead.
- Go 1.18 minimum: lower bar, but loses toolchain directive and newer stdlib features
  (slices, maps packages).

---

## Decision 3: Windows Cross-Compilation from Linux

**Decision**: Cross-compile via `CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOARCH=amd64
GOOS=windows wails build -platform windows/amd64`. Requires `mingw-w64` on the Linux
host (`apt-get install gcc-mingw-w64-x86-64`).

**Rationale**: Wails v2 requires CGo for its OS webview bridge (WebView2 on Windows).
Pure-Go cross-compilation is not possible for the Wails layer. `mingw-w64` is a
standard, widely packaged cross-compiler available in all major Linux distributions
and on GitHub Actions `ubuntu-latest` runners (`sudo apt-get install -y
gcc-mingw-w64-x86-64`). The Windows binary embeds the frontend assets; no separate
WebView2 installation is needed on Windows 10+, which ships WebView2 as a system
component.

**Alternatives considered**:
- WASM / Electron target: rejected — would violate the single-binary, zero-dependency
  constitution principle more severely.
- Docker-based cross-compilation: rejected — violates Constitution §II (no Docker
  required).
- Native Windows CI runner: possible future addition, but the mingw-w64 approach
  keeps the CI matrix simple and avoids Windows-specific runner costs.

---

## Decision 4: Linting Configuration

**Decision**: `golangci-lint v1.57+` with a `.golangci.yml` enabling `govet`,
`errcheck`, `staticcheck`, `gofmt`, `goimports`, `unused`.

**Rationale**: `golangci-lint` is the de facto standard for Go projects. A curated
set of linters catches common errors (unchecked errors, formatting drift) without
excessive noise. The `frontend/` directory is excluded from Go lint by default
(it contains only JS/Svelte/HTML).

**Alternatives considered**:
- `go vet` alone: insufficient — misses many classes of bugs caught by staticcheck.
- Full golangci-lint with all linters enabled: too noisy for a new codebase; can be
  tightened incrementally.

---

## Decision 5: GitHub Actions CI Workflow

**Decision**: Single workflow file `.github/workflows/ci.yml` with two jobs:
`build-linux` (native Go build + tests + lint) and `build-windows` (cross-compile
only, no tests — Windows test execution requires a Windows runner).

**Rationale**: Separating the jobs allows the lint and test job to fail fast without
waiting for the cross-compilation job. Windows test execution is deferred because
running Windows binaries on a Linux runner requires WINE, which is fragile in CI.
SC-006 (CI within 10 minutes) is achievable with parallel jobs.

**Alternatives considered**:
- Single job: slower (serial); mixes concerns.
- Full matrix with Windows runner: higher cost, slower for what is a build-validity
  check at scaffold stage.

---

## Decision 6: Build Interface (Makefile)

**Decision**: `Makefile` at repository root with targets: `build` (Linux amd64),
`build-windows` (Windows amd64 cross-compile), `test`, `lint`, `dev` (Wails dev
server), `clean`.

**Rationale**: A Makefile provides a single, discoverable interface for all
developer commands regardless of CI platform. It satisfies FR-001 (one command to
build), FR-004 (one command to test), FR-005 (one command to lint), and documents
the cross-compilation steps (FR-003). Wails also provides `wails build` and
`wails dev` directly, but the Makefile wraps these with the correct flags.

**Alternatives considered**:
- `go generate` / `task` (Taskfile): no strong reason to add a non-standard tool
  over a universal Makefile for this scope.
- Shell scripts: less discoverable, less composable than Makefile targets.
