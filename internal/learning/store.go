package learning

import (
	"database/sql"
	"fmt"
	"time"
)

func nullableUnix(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}

func unixToTime(v sql.NullInt64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return time.Unix(v.Int64, 0)
}

// UpsertItem inserts a new item or updates front/back + reactivates an existing
// one, preserving its scheduling columns.
func (s *Store) UpsertItem(g GeneratedCard) (bool, error) {
	id := g.ItemID()
	var exists int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE id = ?`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("learning: upsert lookup: %w", err)
	}
	if exists > 0 {
		_, err := s.db.Exec(`UPDATE items SET front=?, back=?, file_path=?, active=1 WHERE id=?`,
			g.Front, g.Back, g.FilePath, id)
		return false, err
	}
	_, err = s.db.Exec(`INSERT INTO items
		(id, node_id, file_path, card_type, front, back, active, reps, lapses, state, created_at)
		VALUES (?,?,?,?,?,?,1,0,0,0,?)`,
		id, g.NodeID, g.FilePath, g.CardType, g.Front, g.Back, time.Now().Unix())
	if err != nil {
		return false, fmt.Errorf("learning: insert item: %w", err)
	}
	return true, nil
}

// RetireMissing deactivates active items of filePath whose id is not in keep.
func (s *Store) RetireMissing(filePath string, keep map[string]bool) (int, error) {
	rows, err := s.db.Query(`SELECT id FROM items WHERE file_path=? AND active=1`, filePath)
	if err != nil {
		return 0, err
	}
	var toRetire []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		if !keep[id] {
			toRetire = append(toRetire, id)
		}
	}
	rows.Close()
	for _, id := range toRetire {
		if _, err := s.db.Exec(`UPDATE items SET active=0 WHERE id=?`, id); err != nil {
			return 0, err
		}
	}
	return len(toRetire), nil
}

// DueQueue returns active items that are due now (due<=now) or new (due NULL),
// due cards first by ascending due date, then new cards.
func (s *Store) DueQueue(now time.Time, limit int) ([]CardView, error) {
	rows, err := s.db.Query(`SELECT id, front, back, card_type, file_path, node_id
		FROM items
		WHERE active=1 AND (due IS NULL OR due <= ?)
		ORDER BY (due IS NULL), due ASC
		LIMIT ?`, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CardView
	for rows.Next() {
		var c CardView
		if err := rows.Scan(&c.ID, &c.Front, &c.Back, &c.CardType, &c.FilePath, &c.NodeID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// LoadSched returns the scheduling state for an item.
func (s *Store) LoadSched(itemID string) (SchedState, bool, error) {
	var (
		st         SchedState
		stab, diff sql.NullFloat64
		due, last  sql.NullInt64
	)
	err := s.db.QueryRow(`SELECT stability, difficulty, due, last_review, reps, lapses, state
		FROM items WHERE id=?`, itemID).
		Scan(&stab, &diff, &due, &last, &st.Reps, &st.Lapses, &st.State)
	if err == sql.ErrNoRows {
		return SchedState{}, false, nil
	}
	if err != nil {
		return SchedState{}, false, err
	}
	st.Stability = stab.Float64
	st.Difficulty = diff.Float64
	st.Due = unixToTime(due)
	st.LastReview = unixToTime(last)
	return st, true, nil
}

// ApplyReview writes the new scheduling state and appends a review log.
func (s *Store) ApplyReview(itemID string, st SchedState, grade Grade, elapsedMs, oldState int, now time.Time) error {
	if _, err := s.db.Exec(`UPDATE items
		SET stability=?, difficulty=?, due=?, last_review=?, reps=?, lapses=?, state=?
		WHERE id=?`,
		st.Stability, st.Difficulty, nullableUnix(st.Due), nullableUnix(st.LastReview),
		st.Reps, st.Lapses, st.State, itemID); err != nil {
		return fmt.Errorf("learning: apply review update: %w", err)
	}
	var cardType string
	if err := s.db.QueryRow(`SELECT card_type FROM items WHERE id=?`, itemID).Scan(&cardType); err != nil {
		return fmt.Errorf("learning: review log card_type lookup: %w", err)
	}
	_, err := s.db.Exec(`INSERT INTO review_logs
		(item_id, reviewed_at, grade, elapsed_ms, old_state, new_stability, card_type)
		VALUES (?,?,?,?,?,?,?)`,
		itemID, now.Unix(), int(grade), elapsedMs, oldState, st.Stability, cardType)
	return err
}

// GetCardView fetches one item's view by id.
func (s *Store) GetCardView(itemID string) (CardView, bool, error) {
	var c CardView
	err := s.db.QueryRow(`SELECT id, front, back, card_type, file_path, node_id
		FROM items WHERE id=?`, itemID).
		Scan(&c.ID, &c.Front, &c.Back, &c.CardType, &c.FilePath, &c.NodeID)
	if err == sql.ErrNoRows {
		return CardView{}, false, nil
	}
	return c, err == nil, err
}

// CountStats returns due/new/total counts over active items.
func (s *Store) CountStats(now time.Time) (Stats, error) {
	var st Stats
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE active=1`).Scan(&st.Total); err != nil {
		return st, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE active=1 AND due IS NOT NULL AND due <= ?`,
		now.Unix()).Scan(&st.Due); err != nil {
		return st, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE active=1 AND due IS NULL`).Scan(&st.New); err != nil {
		return st, err
	}
	return st, nil
}
