package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// verifiedQueries returns every query string appearing in a composite literal
// in the corpus test, by parsing it rather than grepping it.
//
// Parsing matters: a raw substring match over the file TEXT also matches
// commented-out rows, so commenting one out while debugging would leave the
// guard green while that example shipped to the prompt unverified. Comments do
// not survive into the AST.
func verifiedQueries(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "internal/corpustest/corpus_test.go", nil, 0)
	if err != nil {
		t.Fatalf("parse corpus test: %v", err)
	}
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, el := range cl.Elts {
			inner, ok := el.(*ast.CompositeLit)
			if !ok || len(inner.Elts) == 0 {
				continue
			}
			lit, ok := inner.Elts[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			// Unquote handles both `raw` and "interpreted" string literals.
			if s, err := strconv.Unquote(lit.Value); err == nil {
				out[s] = true
			}
		}
		return true
	})
	return out
}

// The corpus verification lives in package corpustest and cannot import this
// package, so it restates the queries. This guard fails when the two drift —
// without it, an example could be added here and never verified, which is
// exactly the hole the corpus test exists to close.
func TestEveryExampleIsVerifiedByTheCorpusTest(t *testing.T) {
	verified := verifiedQueries(t)
	if len(verified) == 0 {
		t.Fatal("parsed no queries from the corpus test — the guard would pass vacuously")
	}
	for _, ex := range queryExamples {
		if !verified[ex.Query] {
			t.Errorf("example %q is not verified by TestQueryExamplesReturnResults — add it there", ex.Query)
		}
	}
}
