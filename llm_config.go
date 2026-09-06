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

// looksLikeEnvVarName reports whether s is shaped like an environment variable
// name: a letter or underscore, then letters, digits and underscores.
//
// It exists because APIKeyEnv is a field users paste SECRETS into. It is meant
// to hold the NAME of a variable, sits next to the field for the key itself,
// and the two are easy to confuse — so anything that echoes its contents back
// can put a live API key on screen, into a screenshot, and into a bug report.
// A real key fails this test: "sk-proj-..." has hyphens.
func looksLikeEnvVarName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// envKeyError explains an unusable APIKeyEnv without ever echoing a value that
// is not plainly a variable name. Naming the variable is genuinely useful when
// the field is used as intended — it tells you what to export — so that case
// keeps its detail.
func envKeyError(name string) error {
	if looksLikeEnvVarName(name) {
		return fmt.Errorf("env var %q is empty", name)
	}
	return errors.New(`this profile's "API key env var" is not a variable name — ` +
		`if you meant to paste the key itself, put it in the "API key" field instead`)
}

// resolveKey returns the profile's secret: the env var if APIKeyEnv is set,
// else the literal APIKey. Never logs or returns the value in an error — see
// envKeyError for the case where APIKeyEnv is itself a pasted key.
func resolveKey(p LLMProfile) (string, error) {
	if p.APIKeyEnv != "" {
		v := os.Getenv(p.APIKeyEnv)
		if v == "" {
			return "", envKeyError(p.APIKeyEnv)
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
			return "", envKeyError(p.APIKeyEnv)
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
