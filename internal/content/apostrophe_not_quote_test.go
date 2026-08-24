package content

import "testing"

// An apostrophe inside prose is ordinary text, not a quote delimiter.
//
// text2N4L (SSTorytime's document importer) writes each sentence as
// "@sen<N>   <prose>", passing the prose through a Sanitize() that replaces
// only "(" and ")" — N4L's arrow metacharacters. Apostrophes are deliberately
// left alone, so possessives and contractions reach the file verbatim.
//
// contentSQStart used to open a multi-line single-quoted block on *any* '
// anywhere in an "@label" line, so every "Man's" and "years'" opened a block
// that the following blank line then closed as unterminated. Darwin.n4l alone
// produced 74 such reports, all spurious; NDA.n4l another 10.
//
// The lines below are real text2N4L output.
func TestApostropheInLabelledProseIsNotAQuote(t *testing.T) {
	for _, tc := range []struct{ name, line string }{
		{"trailing possessive", `@sen6   After five years' work I allowed myself to speculate.`},
		{"possessive", `@sen56   Circumstances favourable to Man's power of Selection.`},
		{"mid-sentence possessive", `@sen276   the fuller's teasel, with its hooks, is only a variety.`},
		{"contraction", `@sen9   It doesn't follow that the species is fixed.`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pf := parseBody(t, "- Ch\n\n"+tc.line+"\n\nafter\n")
			if hasErr(pf, ErrUnterminatedQuote) {
				t.Errorf("apostrophe in prose wrongly opened a quoted block: %+v", pf.Errors)
			}
		})
	}
}

// The construct contentSQStart exists to serve must keep working: an @label
// whose *content* starts with ' opens a genuine multi-line block. The
// collection contains 5 of these, e.g. json_example_1.n4l's "@data '{ ...".
func TestLabelledSingleQuoteBlockStillOpens(t *testing.T) {
	pf := parseBody(t, "- Ch\n\n@data 'a block that\nspans two lines'\n\nafter\n")
	if hasErr(pf, ErrUnterminatedQuote) {
		t.Errorf("closed @label block wrongly reported unterminated: %+v", pf.Errors)
	}
	var joined bool
	for _, x := range nodeTexts(pf) {
		if contains(x, "a block that") && contains(x, "spans two lines") {
			joined = true
		}
	}
	if !joined {
		t.Errorf("@label ' block did not span its physical lines: %q", nodeTexts(pf))
	}
}

// A genuinely unterminated @label block is still reported.
func TestUnterminatedLabelledBlockStillReported(t *testing.T) {
	pf := parseBody(t, "- Ch\n\n@data '{ \"users\": [\n\nafter\n")
	if !hasErr(pf, ErrUnterminatedQuote) {
		t.Errorf("genuinely unterminated block not reported: %+v", pf.Errors)
	}
}

// Parity counting must use the same open/close rules as maskSpecial: an
// in-word ' is not a delimiter. chinese.n4l:956 has five ' characters, of
// which one is the possessive in "X's" — counting all five reports odd.
func TestInWordApostropheDoesNotBreakParity(t *testing.T) {
	pf := parseBody(t, "- Ch\n\n'X zuì xǐhuān de Y' (pinyin has english) 'X's most liked Y'\n\nafter\n")
	if hasErr(pf, ErrUnterminatedQuote) {
		t.Errorf("possessive inside a closed quote wrongly reported unterminated: %+v", pf.Errors)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
