// Package collection owns a Session: one Collection while it is open in Texere.
//
// A Session holds everything that exists only while a Collection is open: the
// learning store, graph search, the explorer, the filesystem watcher, the file
// open in the editor and the set of paths the app is writing itself. Callers
// reach the stores through Read, which guarantees none of them closes under
// the caller. See CONTEXT.md for the vocabulary.
package collection

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"texere/internal/collectionpath"
	"texere/internal/content"
	"texere/internal/explorer"
	"texere/internal/learning"
	"texere/internal/search"
	"texere/internal/sitting"
)

// ErrNoCollection is returned when no Session is open, or the one asked for has closed.
var ErrNoCollection = errors.New("no collection open")

// Event names sent through Notify. They are part of the frontend contract.
const (
	EventCollectionChange       = "collection:change"
	EventFileModifiedExternally = "file:modified-externally"
	EventIDsInjected            = "file:ids-injected"
)

// Notify receives the Session's events. It is called from the watcher
// goroutine as well as from callers' goroutines, so it must be safe for
// concurrent use and must not call back into the Session.
type Notify func(name string, data any)

// Session is one Collection while it is open.
type Session struct {
	dir      string
	registry *content.SSTRegistry
	notify   Notify

	// mu guards the lifecycle of the stores, not access to their data (each
	// store does its own locking). Read holds it shared; RebuildGraph and
	// Close hold it exclusively, so no store is closed or swapped under a
	// running Read.
	mu       sync.RWMutex
	closed   bool
	learn    *learning.Store
	search   *search.SearchService
	explorer *explorer.ExplorerService
	sittings *sitting.Tracker

	watcher   *content.CollectionWatcher
	watchDone chan struct{} // closed when consumeWatchEvents returns

	// fileMu guards openFile and selfWrites. It is separate from mu so the
	// watcher goroutine never waits on a Read or a rebuild, which lets Close
	// wait for that goroutine while it holds mu.
	fileMu     sync.Mutex
	openFile   string
	selfWrites map[string]int // ref-counted paths the app is writing itself
}

// Open opens dir as a Session and returns it with the Collection's chapter
// summaries. It fails if dir is not a directory, cannot be scanned, or its
// learning store cannot be opened. A watcher that cannot start is logged and
// the Session runs without live updates.
func Open(dir string, reg *content.SSTRegistry, notify Notify) (*Session, []content.ChapterSummary, error) {
	if notify == nil {
		notify = func(string, any) {}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("collection: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("collection: %s is not a directory", dir)
	}
	sums, err := content.ScanCollection(dir)
	if err != nil {
		return nil, nil, err
	}
	ls, err := learning.Open(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("collection: open learning store: %w", err)
	}
	s := &Session{
		dir:        dir,
		registry:   reg,
		notify:     notify,
		learn:      ls,
		selfWrites: map[string]int{},
	}
	s.search = search.NewSearchService(dir, reg)
	s.explorer = explorer.NewExplorerService(s.search, reg)
	s.sittings = sitting.New(sitting.Deps{
		Store:     ls,
		SelfWrite: s.SelfWrite,
		SaveNotes: func(path, text string) (string, error) {
			res, err := content.Save(path, text, reg, dir)
			return res.FinalContent, err
		},
	})

	w, err := content.WatchCollection(dir)
	if err != nil {
		log.Printf("collection: no live updates, watcher unavailable: %v", err)
	} else {
		s.watcher = w
		s.watchDone = make(chan struct{})
		go s.consumeWatchEvents(w)
	}
	return s, sums, nil
}

// Dir is the Collection's absolute directory. It never changes for a Session.
func (s *Session) Dir() string { return s.dir }

// Close stops the watcher, waits for its goroutine, and closes every store.
// It waits for running Reads to finish. Calling it again does nothing.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.watcher != nil {
		if err := s.watcher.Close(); err != nil {
			log.Printf("collection: watcher close: %v", err)
		}
		<-s.watchDone
	}
	// Before the learning store closes: ending a Sitting writes to it.
	s.sittings.EndAll()
	s.search.Close()
	_ = s.explorer.Close()
	s.learn.Close()
}

// Read runs fn with the Session's handles, holding them open for the call.
// The handles must not be kept or used after fn returns. fn must not call any
// Session method (see Handles). Returns ErrNoCollection once the Session has closed.
func (s *Session) Read(fn func(h Handles) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrNoCollection
	}
	return fn(Handles{s: s})
}

// SelfWrite runs fn with path marked as being written by the app, so the
// watcher does not report the change as external. Safe to call inside Read
// through Handles.SelfWrite. An empty path runs fn unmarked.
func (s *Session) SelfWrite(path string, fn func() error) error {
	if path == "" {
		return fn()
	}
	s.fileMu.Lock()
	s.selfWrites[path]++
	s.fileMu.Unlock()
	defer func() {
		s.fileMu.Lock()
		if n := s.selfWrites[path]; n > 1 {
			s.selfWrites[path] = n - 1
		} else {
			delete(s.selfWrites, path)
		}
		s.fileMu.Unlock()
	}()
	return fn()
}

// SetOpenFile records the file open in the editor, whose external changes
// and ID injections are reported to the frontend.
func (s *Session) SetOpenFile(path string) {
	s.fileMu.Lock()
	s.openFile = path
	s.fileMu.Unlock()
}

func (s *Session) currentOpenFile() string {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	return s.openFile
}

func (s *Session) isExternalChangeToOpenFile(path string) bool {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	_, self := s.selfWrites[path]
	return path == s.openFile && !self
}

// consumeWatchEvents forwards watcher events until the watcher closes.
func (s *Session) consumeWatchEvents(w *content.CollectionWatcher) {
	defer close(s.watchDone)
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			switch ev.Kind {
			case content.EventAdded, content.EventRemoved, content.EventRenamed:
				s.notify(EventCollectionChange, ev)
			case content.EventModified:
				// Always tell the file tree, whoever wrote the file; only an
				// external write to the open file gets the reload prompt.
				s.notify(EventCollectionChange, ev)
				if s.isExternalChangeToOpenFile(ev.Path) {
					s.notify(EventFileModifiedExternally, map[string]string{"path": ev.Path})
				}
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Printf("collection: watcher: %v", err)
		}
	}
}

// Handles are the Session's parts, valid only inside the Read call that
// produced them. Code holding Handles must not call Session methods or App
// bindings: a nested Read deadlocks once a rebuild or a switch is waiting.
type Handles struct{ s *Session }

// Dir is the Collection's absolute directory.
func (h Handles) Dir() string { return h.s.dir }

// Learning is the study store. Never nil.
func (h Handles) Learning() *learning.Store { return h.s.learn }

// Search is graph search. Never nil; it opens the graph lazily.
func (h Handles) Search() *search.SearchService { return h.s.search }

// Explorer is the 2.5D explorer. Never nil.
func (h Handles) Explorer() *explorer.ExplorerService { return h.s.explorer }

// Path turns a collection-relative path (slash- or OS-separated) into an
// absolute one, refusing anything that escapes the Collection.
func (h Handles) Path(rel string) (string, error) {
	return collectionpath.Resolve(h.s.dir, filepath.FromSlash(rel))
}

// Sittings holds the open Sitting. Never nil.
func (h Handles) Sittings() *sitting.Tracker { return h.s.sittings }

// SelfWrite is Session.SelfWrite, usable inside a Read.
func (h Handles) SelfWrite(path string, fn func() error) error { return h.s.SelfWrite(path, fn) }
