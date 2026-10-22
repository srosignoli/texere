# Implementation Plan: Editor Core — Hybrid Band Layout Workbench

**Branch**: `003-editor-core` | **Date**: 2026-04-09 | **Spec**: `docs/specs/003-editor-core/spec.md`
**Input**: Feature specification from `/docs/specs/003-editor-core/spec.md`

## Summary

Implement the three-band Texere workbench: a top structural-navigation band (Chapter/File → Context Tags → Nodes), a middle button bar (+node, +link, +context, save, search, undo, reminder badge, arrow palette), and a bottom editor area with Source (CodeMirror 6), Read (card renderer), and Tutor (placeholder) tabs. All graph-integration controls (Column 4, Build, Sync, History) and study/LLM actions are deferred. The Go backend is almost complete — three new `App` methods are needed. The frontend is built from scratch on top of the existing Svelte 3 + Vite 3 scaffold.

## Technical Context

**Language/Version**: Go 1.22 (backend) + TypeScript/JavaScript, Svelte 3.49, Vite 3 (frontend)  
**Primary Dependencies**: Wails v2.12, CodeMirror 6 (`@codemirror/state`, `@codemirror/view`, `@codemirror/language`, `@codemirror/autocomplete`), `fsnotify` v1.9 (already in `go.mod`)  
**Storage**: `.n4l` plain-text files only; no DB layer for this feature  
**Testing**: `go test ./...` (backend); no frontend test runner in place yet  
**Target Platform**: Linux amd64 + Windows amd64 cross-compiled  
**Project Type**: Desktop app (Wails v2 — Go binary + embedded webview)  
**Performance Goals**: Chapter (500 nodes) renders in < 500 ms; chip toggle updates Column 3 in < 100 ms; Ctrl+S completes in < 500 ms for 10 k-line files; Column 1 FS update in < 200 ms; Source tab 10 k lines without perceptible lag  
**Constraints**: Pure Go, no CGo, single binary, offline; existing `internal/content` API used as-is  
**Scale/Scope**: Single-user, single-window; flat `.n4l` directory; files up to 10 k lines

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Source-First & Full N4L Fidelity | ✅ PASS | Atomic save (`writeLines` temp+rename) + `InjectMissingIDs` preserves round-trip; Source tab sends raw text to Go for write-back |
| II. Self-Contained Binary | ✅ PASS | CodeMirror 6 + Svelte bundled into binary by Wails; no external runtime; all Go deps are pure Go |
| III. Separation of Concerns | ✅ PASS | Feature touches Layer I (content files) only; no BadgerDB or SQLite writes in this feature |
| IV. Pleasure-Driven Learning | ✅ N/A | Study and Go Neural are deferred to later features |
| V. Transparency & Portability | ✅ PASS | Plain-text editing; no hidden state; user-controlled save |

**Performance Standards applicable to this feature:**

| Standard | Budget | Coverage |
|----------|--------|---------|
| N4L parse + context replay | < 50 ms | `ParseFile` called on chapter select |
| Read view render | < 200 ms | Read tab renders from parsed AST |

No violations. Complexity Tracking table not required.

## Project Structure

### Documentation (this feature)

```text
docs/specs/003-editor-core/
├── plan.md              ← this file
├── research.md          ← Phase 0 output
├── data-model.md        ← Phase 1 output
├── contracts/
│   └── wails-bridge.md  ← Phase 1 output
└── tasks.md             ← Phase 2 output (/speckit-tasks — not created here)
```

### Source Code

```text
app.go                                   # Add 3 new Wails-bound methods; complete consumeWatchEvents

frontend/src/
├── App.svelte                           # Rewrite: three-band layout root
├── stores/
│   └── workbench.js                     # Svelte writable stores (workbench state)
├── lib/
│   ├── n4l-language.js                  # CodeMirror 6 StreamLanguage for N4L syntax
│   ├── n4l-highlight.js                 # CM6 highlight styles (token → CSS class)
│   └── arrow-autocomplete.js            # CM6 CompletionSource drawing from ArrowDef registry
└── components/
    ├── TopBand.svelte                   # Structural nav container (3 columns)
    ├── ColumnChapters.svelte            # Column 1 — Chapter/File list
    ├── ColumnContextTags.svelte         # Column 2 — Context tag chips
    ├── ColumnNodes.svelte               # Column 3 — Node list with sequence connectors
    ├── MiddleBand.svelte                # Button bar + reminder badge + arrow palette
    ├── BottomBand.svelte                # Tab switcher container
    ├── SourceTab.svelte                 # CodeMirror 6 wrapper
    ├── ReadTab.svelte                   # Read-only card renderer
    ├── TutorTab.svelte                  # Placeholder + LLM selector stub
    └── ReminderPanel.svelte             # Slide-out reminder list panel
```

**Structure Decision**: Web application layout (Option 2 adapted for Wails). Backend (`app.go` + `internal/content/`) is the Go layer. Frontend (`frontend/src/`) is the Svelte layer. Wails auto-generates `frontend/wailsjs/` bindings from Go struct tags — do not hand-edit those files.
