package learning

import "time"

// StudyStats3 holds the three due counts driving the focus-switch badges.
type StudyStats3 struct {
	Read    int `json:"read"`
	Process int `json:"process"`
	Recall  int `json:"recall"`
}

func (s *Store) StudyCounts(now time.Time) (StudyStats3, error) {
	var out StudyStats3
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM reading_elements
		WHERE done=0 AND (due IS NULL OR due <= ?)`, now.Unix()).Scan(&out.Read); err != nil {
		return out, err
	}
	p, err := s.CountDueExtracts(now)
	if err != nil {
		return out, err
	}
	out.Process = p
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items
		WHERE active=1 AND (due IS NULL OR due <= ?)`, now.Unix()).Scan(&out.Recall); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) DueReading(now time.Time, limit int) ([]ReadingElement, error) {
	rows, err := s.db.Query(`SELECT id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done, medium, source_url
		FROM reading_elements
		WHERE done=0 AND (due IS NULL OR due <= ?)
		ORDER BY (due IS NULL), due ASC
		LIMIT ?`, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReadingElement
	for rows.Next() {
		r, err := scanReading(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
