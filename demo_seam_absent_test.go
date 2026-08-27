//go:build !demo

package main

import (
	"reflect"
	"strings"
	"testing"
)

// The demo seam bypasses the file dialog, so an exported Demo* method is a
// binding that lets any script in the webview read any file the user can read.
// Production builds must not contain one. Wails generates JS bindings from the
// exported methods of App, so this asserts on exactly what it reflects over.
//
// This test fails if demo_seam.go ever loses its //go:build demo tag, or if a
// new demo-only binding is added without one.
func TestNoDemoBindingsInAProductionBuild(t *testing.T) {
	appType := reflect.TypeOf(&App{})
	for i := 0; i < appType.NumMethod(); i++ {
		name := appType.Method(i).Name
		if strings.HasPrefix(name, "Demo") {
			t.Errorf("exported method %q present in a build without -tags demo; "+
				"it must live in a file tagged //go:build demo", name)
		}
	}
}
