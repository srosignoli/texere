package main

import (
	"os"
	"strings"
	"testing"
)

// The corpus verification lives in package corpustest and cannot import this
// package, so it restates the queries. This guard fails when the two drift —
// without it, an example could be added here and never verified, which is
// exactly the hole the corpus test exists to close.
func TestEveryExampleIsVerifiedByTheCorpusTest(t *testing.T) {
	b, err := os.ReadFile("internal/corpustest/corpus_test.go")
	if err != nil {
		t.Fatalf("read corpus test: %v", err)
	}
	body := string(b)
	for _, ex := range queryExamples {
		if !strings.Contains(body, "{`"+ex.Query+"`,") && !strings.Contains(body, `{"`+ex.Query+`",`) {
			t.Errorf("example %q is not verified by TestQueryExamplesReturnResults — add it there", ex.Query)
		}
	}
}
