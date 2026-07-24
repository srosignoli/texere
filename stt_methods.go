package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"texere/internal/stt"
)

// GetSTTConfig returns the speech-to-text provider config.
func (a *App) GetSTTConfig() LLMConfig {
	cfg, _ := loadConfig()
	return cfg.STT
}

// SaveSTTProfile inserts or updates an STT profile (kind is always
// openai-compatible) and persists the config.
func (a *App) SaveSTTProfile(p LLMProfile) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	p.Kind = "openai-compatible"
	upsertProfile(&cfg.STT, p)
	return saveConfig(cfg)
}

// DeleteSTTProfile removes an STT profile by ID.
func (a *App) DeleteSTTProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	deleteProfile(&cfg.STT, id)
	return saveConfig(cfg)
}

// SetActiveSTTProfile sets the active default STT profile.
func (a *App) SetActiveSTTProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if _, ok := findProfile(&cfg.STT, id); !ok {
		return errors.New("profile not found")
	}
	cfg.STT.ActiveProfileID = id
	return saveConfig(cfg)
}

// TranscribeAudio reads the stored asset and transcribes it via the given STT
// profile (or the active default when profileID==""), returning the text.
func (a *App) TranscribeAudio(profileID, asset string) (string, error) {
	cfg, err := loadConfig()
	if err != nil {
		return "", err
	}
	id := profileID
	if id == "" {
		id = cfg.STT.ActiveProfileID
	}
	p, ok := findProfile(&cfg.STT, id)
	if !ok {
		return "", errors.New("no speech-to-text profile configured")
	}
	key, err := resolveKeyOptional(p)
	if err != nil {
		return "", err
	}
	dir := a.mediaDir()
	if dir == "" {
		return "", errors.New("no collection open")
	}
	name := filepath.Base(asset) // guard against traversal
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	return stt.Transcribe(context.Background(), p.BaseURL, key, p.Model, data, name)
}
