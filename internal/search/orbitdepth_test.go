package search

import "testing"

// extractOrbitDepth must pull a \radius N directive out of the query, clamp it
// to [1, MaxOrbitDepth], and strip it so SST.DecodeSearchField never sees the
// "radius" token as a search term. Absent → DefaultOrbitDepth, query intact.
func TestExtractOrbitDepth(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantDepth int
		wantQuery string
	}{
		{"absent", "eat \\chapter any", DefaultOrbitDepth, "eat \\chapter any"},
		{"explicit 1", "eat \\radius 1", 1, "eat"},
		{"explicit 3", "eat \\radius 3", 3, "eat"},
		{"clamp high", "eat \\radius 9", MaxOrbitDepth, "eat"},
		{"clamp zero", "eat \\radius 0", 1, "eat"},
		{"case insensitive", "eat \\RADIUS 2", 2, "eat"},
		{"leading directive", "\\radius 2 eat", 2, "eat"},
		{"middle directive keeps rest", "eat \\radius 2 \\chapter food", 2, "eat \\chapter food"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotDepth, gotQuery := extractOrbitDepth(tc.in)
			if gotDepth != tc.wantDepth {
				t.Errorf("depth: got %d, want %d", gotDepth, tc.wantDepth)
			}
			if gotQuery != tc.wantQuery {
				t.Errorf("query: got %q, want %q", gotQuery, tc.wantQuery)
			}
		})
	}
}

// A bare word that merely contains "radius" must NOT be treated as the
// directive (the regex requires a backslash and a following number).
func TestExtractOrbitDepth_NotADirective(t *testing.T) {
	for _, q := range []string{"radius of curvature", "\\radiused 2", "gradius 3"} {
		d, out := extractOrbitDepth(q)
		if d != DefaultOrbitDepth || out != q {
			t.Errorf("%q: got depth=%d query=%q; want default + unchanged", q, d, out)
		}
	}
}
