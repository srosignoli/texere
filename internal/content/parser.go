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
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		processLine(pf, state, path, line, lineNum, registry)
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
	kind          BlockType
	forceReverse  bool
}

type pendingRef struct {
	refStr     string  // e.g., "$anchor1.1" or "$PREV.3"
	targetNode *Node   // the node that contains the reference token in its Text
	chapter    *Chapter
}

type parserState struct {
	currentChapter  *Chapter
	effectiveCtx    map[string]struct{}
	dittoParent     *Node
	studyBlock      studyBlockState
	pendingStudy    *StudyMeta
	pendingIDMarker string // shortid from "# @id <shortid>"
	pendingIDLine   int    // line number of the "# @id" marker
	pendingPriority *int
	pendingProv     Provenance
	labelMap        map[string]*Node
	pendingRefs     []pendingRef
}

// ---------------------------------------------------------------------------
// Line dispatching
// ---------------------------------------------------------------------------

func processLine(pf *ParsedFile, state *parserState, path, line string, lineNum int, reg *SSTRegistry) {
	// Chapter title line
	if strings.HasPrefix(line, "- ") {
		openChapter(pf, state, line, lineNum)
		return
	}

	// Context operators (must check before node lines)
	if strings.HasPrefix(line, "::") || strings.HasPrefix(line, "+::") || strings.HasPrefix(line, "-::") {
		parseContextOp(state, line, lineNum)
		return
	}

	// Marker lines (# @...)
	if strings.HasPrefix(line, "# @") || strings.HasPrefix(line, "#@") {
		parseMarker(pf, state, path, line, lineNum)
		return
	}

	// Ditto lines: start with `"  ` (double-quote followed by at least one space)
	if strings.HasPrefix(line, `"`) {
		parseDitto(pf, state, path, line, lineNum, reg)
		return
	}

	// Blank or pure comment lines — reset some pending state
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return
	}

	// Node line
	if state.currentChapter != nil {
		parseNode(pf, state, path, line, lineNum, reg)
	}
	// Lines before the first chapter are ignored
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
	title := strings.TrimPrefix(line, "- ")
	state.currentChapter = &Chapter{
		Title:     title,
		LineStart: lineNum,
		Labels:    make(map[string]*Node),
	}
	state.effectiveCtx = make(map[string]struct{})
	state.dittoParent = nil
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

// parseTags splits a context tag string on commas and spaces, strips trailing
// punctuation like ":" or "::", and returns non-empty tags.
func parseTags(s string) []string {
	// Strip trailing :: or :
	s = strings.TrimRight(s, ": ")
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
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

	// Extract arrows from node text
	cleanText, arrows := extractArrows(nodeText, lineNum, false, reg, path, pf)

	// Build study meta from pending or block
	var sm *StudyMeta
	if state.pendingStudy != nil {
		sm = state.pendingStudy
		state.pendingStudy = nil
	} else if state.studyBlock.kind != BlockTypeNone {
		sm = studyMetaFromBlock(state.studyBlock)
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

	// Collect reminders
	if isReminder(cleanText) {
		pf.Reminders = append(pf.Reminders, Reminder{
			File: path,
			Line: lineNum,
			Text: cleanText,
		})
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
	if state.dittoParent == nil {
		pf.Errors = append(pf.Errors, ParseError{File: path, Line: lineNum, Msg: ErrOrphanDitto})
		return
	}

	// Ditto lines: `"  (arrow) target`
	// Strip the leading `" ` prefix
	content := strings.TrimPrefix(line, `"`)
	content = strings.TrimSpace(content)

	_, arrows := extractArrows(content, lineNum, true, reg, path, pf)

	ditto := DittoLine{
		Line:   lineNum,
		Arrows: arrows,
	}

	// Find the actual node in the chapter slice and append the ditto
	ch := state.currentChapter
	if ch != nil && len(ch.Nodes) > 0 {
		ch.Nodes[len(ch.Nodes)-1].Dittos = append(ch.Nodes[len(ch.Nodes)-1].Dittos, ditto)
	}
}

// ---------------------------------------------------------------------------
// Arrow extraction (T019)
// ---------------------------------------------------------------------------

// arrowRe matches "(code) target" patterns.
var arrowRe = regexp.MustCompile(`\(([^)]+)\)\s*(.*)`)

// extractArrows parses arrow patterns from a line of text.
// Returns the text with arrow tokens removed (for display) and the parsed Arrow slice.
func extractArrows(text string, lineNum int, isDitto bool, reg *SSTRegistry, path string, pf *ParsedFile) (string, []Arrow) {
	m := arrowRe.FindStringSubmatch(text)
	if m == nil {
		return text, nil
	}

	code := strings.TrimSpace(m[1])
	target := strings.TrimSpace(m[2])

	_, registered := reg.Arrows[code]
	if !registered {
		pf.Errors = append(pf.Errors, ParseError{File: path, Line: lineNum, Msg: fmt.Sprintf("%s: %q", ErrUnknownArrow, code)})
	}

	arrow := Arrow{
		Code:         code,
		Target:       target,
		IsRegistered: registered,
		Line:         lineNum,
		IsDitto:      isDitto,
	}

	// The "display text" for a node is the part before the arrow pattern
	displayText := strings.TrimSpace(arrowRe.ReplaceAllString(text, ""))
	return displayText, []Arrow{arrow}
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

// extractClozeTargets finds all {braces} content in text.
func extractClozeTargets(text string) []string {
	var targets []string
	start := -1
	for i, ch := range text {
		if ch == '{' {
			start = i
		} else if ch == '}' && start >= 0 {
			targets = append(targets, text[start+1:i])
			start = -1
		}
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
