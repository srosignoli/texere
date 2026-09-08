//go:build demo

// Demo-only bindings for scripted, unattended demo recordings.
//
// The four importers open a native file dialog owned by the Go process. A
// browser-driven script (see docs/demos/) cannot see or dismiss that dialog, so
// a scripted run would block on the very first step of a study session.
//
// This file exposes the ingest half directly, keyed to an explicit path. It is
// compiled ONLY under -tags demo:
//
//	wails dev   -tags webkit2_41,demo   # seam present
//	wails build -tags webkit2_41        # seam absent from the binary
//
// Production builds never include this file, so Wails generates no JS binding
// for these methods and nothing in the webview can call them. That absence is
// enforced by TestNoDemoBindingsInAProductionBuild in demo_seam_absent_test.go.
//
// Do not add a runtime flag or environment variable as an alternative gate: a
// dormant-but-present binding is still a binding that lets any script in the
// webview read any file the user can read.
//
// CAUTION: a demo-tagged `wails dev` regenerates the tracked JS bindings under
// frontend/wailsjs/ and adds Demo* shims to them. Run
// `git checkout -- frontend/wailsjs/` afterwards;
// TestGeneratedBindingsCarryNoDemoShims fails if you forget.
package main

import (
	"errors"
	"math/rand"
	"time"

	"texere/internal/learning"
)

// DemoImportTextSource ingests the text/HTML file at path, skipping the dialog
// that ImportTextSource would open.
func (a *App) DemoImportTextSource(path string) (*ReadingView, error) {
	return a.importTextFrom(path)
}

// DemoImportPdfSource ingests the PDF at path, skipping the dialog that
// ImportPdfSource would open.
func (a *App) DemoImportPdfSource(path string) (*ReadingView, error) {
	return a.importPdfFrom(path)
}

// DemoImportImage ingests the image at path, skipping the dialog that
// ImportImage would open. Returns {asset, mime}, which is what the notebook's
// "+ Scan" button feeds to newImageCell.
func (a *App) DemoImportImage(path string) (map[string]any, error) {
	return a.importImageFrom(path)
}

// DemoImportAudio ingests the audio file at path, skipping the dialog that
// ImportAudio would open. Returns {asset, mime, durationMs} — what the audio
// cell's Import button hands to the cell.
//
// The recording half of that cell cannot be scripted at all: it goes through
// MediaRecorder and a real capture device, which this seam deliberately does
// not fake. Importing a file is the other half of the same feature, and it is
// the half someone arriving with a memo from their phone actually uses.
func (a *App) DemoImportAudio(path string) (map[string]any, error) {
	return a.importAudioFrom(path)
}

// DemoSeedHistory replays a deterministic study history so the statistics panel
// has something to show. Without it the demo collection is one session old, and
// half the panel reads "Not enough data yet" while the habit charts are a single
// spike at today.
//
// NOTHING HERE FABRICATES A STATISTIC. Every interval, stability, difficulty and
// due date is produced by learning.Schedule — the same call a real review makes
// — applied to the collection's real cards. What is invented is only which grade
// a person pressed on which evening, which is behaviour rather than app output,
// and it is generated from `seed` so two runs produce the same history.
//
// Out of reach through the exported store, and therefore NOT faked: items.
// created_at (the cards-per-week chart) and an extract's creation time (the
// oldest-unprocessed figure). Those stay as they are rather than being written
// behind the store's back.
// fsrs.StateReview, without importing the library here for one constant.
const stateReview = 2

func (a *App) DemoSeedHistory(days, seed int) (map[string]any, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	if days <= 0 || days > 400 {
		days = 42
	}
	// Snapshot the ids once: after the first backdated review these cards have
	// due dates, and re-querying the due queue mid-replay would return a moving
	// target.
	cards, err := a.learnStore.DueQueue(time.Now(), 500)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, errors.New("no cards to build a history from")
	}

	rnd := rand.New(rand.NewSource(int64(seed)))
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// One card is answered badly every time, so the panel has a leech to point
	// at. Five lapses is the threshold the Leeches figure counts.
	leech := cards[len(cards)-1].ID
	// Cards are introduced gradually rather than all on day one, which is what
	// makes the reviews-per-day chart look like study instead of a wall.
	introduced := map[string]time.Time{}
	for i, c := range cards {
		introduced[c.ID] = midnight.AddDate(0, 0, -days+(i*days)/(len(cards)*2+1))
	}

	reviews, lapses := 0, 0
	// Down to and INCLUDING today. streaks() returns the run ending on the last
	// day of the window, which is today, so a history that stops at yesterday
	// reports a current streak of zero however long the run before it was.
	for d := days; d >= 0; d-- {
		day := midnight.AddDate(0, 0, -d).Add(19 * time.Hour)
		if day.After(now) {
			day = now.Add(-2 * time.Minute)
		}
		// Two rest days a fortnight, so the current streak is shorter than the
		// longest one and the habit chart has gaps in it. The most recent week is
		// never skipped: a rest day landing on yesterday leaves a CURRENT STREAK
		// of zero, which reads as a collection someone abandoned.
		if d > 6 && (d%13 == 0 || d%13 == 1) {
			continue
		}
		reviewedToday := false
		for _, c := range cards {
			if introduced[c.ID].After(day) {
				continue
			}
			st, ok, err := a.learnStore.LoadSched(c.ID)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			// Only review what the scheduler says is due by that evening.
			if st.Reps > 0 && st.Due.After(day) {
				continue
			}
			grade := learning.GradeGood
			switch {
			case c.ID == leech:
				// A card answered Again EVERY time never leaves the learning state,
				// and FSRS counts a lapse only against a card in review — so
				// "always fail" produces a leech count of zero, which is the
				// opposite of what it looks like it will do. Let it graduate, then
				// fail it whenever it comes back for review.
				if st.State == stateReview {
					grade = learning.GradeAgain
				}
			case rnd.Float64() < 0.08:
				grade = learning.GradeAgain
			case rnd.Float64() < 0.20:
				grade = learning.GradeHard
			case rnd.Float64() < 0.15:
				grade = learning.GradeEasy
			}
			next, err := learning.Schedule(st, grade, day)
			if err != nil {
				return nil, err
			}
			elapsed := 3000 + rnd.Intn(9000)
			if err := a.learnStore.ApplyReview(c.ID, next, grade, elapsed, st.State, day); err != nil {
				return nil, err
			}
			reviews++
			reviewedToday = true
			if grade == learning.GradeAgain {
				lapses++
			}
		}
		// Keep the last week unbroken. By then the surviving cards have intervals
		// of several days, so a day can pass with nothing due — which leaves a
		// CURRENT STREAK of zero beside a longest streak of twelve, and reads as
		// a collection that was abandoned rather than one in use. On a quiet day
		// inside that window, review whatever is closest to due.
		if d <= 7 && !reviewedToday {
			var soonest string
			var best time.Time
			for _, c := range cards {
				st, ok, err := a.learnStore.LoadSched(c.ID)
				if err != nil || !ok || st.Reps == 0 {
					continue
				}
				if soonest == "" || st.Due.Before(best) {
					soonest, best = c.ID, st.Due
				}
			}
			if soonest != "" {
				st, _, _ := a.learnStore.LoadSched(soonest)
				next, err := learning.Schedule(st, learning.GradeGood, day)
				if err != nil {
					return nil, err
				}
				if err := a.learnStore.ApplyReview(soonest, next, learning.GradeGood, 4000, st.State, day); err != nil {
					return nil, err
				}
				reviews++
			}
		}

		// Reading and processing on most study days, so the pipeline charts show
		// a habit rather than one spike.
		if rnd.Float64() < 0.7 {
			_ = a.learnStore.LogActivity("read", "", "demo", 10+rnd.Intn(25), day)
		}
		if rnd.Float64() < 0.5 {
			_ = a.learnStore.LogActivity("process", "", "demo", 1, day)
		}
	}
	return map[string]any{"cards": len(cards), "reviews": reviews, "lapses": lapses, "days": days}, nil
}
