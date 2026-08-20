package content

import "testing"

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
