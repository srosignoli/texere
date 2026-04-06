package main

import "testing"

// TestAppCreation verifies the App struct can be instantiated and its zero
// value is valid. This is the scaffold-level smoke test confirming the
// application package compiles and the NewApp constructor returns a non-nil value.
func TestAppCreation(t *testing.T) {
	app := NewApp()
	if app == nil {
		t.Fatal("NewApp() returned nil")
	}
}
