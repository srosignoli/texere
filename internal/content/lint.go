package content

import (
	"strconv"
	"strings"
)

// LintCategory classifies a ParseError for the linter's purposes.
type LintCategory int

const (
	// CategoryDefect means the parser produced something other than what was
	// written: the author's intent was lost or reinterpreted. These are the
	// findings that mark a file as having a problem.
	CategoryDefect LintCategory = iota
	// CategoryVocabulary means an undeclared arrow was used. The note itself
	// is fine as written; the relation just will not become a graph edge
	// until the arrow is declared. This never marks a file.
	CategoryVocabulary
	// CategoryUnknown means the Msg did not match any recognised prefix.
	CategoryUnknown
)

// Classify maps a ParseError to its category by matching Msg against the
// Err* constants in errors.go. It is prefix matching only — it does not
// re-derive what the parser found.
func Classify(e ParseError) LintCategory {
	switch {
	case strings.HasPrefix(e.Msg, ErrUnknownArrow+":"):
		return CategoryVocabulary
	case strings.HasPrefix(e.Msg, ErrUnterminatedQuote+":"),
		strings.HasPrefix(e.Msg, ErrUnresolvedRef+":"),
		strings.HasPrefix(e.Msg, ErrMalformedMarker+":"),
		strings.HasPrefix(e.Msg, ErrInvalidPriority+":"),
		strings.HasPrefix(e.Msg, ErrEncoding+":"),
		strings.HasPrefix(e.Msg, ErrOrphanDitto+":"):
		return CategoryDefect
	default:
		return CategoryUnknown
	}
}

// Remediation returns the "what to do about it" half for a ParseError, or ""
// when there is nothing useful to say.
func Remediation(e ParseError) string {
	switch {
	case strings.HasPrefix(e.Msg, ErrUnknownArrow+":"):
		return "This arrow is not declared, so the relation will not become a graph edge. Declare it in SSTconfig to fix."
	case strings.HasPrefix(e.Msg, ErrUnterminatedQuote+":"):
		return "The quoted block was never closed, so its lines were read individually instead of as one block. Add the missing closing quote."
	case strings.HasPrefix(e.Msg, ErrUnresolvedRef+":"):
		return "This reference does not point to anything the parser has seen, so it was dropped. Fix the reference or add the note it should point to."
	case strings.HasPrefix(e.Msg, ErrMalformedMarker+":"):
		return "This marker could not be parsed, so it was ignored rather than acted on. Check its syntax against the marker's expected form."
	case strings.HasPrefix(e.Msg, ErrInvalidPriority+":"):
		return "This priority value is out of range, so it was ignored. Use a priority between the documented bounds."
	case strings.HasPrefix(e.Msg, ErrEncoding+":"):
		return "This file could not be read as valid text, so parsing stopped early. Re-save the file as UTF-8."
	case strings.HasPrefix(e.Msg, ErrOrphanDitto+":"):
		return "This ditto has no preceding line to repeat, so it was ignored. Remove it or add the line it should follow."
	default:
		return ""
	}
}

// ArrowName extracts the arrow from an ErrUnknownArrow message, unquoting
// it. The parser formats the arrow with %q, so the detail is quoted; two
// mentions of the same arrow would otherwise aggregate as separate names.
// Returns "" for any other kind.
func ArrowName(e ParseError) string {
	const prefix = ErrUnknownArrow + ": "
	if !strings.HasPrefix(e.Msg, prefix) {
		return ""
	}
	detail := strings.TrimPrefix(e.Msg, prefix)
	if unquoted, err := strconv.Unquote(detail); err == nil {
		return unquoted
	}
	return detail
}
