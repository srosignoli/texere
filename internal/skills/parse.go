package skills

import "strings"

// Parse reads a skill file. It NEVER fails.
//
// Every design in this repo that silently dropped malformed input has cost a
// review round to find. A file the user is midway through editing must appear
// in the picker looking wrong, rather than vanishing and leaving them unsure
// whether the app read it at all. So anything unparseable degrades to "the
// whole file is the prompt", which is both harmless and visible.
//
// The frontmatter reader is deliberately tiny: three flat `key: value` strings
// do not justify a YAML dependency, and this repo hand-rolls its N4L parsing.
// The limitation is real and documented — flat keys only, no nesting, no
// multi-line values.
func Parse(id string, data []byte) Skill {
	s := Skill{ID: id, Name: id}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	front, body, ok := splitFrontmatter(text)
	if !ok {
		// No frontmatter, or an unterminated block: treat everything as prompt.
		s.Body = strings.TrimSpace(text)
		return s
	}

	for _, line := range strings.Split(front, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			if v := cleanValue(value); v != "" {
				s.Name = v
			}
		case "description":
			s.Description = cleanValue(value)
		case "placeholder":
			s.Placeholder = cleanValue(value)
		}
	}
	s.Body = strings.TrimSpace(body)
	return s
}

// splitFrontmatter returns the block between a leading "---" line and the next
// "---" line, plus everything after it. ok is false when the file does not open
// with a delimiter or the block is never closed — a "---" horizontal rule
// inside the prompt is therefore safe, because only the FIRST closing
// delimiter after a LEADING one counts.
func splitFrontmatter(text string) (front, body string, ok bool) {
	rest, found := strings.CutPrefix(text, "---\n")
	if !found {
		return "", "", false
	}
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		// Also accept a file that ends immediately after the closing delimiter.
		if trimmed, endFound := strings.CutSuffix(rest, "\n---"); endFound {
			return trimmed, "", true
		}
		return "", "", false
	}
	return rest[:end], rest[end+len("\n---\n"):], true
}

// cleanValue trims surrounding whitespace and one layer of matching quotes, so
// `name: "Socratic tutor"` and `name: Socratic tutor` mean the same thing.
func cleanValue(v string) string {
	v = strings.TrimSpace(v)
	for _, q := range []string{`"`, `'`} {
		if len(v) >= 2 && strings.HasPrefix(v, q) && strings.HasSuffix(v, q) {
			return v[1 : len(v)-1]
		}
	}
	return v
}
