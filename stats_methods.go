package main

import (
	"errors"
	"time"
)

// LogReadingSession records time spent reading one document.
//
// Called by ReadingView when it leaves a document, and only then: a peek is
// suppressed at the call site alongside the three progress writes, so glancing
// at an extract never counts as reading.
func (a *App) LogReadingSession(id string, seconds int) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	if seconds <= 0 {
		return nil // opening and immediately leaving is not a session
	}
	return a.learnStore.LogActivity("read", id, "", seconds, time.Now())
}
