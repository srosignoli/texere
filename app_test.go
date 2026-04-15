package main

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/content"
)

// TestAppCreation verifies the App struct can be instantiated and its zero
// value is valid. This is the scaffold-level smoke test confirming the
// application package compiles and the NewApp constructor returns a non-nil value.
func TestAppCreation(t *testing.T) {
	app := NewApp()
	if app == nil {
		t.Fatal("NewApp() returned nil")
	}
}

// TestLoadSaveConfigRoundTrip verifies that saveConfig/loadConfig preserve all
// fields without corruption.
func TestLoadSaveConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	// Override config path by setting XDG_CONFIG_HOME so os.UserConfigDir picks it up
	t.Setenv("XDG_CONFIG_HOME", dir)

	original := &AppConfig{
		CollectionDir: "/tmp/my-collection",
		RecentArrows:  []string{"means", "e.g.", "note"},
	}
	if err := saveConfig(original); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if loaded.CollectionDir != original.CollectionDir {
		t.Errorf("CollectionDir mismatch: got %q, want %q", loaded.CollectionDir, original.CollectionDir)
	}
	if len(loaded.RecentArrows) != len(original.RecentArrows) {
		t.Errorf("RecentArrows length mismatch: got %d, want %d", len(loaded.RecentArrows), len(original.RecentArrows))
	}
	for i, code := range original.RecentArrows {
		if loaded.RecentArrows[i] != code {
			t.Errorf("RecentArrows[%d]: got %q, want %q", i, loaded.RecentArrows[i], code)
		}
	}
}

// TestLoadConfigMissingFile verifies loadConfig returns an empty AppConfig (not
// an error) when no config file exists yet (first-run scenario).
func TestLoadConfigMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig on missing file returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("loadConfig returned nil cfg")
	}
	if cfg.CollectionDir != "" {
		t.Errorf("expected empty CollectionDir, got %q", cfg.CollectionDir)
	}
}

// TestGetArrowRegistryDegradedMode verifies GetArrowRegistry returns an empty
// (non-nil) slice when the SSTRegistry is nil or empty.
func TestGetArrowRegistryDegradedMode(t *testing.T) {
	app := NewApp()
	// registry is nil at zero-value — degraded mode
	defs, err := app.GetArrowRegistry()
	if err != nil {
		t.Fatalf("GetArrowRegistry degraded: %v", err)
	}
	if defs == nil {
		t.Fatal("expected non-nil slice in degraded mode")
	}
	if len(defs) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(defs))
	}
}

// TestGetArrowRegistrySorted verifies that GetArrowRegistry returns defs sorted
// by MetaType then Code when a registry is loaded.
func TestGetArrowRegistrySorted(t *testing.T) {
	app := NewApp()
	app.registry = &content.SSTRegistry{
		Arrows: map[string]content.ArrowDef{
			"z":    {Code: "z", MetaType: "B"},
			"a":    {Code: "a", MetaType: "B"},
			"note": {Code: "note", MetaType: "A"},
		},
	}
	defs, err := app.GetArrowRegistry()
	if err != nil {
		t.Fatalf("GetArrowRegistry: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("expected 3 defs, got %d", len(defs))
	}
	// Expected order: note (A), a (B), z (B)
	expected := []string{"note", "a", "z"}
	for i, want := range expected {
		if defs[i].Code != want {
			t.Errorf("defs[%d].Code = %q, want %q", i, defs[i].Code, want)
		}
	}
}

// TestSaveFileInjectsID verifies SaveFile writes content and injects a # @id
// line for a node that has a # @card marker but no existing # @id.
func TestSaveFileInjectsID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.n4l")

	initial := "- Chapter\n# @card\nSome node\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	app := NewApp()
	// Load a minimal registry (degraded is fine — we just want ID injection)
	reg, _ := content.LoadRegistry(filepath.Join(".", "nonexistent-sstconfig"))
	app.registry = reg

	result, err := app.SaveFile(path, initial)
	if err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	if result == nil {
		t.Fatal("SaveFile returned nil result")
	}
	if len(result.Injections) == 0 {
		t.Error("expected at least one ID injection")
	}
	// FinalContent should contain the injected # @id line
	if result.FinalContent == initial {
		t.Error("FinalContent should differ from input after ID injection")
	}
	for _, inj := range result.Injections {
		if inj.AlreadyHad {
			continue
		}
		if inj.ShortID == "" {
			t.Error("injected ShortID is empty")
		}
	}
}
