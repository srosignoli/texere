package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// makeRecipe returns the command lines of a make target.
func makeRecipe(t *testing.T, target string) []string {
	t.Helper()
	b, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	var out []string
	inTarget := false
	for _, line := range strings.Split(string(b), "\n") {
		if regexp.MustCompile(`^[a-zA-Z0-9_-]+:`).MatchString(line) {
			inTarget = strings.HasPrefix(line, target+":")
			continue
		}
		if inTarget && strings.HasPrefix(line, "\t") {
			out = append(out, strings.TrimSpace(line))
		}
	}
	if len(out) == 0 {
		t.Fatalf("no recipe found for target %q", target)
	}
	return out
}

// -devtools enables the WebKit inspector, so a release binary built with it
// lets anyone right-click into devtools on the shipped app. The release targets
// must not carry it; build-dev exists for local debugging instead.
func TestReleaseBuildTargetsCarryNoDevtools(t *testing.T) {
	for _, target := range []string{"build", "build-windows", "build-macos"} {
		for _, line := range makeRecipe(t, target) {
			if strings.Contains(line, "-devtools") {
				t.Errorf("release target %q carries -devtools:\n  %s", target, line)
			}
		}
	}
}

// The demo seam must never be compiled into a release binary.
func TestReleaseBuildTargetsCarryNoDemoTag(t *testing.T) {
	for _, target := range []string{"build", "build-windows", "build-macos"} {
		for _, line := range makeRecipe(t, target) {
			if regexp.MustCompile(`-tags[= ][^ ]*\bdemo\b`).MatchString(line) {
				t.Errorf("release target %q builds with the demo tag:\n  %s", target, line)
			}
		}
	}
}
