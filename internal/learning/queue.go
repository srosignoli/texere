package learning

import (
	"errors"
	"log"
	"sync"
	"time"
)

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
		ORDER BY (due IS NULL), due ASC`, now.Unix())
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
		// The limit counts rows that resolve, so a skipped row cannot leave
		// the answer short.
		if len(out) == limit {
			break
		}
	}
	return out, rows.Err()
}

// Kind is the kind of Study Element a Turn serves.
type Kind string

const (
	KindSource  Kind = "source"
	KindExtract Kind = "extract"
	KindCard    Kind = "card"
)

// Turn is one Study Element served by the Study Queue. Exactly one of Source,
// Extract and Card is set, the one Kind names.
type Turn struct {
	Kind    Kind
	Source  *ReadingElement
	Extract *Extract
	Card    *CardView
}

// QueueDeps are what the Study Queue reads. OpenSitting reports the Source
// whose Sitting is open, peeks excluded; nil means there never is one.
type QueueDeps struct {
	Store       *Store
	OpenSitting func() (sourceID string, ok bool)
	Now         func() time.Time // nil = time.Now
}

// Queue is the Study Queue of one Session. See CONTEXT.md.
type Queue struct {
	d       QueueDeps
	mu      sync.Mutex
	current *Turn        // served and not yet ended; nil when none
	served  map[Kind]int // Turns served this Session, per kind
	n       int          // Turns served this Session
}

// tieOrder is the order kinds win a tie in: the Card first, the only kind
// with a deadline.
var tieOrder = []Kind{KindCard, KindExtract, KindSource}

// NewQueue returns a Study Queue that has served nothing yet.
func NewQueue(d QueueDeps) *Queue {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Queue{d: d, served: map[Kind]int{}}
}

// Next returns the Turn to study now, or nil when nothing is due.
//
// The current Turn comes back, counted once, until it has ended: a Turn has
// ended when its Study Element is no longer due (grading a Card, any Extract
// action and ending a Sitting all move its due date). Next a Source whose
// Sitting is open is resumed, before the Mix is asked. Otherwise Next serves the kind furthest below its Mix share: the largest
// w/W·(n+1) − served, over the kinds with something due and a weight above 0
// (W is the sum of their weights). Ties go Card, Extract, Source.
func (q *Queue) Next() (*Turn, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.d.Now()
	if q.current != nil {
		due, err := q.d.Store.StillDue(q.current.Kind, q.current.id(), now)
		if err != nil {
			return nil, err
		}
		if due {
			return q.current, nil
		}
		q.current = nil
	}
	if t, err := q.openSitting(now); err != nil || t != nil {
		if t != nil {
			q.serve(t)
		}
		return t, err
	}
	mix, err := q.d.Store.LoadMix()
	if err != nil {
		return nil, err
	}
	weights := map[Kind]int{KindSource: mix.Source, KindExtract: mix.Extract, KindCard: mix.Card}

	candidates := map[Kind]*Turn{}
	total := 0
	for _, k := range tieOrder {
		if weights[k] <= 0 {
			continue
		}
		t, err := q.firstDue(k, now)
		if err != nil {
			return nil, err
		}
		if t != nil {
			candidates[k] = t
			total += weights[k]
		}
	}
	var best *Turn
	bestDeficit := 0.0
	for _, k := range tieOrder {
		t, ok := candidates[k]
		if !ok {
			continue
		}
		d := float64(weights[k])/float64(total)*float64(q.n+1) - float64(q.served[k])
		if best == nil || d > bestDeficit {
			best, bestDeficit = t, d
		}
	}
	if best == nil {
		return nil, nil
	}
	q.serve(best)
	return best, nil
}

// serve counts t and makes it the current Turn.
func (q *Queue) serve(t *Turn) {
	q.served[t.Kind]++
	q.n++
	q.current = t
}

// openSitting is the Turn for a Source whose Sitting is open, or nil.
func (q *Queue) openSitting(now time.Time) (*Turn, error) {
	if q.d.OpenSitting == nil {
		return nil, nil
	}
	id, ok := q.d.OpenSitting()
	if !ok {
		return nil, nil
	}
	r, ok, err := q.d.Store.GetReading(id)
	if err != nil || !ok || !ReadingDue(r, now) {
		return nil, err
	}
	return &Turn{Kind: KindSource, Source: &r}, nil
}

func (t *Turn) id() string {
	switch t.Kind {
	case KindSource:
		return t.Source.ID
	case KindExtract:
		return t.Extract.ID
	default:
		return t.Card.ID
	}
}

// StillDue reports whether the Study Element of kind k and id is due at now,
// by the same rule as the due queries. A row that no longer exists is not due.
func (s *Store) StillDue(k Kind, id string, now time.Time) (bool, error) {
	var q string
	switch k {
	case KindSource:
		q = `SELECT COUNT(*) FROM reading_elements WHERE id=? AND done=0 AND (due IS NULL OR due <= ?)`
	case KindExtract:
		q = `SELECT COUNT(*) FROM extracts WHERE id=? AND active=1 AND (due IS NULL OR due <= ?)`
	default:
		q = `SELECT COUNT(*) FROM items WHERE id=? AND active=1 AND (due IS NULL OR due <= ?)`
	}
	var n int
	if err := s.db.QueryRow(q, id, now.Unix()).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// firstDue is the first due Study Element of kind k, in due order, or nil.
func (q *Queue) firstDue(k Kind, now time.Time) (*Turn, error) {
	switch k {
	case KindSource:
		rs, err := q.d.Store.DueReading(now, 1)
		if err != nil || len(rs) == 0 {
			return nil, err
		}
		return &Turn{Kind: k, Source: &rs[0]}, nil
	case KindExtract:
		es, err := q.d.Store.DueExtracts(now, 1)
		if err != nil || len(es) == 0 {
			return nil, err
		}
		return &Turn{Kind: k, Extract: &es[0]}, nil
	default:
		cs, err := q.d.Store.DueQueue(now, 1)
		if err != nil || len(cs) == 0 {
			return nil, err
		}
		return &Turn{Kind: k, Card: &cs[0]}, nil
	}
}
