package main

import (
	"errors"

	"texere/internal/collection"
	"texere/internal/search"
)

// SearchGraph executes a graph search query against the SSTorytime-KV BadgerDB.
// Returns a SearchResult in all cases — errors are embedded in the result, not
// returned as a Go error, so Wails never surfaces a raw error to the frontend.
func (a *App) SearchGraph(query string) (*search.SearchResult, error) {
	res, err := inCollection(a, func(h collection.Handles) (*search.SearchResult, error) {
		return h.Search().Search(query)
	})
	if errors.Is(err, collection.ErrNoCollection) {
		r := search.NewSearchResult()
		r.GraphUnavailable = true
		return r, nil
	}
	return res, err
}
