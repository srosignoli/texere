package sitting

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"texere/internal/content"
	"texere/internal/learning"
)

// ErrNoSitting is returned by Progress for an id that is not the open Sitting.
var ErrNoSitting = errors.New("sitting: no such open sitting")

// Deps are what a Tracker writes through. SelfWrite brackets every file write
// so the Collection's watcher does not report it as external; SaveNotes saves a
// notes buffer the way the editor does and returns the final on-disk content.
type Deps struct {
	Store     *learning.Store
	SelfWrite func(path string, fn func() error) error
	SaveNotes func(path, text string) (final string, err error)
	Now       func() time.Time // nil = time.Now
}

// Tracker holds the open Sitting. Its methods are serialised by one lock, so a
// Begin never reads a position before an earlier End has written it.
type Tracker struct {
	d    Deps
	mu   sync.Mutex
	open *sitting
}

type sitting struct {
	id        string
	sourceID  string
	notesPath string // absolute
	started   time.Time
	lastPos   string // last position reported, "" if none
}

// New returns a Tracker with nothing open.
func New(d Deps) *Tracker {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Tracker{d: d}
}

var validActions = map[string]bool{"continue": true, "later": true, "done": true}

// Begin opens a Sitting on sourceID and returns its id with the Source as it is
// now, NotesPath resolved to an absolute path. peekAnchor is the passage a peek
// opens at, or "" for a normal Sitting.
func (t *Tracker) Begin(sourceID, peekAnchor string) (string, learning.ReadingElement, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, ok, err := t.d.Store.GetReading(sourceID)
	if err != nil {
		return "", learning.ReadingElement{}, fmt.Errorf("sitting: begin: %w", err)
	}
	if !ok {
		return "", learning.ReadingElement{}, fmt.Errorf("sitting: begin: Source %q not found", sourceID)
	}
	notesPath, err := t.d.Store.Resolve(r.NotesPath)
	if err != nil {
		return "", learning.ReadingElement{}, fmt.Errorf("sitting: begin: %w", err)
	}
	id, err := content.GenerateShortID()
	if err != nil {
		return "", learning.ReadingElement{}, err
	}
	t.open = &sitting{id: id, sourceID: sourceID, notesPath: notesPath, started: t.d.Now()}
	r.NotesPath = notesPath
	return id, r, nil
}

// Progress records position as the open Sitting's progress in the database and
// returns its Source id. The notes file is not touched until End.
func (t *Tracker) Progress(sittingID, position string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.find(sittingID)
	if s == nil {
		return "", ErrNoSitting
	}
	if position != "" {
		s.lastPos = position
		if err := t.setProgress(s.sourceID, position); err != nil {
			return s.sourceID, err
		}
	}
	return s.sourceID, nil
}

// End ends the Sitting. position is the last position the view knows, or "".
// notes is the unsaved notes buffer, or nil when it is unchanged; the final
// notes content is returned when notes was given.
//
// If the notes cannot be saved, End returns the error and the Sitting stays
// open. Everything after the notes save is best-effort. Ending a Sitting that
// is not open does nothing.
func (t *Tracker) End(sittingID, position, action string, notes *string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !validActions[action] {
		return "", fmt.Errorf("sitting: end: unknown action %q", action)
	}
	s := t.find(sittingID)
	if s == nil {
		return "", nil
	}
	return t.end(s, position, action, notes)
}

func (t *Tracker) find(id string) *sitting {
	if t.open == nil || id == "" || t.open.id != id {
		return nil
	}
	return t.open
}

// end runs the end order with t.mu held: notes, marker and progress, reading
// time, reschedule.
func (t *Tracker) end(s *sitting, position, action string, notes *string) (string, error) {
	var final string
	if notes != nil {
		err := t.d.SelfWrite(s.notesPath, func() error {
			var err error
			final, err = t.d.SaveNotes(s.notesPath, *notes)
			return err
		})
		if err != nil {
			return "", fmt.Errorf("sitting: save notes: %w", err)
		}
	}
	t.open = nil
	now := t.d.Now()

	if position != "" {
		s.lastPos = position
	}
	if s.lastPos != "" {
		// After the notes save, so the save cannot overwrite the marker.
		err := t.d.SelfWrite(s.notesPath, func() error {
			return content.UpdateProgressMarker(s.notesPath, s.lastPos)
		})
		if err != nil {
			log.Printf("sitting: progress marker: %v", err)
		}
		if err := t.setProgress(s.sourceID, s.lastPos); err != nil {
			log.Printf("sitting: progress: %v", err)
		}
	}
	if secs := int(now.Sub(s.started).Round(time.Second) / time.Second); secs > 0 {
		if err := t.d.Store.LogActivity("read", s.sourceID, "", secs, now); err != nil {
			log.Printf("sitting: log reading time: %v", err)
		}
	}
	t.reschedule(s.sourceID, action, now)
	return final, nil
}

func (t *Tracker) setProgress(sourceID, position string) error {
	r, ok, err := t.d.Store.GetReading(sourceID)
	if err != nil || !ok {
		return fmt.Errorf("Source %q not found: %v", sourceID, err)
	}
	return t.d.Store.SetReadingProgress(sourceID, position, r.ProjLen)
}

func (t *Tracker) reschedule(sourceID, action string, now time.Time) {
	r, ok, err := t.d.Store.GetReading(sourceID)
	if err != nil || !ok {
		log.Printf("sitting: reschedule: Source %q not found: %v", sourceID, err)
		return
	}
	if err := t.d.Store.ApplyReadingSchedule(sourceID, learning.ScheduleReading(r, action, now)); err != nil {
		log.Printf("sitting: reschedule: %v", err)
	}
}
