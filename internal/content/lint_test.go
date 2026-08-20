package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func perr(msg string) ParseError { return ParseError{File: "x.n4l", Line: 3, Msg: msg} }

func TestClassifyDefects(t *testing.T) {
	// Every kind that means "the parser produced something other than what was
	// written". These are the ones that mark a file.
	for _, msg := range []string{
		ErrUnterminatedQuote + ": quoted block opened here is never closed",
		ErrUnresolvedRef + ": $PREV.2",
		ErrMalformedMarker + ": # @card bogus",
		ErrInvalidPriority + ": 9",
		ErrEncoding + ": invalid utf-8",
	} {
		if got := Classify(perr(msg)); got != CategoryDefect {
			t.Errorf("Classify(%q) = %v, want CategoryDefect", msg, got)
		}
	}
}

func TestClassifyVocabulary(t *testing.T) {
	// An undeclared arrow is not a defect: the note is fine, the relation just
	// will not become a graph edge. 99.4% of findings on the real collection are
	// this, and treating them as defects would mark 39 of 53 files.
	if got := Classify(perr(ErrUnknownArrow + `: "has-frag"`)); got != CategoryVocabulary {
		t.Errorf("got %v, want CategoryVocabulary", got)
	}
}

func TestClassifyUnrecognised(t *testing.T) {
	if got := Classify(perr("something the parser grew later")); got != CategoryUnknown {
		t.Errorf("got %v, want CategoryUnknown", got)
	}
}

// The parser formats the arrow with %q, so the detail is quoted. Aggregating by
// name without unquoting would produce two entries for the same arrow.
func TestArrowNameUnquotes(t *testing.T) {
	if got := ArrowName(perr(ErrUnknownArrow + `: "has-frag"`)); got != "has-frag" {
		t.Errorf("got %q, want %q", got, "has-frag")
	}
}

func TestArrowNameOnNonArrowIsEmpty(t *testing.T) {
	if got := ArrowName(perr(ErrUnresolvedRef + ": $PREV.2")); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// Remediation must exist for every kind a user can actually hit, or the
// Problems tab shows a problem with no advice.
func TestRemediationExistsForEveryLiveKind(t *testing.T) {
	// ErrOrphanDitto is deliberately absent: it is declared in errors.go but
	// never produced anywhere (verified by grep), so it is unreachable.
	for _, k := range []string{
		ErrUnknownArrow, ErrUnterminatedQuote, ErrUnresolvedRef,
		ErrMalformedMarker, ErrInvalidPriority, ErrEncoding,
	} {
		if Remediation(perr(k+": detail")) == "" {
			t.Errorf("no remediation for %q", k)
		}
	}
}

// Three of the six live ParseError kinds never occur in the real collection,
// so their classification and remediation would otherwise ship untested
// against any real input. Each fixture must provoke exactly the kind it
// claims to — a fixture that provokes a different error still makes this
// test pass while proving nothing.
func TestLintFixturesProvokeTheirKind(t *testing.T) {
	reg := &SSTRegistry{Arrows: map[string]ArrowDef{}}
	for file, wantKind := range map[string]string{
		"testdata/lint/malformed-marker.n4l": ErrMalformedMarker,
		"testdata/lint/invalid-priority.n4l": ErrInvalidPriority,
		"testdata/lint/bad-encoding.n4l":     ErrEncoding,
	} {
		pf, perrs := ParseFile(file, reg)
		all := append([]ParseError{}, perrs...)
		if pf != nil {
			all = append(all, pf.Errors...)
		}
		found := false
		for _, e := range all {
			if strings.HasPrefix(e.Msg, wantKind) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s did not produce %q; got %v", file, wantKind, all)
		}
	}
}

// The aggregation property that makes the feature usable: on the real
// collection 34,970 occurrences are only 158 causes. Two files using the same
// undeclared arrow must produce ONE vocabulary row, not two.
func TestLintAggregatesArrowsByName(t *testing.T) {
	dir := t.TempDir()
	// "wibbles" is in neither file's registry, so every use is an unknown arrow.
	mustWrite(t, filepath.Join(dir, "a.n4l"), "- ch\n\nalpha\n    (wibbles) beta\n")
	mustWrite(t, filepath.Join(dir, "b.n4l"), "- ch\n\ngamma\n    (wibbles) delta\n    (wibbles) epsilon\n")

	reg := &SSTRegistry{Arrows: map[string]ArrowDef{}}
	res, err := LintCollection(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Vocabulary) != 1 {
		t.Fatalf("want 1 vocabulary row, got %d: %+v", len(res.Vocabulary), res.Vocabulary)
	}
	v := res.Vocabulary[0]
	if v.Arrow != "wibbles" {
		t.Errorf("arrow = %q, want %q (is it still quoted?)", v.Arrow, "wibbles")
	}
	if v.Uses != 3 {
		t.Errorf("uses = %d, want 3 — occurrences must be summed across files", v.Uses)
	}
	if len(v.Files) != 2 {
		t.Errorf("files = %v, want both a.n4l and b.n4l", v.Files)
	}
}

// The split the whole design rests on. A file with both kinds must yield the
// quote as a defect and the arrow as vocabulary — never the arrow as a defect,
// which is what would mark 39 of 53 real files instead of 7.
func TestLintDefectsAndVocabularyAreSeparate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "mixed.n4l"),
		"- ch\n\nalpha\n    (wibbles) beta\n\n\"an unterminated quote\n")

	reg := &SSTRegistry{Arrows: map[string]ArrowDef{}}
	res, err := LintCollection(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Vocabulary) != 1 || res.Vocabulary[0].Arrow != "wibbles" {
		t.Errorf("vocabulary = %+v, want one entry for wibbles", res.Vocabulary)
	}
	for _, d := range res.Defects {
		if strings.HasPrefix(d.Kind, ErrUnknownArrow) {
			t.Errorf("an undeclared arrow was reported as a defect: %+v", d)
		}
	}
	var quotes int
	for _, d := range res.Defects {
		if strings.HasPrefix(d.Kind, ErrUnterminatedQuote) {
			quotes++
		}
	}
	if quotes != 1 {
		t.Errorf("want 1 unterminated-quote defect, got %d: %+v", quotes, res.Defects)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
