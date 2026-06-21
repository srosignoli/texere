package learning

import (
	"database/sql"
	"math"
	"time"
)

// ReadingElement is one source being read incrementally.
type ReadingElement struct {
	ID           string
	RefPath      string
	NotesPath    string
	Title        string
	Position     string
	ProjLen      int
	Due          time.Time
	LastRead     time.Time
	IntervalDays int
	Done         bool
}

const readingIntervalCap = 30

// ScheduleReading is the pure resurface rule. action ∈ {"continue","later","done"}.
func ScheduleReading(prev ReadingElement, action string, now time.Time) ReadingElement {
	out := prev
	if action == "done" {
		out.Done = true
		return out
	}
	iv := prev.IntervalDays
	if iv < 1 {
		iv = 1
	}
	iv = int(math.Round(float64(iv) * 1.5))
	if iv > readingIntervalCap {
		iv = readingIntervalCap
	}
	out.IntervalDays = iv
	out.Due = now.Add(time.Duration(iv) * 24 * time.Hour)
	out.LastRead = now
	return out
}

func (s *Store) UpsertReading(r ReadingElement) error {
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM reading_elements WHERE id=?`, r.ID).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		_, err := s.db.Exec(`UPDATE reading_elements SET ref_path=?, notes_path=?, title=? WHERE id=?`,
			r.RefPath, r.NotesPath, r.Title, r.ID)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO reading_elements
		(id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.RefPath, r.NotesPath, r.Title, r.Position, r.ProjLen,
		nullableUnix(r.Due), maxInt(r.IntervalDays, 1), nullableUnix(r.LastRead), boolToInt(r.Done), time.Now().Unix())
	return err
}

func (s *Store) SetReadingProgress(id, position string, projLen int) error {
	_, err := s.db.Exec(`UPDATE reading_elements SET position=?, proj_len=? WHERE id=?`, position, projLen, id)
	return err
}

func (s *Store) ApplyReadingSchedule(id string, r ReadingElement) error {
	_, err := s.db.Exec(`UPDATE reading_elements SET due=?, interval_days=?, last_read=?, done=? WHERE id=?`,
		nullableUnix(r.Due), r.IntervalDays, nullableUnix(r.LastRead), boolToInt(r.Done), id)
	return err
}

func (s *Store) GetReading(id string) (ReadingElement, bool, error) {
	row := s.db.QueryRow(`SELECT id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done
		FROM reading_elements WHERE id=?`, id)
	r, err := scanReading(row)
	if err == sql.ErrNoRows {
		return ReadingElement{}, false, nil
	}
	return r, err == nil, err
}

func (s *Store) ListReading() ([]ReadingElement, error) {
	rows, err := s.db.Query(`SELECT id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done
		FROM reading_elements ORDER BY created_at DESC`)
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

type scanner interface {
	Scan(dest ...any) error
}

func scanReading(sc scanner) (ReadingElement, error) {
	var (
		r         ReadingElement
		pos       sql.NullString
		projLen   sql.NullInt64
		due, last sql.NullInt64
		done      int
	)
	if err := sc.Scan(&r.ID, &r.RefPath, &r.NotesPath, &r.Title, &pos, &projLen, &due, &r.IntervalDays, &last, &done); err != nil {
		return ReadingElement{}, err
	}
	r.Position = pos.String
	r.ProjLen = int(projLen.Int64)
	r.Due = unixToTime(due)
	r.LastRead = unixToTime(last)
	r.Done = done != 0
	return r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
