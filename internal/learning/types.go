// types.go — shared types for the learning package.
// Package learning owns the local SQLite scheduling state (learning.db) for
// Texere's spaced-repetition recall: study items derived from N4L cards, their
// FSRS scheduling state, and review history. learning.db is machine-local and
// NOT rebuildable (review history is earned), keyed by each node's # @id.
package learning

import "time"

// Grade is a four-point FSRS review rating.
type Grade int

const (
	GradeAgain Grade = 1
	GradeHard  Grade = 2
	GradeGood  Grade = 3
	GradeEasy  Grade = 4
)

// Card type discriminators stored in items.card_type. Phase 1 generates only
// these two; the column leaves room for "reverse"/"sequence" later.
const (
	CardTypeCloze = "cloze"
	CardTypeBasic = "basic"
)

// SchedState is the FSRS scheduling state of one item. The zero value is a
// never-reviewed "new" card (Due.IsZero()).
type SchedState struct {
	Stability  float64
	Difficulty float64
	Due        time.Time
	LastReview time.Time
	Reps       int
	Lapses     int
	State      int // FSRS state enum (0=New,1=Learning,2=Review,3=Relearning)
}

// GeneratedCard is one card produced from a parsed node, before persistence.
type GeneratedCard struct {
	NodeID        string
	FilePath      string
	CardType      string
	Front         string
	Back          string
	Discriminator string // unique within (NodeID, CardType): brace index or arrow index
}

// ItemID is the stable primary key of the persisted item.
func (g GeneratedCard) ItemID() string {
	return g.NodeID + "::" + g.CardType + "::" + g.Discriminator
}

// CardView is the front-end-facing shape of a due card.
type CardView struct {
	ID       string `json:"id"`
	Front    string `json:"front"`
	Back     string `json:"back"`
	CardType string `json:"cardType"`
	FilePath string `json:"filePath"`
	NodeID   string `json:"nodeId"`
}

// SyncResult summarizes one discovery pass.
type SyncResult struct {
	Added           int      `json:"added"`
	Retired         int      `json:"retired"`
	Total           int      `json:"total"`
	InjectedPaths   []string `json:"injectedPaths,omitempty"` // paths where # @id markers were written
	ExtractsAdded   int      `json:"extractsAdded"`
	ExtractsRetired int      `json:"extractsRetired"`
}

// Stats are minimal counts for the Study entry badge.
type Stats struct {
	Due   int `json:"due"`
	New   int `json:"new"`
	Total int `json:"total"`
}
