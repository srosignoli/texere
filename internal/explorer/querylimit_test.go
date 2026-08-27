package explorer

import "testing"

// withDefaultLimit must inject the explorer's default \limit only when the
// query carries no explicit cap, so the explorer matches the OmniBar graph
// search's breadth. A user-supplied \limit (or any of its aliases) is
// preserved verbatim.
func TestWithDefaultLimit(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no limit short term", "eat \\chapter any", "\\limit 100 eat \\chapter any"},
		{"plain term", "fish", "\\limit 100 fish"},
		{"explicit limit preserved", "fish \\limit 5", "fish \\limit 5"},
		{"limit alias depth preserved", "\\from a \\to b \\depth 3", "\\from a \\to b \\depth 3"},
		{"limit alias range preserved", "fish \\range 7", "fish \\range 7"},
		{"limit alias distance preserved", "fish \\distance 2", "fish \\distance 2"},
		{"case-insensitive limit preserved", "fish \\LIMIT 9", "fish \\LIMIT 9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withDefaultLimit(tc.in); got != tc.want {
				t.Errorf("withDefaultLimit(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// collectClusterChapters returns the distinct, sorted parent chapters across
// a layer's members — splitting comma-joined Chapter fields and dropping
// blanks — so a context grid spanning multiple chapters can be labelled.
func TestCollectClusterChapters(t *testing.T) {
	rs := &ResultSet{NodeIndex: map[nodeKey]int{}}
	add := func(text, chapter string) nodeKey {
		k := nodeKey{Text: text, Chapter: chapter}
		rs.Nodes = append(rs.Nodes, ResultNode{Key: k, Text: text, Chapter: chapter})
		rs.NodeIndex[k] = len(rs.Nodes) - 1
		return k
	}
	a := add("n1", "Cooking")
	b := add("n2", "Travel")
	c := add("n3", "Cooking")             // duplicate chapter
	d := add("n4", "Restaurants, Travel") // comma-joined multi-chapter node
	e := add("n5", "")                    // blank chapter ignored

	got := collectClusterChapters(rs, []nodeKey{a, b, c, d, e})
	want := []string{"Cooking", "Restaurants", "Travel"}
	if len(got) != len(want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v; want %v (sorted, deduped)", got, want)
		}
	}
}

// formatChapters caps the rendered chapter list and appends "+N more".
func TestFormatChaptersIsCappedInFrontend(t *testing.T) {
	// formatChapters lives in the frontend (LayerPlanes.js); this test only
	// guards the backend contract that all distinct chapters are emitted
	// (the cap is a presentation concern). Asserting the full set here keeps
	// the backend honest if the cap ever moves server-side.
	rs := &ResultSet{NodeIndex: map[nodeKey]int{}}
	keys := make([]nodeKey, 0, 5)
	for _, ch := range []string{"E", "D", "C", "B", "A"} {
		k := nodeKey{Text: "n-" + ch, Chapter: ch}
		rs.Nodes = append(rs.Nodes, ResultNode{Key: k, Text: k.Text, Chapter: ch})
		rs.NodeIndex[k] = len(rs.Nodes) - 1
		keys = append(keys, k)
	}
	got := collectClusterChapters(rs, keys)
	if len(got) != 5 || got[0] != "A" || got[4] != "E" {
		t.Fatalf("got %v; want all five chapters sorted A..E", got)
	}
}
