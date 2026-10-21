package sitting

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"texere/internal/content"
	"texere/internal/learning"
)

// fixture is a temp Collection holding one Source with collection-relative
// stored paths, and a Tracker over it driven by a settable clock.
type fixture struct {
	dir      string
	notes    string // absolute notes path
	sourceID string
	store    *learning.Store
	tr       *Tracker
	now      time.Time
	saveErr  error // returned by SaveNotes when set
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"sources", "reading"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "sources", "k.txt"), []byte("some article text"), 0o644); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(dir, "reading", "k.n4l")
	id, err := content.CreateReadingNotes(notes, "sources/k.txt", "k")
	if err != nil {
		t.Fatal(err)
	}
	st, err := learning.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.UpsertReading(learning.ReadingElement{
		ID: id, RefPath: "sources/k.txt", NotesPath: "reading/k.n4l", Title: "k",
		Position: "char=0", ProjLen: 10000, IntervalDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	f := &fixture{dir: dir, notes: notes, sourceID: id, store: st, now: time.Now().Truncate(time.Second)}
	f.tr = New(Deps{
		Store:     st,
		SelfWrite: func(_ string, fn func() error) error { return fn() },
		SaveNotes: func(path, text string) (string, error) {
			if f.saveErr != nil {
				return "", f.saveErr
			}
			return text, os.WriteFile(path, []byte(text), 0o644)
		},
		Now: func() time.Time { return f.now },
	})
	return f
}

func (f *fixture) row(t *testing.T) learning.ReadingElement {
	t.Helper()
	r, ok, err := f.store.GetReading(f.sourceID)
	if err != nil || !ok {
		t.Fatalf("GetReading: ok=%v err=%v", ok, err)
	}
	return r
}

func (f *fixture) file(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(f.notes)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (f *fixture) readMinutes(t *testing.T) int {
	t.Helper()
	w, err := f.store.ActivityWeeks("read", 1, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return w[0].Count
}

func (f *fixture) begin(t *testing.T, peekAnchor string) string {
	t.Helper()
	id, _, err := f.tr.Begin(f.sourceID, peekAnchor)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	return id
}

func TestBeginReturnsTheSourceWithAbsoluteNotesPath(t *testing.T) {
	f := newFixture(t)
	_, src, err := f.tr.Begin(f.sourceID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(src.NotesPath) {
		t.Errorf("NotesPath = %q, want absolute", src.NotesPath)
	}
	if src.Position != "char=0" {
		t.Errorf("Position = %q, want char=0", src.Position)
	}
}

func TestBeginUnknownSourceFails(t *testing.T) {
	f := newFixture(t)
	id, _, err := f.tr.Begin("nope", "")
	if err == nil {
		t.Fatal("Begin on an unknown Source succeeded")
	}
	if _, err := f.tr.Progress(id, "char=1"); !errors.Is(err, ErrNoSitting) {
		t.Errorf("Progress after a failed Begin: err=%v, want ErrNoSitting", err)
	}
}

func TestEndContinueLogsTimeReschedulesAndWritesMarker(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")
	f.now = f.now.Add(600 * time.Second)

	if _, err := f.tr.End(id, "char=500", "continue", nil); err != nil {
		t.Fatal(err)
	}
	r := f.row(t)
	if r.Position != "char=500" {
		t.Errorf("Position = %q, want char=500", r.Position)
	}
	if r.IntervalDays != 2 {
		t.Errorf("IntervalDays = %d, want 2", r.IntervalDays)
	}
	if r.LastRead.Unix() != f.now.Unix() {
		t.Errorf("LastRead = %v, want %v", r.LastRead, f.now)
	}
	if want := f.now.Add(48 * time.Hour).Unix(); r.Due.Unix() != want {
		t.Errorf("Due = %v, want %v", r.Due, time.Unix(want, 0))
	}
	if !strings.Contains(f.file(t), "# @progress char=500") {
		t.Errorf("marker not written:\n%s", f.file(t))
	}
	if got := f.readMinutes(t); got != 10 {
		t.Errorf("logged %d minutes, want 10", got)
	}
}

func TestMarkerSurvivesTheNotesSaveAtEnd(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")
	edited := f.file(t) + "\nA thought I had while reading\n"

	if _, err := f.tr.End(id, "char=500", "continue", &edited); err != nil {
		t.Fatal(err)
	}
	got := f.file(t)
	if !strings.Contains(got, "A thought I had while reading") {
		t.Errorf("notes edit lost:\n%s", got)
	}
	if !strings.Contains(got, "# @progress char=500") || strings.Contains(got, "# @progress char=0") {
		t.Errorf("notes save overwrote the marker:\n%s", got)
	}
}

func TestProgressUpdatesTheDatabaseNotTheFile(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")
	before := f.file(t)

	src, err := f.tr.Progress(id, "page=3")
	if err != nil {
		t.Fatal(err)
	}
	if src != f.sourceID {
		t.Errorf("Progress returned %q, want the Source id %q", src, f.sourceID)
	}
	if f.file(t) != before {
		t.Error("Progress wrote the notes file")
	}
	if p := f.row(t).Position; p != "page=3" {
		t.Errorf("Position = %q, want page=3", p)
	}

	if _, err := f.tr.End(id, "", "continue", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.file(t), "# @progress page=3") {
		t.Errorf("End did not write the last known position:\n%s", f.file(t))
	}
}

func TestEndWithoutAnyPositionWritesNoMarker(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")
	before := f.file(t)
	f.now = f.now.Add(120 * time.Second)

	if _, err := f.tr.End(id, "", "continue", nil); err != nil {
		t.Fatal(err)
	}
	if f.file(t) != before {
		t.Error("a Sitting with no position wrote the notes file")
	}
	r := f.row(t)
	if r.Position != "char=0" {
		t.Errorf("Position = %q, want it left at char=0", r.Position)
	}
	if r.IntervalDays != 2 {
		t.Errorf("not rescheduled: IntervalDays = %d", r.IntervalDays)
	}
	if got := f.readMinutes(t); got != 2 {
		t.Errorf("logged %d minutes, want 2", got)
	}
}

func TestEndTwiceLogsTimeOnce(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")
	f.now = f.now.Add(600 * time.Second)

	if _, err := f.tr.End(id, "", "continue", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.tr.End(id, "", "continue", nil); err != nil {
		t.Fatalf("second End: %v, want a no-op", err)
	}
	if got := f.readMinutes(t); got != 10 {
		t.Errorf("logged %d minutes, want 10", got)
	}
	if iv := f.row(t).IntervalDays; iv != 2 {
		t.Errorf("rescheduled twice: IntervalDays = %d, want 2", iv)
	}
}

func TestEndZeroSecondsLogsNothing(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")

	if _, err := f.tr.End(id, "", "continue", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := f.store.ActivitySince(""); ok {
		t.Error("a zero-second Sitting was written to the activity log")
	}
}

func TestEndDoneMarksDoneWithoutRescheduling(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")

	if _, err := f.tr.End(id, "char=500", "done", nil); err != nil {
		t.Fatal(err)
	}
	r := f.row(t)
	if !r.Done {
		t.Error("Source not marked done")
	}
	if r.IntervalDays != 1 {
		t.Errorf("IntervalDays = %d, want 1 (unchanged)", r.IntervalDays)
	}
}

func TestEndUnknownActionKeepsTheSittingOpen(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")
	before := f.file(t)
	notes := before + "\nedit\n"

	if _, err := f.tr.End(id, "char=9", "soon", &notes); err == nil {
		t.Fatal("End with action \"soon\" succeeded")
	}
	if f.file(t) != before {
		t.Error("an invalid End wrote the notes file")
	}
	if r := f.row(t); r.Position != "char=0" || r.IntervalDays != 1 {
		t.Errorf("an invalid End changed the row: %+v", r)
	}
	if _, err := f.tr.End(id, "char=9", "continue", nil); err != nil {
		t.Fatal(err)
	}
	if p := f.row(t).Position; p != "char=9" {
		t.Errorf("the Sitting was not left open: Position = %q", p)
	}
}

// addSource registers a second Source in f's Collection and returns its id.
func (f *fixture) addSource(t *testing.T, name string) string {
	t.Helper()
	notes := filepath.Join(f.dir, "reading", name+".n4l")
	id, err := content.CreateReadingNotes(notes, "sources/k.txt", name)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.UpsertReading(learning.ReadingElement{
		ID: id, RefPath: "sources/k.txt", NotesPath: "reading/" + name + ".n4l", Title: name,
		Position: "char=0", IntervalDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPeekRecordsNothing(t *testing.T) {
	f := newFixture(t)
	before, file := f.row(t), f.file(t)
	id := f.begin(t, "char=200")

	src, err := f.tr.Progress(id, "char=210")
	if err != nil {
		t.Fatal(err)
	}
	if src != f.sourceID {
		t.Errorf("Progress during a peek returned %q, want %q", src, f.sourceID)
	}
	f.now = f.now.Add(600 * time.Second)
	if _, err := f.tr.End(id, "char=220", "continue", nil); err != nil {
		t.Fatal(err)
	}

	after := f.row(t)
	if after.Position != before.Position || after.IntervalDays != before.IntervalDays ||
		!after.Due.Equal(before.Due) || !after.LastRead.Equal(before.LastRead) {
		t.Errorf("a peek changed the Source:\nbefore %+v\nafter  %+v", before, after)
	}
	if f.file(t) != file {
		t.Error("a peek wrote the notes file")
	}
	if _, ok, _ := f.store.ActivitySince(""); ok {
		t.Error("a peek logged reading time")
	}
}

func TestPeekStillSavesNotes(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "char=200")
	edited := f.file(t) + "\nnoted while peeking\n"

	if _, err := f.tr.End(id, "", "continue", &edited); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.file(t), "noted while peeking") {
		t.Error("notes edited during a peek were not saved")
	}
}

func TestDoneDuringAPeekMarksDone(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "char=200")
	f.now = f.now.Add(600 * time.Second)

	if _, err := f.tr.End(id, "char=220", "done", nil); err != nil {
		t.Fatal(err)
	}
	r := f.row(t)
	if !r.Done {
		t.Error("Done during a peek did not mark the Source done")
	}
	if r.Position != "char=0" {
		t.Errorf("Done during a peek moved progress to %q", r.Position)
	}
	if _, ok, _ := f.store.ActivitySince(""); ok {
		t.Error("Done during a peek logged reading time")
	}
}

func TestBeginEndsTheOpenSitting(t *testing.T) {
	f := newFixture(t)
	a := f.begin(t, "")
	f.now = f.now.Add(60 * time.Second)
	other := f.addSource(t, "other")

	if _, _, err := f.tr.Begin(other, ""); err != nil {
		t.Fatal(err)
	}
	if iv := f.row(t).IntervalDays; iv != 2 {
		t.Errorf("the first Sitting was not ended: IntervalDays = %d", iv)
	}
	if got := f.readMinutes(t); got != 1 {
		t.Errorf("logged %d minutes for the first Sitting, want 1", got)
	}
	if _, err := f.tr.End(a, "", "continue", nil); err != nil {
		t.Errorf("End on the replaced Sitting: %v, want a no-op", err)
	}
	if iv := f.row(t).IntervalDays; iv != 2 {
		t.Errorf("the replaced Sitting ended twice: IntervalDays = %d", iv)
	}
	if _, err := f.tr.Progress(a, "char=1"); !errors.Is(err, ErrNoSitting) {
		t.Errorf("Progress on the replaced Sitting: err=%v, want ErrNoSitting", err)
	}
}

func TestFailedNotesSaveKeepsTheSittingOpen(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")
	f.now = f.now.Add(600 * time.Second)
	f.saveErr = errors.New("disk full")
	notes := "anything"

	if _, err := f.tr.End(id, "char=500", "continue", &notes); err == nil {
		t.Fatal("End reported success although the notes save failed")
	}
	if r := f.row(t); r.Position != "char=0" || r.IntervalDays != 1 {
		t.Errorf("a failed End changed the Source: %+v", r)
	}
	if _, ok, _ := f.store.ActivitySince(""); ok {
		t.Error("a failed End logged reading time")
	}

	f.saveErr = nil
	if _, err := f.tr.End(id, "", "continue", nil); err != nil {
		t.Fatal(err)
	}
	if got := f.readMinutes(t); got != 10 {
		t.Errorf("logged %d minutes after the retry, want 10", got)
	}
}

func TestEndAllEndsTheOpenSitting(t *testing.T) {
	f := newFixture(t)
	f.tr.EndAll() // nothing open: nothing happens
	f.begin(t, "")
	f.now = f.now.Add(600 * time.Second)

	f.tr.EndAll()
	if iv := f.row(t).IntervalDays; iv != 2 {
		t.Errorf("EndAll did not reschedule: IntervalDays = %d", iv)
	}
	if got := f.readMinutes(t); got != 10 {
		t.Errorf("EndAll logged %d minutes, want 10", got)
	}
}

func TestConcurrentEndsLogOnce(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")
	f.now = f.now.Add(600 * time.Second)

	done := make(chan error)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := f.tr.End(id, "char=5", "continue", nil)
			done <- err
		}()
	}
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
	if got := f.readMinutes(t); got != 10 {
		t.Errorf("logged %d minutes, want 10", got)
	}
}

// The view puts the returned notes back into its buffer. If they held the old
// marker, the next save of that buffer would write it back over the new one.
func TestEndReturnsTheNotesAsTheyAreOnDisk(t *testing.T) {
	f := newFixture(t)
	id := f.begin(t, "")
	edited := f.file(t) + "\nan edit\n"

	final, err := f.tr.End(id, "char=500", "continue", &edited)
	if err != nil {
		t.Fatal(err)
	}
	if final != f.file(t) {
		t.Errorf("returned notes differ from the file:\n--- returned ---\n%s\n--- disk ---\n%s", final, f.file(t))
	}
}
