package content

import (
	"sort"
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
		// Every reachable cause is either a line over the 64 KiB scanner
		// limit (parser.go:149, bufio.ErrTooLong) or the file failing to
		// open at all (parser.go:25 — permission denied, removed mid-scan).
		// Neither is an encoding problem, so branch on the detail rather
		// than giving one piece of advice that is wrong in every case.
		if strings.Contains(e.Msg, "token too long") {
			return "A single line in this file is too long for the parser to read (over 64 KB), so parsing stopped there. Break that line up."
		}
		return "This file could not be read to the end. Check that it still exists and is readable."
	case strings.HasPrefix(e.Msg, ErrOrphanDitto+":"):
		return "This ditto has no preceding line to repeat, so it was ignored. Remove it or add the line it should follow."
	default:
		return ""
	}
}

// kindOf returns the Err* constant that e.Msg matches, mirroring the same
// prefixes Classify checks. It backs LintDefect.Kind so callers can filter
// findings by kind without re-parsing Message.
func kindOf(e ParseError) string {
	for _, k := range []string{
		ErrUnknownArrow, ErrOrphanDitto, ErrUnresolvedRef,
		ErrInvalidPriority, ErrMalformedMarker, ErrEncoding, ErrUnterminatedQuote,
	} {
		if strings.HasPrefix(e.Msg, k+":") {
			return k
		}
	}
	return ""
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

// LintDefect is one CategoryDefect finding: a place where the parser produced
// something other than what was written.
type LintDefect struct {
	File        string `json:"file"` // collection-relative
	Line        int    `json:"line"`
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
}

// LintVocabulary aggregates every occurrence of one undeclared arrow across
// the whole collection. This is the entry point that keeps the Problems tab
// usable: on the real collection, 34,970 CategoryVocabulary findings reduce
// to 158 rows here.
type LintVocabulary struct {
	Arrow string   `json:"arrow"`
	Uses  int      `json:"uses"`
	Files []string `json:"files"` // collection-relative, sorted
}

// LintResult is the aggregate report produced by LintCollection.
type LintResult struct {
	Defects     []LintDefect     `json:"defects"`
	Vocabulary  []LintVocabulary `json:"vocabulary"` // sorted by Uses descending
	FilesLinted int              `json:"filesLinted"`
}

// LintCollection parses every .n4l file under dir (using ScanCollection's
// walk, so excluded directories are honoured the same way as the rest of the
// app) and splits every resulting ParseError into a defect or a vocabulary
// occurrence via Classify.
//
// Add no detection logic here: every finding traces back to a ParseError the
// parser already emitted inside ParseFile. This function only walks,
// classifies and aggregates.
func LintCollection(dir string, reg *SSTRegistry) (LintResult, error) {
	summaries, err := ScanCollection(dir)
	if err != nil {
		return LintResult{}, err
	}

	defects := make([]LintDefect, 0)
	vocab := make(map[string]*LintVocabulary)      // arrow name -> aggregate row
	vocabFiles := make(map[string]map[string]bool) // arrow name -> set of relpaths seen

	filesLinted := 0
	for _, s := range summaries {
		// Do not skip on s.ScanError: scanChapterTitles uses a default
		// bufio.Scanner too, so the exact condition that makes ParseFile emit
		// ErrEncoding (a line over 64 KiB, or the file being unreadable) also
		// sets ScanError here. Skipping would drop the file the user most
		// needs told about instead of reporting it. ParseFile is the source
		// of truth for findings; let it speak for itself — an unopenable
		// file still yields a real ParseError (Line: 0) that flows through
		// Classify unchanged.
		filesLinted++

		_, perrs := ParseFile(s.FilePath, reg)
		for _, e := range perrs {
			switch Classify(e) {
			case CategoryDefect:
				defects = append(defects, LintDefect{
					File:        s.RelPath,
					Line:        e.Line,
					Kind:        kindOf(e),
					Message:     e.Msg,
					Remediation: Remediation(e),
				})
			case CategoryVocabulary:
				name := ArrowName(e)
				v, ok := vocab[name]
				if !ok {
					v = &LintVocabulary{Arrow: name}
					vocab[name] = v
					vocabFiles[name] = make(map[string]bool)
				}
				v.Uses++
				vocabFiles[name][s.RelPath] = true
			case CategoryUnknown:
				// Msg did not match any recognised prefix; nothing to file
				// this under until the parser grows a corresponding kind.
			}
		}
	}

	sort.Slice(defects, func(i, j int) bool {
		if defects[i].File != defects[j].File {
			return defects[i].File < defects[j].File
		}
		return defects[i].Line < defects[j].Line
	})

	vocabulary := make([]LintVocabulary, 0, len(vocab))
	for name, v := range vocab {
		files := make([]string, 0, len(vocabFiles[name]))
		for f := range vocabFiles[name] {
			files = append(files, f)
		}
		sort.Strings(files)
		v.Files = files
		vocabulary = append(vocabulary, *v)
	}
	sort.Slice(vocabulary, func(i, j int) bool {
		if vocabulary[i].Uses != vocabulary[j].Uses {
			return vocabulary[i].Uses > vocabulary[j].Uses
		}
		return vocabulary[i].Arrow < vocabulary[j].Arrow
	})

	return LintResult{
		Defects:     defects,
		Vocabulary:  vocabulary,
		FilesLinted: filesLinted,
	}, nil
}
