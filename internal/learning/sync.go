package learning

import (
	"fmt"
	"time"

	"texere/internal/content"
)

// Sync discovers study items across the given files, assigning # @id markers to
// study nodes that lack them (written back to disk), and reconciles the store:
// new cards are inserted, cards no longer generated for a file are retired.
func Sync(store *Store, files []string, reg *content.SSTRegistry) (SyncResult, error) {
	var res SyncResult
	var skipped []string
	for _, path := range files {
		pf, _ := content.ParseFile(path, reg)
		if pf == nil {
			skipped = append(skipped, path)
			continue
		}
		// Assign # @id to study-marked nodes missing one (mutates pf in place).
		if _, err := content.InjectMissingIDs(pf); err != nil {
			return res, err
		}
		cards := GenerateCards(pf, reg)
		keep := make(map[string]bool, len(cards))
		for _, c := range cards {
			added, err := store.UpsertItem(c)
			if err != nil {
				return res, err
			}
			if added {
				res.Added++
			}
			keep[c.ItemID()] = true
		}
		retired, err := store.RetireMissing(path, keep)
		if err != nil {
			return res, err
		}
		res.Retired += retired
	}
	st, err := store.CountStats(time.Now())
	if err != nil {
		return res, err
	}
	res.Total = st.Total
	if len(skipped) > 0 {
		return res, fmt.Errorf("learning: sync skipped %d unparseable file(s): %v", len(skipped), skipped)
	}
	return res, nil
}
