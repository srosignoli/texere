package learning

import (
	"database/sql"
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
