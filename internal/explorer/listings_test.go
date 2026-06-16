package explorer

import (
	"testing"

	"texere/internal/search"
)

// TestBuildResultSet_FromListingsTier_Chapters verifies that a `\chapter any`
// listing (Kind "chapter" rows) is synthesized into a collection map: each
// chapter becomes a hub node, each of its context tags a satellite, joined by
// a contains edge. Chapter hubs are seeds; satellites are not.
func TestBuildResultSet_FromListingsTier_Chapters(t *testing.T) {
	sr := &search.SearchResult{
		Listings: []search.ListingItem{
			{Kind: "chapter", Title: "Cooking", Contexts: []string{"recipes", "desserts"}},
			{Kind: "chapter", Title: "Travel", Contexts: []string{"flights"}},
		},
	}
	rs := BuildResultSet(sr, nil, `\chapter any`)

	// 2 hubs + 3 distinct satellites (recipes, desserts, flights).
	if len(rs.Nodes) != 5 {
		t.Fatalf("nodes: got %d, want 5 (2 chapters + 3 contexts): %+v", len(rs.Nodes), rs.Nodes)
	}
	// 3 contains edges (one per context tag).
	if len(rs.Edges) != 3 {
		t.Fatalf("edges: got %d, want 3", len(rs.Edges))
	}
	for _, e := range rs.Edges {
		if e.Meta != MetaContains {
			t.Errorf("edge %v->%v: meta %v, want MetaContains", e.Src, e.Dst, e.Meta)
		}
	}
	// Chapter hubs are seeds; their context satellites are not.
	cooking := nodeKey{Text: "Cooking", Chapter: "Cooking"}
	recipes := nodeKey{Text: "recipes", Chapter: "Cooking"}
	if !rs.SeedKeys[cooking] {
		t.Errorf("expected chapter hub %v to be a seed", cooking)
	}
	if rs.SeedKeys[recipes] {
		t.Errorf("context satellite %v should NOT be a seed", recipes)
	}
	// Satellites are grouped onto their chapter's layer (primary context = chapter).
	if idx, ok := rs.NodeIndex[recipes]; !ok {
		t.Errorf("satellite %v missing from result", recipes)
	} else if len(rs.Nodes[idx].Contexts) == 0 || rs.Nodes[idx].Contexts[0] != "Cooking" {
		t.Errorf("satellite %v primary context = %v, want [Cooking]", recipes, rs.Nodes[idx].Contexts)
	}
}

// TestBuildResultSet_FromListingsTier_Contexts verifies the dual `\context any`
// view: each context tag is a hub and the chapters using it are satellites.
func TestBuildResultSet_FromListingsTier_Contexts(t *testing.T) {
	sr := &search.SearchResult{
		Listings: []search.ListingItem{
			{Kind: "context", Title: "recipes", Chapters: []string{"Cooking", "Baking"}},
		},
	}
	rs := BuildResultSet(sr, nil, `\context any`)

	if len(rs.Nodes) != 3 { // recipes hub + Cooking + Baking
		t.Fatalf("nodes: got %d, want 3: %+v", len(rs.Nodes), rs.Nodes)
	}
	if len(rs.Edges) != 2 {
		t.Fatalf("edges: got %d, want 2", len(rs.Edges))
	}
	hub := nodeKey{Text: "recipes", Chapter: ""}
	if !rs.SeedKeys[hub] {
		t.Errorf("expected context hub %v to be a seed", hub)
	}
}
