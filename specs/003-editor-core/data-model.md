# Data Model: Editor Core

**Branch**: `003-editor-core` | **Date**: 2026-04-09

---

## 1. Existing Go Types (internal/content — no changes)

These types are already defined in `internal/content/types.go` and are automatically exposed to the frontend as TypeScript interfaces by Wails' binding generator.

### ParsedFile
```go
type ParsedFile struct {
    Path      string       // Absolute path to the .n4l file
    Chapters  []Chapter
    Reminders []Reminder
    Errors    []ParseError
}
```
**Used by**: `ParseFileForUI` Wails binding → frontend reads chapters for Column 2/3, reminders for badge count.

### Chapter
```go
type Chapter struct {
    Title     string
    LineStart int          // 1-indexed line of the "- title" line
    Nodes     []Node
    Labels    map[string]*Node
}
```
**Used by**: Column 2 (derive context tag chips from union of `Node.EffectiveCtx`), Column 3 (node list).

### Node
```go
type Node struct {
    ID           string
    Text         string
    Line         int          // 1-indexed — used to scroll Source tab
    EffectiveCtx []string     // pre-computed context tags at this line (sorted)
    IsSequence   bool
    Arrows       []Arrow
    Dittos       []DittoLine
    StudyMeta    *StudyMeta   // nil if not studyable
    // ... (other fields used by later features)
}
```
**Used by**: Column 3 node items; Source tab scroll-to-line; Read tab card rendering.

### Arrow
```go
type Arrow struct {
    Code         string
    Target       string
    IsRegistered bool
    Line         int
    IsDitto      bool
}
```
**Used by**: Read tab card properties; arrow autocomplete validation.

### ArrowDef
```go
type ArrowDef struct {
    Code          string
    LongName      string
    Reverse       string
    MetaType      string   // groups arrows in the palette
    Studyable     bool
    ClozeEligible bool
}
```
**Used by**: Arrow palette dropdown (grouped by `MetaType`); CM6 autocomplete completions.

### Reminder
```go
type Reminder struct {
    File string
    Line int
    Text string
}
```
**Note**: `Reminder` does not carry chapter or context; the frontend derives these by cross-referencing with the relevant `ParsedFile`.
**Used by**: Reminder badge count; reminder panel list.

### ChapterSummary
```go
type ChapterSummary struct {
    FilePath  string
    FileName  string
    Chapters  []string  // titles only
    ScanError error
}
```
**Used by**: Column 1 initial population via `OpenCollection`.

### CollectionEvent
```go
type CollectionEvent struct {
    Kind     EventKind        // Added | Removed | Renamed | Modified
    Path     string
    Chapters []ChapterSummary // populated for Added/Renamed
}
```
**Used by**: `collection:change` Wails event → Column 1 live update.

### IDInjection
```go
type IDInjection struct {
    FilePath   string
    NodeLine   int
    InsertedAt int
    ShortID    string
    AlreadyHad bool
}
```
**Used by**: `SaveFile` return value; frontend uses this to confirm injections occurred.

---

## 2. New Go Types

### AppConfig (new, in app.go)
```go
// AppConfig is the persisted user configuration.
// Stored at: os.UserConfigDir()/texere/config.json
type AppConfig struct {
    CollectionDir string   `json:"collectionDir"` // abs path; "" = no collection saved yet
    RecentArrows  []string `json:"recentArrows"`  // LRU list of arrow codes; max 10 entries
}
```
**Lifecycle**: loaded in `startup()`; written on every `OpenCollection` call and on "Change collection".  
**Platform paths**: Linux → `~/.config/texere/config.json`; Windows → `%AppData%\Texere\config.json`.

### SaveFileResult (new, in app.go)
```go
// SaveFileResult is returned by the SaveFile Wails-bound method.
type SaveFileResult struct {
    Injections  []content.IDInjection // IDs injected during this save (may be empty)
    FinalContent string               // On-disk content after injection (UTF-8)
}
```
**Rationale**: The frontend needs both the injection list (to report to the user if desired) and the final file content (to sync the CodeMirror document after `# @id` lines are inserted).

---

## 3. Frontend State (Svelte stores — `frontend/src/stores/workbench.js`)

These are runtime-only; not persisted between sessions in this feature.

### workbenchStore (writable)
```js
{
  collectionDir: string | null,        // absolute path of open collection (null = first-launch empty state)
  summaries: ChapterSummary[],         // Column 1 data (from OpenCollection)
  selectedChapterKey: {                // null = nothing selected
    filePath: string,
    chapterTitle: string,
  } | null,
  parsedFile: ParsedFile | null,       // result of last ParseFileForUI call
  activeContextFilter: Set<string>,    // toggled tag names (empty = show all)
  selectedNodeLine: number | null,     // 1-indexed line of node selected in Column 3
  activeTab: 'source' | 'read' | 'tutor',
  isDirty: boolean,                    // unsaved edits in Source tab
  externalModification: string | null, // path of file modified externally (pending prompt)
  arrowDefs: ArrowDef[],              // loaded once at startup from GetArrowRegistry
}
```

### Derived stores (computed from workbenchStore)
```js
// Column 2: unique context tag names from the selected chapter's nodes
contextTags: derived → { name: string, isSequence: boolean, isActive: boolean }[]

// Column 3: nodes from selected chapter filtered by activeContextFilter
filteredNodes: derived → Node[]  (sequence-sorted if _sequence_ active, else file order)

// Reminder badge count: total across all open files
// (parsedFile.Reminders.length for the current file; full cross-file count deferred)
reminderCount: derived → number
```

---

## 4. Key Derivations

### Context Tag Chips (Column 2)
```
chips = unique(flatten(chapter.Nodes.map(n => n.EffectiveCtx)))
isSequence = name === '_sequence_'
isActive = name in workbenchStore.activeContextFilter
```

### Filtered Nodes (Column 3)
```
if activeContextFilter is empty:
    show all chapter.Nodes in file order
else:
    show nodes where EffectiveCtx ⊇ activeContextFilter
    if '_sequence_' in activeContextFilter:
        sort by Node.Line (already file order, which is sequence order)
    else:
        file order (with optional alphabetical sort toggle)
```

### Reminder Count
For this feature: `parsedFile?.Reminders.length ?? 0`. Cross-file aggregation is deferred until multiple files can be held in memory simultaneously (future enhancement).

---

## 5. State Transitions

```
App starts (first launch — no saved config)
  └─ collectionDir = null → empty state shown with "Open collection folder" prompt

App starts (returning launch — saved config found)
  └─ OpenCollection(savedDir) called automatically
  └─ if dir exists: summaries populated, workbench shown
  └─ if dir missing: collectionDir = null → empty state with error notice

User opens collection (OpenCollection called)
  └─ summaries populated, collectionDir set
  └─ config written to disk
  └─ Column 1 renders

User clicks chapter
  └─ selectedChapterKey set
  └─ ParseFileForUI called → parsedFile set
  └─ activeContextFilter cleared
  └─ selectedNodeLine = null
  └─ isDirty = false

User toggles context chip
  └─ activeContextFilter updated
  └─ filteredNodes re-derived → Column 3 updates

User selects node in Column 3
  └─ selectedNodeLine = node.Line
  └─ Source tab scrolls to that line (if active)

User edits in Source tab
  └─ isDirty = true
  └─ Column 1 shows modified indicator

User presses Ctrl+S
  └─ SaveFile(path, content) called
  └─ isDirty = false
  └─ FinalContent replaces editor document
  └─ parsedFile refreshed via ParseFileForUI

collection:change event received
  └─ summaries refreshed (added/removed/renamed files)

file:modified-externally event received
  └─ externalModification = path
  └─ prompt shown; user accepts → reload file; rejects → ignore
```
