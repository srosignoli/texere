package learning

import (
	"testing"
	"time"
)

func TestScheduleNewCardGoodIsDueInFuture(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	out, err := Schedule(SchedState{}, GradeGood, now)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if !out.Due.After(now) {
		t.Fatalf("Due=%v not after now=%v", out.Due, now)
	}
	if out.Reps != 1 {
		t.Fatalf("Reps=%d want 1", out.Reps)
	}
	if out.LastReview.Unix() != now.Unix() {
		t.Fatalf("LastReview=%v want %v", out.LastReview, now)
	}
	if out.Stability <= 0 {
		t.Fatalf("Stability=%v want > 0", out.Stability)
	}
}

func TestScheduleAgainShorterThanEasy(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	again, _ := Schedule(SchedState{}, GradeAgain, now)
	easy, _ := Schedule(SchedState{}, GradeEasy, now)
	if !again.Due.Before(easy.Due) {
		t.Fatalf("Again due %v should be before Easy due %v", again.Due, easy.Due)
	}
}
