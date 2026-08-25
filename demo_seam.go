//go:build demo

// Demo-only bindings for scripted, unattended demo recordings.
//
// The four importers open a native file dialog owned by the Go process. A
// browser-driven script (see docs/demos/) cannot see or dismiss that dialog, so
// a scripted run would block on the very first step of a study session.
//
// This file exposes the ingest half directly, keyed to an explicit path. It is
// compiled ONLY under -tags demo:
//
//	wails dev   -tags webkit2_41,demo   # seam present
//	wails build -tags webkit2_41        # seam absent from the binary
//
// Production builds never include this file, so Wails generates no JS binding
// for these methods and nothing in the webview can call them. That absence is
// enforced by TestNoDemoBindingsInAProductionBuild in demo_seam_absent_test.go.
//
// Do not add a runtime flag or environment variable as an alternative gate: a
// dormant-but-present binding is still a binding that lets any script in the
// webview read any file the user can read.
//
// CAUTION: a demo-tagged `wails dev` regenerates the tracked JS bindings under
// frontend/wailsjs/ and adds Demo* shims to them. Run
// `git checkout -- frontend/wailsjs/` afterwards;
// TestGeneratedBindingsCarryNoDemoShims fails if you forget.
package main

// DemoImportTextSource ingests the text/HTML file at path, skipping the dialog
// that ImportTextSource would open.
func (a *App) DemoImportTextSource(path string) (*ReadingView, error) {
	return a.importTextFrom(path)
}

// DemoImportPdfSource ingests the PDF at path, skipping the dialog that
// ImportPdfSource would open.
func (a *App) DemoImportPdfSource(path string) (*ReadingView, error) {
	return a.importPdfFrom(path)
}
