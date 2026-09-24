package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/collection"
	"texere/internal/content"
)

// session returns the open Session, or collection.ErrNoCollection.
//
// Never call it (or withCollection, inCollection, currentDir, selfWrite) from
// inside a Read callback: switchTo holds curMu while it waits for Reads to
// finish, so the nested call would deadlock.
func (a *App) session() (*collection.Session, error) {
	a.curMu.Lock()
	defer a.curMu.Unlock()
	if a.cur == nil {
		return nil, collection.ErrNoCollection
	}
	return a.cur, nil
}

// withCollection runs fn against the open Session's handles.
func (a *App) withCollection(fn func(h collection.Handles) error) error {
	s, err := a.session()
	if err != nil {
		return err
	}
	return s.Read(fn)
}

// inCollection is withCollection for callbacks that return a value.
func inCollection[T any](a *App, fn func(h collection.Handles) (T, error)) (T, error) {
	var out T
	err := a.withCollection(func(h collection.Handles) error {
		var err error
		out, err = fn(h)
		return err
	})
	return out, err
}

// currentDir is the open Collection's directory. It holds no lock afterwards,
// so it suits calls that only need a path and then block (dialogs, downloads).
func (a *App) currentDir() (string, error) {
	s, err := a.session()
	if err != nil {
		return "", err
	}
	return s.Dir(), nil
}

// selfWrite runs fn with path marked as an app write when a Collection is
// open, and runs it plainly otherwise (a bare file has no watcher to fool).
// Outside callbacks only; inside one use h.SelfWrite.
func (a *App) selfWrite(path string, fn func() error) error {
	s, err := a.session()
	if err != nil {
		return fn()
	}
	return s.SelfWrite(path, fn)
}

// emit sends a frontend event. A no-op before startup and in tests.
func (a *App) emit(name string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// switchTo opens dir as a new Session and makes it current.
//
// If dir fails to open, the current Session stays open. Otherwise the old
// Session is closed before the new one is published: SSTorytime-KV keeps
// process-wide graph state, so a query still running on the old Session must
// finish before any query on the new one can start. Callers arriving during
// the swap wait on curMu.
func (a *App) switchTo(dir string) ([]content.ChapterSummary, error) {
	s, sums, err := collection.Open(dir, a.registry, a.emit)
	if err != nil {
		return nil, err
	}
	a.curMu.Lock()
	if a.cur != nil {
		a.cur.Close()
	}
	a.cur = s
	a.curMu.Unlock()
	return sums, nil
}

// closeSession closes the open Session, if any.
func (a *App) closeSession() {
	a.curMu.Lock()
	if a.cur != nil {
		a.cur.Close()
		a.cur = nil
	}
	a.curMu.Unlock()
}
