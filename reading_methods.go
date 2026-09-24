package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/collection"
	"texere/internal/content"
	"texere/internal/learning"
	"texere/internal/source"
)

// ReadingView is the frontend-facing shape of a reading element.
type ReadingView struct {
	ID        string `json:"id"`
	RefPath   string `json:"refPath"`
	NotesPath string `json:"notesPath"`
	Title     string `json:"title"`
	Position  string `json:"position"`
	Percent   int    `json:"percent"`
	Due       int64  `json:"due"`
	Done      bool   `json:"done"`
	Medium    string `json:"medium"`
	SourceURL string `json:"sourceUrl"`
}

// CreateExtractView is returned after appending an extract.
type CreateExtractView struct {
	ID string `json:"id"`
}

// upsertReadingRelative persists r with NotesPath rewritten to its
// collection-relative form (what the four stored-path columns must hold).
// r itself is left untouched — its absolute NotesPath is what the caller
// still needs to build the *ReadingView returned to the frontend.
func upsertReadingRelative(st *learning.Store, r learning.ReadingElement) error {
	rel, err := st.Rel(r.NotesPath)
	if err != nil {
		return fmt.Errorf("notes path: %w", err)
	}
	stored := r
	stored.NotesPath = rel
	return st.UpsertReading(stored)
}

func toReadingView(r learning.ReadingElement) ReadingView {
	pct := 0
	if r.ProjLen > 0 {
		if off := parsePositionValue(r.Position); off > 0 {
			pct = int(math.Round(float64(off) / float64(r.ProjLen) * 100))
			if pct > 100 {
				pct = 100
			}
		}
	}
	var due int64
	if !r.Due.IsZero() {
		due = r.Due.Unix()
	}
	return ReadingView{ID: r.ID, RefPath: r.RefPath, NotesPath: r.NotesPath, Title: r.Title,
		Position: r.Position, Percent: pct, Due: due, Done: r.Done,
		Medium: r.Medium, SourceURL: r.SourceURL}
}

// char=N anchors are 0-based Unicode code-point (rune) offsets into the source's text projection —
// the same unit the frontend computes from a DOM Range ([...str].length) and Go measures with
// utf8.RuneCountInString. Phase 2b relocation indexes the projection with []rune(proj)[:N].

// parsePositionValue pulls the integer N from a position/anchor string of the
// form "<unit>=N" (e.g. "char=4180" or "page=12"); 0 if absent/malformed.
func parsePositionValue(s string) int {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return 0
	}
	n := 0
	for _, c := range s[i+1:] {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ImportTextSource opens a file dialog, copies the chosen text/HTML file into
// sources/, scaffolds reading/<name>.n4l, and registers a reading element.
func (a *App) ImportTextSource() (*ReadingView, error) {
	if _, err := a.currentDir(); err != nil {
		return nil, err
	}
	picked, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import a text or HTML source",
		Filters: []runtime.FileFilter{{
			DisplayName: "Text / Markdown / HTML",
			Pattern:     "*.txt;*.md;*.html;*.htm",
		}},
	})
	if err != nil {
		return nil, err
	}
	if picked == "" {
		return nil, nil // cancelled
	}
	return a.importTextFrom(picked)
}

// importTextFrom is the ingest half of ImportTextSource: everything after a
// path has been chosen. Kept unexported so Wails generates no JS binding for
// it — the only exported way in stays the dialog. See demo_seam.go for the
// build-tagged exception used by scripted demos.
func (a *App) importTextFrom(picked string) (*ReadingView, error) {
	if picked == "" {
		return nil, errors.New("importTextFrom: empty path")
	}
	return inCollection(a, func(h collection.Handles) (*ReadingView, error) {
		base := filepath.Base(picked)
		srcRel, srcAbs := uniqueDest(h.Dir(), "sources", base)
		if err := copyFile(picked, srcAbs); err != nil {
			return nil, fmt.Errorf("ImportTextSource: copy: %w", err)
		}
		title := strings.TrimSuffix(filepath.Base(srcRel), filepath.Ext(srcRel))
		_, notesAbs := uniqueDest(h.Dir(), "reading", title+".n4l")
		refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
		if err != nil {
			os.Remove(srcAbs) // best-effort: don't litter sources/ on a failed import
			return nil, fmt.Errorf("ImportTextSource: notes: %w", err)
		}
		proj, _ := source.Projection(srcAbs)
		r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
			Position: "char=0", ProjLen: utf8.RuneCountInString(proj), IntervalDays: 1}
		if err := upsertReadingRelative(h.Learning(), r); err != nil {
			os.Remove(srcAbs) // best-effort: don't litter sources/ on a failed import
			return nil, fmt.Errorf("ImportTextSource: register: %w", err)
		}
		v := toReadingView(r)
		return &v, nil
	})
}

// uniqueDest returns (relPath, absPath) under <collection>/<subdir>/<name>,
// suffixing the stem with -2, -3, … until the path does not exist.
func uniqueDest(collectionDir, subdir, name string) (string, string) {
	// uniqueDest has nowhere to report a failure, so this stays best-effort:
	// if the directory cannot be created, the caller's copyFile fails next and
	// reports it as "<Import...>: copy: ...". Less precise than a dedicated
	// mkdir error, but never silent.
	_ = os.MkdirAll(filepath.Join(collectionDir, subdir), 0o755)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 2; ; i++ {
		abs := filepath.Join(collectionDir, subdir, candidate)
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return filepath.ToSlash(filepath.Join(subdir, candidate)), abs
		}
		candidate = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (a *App) GetReadingList() ([]ReadingView, error) {
	return inCollection(a, func(h collection.Handles) ([]ReadingView, error) {
		rs, err := h.Learning().ListReading()
		if err != nil {
			return nil, err
		}
		out := make([]ReadingView, 0, len(rs))
		for _, r := range rs {
			// r.NotesPath is stored collection-relative; the frontend passes
			// ReadingView.NotesPath straight to GetFileContent/SaveFile, which need
			// an absolute path. A row that cannot be resolved under the open
			// collection is dropped from the list rather than risk file I/O outside
			// it — the rest of the library must stay usable.
			abs, rerr := h.Learning().Resolve(r.NotesPath)
			if rerr != nil {
				log.Printf("GetReadingList: %q: %v", r.ID, rerr)
				continue
			}
			r.NotesPath = abs
			out = append(out, toReadingView(r))
		}
		return out, nil
	})
}

// GetSourceProjection returns the text projection of a source file resolved
// against the collection dir. refPath is refused if it escapes the collection.
func (a *App) GetSourceProjection(refPath string) (string, error) {
	return inCollection(a, func(h collection.Handles) (string, error) {
		p, err := h.Path(refPath)
		if err != nil {
			return "", err
		}
		return source.Projection(p)
	})
}

// GetSourceHTML returns the raw contents of a source file (e.g. an imported web
// article) resolved against the collection dir. refPath is stored relative to
// the collection, so — unlike GetFileContent, which takes absolute paths — it
// must be joined here rather than read CWD-relative. refPath is refused if it
// escapes the collection.
func (a *App) GetSourceHTML(refPath string) (string, error) {
	return inCollection(a, func(h collection.Handles) (string, error) {
		p, err := h.Path(refPath)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
}

// CreateLLMSource saves an LLM conversation as a markdown reading source.
//
// Medium is "llm" rather than "text" on purpose. ReadingView routes anything
// that is not "pdf" or "web" to the text pane, so the new medium costs no pane
// work, and it lets the sidebar mark the document as model-generated. That
// distinction matters more here than for other sources: a PDF's provenance is
// self-evident and a web article keeps its SourceURL, while a model-written
// study document can be confidently wrong and is about to become flashcards.
//
// ProjLen is the markdown's rune count, which is exact rather than an estimate:
// source.Projection returns raw file contents for every extension except
// .html/.htm, so a .md file's projection is its text.
func (a *App) CreateLLMSource(title, markdown string) (*ReadingView, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("CreateLLMSource: title is empty")
	}
	if strings.TrimSpace(markdown) == "" {
		return nil, errors.New("CreateLLMSource: nothing to save")
	}
	return inCollection(a, func(h collection.Handles) (*ReadingView, error) {
		slug := slugify(title)
		srcRel, srcAbs := uniqueDest(h.Dir(), "sources", slug+".md")
		if err := os.WriteFile(srcAbs, []byte(markdown), 0o644); err != nil {
			return nil, fmt.Errorf("CreateLLMSource: save: %w", err)
		}
		_, notesAbs := uniqueDest(h.Dir(), "reading", slug+".n4l")
		refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
		if err != nil {
			os.Remove(srcAbs) // best-effort: don't litter sources/ on a failed create
			return nil, fmt.Errorf("CreateLLMSource: notes: %w", err)
		}
		r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
			Position: "char=0", ProjLen: utf8.RuneCountInString(markdown), IntervalDays: 1, Medium: "llm"}
		if err := upsertReadingRelative(h.Learning(), r); err != nil {
			os.Remove(srcAbs)
			return nil, fmt.Errorf("CreateLLMSource: register: %w", err)
		}
		v := toReadingView(r)
		return &v, nil
	})
}

// CreateExtract appends a verbatim extract and signals the notes editor to
// reload (clean buffer only).
//
// advanceProgress moves # @progress and the DB position to where the extract
// was taken. The sidebar's extract peek passes false: a peek is a glance at one
// passage, not a reading position, and extracting during one would otherwise
// rewind a half-read document to wherever the peeked passage happens to be.
// This is the same safety property the three UpdateReadingProgress call sites
// in ReadingView.svelte guard, reached by a different route.
func (a *App) CreateExtract(id, text, position string, advanceProgress bool) (*CreateExtractView, error) {
	return inCollection(a, func(h collection.Handles) (*CreateExtractView, error) {
		r, ok, err := h.Learning().GetReading(id)
		if err != nil || !ok {
			return nil, fmt.Errorf("CreateExtract: reading element %q not found", id)
		}
		// r.NotesPath is stored collection-relative; resolve once and reuse the
		// absolute form for every file operation below.
		notesPath, err := h.Learning().Resolve(r.NotesPath)
		if err != nil {
			return nil, fmt.Errorf("CreateExtract: %w", err)
		}
		anchor := fmt.Sprintf("%s %s", r.RefPath, position)
		// The collection watcher is now recursive, so reading/ writes do fire events;
		// bracket them as self-writes to suppress a false external-mod toast if this
		// notes file also happens to be open in the main editor.
		var exID string
		err = h.SelfWrite(notesPath, func() error {
			var err error
			exID, err = content.AppendExtract(notesPath, text, anchor)
			if err == nil && advanceProgress {
				err = content.UpdateProgressMarker(notesPath, position)
			}
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("CreateExtract: %w", err)
		}
		if advanceProgress {
			_ = h.Learning().SetReadingProgress(id, position, r.ProjLen)
		}
		// Register the new extract in the queue immediately (reuses the classify pass).
		if _, err := learning.Sync(h.Learning(), []string{notesPath}, a.registry); err != nil {
			// Non-fatal: it will be picked up on the next Build Graph.
			log.Printf("CreateExtract: register sync: %v", err)
		}
		a.emit(collection.EventIDsInjected, map[string]string{"path": notesPath})
		return &CreateExtractView{ID: exID}, nil
	})
}

// ImportPdfSource copies a chosen PDF into sources/ and registers a reading element.
func (a *App) ImportPdfSource() (*ReadingView, error) {
	if _, err := a.currentDir(); err != nil {
		return nil, err
	}
	picked, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import a PDF source",
		Filters: []runtime.FileFilter{{DisplayName: "PDF", Pattern: "*.pdf"}},
	})
	if err != nil || picked == "" {
		return nil, err
	}
	return a.importPdfFrom(picked)
}

// importPdfFrom is the ingest half of ImportPdfSource: everything after a path
// has been chosen. Unexported, so Wails generates no JS binding for it.
func (a *App) importPdfFrom(picked string) (*ReadingView, error) {
	if picked == "" {
		return nil, errors.New("importPdfFrom: empty path")
	}
	return inCollection(a, func(h collection.Handles) (*ReadingView, error) {
		base := filepath.Base(picked)
		srcRel, srcAbs := uniqueDest(h.Dir(), "sources", base)
		if err := copyFile(picked, srcAbs); err != nil {
			return nil, fmt.Errorf("ImportPdfSource: copy: %w", err)
		}
		title := strings.TrimSuffix(filepath.Base(srcRel), filepath.Ext(srcRel))
		_, notesAbs := uniqueDest(h.Dir(), "reading", title+".n4l")
		refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
		if err != nil {
			os.Remove(srcAbs)
			return nil, fmt.Errorf("ImportPdfSource: notes: %w", err)
		}
		r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
			Position: "page=1", IntervalDays: 1, Medium: "pdf"}
		if err := upsertReadingRelative(h.Learning(), r); err != nil {
			os.Remove(srcAbs)
			return nil, fmt.Errorf("ImportPdfSource: register: %w", err)
		}
		v := toReadingView(r)
		return &v, nil
	})
}

// ImportWebSource fetches a URL, saves a sanitized self-contained article, and registers it.
func (a *App) ImportWebSource(srcURL string) (*ReadingView, error) {
	dir, err := a.currentDir()
	if err != nil {
		return nil, err
	}
	// The network fetch runs with no lock held: it must not block a rebuild or
	// a collection switch, and it must not hold the Session open while it waits
	// on a remote server. That means the collection open at the start of the
	// fetch may not be the one still open when it finishes; dir is compared
	// against h.Dir() below so the article is never saved into the wrong one.
	art, err := source.FetchAndExtract(srcURL)
	if err != nil {
		return nil, fmt.Errorf("ImportWebSource: %w", err)
	}
	title := art.Title
	if title == "" {
		title = "web article"
	}
	slug := slugify(title)
	return inCollection(a, func(h collection.Handles) (*ReadingView, error) {
		if h.Dir() != dir {
			return nil, fmt.Errorf("ImportWebSource: collection changed during import (was %s, now %s)", dir, h.Dir())
		}
		srcRel, srcAbs := uniqueDest(h.Dir(), "sources", slug+".html")
		if err := os.WriteFile(srcAbs, []byte(art.HTML), 0o644); err != nil {
			os.Remove(srcAbs)
			return nil, fmt.Errorf("ImportWebSource: save: %w", err)
		}
		_, notesAbs := uniqueDest(h.Dir(), "reading", slug+".n4l")
		refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
		if err != nil {
			os.Remove(srcAbs)
			return nil, fmt.Errorf("ImportWebSource: notes: %w", err)
		}
		// article text length for % progress — derived from the in-memory HTML we just
		// saved, avoiding a redundant read+tokenize of srcAbs.
		proj := source.TextFromHTML(art.HTML)
		r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
			Position: "char=0", ProjLen: utf8.RuneCountInString(proj), IntervalDays: 1, Medium: "web", SourceURL: art.URL}
		if err := upsertReadingRelative(h.Learning(), r); err != nil {
			os.Remove(srcAbs)
			return nil, fmt.Errorf("ImportWebSource: register: %w", err)
		}
		v := toReadingView(r)
		return &v, nil
	})
}

// slugify makes a filesystem-safe lowercase slug from a title.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "article"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

// GetSourceBytes returns the base64 of a source file (for pdf.js). refPath is
// refused if it escapes the collection.
func (a *App) GetSourceBytes(refPath string) (string, error) {
	return inCollection(a, func(h collection.Handles) (string, error) {
		p, err := h.Path(refPath)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(data), nil
	})
}

// OpenSourceURL opens the reading element's original URL in the system browser.
func (a *App) OpenSourceURL(id string) error {
	url, err := inCollection(a, func(h collection.Handles) (string, error) {
		r, ok, err := h.Learning().GetReading(id)
		if err != nil || !ok {
			return "", fmt.Errorf("OpenSourceURL: %q not found", id)
		}
		if r.SourceURL == "" {
			return "", errors.New("no original URL")
		}
		return r.SourceURL, nil
	})
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}

// SetReadingProjLen stores a reader-reported total length / page count.
func (a *App) SetReadingProjLen(id string, n int) error {
	return a.withCollection(func(h collection.Handles) error {
		r, ok, err := h.Learning().GetReading(id)
		if err != nil || !ok {
			return fmt.Errorf("SetReadingProjLen: %q not found", id)
		}
		return h.Learning().SetReadingProgress(id, r.Position, n)
	})
}

func (a *App) UpdateReadingProgress(id, position string) error {
	return a.withCollection(func(h collection.Handles) error {
		r, ok, err := h.Learning().GetReading(id)
		if err != nil || !ok {
			return fmt.Errorf("UpdateReadingProgress: %q not found", id)
		}
		// r.NotesPath is stored collection-relative; this is the exact incident
		// site — an unresolved stored path here silently rewrites the # @progress
		// marker of whatever file the (possibly stale) value points to.
		notesPath, err := h.Learning().Resolve(r.NotesPath)
		if err != nil {
			return fmt.Errorf("UpdateReadingProgress: %w", err)
		}
		wErr := h.SelfWrite(notesPath, func() error {
			return content.UpdateProgressMarker(notesPath, position)
		})
		if wErr != nil {
			return wErr
		}
		return h.Learning().SetReadingProgress(id, position, r.ProjLen)
	})
}

// ReadingExtractView is one row in a document's extract tree.
//
// Anchor is the extract's from_anchor VERBATIM — "sources/x.txt char=1173" or
// "sources/p.pdf page=12". It is not parsed into an offset here: the unit
// depends on the medium and only the source pane knows what to do with it.
type ReadingExtractView struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Anchor   string `json:"anchor"`
	Promoted bool   `json:"promoted"`
}

// GetReadingExtracts returns every reading's extracts, keyed by reading id and
// ordered by position within the document.
//
// Read from the .n4l files, NOT from the extracts table. That table is the
// processing QUEUE: GenerateExtracts skips study-marked nodes and Sync retires
// anything that stops qualifying, so promoting an extract to a card drops it
// out. The tree means something different — everything you pulled out of this
// document, promoted or not, because its job is finding your way back to the
// passage. Parsing also keeps the tree from going stale between syncs.
//
// One call for the whole library rather than a lookup per document: the sidebar
// searches extract text across documents whose trees are still collapsed, so it
// needs them all before any one is expanded.
func (a *App) GetReadingExtracts() (map[string][]ReadingExtractView, error) {
	return inCollection(a, func(h collection.Handles) (map[string][]ReadingExtractView, error) {
		rs, err := h.Learning().ListReading()
		if err != nil {
			return nil, err
		}
		// Promotions made before card provenance was preserved dropped the (from)
		// line from the file, leaving the DB row as the only record of where that
		// passage came from. Fall back to it rather than hide the card.
		legacyAnchors, err := h.Learning().ExtractAnchors()
		if err != nil {
			return nil, err
		}

		// ParseFile dereferences the registry unconditionally; an empty one still
		// yields the arrows, flagged unregistered, which is all this needs.
		reg := a.registry
		if reg == nil {
			reg = &content.SSTRegistry{Arrows: map[string]content.ArrowDef{}}
		}

		out := make(map[string][]ReadingExtractView, len(rs))
		for _, r := range rs {
			// r.NotesPath is stored collection-relative; a row that cannot be
			// resolved under the open collection is skipped the same way a parse
			// error is — one bad file must not empty the whole library's trees.
			notesPath, rerr := h.Learning().Resolve(r.NotesPath)
			if rerr != nil {
				log.Printf("GetReadingExtracts: %q: %v", r.ID, rerr)
				continue
			}
			// Parse errors are not fatal here: a notes file with an unknown arrow
			// still yields the nodes that did parse, and one bad file must not empty
			// the whole library's trees.
			pf, _ := content.ParseFile(notesPath, reg)
			if pf == nil {
				continue
			}
			var list []ReadingExtractView
			for _, ch := range pf.Chapters {
				for _, n := range ch.Nodes {
					if n.ID == "" {
						continue
					}
					anchor, ok := nodeFromAnchor(n)
					if !ok {
						// No anchor in the file and none on record means this is an
						// ordinary note, or the document's own node — not an extract.
						if anchor, ok = legacyAnchors[n.ID]; !ok {
							continue
						}
					}
					list = append(list, ReadingExtractView{
						ID: n.ID, Text: n.Text, Anchor: anchor, Promoted: n.StudyMeta != nil,
					})
				}
			}
			// Document order. parsePositionValue pulls N out of "<unit>=N", so this
			// sorts numerically — sorting the anchor strings would put char=100
			// before char=9 and the tree would read out of order.
			sort.SliceStable(list, func(i, j int) bool {
				return parsePositionValue(list[i].Anchor) < parsePositionValue(list[j].Anchor)
			})
			if len(list) > 0 {
				out[r.ID] = list
			}
		}
		return out, nil
	})
}

// nodeFromAnchor returns a node's (from) target, checking ditto continuation
// lines as well as the node line: promoting an extract moves its anchor onto a
// ditto so it stays out of the card's front/back split.
func nodeFromAnchor(n content.Node) (string, bool) {
	for _, ar := range n.Arrows {
		if ar.Code == "from" {
			return ar.Target, true
		}
	}
	for _, d := range n.Dittos {
		for _, ar := range d.Arrows {
			if ar.Code == "from" {
				return ar.Target, true
			}
		}
	}
	return "", false
}

func (a *App) RescheduleReading(id, action string) (*ReadingView, error) {
	return inCollection(a, func(h collection.Handles) (*ReadingView, error) {
		r, ok, err := h.Learning().GetReading(id)
		if err != nil || !ok {
			return nil, fmt.Errorf("RescheduleReading: %q not found", id)
		}
		sched := learning.ScheduleReading(r, action, time.Now())
		if err := h.Learning().ApplyReadingSchedule(id, sched); err != nil {
			return nil, err
		}
		// sched.NotesPath is carried through unchanged from r (ApplyReadingSchedule
		// does not touch notes_path), so it is still the stored, collection-relative
		// value; resolve it before handing the view back to the frontend.
		notesPath, err := h.Learning().Resolve(sched.NotesPath)
		if err != nil {
			return nil, fmt.Errorf("RescheduleReading: %w", err)
		}
		sched.NotesPath = notesPath
		v := toReadingView(sched)
		return &v, nil
	})
}
