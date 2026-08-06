package search

import "strings"

// badDiracToken returns the first whitespace-delimited token that looks like
// Dirac notation but is malformed, or "" when the query is safe to decode.
//
// This guard exists because upstream does not fail gracefully here: any token
// starting '<' and ending '>' reaches SST.DiracNotation, and when the inside
// does not split into 2 or 3 parts on '|' it prints "Bad Dirac notation" and
// calls os.Exit(-1) (SSTorytime.go:6807-6808). That takes the whole process
// down — in the app the window vanishes and unsaved editor buffers go with it.
// The deferred recover() in Search cannot help: os.Exit is not a panic.
//
// The trigger is not exotic. `query-grammar.js` documents proximity search as
// `a <-> b`, which produces the token `<->` — one part, so it exits. The help
// panel and the query-translation prompt both teach it.
//
// Mirrors DiracNotation's own condition exactly (first byte '<', last byte '>')
// so this cannot reject a token upstream would have accepted.
func badDiracToken(query string) string {
	for _, tok := range strings.Fields(query) {
		if len(tok) < 2 || tok[0] != '<' || tok[len(tok)-1] != '>' {
			continue
		}
		if n := len(strings.Split(tok[1:len(tok)-1], "|")); n == 2 || n == 3 {
			continue
		}
		return tok
	}
	return ""
}
