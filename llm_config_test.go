package main

import (
	"os"
	"strings"
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

func TestResolveKeyOptional(t *testing.T) {
	if k, err := resolveKeyOptional(LLMProfile{}); err != nil || k != "" {
		t.Errorf("keyless: got %q err %v, want \"\" nil", k, err)
	}
	if k, err := resolveKeyOptional(LLMProfile{APIKey: "sk-1"}); err != nil || k != "sk-1" {
		t.Errorf("literal: got %q err %v", k, err)
	}
	os.Setenv("TEXERE_TEST_STT_KEY", "envval")
	defer os.Unsetenv("TEXERE_TEST_STT_KEY")
	if k, err := resolveKeyOptional(LLMProfile{APIKeyEnv: "TEXERE_TEST_STT_KEY"}); err != nil || k != "envval" {
		t.Errorf("env: got %q err %v", k, err)
	}
	if _, err := resolveKeyOptional(LLMProfile{APIKeyEnv: "TEXERE_TEST_STT_MISSING"}); err == nil {
		t.Error("expected error for empty named env var")
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

// A key pasted into the "API key env var" field must never come back out in an
// error. That field sits next to the one for the key itself, the two are easy
// to confuse, and the message is rendered in the cell — so echoing it puts a
// live secret on screen and into any screenshot of it.
func TestEnvKeyErrorNeverEchoesAPastedKey(t *testing.T) {
	const pasted = "sk-proj-AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH_IJKL"

	for _, fn := range []struct {
		name string
		call func(LLMProfile) (string, error)
	}{
		{"resolveKey", resolveKey},
		{"resolveKeyOptional", resolveKeyOptional},
	} {
		_, err := fn.call(LLMProfile{APIKeyEnv: pasted})
		if err == nil {
			t.Fatalf("%s: expected an error for an unset variable", fn.name)
		}
		if strings.Contains(err.Error(), pasted) {
			t.Fatalf("%s: the error echoed the pasted key: %v", fn.name, err)
		}
		// It should also say something the user can act on.
		if !strings.Contains(err.Error(), "API key") {
			t.Fatalf("%s: the error does not point at the right field: %v", fn.name, err)
		}
	}
}

// The ordinary case keeps its detail: naming the variable is what tells someone
// which one to export.
func TestEnvKeyErrorNamesARealVariable(t *testing.T) {
	_, err := resolveKey(LLMProfile{APIKeyEnv: "TEXERE_TEST_UNSET_KEY"})
	if err == nil || !strings.Contains(err.Error(), "TEXERE_TEST_UNSET_KEY") {
		t.Fatalf("a real variable name should be named: %v", err)
	}
}

func TestLooksLikeEnvVarName(t *testing.T) {
	yes := []string{"OPENAI_API_KEY", "_private", "A", "a1", "MIXED_case_9"}
	no := []string{
		"",
		"sk-proj-abc",                   // hyphens: the shape of a real key
		"sk_proj abc",                   // spaces
		"9LEADING_DIGIT",                // must not start with a digit
		strings.Repeat("A", 65),         // implausibly long for a name
		"sk-" + strings.Repeat("x", 40), // a key by any other name
	}
	for _, s := range yes {
		if !looksLikeEnvVarName(s) {
			t.Errorf("%q should look like a variable name", s)
		}
	}
	for _, s := range no {
		if looksLikeEnvVarName(s) {
			t.Errorf("%q should NOT look like a variable name", s)
		}
	}
}
