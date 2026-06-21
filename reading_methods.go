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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"

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
	if a.learnStore == nil || a.collectionDir == "" {
		return nil, errors.New("no collection open")
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
	base := filepath.Base(picked)
	srcRel, srcAbs := uniqueDest(a.collectionDir, "sources", base)
	if err := copyFile(picked, srcAbs); err != nil {
		return nil, fmt.Errorf("ImportTextSource: copy: %w", err)
	}
	title := strings.TrimSuffix(filepath.Base(srcRel), filepath.Ext(srcRel))
	_, notesAbs := uniqueDest(a.collectionDir, "reading", title+".n4l")
	refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
	if err != nil {
		os.Remove(srcAbs) // best-effort: don't litter sources/ on a failed import
		return nil, fmt.Errorf("ImportTextSource: notes: %w", err)
	}
	proj, _ := source.Projection(srcAbs)
	r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
		Position: "char=0", ProjLen: utf8.RuneCountInString(proj), IntervalDays: 1}
	if err := a.learnStore.UpsertReading(r); err != nil {
		os.Remove(srcAbs) // best-effort: don't litter sources/ on a failed import
		return nil, fmt.Errorf("ImportTextSource: register: %w", err)
	}
	v := toReadingView(r)
	return &v, nil
}

// uniqueDest returns (relPath, absPath) under <collection>/<subdir>/<name>,
// suffixing the stem with -2, -3, … until the path does not exist.
func uniqueDest(collectionDir, subdir, name string) (string, string) {
	os.MkdirAll(filepath.Join(collectionDir, subdir), 0o755)
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
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	rs, err := a.learnStore.ListReading()
	if err != nil {
		return nil, err
	}
	out := make([]ReadingView, 0, len(rs))
	for _, r := range rs {
		out = append(out, toReadingView(r))
	}
	return out, nil
}

func (a *App) GetSourceProjection(refPath string) (string, error) {
	if a.learnStore == nil || a.collectionDir == "" {
		return "", errors.New("no collection open")
	}
	return source.Projection(filepath.Join(a.collectionDir, filepath.FromSlash(refPath)))
}

// CreateExtract appends a verbatim extract, advances # @progress + DB position,
// and signals the notes editor to reload (clean buffer only).
func (a *App) CreateExtract(id, text, position string) (*CreateExtractView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return nil, fmt.Errorf("CreateExtract: reading element %q not found", id)
	}
	anchor := fmt.Sprintf("%s %s", r.RefPath, position)
	// reading/ is an unwatched subtree (the collection watcher is non-recursive), so this
	// savingPath bracketing is defensive only — no external-mod event fires for notes files.
	// ReadingView refreshes its own buffer via reloadNotes().
	a.savingPath = r.NotesPath
	exID, err := content.AppendExtract(r.NotesPath, text, anchor)
	if err == nil {
		err = content.UpdateProgressMarker(r.NotesPath, position)
	}
	a.savingPath = ""
	if err != nil {
		return nil, fmt.Errorf("CreateExtract: %w", err)
	}
	_ = a.learnStore.SetReadingProgress(id, position, r.ProjLen)
	// Register the new extract in the queue immediately (reuses the classify pass).
	if _, err := learning.Sync(a.learnStore, []string{r.NotesPath}, a.registry); err != nil {
		// Non-fatal: it will be picked up on the next Build Graph.
		log.Printf("CreateExtract: register sync: %v", err)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "file:ids-injected", map[string]string{"path": r.NotesPath})
	}
	return &CreateExtractView{ID: exID}, nil
}

// ImportPdfSource copies a chosen PDF into sources/ and registers a reading element.
func (a *App) ImportPdfSource() (*ReadingView, error) {
	if a.learnStore == nil || a.collectionDir == "" {
		return nil, errors.New("no collection open")
	}
	picked, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import a PDF source",
		Filters: []runtime.FileFilter{{DisplayName: "PDF", Pattern: "*.pdf"}},
	})
	if err != nil || picked == "" {
		return nil, err
	}
	base := filepath.Base(picked)
	srcRel, srcAbs := uniqueDest(a.collectionDir, "sources", base)
	if err := copyFile(picked, srcAbs); err != nil {
		return nil, fmt.Errorf("ImportPdfSource: copy: %w", err)
	}
	title := strings.TrimSuffix(filepath.Base(srcRel), filepath.Ext(srcRel))
	_, notesAbs := uniqueDest(a.collectionDir, "reading", title+".n4l")
	refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
	if err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("ImportPdfSource: notes: %w", err)
	}
	r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
		Position: "page=1", IntervalDays: 1, Medium: "pdf"}
	if err := a.learnStore.UpsertReading(r); err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("ImportPdfSource: register: %w", err)
	}
	v := toReadingView(r)
	return &v, nil
}

// ImportWebSource fetches a URL, saves a sanitized self-contained article, and registers it.
func (a *App) ImportWebSource(srcURL string) (*ReadingView, error) {
	if a.learnStore == nil || a.collectionDir == "" {
		return nil, errors.New("no collection open")
	}
	art, err := source.FetchAndExtract(srcURL)
	if err != nil {
		return nil, fmt.Errorf("ImportWebSource: %w", err)
	}
	title := art.Title
	if title == "" {
		title = "web article"
	}
	slug := slugify(title)
	srcRel, srcAbs := uniqueDest(a.collectionDir, "sources", slug+".html")
	if err := os.WriteFile(srcAbs, []byte(art.HTML), 0o644); err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("ImportWebSource: save: %w", err)
	}
	_, notesAbs := uniqueDest(a.collectionDir, "reading", slug+".n4l")
	refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
	if err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("ImportWebSource: notes: %w", err)
	}
	proj, _ := source.Projection(srcAbs) // article text length for % progress
	r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
		Position: "char=0", ProjLen: utf8.RuneCountInString(proj), IntervalDays: 1, Medium: "web", SourceURL: art.URL}
	if err := a.learnStore.UpsertReading(r); err != nil {
		os.Remove(srcAbs)
		return nil, fmt.Errorf("ImportWebSource: register: %w", err)
	}
	v := toReadingView(r)
	return &v, nil
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

// GetSourceBytes returns the base64 of a source file (for pdf.js).
func (a *App) GetSourceBytes(refPath string) (string, error) {
	if a.collectionDir == "" {
		return "", errors.New("no collection open")
	}
	data, err := os.ReadFile(filepath.Join(a.collectionDir, filepath.FromSlash(refPath)))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// OpenSourceURL opens the reading element's original URL in the system browser.
func (a *App) OpenSourceURL(id string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return fmt.Errorf("OpenSourceURL: %q not found", id)
	}
	if r.SourceURL == "" {
		return errors.New("no original URL")
	}
	runtime.BrowserOpenURL(a.ctx, r.SourceURL)
	return nil
}

// SetReadingProjLen stores a reader-reported total length / page count.
func (a *App) SetReadingProjLen(id string, n int) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return fmt.Errorf("SetReadingProjLen: %q not found", id)
	}
	return a.learnStore.SetReadingProgress(id, r.Position, n)
}

func (a *App) UpdateReadingProgress(id, position string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return fmt.Errorf("UpdateReadingProgress: %q not found", id)
	}
	a.savingPath = r.NotesPath
	wErr := content.UpdateProgressMarker(r.NotesPath, position)
	a.savingPath = ""
	if wErr != nil {
		return wErr
	}
	return a.learnStore.SetReadingProgress(id, position, r.ProjLen)
}

func (a *App) RescheduleReading(id, action string) (*ReadingView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return nil, fmt.Errorf("RescheduleReading: %q not found", id)
	}
	sched := learning.ScheduleReading(r, action, time.Now())
	if err := a.learnStore.ApplyReadingSchedule(id, sched); err != nil {
		return nil, err
	}
	v := toReadingView(sched)
	return &v, nil
}
