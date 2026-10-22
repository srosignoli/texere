# Wails Bridge Contract: Editor Core

**Branch**: `003-editor-core` | **Date**: 2026-04-09

Wails v2 exposes Go methods on the `App` struct as callable async functions in the frontend. TypeScript interfaces for all return types are auto-generated into `frontend/wailsjs/go/main/App.d.ts`. Do not hand-edit that file.

---

## Startup Behaviour (config auto-load)

On `startup()`, `app.go` reads `AppConfig` from `os.UserConfigDir()/texere/config.json`. If `CollectionDir` is non-empty and the directory exists, `OpenCollection` is called immediately — the frontend receives `collection:change` (or the initial `ChapterSummary[]` return) and populates Column 1 without user interaction. If config is absent or `CollectionDir` is empty, no call is made; the frontend renders the empty state.

---

## Existing Methods (no changes)

These are already implemented in `app.go` and used as-is.

### OpenCollection
```go
func (a *App) OpenCollection(dir string) ([]content.ChapterSummary, error)
```
- Scans `dir` for `.n4l` files, starts the `CollectionWatcher`, returns `ChapterSummary[]` for Column 1.
- Called once when the user opens a collection.

### ParseFileForUI
```go
func (a *App) ParseFileForUI(path string) (*content.ParsedFile, []content.ParseError)
```
- Full parse of `path`. Returns `ParsedFile` with all chapters, nodes (including `EffectiveCtx`), reminders.
- **Side-effect**: sets `a.openFilePath = path` so `consumeWatchEvents` can emit `file:modified-externally` for the correct file. No separate `SetOpenFile` method is needed.
- Called when a chapter is selected in Column 1, and after every `SaveFile` call.

---

## New Methods (to be added to app.go)

### OpenCollection (extended — now also persists config)
`OpenCollection` already exists. Its contract is extended: after a successful scan it MUST write `AppConfig{CollectionDir: dir}` to the config file. No signature change.

### GetFileContent
```go
func (a *App) GetFileContent(path string) (string, error)
```
- Returns the raw UTF-8 text of `path` for population of the CodeMirror editor on file open.
- Simple `os.ReadFile` wrapper. Does not parse.
- **When called**: Source tab mounts or reloads after external modification accepted.

### SaveFile
```go
// SaveFileResult is defined in app.go (not in internal/content).
type SaveFileResult struct {
    Injections   []content.IDInjection
    FinalContent string // on-disk content after @id injection
}

func (a *App) SaveFile(path, content string) (*SaveFileResult, error)
```
- Atomically writes `content` to `path` via `writeLines` (temp+rename).
- Parses the saved file and calls `content.InjectMissingIDs`; if any IDs were injected, a second atomic write occurs.
- Reads final on-disk content and returns it as `FinalContent`.
- Sets internal `a.savingPath = path` before writes; clears it after. Used by `consumeWatchEvents` to suppress false `file:modified-externally` events.
- **When called**: Ctrl+S in Source tab (FR-020).

### GetArrowRegistry
```go
func (a *App) GetArrowRegistry() ([]content.ArrowDef, error)
```
- Returns `a.registry.Arrows` as a flat slice, sorted by `MetaType` then `Code`.
- Called once at app startup; result stored in `arrowDefsStore`.
- Returns an empty slice (not error) when registry is in degraded mode (no SSTconfig directory).
- **When called**: `App.svelte` `onMount`.

### ResolveReminder
```go
func (a *App) ResolveReminder(path string, lineNumber int) (*content.WriteBackResult, error)
```
- Delegates directly to `content.ResolveReminder(path, lineNumber)`.
- Already implemented in `internal/content/writeback.go`; this method is just the Wails binding.
- **When called**: User clicks "Resolve" in the reminder panel.

### GetInitialSummaries
```go
func (a *App) GetInitialSummaries() []content.ChapterSummary
```
- Returns the chapter summaries loaded during `startup()` auto-load (from config-persisted `CollectionDir`).
- Returns `nil` if no collection was auto-loaded (first launch or missing directory).
- **When called**: `App.svelte` `onMount`, immediately after Wails runtime is ready, to populate Column 1 without requiring a user action.

### GetStartupError
```go
func (a *App) GetStartupError() string
```
- Returns a human-readable error message if `startup()` attempted to auto-load a collection directory that no longer exists.
- Returns `""` if startup completed without error (including first-launch with no saved directory).
- **When called**: `App.svelte` `onMount`, to decide whether to show the EmptyState with an error notice (FR-000c).

### UpdateRecentArrows
```go
func (a *App) UpdateRecentArrows(codes []string) error
```
- Replaces the `RecentArrows` field in `AppConfig` with `codes` (caller is responsible for LRU ordering and 10-entry cap) and writes config to disk.
- **When called**: Arrow palette selection in MiddleBand (FR-014) after each arrow insertion.

---

## Events (Go → Svelte)

Emitted via `runtime.EventsEmit(a.ctx, eventName, payload)` inside `consumeWatchEvents`.

### collection:change
```
Event name:  "collection:change"
Payload:     content.CollectionEvent
             { Kind: EventKind, Path: string, Chapters: ChapterSummary[] }
```
- Emitted for every `EventAdded`, `EventRemoved`, `EventRenamed` in the watched collection.
- **Not** emitted for `EventModified` (that goes to `file:modified-externally` if applicable).
- Frontend handler: refresh `summaries` in `workbenchStore`.

### file:modified-externally
```
Event name:  "file:modified-externally"
Payload:     { path: string }
```
- Emitted when `EventModified` fires for the currently open file AND `event.Path != a.savingPath`.
- Frontend handler: set `workbenchStore.externalModification = path`, display reload prompt.

---

## Frontend Subscription Pattern (Svelte)

```js
import { EventsOn } from '../wailsjs/runtime/runtime';

onMount(() => {
  const cancelChange = EventsOn('collection:change', (ev) => { /* update summaries */ });
  const cancelModified = EventsOn('file:modified-externally', ({ path }) => { /* prompt */ });
  return () => {
    cancelChange();
    cancelModified();
  };
});
```

---

## Error Handling

All Wails-bound methods that return `error` surface the error message as a rejected Promise on the JavaScript side. The frontend should display a non-blocking toast notification for recoverable errors (save failure, registry degraded) and a blocking dialog for destructive ones (unsaved changes on close).
