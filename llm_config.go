package main

import (
	"errors"
	"fmt"
	"os"
)

// LLMProfile is one configured provider: a name, a wire "kind", endpoint,
// model, and either a literal API key or the name of an env var holding it.
type LLMProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "openai-compatible" | "anthropic"
	BaseURL   string `json:"baseUrl"`
	Model     string `json:"model"`
	APIKey    string `json:"apiKey,omitempty"`
	APIKeyEnv string `json:"apiKeyEnv,omitempty"`
}

// LLMConfig is the global set of profiles plus the active default.
type LLMConfig struct {
	Profiles        []LLMProfile `json:"profiles"`
	ActiveProfileID string       `json:"activeProfileId"`
}

// resolveKey returns the profile's secret: the env var if APIKeyEnv is set,
// else the literal APIKey. Never logs the value.
func resolveKey(p LLMProfile) (string, error) {
	if p.APIKeyEnv != "" {
		v := os.Getenv(p.APIKeyEnv)
		if v == "" {
			return "", fmt.Errorf("env var %q is empty", p.APIKeyEnv)
		}
		return v, nil
	}
	if p.APIKey == "" {
		return "", errors.New("no API key set for profile")
	}
	return p.APIKey, nil
}

// resolveKeyOptional is like resolveKey but permits a keyless profile: it returns
// "" and no error when neither APIKey nor APIKeyEnv is set. Used by STT, whose
// local endpoints (e.g. a whisper server) need no auth.
func resolveKeyOptional(p LLMProfile) (string, error) {
	if p.APIKeyEnv != "" {
		v := os.Getenv(p.APIKeyEnv)
		if v == "" {
			return "", fmt.Errorf("env var %q is empty", p.APIKeyEnv)
		}
		return v, nil
	}
	return p.APIKey, nil // "" is allowed
}

func findProfile(cfg *LLMConfig, id string) (LLMProfile, bool) {
	for _, p := range cfg.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return LLMProfile{}, false
}

// upsertProfile replaces a profile with the same ID, or appends it. The first
// profile added becomes the active default.
func upsertProfile(cfg *LLMConfig, p LLMProfile) {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].ID == p.ID {
			cfg.Profiles[i] = p
			return
		}
	}
	cfg.Profiles = append(cfg.Profiles, p)
	if cfg.ActiveProfileID == "" {
		cfg.ActiveProfileID = p.ID
	}
}

// deleteProfile removes a profile by ID; if it was active, the active default
// falls back to the first remaining profile (or "").
func deleteProfile(cfg *LLMConfig, id string) {
	out := cfg.Profiles[:0]
	for _, p := range cfg.Profiles {
		if p.ID != id {
			out = append(out, p)
		}
	}
	cfg.Profiles = out
	if cfg.ActiveProfileID == id {
		cfg.ActiveProfileID = ""
		if len(cfg.Profiles) > 0 {
			cfg.ActiveProfileID = cfg.Profiles[0].ID
		}
	}
}
