# Data Model: Project Scaffold

**Branch**: `001-project-scaffold`
**Phase**: 1 — Design
**Date**: 2026-04-06

> The scaffold feature introduces no runtime data entities (no database schemas,
> no file formats). The "data model" for this feature is the **package architecture**
> — the Go package contracts that establish the three constitutional layers and
> define the boundaries future features MUST respect.

---

## Package Architecture

### Layer I — Content (`internal/content`)

**Purpose**: All operations on `.n4l` plain-text files. This is the only layer
permitted to read or write `.n4l` files.

**Package path**: `github.com/[owner]/texere/internal/content`

**Scaffold state**: Empty package with `doc.go` establishing intent.

**Future responsibility**:
- Parse `.n4l` syntax into an in-memory AST
- Write AST back to `.n4l` with byte-identical round-trip fidelity
- Expose the canonical `Node`, `Arrow`, `Chapter`, `ContextSet` types

**Constitutional constraint**: No other package may read or write `.n4l` files
directly. All content access MUST go through this layer.

---

### Layer II — Graph (`internal/graph`)

**Purpose**: Knowledge graph storage and querying via SSTorytime-KV (BadgerDB).
This layer is a derived cache — it MUST be fully rebuildable from the content layer.

**Package path**: `github.com/[owner]/texere/internal/graph`

**Scaffold state**: Empty package with `doc.go` establishing intent.

**Future responsibility**:
- Manage BadgerDB lifecycle (open, close, compact)
- Store and retrieve `Node` → `Arrow` → `Node` triples
- Expose graph query operations (radius-N neighbour lookup, pathsolve)
- Trigger rebuild from content layer on demand

**Constitutional constraint**: MUST NOT store canonical knowledge. Losing BadgerDB
state MUST NOT result in permanent data loss. Rebuild from `.n4l` source MUST always
be possible.

---

### Layer III — Learning (`internal/learning`)

**Purpose**: Spaced-repetition scheduling state via SQLite. Derived from content;
lossy rebuild is acceptable (history lost, but user knowledge is not).

**Package path**: `github.com/[owner]/texere/internal/learning`

**Scaffold state**: Empty package with `doc.go` establishing intent.

**Future responsibility**:
- Manage SQLite lifecycle (`learning.db`)
- Store FSRS scheduling parameters per study item
- Expose the unified priority queue
- Expose review history and statistics

**Constitutional constraint**: MUST NOT store canonical knowledge. Items in SQLite
are keyed by node identity derived from content layer; the content layer is
authoritative for what nodes exist.

---

### Application Layer (`app.go`, `main.go`)

**Purpose**: Wails application lifecycle, frontend ↔ backend binding, and
top-level orchestration.

**Scaffold state**: Minimal `App` struct with `startup`, `domReady`, `shutdown`
lifecycle methods. Exposes no backend methods to the frontend at scaffold stage.

**Future responsibility**: Route user actions from the Svelte frontend to the
appropriate internal layer (content / graph / learning).

---

### UI Layer (`frontend/`)

**Purpose**: Svelte-based webview UI rendered by Wails.

**Scaffold state**: Default Svelte + Vite setup. `App.svelte` shows a static
placeholder with the "Texere" application name.

**Future responsibility**: All user-visible UI components, state management,
and Wails JS bindings (`wailsjs/`).

---

## Layer Dependency Rules

```
UI (frontend/)
    ↓ calls (Wails RPC)
app.go
    ↓ uses
internal/content   ←──── canonical source of truth (.n4l files)
    ↑ feeds
internal/graph     ──── derived cache (BadgerDB)
internal/learning  ──── derived state (SQLite)
```

**Forbidden dependencies**:
- `internal/graph` MUST NOT import `internal/learning` (and vice versa)
- `internal/graph` and `internal/learning` MUST NOT write `.n4l` files
- `frontend/` communicates ONLY via Wails-exposed methods on `app.go`

---

## File Artifacts Produced by Scaffold

| Path | Layer | Description |
|---|---|---|
| `main.go` | App | Wails entry point |
| `app.go` | App | App struct + lifecycle methods |
| `wails.json` | App | Wails project configuration |
| `go.mod` | App | Go module (go 1.21, texere module path) |
| `Makefile` | Build | `build`, `build-windows`, `test`, `lint`, `dev`, `clean` |
| `internal/content/doc.go` | Content | Layer I package declaration |
| `internal/graph/doc.go` | Graph | Layer II package declaration |
| `internal/learning/doc.go` | Learning | Layer III package declaration |
| `frontend/src/App.svelte` | UI | Placeholder "Texere" window |
| `.golangci.yml` | Build | Linter configuration |
| `.github/workflows/ci.yml` | CI | GitHub Actions workflow |
| `docs/quickstart.md` | Docs | Developer getting-started guide |
