package content

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// WatchCollection starts a filesystem watcher on dir. Returns a *CollectionWatcher
// whose Events channel emits CollectionEvent values as .n4l files are added, removed,
// renamed, or modified.
//
// The watcher performs a ScanCollection internally on Added/Renamed events and attaches
// the resulting ChapterSummary to the emitted event.
// Modified events are emitted for content changes to existing files.
// Non-.n4l events are silently filtered.
//
// Call CollectionWatcher.Close() to stop the watcher and close both channels.
// Returns an error if dir does not exist or the OS watcher cannot be initialised.
func WatchCollection(dir string) (*CollectionWatcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("content: WatchCollection: %w", err)
	}
	if err := fw.Add(dir); err != nil {
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
				if !strings.EqualFold(filepath.Ext(ev.Name), ".n4l") {
					continue
				}
				ce := translateEvent(ev, dir)
				if ce == nil {
					continue
				}
				events <- *ce
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

// translateEvent maps an fsnotify.Event to a CollectionEvent, performing a chapter
// scan for Added/Renamed events. Returns nil for events that should not be emitted.
func translateEvent(ev fsnotify.Event, _ string) *CollectionEvent {
	switch {
	case ev.Has(fsnotify.Create):
		ce := &CollectionEvent{Kind: EventAdded, Path: ev.Name}
		chapters, err := scanChapterTitles(ev.Name)
		if err == nil {
			ce.Chapters = []ChapterSummary{{
				FilePath: ev.Name,
				FileName: filepath.Base(ev.Name),
				Chapters: chapters,
			}}
		}
		return ce

	case ev.Has(fsnotify.Remove):
		return &CollectionEvent{Kind: EventRemoved, Path: ev.Name}

	case ev.Has(fsnotify.Rename):
		ce := &CollectionEvent{Kind: EventRenamed, Path: ev.Name}
		chapters, err := scanChapterTitles(ev.Name)
		if err == nil {
			ce.Chapters = []ChapterSummary{{
				FilePath: ev.Name,
				FileName: filepath.Base(ev.Name),
				Chapters: chapters,
			}}
		}
		return ce

	case ev.Has(fsnotify.Write):
		// Re-scan chapter titles so the frontend can update the file tree
		// when chapters are added/removed/renamed by an editor save (or by
		// an external tool). The scan is cheap — it only reads the lines
		// that look like chapter headers, not the whole node body.
		ce := &CollectionEvent{Kind: EventModified, Path: ev.Name}
		chapters, err := scanChapterTitles(ev.Name)
		if err == nil {
			ce.Chapters = []ChapterSummary{{
				FilePath: ev.Name,
				FileName: filepath.Base(ev.Name),
				Chapters: chapters,
			}}
		}
		return ce
	}
	return nil
}
