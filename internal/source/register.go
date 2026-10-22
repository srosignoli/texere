package source

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"texere/internal/content"
	"texere/internal/learning"
)

// Import is what differs between the ways a Source comes into a Collection:
// a file the user picked, or content Texere generated (a fetched article, a
// model-written document).
type Import struct {
	Medium    string // "text" | "pdf" | "web" | "llm"
	Title     string // "" = the placed file's name without its extension
	FileName  string // the name to start from in sources/
	NotesName string // "" = the placed file's name without its extension, plus .n4l
	Data      []byte // content to write, or…
	CopyFrom  string // …a file to copy in
	SourceURL string
}

// Register brings a Source into the Collection at collectionDir: its file in
// sources/, its notes in reading/, and its row in st. Names are made unique
// with a -2, -3, … suffix. It returns the stored row, NotesPath absolute.
func Register(collectionDir string, st *learning.Store, im Import) (learning.ReadingElement, error) {
	if im.CopyFrom == "" && len(im.Data) == 0 {
		return learning.ReadingElement{}, errors.New("nothing to import")
	}
	srcRel, srcAbs := uniqueDest(collectionDir, "sources", im.FileName)
	if im.CopyFrom != "" {
		if err := copyFile(im.CopyFrom, srcAbs); err != nil {
			os.Remove(srcAbs) // a copy that failed part-way
			return learning.ReadingElement{}, fmt.Errorf("copy: %w", err)
		}
	} else if err := os.WriteFile(srcAbs, im.Data, 0o644); err != nil {
		os.Remove(srcAbs)
		return learning.ReadingElement{}, fmt.Errorf("save: %w", err)
	}
	stem := strings.TrimSuffix(filepath.Base(srcRel), filepath.Ext(srcRel))
	title := im.Title
	if title == "" {
		title = stem
	}
	notesName := im.NotesName
	if notesName == "" {
		notesName = stem + ".n4l"
	}
	_, notesAbs := uniqueDest(collectionDir, "reading", notesName)
	id, err := content.CreateReadingNotes(notesAbs, srcRel, title)
	if err != nil {
		os.Remove(srcAbs)
		return learning.ReadingElement{}, fmt.Errorf("notes: %w", err)
	}
	r := learning.ReadingElement{ID: id, RefPath: srcRel, NotesPath: notesAbs, Title: title,
		Position: "char=0", IntervalDays: 1, Medium: im.Medium, SourceURL: im.SourceURL}
	if im.Medium == "pdf" {
		// A PDF opens at its first page; its page count is reported by the pane.
		r.Position = "page=1"
	} else {
		proj, _ := Projection(srcAbs)
		r.ProjLen = utf8.RuneCountInString(proj)
	}
	if err := st.UpsertReading(r); err != nil {
		// Both files go: a notes file left in reading/ is an ordinary note to
		// the graph and the linter.
		os.Remove(notesAbs)
		os.Remove(srcAbs)
		return learning.ReadingElement{}, fmt.Errorf("register: %w", err)
	}
	return r, nil
}

// uniqueDest returns (relPath, absPath) under <collection>/<subdir>/<name>,
// suffixing the stem with -2, -3, … until the path does not exist.
func uniqueDest(collectionDir, subdir, name string) (string, string) {
	// uniqueDest has nowhere to report a failure, so this stays best-effort:
	// if the directory cannot be created, the write that follows fails and
	// reports it.
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
