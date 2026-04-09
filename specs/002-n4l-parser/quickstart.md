# Quickstart: `internal/content` Package

**Feature**: `002-n4l-parser` | **Date**: 2026-04-07

---

## Overview

`internal/content` is Layer I of Texere: the only component that reads `.n4l` source files.
Everything else (knowledge graph, learning state, UI) is derived from its output.

The package exposes three capabilities:

| Capability | Function | When used |
|---|---|---|
| Load arrow registry | `LoadRegistry` | Once at app startup |
| Scan collection (fast) | `ScanCollection` | Populate Column 1 |
| Full parse (on-demand) | `ParseFile` | User selects a file |
| Watch for changes | `WatchCollection` | Keep Column 1 live |

---

## Typical Startup Sequence

```go
// 1. Load the SSTconfig arrow registry (once).
registry, err := content.LoadRegistry("path/to/SSTconfig")
if err != nil {
    log.Printf("content: registry degraded: %v", err)
    // app continues in degraded mode — arrows not validated
}

// 2. Scan the collection directory to populate Column 1.
summaries, err := content.ScanCollection("/path/to/notes")
if err != nil {
    return fmt.Errorf("cannot open collection: %w", err)
}
// summaries is sorted alphabetically by filename.
// Each entry has .Chapters []string for multi-chapter display.

// 3. Start watching for filesystem changes.
watcher, err := content.WatchCollection("/path/to/notes")
if err != nil {
    log.Printf("content: watch unavailable: %v", err)
} else {
    go func() {
        for {
            select {
            case ev := <-watcher.Events:
                // Update Column 1 based on ev.Kind and ev.Path.
            case err := <-watcher.Errors:
                log.Printf("watcher error: %v", err)
            }
        }
    }()
}
```

---

## On-Demand Parse (User Selects a File)

```go
// Called when user clicks a file/chapter in Column 1.
parsed, errs := content.ParseFile("/path/to/notes/french.n4l", registry)
if parsed == nil {
    // File could not be read at all — show error to user.
    return
}
if len(errs) > 0 {
    // File parsed with some line errors — show warnings but continue.
    for _, e := range errs {
        log.Printf("parse warning line %d: %s", e.Line, e.Msg)
    }
}

// Access chapters
for _, ch := range parsed.Chapters {
    fmt.Printf("Chapter: %s (%d nodes)\n", ch.Title, len(ch.Nodes))
}

// Access all reminders in the file
for _, r := range parsed.Reminders {
    fmt.Printf("Reminder line %d: %s\n", r.Line, r.Text)
}
```

---

## Node ID Lookup

The stable node ID is embedded in each `Node.ID`. To reconstruct it independently
(e.g., when querying the learning database):

```go
id := content.NodeID(
    "/path/to/notes/french.n4l",
    "vocabulary",
    "",            // no @label
    "petit-déjeuner (means) breakfast",
)
// id == "notes/french.n4l:vocabulary:fnv64a:a3f2c1d0e9b8f7a6"
```

---

## Running Tests

```bash
make test                         # all tests including content package
go test ./internal/content/...    # content package only
go test -bench=. ./internal/content/...  # benchmarks (must complete < 50 ms for BenchmarkParseFile10k)
```

---

## Adding a New Arrow to SSTconfig

1. Edit the relevant file in `SSTconfig/`.
2. Restart the application (or reload registry — `LoadRegistry` is safe to call again).
3. The new arrow code is immediately available for validation and autocomplete.
4. No code changes required.

---

## Write-Back: ID Injection on Save

Called by `app.go` on every Ctrl+S. Injects `# @id` markers for any study-marked nodes
that don't yet have a stable ID — so the scheduling layer can track them across future edits.

```go
// After ParseFile completes following a save event:
injections, err := content.InjectMissingIDs(parsed)
if err != nil {
    log.Printf("content: id injection failed: %v", err)
    // Non-fatal: nodes without IDs simply won't be tracked this save cycle
} else if len(injections) > 0 {
    for _, inj := range injections {
        log.Printf("assigned id %s to node at line %d", inj.ShortID, inj.NodeLine)
    }
    // parsed.Chapters now contains the updated IDs — no re-parse needed
}
```

The result in the `.n4l` file:
```n4l
# Before save:
# @card
petit-déjeuner (means) breakfast

# After InjectMissingIDs:
# @id k7mpq9rx
# @card
petit-déjeuner (means) breakfast
```

## Write-Back: Resolve a Reminder

```go
result, err := content.ResolveReminder("/path/to/notes/french.n4l", reminder.Line)
if err != nil {
    return fmt.Errorf("resolve reminder: %w", err)
}
// The ALL CAPS line is now lowercase in the file.
// All other lines are byte-for-byte identical.
```

## Write-Back: Update Reading Progress

```go
result, err := content.UpdateProgress("/path/to/notes/book.n4l", progressLine, "ch.5 p.112")
if err != nil {
    return fmt.Errorf("update progress: %w", err)
}
```

## Layer Boundaries

`internal/content` MUST NOT:
- Import `internal/graph` or `internal/learning`.
- Open a database connection.
- Replace a file's full content (that is the editor's job, not the content package's).

`internal/content` MAY write to `.n4l` files ONLY through the write-back path (`writeback.go`), and only for these three operations:
- Inject `# @id` markers (on save)
- Lowercase a resolved reminder (user-triggered)
- Update `# @progress` value (user-triggered)

Every write-back operation modifies only the targeted line(s). All other lines are preserved byte-for-byte.

If you need to persist parsed output, pass it to the appropriate layer:
- Study markers → `internal/learning` (on save event)
- Node/arrow graph → `internal/graph` (on build-graph event)
