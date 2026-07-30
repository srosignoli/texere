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

// writeN4LSubject writes a fixture whose SUBJECT is the value under test, with
// an unrelated node in front of it. The preceding node is the point: when a
// subject is eaten by the parser the ditto arrow does not vanish, it silently
// re-parents onto whatever came before — so a fixture with nothing in front
// would hide the failure this test exists to catch.
func writeN4LSubject(t *testing.T, subject string) string {
	t.Helper()
	src := "- shell\n\n:: runbook ::\n\nsomething else entirely\n\n" +
		subject + "\n\"  (cmd) df -h\n"
	path := filepath.Join(t.TempDir(), "shell.n4l")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// lastNode returns the final node parsed from the fixture.
func lastNode(t *testing.T, pf *ParsedFile) Node {
	t.Helper()
	if len(pf.Chapters) == 0 || len(pf.Chapters[0].Nodes) == 0 {
		t.Fatalf("no nodes parsed")
	}
	nodes := pf.Chapters[0].Nodes
	return nodes[len(nodes)-1]
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
		{"leading quote", `"quoted string" is what it said`},
		{"path with slashes", "/var/log/nginx/access.log"},
		{"url keeps its double slash", "see http://example.com/x now"},
		{"separated double slash", "warn: / / TODO fix this"},
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

// The other any-column hazard, and the reason sanitizeTarget separates slashes
// instead of bracketing them: `//` opens an inline comment on every line type,
// and brackets are not consulted when that comment is stripped. `grep` over
// source, `git log` and `docker inspect` all emit `//` routinely, so this is a
// line a user will really promote.
func TestRawDoubleSlashTargetsDoNotRoundTrip(t *testing.T) {
	for _, raw := range []string{"warn: // TODO fix this", "warn: [//] TODO fix this"} {
		t.Run(raw, func(t *testing.T) {
			pf, _ := ParseFile(writeN4L(t, raw), shellReg())
			if got := firstDittoArrow(t, pf).Target; got == raw {
				t.Fatalf("%q round-tripped intact — if the parser now preserves it, "+
					"sanitizeTarget's slash separation should be revisited", raw)
			}
		})
	}
}

// The subject is the OTHER half, and the one the target table cannot speak for.
// sanitizeTarget's comment argues leading markers are inert because the ditto
// prefix means a target is never first on its line. A subject has no prefix, so
// that argument does not carry over — sanitizeSubject exists for exactly this,
// and this is the Go-side proof that what it emits reaches the graph as node
// text, with the command hanging off the right node.
func TestShellPromotedSubjectsSurviveTheParser(t *testing.T) {
	// Each subject is what sanitizeSubject would emit for a user-typed title.
	cases := []struct {
		name    string
		subject string
	}{
		{"plain", "check containers on prod"},
		{"bracketed hash", "[#] incident 4312"},
		{"bracketed dash", "[-] prod runbook"},
		{"bracketed section", "[::] prod ::"},
		{"bracketed context op", "[+::] staging"},
		{"bracketed at", "[@]prod notes"},
		{"bracketed ditto quote", `["]quoted title`},
		{"bracketed single quote", "[']quoted title"},
		{"bracketed annotation", "[#@] marker line"},
		{"brackets from sanitised parens", "restart [prod] db"},
		{"unicode", "café → ok"},
		// " #" opens a trailing comment at ANY column on a node line
		// (parser.go stripNodeComment, applied at parser.go:472). This is the
		// case that caught the first version of sanitizeSubject, which only
		// neutralised leading markers: `incident #4312 - prod` reached the
		// graph as `incident`. A ticket reference is close to the most natural
		// subject a sysadmin types, so this is the common case, not an edge.
		{"mid-line hash bracketed", "incident [#]4312 - prod"},
		{"several mid-line hashes", "a [#] b [#] c"},
		// `//` opens an inline comment on every line type (parser.go:191).
		// Brackets do NOT rescue it — `check [//]prod` parses to `check [` —
		// so sanitizeTarget separates the slashes instead.
		{"separated double slash", "check / /prod/ / hosts"},
		// A hash with no space before it is not a comment, and a URL's slash
		// run is exempted by the parser — neither needs touching.
		{"tight hash needs no escape", "incident#4312"},
		{"url survives", "see http://example.com/x now"},
		// The markers that really are leading-only stay ordinary mid-line.
		{"mid-line dash and at", "prod - runbook for ops@example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf, _ := ParseFile(writeN4LSubject(t, tc.subject), shellReg())
			node := lastNode(t, pf)
			if node.Text != tc.subject {
				t.Fatalf("subject did not survive as node text:\n got %q\nwant %q",
					node.Text, tc.subject)
			}
			if len(node.Dittos) == 0 || len(node.Dittos[0].Arrows) == 0 {
				t.Fatalf("the (cmd) arrow did not attach to the subject node %q", node.Text)
			}
			if got := node.Dittos[0].Arrows[0].Code; got != "cmd" {
				t.Fatalf("arrow code = %q, want %q", got, "cmd")
			}
		})
	}
}

// The hazard sanitizeSubject exists to remove. Each raw subject below is
// something a sysadmin would plausibly type, and each one is destroyed by the
// parser — the node text is lost and the (cmd) arrow re-parents onto the
// PRECEDING node, producing a fact that looks right in the draft cell and is
// wrong in the graph.
//
// The assertion is deliberately weak, like TestRawParensDoNotRoundTrip: it says
// only that the raw subject does not round-trip, which is what justifies
// sanitising, without freezing how the parser mangles it.
func TestRawSubjectMarkersDoNotRoundTrip(t *testing.T) {
	for _, raw := range []string{
		"# incident 4312",
		"- prod runbook",
		":: prod ::",
		"@prod notes",
		`"quoted title`,
		"restart (prod) db",
		// Not a leading marker at all — " #" comments out the rest of the line
		// from wherever it appears.
		"incident #4312 - prod",
		"check //prod// hosts",
	} {
		t.Run(raw, func(t *testing.T) {
			pf, _ := ParseFile(writeN4LSubject(t, raw), shellReg())
			if got := lastNode(t, pf).Text; got == raw {
				t.Fatalf("raw subject %q round-tripped intact — if the parser now "+
					"preserves it, sanitizeSubject's handling of this marker and the "+
					"comment justifying it should both be revisited", raw)
			}
		})
	}
}

// TestShellArrowsAreDeclaredInRealConfig exists so that deleting the cmd/out
// declarations from SSTconfig/arrows-EP-3.sst breaks the build instead of
// silently downgrading promoted shell facts to "unknown" arrows.
//
// shellReg() above is a synthetic registry used to isolate parser behaviour
// from config loading in the table test — it does NOT verify the real
// SSTconfig declarations exist. This test closes that gap by loading the
// actual registry the same way TestLoadRegistryArrows does, and asserting the
// four codes this feature relies on are present with the right pairing.
func TestShellArrowsAreDeclaredInRealConfig(t *testing.T) {
	reg := loadTestRegistry(t)

	cmd, ok := reg.Arrows["cmd"]
	if !ok {
		t.Fatal("want 'cmd' arrow declared in SSTconfig/arrows-EP-3.sst")
	}
	if cmd.Reverse != "runfor" {
		t.Errorf("cmd.Reverse = %q, want %q", cmd.Reverse, "runfor")
	}

	out, ok := reg.Arrows["out"]
	if !ok {
		t.Fatal("want 'out' arrow declared in SSTconfig/arrows-EP-3.sst")
	}
	if out.Reverse != "outof" {
		t.Errorf("out.Reverse = %q, want %q", out.Reverse, "outof")
	}

	if _, ok := reg.Arrows["outof"]; !ok {
		t.Error("want 'outof' arrow declared in SSTconfig/arrows-EP-3.sst")
	}
	if _, ok := reg.Arrows["runfor"]; !ok {
		t.Error("want 'runfor' arrow declared in SSTconfig/arrows-EP-3.sst")
	}
}
