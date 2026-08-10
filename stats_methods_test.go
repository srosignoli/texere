package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"texere/internal/content"
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

// PromoteExtract does not go through rescheduleExtract — it has its own path
// (SeedBasicCardNode / InsertMarkerAboveNode + learning.Sync) and needs its own
// log line. Without it, promotion — the most meaningful processing outcome —
// would never show up in the pipeline panel.
func TestPromoteExtractLogsItsAction(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	p := filepath.Join(dir, "reading", "k.n4l")
	// The (from) arrow must be inline on the same node line — separate-line (from)
	// is parsed as a root node with empty text and cannot produce a card.
	// This matches the format produced by content.AppendExtract.
	os.WriteFile(p, []byte("- K\n# @id e1\nSystem 1 is fast (from) \"sources/k.txt char=0\"\n"), 0o644)
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	reg := &content.SSTRegistry{Arrows: map[string]content.ArrowDef{"from": {Code: "from", LongName: "from"}}}
	a := &App{collectionDir: dir, learnStore: st, registry: reg}

	if _, err := learning.Sync(st, []string{p}, reg); err != nil {
		t.Fatal(err)
	}
	q, err := a.GetExtractQueue(10)
	if err != nil || len(q) != 1 {
		t.Fatalf("GetExtractQueue len=%d err=%v want 1", len(q), err)
	}

	if err := a.PromoteExtract(q[0].ID, "card"); err != nil {
		t.Fatalf("PromoteExtract: %v", err)
	}
	got, err := st.ActivityWeeks("process", 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 1 {
		t.Errorf("processed count = %d, want 1 — promotion must be logged", got[0])
	}
}
