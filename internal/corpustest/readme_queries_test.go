package corpustest

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// queryCell pulls the middle column out of the README's two mapping tables:
// rows look like `| question | `query` | description |`.
var queryCell = regexp.MustCompile("\\| `([^`]+)` \\|")

// Every query the README documents must be safe to run — no crash, no
// GraphError. The queries are READ FROM THE README rather than restated here,
// so the two cannot drift: adding a row to the table puts it under test
// automatically, which a hand-copied list would not.
//
// This does not assert results. The tables are illustrative and some rows
// reference chapters ("Cooking") that the corpus does not have. What it does
// guarantee is that we never document a query the app itself rejects — which
// is how proximity `a <-> b` came to be documented while it crashed the app.
func TestReadmeQueriesAreSafeToRun(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	ms := queryCell.FindAllStringSubmatch(string(b), -1)
	if len(ms) < 20 {
		t.Fatalf("parsed only %d queries from the README; the table format changed and this guard is no longer looking at anything", len(ms))
	}

	svc := corpus(t)
	for _, m := range ms {
		// The README escapes | inside table cells; unescape before searching.
		q := strings.ReplaceAll(m[1], "\\|", "|")
		t.Run(q, func(t *testing.T) {
			res, err := svc.Search(q)
			if err != nil {
				t.Fatalf("Search error: %v", err)
			}
			if res.GraphError != "" {
				t.Fatalf("the README documents a query the app rejects: %q", res.GraphError)
			}
		})
	}
}
