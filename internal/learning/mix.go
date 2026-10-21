package learning

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Mix is the target share of Turns per kind of Study Element, as whole-number
// weights turned into shares when used. A weight of 0 means that kind is never
// served. See CONTEXT.md.
type Mix struct {
	Source  int `json:"source"`
	Extract int `json:"extract"`
	Card    int `json:"card"`
}

// DefaultMix is the Mix of a Collection that never set one.
var DefaultMix = Mix{Source: 20, Extract: 15, Card: 65}

const mixKey = "next_mix"

// LoadMix returns the Collection's Mix, or DefaultMix when none was saved or
// every weight is 0.
func (s *Store) LoadMix() (Mix, error) {
	var raw string
	switch err := s.db.QueryRow(`SELECT value FROM study_settings WHERE key=?`, mixKey).Scan(&raw); err {
	case nil:
	case sql.ErrNoRows:
		return DefaultMix, nil
	default:
		return Mix{}, fmt.Errorf("learning: load mix: %w", err)
	}
	var m Mix
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return Mix{}, fmt.Errorf("learning: load mix: %w", err)
	}
	if m == (Mix{}) {
		return DefaultMix, nil
	}
	return m, nil
}

// SaveMix stores the Collection's Mix. A negative weight is refused and the
// stored Mix is left as it was.
func (s *Store) SaveMix(m Mix) error {
	if m.Source < 0 || m.Extract < 0 || m.Card < 0 {
		return fmt.Errorf("learning: save mix: weights must not be negative, got %+v", m)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO study_settings(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, mixKey, string(raw))
	return err
}
