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
