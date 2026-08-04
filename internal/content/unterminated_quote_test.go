package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseBody(t *testing.T, body string) *ParsedFile {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "f.n4l")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, _ := LoadRegistry(filepath.Join(dir, "none"))
	pf, _ := ParseFile(p, reg)
	if pf == nil {
		t.Fatal("ParseFile returned nil")
	}
	return pf
}

func nodeTexts(pf *ParsedFile) []string {
	var out []string
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			out = append(out, n.Text)
		}
	}
	return out
}

func hasErr(pf *ParsedFile, category string) bool {
	for _, e := range pf.Errors {
		if strings.HasPrefix(e.Msg, category) {
			return true
		}
	}
	return false
}

// A `"` block that is never closed used to swallow every following line to EOF
// into one node's text — silently, with no ParseError. A stray leading quote
// therefore made the whole rest of the file invisible: no nodes, no ids, no
// cards, no extracts, and nothing to tell the user why.
func TestUnterminatedDoubleQuoteDoesNotSwallowTheRestOfTheFile(t *testing.T) {
	pf := parseBody(t, "- Ch\n\n\"an unclosed quotation\n\n# @id keepme\n# @card\nthe front >> the back\n\na later plain note\n")

	if !hasErr(pf, ErrUnterminatedQuote) {
		t.Errorf("no %q error reported; the truncation must not be silent (errors: %+v)", ErrUnterminatedQuote, pf.Errors)
	}

	var found bool
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if strings.Contains(n.Text, "# @card") || strings.Contains(n.Text, "# @id") {
				t.Errorf("marker lines were swallowed into node text: %q", n.Text)
			}
			if strings.HasSuffix(n.ID, ":id:keepme") {
				found = true
				if n.StudyMeta == nil {
					t.Error("node keepme lost its # @card StudyMeta")
				}
			}
		}
	}
	if !found {
		t.Errorf("node after the unclosed quote was lost entirely; nodes = %q", nodeTexts(pf))
	}
}

// The same recovery must apply to the single-quote block form.
func TestUnterminatedSingleQuoteDoesNotSwallowTheRestOfTheFile(t *testing.T) {
	pf := parseBody(t, "- Ch\n\n'an unclosed quotation\n\n# @id keepme\na later note\n")
	if !hasErr(pf, ErrUnterminatedQuote) {
		t.Errorf("no %q error reported (errors: %+v)", ErrUnterminatedQuote, pf.Errors)
	}
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if strings.Contains(n.Text, "# @id") {
				t.Errorf("marker line swallowed into node text: %q", n.Text)
			}
		}
	}
}

// A properly closed multi-line block must keep working exactly as before: one
// node whose text spans the physical lines, and no error.
func TestClosedMultiLineQuoteBlocksAreUnchanged(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"double", "- Ch\n\n\"a quotation that\nspans two lines\"\n\nafter\n"},
		{"single", "- Ch\n\n'a quotation that\nspans two lines'\n\nafter\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pf := parseBody(t, tc.body)
			if hasErr(pf, ErrUnterminatedQuote) {
				t.Errorf("closed block wrongly reported unterminated: %+v", pf.Errors)
			}
			texts := nodeTexts(pf)
			var joined bool
			for _, x := range texts {
				if strings.Contains(x, "spans two lines") && strings.Contains(x, "a quotation that") {
					joined = true
				}
			}
			if !joined {
				t.Errorf("closed multi-line block no longer joins into one node; nodes = %q", texts)
			}
		})
	}
}

// The EOF recovery was not enough. A stray quote can also be "closed" by an
// unrelated quote character further down the file — a `"` inside ordinary prose,
// or an anchor with an odd number of them. The block then ends mid-file and every
// line in between is absorbed as one node's text, silently: no error at all,
// because from the scanner's point of view the block closed normally.
//
// Reported symptom: adding a stray quote above a "# @id" made that node's
// notebook disappear ("No notebook for this line"), because the node itself no
// longer existed.
func TestQuoteBlockDoesNotSwallowStructuralLines(t *testing.T) {
	pf := parseBody(t, "- Ch\n\n\"an unclosed quotation\n\n# @id keepme\nthe node with a notebook\n\nhe said \" and stopped\n\n# @id later\ntail node\n")

	if !hasErr(pf, ErrUnterminatedQuote) {
		t.Errorf("no %q error; a mid-file close is as silent as the EOF case was (errors: %+v)", ErrUnterminatedQuote, pf.Errors)
	}
	var keep, later bool
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if strings.Contains(n.Text, "# @id") {
				t.Errorf("marker line swallowed into node text: %.80q", n.Text)
			}
			if strings.HasSuffix(n.ID, ":id:keepme") {
				keep = true
			}
			if strings.HasSuffix(n.ID, ":id:later") {
				later = true
			}
		}
	}
	if !keep {
		t.Errorf("node keepme was swallowed; its notebook would show \"No notebook for this line\". nodes = %q", nodeTexts(pf))
	}
	if !later {
		t.Error("node later was lost too")
	}

	// The reported symptom was the notebook pane, which resolves a caret line
	// through NodeSpans — so assert the span exists, not merely the node. A node
	// present but unspanned would still show "No notebook for this line".
	var spanned bool
	for _, s := range NodeSpans(pf) {
		if strings.HasSuffix(s.ID, ":id:keepme") {
			spanned = true
		}
	}
	if !spanned {
		t.Errorf("keepme has no NodeSpan, so no caret line resolves to it; spans = %+v", NodeSpans(pf))
	}
}

// A blank line ends a paragraph. The block is documented as a "multi-line
// paragraph", so it must not run past one, or a single stray quote silently
// merges unrelated notes.
func TestQuoteBlockStopsAtABlankLine(t *testing.T) {
	pf := parseBody(t, "- Ch\n\n\"a stray quote\n\nplain note A\n\nplain note B\n\nhe said \" here\n")
	if !hasErr(pf, ErrUnterminatedQuote) {
		t.Errorf("no %q error reported (errors: %+v)", ErrUnterminatedQuote, pf.Errors)
	}
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if strings.Contains(n.Text, "plain note A") && strings.Contains(n.Text, "plain note B") {
				t.Errorf("unrelated notes merged into one node: %.80q", n.Text)
			}
		}
	}
}
