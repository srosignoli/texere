package collection

import (
	"log"

	"texere/internal/content"
	"texere/internal/graph"
	"texere/internal/learning"
	"texere/internal/search"
)

// Sync discovers study items across the Collection into the learning store.
// Writes into the open file are marked as self-writes, and if IDs were
// injected into it EventIDsInjected tells the editor to reload.
func (s *Session) Sync() (learning.SyncResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return learning.SyncResult{}, ErrNoCollection
	}
	return s.syncLocked()
}

// RebuildGraph rebuilds the graph from the Collection's files and then syncs
// study items. It waits for running Reads, and Reads wait for it.
func (s *Session) RebuildGraph() (*graph.BuildResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrNoCollection
	}
	// Release the BadgerDB lock before the build opens the store for writing.
	s.search.Close()
	br, buildErr := graph.BuildFromCollection(s.dir, s.registry)
	// A fresh service rather than a lazy reopen of the old one: the old one's
	// caches describe the previous graph. The explorer must follow it, or it
	// keeps querying the closed service and shows an empty graph until restart.
	s.search = search.NewSearchService(s.dir, s.registry)
	s.explorer.SetSearchService(s.search)

	if _, err := s.syncLocked(); err != nil {
		log.Printf("collection: sync after build: %v", err)
	}
	return br, buildErr
}

// syncLocked is the sync step shared by Sync and RebuildGraph. The caller holds mu.
func (s *Session) syncLocked() (learning.SyncResult, error) {
	open := s.currentOpenFile()
	var res learning.SyncResult
	err := s.SelfWrite(open, func() error {
		var err error
		res, err = learning.Sync(s.learn, s.n4lFiles(), s.registry)
		return err
	})
	if err != nil {
		return res, err
	}
	if open != "" {
		for _, p := range res.InjectedPaths {
			if p == open {
				s.notify(EventIDsInjected, map[string]string{"path": open})
				break
			}
		}
	}
	return res, nil
}

// n4lFiles lists the Collection's readable .n4l files.
func (s *Session) n4lFiles() []string {
	sums, err := content.ScanCollection(s.dir)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(sums))
	for _, sum := range sums {
		if sum.FilePath != "" && sum.ScanError == nil {
			files = append(files, sum.FilePath)
		}
	}
	return files
}
