package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/content"
	"texere/internal/learning"
	"texere/internal/source"
)

// NotebookView is the frontend payload for a processing notebook: persisted
// user cells plus a freshly-computed (never persisted) context payload.
type NotebookView struct {
	NotebookID string          `json:"notebookId"`
	NodeID     string          `json:"nodeId"`
	Chapter    string          `json:"chapter"`
	Cells      json.RawMessage `json:"cells"`
	Context    ContextPayload  `json:"context"`
}

// ContextPayload is the auto-generated pinned context cell (read-only).
type ContextPayload struct {
	ExtractText string      `json:"extractText"`
	Tags        []string    `json:"tags"`
	Orbit       []OrbitItem `json:"orbit"`
	Source      SourceInfo  `json:"source"`
}

type OrbitItem struct {
	ArrowLong    string `json:"arrowLong"`
	NeighborText string `json:"neighborText"`
}

type SourceInfo struct {
	Kind    string `json:"kind"`    // "text" | "web" | "pdf" | "none"
	Anchor  string `json:"anchor"`  // the (from) anchor
	Passage string `json:"passage"` // empty for pdf/none
	RefPath string `json:"refPath"` // collection-relative source path
}

func newNotebookID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "nb-" + hex.EncodeToString(b), nil
}

// OpenNotebook loads (or creates) the notebook for an extract node and returns
// it with a freshly-assembled context payload.
func (a *App) OpenNotebook(nodeID string) (*NotebookView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	nb, ok, err := a.learnStore.GetNotebookByExtract(nodeID)
	if err != nil {
		return nil, err
	}
	if !ok {
		id, err := newNotebookID()
		if err != nil {
			return nil, err
		}
		now := time.Now()
		nb = learning.Notebook{
			NotebookID: id, ExtractNodeID: nodeID,
			Chapter: a.chapterOfNode(nodeID), ContextTags: "[]", CellsJSON: "[]",
			CreatedAt: now, UpdatedAt: now,
		}
		if err := a.learnStore.UpsertNotebook(nb); err != nil {
			return nil, err
		}
	}
	return &NotebookView{
		NotebookID: nb.NotebookID, NodeID: nodeID, Chapter: nb.Chapter,
		Cells: json.RawMessage(nb.CellsJSON), Context: a.assembleContext(nodeID),
	}, nil
}

// SaveNotebook persists just the user cells (debounced autosave from the pane).
func (a *App) SaveNotebook(notebookID, cellsJSON string) error {
	if a.learnStore == nil {
		return errors.New("no collection open")
	}
	return a.learnStore.UpdateNotebookCells(notebookID, cellsJSON, time.Now())
}

// GetNotebook returns a notebook by ID for read-only reopen; its context is
// recomputed from the notebook's extract node.
func (a *App) GetNotebook(notebookID string) (*NotebookView, error) {
	if a.learnStore == nil {
		return nil, errors.New("no collection open")
	}
	nb, ok, err := a.learnStore.GetNotebook(notebookID)
	if err != nil || !ok {
		return nil, errors.New("notebook not found")
	}
	return &NotebookView{
		NotebookID: nb.NotebookID, NodeID: nb.ExtractNodeID, Chapter: nb.Chapter,
		Cells: json.RawMessage(nb.CellsJSON), Context: a.assembleContext(nb.ExtractNodeID),
	}, nil
}

// chapterOfNode returns the chapter title containing the node with id, "" if
// not resolvable.
func (a *App) chapterOfNode(id string) string {
	tgt, err := a.GetExtractTarget(id)
	if err != nil {
		return ""
	}
	pf, _ := content.ParseFile(tgt.FilePath, a.registry)
	if pf == nil {
		return ""
	}
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.ID == id {
				return ch.Title
			}
		}
	}
	return ""
}

// assembleContext builds the read-only context payload: extract text, tags,
// radius-1 orbit, and a text/web source passage. Every input degrades to a
// safe empty value; PDF sources omit the passage.
func (a *App) assembleContext(nodeID string) ContextPayload {
	out := ContextPayload{Tags: []string{}, Orbit: []OrbitItem{}, Source: SourceInfo{Kind: "none"}}
	tgt, err := a.GetExtractTarget(nodeID)
	if err != nil {
		return out
	}
	out.ExtractText = tgt.Text
	out.Source.Anchor = tgt.FromAnchor

	// Orbit + tags from the graph (empty & silent if the graph is unbuilt).
	if res, err := a.SearchGraph(tgt.Text); err == nil && res != nil {
		for _, gn := range res.Nodes {
			if gn.NodeText != tgt.Text {
				continue
			}
			out.Tags = append(out.Tags, gn.ContextTags...)
			for _, oe := range gn.Orbit {
				if oe.Radius == 1 {
					out.Orbit = append(out.Orbit, OrbitItem{ArrowLong: oe.ArrowLong, NeighborText: oe.NeighborText})
				}
			}
			break
		}
	}

	// Source passage: find the reading element whose notes file is this file.
	if tgt.FromAnchor != "" {
		if elems, err := a.learnStore.ListReading(); err == nil {
			for _, e := range elems {
				if e.NotesPath != tgt.FilePath {
					continue
				}
				out.Source.RefPath = e.RefPath
				if e.Medium == "pdf" {
					out.Source.Kind = "pdf"
					break
				}
				out.Source.Kind = e.Medium
				if proj, perr := source.Projection(filepath.Join(a.collectionDir, filepath.FromSlash(e.RefPath))); perr == nil {
					out.Source.Passage = source.Excerpt(proj, tgt.Text)
				}
				break
			}
		}
	}
	return out
}

// notebookLinksChanged notifies the frontend that a file's provenance dots
// need refreshing.
func (a *App) notebookLinksChanged(filePath string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "notebook:links-changed", map[string]string{"path": filePath})
	}
}
