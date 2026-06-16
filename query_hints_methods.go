package main

import "sort"

// Query-hint listing methods: read-only enumerations that power the graph
// query bars' autocomplete (suggesting the user's real chapters, context
// tags, and arrow codes after the matching directive). They reuse the
// existing search pipeline rather than duplicating its lazy-open logic.
//
// All three return a plain []string (possibly empty) and never an error —
// the frontend treats an empty list as "no live values" and falls back to
// keyword-only suggestions, so a missing/unbuilt graph degrades gracefully.

// ListChapters returns every chapter title in the open collection, sorted.
// Backed by the same listing path as `\chapter any`.
func (a *App) ListChapters() []string {
	if a.searchSvc == nil {
		return []string{}
	}
	res, err := a.searchSvc.Search(`\chapter any \limit 10000`)
	if err != nil || res == nil {
		return []string{}
	}
	out := make([]string, 0, len(res.Listings))
	for _, item := range res.Listings {
		if item.Kind == "chapter" && item.Title != "" {
			out = append(out, item.Title)
		}
	}
	sort.Strings(out)
	return out
}

// ListContextTags returns every distinct context tag in the open collection,
// sorted. Backed by the same listing path as `\context any`.
func (a *App) ListContextTags() []string {
	if a.searchSvc == nil {
		return []string{}
	}
	res, err := a.searchSvc.Search(`\context any \limit 10000`)
	if err != nil || res == nil {
		return []string{}
	}
	out := make([]string, 0, len(res.Listings))
	for _, item := range res.Listings {
		if item.Kind == "context" && item.Title != "" {
			out = append(out, item.Title)
		}
	}
	sort.Strings(out)
	return out
}

// ListArrowCodes returns every registered arrow short code, sorted. Read
// straight from the registry (no graph store access needed), so it works even
// before the BadgerDB/Bleve index is opened.
func (a *App) ListArrowCodes() []string {
	if a.registry == nil {
		return []string{}
	}
	out := make([]string, 0, len(a.registry.Arrows))
	for code := range a.registry.Arrows {
		if code != "" {
			out = append(out, code)
		}
	}
	sort.Strings(out)
	return out
}
