package search

// SetBleveSearcherForTest replaces the live Bleve index with a stub.
// Tests use this to assert that routes such as `\chapter` and `\notes`
// never reach the index (FR-004).
func (s *SearchService) SetBleveSearcherForTest(b BleveSearcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bleveIdx != nil {
		_ = s.bleveIdx.Close()
	}
	s.bleveIdx = b
}
