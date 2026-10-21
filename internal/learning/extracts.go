package learning

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"time"
)

// Extract is one distillation candidate: a (from) node not yet promoted to a card.
type Extract struct {
	ID            string
	FilePath      string
	Text          string
	FromAnchor    string
	Due           time.Time
	DeferralCount int
	Active        bool
}

const extractIntervalCapDays = 90

// ScheduleExtract is the pure deferral rule. action ∈ {"later","dismiss","promote"}.
func ScheduleExtract(prev Extract, action string, now time.Time) Extract {
	out := prev
	switch action {
	case "dismiss", "promote":
		out.Active = false
		return out
	case "later":
		out.DeferralCount = prev.DeferralCount + 1
	default:
		return out
	}
	days := math.Pow(1.5, float64(out.DeferralCount))
	if days > extractIntervalCapDays {
		days = extractIntervalCapDays
	}
	out.Due = now.Add(time.Duration(days * 24 * float64(time.Hour)))
	return out
}

func (s *Store) UpsertExtract(e Extract) (bool, error) {
	rel, err := s.rel(e.FilePath)
	if err != nil {
		return false, fmt.Errorf("learning: Extract %q: %w", e.ID, err)
	}
	e.FilePath = rel
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM extracts WHERE id=?`, e.ID).Scan(&exists); err != nil {
		return false, err
	}
	if exists > 0 {
		// Update only the mutable content fields; preserve active/due/deferral so a
		// re-sync of a file that still contains a dismissed extract's (from) node
		// does not resurrect it into the due queue.
		_, err := s.db.Exec(`UPDATE extracts SET file_path=?, text=?, from_anchor=? WHERE id=?`,
			e.FilePath, e.Text, e.FromAnchor, e.ID)
		return false, err
	}
	_, err = s.db.Exec(`INSERT INTO extracts
		(id, file_path, text, from_anchor, due, deferral_count, active, created_at)
		VALUES (?,?,?,?,NULL,0,1,?)`,
		e.ID, e.FilePath, e.Text, e.FromAnchor, time.Now().Unix())
	if err != nil {
		return false, fmt.Errorf("learning: insert extract: %w", err)
	}
	return true, nil
}

func (s *Store) ApplyExtractSchedule(id string, e Extract) error {
	_, err := s.db.Exec(`UPDATE extracts SET due=?, deferral_count=?, active=? WHERE id=?`,
		nullableUnix(e.Due), e.DeferralCount, boolToInt(e.Active), id)
	return err
}

func (s *Store) RetireMissingExtracts(filePath string, keep map[string]bool) (int, error) {
	filePath, err := s.rel(filePath)
	if err != nil {
		return 0, fmt.Errorf("learning: retire: %w", err)
	}
	rows, err := s.db.Query(`SELECT id FROM extracts WHERE file_path=? AND active=1`, filePath)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		if !keep[id] {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if _, err := s.db.Exec(`UPDATE extracts SET active=0 WHERE id=?`, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (s *Store) GetExtract(id string) (Extract, bool, error) {
	row := s.db.QueryRow(`SELECT id, file_path, text, from_anchor, due, deferral_count, active
		FROM extracts WHERE id=?`, id)
	e, err := s.scanExtract(row)
	if err == sql.ErrNoRows {
		return Extract{}, false, nil
	}
	return e, err == nil, err
}

func (s *Store) DueExtracts(now time.Time, limit int) ([]Extract, error) {
	rows, err := s.db.Query(`SELECT id, file_path, text, from_anchor, due, deferral_count, active
		FROM extracts
		WHERE active=1 AND (due IS NULL OR due <= ?)
		ORDER BY (due IS NULL), due ASC`, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Extract
	for rows.Next() {
		e, err := s.scanExtract(rows)
		if errors.Is(err, errUnresolvable) {
			log.Printf("%v (skipped)", err)
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, e)
		// The limit counts rows that resolve, so a skipped row cannot leave
		// the answer short.
		if len(out) == limit {
			break
		}
	}
	return out, rows.Err()
}

func (s *Store) CountDueExtracts(now time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM extracts WHERE active=1 AND (due IS NULL OR due <= ?)`,
		now.Unix()).Scan(&n)
	return n, err
}

// ExtractAnchors returns every extract's from_anchor keyed by extract id,
// retired rows included.
//
// The retired ones are the point. A promotion made before card provenance was
// preserved dropped the (from) line from the .n4l, so for those passages this
// row is the only surviving record of where they came from. The sources tree
// falls back to it rather than hiding a card whose anchor the file has lost.
func (s *Store) ExtractAnchors() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT id, from_anchor FROM extracts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id string
		var anchor sql.NullString
		if err := rows.Scan(&id, &anchor); err != nil {
			return nil, err
		}
		if anchor.String != "" {
			out[id] = anchor.String
		}
	}
	return out, rows.Err()
}

// scanExtract reads one extracts row, FilePath made absolute.
func (s *Store) scanExtract(sc scanner) (Extract, error) {
	var (
		e   Extract
		anc sql.NullString
		due sql.NullInt64
		act int
	)
	if err := sc.Scan(&e.ID, &e.FilePath, &e.Text, &anc, &due, &e.DeferralCount, &act); err != nil {
		return Extract{}, err
	}
	e.FromAnchor = anc.String
	e.Due = unixToTime(due)
	e.Active = act != 0
	abs, err := s.resolve(e.FilePath)
	if err != nil {
		return Extract{}, fmt.Errorf("learning: Extract %q: %w: %w", e.ID, errUnresolvable, err)
	}
	e.FilePath = abs
	return e, nil
}
