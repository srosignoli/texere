package main

import (
	"testing"
	"time"

	"texere/internal/learning"
)

func TestLogReadingSessionRecordsMinutes(t *testing.T) {
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := &App{collectionDir: dir, learnStore: st}

	if err := a.LogReadingSession("r1", 600); err != nil {
		t.Fatalf("LogReadingSession: %v", err)
	}
	got, err := st.ActivityWeeks("read", 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 10 {
		t.Errorf("logged %d minutes, want 10", got[0])
	}
}

// A zero- or negative-length session is noise, not data: it would inflate the
// session count in the pipeline panel while contributing no time.
func TestLogReadingSessionIgnoresEmptySessions(t *testing.T) {
	dir := t.TempDir()
	st, _ := learning.Open(dir)
	t.Cleanup(func() { st.Close() })
	a := &App{collectionDir: dir, learnStore: st}

	if err := a.LogReadingSession("r1", 0); err != nil {
		t.Fatalf("a zero session must be ignored, not an error: %v", err)
	}
	if _, ok, _ := st.ActivitySince(); ok {
		t.Error("a zero-second session was written to the log")
	}
}

func TestRescheduleExtractLogsItsAction(t *testing.T) {
	dir := t.TempDir()
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.UpsertExtract(learning.Extract{
		ID: "e1", FilePath: "/c/n.n4l", Text: "x", Due: time.Now(), Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	a := &App{collectionDir: dir, learnStore: st}

	if err := a.DismissExtract("e1"); err != nil {
		t.Fatalf("DismissExtract: %v", err)
	}
	got, err := st.ActivityWeeks("process", 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 1 {
		t.Errorf("processed count = %d, want 1 — processing must be logged", got[0])
	}
}
