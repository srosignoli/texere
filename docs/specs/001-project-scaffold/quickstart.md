# Quickstart: Texere Development Setup

**Feature**: Project Scaffold (`001-project-scaffold`)
**Updated**: 2026-04-06

This document gets a new contributor from zero to a running Texere build on Linux.

---

## Prerequisites

Install the following tools before proceeding:

| Tool | Minimum version | Install |
|---|---|---|
| Go | 1.21 | https://go.dev/dl/ |
| Node.js | 18 LTS | https://nodejs.org/ |
| Wails CLI | v2 | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |

Verify:

```bash
go version         # must show go1.21 or higher
node --version     # must show v18 or higher
wails version      # must show v2.x
```

For Windows cross-compilation (optional on developer machines; required in CI):

```bash
sudo apt-get install -y gcc-mingw-w64-x86-64
```

---

## Clone and Build

```bash
git clone https://github.com/<owner>/texere.git
cd texere
make build
```

The Linux binary is produced at `build/bin/texere`. Launch it:

```bash
./build/bin/texere
```

A window opens displaying the Texere placeholder screen.

---

## Cross-Compile for Windows

```bash
make build-windows
```

Produces `build/bin/texere.exe`. Requires `gcc-mingw-w64-x86-64` (see Prerequisites).

---

## Run Tests

```bash
make test
```

Runs all Go tests and reports pass/fail counts. On an unmodified scaffold, all tests pass.

---

## Run Linter

```bash
make lint
```

Reports zero violations on an unmodified scaffold. Each violation includes the file
path, line number, rule name, and a description.

---

## Start Development Server

```bash
make dev
```

Opens the application with Wails hot-reload. Frontend changes (Svelte) rebuild
automatically; Go backend changes trigger a full restart.

---

## Project Structure

The repository is organised into three constitutional layers plus the UI and build
infrastructure:

```
texere/
├── main.go                    # Application entry point
├── app.go                     # Wails App struct + lifecycle
├── wails.json                 # Wails project configuration
├── go.mod                     # Go module (minimum Go 1.21)
├── Makefile                   # Developer build interface
│
├── internal/
│   ├── content/               # Layer I: N4L file reading/writing
│   │   └── doc.go             # (scaffold: package declaration only)
│   ├── graph/                 # Layer II: knowledge graph (BadgerDB)
│   │   └── doc.go             # (scaffold: package declaration only)
│   └── learning/              # Layer III: FSRS learning state (SQLite)
│       └── doc.go             # (scaffold: package declaration only)
│
├── frontend/                  # UI layer (Svelte + Vite)
│   └── src/
│       └── App.svelte         # Placeholder "Texere" window
│
├── build/                     # Wails build output and icons
│   └── bin/                   # Compiled binaries (git-ignored)
│
├── .github/
│   └── workflows/
│       └── ci.yml             # GitHub Actions CI
│
└── docs/
    └── quickstart.md          # This file
```

### Where to add new features

| You need to... | Add code in... |
|---|---|
| Read or write `.n4l` files | `internal/content/` |
| Store or query the knowledge graph | `internal/graph/` |
| Read or write FSRS scheduling state | `internal/learning/` |
| Add a UI component or view | `frontend/src/` |
| Expose a backend function to the UI | `app.go` (add a method to `App`) |

### Layer rules (from the [Texere Constitution](../../principles.md))

- Only `internal/content/` may read or write `.n4l` files.
- `internal/graph/` and `internal/learning/` are derived caches — they MUST be
  rebuildable from source. Never store canonical knowledge in them.
- `internal/graph/` and `internal/learning/` MUST NOT import each other.
- All backend → frontend communication goes through exported methods on `App`.

---

## Common Errors

**`go: module requires Go 1.21`**
Your Go toolchain is below the minimum version. Install Go 1.21+ from https://go.dev/dl/.

**`wails: command not found`**
Install Wails: `go install github.com/wailsapp/wails/v2/cmd/wails@latest` and ensure
`$(go env GOPATH)/bin` is on your `PATH`.

**`x86_64-w64-mingw32-gcc: command not found`** (during `make build-windows`)
Install mingw-w64: `sudo apt-get install -y gcc-mingw-w64-x86-64`.

**`npm install` errors during build**
Wails runs `npm install` in `frontend/` automatically on first build. Ensure Node.js
18+ is installed and `npm` is on your PATH.
