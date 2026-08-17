package learning

import (
	"database/sql"
	"strings"
	"time"
)

// Notebook is one processing-notebook lab record. CellsJSON holds only the
// user-authored cells (the context cell is derived and never persisted).
type Notebook struct {
	NotebookID    string
	ExtractNodeID string
	Chapter       string
	ContextTags   string // JSON array of tag strings
	CellsJSON     string // JSON array of cell objects
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UpsertNotebook inserts a new notebook or replaces an existing one by ID.
func (s *Store) UpsertNotebook(nb Notebook) error {
	_, err := s.db.Exec(
		`INSERT INTO notebooks
		   (notebook_id, extract_node_id, chapter, context_tags, cells_json, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?)
		 ON CONFLICT(notebook_id) DO UPDATE SET
		   extract_node_id=excluded.extract_node_id,
		   chapter=excluded.chapter,
		   context_tags=excluded.context_tags,
		   cells_json=excluded.cells_json,
		   updated_at=excluded.updated_at`,
		nb.NotebookID, nb.ExtractNodeID, nb.Chapter, nb.ContextTags, nb.CellsJSON,
		nb.CreatedAt.Unix(), nb.UpdatedAt.Unix())
	return err
}

// UpdateNotebookCells writes just the cells payload + updated timestamp.
func (s *Store) UpdateNotebookCells(id, cellsJSON string, updated time.Time) error {
	_, err := s.db.Exec(
		`UPDATE notebooks SET cells_json=?, updated_at=? WHERE notebook_id=?`,
		cellsJSON, updated.Unix(), id)
	return err
}

func (s *Store) GetNotebook(id string) (Notebook, bool, error) {
	return scanNotebook(s.db.QueryRow(
		`SELECT notebook_id, extract_node_id, chapter, context_tags, cells_json, created_at, updated_at
		 FROM notebooks WHERE notebook_id=?`, id))
}

func (s *Store) GetNotebookByExtract(extractNodeID string) (Notebook, bool, error) {
	return scanNotebook(s.db.QueryRow(
		`SELECT notebook_id, extract_node_id, chapter, context_tags, cells_json, created_at, updated_at
		 FROM notebooks WHERE extract_node_id=? ORDER BY created_at ASC LIMIT 1`, extractNodeID))
}

func scanNotebook(row *sql.Row) (Notebook, bool, error) {
	var (
		nb               Notebook
		created, updated int64
	)
	err := row.Scan(&nb.NotebookID, &nb.ExtractNodeID, &nb.Chapter,
		&nb.ContextTags, &nb.CellsJSON, &created, &updated)
	if err == sql.ErrNoRows {
		return Notebook{}, false, nil
	}
	if err != nil {
		return Notebook{}, false, err
	}
	nb.CreatedAt = time.Unix(created, 0)
	nb.UpdatedAt = time.Unix(updated, 0)
	return nb, true, nil
}

// NotebookLink ties a produced node (identified by the stable
// file+chapter+text tuple, since most nodes have no # @id) to its notebook.
type NotebookLink struct {
	FilePath   string
	Chapter    string
	NodeText   string
	NotebookID string
}

// AddNotebookLinks inserts links, ignoring exact duplicates.
func (s *Store) AddNotebookLinks(links []NotebookLink, created time.Time) error {
	for _, l := range links {
		if _, err := s.db.Exec(
			`INSERT OR IGNORE INTO notebook_links
			   (file_path, chapter, node_text, notebook_id, created_at)
			 VALUES (?,?,?,?,?)`,
			l.FilePath, l.Chapter, l.NodeText, l.NotebookID, created.Unix()); err != nil {
			return err
		}
	}
	return nil
}

// LinksForFile returns every link whose file_path matches.
func (s *Store) LinksForFile(filePath string) ([]NotebookLink, error) {
	rows, err := s.db.Query(
		`SELECT file_path, chapter, node_text, notebook_id
		 FROM notebook_links WHERE file_path=?`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NotebookLink
	for rows.Next() {
		var l NotebookLink
		if err := rows.Scan(&l.FilePath, &l.Chapter, &l.NodeText, &l.NotebookID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// NotebooksForNodes returns node id -> notebook id for the given node ids.
//
// Callers pass the ids they parsed out of a file rather than a path prefix. A
// node id is the node's bare short id (see internal/content/nodeid.go), with
// no file path encoded in it — there is no path prefix to range over in the
// first place. Exact ids have no such ambiguity, and extract_node_id is
// indexed either way.
//
// Where a node somehow has two notebooks the earliest wins, matching
// GetNotebookByExtract's "ORDER BY created_at ASC LIMIT 1".
func (s *Store) NotebooksForNodes(nodeIDs []string) (map[string]string, error) {
	out := map[string]string{}
	// Chunked: SQLite caps host parameters per statement (999 by default), and a
	// large note file can carry more id-bearing nodes than that.
	const chunk = 500
	for start := 0; start < len(nodeIDs); start += chunk {
		end := start + chunk
		if end > len(nodeIDs) {
			end = len(nodeIDs)
		}
		batch := nodeIDs[start:end]
		ph := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		rows, err := s.db.Query(
			`SELECT extract_node_id, notebook_id FROM notebooks
			 WHERE extract_node_id IN (`+ph+`)
			 ORDER BY created_at ASC`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var nodeID, nbID string
			if err := rows.Scan(&nodeID, &nbID); err != nil {
				rows.Close()
				return nil, err
			}
			if _, dup := out[nodeID]; !dup {
				out[nodeID] = nbID
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}
