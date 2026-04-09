package content

import "fmt"

// ParseError describes a single problem found during parsing of a .n4l file.
// A non-empty slice of ParseErrors does NOT mean parsing failed — partial results
// are always returned alongside any errors.
type ParseError struct {
	File string // Absolute path to the file
	Line int    // 1-indexed; 0 means the error is file-level (e.g., encoding error)
	Msg  string // Human-readable description
}

func (e ParseError) Error() string {
	if e.Line == 0 {
		return fmt.Sprintf("content: %s: %s", e.File, e.Msg)
	}
	return fmt.Sprintf("content: %s:%d: %s", e.File, e.Line, e.Msg)
}

// Error category constants for ParseError.Msg prefix matching.
const (
	ErrUnknownArrow    = "unknown arrow"
	ErrOrphanDitto     = "orphan ditto"
	ErrUnresolvedRef   = "unresolved reference"
	ErrInvalidPriority = "invalid priority"
	ErrMalformedMarker = "malformed marker"
	ErrEncoding        = "encoding error"
)
