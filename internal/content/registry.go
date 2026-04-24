package content

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadRegistry reads a SSTconfig directory and builds the arrow + annotation registry
// from all .sst files found there.
//
// File roles by name pattern:
//   - arrows-*.sst  — arrow pair definitions
//   - annotations.sst — annotation symbol → arrow code mappings
//   - closures.sst  — arrow transitivity rules (loaded into LoadErrors for now;
//     transitivity enforcement is the graph layer's responsibility)
//
// Both "//" (line-level) and "#" (inline) comment styles are stripped before parsing.
//
// If sstconfigDir does not exist, a non-nil *SSTRegistry with empty maps and a non-nil
// error is returned (degraded mode). Callers SHOULD log a warning but MUST NOT crash.
func LoadRegistry(sstconfigDir string) (*SSTRegistry, error) {
	reg := &SSTRegistry{
		Arrows:      make(map[string]ArrowDef),
		Annotations: make(map[string]AnnotDef),
	}

	entries, err := os.ReadDir(sstconfigDir)
	if err != nil {
		return reg, fmt.Errorf("content: LoadRegistry: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), ".sst") {
			continue
		}
		path := filepath.Join(sstconfigDir, name)
		baseLower := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))

		switch {
		case baseLower == "annotations":
			if err := loadAnnotations(path, reg); err != nil {
				reg.LoadErrors = append(reg.LoadErrors, fmt.Errorf("content: %s: %w", name, err))
			}
		case baseLower == "closures":
			// Closures define arrow transitivity rules; enforcement is the graph layer's job.
			// We parse them silently so the file doesn't generate spurious errors.
			_ = loadClosures(path, reg)
		default:
			// arrows-*.sst and any other .sst files
			if err := loadArrows(path, reg); err != nil {
				reg.LoadErrors = append(reg.LoadErrors, fmt.Errorf("content: %s: %w", name, err))
			}
		}
	}

	return reg, nil
}

// stripComments removes // and # comments from a line, returning the trimmed remainder.
// "//" takes priority (whole line or inline). "#" strips inline content.
func stripComments(line string) string {
	// Strip // comments first (they may appear anywhere)
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = line[:idx]
	}
	// Strip # comments
	if idx := strings.Index(line, "#"); idx >= 0 {
		line = line[:idx]
	}
	return strings.TrimSpace(line)
}

// loadArrows parses an arrows-*.sst file and populates reg.Arrows.
//
// Format:
//   - First non-blank non-comment line: "- meta-type-name"
//   - Section headers: ":: label ::" (ignored)
//   - Arrow pairs: "+ forward name (code)" optionally followed by "- reverse name (revcode)"
//   - Lines starting with "# RESERVED" flag reserved arrows (still loaded)
func loadArrows(path string, reg *SSTRegistry) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var metaType string
	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// First pass: find meta-type from first content line
	for _, raw := range lines {
		clean := stripComments(raw)
		if clean == "" {
			continue
		}
		if strings.HasPrefix(clean, "- ") {
			metaType = strings.TrimSpace(strings.TrimPrefix(clean, "- "))
			break
		}
	}

	// Second pass: parse arrow pairs.
	// The SSTconfig format places both forward and reverse on the same line:
	//   + forward name (fcode) - reverse name (rcode)
	// or forward-only:
	//   + forward name (fcode)
	// Section headers ":: label ::" apply to all following arrow pairs until the
	// next header; they are captured as the arrow's Context.
	var section string
	for i := 0; i < len(lines); i++ {
		clean := stripComments(lines[i])
		if clean == "" {
			continue
		}
		// Skip meta-type line
		if strings.HasPrefix(clean, "- ") {
			continue
		}
		if strings.HasPrefix(clean, "::") {
			section = strings.TrimSpace(strings.Trim(clean, ":"))
			continue
		}

		// Forward arrow: starts with "+"
		if strings.HasPrefix(clean, "+") {
			fwd, rev, ok := parseArrowPair(clean[1:], metaType)
			if !ok {
				continue
			}
			fwd.Context = section
			reg.Arrows[fwd.Code] = fwd
			if rev.Code != "" {
				rev.Context = section
				reg.Arrows[rev.Code] = rev
			}
		}
	}

	return nil
}

// parseArrowPair parses the portion after the leading "+" character.
// Handles both single-line forward+reverse and forward-only formats:
//
//	" forward name (fcode) - reverse name (rcode)"
//	" forward name (fcode)"
func parseArrowPair(s, metaType string) (fwd, rev ArrowDef, ok bool) {
	s = strings.TrimSpace(s)
	// Find the FIRST "(code)" which terminates the forward name
	fOpen := strings.Index(s, "(")
	fClose := strings.Index(s, ")")
	if fOpen < 0 || fClose < fOpen {
		return
	}
	fCode := strings.TrimSpace(s[fOpen+1 : fClose])
	fName := strings.TrimSpace(s[:fOpen])
	if fCode == "" {
		return
	}

	rest := strings.TrimSpace(s[fClose+1:])
	fwd = ArrowDef{Code: fCode, LongName: fName, MetaType: metaType}

	// Check if rest starts with "-" indicating a reverse definition
	if strings.HasPrefix(rest, "-") {
		revPart := strings.TrimSpace(rest[1:])
		rOpen := strings.Index(revPart, "(")
		rClose := strings.Index(revPart, ")")
		if rOpen >= 0 && rClose > rOpen {
			rCode := strings.TrimSpace(revPart[rOpen+1 : rClose])
			rName := strings.TrimSpace(revPart[:rOpen])
			if rCode != "" {
				fwd.Reverse = rCode
				rev = ArrowDef{Code: rCode, LongName: rName, MetaType: metaType, Reverse: fCode}
			}
		}
	}

	ok = true
	return
}

// loadAnnotations parses annotations.sst and populates reg.Annotations.
//
// Format: lines of the form "symbol (code)" where symbol is one of
// ***, **, =, >>, %>, %<.
func loadAnnotations(path string, reg *SSTRegistry) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		clean := stripComments(scanner.Text())
		if clean == "" {
			continue
		}
		// Skip meta-type line and section headers
		if strings.HasPrefix(clean, "- ") || strings.HasPrefix(clean, "::") {
			continue
		}
		// Parse "symbol (code)"
		open := strings.Index(clean, "(")
		close := strings.Index(clean, ")")
		if open < 0 || close < open {
			continue
		}
		code := strings.TrimSpace(clean[open+1 : close])
		symbol := strings.TrimSpace(clean[:open])
		if symbol == "" || code == "" {
			continue
		}
		reg.Annotations[symbol] = AnnotDef{
			Symbol:    symbol,
			ArrowCode: code,
		}
	}
	return scanner.Err()
}

// loadClosures parses closures.sst. Transitivity rules are noted but not enforced here;
// enforcement is the graph layer's responsibility. We parse silently to avoid errors.
func loadClosures(path string, _ *SSTRegistry) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// Consume lines silently — transitivity is graph layer's concern.
	}
	return scanner.Err()
}
