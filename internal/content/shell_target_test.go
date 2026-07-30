package content

import (
	"os"
	"path/filepath"
	"testing"
)

// shellReg is a minimal registry declaring just the arrows a promoted shell
// fact uses, mirroring how parser_test.go builds registries for its fixtures.
func shellReg() *SSTRegistry {
	return &SSTRegistry{
		Arrows: map[string]ArrowDef{
			"cmd": {Code: "cmd", LongName: "has command", MetaType: "properties"},
			"out": {Code: "out", LongName: "has output", MetaType: "properties"},
		},
		Annotations: map[string]AnnotDef{},
	}
}

// writeN4L writes one promoted-shell-fact fixture and returns its path.
func writeN4L(t *testing.T, target string) string {
	t.Helper()
	src := "- shell\n\n:: runbook ::\n\ncheck containers\n\"  (out) " + target + "\n"
	path := filepath.Join(t.TempDir(), "shell.n4l")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// firstDittoArrow returns the single arrow the fixture declares.
func firstDittoArrow(t *testing.T, pf *ParsedFile) Arrow {
	t.Helper()
	if len(pf.Chapters) == 0 || len(pf.Chapters[0].Nodes) == 0 {
		t.Fatalf("no nodes parsed")
	}
	node := pf.Chapters[0].Nodes[0]
	if len(node.Dittos) == 0 || len(node.Dittos[0].Arrows) == 0 {
		t.Fatalf("no ditto arrow parsed from node %q", node.Text)
	}
	return node.Dittos[0].Arrows[0]
}

// Shell output promoted into N4L arrives as the TARGET of a ditto line. This is
// the Go-side proof of a claim made in JavaScript: that sanitizeTarget's output
// survives this parser with the target intact.
//
// frontend/src/lib/shell-fact.js argues that N4L's leading-position markers are
// safe in a target because the ditto prefix means they are never first on the
// line, and that parentheses are the one real hazard. If either claim is wrong,
// this test fails — which is the point. media-fact.js could only assert it in a
// comment.
func TestShellPromotedTargetsSurviveTheParser(t *testing.T) {
	// Each target is what sanitizeTarget would emit for real shell output.
	cases := []struct {
		name   string
		target string
	}{
		{"plain", "pg-primary"},
		{"ls long form", "-rw-r--r-- 1 root root 4096 Jul 30 10:12 hosts"},
		{"leading hash", "# not a comment here"},
		{"leading colons", ":: not a section"},
		{"leading at", "@reboot /usr/bin/thing"},
		{"docker go template", "docker ps --format '{{.Names}}'"},
		{"brackets from sanitised parens", "at foo[bar] line 3"},
		{"quotes", `he said "hello"`},
		{"path with slashes", "/var/log/nginx/access.log"},
		{"equals and ampersand", "ENV=prod && echo done"},
		{"unicode", "café → ok"},
		{"percent and star", "cpu 12.5% ***"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf, _ := ParseFile(writeN4L(t, tc.target), shellReg())
			got := firstDittoArrow(t, pf)
			if got.Code != "out" {
				t.Fatalf("arrow code = %q, want %q", got.Code, "out")
			}
			if !got.IsRegistered {
				t.Fatalf("arrow %q should be registered by shellReg()", got.Code)
			}
			if got.Target != tc.target {
				t.Fatalf("target did not survive:\n got %q\nwant %q", got.Target, tc.target)
			}
		})
	}
}

// The hazard sanitizeTarget exists to remove: a raw parenthesised target. This
// documents the actual failure mode so nobody "simplifies" the sanitiser away.
//
// The assertion is deliberately weak — it asserts only that a raw target with
// parentheses does NOT round-trip identically, which is what justifies
// sanitising. It does not pin down how the parser mangles it, since that is the
// parser's business and not a contract this test should freeze.
func TestRawParensDoNotRoundTrip(t *testing.T) {
	raw := "at foo(bar) line 3"
	pf, _ := ParseFile(writeN4L(t, raw), shellReg())
	got := firstDittoArrow(t, pf)
	if got.Target == raw {
		t.Fatalf("raw parens round-tripped intact (%q) — if the parser now preserves "+
			"them, sanitizeTarget's paren replacement is no longer required and both "+
			"this test and that comment should be revisited", got.Target)
	}
}
