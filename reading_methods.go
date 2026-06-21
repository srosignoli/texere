package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
}

// ExtractView is returned after appending an extract.
type ExtractView struct {
	ID string `json:"id"`
}

func toReadingView(r learning.ReadingElement) ReadingView {
	pct := 0
	if r.ProjLen > 0 {
		if off := parseCharAnchor(r.Position); off > 0 {
			pct = int(float64(off) / float64(r.ProjLen) * 100)
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
		Position: r.Position, Percent: pct, Due: due, Done: r.Done}
}

// parseCharAnchor pulls N from "char=N" (0 if absent/malformed).
func parseCharAnchor(s string) int {
	i := strings.Index(s, "char=")
	if i < 0 {
		return 0
	}
	n := 0
	for _, c := range s[i+5:] {
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
	notesRel, notesAbs := uniqueDest(a.collectionDir, "reading", title+".n4l")
	_ = notesRel
	refID, err := content.CreateReadingNotes(notesAbs, srcRel, title)
	if err != nil {
		return nil, fmt.Errorf("ImportTextSource: notes: %w", err)
	}
	proj, _ := source.Projection(srcAbs)
	r := learning.ReadingElement{ID: refID, RefPath: srcRel, NotesPath: notesAbs, Title: title,
		Position: "char=0", ProjLen: len(proj), IntervalDays: 1}
	if err := a.learnStore.UpsertReading(r); err != nil {
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
	if a.collectionDir == "" {
		return "", errors.New("no collection open")
	}
	return source.Projection(filepath.Join(a.collectionDir, filepath.FromSlash(refPath)))
}

// CreateExtract appends a verbatim extract, advances # @progress + DB position,
// and signals the notes editor to reload (clean buffer only).
func (a *App) CreateExtract(id, text string, anchorChar int) (*ExtractView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	r, ok, err := a.learnStore.GetReading(id)
	if err != nil || !ok {
		return nil, fmt.Errorf("CreateExtract: reading element %q not found", id)
	}
	anchor := fmt.Sprintf("%s char=%d", r.RefPath, anchorChar)
	a.savingPath = r.NotesPath
	exID, err := content.AppendExtract(r.NotesPath, text, anchor)
	if err == nil {
		err = content.UpdateProgressMarker(r.NotesPath, fmt.Sprintf("char=%d", anchorChar))
	}
	a.savingPath = ""
	if err != nil {
		return nil, fmt.Errorf("CreateExtract: %w", err)
	}
	_ = a.learnStore.SetReadingProgress(id, fmt.Sprintf("char=%d", anchorChar), r.ProjLen)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "file:ids-injected", map[string]string{"path": r.NotesPath})
	}
	return &ExtractView{ID: exID}, nil
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
