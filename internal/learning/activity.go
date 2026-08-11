package learning

import (
	"database/sql"
	"time"
)

// LogActivity records one event the schema cannot otherwise reconstruct.
//
// Only two kinds exist: "read" (a reading session, amount = seconds) and
// "process" (an extract processed, detail = the scheduler's own action string).
// Card and extract *creation* are deliberately absent — items.created_at and
// extracts.created_at already record those, and a second copy could disagree.
func (s *Store) LogActivity(kind, refID, detail string, amount int, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO activity_log (kind, ref_id, at, detail, amount) VALUES (?,?,?,?,?)`,
		kind, refID, at.Unix(), detail, amount)
	return err
}

// ActivitySince returns the timestamp of the earliest logged event of one kind,
// or of any kind when kind is "".
//
// Per-kind on purpose. A single collection-wide flag let one dismissed extract
// switch the READING chart out of its "still collecting" state, so a log one
// second old would assert twelve weeks of zero reading.
//
// ok=false means nothing of that kind is logged. The caller needs that distinct
// from a zero time, which a UI would happily render as 1970.
func (s *Store) ActivitySince(kind string) (time.Time, bool, error) {
	var at sql.NullInt64
	var err error
	if kind == "" {
		err = s.db.QueryRow(`SELECT MIN(at) FROM activity_log`).Scan(&at)
	} else {
		err = s.db.QueryRow(`SELECT MIN(at) FROM activity_log WHERE kind=?`, kind).Scan(&at)
	}
	if err != nil {
		return time.Time{}, false, err
	}
	if !at.Valid {
		return time.Time{}, false, nil
	}
	return time.Unix(at.Int64, 0), true, nil
}

// ActivityWeeks returns exactly `weeks` buckets, oldest first, the last being
// the week containing now. "read" sums amount as minutes; every other kind
// counts rows.
//
// Bucketed in Go rather than SQL because week boundaries have to be local:
// dividing a unix timestamp by 604800 buckets by UTC and drops an evening
// session into the wrong week for anyone west of Greenwich.
func (s *Store) ActivityWeeks(kind string, weeks int, now time.Time) ([]WeekCount, error) {
	if weeks <= 0 {
		return []WeekCount{}, nil // guard before make: a negative length panics
	}
	out := make([]WeekCount, weeks)
	// Start of the bucket window: midnight local, (weeks-1) weeks back.
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -7*(weeks-1))
	for i := range out {
		out[i].WeekStart = start.AddDate(0, 0, 7*i).Format("2006-01-02")
	}

	rows, err := s.db.Query(
		`SELECT at, amount FROM activity_log WHERE kind=? AND at >= ?`, kind, start.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var at int64
		var amount int
		if err := rows.Scan(&at, &amount); err != nil {
			return nil, err
		}
		idx := int(time.Unix(at, 0).Sub(start).Hours() / (24 * 7))
		if idx < 0 || idx >= weeks {
			continue
		}
		if kind == "read" {
			out[idx].Count += amount
		} else {
			out[idx].Count++
		}
	}
	if kind == "read" {
		for i := range out {
			out[i].Count /= 60 // seconds -> whole minutes
		}
	}
	return out, rows.Err()
}
