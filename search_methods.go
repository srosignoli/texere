package main

import "texere/internal/search"

// SearchGraph executes a graph search query against the SSTorytime-KV BadgerDB.
// Returns a SearchResult in all cases — errors are embedded in the result, not
// returned as a Go error, so Wails never surfaces a raw error to the frontend.
func (a *App) SearchGraph(query string) (*search.SearchResult, error) {
	if a.searchSvc == nil {
		r := search.NewSearchResult()
		r.GraphUnavailable = true
		return r, nil
	}
	return a.searchSvc.Search(query)
}
