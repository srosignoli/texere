package learning

import (
	"testing"
	"time"
)

func TestReadingCRUDAndSchedule(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_000_000, 0)

	r := ReadingElement{ID: "r1", RefPath: "sources/k.txt", NotesPath: "/c/reading/k.n4l",
		Title: "K", Position: "char=0", ProjLen: 10000, IntervalDays: 1, Done: false}
	if err := s.UpsertReading(r); err != nil {
		t.Fatalf("UpsertReading: %v", err)
	}
	if err := s.SetReadingProgress("r1", "char=4180", 10000); err != nil {
		t.Fatalf("SetReadingProgress: %v", err)
	}
	got, ok, err := s.GetReading("r1")
	if err != nil || !ok || got.Position != "char=4180" {
		t.Fatalf("GetReading got=%+v ok=%v err=%v", got, ok, err)
	}

	// Schedule "continue": interval grows 1 -> 2 (round(1*1.5)=2), due in the future.
	sched := ScheduleReading(got, "continue", now)
	if sched.IntervalDays != 2 {
		t.Fatalf("interval=%d want 2", sched.IntervalDays)
	}
	if !sched.Due.After(now) {
		t.Fatalf("due %v not after now", sched.Due)
	}
	if err := s.ApplyReadingSchedule("r1", sched); err != nil {
		t.Fatalf("ApplyReadingSchedule: %v", err)
	}

	// "done" marks done.
	d := ScheduleReading(sched, "done", now)
	if !d.Done {
		t.Fatal("expected Done")
	}

	// Interval cap at 30.
	big := ReadingElement{IntervalDays: 25}
	if g := ScheduleReading(big, "continue", now); g.IntervalDays != 30 {
		t.Fatalf("capped interval=%d want 30", g.IntervalDays)
	}

	list, err := s.ListReading()
	if err != nil || len(list) != 1 {
		t.Fatalf("ListReading len=%d err=%v want 1", len(list), err)
	}
}

func TestReadingMediumAndSourceURLRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := ReadingElement{ID: "w1", RefPath: "sources/a.html", NotesPath: "/c/reading/a.n4l",
		Title: "A", IntervalDays: 1, Medium: "web", SourceURL: "https://example.com/a"}
	if err := s.UpsertReading(r); err != nil {
		t.Fatalf("UpsertReading: %v", err)
	}
	got, ok, err := s.GetReading("w1")
	if err != nil || !ok {
		t.Fatalf("GetReading ok=%v err=%v", ok, err)
	}
	if got.Medium != "web" || got.SourceURL != "https://example.com/a" {
		t.Fatalf("got Medium=%q SourceURL=%q", got.Medium, got.SourceURL)
	}
	// A row inserted without a medium defaults to 'text'.
	r2 := ReadingElement{ID: "t1", RefPath: "sources/b.txt", NotesPath: "/c/reading/b.n4l", Title: "B", IntervalDays: 1}
	s.UpsertReading(r2)
	g2, _, _ := s.GetReading("t1")
	if g2.Medium != "text" {
		t.Fatalf("default medium=%q want text", g2.Medium)
	}
}

func TestMigrationV1toV2Additive(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir) // fresh open creates v2 directly
	if err != nil {
		t.Fatal(err)
	}
	// items table (v1) and reading_elements (v2) both exist.
	if _, err := s.db.Exec(`INSERT INTO items
		(id,node_id,file_path,card_type,front,back,active,reps,lapses,state,created_at)
		VALUES ('i','n','/f','cloze','f','b',1,0,0,0,0)`); err != nil {
		t.Fatalf("items missing after v2: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO reading_elements
		(id,ref_path,notes_path,title,interval_days,done,created_at)
		VALUES ('r','s','n','t',1,0,0)`); err != nil {
		t.Fatalf("reading_elements missing: %v", err)
	}
	s.Close()
}

// The same rule the due-reading SQL applies: not done, and never scheduled or
// due by now.
func TestReadingDue(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		r    ReadingElement
		want bool
	}{
		{"never scheduled", ReadingElement{}, true},
		{"due yesterday", ReadingElement{Due: now.Add(-24 * time.Hour)}, true},
		{"due right now", ReadingElement{Due: now}, true},
		{"due tomorrow", ReadingElement{Due: now.Add(24 * time.Hour)}, false},
		{"done, though overdue", ReadingElement{Due: now.Add(-24 * time.Hour), Done: true}, false},
	}
	for _, c := range cases {
		if got := ReadingDue(c.r, now); got != c.want {
			t.Errorf("%s: ReadingDue = %v, want %v", c.name, got, c.want)
		}
	}
}
