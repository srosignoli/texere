//go:build !demo

package main

import (
	"os"
	"strings"
	"testing"
)

// Running `wails dev -tags webkit2_41,demo` regenerates the checked-in JS
// bindings in frontend/wailsjs/ and adds the Demo* shims to them. Those files
// are tracked, so an unnoticed `git add` would ship the seam's shims to
// customers and pollute the generated bindings.
//
// Regenerate with a production build (or `git checkout frontend/wailsjs`) after
// any demo-tagged dev run.
func TestGeneratedBindingsCarryNoDemoShims(t *testing.T) {
	for _, f := range []string{
		"frontend/wailsjs/go/main/App.js",
		"frontend/wailsjs/go/main/App.d.ts",
	} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, "Demo") {
				t.Errorf("%s carries a demo shim — regenerate without -tags demo:\n  %s",
					f, strings.TrimSpace(line))
			}
		}
	}
}
