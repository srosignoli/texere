package content

// ParsedFile is the complete result of a full parse of one .n4l file.
type ParsedFile struct {
	Path      string       // Absolute path to the .n4l file
	Chapters  []Chapter    // In file order
	Reminders []Reminder   // All reminders across all chapters in this file
	Errors    []ParseError // Per-line errors; non-nil does not mean parse failed
}

// Chapter is a named section within a file. Begins at a "- title" line and ends
// at the next "- title" line or EOF.
type Chapter struct {
	Title     string           // Text after "- " on the chapter line
	LineStart int              // 1-indexed line number of the "- title" line
	Nodes     []Node           // In file order, root nodes only
	Labels    map[string]*Node // @label → node pointer (resolved within this chapter)
}

// Node is a root-level knowledge statement. Ditto continuations are embedded in
// Dittos, not in Chapter.Nodes.
type Node struct {
	ID           string      // Stable identifier; "" if node has no # @id marker
	Text         string      // Raw node text (leading whitespace stripped, trailing comment excluded)
	Line         int         // 1-indexed line number of this node line
	EffectiveCtx []string    // Snapshot of context tags active at this node (sorted, deterministic)
	IsSequence   bool        // true if _sequence_ was in EffectiveCtx at this line
	Arrows       []Arrow     // Outgoing arrows declared on this node line
	Dittos       []DittoLine // Continuation lines (in file order)
	StudyMeta    *StudyMeta  // nil if not studyable
	Priority     *int        // nil if no @priority marker; value 0–100
	Provenance   *Provenance // nil if no @source / @ref / @progress markers
	Label        string      // The @label value if this node was labeled; "" otherwise
	IDMarkerLine int         // 1-indexed line number of the "# @id" line above this node; 0 if absent
}

// Arrow is a typed relationship declared on a node or ditto line.
type Arrow struct {
	Code         string // Short code, e.g., "means", "e.g.", "note"
	Target       string // Target string (raw text after the code)
	IsRegistered bool   // false if code not found in SSTRegistry
	Line         int    // 1-indexed line number
	IsDitto      bool   // true if this arrow belongs to a DittoLine, not the root Node
}

// DittoLine is a continuation of the immediately preceding Node.
type DittoLine struct {
	Line   int     // 1-indexed
	Text   string  // Display text of the ditto line (empty when the line is purely an arrow)
	Arrows []Arrow // All arrows declared on this ditto line
}

// CardType enumerates the study card types a node can produce.
type CardType int

const (
	CardTypeBasic           CardType = iota
	CardTypeReverse                  // forced reverse card
	CardTypeCloze                    // fill-in-the-blank
	CardTypeSequenceForward          // recall next item in sequence
	CardTypeSequenceGap              // recall missing item
	CardTypeSequenceOrder            // reconstruct order
)

// BlockType enumerates the study block kinds.
type BlockType int

const (
	BlockTypeNone          BlockType = iota
	BlockTypeStudy                   // # @study
	BlockTypeStudyReverse            // # @study reverse
	BlockTypeStudySequence           // # @study sequence
)

// StudyMeta holds study scheduling metadata extracted from markers.
type StudyMeta struct {
	CardType        CardType  // Derived card type
	ForceReverse    bool      // # @card reverse
	SuppressReverse bool      // # @card no-reverse
	ClozeTargets    []string  // Text inside {{braces}} for cloze cards
	FromBlock       bool      // true if inherited from a @study block
	BlockType       BlockType // Block kind (None if individual marker)
}

// Provenance holds source attribution metadata.
type Provenance struct {
	Source   string // # @source value
	Ref      string // # @ref value
	Progress string // # @progress value
}

// Reminder represents an ALL CAPS line (≥ 3 words) that is an open knowledge-debt item.
type Reminder struct {
	File string // Absolute path to source file
	Line int    // 1-indexed line number
	Text string // Full line text (comment stripped)
}

// ChapterSummary is the lightweight result of the Phase 1 scan (title extraction only).
type ChapterSummary struct {
	FilePath  string   // Absolute path
	FileName  string   // Base name (for display)
	RelPath   string   // Collection-relative, slash-normalized path (e.g. "reading/Foo.n4l")
	Chapters  []string // Chapter titles in file order; empty if no "- title" found
	ScanError error    // Non-nil if file could not be read
}

// SSTRegistry is loaded from the SSTconfig directory at startup.
type SSTRegistry struct {
	Arrows      map[string]ArrowDef // Short code → definition
	Annotations map[string]AnnotDef // Symbol (">>", "%>", "=", "***", "**") → definition
	LoadErrors  []error             // Non-fatal load errors
}

// ArrowDef describes one arrow type from the SSTconfig.
type ArrowDef struct {
	Code          string // Short code, e.g., "means"
	LongName      string // Human name, e.g., "means / is meaning of"
	Reverse       string // Reverse code; "" if one-way
	MetaType      string // SST meta-type (e.g., "contains", "properties")
	Context       string // SSTconfig section header (":: label ::") the pair was declared under; "" if none
	ClozeEligible bool
	// Unknown marks an arrow that was NOT declared in any SSTconfig but was
	// encountered in the source notes. Such arrows are synthesized into the
	// registry (via AugmentWithUnknownArrows) so their links still materialize
	// in the graph — rendered with a distinct "unknown" style — instead of
	// being silently dropped. The authored code doubles as the LongName.
	Unknown bool
}

// AnnotDef maps an annotation symbol to an arrow code.
type AnnotDef struct {
	Symbol    string // e.g., ">>"
	ArrowCode string // Maps to an ArrowDef code
}

// EventKind enumerates the kinds of collection filesystem events.
type EventKind int

const (
	EventAdded EventKind = iota
	EventRemoved
	EventRenamed
	EventModified
)

// CollectionEvent is emitted by CollectionWatcher when a .n4l file changes.
type CollectionEvent struct {
	Kind     EventKind        // Added | Removed | Renamed | Modified
	Path     string           // Absolute path of affected file
	Chapters []ChapterSummary // Populated for Added/Renamed; nil for Removed/Modified
}

// CollectionWatcher watches a directory for .n4l file changes.
// Read from Events and Errors; call Close when done.
type CollectionWatcher struct {
	Events <-chan CollectionEvent
	Errors <-chan error
	close  func() error
}

// Close stops the watcher and closes both channels.
func (w *CollectionWatcher) Close() error {
	if w.close != nil {
		return w.close()
	}
	return nil
}

// IDInjection describes one "# @id" line inserted during a write-back operation.
type IDInjection struct {
	FilePath   string // Absolute path of the modified file
	NodeLine   int    // 1-indexed line number of the node that received an ID
	InsertedAt int    // 1-indexed line number where "# @id <shortid>" was inserted
	ShortID    string // The generated 8-char base-36 ID
	AlreadyHad bool   // true if the node already had a # @id (no write occurred)
}

// WriteBackResult is returned by all write-back operations.
type WriteBackResult struct {
	FilePath     string // Absolute path of the modified file
	LinesChanged []int  // 1-indexed line numbers that were modified or inserted
	Err          error  // Non-nil if the operation failed; file is untouched on error
}
