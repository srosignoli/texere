// Package content implements Layer I of the Texere architecture:
// the sole read/write path for .n4l knowledge files.
//
// # Capabilities
//
// Three exported capabilities form the decode path (read-only):
//
//   - [ScanCollection]: fast directory scan that extracts chapter titles only;
//     used at collection open time to populate Column 1 without full parse overhead.
//   - [ParseFile]: full single-pass state-machine parse of one .n4l file;
//     called on-demand when the user selects a file.
//   - [WatchCollection]: filesystem watcher (via fsnotify) that emits
//     [CollectionEvent] values as files are added, removed, renamed, or modified,
//     keeping Column 1 current without polling.
//
// The write-back path handles three specific structured modifications to .n4l files:
//
//   - [InjectMissingIDs] / [InjectNodeID]: insert a "# @id <shortid>" marker above
//     a study-marked or pipeline-queued node to establish stable system identity.
//   - [ResolveReminder]: lowercase an ALL CAPS reminder line when the user marks it resolved.
//   - [UpdateProgress]: update the value after a "# @progress" marker.
//
// All write-back operations use temp-file-then-rename atomicity.
//
// # Layer Boundary Rules
//
// This package must NOT import internal/graph or internal/learning.
// It outputs plain Go structs only; callers route those structs to the appropriate layer.
// The only external dependency added by this package is github.com/fsnotify/fsnotify
// (pure Go, no CGo).
package content
