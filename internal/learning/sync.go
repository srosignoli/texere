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
		// relPath is the collection-relative form stored in items.file_path and
		// extracts.file_path. pf.Path (== path, absolute) must stay absolute below:
		// InjectMissingIDs/InjectMissingExtractIDs read and write the real file
		// through it. Only the values that end up in the DB are relativized.
		relPath, err := store.Rel(path)
		if err != nil {
			return res, fmt.Errorf("learning: sync: %w", err)
		}
		// Assign # @id to study-marked nodes missing one (mutates pf in place).
		injected, err := content.InjectMissingIDs(pf)
		if err != nil {
			return res, err
		}
		// Only record the path when at least one ID was newly written (i.e. the
		// injection was not already present); AlreadyHad entries mean no disk write.
		wroteIDs := false
		for _, inj := range injected {
			if !inj.AlreadyHad {
				res.InjectedPaths = append(res.InjectedPaths, path)
				wroteIDs = true
				break
			}
		}
		cards := GenerateCards(pf)
		keep := make(map[string]bool, len(cards))
		for _, c := range cards {
			c.FilePath = relPath
			added, err := store.UpsertItem(c)
			if err != nil {
				return res, err
			}
			if added {
				res.Added++
			}
			keep[c.ItemID()] = true
		}
		retired, err := store.RetireMissing(relPath, keep)
		if err != nil {
			return res, err
		}
		res.Retired += retired

		// Only re-parse when InjectMissingIDs actually wrote to disk: a write shifts
		// on-disk line numbers (# @id lines added above nodes) so the extract
		// id-injection below needs fresh offsets. When nothing was written the file
		// is unchanged and pf (already mutated in place) is still valid — the common
		// steady-state path, so skip the second full parse.
		if wroteIDs {
			pf, _ = content.ParseFile(path, reg)
			if pf == nil {
				skipped = append(skipped, path)
				continue
			}
		}

		// Extract ids for hand-authored (from) nodes lacking one (writes back).
		exInjected, err := content.InjectMissingExtractIDs(pf)
		if err != nil {
			return res, err
		}
		// extract injections are always genuine writes (AlreadyHad is never set)
		for _, inj := range exInjected {
			if !inj.AlreadyHad {
				res.InjectedPaths = append(res.InjectedPaths, path)
				break
			}
		}
		exs := GenerateExtracts(pf)
		keepEx := make(map[string]bool, len(exs))
		for _, e := range exs {
			e.FilePath = relPath
			added, err := store.UpsertExtract(e)
			if err != nil {
				return res, err
			}
			if added {
				res.ExtractsAdded++
			}
			keepEx[e.ID] = true
		}
		retiredEx, err := store.RetireMissingExtracts(relPath, keepEx)
		if err != nil {
			return res, err
		}
		res.ExtractsRetired += retiredEx
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
