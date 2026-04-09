# Data Model: N4L File Parser — Layer I Content I/O

**Feature**: `002-n4l-parser` | **Date**: 2026-04-07  
**Package**: `internal/content`

All types are plain Go structs. The parser produces these types; it does not persist them — persistence is the responsibility of callers (`internal/learning` for SQLite, `internal/graph` for BadgerDB).

---

## Core Output Types

### ParsedFile

The complete result of a full parse of one `.n4l` file.

```
ParsedFile
├── Path        string           // Absolute path to the .n4l file
├── Chapters    []Chapter        // In file order
└── Errors      []ParseError     // Per-line errors; non-nil does not mean parse failed
```

### Chapter

A named section within a file. Begins at a `- title` line and ends at the next `- title` line or EOF.

```
Chapter
├── Title       string           // Text after "- " on the chapter line
├── LineStart   int              // 1-indexed line number of the "- title" line
├── Nodes       []Node           // In file order, root nodes only (ditto lines are embedded in their parent)
└── Labels      map[string]*Node // @label → node pointer (resolved within this chapter)
```

### Node

A root-level knowledge statement. Ditto continuations are embedded in `Dittos`, not in `Chapter.Nodes`.

```
Node
├── ID              string          // Stable identifier (see NodeID below); "" if no label or @id marker
├── Text            string          // Raw node text (leading whitespace stripped, trailing comment excluded)
├── Line            int             // 1-indexed line number of this node line
├── EffectiveCtx    []string        // Snapshot of context tags active at this node (sorted, deterministic)
├── IsSequence      bool            // true if _sequence_ was in EffectiveCtx at this line
├── Arrows          []Arrow         // Outgoing arrows declared on this node line
├── Dittos          []DittoLine     // Continuation lines (in file order)
├── StudyMeta       *StudyMeta      // nil if not studyable
├── Priority        *int            // nil if no @priority marker; value 0–100
├── Provenance      *Provenance     // nil if no @source / @ref / @progress markers
├── Label           string          // The @label value if this node was labeled; "" otherwise
└── IDMarkerLine    int             // 1-indexed line number of the "# @id" line above this node; 0 if absent
```

### NodeID

Node identity is **solely system-managed** via `# @id` markers. `@label` is a cross-reference anchor and plays no role in identity.

- **System-IDed node** (`IDMarkerLine > 0`):
  `"<filepath>:<chapter-title>:id:<shortid>"`
  Example: `"notes/french.n4l:vocabulary:id:k7mpq9rx"`
  Stable permanently — the `# @id` value is written into the `.n4l` file on first save and never changes, regardless of any subsequent edits to the node's text, arrows, label, or context.

- **Untracked node** (`IDMarkerLine == 0`):
  `ID == ""`
  The node is parsed and shown in the structural browser but cannot be scheduled. The system assigns a `# @id` on the next save that includes a study marker on this node.

**`@label` is not an ID.** A labeled node that has never been saved with a study marker has `ID == ""`. A labeled node that has been saved with a study marker has both a `Label` (for cross-referencing) and a `# @id` (for scheduling). Renaming or removing the label has zero effect on the node's `ID`.

**ID format**: 8 characters from the base-36 alphabet `[0-9a-z]`, generated with `crypto/rand`. Uniqueness within the file is verified before write (FR-023).

### Arrow

A typed relationship declared on a node or ditto line.

```
Arrow
├── Code        string   // Short code, e.g., "means", "e.g.", "note"
├── Target      string   // Target string (raw text after the code)
├── IsRegistered bool    // false if code not found in SSTRegistry (parse error raised but arrow retained)
├── Line        int      // 1-indexed line number
└── IsDitto     bool     // true if this arrow belongs to a DittoLine, not the root Node
```

### DittoLine

A continuation of the immediately preceding Node (`"  (arrow) target`).

```
DittoLine
├── Line    int      // 1-indexed
└── Arrows  []Arrow  // All arrows declared on this ditto line
```

### StudyMeta

Study scheduling metadata extracted from markers. Applied to the Node it annotates.

```
StudyMeta
├── CardType        CardType    // Basic | Reverse | Cloze | SequenceForward | SequenceGap | SequenceOrder
├── ForceReverse    bool        // # @card reverse
├── SuppressReverse bool        // # @card no-reverse
├── ClozeTargets    []string    // Text inside {braces} for cloze cards
├── FromBlock       bool        // true if inherited from a @study block (not individual marker)
└── BlockType       BlockType   // None | Study | StudyReverse | StudySequence
```

**CardType enum**: `Basic`, `Reverse`, `Cloze`, `SequenceForward`, `SequenceGap`, `SequenceOrder`  
**BlockType enum**: `None`, `Study`, `StudyReverse`, `StudySequence`

### Reminder

An ALL CAPS line (≥ 3 words) representing an open knowledge-debt item.

```
Reminder
├── File    string   // Absolute path to source file
├── Line    int      // 1-indexed line number
└── Text    string   // Full line text (comment stripped)
```

**Note**: Reminders are also accessible via `Node.Text` for nodes that happen to be reminders, but the parser additionally collects them into a flat `ParsedFile.Reminders []Reminder` list for convenience.

Update `ParsedFile` to include:

```
ParsedFile
├── Path        string
├── Chapters    []Chapter
├── Reminders   []Reminder   // All reminders across all chapters in this file
└── Errors      []ParseError
```

### Provenance

Source attribution metadata.

```
Provenance
├── Source      string   // # @source value (e.g., "Thinking, Fast and Slow by Kahneman")
├── Ref         string   // # @ref value (e.g., "sources/kahneman-tfs.pdf")
└── Progress    string   // # @progress value (e.g., "ch.3 p.47")
```

---

## Directory Scan Types

### ChapterSummary

Lightweight result of the Phase 1 scan (title extraction only — no full parse).

```
ChapterSummary
├── FilePath    string   // Absolute path
├── FileName    string   // Base name (for display)
├── Chapters    []string // Chapter titles in file order; empty slice if no "- title" found
└── ScanError   error    // Non-nil if file could not be read
```

---

## Write-Back Types

### IDInjection

Describes one `# @id` line inserted into a file during a write-back operation. Returned by both `InjectMissingIDs` (batch, save-triggered) and `InjectNodeID` (single-node, scheduler-triggered). The same struct is used regardless of whether the injection was for an extract or an item.

```
IDInjection
├── FilePath    string   // Absolute path of the modified file
├── NodeLine    int      // 1-indexed line number of the node that received an ID
├── InsertedAt  int      // 1-indexed line number where "# @id <shortid>" was inserted
├── ShortID     string   // The generated 8-char base-36 ID
└── AlreadyHad  bool     // true if the node already had a # @id (no write occurred)
```

### WriteBackResult

Returned by all write-back operations (ID injection, reminder resolution, `@progress` update).

```
WriteBackResult
├── FilePath     string        // Absolute path of the modified file
├── LinesChanged []int         // 1-indexed line numbers that were modified or inserted
└── Err          error         // Non-nil if the operation failed; file is untouched on error
```

### WriteBackOp (internal)

Not exported. Represents one pending targeted modification before the file is rewritten.

```
writeBackOp
├── LineNumber   int        // 1-indexed target line
├── Kind         opKind     // Insert | Replace
├── Content      string     // New line content (for Replace) or line to insert (for Insert)
└── InsertBefore bool       // For Insert: place before LineNumber (not after)
```

Multiple `writeBackOp` values are sorted by `LineNumber` descending before application so that inserting a line does not shift the line numbers of subsequent ops.

---

## Registry Types

### SSTRegistry

Loaded from the SSTconfig directory at startup. Consumed by the parser for arrow validation and annotation symbol recognition.

```
SSTRegistry
├── Arrows          map[string]ArrowDef   // Short code → definition
├── Annotations     map[string]AnnotDef   // Symbol (">>", "%", "=", "**") → definition
└── LoadErrors      []error               // Non-fatal load errors (unknown files, parse issues)
```

### ArrowDef

```
ArrowDef
├── Code        string   // Short code, e.g., "means"
├── LongName    string   // Human name, e.g., "means / is meaning of"
├── Reverse     string   // Reverse code; "" if one-way
├── MetaType    string   // SST meta-type (e.g., "association", "causation")
├── Studyable   bool     // From SSTconfig comment metadata
└── ClozeEligible bool
```

### AnnotDef

```
AnnotDef
├── Symbol      string   // e.g., ">>"
└── ArrowCode   string   // Maps to an ArrowDef code
```

---

## Error Types

### ParseError

```
ParseError
├── File    string
├── Line    int
└── Msg     string   // Human-readable description of the problem
```

**Error categories** (values of Msg prefix):
- `"unknown arrow"` — short code not in SSTRegistry
- `"orphan ditto"` — `"  (arrow)` line with no preceding node
- `"unresolved reference"` — `$label.N` with no matching `@label`
- `"invalid priority"` — `# @priority N` where N is outside 0–100
- `"malformed marker"` — `# @card` / `# @study` with unrecognised modifier
- `"encoding error"` — non-UTF-8 byte sequence in file

---

## State Machine (Parser Internal)

Not exported. Documented here for implementer reference.

```
parserState
├── chapter         *Chapter            // chapter being built; nil before first "- title"
├── effectiveCtx    map[string]struct{}  // current context tag set
├── dittoParent     *Node               // last root node (for ditto attribution)
├── studyBlock      studyBlockState     // {kind, reversedForced}
├── pendingStudy    *StudyMeta          // individual marker waiting for next node
├── labelMap        map[string]*Node    // @label → node (within current chapter)
├── pendingRefs     []pendingRef        // $label.N refs awaiting resolution
└── lastPriority    *int                // most recent @priority value
```

**Context operator semantics**:

| Line | Operation | Effect on `effectiveCtx` |
|---|---|---|
| `:: tag1, tag2` | Replace | Clear set; add tag1, tag2 |
| `+:: tag3, tag4 ::` | Union | Add tag3, tag4 to existing set |
| `-:: tag3 :` | Subtract | Remove tag3 from existing set |
| `:: ` (empty) | Replace with empty | Clear set entirely |

**Study block boundaries**: A `# @study` block closes on:
- `# @end` marker
- New `# @study` marker (implicit close + open)
- `:: ` context replacement operator
- EOF

`+::` and `-::` do NOT close a study block.

**Ditto resolution**: A `"  ` line (starting with a double-quote and space) is a ditto. It is attributed to `dittoParent`. If `dittoParent` is nil, a `ParseError("orphan ditto")` is emitted and the line is skipped.

---

## Watcher Types

### CollectionWatcher

```
CollectionWatcher
├── Events  <-chan CollectionEvent   // Receive events here
└── Errors  <-chan error             // fsnotify-level errors
// Methods: Close() error
```

### CollectionEvent

```
CollectionEvent
├── Kind        EventKind            // Added | Removed | Renamed | Modified
├── Path        string               // Absolute path of affected file
└── Chapters    []ChapterSummary     // Populated for Added/Renamed; nil for Removed/Modified
```

**EventKind enum**: `Added`, `Removed`, `Renamed`, `Modified`

**Modified events**: Emitted when an existing `.n4l` file's content changes on disk (e.g., edited externally). The caller may choose to re-scan chapter titles and update Column 1 if titles changed, but is not required to trigger a full re-parse (per spec: re-parse on next selection).
