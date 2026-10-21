package learning

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
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
	Medium       string // 'text' | 'web' | 'pdf'
	SourceURL    string // original URL for web; "" otherwise
}

const readingIntervalCap = 30

// ReadingDue reports whether a Source is due to be read at now: not done, and
// never scheduled or due by now. The due-reading SQL (StudyCounts, DueReading)
// applies the same rule.
func ReadingDue(r ReadingElement, now time.Time) bool {
	return !r.Done && (r.Due.IsZero() || !r.Due.After(now))
}

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

// UpsertReading stores r. Its NotesPath may be absolute (under the
// Collection) or already relative; it is stored relative.
func (s *Store) UpsertReading(r ReadingElement) error {
	notes, err := s.rel(r.NotesPath)
	if err != nil {
		return fmt.Errorf("learning: Source %q: %w", r.ID, err)
	}
	r.NotesPath = notes
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM reading_elements WHERE id=?`, r.ID).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		// Intentionally updates only display fields: imports always insert a fresh id,
		// so medium/source_url/position are set once at insert and never overwritten here.
		_, err := s.db.Exec(`UPDATE reading_elements SET ref_path=?, notes_path=?, title=? WHERE id=?`,
			r.RefPath, r.NotesPath, r.Title, r.ID)
		return err
	}
	_, err = s.db.Exec(`INSERT INTO reading_elements
		(id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done, created_at, medium, source_url)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.RefPath, r.NotesPath, r.Title, r.Position, r.ProjLen,
		nullableUnix(r.Due), maxInt(r.IntervalDays, 1), nullableUnix(r.LastRead), boolToInt(r.Done), time.Now().Unix(),
		defaultStr(r.Medium, "text"), r.SourceURL)
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
	row := s.db.QueryRow(`SELECT id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done, medium, source_url
		FROM reading_elements WHERE id=?`, id)
	r, err := s.scanReading(row)
	if err == sql.ErrNoRows {
		return ReadingElement{}, false, nil
	}
	return r, err == nil, err
}

func (s *Store) ListReading() ([]ReadingElement, error) {
	rows, err := s.db.Query(`SELECT id, ref_path, notes_path, title, position, proj_len, due, interval_days, last_read, done, medium, source_url
		FROM reading_elements ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReadingElement
	for rows.Next() {
		r, err := s.scanReading(rows)
		if errors.Is(err, errUnresolvable) {
			log.Printf("%v (skipped)", err)
			continue
		}
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

// scanReading reads one reading_elements row, NotesPath made absolute.
func (s *Store) scanReading(sc scanner) (ReadingElement, error) {
	var (
		r         ReadingElement
		pos       sql.NullString
		projLen   sql.NullInt64
		due, last sql.NullInt64
		done      int
		medium    sql.NullString
		srcURL    sql.NullString
	)
	if err := sc.Scan(&r.ID, &r.RefPath, &r.NotesPath, &r.Title, &pos, &projLen, &due, &r.IntervalDays, &last, &done, &medium, &srcURL); err != nil {
		return ReadingElement{}, err
	}
	r.Position = pos.String
	r.ProjLen = int(projLen.Int64)
	r.Due = unixToTime(due)
	r.LastRead = unixToTime(last)
	r.Done = done != 0
	r.Medium = medium.String
	r.SourceURL = srcURL.String
	abs, err := s.resolve(r.NotesPath)
	if err != nil {
		return ReadingElement{}, fmt.Errorf("learning: Source %q: %w: %w", r.ID, errUnresolvable, err)
	}
	r.NotesPath = abs
	return r, nil
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
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
