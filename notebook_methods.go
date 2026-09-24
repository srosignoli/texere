package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/collection"
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
	return inCollection(a, func(h collection.Handles) (*NotebookView, error) {
		nb, ok, err := h.Learning().GetNotebookByExtract(nodeID)
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
				Chapter: a.chapterOfNode(h, nodeID), ContextTags: "[]", CellsJSON: "[]",
				CreatedAt: now, UpdatedAt: now,
			}
			if err := h.Learning().UpsertNotebook(nb); err != nil {
				return nil, err
			}
			// A gutter dot means "this node has a notebook", so creating one changes
			// what every open editor should be drawing. Without this the note editor
			// keeps its stale marks for as long as it stays mounted — and it stays
			// mounted while Process mode is open, which is exactly when notebooks get
			// created. That was the originally reported symptom: process an extract,
			// switch back, and no dot.
			a.notebooksChanged()
		}
		return &NotebookView{
			NotebookID: nb.NotebookID, NodeID: nodeID, Chapter: nb.Chapter,
			Cells: json.RawMessage(nb.CellsJSON), Context: a.assembleContext(h, nodeID),
		}, nil
	})
}

// PeekNotebook returns the notebook for a node WITHOUT creating one, or nil when
// the node has none.
//
// This is OpenNotebook minus its UpsertNotebook, and that difference is the
// whole point. The Process view calls this as the caret moves, so a creating
// lookup would write a notebook row for every node scrolled past, and
// GetNotebookLinks would then mark those lines as having notebooks — after one
// visit a node WOULD have one, so the pane would start showing it. The feature
// would defeat itself.
//
// Absence returns (nil, nil) rather than an error. Resting the caret on a plain
// line is the common case, not a failure, and an error would flash the pane's
// error banner constantly while navigating.
//
// Note GetNotebookByExtract is misnamed: it is a plain lookup on
// extract_node_id with no validation that the node is a registered extract, so
// it already serves any node id.
func (a *App) PeekNotebook(nodeID string) (*NotebookView, error) {
	return inCollection(a, func(h collection.Handles) (*NotebookView, error) {
		nb, ok, err := h.Learning().GetNotebookByExtract(nodeID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return &NotebookView{
			NotebookID: nb.NotebookID, NodeID: nodeID, Chapter: nb.Chapter,
			Cells: json.RawMessage(nb.CellsJSON), Context: a.assembleContext(h, nodeID),
		}, nil
	})
}

// FileNodeSpans gives the frontend the line range of every id-bearing node in a
// file, so a caret position can be resolved to a node id locally.
//
// Fetched once per file rather than queried per keystroke: caret movement is
// constant, and a round trip plus a file parse on every arrow key would be a
// stream of work for a read that changes nothing.
//
// A missing or unparseable file yields an empty slice, not an error — the
// caller is a navigation aid, not an editor, and has nothing useful to do with
// the failure.
func (a *App) FileNodeSpans(filePath string) ([]content.NodeSpan, error) {
	pf, _ := content.ParseFile(filePath, a.registry)
	spans := content.NodeSpans(pf)
	if spans == nil {
		return []content.NodeSpan{}, nil
	}
	return spans, nil
}

// SaveNotebook persists just the user cells (debounced autosave from the pane).
func (a *App) SaveNotebook(notebookID, cellsJSON string) error {
	return a.withCollection(func(h collection.Handles) error {
		return h.Learning().UpdateNotebookCells(notebookID, cellsJSON, time.Now())
	})
}

// GetNotebook returns a notebook by ID for read-only reopen; its context is
// recomputed from the notebook's extract node.
func (a *App) GetNotebook(notebookID string) (*NotebookView, error) {
	return inCollection(a, func(h collection.Handles) (*NotebookView, error) {
		nb, ok, err := h.Learning().GetNotebook(notebookID)
		if err != nil || !ok {
			return nil, errors.New("notebook not found")
		}
		return &NotebookView{
			NotebookID: nb.NotebookID, NodeID: nb.ExtractNodeID, Chapter: nb.Chapter,
			Cells: json.RawMessage(nb.CellsJSON), Context: a.assembleContext(h, nb.ExtractNodeID),
		}, nil
	})
}

// chapterOfNode returns the chapter title containing the node with id, "" if
// not resolvable.
func (a *App) chapterOfNode(h collection.Handles, id string) string {
	tgt, err := a.extractTarget(h, id)
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
func (a *App) assembleContext(h collection.Handles, nodeID string) ContextPayload {
	out := ContextPayload{Tags: []string{}, Orbit: []OrbitItem{}, Source: SourceInfo{Kind: "none"}}
	tgt, err := a.extractTarget(h, nodeID)
	if err != nil {
		return out
	}
	out.ExtractText = tgt.Text
	out.Source.Anchor = tgt.FromAnchor

	// Orbit + tags from the graph (empty & silent if the graph is unbuilt).
	if res, err := h.Search().Search(tgt.Text); err == nil && res != nil {
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
	// tgt.FilePath is already absolute (GetExtractTarget resolves it); e.NotesPath
	// is still the stored, collection-relative value, so it must be resolved
	// before comparing — otherwise a relative value can never equal an absolute
	// one and this lookup silently stops matching anything.
	if tgt.FromAnchor != "" {
		if elems, err := h.Learning().ListReading(); err == nil {
			for _, e := range elems {
				notesPath, rerr := h.Learning().Resolve(e.NotesPath)
				if rerr != nil || notesPath != tgt.FilePath {
					continue
				}
				out.Source.RefPath = e.RefPath
				if e.Medium == "pdf" {
					out.Source.Kind = "pdf"
					break
				}
				out.Source.Kind = e.Medium
				if p, perr := h.Path(e.RefPath); perr == nil {
					if proj, perr := source.Projection(p); perr == nil {
						out.Source.Passage = source.Excerpt(proj, tgt.Text)
					}
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

// notebooksChanged notifies the frontend that the set of nodes having notebooks
// has changed, so any open editor should re-fetch its gutter marks.
//
// Deliberately carries no file path. A node id is the node's bare short id
// (see internal/content/nodeid.go) — no path or chapter is encoded in it, so
// there is nothing to recover a file from, safely or otherwise. Listeners
// re-fetch marks for whichever file they have open; that is one indexed query
// and one parse, and it is always correct.
func (a *App) notebooksChanged() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "notebook:changed", map[string]string{})
	}
}

// pushedNodeKeys returns the (chapterTitle, nodeText) of every parsed node that
// the pushed text authored as a subject. A node matches when a collapsed
// inserted line equals its text or begins with "<text> " (text then arrow).
// Arrow targets are not matched. Matching is by text, so it survives the line
// shifts SaveFile causes when it injects "# @id" markers.
func pushedNodeKeys(pf *content.ParsedFile, insertedText string) [][2]string {
	if pf == nil {
		return nil
	}
	var lines []string
	for _, raw := range strings.Split(insertedText, "\n") {
		if c := strings.Join(strings.Fields(raw), " "); c != "" {
			lines = append(lines, c)
		}
	}
	var out [][2]string
	seen := map[[2]string]bool{}
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.Text == "" {
				continue
			}
			// Collapse node text the same way as the inserted lines so internal
			// double-spaces/tabs (which the parser only trims at the edges) do
			// not defeat the match. The stored key keeps the raw n.Text.
			nt := strings.Join(strings.Fields(n.Text), " ")
			for _, c := range lines {
				if c == nt || strings.HasPrefix(c, nt+" ") {
					key := [2]string{ch.Title, n.Text}
					if !seen[key] {
						seen[key] = true
						out = append(out, key)
					}
					break
				}
			}
		}
	}
	return out
}

// RecordPush records provenance links for the nodes a pushed draft authored,
// then signals the gutter to refresh. filePath must be the file the text was
// pushed into (already saved to disk by the caller).
func (a *App) RecordPush(notebookID, filePath, insertedText string) ([]string, error) {
	return inCollection(a, func(h collection.Handles) ([]string, error) {
		pf, _ := content.ParseFile(filePath, a.registry)
		keys := pushedNodeKeys(pf, insertedText)
		// filePath is absolute (content-layer, as passed in by the frontend); the
		// notebook_links.file_path column stores the collection-relative form.
		relPath, err := h.Learning().Rel(filePath)
		if err != nil {
			return nil, fmt.Errorf("RecordPush: %w", err)
		}
		links := make([]learning.NotebookLink, 0, len(keys))
		texts := make([]string, 0, len(keys))
		for _, k := range keys {
			links = append(links, learning.NotebookLink{
				FilePath: relPath, Chapter: k[0], NodeText: k[1], NotebookID: notebookID})
			texts = append(texts, k[1])
		}
		if err := h.Learning().AddNotebookLinks(links, time.Now()); err != nil {
			return nil, err
		}
		a.notebookLinksChanged(filePath)
		return texts, nil
	})
}

// NotebookLinkMark is one gutter dot: a line and the notebooks linked to the
// node on that line.
type NotebookLinkMark struct {
	Line        int      `json:"line"`
	NotebookIDs []string `json:"notebookIds"`
}

// NotebookMark is one gutter dot: a line whose node has a notebook.
//
// This replaces NotebookLinkMark as the gutter's source. The old mark meant
// "text on this line was pushed out of a notebook" and was keyed by
// (chapter, node_text), so any edit to the line silently dropped the dot. This
// one is keyed by node id, which is stable across edits.
type NotebookMark struct {
	Line       int    `json:"line"`
	NotebookID string `json:"notebookId"`
}

// NotebookMarks returns the lines of nodes in filePath that have a notebook.
//
// Read-only: it never creates a notebook. One store query for the file, then a
// single parse to turn node ids into line numbers.
func (a *App) NotebookMarks(filePath string) ([]NotebookMark, error) {
	return inCollection(a, func(h collection.Handles) ([]NotebookMark, error) {
		out := []NotebookMark{}
		pf, _ := content.ParseFile(filePath, a.registry)
		if pf == nil {
			return out, nil
		}
		// Ask for exactly the ids this file contains. A node id is now the node's
		// bare short id (see internal/content/nodeid.go) with no file path encoded
		// in it to range over, so an exact-id lookup is the query that makes sense.
		var ids []string
		for _, ch := range pf.Chapters {
			for _, n := range ch.Nodes {
				if n.ID != "" {
					ids = append(ids, n.ID)
				}
			}
		}
		if len(ids) == 0 {
			return out, nil
		}
		byNode, err := h.Learning().NotebooksForNodes(ids)
		if err != nil {
			return nil, err
		}
		for _, ch := range pf.Chapters {
			for _, n := range ch.Nodes {
				if n.ID == "" {
					continue
				}
				if nbID, ok := byNode[n.ID]; ok {
					out = append(out, NotebookMark{Line: n.Line, NotebookID: nbID})
				}
			}
		}
		return out, nil
	})
}

// GetNotebookLinks resolves this file's links to line marks by re-parsing and
// matching each linked (chapter, text) tuple to a current node line.
func (a *App) GetNotebookLinks(filePath string) ([]NotebookLinkMark, error) {
	return inCollection(a, func(h collection.Handles) ([]NotebookLinkMark, error) {
		// filePath is absolute (content-layer); notebook_links.file_path is stored
		// collection-relative, so the lookup key must be converted the same way
		// RecordPush converted it when writing the link.
		relPath, err := h.Learning().Rel(filePath)
		if err != nil {
			return nil, fmt.Errorf("GetNotebookLinks: %w", err)
		}
		links, err := h.Learning().LinksForFile(relPath)
		if err != nil {
			return nil, err
		}
		if len(links) == 0 {
			return []NotebookLinkMark{}, nil
		}
		// (chapter, text) -> set of notebook IDs
		byKey := map[[2]string]map[string]bool{}
		for _, l := range links {
			k := [2]string{l.Chapter, l.NodeText}
			if byKey[k] == nil {
				byKey[k] = map[string]bool{}
			}
			byKey[k][l.NotebookID] = true
		}
		pf, _ := content.ParseFile(filePath, a.registry)
		byLine := map[int]map[string]bool{}
		if pf != nil {
			for _, ch := range pf.Chapters {
				for _, n := range ch.Nodes {
					ids, ok := byKey[[2]string{ch.Title, n.Text}]
					if !ok {
						continue
					}
					if byLine[n.Line] == nil {
						byLine[n.Line] = map[string]bool{}
					}
					for id := range ids {
						byLine[n.Line][id] = true
					}
				}
			}
		}
		out := make([]NotebookLinkMark, 0, len(byLine))
		for line, ids := range byLine {
			list := make([]string, 0, len(ids))
			for id := range ids {
				list = append(list, id)
			}
			sort.Strings(list)
			out = append(out, NotebookLinkMark{Line: line, NotebookIDs: list})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Line < out[j].Line })
		return out, nil
	})
}
