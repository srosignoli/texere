package search

import (
	"testing"

	SST "github.com/srosignoli/sstorytime-kv/pkg/SSTorytime"
)

// ── normalizeForSearch ────────────────────────────────────────────────────────

func TestNormalizeForSearch(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"fángzi", "fangzi"},
		{"(fangzi)", "fangzi"},
		{"  FÁNGZI  ", "fangzi"},
		{"Chinese", "chinese"},
		{"", ""},
		{"nǐ hǎo", "ni hao"},
		{"房子", "房子"}, // CJK unchanged
	}
	for _, tc := range cases {
		got := normalizeForSearch(tc.input)
		if got != tc.want {
			t.Errorf("normalizeForSearch(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

// ── classifyQuery ─────────────────────────────────────────────────────────────

func boolToPageNr(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestClassifyQuery(t *testing.T) {
	svc := &SearchService{}

	cases := []struct {
		name   string
		params SST.SearchParameters
		want   queryRoute
	}{
		{
			name:   "plain name → nodeSearch",
			params: SST.SearchParameters{Name: []string{"fangzi"}},
			want:   routeNodeSearch,
		},
		{
			name:   "name + chapter → nodeSearch",
			params: SST.SearchParameters{Name: []string{"fangzi"}, Chapter: "chinese"},
			want:   routeNodeSearch,
		},
		{
			name:   "from + to → structural",
			params: SST.SearchParameters{From: []string{"house"}, To: []string{"fangzi"}},
			want:   routeStructural,
		},
		{
			name:   "from only (forward cone) → structural",
			params: SST.SearchParameters{From: []string{"start"}},
			want:   routeStructural,
		},
		{
			name:   "to only (backward cone) → structural",
			params: SST.SearchParameters{To: []string{"target"}},
			want:   routeStructural,
		},
		{
			name:   "from + arrows, no name → structural (not listing)",
			params: SST.SearchParameters{From: []string{"start"}, Arrows: []string{"lt"}},
			want:   routeStructural,
		},
		{
			name:   "sequence → structural",
			params: SST.SearchParameters{Sequence: true},
			want:   routeStructural,
		},
		{
			name:   "chapter only → listing",
			params: SST.SearchParameters{Chapter: "chinese"},
			want:   routeListing,
		},
		{
			name:   "arrows without name → listing",
			params: SST.SearchParameters{Arrows: []string{"hp"}},
			want:   routeListing,
		},
		{
			name:   "pagenr → notes",
			params: SST.SearchParameters{PageNr: 1},
			want:   routeNotes,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.classifyQuery(tc.params)
			if got != tc.want {
				t.Errorf("got %v; want %v", got, tc.want)
			}
		})
	}
}
