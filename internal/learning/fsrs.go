package learning

import (
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

// gradeToRating maps our Grade to the library's Rating.
func gradeToRating(g Grade) fsrs.Rating {
	switch g {
	case GradeAgain:
		return fsrs.Again
	case GradeHard:
		return fsrs.Hard
	case GradeEasy:
		return fsrs.Easy
	default:
		return fsrs.Good
	}
}

// Schedule applies FSRS to prev (the zero value is a new card) and returns the
// updated scheduling state.
//
// API note: v3.3+ exposes Repeat on the FSRS struct (via NewFSRS), not on
// Parameters directly. RecordLog is map[Rating]SchedulingInfo.
func Schedule(prev SchedState, grade Grade, now time.Time) (SchedState, error) {
	card := fsrs.NewCard()
	if prev.Reps > 0 || !prev.Due.IsZero() {
		card.Due = prev.Due
		card.Stability = prev.Stability
		card.Difficulty = prev.Difficulty
		card.Reps = uint64(prev.Reps)
		card.Lapses = uint64(prev.Lapses)
		card.State = fsrs.State(prev.State)
		card.LastReview = prev.LastReview
	}
	f := fsrs.NewFSRS(fsrs.DefaultParam())
	recordLog := f.Repeat(card, now)
	c := recordLog[gradeToRating(grade)].Card
	return SchedState{
		Stability:  c.Stability,
		Difficulty: c.Difficulty,
		Due:        c.Due,
		LastReview: now,
		Reps:       int(c.Reps),
		Lapses:     int(c.Lapses),
		State:      int(c.State),
	}, nil
}
