package content

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ParseFile parses the .n4l file at path and returns the complete in-memory
// representation of its content.
//
// A non-nil *ParsedFile is always returned, even if ParseErrors are present —
// partial results are always returned alongside errors. Returns nil only if the
// file cannot be read at all.
//
// Performance contract: < 50 ms for a 10,000-line file.
func ParseFile(path string, registry *SSTRegistry) (*ParsedFile, []ParseError) {
	f, err := os.Open(path)
	if err != nil {
		return nil, []ParseError{{File: path, Line: 0, Msg: ErrEncoding + ": " + err.Error()}}
	}
	defer f.Close()

	pf := &ParsedFile{Path: path}
	state := &parserState{
		labelMap: make(map[string]*Node),
	}

	scanner := bufio.NewScanner(f)
	lineNum := 0

	// Multi-line single-quoted block state.
	// N4L allows content to span multiple physical lines when wrapped in '...'.
	// We only enter multi-line mode when the CONTENT TOKEN itself starts with '
	// (i.e. the trimmed line begins with ' or matches @label '...). This avoids
	// false positives from apostrophes inside comment lines or node text.
	var mlBuf []string // accumulated raw lines
	mlStart := 0       // line number of the opening line
	inSQ := false      // inside an unclosed single-quoted block
	inDQ := false      // inside an unclosed double-quoted paragraph block

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if inSQ {
			// Accumulate continuation lines; toggle inSQ for each ' encountered.
			mlBuf = append(mlBuf, line)
			for _, ch := range line {
				if ch == '\'' {
					inSQ = !inSQ
				}
			}
			if !inSQ {
				// Block closed — process the whole joined block as one line.
				processLine(pf, state, path, strings.Join(mlBuf, "\n"), mlStart, registry)
				mlBuf = nil
			}
			continue
		}

		if inDQ {
			// Accumulate continuation lines; toggle inDQ for each " encountered.
			mlBuf = append(mlBuf, line)
			for _, ch := range line {
				if ch == '"' {
					inDQ = !inDQ
				}
			}
			if !inDQ {
				// Block closed — strip the outer "..." delimiters and process as a
				// plain node (not a ditto) so the inner content is never re-parsed
				// as a ditto line or arrow target.
				joined := strings.Join(mlBuf, "\n")
				tj := strings.TrimSpace(joined)
				if len(tj) >= 2 && tj[0] == '"' && tj[len(tj)-1] == '"' {
					tj = tj[1 : len(tj)-1] // strip outer "..."
				}
				processLine(pf, state, path, tj, mlStart, registry)
				mlBuf = nil
			}
			continue
		}

		// Single-quote multi-line: only when the content token itself starts with '.
		// Eligible: lines trimmed to start with ' or @label '...
		if contentSQStart(line) {
			openQuote := false
			for _, ch := range line {
				if ch == '\'' {
					openQuote = !openQuote
				}
			}
			if openQuote {
				mlBuf = []string{line}
				mlStart = lineNum
				inSQ = true
				continue
			}
		}

		// Double-quote multi-line paragraph: a "..." block whose content spans
		// multiple physical lines. Only triggered when:
		//   • The trimmed line starts with "
		//   • The content after " has no " character (no inline closing quote)
		//   • The content has non-empty display text and no arrow pattern
		if contentDQMultiLine(line) {
			mlBuf = []string{line}
			mlStart = lineNum
			inDQ = true
			continue
		}

		processLine(pf, state, path, line, lineNum, registry)
	}
	// An unclosed block at EOF is a typo, not a construct. Joining the buffer into
	// a single line silently absorbed every line below the opening quote — node
	// text, "# @id" and "# @card" markers, arrows, all of it — so one stray
	// leading quote made the remainder of the file invisible: no nodes, no ids,
	// no cards, no extracts, and not one ParseError to explain it.
	//
	// Recover instead: parse the buffered lines individually, exactly as they
	// would have been read had the quote never opened, and report the opening
	// line. The opening line itself is reparsed verbatim, so a lone `"` there
	// still reads as a ditto (or an orphan-ditto error) — which is what an
	// unmatched quote line means in N4L anyway.
	if (inSQ || inDQ) && len(mlBuf) > 0 {
		pf.Errors = append(pf.Errors, ParseError{
			File: path,
			Line: mlStart,
			Msg:  ErrUnterminatedQuote + ": quoted block opened here is never closed; its lines were read individually",
		})
		for i, raw := range mlBuf {
			processLine(pf, state, path, raw, mlStart+i, registry)
		}
	}

	if err := scanner.Err(); err != nil {
		pf.Errors = append(pf.Errors, ParseError{File: path, Line: lineNum, Msg: ErrEncoding + ": " + err.Error()})
	}

	// Close the last open chapter
	if state.currentChapter != nil {
		closeChapter(state)
		pf.Chapters = append(pf.Chapters, *state.currentChapter)
	}

	// Second pass: resolve pending label references
	resolvePendingRefs(pf, state, path)

	return pf, pf.Errors
}

// ---------------------------------------------------------------------------
// Internal state
// ---------------------------------------------------------------------------

type studyBlockState struct {
	kind         BlockType
	forceReverse bool
}

type pendingRef struct {
	refStr     string // e.g., "$anchor1.1" or "$PREV.3"
	targetNode *Node  // the node that contains the reference token in its Text
	chapter    *Chapter
}

type parserState struct {
	currentChapter       *Chapter
	effectiveCtx         map[string]struct{}
	dittoParent          *Node
	dittoParentFromQuote bool // true when dittoParent was created from a `"` line
	dittoParentIndent    int  // leading-space count of the `"` line that created dittoParent
	studyBlock           studyBlockState
	pendingStudy         *StudyMeta
	pendingIDMarker      string // shortid from "# @id <shortid>"
	pendingIDLine        int    // line number of the "# @id" marker
	pendingPriority      *int
	pendingProv          Provenance
	labelMap             map[string]*Node
	pendingRefs          []pendingRef
	// prevObject holds the most recent real (non-ditto) arrow target seen, so a
	// lone `"` in object position — N4L object-ditto, e.g. `tram (fwd) "` meaning
	// `tram (fwd) target 3` — resolves to the line above's object instead of
	// creating a spurious node literally named `"`. Reset at each chapter.
	prevObject string
}

// ---------------------------------------------------------------------------
// Line dispatching
// ---------------------------------------------------------------------------

func processLine(pf *ParsedFile, state *parserState, path, line string, lineNum int, reg *SSTRegistry) {
	// Strip // inline comments first (preserves leading whitespace for ditto indent detection).
	line = stripLineComment(line)

	// Strip leading whitespace first — N4L files freely indent all line types.
	trimmed := strings.TrimSpace(line)

	// Chapter title: "- title" or "-title"; rejects "--", "-::" and bare "-".
	if len(trimmed) > 1 && trimmed[0] == '-' && trimmed[1] != '-' && trimmed[1] != ':' && strings.TrimSpace(trimmed[1:]) != "" {
		openChapter(pf, state, trimmed, lineNum)
		return
	}
	if trimmed == "" {
		return
	}

	// Context operators (check before comments — "::" is not a comment char)
	if strings.HasPrefix(trimmed, "::") || strings.HasPrefix(trimmed, "+::") || strings.HasPrefix(trimmed, "-::") {
		parseContextOp(state, trimmed, lineNum)
		return
	}

	// Marker lines (# @...) — must come before the generic comment check
	if strings.HasPrefix(trimmed, "# @") || strings.HasPrefix(trimmed, "#@") {
		parseMarker(pf, state, path, trimmed, lineNum)
		return
	}

	// Pure comment lines
	if strings.HasPrefix(trimmed, "#") {
		return
	}

	// Ditto lines: `"` followed by content — pass the original (unstripped) line
	// so parseDitto can distinguish column-0 root nodes from indented dittos.
	if strings.HasPrefix(trimmed, `"`) {
		parseDitto(pf, state, path, line, lineNum, reg)
		return
	}

	// Labeled ditto: @label "... — a label prefix directly on a ditto-style line.
	// Register the label pointing to the current parent node, then dispatch to
	// parseDitto so the line appears as a continuation of that node rather than
	// as a standalone root. Example: `@meirule " (rule) 没/méi Negates Past Actions`
	if m := labelPrefixRe.FindStringSubmatch(trimmed); m != nil {
		rest := strings.TrimSpace(trimmed[len(m[0]):])
		if strings.HasPrefix(rest, `"`) {
			ch := state.currentChapter
			if ch != nil && len(ch.Nodes) > 0 {
				state.labelMap[m[1]] = &ch.Nodes[len(ch.Nodes)-1]
			}
			parseDitto(pf, state, path, rest, lineNum, reg)
			return
		}
	}

	// Node line (ignored before first chapter)
	if state.currentChapter != nil {
		parseNode(pf, state, path, trimmed, lineNum, reg)
	}
}

// ---------------------------------------------------------------------------
// Chapter management
// ---------------------------------------------------------------------------

func openChapter(pf *ParsedFile, state *parserState, line string, lineNum int) {
	// Close previous chapter
	if state.currentChapter != nil {
		closeChapter(state)
		pf.Chapters = append(pf.Chapters, *state.currentChapter)
	}
	title := strings.TrimSpace(line[1:]) // handles both "- title" and "-title"
	state.currentChapter = &Chapter{
		Title:     title,
		LineStart: lineNum,
		Labels:    make(map[string]*Node),
	}
	state.effectiveCtx = make(map[string]struct{})
	state.dittoParent = nil
	state.dittoParentFromQuote = false
	state.dittoParentIndent = 0
	state.prevObject = ""
	state.labelMap = make(map[string]*Node)
	// Study block closed by chapter boundary? No — blocks persist across chapters in same file
}

func closeChapter(state *parserState) {
	state.currentChapter.Labels = state.labelMap
}

// ---------------------------------------------------------------------------
// Context state machine (T016)
// ---------------------------------------------------------------------------

func parseContextOp(state *parserState, line string, _ int) {
	if state.effectiveCtx == nil {
		state.effectiveCtx = make(map[string]struct{})
	}

	switch {
	case strings.HasPrefix(line, "+::"):
		// Union: add tags to existing set
		tags := parseTags(strings.TrimPrefix(line, "+::"))
		for _, t := range tags {
			state.effectiveCtx[t] = struct{}{}
		}
	case strings.HasPrefix(line, "-::"):
		// Subtract: remove tags from existing set
		tags := parseTags(strings.TrimPrefix(line, "-::"))
		for _, t := range tags {
			delete(state.effectiveCtx, t)
		}
	default:
		// Replace (starts with "::")
		tags := parseTags(strings.TrimPrefix(line, "::"))
		state.effectiveCtx = make(map[string]struct{})
		for _, t := range tags {
			state.effectiveCtx[t] = struct{}{}
		}
		// ":: " (replace) closes the current study block
		state.studyBlock = studyBlockState{}
	}
}

// parseTags extracts context tags from a raw context-operator string.
// Tags are comma-separated; each entry is trimmed of surrounding whitespace.
// The optional trailing "::" closing delimiter is stripped before splitting.
func parseTags(s string) []string {
	// Strip trailing "::" or stray ":" / spaces (closing delimiter)
	s = strings.TrimRight(s, ": ")
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	tags := parts[:0]
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// snapshotCtx returns a sorted copy of the current effective context.
func snapshotCtx(state *parserState) []string {
	if len(state.effectiveCtx) == 0 {
		return nil
	}
	tags := make([]string, 0, len(state.effectiveCtx))
	for t := range state.effectiveCtx {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

// ---------------------------------------------------------------------------
// Marker lines (T020 + T023 + T030)
// ---------------------------------------------------------------------------

func parseMarker(pf *ParsedFile, state *parserState, path, line string, lineNum int) {
	// Normalise: ensure there's a space after #
	marker := line
	if strings.HasPrefix(marker, "#@") {
		marker = "# " + marker[1:]
	}
	rest := strings.TrimSpace(strings.TrimPrefix(marker, "#"))
	rest = strings.TrimPrefix(rest, "@")
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "id":
		// # @id <shortid>
		if len(parts) >= 2 {
			state.pendingIDMarker = parts[1]
			state.pendingIDLine = lineNum
		}

	case "card":
		// # @card, # @card reverse, # @card no-reverse
		sm := &StudyMeta{CardType: CardTypeBasic}
		if len(parts) >= 2 {
			switch strings.Join(parts[1:], " ") {
			case "reverse":
				sm.ForceReverse = true
				sm.CardType = CardTypeReverse
			case "no-reverse":
				sm.SuppressReverse = true
			default:
				pf.Errors = append(pf.Errors, ParseError{File: path, Line: lineNum, Msg: ErrMalformedMarker + ": # @card " + strings.Join(parts[1:], " ")})
			}
		}
		state.pendingStudy = sm

	case "cloze":
		state.pendingStudy = &StudyMeta{CardType: CardTypeCloze}

	case "study":
		// # @study, # @study reverse, # @study sequence
		modifier := ""
		if len(parts) >= 2 {
			modifier = strings.Join(parts[1:], " ")
		}
		var bt BlockType
		switch modifier {
		case "":
			bt = BlockTypeStudy
		case "reverse":
			bt = BlockTypeStudyReverse
		case "sequence":
			bt = BlockTypeStudySequence
		default:
			pf.Errors = append(pf.Errors, ParseError{File: path, Line: lineNum, Msg: ErrMalformedMarker + ": # @study " + modifier})
			return
		}
		state.studyBlock = studyBlockState{kind: bt}
		state.pendingStudy = nil

	case "end":
		state.studyBlock = studyBlockState{}

	case "priority":
		if len(parts) >= 2 {
			n, err := strconv.Atoi(parts[1])
			if err != nil || n < 0 || n > 100 {
				pf.Errors = append(pf.Errors, ParseError{File: path, Line: lineNum, Msg: fmt.Sprintf("%s: %s", ErrInvalidPriority, parts[1])})
			} else {
				v := n
				state.pendingPriority = &v
			}
		}

	case "source":
		state.pendingProv.Source = strings.Join(parts[1:], " ")

	case "ref":
		state.pendingProv.Ref = strings.Join(parts[1:], " ")

	case "progress":
		state.pendingProv.Progress = strings.Join(parts[1:], " ")
	}
}

// ---------------------------------------------------------------------------
// Node lines (T015 + T018 + T019 + T020)
// ---------------------------------------------------------------------------

var labelPrefixRe = regexp.MustCompile(`^@([A-Za-z][A-Za-z0-9_-]*)\s+`)
var refTokenRe = regexp.MustCompile(`\$([A-Za-z][A-Za-z0-9_-]*)\.(\d+)|\$(PREV)\.(\d+)`)

// hasFromArrowToken reports whether text contains a "(from)" extract-anchor arrow
// whose target is a quoted string — i.e. (from) followed (after optional spaces/tabs)
// by a double-quote. This is the format AppendExtract always emits:
//
//	<text> (from) "<anchor>"
//
// A card whose front text contains a bare "(from)" without a quoted target (e.g.
// "Where does rain come (from)? >> clouds") must NOT match, so that the ">>"
// remains literal card text instead of being swallowed as an arrow target.
func hasFromArrowToken(text string) bool {
	i := strings.Index(text, "(from)")
	if i < 0 {
		return false
	}
	rest := strings.TrimLeft(text[i+len("(from)"):], " \t")
	return strings.HasPrefix(rest, `"`)
}

func parseNode(pf *ParsedFile, state *parserState, path, line string, lineNum int, reg *SSTRegistry) {
	if state.currentChapter == nil {
		return
	}

	rawText := line
	nodeText := strings.TrimSpace(rawText)

	// Strip trailing inline comment from node text for display
	nodeText = stripNodeComment(nodeText)

	// Detect @label prefix
	var label string
	if m := labelPrefixRe.FindStringSubmatch(nodeText); m != nil {
		label = m[1]
		nodeText = strings.TrimPrefix(nodeText, m[0])
		nodeText = strings.TrimSpace(nodeText)
	}

	// Build study meta first — a study marker (without a (from) anchor) means this
	// line is a card, not a relationship, so its text is taken literally and arrows
	// are NOT parsed from it.
	var sm *StudyMeta
	if state.pendingStudy != nil {
		sm = state.pendingStudy
		state.pendingStudy = nil
	} else if state.studyBlock.kind != BlockTypeNone {
		sm = studyMetaFromBlock(state.studyBlock)
	}

	var cleanText string
	var arrows []Arrow
	if sm != nil && !hasFromArrowToken(nodeText) {
		// Card/cloze/study node without a (from) anchor: the whole line is literal
		// card text (with ">>" kept literal for the generator to split).
		cleanText = nodeText
	} else {
		cleanText, arrows = extractArrows(nodeText, lineNum, false, reg, path, pf)
		resolveObjectDittos(state, arrows)
	}

	// Extract cloze targets from node text
	if sm != nil && sm.CardType == CardTypeCloze {
		sm.ClozeTargets = extractClozeTargets(cleanText)
	}

	// Apply priority and provenance
	var prio *int
	if state.pendingPriority != nil {
		prio = state.pendingPriority
		state.pendingPriority = nil
	}
	var prov *Provenance
	if state.pendingProv != (Provenance{}) {
		p := state.pendingProv
		prov = &p
		state.pendingProv = Provenance{}
	}

	// Compute node ID from pending # @id marker
	var id string
	var idMarkerLine int
	if state.pendingIDMarker != "" {
		idMarkerLine = state.pendingIDLine
		id = NodeID(path, state.currentChapter.Title, state.pendingIDMarker)
		state.pendingIDMarker = ""
		state.pendingIDLine = 0
	}

	ctx := snapshotCtx(state)
	_, isSeq := state.effectiveCtx["_sequence_"]

	node := &Node{
		ID:           id,
		Text:         cleanText,
		Line:         lineNum,
		EffectiveCtx: ctx,
		IsSequence:   isSeq,
		Arrows:       arrows,
		StudyMeta:    sm,
		Priority:     prio,
		Provenance:   prov,
		Label:        label,
		IDMarkerLine: idMarkerLine,
	}

	// Register label
	if label != "" {
		state.labelMap[label] = node
	}

	// Collect $label.N references for second-pass resolution
	if refs := refTokenRe.FindAllString(rawText, -1); len(refs) > 0 {
		for _, r := range refs {
			state.pendingRefs = append(state.pendingRefs, pendingRef{
				refStr:     r,
				targetNode: node,
				chapter:    state.currentChapter,
			})
		}
	}

	state.currentChapter.Nodes = append(state.currentChapter.Nodes, *node)
	state.dittoParent = &state.currentChapter.Nodes[len(state.currentChapter.Nodes)-1]
	state.dittoParentFromQuote = false
	state.dittoParentIndent = 0

	// Collect reminders
	if isReminder(cleanText) {
		pf.Reminders = append(pf.Reminders, Reminder{
			File: path,
			Line: lineNum,
			Text: cleanText,
		})
	}
}

// resolveObjectDittos rewrites N4L object-ditto targets in place. A lone `"`
// in object position (e.g. `tram (fwd) "`) means "repeat the previous arrow's
// object" — the value on the line above. extractArrows already strips paired
// `"…"`, so any surviving single-`"` target is unambiguously an object-ditto.
// Each resolved/real target updates state.prevObject so a run of dittos all
// inherit the same original object. Targets that can't resolve (no prior
// object yet) are left untouched for the downstream unknown-target handling.
func resolveObjectDittos(state *parserState, arrows []Arrow) {
	for i := range arrows {
		if arrows[i].Target == `"` {
			if state.prevObject != "" {
				arrows[i].Target = state.prevObject
			}
			continue
		}
		if arrows[i].Target != "" {
			state.prevObject = arrows[i].Target
		}
	}
}

// stripNodeComment removes a trailing "# ..." comment from a node text line.
func stripNodeComment(s string) string {
	// Only strip if there's a " #" (space before hash) to avoid stripping
	// comment chars inside arrow targets.
	if idx := strings.Index(s, " #"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// ---------------------------------------------------------------------------
// Ditto lines (T017)
// ---------------------------------------------------------------------------

func parseDitto(pf *ParsedFile, state *parserState, path, line string, lineNum int, reg *SSTRegistry) {
	// Count leading spaces/tabs to determine indentation level.
	currentIndent := 0
	for _, ch := range line {
		if ch == ' ' {
			currentIndent++
		} else if ch == '\t' {
			currentIndent += 4
		} else {
			break
		}
	}

	trimmed := strings.TrimSpace(line)
	content := strings.TrimSpace(strings.TrimPrefix(trimmed, `"`))
	if content == "" {
		return
	}

	// Determine root vs. ditto using three rules in priority order:
	//
	//  1. No preceding node → always a root node.
	//  2. Preceding node was itself a `"` root AND this line is strictly MORE
	//     indented → always a ditto (indented annotation beneath a `"` root).
	//  3. Otherwise → inspect the display text:
	//       - Non-empty display text (content before the first arrow) → root.
	//       - Empty display text (pure arrow line such as `(note) ...`) → ditto.
	//
	// Rule 3 makes `"` lines with content (e.g. LaTeX math followed by an arrow)
	// behave as root nodes regardless of whether the preceding root was a regular
	// line or a `"` line, while keeping pure annotation lines as dittos in
	// simple.n4l-style files.
	isRoot := false
	if state.dittoParent == nil {
		isRoot = true
	} else if state.dittoParentFromQuote && currentIndent > state.dittoParentIndent {
		isRoot = false // indented beneath a `"` root → annotation ditto
	} else {
		isRoot = quoteLineHasDisplayText(content)
	}

	if isRoot {
		parseNode(pf, state, path, content, lineNum, reg)
		// parseNode resets dittoParentFromQuote/Indent; override to record that
		// this dittoParent was established by a `"` line.
		state.dittoParentFromQuote = true
		state.dittoParentIndent = currentIndent
		return
	}

	// Ditto: annotation/continuation of the immediately preceding node.
	displayText, arrows := extractArrows(content, lineNum, true, reg, path, pf)
	resolveObjectDittos(state, arrows)
	ditto := DittoLine{
		Line:   lineNum,
		Text:   displayText,
		Arrows: arrows,
	}
	ch := state.currentChapter
	if ch != nil && len(ch.Nodes) > 0 {
		ch.Nodes[len(ch.Nodes)-1].Dittos = append(ch.Nodes[len(ch.Nodes)-1].Dittos, ditto)
	}
}

// quoteLineHasDisplayText reports whether a `"` line's content (after the `"`
// prefix is stripped) has non-empty display text — i.e., text that precedes
// the first arrow code. Lines that start directly with an arrow such as
// `(note) some annotation` return false (pure ditto); lines with textual or
// mathematical content before the arrow return true (root node).
func quoteLineHasDisplayText(content string) bool {
	masked := maskSpecial(content)
	arrowStart := strings.IndexByte(masked, '(')
	var display string
	if arrowStart < 0 {
		display = stripTrailingQuote(content)
	} else {
		display = strings.TrimSpace(content[:arrowStart])
		if len(display) > 0 && display[len(display)-1] == '"' {
			display = strings.TrimSpace(display[:len(display)-1])
		}
	}
	return display != ""
}

// ---------------------------------------------------------------------------
// Arrow extraction (T019)
// ---------------------------------------------------------------------------

// contentSQStart reports whether line begins a content token that starts with a
// single quote. Only two patterns qualify:
//
//	'...        — trimmed line starts directly with '
//	@label '... — trimmed line is a labeled node whose content starts with '
//
// This deliberately excludes comment lines (#), context ops (::), chapter
// headers (-), ditto lines ("), and regular node text with embedded apostrophes,
// preventing false multi-line triggers from stray ' in comments or prose.
func contentSQStart(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "'") {
		return true
	}
	if strings.HasPrefix(trimmed, "@") {
		// @label followed eventually by '
		idx := strings.IndexByte(trimmed, '\'')
		return idx > 0
	}
	return false
}

// contentDQMultiLine reports whether line is the opening of a multi-line
// double-quoted paragraph block — i.e. a `"` node line whose content spans
// multiple physical lines. The three conditions that must all hold:
//
//  1. Trimmed line starts with `"` (N4L ditto/node prefix).
//  2. The content after the `"` prefix contains no `"` character — if a
//     closing `"` were present, the block would already be closed on this line.
//  3. The content has non-empty display text with no arrow pattern in the
//     masked text — a pure-arrow ditto like `"  (means) ...` is never a
//     paragraph opener.
func contentDQMultiLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, `"`) {
		return false
	}
	content := strings.TrimSpace(trimmed[1:]) // strip the leading "
	if content == "" {
		return false
	}
	// Must not contain a closing " anywhere — that would make it single-line.
	if strings.ContainsRune(content, '"') {
		return false
	}
	// Must have no arrow pattern in the masked text (arrows → single-line ditto).
	masked := maskSpecial(content)
	if strings.IndexByte(masked, '(') >= 0 {
		return false
	}
	// Must have non-empty display text.
	return strings.TrimSpace(content) != ""
}

// maskSpecial replaces content that must not be scanned for arrows with space
// characters of the same length, preserving character-level positions so that
// an index into the masked string is also a valid index into the original.
//
// Masked regions:
//   - LaTeX inline math  \( ... \)
//   - LaTeX display math \[ ... \]
//   - Single-quoted strings  '...'
//
// Double-quoted strings are NOT masked here because in N4L files the line
// prefix `"` has already been stripped, making any remaining `"` characters
// ambiguous (they may be closing quotes of the display text or opening quotes
// of an arrow target).
func maskSpecial(text string) string {
	b := []byte(text)
	i := 0
	for i < len(b) {
		// LaTeX inline math \( ... \)
		if i+1 < len(b) && b[i] == '\\' && b[i+1] == '(' {
			start := i
			i += 2
			for i+1 < len(b) {
				if b[i] == '\\' && b[i+1] == ')' {
					i += 2
					break
				}
				i++
			}
			for j := start; j < i; j++ {
				b[j] = ' '
			}
			continue
		}
		// LaTeX display math \[ ... \]
		if i+1 < len(b) && b[i] == '\\' && b[i+1] == '[' {
			start := i
			i += 2
			for i+1 < len(b) {
				if b[i] == '\\' && b[i+1] == ']' {
					i += 2
					break
				}
				i++
			}
			for j := start; j < i; j++ {
				b[j] = ' '
			}
			continue
		}
		// Single-quoted strings '...'
		//
		// Two rules prevent apostrophes in natural language from being treated as
		// string delimiters:
		//
		//   Opening rule: the ' must NOT be preceded by an ASCII letter.
		//     Rejects contractions like "don't", "I'm", "it's" where the apostrophe
		//     follows a letter.  Chinese/multi-byte chars before ' are fine (their
		//     high bytes are not in [A-Za-z]) so "méishì 'r" still reaches the
		//     closing-rule check.
		//
		//   Closing rule: a candidate closing ' must NOT be followed by an ASCII
		//     letter.  Rejects "méishì 'r (ph) ... it's okay" — the ' in "it's"
		//     is followed by 's', so it is not a valid close, and the whole region
		//     is left unmasked.
		if b[i] == '\'' {
			// Opening rule.
			if i > 0 && isASCIILetter(b[i-1]) {
				i++
				continue
			}
			// Scan forward for the first valid closing quote.
			j := i + 1
			for j < len(b) {
				if b[j] == '\'' && (j+1 >= len(b) || !isASCIILetter(b[j+1])) {
					break // Valid close found.
				}
				j++
			}
			if j < len(b) {
				// Mask the entire quoted region including both delimiters.
				for k := i; k <= j; k++ {
					b[k] = ' '
				}
				i = j + 1
			} else {
				// No valid closing quote — skip this apostrophe.
				i++
			}
			continue
		}
		i++
	}
	return string(b)
}

// stripLineComment removes a trailing // comment from s, leaving the content
// before it (right-trimmed). Occurrences inside LaTeX blocks or single-quoted
// strings (already masked by maskSpecial) are ignored, as are URL protocol
// separators such as "://" (the // is skipped when immediately preceded by ':').
func stripLineComment(s string) string {
	masked := maskSpecial(s)
	for i := 0; i < len(masked)-1; i++ {
		if masked[i] == '/' && masked[i+1] == '/' {
			// Part of a URL protocol separator ("://", "file:///", ...) — not a
			// comment. Walk back to the start of this slash run; if it begins
			// immediately after a ':', the slashes belong to the URL scheme.
			j := i
			for j > 0 && masked[j-1] == '/' {
				j--
			}
			if j > 0 && masked[j-1] == ':' {
				continue
			}
			return strings.TrimRight(s[:i], " \t")
		}
	}
	return s
}

// stripTrailingQuote removes at most one trailing `"` character (and surrounding
// whitespace) from s. It handles the N4L convention where a line such as
// `"Display text" (code) "Target"` uses the leading `"` as both the ditto prefix
// and the opening quote; after the prefix is stripped, the display text retains a
// spurious closing `"`.
//
// It also strips matching outer single quotes from verbatim blocks such as
// `'multi-line content'` so that the stored node text contains only the content.
// isASCIILetter reports whether b is an ASCII letter (a–z or A–Z).
func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func stripTrailingQuote(s string) string {
	s = strings.TrimSpace(s)
	// Strip matching outer single quotes (verbatim / code-block convention).
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	// Strip a single spurious trailing double-quote.
	if len(s) > 0 && s[len(s)-1] == '"' {
		s = strings.TrimSpace(s[:len(s)-1])
	}
	return s
}

// extractArrows parses arrow patterns from a line of text.
// Returns the display text (arrow tokens removed) and the parsed Arrow slice.
//
// Arrow detection is performed on a masked copy of the text (LaTeX blocks and
// single-quoted strings replaced with spaces) so that parentheses inside math
// or quoted content are never misidentified as arrow codes.
func extractArrows(text string, lineNum int, isDitto bool, reg *SSTRegistry, path string, pf *ParsedFile) (string, []Arrow) {
	var arrows []Arrow
	displayText := text
	remaining := text
	firstPass := true

	for {
		masked := maskSpecial(remaining)

		// Find the next unmasked '(' — start of a potential (code) arrow.
		arrowStart := strings.IndexByte(masked, '(')
		if arrowStart < 0 {
			break
		}

		// Find the matching closing ')'.
		relClose := strings.IndexByte(masked[arrowStart+1:], ')')
		if relClose < 0 {
			break
		}
		closeCode := arrowStart + 1 + relClose

		code := strings.TrimSpace(remaining[arrowStart+1 : closeCode])
		afterCode := remaining[closeCode+1:]

		// Target = text up to the next unmasked '(' (start of next arrow), or end of string.
		maskedAfter := maskSpecial(afterCode)
		nextArrow := strings.IndexByte(maskedAfter, '(')
		var target, nextRemaining string
		if nextArrow >= 0 {
			target = strings.TrimSpace(afterCode[:nextArrow])
			nextRemaining = afterCode[nextArrow:]
		} else {
			target = strings.TrimSpace(afterCode)
			nextRemaining = ""
		}

		// Strip outer double-quotes from the target (e.g. `"LaTeX expression"`).
		if len(target) >= 2 && target[0] == '"' && target[len(target)-1] == '"' {
			target = strings.TrimSpace(target[1 : len(target)-1])
		}

		// An arrow must have a non-empty target. A `(code)` with nothing after it
		// is parenthetical prose — stop scanning.
		if target == "" {
			break
		}

		if firstPass {
			// Display text is everything before the first arrow; strip trailing `"`.
			displayText = strings.TrimSpace(remaining[:arrowStart])
			if len(displayText) > 0 && displayText[len(displayText)-1] == '"' {
				displayText = strings.TrimSpace(displayText[:len(displayText)-1])
			}
			firstPass = false
		}

		_, registered := reg.Arrows[code]
		if !registered {
			pf.Errors = append(pf.Errors, ParseError{File: path, Line: lineNum, Msg: fmt.Sprintf("%s: %q", ErrUnknownArrow, code)})
		}

		arrows = append(arrows, Arrow{
			Code:         code,
			Target:       target,
			IsRegistered: registered,
			Line:         lineNum,
			IsDitto:      isDitto,
		})

		remaining = nextRemaining
		if remaining == "" {
			break
		}
	}

	if len(arrows) == 0 {
		return stripTrailingQuote(text), nil
	}
	return displayText, arrows
}

// ---------------------------------------------------------------------------
// Label second-pass resolution (T018)
// ---------------------------------------------------------------------------

func resolvePendingRefs(pf *ParsedFile, state *parserState, path string) {
	// Build a global label map from all chapters
	allLabels := make(map[string]*Node)
	for i := range pf.Chapters {
		for k, v := range pf.Chapters[i].Labels {
			allLabels[k] = v
		}
	}

	for _, ref := range state.pendingRefs {
		m := refTokenRe.FindStringSubmatch(ref.refStr)
		if m == nil {
			continue
		}

		var labelName string
		if m[3] == "PREV" {
			// $PREV.N — find the node preceding the target node in its chapter
			labelName = resolvePREV(ref.targetNode, ref.chapter)
		} else {
			labelName = m[1]
		}

		if labelName == "" {
			pf.Errors = append(pf.Errors, ParseError{
				File: path,
				Line: ref.targetNode.Line,
				Msg:  fmt.Sprintf("%s: %s", ErrUnresolvedRef, ref.refStr),
			})
			continue
		}

		if _, ok := allLabels[labelName]; !ok {
			pf.Errors = append(pf.Errors, ParseError{
				File: path,
				Line: ref.targetNode.Line,
				Msg:  fmt.Sprintf("%s: %s", ErrUnresolvedRef, ref.refStr),
			})
		}
		// Reference resolved — the arrow target already contains the ref string;
		// callers can look up the label in Chapter.Labels for the actual node pointer.
	}
}

func resolvePREV(node *Node, ch *Chapter) string {
	if ch == nil {
		return ""
	}
	for i, n := range ch.Nodes {
		if n.Line == node.Line && i > 0 {
			return ch.Nodes[i-1].Label
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Study meta helpers (T023)
// ---------------------------------------------------------------------------

func studyMetaFromBlock(block studyBlockState) *StudyMeta {
	sm := &StudyMeta{FromBlock: true, BlockType: block.kind}
	switch block.kind {
	case BlockTypeStudy:
		sm.CardType = CardTypeBasic
	case BlockTypeStudyReverse:
		sm.CardType = CardTypeReverse
		sm.ForceReverse = true
	case BlockTypeStudySequence:
		sm.CardType = CardTypeSequenceForward
	}
	return sm
}

// HasClozeTargets reports whether text contains at least one {{double-brace}}
// cloze group. Used to refuse promoting an extract to a cloze card when the
// passage has no deletion marked (which would produce zero cards).
func HasClozeTargets(text string) bool {
	return len(extractClozeTargets(text)) > 0
}

// extractClozeTargets finds the contents of each {{double-brace}} group in text,
// in order. A single {brace} is literal text and is ignored.
func extractClozeTargets(text string) []string {
	var targets []string
	i := 0
	for i+1 < len(text) {
		if text[i] == '{' && text[i+1] == '{' {
			if j := strings.Index(text[i+2:], "}}"); j >= 0 {
				targets = append(targets, text[i+2:i+2+j])
				i = i + 2 + j + 2
				continue
			}
			break // unmatched "{{" — no further groups
		}
		i++
	}
	return targets
}

// ---------------------------------------------------------------------------
// Reminder detection (T029)
// ---------------------------------------------------------------------------

var allCapsTokenRe = regexp.MustCompile(`^[A-Z0-9\-_']+$`)

// isReminder returns true if text is an ALL CAPS reminder (≥ 3 whitespace-separated
// tokens, each matching [A-Z0-9\-_']+).
func isReminder(text string) bool {
	tokens := strings.Fields(text)
	if len(tokens) < 3 {
		return false
	}
	for _, t := range tokens {
		if !allCapsTokenRe.MatchString(t) {
			return false
		}
	}
	return true
}

// isReminderLine checks a raw file line (after stripping trailing inline comment)
// for the reminder pattern.
func isReminderLine(line string) bool {
	clean := stripNodeComment(strings.TrimSpace(line))
	return isReminder(clean)
}

// isAllUpper checks that all rune characters are uppercase or non-letter.
// Used by callers outside this file.
func isAllUpper(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// Ensure both helpers are reachable from outside without lint noise.
var (
	_ = isAllUpper
	_ = isReminderLine
)
