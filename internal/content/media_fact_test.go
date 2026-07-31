package content

import (
	"os"
	"path/filepath"
	"testing"
)

// A promoted media fact has a shape the shell fact does not: an unsanitised URL
// as a target, and a `(note)` line carrying a title plus an "@ m:ss" clock.
//
// frontend/src/lib/media-fact.js makes two claims that only a Go test can
// settle. First, that the URL must NOT be sanitised, because the parser exempts
// a slash run following a colon — if that were wrong, every promoted video
// would silently lose its link. Second, that the "@" clock survives; the file
// asserts this in a comment reading "Verified:", with nothing behind it.
//
// The sanitised subject and title shapes themselves are already covered by
// shell_target_test.go, since media-fact.js and shell-fact.js now share
// n4l-escape.js. What is tested here is only what is specific to media.

// writeMediaFact writes a two-line media fact and returns its path.
func writeMediaFact(t *testing.T, subject, arrow, target string) string {
	t.Helper()
	src := "- media\n\n:: promoted ::\n\n" + subject + "\n\"  (" + arrow + ") " + target + "\n"
	path := filepath.Join(t.TempDir(), "media.n4l")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func mediaReg() *SSTRegistry {
	return &SSTRegistry{
		Arrows: map[string]ArrowDef{
			"pic":  {Code: "pic", LongName: "has picture", MetaType: "properties"},
			"vid":  {Code: "vid", LongName: "has video", MetaType: "properties"},
			"note": {Code: "note", LongName: "note/remark", MetaType: "properties"},
		},
		Annotations: map[string]AnnotDef{},
	}
}

// The URL is the one target this feature deliberately does not sanitise. If a
// slash run after a colon were NOT exempt, `https://` would be truncated to
// `https:` and every promoted video would lose its link.
func TestMediaVideoURLSurvivesUnsanitised(t *testing.T) {
	urls := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=223s",
		"https://commons.wikimedia.org/wiki/File:Example.webm#t=90",
		"https://peertube.example.org/w/abc123?start=42",
		"file:///home/user/clip.webm",
	}
	for _, u := range urls {
		t.Run(u, func(t *testing.T) {
			pf, _ := ParseFile(writeMediaFact(t, "demo clip", "vid", u), mediaReg())
			got := firstDittoArrow(t, pf)
			if got.Code != "vid" {
				t.Fatalf("arrow code = %q, want %q", got.Code, "vid")
			}
			if got.Target != u {
				t.Fatalf("URL did not survive byte-exact:\n got %q\nwant %q", got.Target, u)
			}
		})
	}
}

// media-fact.js appends the clock with "@" and explains at length why that is
// safe on a ditto line. That claim had no test until now.
func TestMediaNoteClockSurvives(t *testing.T) {
	cases := []struct{ name, target string }{
		{"title and clock", "Never Gonna Give You Up @ 3:43"},
		{"clock past an hour", "Long lecture @ 1:02:07"},
		{"title already starting with at", "@handle explains it @ 0:12"},
		{"sanitised parens in title", "Never Gonna Give You Up [Official Video] @ 3:43"},
		{"no clock at all", "Never Gonna Give You Up"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf, _ := ParseFile(writeMediaFact(t, "rickroll", "note", tc.target), mediaReg())
			got := firstDittoArrow(t, pf)
			if got.Target != tc.target {
				t.Fatalf("note target did not survive:\n got %q\nwant %q", got.Target, tc.target)
			}
		})
	}
}

// The asset path is a content hash, so it cannot itself carry N4L syntax — but
// it is worth pinning that the whole picture-fact shape parses, since that is
// the most common promotion in the app.
func TestMediaPictureFactParses(t *testing.T) {
	pf, _ := ParseFile(writeMediaFact(t, "manzana", "pic", "media/a3f9c1.jpg"), mediaReg())
	if len(pf.Chapters) == 0 || len(pf.Chapters[0].Nodes) == 0 {
		t.Fatal("no nodes parsed")
	}
	if node := pf.Chapters[0].Nodes[0]; node.Text != "manzana" {
		t.Fatalf("subject = %q, want %q", node.Text, "manzana")
	}
	got := firstDittoArrow(t, pf)
	if got.Code != "pic" || got.Target != "media/a3f9c1.jpg" {
		t.Fatalf("got %+v", got)
	}
}
