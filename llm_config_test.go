package main

import (
	"os"
	"testing"
)

func TestResolveKey(t *testing.T) {
	if _, err := resolveKey(LLMProfile{}); err == nil {
		t.Fatal("empty profile should error")
	}
	if k, err := resolveKey(LLMProfile{APIKey: "lit"}); err != nil || k != "lit" {
		t.Fatalf("literal: %q %v", k, err)
	}
	os.Setenv("TEXERE_TEST_KEY", "envval")
	defer os.Unsetenv("TEXERE_TEST_KEY")
	if k, err := resolveKey(LLMProfile{APIKeyEnv: "TEXERE_TEST_KEY", APIKey: "ignored"}); err != nil || k != "envval" {
		t.Fatalf("env precedence: %q %v", k, err)
	}
	if _, err := resolveKey(LLMProfile{APIKeyEnv: "TEXERE_MISSING_ENV"}); err == nil {
		t.Fatal("missing env should error")
	}
}

func TestProfileCRUD(t *testing.T) {
	cfg := &LLMConfig{}
	upsertProfile(cfg, LLMProfile{ID: "a", Name: "A"})
	if len(cfg.Profiles) != 1 || cfg.ActiveProfileID != "a" {
		t.Fatalf("first upsert should set active: %+v", cfg)
	}
	upsertProfile(cfg, LLMProfile{ID: "b", Name: "B"})
	upsertProfile(cfg, LLMProfile{ID: "a", Name: "A2"}) // update, not append
	if len(cfg.Profiles) != 2 {
		t.Fatalf("update should not append: %+v", cfg.Profiles)
	}
	if p, ok := findProfile(cfg, "a"); !ok || p.Name != "A2" {
		t.Fatalf("update lost: %+v", p)
	}
	deleteProfile(cfg, "a") // active was "a" → falls back to "b"
	if _, ok := findProfile(cfg, "a"); ok {
		t.Fatal("delete failed")
	}
	if cfg.ActiveProfileID != "b" {
		t.Fatalf("active should fall back to b, got %q", cfg.ActiveProfileID)
	}
}
