package main

import "testing"

// TestRevealCommand pins the per-platform mapping RevealSkillsDir relies on.
// Actually launching a file manager is not tested here (there is none in
// CI); only the platform decision is.
func TestRevealCommand(t *testing.T) {
	const dir = "/some/dir"
	cases := []struct {
		goos     string
		wantName string
		wantArgs []string
	}{
		{"linux", "xdg-open", []string{dir}},
		{"windows", "explorer", []string{dir}},
		{"darwin", "open", []string{dir}},
		{"freebsd", "xdg-open", []string{dir}}, // unrecognised GOOS falls back to xdg-open
	}
	for _, c := range cases {
		t.Run(c.goos, func(t *testing.T) {
			name, args := revealCommand(c.goos, dir)
			if name != c.wantName {
				t.Errorf("revealCommand(%q, ...) name = %q, want %q", c.goos, name, c.wantName)
			}
			if len(args) != len(c.wantArgs) {
				t.Fatalf("revealCommand(%q, ...) args = %v, want %v", c.goos, args, c.wantArgs)
			}
			for i := range args {
				if args[i] != c.wantArgs[i] {
					t.Errorf("revealCommand(%q, ...) args = %v, want %v", c.goos, args, c.wantArgs)
				}
			}
		})
	}
}
