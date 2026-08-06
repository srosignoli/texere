# Quickstart: Texere Development Setup

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
| System WebKit libs | — | `sudo apt-get install -y libwebkit2gtk-4.1-dev libgtk-3-dev pkg-config build-essential` |

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
├── app_test.go                # Scaffold smoke test
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

### Layer rules (from the [Texere Constitution](.specify/memory/constitution.md))

- Only `internal/content/` may read or write `.n4l` files.
- `internal/graph/` and `internal/learning/` are derived caches — they MUST be
  rebuildable from source. Never store canonical knowledge in them.
- `internal/graph/` and `internal/learning/` MUST NOT import each other.
- All backend → frontend communication goes through exported methods on `App`.

---

## Common Errors

**`go: module requires Go 1.21`**
Your Go toolchain is below the minimum version. Install Go 1.21+ from https://go.dev/dl/.

**`internal error: error "package requires newer Go version go1.25" ... without position`**
Printed by `wails build` during "Generating bindings", then the build succeeds.
Harmless, and the bindings are complete — but worth knowing why it appears.

The Wails CLI type-checks the project with the `go/types` bundled into its own
binary. If that binary was compiled with an older Go than `go.mod` requires, it
cannot parse the `go 1.25.0` directive and reports this. Check with:

```bash
go version -m "$(go env GOPATH)/bin/wails" | head -1
```

If that prints a version below the one in `go.mod`, rebuild the CLI with your
current toolchain — same Wails version, newer compiler:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
```

To confirm bindings really were generated in full, compare the exported `App`
methods against the generated file; the two counts must match:

```bash
grep -rhoE "^func \(a \*App\) [A-Z][A-Za-z0-9]*" *.go | sed 's/.*) //' | sort -u | wc -l
grep -oE "^export function [A-Z][A-Za-z0-9]*" frontend/wailsjs/go/main/App.js | sort -u | wc -l
```

**`wails: command not found`**
Install Wails: `go install github.com/wailsapp/wails/v2/cmd/wails@latest` and ensure
`$(go env GOPATH)/bin` is on your `PATH`.

**`pkg-config: webkit2gtk-4.0 not found`** (during `make build`)
Install system WebKit libraries: `sudo apt-get install -y libwebkit2gtk-4.1-dev libgtk-3-dev pkg-config build-essential`.
Note: Wails uses the `webkit2_41` build tag to target the 4.1 API — this is already set in the Makefile.

**`x86_64-w64-mingw32-gcc: command not found`** (during `make build-windows`)
Install mingw-w64: `sudo apt-get install -y gcc-mingw-w64-x86-64`.

**`npm install` errors during build**
Wails runs `npm install` in `frontend/` automatically on first build. Ensure Node.js
18+ is installed and `npm` is on your PATH.

---

## Manual Test Matrix: Graph Explorer Edit Card

This section documents manual testing of the graph explorer's edit card feature (Ctrl+G to open the 3D explorer, click a node to pin it, press Enter to open an edit card on that node's source line, edit and save).

### Edit Card: Normal Flow

**Steps:**
1. Open a Texere collection with at least one `.n4l` file
2. Press Ctrl+G to open the 3D graph explorer
3. Click a node in the graph to pin it
4. Press Enter
5. An edit card opens, centered over the graph, showing the pinned node's source line
6. Edit the text in the card
7. Press Ctrl+S to save
8. Press Esc to close the card

**Expected result:** The card closes, the graph rebuilds and re-anchors, and the pinned node remains pinned at its new position.

### Edit Card: No Change, Instant Return

**Steps:**
1. Open a Texere collection
2. Press Ctrl+G, click a node to pin it, press Enter
3. The edit card opens
4. Without editing, press Ctrl+S

**Expected result:** The card closes immediately without triggering a graph rebuild. The explorer remains in the same view with the node still pinned.

### Edit Card: Pinned Node Text Changed

**Steps:**
1. Open a Texere collection
2. Press Ctrl+G, click a node to pin it, press Enter
3. The edit card opens on that node's source line
4. Edit the source text (e.g., change a tag or relation)
5. Press Ctrl+S to save

**Expected result:** The card closes, the graph rebuilds, the search query is refreshed to the new node text, an info-level log message appears ("pinned node text changed"), and the pinned node stays anchored to the new query.

### Edit Card: Other Line Edited, Pin Restored

**Steps:**
1. Open a Texere collection with multiple `.n4l` lines
2. Press Ctrl+G, click a node to pin it, press Enter
3. The edit card opens on that node's line
4. Navigate to a different line in the same file (Ctrl+G, Ctrl+H, or arrow keys) and edit it
5. Press Ctrl+S to save
6. Press Esc to close the card

**Expected result:** The card closes, the graph rebuilds and re-anchors to the original pinned node (not the edited other line). The pin is restored to its original target.

### Edit Card: Arrow-Target Node Opens at Its Reference Line

**Steps:**
1. Open a Texere collection
2. Press Ctrl+G to open the explorer
3. Locate a node that is only ever an arrow target (appears on the right side of relations, never as a source on the left side of a line)
4. Click that node to pin it
5. Press Enter

**Expected result:** The edit card opens on the line that *references* this node (the line whose arrow points at it). Such object/target nodes are common, so this is a normal flow, not an edge case. Only a node whose text appears nowhere in any parsed file (neither as a subject, a ditto, nor an arrow target) is truly unresolvable.

### Edit Card: Truly Unresolvable Node Warns

**Steps:**
1. With the explorer open, pin a node whose text does not appear anywhere in the loaded `.n4l` files (e.g. a synthesized node with no on-disk source)
2. Press Enter

**Expected result:** A warning-level log message appears ("no editable source line for ...") in the log panel. The edit card does NOT open and the existing pin is unchanged.

### Edit Card: Esc Closes Card, Not Explorer

**Steps:**
1. Open a Texere collection
2. Press Ctrl+G, click a node to pin it, press Enter
3. The edit card opens
4. Press Esc

**Expected result:** The edit card closes and returns focus to the graph explorer (if no change was made) or triggers a rebuild (if a change was saved). The explorer itself remains open and visible behind the card.
