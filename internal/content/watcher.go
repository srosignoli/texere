package content

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// WatchCollection starts a recursive filesystem watcher on dir. Returns a
// *CollectionWatcher whose Events channel emits CollectionEvent values as .n4l
// files are added, removed, renamed, or modified anywhere under dir (excluding
// the directories rejected by isExcludedDir).
//
// At start, a watch is added for every non-excluded directory in the tree. When
// a new directory is created later, it is watched too, and any .n4l files
// already inside it are surfaced as EventAdded — so subtrees created after open
// (e.g. reading/ on first import) self-heal without a re-open.
//
// Each emitted ChapterSummary carries a collection-relative RelPath.
// Call CollectionWatcher.Close() to stop the watcher and close both channels.
// Returns an error if dir does not exist or the OS watcher cannot be initialised.
func WatchCollection(dir string) (*CollectionWatcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("content: WatchCollection: %w", err)
	}
	if err := addWatchRecursive(fw, dir, dir); err != nil {
		fw.Close()
		return nil, fmt.Errorf("content: WatchCollection: %w", err)
	}

	events := make(chan CollectionEvent, 16)
	errors := make(chan error, 4)

	go func() {
		defer close(events)
		defer close(errors)
		for {
			select {
			case ev, ok := <-fw.Events:
				if !ok {
					return
				}
				for _, ce := range translateEvent(fw, ev, dir) {
					events <- ce
				}
			case err, ok := <-fw.Errors:
				if !ok {
					return
				}
				errors <- err
			}
		}
	}()

	cw := &CollectionWatcher{
		Events: events,
		Errors: errors,
		close:  fw.Close,
	}
	return cw, nil
}

// addWatchRecursive adds a watch for dir and every non-excluded directory below
// it. root is the collection root, which is never excluded regardless of name.
func addWatchRecursive(fw *fsnotify.Watcher, root, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			return fs.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && isExcludedDir(d.Name()) {
			return fs.SkipDir
		}
		return fw.Add(path)
	})
}

// translateEvent maps one fsnotify.Event to zero or more CollectionEvents.
//
// A directory-create self-heals: the new subtree is watched and its existing
// .n4l files are surfaced as EventAdded (which may be several events). All other
// events concern a single .n4l file.
func translateEvent(fw *fsnotify.Watcher, ev fsnotify.Event, root string) []CollectionEvent {
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if isExcludedDir(filepath.Base(ev.Name)) {
				return nil
			}
			// Best-effort: the dir may vanish before we watch it, or watches
			// may be exhausted; its existing files are still surfaced below.
			_ = addWatchRecursive(fw, root, ev.Name)
			// A .n4l landing in the new dir between the walk snapshot and the
			// watch taking effect may be surfaced both here and by its own
			// Create event; that duplicate EventAdded is tolerated because the
			// frontend upserts by FilePath.
			return scanSubtreeAdded(root, ev.Name)
		}
	}

	if !strings.EqualFold(filepath.Ext(ev.Name), ".n4l") {
		return nil
	}

	switch {
	case ev.Has(fsnotify.Create):
		return []CollectionEvent{{Kind: EventAdded, Path: ev.Name, Chapters: summaryFor(root, ev.Name)}}
	case ev.Has(fsnotify.Remove):
		return []CollectionEvent{{Kind: EventRemoved, Path: ev.Name}}
	case ev.Has(fsnotify.Rename):
		return []CollectionEvent{{Kind: EventRenamed, Path: ev.Name, Chapters: summaryFor(root, ev.Name)}}
	case ev.Has(fsnotify.Write):
		// Re-scan chapter titles so the file tree updates when chapters are
		// added/removed/renamed by an editor save or external tool. Cheap:
		// reads only header-looking lines, not the whole node body.
		return []CollectionEvent{{Kind: EventModified, Path: ev.Name, Chapters: summaryFor(root, ev.Name)}}
	}
	return nil
}

// summaryFor builds the single-element Chapters slice attached to an Added /
// Renamed / Modified event. Returns nil when the file cannot be scanned, so the
// frontend's "Chapters present" guards behave exactly as before.
func summaryFor(root, path string) []ChapterSummary {
	chapters, err := scanChapterTitles(path)
	if err != nil {
		return nil
	}
	rel, relErr := filepath.Rel(root, path)
	if relErr != nil {
		rel = filepath.Base(path)
	}
	return []ChapterSummary{{
		FilePath: path,
		FileName: filepath.Base(path),
		RelPath:  filepath.ToSlash(rel),
		Chapters: chapters,
	}}
}

// scanSubtreeAdded walks a newly appeared directory and returns one EventAdded
// per .n4l file inside it (respecting the denylist for nested directories).
func scanSubtreeAdded(root, dir string) []CollectionEvent {
	var out []CollectionEvent
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			return fs.SkipDir
		}
		if d.IsDir() {
			if path != dir && isExcludedDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".n4l") {
			return nil
		}
		out = append(out, CollectionEvent{Kind: EventAdded, Path: path, Chapters: summaryFor(root, path)})
		return nil
	})
	return out
}
