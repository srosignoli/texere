package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"texere/internal/content"
	"texere/internal/learning"
)

// copyDirRecursive copies src to dst, preserving relative structure and file
// contents (mode included). Used to simulate a collection being duplicated —
// backed up, copied for verification, moved to a new machine — with its
// learning.db carried along, exactly as the incident report describes.
func copyDirRecursive(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("copyDirRecursive: %v", err)
	}
}

// TestStudyDoesNotTouchTheOriginalCollection is the regression test for the
// reported incident: a copied collection's learning.db still held the
// ORIGINAL's absolute file paths (the row was written before every call site
// was converted to Store.Rel/Store.Resolve), and driving the Study/Read
// write path against the copy silently read — and wrote — through to the
// original files instead of the copy.
//
// The row is seeded with UpsertReading directly (bypassing the App layer) so
// this test does not depend on the very call sites it is trying to verify —
// it reproduces the on-disk shape a missed conversion would produce: a
// reading_elements.notes_path holding an absolute path into a DIFFERENT
// directory than the one the store was just opened against.
func TestStudyDoesNotTouchTheOriginalCollection(t *testing.T) {
	original := t.TempDir()
	if err := os.MkdirAll(filepath.Join(original, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(original, "reading"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(original, "sources", "k.txt"), []byte("some article text"), 0o644); err != nil {
		t.Fatal(err)
	}
	notesOriginal := filepath.Join(original, "reading", "k.n4l")
	refID, err := content.CreateReadingNotes(notesOriginal, "sources/k.txt", "k")
	if err != nil {
		t.Fatal(err)
	}

	store, err := learning.Open(original)
	if err != nil {
		t.Fatal(err)
	}
	// Seed the row the way an unconverted call site would: with the caller's
	// raw, absolute NotesPath. The schema is already at v7 by the time Open()
	// returns, so no migration will ever touch a row inserted afterward — this
	// is exactly what a missed call-site conversion leaves behind on disk.
	if err := store.UpsertReading(learning.ReadingElement{
		ID: refID, RefPath: "sources/k.txt", NotesPath: notesOriginal, Title: "k",
		Position: "char=0", ProjLen: 10000, IntervalDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Snapshot the original note before anything ever touches the copy.
	beforeContent, err := os.ReadFile(notesOriginal)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(notesOriginal)
	if err != nil {
		t.Fatal(err)
	}
	beforeMtime := beforeInfo.ModTime()

	// Copy the whole collection — database included — to a second directory.
	cp := filepath.Join(t.TempDir(), "copy")
	copyDirRecursive(t, original, cp)

	a, _ := newTestApp(t, cp, nil)

	// Drive the real Study/Read write path against the COPY. The stored
	// NotesPath still points at the ORIGINAL's absolute location, which lies
	// outside the copy's root — Store.Resolve (via collectionpath.Resolve) must
	// refuse it rather than silently follow it, so this call is expected to
	// fail. What matters is *where nothing gets written*, not the error itself.
	updateErr := a.UpdateReadingProgress(refID, "char=500")

	// The ORIGINAL file must be untouched: same bytes, same mtime. This is the
	// incident's actual shape — a function that can "succeed" while writing to
	// the wrong file — so a check on UpdateReadingProgress's return value alone
	// would not have caught it.
	afterContent, err := os.ReadFile(notesOriginal)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterContent) != string(beforeContent) {
		t.Fatalf("the ORIGINAL collection's note was rewritten (UpdateReadingProgress err=%v):\n--- before ---\n%s\n--- after ---\n%s",
			updateErr, beforeContent, afterContent)
	}
	afterInfo, err := os.Stat(notesOriginal)
	if err != nil {
		t.Fatal(err)
	}
	if !afterInfo.ModTime().Equal(beforeMtime) {
		t.Fatalf("the ORIGINAL collection's note mtime changed (UpdateReadingProgress err=%v): before=%v after=%v",
			updateErr, beforeMtime, afterInfo.ModTime())
	}

	// A stored path that escapes the currently-open collection must be refused
	// loudly, not silently swallowed into a no-op that looks like success.
	if updateErr == nil {
		t.Fatal("UpdateReadingProgress against a row whose stored path escapes the copy's root returned nil error; " +
			"it must refuse rather than silently succeed without writing anywhere resolvable")
	}
}

// TestStudyUpdatesTheCopyWhenStoredPathsAreRelative is the companion positive
// path: once a row's stored path is properly collection-relative — the normal
// state after this fix, and after Task 2's migration successfully rehomes a
// legacy row — driving the same Study/Read write path against a copy must
// write to the COPY's file, not merely refuse to touch the original.
func TestStudyUpdatesTheCopyWhenStoredPathsAreRelative(t *testing.T) {
	original := t.TempDir()
	if err := os.MkdirAll(filepath.Join(original, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(original, "reading"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(original, "sources", "k.txt"), []byte("some article text"), 0o644); err != nil {
		t.Fatal(err)
	}
	notesOriginal := filepath.Join(original, "reading", "k.n4l")
	refID, err := content.CreateReadingNotes(notesOriginal, "sources/k.txt", "k")
	if err != nil {
		t.Fatal(err)
	}

	store, err := learning.Open(original)
	if err != nil {
		t.Fatal(err)
	}
	// Seed the row the way a converted call site does: collection-relative.
	if err := store.UpsertReading(learning.ReadingElement{
		ID: refID, RefPath: "sources/k.txt", NotesPath: "reading/k.n4l", Title: "k",
		Position: "char=0", ProjLen: 10000, IntervalDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	beforeContent, err := os.ReadFile(notesOriginal)
	if err != nil {
		t.Fatal(err)
	}

	cp := filepath.Join(t.TempDir(), "copy")
	copyDirRecursive(t, original, cp)

	a, _ := newTestApp(t, cp, nil)

	if err := a.UpdateReadingProgress(refID, "char=500"); err != nil {
		t.Fatalf("UpdateReadingProgress: %v", err)
	}

	// Original untouched.
	afterContent, err := os.ReadFile(notesOriginal)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterContent) != string(beforeContent) {
		t.Fatalf("the ORIGINAL collection's note was rewritten:\n--- before ---\n%s\n--- after ---\n%s", beforeContent, afterContent)
	}

	// Copy correctly updated.
	notesCopy := filepath.Join(cp, "reading", "k.n4l")
	copyContent, err := os.ReadFile(notesCopy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(copyContent), "# @progress char=500") {
		t.Fatalf("the COPY's note was not updated; got:\n%s", copyContent)
	}
}
